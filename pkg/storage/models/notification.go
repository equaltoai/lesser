package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
)

// Notification represents a user notification
type Notification struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - using user ID as partition key with notification sort key
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "user#{userID}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "notif#{timestamp}#{notificationID}"

	// GSI1 - Notification type queries
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk"` // Format: "NOTIF_TYPE#{type}"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk"` // Format: "{created_at}#{userID}#{id}"

	// GSI2 - Actor notifications (who triggered notifications)
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2_pk"` // Format: "NOTIF_ACTOR#{actorID}"
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2_sk"` // Format: "{created_at}#{userID}#{id}"

	// GSI3 - Group key for notification consolidation
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty" json:"gsi3_pk"` // Format: "NOTIF_GROUP#{groupKey}"
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty" json:"gsi3_sk"` // Format: "{created_at}#{id}"

	// Core notification data
	ID     string `theorydb:"attr:id" json:"id"`
	UserID string `theorydb:"attr:userID" json:"user_id"` // User receiving the notification
	Type   string `theorydb:"attr:type" json:"type"`      // "mention", "reblog", "favourite", "follow", "follow_request", etc.

	// Actor information (who triggered the notification)
	ActorID   string `theorydb:"attr:actorID" json:"actor_id"`
	ActorType string `theorydb:"attr:actorType" json:"actor_type,omitempty"` // "user", "remote_actor"

	// Target information (what the notification is about)
	TargetID   string `theorydb:"attr:targetID" json:"target_id,omitempty"`
	TargetType string `theorydb:"attr:targetType" json:"target_type,omitempty"` // "status", "user", "account"

	// Notification content
	Title string `theorydb:"attr:title" json:"title,omitempty"`
	Body  string `theorydb:"attr:body" json:"body,omitempty"`

	// Additional data payload
	Data map[string]interface{} `theorydb:"attr:data" json:"data,omitempty"`

	// Status tracking
	IsRead bool       `theorydb:"attr:isRead" json:"is_read"`
	ReadAt *time.Time `theorydb:"attr:readAt" json:"read_at,omitempty"`

	// Push notification status
	PushSent   bool       `theorydb:"attr:pushSent" json:"push_sent"`
	PushSentAt *time.Time `theorydb:"attr:pushSentAt" json:"push_sent_at,omitempty"`
	PushError  string     `theorydb:"attr:pushError" json:"push_error,omitempty"`

	// Grouping for similar notifications
	GroupKey   string `theorydb:"attr:groupKey" json:"group_key"`     // Key for grouping similar notifications
	GroupCount int    `theorydb:"attr:groupCount" json:"group_count"` // Number of notifications in this group

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// TTL for automatic cleanup (30 days)
	ExpiresAt int64 `theorydb:"ttl,attr:ttl" json:"expires_at"` // Unix timestamp

	// Version for optimistic locking
	Version int `theorydb:"version,attr:version" json:"version"`
}

// NotificationBuilder helps create notifications with proper defaults
type NotificationBuilder struct {
	notification *Notification
}

// TableName returns the DynamoDB table backing NotificationBuilder.
func (NotificationBuilder) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table name for the Notification model
func (Notification) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (n *Notification) BeforeCreate() error {
	now := time.Now()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	if n.UpdatedAt.IsZero() {
		// On create, keep updated_at consistent with created_at even when the
		// caller supplies a deterministic CreatedAt (e.g., comm-worker deliveries).
		n.UpdatedAt = n.CreatedAt
	}

	// Generate ID if not provided
	if err := common.ValidateRequiredParam("ID", n.ID); err != nil {
		n.ID = uuid.New().String()
	}

	// Set defaults
	n.IsRead = false
	n.PushSent = false
	n.GroupCount = 1

	// Set expiry to 30 days
	n.ExpiresAt = n.CreatedAt.Add(30 * 24 * time.Hour).Unix()

	// Generate group key for similar notifications
	if err := common.ValidateRequiredParam("GroupKey", n.GroupKey); err != nil {
		n.GroupKey = n.generateGroupKey()
	}

	// Set up primary key
	n.PK = "USER#" + n.UserID
	timestamp := n.CreatedAt.Format("20060102150405")
	n.SK = fmt.Sprintf("notif#%s#%s", timestamp, n.ID)

	// Set up GSI keys
	n.setupGSIKeys()

	return n.Validate()
}

// BeforeUpdate sets up the model before update
func (n *Notification) BeforeUpdate() error {
	n.UpdatedAt = time.Now()

	// Update GSI keys in case type or other indexed fields changed
	n.setupGSIKeys()

	return n.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (n *Notification) setupGSIKeys() {
	createdAtStr := n.CreatedAt.Format(time.RFC3339)

	// GSI1 - Notification type queries
	n.GSI1PK = "NOTIF_TYPE#" + n.Type
	n.GSI1SK = fmt.Sprintf("%s#%s#%s", createdAtStr, n.UserID, n.ID)

	// GSI2 - Actor notifications
	if err := common.ValidateRequiredParam("ActorID", n.ActorID); err == nil {
		n.GSI2PK = "NOTIF_ACTOR#" + n.ActorID
		n.GSI2SK = fmt.Sprintf("%s#%s#%s", createdAtStr, n.UserID, n.ID)
	} else {
		n.GSI2PK = ""
		n.GSI2SK = ""
	}

	// GSI3 - Group key for notification consolidation
	n.GSI3PK = "NOTIF_GROUP#" + n.GroupKey
	n.GSI3SK = fmt.Sprintf("%s#%s", createdAtStr, n.ID)
}

// Validate performs validation on the Notification
func (n *Notification) Validate() error {
	if err := common.ValidateRequiredParam("ID", strings.TrimSpace(n.ID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("UserID", strings.TrimSpace(n.UserID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("Type", strings.TrimSpace(n.Type)); err != nil {
		return err
	}
	if !isValidNotificationType(n.Type) {
		return fmt.Errorf("%w: %s", ErrInvalidNotificationType, n.Type)
	}
	if err := common.ValidateRequiredParam("ActorID", strings.TrimSpace(n.ActorID)); err != nil {
		return err
	}

	return nil
}

// generateGroupKey creates a key for grouping similar notifications
func (n *Notification) generateGroupKey() string {
	// Group by type, actor, and target within a time window (1 hour)
	window := n.CreatedAt.Truncate(time.Hour).Format("2006010215")
	return fmt.Sprintf("%s:%s:%s:%s:%s", n.UserID, n.Type, n.ActorID, n.TargetID, window)
}

// MarkRead marks the notification as read
func (n *Notification) MarkRead() {
	if !n.IsRead {
		n.IsRead = true
		now := time.Now()
		n.ReadAt = &now
	}
}

// MarkUnread marks the notification as unread
func (n *Notification) MarkUnread() {
	n.IsRead = false
	n.ReadAt = nil
}

// MarkPushSent marks the push notification as sent
func (n *Notification) MarkPushSent() {
	n.PushSent = true
	now := time.Now()
	n.PushSentAt = &now
	n.PushError = ""
}

// MarkPushFailed marks the push notification as failed
func (n *Notification) MarkPushFailed(errorMsg string) {
	n.PushSent = false
	n.PushError = errorMsg
}

// ShouldSendPush determines if a push notification should be sent
func (n *Notification) ShouldSendPush() bool {
	return !n.PushSent && common.ValidateRequiredParam("PushError", n.PushError) != nil
}

// SetData sets a data field
func (n *Notification) SetData(key string, value interface{}) {
	if n.Data == nil {
		n.Data = make(map[string]interface{})
	}
	n.Data[key] = value
}

// GetData gets a data field
func (n *Notification) GetData(key string) (interface{}, bool) {
	if n.Data == nil {
		return nil, false
	}
	value, exists := n.Data[key]
	return value, exists
}

// IncrementGroupCount increments the group count for consolidated notifications
func (n *Notification) IncrementGroupCount() {
	n.GroupCount++
}

// isValidNotificationType checks if the notification type is valid
func isValidNotificationType(notifType string) bool {
	validTypes := map[string]bool{
		"mention":                true,
		"reply":                  true,
		"reblog":                 true,
		"favourite":              true,
		"follow":                 true,
		"follow_request":         true,
		"poll":                   true,
		"status":                 true,
		"update":                 true,
		"admin.sign_up":          true,
		"admin.report":           true,
		"communication:inbound":  true,
		"communication:outbound": true,
	}
	return validTypes[strings.ToLower(notifType)]
}

// NewNotificationBuilder creates a new notification builder
func NewNotificationBuilder() *NotificationBuilder {
	return &NotificationBuilder{
		notification: &Notification{},
	}
}

// ForUser sets the user ID for the notification
func (nb *NotificationBuilder) ForUser(userID string) *NotificationBuilder {
	nb.notification.UserID = userID
	return nb
}

// OfType sets the notification type
func (nb *NotificationBuilder) OfType(notifType string) *NotificationBuilder {
	nb.notification.Type = notifType
	return nb
}

// FromActor sets the actor who triggered the notification
func (nb *NotificationBuilder) FromActor(actorID, actorType string) *NotificationBuilder {
	nb.notification.ActorID = actorID
	nb.notification.ActorType = actorType
	return nb
}

// AboutTarget sets the target of the notification
func (nb *NotificationBuilder) AboutTarget(targetID, targetType string) *NotificationBuilder {
	nb.notification.TargetID = targetID
	nb.notification.TargetType = targetType
	return nb
}

// WithContent sets the notification content
func (nb *NotificationBuilder) WithContent(title, body string) *NotificationBuilder {
	nb.notification.Title = title
	nb.notification.Body = body
	return nb
}

// WithData adds data to the notification
func (nb *NotificationBuilder) WithData(key string, value interface{}) *NotificationBuilder {
	if nb.notification.Data == nil {
		nb.notification.Data = make(map[string]interface{})
	}
	nb.notification.Data[key] = value
	return nb
}

// WithGroupKey sets a custom group key
func (nb *NotificationBuilder) WithGroupKey(groupKey string) *NotificationBuilder {
	nb.notification.GroupKey = groupKey
	return nb
}

// Build creates the notification
func (nb *NotificationBuilder) Build() *Notification {
	return nb.notification
}

// Convenience functions for common notification types

// NewMentionNotification creates a mention notification
func NewMentionNotification(userID, actorID, statusID string) *Notification {
	return NewNotificationBuilder().
		ForUser(userID).
		OfType("mention").
		FromActor(actorID, "user").
		AboutTarget(statusID, "status").
		WithContent("New mention", "You were mentioned in a post").
		Build()
}

// NewReplyNotification creates a reply notification.
func NewReplyNotification(userID, actorID, statusID string) *Notification {
	return NewNotificationBuilder().
		ForUser(userID).
		OfType("reply").
		FromActor(actorID, "user").
		AboutTarget(statusID, "status").
		WithContent("New reply", "Someone replied to your post").
		Build()
}

// NewFollowNotification creates a follow notification
func NewFollowNotification(userID, followerID string) *Notification {
	return NewNotificationBuilder().
		ForUser(userID).
		OfType("follow").
		FromActor(followerID, "user").
		AboutTarget(followerID, "user").
		WithContent("New follower", "Someone started following you").
		Build()
}

// NewReblogNotification creates a reblog notification
func NewReblogNotification(userID, rebloggerID, statusID string) *Notification {
	return NewNotificationBuilder().
		ForUser(userID).
		OfType("reblog").
		FromActor(rebloggerID, "user").
		AboutTarget(statusID, "status").
		WithContent("Your post was reblogged", "Someone reblogged your post").
		Build()
}

// NewFavouriteNotification creates a favourite notification
func NewFavouriteNotification(userID, likerID, statusID string) *Notification {
	return NewNotificationBuilder().
		ForUser(userID).
		OfType("favourite").
		FromActor(likerID, "user").
		AboutTarget(statusID, "status").
		WithContent("Your post was favourited", "Someone favourited your post").
		Build()
}

// NewFollowRequestNotification creates a follow request notification
func NewFollowRequestNotification(userID, requesterID string) *Notification {
	return NewNotificationBuilder().
		ForUser(userID).
		OfType("follow_request").
		FromActor(requesterID, "user").
		AboutTarget(requesterID, "user").
		WithContent("New follow request", "Someone requested to follow you").
		Build()
}

// UpdateKeys updates the GSI keys for this notification (required by DynamORM)
func (n *Notification) UpdateKeys() error {
	// Set primary keys
	if err := common.ValidateRequiredParam("n.UserID", n.UserID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("n.ID", n.ID); err != nil {
		return err
	}

	// Set up primary key
	n.PK = "USER#" + n.UserID
	timestamp := n.CreatedAt.Format("20060102150405")
	n.SK = fmt.Sprintf("notif#%s#%s", timestamp, n.ID)

	// Set GSI keys
	n.setupGSIKeys()
	return nil
}

// GetPK returns the primary key for BaseRepository interface
func (n *Notification) GetPK() string {
	return n.PK
}

// GetSK returns the sort key for BaseRepository interface
func (n *Notification) GetSK() string {
	return n.SK
}
