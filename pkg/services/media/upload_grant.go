package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	mediaprocessor "github.com/equaltoai/lesser/pkg/media"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UploadGrantTTL bounds every upload grant. The minted presigned PUT URL is a
// bearer capability for writing unverified bytes into the internal bucket, so
// the window must be short: fifteen minutes comfortably covers real uploads up
// to the size cap even on slow links while tightly bounding the exposure of a
// leaked URL, and it stays on the same order as the existing five-minute read
// presigns. The DynamoDB TTL attribute self-cleans grant rows after the same
// bound.
const UploadGrantTTL = 15 * time.Minute

var (
	// ErrUploadGrantUnavailable reports that the upload grant surface is not
	// wired (missing repository or object-store capability); it fails closed.
	ErrUploadGrantUnavailable = errors.New("upload grant service is unavailable")

	// ErrUploadGrantNotFound reports an unknown grant for the caller; owner
	// scoping is enforced by the repository key construction.
	ErrUploadGrantNotFound = errors.New("upload grant not found")

	// ErrUploadGrantExpired reports a grant past its bounded expiry; finalize
	// fails closed and never admits an asset from an expired grant.
	ErrUploadGrantExpired = errors.New("upload grant has expired")

	// ErrUploadGrantNotMinted reports a finalize attempt on a grant that was
	// already consumed (used or failed digest).
	ErrUploadGrantNotMinted = errors.New("upload grant is not minted")

	// ErrUploadGrantAlreadyConsumed reports that a concurrent finalize won the
	// single-use consume; the caller must not retry against the same grant.
	ErrUploadGrantAlreadyConsumed = errors.New("upload grant was already consumed by another finalize")

	// ErrUploadGrantDigestMismatch reports that the uploaded object's actual
	// bytes (or their size/content type) do not match the grant's declaration.
	// The grant is consumed and the object is deleted.
	ErrUploadGrantDigestMismatch = errors.New("uploaded bytes do not match the declared upload grant")

	// ErrUploadGrantObjectMissing reports a finalize before the caller PUT the
	// declared bytes. The grant is left minted so the PUT can be retried.
	ErrUploadGrantObjectMissing = errors.New("uploaded object not found; PUT the declared bytes before finalizing")

	// ErrUploadGrantObjectEmpty reports an empty uploaded object; the grant is
	// left minted for a retried PUT and the empty object is removed.
	ErrUploadGrantObjectEmpty = errors.New("uploaded object is empty")
)

// uploadGrantObjectStore is the S3 capability the presigned-companion
// transport needs beyond the read presigner: a constrained PUT presigner, a
// full-object download for digest verification, and deletion for the
// fail-closed mismatch path. It is type-asserted from the media service's S3
// service so existing deployments that lack the capability fail closed.
type uploadGrantObjectStore interface {
	// PresignPutObject mints a presigned PUT whose signed headers bind the
	// content type, the exact sha256 of the intended bytes (S3 validates the
	// body checksum at upload), and SSE-KMS encryption under the instance key.
	PresignPutObject(ctx context.Context, bucket, key, contentType, contentSHA256Hex, kmsKeyID string, expiry time.Duration) (string, error)
	// DownloadFile returns the stored object's bytes and stored content type.
	DownloadFile(ctx context.Context, bucket, key string) ([]byte, string, error)
	// DeleteFile removes one stored object.
	DeleteFile(ctx context.Context, bucket, key string) error
}

// MintUploadGrantInput declares the constraints the minted grant binds.
type MintUploadGrantInput struct {
	// Owner is the actor who may PUT and finalize; the grant row lives in this
	// actor's partition.
	Owner string
	// ContentType is the declared media type, signed into the presigned PUT.
	ContentType string
	// MaxSizeBytes is the declared size cap; finalize fails closed beyond it.
	MaxSizeBytes int64
	// ContentSHA256 is the hex-encoded sha256 of the exact intended bytes.
	ContentSHA256 string
}

// SetUploadGrantRepository wires the grant storage; upload grant operations
// fail closed until it is set.
func (s *Service) SetUploadGrantRepository(repo interfaces.UploadGrantRepository) {
	if s == nil {
		return
	}
	s.uploadGrantRepo = repo
}

// MintUploadGrant creates a one-time, hash-bound, actor-scoped upload grant
// and returns the presigned PUT URL bound to its constraints. The object key
// embeds a media ID minted with the grant so the PUT target and the final
// media record share one unguessable identity.
func (s *Service) MintUploadGrant(ctx context.Context, input MintUploadGrantInput) (*models.UploadGrant, string, error) {
	if s == nil || s.uploadGrantRepo == nil {
		return nil, "", ErrUploadGrantUnavailable
	}
	owner := strings.TrimSpace(input.Owner)
	contentType := strings.ToLower(strings.TrimSpace(input.ContentType))
	contentSHA256 := strings.ToLower(strings.TrimSpace(input.ContentSHA256))
	if owner == "" {
		return nil, "", errors.Join(ErrMediaValidationFailed, errors.New("owner is required"))
	}
	if !s.isValidMediaType(contentType) {
		return nil, "", errors.Join(ErrMediaValidationFailed, errors.New("unsupported content type"))
	}
	maxSize := s.maxFileSize
	if maxSize <= 0 {
		maxSize = 50 * 1024 * 1024
	}
	if input.MaxSizeBytes <= 0 || input.MaxSizeBytes > maxSize {
		return nil, "", errors.Join(ErrMediaValidationFailed, fmt.Errorf("max size must be between 1 and %d bytes", maxSize))
	}
	if !isValidSHA256Hex(contentSHA256) {
		return nil, "", errors.Join(ErrMediaValidationFailed, errors.New("declared sha256 must be 64 lowercase hex characters"))
	}
	bucket := strings.TrimSpace(s.s3Bucket)
	if bucket == "" {
		return nil, "", errors.Join(ErrMediaStorageFailed, errors.New("media S3 bucket is unavailable"))
	}
	if strings.TrimSpace(s.editorialKMSKeyID) == "" {
		return nil, "", errors.Join(ErrMediaStorageFailed, errors.New("editorial media KMS key is unavailable"))
	}
	store, ok := s.s3Service.(uploadGrantObjectStore)
	if !ok || store == nil {
		return nil, "", errors.Join(ErrUploadGrantUnavailable, errors.New("presigned PUT capability is unavailable"))
	}

	now := time.Now().UTC()
	grantID := uuid.New().String()
	mediaID := uuid.New().String()
	fileName := mediaID + extensionForContentType(contentType)
	s3Key := s.generateS3Key(mediaID, fileName)
	expiresAt := now.Add(UploadGrantTTL)

	url, err := store.PresignPutObject(ctx, bucket, s3Key, contentType, contentSHA256, s.editorialKMSKeyID, UploadGrantTTL)
	if err != nil {
		return nil, "", errors.Join(ErrMediaStorageFailed, err)
	}

	grant := &models.UploadGrant{
		Owner:         owner,
		GrantID:       grantID,
		ContentType:   contentType,
		MaxSizeBytes:  input.MaxSizeBytes,
		ContentSHA256: contentSHA256,
		S3Bucket:      bucket,
		S3Key:         s3Key,
		MediaID:       mediaID,
		FileName:      fileName,
		Status:        models.UploadGrantStatusMinted,
		GrantedAt:     now,
		ExpiresAt:     expiresAt,
		ExpiresAtTTL:  expiresAt.Unix(),
	}
	if err := s.uploadGrantRepo.CreateUploadGrant(ctx, grant); err != nil {
		return nil, "", err
	}
	s.logger.Info("minted upload grant",
		zap.String("owner", owner),
		zap.String("grant_id", grantID),
		zap.String("content_type", contentType),
		zap.Int64("max_size_bytes", input.MaxSizeBytes),
		zap.String("s3_key", s3Key),
	)
	return grant, url, nil
}

// FinalizeUploadGrant verifies that the stored object's actual bytes match the
// grant's declared sha256 (and declared size/type bounds) BEFORE any media
// record exists, consumes the grant exactly once, and only then creates the
// internal editorial media record through the M0/M1 pipeline. On any mismatch
// the grant is consumed to FAILED_DIGEST and the unverified object is deleted;
// a concurrent finalize loses the single-use consume and fails closed.
func (s *Service) FinalizeUploadGrant(ctx context.Context, ownerID, grantID string) (*models.Media, error) {
	if s == nil || s.uploadGrantRepo == nil {
		return nil, ErrUploadGrantUnavailable
	}
	ownerID = strings.TrimSpace(ownerID)
	grantID = strings.TrimSpace(grantID)
	if ownerID == "" || grantID == "" {
		return nil, errors.Join(ErrMediaValidationFailed, errors.New("owner and grantID are required"))
	}
	grant, err := s.uploadGrantRepo.GetUploadGrant(ctx, ownerID, grantID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrUploadGrantNotFound
		}
		return nil, err
	}
	now := time.Now().UTC()
	if !grant.IsMinted() {
		return nil, ErrUploadGrantNotMinted
	}
	if grant.Expired(now) {
		return nil, ErrUploadGrantExpired
	}
	store, ok := s.s3Service.(uploadGrantObjectStore)
	if !ok || store == nil {
		return nil, errors.Join(ErrUploadGrantUnavailable, errors.New("presigned PUT capability is unavailable"))
	}

	bytes, storedType, err := store.DownloadFile(ctx, grant.S3Bucket, grant.S3Key)
	if err != nil {
		return nil, errors.Join(ErrUploadGrantObjectMissing, err)
	}
	if len(bytes) == 0 {
		_ = store.DeleteFile(ctx, grant.S3Bucket, grant.S3Key)
		return nil, ErrUploadGrantObjectEmpty
	}

	if reason := verifyUploadedObject(grant, bytes, storedType); reason != "" {
		// Consume first: only the winner of the single-use consume may delete
		// the object. A loser must not delete bytes the winning finalize is
		// about to admit as an asset.
		if err := s.consumeUploadGrant(ctx, grant, models.UploadGrantStatusFailedDigest, reason, now); err != nil {
			return nil, err
		}
		if err := store.DeleteFile(ctx, grant.S3Bucket, grant.S3Key); err != nil {
			s.logger.Warn("failed to delete unverified upload object",
				zap.String("grant_id", grantID),
				zap.String("s3_key", grant.S3Key),
				zap.Error(err))
		}
		return nil, errors.Join(ErrUploadGrantDigestMismatch, fmt.Errorf("%s", reason))
	}

	if err := s.consumeUploadGrant(ctx, grant, models.UploadGrantStatusUsed, "", now); err != nil {
		return nil, err
	}
	media, err := s.createMediaFromVerifiedUpload(ctx, grant, bytes, now)
	if err != nil {
		// The grant is consumed; remove the object so no asset and no orphan
		// bytes survive a failed record creation.
		if deleteErr := store.DeleteFile(ctx, grant.S3Bucket, grant.S3Key); deleteErr != nil {
			s.logger.Warn("failed to delete object after media record creation failure",
				zap.String("grant_id", grantID),
				zap.String("s3_key", grant.S3Key),
				zap.Error(deleteErr))
		}
		return nil, errors.Join(ErrMediaCreateFailed, err)
	}
	s.logger.Info("finalized upload grant into editorial media",
		zap.String("owner", ownerID),
		zap.String("grant_id", grantID),
		zap.String("media_id", media.MediaID),
		zap.String("content_hash", media.ContentHash),
	)
	return media, nil
}

// UploadGrant returns one grant with its inspectable lifecycle state for the
// owner, plus a fresh presigned PUT URL while the grant is still minted (so a
// transient PUT failure can be retried). The URL is best-effort on this query
// path; the mint response is authoritative.
func (s *Service) UploadGrant(ctx context.Context, ownerID, grantID string) (*models.UploadGrant, string, error) {
	if s == nil || s.uploadGrantRepo == nil {
		return nil, "", ErrUploadGrantUnavailable
	}
	grant, err := s.uploadGrantRepo.GetUploadGrant(ctx, strings.TrimSpace(ownerID), strings.TrimSpace(grantID))
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, "", ErrUploadGrantNotFound
		}
		return nil, "", err
	}
	now := time.Now().UTC()
	if !grant.IsMinted() || grant.Expired(now) {
		return grant, "", nil
	}
	store, ok := s.s3Service.(uploadGrantObjectStore)
	if !ok || store == nil {
		return grant, "", nil
	}
	remaining := grant.ExpiresAt.Sub(now)
	if remaining <= 0 {
		return grant, "", nil
	}
	url, err := store.PresignPutObject(ctx, grant.S3Bucket, grant.S3Key, grant.ContentType, grant.ContentSHA256, s.editorialKMSKeyID, remaining)
	if err != nil {
		s.logger.Warn("failed to re-presign upload grant PUT",
			zap.String("grant_id", grantID),
			zap.Error(err))
		return grant, "", nil
	}
	return grant, url, nil
}

// consumeUploadGrant runs the version-conditioned single-use consume and maps
// the race onto ErrUploadGrantAlreadyConsumed.
func (s *Service) consumeUploadGrant(ctx context.Context, grant *models.UploadGrant, status, failureReason string, now time.Time) error {
	if err := s.uploadGrantRepo.ConsumeUploadGrant(ctx, grant, status, failureReason, now); err != nil {
		if errors.Is(err, interfaces.ErrUploadGrantConsumed) {
			return errors.Join(ErrUploadGrantAlreadyConsumed, err)
		}
		return err
	}
	return nil
}

// verifyUploadedObject returns "" when the uploaded bytes satisfy the grant's
// declared size and digest bounds (and the stored content type agrees), or a
// human-readable reason for the mismatch. The digest is computed here from the
// actual stored bytes: neither Content-Length nor any client claim is trusted.
func verifyUploadedObject(grant *models.UploadGrant, bytes []byte, storedType string) string {
	if grant == nil {
		return "upload grant is missing"
	}
	if int64(len(bytes)) > grant.MaxSizeBytes {
		return fmt.Sprintf("uploaded object size %d exceeds declared cap %d", len(bytes), grant.MaxSizeBytes)
	}
	actual := sha256.Sum256(bytes)
	actualHex := hex.EncodeToString(actual[:])
	if actualHex != strings.ToLower(strings.TrimSpace(grant.ContentSHA256)) {
		return "uploaded object digest does not match the declared sha256"
	}
	storedType = strings.TrimSpace(storedType)
	if storedType != "" && !strings.EqualFold(storedType, strings.TrimSpace(grant.ContentType)) {
		return fmt.Sprintf("uploaded object content type %q does not match declared %q", storedType, grant.ContentType)
	}
	return ""
}

// createMediaFromVerifiedUpload creates the internal editorial media record
// for bytes that passed the digest gate, mirroring the M0 pipeline's record
// construction: internal visibility, sha256-bound provenance, and synchronous
// image dimension processing (images become ready; non-images stay pending,
// exactly as UploadMedia leaves them).
func (s *Service) createMediaFromVerifiedUpload(ctx context.Context, grant *models.UploadGrant, bytes []byte, now time.Time) (*models.Media, error) {
	contentHash := "sha256:" + strings.ToLower(strings.TrimSpace(grant.ContentSHA256))
	media := &models.Media{
		MediaID:     grant.MediaID,
		UserID:      grant.Owner,
		FileName:    grant.FileName,
		ContentType: grant.ContentType,
		FileSize:    int64(len(bytes)),
		ContentHash: contentHash,
		S3Bucket:    grant.S3Bucket,
		S3Key:       grant.S3Key,
		Status:      models.StatusPending,
		Visibility:  models.MediaVisibilityInternal,
		Provenance: &models.MediaProvenance{
			Origin:           models.EditorialMediaOriginSupplied,
			ResponsibleActor: grant.Owner,
			RecordedAt:       now,
			ContentIntegrity: contentHash,
		},
	}
	if media.IsImage() {
		processed, err := mediaprocessor.ProcessImage(bytes, grant.ContentType)
		if err != nil {
			s.logger.Warn("failed to process uploaded image, marking failed",
				zap.String("media_id", media.MediaID),
				zap.Error(err))
			media.Status = models.StatusFailed
		} else if original, exists := processed["original"]; exists {
			media.Width = original.Width
			media.Height = original.Height
			media.Blurhash = original.Blurhash
			media.Status = models.StatusReady
		} else {
			media.Status = models.StatusFailed
		}
	}
	if err := s.mediaRepo.CreateMedia(ctx, media); err != nil {
		return nil, err
	}
	return media, nil
}

// isValidSHA256Hex reports whether value is exactly 64 lowercase hex digits.
func isValidSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// extensionForContentType derives a file extension from a supported media type.
func extensionForContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/ogg":
		return ".ogv"
	case "video/avi", "video/x-msvideo":
		return ".avi"
	case "video/mov", "video/quicktime":
		return ".mov"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".oga"
	case "audio/aac":
		return ".aac"
	case "audio/flac":
		return ".flac"
	default:
		return ""
	}
}
