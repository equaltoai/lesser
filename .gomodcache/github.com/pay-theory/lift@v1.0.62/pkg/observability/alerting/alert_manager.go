package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AlertManager manages alerts with rules, channels, and escalation
type AlertManager struct {
	escalator    AlertEscalator
	rules        map[string]*AlertRule
	channels     map[string]AlertChannel
	activeAlerts map[string]*Alert
	stopCh       chan struct{}
	alertHistory []Alert
	processors   []AlertProcessor
	config       AlertManagerConfig
	mu           sync.RWMutex
	running      bool
}

// AlertManagerConfig configures the alert manager
type AlertManagerConfig struct {
	Enabled             bool          `json:"enabled"`
	MaxActiveAlerts     int           `json:"max_active_alerts"`
	MaxHistorySize      int           `json:"max_history_size"`
	DefaultEscalation   time.Duration `json:"default_escalation"`
	AlertTimeout        time.Duration `json:"alert_timeout"`
	DeduplicationWindow time.Duration `json:"deduplication_window"`
	BatchSize           int           `json:"batch_size"`
	FlushInterval       time.Duration `json:"flush_interval"`
}

// AlertRule defines conditions and actions for alerts
type AlertRule struct {
	UpdatedAt   time.Time         `json:"updated_at"`
	CreatedAt   time.Time         `json:"created_at"`
	Suppression *SuppressionRule  `json:"suppression,omitempty"`
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`
	Severity    AlertSeverity     `json:"severity"`
	ID          string            `json:"id"`
	Priority    AlertPriority     `json:"priority"`
	Description string            `json:"description"`
	Name        string            `json:"name"`
	CreatedBy   string            `json:"created_by"`
	Actions     []AlertAction     `json:"actions"`
	Conditions  []AlertCondition  `json:"conditions"`
	Frequency   time.Duration     `json:"frequency"`
	Enabled     bool              `json:"enabled"`
}

// AlertCondition defines a condition for triggering alerts
type AlertCondition struct {
	Metric      string        `json:"metric"`
	Operator    Operator      `json:"operator"`
	Aggregation string        `json:"aggregation"`
	Threshold   float64       `json:"threshold"`
	Duration    time.Duration `json:"duration"`
}

// AlertAction defines an action to take when an alert fires
type AlertAction struct {
	Parameters map[string]any `json:"parameters"`
	Type       ActionType     `json:"type"`
	Channel    string         `json:"channel"`
	Template   string         `json:"template"`
	Enabled    bool           `json:"enabled"`
}

// Alert represents an active or historical alert
type Alert struct {
	StartTime   time.Time         `json:"start_time"`
	Metadata    map[string]any    `json:"metadata"`
	Annotations map[string]string `json:"annotations"`
	Labels      map[string]string `json:"labels"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	Severity    AlertSeverity     `json:"severity"`
	Status      AlertStatus       `json:"status"`
	State       AlertState        `json:"state"`
	Priority    AlertPriority     `json:"priority"`
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Name        string            `json:"name"`
	RuleID      string            `json:"rule_id"`
	Events      []AlertEvent      `json:"events"`
	Escalations []AlertEscalation `json:"escalations"`
	Duration    time.Duration     `json:"duration"`
	Value       float64           `json:"value"`
	Threshold   float64           `json:"threshold"`
}

// AlertEvent represents an event in an alert's lifecycle
type AlertEvent struct {
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
	ID        string         `json:"id"`
	Type      AlertEventType `json:"type"`
	Message   string         `json:"message"`
	User      string         `json:"user,omitempty"`
}

// AlertEscalation represents an escalation step
type AlertEscalation struct {
	Timestamp time.Time     `json:"timestamp"`
	Channels  []string      `json:"channels"`
	Level     int           `json:"level"`
	Duration  time.Duration `json:"duration"`
	Completed bool          `json:"completed"`
}

// AlertChannel defines how alerts are delivered
type AlertChannel interface {
	Send(ctx context.Context, alert *Alert) error
	Validate() error
	GetType() ChannelType
	GetConfig() map[string]any
}

// AlertProcessor processes alerts before delivery
type AlertProcessor interface {
	Process(ctx context.Context, alert *Alert) (*Alert, error)
	GetPriority() int
}

// AlertEscalator handles alert escalation
type AlertEscalator interface {
	ShouldEscalate(alert *Alert) bool
	Escalate(ctx context.Context, alert *Alert) error
	GetEscalationLevels() []EscalationLevel
}

// SuppressionRule defines when alerts should be suppressed
type SuppressionRule struct {
	StartTime   string                 `json:"start_time"`
	EndTime     string                 `json:"end_time"`
	Description string                 `json:"description"`
	Days        []string               `json:"days"`
	Conditions  []SuppressionCondition `json:"conditions"`
	Enabled     bool                   `json:"enabled"`
}

// SuppressionCondition defines a condition for suppression
type SuppressionCondition struct {
	Label    string `json:"label"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// EscalationLevel defines an escalation level
type EscalationLevel struct {
	Channels []string      `json:"channels"`
	Actions  []string      `json:"actions"`
	Level    int           `json:"level"`
	Duration time.Duration `json:"duration"`
}

// Enums and constants
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityError    AlertSeverity = "error"
	AlertSeverityCritical AlertSeverity = "critical"
)

type AlertPriority string

const (
	AlertPriorityLow    AlertPriority = "low"
	AlertPriorityMedium AlertPriority = "medium"
	AlertPriorityHigh   AlertPriority = "high"
	AlertPriorityUrgent AlertPriority = "urgent"
)

type AlertStatus string

const (
	AlertStatusActive     AlertStatus = "active"
	AlertStatusPending    AlertStatus = "pending"
	AlertStatusResolved   AlertStatus = "resolved"
	AlertStatusSuppressed AlertStatus = "suppressed"
	AlertStatusExpired    AlertStatus = "expired"
)

type AlertState string

const (
	AlertStateTriggered AlertState = "triggered"
	AlertStateFiring    AlertState = "firing"
	AlertStateClearing  AlertState = "clearing"
	AlertStateCleared   AlertState = "cleared"
)

type AlertEventType string

const (
	AlertEventTriggered  AlertEventType = "triggered"
	AlertEventResolved   AlertEventType = "resolved"
	AlertEventSuppressed AlertEventType = "suppressed"
	AlertEventEscalated  AlertEventType = "escalated"
	AlertEventExpired    AlertEventType = "expired"
	AlertEventUpdated    AlertEventType = "updated"
)

type ActionType string

const (
	ActionTypeEmail     ActionType = "email"
	ActionTypeSlack     ActionType = "slack"
	ActionTypeWebhook   ActionType = "webhook"
	ActionTypePagerDuty ActionType = "pagerduty"
	ActionTypeSMS       ActionType = "sms"
	ActionTypeTicket    ActionType = "ticket"
)

type ChannelType string

const (
	ChannelTypeEmail     ChannelType = "email"
	ChannelTypeSlack     ChannelType = "slack"
	ChannelTypeWebhook   ChannelType = "webhook"
	ChannelTypePagerDuty ChannelType = "pagerduty"
	ChannelTypeSMS       ChannelType = "sms"
)

type Operator string

const (
	OperatorGreaterThan      Operator = "gt"
	OperatorGreaterThanEqual Operator = "gte"
	OperatorLessThan         Operator = "lt"
	OperatorLessThanEqual    Operator = "lte"
	OperatorEqual            Operator = "eq"
	OperatorNotEqual         Operator = "ne"
	OperatorContains         Operator = "contains"
	OperatorNotContains      Operator = "not_contains"
)

// NewAlertManager creates a new alert manager
func NewAlertManager(config AlertManagerConfig) *AlertManager {
	return &AlertManager{
		config:       config,
		rules:        make(map[string]*AlertRule),
		channels:     make(map[string]AlertChannel),
		activeAlerts: make(map[string]*Alert),
		alertHistory: make([]Alert, 0, config.MaxHistorySize),
		processors:   make([]AlertProcessor, 0),
		stopCh:       make(chan struct{}),
	}
}

// Start starts the alert manager
func (am *AlertManager) Start(ctx context.Context) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.running {
		return fmt.Errorf("alert manager already running")
	}

	if !am.config.Enabled {
		return fmt.Errorf("alert manager not enabled")
	}

	am.running = true

	// Start background processing
	go am.runAlertProcessor(ctx)
	go am.runEscalationProcessor(ctx)
	go am.runCleanupProcessor(ctx)

	return nil
}

// Stop stops the alert manager
func (am *AlertManager) Stop() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if !am.running {
		return nil
	}

	close(am.stopCh)
	am.running = false

	return nil
}

// AddRule adds an alert rule
func (am *AlertManager) AddRule(rule *AlertRule) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if rule.ID == "" {
		return fmt.Errorf("rule ID cannot be empty")
	}

	rule.UpdatedAt = time.Now()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}

	am.rules[rule.ID] = rule
	return nil
}

// RemoveRule removes an alert rule
func (am *AlertManager) RemoveRule(ruleID string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	delete(am.rules, ruleID)
	return nil
}

// AddChannel adds an alert channel
func (am *AlertManager) AddChannel(name string, channel AlertChannel) error {
	if err := channel.Validate(); err != nil {
		return fmt.Errorf("invalid channel: %w", err)
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	am.channels[name] = channel
	return nil
}

// RemoveChannel removes an alert channel
func (am *AlertManager) RemoveChannel(name string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	delete(am.channels, name)
}

// TriggerAlert triggers an alert based on metric data
func (am *AlertManager) TriggerAlert(ctx context.Context, ruleID string, value float64, labels map[string]string) error {
	am.mu.RLock()
	rule, exists := am.rules[ruleID]
	am.mu.RUnlock()

	if !exists || !rule.Enabled {
		return fmt.Errorf("rule %s not found or disabled", ruleID)
	}

	// Check if alert should be suppressed
	if am.shouldSuppress(rule, labels) {
		return nil
	}

	// Create alert
	alert := &Alert{
		ID:          fmt.Sprintf("%s-%d", ruleID, time.Now().Unix()),
		RuleID:      ruleID,
		Name:        rule.Name,
		Description: rule.Description,
		Severity:    rule.Severity,
		Priority:    rule.Priority,
		Status:      AlertStatusPending,
		State:       AlertStateTriggered,
		StartTime:   time.Now(),
		Value:       value,
		Labels:      labels,
		Annotations: rule.Annotations,
		Events:      []AlertEvent{},
		Escalations: []AlertEscalation{},
		Metadata:    make(map[string]any),
	}

	// Set threshold from conditions
	if len(rule.Conditions) > 0 {
		alert.Threshold = rule.Conditions[0].Threshold
	}

	// Add initial event
	alert.Events = append(alert.Events, AlertEvent{
		ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()),
		Type:      AlertEventTriggered,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Alert triggered: %s", rule.Name),
	})

	// Process alert through processors
	processedAlert := alert
	for _, processor := range am.processors {
		var err error
		processedAlert, err = processor.Process(ctx, processedAlert)
		if err != nil {
			continue // Skip failed processors
		}
	}

	// Store alert
	am.mu.Lock()
	am.activeAlerts[alert.ID] = processedAlert
	am.mu.Unlock()

	// Execute actions
	return am.executeActions(ctx, processedAlert, rule)
}

// ResolveAlert resolves an active alert
func (am *AlertManager) ResolveAlert(alertID string, message string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	alert, exists := am.activeAlerts[alertID]
	if !exists {
		return fmt.Errorf("alert %s not found", alertID)
	}

	// Update alert
	now := time.Now()
	alert.Status = AlertStatusResolved
	alert.State = AlertStateCleared
	alert.EndTime = &now
	alert.Duration = now.Sub(alert.StartTime)

	// Add resolution event
	alert.Events = append(alert.Events, AlertEvent{
		ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()),
		Type:      AlertEventResolved,
		Timestamp: now,
		Message:   message,
	})

	// Move to history
	am.addToHistory(*alert)
	delete(am.activeAlerts, alertID)

	return nil
}

// GetActiveAlerts returns all active alerts
func (am *AlertManager) GetActiveAlerts() map[string]*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make(map[string]*Alert)
	for k, v := range am.activeAlerts {
		result[k] = v
	}
	return result
}

// GetAlertHistory returns alert history
func (am *AlertManager) GetAlertHistory(limit int) []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if limit <= 0 || limit > len(am.alertHistory) {
		limit = len(am.alertHistory)
	}

	result := make([]Alert, limit)
	copy(result, am.alertHistory[len(am.alertHistory)-limit:])
	return result
}

// executeActions executes alert actions
func (am *AlertManager) executeActions(ctx context.Context, alert *Alert, rule *AlertRule) error {
	for _, action := range rule.Actions {
		if !action.Enabled {
			continue
		}

		channel, exists := am.channels[action.Channel]
		if !exists {
			continue
		}

		// Execute action in background
		go func(ch AlertChannel, a *Alert) {
			if err := ch.Send(ctx, a); err != nil {
				// Record a non-fatal send error as an alert event
				am.mu.Lock()
				a.Events = append(a.Events, AlertEvent{
					ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()),
					Type:      AlertEventUpdated,
					Timestamp: time.Now(),
					Message:   fmt.Sprintf("action send failed on channel %s: %v", ch.GetType(), err),
				})
				am.mu.Unlock()
			}
		}(channel, alert)
	}

	return nil
}

// shouldSuppress checks if alert should be suppressed
func (am *AlertManager) shouldSuppress(rule *AlertRule, labels map[string]string) bool {
	if rule.Suppression == nil || !rule.Suppression.Enabled {
		return false
	}

	// Check time-based suppression
	if rule.Suppression.StartTime != "" && rule.Suppression.EndTime != "" {
		// Simple time check - in production, this would be more sophisticated
		return false
	}

	// Check condition-based suppression
	for _, condition := range rule.Suppression.Conditions {
		labelValue, exists := labels[condition.Label]
		if !exists {
			continue
		}

		switch condition.Operator {
		case "eq":
			if labelValue == condition.Value {
				return true
			}
		case "ne":
			if labelValue != condition.Value {
				return true
			}
		}
	}

	return false
}

// addToHistory adds an alert to history
func (am *AlertManager) addToHistory(alert Alert) {
	if len(am.alertHistory) >= am.config.MaxHistorySize {
		// Remove oldest alert
		am.alertHistory = am.alertHistory[1:]
	}
	am.alertHistory = append(am.alertHistory, alert)
}

// Background processors
func (am *AlertManager) runAlertProcessor(ctx context.Context) {
	ticker := time.NewTicker(am.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			am.processAlerts(ctx)
		case <-am.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (am *AlertManager) runEscalationProcessor(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			am.processEscalations(ctx)
		case <-am.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (am *AlertManager) runCleanupProcessor(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			am.cleanupExpiredAlerts()
		case <-am.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (am *AlertManager) processAlerts(_ context.Context) {
	// Update alert states and durations
	am.mu.Lock()
	defer am.mu.Unlock()

	for _, alert := range am.activeAlerts {
		alert.Duration = time.Since(alert.StartTime)

		// Update state based on duration
		if alert.Duration > time.Minute && alert.State == AlertStateTriggered {
			alert.State = AlertStateFiring
			alert.Status = AlertStatusActive
		}
	}
}

func (am *AlertManager) processEscalations(ctx context.Context) {
	// Check for alerts that need escalation
	if am.escalator == nil {
		return
	}

	am.mu.RLock()
	defer am.mu.RUnlock()

	for _, alert := range am.activeAlerts {
		if am.escalator.ShouldEscalate(alert) {
			go func(a *Alert) {
				if err := am.escalator.Escalate(ctx, a); err != nil {
					// Log error but don't fail the check
					// AlertManager doesn't have a logger field, so we'll skip logging
					_ = err
				}
			}(alert)
		}
	}
}

func (am *AlertManager) cleanupExpiredAlerts() {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Remove alerts that have exceeded timeout
	for id, alert := range am.activeAlerts {
		if time.Since(alert.StartTime) > am.config.AlertTimeout {
			now := time.Now()
			alert.Status = AlertStatusExpired
			alert.EndTime = &now
			alert.Duration = now.Sub(alert.StartTime)

			am.addToHistory(*alert)
			delete(am.activeAlerts, id)
		}
	}
}

// SetEscalator sets the alert escalator
func (am *AlertManager) SetEscalator(escalator AlertEscalator) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.escalator = escalator
}

// AddProcessor adds an alert processor
func (am *AlertManager) AddProcessor(processor AlertProcessor) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.processors = append(am.processors, processor)
}

// GetMetrics returns alert manager metrics
func (am *AlertManager) GetMetrics() AlertManagerMetrics {
	am.mu.RLock()
	defer am.mu.RUnlock()

	return AlertManagerMetrics{
		ActiveAlerts:    len(am.activeAlerts),
		TotalRules:      len(am.rules),
		TotalChannels:   len(am.channels),
		HistorySize:     len(am.alertHistory),
		ProcessorsCount: len(am.processors),
	}
}

// AlertManagerMetrics represents metrics about the alert manager
type AlertManagerMetrics struct {
	ActiveAlerts    int `json:"active_alerts"`
	TotalRules      int `json:"total_rules"`
	TotalChannels   int `json:"total_channels"`
	HistorySize     int `json:"history_size"`
	ProcessorsCount int `json:"processors_count"`
}
