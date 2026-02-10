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
	var loader *dataloader.Loader
	if loaders := GetLoaders(ctx); loaders != nil {
		loader = loaders.ViewerBookmarkedLoader
	}
	return r.resolveViewerStatusFlag(ctx, obj, loader, "viewer bookmarked loader failed", r.viewerBookmarkedFallback, "failed to resolve viewer bookmarked state")
}

func (r *objectResolver) ViewerPinned(ctx context.Context, obj *model.Object) (bool, error) {
	var loader *dataloader.Loader
	if loaders := GetLoaders(ctx); loaders != nil {
		loader = loaders.ViewerPinnedLoader
	}
	return r.resolveViewerStatusFlag(ctx, obj, loader, "viewer pinned loader failed", r.viewerPinnedFallback, "failed to resolve viewer pinned state")
}

type viewerStatusFlagFallback func(ctx context.Context, viewerUsername, statusID string) (bool, error)

func (r *objectResolver) resolveViewerStatusFlag(ctx context.Context, obj *model.Object, loader *dataloader.Loader, loaderFailMsg string, fallback viewerStatusFlagFallback, warnMsg string) (bool, error) {
	viewerUsername := getUsernameFromContext(ctx)
	statusID := ""
	if obj != nil {
		statusID = strings.TrimSpace(obj.ID)
	}
	if viewerUsername == "" || statusID == "" {
		return false, nil
	}

	if loader != nil {
		thunk := loader.Load(ctx, dataloader.StringKey(statusID))
		value, err := thunk()
		if err == nil {
			if flag, ok := value.(bool); ok {
				return flag, nil
			}
		} else if r.Logger != nil {
			r.Logger.Debug(loaderFailMsg, zap.Error(err))
		}
	}

	if fallback == nil {
		return false, nil
	}

	flag, err := fallback(ctx, viewerUsername, statusID)
	if err != nil {
		if r.Logger != nil {
			r.Logger.Warn(warnMsg,
				zap.String("viewer", viewerUsername),
				zap.String("status_id", statusID),
				zap.Error(err))
		}
		return false, nil
	}

	return flag, nil
}

func (r *objectResolver) viewerBookmarkedFallback(ctx context.Context, viewerUsername, statusID string) (bool, error) {
	if r.Storage == nil || r.Storage.Bookmark() == nil {
		return false, nil
	}

	bookmarked, err := r.Storage.Bookmark().CheckBookmarksForStatuses(ctx, viewerUsername, []string{statusID})
	if err != nil {
		return false, err
	}

	return bookmarked[statusID], nil
}

func (r *objectResolver) viewerPinnedFallback(ctx context.Context, viewerUsername, statusID string) (bool, error) {
	if r.Storage == nil || r.Storage.Social() == nil {
		return false, nil
	}

	return r.Storage.Social().IsStatusPinned(ctx, viewerUsername, statusID)
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
