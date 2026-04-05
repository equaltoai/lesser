package graph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
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
	if store == nil {
		return nil
	}

	resolution, err := federation.NewRemoteSearchService(store).ResolveExactActor(ctx, actorID, localDomain)
	if err != nil || resolution == nil {
		return nil
	}

	return resolution.Actor
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
