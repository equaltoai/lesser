package graph

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/transformations"
	"go.uber.org/zap"
)

func (r *Resolver) resolveActorByID(ctx context.Context, actorID string) *activitypub.Actor {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil
	}

	localDomain := ""
	if r.Config != nil {
		localDomain = r.Config.Domain
	}

	store := r.getStorageForResolution()
	if actor := r.resolveActorFromStorage(ctx, store, actorID, localDomain); actor != nil {
		return actor
	}

	return r.buildPlaceholderActor(actorID, localDomain)
}

func (r *Resolver) getStorageForResolution() core.RepositoryStorage {
	if r.Storage != nil {
		return r.Storage
	}
	if r.Registry != nil {
		return r.Registry.GetStorage()
	}
	return nil
}

func (r *Resolver) resolveActorFromStorage(ctx context.Context, store core.RepositoryStorage, actorID, localDomain string) *activitypub.Actor {
	if store == nil || store.Actor() == nil {
		return nil
	}

	if strings.Contains(actorID, "://") {
		return resolveActorFromURL(ctx, store, actorID, localDomain)
	}

	return resolveActorFromHandleOrUsername(ctx, store, actorID, localDomain)
}

func resolveActorFromURL(ctx context.Context, store core.RepositoryStorage, actorID, localDomain string) *activitypub.Actor {
	parsedURL, err := neturl.Parse(actorID)
	if err != nil || parsedURL.Host == "" {
		return nil
	}

	username := strings.TrimPrefix(transformations.ExtractUsernameFromActorID(actorID), "@")
	if username == "" {
		return nil
	}

	if localDomain != "" && strings.EqualFold(parsedURL.Host, localDomain) {
		actor, err := store.Actor().GetActor(ctx, username)
		if err == nil && actor != nil {
			return actor
		}
		return nil
	}

	handle := fmt.Sprintf("%s@%s", username, parsedURL.Host)
	actor, err := store.Actor().GetCachedRemoteActor(ctx, handle)
	if err == nil && actor != nil {
		return actor
	}

	return nil
}

func resolveActorFromHandleOrUsername(ctx context.Context, store core.RepositoryStorage, actorID, localDomain string) *activitypub.Actor {
	value := strings.TrimPrefix(actorID, "@")
	if value == "" {
		return nil
	}

	if localUsername, ok := localUsernameFromHandle(value, localDomain); ok {
		actor, err := store.Actor().GetActor(ctx, localUsername)
		if err == nil && actor != nil {
			return actor
		}
		return nil
	}

	if strings.Contains(value, "@") {
		actor, err := store.Actor().GetCachedRemoteActor(ctx, value)
		if err == nil && actor != nil {
			return actor
		}
		return nil
	}

	actor, err := store.Actor().GetActor(ctx, value)
	if err == nil && actor != nil {
		return actor
	}
	return nil
}

func localUsernameFromHandle(handle, localDomain string) (string, bool) {
	if localDomain == "" {
		return "", false
	}

	parts := strings.Split(handle, "@")
	if len(parts) != 2 {
		return "", false
	}

	if !strings.EqualFold(parts[1], localDomain) {
		return "", false
	}

	return parts[0], true
}

func (r *Resolver) buildPlaceholderActor(actorID, localDomain string) *activitypub.Actor {
	now := time.Now()

	id := actorID
	if !strings.Contains(id, "://") && localDomain != "" && !strings.Contains(id, "@") {
		id = fmt.Sprintf("https://%s/users/%s", localDomain, id)
	}

	if common.ValidateRequiredParam("actor_id", id) != nil {
		if r.Logger != nil {
			r.Logger.Warn("unable to build placeholder actor", zap.String("actor_id", actorID))
		}
		return nil
	}

	username := placeholderUsername(actorID)

	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:        id,
			Type:      "Person",
			Published: &now,
			Updated:   &now,
		},
		PreferredUsername: username,
	}
}

func placeholderUsername(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ""
	}

	if strings.Contains(actorID, "://") {
		return transformations.ExtractUsernameFromActorID(actorID)
	}

	if strings.Contains(actorID, "@") {
		parts := strings.Split(strings.TrimPrefix(actorID, "@"), "@")
		if len(parts) > 0 {
			return parts[0]
		}
	}

	return strings.TrimPrefix(actorID, "@")
}
