package models

import (
	"fmt"
	"time"
)

// NotificationPreferences represents user notification preferences in DynamoDB
type NotificationPreferences struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Keys - using legacy pattern
	PK string `dynamorm:"pk,attr:PK" json:"-"` // USER#username
	SK string `dynamorm:"sk,attr:SK" json:"-"` // NOTIFICATION_PREFS

	// Fields matching legacy storage.NotificationPreferences
	Username        string    `dynamorm:"attr:username" json:"username"`
	EmailEnabled    bool      `dynamorm:"attr:emailEnabled" json:"email_enabled"`
	PushEnabled     bool      `dynamorm:"attr:pushEnabled" json:"push_enabled"`
	FollowEnabled   bool      `dynamorm:"attr:followEnabled" json:"follow_enabled"`
	MentionEnabled  bool      `dynamorm:"attr:mentionEnabled" json:"mention_enabled"`
	ReblogEnabled   bool      `dynamorm:"attr:reblogEnabled" json:"reblog_enabled"`
	FavoriteEnabled bool      `dynamorm:"attr:favoriteEnabled" json:"favorite_enabled"`
	PollEnabled     bool      `dynamorm:"attr:pollEnabled" json:"poll_enabled"`
	UpdatedAt       time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// Additional notification preferences
	FollowNotifications      bool `dynamorm:"attr:followNotifications" json:"follow_notifications"`
	MentionNotifications     bool `dynamorm:"attr:mentionNotifications" json:"mention_notifications"`
	ReblogNotifications      bool `dynamorm:"attr:reblogNotifications" json:"reblog_notifications"`
	FavoriteNotifications    bool `dynamorm:"attr:favoriteNotifications" json:"favorite_notifications"`
	PollNotifications        bool `dynamorm:"attr:pollNotifications" json:"poll_notifications"`
	NewFollowerNotifications bool `dynamorm:"attr:newFollowerNotifications" json:"new_follower_notifications"`
	DigestEmail              bool `dynamorm:"attr:digestEmail" json:"digest_email"`
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
