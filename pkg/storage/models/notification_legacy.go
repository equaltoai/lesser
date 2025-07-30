package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NotificationLegacy represents a notification with legacy key patterns
// Used for compatibility with existing notification data
type NotificationLegacy struct {
	PK        string `dynamorm:"pk" json:"PK"`        // NOTIFICATIONS#username
	SK        string `dynamorm:"sk" json:"SK"`        // timestamp#notificationID
	ID        string `json:"ID"`
	Type      string `json:"Type"`
	Username  string `json:"Username"`
	AccountID string `json:"AccountID"`
	StatusID  string `json:"StatusID,omitempty"`
	Read      bool   `json:"Read"`
	CreatedAt int64  `json:"CreatedAt"` // Unix timestamp for sorting
	TTL       int64  `dynamorm:"ttl" json:"TTL"`       // 30 days auto-deletion
}

// TableName returns the DynamoDB table name
func (NotificationLegacy) TableName() string {
	return "lesser-main"
}

// UpdateKeys updates the notification keys
func (n *NotificationLegacy) UpdateKeys() {
	// Keys are set during creation
}

// SetPrimaryKey sets the primary key for the notification
func (n *NotificationLegacy) SetPrimaryKey(username string) {
	n.PK = fmt.Sprintf("NOTIFICATIONS#%s", username)
}

// SetSortKey sets the sort key for the notification
func (n *NotificationLegacy) SetSortKey(timestamp int64, id string) {
	n.SK = fmt.Sprintf("%d#%s", timestamp, id)
}

// NewNotificationLegacy creates a new legacy notification with proper initialization
func NewNotificationLegacy(username, notificationType, accountID string) *NotificationLegacy {
	id := uuid.New().String()
	now := time.Now()
	timestamp := now.Unix()

	n := &NotificationLegacy{
		ID:        id,
		Type:      notificationType,
		Username:  username,
		AccountID: accountID,
		Read:      false,
		CreatedAt: timestamp,
		TTL:       now.Add(30 * 24 * time.Hour).Unix(), // 30 days TTL
	}

	// Set keys
	n.SetPrimaryKey(username)
	n.SetSortKey(timestamp, id)

	return n
}