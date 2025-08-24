// Package observability provides webhook delivery service for alerts with retry logic and dead letter handling
package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// WebhookConfig contains webhook endpoint configuration
type WebhookConfig struct {
	ID             string            `json:"id"`
	URL            string            `json:"url"`
	Headers        map[string]string `json:"headers"`
	Timeout        time.Duration     `json:"timeout"`
	MaxAttempts    int               `json:"max_attempts"`
	RetryInterval  time.Duration     `json:"retry_interval"`
	SecretToken    string            `json:"secret_token,omitempty"`
	VerifySSL      bool              `json:"verify_ssl"`
	Enabled        bool              `json:"enabled"`
	AlertTypes     []string          `json:"alert_types,omitempty"`     // Filter by alert types
	SeverityLevels []string          `json:"severity_levels,omitempty"` // Filter by severity
	Services       []string          `json:"services,omitempty"`        // Filter by services
}

// WebhookDeliveryService handles webhook delivery with retry logic and dead letter handling
type WebhookDeliveryService struct {
	logger         *zap.Logger
	httpClient     *http.Client
	webhookRepo    *StandaloneWebhookRepository
	alertRepo      *StandaloneAlertRepository
	deadLetterRepo *StandaloneDeadLetterRepository
	enabled        bool

	// Configuration
	defaultTimeout       time.Duration
	defaultMaxAttempts   int
	defaultRetryInterval time.Duration
}

// WebhookDeliveryConfig contains configuration for webhook delivery service
type WebhookDeliveryConfig struct {
	Logger               *zap.Logger
	WebhookRepository    *StandaloneWebhookRepository
	AlertRepository      *StandaloneAlertRepository
	DeadLetterRepository *StandaloneDeadLetterRepository
	HTTPTimeout          time.Duration
	MaxAttempts          int
	RetryInterval        time.Duration
	Enabled              bool
}

// NewWebhookDeliveryService creates a new webhook delivery service
func NewWebhookDeliveryService(config *WebhookDeliveryConfig) *WebhookDeliveryService {
	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = 30 * time.Second
	}
	if config.MaxAttempts == 0 {
		config.MaxAttempts = 5
	}
	if config.RetryInterval == 0 {
		config.RetryInterval = 30 * time.Second
	}

	httpClient := &http.Client{
		Timeout: config.HTTPTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &WebhookDeliveryService{
		logger:               config.Logger,
		httpClient:           httpClient,
		webhookRepo:          config.WebhookRepository,
		alertRepo:            config.AlertRepository,
		deadLetterRepo:       config.DeadLetterRepository,
		enabled:              config.Enabled,
		defaultTimeout:       config.HTTPTimeout,
		defaultMaxAttempts:   config.MaxAttempts,
		defaultRetryInterval: config.RetryInterval,
	}
}

// DeliverAlert delivers an alert to all configured webhooks
func (w *WebhookDeliveryService) DeliverAlert(ctx context.Context, alert *models.Alert) error {
	if !w.enabled {
		w.logger.Debug("webhook delivery disabled, skipping alert",
			zap.String("alert_id", alert.AlertID),
			zap.String("alert_type", alert.Type))
		return nil
	}

	// Get webhook configurations that match this alert
	webhooks, err := w.getMatchingWebhooks(ctx, alert)
	if err != nil {
		w.logger.Error("failed to get matching webhooks",
			zap.String("alert_id", alert.AlertID),
			zap.Error(err))
		return fmt.Errorf("failed to get matching webhooks: %w", err)
	}

	if len(webhooks) == 0 {
		w.logger.Debug("no matching webhooks found for alert",
			zap.String("alert_id", alert.AlertID),
			zap.String("alert_type", alert.Type),
			zap.String("severity", alert.Severity))
		return nil
	}

	// Create delivery tasks for each webhook
	var lastErr error
	successCount := 0

	for _, webhook := range webhooks {
		if !webhook.Enabled {
			continue
		}

		delivery := w.createDelivery(alert, webhook)

		// Attempt immediate delivery
		err := w.deliverWebhook(ctx, delivery, alert)
		if err != nil {
			w.logger.Error("webhook delivery failed",
				zap.String("alert_id", alert.AlertID),
				zap.String("webhook_id", webhook.ID),
				zap.String("url", webhook.URL),
				zap.Error(err))
			lastErr = err

			// Store failed delivery for retry
			if storeErr := w.webhookRepo.CreateDelivery(ctx, delivery); storeErr != nil {
				w.logger.Error("failed to store delivery record",
					zap.String("delivery_id", delivery.DeliveryID),
					zap.Error(storeErr))
			}
		} else {
			successCount++
			w.logger.Info("webhook delivered successfully",
				zap.String("alert_id", alert.AlertID),
				zap.String("webhook_id", webhook.ID),
				zap.String("url", webhook.URL))
		}
	}

	// Update alert delivery status
	alert.RecordDeliveryAttempt(successCount > 0)
	if updateErr := w.alertRepo.Update(ctx, alert); updateErr != nil {
		w.logger.Error("failed to update alert delivery status",
			zap.String("alert_id", alert.AlertID),
			zap.Error(updateErr))
	}

	if successCount == 0 && lastErr != nil {
		return fmt.Errorf("all webhook deliveries failed, last error: %w", lastErr)
	}

	return nil
}

// RetryFailedDeliveries processes failed deliveries that are ready for retry
func (w *WebhookDeliveryService) RetryFailedDeliveries(ctx context.Context) error {
	if !w.enabled {
		return nil
	}

	// Get failed deliveries ready for retry
	deliveries, err := w.webhookRepo.GetPendingRetries(ctx, 100)
	if err != nil {
		w.logger.Error("failed to get pending retries", zap.Error(err))
		return fmt.Errorf("failed to get pending retries: %w", err)
	}

	if len(deliveries) == 0 {
		return nil
	}

	w.logger.Info("processing failed webhook deliveries",
		zap.Int("count", len(deliveries)))

	successCount := 0
	for _, delivery := range deliveries {
		// Get the original alert
		alert, err := w.alertRepo.GetByID(ctx, delivery.AlertID)
		if err != nil {
			w.logger.Error("failed to get alert for retry",
				zap.String("alert_id", delivery.AlertID),
				zap.String("delivery_id", delivery.DeliveryID),
				zap.Error(err))
			continue
		}

		// Attempt delivery
		delivery.AttemptNumber++
		err = w.deliverWebhook(ctx, delivery, alert)
		if err != nil {
			w.logger.Error("webhook retry failed",
				zap.String("alert_id", delivery.AlertID),
				zap.String("delivery_id", delivery.DeliveryID),
				zap.Int("attempt", delivery.AttemptNumber),
				zap.Error(err))

			// Check if we should send to dead letter queue
			if !delivery.CanRetry() {
				if dlqErr := w.sendToDeadLetterQueue(ctx, delivery, alert, err); dlqErr != nil {
					w.logger.Error("failed to send to dead letter queue",
						zap.String("delivery_id", delivery.DeliveryID),
						zap.Error(dlqErr))
				}
			}
		} else {
			successCount++
			w.logger.Info("webhook retry succeeded",
				zap.String("alert_id", delivery.AlertID),
				zap.String("delivery_id", delivery.DeliveryID),
				zap.Int("attempt", delivery.AttemptNumber))
		}

		// Update delivery record
		if updateErr := w.webhookRepo.UpdateDelivery(ctx, delivery); updateErr != nil {
			w.logger.Error("failed to update delivery record",
				zap.String("delivery_id", delivery.DeliveryID),
				zap.Error(updateErr))
		}
	}

	w.logger.Info("completed webhook retry processing",
		zap.Int("total", len(deliveries)),
		zap.Int("successful", successCount))

	return nil
}

// getMatchingWebhooks returns webhook configurations that match the alert criteria
func (w *WebhookDeliveryService) getMatchingWebhooks(_ context.Context, alert *models.Alert) ([]*WebhookConfig, error) {
	// This would normally query from a configuration store
	// For now, return a default configuration from environment or config
	webhooks := []*WebhookConfig{}

	// Add default webhook from environment if configured
	if webhookURL := w.getWebhookURLFromEnv(); webhookURL != "" {
		webhook := &WebhookConfig{
			ID:             "default",
			URL:            webhookURL,
			Headers:        map[string]string{"Content-Type": "application/json"},
			Timeout:        w.defaultTimeout,
			MaxAttempts:    w.defaultMaxAttempts,
			RetryInterval:  w.defaultRetryInterval,
			VerifySSL:      true,
			Enabled:        true,
			AlertTypes:     []string{}, // Accept all types
			SeverityLevels: []string{}, // Accept all severities
			Services:       []string{}, // Accept all services
		}

		if w.matchesWebhookFilters(alert, webhook) {
			webhooks = append(webhooks, webhook)
		}
	}

	return webhooks, nil
}

// getWebhookURLFromEnv gets webhook URL from centralized configuration
func (w *WebhookDeliveryService) getWebhookURLFromEnv() string {
	cfg := config.Get()

	// Check primary webhook URL from config
	if cfg.AlertWebhookURL != "" {
		return cfg.AlertWebhookURL
	}

	// For backward compatibility, we might need to add other webhook URL fields to config
	// For now, return empty if no webhook URL is configured
	return ""
}

// matchesWebhookFilters checks if alert matches webhook filter criteria
func (w *WebhookDeliveryService) matchesWebhookFilters(alert *models.Alert, webhook *WebhookConfig) bool {
	// Check alert type filter
	if len(webhook.AlertTypes) > 0 {
		if !stringContains(webhook.AlertTypes, alert.Type) {
			return false
		}
	}

	// Check severity filter
	if len(webhook.SeverityLevels) > 0 {
		if !stringContains(webhook.SeverityLevels, alert.Severity) {
			return false
		}
	}

	// Check service filter
	if len(webhook.Services) > 0 {
		if !stringContains(webhook.Services, alert.Service) {
			return false
		}
	}

	return true
}

// createDelivery creates a new webhook delivery record
func (w *WebhookDeliveryService) createDelivery(alert *models.Alert, webhook *WebhookConfig) *models.WebhookDelivery {
	now := time.Now()

	delivery := &models.WebhookDelivery{
		DeliveryID:    uuid.New().String(),
		AlertID:       alert.AlertID,
		WebhookID:     webhook.ID,
		URL:           webhook.URL,
		Headers:       webhook.Headers,
		Timeout:       int(webhook.Timeout.Seconds()),
		Status:        "pending",
		AttemptNumber: 1,
		MaxAttempts:   webhook.MaxAttempts,
		ScheduledAt:   now,
		RetryInterval: int(webhook.RetryInterval.Seconds()),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	return delivery
}

// deliverWebhook attempts to deliver a webhook
func (w *WebhookDeliveryService) deliverWebhook(ctx context.Context, delivery *models.WebhookDelivery, alert *models.Alert) error {
	startTime := time.Now()
	delivery.MarkStarted()

	// Prepare webhook payload
	payload, err := w.prepareWebhookPayload(alert)
	if err != nil {
		duration := time.Since(startTime)
		delivery.MarkFailed(err.Error(), "payload_preparation", 0, "", duration)
		return fmt.Errorf("failed to prepare webhook payload: %w", err)
	}

	delivery.RequestBody = string(payload)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", delivery.URL, bytes.NewReader(payload))
	if err != nil {
		duration := time.Since(startTime)
		delivery.MarkFailed(err.Error(), "request_creation", 0, "", duration)
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range delivery.Headers {
		req.Header.Set(key, value)
	}

	// Add signature if secret token provided
	if delivery.WebhookID != "default" {
		// TODO: Add HMAC signature generation for webhook security
		_ = delivery.WebhookID // Prevent unused variable warning
	}

	// Send request
	resp, err := w.httpClient.Do(req)
	if err != nil {
		duration := time.Since(startTime)
		errorType := w.categorizeError(err)
		delivery.MarkFailed(err.Error(), errorType, 0, "", duration)
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		duration := time.Since(startTime)
		delivery.MarkFailed(err.Error(), "response_read", resp.StatusCode, "", duration)
		return fmt.Errorf("failed to read response: %w", err)
	}

	duration := time.Since(startTime)

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorType := w.categorizeHTTPError(resp.StatusCode)
		delivery.MarkFailed(
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status),
			errorType,
			resp.StatusCode,
			string(responseBody),
			duration,
		)
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	// Success
	responseHeaders := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			responseHeaders[key] = values[0]
		}
	}

	delivery.MarkSuccess(resp.StatusCode, string(responseBody), responseHeaders, duration)
	return nil
}

// prepareWebhookPayload creates the JSON payload for webhook delivery
func (w *WebhookDeliveryService) prepareWebhookPayload(alert *models.Alert) ([]byte, error) {
	// Create a webhook-friendly payload
	payload := map[string]interface{}{
		"alert_id":    alert.AlertID,
		"type":        alert.Type,
		"severity":    alert.Severity,
		"priority":    alert.Priority,
		"status":      alert.Status,
		"title":       alert.Title,
		"description": alert.Description,
		"message":     alert.Message,
		"service":     alert.Service,
		"region":      alert.Region,
		"source":      alert.Source,
		"runbook_url": alert.RunbookURL,
		"fired_at":    alert.FiredAt.Format(time.RFC3339),
		"dimensions":  alert.Dimensions,
		"metadata":    alert.Metadata,
		"values":      alert.Values,
		"thresholds":  alert.Thresholds,
	}

	if alert.ResolvedAt != nil {
		payload["resolved_at"] = alert.ResolvedAt.Format(time.RFC3339)
	}

	return json.Marshal(payload)
}

// categorizeError categorizes network errors for better retry logic
func (w *WebhookDeliveryService) categorizeError(err error) string {
	errStr := err.Error()

	if strings.Contains(errStr, "timeout") {
		return ErrorTypeTimeout
	}
	if strings.Contains(errStr, "connection refused") {
		return "connection_refused"
	}
	if strings.Contains(errStr, "no such host") {
		return "dns_error"
	}
	if strings.Contains(errStr, "certificate") || strings.Contains(errStr, "tls") {
		return "tls_error"
	}
	if urlErr, ok := err.(*url.Error); ok {
		if urlErr.Timeout() {
			return "timeout"
		}
	}

	return "network_error"
}

// categorizeHTTPError categorizes HTTP status codes for better retry logic
func (w *WebhookDeliveryService) categorizeHTTPError(statusCode int) string {
	switch {
	case statusCode >= 400 && statusCode < 500:
		// Client errors - usually shouldn't retry
		switch statusCode {
		case 408: // Request Timeout
			return "timeout"
		case 429: // Too Many Requests
			return "rate_limit"
		default:
			return "client_error"
		}
	case statusCode >= 500:
		// Server errors - can retry
		return "server_error"
	default:
		return "http_error"
	}
}

// sendToDeadLetterQueue sends failed deliveries to dead letter queue for manual investigation
func (w *WebhookDeliveryService) sendToDeadLetterQueue(ctx context.Context, delivery *models.WebhookDelivery, alert *models.Alert, lastError error) error {
	if w.deadLetterRepo == nil {
		w.logger.Warn("dead letter repository not configured, cannot store failed delivery",
			zap.String("delivery_id", delivery.DeliveryID))
		return nil
	}

	deadLetter := &models.DeadLetterMessage{
		MessageID:     uuid.New().String(),
		OriginalType:  "webhook_delivery",
		OriginalID:    delivery.DeliveryID,
		ErrorMessage:  lastError.Error(),
		ErrorType:     delivery.ErrorType,
		AttemptCount:  delivery.AttemptNumber,
		LastAttemptAt: time.Now(),
		Payload: map[string]interface{}{
			"delivery": delivery,
			"alert":    alert,
		},
		CreatedAt: time.Now(),
	}

	return w.deadLetterRepo.Create(ctx, deadLetter)
}

// ValidateWebhookURL validates a webhook URL
func ValidateWebhookURL(webhookURL string) error {
	if webhookURL == "" {
		return fmt.Errorf("webhook URL cannot be empty")
	}

	parsedURL, err := url.Parse(webhookURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https scheme")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("webhook URL must have a host")
	}

	return nil
}

// Helper functions

func stringContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
