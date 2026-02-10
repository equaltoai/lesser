package graph

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	mediaStreaming "github.com/equaltoai/lesser/pkg/media/streaming"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/storage/models"
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

// MediaLibrary is the resolver for the mediaLibrary field.
func (r *queryResolver) MediaLibrary(ctx context.Context, filter *model.MediaFilterInput, first *int, after *model.Cursor) (*model.MediaConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	args, err := r.buildMediaLibraryArgs(ctx, username, filter, first, after)
	if err != nil {
		return nil, err
	}

	mediaService := r.Registry.Media()
	if mediaService == nil {
		return nil, ErrMediaServiceUnavailable
	}

	result, err := mediaService.ListMedia(ctx, args.query)
	if err != nil {
		r.Logger.Error("Failed to list media",
			zap.String("owner", args.query.Owner),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list media"), err)
	}

	edges := r.buildMediaEdges(result.Items)
	startCursor, endCursor := computeMediaCursors(edges)
	hasNext := result.HasMore || result.NextCursor != ""
	totalCount := computeMediaTotalCount(result, edges)

	r.trackDynamoOperation(ctx, DynamoOperationQuery, int64(len(result.Items)))

	return &model.MediaConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNext,
			HasPreviousPage: args.hasPrevious,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: totalCount,
	}, nil
}

type mediaLibraryArgs struct {
	query       *media.ListMediaQuery
	hasPrevious bool
}

func (r *queryResolver) buildMediaLibraryArgs(ctx context.Context, username string, filter *model.MediaFilterInput, first *int, after *model.Cursor) (*mediaLibraryArgs, error) {
	limit := 20
	if first != nil {
		limit = *first
	}
	if limit <= 0 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}

	owner := strings.TrimSpace(username)
	mediaType := ""
	mimeType := ""
	var sincePtr, untilPtr *time.Time

	if filter != nil {
		if filter.OwnerID != nil && strings.TrimSpace(*filter.OwnerID) != "" {
			owner = strings.TrimSpace(*filter.OwnerID)
		}
		if filter.OwnerUsername != nil && strings.TrimSpace(*filter.OwnerUsername) != "" {
			owner = strings.TrimSpace(*filter.OwnerUsername)
		}
		if filter.MediaType != nil {
			mediaType = strings.ToLower(string(*filter.MediaType))
		}
		if filter.MimeType != nil {
			mimeType = strings.TrimSpace(*filter.MimeType)
		}
		if filter.Since != nil {
			s := time.Time(*filter.Since)
			sincePtr = &s
		}
		if filter.Until != nil {
			u := time.Time(*filter.Until)
			untilPtr = &u
		}
	}

	if owner == "" {
		return nil, common.ErrValidation("owner", "owner is required").InternalError
	}

	if !strings.EqualFold(owner, username) && !r.isAdmin(ctx, username) {
		return nil, ErrAdminPrivilegesRequired
	}

	cursor := ""
	hasPrev := false
	if after != nil && *after != "" {
		cursor = string(*after)
		hasPrev = true
	}

	return &mediaLibraryArgs{
		query: &media.ListMediaQuery{
			Owner:     owner,
			Requester: username,
			MediaType: mediaType,
			MimeType:  mimeType,
			Cursor:    cursor,
			Limit:     limit,
			Since:     sincePtr,
			Until:     untilPtr,
		},
		hasPrevious: hasPrev,
	}, nil
}

func (r *queryResolver) buildMediaEdges(items []*models.Media) []*model.MediaEdge {
	edges := make([]*model.MediaEdge, 0, len(items))
	for _, item := range items {
		node := r.convertMediaToGraphQL(item)
		if node == nil {
			continue
		}

		cursorValue := item.GSI1SK
		if cursorValue == "" {
			cursorValue = item.MediaID
		}

		cursor := model.Cursor(cursorValue)
		edges = append(edges, &model.MediaEdge{
			Node:   node,
			Cursor: cursor,
		})
	}
	return edges
}

func computeMediaCursors(edges []*model.MediaEdge) (*model.Cursor, *model.Cursor) {
	if len(edges) == 0 {
		return nil, nil
	}

	start := edges[0].Cursor
	end := edges[len(edges)-1].Cursor
	return &start, &end
}

func computeMediaTotalCount(result *media.ListMediaResult, edges []*model.MediaEdge) int {
	if result.Total < 0 {
		return len(edges)
	}
	return int(result.Total)
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
func (r *queryResolver) SupportedBitrates(ctx context.Context, mediaID string) ([]*model.Bitrate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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
