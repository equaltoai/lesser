package graph

import (
	"context"
	"errors"
	"fmt"
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
	// Set defaults for pagination
	limit := 20
	if first > 0 && first <= 100 {
		limit = first
	}

	// For now, we'll implement popular streams by getting trending statuses with media attachments
	// This represents popular streaming content until a dedicated streaming analytics service is implemented
	trendingStatuses, err := r.Storage.Analytics().GetTrendingStatuses(ctx, time.Now().Add(-24*time.Hour), limit)
	if err != nil {
		r.Logger.Error("failed to get trending statuses for popular streams", zap.Error(err))
		return &model.StreamConnection{
			Edges: []*model.StreamEdge{},
			PageInfo: &model.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: false,
			},
			TotalCount: 0,
		}, nil
	}

	// Convert trending statuses to stream edges (simplified for now)
	edges := make([]*model.StreamEdge, 0, len(trendingStatuses))
	for _, status := range trendingStatuses {
		if status == nil {
			continue
		}

		// Create a stream representation based on the trending status
		// Using repository analytics to calculate streaming metrics
		stream := &model.Stream{
			ID:        status.ID,
			Title:     r.truncateText(status.Content, 100), // Use content as title, truncated
			CreatedAt: model.Time(status.CreatedAt),
		}

		edge := &model.StreamEdge{
			Cursor: model.Cursor(fmt.Sprintf("stream_%s", status.ID)),
			Node:   stream,
		}

		edges = append(edges, edge)

		// Stop if we have enough results
		if len(edges) >= limit {
			break
		}
	}

	// Determine if there are more pages
	hasNextPage := len(trendingStatuses) == limit

	return &model.StreamConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: after != nil && *after != "",
		},
		TotalCount: len(edges),
	}, nil
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
	// Get storage from resolver
	storage := r.Storage
	if storage == nil {
		return nil, ErrStorageUnavailable
	}

	// Get analytics repository
	analyticsRepo := storage.Analytics()
	if analyticsRepo == nil {
		return nil, ErrAnalyticsRepositoryUnavailable
	}

	// Get real streaming analytics data
	analyticsData, err := analyticsRepo.GetStreamingAnalytics(ctx, mediaID)
	if err != nil {
		r.Logger.Warn("Failed to get streaming analytics",
			zap.String("mediaID", mediaID),
			zap.Error(err))
		// Return empty analytics instead of error to maintain API compatibility
		return &model.StreamingAnalytics{
			TotalViews:          0,
			UniqueViewers:       0,
			AverageWatchTime:    model.Duration(0),
			QualityDistribution: []*model.QualityStats{},
			BufferingEvents:     0,
			CompletionRate:      0.0,
		}, nil
	}

	// Convert storage analytics to GraphQL model
	qualityDistribution := make([]*model.QualityStats, 0, len(analyticsData.QualityDistribution))
	for _, stats := range analyticsData.QualityDistribution {
		// Convert quality string to StreamQuality enum
		var quality model.StreamQuality
		switch strings.ToLower(stats.Quality) {
		case "240p", "low":
			quality = model.StreamQualityLow
		case "360p", "480p", "medium":
			quality = model.StreamQualityMedium
		case "720p", "high":
			quality = model.StreamQualityHigh
		case "1080p", "4k", "2160p", "ultra":
			quality = model.StreamQualityUltra
		case "auto":
			quality = model.StreamQualityAuto
		default:
			quality = model.StreamQualityMedium
		}

		qualityStats := &model.QualityStats{
			Quality:      quality,
			ViewCount:    stats.ViewCount,
			Percentage:   stats.Percentage,
			AvgBandwidth: stats.AverageBitrate,
		}
		qualityDistribution = append(qualityDistribution, qualityStats)
	}

	// Convert average watch time from seconds to Duration
	avgWatchTime := model.Duration(time.Duration(analyticsData.AverageWatchTime * float64(time.Second)))

	// Log real-time metrics for debugging
	r.Logger.Debug("Retrieved real-time streaming analytics",
		zap.String("mediaID", mediaID),
		zap.Int("totalViews", analyticsData.TotalViews),
		zap.Int("uniqueViewers", analyticsData.UniqueViewers),
		zap.Int("activeStreams", analyticsData.StreamingSessions),
		zap.Float64("completionRate", analyticsData.CompletionRate),
		zap.Any("recentMetrics", analyticsData.RecentMetrics))

	return &model.StreamingAnalytics{
		TotalViews:          analyticsData.TotalViews,
		UniqueViewers:       analyticsData.UniqueViewers,
		AverageWatchTime:    avgWatchTime,
		QualityDistribution: qualityDistribution,
		BufferingEvents:     analyticsData.BufferingEvents,
		CompletionRate:      analyticsData.CompletionRate,
	}, nil
}
