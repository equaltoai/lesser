package models

import (
	"fmt"
	"time"
)

// NotificationPreferences represents user notification preferences in DynamoDB
type NotificationPreferences struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys - using legacy pattern
	PK string `theorydb:"pk,attr:PK" json:"-"` // USER#username
	SK string `theorydb:"sk,attr:SK" json:"-"` // NOTIFICATION_PREFS

	// Fields matching legacy storage.NotificationPreferences
	Username        string    `theorydb:"attr:username" json:"username"`
	EmailEnabled    bool      `theorydb:"attr:emailEnabled" json:"email_enabled"`
	PushEnabled     bool      `theorydb:"attr:pushEnabled" json:"push_enabled"`
	FollowEnabled   bool      `theorydb:"attr:followEnabled" json:"follow_enabled"`
	MentionEnabled  bool      `theorydb:"attr:mentionEnabled" json:"mention_enabled"`
	ReblogEnabled   bool      `theorydb:"attr:reblogEnabled" json:"reblog_enabled"`
	FavoriteEnabled bool      `theorydb:"attr:favoriteEnabled" json:"favorite_enabled"`
	PollEnabled     bool      `theorydb:"attr:pollEnabled" json:"poll_enabled"`
	UpdatedAt       time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// Additional notification preferences
	FollowNotifications      bool `theorydb:"attr:followNotifications" json:"follow_notifications"`
	MentionNotifications     bool `theorydb:"attr:mentionNotifications" json:"mention_notifications"`
	ReblogNotifications      bool `theorydb:"attr:reblogNotifications" json:"reblog_notifications"`
	FavoriteNotifications    bool `theorydb:"attr:favoriteNotifications" json:"favorite_notifications"`
	PollNotifications        bool `theorydb:"attr:pollNotifications" json:"poll_notifications"`
	NewFollowerNotifications bool `theorydb:"attr:newFollowerNotifications" json:"new_follower_notifications"`
	DigestEmail              bool `theorydb:"attr:digestEmail" json:"digest_email"`
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
