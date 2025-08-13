// Package media provides the core Media Service for the Lesser project's API alignment.
// This service handles all media operations including file uploads, processing,
// metadata updates, and storage management. It emits appropriate events for real-time
// streaming and queues async processing for thumbnails and optimization.
package media

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides media operations
type Service struct {
	mediaRepo   interfaces.MediaRepository
	publisher   streaming.Publisher
	logger      *zap.Logger
	s3Bucket    string
	cdnDomain   string
	maxFileSize int64 // Maximum file size in bytes
}

// S3Service defines the interface for S3 operations (for abstraction/mocking)
type S3Service interface {
	UploadFile(ctx context.Context, bucket, key string, data []byte, contentType string) (string, error)
	DeleteFile(ctx context.Context, bucket, key string) error
	GeneratePresignedURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// ProcessingQueue defines the interface for async media processing
type ProcessingQueue interface {
	QueueMediaProcessing(ctx context.Context, mediaID string, processingType string) error
}

// NewService creates a new Media Service with the required dependencies
func NewService(
	mediaRepo interfaces.MediaRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
	s3Bucket string,
	cdnDomain string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		mediaRepo:   mediaRepo,
		publisher:   publisher,
		logger:      logger,
		s3Bucket:    s3Bucket,
		cdnDomain:   cdnDomain,
		maxFileSize: 50 * 1024 * 1024, // 50MB default
	}
}

// SetMaxFileSize sets the maximum allowed file size
func (s *Service) SetMaxFileSize(maxSize int64) {
	s.maxFileSize = maxSize
}

// Command structs for operations

// UploadMediaCommand contains all data needed to upload a media file
type UploadMediaCommand struct {
	UserID      string `json:"user_id" validate:"required"`
	FileName    string `json:"file_name" validate:"required"`
	ContentType string `json:"content_type" validate:"required"`
	FileData    []byte `json:"file_data" validate:"required"`
	Description string `json:"description" validate:"max=1500"` // Alt text
	Focus       string `json:"focus"`                           // Focus point for cropping (x,y)
}

// UpdateMediaCommand contains all data needed to update media metadata
type UpdateMediaCommand struct {
	MediaID     string `json:"media_id" validate:"required"`
	UserID      string `json:"user_id" validate:"required"`     // Must be the media owner
	Description string `json:"description" validate:"max=1500"` // Alt text
	Focus       string `json:"focus"`                           // Focus point for cropping (x,y)
}

// GetMediaQuery contains parameters for retrieving media
type GetMediaQuery struct {
	MediaID  string `json:"media_id" validate:"required"`
	ViewerID string `json:"viewer_id"` // User requesting the media (for privacy checks)
}

// Result structs for operations

// Result contains media and associated events that were emitted
type Result struct {
	Media  *models.Media      `json:"media"`
	Events []*streaming.Event `json:"events"`
}

// UpdateResult contains updated media and events
type UpdateResult struct {
	Media  *models.Media      `json:"media"`
	Events []*streaming.Event `json:"events"`
}

// UploadMedia uploads a new media file, validates it, stores it in S3, creates the record,
// and queues async processing for thumbnails and analysis
func (s *Service) UploadMedia(ctx context.Context, cmd *UploadMediaCommand) (*Result, error) {
	s.logger.Info("uploading media",
		zap.String("user_id", cmd.UserID),
		zap.String("file_name", cmd.FileName),
		zap.String("content_type", cmd.ContentType),
		zap.Int("file_size", len(cmd.FileData)))

	// Validate the command
	if err := s.validateUploadCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Create media record
	media := &models.Media{
		MediaID:     uuid.New().String(),
		UserID:      cmd.UserID,
		FileName:    cmd.FileName,
		ContentType: cmd.ContentType,
		FileSize:    int64(len(cmd.FileData)),
		Description: cmd.Description,
		Focus:       cmd.Focus,
		Status:      models.StatusPending,
	}

	// Generate S3 key and simulate upload
	s3Key := s.generateS3Key(media.MediaID, cmd.FileName)
	media.S3Bucket = s.s3Bucket
	media.S3Key = s3Key

	// Generate CDN URL
	if s.cdnDomain != "" {
		media.CDNUrl = fmt.Sprintf("https://%s/%s", s.cdnDomain, s3Key)
	}

	// Analyze media dimensions for images (simulated)
	if media.IsImage() {
		// In real implementation, this would analyze the image
		// For now, we'll set placeholder values
		media.Width = 1920
		media.Height = 1080
		media.Blurhash = "L6PZfSi_.AyE_3t7t7R**0o#DgR4" // Example blurhash
	}

	// Store the media record
	if err := s.mediaRepo.CreateMedia(ctx, media); err != nil {
		return nil, fmt.Errorf("failed to store media record: %w", err)
	}

	s.logger.Info("uploaded media successfully",
		zap.String("media_id", media.MediaID),
		zap.String("user_id", cmd.UserID))

	// Emit events
	events := s.emitMediaUploadedEvents(ctx, media)

	// Queue async processing (thumbnails, analysis, etc.)
	s.queueMediaProcessing(ctx, media)

	return &Result{
		Media:  media,
		Events: events,
	}, nil
}

// UpdateMedia updates media metadata (alt text, focus points) and emits events
func (s *Service) UpdateMedia(ctx context.Context, cmd *UpdateMediaCommand) (*UpdateResult, error) {
	s.logger.Info("updating media",
		zap.String("media_id", cmd.MediaID),
		zap.String("user_id", cmd.UserID))

	// Validate the command
	if err := s.validateUpdateCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Get existing media
	media, err := s.mediaRepo.GetMedia(ctx, cmd.MediaID)
	if err != nil {
		return nil, fmt.Errorf("failed to get media: %w", err)
	}

	// Verify ownership
	if media.UserID != cmd.UserID {
		return nil, fmt.Errorf("unauthorized: only the media owner can update metadata")
	}

	// Update metadata
	media.Description = cmd.Description
	media.Focus = cmd.Focus

	// Store the updated media
	if err := s.mediaRepo.UpdateMedia(ctx, media); err != nil {
		return nil, fmt.Errorf("failed to update media: %w", err)
	}

	s.logger.Info("updated media successfully",
		zap.String("media_id", cmd.MediaID))

	// Emit events
	events := s.emitMediaUpdatedEvents(ctx, media)

	return &UpdateResult{
		Media:  media,
		Events: events,
	}, nil
}

// GetMedia retrieves media with privacy checks
func (s *Service) GetMedia(ctx context.Context, query *GetMediaQuery) (*models.Media, error) {
	s.logger.Debug("getting media",
		zap.String("media_id", query.MediaID),
		zap.String("viewer_id", query.ViewerID))

	// Get the media
	media, err := s.mediaRepo.GetMedia(ctx, query.MediaID)
	if err != nil {
		return nil, fmt.Errorf("failed to get media: %w", err)
	}

	// Apply privacy checks
	if err := s.checkMediaAccess(media, query.ViewerID); err != nil {
		return nil, err
	}

	// Mark media as used if not already marked
	if media.UsageCount == 0 {
		media.MarkUsed()
		if err := s.mediaRepo.UpdateMedia(ctx, media); err != nil {
			s.logger.Warn("failed to mark media as used",
				zap.String("media_id", query.MediaID),
				zap.Error(err))
		}
	}

	return media, nil
}

// Private helper methods

func (s *Service) validateUploadCommand(_ context.Context, cmd *UploadMediaCommand) error {
	if strings.TrimSpace(cmd.UserID) == "" {
		return fmt.Errorf("user_id is required")
	}

	if strings.TrimSpace(cmd.FileName) == "" {
		return fmt.Errorf("file_name is required")
	}

	if strings.TrimSpace(cmd.ContentType) == "" {
		return fmt.Errorf("content_type is required")
	}

	if len(cmd.FileData) == 0 {
		return fmt.Errorf("file_data is required")
	}

	// Validate file size
	if int64(len(cmd.FileData)) > s.maxFileSize {
		return fmt.Errorf("file size %d exceeds maximum %d bytes", len(cmd.FileData), s.maxFileSize)
	}

	// Validate content type
	if !s.isValidMediaType(cmd.ContentType) {
		return fmt.Errorf("unsupported content type: %s", cmd.ContentType)
	}

	// Validate file extension matches content type
	if !s.validateFileExtension(cmd.FileName, cmd.ContentType) {
		return fmt.Errorf("file extension does not match content type")
	}

	// Validate description length
	if len(cmd.Description) > 1500 {
		return fmt.Errorf("description too long (max 1500 characters)")
	}

	// Validate focus point format if provided
	if cmd.Focus != "" && !s.isValidFocusPoint(cmd.Focus) {
		return fmt.Errorf("invalid focus point format (expected 'x,y' where x,y are between -1.0 and 1.0)")
	}

	return nil
}

func (s *Service) validateUpdateCommand(_ context.Context, cmd *UpdateMediaCommand) error {
	if strings.TrimSpace(cmd.MediaID) == "" {
		return fmt.Errorf("media_id is required")
	}

	if strings.TrimSpace(cmd.UserID) == "" {
		return fmt.Errorf("user_id is required")
	}

	// Validate description length
	if len(cmd.Description) > 1500 {
		return fmt.Errorf("description too long (max 1500 characters)")
	}

	// Validate focus point format if provided
	if cmd.Focus != "" && !s.isValidFocusPoint(cmd.Focus) {
		return fmt.Errorf("invalid focus point format (expected 'x,y' where x,y are between -1.0 and 1.0)")
	}

	return nil
}

func (s *Service) isValidMediaType(contentType string) bool {
	validTypes := map[string]bool{
		// Images
		"image/jpeg":    true,
		"image/jpg":     true,
		"image/png":     true,
		"image/gif":     true,
		"image/webp":    true,
		"image/svg+xml": true,
		"image/bmp":     true,
		"image/tiff":    true,

		// Videos
		"video/mp4":       true,
		"video/webm":      true,
		"video/ogg":       true,
		"video/avi":       true,
		"video/mov":       true,
		"video/quicktime": true,
		"video/x-msvideo": true,

		// Audio
		"audio/mpeg":  true,
		"audio/mp3":   true,
		"audio/wav":   true,
		"audio/ogg":   true,
		"audio/aac":   true,
		"audio/flac":  true,
		"audio/x-wav": true,
		"audio/webm":  true,
	}

	return validTypes[strings.ToLower(contentType)]
}

func (s *Service) validateFileExtension(fileName, contentType string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))

	// Get expected MIME type from extension
	expectedType := mime.TypeByExtension(ext)
	if expectedType == "" {
		return false // Unknown extension
	}

	// Compare base types (ignore charset and other parameters)
	expectedBase := strings.Split(expectedType, ";")[0]
	actualBase := strings.Split(contentType, ";")[0]

	return strings.EqualFold(expectedBase, actualBase)
}

func (s *Service) isValidFocusPoint(focus string) bool {
	// Focus point should be in format "x,y" where x,y are floats between -1.0 and 1.0
	parts := strings.Split(focus, ",")
	if len(parts) != 2 {
		return false
	}

	// This is a simplified validation - in production you'd parse and validate the floats
	return len(strings.TrimSpace(parts[0])) > 0 && len(strings.TrimSpace(parts[1])) > 0
}

func (s *Service) generateS3Key(mediaID, fileName string) string {
	ext := filepath.Ext(fileName)
	timestamp := time.Now().Format("2006/01/02")
	return fmt.Sprintf("media/%s/%s%s", timestamp, mediaID, ext)
}

func (s *Service) checkMediaAccess(media *models.Media, viewerID string) error {
	// Basic privacy check - owner can always access their media
	if media.UserID == viewerID {
		return nil
	}

	// Check if media is marked as NSFW and viewer restrictions
	if media.IsNSFW {
		// In a real implementation, you'd check viewer preferences/settings
		s.logger.Debug("media marked as NSFW",
			zap.String("media_id", media.MediaID))
	}

	// Media is generally accessible if it's in "ready" status
	if !media.IsReady() {
		return fmt.Errorf("media not ready for viewing")
	}

	return nil
}

func (s *Service) emitMediaUploadedEvents(ctx context.Context, media *models.Media) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "media.uploaded",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"media": media,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", media.UserID)
	if err := s.publisher.PublishToUser(ctx, media.UserID, &userEvent); err != nil {
		s.logger.Error("failed to publish to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) emitMediaUpdatedEvents(ctx context.Context, media *models.Media) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "media.updated",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"media": media,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", media.UserID)
	if err := s.publisher.PublishToUser(ctx, media.UserID, &userEvent); err != nil {
		s.logger.Error("failed to publish to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) emitMediaProcessedEvents(ctx context.Context, media *models.Media) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "media.processed",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"media": media,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", media.UserID)
	if err := s.publisher.PublishToUser(ctx, media.UserID, &userEvent); err != nil {
		s.logger.Error("failed to publish to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) emitMediaFailedEvents(ctx context.Context, media *models.Media, errorMsg string) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      "media.failed",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"media": media,
			"error": errorMsg,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", media.UserID)
	if err := s.publisher.PublishToUser(ctx, media.UserID, &userEvent); err != nil {
		s.logger.Error("failed to publish to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) queueMediaProcessing(_ context.Context, media *models.Media) {
	// In a real implementation, this would queue processing jobs
	// For now, we'll just log the intent
	s.logger.Info("queuing media processing",
		zap.String("media_id", media.MediaID),
		zap.String("content_type", media.ContentType))

	// Queue different processing based on media type
	if media.IsImage() {
		s.logger.Debug("queuing image processing (thumbnails, blurhash)",
			zap.String("media_id", media.MediaID))
	} else if media.IsVideo() {
		s.logger.Debug("queuing video processing (thumbnails, transcoding)",
			zap.String("media_id", media.MediaID))
	} else if media.IsAudio() {
		s.logger.Debug("queuing audio processing (waveform, metadata)",
			zap.String("media_id", media.MediaID))
	}
}

// Additional methods for processing callbacks (would be called by async processors)

// MarkMediaProcessed marks media as successfully processed and emits events
func (s *Service) MarkMediaProcessed(ctx context.Context, mediaID string, variants map[string]models.MediaVariant) error {
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("failed to get media: %w", err)
	}

	// Update media status
	media.SetProcessed()

	// Add variants
	for name, variant := range variants {
		media.AddVariant(name, variant)
	}

	// Store updates
	if err := s.mediaRepo.UpdateMedia(ctx, media); err != nil {
		return fmt.Errorf("failed to update media: %w", err)
	}

	// Emit processed event
	s.emitMediaProcessedEvents(ctx, media)

	s.logger.Info("marked media as processed",
		zap.String("media_id", mediaID),
		zap.Int("variants_count", len(variants)))

	return nil
}

// MarkMediaFailed marks media as failed and emits events
func (s *Service) MarkMediaFailed(ctx context.Context, mediaID string, errorMsg string) error {
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("failed to get media: %w", err)
	}

	// Update media status
	media.SetFailed(errorMsg)

	// Store updates
	if err := s.mediaRepo.UpdateMedia(ctx, media); err != nil {
		return fmt.Errorf("failed to update media: %w", err)
	}

	// Emit failed event
	s.emitMediaFailedEvents(ctx, media, errorMsg)

	s.logger.Warn("marked media as failed",
		zap.String("media_id", mediaID),
		zap.String("error", errorMsg))

	return nil
}

// GetStreamingURL returns a media streaming URL and metadata for GraphQL
func (s *Service) GetStreamingURL(ctx context.Context, mediaID string) (*model.MediaStream, error) {
	s.logger.Debug("getting media streaming URL",
		zap.String("media_id", mediaID))

	// Get the media record
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("failed to get media: %w", err)
	}

	// Verify media is ready for streaming
	if !media.IsReady() {
		return nil, fmt.Errorf("media not ready for streaming")
	}

	// Get the media URL (use CDN if available, otherwise construct S3 URL)
	mediaURL := media.CDNUrl
	if mediaURL == "" {
		// Construct S3 URL as fallback
		mediaURL = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", media.S3Bucket, media.S3Key)
	}

	// Get thumbnail URL
	thumbnailURL := mediaURL // Default to same URL
	if thumbnailVariant, exists := media.GetVariant("thumbnail"); exists {
		if thumbnailVariant.CDNUrl != "" {
			thumbnailURL = thumbnailVariant.CDNUrl
		} else {
			thumbnailURL = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", media.S3Bucket, thumbnailVariant.S3Key)
		}
	}

	// Convert models.Media to model.MediaStream
	mediaStream := &model.MediaStream{
		ID:           media.MediaID,
		URL:          mediaURL,
		ThumbnailURL: thumbnailURL,
		Duration:     media.Duration,
		ExpiresAt:    model.Time(time.Now().Add(24 * time.Hour)), // Default 24h expiry
	}

	// Add bitrates if this is a video with variants
	if media.IsVideo() && len(media.Variants) > 0 {
		var bitrates []*model.Bitrate
		for name, variant := range media.Variants {
			if variant.Width > 0 && variant.Height > 0 {
				// Map quality name to StreamQuality
				quality := model.StreamQualityMedium // Default
				switch strings.ToUpper(name) {
				case "LOW", "THUMBNAIL":
					quality = model.StreamQualityLow
				case "MEDIUM", "STANDARD":
					quality = model.StreamQualityMedium
				case "HIGH", "HD":
					quality = model.StreamQualityHigh
				case "ULTRA", "4K":
					quality = model.StreamQualityUltra
				}

				bitrate := &model.Bitrate{
					Quality:       quality,
					Width:         variant.Width,
					Height:        variant.Height,
					BitsPerSecond: int(variant.FileSize * 8 / int64(media.Duration)), // Rough estimate
					Codec:         "h264",                                            // Default codec
				}
				bitrates = append(bitrates, bitrate)
			}
		}
		mediaStream.Bitrates = bitrates
	}

	s.logger.Debug("returning media streaming URL",
		zap.String("media_id", mediaID),
		zap.String("url", mediaStream.URL))

	return mediaStream, nil
}
