package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// SLAMonitor manages Service Level Agreement monitoring
type SLAMonitor struct {
	metrics    map[string]*SLAMetrics
	alerts     *AlertManager
	reports    *ReportGenerator
	collectors map[string]MetricCollector
	config     SLAConfig
	mu         sync.RWMutex
}

// SLAConfig holds SLA configuration
type SLAConfig struct {
	ApplicationName string          `json:"application_name"`
	Environment     string          `json:"environment"`
	SLOs            []SLO           `json:"slos"`
	AlertRules      []AlertRule     `json:"alert_rules"`
	Reporting       ReportConfig    `json:"reporting"`
	Thresholds      ThresholdConfig `json:"thresholds"`
}

// SLO represents a Service Level Objective
type SLO struct {
	Type        SLOType       `json:"type"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Target      float64       `json:"target"`
	Window      time.Duration `json:"window"`
	Critical    bool          `json:"critical"`
	Enabled     bool          `json:"enabled"`
}

// SLOType defines types of SLOs
type SLOType string

const (
	SLOAvailability SLOType = "availability"
	SLOLatency      SLOType = "latency"
	SLOErrorRate    SLOType = "error_rate"
	SLOThroughput   SLOType = "throughput"
)

// SLAMetrics holds SLA metrics
type SLAMetrics struct {
	LastUpdated  time.Time         `json:"last_updated"`
	SLOName      string            `json:"slo_name"`
	Status       SLAStatus         `json:"status"`
	History      []MetricDataPoint `json:"history"`
	Violations   []SLAViolation    `json:"violations"`
	CurrentValue float64           `json:"current_value"`
	Target       float64           `json:"target"`
	ErrorBudget  float64           `json:"error_budget"`
	BurnRate     float64           `json:"burn_rate"`
}

// SLAStatus represents SLA status
type SLAStatus string

const (
	SLAStatusHealthy  SLAStatus = "healthy"
	SLAStatusWarning  SLAStatus = "warning"
	SLAStatusCritical SLAStatus = "critical"
	SLAStatusViolated SLAStatus = "violated"
)

// MetricDataPoint represents a metric data point
type MetricDataPoint struct {
	Timestamp time.Time         `json:"timestamp"`
	Tags      map[string]string `json:"tags,omitempty"`
	Value     float64           `json:"value"`
}

// SLAViolation represents an SLA violation
type SLAViolation struct {
	Timestamp   time.Time     `json:"timestamp"`
	ResolvedAt  *time.Time    `json:"resolved_at,omitempty"`
	ID          string        `json:"id"`
	SLOName     string        `json:"slo_name"`
	Severity    Severity      `json:"severity"`
	Description string        `json:"description"`
	Duration    time.Duration `json:"duration"`
	ActualValue float64       `json:"actual_value"`
	TargetValue float64       `json:"target_value"`
	Resolved    bool          `json:"resolved"`
}

// AlertRule defines alerting rules
type AlertRule struct {
	Name      string        `json:"name"`
	SLOName   string        `json:"slo_name"`
	Condition string        `json:"condition"`
	Severity  Severity      `json:"severity"`
	Actions   []AlertAction `json:"actions"`
	Threshold float64       `json:"threshold"`
	Duration  time.Duration `json:"duration"`
	Enabled   bool          `json:"enabled"`
}

// Severity levels
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// AlertAction defines alert actions
type AlertAction struct {
	Config map[string]any `json:"config"`
	Type   string         `json:"type"`
}

// ReportConfig defines reporting configuration
type ReportConfig struct {
	Format     string        `json:"format"`
	Template   string        `json:"template"`
	Recipients []string      `json:"recipients"`
	Frequency  time.Duration `json:"frequency"`
	Enabled    bool          `json:"enabled"`
}

// ThresholdConfig defines threshold configuration
type ThresholdConfig struct {
	WarningThreshold  float64 `json:"warning_threshold"`
	CriticalThreshold float64 `json:"critical_threshold"`
	ErrorBudgetAlert  float64 `json:"error_budget_alert"`
}

// MetricCollector interface for collecting metrics
type MetricCollector interface {
	CollectMetrics(ctx context.Context) ([]MetricDataPoint, error)
	GetMetricType() SLOType
}

// NewSLAMonitor creates a new SLA monitor
func NewSLAMonitor(config SLAConfig) *SLAMonitor {
	return &SLAMonitor{
		config:     config,
		metrics:    make(map[string]*SLAMetrics),
		alerts:     NewAlertManager(),
		reports:    NewReportGenerator(),
		collectors: make(map[string]MetricCollector),
	}
}

// Start starts SLA monitoring
func (sm *SLAMonitor) Start(ctx context.Context) error {
	// Initialize metrics for each SLO
	for _, slo := range sm.config.SLOs {
		if slo.Enabled {
			sm.metrics[slo.Name] = &SLAMetrics{
				SLOName:     slo.Name,
				Target:      slo.Target,
				Status:      SLAStatusHealthy,
				ErrorBudget: 100.0,
				History:     make([]MetricDataPoint, 0),
				Violations:  make([]SLAViolation, 0),
				LastUpdated: time.Now(),
			}
		}
	}

	// Start metric collection
	go sm.startMetricCollection(ctx)

	// Start alert processing
	go sm.startAlertProcessing(ctx)

	// Start report generation
	if sm.config.Reporting.Enabled {
		go sm.startReportGeneration(ctx)
	}

	return nil
}

// RegisterCollector registers a metric collector
func (sm *SLAMonitor) RegisterCollector(name string, collector MetricCollector) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.collectors[name] = collector
}

// startMetricCollection starts collecting metrics
func (sm *SLAMonitor) startMetricCollection(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.collectAndProcessMetrics(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// collectAndProcessMetrics collects and processes metrics
func (sm *SLAMonitor) collectAndProcessMetrics(ctx context.Context) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for name, collector := range sm.collectors {
		dataPoints, err := collector.CollectMetrics(ctx)
		if err != nil {
			continue
		}

		for _, slo := range sm.config.SLOs {
			if slo.Name == name && slo.Enabled {
				sm.processMetrics(slo, dataPoints)
			}
		}
	}
}

// processMetrics processes collected metrics
func (sm *SLAMonitor) processMetrics(slo SLO, dataPoints []MetricDataPoint) {
	metrics, exists := sm.metrics[slo.Name]
	if !exists {
		return
	}

	// Add new data points
	metrics.History = append(metrics.History, dataPoints...)

	// Keep only recent data within the SLO window
	cutoff := time.Now().Add(-slo.Window)
	filteredHistory := make([]MetricDataPoint, 0)
	for _, point := range metrics.History {
		if point.Timestamp.After(cutoff) {
			filteredHistory = append(filteredHistory, point)
		}
	}
	metrics.History = filteredHistory

	// Calculate current value based on SLO type
	metrics.CurrentValue = sm.calculateSLOValue(slo.Type, filteredHistory)

	// Update error budget and burn rate
	sm.updateErrorBudget(metrics, slo)

	// Check for violations
	sm.checkViolations(slo, metrics)

	// Update status
	sm.updateSLAStatus(metrics, slo)

	metrics.LastUpdated = time.Now()
}

// calculateSLOValue calculates SLO value based on type
func (sm *SLAMonitor) calculateSLOValue(sloType SLOType, dataPoints []MetricDataPoint) float64 {
	if len(dataPoints) == 0 {
		return 0.0
	}

	switch sloType {
	case SLOAvailability:
		return sm.calculateAvailability(dataPoints)
	case SLOLatency:
		return sm.calculatePercentile(dataPoints, 95.0)
	case SLOErrorRate:
		return sm.calculateErrorRate(dataPoints)
	case SLOThroughput:
		return sm.calculateThroughput(dataPoints)
	default:
		return 0.0
	}
}

// calculateAvailability calculates availability percentage
func (sm *SLAMonitor) calculateAvailability(dataPoints []MetricDataPoint) float64 {
	if len(dataPoints) == 0 {
		return 0.0
	}

	successCount := 0
	for _, point := range dataPoints {
		if point.Value > 0 {
			successCount++
		}
	}

	return (float64(successCount) / float64(len(dataPoints))) * 100.0
}

// calculatePercentile calculates percentile value
func (sm *SLAMonitor) calculatePercentile(dataPoints []MetricDataPoint, _ float64) float64 {
	if len(dataPoints) == 0 {
		return 0.0
	}

	// Simple percentile calculation (in production, use proper sorting)
	sum := 0.0
	for _, point := range dataPoints {
		sum += point.Value
	}

	return sum / float64(len(dataPoints))
}

// calculateErrorRate calculates error rate percentage
func (sm *SLAMonitor) calculateErrorRate(dataPoints []MetricDataPoint) float64 {
	if len(dataPoints) == 0 {
		return 0.0
	}

	errorCount := 0
	for _, point := range dataPoints {
		if point.Value > 0 {
			errorCount++
		}
	}

	return (float64(errorCount) / float64(len(dataPoints))) * 100.0
}

// calculateThroughput calculates throughput
func (sm *SLAMonitor) calculateThroughput(dataPoints []MetricDataPoint) float64 {
	if len(dataPoints) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, point := range dataPoints {
		sum += point.Value
	}

	return sum
}

// updateErrorBudget updates error budget and burn rate
func (sm *SLAMonitor) updateErrorBudget(metrics *SLAMetrics, slo SLO) {
	// Calculate error budget based on SLO type
	switch slo.Type {
	case SLOAvailability:
		allowedDowntime := (100.0 - slo.Target) / 100.0
		actualDowntime := (100.0 - metrics.CurrentValue) / 100.0
		metrics.ErrorBudget = ((allowedDowntime - actualDowntime) / allowedDowntime) * 100.0
	case SLOErrorRate:
		allowedErrors := slo.Target
		actualErrors := metrics.CurrentValue
		metrics.ErrorBudget = ((allowedErrors - actualErrors) / allowedErrors) * 100.0
	default:
		metrics.ErrorBudget = 100.0
	}

	// Ensure error budget is within bounds
	if metrics.ErrorBudget < 0 {
		metrics.ErrorBudget = 0
	} else if metrics.ErrorBudget > 100 {
		metrics.ErrorBudget = 100
	}

	// Calculate burn rate (simplified)
	if len(metrics.History) > 1 {
		recent := metrics.History[len(metrics.History)-1]
		previous := metrics.History[len(metrics.History)-2]
		timeDiff := recent.Timestamp.Sub(previous.Timestamp).Hours()

		if timeDiff > 0 {
			budgetChange := previous.Value - recent.Value
			metrics.BurnRate = budgetChange / timeDiff
		}
	}
}

// checkViolations checks for SLA violations
func (sm *SLAMonitor) checkViolations(slo SLO, metrics *SLAMetrics) {
	violated := false

	switch slo.Type {
	case SLOAvailability:
		violated = metrics.CurrentValue < slo.Target
	case SLOLatency:
		violated = metrics.CurrentValue > slo.Target
	case SLOErrorRate:
		violated = metrics.CurrentValue > slo.Target
	case SLOThroughput:
		violated = metrics.CurrentValue < slo.Target
	}

	if violated {
		violation := SLAViolation{
			ID:          fmt.Sprintf("violation-%d", time.Now().Unix()),
			SLOName:     slo.Name,
			Timestamp:   time.Now(),
			Severity:    sm.determineSeverity(metrics, slo),
			ActualValue: metrics.CurrentValue,
			TargetValue: slo.Target,
			Description: fmt.Sprintf("SLO %s violated: actual=%.2f, target=%.2f", slo.Name, metrics.CurrentValue, slo.Target),
			Resolved:    false,
		}

		metrics.Violations = append(metrics.Violations, violation)

		// Trigger alert
		sm.alerts.TriggerAlert(Alert{
			ID:        violation.ID,
			Type:      "sla_violation",
			Severity:  violation.Severity,
			Message:   violation.Description,
			Timestamp: violation.Timestamp,
			Metadata: map[string]any{
				"slo_name":     slo.Name,
				"actual_value": metrics.CurrentValue,
				"target_value": slo.Target,
				"error_budget": metrics.ErrorBudget,
			},
		})
	}
}

// determineSeverity determines violation severity
func (sm *SLAMonitor) determineSeverity(metrics *SLAMetrics, slo SLO) Severity {
	if slo.Critical {
		return SeverityCritical
	}

	if metrics.ErrorBudget < sm.config.Thresholds.CriticalThreshold {
		return SeverityCritical
	} else if metrics.ErrorBudget < sm.config.Thresholds.WarningThreshold {
		return SeverityWarning
	}

	return SeverityInfo
}

// updateSLAStatus updates SLA status
func (sm *SLAMonitor) updateSLAStatus(metrics *SLAMetrics, _ SLO) {
	switch {
	case metrics.ErrorBudget < sm.config.Thresholds.CriticalThreshold:
		metrics.Status = SLAStatusCritical
	case metrics.ErrorBudget < sm.config.Thresholds.WarningThreshold:
		metrics.Status = SLAStatusWarning
	default:
		metrics.Status = SLAStatusHealthy
	}

	// Check for active violations
	activeViolations := 0
	for _, violation := range metrics.Violations {
		if !violation.Resolved {
			activeViolations++
		}
	}

	if activeViolations > 0 {
		metrics.Status = SLAStatusViolated
	}
}

// startAlertProcessing starts alert processing
func (sm *SLAMonitor) startAlertProcessing(ctx context.Context) {
	for {
		select {
		case alert := <-sm.alerts.GetAlertChannel():
			sm.processAlert(ctx, alert)
		case <-ctx.Done():
			return
		}
	}
}

// processAlert processes an alert
func (sm *SLAMonitor) processAlert(ctx context.Context, alert Alert) {
	// Find matching alert rules
	for _, rule := range sm.config.AlertRules {
		if sm.matchesAlertRule(alert, rule) {
			sm.executeAlertActions(ctx, alert, rule)
		}
	}
}

// matchesAlertRule checks if alert matches rule
func (sm *SLAMonitor) matchesAlertRule(alert Alert, rule AlertRule) bool {
	if !rule.Enabled {
		return false
	}

	if alert.Type == "sla_violation" {
		if sloName, ok := alert.Metadata["slo_name"].(string); ok {
			return sloName == rule.SLOName
		}
	}

	return false
}

// executeAlertActions executes alert actions
func (sm *SLAMonitor) executeAlertActions(ctx context.Context, alert Alert, rule AlertRule) {
	for _, action := range rule.Actions {
		switch action.Type {
		case "email":
			sm.sendEmailAlert(ctx, alert, action.Config)
		case "slack":
			sm.sendSlackAlert(ctx, alert, action.Config)
		case "webhook":
			sm.sendWebhookAlert(ctx, alert, action.Config)
		}
	}
}

// sendEmailAlert sends email alert
func (sm *SLAMonitor) sendEmailAlert(_ context.Context, _ Alert, _ map[string]any) {
	// Implementation would send email
}

// sendSlackAlert sends Slack alert
func (sm *SLAMonitor) sendSlackAlert(_ context.Context, _ Alert, _ map[string]any) {
	// Implementation would send Slack message
}

// sendWebhookAlert sends webhook alert
func (sm *SLAMonitor) sendWebhookAlert(_ context.Context, _ Alert, _ map[string]any) {
	// Implementation would send webhook
}

// startReportGeneration starts report generation
func (sm *SLAMonitor) startReportGeneration(ctx context.Context) {
	ticker := time.NewTicker(sm.config.Reporting.Frequency)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.generateReport(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// generateReport generates SLA report
func (sm *SLAMonitor) generateReport(ctx context.Context) {
	report := sm.reports.GenerateSLAReport(sm.GetMetrics())

	// Send report to recipients
	for _, recipient := range sm.config.Reporting.Recipients {
		sm.sendReport(ctx, recipient, report)
	}
}

// sendReport sends report to recipient
func (sm *SLAMonitor) sendReport(_ context.Context, _ string, _ []byte) {
	// Implementation would send report
}

// GetMetrics returns current SLA metrics
func (sm *SLAMonitor) GetMetrics() map[string]*SLAMetrics {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	metrics := make(map[string]*SLAMetrics)
	for name, metric := range sm.metrics {
		metricCopy := *metric
		metrics[name] = &metricCopy
	}

	return metrics
}

// GetSLOStatus returns status of a specific SLO
func (sm *SLAMonitor) GetSLOStatus(sloName string) (*SLAMetrics, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	metrics, exists := sm.metrics[sloName]
	if !exists {
		return nil, fmt.Errorf("SLO not found: %s", sloName)
	}

	metricsCopy := *metrics
	return &metricsCopy, nil
}

// Alert represents an alert
type Alert struct {
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Severity  Severity       `json:"severity"`
	Message   string         `json:"message"`
}

// AlertManager manages alerts
type AlertManager struct {
	alertChannel chan Alert
}

// NewAlertManager creates a new alert manager
func NewAlertManager() *AlertManager {
	return &AlertManager{
		alertChannel: make(chan Alert, 100),
	}
}

// TriggerAlert triggers an alert
func (am *AlertManager) TriggerAlert(alert Alert) {
	select {
	case am.alertChannel <- alert:
	default:
		// Channel full, drop alert
	}
}

// GetAlertChannel returns the alert channel
func (am *AlertManager) GetAlertChannel() <-chan Alert {
	return am.alertChannel
}

// ReportGenerator generates reports
type ReportGenerator struct{}

// NewReportGenerator creates a new report generator
func NewReportGenerator() *ReportGenerator {
	return &ReportGenerator{}
}

// GenerateSLAReport generates an SLA report
func (rg *ReportGenerator) GenerateSLAReport(metrics map[string]*SLAMetrics) []byte {
	report := map[string]any{
		"timestamp": time.Now(),
		"metrics":   metrics,
		"summary": map[string]any{
			"total_slos":    len(metrics),
			"healthy_slos":  rg.countByStatus(metrics, SLAStatusHealthy),
			"warning_slos":  rg.countByStatus(metrics, SLAStatusWarning),
			"critical_slos": rg.countByStatus(metrics, SLAStatusCritical),
			"violated_slos": rg.countByStatus(metrics, SLAStatusViolated),
		},
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		// Fallback to error message if marshaling fails
		return []byte(fmt.Sprintf(`{"error": "failed to marshal report: %v"}`, err))
	}
	return data
}

// countByStatus counts metrics by status
func (rg *ReportGenerator) countByStatus(metrics map[string]*SLAMetrics, status SLAStatus) int {
	count := 0
	for _, metric := range metrics {
		if metric.Status == status {
			count++
		}
	}
	return count
}
