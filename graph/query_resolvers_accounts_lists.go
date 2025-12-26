package graph

import (
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storageTypes "github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// Markers returns timeline markers (last read positions) for the current viewer.
func (r *queryResolver) Markers(ctx context.Context, timelines []model.MarkerTimeline) (*model.MarkerSet, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	accountService := r.Registry.Accounts()
	if accountService == nil {
		return nil, errors.New("accounts service is not available")
	}

	// Convert GraphQL enum values to storage timeline keys.
	var timelineKeys []string
	if len(timelines) > 0 {
		timelineKeys = make([]string, 0, len(timelines))
		for _, timeline := range timelines {
			timelineKeys = append(timelineKeys, markerTimelineToKey(timeline))
		}
	}

	result, err := accountService.GetMarkers(ctx, &accounts.GetMarkersQuery{
		Username:  username,
		Timelines: timelineKeys,
	})
	if err != nil {
		r.Logger.Error("Failed to get markers",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get markers"), err)
	}

	markers := result.Markers
	if markers == nil {
		markers = map[string]*storageTypes.Marker{}
	}

	return &model.MarkerSet{
		Home:          convertStorageMarker(markers[TimelineTypeHome]),
		Notifications: convertStorageMarker(markers[ServiceTypeNotifications]),
	}, nil
}

// Favourites returns objects favorited (liked) by the current viewer.
func (r *queryResolver) Favourites(ctx context.Context, first *int, after *model.Cursor) (*model.ObjectConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	limit := 20
	if first != nil && *first > 0 {
		limit = *first
		if limit > 40 {
			limit = 40
		}
	}

	cursor := ""
	if after != nil {
		cursor = string(*after)
	}

	account, err := r.Registry.Accounts().GetAccount(ctx, username)
	if err != nil || account == nil || account.Actor == nil {
		r.Logger.Error("Failed to load viewer account for favourites",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get viewer account"), err)
	}

	repos := r.Registry.GetStorage()
	if repos == nil || repos.Like() == nil || repos.Status() == nil {
		return nil, errors.New("like/status repository is not available")
	}

	likes, nextCursor, err := repos.Like().GetActorLikes(ctx, account.Actor.ID, limit, cursor)
	if err != nil {
		r.Logger.Error("Failed to list favourites",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list favourites"), err)
	}

	type likeEdge struct {
		statusID string
		cursor   string
	}

	likeEdges := make([]likeEdge, 0, len(likes))
	statusIDs := make([]string, 0, len(likes))

	for _, like := range likes {
		if like == nil {
			continue
		}

		statusID := extractStatusIDFromObject(like.Object)
		if statusID == "" {
			continue
		}

		edgeCursor := like.GSI1SK
		if edgeCursor == "" {
			edgeCursor = fmt.Sprintf("%s#%s", like.CreatedAt.Format(time.RFC3339), like.Object)
		}

		likeEdges = append(likeEdges, likeEdge{
			statusID: statusID,
			cursor:   edgeCursor,
		})
		statusIDs = append(statusIDs, statusID)
	}

	statuses, err := repos.Status().GetStatusesByIDs(ctx, statusIDs)
	if err != nil {
		r.Logger.Error("Failed to load favourited statuses",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to load favourites"), err)
	}

	statusByID := make(map[string]*storageModels.Status, len(statuses))
	for _, status := range statuses {
		if status != nil {
			statusByID[status.StatusID] = status
		}
	}

	edges := make([]*model.ObjectEdge, 0, len(likeEdges))
	for _, edge := range likeEdges {
		status := statusByID[edge.statusID]
		if status == nil || status.Deleted {
			continue
		}
		edges = append(edges, &model.ObjectEdge{
			Node:   r.convertStatusToObject(ctx, status),
			Cursor: model.Cursor(edge.cursor),
		})
	}

	var startCursor, endCursor *model.Cursor
	if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		startCursor = &edges[0].Cursor
	}

	hasNextPage := nextCursor != ""
	if hasNextPage {
		cursorVal := model.Cursor(nextCursor)
		endCursor = &cursorVal
	} else if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.ObjectConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}, nil
}

// Bookmarks returns objects bookmarked by the current viewer.
func (r *queryResolver) Bookmarks(ctx context.Context, first *int, after *model.Cursor) (*model.ObjectConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	limit := 20
	if first != nil && *first > 0 {
		limit = *first
		if limit > 40 {
			limit = 40
		}
	}

	cursor := ""
	if after != nil {
		cursor = string(*after)
	}

	repos := r.Registry.GetStorage()
	if repos == nil || repos.Bookmark() == nil || repos.Status() == nil {
		return nil, errors.New("bookmark/status repository is not available")
	}

	bookmarks, nextCursor, err := repos.Bookmark().GetUserBookmarks(ctx, username, limit, cursor)
	if err != nil {
		r.Logger.Error("Failed to list bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list bookmarks"), err)
	}

	type bookmarkEdge struct {
		statusID string
		cursor   string
	}

	bookmarkEdges := make([]bookmarkEdge, 0, len(bookmarks))
	statusIDs := make([]string, 0, len(bookmarks))

	for _, bookmark := range bookmarks {
		if bookmark == nil {
			continue
		}

		statusID := extractStatusIDFromObject(bookmark.ObjectID)
		if statusID == "" {
			continue
		}

		bookmarkEdges = append(bookmarkEdges, bookmarkEdge{
			statusID: statusID,
			cursor:   bookmark.SK,
		})
		statusIDs = append(statusIDs, statusID)
	}

	statuses, err := repos.Status().GetStatusesByIDs(ctx, statusIDs)
	if err != nil {
		r.Logger.Error("Failed to load bookmarked statuses",
			zap.String("username", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to load bookmarks"), err)
	}

	statusByID := make(map[string]*storageModels.Status, len(statuses))
	for _, status := range statuses {
		if status != nil {
			statusByID[status.StatusID] = status
		}
	}

	edges := make([]*model.ObjectEdge, 0, len(bookmarkEdges))
	for _, edge := range bookmarkEdges {
		status := statusByID[edge.statusID]
		if status == nil || status.Deleted {
			continue
		}
		edges = append(edges, &model.ObjectEdge{
			Node:   r.convertStatusToObject(ctx, status),
			Cursor: model.Cursor(edge.cursor),
		})
	}

	var startCursor, endCursor *model.Cursor
	if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		startCursor = &edges[0].Cursor
	}

	hasNextPage := nextCursor != ""
	if hasNextPage {
		cursorVal := model.Cursor(nextCursor)
		endCursor = &cursorVal
	} else if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.ObjectConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}, nil
}

// CommunityNotesByAuthor returns community notes authored by the given user.
func (r *queryResolver) CommunityNotesByAuthor(ctx context.Context, username string, first *int, after *model.Cursor) (*model.CommunityNoteConnection, error) {
	resolvedUsername := deriveUsernameFromIRI(username)
	if err := common.ValidateRequiredParam("username", resolvedUsername); err != nil {
		return nil, err
	}

	limit := 20
	if first != nil && *first > 0 {
		limit = *first
		if limit > 100 {
			limit = 100
		}
	}

	cursor := ""
	if after != nil {
		cursor = string(*after)
	}

	authorID := config.Get().ActorURL(resolvedUsername)
	result, err := r.Registry.Notes().GetCommunityNotesByAuthor(ctx, &notes.GetCommunityNotesByAuthorQuery{
		AuthorID: authorID,
		Limit:    limit,
		Cursor:   cursor,
	})
	if err != nil {
		r.Logger.Error("Failed to list community notes by author",
			zap.String("username", resolvedUsername),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list community notes"), err)
	}

	authorActor := r.resolveActorByUsernameOrID(ctx, resolvedUsername, authorID)

	edges := make([]*model.CommunityNoteEdge, 0, len(result.Notes))
	for _, note := range result.Notes {
		if note == nil {
			continue
		}

		edgeCursor := fmt.Sprintf("%s#%s", note.CreatedAt.Format(time.RFC3339), note.ID)
		edges = append(edges, &model.CommunityNoteEdge{
			Node: &model.CommunityNote{
				ID:         note.ID,
				Author:     authorActor,
				Content:    note.Content,
				Helpful:    note.HelpfulVotes,
				NotHelpful: note.NotHelpfulVotes,
				CreatedAt:  model.Time(note.CreatedAt),
			},
			Cursor: model.Cursor(edgeCursor),
		})
	}

	var startCursor, endCursor *model.Cursor
	if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		startCursor = &edges[0].Cursor
	}

	hasNextPage := result.NextCursor != ""
	if hasNextPage {
		cursorVal := model.Cursor(result.NextCursor)
		endCursor = &cursorVal
	} else if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.CommunityNoteConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNextPage,
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}, nil
}

func (r *Resolver) resolveActorByUsernameOrID(ctx context.Context, username string, actorID string) *activitypub.Actor {
	if err := common.ValidateRequiredParam("username", username); err == nil {
		if account, accountErr := r.Registry.Accounts().GetAccount(ctx, username); accountErr == nil && account != nil {
			return r.convertAccountToActor(account)
		}
	}

	repos := r.Registry.GetStorage()
	if repos != nil && repos.Actor() != nil && actorID != "" {
		if actor, actorErr := repos.Actor().GetActor(ctx, actorID); actorErr == nil && actor != nil {
			return actor
		}
	}

	derived := deriveUsernameFromIRI(actorID)
	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   actorID,
			Type: activitypub.PersonType,
		},
		PreferredUsername: derived,
	}
}

func markerTimelineToKey(timeline model.MarkerTimeline) string {
	switch timeline {
	case model.MarkerTimelineHome:
		return TimelineTypeHome
	case model.MarkerTimelineNotifications:
		return ServiceTypeNotifications
	default:
		return strings.ToLower(timeline.String())
	}
}

func convertStorageMarker(marker *storageTypes.Marker) *model.Marker {
	if marker == nil {
		return nil
	}

	return &model.Marker{
		LastReadID: marker.LastReadID,
		UpdatedAt:  model.Time(marker.UpdatedAt),
		Version:    marker.Version,
	}
}

func extractStatusIDFromObject(objectID string) string {
	candidate := strings.TrimSpace(objectID)
	if candidate == "" {
		return ""
	}

	if idx := strings.Index(candidate, "?"); idx >= 0 {
		candidate = candidate[:idx]
	}
	candidate = strings.TrimSuffix(candidate, ".json")
	candidate = strings.TrimRight(candidate, "/")

	if strings.Contains(candidate, "/statuses/") {
		parts := strings.Split(candidate, "/statuses/")
		if len(parts) >= 2 {
			candidate = parts[len(parts)-1]
			candidate = strings.TrimRight(candidate, "/")
			if idx := strings.Index(candidate, "/"); idx >= 0 {
				candidate = candidate[:idx]
			}
		}
		return candidate
	}

	// If it's a URL, use the last path segment.
	if strings.Contains(candidate, "://") {
		if parsed, err := neturl.Parse(candidate); err == nil && parsed != nil {
			path := strings.Trim(parsed.Path, "/")
			if path == "" {
				return ""
			}
			if slash := strings.LastIndex(path, "/"); slash >= 0 {
				return strings.TrimSuffix(path[slash+1:], ".json")
			}
			return strings.TrimSuffix(path, ".json")
		}
	}

	return candidate
}
