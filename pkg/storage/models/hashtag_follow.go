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

// UpdateKeys updates the primary and sort keys for DynamoDB
func (h *HashtagFollow) UpdateKeys(userID, hashtag string) {
	h.PK = "USER#" + userID
	h.SK = "HASHTAG_FOLLOW#" + hashtag
	h.UserID = userID
	h.Hashtag = hashtag
}