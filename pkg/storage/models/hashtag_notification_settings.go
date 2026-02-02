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

// TableName returns the DynamoDB table backing NotificationFilter.
func (NotificationFilter) TableName() string {
	return MainTableName
}

// HashtagNotificationSettings stores per-hashtag notification preferences for a user.
type HashtagNotificationSettings struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"pk"` // user#{userID}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // settings#{name}

	UserID     string               `theorydb:"attr:userID" json:"user_id"`
	Hashtag    string               `theorydb:"attr:hashtag" json:"hashtag"`
	Level      string               `theorydb:"attr:level" json:"level"`
	Muted      bool                 `theorydb:"attr:muted" json:"muted"`
	MutedUntil *time.Time           `theorydb:"attr:mutedUntil" json:"muted_until,omitempty"`
	Filters    []NotificationFilter `theorydb:"attr:filters" json:"filters,omitempty"`
	CreatedAt  time.Time            `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt  time.Time            `theorydb:"attr:updatedAt" json:"updated_at"`
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
	// Validate required fields
	if h.UserID == "" {
		return fmt.Errorf("UserID is required")
	}
	if h.Hashtag == "" {
		return fmt.Errorf("Hashtag is required")
	}

	// Set primary keys
	h.PK = fmt.Sprintf("user#%s", h.UserID)
	h.SK = fmt.Sprintf("settings#%s", h.Hashtag)

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

// TableName returns the DynamoDB table backing HashtagNotificationSettings.
func (HashtagNotificationSettings) TableName() string {
	return MainTableName
}
