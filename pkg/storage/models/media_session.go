package models

import (
	"fmt"
	"time"
)

// MediaSession represents a streaming session using DynamORM
type MediaSession struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB Keys - preserving legacy patterns
	PK string `theorydb:"pk,attr:PK" json:"pk"` // SESSION#{sessionID}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // METADATA

	// GSI keys for querying by user and media
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1pk"` // USER#{userID}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1sk"` // SESSION#{startTime}

	// Business fields
	SessionID        string     `theorydb:"attr:sessionID" json:"session_id"`
	UserID           string     `theorydb:"attr:userID" json:"user_id"`
	MediaID          string     `theorydb:"attr:mediaID" json:"media_id"`
	Format           string     `theorydb:"attr:format" json:"format"`                  // hls, dash
	CurrentQuality   string     `theorydb:"attr:currentQuality" json:"current_quality"` // 4k, 1080p, 720p, etc.
	StartTime        time.Time  `theorydb:"attr:startTime" json:"start_time"`
	EndTime          *time.Time `theorydb:"attr:endTime" json:"end_time,omitempty"`
	LastSegmentIndex int        `theorydb:"attr:lastSegmentIndex" json:"last_segment_index"`
	BytesTransferred int64      `theorydb:"attr:bytesTransferred" json:"bytes_transferred"`
	BufferHealth     float64    `theorydb:"attr:bufferHealth" json:"buffer_health"` // 0.0 to 1.0
	Active           bool       `theorydb:"attr:active" json:"active"`
	Duration         float64    `theorydb:"attr:duration" json:"duration,omitempty"` // in seconds
	LastUpdate       *time.Time `theorydb:"attr:lastUpdate" json:"last_update,omitempty"`

	// TTL for automatic cleanup (24 hours default)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys sets the GSI keys based on the current values
func (m *MediaSession) UpdateKeys() error {
	// Set primary keys
	m.PK = fmt.Sprintf(KeyPatternSession, m.SessionID)
	m.SK = SKMetadata

	// Set GSI1 keys for user-based queries (most recent first)
	m.GSI1PK = fmt.Sprintf(KeyPatternUser, m.UserID)
	m.GSI1SK = fmt.Sprintf(KeyPatternSession, m.StartTime.Format(time.RFC3339))
	return nil
}

// GetPK returns the partition key
func (m *MediaSession) GetPK() string {
	return m.PK
}

// GetSK returns the sort key
func (m *MediaSession) GetSK() string {
	return m.SK
}

// SetTTL sets the TTL for automatic session cleanup
func (m *MediaSession) SetTTL(ttl time.Duration) {
	m.TTL = time.Now().Add(ttl).Unix()
}

// TableName returns the DynamoDB table backing MediaSession.
func (MediaSession) TableName() string {
	return MainTableName
}

// QualityChange represents a quality change event for analytics
type QualityChange struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB Keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // QUALITY#{sessionID}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // timestamp (nanoseconds)

	// Business fields
	SessionID string    `theorydb:"attr:sessionID" json:"session_id"`
	Quality   string    `theorydb:"attr:quality" json:"quality"`
	Timestamp time.Time `theorydb:"attr:timestamp" json:"timestamp"`

	// TTL for analytics cleanup (7 days)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys sets the keys for quality change tracking
func (q *QualityChange) UpdateKeys() error {
	q.PK = fmt.Sprintf("QUALITY#%s", q.SessionID)
	q.SK = fmt.Sprintf("%d", q.Timestamp.UnixNano())
	q.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key
func (q *QualityChange) GetPK() string {
	return q.PK
}

// GetSK returns the sort key
func (q *QualityChange) GetSK() string {
	return q.SK
}

// TableName returns the DynamoDB table backing QualityChange.
func (QualityChange) TableName() string {
	return MainTableName
}
