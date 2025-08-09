package models

import (
	"fmt"
	"time"
)

// HashtagMute represents a user muting a hashtag
type HashtagMute struct {
	PK        string    `dynamorm:"pk" json:"pk"` // USER#username
	SK        string    `dynamorm:"sk" json:"sk"` // HASHTAG_MUTE#tagname
	Username  string    `json:"username"`
	Hashtag   string    `json:"hashtag"`
	CreatedAt time.Time `json:"created_at"`
	TTL       int64     `dynamorm:"ttl" json:"ttl,omitempty"` // Optional expiration
}

// UpdateKeys updates the PK/SK for a hashtag mute
func (h *HashtagMute) UpdateKeys() {
	if h.Username != "" && h.Hashtag != "" {
		h.PK = fmt.Sprintf(KeyPatternUser, h.Username)
		h.SK = fmt.Sprintf("HASHTAG_MUTE#%s", h.Hashtag)
	}
}

// TableName returns the DynamoDB table name
func (h *HashtagMute) TableName() string {
	return "lesser"
}
