package models

import (
	"fmt"
	"strings"
	"time"
)

// Hashtag represents hashtag metadata stored in DynamoDB using DynamORM
type Hashtag struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - hashtag metadata
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "HASHTAG#{hashtag_name}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "METADATA"

	// GSI3 - Hashtag search by prefix
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty" json:"gsi3_pk"` // Format: "HASHTAG_SEARCH#{first_2_chars}"
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty" json:"gsi3_sk"` // Format: "{hashtag_name}"

	// Core hashtag data
	Name       string    `theorydb:"attr:name" json:"name"`              // Hashtag name (lowercase, no #)
	URL        string    `theorydb:"attr:url" json:"url"`                // Public URL for the hashtag
	UsageCount int64     `theorydb:"attr:usageCount" json:"usage_count"` // Total number of times used
	FirstSeen  time.Time `theorydb:"attr:firstSeen" json:"first_seen"`   // When first seen
	LastUsed   time.Time `theorydb:"attr:lastUsed" json:"last_used"`     // When last used
	UpdatedAt  time.Time `theorydb:"attr:updatedAt" json:"updated_at"`   // Last metadata update

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
}

// UpdateKeys updates the GSI keys when the hashtag data changes
//
// NOTE (wave part 2 batch S3 closeout, #1469 / #1501): there is deliberately
// no global "HASHTAGS#ALL" GSI listing key here. The metadata rows
// (HASHTAG#<name> / METADATA) had exactly one writer —
// HashtagRepository.IndexHashtag, deleted in batch S3 as zero-caller — and
// the metadata read family was deleted with it (getCandidateHashtags, the
// hashtag_repository.go date-range reads, the trend-aggregator's
// GetRecentHashtags step). The model itself is kept: GetHashtagInfo performs
// a live point read of the same row shape, and the BaseModel interface
// (base_repository.go) requires UpdateKeys for the embedded
// EnhancedBaseRepository[*models.Hashtag] (see
// docs/architecture/dynamodb-scan-inventory.md).
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

// TableName returns the DynamoDB table backing Hashtag.
func (Hashtag) TableName() string {
	return MainTableName
}

// HashtagUsage represents a single usage of a hashtag stored in DynamoDB using DynamORM
type HashtagUsage struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - hashtag usage
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "HASHTAG#{hashtag_name}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "USAGE#{timestamp}#{status_id}"

	// Usage data
	StatusID   string    `theorydb:"attr:statusID" json:"status_id"`
	AuthorID   string    `theorydb:"attr:authorID" json:"author_id"`
	UsedAt     time.Time `theorydb:"attr:usedAt" json:"used_at"`
	Visibility string    `theorydb:"attr:visibility" json:"visibility"`

	// TTL for automatic cleanup (30 days)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
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
