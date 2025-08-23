package models

import "time"

// HashtagFollow represents a user following a hashtag
type HashtagFollow struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"` // USER#username
	SK string `dynamorm:"sk" json:"-"` // HASHTAG_FOLLOW#hashtag

	// Fields
	UserID               string    `json:"user_id"`
	Hashtag              string    `json:"hashtag"`
	NotificationsEnabled bool      `json:"notifications_enabled"`
	Muted                bool      `json:"muted"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// UpdateKeysWithParams updates the primary and sort keys for DynamoDB
func (h *HashtagFollow) UpdateKeysWithParams(userID, hashtag string) {
	h.PK = "USER#" + userID
	h.SK = "HASHTAG_FOLLOW#" + hashtag
	h.UserID = userID
	h.Hashtag = hashtag
}

// UpdateKeys implements BaseModel interface - updates keys without parameters
func (h *HashtagFollow) UpdateKeys() error {
	// For HashtagFollow, we need userID and hashtag to be set in some way
	// Assume they are already set in the struct fields
	if h.UserID != "" && h.Hashtag != "" {
		h.UpdateKeysWithParams(h.UserID, h.Hashtag)
	}
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
