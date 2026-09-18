package media

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// AvatarS3Prefix namespaces every avatar object so the serve route can never
	// address media outside this prefix.
	AvatarS3Prefix = "avatars/"

	// avatarServePathPrefix is the public serve path every stored avatar URL uses.
	avatarServePathPrefix = "/api/v1/avatars/"

	// AvatarMaxUploadBytes caps avatar uploads at 512 KiB. The platform request
	// cap is 512 KiB (apptheory Limits.MaxRequestBytes in cmd/api/main.go), which
	// already bounds the route, so the handler can never receive more.
	AvatarMaxUploadBytes = 524288

	// AvatarMaxServeBytes bounds the object read to the upload cap.
	AvatarMaxServeBytes = AvatarMaxUploadBytes

	// AvatarCacheControl marks avatar responses as immutable: the object key
	// embeds a fresh UUID per upload, so bytes never change under a key.
	AvatarCacheControl = "public, max-age=31536000, immutable"
)

const avatarSVGContentType = "image/svg+xml"

var avatarAllowedContentTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/gif":  {},
	"image/webp": {},
}

var avatarIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// S3ObjectReader is the optional read capability of the object store backing
// avatars. It is type-asserted from S3Service so deployments that only write
// objects fail closed on the serve route.
type S3ObjectReader interface {
	DownloadFile(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, string, error)
}

// StoreAvatarCommand contains the bytes and declared type for an avatar upload.
type StoreAvatarCommand struct {
	UserID      string
	FileName    string
	ContentType string
	FileData    []byte
}

// StoredAvatar describes an avatar object after a successful upload.
type StoredAvatar struct {
	AvatarID    string
	ContentType string
	Size        int64
	S3Key       string
}

// IsValidAvatarID reports whether id is an opaque lowercase UUID (8-4-4-4-12).
// The serve route derives the S3 key only from a validated id, never from a
// caller-supplied URL, which is the path-traversal guard.
func IsValidAvatarID(id string) bool {
	return avatarIDPattern.MatchString(id)
}

// AvatarObjectKey returns the S3 key for an avatar id.
func AvatarObjectKey(id string) string {
	return AvatarS3Prefix + id
}

// AvatarIDFromURL extracts the avatar id embedded in a served avatar URL. It
// returns true only when the URL path is exactly /api/v1/avatars/<valid avatar
// id>, so callers such as the clear-avatar cleanup never act on a URL that is
// not one of this instance's avatar objects (a missing.png fallback, a media
// CDN URL, an arbitrary external profile image).
func AvatarIDFromURL(rawURL string) (string, bool) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", false
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", false
	}
	if !strings.HasPrefix(parsed.Path, avatarServePathPrefix) {
		return "", false
	}

	id := strings.TrimPrefix(parsed.Path, avatarServePathPrefix)
	if !IsValidAvatarID(id) {
		return "", false
	}
	return id, true
}

// StoreAvatar validates and stores avatar bytes under a freshly minted id.
func (s *Service) StoreAvatar(ctx context.Context, cmd *StoreAvatarCommand) (*StoredAvatar, error) {
	if s == nil || s.s3Service == nil || strings.TrimSpace(s.s3Bucket) == "" {
		return nil, ErrAvatarStoreUnavailable
	}
	if cmd == nil || len(cmd.FileData) == 0 {
		return nil, ErrAvatarRequired
	}
	if int64(len(cmd.FileData)) > AvatarMaxUploadBytes {
		return nil, ErrAvatarTooLarge
	}

	declared, ok := normalizeAvatarContentType(cmd.ContentType)
	if !ok {
		return nil, ErrAvatarContentTypeNotAllowed
	}
	sniffed, ok := normalizeAvatarContentType(http.DetectContentType(cmd.FileData))
	if !ok || sniffed != declared {
		return nil, ErrAvatarContentTypeNotAllowed
	}

	id := uuid.New().String()
	key := AvatarObjectKey(id)
	if _, err := s.s3Service.UploadFile(ctx, s.s3Bucket, key, cmd.FileData, sniffed); err != nil {
		return nil, errors.Join(ErrMediaStorageFailed, err)
	}

	s.logger.Info("stored avatar",
		zap.String("user_id", cmd.UserID),
		zap.String("avatar_id", id),
		zap.String("content_type", sniffed),
		zap.Int("size", len(cmd.FileData)))

	return &StoredAvatar{
		AvatarID:    id,
		ContentType: sniffed,
		Size:        int64(len(cmd.FileData)),
		S3Key:       key,
	}, nil
}

// GetAvatar returns the stored bytes and content type for an avatar id.
func (s *Service) GetAvatar(ctx context.Context, id string) ([]byte, string, error) {
	if !IsValidAvatarID(id) {
		return nil, "", ErrAvatarInvalidID
	}
	if s == nil || s.s3Service == nil || strings.TrimSpace(s.s3Bucket) == "" {
		return nil, "", ErrAvatarStoreUnavailable
	}
	reader, ok := s.s3Service.(S3ObjectReader)
	if !ok || reader == nil {
		return nil, "", ErrAvatarStoreUnavailable
	}

	data, storedType, err := reader.DownloadFile(ctx, s.s3Bucket, AvatarObjectKey(id), AvatarMaxServeBytes)
	if err != nil {
		if isAvatarObjectNotFound(err) {
			return nil, "", ErrAvatarNotFound
		}
		return nil, "", errors.Join(ErrMediaRetrievalFailed, err)
	}

	normalized, ok := normalizeAvatarContentType(storedType)
	if !ok {
		return nil, "", ErrAvatarContentTypeNotAllowed
	}
	return data, normalized, nil
}

// DeleteAvatar removes the stored object for an avatar id.
func (s *Service) DeleteAvatar(ctx context.Context, id string) error {
	if !IsValidAvatarID(id) {
		return ErrAvatarInvalidID
	}
	if s == nil || s.s3Service == nil || strings.TrimSpace(s.s3Bucket) == "" {
		return ErrAvatarStoreUnavailable
	}
	if err := s.s3Service.DeleteFile(ctx, s.s3Bucket, AvatarObjectKey(id)); err != nil {
		if isAvatarObjectNotFound(err) {
			return nil
		}
		return errors.Join(ErrMediaDeleteFailed, err)
	}
	return nil
}

func normalizeAvatarContentType(contentType string) (string, bool) {
	trimmed := strings.TrimSpace(contentType)
	if trimmed == "" {
		return "", false
	}
	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err != nil {
		return "", false
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == avatarSVGContentType {
		return "", false
	}
	if _, ok := avatarAllowedContentTypes[mediaType]; !ok {
		return "", false
	}
	return mediaType, true
}

func isAvatarObjectNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAvatarNotFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return true
		}
	}
	return false
}
