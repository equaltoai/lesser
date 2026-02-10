package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/graph-gophers/dataloader"
	"go.uber.org/zap"
)

type objectResolver struct{ *Resolver }

func (r *objectResolver) ViewerFavourited(ctx context.Context, obj *model.Object) (bool, error) {
	viewerUsername := getUsernameFromContext(ctx)
	if viewerUsername == "" || obj == nil || strings.TrimSpace(obj.ID) == "" {
		return false, nil
	}

	actorID := r.viewerActorID(viewerUsername)
	objectID := r.objectIDForGraphQLObject(obj)
	if actorID == "" || objectID == "" {
		return false, nil
	}

	if loaders := GetLoaders(ctx); loaders != nil && loaders.ViewerFavouritedLoader != nil {
		key := dataloader.StringKey(actorID + "\n" + objectID)
		thunk := loaders.ViewerFavouritedLoader.Load(ctx, key)
		value, err := thunk()
		if err == nil {
			if liked, ok := value.(bool); ok {
				return liked, nil
			}
		} else if r.Logger != nil {
			r.Logger.Debug("viewer favourited loader failed", zap.Error(err))
		}
	}

	if r.Storage == nil || r.Storage.Like() == nil {
		return false, nil
	}

	liked, err := r.Storage.Like().HasLiked(ctx, actorID, objectID)
	if err != nil && r.Logger != nil {
		r.Logger.Warn("failed to resolve viewer favourited state",
			zap.String("viewer", viewerUsername),
			zap.String("actor_id", actorID),
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	return err == nil && liked, nil
}

func (r *objectResolver) ViewerBookmarked(ctx context.Context, obj *model.Object) (bool, error) {
	viewerUsername := getUsernameFromContext(ctx)
	statusID := strings.TrimSpace("")
	if obj != nil {
		statusID = strings.TrimSpace(obj.ID)
	}
	if viewerUsername == "" || statusID == "" {
		return false, nil
	}

	if loaders := GetLoaders(ctx); loaders != nil && loaders.ViewerBookmarkedLoader != nil {
		thunk := loaders.ViewerBookmarkedLoader.Load(ctx, dataloader.StringKey(statusID))
		value, err := thunk()
		if err == nil {
			if bookmarked, ok := value.(bool); ok {
				return bookmarked, nil
			}
		} else if r.Logger != nil {
			r.Logger.Debug("viewer bookmarked loader failed", zap.Error(err))
		}
	}

	if r.Storage == nil || r.Storage.Bookmark() == nil {
		return false, nil
	}

	bookmarked, err := r.Storage.Bookmark().CheckBookmarksForStatuses(ctx, viewerUsername, []string{statusID})
	if err != nil && r.Logger != nil {
		r.Logger.Warn("failed to resolve viewer bookmarked state",
			zap.String("viewer", viewerUsername),
			zap.String("status_id", statusID),
			zap.Error(err))
		return false, nil
	}

	return bookmarked[statusID], nil
}

func (r *objectResolver) ViewerPinned(ctx context.Context, obj *model.Object) (bool, error) {
	viewerUsername := getUsernameFromContext(ctx)
	statusID := strings.TrimSpace("")
	if obj != nil {
		statusID = strings.TrimSpace(obj.ID)
	}
	if viewerUsername == "" || statusID == "" {
		return false, nil
	}

	if loaders := GetLoaders(ctx); loaders != nil && loaders.ViewerPinnedLoader != nil {
		thunk := loaders.ViewerPinnedLoader.Load(ctx, dataloader.StringKey(statusID))
		value, err := thunk()
		if err == nil {
			if pinned, ok := value.(bool); ok {
				return pinned, nil
			}
		} else if r.Logger != nil {
			r.Logger.Debug("viewer pinned loader failed", zap.Error(err))
		}
	}

	if r.Storage == nil || r.Storage.Social() == nil {
		return false, nil
	}

	pinned, err := r.Storage.Social().CheckPinnedStatuses(ctx, viewerUsername, []string{statusID})
	if err != nil && r.Logger != nil {
		r.Logger.Warn("failed to resolve viewer pinned state",
			zap.String("viewer", viewerUsername),
			zap.String("status_id", statusID),
			zap.Error(err))
		return false, nil
	}

	return pinned[statusID], nil
}

func (r *objectResolver) viewerActorID(viewerUsername string) string {
	viewerUsername = strings.TrimSpace(viewerUsername)
	if viewerUsername == "" {
		return ""
	}
	if strings.Contains(viewerUsername, "://") {
		return viewerUsername
	}
	if r == nil || r.Config == nil || strings.TrimSpace(r.Config.Domain) == "" {
		return viewerUsername
	}
	return fmt.Sprintf("https://%s/users/%s", strings.TrimSpace(r.Config.Domain), viewerUsername)
}

func (r *objectResolver) objectIDForGraphQLObject(obj *model.Object) string {
	if obj == nil {
		return ""
	}
	rawID := strings.TrimSpace(obj.ID)
	if rawID == "" {
		return ""
	}
	if strings.Contains(rawID, "://") {
		return rawID
	}

	authorUsername := ""
	authorID := ""
	if obj.Actor != nil {
		authorUsername = strings.TrimSpace(obj.Actor.PreferredUsername)
		authorID = strings.TrimSpace(obj.Actor.ID)
	}
	if authorUsername == "" && authorID != "" {
		authorUsername = extractUsernameFromActorIdentifier(authorID)
	}
	if authorUsername == "" {
		return ""
	}

	domain := ""
	if authorID != "" {
		domain = actorDomainFromID(authorID, "")
	}
	if domain == "" && r != nil && r.Config != nil {
		domain = strings.TrimSpace(r.Config.Domain)
	}
	if domain == "" {
		return ""
	}

	return fmt.Sprintf("https://%s/users/%s/statuses/%s", domain, authorUsername, rawID)
}
