package models

import (
	"fmt"
	"strings"
	"time"
)

// Hashtag represents hashtag metadata stored in DynamoDB using DynamORM
type Hashtag struct {
	// Primary key - hashtag metadata
	PK string `dynamorm:"pk" json:"pk"` // Format: "HASHTAG#{hashtag_name}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "METADATA"

	// GSI3 - Hashtag search by prefix
	GSI3PK string `dynamorm:"index:hashtag-search-index,pk" json:"gsi3_pk"` // Format: "HASHTAG_SEARCH#{first_2_chars}"
	GSI3SK string `dynamorm:"index:hashtag-search-index,sk" json:"gsi3_sk"` // Format: "{hashtag_name}"

	// Core hashtag data
	Name       string    `json:"name"`        // Hashtag name (lowercase, no #)
	URL        string    `json:"url"`         // Public URL for the hashtag
	UsageCount int64     `json:"usage_count"` // Total number of times used
	FirstSeen  time.Time `json:"first_seen"`  // When first seen
	LastUsed   time.Time `json:"last_used"`   // When last used
	UpdatedAt  time.Time `json:"updated_at"`  // Last metadata update

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
}

// UpdateKeys updates the GSI keys when the hashtag data changes
func (h *Hashtag) UpdateKeys() {
	tagLower := strings.ToLower(strings.TrimPrefix(h.Name, "#"))
	h.PK = fmt.Sprintf(KeyPatternHashtag, tagLower)
	h.SK = SKMetadata
	h.GSI3PK = fmt.Sprintf(KeyPatternHashtagSearch, getHashtagPrefix(tagLower))
	h.GSI3SK = tagLower
}

// HashtagUsage represents a single usage of a hashtag stored in DynamoDB using DynamORM
type HashtagUsage struct {
	// Primary key - hashtag usage
	PK string `dynamorm:"pk" json:"pk"` // Format: "HASHTAG#{hashtag_name}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "USAGE#{timestamp}#{status_id}"

	// Usage data
	StatusID   string    `json:"status_id"`
	AuthorID   string    `json:"author_id"`
	UsedAt     time.Time `json:"used_at"`
	Visibility string    `json:"visibility"`

	// TTL for automatic cleanup (30 days)
	TTL int64 `dynamorm:"ttl" json:"ttl"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
}

// UpdateKeys updates the keys when the usage data changes
func (hu *HashtagUsage) UpdateKeys(hashtag string) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	hu.PK = fmt.Sprintf(KeyPatternHashtag, tagLower)
	hu.SK = fmt.Sprintf("USAGE#%d#%s", hu.UsedAt.Unix(), hu.StatusID)
}

// getHashtagPrefix returns the first 2 characters of a hashtag for GSI partitioning
func getHashtagPrefix(hashtag string) string {
	tag := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	if len(tag) >= 2 {
		return tag[:2]
	}
	return tag
}
