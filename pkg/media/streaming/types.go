package streaming

import (
	"time"

	"github.com/equaltoai/lesser/pkg/storage/types"
)

// Quality represents video quality level
type Quality = types.Quality

// MediaFormat represents streaming media format
type MediaFormat = types.MediaFormat

// StreamingSession represents an active streaming session
//
//nolint:revive // Type alias for storage type
type StreamingSession = types.StreamingSession

// Re-export constants
const (
	QualityAuto   = types.QualityAuto
	QualityLow    = types.QualityLow
	QualityMedium = types.QualityMedium
	QualityHigh   = types.QualityHigh
	QualitySource = types.QualitySource

	// Additional quality levels specific to streaming
	Quality4K    Quality = "4k"
	Quality1080p Quality = "1080p"
	Quality720p  Quality = "720p"
	Quality480p  Quality = "480p"
	Quality360p  Quality = "360p"
	Quality240p  Quality = "240p"
)

// Streaming format constants
const (
	FormatHLS    = types.FormatHLS
	FormatDASH   = types.FormatDASH
	FormatSource = types.FormatSource
)

// QualityInfo contains information about a quality level
type QualityInfo struct {
	Quality    Quality
	Width      int
	Height     int
	Bitrate    int    // in kbps
	Bandwidth  int    // required bandwidth in kbps
	Resolution string // e.g., "1920x1080"
}

// Segment represents a media segment
type Segment struct {
	Index    int
	Duration float64 // in seconds
	URL      string
	Size     int64 // in bytes
}

// HLSManifest represents an HLS master playlist
type HLSManifest struct {
	MediaID       string
	Duration      float64
	Variants      []HLSVariant
	MasterURL     string
	GeneratedAt   time.Time
	CacheDuration time.Duration
}

// HLSVariant represents a quality variant in HLS
type HLSVariant struct {
	Quality     Quality
	Bandwidth   int
	Resolution  string
	PlaylistURL string
	Codecs      string
}

// DASHManifest represents a DASH MPD (Media Presentation Description)
type DASHManifest struct {
	MediaID         string
	Duration        float64
	MinBufferTime   float64
	Representations []DASHRepresentation
	ManifestURL     string
	GeneratedAt     time.Time
	CacheDuration   time.Duration

	// Live streaming specific fields
	IsLive                     bool
	AvailabilityStartTime      time.Time
	PublishTime                time.Time
	TimeShiftBufferDepth       float64 // in seconds
	SuggestedPresentationDelay float64 // in seconds
	MinimumUpdatePeriod        float64 // in seconds
}

// DASHRepresentation represents a quality representation in DASH
type DASHRepresentation struct {
	ID              string
	Quality         Quality
	Bandwidth       int
	Width           int
	Height          int
	Codecs          string
	BaseURL         string
	SegmentTemplate string
}

// BandwidthStats tracks bandwidth usage for a user
type BandwidthStats struct {
	UserID            string
	TotalBytes        int64
	SessionBytes      int64
	AverageBandwidth  int // in kbps
	PeakBandwidth     int // in kbps
	LastMeasurement   time.Time
	MeasurementWindow time.Duration
}

// BandwidthMeasurement represents a single bandwidth measurement
type BandwidthMeasurement struct {
	UserID    string
	Bandwidth int // in kbps
	Timestamp time.Time
}

// MediaStreamer is the main interface for media streaming functionality
type MediaStreamer interface {
	// Manifest generation
	GenerateHLSManifest(mediaID string) (*HLSManifest, error)
	GenerateDASHManifest(mediaID string) (*DASHManifest, error)

	// Segment management
	GetSegmentURL(mediaID string, quality Quality, segment int) (string, error)
	GetSegmentURLs(mediaID string, quality Quality, startSegment, count int) ([]string, error)

	// Bandwidth tracking
	TrackBandwidth(userID string, bytesTransferred int64) error
	GetBandwidthStats(userID string) (*BandwidthStats, error)

	// Quality selection
	GetOptimalQuality(userID string, availableBandwidth int) Quality
	GetAvailableQualities(mediaID string) ([]QualityInfo, error)

	// Session management
	StartSession(userID, mediaID string, format MediaFormat) (*StreamingSession, error)
	UpdateSession(sessionID string, quality Quality, segmentIndex int, bytesTransferred int64) error
	EndSession(sessionID string) error
	GetSession(sessionID string) (*StreamingSession, error)
}

// MediaStorage interface for interacting with media files
type MediaStorage interface {
	GetManifestPath(mediaID string, format MediaFormat, quality Quality) string
	GetSegmentPath(mediaID string, quality Quality, segmentIndex int) string
	GetMediaMetadata(mediaID string) (*MediaMetadata, error)
	ManifestExists(mediaID string, format MediaFormat) (bool, error)
	GetKeyframeData(mediaID string, quality Quality) ([]byte, error)
}

// MediaMetadata contains metadata about a media file
type MediaMetadata struct {
	MediaID            string
	OriginalURL        string
	Duration           float64
	Width              int
	Height             int
	Bitrate            int
	FileSize           int64
	ProcessedAt        time.Time
	AvailableQualities []Quality
	Status             ProcessingStatus

	// Codec information for HLS/DASH manifest generation
	VideoCodec      string                       `json:"video_codec,omitempty"`      // e.g., "avc1.640028"
	AudioCodec      string                       `json:"audio_codec,omitempty"`      // e.g., "mp4a.40.2"
	VideoProfile    string                       `json:"video_profile,omitempty"`    // e.g., "High", "Main", "Baseline"
	VideoLevel      string                       `json:"video_level,omitempty"`      // e.g., "4.0", "3.1"
	QualitySettings map[Quality]QualityCodecInfo `json:"quality_settings,omitempty"` // Per-quality codec info

	// Live streaming specific fields
	IsLive    bool      `json:"is_live,omitempty"`
	StartTime time.Time `json:"start_time,omitempty"`

	// I-frame/keyframe information for trick play
	KeyframePositions []float64 `json:"keyframe_positions,omitempty"` // PTS positions of keyframes in seconds
}

// QualityCodecInfo contains codec information for a specific quality level
type QualityCodecInfo struct {
	VideoCodec string `json:"video_codec"` // H.264 profile/level string like "avc1.640028"
	AudioCodec string `json:"audio_codec"` // Audio codec string like "mp4a.40.2"
	Bandwidth  int    `json:"bandwidth"`   // Required bandwidth in bps
	Width      int    `json:"width"`       // Video width in pixels
	Height     int    `json:"height"`      // Video height in pixels
}

// ProcessingStatus represents the processing status of media
type ProcessingStatus string

// Processing status constants
const (
	StatusPending    ProcessingStatus = "pending"
	StatusProcessing ProcessingStatus = "processing"
	StatusComplete   ProcessingStatus = "complete"
	StatusFailed     ProcessingStatus = "failed"
)

// StreamingConfig holds configuration for the streaming service
//
//nolint:revive // Streaming prefix clarifies this is streaming-specific config
type StreamingConfig struct {
	CDNBaseURL         string
	S3Bucket           string
	S3Region           string
	SegmentDuration    int // in seconds
	ManifestCacheTTL   time.Duration
	MaxConcurrentJobs  int
	EnableCostTracking bool
	DefaultQuality     Quality

	// Bandwidth thresholds for quality selection (in kbps)
	Bandwidth4K    int
	Bandwidth1080p int
	Bandwidth720p  int
	Bandwidth480p  int
	Bandwidth360p  int
	Bandwidth240p  int
}

// QualitySelector interface for adaptive bitrate selection
type QualitySelector interface {
	SelectQuality(bandwidth int, bufferHealth float64, availableQualities []Quality) Quality
	UpdateMetrics(sessionID string, rebufferEvents int, qualitySwitches int)
	GetQualityMetrics(sessionID string) *QualityMetrics
}

// QualityMetrics tracks quality selection metrics
type QualityMetrics struct {
	SessionID         string
	AverageQuality    Quality
	QualitySwitches   int
	RebufferEvents    int
	TimeInEachQuality map[Quality]time.Duration
	LastQualityChange time.Time
}

// StreamingError represents an error in streaming operations
//
//nolint:revive // Streaming prefix clarifies this is streaming-specific error
type StreamingError struct {
	Code    string
	Message string
	MediaID string
	Details map[string]any
}

func (e *StreamingError) Error() string {
	return e.Message
}

// GetQualityInfo returns quality information for a given quality level
func GetQualityInfo(quality Quality) QualityInfo {
	switch quality {
	case Quality4K:
		return QualityInfo{
			Quality:    Quality4K,
			Width:      3840,
			Height:     2160,
			Bitrate:    15000,
			Bandwidth:  20000,
			Resolution: "3840x2160",
		}
	case Quality1080p:
		return QualityInfo{
			Quality:    Quality1080p,
			Width:      1920,
			Height:     1080,
			Bitrate:    5000,
			Bandwidth:  8000,
			Resolution: "1920x1080",
		}
	case Quality720p:
		return QualityInfo{
			Quality:    Quality720p,
			Width:      1280,
			Height:     720,
			Bitrate:    2500,
			Bandwidth:  4000,
			Resolution: "1280x720",
		}
	case Quality480p:
		return QualityInfo{
			Quality:    Quality480p,
			Width:      854,
			Height:     480,
			Bitrate:    1000,
			Bandwidth:  2000,
			Resolution: "854x480",
		}
	case Quality360p:
		return QualityInfo{
			Quality:    Quality360p,
			Width:      640,
			Height:     360,
			Bitrate:    600,
			Bandwidth:  1000,
			Resolution: "640x360",
		}
	case Quality240p:
		return QualityInfo{
			Quality:    Quality240p,
			Width:      426,
			Height:     240,
			Bitrate:    300,
			Bandwidth:  500,
			Resolution: "426x240",
		}
	default:
		return QualityInfo{
			Quality:    Quality480p,
			Width:      854,
			Height:     480,
			Bitrate:    1000,
			Bandwidth:  2000,
			Resolution: "854x480",
		}
	}
}

// GetQualitiesByBandwidth returns qualities that can be supported by the given bandwidth
func GetQualitiesByBandwidth(bandwidth int) []Quality {
	var qualities []Quality

	qualityMap := map[Quality]int{
		Quality240p:  500,
		Quality360p:  1000,
		Quality480p:  2000,
		Quality720p:  4000,
		Quality1080p: 8000,
		Quality4K:    20000,
	}

	// Add qualities in order from lowest to highest
	for _, q := range []Quality{Quality240p, Quality360p, Quality480p, Quality720p, Quality1080p, Quality4K} {
		if bandwidth >= qualityMap[q] {
			qualities = append(qualities, q)
		}
	}

	return qualities
}
