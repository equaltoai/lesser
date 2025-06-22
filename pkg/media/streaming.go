package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// StreamingService handles media streaming operations
type StreamingService interface {
	GenerateStreamingURL(mediaID string, quality string) (*StreamingURL, error)
	GetStreamingAnalytics(mediaID string) (*StreamingAnalytics, error)
	RecordStreamingEvent(event *StreamingEvent) error
	GenerateHLSManifest(mediaID string) (*HLSManifest, error)
	GenerateDASHManifest(mediaID string) (*DASHManifest, error)
}

// StreamingURL represents a signed streaming URL
type StreamingURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Quality   string    `json:"quality"`
	Protocol  string    `json:"protocol"` // HLS, DASH, Progressive
}

// StreamingAnalytics contains analytics for a media item
type StreamingAnalytics struct {
	MediaID           string            `json:"media_id"`
	ViewCount         int64             `json:"view_count"`
	BandwidthUsed     int64             `json:"bandwidth_used"` // bytes
	QualityBreakdown  map[string]int64  `json:"quality_breakdown"`
	GeographicData    map[string]int64  `json:"geographic_data"`
	BufferingEvents   int64             `json:"buffering_events"`
	AverageWatchTime  float64           `json:"average_watch_time"` // seconds
	PeakConcurrent    int64             `json:"peak_concurrent"`
	LastUpdated       time.Time         `json:"last_updated"`
}

// StreamingEvent represents a streaming event for analytics
type StreamingEvent struct {
	MediaID     string    `json:"media_id"`
	UserID      string    `json:"user_id,omitempty"`
	EventType   string    `json:"event_type"` // play, pause, buffer, quality_change, error
	Quality     string    `json:"quality"`
	Timestamp   time.Time `json:"timestamp"`
	Duration    float64   `json:"duration,omitempty"`    // seconds
	BytesLoaded int64     `json:"bytes_loaded,omitempty"`
	Country     string    `json:"country,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
}

// HLSManifest represents HLS streaming manifest
type HLSManifest struct {
	MediaID   string               `json:"media_id"`
	MasterURL string               `json:"master_url"`
	Variants  []HLSVariant         `json:"variants"`
	Duration  float64              `json:"duration"`
	CreatedAt time.Time            `json:"created_at"`
}

// HLSVariant represents a quality variant in HLS
type HLSVariant struct {
	Quality    string `json:"quality"`
	Bandwidth  int    `json:"bandwidth"`
	Resolution string `json:"resolution"`
	Codecs     string `json:"codecs"`
	URL        string `json:"url"`
}

// DASHManifest represents DASH streaming manifest
type DASHManifest struct {
	MediaID         string            `json:"media_id"`
	ManifestURL     string            `json:"manifest_url"`
	VideoTracks     []DASHTrack       `json:"video_tracks"`
	AudioTracks     []DASHTrack       `json:"audio_tracks"`
	Duration        float64           `json:"duration"`
	CreatedAt       time.Time         `json:"created_at"`
}

// DASHTrack represents a track in DASH
type DASHTrack struct {
	ID         string `json:"id"`
	Type       string `json:"type"` // video, audio
	Quality    string `json:"quality"`
	Bandwidth  int    `json:"bandwidth"`
	Resolution string `json:"resolution,omitempty"`
	Codecs     string `json:"codecs"`
	URL        string `json:"url"`
}

// streamingService implements StreamingService
type streamingService struct {
	distributionDomain string
	keyPairID          string
	privateKey         []byte
	cloudFront         *cloudfront.Client
	cloudWatch         *cloudwatch.Client
}

// NewStreamingService creates a new streaming service
func NewStreamingService(distributionDomain, keyPairID string, privateKey []byte) (StreamingService, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &streamingService{
		distributionDomain: distributionDomain,
		keyPairID:          keyPairID,
		privateKey:         privateKey,
		cloudFront:         cloudfront.NewFromConfig(cfg),
		cloudWatch:         cloudwatch.NewFromConfig(cfg),
	}, nil
}

// GenerateStreamingURL generates a signed CloudFront URL for streaming
func (s *streamingService) GenerateStreamingURL(mediaID string, quality string) (*StreamingURL, error) {
	// Determine the object path based on quality
	var objectPath string
	var protocol string
	
	switch quality {
	case "4k", "2160p":
		objectPath = fmt.Sprintf("media/%s/4k/index.m3u8", mediaID)
		protocol = "HLS"
	case "1080p":
		objectPath = fmt.Sprintf("media/%s/1080p/index.m3u8", mediaID)
		protocol = "HLS"
	case "720p":
		objectPath = fmt.Sprintf("media/%s/720p/index.m3u8", mediaID)
		protocol = "HLS"
	case "480p":
		objectPath = fmt.Sprintf("media/%s/480p/index.m3u8", mediaID)
		protocol = "HLS"
	case "adaptive":
		objectPath = fmt.Sprintf("media/%s/master.m3u8", mediaID)
		protocol = "HLS"
	case "dash":
		objectPath = fmt.Sprintf("media/%s/manifest.mpd", mediaID)
		protocol = "DASH"
	default:
		objectPath = fmt.Sprintf("media/%s/720p/index.m3u8", mediaID)
		protocol = "HLS"
		quality = "720p"
	}

	// Generate expiry time (1 hour from now)
	expiresAt := time.Now().Add(time.Hour)
	
	// Create the URL to sign
	baseURL := fmt.Sprintf("https://%s/%s", s.distributionDomain, objectPath)
	
	// Sign the URL
	signedURL, err := s.signURL(baseURL, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to sign URL: %w", err)
	}

	return &StreamingURL{
		URL:       signedURL,
		ExpiresAt: expiresAt,
		Quality:   quality,
		Protocol:  protocol,
	}, nil
}

// signURL creates a signed CloudFront URL
func (s *streamingService) signURL(rawURL string, expiresAt time.Time) (string, error) {
	// Parse the URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Create the policy statement
	policy := fmt.Sprintf(`{
		"Statement": [{
			"Resource": "%s",
			"Condition": {
				"DateLessThan": {
					"AWS:EpochTime": %d
				}
			}
		}]
	}`, rawURL, expiresAt.Unix())

	// Create signature
	signature, err := s.createSignature(policy)
	if err != nil {
		return "", fmt.Errorf("failed to create signature: %w", err)
	}

	// Add query parameters
	values := u.Query()
	values.Set("Expires", strconv.FormatInt(expiresAt.Unix(), 10))
	values.Set("Signature", signature)
	values.Set("Key-Pair-Id", s.keyPairID)
	u.RawQuery = values.Encode()

	return u.String(), nil
}

// createSignature creates a signature for CloudFront URL signing
func (s *streamingService) createSignature(policy string) (string, error) {
	// Create HMAC signature
	h := hmac.New(sha256.New, s.privateKey)
	h.Write([]byte(policy))
	signature := h.Sum(nil)
	
	// Base64 encode and make URL safe
	encoded := base64.StdEncoding.EncodeToString(signature)
	encoded = url.QueryEscape(encoded)
	
	return encoded, nil
}

// GenerateHLSManifest generates an HLS master manifest
func (s *streamingService) GenerateHLSManifest(mediaID string) (*HLSManifest, error) {
	baseURL := fmt.Sprintf("https://%s/media/%s", s.distributionDomain, mediaID)
	
	variants := []HLSVariant{
		{
			Quality:    "480p",
			Bandwidth:  1000000,
			Resolution: "854x480",
			Codecs:     "avc1.42001e,mp4a.40.2",
			URL:        fmt.Sprintf("%s/480p/index.m3u8", baseURL),
		},
		{
			Quality:    "720p",
			Bandwidth:  2500000,
			Resolution: "1280x720",
			Codecs:     "avc1.42001f,mp4a.40.2",
			URL:        fmt.Sprintf("%s/720p/index.m3u8", baseURL),
		},
		{
			Quality:    "1080p",
			Bandwidth:  5000000,
			Resolution: "1920x1080",
			Codecs:     "avc1.420020,mp4a.40.2",
			URL:        fmt.Sprintf("%s/1080p/index.m3u8", baseURL),
		},
		{
			Quality:    "4k",
			Bandwidth:  15000000,
			Resolution: "3840x2160",
			Codecs:     "avc1.420020,mp4a.40.2",
			URL:        fmt.Sprintf("%s/4k/index.m3u8", baseURL),
		},
	}

	return &HLSManifest{
		MediaID:   mediaID,
		MasterURL: fmt.Sprintf("%s/master.m3u8", baseURL),
		Variants:  variants,
		Duration:  0, // Would be populated from actual media metadata
		CreatedAt: time.Now(),
	}, nil
}

// GenerateDASHManifest generates a DASH manifest
func (s *streamingService) GenerateDASHManifest(mediaID string) (*DASHManifest, error) {
	baseURL := fmt.Sprintf("https://%s/media/%s", s.distributionDomain, mediaID)
	
	videoTracks := []DASHTrack{
		{
			ID:         "video-480p",
			Type:       "video",
			Quality:    "480p",
			Bandwidth:  1000000,
			Resolution: "854x480",
			Codecs:     "avc1.42001e",
			URL:        fmt.Sprintf("%s/480p/video.mp4", baseURL),
		},
		{
			ID:         "video-720p",
			Type:       "video",
			Quality:    "720p",
			Bandwidth:  2500000,
			Resolution: "1280x720",
			Codecs:     "avc1.42001f",
			URL:        fmt.Sprintf("%s/720p/video.mp4", baseURL),
		},
		{
			ID:         "video-1080p",
			Type:       "video",
			Quality:    "1080p",
			Bandwidth:  5000000,
			Resolution: "1920x1080",
			Codecs:     "avc1.420020",
			URL:        fmt.Sprintf("%s/1080p/video.mp4", baseURL),
		},
	}
	
	audioTracks := []DASHTrack{
		{
			ID:        "audio-128k",
			Type:      "audio",
			Quality:   "128k",
			Bandwidth: 128000,
			Codecs:    "mp4a.40.2",
			URL:       fmt.Sprintf("%s/audio/128k.mp4", baseURL),
		},
		{
			ID:        "audio-256k",
			Type:      "audio",
			Quality:   "256k",
			Bandwidth: 256000,
			Codecs:    "mp4a.40.2",
			URL:       fmt.Sprintf("%s/audio/256k.mp4", baseURL),
		},
	}

	return &DASHManifest{
		MediaID:     mediaID,
		ManifestURL: fmt.Sprintf("%s/manifest.mpd", baseURL),
		VideoTracks: videoTracks,
		AudioTracks: audioTracks,
		Duration:    0, // Would be populated from actual media metadata
		CreatedAt:   time.Now(),
	}, nil
}

// GetStreamingAnalytics retrieves analytics for a media item
func (s *streamingService) GetStreamingAnalytics(mediaID string) (*StreamingAnalytics, error) {
	// This would typically query from a database or analytics service
	// For now, return a placeholder with CloudWatch metrics
	
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)
	
	// Query CloudWatch for basic metrics
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  &[]string{"AWS/CloudFront"}[0],
		MetricName: &[]string{"Requests"}[0],
		Dimensions: []types.Dimension{
			{
				Name:  &[]string{"DistributionId"}[0],
				Value: &s.distributionDomain,
			},
		},
		StartTime:  &startTime,
		EndTime:    &endTime,
		Period:     &[]int32{3600}[0], // 1 hour periods
		Statistics: []types.Statistic{types.StatisticSum},
	}
	
	result, err := s.cloudWatch.GetMetricStatistics(context.TODO(), input)
	if err != nil {
		return nil, fmt.Errorf("failed to get CloudWatch metrics: %w", err)
	}
	
	var totalViews int64
	if len(result.Datapoints) > 0 {
		for _, point := range result.Datapoints {
			if point.Sum != nil {
				totalViews += int64(*point.Sum)
			}
		}
	}

	return &StreamingAnalytics{
		MediaID:     mediaID,
		ViewCount:   totalViews,
		QualityBreakdown: map[string]int64{
			"480p":  totalViews * 30 / 100,  // Estimated breakdown
			"720p":  totalViews * 40 / 100,
			"1080p": totalViews * 25 / 100,
			"4k":    totalViews * 5 / 100,
		},
		GeographicData: map[string]int64{
			"US": totalViews * 60 / 100,
			"EU": totalViews * 25 / 100,
			"AS": totalViews * 15 / 100,
		},
		BufferingEvents:  totalViews * 2 / 100, // 2% buffering rate
		AverageWatchTime: 45.0,                 // 45 seconds average
		PeakConcurrent:   totalViews / 24,      // Estimate based on 24h spread
		LastUpdated:      time.Now(),
	}, nil
}

// RecordStreamingEvent records a streaming event for analytics
func (s *streamingService) RecordStreamingEvent(event *StreamingEvent) error {
	// Send custom metric to CloudWatch
	metricData := []types.MetricDatum{
		{
			MetricName: &[]string{"StreamingEvent"}[0],
			Dimensions: []types.Dimension{
				{
					Name:  &[]string{"MediaID"}[0],
					Value: &event.MediaID,
				},
				{
					Name:  &[]string{"EventType"}[0],
					Value: &event.EventType,
				},
				{
					Name:  &[]string{"Quality"}[0],
					Value: &event.Quality,
				},
			},
			Timestamp: &event.Timestamp,
			Value:     &[]float64{1.0}[0],
			Unit:      types.StandardUnitCount,
		},
	}
	
	if event.BytesLoaded > 0 {
		metricData = append(metricData, types.MetricDatum{
			MetricName: &[]string{"BytesLoaded"}[0],
			Dimensions: []types.Dimension{
				{
					Name:  &[]string{"MediaID"}[0],
					Value: &event.MediaID,
				},
				{
					Name:  &[]string{"Quality"}[0],
					Value: &event.Quality,
				},
			},
			Timestamp: &event.Timestamp,
			Value:     &[]float64{float64(event.BytesLoaded)}[0],
			Unit:      types.StandardUnitBytes,
		})
	}
	
	input := &cloudwatch.PutMetricDataInput{
		Namespace:  &[]string{"Lesser/Streaming"}[0],
		MetricData: metricData,
	}
	
	_, err := s.cloudWatch.PutMetricData(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to record streaming event: %w", err)
	}
	
	return nil
}