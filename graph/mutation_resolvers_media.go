package graph

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	mediatools "github.com/equaltoai/lesser/pkg/media"
	mediasvc "github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// UpdateMedia is the resolver for the updateMedia field.
func (r *mutationResolver) UpdateMedia(ctx context.Context, id string, input model.UpdateMediaInput) (*model.Media, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	cmd := &mediasvc.UpdateMediaCommand{
		UserID:  username,
		MediaID: id,
	}

	if input.Description != nil {
		cmd.Description = *input.Description
	}

	if input.Focus != nil {
		// Combine X and Y coordinates into a single focus string
		cmd.Focus = fmt.Sprintf("%.2f,%.2f", input.Focus.X, input.Focus.Y)
	}

	result, err := r.Registry.Media().UpdateMedia(ctx, cmd)
	if err != nil {
		r.Logger.Error("Failed to update media",
			zap.String("user", username),
			zap.String("media", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to update media"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)
	return r.convertMediaToGraphQL(result.Media), nil
}

// UploadMedia is the resolver for the uploadMedia field.
func (r *mutationResolver) UploadMedia(ctx context.Context, input model.UploadMediaInput) (*model.UploadMediaPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	maxUploadSize := r.getMaxUploadSize()
	fileBytes, err := r.readUploadFile(input.File, maxUploadSize)
	if err != nil {
		r.Logger.Error("failed to process upload stream",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	contentType := detectUploadContentType(input.File, fileBytes)
	filename, err := normalizeUploadFilename(input.File, input.Filename, contentType)
	if err != nil {
		return nil, err
	}
	description, err := validateUploadDescription(input.Description)
	if err != nil {
		return nil, err
	}
	focus, err := normalizeUploadFocus(input.Focus)
	if err != nil {
		return nil, err
	}
	sensitive := false
	if input.Sensitive != nil {
		sensitive = *input.Sensitive
	}
	spoilerText, err := validateUploadSpoilerText(input.SpoilerText)
	if err != nil {
		return nil, err
	}
	mediaCategory, err := normalizeUploadMediaCategory(input.MediaType, contentType)
	if err != nil {
		return nil, err
	}

	mediaService := r.Registry.Media()
	if mediaService == nil {
		return nil, ErrMediaServiceUnavailable
	}

	result, err := mediaService.UploadMedia(ctx, &mediasvc.UploadMediaCommand{
		UserID:        username,
		FileName:      filename,
		ContentType:   contentType,
		FileData:      fileBytes,
		Description:   description,
		Focus:         focus,
		Sensitive:     sensitive,
		SpoilerText:   spoilerText,
		MediaCategory: mediaCategory,
	})
	if err != nil {
		r.Logger.Error("failed to upload media via GraphQL",
			zap.String("user", username),
			zap.String("filename", filename),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to upload media"), err)
	}

	if result == nil || result.Media == nil {
		return nil, errors.New("failed to upload media: empty result")
	}

	r.trackDynamoOperation(ctx, "write", 1)
	r.trackS3Operation(ctx, "put", 1)

	graphQLMedia := r.convertMediaToGraphQL(result.Media)
	if graphQLMedia == nil {
		return nil, errors.New("failed to convert media result")
	}

	r.Logger.Info("media uploaded via GraphQL",
		zap.String("user", username),
		zap.String("media_id", result.Media.MediaID),
		zap.String("filename", filename),
		zap.String("content_type", contentType),
		zap.Int("file_size", len(fileBytes)))

	payload := &model.UploadMediaPayload{
		Media:    graphQLMedia,
		UploadID: result.Media.MediaID,
	}

	return payload, nil
}

func (r *mutationResolver) getMaxUploadSize() int64 {
	const defaultMaxUploadSize int64 = 10 * 1024 * 1024 // 10MB

	if r.Config != nil && r.Config.MaxUploadSize > 0 {
		return r.Config.MaxUploadSize
	}

	if registryConfig := r.Registry.GetConfig(); registryConfig != nil && registryConfig.Config != nil && registryConfig.Config.MaxUploadSize > 0 {
		return registryConfig.Config.MaxUploadSize
	}

	return defaultMaxUploadSize
}

func (r *mutationResolver) readUploadFile(upload graphql.Upload, maxSize int64) ([]byte, error) {
	if upload.File == nil {
		return nil, common.ErrValidation("file", "media file is required").InternalError
	}

	reader := io.LimitReader(upload.File, maxSize+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Join(errors.New("failed to read upload"), err)
	}

	if int64(len(data)) > maxSize {
		return nil, common.ErrValidation("file", fmt.Sprintf("file exceeds maximum size of %d bytes", maxSize)).InternalError
	}

	if len(data) == 0 {
		return nil, common.ErrValidation("file", "uploaded file is empty").InternalError
	}

	if closer, ok := upload.File.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			r.Logger.Debug("failed to close upload stream", zap.Error(err))
		}
	}

	return data, nil
}

func normalizeUploadFilename(upload graphql.Upload, override *string, contentType string) (string, error) {
	var name string
	if override != nil && strings.TrimSpace(*override) != "" {
		name = strings.TrimSpace(*override)
	} else if strings.TrimSpace(upload.Filename) != "" {
		name = strings.TrimSpace(upload.Filename)
	} else {
		name = fmt.Sprintf("upload-%d", time.Now().Unix())
	}

	return ensureFilenameExtension(name, contentType)
}

func detectUploadContentType(upload graphql.Upload, data []byte) string {
	if contentType := strings.TrimSpace(upload.ContentType); contentType != "" {
		return contentType
	}

	sniffLength := len(data)
	if sniffLength > 512 {
		sniffLength = 512
	}

	contentType := http.DetectContentType(data[:sniffLength])
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return contentType
}

func validateUploadDescription(description *string) (string, error) {
	if description == nil {
		return "", nil
	}

	trimmed := strings.TrimSpace(*description)
	if trimmed == "" {
		return "", nil
	}

	if err := common.ValidateMediaDescription(trimmed); err != nil {
		return "", common.ErrValidation("description", err.Error()).InternalError
	}

	return trimmed, nil
}

func normalizeUploadFocus(focusInput *model.FocusInput) (string, error) {
	if focusInput == nil {
		return "", nil
	}

	focus := fmt.Sprintf("%.2f,%.2f", focusInput.X, focusInput.Y)
	if err := common.ValidateMediaFocus(focus); err != nil {
		return "", common.ErrValidation("focus", err.Error()).InternalError
	}

	return focus, nil
}

func validateUploadSpoilerText(spoiler *string) (string, error) {
	if spoiler == nil {
		return "", nil
	}

	trimmed := strings.TrimSpace(*spoiler)
	if trimmed == "" {
		return "", nil
	}

	if err := common.ValidateSpoilerText(trimmed); err != nil {
		return "", common.ErrValidation("spoilerText", err.Error()).InternalError
	}

	return trimmed, nil
}

func normalizeUploadMediaCategory(category *model.MediaCategory, contentType string) (models.MediaCategory, error) {
	if category == nil {
		return models.DetermineMediaCategory(contentType), nil
	}

	value := strings.TrimSpace(string(*category))
	if value == "" {
		return models.DetermineMediaCategory(contentType), nil
	}

	if normalized, ok := models.NormalizeMediaCategory(value); ok {
		return normalized, nil
	}

	return "", common.ErrValidation("mediaType", fmt.Sprintf("unsupported media category '%s'", value)).InternalError
}

func ensureFilenameExtension(filename, contentType string) (string, error) {
	trimmed := strings.TrimSpace(filename)
	if trimmed == "" {
		return "", common.ErrValidation("filename", "filename cannot be blank").InternalError
	}

	filenameWithExt, err := mediatools.EnsureFilenameHasExtension(trimmed, contentType)
	if err != nil {
		return "", common.ErrValidation("filename", err.Error()).InternalError
	}

	return filenameWithExt, nil
}

// RequestStreamingURL implements MutationResolver
func (r *mutationResolver) RequestStreamingURL(ctx context.Context, mediaID string, quality *model.StreamQuality) (*model.MediaStream, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get media service from registry
	mediaSvc := r.Registry.Media()
	if mediaSvc == nil {
		return nil, ErrMediaServiceUnavailable
	}

	// Convert quality enum to string if provided
	var qualityStr *string
	if quality != nil {
		str := strings.ToLower(string(*quality))
		// Map enum values to quality strings
		switch *quality {
		case model.StreamQualityLow:
			str = "480p"
		case model.StreamQualityMedium:
			str = "720p"
		case model.StreamQualityHigh:
			str = "1080p"
		case model.StreamQualityUltra:
			str = "2160p"
		case model.StreamQualityAuto:
			qualityStr = nil // Auto-select quality
		}
		if qualityStr == nil && *quality != model.StreamQualityAuto {
			qualityStr = &str
		}
	}

	// Generate signed streaming URL
	session, err := mediaSvc.GenerateSignedStreamURL(ctx, mediaID, username, qualityStr)
	if err != nil {
		r.Logger.Error("failed to generate signed streaming URL",
			zap.String("user", username),
			zap.String("media_id", mediaID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to generate signed streaming URL"), err)
	}

	// Get media renditions for additional metadata
	renditions, err := mediaSvc.GetMediaRenditions(ctx, mediaID)
	if err != nil {
		r.Logger.Warn("failed to get media renditions",
			zap.String("media_id", mediaID),
			zap.Error(err))
		// Continue with session info only
	}

	// Build MediaStream response
	stream := &model.MediaStream{
		ID:        mediaID,
		URL:       session.URL,
		ExpiresAt: model.Time(session.ExpiresAt),
	}

	// Add thumbnail if available
	if renditions != nil && len(renditions.ThumbnailURLs) > 0 {
		stream.ThumbnailURL = renditions.ThumbnailURLs[0]
	}

	// Add bitrates
	if renditions != nil {
		bitrates := make([]*model.Bitrate, 0, len(renditions.Variants))
		for _, variant := range renditions.Variants {
			qualityEnum := mapQualityToEnum(variant.Quality)
			// Filter by requested quality if specified
			if quality != nil && qualityEnum != *quality {
				continue
			}
			bitrate := &model.Bitrate{
				Quality:       qualityEnum,
				BitsPerSecond: variant.Bitrate,
				Width:         variant.Width,
				Height:        variant.Height,
				Codec:         variant.Codec,
			}
			bitrates = append(bitrates, bitrate)
		}
		stream.Bitrates = bitrates
	}

	// Get duration from media record
	media, err := r.Storage.Media().GetMedia(ctx, mediaID)
	if err == nil && media != nil {
		stream.Duration = media.Duration
	}

	r.Logger.Info("streaming URL requested",
		zap.String("user", username),
		zap.String("media_id", mediaID),
		zap.Any("quality", quality))

	return stream, nil
}

// PreloadMedia implements MutationResolver
func (r *mutationResolver) PreloadMedia(ctx context.Context, mediaIDs []string) ([]*model.MediaStream, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get media service from registry
	mediaSvc := r.Registry.Media()
	if mediaSvc == nil {
		return nil, ErrMediaServiceUnavailable
	}

	// Preload media manifests and prime CDN cache
	successfulIDs, err := mediaSvc.PreloadMedia(ctx, username, mediaIDs)
	if err != nil {
		r.Logger.Error("failed to preload media",
			zap.String("user", username),
			zap.Int("requested", len(mediaIDs)),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to preload media"), err)
	}

	// Build response streams for successfully preloaded media
	var streams []*model.MediaStream
	for _, mediaID := range successfulIDs {
		// Get renditions for each preloaded media
		renditions, err := mediaSvc.GetMediaRenditions(ctx, mediaID)
		if err != nil {
			r.Logger.Warn("failed to get renditions for preloaded media",
				zap.String("media_id", mediaID),
				zap.Error(err))
			continue
		}

		stream := &model.MediaStream{
			ID:        mediaID,
			URL:       renditions.HLSMasterURL,
			ExpiresAt: model.Time(time.Now().Add(24 * time.Hour)),
		}

		// Set HLS and DASH URLs
		if renditions.HLSMasterURL != "" {
			stream.HlsPlaylistURL = &renditions.HLSMasterURL
		}
		if renditions.DASHManifestURL != "" {
			stream.DashManifestURL = &renditions.DASHManifestURL
		}

		// Set thumbnail
		if len(renditions.ThumbnailURLs) > 0 {
			stream.ThumbnailURL = renditions.ThumbnailURLs[0]
		}

		// Convert variants to bitrates
		bitrates := make([]*model.Bitrate, 0, len(renditions.Variants))
		for _, variant := range renditions.Variants {
			bitrate := &model.Bitrate{
				Quality:       mapQualityToEnum(variant.Quality),
				BitsPerSecond: variant.Bitrate,
				Width:         variant.Width,
				Height:        variant.Height,
				Codec:         variant.Codec,
			}
			bitrates = append(bitrates, bitrate)
		}
		stream.Bitrates = bitrates

		// Get duration
		media, err := r.Storage.Media().GetMedia(ctx, mediaID)
		if err == nil && media != nil {
			stream.Duration = media.Duration
		}

		streams = append(streams, stream)
	}

	r.Logger.Info("media preloaded",
		zap.String("user", username),
		zap.Int("requested", len(mediaIDs)),
		zap.Int("loaded", len(streams)))

	return streams, nil
}

// ReportStreamingQuality implements MutationResolver
func (r *mutationResolver) ReportStreamingQuality(ctx context.Context, input model.StreamingQualityInput) (*model.StreamingQualityReport, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get media service to record quality metrics
	mediaSvc := r.Registry.Media()
	if mediaSvc == nil {
		return nil, ErrMediaServiceUnavailable
	}

	// Record the quality report (would be stored in metrics repository)
	reportID := fmt.Sprintf("sqr-%s-%s", input.MediaID, generateID()[:8])

	// Log the quality report for monitoring
	r.Logger.Info("streaming quality reported",
		zap.String("user", username),
		zap.String("media_id", input.MediaID),
		zap.String("quality", string(input.Quality)),
		zap.Int("buffering_events", input.BufferingEvents),
		zap.Int("watch_time", input.WatchTime))

	return &model.StreamingQualityReport{
		Success:  true,
		MediaID:  input.MediaID,
		Quality:  input.Quality,
		ReportID: reportID,
	}, nil
}
