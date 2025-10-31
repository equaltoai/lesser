package models

import (
	"fmt"
	"time"
)

// NotificationPreferences represents user notification preferences in DynamoDB
type NotificationPreferences struct {
	// Keys - using legacy pattern
	PK string `dynamorm:"pk" json:"-"` // USER#username
	SK string `dynamorm:"sk" json:"-"` // NOTIFICATION_PREFS

	// Fields matching legacy storage.NotificationPreferences
	Username        string    `json:"username"`
	EmailEnabled    bool      `json:"email_enabled"`
	PushEnabled     bool      `json:"push_enabled"`
	FollowEnabled   bool      `json:"follow_enabled"`
	MentionEnabled  bool      `json:"mention_enabled"`
	ReblogEnabled   bool      `json:"reblog_enabled"`
	FavoriteEnabled bool      `json:"favorite_enabled"`
	PollEnabled     bool      `json:"poll_enabled"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Additional notification preferences
	FollowNotifications      bool `json:"follow_notifications"`
	MentionNotifications     bool `json:"mention_notifications"`
	ReblogNotifications      bool `json:"reblog_notifications"`
	FavoriteNotifications    bool `json:"favorite_notifications"`
	PollNotifications        bool `json:"poll_notifications"`
	NewFollowerNotifications bool `json:"new_follower_notifications"`
	DigestEmail              bool `json:"digest_email"`
}

// TableName returns the DynamoDB table backing NotificationPreferences.
func (NotificationPreferences) TableName() string {
	return MainTableName
}

// UpdateKeys updates the primary and GSI keys based on the model's fields
func (n *NotificationPreferences) UpdateKeys() {
	n.PK = fmt.Sprintf(KeyPatternUser, n.Username)
	n.SK = "NOTIFICATION_PREFS"
}

// BeforeCreate is called before creating a new notification preferences record
func (n *NotificationPreferences) BeforeCreate() error {
	n.UpdateKeys()
	n.UpdatedAt = time.Now()
	return nil
}

// BeforeUpdate is called before updating notification preferences
func (n *NotificationPreferences) BeforeUpdate() error {
	n.UpdateKeys()
	n.UpdatedAt = time.Now()
	return nil
}
