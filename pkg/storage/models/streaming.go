package models

import (
	"fmt"
	"time"
)

// StreamingPreferences model for DynamORM
type StreamingPreferences struct {
	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk" json:"pk"` // STREAMING_PREFS#{username}
	SK string `dynamorm:"sk" json:"sk"` // CURRENT or VERSION#{version}#{timestamp} or DEVICE#{deviceID}

	// GSI keys for querying
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // USER#{username}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // STREAMING_PREFS#{timestamp}
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2pk"` // DEVICE#{deviceID}
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2sk"` // STREAMING_PREFS#{username}

	// Business fields
	Username          string `json:"username"`
	DeviceID          string `json:"device_id,omitempty"`
	DefaultQuality    string `json:"default_quality"` // auto/4k/1080p/720p/480p
	AutoQuality       bool   `json:"auto_quality"`
	PreloadNext       bool   `json:"preload_next"`
	DataSaverMode     bool   `json:"data_saver_mode"`
	PreferredCodec    string `json:"preferred_codec"`    // h264/h265/av1/vp9
	MaxBandwidthMbps  int64  `json:"max_bandwidth_mbps"` // 0 means unlimited
	BufferSizeSeconds int    `json:"buffer_size_seconds"`

	// Version control
	Version       int `json:"version"`
	SchemaVersion int `json:"schema_version"`

	// Optional features - using pointers to detect nil vs false
	HDREnabled              *bool  `json:"hdr_enabled,omitempty"`
	ColorSpace              string `json:"color_space,omitempty"`
	SubtitleEnabled         *bool  `json:"subtitle_enabled,omitempty"`
	SubtitleLanguage        string `json:"subtitle_language,omitempty"`
	AudioDescriptionEnabled *bool  `json:"audio_description_enabled,omitempty"`
	ClosedCaptionsEnabled   *bool  `json:"closed_captions_enabled,omitempty"`

	// Timestamps
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for automatic cleanup of old versions
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the GSI keys based on the current values
func (s *StreamingPreferences) UpdateKeys() {
	// Set primary keys
	s.PK = fmt.Sprintf("STREAMING_PREFS#%s", s.Username)

	// Set GSI1 keys for user-based queries
	s.GSI1PK = fmt.Sprintf(KeyPatternUser, s.Username)
	s.GSI1SK = fmt.Sprintf("STREAMING_PREFS#%s", s.UpdatedAt.Format(time.RFC3339))

	// Set GSI2 keys for device-based queries (only if device ID is present)
	if s.DeviceID != "" {
		s.GSI2PK = fmt.Sprintf(KeyPatternDevice, s.DeviceID)
		s.GSI2SK = fmt.Sprintf("STREAMING_PREFS#%s", s.Username)
	}
}

// SetCurrentPreference sets this as the current preference for a user
func (s *StreamingPreferences) SetCurrentPreference() {
	s.SK = SKCurrent
	s.UpdateKeys()
}

// SetVersionedPreference sets this as a versioned preference in history
func (s *StreamingPreferences) SetVersionedPreference() {
	s.SK = fmt.Sprintf("VERSION#%d#%s", s.Version, s.UpdatedAt.Format(time.RFC3339))
	s.UpdateKeys()

	// Set TTL to 30 days for version history
	s.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
}

// SetDevicePreference sets this as a device-specific preference
func (s *StreamingPreferences) SetDevicePreference(deviceID string) {
	s.DeviceID = deviceID
	s.SK = fmt.Sprintf(KeyPatternDevice, deviceID)
	s.UpdateKeys()
}

// SetBackupPreference sets this as a backup before migration
func (s *StreamingPreferences) SetBackupPreference() {
	s.SK = fmt.Sprintf("BACKUP#%s", time.Now().Format(time.RFC3339))
	s.UpdateKeys()

	// Set TTL to 90 days for backups
	s.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
}

// GetDefaultStreamingPreferences returns default streaming preferences for a user
func GetDefaultStreamingPreferences(username string) *StreamingPreferences {
	prefs := &StreamingPreferences{
		Username:          username,
		DefaultQuality:    "auto",
		AutoQuality:       true,
		PreloadNext:       true,
		DataSaverMode:     false,
		PreferredCodec:    "h264",
		MaxBandwidthMbps:  0, // 0 means unlimited
		BufferSizeSeconds: 10,
		Version:           1,
		SchemaVersion:     1,
		UpdatedAt:         time.Now(),
	}

	prefs.SetCurrentPreference()
	return prefs
}
