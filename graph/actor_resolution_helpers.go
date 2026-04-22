package graph

import (
	"context"
	"fmt"
	neturl "net/url"
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

func (r *Resolver) localActorDomain() string {
	if r == nil || r.Config == nil {
		return ""
	}
	return r.Config.Domain
}

func (r *Resolver) resolveExactActorLookup(ctx context.Context, actorID string) (*federation.ExactActorResolution, error) {
	store := r.getStorageForResolution()
	if store == nil {
		return nil, common.ActorNotFoundError{Username: actorID}
	}

	resolution, err := federation.NewRemoteSearchService(store).ResolveExactActor(ctx, actorID, r.localActorDomain())
	if err == nil || !graphActorLookupNotFound(err) {
		return resolution, err
	}

	username := r.localUsernameForLookup(actorID)
	if username == "" || r.Registry == nil || r.Registry.Accounts() == nil {
		return nil, err
	}

	account, accountErr := r.Registry.Accounts().GetAccount(ctx, username)
	if accountErr != nil || account == nil {
		return nil, err
	}

	actor := r.convertAccountToActor(account)
	return &federation.ExactActorResolution{
		Actor:         actor,
		ActorIdentity: federation.DescribeActorIdentity(actor, r.localActorDomain()),
	}, nil
}

func (r *Resolver) resolveStoredActorLookup(ctx context.Context, actorID string) (*federation.ExactActorResolution, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, common.ActorNotFoundError{Username: actorID}
	}

	store := r.getStorageForResolution()
	if store == nil || store.Actor() == nil {
		return nil, common.ActorNotFoundError{Username: actorID}
	}

	if username := r.localUsernameForLookup(actorID); username != "" {
		actor, err := store.Actor().GetActorByUsername(ctx, username)
		if err == nil && actor != nil {
			return &federation.ExactActorResolution{
				Actor:         actor,
				ActorIdentity: federation.DescribeActorIdentity(actor, r.localActorDomain()),
			}, nil
		}
		if err != nil && !graphActorLookupNotFound(err) {
			return nil, err
		}
	}

	actor, err := store.Actor().GetCachedRemoteActor(ctx, actorID)
	if err == nil && actor != nil {
		return &federation.ExactActorResolution{
			Actor:         actor,
			ActorIdentity: federation.DescribeActorIdentity(actor, r.localActorDomain()),
		}, nil
	}
	if err != nil && !graphActorLookupNotFound(err) {
		return nil, err
	}

	return nil, common.ActorNotFoundError{Username: actorID}
}

func graphActorLookupNotFound(err error) bool {
	if err == nil {
		return false
	}
	if common.IsNotFound(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func (r *Resolver) materializeActorResolution(ctx context.Context, resolution *federation.ExactActorResolution) *activitypub.Actor {
	if resolution == nil || resolution.Actor == nil {
		return nil
	}

	if resolution.IsRemote {
		return resolution.Actor
	}

	username := strings.TrimSpace(resolution.Username)
	if username == "" {
		username = strings.TrimSpace(resolution.Actor.PreferredUsername)
	}
	if username == "" || r.Registry == nil || r.Registry.Accounts() == nil {
		return resolution.Actor
	}

	account, err := r.Registry.Accounts().GetAccount(ctx, username)
	if err != nil || account == nil {
		return resolution.Actor
	}

	return r.convertAccountToActor(account)
}

func (r *Resolver) localUsernameForLookup(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ""
	}

	localDomain := strings.TrimSpace(r.localActorDomain())
	if localDomain == "" {
		return strings.TrimPrefix(actorID, "@")
	}

	if strings.HasPrefix(strings.ToLower(actorID), "http://") || strings.HasPrefix(strings.ToLower(actorID), "https://") {
		parsed, err := neturl.Parse(actorID)
		if err != nil || !strings.EqualFold(strings.TrimSpace(parsed.Hostname()), localDomain) {
			return ""
		}
		return strings.TrimPrefix(transformations.ExtractUsernameFromActorID(actorID), "@")
	}

	value := strings.TrimPrefix(actorID, "@")
	if strings.Count(value, "@") == 1 {
		parts := strings.Split(value, "@")
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[1]), localDomain) {
			return strings.TrimSpace(parts[0])
		}
		return ""
	}

	if strings.Contains(value, "@") {
		return ""
	}

	return value
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
