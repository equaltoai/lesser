package models

import "time"

// Notification represents a Mastodon-compatible notification
type Notification struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Account   Account   `json:"account"`
	Status    *Status   `json:"status,omitempty"`
}

// Notification type constants
const (
	NotificationTypeFollow    = "follow"
	NotificationTypeMention   = "mention"
	NotificationTypeFavourite = "favourite"
	NotificationTypeReblog    = "reblog"
)

// NotificationFilter represents parameters for filtering notifications
type NotificationFilter struct {
	Types        []string // Filter by notification types
	ExcludeTypes []string // Exclude specific notification types
	AccountID    string   // Filter by account
	Limit        int      // Maximum number to return
	MinID        string   // Return results newer than this ID
	MaxID        string   // Return results older than this ID
	SinceID      string   // Return results newer than this ID (for polling)
}

// ClearNotificationsRequest represents a request to clear notifications
type ClearNotificationsRequest struct {
	// Empty for now, but may have fields in the future
}
