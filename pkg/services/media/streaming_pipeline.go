// Package media provides streaming pipeline integration
package media

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/services/media/transcoding"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// SubmitTranscodeJobCommand contains parameters for submitting a transcode job
type SubmitTranscodeJobCommand struct {
	MediaID        string
	UserID         string
	Username       string
	SourceBucket   string
	SourceKey      string
	ContentType    string
	Duration       int
	Width          int
	Height         int
	QualityLevels  []string // ["480p", "720p", "1080p"]
	GenerateHLS    bool
	GenerateDASH   bool
	ThumbnailCount int
}

// TranscodeJobResult contains the result of submitting a transcode job
type TranscodeJobResult struct {
	JobID             string
	MediaConvertJobID string
	EstimatedCostUSD  float64
	EstimatedDuration time.Duration
	QualityLevels     []string
	Status            string
}

// Renditions contains available renditions for a media item
type Renditions struct {
	MediaID           string
	HLSMasterURL      string
	DASHManifestURL   string
	Variants          []RenditionVariant
	ThumbnailURLs     []string
	TranscodingStatus string
	LastUpdated       time.Time
}

// RenditionVariant represents a transcoded variant
type RenditionVariant struct {
	Quality        string
	Width          int
	Height         int
	Bitrate        int
	Codec          string
	HLSPlaylistURL string
	DASHSegmentURL string
	FileSize       int64
	Format         string
}

// StreamSession contains streaming session information
type StreamSession struct {
	SessionID    string
	MediaID      string
	URL          string
	Quality      string
	Format       string
	ExpiresAt    time.Time
	Bitrate      int
	BufferHealth float64
}

var (
	// ErrTranscodingServiceUnavailable is returned when transcoding service is not available
	ErrTranscodingServiceUnavailable = errors.New("transcoding service unavailable")
	// ErrManifestServiceUnavailable is returned when manifest service is not available
	ErrManifestServiceUnavailable = errors.New("manifest service unavailable")
	// ErrCloudFrontServiceUnavailable is returned when CloudFront service is not available
	ErrCloudFrontServiceUnavailable = errors.New("cloudfront service unavailable")
	// ErrTranscodingJobNotFound is returned when a transcoding job is not found
	ErrTranscodingJobNotFound = errors.New("transcoding job not found")
)

// SubmitTranscodeJob submits a media item for transcoding
func (s *Service) SubmitTranscodeJob(ctx context.Context, cmd *SubmitTranscodeJobCommand) (*TranscodeJobResult, error) {
	s.logger.Info("submitting transcode job",
		zap.String("media_id", cmd.MediaID),
		zap.String("user_id", cmd.UserID))

	// Validate that transcoding service is available
	if s.transcoder == nil {
		return nil, ErrTranscodingServiceUnavailable
	}

	// Build transcode request
	req := &transcoding.TranscodeRequest{
		MediaID:        cmd.MediaID,
		UserID:         cmd.UserID,
		Username:       cmd.Username,
		SourceBucket:   cmd.SourceBucket,
		SourceKey:      cmd.SourceKey,
		ContentType:    cmd.ContentType,
		Duration:       cmd.Duration,
		Width:          cmd.Width,
		Height:         cmd.Height,
		QualityLevels:  cmd.QualityLevels,
		GenerateHLS:    cmd.GenerateHLS,
		GenerateDASH:   cmd.GenerateDASH,
		ThumbnailCount: cmd.ThumbnailCount,
	}

	// Submit job to MediaConvert
	result, err := s.transcoder.SubmitJob(ctx, req)
	if err != nil {
		s.logger.Error("failed to submit transcode job",
			zap.String("media_id", cmd.MediaID),
			zap.Error(err))
		return nil, err
	}

	// Create transcoding job record
	job := s.transcoder.ConvertToTranscodingJob(req, result)
	if err := s.mediaRepo.CreateTranscodingJob(ctx, job); err != nil {
		s.logger.Warn("failed to create transcoding job record",
			zap.String("job_id", job.JobID),
			zap.Error(err))
		// Don't fail the request if we can't save the record
	}

	// Mark media as processing
	if err := s.mediaRepo.MarkMediaProcessing(ctx, cmd.MediaID); err != nil {
		s.logger.Warn("failed to mark media as processing",
			zap.String("media_id", cmd.MediaID),
			zap.Error(err))
	}

	return &TranscodeJobResult{
		JobID:             result.JobID,
		MediaConvertJobID: result.MediaConvertJobID,
		EstimatedCostUSD:  result.EstimatedCostUSD,
		EstimatedDuration: result.EstimatedDuration,
		QualityLevels:     result.QualityLevels,
		Status:            result.Status,
	}, nil
}

// GetMediaRenditions retrieves available renditions for a media item
func (s *Service) GetMediaRenditions(ctx context.Context, mediaID string) (*Renditions, error) {
	s.logger.Debug("getting media renditions", zap.String("media_id", mediaID))

	// Get media record
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}

	renditions := &Renditions{
		MediaID:           mediaID,
		TranscodingStatus: media.Status,
		LastUpdated:       media.UpdatedAt,
		Variants:          []RenditionVariant{},
	}

	// If media is not ready, return with status only
	if !media.IsReady() {
		return renditions, nil
	}

	// Get manifest info if manifest service is available
	if s.manifestService != nil {
		outputPrefix := mediaID
		manifestInfo, err := s.manifestService.GetManifestInfo(ctx, mediaID, outputPrefix)
		if err != nil {
			s.logger.Warn("failed to get manifest info",
				zap.String("media_id", mediaID),
				zap.Error(err))
			// Return renditions with media variants instead
			return s.buildRenditionsFromMediaVariants(media)
		}

		// Populate from manifest info
		renditions.HLSMasterURL = manifestInfo.HLSMasterURL
		renditions.DASHManifestURL = manifestInfo.DASHManifestURL
		renditions.ThumbnailURLs = manifestInfo.ThumbnailURLs

		// Convert manifest variants to rendition variants
		for _, variant := range manifestInfo.Variants {
			renditions.Variants = append(renditions.Variants, RenditionVariant{
				Quality:        variant.Quality,
				Width:          variant.Width,
				Height:         variant.Height,
				Bitrate:        variant.Bitrate,
				Codec:          variant.Codec,
				HLSPlaylistURL: variant.HLSPlaylistURL,
				DASHSegmentURL: variant.DASHSegmentURL,
			})
		}
	} else {
		// Fallback to media variants
		return s.buildRenditionsFromMediaVariants(media)
	}

	return renditions, nil
}

// GenerateSignedStreamURL generates a signed streaming URL for a media item owned by viewerID.
func (s *Service) GenerateSignedStreamURL(ctx context.Context, mediaID string, viewerID string, quality *string) (*StreamSession, error) {
	s.logger.Debug("generating signed stream URL",
		zap.String("media_id", mediaID),
		zap.String("viewer_id", viewerID),
		zap.Any("quality", quality))

	// Validate CloudFront service is available
	if s.cloudfrontService == nil {
		return nil, ErrCloudFrontServiceUnavailable
	}

	// Get media record
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}

	if !s.isOwnedByViewer(media, viewerID) {
		s.logger.Warn("denying signed stream URL ownership mismatch",
			zap.String("media_id", mediaID),
			zap.String("media_owner", media.UserID),
			zap.String("viewer_id", viewerID))
		return nil, ErrMediaUnauthorizedAccess
	}

	// Verify media is ready
	if !media.IsReady() {
		return nil, ErrMediaNotReadyForStreaming
	}

	// Determine format (prefer HLS)
	format := "hls"
	ttl := 24 * time.Hour

	// Generate signed URL
	signedURL, err := s.cloudfrontService.SignStreamingURL(mediaID, format, quality, ttl)
	if err != nil {
		s.logger.Error("failed to generate signed URL",
			zap.String("media_id", mediaID),
			zap.Error(err))
		return nil, err
	}

	// Get bitrate for quality
	bitrate := 3000000 // Default 3 Mbps
	if quality != nil {
		_, _, bitrate = s.getQualityParams(*quality)
	}

	session := &StreamSession{
		SessionID:    fmt.Sprintf("session_%s_%d", mediaID, time.Now().Unix()),
		MediaID:      mediaID,
		URL:          signedURL,
		Format:       format,
		ExpiresAt:    time.Now().Add(ttl),
		Bitrate:      bitrate,
		BufferHealth: 1.0,
	}

	if quality != nil {
		session.Quality = *quality
	} else {
		session.Quality = "auto"
	}

	return session, nil
}

// PreloadMedia preloads manifests and primes CDN cache for media items owned by viewerID.
func (s *Service) PreloadMedia(ctx context.Context, viewerID string, mediaIDs []string) ([]string, error) {
	s.logger.Info("preloading media", zap.Int("count", len(mediaIDs)))

	successfulIDs := []string{}
	var preloadErrors []error

	// Verify each media item is ready
	for _, mediaID := range mediaIDs {
		media, err := s.mediaRepo.GetMedia(ctx, mediaID)
		if err != nil {
			s.logger.Warn("failed to preload media",
				zap.String("media_id", mediaID),
				zap.Error(err))
			preloadErrors = append(preloadErrors, err)
			continue
		}

		if !s.isOwnedByViewer(media, viewerID) {
			s.logger.Warn("skipping media preload ownership mismatch",
				zap.String("media_id", mediaID),
				zap.String("media_owner", media.UserID),
				zap.String("viewer_id", viewerID))
			preloadErrors = append(preloadErrors, ErrMediaUnauthorizedAccess)
			continue
		}

		if media.IsReady() {
			successfulIDs = append(successfulIDs, mediaID)
		} else {
			s.logger.Warn("media not ready for preload",
				zap.String("media_id", mediaID),
				zap.String("status", media.Status))
		}
	}

	// If manifest service is available, preload only media that passed ownership
	// and readiness checks.
	if len(successfulIDs) > 0 && s.manifestService != nil {
		if err := s.manifestService.PreloadManifests(ctx, successfulIDs); err != nil {
			s.logger.Warn("failed to preload manifests", zap.Error(err))
			preloadErrors = append(preloadErrors, err)
		}
	}

	// If all preloads failed, return error
	if len(successfulIDs) == 0 && len(preloadErrors) > 0 {
		return nil, fmt.Errorf("failed to preload any media: %v", preloadErrors)
	}

	s.logger.Info("media preloaded",
		zap.Int("successful", len(successfulIDs)),
		zap.Int("failed", len(preloadErrors)))

	return successfulIDs, nil
}

// UpdateMediaFromTranscodingJob updates media record with transcoding job results
func (s *Service) UpdateMediaFromTranscodingJob(ctx context.Context, jobID string) error {
	s.logger.Info("updating media from transcoding job", zap.String("job_id", jobID))

	// Get transcoding job
	job, err := s.mediaRepo.GetTranscodingJob(ctx, jobID)
	if err != nil {
		return errors.Join(ErrTranscodingJobNotFound, err)
	}

	// Get media record
	media, err := s.mediaRepo.GetMedia(ctx, job.MediaID)
	if err != nil {
		return errors.Join(ErrMediaRetrievalFailed, err)
	}

	// Update media based on job status
	switch job.Status {
	case "completed":
		// Mark as ready
		media.SetProcessed()

		// Add variants from transcoding job
		for quality, format := range job.OutputVariants {
			size := job.OutputSizes[quality]
			variant := models.MediaVariant{
				S3Key:       fmt.Sprintf("%s/hls/%s.m3u8", job.MediaID, quality),
				Width:       0, // Will be set by quality params
				Height:      0,
				FileSize:    size,
				ContentType: format,
				Quality:     quality,
			}

			// Set dimensions based on quality
			width, height, _ := s.getQualityParams(quality)
			variant.Width = width
			variant.Height = height

			media.AddVariant(quality, variant)
		}
	case "failed":
		// Mark as failed
		media.SetFailed(job.ErrorMessage)
	}

	// Update media record
	if err := s.mediaRepo.UpdateMedia(ctx, media); err != nil {
		return errors.Join(ErrMediaUpdateFailed, err)
	}

	s.logger.Info("media updated from transcoding job",
		zap.String("media_id", media.MediaID),
		zap.String("status", media.Status))

	return nil
}

// buildRenditionsFromMediaVariants builds renditions from media variants (fallback)
func (s *Service) buildRenditionsFromMediaVariants(media *models.Media) (*Renditions, error) {
	renditions := &Renditions{
		MediaID:           media.MediaID,
		TranscodingStatus: media.Status,
		LastUpdated:       media.UpdatedAt,
		Variants:          []RenditionVariant{},
	}

	// Build variants from media variants
	for name, variant := range media.Variants {
		width, height, bitrate := s.getQualityParams(name)

		renditionVariant := RenditionVariant{
			Quality:  name,
			Width:    width,
			Height:   height,
			Bitrate:  bitrate,
			Codec:    "h264",
			FileSize: variant.FileSize,
			Format:   variant.ContentType,
		}

		// Build URLs
		if variant.CDNUrl != "" {
			renditionVariant.HLSPlaylistURL = variant.CDNUrl
		} else {
			renditionVariant.HLSPlaylistURL = fmt.Sprintf("https://%s.s3.amazonaws.com/%s",
				media.S3Bucket, variant.S3Key)
		}

		renditions.Variants = append(renditions.Variants, renditionVariant)
	}

	// Set primary URLs
	if media.CDNUrl != "" {
		renditions.HLSMasterURL = media.CDNUrl
	}

	return renditions, nil
}

// getQualityParams returns width, height, and bitrate for a quality level
func (s *Service) getQualityParams(quality string) (width, height, bitrate int) {
	switch quality {
	case "2160p", "4k":
		return 3840, 2160, 15000000
	case "1080p":
		return 1920, 1080, 5000000
	case "720p":
		return 1280, 720, 3000000
	case "480p":
		return 854, 480, 1500000
	case "360p":
		return 640, 360, 800000
	case "240p":
		return 426, 240, 400000
	default:
		return 1280, 720, 3000000
	}
}
