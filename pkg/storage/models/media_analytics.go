package models

import (
	"fmt"
	"time"
	"github.com/equaltoai/lesser/pkg/common"
)

// MediaAnalytics tracks media streaming analytics
type MediaAnalytics struct {
	// DynamoDB Keys
	PK string `dynamorm:"pk" json:"pk"` // MEDIA_ANALYTICS#{format} or MANIFEST#{format}
	SK string `dynamorm:"sk" json:"sk"` // {timestamp}#{mediaID} or {date}#{mediaID}

	// GSI keys for querying
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // DATE#{date}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // {format}#{timestamp}

	// Business fields
	MediaID   string    `json:"media_id"`
	Format    string    `json:"format"`    // hls, dash
	Duration  float64   `json:"duration"`  // Media duration in seconds
	Timestamp time.Time `json:"timestamp"` // When the event occurred
	Date      string    `json:"date"`      // YYYY-MM-DD for daily aggregation

	// Metadata
	EventType string `json:"event_type,omitempty"` // manifest_generated, quality_changed, etc.
	UserID    string `json:"user_id,omitempty"`    // User requesting the manifest
	Quality   string `json:"quality,omitempty"`    // Video quality if applicable

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the GSI keys based on the current values
func (m *MediaAnalytics) UpdateKeys() {
	// Set GSI1 keys for date-based queries
	m.GSI1PK = fmt.Sprintf("DATE#%s", m.Date)
	m.GSI1SK = fmt.Sprintf("%s#%s", m.Format, m.Timestamp.Format(time.RFC3339))
}

// SetManifestGeneration configures this record for manifest generation tracking
func (m *MediaAnalytics) SetManifestGeneration(mediaID, format string, duration float64) {
	m.MediaID = mediaID
	m.Format = format
	m.Duration = duration
	m.EventType = "manifest_generated"
	m.Timestamp = time.Now()
	m.Date = m.Timestamp.Format(common.DateFormat)

	// Set primary keys
	m.PK = fmt.Sprintf("MANIFEST#%s", format)
	m.SK = fmt.Sprintf("%d#%s", m.Timestamp.Unix(), mediaID)

	// Set TTL to 30 days
	m.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()

	m.UpdateKeys()
}

// SetQualityChange configures this record for quality change tracking
func (m *MediaAnalytics) SetQualityChange(mediaID, userID, _, newQuality string) {
	m.MediaID = mediaID
	m.UserID = userID
	m.Quality = newQuality
	m.EventType = "quality_changed"
	m.Timestamp = time.Now()
	m.Date = m.Timestamp.Format(common.DateFormat)

	// Set primary keys
	m.PK = fmt.Sprintf("QUALITY_CHANGE#%s", userID)
	m.SK = fmt.Sprintf("%d#%s#%s", m.Timestamp.Unix(), mediaID, newQuality)

	// Set TTL to 7 days
	m.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()

	m.UpdateKeys()
}

// SetGeneralEvent configures this record for general media events
func (m *MediaAnalytics) SetGeneralEvent(eventType, mediaID, userID string) {
	m.MediaID = mediaID
	m.UserID = userID
	m.EventType = eventType
	m.Timestamp = time.Now()
	m.Date = m.Timestamp.Format(common.DateFormat)

	// Set primary keys
	m.PK = fmt.Sprintf("MEDIA_EVENT#%s", eventType)
	m.SK = fmt.Sprintf("%d#%s", m.Timestamp.Unix(), mediaID)

	// Set TTL based on event type
	switch eventType {
	case "session_start", "session_end":
		m.TTL = time.Now().Add(7 * 24 * time.Hour).Unix() // 7 days
	default:
		m.TTL = time.Now().Add(30 * 24 * time.Hour).Unix() // 30 days
	}

	m.UpdateKeys()
}
