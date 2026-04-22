package graph

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// Object is the resolver for the object field.
func (r *queryResolver) Object(ctx context.Context, id string) (*model.Object, error) {
	viewerUsername := r.optionalAuth(ctx)
	note, err := r.loadVisibleNoteForViewer(ctx, id, viewerUsername)
	if err != nil {
		r.Logger.Error("Failed to get object",
			zap.String("id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get object"), err)
	}
	if note == nil {
		return nil, nil
	}

	return r.convertStatusToObject(ctx, note), nil
}

func (r *queryResolver) loadVisibleNoteForViewer(ctx context.Context, statusID, viewerID string) (*models.Status, error) {
	notesSvc := r.notesService()
	if notesSvc == nil {
		return nil, errors.New("notes service is not available")
	}

	status, err := notesSvc.GetNoteWithViewer(ctx, &notes.GetNoteQuery{
		StatusID: statusID,
		ViewerID: viewerID,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, nil
		}
		return nil, err
	}

	return status, nil
}

func timelinePaginationOptions(first *int, after *model.Cursor) interfaces.PaginationOptions {
	pagination := interfaces.PaginationOptions{Limit: 20}
	if first != nil && *first > 0 && *first <= 100 {
		pagination.Limit = *first
	}
	if after != nil {
		pagination.Cursor = string(*after)
	}
	return pagination
}

func applyTimelineTypeFilter(username string, timelineType model.TimelineType, hashtag *string, listID *string, actorID *string, query *notes.ListNotesQuery) error {
	switch timelineType {
	case model.TimelineTypeHome:
		if err := common.ValidateRequiredParam("username", username); err != nil {
			return ErrAuthRequiredForHome
		}
		query.TimelineType = TimelineTypeHome
	case model.TimelineTypePublic:
		query.TimelineType = StreamNamePublic
	case model.TimelineTypeLocal:
		query.TimelineType = common.TimelineLocal
	case model.TimelineTypeHashtag:
		if hashtag == nil {
			return ErrHashtagParameterRequired
		}
		if err := common.ValidateRequiredParam("hashtag", *hashtag); err != nil {
			return ErrHashtagParameterRequired
		}
		query.TimelineType = TimelineTypeHashtag
		query.Hashtag = *hashtag
	case model.TimelineTypeList:
		if listID == nil {
			return ErrListIDParameterRequired
		}
		if err := common.ValidateRequiredParam("listID", *listID); err != nil {
			return ErrListIDParameterRequired
		}
		query.TimelineType = TimelineTypeList
		// List timeline is handled differently - need to get list members
		// and fetch their posts
	case model.TimelineTypeDirect:
		if err := common.ValidateRequiredParam("username", username); err != nil {
			return ErrAuthRequiredForDirect
		}
		query.TimelineType = TimelineTypeDirect
	case model.TimelineTypeActor:
		if actorID == nil {
			return ErrActorIDParameterRequired
		}
		if err := common.ValidateRequiredParam("actorID", *actorID); err != nil {
			return ErrActorIDParameterRequired
		}
		query.TimelineType = TimelineTypeUser // Map to internal user timeline
		query.AuthorID = *actorID
	default:
		return ErrUnsupportedTimelineTypeWithValue(timelineType)
	}

	return nil
}

func isAgentObject(obj *model.Object) bool {
	if obj == nil || obj.Actor == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(obj.Actor.Type), activitypub.ServiceType)
}

func (r *queryResolver) timelineObjectEdges(ctx context.Context, notesIn []*models.Status, excludeAgents bool) []*model.ObjectEdge {
	edges := make([]*model.ObjectEdge, 0, len(notesIn))
	for _, note := range notesIn {
		obj := r.convertStatusToObject(ctx, note)
		if excludeAgents && isAgentObject(obj) {
			continue
		}

		edges = append(edges, &model.ObjectEdge{
			Node:   obj,
			Cursor: model.Cursor(note.StatusID),
		})
	}
	return edges
}

// Timeline is the resolver for the timeline field.
func (r *queryResolver) Timeline(ctx context.Context, timelineType model.TimelineType, hashtag *string, listID *string, actorID *string, first *int, after *model.Cursor, mediaOnly *bool, excludeAgents *bool) (*model.ObjectConnection, error) {
	username := r.optionalAuth(ctx)
	shouldExcludeAgents := excludeAgents != nil && *excludeAgents

	pagination := timelinePaginationOptions(first, after)

	// Build query based on timeline type
	query := &notes.ListNotesQuery{
		ViewerID:   username,
		Pagination: pagination,
	}

	if mediaOnly != nil && *mediaOnly {
		query.OnlyMedia = true
	}

	if err := applyTimelineTypeFilter(username, timelineType, hashtag, listID, actorID, query); err != nil {
		return nil, err
	}

	// Get timeline using service
	result, err := r.Registry.Notes().ListNotes(ctx, query)
	if err != nil {
		r.Logger.Error("Failed to get timeline",
			zap.String("type", string(timelineType)),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get timeline"), err)
	}

	edges := r.timelineObjectEdges(ctx, result.Notes, shouldExcludeAgents)

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
	viewerUsername := r.optionalAuth(ctx)
	normalizedSearchType := normalizeGraphQLSearchType(searchType)

	searchQuery := &search.Query{
		Query:     query,
		AccountID: viewerUsername,
		Type:      normalizedSearchType,
		Limit:     20,
	}

	if first != nil && *first > 0 && *first <= 100 {
		searchQuery.Limit = *first
	}

	if after != nil {
		// Convert cursor to offset
		searchQuery.Offset = 20
	}

	if exact, handled, err := r.searchExactActorQuery(ctx, query, normalizedSearchType); handled {
		return exact, err
	}

	result, err := r.Registry.Search().Search(ctx, searchQuery)
	if err != nil {
		r.Logger.Error("Failed to search",
			zap.String("query", query),
			zap.Error(err))
		return nil, errors.Join(errors.New("search failed"), err)
	}

	return r.searchResultToGraphQL(ctx, result, viewerUsername), nil
}

func normalizeGraphQLSearchType(searchType *string) string {
	if searchType == nil {
		return QueryTypeAll
	}

	return canonicalGraphQLSearchType(*searchType)
}

func canonicalGraphQLSearchType(searchType string) string {
	switch strings.ToLower(strings.TrimSpace(searchType)) {
	case "", QueryTypeAll:
		return QueryTypeAll
	case "account", QueryTypeAccounts:
		return QueryTypeAccounts
	case "status", "statuses":
		return "statuses"
	case "hashtag", "hashtags":
		return "hashtags"
	default:
		return strings.ToLower(strings.TrimSpace(searchType))
	}
}

func (r *queryResolver) searchExactActorQuery(ctx context.Context, query string, searchType string) (*model.SearchResult, bool, error) {
	if !supportsExactActorGraphQLSearch(searchType) || !looksLikeExactActorQuery(query) {
		return nil, false, nil
	}

	resolution, err := r.resolveExactActorLookup(ctx, query)
	if err != nil {
		if graphActorLookupNotFound(err) {
			return emptyGraphSearchResult(), true, nil
		}

		r.Logger.Error("Failed exact actor search",
			zap.String("query", query),
			zap.Error(err))
		return nil, true, errors.Join(errors.New("search failed"), err)
	}

	actor := r.materializeActorResolution(ctx, resolution)
	if actor == nil {
		return emptyGraphSearchResult(), true, nil
	}

	return &model.SearchResult{
		Accounts: []*activitypub.Actor{actor},
		Statuses: []*model.Object{},
		Hashtags: []*activitypub.Tag{},
	}, true, nil
}

func supportsExactActorGraphQLSearch(searchType string) bool {
	switch strings.ToLower(strings.TrimSpace(searchType)) {
	case "", QueryTypeAll, QueryTypeAccounts:
		return true
	default:
		return false
	}
}

func looksLikeExactActorQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "http://") || strings.HasPrefix(strings.ToLower(trimmed), "https://") {
		return true
	}
	return strings.Count(strings.TrimPrefix(trimmed, "@"), "@") == 1
}

func emptyGraphSearchResult() *model.SearchResult {
	return &model.SearchResult{
		Accounts: []*activitypub.Actor{},
		Statuses: []*model.Object{},
		Hashtags: []*activitypub.Tag{},
	}
}

func (r *queryResolver) searchResultToGraphQL(ctx context.Context, result *search.Result, viewerUsername string) *model.SearchResult {
	accounts := make([]*activitypub.Actor, 0)
	statuses := make([]*model.Object, 0)
	hashtags := make([]*activitypub.Tag, 0)

	if result == nil {
		return &model.SearchResult{
			Accounts: accounts,
			Statuses: statuses,
			Hashtags: hashtags,
		}
	}

	accounts = make([]*activitypub.Actor, 0, len(result.Accounts))
	for _, account := range result.Accounts {
		actor := account.Actor
		if actor != nil {
			identity := federation.DescribeActorIdentity(actor, r.localActorDomain())
			if !identity.IsRemote && identity.Username != "" {
				actor = r.materializeActorResolution(ctx, &federation.ExactActorResolution{
					Actor:         actor,
					ActorIdentity: identity,
				})
			}
		}
		if actor != nil {
			accounts = append(accounts, actor)
		}
	}

	statuses = make([]*model.Object, 0, len(result.Statuses))
	for _, statusResult := range result.Statuses {
		obj := r.convertSearchStatusToObject(ctx, viewerUsername, statusResult.Status)
		if obj != nil {
			statuses = append(statuses, obj)
		}
	}

	hashtags = make([]*activitypub.Tag, 0, len(result.Hashtags))
	for _, tag := range result.Hashtags {
		hashtags = append(hashtags, &activitypub.Tag{
			Type: "Hashtag",
			Name: tag.Name,
			Href: tag.URL,
		})
	}

	return &model.SearchResult{
		Accounts: accounts,
		Statuses: statuses,
		Hashtags: hashtags,
	}
}

func (r *queryResolver) convertSearchStatusToObject(ctx context.Context, viewerUsername string, status any) *model.Object {
	switch v := status.(type) {
	case *models.Status:
		visible, err := r.loadVisibleNoteForViewer(ctx, v.StatusID, viewerUsername)
		if err != nil || visible == nil {
			return nil
		}
		return r.convertStatusToObject(ctx, visible)
	case *activitypub.Note:
		return r.convertNoteToObject(ctx, v)
	case *storage.StatusSearchResult:
		return r.loadStatusSearchResult(ctx, viewerUsername, v)
	case storage.StatusSearchResult:
		return r.loadStatusSearchResult(ctx, viewerUsername, &v)
	case map[string]any:
		statusID := ""
		if vID, ok := v["status_id"].(string); ok {
			statusID = strings.TrimSpace(vID)
		}
		if statusID == "" {
			if vID, ok := v["id"].(string); ok {
				statusID = strings.TrimSpace(vID)
			}
		}
		if statusID == "" {
			return nil
		}
		notesSvc := r.notesService()
		if notesSvc == nil {
			return nil
		}
		full, err := notesSvc.GetNoteWithViewer(ctx, &notes.GetNoteQuery{
			StatusID: statusID,
			ViewerID: viewerUsername,
		})
		if err != nil || full == nil {
			return nil
		}
		return r.convertStatusToObject(ctx, full)
	default:
		return nil
	}
}

func (r *queryResolver) loadStatusSearchResult(ctx context.Context, viewerUsername string, result *storage.StatusSearchResult) *model.Object {
	if result == nil {
		return nil
	}

	statusID := strings.TrimSpace(result.StatusID)
	if statusID == "" {
		statusID = strings.TrimSpace(result.ID)
	}
	if statusID == "" && strings.TrimSpace(result.URL) != "" {
		statusID = statusIDFromURL(result.URL)
	}
	if statusID == "" {
		return nil
	}

	notesSvc := r.notesService()
	if notesSvc == nil {
		return nil
	}

	full, err := notesSvc.GetNoteWithViewer(ctx, &notes.GetNoteQuery{
		StatusID: statusID,
		ViewerID: viewerUsername,
	})
	if err != nil || full == nil {
		return nil
	}
	return r.convertStatusToObject(ctx, full)
}

func statusIDFromURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Path == "" {
		return ""
	}

	path := strings.TrimSuffix(u.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	statusID := parts[len(parts)-1]
	return strings.TrimSpace(statusID)
}

// ThreadContext returns the context of a thread
func (r *queryResolver) ThreadContext(ctx context.Context, noteID string) (*model.ThreadContext, error) {
	viewerUsername := r.optionalAuth(ctx)
	storage := r.Storage
	if storage == nil {
		return nil, ErrStorageUnavailable
	}

	statusRepo := storage.Status()
	if statusRepo == nil {
		return nil, ErrStatusRepositoryUnavailable
	}

	status, err := r.loadVisibleNoteForViewer(ctx, noteID, viewerUsername)
	if err != nil {
		return nil, errors.Join(errors.New("failed to get status"), err)
	}
	if status == nil {
		return nil, nil
	}

	replies, err := r.getThreadReplies(ctx, statusRepo, noteID, viewerUsername)
	if err != nil {
		return nil, err
	}

	ancestors := r.buildThreadAncestors(ctx, statusRepo, status, viewerUsername)

	engagement, err := r.calculateEngagementMetrics(ctx, storage, noteID)
	if err != nil {
		return nil, err
	}

	threadMetrics := r.calculateThreadMetrics(ctx, statusRepo, status, replies, viewerUsername)
	rootNote := r.createRootNoteObject(ctx, status, replies, engagement)
	syncStatus := r.determineSyncStatus(replies)

	descendants := r.convertStatusesToObjects(ctx, replies)

	return &model.ThreadContext{
		RootNote:         rootNote,
		Ancestors:        ancestors,
		Descendants:      descendants,
		ReplyCount:       len(replies),
		ParticipantCount: len(threadMetrics.participants),
		LastActivity:     model.Time(threadMetrics.lastActivity),
		MissingPosts:     threadMetrics.missingPosts,
		SyncStatus:       syncStatus,
	}, nil
}

func (r *queryResolver) buildThreadAncestors(ctx context.Context, statusRepo interfaces.StatusRepository, status *models.Status, viewerUsername string) []*model.Object {
	ancestors := make([]*model.Object, 0)
	if status == nil || statusRepo == nil {
		return ancestors
	}

	if strings.TrimSpace(status.InReplyToID) == "" {
		return ancestors
	}

	const maxAncestors = 50
	visited := make(map[string]struct{}, maxAncestors+1)
	if status.StatusID != "" {
		visited[status.StatusID] = struct{}{}
	}

	current := status
	ancestorStatuses := make([]*models.Status, 0, 4)
	for len(ancestorStatuses) < maxAncestors {
		parentID := strings.TrimSpace(current.InReplyToID)
		if parentID == "" {
			break
		}
		if _, ok := visited[parentID]; ok {
			break
		}
		visited[parentID] = struct{}{}

		parent, err := r.loadVisibleNoteForViewer(ctx, parentID, viewerUsername)
		if err != nil || parent == nil || parent.Deleted {
			break
		}

		ancestorStatuses = append(ancestorStatuses, parent)
		current = parent
	}

	for i := len(ancestorStatuses) - 1; i >= 0; i-- {
		if obj := r.convertStatusToObject(ctx, ancestorStatuses[i]); obj != nil {
			ancestors = append(ancestors, obj)
		}
	}

	return ancestors
}

func (r *queryResolver) convertStatusesToObjects(ctx context.Context, statuses []*models.Status) []*model.Object {
	objects := make([]*model.Object, 0, len(statuses))
	for _, status := range statuses {
		if status == nil {
			continue
		}
		if obj := r.convertStatusToObject(ctx, status); obj != nil {
			objects = append(objects, obj)
		}
	}
	return objects
}
