package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// =============================================================================
// Test Helpers: RoundTripper stub (no network)
// =============================================================================

// webhookRTFunc implements http.RoundTripper for testing webhook delivery without network.
type webhookRTFunc func(*http.Request) (*http.Response, error)

func (f webhookRTFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// errorReader simulates a body that errors on Read
type errorReader struct{}

func (e errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read failure")
}

func (e errorReader) Close() error {
	return nil
}

// =============================================================================
// Tests for ValidateWebhookURL
// =============================================================================

func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
		errContains string
	}{
		{
			name:        "empty URL returns error",
			url:         "",
			expectError: true,
			errContains: "cannot be empty",
		},
		{
			name:        "invalid URL parse returns error",
			url:         "://not-a-valid-url",
			expectError: true,
			errContains: "invalid webhook URL",
		},
		{
			name:        "ftp scheme returns error",
			url:         "ftp://example.com/webhook",
			expectError: true,
			errContains: "http or https",
		},
		{
			name:        "file scheme returns error",
			url:         "file:///etc/passwd",
			expectError: true,
			errContains: "http or https",
		},
		{
			name:        "missing scheme returns error",
			url:         "example.com/webhook",
			expectError: true,
			errContains: "http or https",
		},
		{
			name:        "missing host returns error",
			url:         "https:///webhook",
			expectError: true,
			errContains: "must have a host",
		},
		{
			name:        "valid http URL returns nil",
			url:         "http://example.com/webhook",
			expectError: false,
		},
		{
			name:        "valid https URL returns nil",
			url:         "https://example.com/webhook",
			expectError: false,
		},
		{
			name:        "valid https URL with port returns nil",
			url:         "https://example.com:8443/webhook",
			expectError: false,
		},
		{
			name:        "valid https URL with query params returns nil",
			url:         "https://example.com/webhook?token=abc123",
			expectError: false,
		},
		{
			name:        "valid localhost URL returns nil",
			url:         "http://localhost:8080/webhook",
			expectError: true,
			errContains: "blocked",
		},
		{
			name:        "valid IP URL returns nil",
			url:         "http://192.168.1.1/webhook",
			expectError: true,
			errContains: "blocked",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWebhookURL(tc.url)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// =============================================================================
// Tests for generateHMACSignature
// =============================================================================

func TestGenerateHMACSignature(t *testing.T) {
	tests := []struct {
		name           string
		payload        []byte
		secret         string
		expectedPrefix string
		expectSame     bool // When true, same payload+secret gives same output
	}{
		{
			name:           "deterministic output for known payload and secret",
			payload:        []byte(`{"alert_id":"123","type":"test"}`),
			secret:         "my_secret_token",
			expectedPrefix: "sha256=",
			expectSame:     true,
		},
		{
			name:           "empty payload works",
			payload:        []byte(""),
			secret:         "secret",
			expectedPrefix: "sha256=",
			expectSame:     true,
		},
		{
			name:           "empty secret works",
			payload:        []byte("test payload"),
			secret:         "",
			expectedPrefix: "sha256=",
			expectSame:     true,
		},
		{
			name:           "unicode payload works",
			payload:        []byte(`{"message":"Hello, 世界"}`),
			secret:         "unicode_secret_🔑",
			expectedPrefix: "sha256=",
			expectSame:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sig := generateHMACSignature(tc.payload, tc.secret)

			// Verify prefix
			assert.True(t, strings.HasPrefix(sig, tc.expectedPrefix),
				"signature should have %s prefix, got: %s", tc.expectedPrefix, sig)

			// Verify it's a hex string after the prefix
			hexPart := strings.TrimPrefix(sig, tc.expectedPrefix)
			assert.NotEmpty(t, hexPart, "hex part should not be empty")
			// SHA256 produces 64 hex characters
			assert.Len(t, hexPart, 64, "hex part should be 64 characters (SHA256)")

			// Verify deterministic behavior
			if tc.expectSame {
				sig2 := generateHMACSignature(tc.payload, tc.secret)
				assert.Equal(t, sig, sig2, "same payload and secret should produce same signature")
			}
		})
	}

	// Verify different secrets produce different signatures
	t.Run("different secrets produce different signatures", func(t *testing.T) {
		payload := []byte(`{"test":"data"}`)
		sig1 := generateHMACSignature(payload, "secret1")
		sig2 := generateHMACSignature(payload, "secret2")
		assert.NotEqual(t, sig1, sig2)
	})

	// Verify different payloads produce different signatures
	t.Run("different payloads produce different signatures", func(t *testing.T) {
		secret := "shared_secret"
		sig1 := generateHMACSignature([]byte(`{"a":"1"}`), secret)
		sig2 := generateHMACSignature([]byte(`{"a":"2"}`), secret)
		assert.NotEqual(t, sig1, sig2)
	})
}

// =============================================================================
// Tests for prepareWebhookPayload
// =============================================================================

func TestPrepareWebhookPayload(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &WebhookDeliveryService{logger: logger}

	t.Run("includes required fields", func(t *testing.T) {
		firedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
		alert := &models.Alert{
			AlertID:     "alert-123",
			Type:        "error_rate",
			Severity:    "critical",
			Priority:    "P0",
			Status:      "firing",
			Title:       "High Error Rate",
			Description: "Error rate exceeded threshold",
			Message:     "Error rate is at 15%",
			Service:     "api-gateway",
			Region:      "us-east-1",
			Source:      "cloudwatch",
			RunbookURL:  "https://docs.example.com/runbooks/error-rate",
			FiredAt:     firedAt,
			Dimensions:  map[string]string{"env": "prod"},
			Metadata:    map[string]interface{}{"key": "value"},
			Values:      map[string]float64{"current": 15.0},
			Thresholds:  map[string]float64{"max": 10.0},
		}

		payload, err := service.prepareWebhookPayload(alert)
		require.NoError(t, err)
		require.NotEmpty(t, payload)

		// Parse and verify JSON
		var result map[string]interface{}
		err = json.Unmarshal(payload, &result)
		require.NoError(t, err)

		// Check required fields
		assert.Equal(t, "alert-123", result["alert_id"])
		assert.Equal(t, "error_rate", result["type"])
		assert.Equal(t, "critical", result["severity"])
		assert.Equal(t, "P0", result["priority"])
		assert.Equal(t, "firing", result["status"])
		assert.Equal(t, "High Error Rate", result["title"])
		assert.Equal(t, "Error rate exceeded threshold", result["description"])
		assert.Equal(t, "Error rate is at 15%", result["message"])
		assert.Equal(t, "api-gateway", result["service"])
		assert.Equal(t, "us-east-1", result["region"])
		assert.Equal(t, "cloudwatch", result["source"])
		assert.Equal(t, "https://docs.example.com/runbooks/error-rate", result["runbook_url"])
		assert.Equal(t, "2025-01-15T10:30:00Z", result["fired_at"])

		// Verify resolved_at is NOT present (nil ResolvedAt)
		_, hasResolvedAt := result["resolved_at"]
		assert.False(t, hasResolvedAt, "resolved_at should not be present when nil")

		// Check nested fields exist
		assert.NotNil(t, result["dimensions"])
		assert.NotNil(t, result["metadata"])
		assert.NotNil(t, result["values"])
		assert.NotNil(t, result["thresholds"])
	})

	t.Run("includes resolved_at when present", func(t *testing.T) {
		firedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
		resolvedAt := time.Date(2025, 1, 15, 11, 45, 0, 0, time.UTC)
		alert := &models.Alert{
			AlertID:    "alert-456",
			Type:       "latency",
			Severity:   "warning",
			Priority:   "P2",
			Status:     "resolved",
			Title:      "Resolved Alert",
			FiredAt:    firedAt,
			ResolvedAt: &resolvedAt,
		}

		payload, err := service.prepareWebhookPayload(alert)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(payload, &result)
		require.NoError(t, err)

		// Check resolved_at IS present
		resolvedAtStr, hasResolvedAt := result["resolved_at"]
		assert.True(t, hasResolvedAt, "resolved_at should be present")
		assert.Equal(t, "2025-01-15T11:45:00Z", resolvedAtStr)
	})

	t.Run("handles nil maps gracefully", func(t *testing.T) {
		alert := &models.Alert{
			AlertID:    "alert-789",
			Type:       "test",
			Severity:   "info",
			Status:     "firing",
			FiredAt:    time.Now(),
			Dimensions: nil,
			Metadata:   nil,
			Values:     nil,
			Thresholds: nil,
		}

		payload, err := service.prepareWebhookPayload(alert)
		require.NoError(t, err)
		require.NotEmpty(t, payload)

		var result map[string]interface{}
		err = json.Unmarshal(payload, &result)
		require.NoError(t, err)

		// Nil maps should serialize as null
		assert.Nil(t, result["dimensions"])
		assert.Nil(t, result["metadata"])
		assert.Nil(t, result["values"])
		assert.Nil(t, result["thresholds"])
	})

	t.Run("produces valid JSON", func(t *testing.T) {
		alert := &models.Alert{
			AlertID:  "json-test",
			Type:     "test",
			Severity: "info",
			Status:   "firing",
			FiredAt:  time.Now(),
		}

		payload, err := service.prepareWebhookPayload(alert)
		require.NoError(t, err)

		// Verify it's valid JSON by parsing it
		assert.True(t, json.Valid(payload), "payload should be valid JSON")
	})
}

// =============================================================================
// Tests for categorizeHTTPError (on WebhookDeliveryService)
// =============================================================================

func TestWebhookDeliveryService_CategorizeHTTPError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &WebhookDeliveryService{logger: logger}

	tests := []struct {
		name       string
		statusCode int
		expected   string
	}{
		{
			name:       "408 returns timeout",
			statusCode: 408,
			expected:   "timeout",
		},
		{
			name:       "429 returns rate_limit",
			statusCode: 429,
			expected:   "rate_limit",
		},
		{
			name:       "400 returns client_error",
			statusCode: 400,
			expected:   "client_error",
		},
		{
			name:       "401 returns client_error",
			statusCode: 401,
			expected:   "client_error",
		},
		{
			name:       "403 returns client_error",
			statusCode: 403,
			expected:   "client_error",
		},
		{
			name:       "404 returns client_error",
			statusCode: 404,
			expected:   "client_error",
		},
		{
			name:       "422 returns client_error",
			statusCode: 422,
			expected:   "client_error",
		},
		{
			name:       "500 returns server_error",
			statusCode: 500,
			expected:   "server_error",
		},
		{
			name:       "502 returns server_error",
			statusCode: 502,
			expected:   "server_error",
		},
		{
			name:       "503 returns server_error",
			statusCode: 503,
			expected:   "server_error",
		},
		{
			name:       "504 returns server_error",
			statusCode: 504,
			expected:   "server_error",
		},
		{
			name:       "599 returns server_error",
			statusCode: 599,
			expected:   "server_error",
		},
		{
			name:       "200 returns http_error",
			statusCode: 200,
			expected:   "http_error",
		},
		{
			name:       "301 returns http_error",
			statusCode: 301,
			expected:   "http_error",
		},
		{
			name:       "100 returns http_error",
			statusCode: 100,
			expected:   "http_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := service.categorizeHTTPError(tc.statusCode)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// Tests for categorizeError (on WebhookDeliveryService)
// =============================================================================

func TestWebhookDeliveryService_CategorizeError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &WebhookDeliveryService{logger: logger}

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "timeout error returns timeout",
			err:      errors.New("request timeout: dial tcp"),
			expected: ErrorTypeTimeout,
		},
		{
			name:     "connection refused returns connection_refused",
			err:      errors.New("dial tcp: connection refused"),
			expected: "connection_refused",
		},
		{
			name:     "no such host returns dns_error",
			err:      errors.New("lookup example.com: no such host"),
			expected: "dns_error",
		},
		{
			name:     "certificate error returns tls_error",
			err:      errors.New("x509: certificate signed by unknown authority"),
			expected: "tls_error",
		},
		{
			name:     "tls handshake error returns tls_error",
			err:      errors.New("tls handshake failure"),
			expected: "tls_error",
		},
		{
			name:     "unknown error returns network_error",
			err:      errors.New("some random error"),
			expected: "network_error",
		},
		{
			name:     "empty error returns network_error",
			err:      errors.New(""),
			expected: "network_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := service.categorizeError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// Tests for deliverWebhook with RoundTripper stub
// =============================================================================

func TestWebhookDeliveryService_DeliverWebhook_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("2xx success captures response data", func(t *testing.T) {
		var capturedRequest *http.Request
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			capturedRequest = req
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"status":"received"}`)),
				Header:     http.Header{"X-Custom": []string{"value"}},
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		firedAt := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
		alert := &models.Alert{
			AlertID:  "test-alert",
			Type:     "test",
			Severity: "info",
			Status:   "firing",
			FiredAt:  firedAt,
		}

		delivery := &models.WebhookDelivery{
			DeliveryID:  "delivery-1",
			AlertID:     "test-alert",
			WebhookID:   "webhook-1",
			URL:         "https://example.com/webhook",
			Headers:     map[string]string{"Content-Type": "application/json"},
			MaxAttempts: 3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)

		// Verify delivery status
		assert.Equal(t, "success", delivery.Status)
		assert.Equal(t, 200, delivery.ResponseCode)
		assert.Equal(t, `{"status":"received"}`, delivery.ResponseBody)
		assert.NotEmpty(t, delivery.RequestBody)
		assert.NotNil(t, delivery.ResponseHeaders)
		assert.Equal(t, "value", delivery.ResponseHeaders["X-Custom"])
		assert.NotNil(t, delivery.CompletedAt)
		assert.GreaterOrEqual(t, delivery.Duration, int64(0))

		// Verify request was made correctly
		require.NotNil(t, capturedRequest)
		assert.Equal(t, "POST", capturedRequest.Method)
		assert.Equal(t, "application/json", capturedRequest.Header.Get("Content-Type"))
	})

	t.Run("201 is treated as success", func(t *testing.T) {
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 201,
				Body:       io.NopCloser(strings.NewReader("created")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			MaxAttempts: 3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)
		assert.Equal(t, "success", delivery.Status)
		assert.Equal(t, 201, delivery.ResponseCode)
	})

	t.Run("202 is treated as success", func(t *testing.T) {
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 202,
				Body:       io.NopCloser(strings.NewReader("accepted")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			MaxAttempts: 3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)
		assert.Equal(t, "success", delivery.Status)
		assert.Equal(t, 202, delivery.ResponseCode)
	})
}

func TestWebhookDeliveryService_CreateDelivery_TLSVerificationPolicy(t *testing.T) {
	logger := zaptest.NewLogger(t)

	svc := &WebhookDeliveryService{logger: logger}
	alert := &models.Alert{AlertID: "a1"}
	webhook := &WebhookConfig{
		ID:            "w1",
		URL:           "https://example.com/webhook",
		Headers:       map[string]string{"Content-Type": "application/json"},
		Timeout:       time.Second,
		MaxAttempts:   1,
		RetryInterval: time.Second,
		VerifySSL:     false,
		Enabled:       true,
	}

	t.Run("blocks insecure TLS without override env", func(t *testing.T) {
		delivery := svc.createDelivery(alert, webhook)
		assert.False(t, delivery.InsecureSkipTLSVerify)
	})

	t.Run("allows insecure TLS with override env", func(t *testing.T) {
		t.Setenv(common.InsecureTLSOverrideEnvVar, "true")
		delivery := svc.createDelivery(alert, webhook)
		assert.True(t, delivery.InsecureSkipTLSVerify)
	})
}

func TestWebhookDeliveryService_DeliverWebhook_UsesInsecureClientWhenEnabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	t.Setenv(common.InsecureTLSOverrideEnvVar, "true")

	secureRT := webhookRTFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("secure client should not be used")
	})
	insecureRT := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	service := &WebhookDeliveryService{
		logger:         logger,
		httpClient:     &http.Client{Transport: secureRT},
		insecureClient: &http.Client{Transport: insecureRT},
	}

	alert := &models.Alert{AlertID: "test-alert", FiredAt: time.Now()}
	delivery := &models.WebhookDelivery{
		URL:                   "https://example.com/webhook",
		MaxAttempts:           1,
		InsecureSkipTLSVerify: true,
	}

	err := service.deliverWebhook(context.Background(), delivery, alert)
	require.NoError(t, err)
	assert.Equal(t, "success", delivery.Status)
	assert.Equal(t, 200, delivery.ResponseCode)
}

func TestWebhookDeliveryService_DeliverWebhook_WithSignature(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("adds signature headers when secret token is set", func(t *testing.T) {
		var capturedRequest *http.Request
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			capturedRequest = req
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			SecretToken: "my-secret-token",
			MaxAttempts: 3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)

		require.NotNil(t, capturedRequest)
		// Both signature headers should be set
		assert.NotEmpty(t, capturedRequest.Header.Get("X-Webhook-Signature"))
		assert.NotEmpty(t, capturedRequest.Header.Get("X-Webhook-Signature-256"))
		// Both should have the sha256= prefix
		assert.True(t, strings.HasPrefix(capturedRequest.Header.Get("X-Webhook-Signature"), "sha256="))
		assert.True(t, strings.HasPrefix(capturedRequest.Header.Get("X-Webhook-Signature-256"), "sha256="))
		// Both should be equal (same signature)
		assert.Equal(t, capturedRequest.Header.Get("X-Webhook-Signature"), capturedRequest.Header.Get("X-Webhook-Signature-256"))
	})

	t.Run("no signature headers when secret token is empty", func(t *testing.T) {
		var capturedRequest *http.Request
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			capturedRequest = req
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			SecretToken: "", // No secret
			MaxAttempts: 3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)

		require.NotNil(t, capturedRequest)
		assert.Empty(t, capturedRequest.Header.Get("X-Webhook-Signature"))
		assert.Empty(t, capturedRequest.Header.Get("X-Webhook-Signature-256"))
	})
}

func TestWebhookDeliveryService_DeliverWebhook_HTTPError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name            string
		statusCode      int
		retryRemaining  bool // AttemptNumber < MaxAttempts
		expectedStatus  string
		expectedErrType string
		expectError     bool
		expectNextRetry bool
	}{
		{
			name:            "404 with retries remaining sets retrying status",
			statusCode:      404,
			retryRemaining:  true,
			expectedStatus:  "retrying",
			expectedErrType: "client_error",
			expectError:     true,
			expectNextRetry: true,
		},
		{
			name:            "500 with retries remaining sets retrying status",
			statusCode:      500,
			retryRemaining:  true,
			expectedStatus:  "retrying",
			expectedErrType: "server_error",
			expectError:     true,
			expectNextRetry: true,
		},
		{
			name:            "429 with retries remaining sets retrying status",
			statusCode:      429,
			retryRemaining:  true,
			expectedStatus:  "retrying",
			expectedErrType: "rate_limit",
			expectError:     true,
			expectNextRetry: true,
		},
		{
			name:            "408 with retries remaining sets retrying status",
			statusCode:      408,
			retryRemaining:  true,
			expectedStatus:  "retrying",
			expectedErrType: "timeout",
			expectError:     true,
			expectNextRetry: true,
		},
		{
			name:            "500 without retries remaining sets failed status",
			statusCode:      500,
			retryRemaining:  false,
			expectedStatus:  "failed",
			expectedErrType: "server_error",
			expectError:     true,
			expectNextRetry: false,
		},
		{
			name:            "503 with retries remaining",
			statusCode:      503,
			retryRemaining:  true,
			expectedStatus:  "retrying",
			expectedErrType: "server_error",
			expectError:     true,
			expectNextRetry: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tc.statusCode,
					Status:     http.StatusText(tc.statusCode),
					Body:       io.NopCloser(strings.NewReader(`{"error":"something went wrong"}`)),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			})

			service := &WebhookDeliveryService{
				logger:     logger,
				httpClient: &http.Client{Transport: rt},
			}

			alert := &models.Alert{
				AlertID: "test-alert",
				FiredAt: time.Now(),
			}

			attemptNumber := 1
			maxAttempts := 3
			if !tc.retryRemaining {
				attemptNumber = maxAttempts // No more retries
			}

			delivery := &models.WebhookDelivery{
				URL:           "https://example.com/webhook",
				AttemptNumber: attemptNumber,
				MaxAttempts:   maxAttempts,
			}

			err := service.deliverWebhook(context.Background(), delivery, alert)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "webhook returned status")
			}

			assert.Equal(t, tc.expectedStatus, delivery.Status)
			assert.Equal(t, tc.expectedErrType, delivery.ErrorType)
			assert.Equal(t, tc.statusCode, delivery.ResponseCode)
			assert.Equal(t, `{"error":"something went wrong"}`, delivery.ResponseBody)

			if tc.expectNextRetry {
				assert.NotNil(t, delivery.NextRetryAt, "NextRetryAt should be set when retries remain")
			} else {
				assert.Nil(t, delivery.NextRetryAt, "NextRetryAt should be nil when no retries remain")
			}
		})
	}
}

func TestWebhookDeliveryService_DeliverWebhook_TransportError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name            string
		transportErr    error
		expectedErrType string
	}{
		{
			name:            "timeout error",
			transportErr:    errors.New("dial tcp: connection timeout exceeded"),
			expectedErrType: ErrorTypeTimeout,
		},
		{
			name:            "no such host error",
			transportErr:    errors.New("lookup nonexistent.example.com: no such host"),
			expectedErrType: "dns_error",
		},
		{
			name:            "tls error",
			transportErr:    errors.New("tls: handshake failure"),
			expectedErrType: "tls_error",
		},
		{
			name:            "certificate error",
			transportErr:    errors.New("x509: certificate verification failed"),
			expectedErrType: "tls_error",
		},
		{
			name:            "connection refused",
			transportErr:    errors.New("dial tcp 127.0.0.1:8080: connection refused"),
			expectedErrType: "connection_refused",
		},
		{
			name:            "generic network error",
			transportErr:    errors.New("read: connection reset by peer"),
			expectedErrType: "network_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
				return nil, tc.transportErr
			})

			service := &WebhookDeliveryService{
				logger:     logger,
				httpClient: &http.Client{Transport: rt},
			}

			alert := &models.Alert{
				AlertID: "test-alert",
				FiredAt: time.Now(),
			}

			delivery := &models.WebhookDelivery{
				URL:           "https://example.com/webhook",
				AttemptNumber: 1,
				MaxAttempts:   3,
			}

			err := service.deliverWebhook(context.Background(), delivery, alert)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), "request failed")
			assert.Equal(t, tc.expectedErrType, delivery.ErrorType,
				"error type should be %q for transport error %q", tc.expectedErrType, tc.transportErr)
			// Status should be "retrying" since attempts remain
			assert.Equal(t, "retrying", delivery.Status)
			assert.NotNil(t, delivery.NextRetryAt)
		})
	}
}

func TestWebhookDeliveryService_DeliverWebhook_ResponseReadFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("response read failure marks delivery as failed", func(t *testing.T) {
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       errorReader{}, // Body that errors on Read
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:           "https://example.com/webhook",
			AttemptNumber: 1,
			MaxAttempts:   3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read response")
		assert.Equal(t, "response_read", delivery.ErrorType)
		// Since attempts remain and error is not permanent, status should be retrying
		assert.Equal(t, "retrying", delivery.Status)
		assert.Equal(t, 200, delivery.ResponseCode)
	})
}

func TestWebhookDeliveryService_DeliverWebhook_RequestBodyCapture(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("captures request body as JSON", func(t *testing.T) {
		var capturedBody []byte
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			capturedBody = body
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID:  "capture-test",
			Type:     "test",
			Severity: "info",
			FiredAt:  time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			MaxAttempts: 3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)

		// Verify RequestBody is set and non-empty
		assert.NotEmpty(t, delivery.RequestBody)
		// Verify it's valid JSON
		assert.True(t, json.Valid([]byte(delivery.RequestBody)))

		// Verify it matches what was sent
		assert.Equal(t, delivery.RequestBody, string(capturedBody))

		// Verify it contains expected fields
		var parsed map[string]interface{}
		err = json.Unmarshal([]byte(delivery.RequestBody), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "capture-test", parsed["alert_id"])
		assert.Equal(t, "test", parsed["type"])
	})
}

func TestWebhookDeliveryService_DeliverWebhook_CustomHeaders(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("sets custom headers from delivery config", func(t *testing.T) {
		var capturedRequest *http.Request
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			capturedRequest = req
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			MaxAttempts: 3,
			Headers: map[string]string{
				"Content-Type":    "application/json",
				"Authorization":   "Bearer token123",
				"X-Custom-Header": "custom-value",
			},
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)

		require.NotNil(t, capturedRequest)
		assert.Equal(t, "application/json", capturedRequest.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer token123", capturedRequest.Header.Get("Authorization"))
		assert.Equal(t, "custom-value", capturedRequest.Header.Get("X-Custom-Header"))
	})
}

// =============================================================================
// Tests for stringContains helper
// =============================================================================

func TestStringContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "item in slice returns true",
			slice:    []string{"a", "b", "c"},
			item:     "b",
			expected: true,
		},
		{
			name:     "item not in slice returns false",
			slice:    []string{"a", "b", "c"},
			item:     "d",
			expected: false,
		},
		{
			name:     "empty slice returns false",
			slice:    []string{},
			item:     "a",
			expected: false,
		},
		{
			name:     "nil slice returns false",
			slice:    nil,
			item:     "a",
			expected: false,
		},
		{
			name:     "empty item in slice returns true",
			slice:    []string{"", "a"},
			item:     "",
			expected: true,
		},
		{
			name:     "case sensitive match",
			slice:    []string{"ABC", "def"},
			item:     "abc",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stringContains(tc.slice, tc.item)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// Tests for matchesWebhookFilters
// =============================================================================

func TestMatchesWebhookFilters(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := &WebhookDeliveryService{logger: logger}

	t.Run("matches when no filters set", func(t *testing.T) {
		alert := &models.Alert{
			Type:     "error_rate",
			Severity: "critical",
			Service:  "api-gateway",
		}
		webhook := &WebhookConfig{
			AlertTypes:     []string{},
			SeverityLevels: []string{},
			Services:       []string{},
		}

		result := service.matchesWebhookFilters(alert, webhook)
		assert.True(t, result)
	})

	t.Run("matches when alert type matches", func(t *testing.T) {
		alert := &models.Alert{
			Type:     "error_rate",
			Severity: "critical",
			Service:  "api-gateway",
		}
		webhook := &WebhookConfig{
			AlertTypes:     []string{"latency", "error_rate"},
			SeverityLevels: []string{},
			Services:       []string{},
		}

		result := service.matchesWebhookFilters(alert, webhook)
		assert.True(t, result)
	})

	t.Run("does not match when alert type doesn't match", func(t *testing.T) {
		alert := &models.Alert{
			Type:     "error_rate",
			Severity: "critical",
			Service:  "api-gateway",
		}
		webhook := &WebhookConfig{
			AlertTypes:     []string{"latency", "cost"},
			SeverityLevels: []string{},
			Services:       []string{},
		}

		result := service.matchesWebhookFilters(alert, webhook)
		assert.False(t, result)
	})

	t.Run("matches when severity matches", func(t *testing.T) {
		alert := &models.Alert{
			Type:     "error_rate",
			Severity: "critical",
			Service:  "api-gateway",
		}
		webhook := &WebhookConfig{
			AlertTypes:     []string{},
			SeverityLevels: []string{"critical", "warning"},
			Services:       []string{},
		}

		result := service.matchesWebhookFilters(alert, webhook)
		assert.True(t, result)
	})

	t.Run("does not match when severity doesn't match", func(t *testing.T) {
		alert := &models.Alert{
			Type:     "error_rate",
			Severity: "info",
			Service:  "api-gateway",
		}
		webhook := &WebhookConfig{
			AlertTypes:     []string{},
			SeverityLevels: []string{"critical", "warning"},
			Services:       []string{},
		}

		result := service.matchesWebhookFilters(alert, webhook)
		assert.False(t, result)
	})

	t.Run("matches when service matches", func(t *testing.T) {
		alert := &models.Alert{
			Type:     "error_rate",
			Severity: "critical",
			Service:  "api-gateway",
		}
		webhook := &WebhookConfig{
			AlertTypes:     []string{},
			SeverityLevels: []string{},
			Services:       []string{"api-gateway", "worker"},
		}

		result := service.matchesWebhookFilters(alert, webhook)
		assert.True(t, result)
	})

	t.Run("does not match when service doesn't match", func(t *testing.T) {
		alert := &models.Alert{
			Type:     "error_rate",
			Severity: "critical",
			Service:  "api-gateway",
		}
		webhook := &WebhookConfig{
			AlertTypes:     []string{},
			SeverityLevels: []string{},
			Services:       []string{"worker", "scheduler"},
		}

		result := service.matchesWebhookFilters(alert, webhook)
		assert.False(t, result)
	})

	t.Run("all filters must match", func(t *testing.T) {
		alert := &models.Alert{
			Type:     "error_rate",
			Severity: "critical",
			Service:  "api-gateway",
		}
		webhook := &WebhookConfig{
			AlertTypes:     []string{"error_rate"},
			SeverityLevels: []string{"critical"},
			Services:       []string{"api-gateway"},
		}

		result := service.matchesWebhookFilters(alert, webhook)
		assert.True(t, result)
	})

	t.Run("fails if any filter doesn't match", func(t *testing.T) {
		alert := &models.Alert{
			Type:     "error_rate",
			Severity: "critical",
			Service:  "api-gateway",
		}
		webhook := &WebhookConfig{
			AlertTypes:     []string{"error_rate"},
			SeverityLevels: []string{"warning"}, // Severity doesn't match
			Services:       []string{"api-gateway"},
		}

		result := service.matchesWebhookFilters(alert, webhook)
		assert.False(t, result)
	})
}

// =============================================================================
// Additional edge case tests
// =============================================================================

func TestWebhookDeliveryService_DeliverWebhook_MarkStarted(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("marks delivery as started before making request", func(t *testing.T) {
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			MaxAttempts: 3,
		}

		beforeStart := time.Now()
		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)

		// StartedAt should be set
		require.NotNil(t, delivery.StartedAt)
		assert.True(t, delivery.StartedAt.After(beforeStart.Add(-time.Second)) || delivery.StartedAt.Equal(beforeStart.Add(-time.Second)))
	})
}

func TestWebhookDeliveryService_DeliverWebhook_Duration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("captures duration in milliseconds", func(t *testing.T) {
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			MaxAttempts: 3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)

		// Duration should be positive and at least 10ms
		assert.Greater(t, delivery.Duration, int64(0))
		assert.GreaterOrEqual(t, delivery.Duration, int64(10))
	})
}

func TestWebhookDeliveryService_DeliverWebhook_ResponseBodyTooLarge(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("captures full response body for normal size", func(t *testing.T) {
		expectedBody := `{"result":"success","data":{"id":123}}`
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(expectedBody)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			MaxAttempts: 3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)

		assert.Equal(t, expectedBody, delivery.ResponseBody)
	})
}

func TestWebhookDeliveryService_DeliverWebhook_EmptyResponseBody(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("handles empty response body", func(t *testing.T) {
		rt := webhookRTFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 204, // No Content
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})

		service := &WebhookDeliveryService{
			logger:     logger,
			httpClient: &http.Client{Transport: rt},
		}

		alert := &models.Alert{
			AlertID: "test-alert",
			FiredAt: time.Now(),
		}

		delivery := &models.WebhookDelivery{
			URL:         "https://example.com/webhook",
			MaxAttempts: 3,
		}

		err := service.deliverWebhook(context.Background(), delivery, alert)
		require.NoError(t, err)

		assert.Equal(t, "success", delivery.Status)
		assert.Equal(t, 204, delivery.ResponseCode)
		assert.Empty(t, delivery.ResponseBody)
	})
}
