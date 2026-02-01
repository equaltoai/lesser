package models

import (
	"fmt"
	"time"
)

// HashtagMute represents a user muting a hashtag
type HashtagMute struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK        string    `theorydb:"pk,attr:PK" json:"pk"` // user#{userID}
	SK        string    `theorydb:"sk,attr:SK" json:"sk"` // mute#{name}
	Username  string    `theorydb:"attr:username" json:"username"`
	Hashtag   string    `theorydb:"attr:hashtag" json:"hashtag"`
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	TTL       int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"` // Optional expiration
}

// UpdateKeys updates the PK/SK for a hashtag mute
func (h *HashtagMute) UpdateKeys() {
	if h.Username != "" && h.Hashtag != "" {
		h.PK = fmt.Sprintf("user#%s", h.Username)
		h.SK = fmt.Sprintf("mute#%s", h.Hashtag)
	}
}

// TableName returns the DynamoDB table backing HashtagMute.
func (HashtagMute) TableName() string {
	return MainTableName
}
