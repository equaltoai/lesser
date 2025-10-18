package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/media"
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

	cmd := &media.UpdateMediaCommand{
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
	session, err := mediaSvc.GenerateSignedStreamURL(ctx, mediaID, qualityStr)
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
	successfulIDs, err := mediaSvc.PreloadMedia(ctx, mediaIDs)
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
