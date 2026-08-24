// Package media provides the core Media Service for the Lesser project's API alignment.
// This service handles all media operations including file uploads, processing,
// metadata updates, and storage management. It emits appropriate events for real-time
// streaming and queues async processing for thumbnails and optimization.
package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
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
	objectDeleter     ObjectDeleter
	metadataDeleter   MetadataDeleter
	s3Service         S3Service
	editorialKMSKeyID string
	orphanSource      OrphanedPublishedMintSource
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

// ObjectDeleter removes one physical media object from its backing store.
type ObjectDeleter interface {
	DeleteMediaObject(ctx context.Context, bucket, key string) error
}

// MetadataDeleter removes processor metadata associated with a media ID.
type MetadataDeleter interface {
	DeleteMediaMetadata(ctx context.Context, mediaID string) error
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
}

// InternalS3Service stores an object under the instance KMS key. The public
// CloudFront origin has no permission to decrypt these objects, while Lesser's
// authorized Lambda role can presign exact-object reads.
type InternalS3Service interface {
	UploadInternalFile(ctx context.Context, bucket, key string, data []byte, contentType, kmsKeyID string) (string, error)
}

// S3Presigner issues short-lived object reads without making an internal asset
// part of the unsigned public CDN surface.
type S3Presigner interface {
	GeneratePresignedURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// PublishedMediaCopier copies the exact original bytes of an internal editorial
// asset to the durable unsigned serving surface at the publish transition. The
// destination object is SSE-S3 (the CloudFront origin can serve it) while the
// source remains SSE-KMS under the instance key.
type PublishedMediaCopier interface {
	CopyFileToPublished(ctx context.Context, bucket, sourceKey, destinationKey, contentType string) (string, error)
}

// OrphanedPublishedMintSource enumerates durable published mints that no live
// article references: assets minted by a publish whose draft is terminally
// failed and whose compensating rollback never ran or failed. The registry
// wires it from the CMS side; reconciliation is a no-op without it.
// RecheckOrphanedPublishedMint re-verifies one candidate's orphan premise at
// unpublish time so the enumerate-then-unpublish window cannot clear an asset a
// concurrent republish just made live.
type OrphanedPublishedMintSource interface {
	ListOrphanedPublishedMintIDs(ctx context.Context) ([]string, error)
	RecheckOrphanedPublishedMint(ctx context.Context, mediaID string) (bool, error)
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

// SetDeletionDependencies wires physical-object and processor-metadata cleanup.
func (s *Service) SetDeletionDependencies(objectDeleter ObjectDeleter, metadataDeleter MetadataDeleter) {
	s.objectDeleter = objectDeleter
	s.metadataDeleter = metadataDeleter
}

// SetS3Service wires the object-storage client used for original media bytes.
func (s *Service) SetS3Service(s3Service S3Service) {
	s.s3Service = s3Service
}

// SetOrphanPublishedMintSource wires the enumeration of orphaned durable
// published mints used by ReconcileOrphanedPublishedMedia. Leaving it unwired
// makes reconciliation a no-op; the registry wires it from the CMS side.
func (s *Service) SetOrphanPublishedMintSource(source OrphanedPublishedMintSource) {
	s.orphanSource = source
}

// SetEditorialKMSKeyID configures the instance key used to keep internal
// editorial originals outside the unsigned CDN read surface.
func (s *Service) SetEditorialKMSKeyID(keyID string) {
	s.editorialKMSKeyID = strings.TrimSpace(keyID)
}

// SetMaxFileSize sets the maximum allowed file size
func (s *Service) SetMaxFileSize(maxSize int64) {
	s.maxFileSize = maxSize
}

// Command structs for operations

// UploadMediaCommand contains all data needed to upload a media file
type UploadMediaCommand struct {
	UserID        string                  `json:"user_id" validate:"required"`
	FileName      string                  `json:"file_name" validate:"required"`
	ContentType   string                  `json:"content_type" validate:"required"`
	FileData      []byte                  `json:"file_data" validate:"required"`
	Description   string                  `json:"description" validate:"max=1500"` // Alt text
	Focus         string                  `json:"focus"`                           // Focus point for cropping (x,y)
	Sensitive     bool                    `json:"sensitive"`
	SpoilerText   string                  `json:"spoiler_text"`
	MediaCategory models.MediaCategory    `json:"media_category"`
	Editorial     bool                    `json:"editorial"`
	Provenance    *models.MediaProvenance `json:"provenance,omitempty"`
}

// UpdateMediaCommand contains all data needed to update media metadata
type UpdateMediaCommand struct {
	MediaID     string `json:"media_id" validate:"required"`
	UserID      string `json:"user_id" validate:"required"`     // Must be the media owner
	Description string `json:"description" validate:"max=1500"` // Alt text
	Focus       string `json:"focus"`                           // Focus point for cropping (x,y)
}

// DeleteMediaCommand identifies media and the owner requesting its removal.
type DeleteMediaCommand struct {
	MediaID string `json:"media_id" validate:"required"`
	UserID  string `json:"user_id" validate:"required"`
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

// EditorialAccess is a short-lived, exact-byte read for an internal asset.
// Authorization of the bound draft is deliberately performed by the CMS
// service before this storage capability is invoked.
type EditorialAccess struct {
	URL         string
	ExpiresAt   time.Time
	ContentHash string
}

// PublishedMedia is the durable public serving minted for one internal
// editorial asset at the publish transition. The URL serves the exact approved
// original bytes indefinitely: no expiring presignature, no temporary
// generator URL, no dependence on the internal KMS-read posture.
type PublishedMedia struct {
	MediaID     string
	ContentHash string
	ContentType string
	FileSize    int64
	Width       int
	Height      int
	URL         string
	S3Key       string
	PublishedAt time.Time
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
		ContentHash:   contentHash(cmd.FileData),
		Description:   cmd.Description,
		Focus:         cmd.Focus,
		IsNSFW:        cmd.Sensitive,
		SpoilerText:   strings.TrimSpace(cmd.SpoilerText),
		MediaCategory: cmd.MediaCategory,
		Status:        models.StatusPending,
	}
	if cmd.Editorial {
		media.Visibility = models.MediaVisibilityInternal
		media.Provenance = cmd.Provenance
		if err := media.Provenance.Normalize(media.UserID, media.ContentHash, time.Now().UTC()); err != nil {
			return nil, errors.Join(ErrMediaValidationFailed, err)
		}
	} else {
		media.Visibility = models.MediaVisibilityPublic
	}

	// Persist the original bytes before publishing a media record that points at them.
	s3Key := s.generateS3Key(media.MediaID, cmd.FileName)
	media.S3Bucket = s.s3Bucket
	media.S3Key = s3Key
	if s.s3Service == nil {
		return nil, errors.Join(ErrMediaStorageFailed, errors.New("media S3 service is unavailable"))
	}
	if err := s.uploadOriginal(ctx, media, cmd.FileData); err != nil {
		return nil, errors.Join(ErrMediaStorageFailed, err)
	}

	// Generate CDN URL
	if !media.IsInternalEditorial() && s.cdnDomain != "" {
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
			if media.IsInternalEditorial() {
				media.Status = models.StatusFailed
			}
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
			if media.IsInternalEditorial() {
				media.Status = models.StatusReady
			}
		}
	}

	// Store the media record
	if err := s.mediaRepo.CreateMedia(ctx, media); err != nil {
		if cleanupErr := s.s3Service.DeleteFile(ctx, s.s3Bucket, s3Key); cleanupErr != nil {
			s.logger.Warn("failed to clean up media object after record failure",
				zap.String("bucket", s.s3Bucket),
				zap.String("key", s3Key),
				zap.Error(cleanupErr))
		}
		return nil, errors.Join(ErrMediaStorageFailed, err)
	}

	s.logger.Info("uploaded media successfully",
		zap.String("media_id", media.MediaID),
		zap.String("user_id", cmd.UserID))

	// Emit events
	events := s.emitMediaUploadedEvents(ctx, media)

	// The existing processor writes unsigned CDN variants. Internal editorial
	// images are validated synchronously above and deliberately do not enter
	// that public-derivative pipeline. Public/social uploads retain the M0 queue.
	if !media.IsInternalEditorial() {
		if err := s.queueMediaProcessing(ctx, media); err != nil {
			// Don't fail the upload if queueing fails - just log the error
			s.logger.Warn("failed to queue media processing, processing will be skipped",
				zap.String("media_id", media.MediaID),
				zap.Error(err))
		}
	}

	return &Result{
		Media:  media,
		Events: events,
	}, nil
}

func (s *Service) uploadOriginal(ctx context.Context, media *models.Media, data []byte) error {
	if media.IsInternalEditorial() {
		if s.editorialKMSKeyID == "" {
			return errors.New("editorial media KMS key is unavailable")
		}
		internalStore, ok := s.s3Service.(InternalS3Service)
		if !ok {
			return errors.New("internal editorial media storage is unavailable")
		}
		_, err := internalStore.UploadInternalFile(
			ctx,
			media.S3Bucket,
			media.S3Key,
			data,
			media.ContentType,
			s.editorialKMSKeyID,
		)
		return err
	}
	_, err := s.s3Service.UploadFile(ctx, media.S3Bucket, media.S3Key, data, media.ContentType)
	return err
}

func contentHash(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
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
	if media == nil || media.UserID != cmd.UserID {
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

// DeleteMedia deletes an owned media record and notifies the owner's stream.
func (s *Service) DeleteMedia(ctx context.Context, cmd *DeleteMediaCommand) error {
	if cmd == nil {
		return errors.Join(ErrMediaValidationFailed, errors.New("delete media command cannot be nil"))
	}
	cmd.MediaID = strings.TrimSpace(cmd.MediaID)
	cmd.UserID = strings.TrimSpace(cmd.UserID)
	if err := common.ValidateRequiredParam("media_id", cmd.MediaID); err != nil {
		return errors.Join(ErrMediaValidationFailed, err)
	}
	if err := common.ValidateRequiredParam("user_id", cmd.UserID); err != nil {
		return errors.Join(ErrMediaValidationFailed, err)
	}

	media, err := s.mediaRepo.GetMedia(ctx, cmd.MediaID)
	if err != nil {
		if apperrors.HasCode(err, apperrors.CodeNotFound) {
			return hiddenMediaDeleteError()
		}
		return errors.Join(ErrMediaRetrievalFailed, err)
	}
	if media == nil || media.UserID != cmd.UserID {
		return hiddenMediaDeleteError()
	}
	if media.UsageCount > 0 {
		return apperrors.NewAppError(apperrors.CodeConflict, apperrors.CategoryBusiness, "media is still referenced").
			WithInternalError(ErrMediaInUse)
	}
	if err := s.deleteMediaObjects(ctx, media); err != nil {
		return errors.Join(ErrMediaDeleteFailed, err)
	}
	if s.metadataDeleter != nil {
		if err := s.metadataDeleter.DeleteMediaMetadata(ctx, media.MediaID); err != nil {
			return errors.Join(ErrMediaDeleteFailed, err)
		}
	}
	if err := s.mediaRepo.DeleteMedia(ctx, cmd.MediaID); err != nil {
		return errors.Join(ErrMediaDeleteFailed, err)
	}
	s.emitMediaDeletedEvents(ctx, media)
	s.logger.Info("deleted media successfully", zap.String("media_id", cmd.MediaID), zap.String("user_id", cmd.UserID))
	return nil
}

func hiddenMediaDeleteError() error {
	return apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "media not found").
		WithInternalError(ErrMediaUnauthorizedAccess)
}

func (s *Service) deleteMediaObjects(ctx context.Context, media *models.Media) error {
	if media == nil {
		return nil
	}
	keys := make([]string, 0, len(media.Variants)+1)
	seen := make(map[string]struct{}, len(media.Variants)+1)
	appendKey := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	appendKey(media.S3Key)
	for _, variant := range media.Variants {
		appendKey(variant.S3Key)
	}
	if len(keys) == 0 {
		return nil
	}
	if s.objectDeleter == nil {
		return errors.New("media object deletion is unavailable")
	}
	bucket := strings.TrimSpace(media.S3Bucket)
	if bucket == "" {
		return errors.New("media storage bucket is unavailable")
	}
	for _, key := range keys {
		if err := s.objectDeleter.DeleteMediaObject(ctx, bucket, key); err != nil {
			return fmt.Errorf("delete media object %q: %w", key, err)
		}
	}
	return nil
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

// IssueEditorialAccess signs a read for one internal object. Callers must first
// prove that this media ID is bound to a draft the current actor owns or has an
// active review grant for; this method intentionally grants no list capability.
func (s *Service) IssueEditorialAccess(ctx context.Context, mediaID string) (*EditorialAccess, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("media ID is required"))
	}
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}
	if media == nil || !media.IsInternalEditorial() {
		return nil, ErrMediaUnauthorizedAccess
	}
	bucket := strings.TrimSpace(media.S3Bucket)
	key := strings.TrimSpace(media.S3Key)
	if bucket == "" || key == "" {
		return nil, errors.Join(ErrMediaStorageFailed, errors.New("internal media storage location is unavailable"))
	}
	presigner, ok := s.s3Service.(S3Presigner)
	if !ok || presigner == nil {
		return nil, errors.Join(ErrMediaStorageFailed, errors.New("internal media presigner is unavailable"))
	}
	const ttl = 5 * time.Minute
	url, err := presigner.GeneratePresignedURL(ctx, bucket, key, ttl)
	if err != nil {
		return nil, errors.Join(ErrMediaStorageFailed, err)
	}
	return &EditorialAccess{URL: url, ExpiresAt: time.Now().UTC().Add(ttl), ContentHash: media.ContentHash}, nil
}

// PublishMediaDurably transitions one internal editorial asset to durable
// public serving of its exact approved bytes. The publish transition is the
// single point where durable public serving is minted: before it, internal
// assets expose no unsigned URL through the application contract.
func (s *Service) PublishMediaDurably(ctx context.Context, mediaID string) (*PublishedMedia, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("media ID is required"))
	}
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}
	if media == nil {
		return nil, ErrMediaUnauthorizedAccess
	}
	if !media.IsInternalEditorial() {
		return nil, errors.Join(ErrMediaUnauthorizedAccess, errors.New("durable published serving is minted only for internal editorial assets"))
	}
	if media.Provenance == nil || media.Provenance.ContentIntegrity != media.ContentHash {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("editorial media integrity is unavailable"))
	}
	if !media.EditorialLifecycleAvailableForPublish() {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("editorial media lifecycle does not allow publication"))
	}
	if !media.IsReady() {
		return nil, ErrMediaNotReady
	}
	bucket := strings.TrimSpace(media.S3Bucket)
	sourceKey := strings.TrimSpace(media.S3Key)
	if bucket == "" || sourceKey == "" {
		return nil, errors.Join(ErrMediaStorageFailed, errors.New("internal media storage location is unavailable"))
	}
	copier, ok := s.s3Service.(PublishedMediaCopier)
	if !ok || copier == nil {
		return nil, errors.Join(ErrMediaStorageFailed, errors.New("durable published copy capability is unavailable"))
	}
	if strings.TrimSpace(s.cdnDomain) == "" {
		return nil, errors.Join(ErrMediaStorageFailed, errors.New("CDN domain is required to mint durable published serving"))
	}
	destinationKey := "published/" + sourceKey
	location, err := copier.CopyFileToPublished(ctx, bucket, sourceKey, destinationKey, media.ContentType)
	if err != nil {
		return nil, errors.Join(ErrMediaStorageFailed, err)
	}
	_ = location
	publishedURL := fmt.Sprintf("https://%s/%s", s.cdnDomain, destinationKey)
	publishedAt := time.Now().UTC()
	if err := s.mediaRepo.UpdateMediaPublishedState(ctx, mediaID, destinationKey, publishedURL, publishedAt, media.ModelVersion); err != nil {
		// The copy is already live on the unsigned CDN surface but no record
		// references it. Compensate with a best-effort delete of the
		// deterministic published key; a cleanup failure is logged, never
		// surfaced, so the caller still sees the record-write error.
		s.deletePublishedObject(ctx, bucket, destinationKey)
		return nil, errors.Join(ErrMediaUpdateFailed, err)
	}
	s.logger.Info("minted durable published serving",
		zap.String("media_id", mediaID),
		zap.String("published_key", destinationKey),
		zap.String("published_url", publishedURL))
	return &PublishedMedia{
		MediaID:     media.MediaID,
		ContentHash: media.ContentHash,
		ContentType: media.ContentType,
		FileSize:    media.FileSize,
		Width:       media.Width,
		Height:      media.Height,
		URL:         publishedURL,
		S3Key:       destinationKey,
		PublishedAt: publishedAt,
	}, nil
}

// deletePublishedObject best-effort removes one durable published object. The
// deterministic published key makes the compensating delete idempotent, and the
// original error is preserved: cleanup failures are logged at error level so an
// orphaned object stays alarmable, but never surfaced over the original error.
func (s *Service) deletePublishedObject(ctx context.Context, bucket, destinationKey string) {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(destinationKey) == "" {
		return
	}
	if err := s.s3Service.DeleteFile(ctx, bucket, destinationKey); err != nil {
		s.logger.Error("failed to compensate orphaned durable published serving",
			zap.String("bucket", bucket),
			zap.String("published_key", destinationKey),
			zap.Error(err))
	}
}

// UnpublishMediaDurably best-effort removes durable public serving minted for
// one internal asset. It clears the record state first under the observed model
// version (a concurrent re-mint advances the version and is left intact) and
// only then deletes the deterministic published object, re-reading the record
// after the clear so a re-mint that lands between the two steps keeps its
// serving. Assets without published state are a no-op, so repeated rollback is
// idempotent.
func (s *Service) UnpublishMediaDurably(ctx context.Context, mediaID string) error {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return nil
	}
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		// Unreadable or missing media means there is nothing durable to roll
		// back; compensation is best-effort.
		s.logger.Debug("unpublish rollback skipped for unreadable media",
			zap.String("media_id", mediaID), zap.Error(err))
		return nil
	}
	if media == nil || !media.IsInternalEditorial() || !media.IsPublished() {
		return nil
	}
	bucket := strings.TrimSpace(media.S3Bucket)
	publishedKey := strings.TrimSpace(media.PublishedS3Key)
	if err := s.mediaRepo.ClearMediaPublishedState(ctx, mediaID, media.ModelVersion); err != nil {
		// A failed clear leaves the record published. The failure is logged at
		// error level so the orphan stays alarmable, then swallowed to preserve
		// the best-effort contract; ReconcileOrphanedPublishedMedia can retry it.
		s.logger.Error("failed to clear published serving on rollback",
			zap.String("media_id", mediaID), zap.Error(err))
		return nil
	}
	// Re-read the record after the clear. A concurrent re-mint between the two
	// steps advances the model version and re-publishes; deleting the
	// deterministic object then would remove a legitimate concurrent mint's
	// serving. Only delete when the record still shows no published state.
	fresh, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		s.logger.Warn("failed to re-read media after clearing published serving",
			zap.String("media_id", mediaID), zap.Error(err))
		return nil
	}
	if fresh == nil || fresh.IsPublished() {
		// A concurrent re-mint re-published the record; leave its serving intact.
		return nil
	}
	// Residual TOCTOU window: between the fresh re-read above and this delete,
	// a concurrent same-key re-mint can land and still lose its object. Fully
	// closing the window needs a conditional delete or per-mint object keys
	// (schema/scale change), which is deliberately out of scope; a post-delete
	// re-check is pointless because the object is already gone and the
	// deterministic key makes a later reconcile idempotent. The version-guarded
	// clear above already bounds the window to this single delete, and the
	// reconcile path re-verifies the orphan premise before unpublishing, so the
	// residual risk is a re-mint landing within this delete's latency and
	// losing its serving, which the operator can re-mint by re-publishing.
	s.deletePublishedObject(ctx, bucket, publishedKey)
	return nil
}

// ReconcileOrphanedPublishedMedia re-runs the best-effort unpublish for every
// durable published mint the wired OrphanedPublishedMintSource reports as
// orphaned (owning draft terminally failed, no live article reference). It is
// idempotent: UnpublishMediaDurably is a no-op once the record is unpublished
// and version-guarded against concurrent re-mints, so repeated reconciliation
// is safe and a live published asset is never touched. Before each unpublish
// the source re-verifies the candidate's orphan premise at the current state,
// so a draft that was republished (or an article that appeared) between the
// enumeration and the unpublish aborts that candidate.
func (s *Service) ReconcileOrphanedPublishedMedia(ctx context.Context) error {
	if s.orphanSource == nil {
		return nil
	}
	orphanedIDs, err := s.orphanSource.ListOrphanedPublishedMintIDs(ctx)
	if err != nil {
		return errors.Join(ErrMediaRetrievalFailed, err)
	}
	for _, mediaID := range orphanedIDs {
		mediaID = strings.TrimSpace(mediaID)
		if mediaID == "" {
			continue
		}
		// Re-verify the orphan premise at unpublish time. Unverifiable or
		// changed candidates are skipped fail closed: a live published asset is
		// never unpublished on a stale enumeration.
		stillOrphaned, err := s.orphanSource.RecheckOrphanedPublishedMint(ctx, mediaID)
		if err != nil {
			s.logger.Warn("orphan reconciliation re-check failed; skipping candidate",
				zap.String("media_id", mediaID), zap.Error(err))
			continue
		}
		if !stillOrphaned {
			s.logger.Info("orphan reconciliation candidate no longer orphaned; skipping",
				zap.String("media_id", mediaID))
			continue
		}
		s.logger.Info("reconciling orphaned published media mint",
			zap.String("media_id", mediaID))
		if err := s.UnpublishMediaDurably(ctx, mediaID); err != nil {
			// UnpublishMediaDurably is best-effort and reports failures at error
			// level; keep reconciling the remaining candidates.
			s.logger.Warn("orphan reconciliation unpublish failed",
				zap.String("media_id", mediaID), zap.Error(err))
		}
	}
	return nil
}

// UpdateEditorialLifecycleCommand identifies the asset and the owner requesting
// an explicit editorial lifecycle change.
type UpdateEditorialLifecycleCommand struct {
	MediaID             string
	UserID              string
	Lifecycle           models.EditorialLifecycle
	SupersededByMediaID string
}

// UpdateEditorialLifecycle applies an explicit editorial lifecycle change to an
// internal asset. Withdrawn, superseded, and unavailable states are inspectable
// through the draft preview surface and block publication until re-review.
func (s *Service) UpdateEditorialLifecycle(ctx context.Context, cmd *UpdateEditorialLifecycleCommand) (*models.Media, error) {
	if cmd == nil {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("update editorial lifecycle command cannot be nil"))
	}
	cmd.MediaID = strings.TrimSpace(cmd.MediaID)
	cmd.UserID = strings.TrimSpace(cmd.UserID)
	if cmd.MediaID == "" || cmd.UserID == "" {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("media ID and owner are required"))
	}
	media, err := s.mediaRepo.GetMedia(ctx, cmd.MediaID)
	if err != nil {
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}
	if media == nil || media.UserID != cmd.UserID {
		return nil, ErrMediaUnauthorizedAccess
	}
	if !media.IsInternalEditorial() {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("editorial lifecycle applies only to internal editorial media"))
	}
	lifecycle := models.EditorialLifecycle(strings.ToLower(strings.TrimSpace(string(cmd.Lifecycle))))
	if lifecycle == "" || lifecycle == models.EditorialLifecycleAvailable {
		lifecycle = ""
	} else {
		switch lifecycle {
		case models.EditorialLifecycleWithdrawn, models.EditorialLifecycleSuperseded, models.EditorialLifecycleUnavailable:
		default:
			return nil, errors.Join(ErrMediaValidationFailed, fmt.Errorf("invalid editorial lifecycle %q", lifecycle))
		}
	}
	cmd.SupersededByMediaID = strings.TrimSpace(cmd.SupersededByMediaID)
	if cmd.SupersededByMediaID != "" && lifecycle != models.EditorialLifecycleSuperseded {
		// Mirror the model's whole-write validation: the successor attribute is
		// meaningful only under the superseded lifecycle. Rejecting it here keeps
		// the field-scoped writer from persisting a model-invalid state that
		// would later block unrelated metadata updates.
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("superseded-by media ID requires the superseded lifecycle"))
	}
	if lifecycle == models.EditorialLifecycleSuperseded && cmd.SupersededByMediaID == "" {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("superseded editorial media must name the superseding asset"))
	}
	if lifecycle == models.EditorialLifecycleSuperseded {
		successor, getErr := s.mediaRepo.GetMedia(ctx, cmd.SupersededByMediaID)
		if getErr != nil {
			return nil, errors.Join(ErrMediaRetrievalFailed, getErr)
		}
		if successor == nil || strings.TrimSpace(successor.UserID) != cmd.UserID {
			return nil, ErrMediaUnauthorizedAccess
		}
		if !successor.IsInternalEditorial() {
			return nil, errors.Join(ErrMediaValidationFailed, errors.New("superseding media must be an internal editorial asset"))
		}
	}
	if err := s.mediaRepo.UpdateMediaEditorialState(ctx, cmd.MediaID, lifecycle, cmd.SupersededByMediaID, media.ModelVersion); err != nil {
		return nil, errors.Join(ErrMediaUpdateFailed, err)
	}
	return s.mediaRepo.GetMedia(ctx, cmd.MediaID)
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
	if cmd.Editorial && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(cmd.ContentType)), "image/") {
		return common.ErrValidation("contentType", "editorial media currently requires an image").InternalError
	}

	if err := ValidateSVGUpload(cmd.ContentType, cmd.FileData); err != nil {
		return err
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

	return validTypes[normalizedContentType(contentType)]
}

func (s *Service) validateFileExtension(fileName, contentType string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))

	// Get expected MIME type from extension
	expectedType := mime.TypeByExtension(ext)
	if err := common.ValidateRequiredParam("expected_type", expectedType); err != nil {
		return false // Unknown extension
	}

	// Compare base types (ignore charset and other parameters)
	expectedBase := normalizedContentType(expectedType)
	actualBase := normalizedContentType(contentType)

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
	if media.IsInternalEditorial() {
		return ErrMediaUnauthorizedAccess
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

func (s *Service) isOwnedByViewer(media *models.Media, viewerID string) bool {
	return media != nil && strings.TrimSpace(viewerID) != "" && media.UserID == viewerID
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

func (s *Service) emitMediaDeletedEvents(ctx context.Context, media *models.Media) []*streaming.Event {
	if s.publisher == nil || media == nil {
		return nil
	}
	event := &streaming.Event{
		Type:      streaming.MediaDeleted,
		Stream:    fmt.Sprintf("user:%s", media.UserID),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"media_id": media.MediaID,
		},
	}
	if err := s.publisher.PublishToUser(ctx, media.UserID, event); err != nil {
		s.logger.Error("failed to publish media deletion to user stream", zap.Error(err))
		return nil
	}
	return []*streaming.Event{event}
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
	job := &models.MediaJob{
		JobID:    jobID,
		MediaID:  media.MediaID,
		Username: media.UserID,
		Status:   models.StatusPending,
		S3Key:    media.S3Key,
		MimeType: media.ContentType,
		FileHash: media.ContentHash,
		FileSize: media.FileSize,
	}
	if err := s.mediaRepo.CreateMediaJob(ctx, job); err != nil {
		return errors.Join(ErrMediaProcessingQueueFailed, err)
	}

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

// GetStreamingURL returns a media streaming URL and metadata for GraphQL.
// The viewer must own the media; public status/object resolvers should expose
// already-authorized attachment URLs instead of minting owner-scoped stream URLs.
func (s *Service) GetStreamingURL(ctx context.Context, mediaID string, viewerID string) (*model.MediaStream, error) {
	s.logger.Debug("getting media streaming URL",
		zap.String("media_id", mediaID),
		zap.String("viewer_id", viewerID))

	// Get the media record
	media, err := s.mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, errors.Join(ErrMediaRetrievalFailed, err)
	}

	if !s.isOwnedByViewer(media, viewerID) {
		s.logger.Warn("denying media streaming URL ownership mismatch",
			zap.String("media_id", mediaID),
			zap.String("media_owner", media.UserID),
			zap.String("viewer_id", viewerID))
		return nil, ErrMediaUnauthorizedAccess
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
