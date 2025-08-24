// Package observability provides real-time latency alerting with configurable thresholds
package observability

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// Alert severity constants
const (
	AlertSeverityInfo     = "info"
	AlertSeverityWarning  = "warning"
	AlertSeverityError    = "error"
	AlertSeverityCritical = "critical"
	AlertSeverityUnknown  = "unknown"
)

// LatencyAlerter manages real-time latency alerting
type LatencyAlerter struct {
	logger          *zap.Logger
	metricsRecorder MetricsRecorder
	alertRules      map[string]*AlertRule
	alertHistory    map[string]*AlertHistory
	mu              sync.RWMutex
	enabled         bool
}

// AlertRule defines the conditions for triggering an alert
type AlertRule struct {
	Name          string        `json:"name"`
	Operation     string        `json:"operation"`
	Service       string        `json:"service"`
	Threshold     float64       `json:"threshold_ms"`
	P95Threshold  float64       `json:"p95_threshold_ms"`
	P99Threshold  float64       `json:"p99_threshold_ms"`
	WindowSize    time.Duration `json:"window_size"`
	MinDataPoints int           `json:"min_data_points"`
	AlertCooldown time.Duration `json:"alert_cooldown"`
	Severity      AlertSeverity `json:"severity"`
	Enabled       bool          `json:"enabled"`
	Conditions    []Condition   `json:"conditions"`
	Actions       []AlertAction `json:"actions"`
}

// Condition represents a condition that must be met for an alert
type Condition struct {
	Type       ConditionType `json:"type"`                 // "latency", "error_rate", "throughput"
	Operator   string        `json:"operator"`             // ">", "<", ">=", "<=", "=="
	Value      float64       `json:"value"`                // Threshold value
	Percentile string        `json:"percentile,omitempty"` // "p50", "p95", "p99"
}

// AlertAction defines what to do when an alert is triggered
type AlertAction struct {
	Type    ActionType             `json:"type"`
	Config  map[string]interface{} `json:"config"`
	Enabled bool                   `json:"enabled"`
}

// AlertHistory tracks alert firing history
type AlertHistory struct {
	RuleName         string     `json:"rule_name"`
	LastFired        time.Time  `json:"last_fired"`
	LastResolved     time.Time  `json:"last_resolved"`
	FireCount        int        `json:"fire_count"`
	CurrentState     AlertState `json:"current_state"`
	LastLatency      float64    `json:"last_latency_ms"`
	LastP95          float64    `json:"last_p95_ms"`
	LastP99          float64    `json:"last_p99_ms"`
	ConsecutiveFires int        `json:"consecutive_fires"`
}

// Alert represents a triggered alert
type Alert struct {
	RuleName   string                 `json:"rule_name"`
	Service    string                 `json:"service"`
	Operation  string                 `json:"operation"`
	Severity   AlertSeverity          `json:"severity"`
	State      AlertState             `json:"state"`
	Message    string                 `json:"message"`
	Timestamp  time.Time              `json:"timestamp"`
	Values     map[string]float64     `json:"values"`
	Dimensions map[string]string      `json:"dimensions"`
	Actions    []AlertAction          `json:"actions"`
	Context    map[string]interface{} `json:"context"`
}

// AlertSeverity represents the severity level of alerts
type AlertSeverity int

const (
	// SeverityInfo represents informational alerts
	SeverityInfo AlertSeverity = iota
	// SeverityWarning represents warning level alerts
	SeverityWarning
	// SeverityError represents error level alerts
	SeverityError
	// SeverityCritical represents critical level alerts
	SeverityCritical
)

func (s AlertSeverity) String() string {
	switch s {
	case SeverityInfo:
		return AlertSeverityInfo
	case SeverityWarning:
		return AlertSeverityWarning
	case SeverityError:
		return "error"
	case SeverityCritical:
		return AlertSeverityCritical
	default:
		return AlertSeverityUnknown
	}
}

// AlertState represents the current state of an alert
type AlertState int

const (
	// StateNormal represents the normal (not firing) state
	StateNormal AlertState = iota
	// StateFiring represents an actively firing alert
	StateFiring
	// StateResolved represents a resolved alert
	StateResolved
)

func (s AlertState) String() string {
	switch s {
	case StateNormal:
		return "normal"
	case StateFiring:
		return "firing"
	case StateResolved:
		return "resolved"
	default:
		return AlertSeverityUnknown
	}
}

// ConditionType represents the type of alert condition
type ConditionType int

const (
	// ConditionLatency represents latency-based alerting
	ConditionLatency ConditionType = iota
	// ConditionErrorRate represents error rate based alerting
	ConditionErrorRate
	// ConditionThroughput represents throughput based alerting
	ConditionThroughput
)

func (c ConditionType) String() string {
	switch c {
	case ConditionLatency:
		return "latency"
	case ConditionErrorRate:
		return "error_rate"
	case ConditionThroughput:
		return "throughput"
	default:
		return AlertSeverityUnknown
	}
}

// ActionType represents the type of action to take when an alert fires
type ActionType int

const (
	// ActionLog represents logging the alert
	ActionLog ActionType = iota
	// ActionMetric represents recording a metric for the alert
	ActionMetric
	// ActionWebhook represents sending a webhook for the alert
	ActionWebhook
	// ActionEmail represents sending an email for the alert
	ActionEmail
	// ActionSlack represents sending a Slack message for the alert
	ActionSlack
)

func (a ActionType) String() string {
	switch a {
	case ActionLog:
		return "log"
	case ActionMetric:
		return "metric"
	case ActionWebhook:
		return "webhook"
	case ActionEmail:
		return "email"
	case ActionSlack:
		return "slack"
	default:
		return AlertSeverityUnknown
	}
}

// NewLatencyAlerter creates a new latency alerter
func NewLatencyAlerter(logger *zap.Logger, recorder MetricsRecorder) *LatencyAlerter {
	alerter := &LatencyAlerter{
		logger:          logger,
		metricsRecorder: recorder,
		alertRules:      make(map[string]*AlertRule),
		alertHistory:    make(map[string]*AlertHistory),
		enabled:         true,
	}

	// Add default alert rules
	alerter.addDefaultRules()

	return alerter
}

// addDefaultRules adds standard latency alert rules
func (la *LatencyAlerter) addDefaultRules() {
	// Critical API latency alert
	la.AddRule(&AlertRule{
		Name:          "api_latency_critical",
		Operation:     "api_endpoint",
		Service:       "api",
		Threshold:     2000, // 2 seconds
		P95Threshold:  1000, // 1 second P95
		P99Threshold:  2000, // 2 seconds P99
		WindowSize:    5 * time.Minute,
		MinDataPoints: 3,
		AlertCooldown: 10 * time.Minute,
		Severity:      SeverityCritical,
		Enabled:       true,
		Conditions: []Condition{
			{
				Type:     ConditionLatency,
				Operator: ">",
				Value:    2000,
			},
		},
		Actions: []AlertAction{
			{
				Type:    ActionLog,
				Enabled: true,
				Config: map[string]interface{}{
					"level": "error",
				},
			},
			{
				Type:    ActionMetric,
				Enabled: true,
				Config: map[string]interface{}{
					"metric_name": "LatencyAlert",
				},
			},
		},
	})

	// Database operation latency warning
	la.AddRule(&AlertRule{
		Name:          "database_latency_warning",
		Operation:     "database_operation",
		Service:       "api",
		Threshold:     1000, // 1 second
		P95Threshold:  500,  // 500ms P95
		P99Threshold:  1000, // 1 second P99
		WindowSize:    10 * time.Minute,
		MinDataPoints: 5,
		AlertCooldown: 15 * time.Minute,
		Severity:      SeverityWarning,
		Enabled:       true,
		Conditions: []Condition{
			{
				Type:       ConditionLatency,
				Operator:   ">",
				Value:      500,
				Percentile: "p95",
			},
		},
		Actions: []AlertAction{
			{
				Type:    ActionLog,
				Enabled: true,
				Config: map[string]interface{}{
					"level": "warn",
				},
			},
		},
	})

	// Federation latency error
	la.AddRule(&AlertRule{
		Name:          "federation_latency_error",
		Operation:     "federation_request",
		Service:       "api",
		Threshold:     5000, // 5 seconds
		P95Threshold:  3000, // 3 seconds P95
		P99Threshold:  5000, // 5 seconds P99
		WindowSize:    15 * time.Minute,
		MinDataPoints: 2,
		AlertCooldown: 20 * time.Minute,
		Severity:      SeverityError,
		Enabled:       true,
		Conditions: []Condition{
			{
				Type:     ConditionLatency,
				Operator: ">",
				Value:    5000,
			},
		},
		Actions: []AlertAction{
			{
				Type:    ActionLog,
				Enabled: true,
				Config: map[string]interface{}{
					"level": "error",
				},
			},
		},
	})
}

// AddRule adds an alert rule
func (la *LatencyAlerter) AddRule(rule *AlertRule) {
	la.mu.Lock()
	defer la.mu.Unlock()

	la.alertRules[rule.Name] = rule

	// Initialize alert history
	la.alertHistory[rule.Name] = &AlertHistory{
		RuleName:     rule.Name,
		CurrentState: StateNormal,
	}

	la.logger.Info("added latency alert rule",
		zap.String("rule_name", rule.Name),
		zap.String("operation", rule.Operation),
		zap.Float64("threshold_ms", rule.Threshold),
		zap.String("severity", rule.Severity.String()))
}

// RemoveRule removes an alert rule
func (la *LatencyAlerter) RemoveRule(ruleName string) {
	la.mu.Lock()
	defer la.mu.Unlock()

	delete(la.alertRules, ruleName)
	delete(la.alertHistory, ruleName)

	la.logger.Info("removed latency alert rule", zap.String("rule_name", ruleName))
}

// CheckLatency checks latency against all applicable rules
func (la *LatencyAlerter) CheckLatency(ctx context.Context, operation, service string, latencyMs float64, p95Ms, p99Ms float64) {
	if !la.enabled {
		return
	}

	la.mu.RLock()
	defer la.mu.RUnlock()

	for _, rule := range la.alertRules {
		if !rule.Enabled {
			continue
		}

		// Check if rule applies to this operation and service
		if rule.Operation != "" && rule.Operation != operation {
			continue
		}
		if rule.Service != "" && rule.Service != service {
			continue
		}

		// Check rule conditions
		la.evaluateRule(ctx, rule, operation, service, latencyMs, p95Ms, p99Ms)
	}
}

// evaluateRule evaluates a specific alert rule
func (la *LatencyAlerter) evaluateRule(ctx context.Context, rule *AlertRule, operation, service string, latencyMs, p95Ms, p99Ms float64) {
	history := la.alertHistory[rule.Name]

	// Check if we're in cooldown period
	if history.CurrentState == StateFiring &&
		time.Since(history.LastFired) < rule.AlertCooldown {
		return
	}

	// Evaluate conditions
	shouldFire := la.shouldFireAlert(rule, latencyMs, p95Ms, p99Ms)

	// Update history
	history.LastLatency = latencyMs
	history.LastP95 = p95Ms
	history.LastP99 = p99Ms

	if shouldFire && history.CurrentState != StateFiring {
		// Fire alert
		la.fireAlert(ctx, rule, operation, service, latencyMs, p95Ms, p99Ms)

		history.CurrentState = StateFiring
		history.LastFired = time.Now()
		history.FireCount++
		history.ConsecutiveFires++

	} else if !shouldFire && history.CurrentState == StateFiring {
		// Resolve alert
		la.resolveAlert(ctx, rule, operation, service)

		history.CurrentState = StateResolved
		history.LastResolved = time.Now()
		history.ConsecutiveFires = 0
	}
}

// shouldFireAlert determines if an alert should fire based on conditions
func (la *LatencyAlerter) shouldFireAlert(rule *AlertRule, latencyMs, p95Ms, p99Ms float64) bool {
	// Simple threshold check (legacy)
	if latencyMs > rule.Threshold {
		return true
	}

	if p95Ms > rule.P95Threshold && rule.P95Threshold > 0 {
		return true
	}

	if p99Ms > rule.P99Threshold && rule.P99Threshold > 0 {
		return true
	}

	// Advanced condition evaluation
	for _, condition := range rule.Conditions {
		if la.evaluateCondition(condition, latencyMs, p95Ms, p99Ms) {
			return true
		}
	}

	return false
}

// evaluateCondition evaluates a single condition
func (la *LatencyAlerter) evaluateCondition(condition Condition, latencyMs, p95Ms, p99Ms float64) bool {
	var value float64

	switch condition.Type {
	case ConditionLatency:
		switch condition.Percentile {
		case "p95":
			value = p95Ms
		case "p99":
			value = p99Ms
		default:
			value = latencyMs
		}
	default:
		return false
	}

	switch condition.Operator {
	case ">":
		return value > condition.Value
	case ">=":
		return value >= condition.Value
	case "<":
		return value < condition.Value
	case "<=":
		return value <= condition.Value
	case "==":
		return value == condition.Value
	default:
		return false
	}
}

// fireAlert triggers an alert
func (la *LatencyAlerter) fireAlert(ctx context.Context, rule *AlertRule, operation, service string, latencyMs, p95Ms, p99Ms float64) {
	alert := &Alert{
		RuleName:  rule.Name,
		Service:   service,
		Operation: operation,
		Severity:  rule.Severity,
		State:     StateFiring,
		Message:   la.formatAlertMessage(rule, latencyMs, p95Ms, p99Ms, true),
		Timestamp: time.Now(),
		Values: map[string]float64{
			"latency_ms": latencyMs,
			"p95_ms":     p95Ms,
			"p99_ms":     p99Ms,
			"threshold":  rule.Threshold,
		},
		Dimensions: map[string]string{
			"service":   service,
			"operation": operation,
			"severity":  rule.Severity.String(),
		},
		Actions: rule.Actions,
		Context: map[string]interface{}{
			"rule_name":   rule.Name,
			"window_size": rule.WindowSize.String(),
		},
	}

	// Execute alert actions
	la.executeActions(ctx, alert)

	la.logger.Error("latency alert fired",
		zap.String("rule_name", rule.Name),
		zap.String("operation", operation),
		zap.String("service", service),
		zap.Float64("latency_ms", latencyMs),
		zap.Float64("p95_ms", p95Ms),
		zap.Float64("p99_ms", p99Ms),
		zap.Float64("threshold", rule.Threshold),
		zap.String("severity", rule.Severity.String()),
		zap.String("message", alert.Message))
}

// resolveAlert resolves a fired alert
func (la *LatencyAlerter) resolveAlert(ctx context.Context, rule *AlertRule, operation, service string) {
	alert := &Alert{
		RuleName:  rule.Name,
		Service:   service,
		Operation: operation,
		Severity:  rule.Severity,
		State:     StateResolved,
		Message:   fmt.Sprintf("Latency alert resolved for %s.%s", service, operation),
		Timestamp: time.Now(),
		Dimensions: map[string]string{
			"service":   service,
			"operation": operation,
			"severity":  rule.Severity.String(),
		},
		Actions: rule.Actions,
	}

	// Execute resolution actions
	la.executeActions(ctx, alert)

	la.logger.Info("latency alert resolved",
		zap.String("rule_name", rule.Name),
		zap.String("operation", operation),
		zap.String("service", service),
		zap.String("severity", rule.Severity.String()))
}

// executeActions executes the actions for an alert
func (la *LatencyAlerter) executeActions(ctx context.Context, alert *Alert) {
	for _, action := range alert.Actions {
		if !action.Enabled {
			continue
		}

		switch action.Type {
		case ActionLog:
			la.executeLogAction(alert, action)
		case ActionMetric:
			la.executeMetricAction(ctx, alert, action)
		case ActionWebhook:
			la.executeWebhookAction(ctx, alert, action)
		default:
			la.logger.Warn("unsupported alert action type",
				zap.String("action_type", action.Type.String()),
				zap.String("rule_name", alert.RuleName))
		}
	}
}

// executeLogAction logs the alert
func (la *LatencyAlerter) executeLogAction(alert *Alert, action AlertAction) {
	level, ok := action.Config["level"].(string)
	if !ok {
		level = AlertSeverityInfo
	}

	logFields := []zap.Field{
		zap.String("alert_type", "latency"),
		zap.String("rule_name", alert.RuleName),
		zap.String("service", alert.Service),
		zap.String("operation", alert.Operation),
		zap.String("severity", alert.Severity.String()),
		zap.String("state", alert.State.String()),
		zap.Time("timestamp", alert.Timestamp),
		zap.Any("values", alert.Values),
	}

	switch level {
	case "error":
		la.logger.Error(alert.Message, logFields...)
	case "warn":
		la.logger.Warn(alert.Message, logFields...)
	case AlertSeverityInfo:
		la.logger.Info(alert.Message, logFields...)
	case "debug":
		la.logger.Debug(alert.Message, logFields...)
	default:
		la.logger.Info(alert.Message, logFields...)
	}
}

// executeMetricAction records an alert metric
func (la *LatencyAlerter) executeMetricAction(ctx context.Context, alert *Alert, action AlertAction) {
	if la.metricsRecorder == nil {
		return
	}

	metricName, ok := action.Config["metric_name"].(string)
	if !ok {
		metricName = "Alert"
	}

	// Record alert metric
	if dmr, ok := la.metricsRecorder.(*DefaultMetricsRecorder); ok {
		metric := &models.MetricRecord{
			MetricType:       "alert",
			ServiceName:      dmr.serviceName,
			Timestamp:        alert.Timestamp,
			AggregationLevel: "raw",
			Unit:             "count",
			Count:            1,
			Sum:              1,
			Min:              1,
			Max:              1,
			P50:              1,
			P95:              1,
			P99:              1,
			Dimensions:       alert.Dimensions,
		}

		metric.AddDimension("alert_name", alert.RuleName)
		metric.AddDimension("alert_state", alert.State.String())
		metric.AddDimension("metric_name", metricName)

		if err := dmr.createMetricFn(ctx, metric); err != nil {
			la.logger.Warn("failed to record alert metric",
				zap.String("rule_name", alert.RuleName),
				zap.Error(err))
		}
	}
}

// executeWebhookAction sends a webhook for the alert (placeholder)
func (la *LatencyAlerter) executeWebhookAction(_ context.Context, alert *Alert, _ AlertAction) {
	// TODO: Implement webhook sending
	la.logger.Debug("webhook action triggered",
		zap.String("rule_name", alert.RuleName),
		zap.String("state", alert.State.String()))
}

// formatAlertMessage formats a human-readable alert message
func (la *LatencyAlerter) formatAlertMessage(rule *AlertRule, latencyMs, p95Ms, p99Ms float64, firing bool) string {
	action := "fired"
	if !firing {
		action = "resolved"
	}

	return fmt.Sprintf("Latency alert '%s' %s: %s.%s latency %.1fms (P95: %.1fms, P99: %.1fms) exceeds threshold %.1fms",
		rule.Name,
		action,
		rule.Service,
		rule.Operation,
		latencyMs,
		p95Ms,
		p99Ms,
		rule.Threshold)
}

// GetAlertRules returns all configured alert rules
func (la *LatencyAlerter) GetAlertRules() map[string]*AlertRule {
	la.mu.RLock()
	defer la.mu.RUnlock()

	rules := make(map[string]*AlertRule)
	for name, rule := range la.alertRules {
		// Deep copy to prevent modification
		ruleCopy := *rule
		rules[name] = &ruleCopy
	}

	return rules
}

// GetAlertHistory returns alert history
func (la *LatencyAlerter) GetAlertHistory() map[string]*AlertHistory {
	la.mu.RLock()
	defer la.mu.RUnlock()

	history := make(map[string]*AlertHistory)
	for name, hist := range la.alertHistory {
		// Deep copy to prevent modification
		histCopy := *hist
		history[name] = &histCopy
	}

	return history
}

// SetEnabled enables or disables the alerter
func (la *LatencyAlerter) SetEnabled(enabled bool) {
	la.mu.Lock()
	defer la.mu.Unlock()

	la.enabled = enabled

	la.logger.Info("latency alerter enabled state changed",
		zap.Bool("enabled", enabled))
}
