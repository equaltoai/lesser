package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// Object is the resolver for the object field.
func (r *queryResolver) Object(ctx context.Context, id string) (*model.Object, error) {
	// Get object using notes service
	note, err := r.Registry.Notes().GetNote(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		r.Logger.Error("Failed to get object",
			zap.String("id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get object"), err)
	}

	return r.convertStatusToObject(ctx, note), nil
}

// Timeline is the resolver for the timeline field.
func (r *queryResolver) Timeline(ctx context.Context, timelineType model.TimelineType, hashtag *string, listID *string, actorID *string, first *int, after *model.Cursor, mediaOnly *bool) (*model.ObjectConnection, error) {
	username := r.optionalAuth(ctx)

	// Build pagination
	pagination := interfaces.PaginationOptions{
		Limit: 20,
	}

	if first != nil && *first > 0 && *first <= 100 {
		pagination.Limit = *first
	}

	if after != nil {
		pagination.Cursor = string(*after)
	}

	// Build query based on timeline type
	query := &notes.ListNotesQuery{
		ViewerID:   username,
		Pagination: pagination,
	}

	if mediaOnly != nil && *mediaOnly {
		query.OnlyMedia = true
	}

	// Set timeline filter
	switch timelineType {
	case model.TimelineTypeHome:
		if err := common.ValidateRequiredParam("username", username); err != nil {
			return nil, ErrAuthRequiredForHome
		}
		query.TimelineType = TimelineTypeHome
	case model.TimelineTypePublic:
		query.TimelineType = StreamNamePublic
	case model.TimelineTypeLocal:
		query.TimelineType = "local"
	case model.TimelineTypeHashtag:
		if hashtag == nil {
			return nil, ErrHashtagParameterRequired
		}
		if err := common.ValidateRequiredParam("hashtag", *hashtag); err != nil {
			return nil, ErrHashtagParameterRequired
		}
		query.TimelineType = TimelineTypeHashtag
		query.Hashtag = *hashtag
	case model.TimelineTypeList:
		if listID == nil {
			return nil, ErrListIDParameterRequired
		}
		if err := common.ValidateRequiredParam("listID", *listID); err != nil {
			return nil, ErrListIDParameterRequired
		}
		query.TimelineType = TimelineTypeList
		// List timeline is handled differently - need to get list members
		// and fetch their posts
	case model.TimelineTypeDirect:
		if err := common.ValidateRequiredParam("username", username); err != nil {
			return nil, ErrAuthRequiredForDirect
		}
		query.TimelineType = TimelineTypeDirect
	case model.TimelineTypeActor:
		if actorID == nil {
			return nil, ErrActorIDParameterRequired
		}
		if err := common.ValidateRequiredParam("actorID", *actorID); err != nil {
			return nil, ErrActorIDParameterRequired
		}
		query.TimelineType = "user" // Map to internal "user" timeline
		query.AuthorID = *actorID
	default:
		return nil, ErrUnsupportedTimelineTypeWithValue(timelineType)
	}

	// Get timeline using service
	result, err := r.Registry.Notes().ListNotes(ctx, query)
	if err != nil {
		r.Logger.Error("Failed to get timeline",
			zap.String("type", string(timelineType)),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get timeline"), err)
	}

	// Convert to GraphQL connection
	edges := make([]*model.ObjectEdge, len(result.Notes))
	for i, note := range result.Notes {
		edges[i] = &model.ObjectEdge{
			Node:   r.convertStatusToObject(ctx, note),
			Cursor: model.Cursor(note.StatusID),
		}
	}

	var startCursor, endCursor *model.Cursor
	if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.ObjectConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     result.Pagination != nil && result.Pagination.HasMore,
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}, nil
}

// Search is the resolver for the search field.
func (r *queryResolver) Search(ctx context.Context, query string, searchType *string, first *int, after *model.Cursor) (*model.SearchResult, error) {

	searchQuery := &search.Query{
		Query: query,
		Type:  "all",
		Limit: 20,
	}

	if searchType != nil {
		searchQuery.Type = *searchType
	}

	if first != nil && *first > 0 && *first <= 100 {
		searchQuery.Limit = *first
	}

	if after != nil {
		// Convert cursor to offset
		searchQuery.Offset = 20
	}

	result, err := r.Registry.Search().Search(ctx, searchQuery)
	if err != nil {
		r.Logger.Error("Failed to search",
			zap.String("query", query),
			zap.Error(err))
		return nil, errors.Join(errors.New("search failed"), err)
	}

	accounts := make([]*activitypub.Actor, len(result.Accounts))
	for i, account := range result.Accounts {
		accounts[i] = account.Actor
	}

	statuses := make([]*model.Object, len(result.Statuses))
	for i, statusResult := range result.Statuses {
		// Status is an interface{}, need to type assert
		if s, ok := statusResult.Status.(*models.Status); ok {
			statuses[i] = r.convertStatusToObject(ctx, s)
		} else if n, ok := statusResult.Status.(*activitypub.Note); ok {
			statuses[i] = r.convertNoteToObject(ctx, n)
		}
	}

	hashtags := make([]*activitypub.Tag, len(result.Hashtags))
	for i, tag := range result.Hashtags {
		hashtags[i] = &activitypub.Tag{
			Type: "Hashtag",
			Name: tag.Name,
			Href: tag.URL,
		}
	}

	return &model.SearchResult{
		Accounts: accounts,
		Statuses: statuses,
		Hashtags: hashtags,
	}, nil
}

// ThreadContext returns the context of a thread
func (r *queryResolver) ThreadContext(ctx context.Context, noteID string) (*model.ThreadContext, error) {
	storage := r.Storage
	if storage == nil {
		return nil, ErrStorageUnavailable
	}

	statusRepo := storage.Status()
	if statusRepo == nil {
		return nil, ErrStatusRepositoryUnavailable
	}

	status, err := statusRepo.GetStatus(ctx, noteID)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get status"), err)
	}

	replies, err := r.getThreadReplies(ctx, statusRepo, noteID)
	if err != nil {
		return nil, err
	}

	engagement, err := r.calculateEngagementMetrics(ctx, storage, noteID)
	if err != nil {
		return nil, err
	}

	threadMetrics := r.calculateThreadMetrics(ctx, statusRepo, status, replies)
	rootNote := r.createRootNoteObject(ctx, status, replies, engagement)
	syncStatus := r.determineSyncStatus(replies)

	return &model.ThreadContext{
		RootNote:         rootNote,
		ReplyCount:       len(replies),
		ParticipantCount: len(threadMetrics.participants),
		LastActivity:     model.Time(threadMetrics.lastActivity),
		MissingPosts:     threadMetrics.missingPosts,
		SyncStatus:       syncStatus,
	}, nil
}
