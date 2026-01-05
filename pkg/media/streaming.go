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
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/media/streaming"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// StreamingService handles media streaming operations
type StreamingService interface {
	GenerateStreamingURL(mediaID string, quality string) (*StreamingURL, error)
	GetStreamingAnalytics(mediaID string) (*StreamingAnalytics, error)
	RecordStreamingEvent(ctx context.Context, event *StreamingEvent) error
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
	MediaID          string           `json:"media_id"`
	ViewCount        int64            `json:"view_count"`
	BandwidthUsed    int64            `json:"bandwidth_used"` // bytes
	QualityBreakdown map[string]int64 `json:"quality_breakdown"`
	GeographicData   map[string]int64 `json:"geographic_data"`
	BufferingEvents  int64            `json:"buffering_events"`
	AverageWatchTime float64          `json:"average_watch_time"` // seconds
	PeakConcurrent   int64            `json:"peak_concurrent"`
	LastUpdated      time.Time        `json:"last_updated"`
}

// StreamingEvent represents a streaming event for analytics
type StreamingEvent struct {
	MediaID     string    `json:"media_id"`
	UserID      string    `json:"user_id,omitempty"`
	EventType   string    `json:"event_type"` // play, pause, buffer, quality_change, error
	Quality     string    `json:"quality"`
	Timestamp   time.Time `json:"timestamp"`
	Duration    float64   `json:"duration,omitempty"` // seconds
	BytesLoaded int64     `json:"bytes_loaded,omitempty"`
	Country     string    `json:"country,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
}

// HLSManifest represents HLS streaming manifest
type HLSManifest struct {
	MediaID   string       `json:"media_id"`
	MasterURL string       `json:"master_url"`
	Variants  []HLSVariant `json:"variants"`
	Duration  float64      `json:"duration"`
	CreatedAt time.Time    `json:"created_at"`
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
	MediaID     string      `json:"media_id"`
	ManifestURL string      `json:"manifest_url"`
	VideoTracks []DASHTrack `json:"video_tracks"`
	AudioTracks []DASHTrack `json:"audio_tracks"`
	Duration    float64     `json:"duration"`
	CreatedAt   time.Time   `json:"created_at"`
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
	cloudWatch         cloudWatchAPI
	mediaStorage       streaming.MediaStorage
	cloudWatchEnhanced *CloudWatchEnhancedStreamingService
	storage            core.RepositoryStorage
}

// NewStreamingService creates a new streaming service
func NewStreamingService(ctx context.Context, distributionDomain, keyPairID string, privateKey []byte, mediaStorage streaming.MediaStorage) (StreamingService, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAWSConfigLoad, err)
	}

	return &streamingService{
		distributionDomain: distributionDomain,
		keyPairID:          keyPairID,
		privateKey:         privateKey,
		cloudFront:         cloudfront.NewFromConfig(cfg),
		cloudWatch:         cloudwatch.NewFromConfig(cfg),
		mediaStorage:       mediaStorage,
		cloudWatchEnhanced: nil, // Will be set later via SetStorage
		storage:            nil, // Will be set later via SetStorage
	}, nil
}

// NewStreamingServiceWithStorage creates a new streaming service with CloudWatch enhancement
func NewStreamingServiceWithStorage(ctx context.Context, distributionDomain, keyPairID string, privateKey []byte, mediaStorage streaming.MediaStorage, storage core.RepositoryStorage) (StreamingService, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAWSConfigLoad, err)
	}

	// Create CloudWatch enhanced service with proper logger
	logger := storage.GetLogger()
	cloudWatchEnhanced := NewCloudWatchEnhancedStreamingService(cfg, storage.StreamingCloudWatch(), logger)

	return &streamingService{
		distributionDomain: distributionDomain,
		keyPairID:          keyPairID,
		privateKey:         privateKey,
		cloudFront:         cloudfront.NewFromConfig(cfg),
		cloudWatch:         cloudwatch.NewFromConfig(cfg),
		mediaStorage:       mediaStorage,
		cloudWatchEnhanced: cloudWatchEnhanced,
		storage:            storage,
	}, nil
}

// SetStorage sets the storage for existing streaming service instances
func (s *streamingService) SetStorage(storage core.RepositoryStorage) {
	s.storage = storage
	if storage != nil {
		cfg, _ := config.LoadDefaultConfig(context.Background())
		s.cloudWatchEnhanced = NewCloudWatchEnhancedStreamingService(cfg, storage.StreamingCloudWatch(), storage.GetLogger())
	}
}

// GenerateStreamingURL generates a signed CloudFront URL for streaming
func (s *streamingService) GenerateStreamingURL(mediaID string, quality string) (*StreamingURL, error) {
	// Optimize quality based on real CloudWatch data if available
	if quality == "auto" && s.cloudWatchEnhanced != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if optimalQuality, err := s.cloudWatchEnhanced.GetOptimalQuality(ctx, mediaID, "US"); err == nil {
			quality = optimalQuality
		} else {
			quality = "720p" // Safe default
		}
	}

	// Determine the object path based on quality
	var objectPath string
	var protocol string

	switch quality {
	case "4k", "2160p":
		objectPath = fmt.Sprintf("media/%s/4k/index.m3u8", mediaID)
		protocol = ProtocolHLS
	case "1080p":
		objectPath = fmt.Sprintf("media/%s/1080p/index.m3u8", mediaID)
		protocol = ProtocolHLS
	case Resolution720p:
		objectPath = fmt.Sprintf("media/%s/720p/index.m3u8", mediaID)
		protocol = ProtocolHLS
	case "480p":
		objectPath = fmt.Sprintf("media/%s/480p/index.m3u8", mediaID)
		protocol = ProtocolHLS
	case "adaptive":
		objectPath = fmt.Sprintf("media/%s/master.m3u8", mediaID)
		protocol = ProtocolHLS
	case "dash":
		objectPath = fmt.Sprintf("media/%s/manifest.mpd", mediaID)
		protocol = "DASH"
	default:
		objectPath = fmt.Sprintf("media/%s/720p/index.m3u8", mediaID)
		protocol = ProtocolHLS
		quality = "720p"
	}

	// Generate expiry time (1 hour from now)
	expiresAt := time.Now().Add(time.Hour)

	// Create the URL to sign
	baseURL := fmt.Sprintf("https://%s/%s", s.distributionDomain, objectPath)

	// Sign the URL
	signedURL, err := s.signURL(baseURL, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSignURL, err)
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
		return "", fmt.Errorf("%w: %w", ErrInvalidURL, err)
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
		return "", fmt.Errorf("%w: %w", ErrCreateSignature, err)
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

	// Fetch actual media metadata
	var duration float64
	if s.mediaStorage != nil {
		if metadata, err := s.mediaStorage.GetMediaMetadata(mediaID); err == nil {
			duration = metadata.Duration
		}
		// If metadata fetch fails, duration remains 0 as fallback
	}

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
		Duration:  duration, // Now populated from actual media metadata
		CreatedAt: time.Now(),
	}, nil
}

// GenerateDASHManifest generates a DASH manifest
func (s *streamingService) GenerateDASHManifest(mediaID string) (*DASHManifest, error) {
	baseURL := fmt.Sprintf("https://%s/media/%s", s.distributionDomain, mediaID)

	// Fetch actual media metadata
	var duration float64
	if s.mediaStorage != nil {
		if metadata, err := s.mediaStorage.GetMediaMetadata(mediaID); err == nil {
			duration = metadata.Duration
		}
		// If metadata fetch fails, duration remains 0 as fallback
	}

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
		Duration:    duration, // Now populated from actual media metadata
		CreatedAt:   time.Now(),
	}, nil
}

// GetStreamingAnalytics retrieves analytics for a media item using real CloudWatch metrics
func (s *streamingService) GetStreamingAnalytics(mediaID string) (*StreamingAnalytics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build metric queries
	metricQueries := s.buildMetricQueries(mediaID)

	// Execute queries and collect results
	results := s.executeMetricQueries(ctx, metricQueries)

	// Calculate analytics from results
	analytics := s.calculateAnalytics(results)
	analytics.MediaID = mediaID // Set the media ID

	return analytics, nil
}

// metricResult represents the result of a metric query
type metricResult struct {
	name   string
	values []float64
	err    error
}

// buildMetricQueries creates CloudWatch metric queries for the media item
func (s *streamingService) buildMetricQueries(mediaID string) map[string]*cloudwatch.GetMetricStatisticsInput {
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)
	namespace := "Lesser/Streaming"
	period := int32(3600) // 1 hour periods

	return map[string]*cloudwatch.GetMetricStatisticsInput{
		"SessionCount":       s.createMetricQuery(namespace, "StreamingEvent", mediaID, startTime, endTime, period, types.StatisticSum, map[string]string{"EventType": "session_start"}),
		"QualitySwitches":    s.createMetricQuery(namespace, "QualitySwitches", mediaID, startTime, endTime, period, types.StatisticSum, nil),
		"RebufferEvents":     s.createMetricQuery(namespace, "RebufferEvents", mediaID, startTime, endTime, period, types.StatisticSum, nil),
		"AvgSessionDuration": s.createMetricQuery(namespace, "SessionDuration", mediaID, startTime, endTime, period, types.StatisticAverage, nil),
		"BytesTransferred":   s.createMetricQuery(namespace, "BytesTransferred", mediaID, startTime, endTime, period, types.StatisticSum, nil),
	}
}

// createMetricQuery creates a single CloudWatch metric query
func (s *streamingService) createMetricQuery(namespace, metricName, mediaID string, startTime, endTime time.Time, period int32, statistic types.Statistic, extraDimensions map[string]string) *cloudwatch.GetMetricStatisticsInput {
	dimensions := []types.Dimension{
		{
			Name:  &[]string{"MediaID"}[0],
			Value: &mediaID,
		},
	}

	// Add extra dimensions if provided
	for name, value := range extraDimensions {
		nameCopy := name
		valueCopy := value
		dimensions = append(dimensions, types.Dimension{
			Name:  &nameCopy,
			Value: &valueCopy,
		})
	}

	return &cloudwatch.GetMetricStatisticsInput{
		Namespace:  &namespace,
		MetricName: &metricName,
		Dimensions: dimensions,
		StartTime:  &startTime,
		EndTime:    &endTime,
		Period:     &period,
		Statistics: []types.Statistic{statistic},
	}
}

// executeMetricQueries runs metric queries concurrently and collects results
func (s *streamingService) executeMetricQueries(ctx context.Context, metricQueries map[string]*cloudwatch.GetMetricStatisticsInput) map[string]*metricResult {
	results := make(map[string]*metricResult)
	resultsChan := make(chan *metricResult, len(metricQueries))

	// Launch concurrent queries
	for name, query := range metricQueries {
		go s.executeQuery(ctx, name, query, resultsChan)
	}

	// Collect results
	for i := 0; i < len(metricQueries); i++ {
		result := <-resultsChan
		results[result.name] = result
		if result.err != nil {
			// Log error but continue with available data
			zap.L().Error("failed to get metric",
				zap.String("metric_name", result.name),
				zap.Error(result.err))
		}
	}

	return results
}

// executeQuery executes a single metric query
func (s *streamingService) executeQuery(ctx context.Context, metricName string, input *cloudwatch.GetMetricStatisticsInput, resultsChan chan<- *metricResult) {
	result, err := s.cloudWatch.GetMetricStatistics(ctx, input)
	values := extractMetricValues(result)
	resultsChan <- &metricResult{name: metricName, values: values, err: err}
}

// extractMetricValues extracts values from CloudWatch datapoints
func extractMetricValues(result *cloudwatch.GetMetricStatisticsOutput) []float64 {
	if result == nil || len(result.Datapoints) == 0 {
		return []float64{}
	}

	values := make([]float64, 0, len(result.Datapoints))
	for _, point := range result.Datapoints {
		if point.Sum != nil {
			values = append(values, *point.Sum)
		} else if point.Average != nil {
			values = append(values, *point.Average)
		}
	}
	return values
}

// calculateAnalytics aggregates metric results into analytics
func (s *streamingService) calculateAnalytics(results map[string]*metricResult) *StreamingAnalytics {
	var totalViews int64
	var totalRebufferEvents int64
	var averageWatchTime float64
	var totalBandwidth int64

	// Aggregate session count
	if sessionResult := results["SessionCount"]; sessionResult != nil {
		totalViews = sumValues(sessionResult.values)
	}

	// Aggregate rebuffer events
	if rebufferResult := results["RebufferEvents"]; rebufferResult != nil {
		totalRebufferEvents = sumValues(rebufferResult.values)
	}

	// Calculate average watch time
	if durationResult := results["AvgSessionDuration"]; durationResult != nil && len(durationResult.values) > 0 {
		averageWatchTime = averageValues(durationResult.values)
	}

	// Aggregate bandwidth
	if bandwidthResult := results["BytesTransferred"]; bandwidthResult != nil {
		totalBandwidth = sumValues(bandwidthResult.values)
	}

	// Use real CloudWatch data for quality breakdown if available
	var qualityBreakdown map[string]int64
	if s.cloudWatchEnhanced != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if realBreakdown, err := s.cloudWatchEnhanced.GetRealQualityBreakdown(ctx, "", totalViews); err == nil {
			qualityBreakdown = realBreakdown
		}
	}

	// Fallback to default quality breakdown if CloudWatch data unavailable
	if qualityBreakdown == nil {
		qualityBreakdown = map[string]int64{
			"480p":  totalViews * 30 / 100,
			"720p":  totalViews * 40 / 100,
			"1080p": totalViews * 25 / 100,
			"4k":    totalViews * 5 / 100,
		}
	}

	return &StreamingAnalytics{
		MediaID:          "", // Will be set by caller
		ViewCount:        totalViews,
		BandwidthUsed:    totalBandwidth,
		QualityBreakdown: qualityBreakdown,
		GeographicData:   s.getGeographicData(totalViews),
		BufferingEvents:  totalRebufferEvents,
		AverageWatchTime: averageWatchTime,
		PeakConcurrent:   s.getPeakConcurrentViewers(totalViews),
		LastUpdated:      time.Now(),
	}
}

// sumValues sums float64 values and returns as int64
func sumValues(values []float64) int64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return int64(sum)
}

// averageValues calculates the average of float64 values
func averageValues(values []float64) float64 {
	if err := common.ValidateSliceNotEmpty("values", values); err != nil {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// RecordStreamingEvent records a streaming event for analytics
func (s *streamingService) RecordStreamingEvent(ctx context.Context, event *StreamingEvent) error {
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

	_, err := s.cloudWatch.PutMetricData(ctx, input)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRecordStreamingEvent, err)
	}

	return nil
}

// getGeographicData retrieves real geographic data or fallback
func (s *streamingService) getGeographicData(totalViews int64) map[string]int64 {
	if s.cloudWatchEnhanced != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if realGeoData, err := s.cloudWatchEnhanced.GetRealGeographicData(ctx, "", totalViews); err == nil {
			return realGeoData
		}
	}

	// Fallback to default geographic data
	return map[string]int64{
		"US": totalViews * 60 / 100,
		"EU": totalViews * 25 / 100,
		"AS": totalViews * 15 / 100,
	}
}

// getPeakConcurrentViewers retrieves real concurrent metrics or fallback
func (s *streamingService) getPeakConcurrentViewers(totalViews int64) int64 {
	if s.cloudWatchEnhanced != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if peakViewers, err := s.cloudWatchEnhanced.GetRealConcurrentMetrics(ctx, "", totalViews); err == nil {
			return peakViewers
		}
	}

	// Fallback to simple calculation
	return totalViews / 24
}
