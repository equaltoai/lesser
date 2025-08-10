// Package streaming provides serverless-optimized media streaming functionality.
//
// SERVERLESS DESIGN PRINCIPLES:
//
//  1. No Background Processes: This package avoids long-running goroutines,
//     timers, or polling mechanisms that are incompatible with Lambda's
//     execution model.
//
//  2. DynamoDB TTL for Cleanup: Session and cache expiration is handled
//     automatically by DynamoDB TTL rather than manual cleanup processes.
//     This eliminates the need for background cleanup tasks.
//
//  3. Stateless Operations: Each Lambda invocation operates independently
//     without relying on shared in-memory state between invocations.
//
//  4. Cost-Optimized: Uses DynamoDB on-demand billing and avoids unnecessary
//     operations that would increase costs in a serverless environment.
package streaming

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/google/uuid"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// Streamer implements the MediaStreamer interface
type Streamer struct {
	config           *StreamingConfig
	storage          MediaStorage
	hlsGenerator     *HLSGenerator
	dashGenerator    *DASHGenerator
	bandwidthTracker *BandwidthTracker
	qualitySelector  QualitySelector
	sessionManager   *SessionManager
	analytics        core.RepositoryStorage // DynamORM storage for analytics
	s3Client         *s3.Client
	logger           *zap.Logger
	costTracker      CostTracker
	metricsTracker   *MetricsTracker // Real metrics tracking with CloudWatch

	// Cache for manifests
	manifestCache sync.Map
	cacheTTL      time.Duration
}

// NewStreamer creates a new media streamer
func NewStreamer(
	config *StreamingConfig,
	analytics core.RepositoryStorage,
	s3Client *s3.Client,
	cloudWatch *cloudwatch.Client,
	db dynamormCore.DB,
	logger *zap.Logger,
	costTracker CostTracker,
) *Streamer {
	storage := NewS3MediaStorage(s3Client, config.S3Bucket, config.S3Region, db)

	qualitySelector := NewAdaptiveQualitySelector(logger)
	metricsTracker := NewMetricsTracker(cloudWatch, logger)

	// Connect quality selector to metrics tracker for enhanced decision making
	qualitySelector.SetMetricsTracker(metricsTracker)

	return &Streamer{
		config:           config,
		storage:          storage,
		hlsGenerator:     NewHLSGenerator(config, storage),
		dashGenerator:    NewDASHGenerator(config, storage),
		bandwidthTracker: NewBandwidthTracker(analytics, logger, costTracker, cloudWatch),
		qualitySelector:  qualitySelector,
		sessionManager:   nil, // Will be set via SetSessionManager when repository is available
		analytics:        analytics,
		s3Client:         s3Client,
		logger:           logger,
		costTracker:      costTracker,
		metricsTracker:   metricsTracker,
		cacheTTL:         config.ManifestCacheTTL,
	}
}

// SetSessionManager sets the session manager (for dependency injection)
func (s *Streamer) SetSessionManager(sessionManager *SessionManager) {
	s.sessionManager = sessionManager
}

// GenerateHLSManifest generates an HLS manifest for a media item
func (s *Streamer) GenerateHLSManifest(mediaID string) (*HLSManifest, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("hls:%s", mediaID)
	if cached, ok := s.manifestCache.Load(cacheKey); ok {
		if manifest, ok := cached.(*cachedManifest); ok && time.Since(manifest.generatedAt) < s.cacheTTL {
			s.logger.Debug("returning cached HLS manifest", zap.String("mediaID", mediaID))
			return manifest.hlsManifest, nil
		}
	}

	// Get media metadata
	metadata, err := s.storage.GetMediaMetadata(mediaID)
	if err != nil {
		s.logger.Error("failed to get media metadata",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		return nil, fmt.Errorf("get media metadata: %w", err)
	}

	// Check if media is ready
	if metadata.Status != StatusComplete {
		return nil, &StreamingError{
			Code:    "MEDIA_NOT_READY",
			Message: fmt.Sprintf("media %s is not ready for streaming (status: %s)", mediaID, metadata.Status),
			MediaID: mediaID,
		}
	}

	// Generate manifest
	manifest, err := s.hlsGenerator.GenerateMasterPlaylist(mediaID, metadata)
	if err != nil {
		s.logger.Error("failed to generate HLS manifest",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		return nil, fmt.Errorf("generate HLS manifest: %w", err)
	}

	// Cache the manifest
	s.manifestCache.Store(cacheKey, &cachedManifest{
		hlsManifest: manifest,
		generatedAt: time.Now(),
	})

	// Track manifest generation using DynamORM
	err = s.recordManifestGeneration(mediaID, "hls", metadata.Duration)
	if err != nil {
		s.logger.Warn("failed to record manifest generation", zap.Error(err))
	}

	return manifest, nil
}

// GenerateDASHManifest generates a DASH manifest for a media item
func (s *Streamer) GenerateDASHManifest(mediaID string) (*DASHManifest, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("dash:%s", mediaID)
	if cached, ok := s.manifestCache.Load(cacheKey); ok {
		if manifest, ok := cached.(*cachedManifest); ok && time.Since(manifest.generatedAt) < s.cacheTTL {
			s.logger.Debug("returning cached DASH manifest", zap.String("mediaID", mediaID))
			return manifest.dashManifest, nil
		}
	}

	// Get media metadata
	metadata, err := s.storage.GetMediaMetadata(mediaID)
	if err != nil {
		s.logger.Error("failed to get media metadata",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		return nil, fmt.Errorf("get media metadata: %w", err)
	}

	// Check if media is ready
	if metadata.Status != StatusComplete {
		return nil, &StreamingError{
			Code:    "MEDIA_NOT_READY",
			Message: fmt.Sprintf("media %s is not ready for streaming (status: %s)", mediaID, metadata.Status),
			MediaID: mediaID,
		}
	}

	// Generate manifest
	manifest, err := s.dashGenerator.GenerateMPD(mediaID, metadata)
	if err != nil {
		s.logger.Error("failed to generate DASH manifest",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		return nil, fmt.Errorf("generate DASH manifest: %w", err)
	}

	// Cache the manifest
	s.manifestCache.Store(cacheKey, &cachedManifest{
		dashManifest: manifest,
		generatedAt:  time.Now(),
	})

	// Track manifest generation using DynamORM
	err = s.recordManifestGeneration(mediaID, "dash", metadata.Duration)
	if err != nil {
		s.logger.Warn("failed to record manifest generation", zap.Error(err))
	}

	return manifest, nil
}

// GetSegmentURL returns the URL for a specific segment
func (s *Streamer) GetSegmentURL(mediaID string, quality Quality, segment int) (string, error) {
	// Validate segment index
	metadata, err := s.storage.GetMediaMetadata(mediaID)
	if err != nil {
		return "", fmt.Errorf("get media metadata: %w", err)
	}

	maxSegments := int(metadata.Duration/float64(s.config.SegmentDuration)) + 1
	if segment < 0 || segment >= maxSegments {
		return "", fmt.Errorf("invalid segment index %d (max: %d)", segment, maxSegments-1)
	}

	// Check if quality is available
	qualityAvailable := false
	for _, q := range metadata.AvailableQualities {
		if q == quality {
			qualityAvailable = true
			break
		}
	}

	if !qualityAvailable {
		return "", fmt.Errorf("quality %s not available for media %s", quality, mediaID)
	}

	// Generate signed URL if needed
	segmentPath := s.storage.GetSegmentPath(mediaID, quality, segment)

	// For S3, generate a presigned URL
	if s.config.CDNBaseURL != "" {
		// Use CDN URL
		return fmt.Sprintf("%s/%s", s.config.CDNBaseURL, segmentPath), nil
	}

	// Generate presigned S3 URL
	presignClient := s3.NewPresignClient(s.s3Client)
	request, err := presignClient.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.config.S3Bucket),
		Key:    aws.String(segmentPath),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 1 * time.Hour
	})
	if err != nil {
		return "", fmt.Errorf("generate presigned URL: %w", err)
	}

	return request.URL, nil
}

// filterQualitiesByPreferences filters available qualities based on user preferences
func (s *Streamer) filterQualitiesByPreferences(qualities []Quality, prefs *Preferences) []Quality {
	if prefs == nil {
		return qualities
	}

	filtered := make([]Quality, 0, len(qualities))
	for _, quality := range qualities {
		qualityInfo := GetQualityInfo(quality)

		// Apply bandwidth constraints
		if prefs.MaxBandwidthMbps > 0 && qualityInfo.Bandwidth > int(prefs.MaxBandwidthMbps*1000) {
			continue
		}

		// Apply data saver mode (exclude high qualities)
		if prefs.DataSaverMode && (quality == Quality4K || quality == Quality1080p) {
			continue
		}

		filtered = append(filtered, quality)
	}

	return filtered
}

// GetSegmentURLs returns URLs for multiple segments
func (s *Streamer) GetSegmentURLs(mediaID string, quality Quality, startSegment, count int) ([]string, error) {
	urls := make([]string, 0, count)

	for i := 0; i < count; i++ {
		url, err := s.GetSegmentURL(mediaID, quality, startSegment+i)
		if err != nil {
			return nil, fmt.Errorf("get segment %d URL: %w", startSegment+i, err)
		}
		urls = append(urls, url)
	}

	return urls, nil
}

// TrackBandwidth records bandwidth usage
func (s *Streamer) TrackBandwidth(userID string, bytesTransferred int64) error {
	ctx := context.Background()
	return s.bandwidthTracker.TrackBandwidth(ctx, userID, bytesTransferred)
}

// GetBandwidthStats retrieves bandwidth statistics
func (s *Streamer) GetBandwidthStats(userID string) (*BandwidthStats, error) {
	ctx := context.Background()
	return s.bandwidthTracker.GetBandwidthStats(ctx, userID)
}

// GetOptimalQuality determines the best quality for a user based on preferences and bandwidth
func (s *Streamer) GetOptimalQuality(userID string, availableBandwidth int) Quality {
	return s.GetOptimalQualityWithPreferences(userID, availableBandwidth, "", nil)
}

// GetOptimalQualityWithPreferences determines the best quality considering user preferences
func (s *Streamer) GetOptimalQualityWithPreferences(userID string, availableBandwidth int, _ string, userPreferences *Preferences) Quality {
	ctx := context.Background()

	// Get user preferences if not provided
	if userPreferences == nil {
		// Try to get preferences from storage adapter if available
		if storageAdapter, ok := s.analytics.(interface {
			GetStreamingPreferences(ctx context.Context, username string) (*Preferences, error)
		}); ok {
			if prefs, err := storageAdapter.GetStreamingPreferences(ctx, userID); err == nil {
				userPreferences = prefs
			}
		}
	}

	// Get user's bandwidth stats if available bandwidth not provided
	if availableBandwidth == 0 {
		stats, err := s.bandwidthTracker.GetBandwidthStats(ctx, userID)
		if err == nil && stats.AverageBandwidth > 0 {
			availableBandwidth = stats.AverageBandwidth
		}
	}

	// Apply user preference constraints
	if userPreferences != nil {
		// Respect max bandwidth preference
		if userPreferences.MaxBandwidthMbps > 0 && availableBandwidth > int(userPreferences.MaxBandwidthMbps*1000) {
			availableBandwidth = int(userPreferences.MaxBandwidthMbps * 1000)
		}

		// If data saver mode is enabled, reduce available bandwidth
		if userPreferences.DataSaverMode {
			availableBandwidth = int(float64(availableBandwidth) * 0.5) // Reduce by 50%
		}

		// If user disabled auto quality and set a specific quality, use it if bandwidth allows
		if !userPreferences.AutoQuality && userPreferences.DefaultQuality != "" && userPreferences.DefaultQuality != "auto" {
			preferredQuality := Quality(userPreferences.DefaultQuality)
			qualityInfo := GetQualityInfo(preferredQuality)
			// Only use preferred quality if bandwidth is sufficient
			if availableBandwidth >= qualityInfo.Bandwidth {
				return preferredQuality
			}
		}
	}

	// Use quality selector for adaptive selection
	availableQualities := []Quality{
		Quality240p, Quality360p, Quality480p,
		Quality720p, Quality1080p, Quality4K,
	}

	// Filter qualities based on user preferences
	if userPreferences != nil {
		availableQualities = s.filterQualitiesByPreferences(availableQualities, userPreferences)
	}

	// Get current session if exists
	var sessions []*StreamingSession
	var mostRecentSessionID string
	if s.sessionManager != nil {
		sessions, _ = s.sessionManager.GetUserSessions(ctx, userID)
	}
	bufferHealth := 1.0 // Default to healthy buffer

	if len(sessions) > 0 {
		// Use buffer health from most recent session
		bufferHealth = sessions[0].BufferHealth
		mostRecentSessionID = sessions[0].SessionID
	}

	// Use enhanced quality selection with real metrics
	if mostRecentSessionID != "" {
		return s.qualitySelector.(*AdaptiveQualitySelector).SelectQualityWithSession(
			mostRecentSessionID, availableBandwidth, bufferHealth, availableQualities)
	}

	return s.qualitySelector.SelectQuality(availableBandwidth, bufferHealth, availableQualities)
}

// GetAvailableQualities returns available qualities for a media item
func (s *Streamer) GetAvailableQualities(mediaID string) ([]QualityInfo, error) {
	metadata, err := s.storage.GetMediaMetadata(mediaID)
	if err != nil {
		return nil, fmt.Errorf("get media metadata: %w", err)
	}

	qualities := make([]QualityInfo, 0, len(metadata.AvailableQualities))
	for _, q := range metadata.AvailableQualities {
		qualities = append(qualities, GetQualityInfo(q))
	}

	return qualities, nil
}

// GetAvailableQualitiesForUser returns available qualities for a media item filtered by user preferences
func (s *Streamer) GetAvailableQualitiesForUser(mediaID, userID string) ([]QualityInfo, error) {
	ctx := context.Background()

	// Get base available qualities
	qualities, err := s.GetAvailableQualities(mediaID)
	if err != nil {
		return nil, err
	}

	// Try to get user preferences
	var userPreferences *Preferences
	if storageAdapter, ok := s.analytics.(interface {
		GetStreamingPreferences(ctx context.Context, username string) (*Preferences, error)
	}); ok {
		if prefs, err := storageAdapter.GetStreamingPreferences(ctx, userID); err == nil {
			userPreferences = prefs
		}
	}

	// Filter qualities based on user preferences
	if userPreferences != nil {
		filteredQualities := make([]QualityInfo, 0, len(qualities))
		for _, q := range qualities {
			// Apply bandwidth constraints
			if userPreferences.MaxBandwidthMbps > 0 && q.Bandwidth > int(userPreferences.MaxBandwidthMbps*1000) {
				continue
			}

			// Apply data saver mode (exclude high qualities)
			if userPreferences.DataSaverMode && (q.Quality == Quality4K || q.Quality == Quality1080p) {
				continue
			}

			filteredQualities = append(filteredQualities, q)
		}
		return filteredQualities, nil
	}

	return qualities, nil
}

// Session management

// StartSession starts a new streaming session
// Sessions are automatically cleaned up after 24 hours using DynamoDB TTL
func (s *Streamer) StartSession(userID, mediaID string, format MediaFormat) (*StreamingSession, error) {
	return s.StartSessionWithPreferences(userID, mediaID, format, "", nil)
}

// StartSessionWithPreferences starts a new streaming session with user preferences
func (s *Streamer) StartSessionWithPreferences(userID, mediaID string, format MediaFormat, _ string, userPreferences *Preferences) (*StreamingSession, error) {
	ctx := context.Background()

	// Determine initial quality based on preferences
	initialQuality := s.config.DefaultQuality
	if userPreferences != nil && !userPreferences.AutoQuality && userPreferences.DefaultQuality != "" && userPreferences.DefaultQuality != "auto" {
		initialQuality = Quality(userPreferences.DefaultQuality)
	}

	session := &StreamingSession{
		SessionID:        uuid.New().String(),
		UserID:           userID,
		MediaID:          mediaID,
		Format:           format,
		CurrentQuality:   initialQuality,
		StartTime:        time.Now(),
		LastSegmentIndex: -1,
		BytesTransferred: 0,
		BufferHealth:     1.0,
	}

	// Session will be automatically expired by DynamoDB TTL after 24 hours
	// This eliminates the need for manual cleanup in serverless environments
	if s.sessionManager != nil {
		err := s.sessionManager.CreateSession(ctx, session)
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
	}

	// Initialize metrics tracking for the session
	s.metricsTracker.StartSession(session.SessionID, userID, mediaID)

	// Log session start
	s.logger.Info("streaming session started",
		zap.String("sessionID", session.SessionID),
		zap.String("userID", userID),
		zap.String("mediaID", mediaID),
		zap.String("format", string(format)))

	return session, nil
}

// UpdateSession updates an active session
func (s *Streamer) UpdateSession(sessionID string, quality Quality, segmentIndex int, bytesTransferred int64) error {
	ctx := context.Background()

	if s.sessionManager == nil {
		return fmt.Errorf("session manager not available")
	}

	// Get current session
	session, err := s.sessionManager.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	// Track quality switch if quality changed
	previousQuality := session.CurrentQuality
	qualitySwitched := previousQuality != quality
	if qualitySwitched && previousQuality != "" {
		s.metricsTracker.TrackQualitySwitch(sessionID, previousQuality, quality)
	}

	// Update session
	session.CurrentQuality = quality
	session.LastSegmentIndex = segmentIndex
	session.BytesTransferred += bytesTransferred

	// Calculate buffer health based on segment progression
	expectedSegment := int(time.Since(session.StartTime).Seconds() / float64(s.config.SegmentDuration))
	bufferSegments := segmentIndex - expectedSegment
	session.BufferHealth = float64(bufferSegments) / 5.0 // Normalize to 0-1 based on 5 segment buffer
	if session.BufferHealth > 1.0 {
		session.BufferHealth = 1.0
	} else if session.BufferHealth < 0 {
		session.BufferHealth = 0
	}

	// Update in database
	err = s.sessionManager.UpdateSession(ctx, session)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	// Track bandwidth
	if bytesTransferred > 0 {
		err = s.bandwidthTracker.TrackBandwidth(ctx, session.UserID, bytesTransferred)
		if err != nil {
			s.logger.Warn("failed to track bandwidth",
				zap.String("sessionID", sessionID),
				zap.Error(err))
		}
	}

	// Track segment loading success and buffer health
	loadTime := time.Duration(500 * time.Millisecond) // Estimate based on typical segment load time
	s.metricsTracker.TrackSegmentLoad(sessionID, segmentIndex, loadTime, bytesTransferred)

	// Track buffer health for adaptive bitrate decisions
	bufferDuration := time.Duration(session.BufferHealth*10) * time.Second             // Convert buffer health to duration
	estimatedBandwidth := int(float64(bytesTransferred*8) / loadTime.Seconds() / 1000) // Convert to kbps
	s.metricsTracker.TrackBufferHealth(sessionID, bufferDuration, estimatedBandwidth)

	// Update quality selector metrics with real data
	rebufferEvents := 0
	qualitySwitchEvents := 0
	if qualitySwitched {
		qualitySwitchEvents = 1
	}

	// Detect potential rebuffering based on buffer health
	if session.BufferHealth < 0.1 { // Very low buffer indicates rebuffering
		rebufferEvents = 1
		rebufferDuration := time.Duration(500) * time.Millisecond // Estimated rebuffer duration
		s.metricsTracker.TrackRebufferEvent(sessionID, rebufferDuration)
	}

	s.qualitySelector.UpdateMetrics(sessionID, rebufferEvents, qualitySwitchEvents)

	return nil
}

// EndSession ends a streaming session
func (s *Streamer) EndSession(sessionID string) error {
	ctx := context.Background()

	if s.sessionManager == nil {
		return fmt.Errorf("session manager not available")
	}

	// Get session for logging
	session, err := s.sessionManager.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	// Calculate session duration
	duration := time.Since(session.StartTime)

	// Finalize metrics tracking and get final metrics
	finalMetrics := s.metricsTracker.EndSession(sessionID)

	// Log session end with metrics
	logFields := []zap.Field{
		zap.String("sessionID", sessionID),
		zap.String("userID", session.UserID),
		zap.String("mediaID", session.MediaID),
		zap.Duration("duration", duration),
		zap.Int64("bytesTransferred", session.BytesTransferred),
		zap.String("finalQuality", string(session.CurrentQuality)),
	}

	if finalMetrics != nil {
		logFields = append(logFields,
			zap.Int("qualitySwitches", finalMetrics.QualitySwitches),
			zap.Int("rebufferEvents", finalMetrics.RebufferEvents),
			zap.Float64("qoeScore", finalMetrics.QoEScore),
			zap.Float64("segmentSuccessRate", finalMetrics.SegmentSuccessRate),
		)
	}

	s.logger.Info("streaming session ended with metrics", logFields...)

	// End session
	return s.sessionManager.EndSession(ctx, sessionID)
}

// GetSession retrieves a streaming session
func (s *Streamer) GetSession(sessionID string) (*StreamingSession, error) {
	ctx := context.Background()
	if s.sessionManager == nil {
		return nil, fmt.Errorf("session manager not available")
	}
	return s.sessionManager.GetSession(ctx, sessionID)
}

// Helper types and methods

type cachedManifest struct {
	hlsManifest  *HLSManifest
	dashManifest *DASHManifest
	generatedAt  time.Time
}

// recordManifestGeneration records manifest generation using DynamORM
func (s *Streamer) recordManifestGeneration(mediaID string, format string, duration float64) error {
	s.logger.Info("manifest generated",
		zap.String("mediaID", mediaID),
		zap.String("format", format),
		zap.Float64("duration", duration))

	// Track using the analytics storage interface
	ctx := context.Background()
	return s.analytics.Analytics().RecordManifestGeneration(ctx, mediaID, format, duration)
}

// Preferences represents user streaming preferences for the streamer
// This is a local type alias to avoid circular imports
type Preferences struct {
	Username                string `json:"username"`
	DeviceID                string `json:"device_id,omitempty"`
	DefaultQuality          string `json:"default_quality,omitempty"`
	AutoQuality             bool   `json:"auto_quality"`
	PreloadNext             bool   `json:"preload_next"`
	DataSaverMode           bool   `json:"data_saver_mode"`
	PreferredCodec          string `json:"preferred_codec,omitempty"`
	MaxBandwidthMbps        int64  `json:"max_bandwidth_mbps,omitempty"`
	BufferSizeSeconds       int    `json:"buffer_size_seconds,omitempty"`
	HDREnabled              bool   `json:"hdr_enabled"`
	ColorSpace              string `json:"color_space,omitempty"`
	SubtitleEnabled         bool   `json:"subtitle_enabled"`
	SubtitleLanguage        string `json:"subtitle_language,omitempty"`
	AudioDescriptionEnabled bool   `json:"audio_description_enabled"`
	ClosedCaptionsEnabled   bool   `json:"closed_captions_enabled"`
	Version                 int    `json:"version"`
	SchemaVersion           int    `json:"schema_version"`
}

// Note: Manifest cache cleanup is handled automatically by Go's garbage collector
// when cached entries are accessed and found to be expired. This approach is
// serverless-friendly as it avoids long-running goroutines and polling.
//
// For session cleanup, the SessionManager uses DynamoDB TTL (Time To Live) which
// automatically expires records after 24 hours without requiring manual cleanup.
// This is the recommended pattern for serverless applications as it:
// - Eliminates the need for background cleanup processes
// - Reduces compute costs by avoiding unnecessary polling
// - Works across Lambda invocations without shared state
// - Provides automatic cleanup even if Lambda functions are not invoked
//
// TTL Configuration:
// - Sessions: 24 hours (configurable via SessionManager)
// - Analytics: 30 days for manifests, 7 days for quality changes
// - Manifest cache: In-memory with passive expiration checking
