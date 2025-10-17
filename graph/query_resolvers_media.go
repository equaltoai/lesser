package graph

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	mediaStreaming "github.com/equaltoai/lesser/pkg/media/streaming"
	"github.com/equaltoai/lesser/pkg/services/media"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// Media is the resolver for the media field.
func (r *queryResolver) Media(ctx context.Context, id string) (*model.Media, error) {
	username := r.optionalAuth(ctx)

	result, err := r.Registry.Media().GetMedia(ctx, &media.GetMediaQuery{
		MediaID:  id,
		ViewerID: username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		r.Logger.Error("Failed to get media",
			zap.String("id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get media"), err)
	}

	return r.convertMediaToGraphQL(result), nil
}

// MediaStreamURL implements QueryResolver.
func (r *queryResolver) MediaStreamURL(ctx context.Context, mediaID string) (*model.MediaStream, error) {
	// Use media service to get streaming URL
	mediaService := r.Registry.Media()
	if mediaService == nil {
		return nil, ErrMediaServiceUnavailable
	}

	// Get media renditions (HLS/DASH manifests)
	renditions, err := mediaService.GetMediaRenditions(ctx, mediaID)
	if err != nil {
		r.Logger.Error("Failed to get media renditions",
			zap.String("media_id", mediaID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get media renditions"), err)
	}

	// Build MediaStream from renditions
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

	// Set thumbnail URL
	if len(renditions.ThumbnailURLs) > 0 {
		stream.ThumbnailURL = renditions.ThumbnailURLs[0]
	}

	// Convert rendition variants to bitrates
	bitrates := make([]*model.Bitrate, 0, len(renditions.Variants))
	for _, variant := range renditions.Variants {
		quality := mapQualityToEnum(variant.Quality)
		bitrate := &model.Bitrate{
			Quality:       quality,
			BitsPerSecond: variant.Bitrate,
			Width:         variant.Width,
			Height:        variant.Height,
			Codec:         variant.Codec,
		}
		bitrates = append(bitrates, bitrate)
	}
	stream.Bitrates = bitrates

	// Set duration (get from media record)
	media, err := r.Storage.Media().GetMedia(ctx, mediaID)
	if err == nil && media != nil {
		stream.Duration = media.Duration
	}

	return stream, nil
}

// PopularStreams returns popular streaming endpoints
func (r *queryResolver) PopularStreams(ctx context.Context, first int, after *string) (*model.StreamConnection, error) {
	// Get streaming analytics service
	service := r.Registry.StreamingAnalytics()
	if service == nil {
		r.Logger.Warn("streaming analytics service not available")
		// Return empty connection for graceful degradation
		return &model.StreamConnection{
			Edges: []*model.StreamEdge{},
			PageInfo: &model.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: false,
				StartCursor:     nil,
				EndCursor:       nil,
			},
			TotalCount: 0,
		}, nil
	}

	// Get popular streams from service
	connection, err := service.GetPopularStreams(ctx, first, after)
	if err != nil {
		r.Logger.Error("failed to get popular streams",
			zap.Int("first", first),
			zap.Error(err))
		return nil, err
	}

	return connection, nil
}

// SupportedBitrates returns supported bitrates for a media item
func (r *queryResolver) SupportedBitrates(_ context.Context, mediaID string) ([]*model.Bitrate, error) {
	// Import the streaming package's quality information
	qualityLevels := []mediaStreaming.Quality{
		mediaStreaming.Quality4K,
		mediaStreaming.Quality1080p,
		mediaStreaming.Quality720p,
		mediaStreaming.Quality480p,
		mediaStreaming.Quality360p,
		mediaStreaming.Quality240p,
	}

	bitrates := make([]*model.Bitrate, 0, len(qualityLevels))

	for _, quality := range qualityLevels {
		qualityInfo := mediaStreaming.GetQualityInfo(quality)

		bitrate := &model.Bitrate{
			Quality:       model.StreamQuality(qualityInfo.Quality),
			BitsPerSecond: qualityInfo.Bitrate * 1000, // Convert kbps to bps
			Width:         qualityInfo.Width,
			Height:        qualityInfo.Height,
			Codec:         "h264", // Default codec
		}

		bitrates = append(bitrates, bitrate)
	}

	r.Logger.Debug("returned supported bitrates",
		zap.String("media_id", mediaID),
		zap.Int("bitrate_count", len(bitrates)))

	return bitrates, nil
}

// StreamingAnalytics returns streaming analytics for a media item
func (r *queryResolver) StreamingAnalytics(ctx context.Context, mediaID string) (*model.StreamingAnalytics, error) {
	// Get streaming analytics service
	service := r.Registry.StreamingAnalytics()
	if service == nil {
		r.Logger.Warn("streaming analytics service not available")
		// Return empty analytics for graceful degradation
		return &model.StreamingAnalytics{
			TotalViews:          0,
			UniqueViewers:       0,
			AverageWatchTime:    model.Duration(0),
			QualityDistribution: []*model.QualityStats{},
			BufferingEvents:     0,
			CompletionRate:      0.0,
		}, nil
	}

	// Get analytics data from service
	analytics, err := service.GetStreamingAnalytics(ctx, mediaID)
	if err != nil {
		r.Logger.Error("failed to get streaming analytics",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		return nil, err
	}

	return analytics, nil
}
