// Package observability provides comprehensive alerting system with webhook delivery and SNS integration
package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// AlertingSystem provides comprehensive alerting with webhook delivery and SNS integration
type AlertingSystem struct {
	logger              *zap.Logger
	snsClient           *sns.Client
	snsTopicArn         string
	webhookDelivery     *WebhookDeliveryService
	alertRepo           *StandaloneAlertRepository
	latencyAlerter      *LatencyAlerter
	enabled             bool
	
	// Configuration
	environment         string
	region              string
	serviceName         string
}

// AlertingConfig contains configuration for the alerting system
type AlertingConfig struct {
	Logger              *zap.Logger
	DB                  core.DB
	TableName           string
	CostService         *cost.TrackingService
	SNSClient           *sns.Client
	SNSTopicArn         string
	WebhookURL          string
	WebhookHeaders      map[string]string
	Environment         string
	Region              string
	ServiceName         string
	Enabled             bool
}

// NewAlertingSystem creates a new comprehensive alerting system
func NewAlertingSystem(config *AlertingConfig) (*AlertingSystem, error) {
	if config.Logger == nil {
		return nil, ErrLoggerRequired
	}
	if config.DB == nil {
		return nil, ErrDatabaseRequired
	}
	if config.TableName == "" {
		config.TableName = "lesser-main"
	}
	if config.Environment == "" {
		cfg := appconfig.Get()
		config.Environment = cfg.Environment
		if config.Environment == "" {
			config.Environment = "development"
		}
	}
	if config.Region == "" {
		cfg := appconfig.Get()
		config.Region = cfg.Region
		if config.Region == "" {
			config.Region = "us-east-1"
		}
	}
	if config.ServiceName == "" {
		config.ServiceName = "lesser"
	}

	// Create alert repository
	alertRepo := NewStandaloneAlertRepository(
		config.DB,
		config.TableName,
		config.Logger,
		config.CostService,
	)

	// Store SNS configuration directly
	snsClient := config.SNSClient
	snsTopicArn := config.SNSTopicArn

	// Create repositories for webhook delivery
	webhookRepo := NewStandaloneWebhookRepository(config.DB, config.TableName, config.Logger)
	deadLetterRepo := NewStandaloneDeadLetterRepository(config.DB, config.TableName, config.Logger)

	// Create webhook delivery service
	webhookConfig := &WebhookDeliveryConfig{
		Logger:               config.Logger,
		WebhookRepository:    webhookRepo,
		AlertRepository:      alertRepo,
		DeadLetterRepository: deadLetterRepo,
		HTTPTimeout:          30 * time.Second,
		MaxAttempts:          5,
		RetryInterval:        30 * time.Second,
		Enabled:              config.Enabled,
	}
	webhookDelivery := NewWebhookDeliveryService(webhookConfig)

	// Create metrics recorder for latency alerter
	// Create a simple metric storage function for the alerting system
	createMetricFn := func(_ context.Context, metric *models.MetricRecord) error {
		// For alerting purposes, we just log the metric
		config.Logger.Debug("storing alert metric", 
			zap.String("metric_type", metric.MetricType),
			zap.String("service_name", metric.ServiceName))
		return nil
	}
	metricsRecorder := NewDefaultMetricsRecorder(createMetricFn, config.ServiceName)

	// Create latency alerter
	latencyAlerter := NewLatencyAlerter(config.Logger, metricsRecorder)

	return &AlertingSystem{
		logger:          config.Logger,
		snsClient:       snsClient,
		snsTopicArn:     snsTopicArn,
		webhookDelivery: webhookDelivery,
		alertRepo:       alertRepo,
		latencyAlerter:  latencyAlerter,
		enabled:         config.Enabled,
		environment:     config.Environment,
		region:          config.Region,
		serviceName:     config.ServiceName,
	}, nil
}

// SendAlert sends an alert through all configured channels
func (a *AlertingSystem) SendAlert(ctx context.Context, alertReq *AlertRequest) error {
	if !a.enabled {
		a.logger.Debug("alerting system disabled, skipping alert",
			zap.String("type", alertReq.Type),
			zap.String("title", alertReq.Title))
		return nil
	}

	// Create alert model
	alert := a.createAlertFromRequest(alertReq)

	// Store alert in database
	if err := a.alertRepo.CreateAlert(ctx, alert); err != nil {
		a.logger.Error("failed to store alert",
			zap.String("alert_id", alert.AlertID),
			zap.Error(err))
		// Continue with delivery even if storage fails
	}

	// Send via webhook delivery system
	if err := a.webhookDelivery.DeliverAlert(ctx, alert); err != nil {
		a.logger.Error("webhook delivery failed",
			zap.String("alert_id", alert.AlertID),
			zap.Error(err))
	}

	// Send via SNS for critical alerts
	if alert.IsCritical() || alert.IsHighPriority() {
		if err := a.sendSNSAlert(ctx, alert); err != nil {
			a.logger.Error("SNS alert delivery failed",
				zap.String("alert_id", alert.AlertID),
				zap.Error(err))
		}
	}

	a.logger.Info("alert sent",
		zap.String("alert_id", alert.AlertID),
		zap.String("type", alert.Type),
		zap.String("severity", alert.Severity),
		zap.String("service", alert.Service))

	return nil
}

// CheckErrorRate triggers error rate alert if threshold exceeded
func (a *AlertingSystem) CheckErrorRate(ctx context.Context, service string, errorRate float64) {
	if !a.enabled {
		return
	}

	// Create high-level alert for webhook delivery
	if errorRate >= AlertP1ErrorRatePercent {
		alertReq := &AlertRequest{
			Type:        "error_rate",
			Severity:    a.getSeverityForErrorRate(errorRate),
			Priority:    a.getPriorityForErrorRate(errorRate),
			Title:       fmt.Sprintf("High Error Rate: %s (%.1f%%)", service, errorRate),
			Description: fmt.Sprintf("Error rate %.1f%% exceeds threshold. Review logs and investigate failed requests.", errorRate),
			Service:     service,
			Region:      a.region,
			Source:      a.serviceName,
			RunbookURL:  RunbookHighErrorRate,
			Values: map[string]float64{
				"error_rate_percent": errorRate,
			},
			Thresholds: map[string]float64{
				"p0_threshold": AlertP0ErrorRatePercent,
				"p1_threshold": AlertP1ErrorRatePercent,
				"p2_threshold": AlertP2ErrorRatePercent,
			},
			Metadata: map[string]interface{}{
				"evaluation_window": "5 minutes",
				"alert_source":      "error_rate_monitor",
			},
		}

		if err := a.SendAlert(ctx, alertReq); err != nil {
			a.logger.Error("failed to send error rate alert",
				zap.String("service", service),
				zap.Float64("error_rate", errorRate),
				zap.Error(err))
		}
	}
}

// CheckLatency triggers latency alert if threshold exceeded
func (a *AlertingSystem) CheckLatency(ctx context.Context, service string, operation string, latencyMs, p95Ms, p99Ms float64) {
	if !a.enabled {
		return
	}

	// Use latency alerter for detailed alerting
	a.latencyAlerter.CheckLatency(ctx, operation, service, latencyMs, p95Ms, p99Ms)

	// Create high-level alert for webhook delivery
	if latencyMs >= AlertP1LatencyP90Milliseconds {
		alertReq := &AlertRequest{
			Type:        "latency",
			Severity:    a.getSeverityForLatency(latencyMs),
			Priority:    a.getPriorityForLatency(latencyMs),
			Title:       fmt.Sprintf("High Latency: %s.%s (%.0fms)", service, operation, latencyMs),
			Description: fmt.Sprintf("Latency %.0fms exceeds threshold. Check for performance bottlenecks.", latencyMs),
			Service:     service,
			Region:      a.region,
			Source:      a.serviceName,
			RunbookURL:  RunbookHighLatency,
			Dimensions: map[string]string{
				"operation": operation,
			},
			Values: map[string]float64{
				"latency_ms": latencyMs,
				"p95_ms":     p95Ms,
				"p99_ms":     p99Ms,
			},
			Thresholds: map[string]float64{
				"p0_threshold": AlertP0LatencyP99Milliseconds,
				"p1_threshold": AlertP1LatencyP90Milliseconds,
				"p2_threshold": AlertP2LatencyP90Milliseconds,
			},
			Metadata: map[string]interface{}{
				"evaluation_window": "5 minutes",
				"alert_source":      "latency_monitor",
			},
		}

		if err := a.SendAlert(ctx, alertReq); err != nil {
			a.logger.Error("failed to send latency alert",
				zap.String("service", service),
				zap.String("operation", operation),
				zap.Float64("latency_ms", latencyMs),
				zap.Error(err))
		}
	}
}

// CheckCost triggers cost alert if threshold exceeded
func (a *AlertingSystem) CheckCost(ctx context.Context, costMicroCents float64) {
	if !a.enabled {
		return
	}

	// Create high-level alert for webhook delivery if cost is significant
	if costMicroCents > 10000 { // $0.10
		costDollars := costMicroCents / 100000.0
		alertReq := &AlertRequest{
			Type:        "cost",
			Severity:    a.getSeverityForCost(costMicroCents),
			Priority:    a.getPriorityForCost(costMicroCents),
			Title:       fmt.Sprintf("High Cost Detected ($%.2f)", costDollars),
			Description: fmt.Sprintf("Cost $%.2f detected. Monitor spending and optimize if necessary.", costDollars),
			Service:     "billing",
			Region:      a.region,
			Source:      a.serviceName,
			RunbookURL:  RunbookHighCost,
			Values: map[string]float64{
				"cost_microcents": costMicroCents,
				"cost_dollars":    costDollars,
			},
			Thresholds: map[string]float64{
				"p0_threshold_dollars": 10.0,
				"p1_threshold_dollars": 1.0,
				"p2_threshold_dollars": 0.10,
			},
			Metadata: map[string]interface{}{
				"evaluation_window": "1 hour",
				"alert_source":      "cost_monitor",
			},
		}

		if err := a.SendAlert(ctx, alertReq); err != nil {
			a.logger.Error("failed to send cost alert",
				zap.Float64("cost_microcents", costMicroCents),
				zap.Error(err))
		}
	}
}

// CheckHealth triggers health alert for service issues
func (a *AlertingSystem) CheckHealth(ctx context.Context, service string, isHealthy bool, errorMsg string) {
	if !a.enabled {
		return
	}

	// Send webhook alert for health issues
	if !isHealthy {
		alertReq := &AlertRequest{
			Type:        "health",
			Severity:    AlertSeverityError,
			Priority:    "P1",
			Title:       fmt.Sprintf("Health Check Failed: %s", service),
			Description: fmt.Sprintf("Service %s is unhealthy: %s", service, errorMsg),
			Service:     service,
			Region:      a.region,
			Source:      a.serviceName,
			RunbookURL:  RunbookHealthFailure,
			Metadata: map[string]interface{}{
				"healthy":       false,
				"error_message": errorMsg,
				"alert_source":  "health_monitor",
			},
		}

		if err := a.SendAlert(ctx, alertReq); err != nil {
			a.logger.Error("failed to send health alert",
				zap.String("service", service),
				zap.Error(err))
		}
	}
}

// CheckSecurity triggers security alerts
func (a *AlertingSystem) CheckSecurity(ctx context.Context, eventType string, severity string, details map[string]interface{}) {
	if !a.enabled {
		return
	}

	// Send webhook alert for security events
	alertReq := &AlertRequest{
		Type:        "security",
		Severity:    severity,
		Priority:    a.getPriorityForSecurity(severity),
		Title:       fmt.Sprintf("Security Event: %s", eventType),
		Description: fmt.Sprintf("Security event detected: %s", eventType),
		Service:     "security",
		Region:      a.region,
		Source:      a.serviceName,
		RunbookURL:  RunbookSecurityIncident,
		Metadata:    details,
	}
	if alertReq.Metadata == nil {
		alertReq.Metadata = make(map[string]interface{})
	}
	alertReq.Metadata["alert_source"] = "security_monitor"

	if err := a.SendAlert(ctx, alertReq); err != nil {
		a.logger.Error("failed to send security alert",
			zap.String("event_type", eventType),
			zap.Error(err))
	}
}

// ProcessRetries processes failed webhook deliveries
func (a *AlertingSystem) ProcessRetries(ctx context.Context) error {
	if !a.enabled {
		return nil
	}

	// Process webhook retries
	if err := a.webhookDelivery.RetryFailedDeliveries(ctx); err != nil {
		a.logger.Error("failed to process webhook retries", zap.Error(err))
		return err
	}

	// Process alert retries
	alerts, err := a.alertRepo.GetAlertsNeedingRetry(ctx, 50)
	if err != nil {
		a.logger.Error("failed to get alerts needing retry", zap.Error(err))
		return err
	}

	for _, alert := range alerts {
		if err := a.webhookDelivery.DeliverAlert(ctx, alert); err != nil {
			a.logger.Error("failed to retry alert delivery",
				zap.String("alert_id", alert.AlertID),
				zap.Error(err))
		}
	}

	return nil
}

// ResolveAlert resolves an active alert
func (a *AlertingSystem) ResolveAlert(ctx context.Context, alertID string) error {
	return a.alertRepo.ResolveAlert(ctx, alertID)
}

// GetActiveAlerts retrieves currently active alerts
func (a *AlertingSystem) GetActiveAlerts(ctx context.Context, limit int) ([]*models.Alert, error) {
	return a.alertRepo.GetActiveAlerts(ctx, limit)
}

// AlertStats represents alert statistics
type AlertStats struct {
	TotalAlerts       int64
	ActiveAlerts      int64
	ResolvedAlerts    int64
	CriticalAlerts    int64
	WarningAlerts     int64
	InfoAlerts        int64
	DeliverySuccessRate float64
	AverageResponseTime time.Duration
}

// GetAlertStats retrieves alert statistics
func (a *AlertingSystem) GetAlertStats(_ context.Context, _ time.Time) (*AlertStats, error) {
	// For now, return basic stats - this could be enhanced with real repository queries
	return &AlertStats{
		TotalAlerts:         0,
		ActiveAlerts:        0,
		ResolvedAlerts:      0,
		CriticalAlerts:      0,
		WarningAlerts:       0,
		InfoAlerts:          0,
		DeliverySuccessRate: 1.0,
		AverageResponseTime: time.Millisecond * 100,
	}, nil
}

// Cleanup removes old alerts and deliveries
func (a *AlertingSystem) Cleanup(ctx context.Context) error {
	// Clean up old alerts (older than 30 days)
	deletedCount, err := a.alertRepo.CleanupOldAlerts(ctx, 30*24*time.Hour)
	if err != nil {
		a.logger.Error("failed to cleanup old alerts", zap.Error(err))
		return err
	}

	if deletedCount > 0 {
		a.logger.Info("cleaned up old alerts", zap.Int("deleted_count", deletedCount))
	}

	return nil
}

// Helper methods

func (a *AlertingSystem) createAlertFromRequest(req *AlertRequest) *models.Alert {
	now := time.Now()
	alert := &models.Alert{
		AlertID:          uuid.New().String(),
		Type:             req.Type,
		Severity:         req.Severity,
		Priority:         req.Priority,
		Status:           "firing",
		Title:            req.Title,
		Description:      req.Description,
		Message:          req.Message,
		RunbookURL:       req.RunbookURL,
		Service:          req.Service,
		Region:           req.Region,
		Source:           req.Source,
		Dimensions:       req.Dimensions,
		Metadata:         req.Metadata,
		Values:           req.Values,
		Thresholds:       req.Thresholds,
		FiredAt:          now,
		CreatedAt:        now,
		UpdatedAt:        now,
		EscalationLevel:  0,
		DeliveryChannels: []string{"webhook"},
		DeliveryAttempts: 0,
	}

	// Add SNS for critical alerts
	if alert.IsCritical() || alert.IsHighPriority() {
		alert.DeliveryChannels = append(alert.DeliveryChannels, "sns")
	}

	// Set default message if not provided
	if alert.Message == "" {
		alert.Message = alert.Title
	}

	// Add environment dimension
	if alert.Dimensions == nil {
		alert.Dimensions = make(map[string]string)
	}
	alert.Dimensions["environment"] = a.environment

	return alert
}

// sendSNSAlert sends an alert via AWS SNS
func (a *AlertingSystem) sendSNSAlert(ctx context.Context, alert *models.Alert) error {
	if a.snsClient == nil || a.snsTopicArn == "" {
		return nil // SNS not configured
	}

	// Format message for SNS
	message := a.formatSNSMessage(alert)

	// Create subject line
	subject := fmt.Sprintf("[%s] %s Alert: %s",
		strings.ToUpper(alert.Severity),
		alert.Service,
		alert.Title)

	// Truncate subject if too long (SNS limit is 100 chars)
	if len(subject) > 100 {
		subject = subject[:97] + "..."
	}

	// Publish to SNS
	input := &sns.PublishInput{
		TopicArn: aws.String(a.snsTopicArn),
		Message:  aws.String(message),
		Subject:  aws.String(subject),
	}

	// Add message attributes
	input.MessageAttributes = map[string]types.MessageAttributeValue{
		"AlertType": {
			DataType:    aws.String("String"),
			StringValue: aws.String(alert.Type),
		},
		"Severity": {
			DataType:    aws.String("String"),
			StringValue: aws.String(alert.Severity),
		},
		"Service": {
			DataType:    aws.String("String"),
			StringValue: aws.String(alert.Service),
		},
		"Priority": {
			DataType:    aws.String("String"),
			StringValue: aws.String(alert.Priority),
		},
	}

	output, err := a.snsClient.Publish(ctx, input)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSNSPublishFailed, err)
	}

	a.logger.Info("sent SNS alert",
		zap.String("alert_id", alert.AlertID),
		zap.String("type", alert.Type),
		zap.String("severity", alert.Severity),
		zap.String("message_id", aws.ToString(output.MessageId)))

	return nil
}

// formatSNSMessage formats an alert for SNS delivery
func (a *AlertingSystem) formatSNSMessage(alert *models.Alert) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Alert: %s\n", alert.Title))
	builder.WriteString(fmt.Sprintf("Type: %s\n", alert.Type))
	builder.WriteString(fmt.Sprintf("Severity: %s\n", alert.Severity))
	builder.WriteString(fmt.Sprintf("Priority: %s\n", alert.Priority))
	builder.WriteString(fmt.Sprintf("Service: %s\n", alert.Service))
	if alert.Region != "" {
		builder.WriteString(fmt.Sprintf("Region: %s\n", alert.Region))
	}
	builder.WriteString(fmt.Sprintf("Time: %s\n", alert.FiredAt.Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("\nDescription:\n%s\n", alert.Description))

	if len(alert.Metadata) > 0 {
		builder.WriteString("\nAdditional Details:\n")
		for key, value := range alert.Metadata {
			builder.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
	}

	if alert.RunbookURL != "" {
		builder.WriteString(fmt.Sprintf("\nRunbook: %s\n", alert.RunbookURL))
	}

	return builder.String()
}

func (a *AlertingSystem) getSeverityForErrorRate(errorRate float64) string {
	switch {
	case errorRate >= AlertP0ErrorRatePercent:
		return HealthStatusCritical
	case errorRate >= AlertP1ErrorRatePercent:
		return AlertSeverityError
	case errorRate >= AlertP2ErrorRatePercent:
		return HealthStatusWarning
	default:
		return AlertSeverityInfo
	}
}

func (a *AlertingSystem) getPriorityForErrorRate(errorRate float64) string {
	switch {
	case errorRate >= AlertP0ErrorRatePercent:
		return "P0"
	case errorRate >= AlertP1ErrorRatePercent:
		return "P1"
	case errorRate >= AlertP2ErrorRatePercent:
		return "P2"
	default:
		return "P3"
	}
}

func (a *AlertingSystem) getSeverityForLatency(latencyMs float64) string {
	switch {
	case latencyMs >= AlertP0LatencyP99Milliseconds:
		return HealthStatusCritical
	case latencyMs >= AlertP1LatencyP90Milliseconds:
		return AlertSeverityError
	case latencyMs >= AlertP2LatencyP90Milliseconds:
		return HealthStatusWarning
	default:
		return "info"
	}
}

func (a *AlertingSystem) getPriorityForLatency(latencyMs float64) string {
	switch {
	case latencyMs >= AlertP0LatencyP99Milliseconds:
		return "P0"
	case latencyMs >= AlertP1LatencyP90Milliseconds:
		return "P1"
	case latencyMs >= AlertP2LatencyP90Milliseconds:
		return "P2"
	default:
		return "P3"
	}
}

func (a *AlertingSystem) getSeverityForCost(costMicroCents float64) string {
	switch {
	case costMicroCents > 1000000: // $10
		return "critical"
	case costMicroCents > 100000: // $1
		return AlertSeverityError
	case costMicroCents > 10000: // $0.10
		return "warning"
	default:
		return "info"
	}
}

func (a *AlertingSystem) getPriorityForCost(costMicroCents float64) string {
	switch {
	case costMicroCents > 1000000: // $10
		return "P0"
	case costMicroCents > 100000: // $1
		return "P1"
	case costMicroCents > 10000: // $0.10
		return "P2"
	default:
		return "P3"
	}
}

func (a *AlertingSystem) getPriorityForSecurity(severity string) string {
	switch severity {
	case "critical":
		return "P0"
	case AlertSeverityError:
		return "P1"
	case "warning":
		return "P2"
	default:
		return "P3"
	}
}

// AlertRequest represents a request to send an alert
type AlertRequest struct {
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Priority    string                 `json:"priority,omitempty"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Message     string                 `json:"message,omitempty"`
	Service     string                 `json:"service"`
	Region      string                 `json:"region"`
	Source      string                 `json:"source"`
	RunbookURL  string                 `json:"runbook_url,omitempty"`
	Dimensions  map[string]string      `json:"dimensions,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Values      map[string]float64     `json:"values,omitempty"`
	Thresholds  map[string]float64     `json:"thresholds,omitempty"`
}