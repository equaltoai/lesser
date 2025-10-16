package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
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

	// Get streaming URL from media service
	stream, err := mediaSvc.GetStreamingURL(ctx, mediaID)
	if err != nil {
		r.Logger.Error("failed to get streaming URL",
			zap.String("user", username),
			zap.String("media_id", mediaID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get streaming URL"), err)
	}

	// Filter bitrates by requested quality if specified
	if quality != nil && stream.Bitrates != nil {
		var filteredBitrates []*model.Bitrate
		for _, bitrate := range stream.Bitrates {
			if bitrate.Quality == *quality {
				filteredBitrates = append(filteredBitrates, bitrate)
				break // Return only the matching quality
			}
		}
		if err := common.ValidateSliceNotEmpty("filtered_bitrates", filteredBitrates); err == nil {
			stream.Bitrates = filteredBitrates
		}
	}

	r.Logger.Info("streaming URL requested",
		zap.String("user", username),
		zap.String("media_id", mediaID))

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

	// Preload media for faster streaming
	var streams []*model.MediaStream
	var preloadErrors []error

	for _, mediaID := range mediaIDs {
		// Get streaming URL for each media
		stream, err := mediaSvc.GetStreamingURL(ctx, mediaID)
		if err != nil {
			r.Logger.Warn("failed to preload media",
				zap.String("user", username),
				zap.String("media_id", mediaID),
				zap.Error(err))
			preloadErrors = append(preloadErrors, err)
			// Continue processing other media items
			continue
		}
		streams = append(streams, stream)
	}

	// If all preloads failed, return error
	streamsEmpty := common.ValidateSliceNotEmpty("streams", streams) != nil
	errorsNotEmpty := common.ValidateSliceNotEmpty("errors", preloadErrors) == nil
	if streamsEmpty && errorsNotEmpty {
		return nil, errors.Join(errors.New("failed to preload any media"), preloadErrors[0])
	}

	r.Logger.Info("media preloaded",
		zap.String("user", username),
		zap.Int("requested", len(mediaIDs)),
		zap.Int("loaded", len(streams)),
		zap.Int("failed", len(preloadErrors)))

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
