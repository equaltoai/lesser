package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubSNS struct {
	publishInputs []*sns.PublishInput
	publishOut    *sns.PublishOutput
	publishErr    error
}

func (s *stubSNS) Publish(_ context.Context, params *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	s.publishInputs = append(s.publishInputs, params)
	if s.publishErr != nil {
		return nil, s.publishErr
	}
	if s.publishOut != nil {
		return s.publishOut, nil
	}
	return &sns.PublishOutput{MessageId: aws.String("msg")}, nil
}

func TestNewAlertManagerDefaults(t *testing.T) {
	am := NewAlertManager(zap.NewNop())
	require.NotNil(t, am)
	require.NotNil(t, am.httpClient)
	require.NotNil(t, am.rateLimiter)
	assert.True(t, am.enabled)
}

func TestNewAlertManagerWithConfigWebhookDefaultsAndFallbacks(t *testing.T) {
	cfg := appconfig.Get()
	origTopic := cfg.AlertSNSTopicArn
	origWebhook := cfg.AlertWebhookURL
	t.Cleanup(func() {
		cfg.AlertSNSTopicArn = origTopic
		cfg.AlertWebhookURL = origWebhook
	})

	cfg.AlertSNSTopicArn = "arn:from-config"
	cfg.AlertWebhookURL = "http://config.invalid/webhook"

	t.Run("explicit_webhook_adds_default_headers", func(t *testing.T) {
		am := NewAlertManagerWithConfig(&AlertManagerConfig{
			Logger:      zap.NewNop(),
			WebhookURL:  "http://explicit.invalid/webhook",
			SNSTopicArn: "",
			Enabled:     true,
		})

		require.NotNil(t, am.webhookConfig)
		assert.Equal(t, "http://explicit.invalid/webhook", am.webhookConfig.URL)
		assert.Equal(t, "application/json", am.webhookConfig.Headers["Content-Type"])
		assert.Equal(t, "arn:from-config", am.snsTopicArn)
	})

	t.Run("fallback_webhook_from_central_config", func(t *testing.T) {
		am := NewAlertManagerWithConfig(&AlertManagerConfig{
			Logger:  zap.NewNop(),
			Enabled: true,
		})

		require.NotNil(t, am.webhookConfig)
		assert.Equal(t, cfg.AlertWebhookURL, am.webhookConfig.URL)
	})
}

func TestAlertManagerSendSNS(t *testing.T) {
	logger := zap.NewNop()
	stub := &stubSNS{publishOut: &sns.PublishOutput{MessageId: aws.String("msg-id")}}

	am := &AlertManager{
		logger:      logger,
		enabled:     true,
		snsClient:   stub,
		snsTopicArn: "arn:topic",
		rateLimiter: NewAlertRateLimiter(0),
	}

	longTitle := strings.Repeat("x", 200)
	alert := &Alert{
		Type:        AlertTypeErrorRate,
		Severity:    SeverityCritical,
		Title:       longTitle,
		Description: "desc",
		Service:     "svc",
		Region:      "us-east-1",
		Timestamp:   time.Now(),
		Metadata:    map[string]interface{}{"k": "v"},
		Source:      "src",
	}

	require.NoError(t, am.sendSNS(context.Background(), alert))
	require.Len(t, stub.publishInputs, 1)

	input := stub.publishInputs[0]
	require.NotNil(t, input.Subject)
	assert.LessOrEqual(t, len(*input.Subject), 100)
	assert.True(t, strings.HasSuffix(*input.Subject, "..."))

	require.NotNil(t, input.MessageAttributes)
	attr := input.MessageAttributes["AlertType"]
	require.NotNil(t, attr.StringValue)
	assert.Equal(t, string(AlertTypeErrorRate), *attr.StringValue)

	stub.publishErr = errors.New("publish failed")
	require.Error(t, am.sendSNS(context.Background(), alert))
}

func TestSendAlert_SNSSuccess(t *testing.T) {
	stub := &stubSNS{}
	am := &AlertManager{
		logger:      zap.NewNop(),
		enabled:     true,
		snsClient:   stub,
		snsTopicArn: "arn:topic",
		rateLimiter: NewAlertRateLimiter(0),
	}

	alert := &Alert{
		Type:     AlertTypeHealth,
		Severity: SeverityError,
		Title:    "Health failed",
		Service:  "svc",
	}

	require.NoError(t, am.SendAlert(context.Background(), alert))
	require.Len(t, stub.publishInputs, 1)
}

func TestCheckSecurityAndQueueAndFederationAndColdStarts(t *testing.T) {
	capture := &alertCapture{}
	client := newTestHTTPClient(200, capture.handler)
	am := newTestAlertManager("http://webhook.invalid/alerts", client)

	am.CheckSecurity(context.Background(), "login_failed", SeverityError, map[string]interface{}{"ip": "1.2.3.4"})
	alerts := capture.getAlerts()
	require.Len(t, alerts, 1)
	assert.Equal(t, AlertTypeSecurity, alerts[0].Type)
	assert.Equal(t, SeverityError, alerts[0].Severity)
	assert.Equal(t, "security", alerts[0].Service)

	am.CheckQueueDepth(context.Background(), "queue", observability.AlertP2QueueDepthMessages-1)
	assert.Len(t, capture.getAlerts(), 1)

	am.CheckQueueDepth(context.Background(), "queue", observability.AlertP2QueueDepthMessages)
	alerts = capture.getAlerts()
	require.Len(t, alerts, 2)
	assert.Equal(t, AlertTypeCapacity, alerts[1].Type)
	assert.Equal(t, "P2", alerts[1].Priority)

	am.CheckFederationHealth(context.Background(), "example.com", 50.0, 9) // insufficient data
	assert.Len(t, capture.getAlerts(), 2)

	am.CheckFederationHealth(context.Background(), "example.com", 10.0, 10)
	alerts = capture.getAlerts()
	require.Len(t, alerts, 3)
	assert.Equal(t, "P2", alerts[2].Priority)

	am.CheckColdStarts(context.Background(), "svc", observability.AlertP2ColdStartsPerMinute-1)
	assert.Len(t, capture.getAlerts(), 3)

	am.CheckColdStarts(context.Background(), "svc", observability.AlertP2ColdStartsPerMinute)
	alerts = capture.getAlerts()
	require.Len(t, alerts, 4)
	assert.Equal(t, "P2", alerts[3].Priority)
}

func TestCheckLatencyAdditionalBranches(t *testing.T) {
	capture := &alertCapture{}
	client := newTestHTTPClient(200, capture.handler)
	am := newTestAlertManager("http://webhook.invalid/alerts", client)

	am.CheckLatency(context.Background(), "svc", float64(observability.AlertP1LatencyP90Milliseconds)*2, "P99")
	alerts := capture.getAlerts()
	require.Len(t, alerts, 1)
	assert.Equal(t, "P1", alerts[0].Priority)

	am.CheckLatency(context.Background(), "svc", float64(observability.AlertP2LatencyP90Milliseconds)*2, "P99")
	alerts = capture.getAlerts()
	require.Len(t, alerts, 2)
	assert.Equal(t, "P2", alerts[1].Priority)

	am.CheckLatency(context.Background(), "svc", float64(observability.AlertP0LatencyP99Milliseconds/2), "P90")
	alerts = capture.getAlerts()
	require.Len(t, alerts, 3)
	assert.Equal(t, "P0", alerts[2].Priority)

	am.CheckLatency(context.Background(), "svc", observability.AlertP1LatencyP90Milliseconds, "P90")
	alerts = capture.getAlerts()
	require.Len(t, alerts, 4)
	assert.Equal(t, "P1", alerts[3].Priority)

	am.CheckLatency(context.Background(), "svc", 1, "P50") // invalid percentile
	assert.Len(t, capture.getAlerts(), 4)
}

func TestCheckCapacityAdditionalSeverities(t *testing.T) {
	capture := &alertCapture{}
	client := newTestHTTPClient(200, capture.handler)
	am := newTestAlertManager("http://webhook.invalid/alerts", client)

	am.CheckCapacity(context.Background(), "CPU", 90.0)
	am.CheckCapacity(context.Background(), "CPU", 99.0)

	alerts := capture.getAlerts()
	require.Len(t, alerts, 2)
	assert.Equal(t, SeverityError, alerts[0].Severity)
	assert.Equal(t, SeverityCritical, alerts[1].Severity)

	// Ensure payload remains valid JSON for dashboards.
	payload, err := json.Marshal(alerts[1])
	require.NoError(t, err)
	assert.NotEmpty(t, payload)
}
