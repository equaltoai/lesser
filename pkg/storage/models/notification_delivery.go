package models

import (
	"fmt"
	"github.com/equaltoai/lesser/pkg/common"
	"time"
)

// NotificationDelivery tracks delivery attempts for notifications
type NotificationDelivery struct {
	// Primary keys - using notification ID and delivery method
	PK string `dynamorm:"pk" json:"pk"` // Format: "NOTIFICATION#{notificationID}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "DELIVERY#{method}"

	// Core fields
	NotificationID string    `json:"notification_id"`
	DeliveryMethod string    `json:"delivery_method"` // "push", "websocket", "web"
	Status         string    `json:"status"`          // "pending", "sent", "failed"
	AttemptCount   int       `json:"attempt_count"`
	LastAttempt    time.Time `json:"last_attempt"`
	Error          string    `json:"error,omitempty"` // Error message if failed
	SentAt         time.Time `json:"sent_at,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for cleanup (7 days after creation)
	TTL int64 `dynamorm:"ttl" json:"ttl"`
}

// TableName returns the DynamoDB table name
func (NotificationDelivery) TableName() string {
	return MainTableName
}

// UpdateKeys updates the primary keys based on the model's fields
func (n *NotificationDelivery) UpdateKeys() {
	n.PK = fmt.Sprintf("NOTIFICATION#%s", n.NotificationID)
	n.SK = fmt.Sprintf("DELIVERY#%s", n.DeliveryMethod)
}

// BeforeCreate is called before creating a new delivery record
func (n *NotificationDelivery) BeforeCreate() error {
	now := time.Now()
	n.CreatedAt = now
	n.UpdatedAt = now
	n.LastAttempt = now

	// Set TTL to 7 days from now
	n.TTL = now.Add(7 * 24 * time.Hour).Unix()

	// Set initial status
	if err := common.ValidateRequiredParam("n.Status", n.Status); err != nil {
		n.Status = StatusPending
	}

	// Initialize attempt count
	if n.AttemptCount == 0 {
		n.AttemptCount = 1
	}

	n.UpdateKeys()
	return n.Validate()
}

// BeforeUpdate is called before updating a delivery record
func (n *NotificationDelivery) BeforeUpdate() error {
	n.UpdatedAt = time.Now()
	n.UpdateKeys()
	return n.Validate()
}

// Validate performs validation on the NotificationDelivery
func (n *NotificationDelivery) Validate() error {
	if err := common.ValidateRequiredParam("n.NotificationID", n.NotificationID); err != nil {
		return ErrNotificationIDRequired
	}
	if err := common.ValidateRequiredParam("n.DeliveryMethod", n.DeliveryMethod); err != nil {
		return ErrDeliveryMethodRequired
	}
	if !isValidDeliveryMethod(n.DeliveryMethod) {
		return fmt.Errorf("%w: %s", ErrInvalidDeliveryMethod, n.DeliveryMethod)
	}
	if !isValidDeliveryStatus(n.Status) {
		return fmt.Errorf("%w: %s", ErrInvalidDeliveryStatus, n.Status)
	}
	return nil
}

// MarkSent marks the delivery as sent
func (n *NotificationDelivery) MarkSent() {
	n.Status = "sent"
	n.SentAt = time.Now()
	n.Error = ""
}

// MarkFailed marks the delivery as failed
func (n *NotificationDelivery) MarkFailed(errorMsg string) {
	n.Status = StatusFailed
	n.Error = errorMsg
	n.LastAttempt = time.Now()
	n.AttemptCount++
}

// MarkPending marks the delivery as pending
func (n *NotificationDelivery) MarkPending() {
	n.Status = StatusPending
	n.Error = ""
}

// IncrementAttempt increments the attempt counter
func (n *NotificationDelivery) IncrementAttempt() {
	n.AttemptCount++
	n.LastAttempt = time.Now()
}

// CanRetry determines if this delivery can be retried
func (n *NotificationDelivery) CanRetry() bool {
	if n.Status == "sent" {
		return false
	}
	// Allow up to 3 attempts
	return n.AttemptCount < 3
}

// isValidDeliveryMethod checks if the delivery method is valid
func isValidDeliveryMethod(method string) bool {
	validMethods := map[string]bool{
		"push":      true,
		"websocket": true,
		"web":       true,
	}
	// Email and SMS are not supported by Lesser
	if method == "email" || method == "sms" {
		return false
	}
	return validMethods[method]
}

// isValidDeliveryStatus checks if the delivery status is valid
func isValidDeliveryStatus(status string) bool {
	validStatuses := map[string]bool{
		StatusPending: true,
		"sent":        true,
		StatusFailed:  true,
	}
	return validStatuses[status]
}

// NewNotificationDelivery creates a new delivery tracking record
func NewNotificationDelivery(notificationID, method string) *NotificationDelivery {
	return &NotificationDelivery{
		NotificationID: notificationID,
		DeliveryMethod: method,
		Status:         StatusPending,
		AttemptCount:   0,
	}
}
