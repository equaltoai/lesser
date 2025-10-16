package models

import (
	"fmt"
	"time"
)

// NotificationFilter represents a lightweight filter configuration for hashtag notifications.
type NotificationFilter struct {
	Types        []string `json:"types,omitempty"`
	AccountID    string   `json:"account_id,omitempty"`
	MinID        string   `json:"min_id,omitempty"`
	MaxID        string   `json:"max_id,omitempty"`
	SinceID      string   `json:"since_id,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	ExcludeTypes []string `json:"exclude_types,omitempty"`
}

// HashtagNotificationSettings stores per-hashtag notification preferences for a user.
type HashtagNotificationSettings struct {
	PK         string               `dynamorm:"pk" json:"pk"` // user#{userID}
	SK         string               `dynamorm:"sk" json:"sk"` // settings#{name}
	UserID     string               `json:"user_id"`
	Hashtag    string               `json:"hashtag"`
	Level      string               `json:"level"`
	Muted      bool                 `json:"muted"`
	MutedUntil *time.Time           `json:"muted_until,omitempty"`
	Filters    []NotificationFilter `json:"filters,omitempty"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
}

// UpdateKeysWithParams ensures composite keys are populated.
func (h *HashtagNotificationSettings) UpdateKeysWithParams(userID, hashtag string) {
	h.PK = fmt.Sprintf("user#%s", userID)
	h.SK = fmt.Sprintf("settings#%s", hashtag)
	h.UserID = userID
	h.Hashtag = hashtag
}

// UpdateKeys implements the BaseModel interface.
func (h *HashtagNotificationSettings) UpdateKeys() error {
	if h.UserID != "" && h.Hashtag != "" {
		h.UpdateKeysWithParams(h.UserID, h.Hashtag)
	}
	return nil
}

// GetPK returns the partition key for the BaseModel interface.
func (h *HashtagNotificationSettings) GetPK() string {
	return h.PK
}

// GetSK returns the sort key for the BaseModel interface.
func (h *HashtagNotificationSettings) GetSK() string {
	return h.SK
}
