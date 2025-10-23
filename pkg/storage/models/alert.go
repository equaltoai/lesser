package models

import (
	"fmt"
	"time"
)

// Alert represents a system alert that can be triggered for monitoring purposes
type Alert struct {
	// Primary key: ALERT#{alert_id}
	PK string `dynamorm:"pk" json:"pk"`
	// Sort key: METADATA
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1: Alert type index for querying by alert type
	// GSI1PK: ALERT_TYPE#{type}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	// GSI1SK: TIMESTAMP#{timestamp}
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`

	// GSI2: Service index for querying alerts by service
	// GSI2PK: SERVICE#{service}
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2_pk"`
	// GSI2SK: SEVERITY#{severity}#TIMESTAMP#{timestamp}
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2_sk"`

	// GSI3: Status index for querying alerts by status
	// GSI3PK: STATUS#{status}
	GSI3PK string `dynamorm:"index:GSI3,pk" json:"gsi3_pk"`
	// GSI3SK: PRIORITY#{priority}#TIMESTAMP#{timestamp}
	GSI3SK string `dynamorm:"index:GSI3,sk" json:"gsi3_sk"`

	// Alert identification
	AlertID  string `json:"alert_id"`
	Type     string `json:"type"`     // error_rate, latency, cost, health, security, capacity
	Severity string `json:"severity"` // info, warning, error, critical
	Priority string `json:"priority"` // P0, P1, P2
	Status   string `json:"status"`   // firing, resolved, acknowledged, suppressed

	// Alert content
	Title       string `json:"title"`
	Description string `json:"description"`
	Message     string `json:"message"`
	RunbookURL  string `json:"runbook_url,omitempty"`

	// Context information
	Service    string            `json:"service"`
	Region     string            `json:"region"`
	Source     string            `json:"source"`
	Dimensions map[string]string `json:"dimensions"`

	// Alert data
	Metadata   map[string]interface{} `json:"metadata"`
	Values     map[string]float64     `json:"values"`
	Thresholds map[string]float64     `json:"thresholds"`

	// Timing information
	FiredAt    time.Time  `json:"fired_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`

	// Escalation and delivery
	EscalationLevel  int        `json:"escalation_level"`
	DeliveryChannels []string   `json:"delivery_channels"` // webhook, sns, email, slack
	DeliveryAttempts int        `json:"delivery_attempts"`
	LastDeliveryAt   *time.Time `json:"last_delivery_at,omitempty"`
	NextRetryAt      *time.Time `json:"next_retry_at,omitempty"`

	// Alert grouping and suppression
	GroupKey         string     `json:"group_key,omitempty"`
	SuppressionUntil *time.Time `json:"suppression_until,omitempty"`

	// TTL for automatic cleanup (alerts older than 30 days)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the partition and sort keys based on alert data
func (a *Alert) UpdateKeys() error {
	if a.AlertID == "" {
		return ErrAlertIDRequired
	}

	// Primary key: ALERT#{alert_id}
	a.PK = fmt.Sprintf("ALERT#%s", a.AlertID)
	a.SK = SKMetadata

	// GSI1: Alert type index
	if a.Type != "" {
		a.GSI1PK = fmt.Sprintf("ALERT_TYPE#%s", a.Type)
		a.GSI1SK = fmt.Sprintf("TIMESTAMP#%s", a.FiredAt.Format(time.RFC3339))
	}

	// GSI2: Service index
	if a.Service != "" {
		a.GSI2PK = fmt.Sprintf("SERVICE#%s", a.Service)
		a.GSI2SK = fmt.Sprintf("SEVERITY#%s#TIMESTAMP#%s", a.Severity, a.FiredAt.Format(time.RFC3339))
	}

	// GSI3: Status index
	if a.Status != "" {
		a.GSI3PK = fmt.Sprintf("STATUS#%s", a.Status)
		priority := a.Priority
		if priority == "" {
			priority = "P2" // Default priority
		}
		a.GSI3SK = fmt.Sprintf("PRIORITY#%s#TIMESTAMP#%s", priority, a.FiredAt.Format(time.RFC3339))
	}

	// Set TTL for cleanup (30 days from creation)
	if a.TTL == 0 {
		a.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}

	return nil
}

// GetPK returns the partition key
func (a *Alert) GetPK() string {
	return a.PK
}

// GetSK returns the sort key
func (a *Alert) GetSK() string {
	return a.SK
}

// IsActive returns true if the alert is currently active (firing)
func (a *Alert) IsActive() bool {
	return a.Status == "firing"
}

// IsResolved returns true if the alert has been resolved
func (a *Alert) IsResolved() bool {
	return a.Status == string(ModerationStatusResolved)
}

// IsCritical returns true if the alert is critical severity
func (a *Alert) IsCritical() bool {
	return a.Severity == string(AdvancedSeverityCritical)
}

// IsHighPriority returns true if the alert is P0 or P1
func (a *Alert) IsHighPriority() bool {
	return a.Priority == "P0" || a.Priority == "P1"
}

// ShouldRetry returns true if the alert delivery should be retried
func (a *Alert) ShouldRetry() bool {
	if a.NextRetryAt == nil {
		return false
	}
	return time.Now().After(*a.NextRetryAt) && a.DeliveryAttempts < 5
}

// CalculateNextRetry calculates the next retry time using exponential backoff
func (a *Alert) CalculateNextRetry() time.Time {
	// Exponential backoff: 1min, 2min, 4min, 8min, 16min
	delay := time.Duration(1<<a.DeliveryAttempts) * time.Minute
	if delay > 16*time.Minute {
		delay = 16 * time.Minute
	}
	return time.Now().Add(delay)
}

// AddDimension adds a dimension to the alert
func (a *Alert) AddDimension(key, value string) {
	if a.Dimensions == nil {
		a.Dimensions = make(map[string]string)
	}
	a.Dimensions[key] = value
}

// AddMetadata adds metadata to the alert
func (a *Alert) AddMetadata(key string, value interface{}) {
	if a.Metadata == nil {
		a.Metadata = make(map[string]interface{})
	}
	a.Metadata[key] = value
}

// AddValue adds a value to the alert
func (a *Alert) AddValue(key string, value float64) {
	if a.Values == nil {
		a.Values = make(map[string]float64)
	}
	a.Values[key] = value
}

// AddThreshold adds a threshold to the alert
func (a *Alert) AddThreshold(key string, value float64) {
	if a.Thresholds == nil {
		a.Thresholds = make(map[string]float64)
	}
	a.Thresholds[key] = value
}

// Resolve marks the alert as resolved
func (a *Alert) Resolve() {
	a.Status = string(ModerationStatusResolved)
	now := time.Now()
	a.ResolvedAt = &now
	a.UpdatedAt = now
}

// Acknowledge marks the alert as acknowledged
func (a *Alert) Acknowledge() {
	a.Status = "acknowledged"
	a.UpdatedAt = time.Now()
}

// Suppress suppresses the alert until the specified time
func (a *Alert) Suppress(until time.Time) {
	a.Status = "suppressed"
	a.SuppressionUntil = &until
	a.UpdatedAt = time.Now()
}

// Escalate increases the escalation level
func (a *Alert) Escalate() {
	a.EscalationLevel++
	a.UpdatedAt = time.Now()
}

// RecordDeliveryAttempt records a delivery attempt
func (a *Alert) RecordDeliveryAttempt(success bool) {
	a.DeliveryAttempts++
	now := time.Now()
	a.LastDeliveryAt = &now
	a.UpdatedAt = now

	if !success {
		nextRetry := a.CalculateNextRetry()
		a.NextRetryAt = &nextRetry
	} else {
		a.NextRetryAt = nil
	}
}

// TableName returns the DynamoDB table backing Alert.
func (Alert) TableName() string {
	return MainTableName
}

// WebhookDelivery represents a webhook delivery attempt for an alert
type WebhookDelivery struct {
	// Primary key: WEBHOOK#{webhook_id}
	PK string `dynamorm:"pk" json:"pk"`
	// Sort key: DELIVERY#{delivery_id}
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1: Alert index for querying deliveries by alert
	// GSI1PK: ALERT#{alert_id}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	// GSI1SK: STATUS#{status}#TIMESTAMP#{timestamp}
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`

	// GSI2: Status index for querying by delivery status
	// GSI2PK: STATUS#{status}
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2_pk"`
	// GSI2SK: TIMESTAMP#{timestamp}
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2_sk"`

	// Delivery identification
	DeliveryID string `json:"delivery_id"`
	AlertID    string `json:"alert_id"`
	WebhookID  string `json:"webhook_id"`

	// Webhook configuration
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	SecretToken string            `json:"secret_token,omitempty"`
	Timeout     int               `json:"timeout_seconds"`

	// Delivery details
	Status        string `json:"status"` // pending, success, failed, retrying
	AttemptNumber int    `json:"attempt_number"`
	MaxAttempts   int    `json:"max_attempts"`

	// Request/Response data
	RequestBody     string            `json:"request_body"`
	ResponseCode    int               `json:"response_code,omitempty"`
	ResponseBody    string            `json:"response_body,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`

	// Error information
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorType    string `json:"error_type,omitempty"` // network, timeout, server_error, client_error

	// Timing information
	ScheduledAt time.Time  `json:"scheduled_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Duration    int64      `json:"duration_ms,omitempty"` // Duration in milliseconds

	// Retry configuration
	NextRetryAt   *time.Time `json:"next_retry_at,omitempty"`
	RetryInterval int        `json:"retry_interval_seconds"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for cleanup (deliveries older than 7 days)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the partition and sort keys based on delivery data
func (w *WebhookDelivery) UpdateKeys() error {
	if w.DeliveryID == "" {
		return ErrDeliveryIDRequired
	}
	if w.WebhookID == "" {
		return ErrWebhookIDRequired
	}

	// Primary key: WEBHOOK#{webhook_id}
	w.PK = fmt.Sprintf("WEBHOOK#%s", w.WebhookID)
	w.SK = fmt.Sprintf("DELIVERY#%s", w.DeliveryID)

	// GSI1: Alert index
	if w.AlertID != "" {
		w.GSI1PK = fmt.Sprintf("ALERT#%s", w.AlertID)
		w.GSI1SK = fmt.Sprintf("STATUS#%s#TIMESTAMP#%s", w.Status, w.ScheduledAt.Format(time.RFC3339))
	}

	// GSI2: Status index
	if w.Status != "" {
		w.GSI2PK = fmt.Sprintf("STATUS#%s", w.Status)
		w.GSI2SK = fmt.Sprintf("TIMESTAMP#%s", w.ScheduledAt.Format(time.RFC3339))
	}

	// Set TTL for cleanup (7 days from creation)
	if w.TTL == 0 {
		w.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
	}

	return nil
}

// GetPK returns the partition key
func (w *WebhookDelivery) GetPK() string {
	return w.PK
}

// GetSK returns the sort key
func (w *WebhookDelivery) GetSK() string {
	return w.SK
}

// IsComplete returns true if the delivery is complete (success or failed with no retries)
func (w *WebhookDelivery) IsComplete() bool {
	return w.Status == "success" || (w.Status == DeliveryStatusFailed && w.AttemptNumber >= w.MaxAttempts)
}

// CanRetry returns true if the delivery can be retried
func (w *WebhookDelivery) CanRetry() bool {
	return w.Status == DeliveryStatusFailed && w.AttemptNumber < w.MaxAttempts
}

// ShouldRetry returns true if the delivery should be retried now
func (w *WebhookDelivery) ShouldRetry() bool {
	if !w.CanRetry() {
		return false
	}
	if w.NextRetryAt == nil {
		return true
	}
	return time.Now().After(*w.NextRetryAt)
}

// MarkStarted marks the delivery as started
func (w *WebhookDelivery) MarkStarted() {
	now := time.Now()
	w.StartedAt = &now
	w.UpdatedAt = now
}

// MarkSuccess marks the delivery as successful
func (w *WebhookDelivery) MarkSuccess(responseCode int, responseBody string, responseHeaders map[string]string, duration time.Duration) {
	w.Status = "success"
	w.ResponseCode = responseCode
	w.ResponseBody = responseBody
	w.ResponseHeaders = responseHeaders
	w.Duration = duration.Milliseconds()

	now := time.Now()
	w.CompletedAt = &now
	w.UpdatedAt = now
	w.NextRetryAt = nil
}

// MarkFailed marks the delivery as failed
func (w *WebhookDelivery) MarkFailed(errorMessage, errorType string, responseCode int, responseBody string, duration time.Duration) {
	w.Status = DeliveryStatusFailed
	w.ErrorMessage = errorMessage
	w.ErrorType = errorType
	w.ResponseCode = responseCode
	w.ResponseBody = responseBody
	w.Duration = duration.Milliseconds()

	now := time.Now()
	w.CompletedAt = &now
	w.UpdatedAt = now

	// Calculate next retry if attempts remaining
	if w.AttemptNumber < w.MaxAttempts {
		w.Status = "retrying"
		nextRetry := w.calculateNextRetry()
		w.NextRetryAt = &nextRetry
	}
}

// calculateNextRetry calculates the next retry time using exponential backoff
func (w *WebhookDelivery) calculateNextRetry() time.Time {
	// Exponential backoff with jitter: base interval * 2^(attempt-1)
	baseInterval := time.Duration(w.RetryInterval) * time.Second
	if baseInterval == 0 {
		baseInterval = 30 * time.Second // Default 30 seconds
	}

	delay := baseInterval * time.Duration(1<<(w.AttemptNumber-1))
	maxDelay := 15 * time.Minute
	if delay > maxDelay {
		delay = maxDelay
	}

	return time.Now().Add(delay)
}

// TableName returns the DynamoDB table backing WebhookDelivery.
func (WebhookDelivery) TableName() string {
	return MainTableName
}

// DeadLetterMessage represents a message that failed processing
type DeadLetterMessage struct {
	PK            string                 `dynamorm:"pk" json:"pk"`
	SK            string                 `dynamorm:"sk" json:"sk"`
	MessageID     string                 `json:"message_id"`
	OriginalType  string                 `json:"original_type"`
	OriginalID    string                 `json:"original_id"`
	ErrorMessage  string                 `json:"error_message"`
	ErrorType     string                 `json:"error_type"`
	AttemptCount  int                    `json:"attempt_count"`
	LastAttemptAt time.Time              `json:"last_attempt_at"`
	Payload       map[string]interface{} `json:"payload"`
	CreatedAt     time.Time              `json:"created_at"`
	TTL           int64                  `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the partition and sort keys
func (d *DeadLetterMessage) UpdateKeys() error {
	if d.MessageID == "" {
		return ErrMessageIDRequired
	}

	d.PK = fmt.Sprintf("DLQ#%s", d.OriginalType)
	d.SK = fmt.Sprintf("MESSAGE#%s", d.MessageID)

	// Set TTL for cleanup (30 days)
	if d.TTL == 0 {
		d.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}

	return nil
}

// GetPK returns the partition key
func (d *DeadLetterMessage) GetPK() string {
	return d.PK
}

// GetSK returns the sort key
func (d *DeadLetterMessage) GetSK() string {
	return d.SK
}

// TableName returns the DynamoDB table backing DeadLetterMessage.
func (DeadLetterMessage) TableName() string {
	return MainTableName
}
