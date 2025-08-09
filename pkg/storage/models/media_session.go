package models

import (
	"fmt"
	"time"
)

// MediaSession represents a streaming session using DynamORM
type MediaSession struct {
	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk" json:"pk"` // SESSION#{sessionID}
	SK string `dynamorm:"sk" json:"sk"` // METADATA

	// GSI keys for querying by user and media
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // USER#{userID}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // SESSION#{startTime}

	// Business fields
	SessionID        string     `json:"session_id"`
	UserID           string     `json:"user_id"`
	MediaID          string     `json:"media_id"`
	Format           string     `json:"format"`          // hls, dash
	CurrentQuality   string     `json:"current_quality"` // 4k, 1080p, 720p, etc.
	StartTime        time.Time  `json:"start_time"`
	EndTime          *time.Time `json:"end_time,omitempty"`
	LastSegmentIndex int        `json:"last_segment_index"`
	BytesTransferred int64      `json:"bytes_transferred"`
	BufferHealth     float64    `json:"buffer_health"` // 0.0 to 1.0
	Active           bool       `json:"active"`
	Duration         float64    `json:"duration,omitempty"` // in seconds
	LastUpdate       *time.Time `json:"last_update,omitempty"`

	// TTL for automatic cleanup (24 hours default)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the GSI keys based on the current values
func (m *MediaSession) UpdateKeys() {
	// Set primary keys
	m.PK = fmt.Sprintf(KeyPatternSession, m.SessionID)
	m.SK = SKMetadata

	// Set GSI1 keys for user-based queries (most recent first)
	m.GSI1PK = fmt.Sprintf(KeyPatternUser, m.UserID)
	m.GSI1SK = fmt.Sprintf(KeyPatternSession, m.StartTime.Format(time.RFC3339))
}

// SetTTL sets the TTL for automatic session cleanup
func (m *MediaSession) SetTTL(ttl time.Duration) {
	m.TTL = time.Now().Add(ttl).Unix()
}

// QualityChange represents a quality change event for analytics
type QualityChange struct {
	// DynamoDB Keys
	PK string `dynamorm:"pk" json:"pk"` // QUALITY#{sessionID}
	SK string `dynamorm:"sk" json:"sk"` // timestamp (nanoseconds)

	// Business fields
	SessionID string    `json:"session_id"`
	Quality   string    `json:"quality"`
	Timestamp time.Time `json:"timestamp"`

	// TTL for analytics cleanup (7 days)
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the keys for quality change tracking
func (q *QualityChange) UpdateKeys() {
	q.PK = fmt.Sprintf("QUALITY#%s", q.SessionID)
	q.SK = fmt.Sprintf("%d", q.Timestamp.UnixNano())
	q.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
}
