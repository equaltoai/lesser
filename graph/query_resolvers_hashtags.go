package graph

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const (
	defaultHashtagTimelineLimit = 20
)

// Hashtag implements QueryResolver.
func (r *queryResolver) Hashtag(ctx context.Context, name string) (*model.Hashtag, error) {
	viewerID := r.optionalAuth(ctx)

	// Get hashtag service
	hashtagService := r.Registry.Hashtags()
	if hashtagService == nil {
		r.Logger.Error("hashtag service not available")
		return nil, fmt.Errorf("hashtag service not available")
	}

	// Call service to get hashtag
	hashtag, err := hashtagService.GetHashtag(ctx, name, viewerID)
	if err != nil {
		r.Logger.Error("failed to get hashtag",
			zap.String("name", name),
			zap.String("viewer", viewerID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get hashtag: %w", err)
	}

	// Convert to GraphQL model using the converter
	return r.convertHashtagToModel(ctx, hashtag, viewerID), nil
}

// FollowedHashtags implements QueryResolver.
func (r *queryResolver) FollowedHashtags(ctx context.Context, first *int, after *string) (*model.HashtagConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	limit := 20
	if first != nil && *first > 0 {
		limit = *first
	}

	var cursor string
	if after != nil {
		cursor = *after
	}

	// Get hashtag service
	hashtagService := r.Registry.Hashtags()
	if hashtagService == nil {
		r.Logger.Error("hashtag service not available")
		return nil, fmt.Errorf("hashtag service not available")
	}

	// Get followed hashtags from service
	hashtags, nextCursor, err := hashtagService.GetFollowedHashtags(ctx, username, &interfaces.PaginationOptions{
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		r.Logger.Error("failed to get followed hashtags",
			zap.String("user", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get followed hashtags: %w", err)
	}

	// Convert to edges
	edges := make([]*model.HashtagEdge, 0, len(hashtags))
	for _, hashtag := range hashtags {
		if hashtag == nil {
			continue
		}
		edges = append(edges, &model.HashtagEdge{
			Node:   r.convertHashtagToModel(ctx, hashtag, username),
			Cursor: model.Cursor(hashtag.Name), // Use hashtag name as cursor
		})
	}

	return &model.HashtagConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     nextCursor != "",
			HasPreviousPage: false,
		},
	}, nil
}

// HashtagTimeline implements QueryResolver.
func (r *queryResolver) HashtagTimeline(ctx context.Context, hashtag string, first *int, after *string, mediaOnly *bool) (*model.PostConnection, error) {
	viewerID := r.optionalAuth(ctx)

	limit, cursor := normalizeTimelineArgs(first, after, defaultHashtagTimelineLimit)

	posts, err := r.fetchHashtagTimelinePosts(ctx, hashtag, viewerID, limit, cursor)
	if err != nil {
		return nil, err
	}

	statusRepo := r.Storage.Status()
	if statusRepo == nil && mediaOnly != nil && *mediaOnly {
		r.Logger.Warn("status repository unavailable for media filter",
			zap.String("hashtag", hashtag))
	}

	statusByID := r.hydrateHashtagTimelineStatuses(ctx, statusRepo, posts, hashtag)
	requireMedia := mediaOnly != nil && *mediaOnly && statusRepo != nil
	edges := r.buildHashtagTimelineEdges(ctx, posts, statusByID, requireMedia)

	r.trackDynamoOperation(ctx, "read", int64(len(posts)))

	return buildPostConnection(edges, len(posts) == limit), nil
}

// MultiHashtagTimeline implements QueryResolver.
func (r *queryResolver) MultiHashtagTimeline(ctx context.Context, hashtags []string, mode model.HashtagMode, first *int, after *string) (*model.PostConnection, error) {
	viewerID := r.optionalAuth(ctx)

	if len(hashtags) == 0 {
		return &model.PostConnection{
			Edges: []*model.PostEdge{},
			PageInfo: &model.PageInfo{
				HasNextPage:     false,
				HasPreviousPage: false,
			},
		}, nil
	}

	limit := 20
	if first != nil && *first > 0 {
		limit = *first
	}

	var cursor string
	if after != nil {
		cursor = *after
	}

	hashtagRepo := r.Storage.Hashtag()
	if hashtagRepo == nil {
		r.Logger.Error("hashtag repository not available")
		return nil, fmt.Errorf("hashtag repository not available")
	}

	var maxID *string
	if cursor != "" {
		maxID = &cursor
	}

	// For multi-hashtag timelines, we'll query each hashtag and merge results
	// In a production system, this would be optimized with a proper multi-tag query
	allPosts := make(map[string]*model.Object)
	postMap := make(map[string][]string) // statusID -> hashtags

	for _, tag := range hashtags {
		posts, err := hashtagRepo.GetHashtagTimelineAdvanced(ctx, tag, maxID, limit, "public")
		if err != nil {
			r.Logger.Warn("failed to get timeline for hashtag in multi-query",
				zap.String("hashtag", tag),
				zap.Error(err))
			continue
		}

		for _, post := range posts {
			if post == nil {
				continue
			}
			if post.Visibility != "" && post.Visibility != models.VisibilityPublic {
				continue
			}

			if _, exists := allPosts[post.StatusID]; !exists {
				allPosts[post.StatusID] = &model.Object{
					ID:          post.StatusID,
					Content:     post.Content,
					ContentHash: contentHashForObjectID(post.StatusID),
				}
			}
			postMap[post.StatusID] = append(postMap[post.StatusID], tag)
		}
	}

	// Filter based on mode
	edges := make([]*model.PostEdge, 0, len(allPosts))
	for statusID, post := range allPosts {
		includePost := false
		if mode == model.HashtagModeAny {
			// ANY mode: post has at least one of the hashtags
			includePost = len(postMap[statusID]) > 0
		} else {
			// ALL mode: post has all the hashtags
			includePost = len(postMap[statusID]) >= len(hashtags)
		}

		if includePost {
			edges = append(edges, &model.PostEdge{
				Node:   post,
				Cursor: model.Cursor(statusID),
			})
		}
	}

	// Track cost
	r.trackDynamoOperation(ctx, "read", int64(len(edges)))

	r.Logger.Info("multi-hashtag timeline query",
		zap.String("viewer", viewerID),
		zap.Strings("hashtags", hashtags),
		zap.String("mode", string(mode)),
		zap.Int("result_count", len(edges)))

	hasNextPage := len(edges) >= limit

	return &model.PostConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: false,
		},
	}, nil
}

func normalizeTimelineArgs(first *int, after *string, defaultLimit int) (int, string) {
	limit := defaultLimit
	if first != nil && *first > 0 {
		limit = *first
	}

	cursor := ""
	if after != nil {
		cursor = *after
	}

	return limit, cursor
}

func (r *queryResolver) fetchHashtagTimelinePosts(ctx context.Context, hashtag string, viewerID string, limit int, cursor string) ([]*storage.StatusSearchResult, error) {
	hashtagRepo := r.Storage.Hashtag()
	if hashtagRepo == nil {
		r.Logger.Error("hashtag repository not available")
		return nil, fmt.Errorf("hashtag repository not available")
	}

	var maxID *string
	if cursor != "" {
		maxID = &cursor
	}

	posts, err := hashtagRepo.GetHashtagTimelineAdvanced(ctx, hashtag, maxID, limit, "public")
	if err != nil {
		r.Logger.Error("failed to get hashtag timeline",
			zap.String("hashtag", hashtag),
			zap.String("viewer", viewerID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get hashtag timeline: %w", err)
	}

	return posts, nil
}

func (r *queryResolver) hydrateHashtagTimelineStatuses(ctx context.Context, statusRepo interfaces.StatusRepository, posts []*storage.StatusSearchResult, hashtag string) map[string]*models.Status {
	if statusRepo == nil {
		return nil
	}

	statusIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		if post != nil && post.StatusID != "" {
			statusIDs = append(statusIDs, post.StatusID)
		}
	}

	if len(statusIDs) == 0 {
		return nil
	}

	statuses, err := statusRepo.GetStatusesByIDs(ctx, statusIDs)
	if err != nil {
		r.Logger.Warn("failed to hydrate hashtag timeline statuses",
			zap.String("hashtag", hashtag),
			zap.Int("requested", len(statusIDs)),
			zap.Error(err))
		return nil
	}

	statusByID := make(map[string]*models.Status, len(statuses))
	for _, status := range statuses {
		if status != nil && status.StatusID != "" {
			statusByID[status.StatusID] = status
		}
	}

	return statusByID
}

func (r *queryResolver) buildHashtagTimelineEdges(ctx context.Context, posts []*storage.StatusSearchResult, statusByID map[string]*models.Status, requireMedia bool) []*model.PostEdge {
	edges := make([]*model.PostEdge, 0, len(posts))
	for _, post := range posts {
		if post == nil {
			continue
		}

		var status *models.Status
		if statusByID != nil {
			status = statusByID[post.StatusID]
		}
		if status != nil && status.Visibility != "" && status.Visibility != models.VisibilityPublic {
			continue
		}
		if status == nil && post.Visibility != "" && post.Visibility != models.VisibilityPublic {
			continue
		}

		if requireMedia && (status == nil || !status.HasMedia()) {
			continue
		}

		cursorValue := post.StatusID
		if post.ID != "" {
			cursorValue = post.ID
		}

		postModel := r.buildTimelineObject(ctx, status, post)

		edges = append(edges, &model.PostEdge{
			Node:   postModel,
			Cursor: model.Cursor(cursorValue),
		})
	}

	return edges
}

func (r *queryResolver) buildTimelineObject(ctx context.Context, status *models.Status, post *storage.StatusSearchResult) *model.Object {
	if status != nil {
		return r.convertStatusToObject(ctx, status)
	}

	content := ""
	statusID := ""
	if post != nil {
		content = post.Content
		statusID = post.StatusID
	}

	return &model.Object{
		ID:          statusID,
		Content:     content,
		ContentHash: contentHashForObjectID(statusID),
	}
}

func buildPostConnection(edges []*model.PostEdge, hasNextPage bool) *model.PostConnection {
	startCursor, endCursor := deriveEdgeCursors(edges)

	return &model.PostConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: false,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}
}

func deriveEdgeCursors(edges []*model.PostEdge) (*model.Cursor, *model.Cursor) {
	if len(edges) == 0 {
		return nil, nil
	}

	start := edges[0].Cursor
	end := edges[len(edges)-1].Cursor
	return &start, &end
}

// SuggestedHashtags implements QueryResolver.
func (r *queryResolver) SuggestedHashtags(ctx context.Context, limit *int) ([]*model.HashtagSuggestion, error) {
	viewerID := r.optionalAuth(ctx)

	count := 10
	if limit != nil && *limit > 0 {
		count = *limit
	}

	// For now, return trending hashtags from the repository
	// In a full implementation, this would use a dedicated suggestions service
	hashtagRepo := r.Storage.Hashtag()
	if hashtagRepo == nil {
		r.Logger.Error("hashtag repository not available")
		return []*model.HashtagSuggestion{}, nil
	}

	// Get trending hashtags (using GetTrendingHashtags if available)
	// For now, we'll return an empty list as a stub
	// A full implementation would query trending data
	suggestions := make([]*model.HashtagSuggestion, 0)

	r.Logger.Info("suggested hashtags query",
		zap.String("viewer", viewerID),
		zap.Int("limit", count),
		zap.Int("result_count", len(suggestions)))

	return suggestions, nil
}
