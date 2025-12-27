package monitoring

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (rt roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

func newTestHTTPClient(statusCode int, handler func(*http.Request, []byte)) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			_ = req.Body.Close()

			if handler != nil {
				handler(req, body)
			}

			return &http.Response{
				StatusCode: statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(http.NoBody),
				Request:    req,
			}, nil
		}),
	}
}

func TestAlertRateLimiter_ShouldAlert(t *testing.T) {
	rl := NewAlertRateLimiter(50 * time.Millisecond)
	require.True(t, rl.ShouldAlert("k"))
	require.False(t, rl.ShouldAlert("k"))

	rl.mu.Lock()
	rl.alerts["k"] = time.Now().Add(-time.Second)
	rl.mu.Unlock()

	require.True(t, rl.ShouldAlert("k"))
}

func TestAlertRateLimiter_ShouldAlert_ThresholdBehavior(t *testing.T) {
	threshold := 5 * time.Minute
	rl := NewAlertRateLimiter(threshold)

	// First alert should pass
	require.True(t, rl.ShouldAlert("test-key"))

	// Immediately after, should be rate-limited
	require.False(t, rl.ShouldAlert("test-key"))

	// Manipulate the timestamp to simulate time passing (no sleeps!)
	rl.mu.Lock()
	rl.alerts["test-key"] = time.Now().Add(-threshold - time.Second)
	rl.mu.Unlock()

	// Now it should pass again
	require.True(t, rl.ShouldAlert("test-key"))
}

func TestAlertRateLimiter_DifferentKeys(t *testing.T) {
	rl := NewAlertRateLimiter(1 * time.Hour)

	require.True(t, rl.ShouldAlert("key-a"))
	require.True(t, rl.ShouldAlert("key-b"))
	require.True(t, rl.ShouldAlert("key-c"))

	// Same keys should be rate-limited
	require.False(t, rl.ShouldAlert("key-a"))
	require.False(t, rl.ShouldAlert("key-b"))
}

func TestParseInt(t *testing.T) {
	require.Equal(t, 123, parseInt("123"))
	require.Equal(t, 0, parseInt("not-a-number"))
}

func TestGetEvaluationWindow_Default(t *testing.T) {
	require.Equal(t, 5, getEvaluationWindow("unknown"))
}

// =============================================================================
// SendAlert Tests
// =============================================================================

func TestSendAlert_Disabled(t *testing.T) {
	logger := zap.NewNop()
	am := &AlertManager{
		logger:      logger,
		enabled:     false,
		rateLimiter: NewAlertRateLimiter(5 * time.Minute),
	}

	alert := &Alert{
		Type:     AlertTypeErrorRate,
		Severity: SeverityError,
		Title:    "Test Alert",
		Service:  "test-service",
	}

	err := am.SendAlert(context.Background(), alert)
	require.NoError(t, err)
	// No webhook or SNS calls should happen when disabled
}

func TestSendAlert_RateLimited(t *testing.T) {
	calls := 0
	client := newTestHTTPClient(http.StatusOK, func(_ *http.Request, _ []byte) {
		calls++
	})

	logger := zap.NewNop()
	am := &AlertManager{
		logger:     logger,
		enabled:    true,
		httpClient: client,
		webhookConfig: &WebhookConfig{
			URL:     "http://webhook.invalid/alerts",
			Headers: map[string]string{"Content-Type": "application/json"},
			Timeout: 5 * time.Second,
		},
		rateLimiter: NewAlertRateLimiter(5 * time.Minute),
	}

	alert := &Alert{
		Type:     AlertTypeErrorRate,
		Severity: SeverityError,
		Title:    "Test Alert",
		Service:  "test-service",
	}

	// First call should go through
	err := am.SendAlert(context.Background(), alert)
	require.NoError(t, err)
	require.Equal(t, 1, calls)

	// Second call should be rate-limited
	err = am.SendAlert(context.Background(), alert)
	require.NoError(t, err)
	require.Equal(t, 1, calls) // Webhook should NOT be called
}

func TestSendAlert_WebhookSuccess(t *testing.T) {
	var receivedBody []byte
	client := newTestHTTPClient(http.StatusOK, func(_ *http.Request, body []byte) {
		receivedBody = body
	})

	logger := zap.NewNop()
	am := &AlertManager{
		logger:     logger,
		enabled:    true,
		httpClient: client,
		webhookConfig: &WebhookConfig{
			URL:     "http://webhook.invalid/alerts",
			Headers: map[string]string{"Content-Type": "application/json"},
			Timeout: 5 * time.Second,
		},
		rateLimiter: NewAlertRateLimiter(5 * time.Minute),
	}

	alert := &Alert{
		Type:        AlertTypeLatency,
		Severity:    SeverityWarning,
		Title:       "High Latency Detected",
		Description: "P99 latency exceeded threshold",
		Service:     "api-service",
		Metadata:    map[string]interface{}{"latency_ms": 1500},
	}

	err := am.SendAlert(context.Background(), alert)
	require.NoError(t, err)

	// Verify JSON payload
	var received Alert
	err = json.Unmarshal(receivedBody, &received)
	require.NoError(t, err)
	assert.Equal(t, AlertTypeLatency, received.Type)
	assert.Equal(t, SeverityWarning, received.Severity)
	assert.Equal(t, "High Latency Detected", received.Title)
	assert.Equal(t, "api-service", received.Service)
}

func TestSendAlert_WebhookNon2xx(t *testing.T) {
	client := newTestHTTPClient(http.StatusInternalServerError, nil)

	logger := zap.NewNop()
	am := &AlertManager{
		logger:     logger,
		enabled:    true,
		httpClient: client,
		webhookConfig: &WebhookConfig{
			URL:     "http://webhook.invalid/alerts",
			Headers: map[string]string{"Content-Type": "application/json"},
			Timeout: 5 * time.Second,
		},
		rateLimiter: NewAlertRateLimiter(5 * time.Minute),
	}

	alert := &Alert{
		Type:     AlertTypeErrorRate,
		Severity: SeverityError,
		Title:    "Error Alert",
		Service:  "test-service",
	}

	err := am.SendAlert(context.Background(), alert)
	// Should return error when webhook fails and no SNS configured
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-2xx")
}

func TestSendAlert_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header
	client := newTestHTTPClient(http.StatusOK, func(req *http.Request, _ []byte) {
		receivedHeaders = req.Header.Clone()
	})

	logger := zap.NewNop()
	am := &AlertManager{
		logger:     logger,
		enabled:    true,
		httpClient: client,
		webhookConfig: &WebhookConfig{
			URL: "http://webhook.invalid/alerts",
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"X-Custom-Auth": "secret-token",
			},
			Timeout: 5 * time.Second,
		},
		rateLimiter: NewAlertRateLimiter(5 * time.Minute),
	}

	alert := &Alert{Type: AlertTypeHealth, Severity: SeverityInfo, Title: "Test", Service: "svc"}
	err := am.SendAlert(context.Background(), alert)
	require.NoError(t, err)

	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
	assert.Equal(t, "secret-token", receivedHeaders.Get("X-Custom-Auth"))
}

// =============================================================================
// Check* Method Tests (via httptest.Server)
// =============================================================================

// alertCapture is a helper to capture alert payloads
type alertCapture struct {
	mu     sync.Mutex
	alerts []Alert
}

func (ac *alertCapture) handler(req *http.Request, body []byte) {
	var alert Alert
	if err := json.Unmarshal(body, &alert); err != nil {
		return
	}

	ac.mu.Lock()
	ac.alerts = append(ac.alerts, alert)
	ac.mu.Unlock()
}

func (ac *alertCapture) getAlerts() []Alert {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return append([]Alert{}, ac.alerts...)
}

func newTestAlertManager(webhookURL string, client *http.Client) *AlertManager {
	return &AlertManager{
		logger:     zap.NewNop(),
		enabled:    true,
		httpClient: client,
		webhookConfig: &WebhookConfig{
			URL:     webhookURL,
			Headers: map[string]string{"Content-Type": "application/json"},
			Timeout: 5 * time.Second,
		},
		rateLimiter: NewAlertRateLimiter(0), // Disable rate limiting for these tests
	}
}

func TestCheckErrorRate_WebhookPayload(t *testing.T) {
	capture := &alertCapture{}
	client := newTestHTTPClient(http.StatusOK, capture.handler)
	am := newTestAlertManager("http://webhook.invalid/alerts", client)

	// Error rate below all thresholds - no alert
	am.CheckErrorRate(context.Background(), "test-service", 0.5)
	assert.Empty(t, capture.getAlerts())

	// Error rate at P2 threshold (2%)
	am.CheckErrorRate(context.Background(), "test-service2", 2.5)
	alerts := capture.getAlerts()
	require.Len(t, alerts, 1)
	assert.Equal(t, AlertTypeErrorRate, alerts[0].Type)
	assert.Equal(t, SeverityWarning, alerts[0].Severity)
	assert.Equal(t, "P2", alerts[0].Priority)
	assert.Equal(t, "test-service2", alerts[0].Service)
	assert.Contains(t, alerts[0].Title, "High Error Rate")
}

func TestCheckErrorRate_Severities(t *testing.T) {
	tests := []struct {
		name         string
		errorRate    float64
		wantSeverity AlertSeverity
		wantPriority string
		wantAlert    bool
	}{
		{"below threshold", 0.5, "", "", false},
		{"P2 threshold", 2.5, SeverityWarning, "P2", true},
		{"P1 threshold", 6.0, SeverityError, "P1", true},
		{"P0 threshold", 15.0, SeverityCritical, "P0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &alertCapture{}
			client := newTestHTTPClient(http.StatusOK, capture.handler)
			am := newTestAlertManager("http://webhook.invalid/alerts", client)
			am.CheckErrorRate(context.Background(), "svc", tt.errorRate)

			alerts := capture.getAlerts()
			if !tt.wantAlert {
				assert.Empty(t, alerts)
				return
			}

			require.Len(t, alerts, 1)
			assert.Equal(t, tt.wantSeverity, alerts[0].Severity)
			assert.Equal(t, tt.wantPriority, alerts[0].Priority)
		})
	}
}

func TestCheckLatency_WebhookPayload(t *testing.T) {
	capture := &alertCapture{}
	client := newTestHTTPClient(http.StatusOK, capture.handler)
	am := newTestAlertManager("http://webhook.invalid/alerts", client)

	// Below threshold - no alert
	am.CheckLatency(context.Background(), "api-service", 100, "P90")
	assert.Empty(t, capture.getAlerts())

	// P90 at P2 threshold (1000ms)
	am.CheckLatency(context.Background(), "api-service2", 1100, "P90")
	alerts := capture.getAlerts()
	require.Len(t, alerts, 1)
	assert.Equal(t, AlertTypeLatency, alerts[0].Type)
	assert.Equal(t, SeverityWarning, alerts[0].Severity)
	assert.Equal(t, "api-service2", alerts[0].Service)
	assert.Contains(t, alerts[0].Title, "High Latency")
	assert.Contains(t, alerts[0].Title, "P90")
}

func TestCheckLatency_P99Percentile(t *testing.T) {
	capture := &alertCapture{}
	client := newTestHTTPClient(http.StatusOK, capture.handler)
	am := newTestAlertManager("http://webhook.invalid/alerts", client)

	// P99 at critical threshold (5000ms)
	am.CheckLatency(context.Background(), "critical-service", 5500, "P99")

	alerts := capture.getAlerts()
	require.Len(t, alerts, 1)
	assert.Equal(t, SeverityCritical, alerts[0].Severity)
	assert.Equal(t, "P0", alerts[0].Priority)
	assert.Contains(t, alerts[0].Title, "P99")
}

func TestCheckCost_WebhookPayload(t *testing.T) {
	capture := &alertCapture{}
	client := newTestHTTPClient(http.StatusOK, capture.handler)
	am := newTestAlertManager("http://webhook.invalid/alerts", client)

	// Below threshold - no alert
	am.CheckCost(context.Background(), 5000) // $0.05
	assert.Empty(t, capture.getAlerts())

	// At warning threshold ($0.10 = 10000 microcents)
	am.CheckCost(context.Background(), 15000)
	alerts := capture.getAlerts()
	require.Len(t, alerts, 1)
	assert.Equal(t, AlertTypeCost, alerts[0].Type)
	assert.Equal(t, SeverityWarning, alerts[0].Severity)
	assert.Equal(t, "billing", alerts[0].Service)
	assert.Contains(t, alerts[0].Title, "High Cost")
}

func TestCheckCost_Severities(t *testing.T) {
	tests := []struct {
		name         string
		costMicro    float64
		wantSeverity AlertSeverity
		wantAlert    bool
	}{
		{"below threshold", 5000, "", false},                    // $0.05
		{"warning threshold", 15000, SeverityWarning, true},     // $0.15
		{"error threshold", 150000, SeverityError, true},        // $1.50
		{"critical threshold", 1500000, SeverityCritical, true}, // $15
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &alertCapture{}
			client := newTestHTTPClient(http.StatusOK, capture.handler)
			am := newTestAlertManager("http://webhook.invalid/alerts", client)
			am.CheckCost(context.Background(), tt.costMicro)

			alerts := capture.getAlerts()
			if !tt.wantAlert {
				assert.Empty(t, alerts)
				return
			}

			require.Len(t, alerts, 1)
			assert.Equal(t, tt.wantSeverity, alerts[0].Severity)
		})
	}
}

func TestCheckHealth_WebhookPayload(t *testing.T) {
	capture := &alertCapture{}
	client := newTestHTTPClient(http.StatusOK, capture.handler)
	am := newTestAlertManager("http://webhook.invalid/alerts", client)

	// Healthy service - no alert
	am.CheckHealth(context.Background(), "healthy-service", true, "")
	assert.Empty(t, capture.getAlerts())

	// Unhealthy service - alert
	am.CheckHealth(context.Background(), "broken-service", false, "Connection refused")
	alerts := capture.getAlerts()
	require.Len(t, alerts, 1)
	assert.Equal(t, AlertTypeHealth, alerts[0].Type)
	assert.Equal(t, SeverityError, alerts[0].Severity)
	assert.Equal(t, "broken-service", alerts[0].Service)
	assert.Contains(t, alerts[0].Description, "Connection refused")
}

func TestCheckCapacity_WebhookPayload(t *testing.T) {
	capture := &alertCapture{}
	client := newTestHTTPClient(http.StatusOK, capture.handler)
	am := newTestAlertManager("http://webhook.invalid/alerts", client)

	// Below threshold - no alert
	am.CheckCapacity(context.Background(), "CPU", 50.0)
	assert.Empty(t, capture.getAlerts())

	// At warning threshold (75%)
	am.CheckCapacity(context.Background(), "Memory", 80.0)
	alerts := capture.getAlerts()
	require.Len(t, alerts, 1)
	assert.Equal(t, AlertTypeCapacity, alerts[0].Type)
	assert.Equal(t, SeverityWarning, alerts[0].Severity)
	assert.Contains(t, alerts[0].Title, "Memory")
	assert.Contains(t, alerts[0].Title, "80.0%")
}

func TestFormatSNSMessage(t *testing.T) {
	am := &AlertManager{logger: zap.NewNop()}
	alert := &Alert{
		Type:        AlertTypeErrorRate,
		Severity:    SeverityCritical,
		Title:       "High Error Rate",
		Description: "Error rate exceeded 10%",
		Service:     "api-service",
		Region:      "us-east-1",
		Timestamp:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Metadata: map[string]interface{}{
			"error_rate": 12.5,
		},
	}

	message := am.formatSNSMessage(alert)

	assert.Contains(t, message, "High Error Rate")
	assert.Contains(t, message, "error_rate")
	assert.Contains(t, message, "api-service")
	assert.Contains(t, message, "us-east-1")
	assert.Contains(t, message, "critical")
}
