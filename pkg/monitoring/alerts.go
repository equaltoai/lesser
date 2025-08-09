// Package monitoring provides production alerting and monitoring capabilities for serverless applications.
package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"go.uber.org/zap"
)

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	// SeverityInfo represents informational alerts
	SeverityInfo AlertSeverity = "info"
	// SeverityWarning represents warning level alerts
	SeverityWarning AlertSeverity = "warning"
	// SeverityError represents error level alerts
	SeverityError AlertSeverity = "error"
	// SeverityCritical represents critical alerts requiring immediate attention
	SeverityCritical AlertSeverity = "critical"
)

// AlertType represents the type of alert
type AlertType string

const (
	// AlertTypeErrorRate represents high error rate alerts
	AlertTypeErrorRate AlertType = "error_rate"
	// AlertTypeLatency represents high latency alerts
	AlertTypeLatency AlertType = "latency"
	// AlertTypeCost represents cost threshold alerts
	AlertTypeCost AlertType = "cost"
	// AlertTypeHealth represents health check failure alerts
	AlertTypeHealth AlertType = "health"
	// AlertTypeSecurity represents security-related alerts
	AlertTypeSecurity AlertType = "security"
	// AlertTypeCapacity represents capacity/scaling alerts
	AlertTypeCapacity AlertType = "capacity"
)

// Alert represents an alert to be sent
type Alert struct {
	Type        AlertType     `json:"type"`
	Severity    AlertSeverity `json:"severity"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Service     string        `json:"service"`
	Region      string        `json:"region"`
	Timestamp   time.Time     `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata"`
	Source      string        `json:"source"`
}

// WebhookConfig contains webhook endpoint configuration
type WebhookConfig struct {
	URL     string
	Headers map[string]string
	Timeout time.Duration
}

// AlertManager handles production alerting
type AlertManager struct {
	logger        *zap.Logger
	snsClient     *sns.Client
	snsTopicArn   string
	webhookConfig *WebhookConfig
	httpClient    *http.Client
	rateLimiter   *AlertRateLimiter
	enabled       bool
}

// AlertManagerConfig contains configuration for the alert manager
type AlertManagerConfig struct {
	Logger        *zap.Logger
	SNSClient     *sns.Client
	SNSTopicArn   string
	WebhookURL    string
	WebhookHeaders map[string]string
	Enabled       bool
}

// AlertRateLimiter prevents alert flooding
type AlertRateLimiter struct {
	mu        sync.RWMutex
	alerts    map[string]time.Time
	threshold time.Duration
}

// NewAlertRateLimiter creates a new rate limiter for alerts
func NewAlertRateLimiter(threshold time.Duration) *AlertRateLimiter {
	return &AlertRateLimiter{
		alerts:    make(map[string]time.Time),
		threshold: threshold,
	}
}

// ShouldAlert checks if an alert should be sent based on rate limiting
func (rl *AlertRateLimiter) ShouldAlert(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if lastAlert, exists := rl.alerts[key]; exists {
		if time.Since(lastAlert) < rl.threshold {
			return false
		}
	}

	rl.alerts[key] = time.Now()
	return true
}

// NewAlertManager creates a new alert manager
func NewAlertManager(logger *zap.Logger) *AlertManager {
	// Basic initialization for backward compatibility
	return &AlertManager{
		logger:      logger,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		rateLimiter: NewAlertRateLimiter(5 * time.Minute), // Prevent duplicate alerts within 5 minutes
		enabled:     true,
	}
}

// NewAlertManagerWithConfig creates a new alert manager with full configuration
func NewAlertManagerWithConfig(config *AlertManagerConfig) *AlertManager {
	am := &AlertManager{
		logger:      config.Logger,
		snsClient:   config.SNSClient,
		snsTopicArn: config.SNSTopicArn,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		rateLimiter: NewAlertRateLimiter(5 * time.Minute),
		enabled:     config.Enabled,
	}

	// Configure webhook if provided
	if config.WebhookURL != "" {
		am.webhookConfig = &WebhookConfig{
			URL:     config.WebhookURL,
			Headers: config.WebhookHeaders,
			Timeout: 10 * time.Second,
		}
		if am.webhookConfig.Headers == nil {
			am.webhookConfig.Headers = make(map[string]string)
		}
		// Set default content type if not specified
		if _, exists := am.webhookConfig.Headers["Content-Type"]; !exists {
			am.webhookConfig.Headers["Content-Type"] = "application/json"
		}
	}

	// Try to configure from environment if not provided
	if am.snsTopicArn == "" {
		am.snsTopicArn = os.Getenv("ALERT_SNS_TOPIC_ARN")
	}
	if am.webhookConfig == nil && os.Getenv("ALERT_WEBHOOK_URL") != "" {
		am.webhookConfig = &WebhookConfig{
			URL:     os.Getenv("ALERT_WEBHOOK_URL"),
			Headers: map[string]string{"Content-Type": "application/json"},
			Timeout: 10 * time.Second,
		}
	}

	return am
}

// SendAlert sends an alert through configured channels (webhook and/or SNS)
func (am *AlertManager) SendAlert(ctx context.Context, alert *Alert) error {
	if !am.enabled {
		am.logger.Debug("alerting disabled, skipping alert",
			zap.String("type", string(alert.Type)),
			zap.String("title", alert.Title))
		return nil
	}

	// Generate unique key for rate limiting
	alertKey := fmt.Sprintf("%s:%s:%s", alert.Type, alert.Service, alert.Severity)
	if !am.rateLimiter.ShouldAlert(alertKey) {
		am.logger.Debug("alert rate limited",
			zap.String("key", alertKey),
			zap.String("title", alert.Title))
		return nil
	}

	// Set timestamp if not provided
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now()
	}

	// Set source if not provided
	if alert.Source == "" {
		alert.Source = "lesser-monitoring"
	}

	var lastErr error
	alertSent := false

	// Send via webhook if configured
	if am.webhookConfig != nil && am.webhookConfig.URL != "" {
		if err := am.sendWebhook(ctx, alert); err != nil {
			am.logger.Error("failed to send webhook alert",
				zap.Error(err),
				zap.String("alert_type", string(alert.Type)))
			lastErr = err
		} else {
			alertSent = true
		}
	}

	// Send via SNS if configured
	if am.snsClient != nil && am.snsTopicArn != "" {
		if err := am.sendSNS(ctx, alert); err != nil {
			am.logger.Error("failed to send SNS alert",
				zap.Error(err),
				zap.String("alert_type", string(alert.Type)))
			lastErr = err
		} else {
			alertSent = true
		}
	}

	if !alertSent && lastErr != nil {
		return fmt.Errorf("failed to send alert via any channel: %w", lastErr)
	}

	if !alertSent {
		am.logger.Warn("no alert channels configured",
			zap.String("alert_type", string(alert.Type)),
			zap.String("title", alert.Title))
	}

	return nil
}

// sendWebhook sends an alert via webhook
func (am *AlertManager) sendWebhook(ctx context.Context, alert *Alert) error {
	// Marshal alert to JSON
	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", am.webhookConfig.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	// Set headers
	for key, value := range am.webhookConfig.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := am.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-2xx status: %d", resp.StatusCode)
	}

	am.logger.Info("sent webhook alert",
		zap.String("type", string(alert.Type)),
		zap.String("severity", string(alert.Severity)),
		zap.String("title", alert.Title))

	return nil
}

// sendSNS sends an alert via AWS SNS
func (am *AlertManager) sendSNS(ctx context.Context, alert *Alert) error {
	// Format message for SNS
	message := am.formatSNSMessage(alert)

	// Create subject line
	subject := fmt.Sprintf("[%s] %s Alert: %s", 
		strings.ToUpper(string(alert.Severity)),
		alert.Service,
		alert.Title)

	// Truncate subject if too long (SNS limit is 100 chars)
	if len(subject) > 100 {
		subject = subject[:97] + "..."
	}

	// Publish to SNS
	input := &sns.PublishInput{
		TopicArn: aws.String(am.snsTopicArn),
		Message:  aws.String(message),
		Subject:  aws.String(subject),
	}

	// Add message attributes
	input.MessageAttributes = map[string]types.MessageAttributeValue{
		"AlertType": {
			DataType:    aws.String("String"),
			StringValue: aws.String(string(alert.Type)),
		},
		"Severity": {
			DataType:    aws.String("String"),
			StringValue: aws.String(string(alert.Severity)),
		},
		"Service": {
			DataType:    aws.String("String"),
			StringValue: aws.String(alert.Service),
		},
	}

	output, err := am.snsClient.Publish(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to publish to SNS: %w", err)
	}

	am.logger.Info("sent SNS alert",
		zap.String("type", string(alert.Type)),
		zap.String("severity", string(alert.Severity)),
		zap.String("message_id", aws.ToString(output.MessageId)))

	return nil
}

// formatSNSMessage formats an alert for SNS delivery
func (am *AlertManager) formatSNSMessage(alert *Alert) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Alert: %s\n", alert.Title))
	builder.WriteString(fmt.Sprintf("Type: %s\n", alert.Type))
	builder.WriteString(fmt.Sprintf("Severity: %s\n", alert.Severity))
	builder.WriteString(fmt.Sprintf("Service: %s\n", alert.Service))
	if alert.Region != "" {
		builder.WriteString(fmt.Sprintf("Region: %s\n", alert.Region))
	}
	builder.WriteString(fmt.Sprintf("Time: %s\n", alert.Timestamp.Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("\nDescription:\n%s\n", alert.Description))

	if len(alert.Metadata) > 0 {
		builder.WriteString("\nAdditional Details:\n")
		for key, value := range alert.Metadata {
			builder.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
	}

	return builder.String()
}

// CheckErrorRate triggers alert if error rate is too high
func (am *AlertManager) CheckErrorRate(ctx context.Context, errorRate float64) {
	var severity AlertSeverity
	var threshold float64

	// Determine severity based on error rate
	switch {
	case errorRate > 10.0:
		severity = SeverityCritical
		threshold = 10.0
	case errorRate > 5.0:
		severity = SeverityError
		threshold = 5.0
	case errorRate > 2.0:
		severity = SeverityWarning
		threshold = 2.0
	default:
		// No alert needed
		return
	}

	alert := &Alert{
		Type:        AlertTypeErrorRate,
		Severity:    severity,
		Title:       fmt.Sprintf("High Error Rate Detected (%.1f%%)", errorRate),
		Description: fmt.Sprintf("Error rate %.1f%% exceeds threshold of %.1f%%", errorRate, threshold),
		Service:     "api",
		Region:      os.Getenv("AWS_REGION"),
		Metadata: map[string]interface{}{
			"error_rate_percent": errorRate,
			"threshold_percent":  threshold,
		},
	}

	if err := am.SendAlert(ctx, alert); err != nil {
		am.logger.Error("failed to send error rate alert",
			zap.Error(err),
			zap.Float64("error_rate", errorRate))
	}
}

// CheckLatency triggers alert if latency is too high
func (am *AlertManager) CheckLatency(ctx context.Context, latencyMs float64) {
	var severity AlertSeverity
	var threshold float64

	// Determine severity based on latency
	switch {
	case latencyMs > 5000:
		severity = SeverityCritical
		threshold = 5000
	case latencyMs > 2000:
		severity = SeverityError
		threshold = 2000
	case latencyMs > 1000:
		severity = SeverityWarning
		threshold = 1000
	default:
		// No alert needed
		return
	}

	alert := &Alert{
		Type:        AlertTypeLatency,
		Severity:    severity,
		Title:       fmt.Sprintf("High Latency Detected (%.0fms)", latencyMs),
		Description: fmt.Sprintf("Latency %.0fms exceeds threshold of %.0fms", latencyMs, threshold),
		Service:     "api",
		Region:      os.Getenv("AWS_REGION"),
		Metadata: map[string]interface{}{
			"latency_ms":   latencyMs,
			"threshold_ms": threshold,
		},
	}

	if err := am.SendAlert(ctx, alert); err != nil {
		am.logger.Error("failed to send latency alert",
			zap.Error(err),
			zap.Float64("latency_ms", latencyMs))
	}
}

// CheckCost triggers alert if cost is too high
func (am *AlertManager) CheckCost(ctx context.Context, costMicroCents float64) {
	var severity AlertSeverity
	var threshold float64

	// Determine severity based on cost (in microcents)
	switch {
	case costMicroCents > 1000000: // $10
		severity = SeverityCritical
		threshold = 1000000
	case costMicroCents > 100000: // $1
		severity = SeverityError
		threshold = 100000
	case costMicroCents > 10000: // $0.10
		severity = SeverityWarning
		threshold = 10000
	default:
		// No alert needed
		return
	}

	// Convert to dollars for display
	costDollars := costMicroCents / 100000.0
	thresholdDollars := threshold / 100000.0

	alert := &Alert{
		Type:        AlertTypeCost,
		Severity:    severity,
		Title:       fmt.Sprintf("High Cost Detected ($%.2f)", costDollars),
		Description: fmt.Sprintf("Cost $%.2f exceeds threshold of $%.2f", costDollars, thresholdDollars),
		Service:     "billing",
		Region:      os.Getenv("AWS_REGION"),
		Metadata: map[string]interface{}{
			"cost_microcents":      costMicroCents,
			"cost_dollars":         costDollars,
			"threshold_microcents": threshold,
			"threshold_dollars":    thresholdDollars,
		},
	}

	if err := am.SendAlert(ctx, alert); err != nil {
		am.logger.Error("failed to send cost alert",
			zap.Error(err),
			zap.Float64("cost_micro_cents", costMicroCents))
	}
}

// CheckHealth triggers alert for health check failures
func (am *AlertManager) CheckHealth(ctx context.Context, service string, isHealthy bool, errorMsg string) {
	if isHealthy {
		// No alert for healthy services
		return
	}

	alert := &Alert{
		Type:        AlertTypeHealth,
		Severity:    SeverityError,
		Title:       fmt.Sprintf("Health Check Failed: %s", service),
		Description: fmt.Sprintf("Service %s is unhealthy: %s", service, errorMsg),
		Service:     service,
		Region:      os.Getenv("AWS_REGION"),
		Metadata: map[string]interface{}{
			"healthy":      false,
			"error_message": errorMsg,
		},
	}

	if err := am.SendAlert(ctx, alert); err != nil {
		am.logger.Error("failed to send health alert",
			zap.Error(err),
			zap.String("service", service))
	}
}

// CheckSecurity triggers alert for security events
func (am *AlertManager) CheckSecurity(ctx context.Context, eventType string, severity AlertSeverity, details map[string]interface{}) {
	alert := &Alert{
		Type:        AlertTypeSecurity,
		Severity:    severity,
		Title:       fmt.Sprintf("Security Event: %s", eventType),
		Description: fmt.Sprintf("Security event detected: %s", eventType),
		Service:     "security",
		Region:      os.Getenv("AWS_REGION"),
		Metadata:    details,
	}

	if err := am.SendAlert(ctx, alert); err != nil {
		am.logger.Error("failed to send security alert",
			zap.Error(err),
			zap.String("event_type", eventType))
	}
}

// CheckCapacity triggers alert for capacity issues
func (am *AlertManager) CheckCapacity(ctx context.Context, resource string, utilization float64) {
	var severity AlertSeverity
	var threshold float64

	// Determine severity based on utilization percentage
	switch {
	case utilization > 95.0:
		severity = SeverityCritical
		threshold = 95.0
	case utilization > 85.0:
		severity = SeverityError
		threshold = 85.0
	case utilization > 75.0:
		severity = SeverityWarning
		threshold = 75.0
	default:
		// No alert needed
		return
	}

	alert := &Alert{
		Type:        AlertTypeCapacity,
		Severity:    severity,
		Title:       fmt.Sprintf("High %s Utilization (%.1f%%)", resource, utilization),
		Description: fmt.Sprintf("%s utilization %.1f%% exceeds threshold of %.1f%%", resource, utilization, threshold),
		Service:     "infrastructure",
		Region:      os.Getenv("AWS_REGION"),
		Metadata: map[string]interface{}{
			"resource":            resource,
			"utilization_percent": utilization,
			"threshold_percent":   threshold,
		},
	}

	if err := am.SendAlert(ctx, alert); err != nil {
		am.logger.Error("failed to send capacity alert",
			zap.Error(err),
			zap.String("resource", resource),
			zap.Float64("utilization", utilization))
	}
}
