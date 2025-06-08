package streaming

import (
	"time"
)

// Quality represents video quality levels
type Quality string

const (
	QualityAuto  Quality = "auto"
	Quality4K    Quality = "4k"
	Quality1080p Quality = "1080p"
	Quality720p  Quality = "720p"
	Quality480p  Quality = "480p"
	Quality360p  Quality = "360p"
	Quality240p  Quality = "240p"
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

// MediaFormat represents the streaming format
type MediaFormat string

const (
	FormatHLS  MediaFormat = "hls"
	FormatDASH MediaFormat = "dash"
)

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

// StreamingSession represents an active streaming session
type StreamingSession struct {
	SessionID        string
	UserID           string
	MediaID          string
	Format           MediaFormat
	CurrentQuality   Quality
	StartTime        time.Time
	LastSegmentIndex int
	BytesTransferred int64
	BufferHealth     float64 // 0.0 to 1.0
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
}

// ProcessingStatus represents the processing status of media
type ProcessingStatus string

const (
	StatusPending    ProcessingStatus = "pending"
	StatusProcessing ProcessingStatus = "processing"
	StatusComplete   ProcessingStatus = "complete"
	StatusFailed     ProcessingStatus = "failed"
)

// StreamingConfig holds configuration for the streaming service
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

// Error types
type StreamingError struct {
	Code    string
	Message string
	MediaID string
	Details map[string]interface{}
}

func (e *StreamingError) Error() string {
	return e.Message
}

// Helper functions
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
