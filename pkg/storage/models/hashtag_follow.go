package models

import (
	"fmt"
	"time"
)

// HashtagFollow represents a user following a hashtag
type HashtagFollow struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK" json:"-"` // user#{userID}
	SK string `theorydb:"sk,attr:SK" json:"-"` // hashtag#{name}

	// Fields
	UserID               string    `theorydb:"attr:userID" json:"user_id"`
	Hashtag              string    `theorydb:"attr:hashtag" json:"hashtag"`
	NotificationsEnabled bool      `theorydb:"attr:notificationsEnabled" json:"notifications_enabled"`
	Muted                bool      `theorydb:"attr:muted" json:"muted"`
	CreatedAt            time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt            time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// UpdateKeysWithParams updates the primary and sort keys for DynamoDB
func (h *HashtagFollow) UpdateKeysWithParams(userID, hashtag string) {
	h.PK = "user#" + userID
	h.SK = "hashtag#" + hashtag
	h.UserID = userID
	h.Hashtag = hashtag
}

// UpdateKeys implements BaseModel interface - updates keys without parameters
func (h *HashtagFollow) UpdateKeys() error {
	// Validate required fields
	if h.UserID == "" {
		return fmt.Errorf("UserID is required")
	}
	if h.Hashtag == "" {
		return fmt.Errorf("Hashtag is required")
	}

	// Set primary keys
	h.PK = "user#" + h.UserID
	h.SK = "hashtag#" + h.Hashtag

	return nil
}

// GetPK returns the partition key for BaseModel interface
func (h *HashtagFollow) GetPK() string {
	return h.PK
}

// GetSK returns the sort key for BaseModel interface
func (h *HashtagFollow) GetSK() string {
	return h.SK
}

// TableName returns the DynamoDB table backing HashtagFollow.
func (HashtagFollow) TableName() string {
	return MainTableName
}
