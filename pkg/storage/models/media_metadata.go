package models

import (
	"fmt"
	"strings"
	"time"
)

// MediaMetadata represents metadata about media files stored in S3
// This model handles the DynamoDB storage of metadata while S3 handles actual file storage
type MediaMetadata struct {
	// Primary keys following legacy patterns
	PK string `dynamorm:"pk" json:"pk"` // MEDIA#{mediaID}
	SK string `dynamorm:"sk" json:"sk"` // METADATA

	// GSI keys for queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // STATUS#{status}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // PROCESSED#{timestamp}

	// Media identification
	MediaID string `json:"media_id"`

	// Basic metadata
	OriginalURL        string  `json:"original_url,omitempty"`
	Duration           float64 `json:"duration"`           // Duration in seconds
	Width              int     `json:"width"`              // Video width in pixels
	Height             int     `json:"height"`             // Video height in pixels
	Bitrate            int     `json:"bitrate"`            // Bitrate in kbps
	FileSize           int64   `json:"file_size"`          // Size in bytes
	ProcessedAt        time.Time `json:"processed_at"`     // When processing completed
	AvailableQualities []string  `json:"available_qualities"` // Available quality levels
	Status             string    `json:"status"`           // pending, processing, complete, failed

	// Codec information for HLS/DASH manifest generation
	VideoCodec   string                     `json:"video_codec,omitempty"`   // e.g., "avc1.640028"
	AudioCodec   string                     `json:"audio_codec,omitempty"`   // e.g., "mp4a.40.2"
	VideoProfile string                     `json:"video_profile,omitempty"` // e.g., "High", "Main", "Baseline"
	VideoLevel   string                     `json:"video_level,omitempty"`   // e.g., "4.0", "3.1"
	QualitySettings map[string]QualityCodecInfo `json:"quality_settings,omitempty"` // Per-quality codec info

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for cleanup of failed/temporary media
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// QualityCodecInfo contains codec information for a specific quality level
type QualityCodecInfo struct {
	VideoCodec string `json:"video_codec"` // H.264 profile/level string like "avc1.640028"
	AudioCodec string `json:"audio_codec"` // Audio codec string like "mp4a.40.2"
	Bandwidth  int    `json:"bandwidth"`   // Required bandwidth in bps
	Width      int    `json:"width"`       // Video width in pixels
	Height     int    `json:"height"`      // Video height in pixels
}

// TableName returns the DynamoDB table name
func (MediaMetadata) TableName() string {
	return "lesser-main"
}

// UpdateKeys sets the primary and GSI keys based on the media metadata
func (m *MediaMetadata) UpdateKeys() {
	// Set primary keys following legacy pattern: MEDIA#{mediaID} / METADATA
	m.PK = fmt.Sprintf("MEDIA#%s", m.MediaID)
	m.SK = "METADATA"

	// Set GSI1 keys for status-based queries
	m.GSI1PK = fmt.Sprintf("STATUS#%s", m.Status)
	m.GSI1SK = fmt.Sprintf("PROCESSED#%s", m.ProcessedAt.Format(time.RFC3339))
}

// BeforeCreate sets up the model before creation
func (m *MediaMetadata) BeforeCreate() error {
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now

	// Set default processing timestamp if not set
	if m.ProcessedAt.IsZero() {
		m.ProcessedAt = now
	}

	// Set default status if not set
	if m.Status == "" {
		m.Status = "pending"
	}

	// Ensure keys are set
	m.UpdateKeys()

	return m.Validate()
}

// BeforeUpdate sets up the model before update
func (m *MediaMetadata) BeforeUpdate() error {
	m.UpdatedAt = time.Now()

	// Update processed timestamp when status changes to complete
	if m.Status == "complete" && m.ProcessedAt.IsZero() {
		m.ProcessedAt = m.UpdatedAt
	}

	// Update keys
	m.UpdateKeys()

	return m.Validate()
}

// Validate performs validation on the MediaMetadata
func (m *MediaMetadata) Validate() error {
	if strings.TrimSpace(m.MediaID) == "" {
		return fmt.Errorf("MediaID is required")
	}

	// Validate status
	validStatuses := []string{"pending", "processing", "complete", "failed"}
	isValidStatus := false
	for _, vs := range validStatuses {
		if m.Status == vs {
			isValidStatus = true
			break
		}
	}
	if !isValidStatus {
		return fmt.Errorf("invalid status: %s", m.Status)
	}

	// Validate dimensions for video
	if m.Width < 0 {
		return fmt.Errorf("width must be non-negative")
	}
	if m.Height < 0 {
		return fmt.Errorf("height must be non-negative")
	}

	// Validate duration
	if m.Duration < 0 {
		return fmt.Errorf("duration must be non-negative")
	}

	// Validate file size
	if m.FileSize < 0 {
		return fmt.Errorf("file size must be non-negative")
	}

	return nil
}

// IsComplete returns true if the media processing is complete
func (m *MediaMetadata) IsComplete() bool {
	return m.Status == "complete"
}

// IsFailed returns true if the media processing failed
func (m *MediaMetadata) IsFailed() bool {
	return m.Status == "failed"
}

// IsProcessing returns true if the media is currently being processed
func (m *MediaMetadata) IsProcessing() bool {
	return m.Status == "processing"
}

// IsPending returns true if the media is pending processing
func (m *MediaMetadata) IsPending() bool {
	return m.Status == "pending"
}

// SetProcessing marks the media as being processed
func (m *MediaMetadata) SetProcessing() {
	m.Status = "processing"
	m.UpdateKeys()
}

// SetComplete marks the media as processing complete
func (m *MediaMetadata) SetComplete() {
	m.Status = "complete"
	m.ProcessedAt = time.Now()
	m.UpdateKeys()
}

// SetFailed marks the media as processing failed
func (m *MediaMetadata) SetFailed() {
	m.Status = "failed"
	m.UpdateKeys()
	// Set TTL for failed media to be cleaned up after 7 days
	m.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
}

// AddQuality adds a quality level to the available qualities
func (m *MediaMetadata) AddQuality(quality string) {
	// Check if quality already exists
	for _, q := range m.AvailableQualities {
		if q == quality {
			return // Already exists
		}
	}
	m.AvailableQualities = append(m.AvailableQualities, quality)
}

// HasQuality returns true if the specified quality is available
func (m *MediaMetadata) HasQuality(quality string) bool {
	for _, q := range m.AvailableQualities {
		if q == quality {
			return true
		}
	}
	return false
}

// GetCodecInfo returns codec information for a specific quality
func (m *MediaMetadata) GetCodecInfo(quality string) (QualityCodecInfo, bool) {
	if m.QualitySettings == nil {
		return QualityCodecInfo{}, false
	}
	info, exists := m.QualitySettings[quality]
	return info, exists
}

// SetCodecInfo sets codec information for a specific quality
func (m *MediaMetadata) SetCodecInfo(quality string, info QualityCodecInfo) {
	if m.QualitySettings == nil {
		m.QualitySettings = make(map[string]QualityCodecInfo)
	}
	m.QualitySettings[quality] = info
}