package models

import (
	"fmt"
	"time"
)

// StreamingPreferences model for DynamORM
type StreamingPreferences struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB Keys - preserving legacy patterns
	PK string `theorydb:"pk,attr:PK" json:"pk"` // STREAMING_PREFS#{username}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // CURRENT or VERSION#{version}#{timestamp} or DEVICE#{deviceID}

	// GSI keys for querying
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1pk"` // USER#{username}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1sk"` // STREAMING_PREFS#{timestamp}
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"gsi2pk"` // DEVICE#{deviceID}
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"gsi2sk"` // STREAMING_PREFS#{username}

	// Business fields
	Username          string `theorydb:"attr:username" json:"username"`
	DeviceID          string `theorydb:"attr:deviceID" json:"device_id,omitempty"`
	DefaultQuality    string `theorydb:"attr:defaultQuality" json:"default_quality"` // auto/4k/1080p/720p/480p
	AutoQuality       bool   `theorydb:"attr:autoQuality" json:"auto_quality"`
	PreloadNext       bool   `theorydb:"attr:preloadNext" json:"preload_next"`
	DataSaverMode     bool   `theorydb:"attr:dataSaverMode" json:"data_saver_mode"`
	PreferredCodec    string `theorydb:"attr:preferredCodec" json:"preferred_codec"`      // h264/h265/av1/vp9
	MaxBandwidthMbps  int64  `theorydb:"attr:maxBandwidthMbps" json:"max_bandwidth_mbps"` // 0 means unlimited
	BufferSizeSeconds int    `theorydb:"attr:bufferSizeSeconds" json:"buffer_size_seconds"`

	// Version control
	Version       int `theorydb:"attr:version" json:"version"`
	SchemaVersion int `theorydb:"attr:schemaVersion" json:"schema_version"`

	// Optional features - using pointers to detect nil vs false
	HDREnabled              *bool  `theorydb:"attr:hdrEnabled" json:"hdr_enabled,omitempty"`
	ColorSpace              string `theorydb:"attr:colorSpace" json:"color_space,omitempty"`
	SubtitleEnabled         *bool  `theorydb:"attr:subtitleEnabled" json:"subtitle_enabled,omitempty"`
	SubtitleLanguage        string `theorydb:"attr:subtitleLanguage" json:"subtitle_language,omitempty"`
	AudioDescriptionEnabled *bool  `theorydb:"attr:audioDescriptionEnabled" json:"audio_description_enabled,omitempty"`
	ClosedCaptionsEnabled   *bool  `theorydb:"attr:closedCaptionsEnabled" json:"closed_captions_enabled,omitempty"`

	// Timestamps
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// TTL for automatic cleanup of old versions
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing StreamingPreferences.
func (StreamingPreferences) TableName() string {
	return MainTableName
}

// UpdateKeys sets the GSI keys based on the current values
func (s *StreamingPreferences) UpdateKeys() error {
	// Validate required fields
	if s.Username == "" {
		return fmt.Errorf("username is required")
	}

	// Set primary keys
	s.PK = fmt.Sprintf("STREAMING_PREFS#%s", s.Username)

	// Note: SK should be set by helper methods (SetCurrentPreference, SetVersionedPreference, etc.)
	// If not set, default to CURRENT
	if s.SK == "" {
		s.SK = SKCurrent
	}

	// Set GSI1 keys for user-based queries
	s.GSI1PK = fmt.Sprintf(KeyPatternUser, s.Username)
	s.GSI1SK = fmt.Sprintf("STREAMING_PREFS#%s", s.UpdatedAt.Format(time.RFC3339))

	// Set GSI2 keys for device-based queries (only if device ID is present)
	if s.DeviceID != "" {
		s.GSI2PK = fmt.Sprintf(KeyPatternDevice, s.DeviceID)
		s.GSI2SK = fmt.Sprintf("STREAMING_PREFS#%s", s.Username)
	}

	return nil
}

// GetPK returns the partition key
func (s *StreamingPreferences) GetPK() string {
	return s.PK
}

// GetSK returns the sort key
func (s *StreamingPreferences) GetSK() string {
	return s.SK
}

// SetCurrentPreference sets this as the current preference for a user
func (s *StreamingPreferences) SetCurrentPreference() {
	s.SK = SKCurrent
	_ = s.UpdateKeys() // Ignore error as this is internal model operation
}

// SetVersionedPreference sets this as a versioned preference in history
func (s *StreamingPreferences) SetVersionedPreference() {
	s.SK = fmt.Sprintf("VERSION#%d#%s", s.Version, s.UpdatedAt.Format(time.RFC3339))
	_ = s.UpdateKeys() // Ignore error as this is internal model operation

	// Set TTL to 30 days for version history
	s.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
}

// SetDevicePreference sets this as a device-specific preference
func (s *StreamingPreferences) SetDevicePreference(deviceID string) {
	s.DeviceID = deviceID
	s.SK = fmt.Sprintf(KeyPatternDevice, deviceID)
	_ = s.UpdateKeys() // Ignore error as this is internal model operation
}

// SetBackupPreference sets this as a backup before migration
func (s *StreamingPreferences) SetBackupPreference() {
	s.SK = fmt.Sprintf("BACKUP#%s", time.Now().Format(time.RFC3339))
	_ = s.UpdateKeys() // Ignore error as this is internal model operation

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
