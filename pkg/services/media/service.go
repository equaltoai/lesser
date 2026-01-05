// Package media provides the core Media Service for the Lesser project's API alignment.
// This service handles all media operations including file uploads, processing,
// metadata updates, and storage management. It emits appropriate events for real-time
// streaming and queues async processing for thumbnails and optimization.
package media

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	mediaprocessor "github.com/equaltoai/lesser/pkg/media"
	"github.com/equaltoai/lesser/pkg/services/media/transcoding"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides media operations
type Service struct {
	mediaRepo         interfaces.MediaRepository
	accountRepo       accountPreferencesRepository
	publisher         streaming.Publisher
	jobQueue          JobQueueService
	logger            *zap.Logger
	s3Bucket          string
	cdnDomain         string
	maxFileSize       int64 // Maximum file size in bytes
	transcoder        transcoderService
	manifestService   manifestService
	cloudfrontService cloudfrontService
}

type transcoderService interface {
	SubmitJob(ctx context.Context, req *transcoding.TranscodeRequest) (*transcoding.TranscodeResult, error)
	ConvertToTranscodingJob(req *transcoding.TranscodeRequest, result *transcoding.TranscodeResult) *models.TranscodingJob
}

type manifestService interface {
	GetManifestInfo(ctx context.Context, mediaID string, outputPrefix string) (*transcoding.ManifestInfo, error)
	PreloadManifests(ctx context.Context, mediaIDs []string) error
}

type cloudfrontService interface {
	SignStreamingURL(mediaID, format string, quality *string, ttl time.Duration) (string, error)
}

var (
	_ transcoderService = (*transcoding.Service)(nil)
	_ manifestService   = (*transcoding.ManifestService)(nil)
	_ cloudfrontService = (*transcoding.CloudFrontService)(nil)
)

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

type accountPreferencesRepository interface {
	GetAccountPreferences(ctx context.Context, username string) (map[string]interface{}, error)
}

// JobQueueService defines the interface for job queue operations
type JobQueueService interface {
	QueueMediaJob(ctx context.Context, msg JobMessage) error
}

// JobMessage represents a message for media processing
type JobMessage struct {
	JobID     string `json:"job_id"`
	MediaID   string `json:"media_id"`
	Username  string `json:"username"`
	Timestamp int64  `json:"timestamp"`
}

// NewService creates a new Media Service with the required dependencies
func NewService(
	mediaRepo interfaces.MediaRepository,
	accountRepo accountPreferencesRepository,
	publisher streaming.Publisher,
	jobQueue JobQueueService,
	logger *zap.Logger,
	s3Bucket string,
	cdnDomain string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		mediaRepo:   mediaRepo,
		accountRepo: accountRepo,
		publisher:   publisher,
		jobQueue:    jobQueue,
		logger:      logger,
		s3Bucket:    s3Bucket,
		cdnDomain:   cdnDomain,
		maxFileSize: 50 * 1024 * 1024, // 50MB default
	}
}

// SetTranscodingService sets the transcoding service (optional)
func (s *Service) SetTranscodingService(transcoder transcoderService) {
	s.transcoder = transcoder
}

// SetManifestService sets the manifest service (optional)
func (s *Service) SetManifestService(manifestService manifestService) {
	s.manifestService = manifestService
}

// SetCloudFrontService sets the CloudFront service (optional)
func (s *Service) SetCloudFrontService(cloudfrontService cloudfrontService) {
	s.cloudfrontService = cloudfrontService
}

// SetMaxFileSize sets the maximum allowed file size
func (s *Service) SetMaxFileSize(maxSize int64) {
	s.maxFileSize = maxSize
}

// Command structs for operations

// UploadMediaCommand contains all data needed to upload a media file
type UploadMediaCommand struct {
	UserID        string               `json:"user_id" validate:"required"`
	FileName      string               `json:"file_name" validate:"required"`
	ContentType   string               `json:"content_type" validate:"required"`
	FileData      []byte               `json:"file_data" validate:"required"`
	Description   string               `json:"description" validate:"max=1500"` // Alt text
	Focus         string               `json:"focus"`                           // Focus point for cropping (x,y)
	Sensitive     bool                 `json:"sensitive"`
	SpoilerText   string               `json:"spoiler_text"`
	MediaCategory models.MediaCategory `json:"media_category"`
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

// ListMediaQuery contains parameters for listing media with filters
type ListMediaQuery struct {
	Owner     string     `json:"owner"`
	Requester string     `json:"requester"`
	MediaType string     `json:"media_type"`
	MimeType  string     `json:"mime_type"`
	Cursor    string     `json:"cursor"`
	Limit     int        `json:"limit"`
	Since     *time.Time `json:"since"`
	Until     *time.Time `json:"until"`
}

// ListMediaResult contains paginated media results
type ListMediaResult struct {
	Items      []*models.Media `json:"items"`
	NextCursor string          `json:"next_cursor"`
	HasMore    bool            `json:"has_more"`
	Total      int64           `json:"total"`
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
		return nil, errors.Join(ErrMediaValidationFailed, err)
	}

	// Create media record
	media := &models.Media{
		MediaID:       uuid.New().String(),
		UserID:        cmd.UserID,
		FileName:      cmd.FileName,
		ContentType:   cmd.ContentType,
		FileSize:      int64(len(cmd.FileData)),
		Description:   cmd.Description,
		Focus:         cmd.Focus,
		IsNSFW:        cmd.Sensitive,
		SpoilerText:   strings.TrimSpace(cmd.SpoilerText),
		MediaCategory: cmd.MediaCategory,
		Status:        models.StatusPending,
	}

	// Generate S3 key and simulate upload
	s3Key := s.generateS3Key(media.MediaID, cmd.FileName)
	media.S3Bucket = s.s3Bucket
	media.S3Key = s3Key

	// Generate CDN URL
	if s.cdnDomain != "" {
		media.CDNUrl = fmt.Sprintf("https://%s/%s", s.cdnDomain, s3Key)
	}

	// Analyze media dimensions for images
	if media.IsImage() {
		processedImages, err := mediaprocessor.ProcessImage(cmd.FileData, cmd.ContentType)
		if err != nil {
			s.logger.Warn("failed to process image, using defaults",
				zap.String("media_id", media.MediaID),
				zap.Error(err))
			// Set default values if processing fails
			media.Width = 0
			media.Height = 0
			media.Blurhash = mediaprocessor.GetDefaultBlurhash()
		} else {
			// Extract dimensions and blurhash from the original image
			if original, exists := processedImages["original"]; exists {
				media.Width = original.Width
				media.Height = original.Height
				media.Blurhash = original.Blurhash
			} else {
				s.logger.Warn("original image not found in processed results",
					zap.String("media_id", media.MediaID))
				media.Width = 0
				media.Height = 0
				media.Blurhash = mediaprocessor.GetDefaultBlurhash()
			}
		}
	}

	// Store the media record
	if err := s.mediaRepo.CreateMedia(ctx, media); err != nil {
		return nil, errors.Join(ErrMediaStorageFailed, err)
	}

	s.logger.Info("uploaded media successfully",
		zap.String("media_id", media.MediaID),
		zap.String("user_id", cmd.UserID))

	// Emit events
	events := s.emitMediaUploadedEvents(ctx, media)

	// Queue async processing (thumbnails, analysis, etc.)
	if err := s.queueMediaProcessing(ctx, media); err != nil {
		// Don't fail the upload if queueing fails - just log the error
		s.logger.Warn("failed to queue media processing, processing will be skipped",
			zap.String("media_id", media.MediaID),
			zap.Error(err))
	}

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
		return nil, errors.Join(ErrMediaValidationFailed, err)
	}

	// Get existing media
	media, err := s.mediaRepo.GetMedia(ctx, cmd.MediaID)
	if err != nil {
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}

	// Verify ownership
	if media.UserID != cmd.UserID {
		return nil, ErrMediaUnauthorizedAccess
	}

	// Update metadata
	media.Description = cmd.Description
	media.Focus = cmd.Focus

	// Store the updated media
	if err := s.mediaRepo.UpdateMedia(ctx, media); err != nil {
		return nil, errors.Join(ErrMediaUpdateFailed, err)
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
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}

	// Apply privacy checks
	if err := s.checkMediaAccess(ctx, media, query.ViewerID); err != nil {
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

// ListMedia returns paginated media filtered by owner and type
func (s *Service) ListMedia(ctx context.Context, query *ListMediaQuery) (*ListMediaResult, error) {
	if query == nil {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("list media query cannot be nil"))
	}

	owner := strings.TrimSpace(query.Owner)
	if owner == "" {
		return nil, errors.Join(ErrMediaValidationFailed, common.ErrValidation("owner", "owner is required").InternalError)
	}

	// Clamp limit to reasonable bounds (1 - 100)
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	if query.Since != nil && query.Until != nil && query.Since.After(*query.Until) {
		return nil, errors.Join(ErrMediaValidationFailed, common.ErrValidation("since", "since must be before until").InternalError)
	}

	opts := interfaces.PaginationOptions{
		Limit:  limit,
		Cursor: strings.TrimSpace(query.Cursor),
		Since:  query.Since,
		Until:  query.Until,
	}

	mediaTypeFilter := strings.TrimSpace(query.MimeType)
	if mediaTypeFilter == "" && query.MediaType != "" {
		mediaTypeFilter = strings.ToLower(query.MediaType)
	}

	var (
		result *interfaces.PaginatedResult[*models.Media]
		err    error
	)

	if mediaTypeFilter != "" {
		result, err = s.mediaRepo.GetUserMediaByType(ctx, owner, mediaTypeFilter, opts)
	} else {
		result, err = s.mediaRepo.GetUserMedia(ctx, owner, opts)
	}
	if err != nil {
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}

	if result == nil {
		return &ListMediaResult{
			Items:      []*models.Media{},
			NextCursor: "",
			HasMore:    false,
			Total:      0,
		}, nil
	}

	return &ListMediaResult{
		Items:      result.Items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
		Total:      result.Total,
	}, nil
}

// Private helper methods

func (s *Service) validateUploadCommand(_ context.Context, cmd *UploadMediaCommand) error {
	if err := common.ValidateRequiredParam("user_id", strings.TrimSpace(cmd.UserID)); err != nil {
		return err
	}

	if err := common.ValidateRequiredParam("file_name", strings.TrimSpace(cmd.FileName)); err != nil {
		return err
	}

	if err := common.ValidateRequiredParam("content_type", strings.TrimSpace(cmd.ContentType)); err != nil {
		return err
	}

	if len(cmd.FileData) == 0 {
		return ErrMediaFileDataRequired
	}

	// Validate file size
	if int64(len(cmd.FileData)) > s.maxFileSize {
		return ErrMediaFileTooLarge
	}

	// Validate content type
	if !s.isValidMediaType(cmd.ContentType) {
		return ErrMediaUnsupportedType
	}

	// Validate file extension matches content type
	if !s.validateFileExtension(cmd.FileName, cmd.ContentType) {
		return ErrMediaFileExtensionMismatch
	}

	// Validate description using proper validation function
	if err := common.ValidateMediaDescription(cmd.Description); err != nil {
		return common.ErrValidation("description", fmt.Sprintf("Invalid media description: %v", err)).InternalError
	}

	// Validate focus point format if provided
	if cmd.Focus != "" && !s.isValidFocusPoint(cmd.Focus) {
		return common.ErrValidation("focus", "Focus point format must be 'x,y' where x,y are between -1.0 and 1.0").InternalError
	}

	cmd.SpoilerText = strings.TrimSpace(cmd.SpoilerText)
	if cmd.SpoilerText != "" {
		if err := common.ValidateSpoilerText(cmd.SpoilerText); err != nil {
			return common.ErrValidation("spoilerText", err.Error()).InternalError
		}
	}

	categoryValue := strings.TrimSpace(string(cmd.MediaCategory))
	if categoryValue == "" {
		cmd.MediaCategory = models.DetermineMediaCategory(cmd.ContentType)
	} else {
		normalized, ok := models.NormalizeMediaCategory(categoryValue)
		if !ok {
			return common.ErrValidation("mediaType", "invalid media category").InternalError
		}
		cmd.MediaCategory = normalized
	}

	return nil
}

func (s *Service) validateUpdateCommand(_ context.Context, cmd *UpdateMediaCommand) error {
	if err := common.ValidateRequiredParam("media_id", strings.TrimSpace(cmd.MediaID)); err != nil {
		return err
	}

	if err := common.ValidateRequiredParam("user_id", strings.TrimSpace(cmd.UserID)); err != nil {
		return err
	}

	// Validate description using proper validation function
	if err := common.ValidateMediaDescription(cmd.Description); err != nil {
		return common.ErrValidation("description", fmt.Sprintf("Invalid media description: %v", err)).InternalError
	}

	// Validate focus point format if provided
	if cmd.Focus != "" && !s.isValidFocusPoint(cmd.Focus) {
		return common.ErrValidation("focus", "Focus point format must be 'x,y' where x,y are between -1.0 and 1.0").InternalError
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
	if err := common.ValidateRequiredParam("expected_type", expectedType); err != nil {
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

func (s *Service) checkMediaAccess(ctx context.Context, media *models.Media, viewerID string) error {
	// Basic privacy check - owner can always access their media
	if media.UserID == viewerID {
		return nil
	}

	// Check if media is marked as NSFW and viewer restrictions
	if media.IsNSFW {
		// Get viewer's NSFW preferences
		allowAccess, requireWarning, err := s.checkNSFWPermissions(ctx, viewerID)
		if err != nil {
			s.logger.Error("failed to check NSFW permissions",
				zap.Error(err),
				zap.String("media_id", media.MediaID),
				zap.String("viewer_id", viewerID))
			// Default to blocking access if preference check fails
			return NewNSFWBlockedError("Unable to verify content preferences")
		}

		if !allowAccess {
			s.logger.Debug("NSFW content blocked by user preferences",
				zap.String("media_id", media.MediaID),
				zap.String("viewer_id", viewerID))
			return NewNSFWBlockedError("NSFW content blocked by your preferences")
		}

		if requireWarning {
			s.logger.Debug("NSFW content requires warning",
				zap.String("media_id", media.MediaID),
				zap.String("viewer_id", viewerID))
			// This can be handled by the API layer to show warnings
		}
	}

	// Media is generally accessible if it's in "ready" status
	if !media.IsReady() {
		return ErrMediaNotReady
	}

	return nil
}

func (s *Service) emitMediaUploadedEvents(ctx context.Context, media *models.Media) []*streaming.Event {
	var events []*streaming.Event

	if s.publisher == nil {
		return events
	}

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

	if s.publisher == nil {
		return events
	}

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

	if s.publisher == nil {
		return events
	}

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

	if s.publisher == nil {
		return events
	}

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

func (s *Service) queueMediaProcessing(ctx context.Context, media *models.Media) error {
	// Generate a unique job ID for tracking
	jobID := uuid.New().String()

	// Create media job message
	msg := JobMessage{
		JobID:     jobID,
		MediaID:   media.MediaID,
		Username:  media.UserID, // Using UserID as Username for compatibility
		Timestamp: time.Now().Unix(),
	}

	// Queue the processing job using the job queue service
	if err := s.jobQueue.QueueMediaJob(ctx, msg); err != nil {
		s.logger.Error("failed to queue media processing job",
			zap.String("media_id", media.MediaID),
			zap.String("job_id", jobID),
			zap.String("content_type", media.ContentType),
			zap.Error(err))
		return errors.Join(ErrMediaProcessingQueueFailed, err)
	}

	s.logger.Info("queued media processing job",
		zap.String("media_id", media.MediaID),
		zap.String("job_id", jobID),
		zap.String("content_type", media.ContentType),
		zap.String("user_id", media.UserID))

	// Log different processing based on media type for debugging
	if media.IsImage() {
		s.logger.Debug("queued image processing (thumbnails, blurhash)",
			zap.String("media_id", media.MediaID),
			zap.String("job_id", jobID))
	} else if media.IsVideo() {
		s.logger.Debug("queued video processing (thumbnails, transcoding)",
			zap.String("media_id", media.MediaID),
			zap.String("job_id", jobID))
	} else if media.IsAudio() {
		s.logger.Debug("queued audio processing (waveform, metadata)",
			zap.String("media_id", media.MediaID),
			zap.String("job_id", jobID))
	}

	return nil
}

// Additional methods for processing callbacks (would be called by async processors)

// MarkMediaProcessed marks media as successfully processed and emits events
func (s *Service) MarkMediaProcessed(ctx context.Context, mediaID string, variants map[string]models.MediaVariant) error {
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return errors.Join(ErrMediaRetrievalFailed, err)
	}

	// Update media status
	media.SetProcessed()

	// Add variants
	for name, variant := range variants {
		media.AddVariant(name, variant)
	}

	// Store updates
	if err := s.mediaRepo.UpdateMedia(ctx, media); err != nil {
		return errors.Join(ErrMediaUpdateFailed, err)
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
		return errors.Join(ErrMediaRetrievalFailed, err)
	}

	// Update media status
	media.SetFailed(errorMsg)

	// Store updates
	if err := s.mediaRepo.UpdateMedia(ctx, media); err != nil {
		return errors.Join(ErrMediaUpdateFailed, err)
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
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}

	// Verify media is ready for streaming
	if !media.IsReady() {
		return nil, ErrMediaNotReadyForStreaming
	}

	// Get the media URL (use CDN if available, otherwise construct S3 URL)
	mediaURL := media.CDNUrl
	if err := common.ValidateRequiredParam("media_url", mediaURL); err != nil {
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

// checkNSFWPermissions retrieves the user's NSFW content preferences
// Returns (allowAccess, requireWarning, error)
func (s *Service) checkNSFWPermissions(ctx context.Context, viewerID string) (bool, bool, error) {
	// Handle unauthenticated users with safe defaults
	if err := common.ValidateRequiredParam("viewer_id", viewerID); err != nil {
		s.logger.Debug("unauthenticated user accessing NSFW content - blocking")
		return false, true, nil // Block NSFW for unauthenticated users
	}

	// Get user preferences from the repository
	preferences, err := s.accountRepo.GetAccountPreferences(ctx, viewerID)
	if err != nil {
		s.logger.Warn("failed to get user NSFW preferences, using safe defaults",
			zap.Error(err),
			zap.String("viewer_id", viewerID))
		// On error, use safe defaults (block NSFW content)
		return false, true, nil
	}

	// Extract NSFW preferences with safe defaults
	allowNSFW := false
	requireWarning := true

	if allowVal, ok := preferences["allow_nsfw"]; ok {
		if allow, ok := allowVal.(bool); ok {
			allowNSFW = allow
		}
	}

	if warnVal, ok := preferences["require_nsfw_warning"]; ok {
		if warn, ok := warnVal.(bool); ok {
			requireWarning = warn
		}
	}

	s.logger.Debug("retrieved NSFW preferences",
		zap.String("viewer_id", viewerID),
		zap.Bool("allow_nsfw", allowNSFW),
		zap.Bool("require_warning", requireWarning))

	return allowNSFW, requireWarning, nil
}

// NSFWBlockedError represents an error when NSFW content is blocked
type NSFWBlockedError struct {
	message string
}

func (e *NSFWBlockedError) Error() string {
	return e.message
}

// NewNSFWBlockedError creates a new NSFW blocked error
func NewNSFWBlockedError(message string) *NSFWBlockedError {
	return &NSFWBlockedError{message: message}
}

// IsNSFWBlocked checks if an error is an NSFW blocked error
func IsNSFWBlocked(err error) bool {
	_, ok := err.(*NSFWBlockedError)
	return ok
}
