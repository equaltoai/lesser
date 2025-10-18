// Package types defines shared data structures for media objects.
package types //nolint:revive // Standard types package name

import (
	"time"
)

// Quality represents the quality level of a media stream
type Quality string

// Quality constants
const (
	QualityAuto   Quality = "auto"
	QualityLow    Quality = "low"
	QualityMedium Quality = "medium"
	QualityHigh   Quality = "high"
	QualitySource Quality = "source"
)

// MediaFormat represents the streaming format
type MediaFormat string

// MediaFormat constants
const (
	FormatHLS    MediaFormat = "hls"
	FormatDASH   MediaFormat = "dash"
	FormatSource MediaFormat = "source"
)

// StreamingSession represents an active streaming session
//
//nolint:revive // Streaming prefix clarifies this is streaming-specific session
type StreamingSession struct {
	SessionID        string      `json:"session_id"`
	UserID           string      `json:"user_id"`
	MediaID          string      `json:"media_id"`
	Format           MediaFormat `json:"format"`
	CurrentQuality   Quality     `json:"current_quality"`
	StartTime        time.Time   `json:"start_time"`
	LastActivityTime time.Time   `json:"last_activity_time"`
	BytesDelivered   int64       `json:"bytes_delivered"`
	DurationWatched  int64       `json:"duration_watched"`
	UserAgent        string      `json:"user_agent"`
	IPAddress        string      `json:"ip_address"`
	Error            string      `json:"error,omitempty"`
	TTL              int64       `json:"ttl,omitempty"`

	// Additional fields for streaming state
	LastSegmentIndex int     `json:"last_segment_index"`
	BytesTransferred int64   `json:"bytes_transferred"`
	BufferHealth     float64 `json:"buffer_health"`
}
