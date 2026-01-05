package observability

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestLatencyAlerter_FireAndResolve(t *testing.T) {
	logger := zaptest.NewLogger(t)

	var recorded []*models.MetricRecord
	createMetricFn := func(_ context.Context, metric *models.MetricRecord) error {
		recorded = append(recorded, metric)
		return nil
	}
	recorder := NewDefaultMetricsRecorder(createMetricFn, "svc")

	alerter := NewLatencyAlerter(logger, recorder, nil)

	rule := &AlertRule{
		Name:          "test_rule",
		Operation:     "op",
		Service:       "svc",
		Threshold:     10,
		P95Threshold:  0,
		P99Threshold:  0,
		WindowSize:    time.Minute,
		MinDataPoints: 1,
		AlertCooldown: 0,
		Severity:      SeverityWarning,
		Enabled:       true,
		Conditions: []Condition{
			{Type: ConditionLatency, Operator: ">", Value: 10},
		},
		Actions: []AlertAction{
			{Type: ActionLog, Enabled: true, Config: map[string]interface{}{"level": "debug"}},
			{Type: ActionMetric, Enabled: true, Config: map[string]interface{}{"metric_name": "LatencyAlert"}},
			{Type: ActionWebhook, Enabled: true, Config: map[string]interface{}{"webhook_url": "https://example.com"}},
			{Type: ActionSlack, Enabled: true, Config: map[string]interface{}{}},
		},
	}
	alerter.AddRule(rule)

	alerter.CheckLatency(context.Background(), "op", "svc", 20, 0, 0)
	history := alerter.GetAlertHistory()
	require.Equal(t, StateFiring, history["test_rule"].CurrentState)
	require.NotEmpty(t, recorded)

	alerter.CheckLatency(context.Background(), "op", "svc", 0, 0, 0)
	history = alerter.GetAlertHistory()
	require.Equal(t, StateResolved, history["test_rule"].CurrentState)
}

func TestLatencyAlerter_EvaluateConditionOperators(t *testing.T) {
	logger := zaptest.NewLogger(t)
	alerter := NewLatencyAlerter(logger, nil, nil)

	assert.True(t, alerter.evaluateCondition(Condition{Type: ConditionLatency, Operator: ">", Value: 1}, 2, 0, 0))
	assert.True(t, alerter.evaluateCondition(Condition{Type: ConditionLatency, Operator: ">=", Value: 2}, 2, 0, 0))
	assert.True(t, alerter.evaluateCondition(Condition{Type: ConditionLatency, Operator: "<", Value: 3}, 2, 0, 0))
	assert.True(t, alerter.evaluateCondition(Condition{Type: ConditionLatency, Operator: "<=", Value: 2}, 2, 0, 0))
	assert.True(t, alerter.evaluateCondition(Condition{Type: ConditionLatency, Operator: "==", Value: 2}, 2, 0, 0))
	assert.False(t, alerter.evaluateCondition(Condition{Type: ConditionLatency, Operator: "??", Value: 2}, 2, 0, 0))
	assert.False(t, alerter.evaluateCondition(Condition{Type: ConditionErrorRate, Operator: ">", Value: 1}, 2, 0, 0))

	// Percentile handling
	assert.True(t, alerter.evaluateCondition(Condition{Type: ConditionLatency, Operator: ">", Value: 1, Percentile: "p95"}, 0, 2, 0))
	assert.True(t, alerter.evaluateCondition(Condition{Type: ConditionLatency, Operator: ">", Value: 1, Percentile: "p99"}, 0, 0, 2))
}

func TestLatencyAlerter_RulesAndEnabledFlag(t *testing.T) {
	logger := zaptest.NewLogger(t)
	alerter := NewLatencyAlerter(logger, nil, nil)

	rules := alerter.GetAlertRules()
	require.NotEmpty(t, rules)

	alerter.SetEnabled(false)
	alerter.CheckLatency(context.Background(), "api_endpoint", "api", 999999, 0, 0)
}

func TestLatencyAlerter_MiscBranches(t *testing.T) {
	logger := zaptest.NewLogger(t)
	alerter := NewLatencyAlerter(logger, nil, nil)

	assert.Equal(t, "latency", ConditionLatency.String())
	assert.Equal(t, "error_rate", ConditionErrorRate.String())
	assert.Equal(t, "throughput", ConditionThroughput.String())
	assert.Equal(t, AlertSeverityUnknown, ConditionType(99).String())

	assert.Equal(t, "normal", StateNormal.String())
	assert.Equal(t, "firing", StateFiring.String())
	assert.Equal(t, "resolved", StateResolved.String())
	assert.Equal(t, AlertSeverityUnknown, AlertState(99).String())

	assert.Equal(t, "log", ActionLog.String())
	assert.Equal(t, "metric", ActionMetric.String())
	assert.Equal(t, "webhook", ActionWebhook.String())
	assert.Equal(t, "email", ActionEmail.String())
	assert.Equal(t, "slack", ActionSlack.String())
	assert.Equal(t, AlertSeverityUnknown, ActionType(99).String())

	assert.Equal(t, "critical", alerter.mapSeverityToPriority(SeverityCritical))
	assert.Equal(t, "high", alerter.mapSeverityToPriority(SeverityError))
	assert.Equal(t, "medium", alerter.mapSeverityToPriority(SeverityWarning))
	assert.Equal(t, "low", alerter.mapSeverityToPriority(SeverityInfo))
	assert.Equal(t, "medium", alerter.mapSeverityToPriority(AlertSeverity(99)))

	msg := alerter.formatAlertMessage(&AlertRule{Name: "r", Service: "svc", Operation: "op", Threshold: 1}, 2, 3, 4, true)
	assert.Contains(t, msg, "fired")

	alerter.RemoveRule("api_latency_critical")

	// Execute webhook action with a configured (disabled) delivery service so we exercise payload shaping.
	alerter.webhookDelivery = &WebhookDeliveryService{logger: logger, enabled: false}
	alerter.executeWebhookAction(context.Background(), &Alert{
		RuleName:  "rule",
		Service:   "svc",
		Operation: "op",
		Severity:  SeverityWarning,
		State:     StateResolved,
		Message:   "m",
		Timestamp: time.Now(),
		Values:    map[string]float64{"latency_ms": 1},
		Dimensions: map[string]string{
			"service":   "svc",
			"operation": "op",
		},
		Context: map[string]interface{}{
			"threshold": 1.0,
			"ignore":    "x",
		},
		Actions: []AlertAction{{Type: ActionWebhook, Enabled: true, Config: map[string]interface{}{"webhook_url": "https://example.com"}}},
	}, AlertAction{Type: ActionWebhook, Enabled: true, Config: map[string]interface{}{"webhook_url": "https://example.com"}})
}
