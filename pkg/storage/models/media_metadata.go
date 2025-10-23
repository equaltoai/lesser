package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
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
	OriginalURL        string    `json:"original_url,omitempty"`
	Duration           float64   `json:"duration"`            // Duration in seconds
	Width              int       `json:"width"`               // Video width in pixels
	Height             int       `json:"height"`              // Video height in pixels
	Bitrate            int       `json:"bitrate"`             // Bitrate in kbps
	FileSize           int64     `json:"file_size"`           // Size in bytes
	Blurhash           string    `json:"blurhash,omitempty"`  // Blurhash for images/video thumbnails
	ProcessedAt        time.Time `json:"processed_at"`        // When processing completed
	AvailableQualities []string  `json:"available_qualities"` // Available quality levels
	Status             string    `json:"status"`              // pending, processing, complete, failed

	// Codec information for HLS/DASH manifest generation
	VideoCodec      string                      `json:"video_codec,omitempty"`      // e.g., "avc1.640028"
	AudioCodec      string                      `json:"audio_codec,omitempty"`      // e.g., "mp4a.40.2"
	VideoProfile    string                      `json:"video_profile,omitempty"`    // e.g., "High", "Main", "Baseline"
	VideoLevel      string                      `json:"video_level,omitempty"`      // e.g., "4.0", "3.1"
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

// TableName returns the DynamoDB table backing QualityCodecInfo.
func (QualityCodecInfo) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table name
func (MediaMetadata) TableName() string {
	return MainTableName
}

// UpdateKeys sets the primary and GSI keys based on the media metadata
func (m *MediaMetadata) UpdateKeys() error {
	// Set primary keys following legacy pattern: MEDIA#{mediaID} / METADATA
	m.PK = fmt.Sprintf("MEDIA#%s", m.MediaID)
	m.SK = SKMetadata

	// Set GSI1 keys for status-based queries
	m.GSI1PK = fmt.Sprintf(KeyPatternStatus, m.Status)
	m.GSI1SK = fmt.Sprintf("PROCESSED#%s", m.ProcessedAt.Format(time.RFC3339))

	return nil
}

// GetPK returns the partition key (required by BaseModel)
func (m *MediaMetadata) GetPK() string {
	return m.PK
}

// GetSK returns the sort key (required by BaseModel)
func (m *MediaMetadata) GetSK() string {
	return m.SK
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
	if err := common.ValidateRequiredParam("m.Status", m.Status); err != nil {
		m.Status = StatusPending
	}

	// Ensure keys are set
	if err := m.UpdateKeys(); err != nil {
		return err
	}

	return m.Validate()
}

// BeforeUpdate sets up the model before update
func (m *MediaMetadata) BeforeUpdate() error {
	m.UpdatedAt = time.Now()

	// Update processed timestamp when status changes to complete
	if m.Status == StatusComplete && m.ProcessedAt.IsZero() {
		m.ProcessedAt = m.UpdatedAt
	}

	// Update keys
	if err := m.UpdateKeys(); err != nil {
		return err
	}

	return m.Validate()
}

// Validate performs validation on the MediaMetadata
func (m *MediaMetadata) Validate() error {
	if err := common.ValidateRequiredParam("strings.TrimSpace(m.MediaID)", strings.TrimSpace(m.MediaID)); err != nil {
		return ErrMediaMetadataIDRequired
	}

	// Validate status
	validStatuses := []string{StatusPending, StatusProcessing, StatusComplete, StatusFailed}
	isValidStatus := false
	for _, vs := range validStatuses {
		if m.Status == vs {
			isValidStatus = true
			break
		}
	}
	if !isValidStatus {
		return fmt.Errorf("%w: %s", ErrMediaMetadataInvalidStatus, m.Status)
	}

	// Validate dimensions for video
	if m.Width < 0 {
		return ErrMediaMetadataWidthNegative
	}
	if m.Height < 0 {
		return ErrMediaMetadataHeightNegative
	}

	// Validate duration
	if m.Duration < 0 {
		return ErrMediaMetadataDurationNegative
	}

	// Validate file size
	if m.FileSize < 0 {
		return ErrMediaMetadataFileSizeNegative
	}

	return nil
}

// IsComplete returns true if the media processing is complete
func (m *MediaMetadata) IsComplete() bool {
	return m.Status == StatusComplete
}

// IsFailed returns true if the media processing failed
func (m *MediaMetadata) IsFailed() bool {
	return m.Status == StatusFailed
}

// IsProcessing returns true if the media is currently being processed
func (m *MediaMetadata) IsProcessing() bool {
	return m.Status == StatusProcessing
}

// IsPending returns true if the media is pending processing
func (m *MediaMetadata) IsPending() bool {
	return m.Status == StatusPending
}

// SetProcessing marks the media as being processed
func (m *MediaMetadata) SetProcessing() {
	m.Status = StatusProcessing
	_ = m.UpdateKeys() // Safe to ignore error for simple setter
}

// SetComplete marks the media as processing complete
func (m *MediaMetadata) SetComplete() {
	m.Status = "complete"
	m.ProcessedAt = time.Now()
	_ = m.UpdateKeys() // Ignore error for these simple setters
}

// SetFailed marks the media as processing failed
func (m *MediaMetadata) SetFailed() {
	m.Status = StatusFailed
	_ = m.UpdateKeys() // Ignore error for these simple setters
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
