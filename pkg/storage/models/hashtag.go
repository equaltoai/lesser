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
func (h *Hashtag) UpdateKeys() error {
	tagLower := strings.ToLower(strings.TrimPrefix(h.Name, "#"))
	h.PK = fmt.Sprintf(KeyPatternHashtag, tagLower)
	h.SK = SKMetadata
	h.GSI3PK = fmt.Sprintf(KeyPatternHashtagSearch, getHashtagPrefix(tagLower))
	h.GSI3SK = tagLower
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (h *Hashtag) GetPK() string {
	return h.PK
}

// GetSK returns the sort key for BaseModel interface
func (h *Hashtag) GetSK() string {
	return h.SK
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

// TableName ensures hashtag usage records live in the shared single-table schema.
func (HashtagUsage) TableName() string {
	return MainTableName
}

// UpdateKeysWithHashtag updates the keys when the usage data changes (parameterized version)
func (hu *HashtagUsage) UpdateKeysWithHashtag(hashtag string) {
	tagLower := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	hu.PK = fmt.Sprintf(KeyPatternHashtag, tagLower)
	hu.SK = fmt.Sprintf("USAGE#%d#%s", hu.UsedAt.Unix(), hu.StatusID)
}

// UpdateKeys implements BaseModel interface - updates keys without parameters
func (hu *HashtagUsage) UpdateKeys() error {
	// For HashtagUsage, we need the hashtag to be set in some way
	// This is a limitation - we'll need to call the parameterized version
	// For now, return nil assuming keys are already set
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (hu *HashtagUsage) GetPK() string {
	return hu.PK
}

// GetSK returns the sort key for BaseModel interface
func (hu *HashtagUsage) GetSK() string {
	return hu.SK
}

// getHashtagPrefix returns the first 2 characters of a hashtag for GSI partitioning
func getHashtagPrefix(hashtag string) string {
	tag := strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	if len(tag) >= 2 {
		return tag[:2]
	}
	return tag
}
