package routing

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

const sharedInboxFollowersLookupLimit = 1000

type sharedInboxActorRepository interface {
	GetActorByUsername(ctx context.Context, username string) (*activitypub.Actor, error)
}

type sharedInboxRelationshipRepository interface {
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	DeleteRelationship(ctx context.Context, followerUsername, followingUsername string) error
}

type sharedInboxActivityRepository interface {
	GetActivity(ctx context.Context, id string) (*activitypub.Activity, error)
}

type sharedInboxObjectRepository interface {
	GetObject(ctx context.Context, id string) (any, error)
}

type sharedInboxTargetResolver struct {
	actorRepository        sharedInboxActorRepository
	relationshipRepository sharedInboxRelationshipRepository
	activityRepository     sharedInboxActivityRepository
	objectRepository       sharedInboxObjectRepository
	localDomain            string
}

func (r sharedInboxTargetResolver) Resolve(ctx context.Context, activity *activitypub.Activity) ([]*activitypub.Actor, error) {
	usernames := make(map[string]struct{})
	followerHandles := make(map[string]struct{})

	actorOwner := r.localOrRemoteIdentity(activity.Actor)
	r.addAddressTargets(usernames, followerHandles, actorOwner, activity.To...)
	r.addAddressTargets(usernames, followerHandles, actorOwner, activity.CC...)
	r.addAddressTargets(usernames, followerHandles, actorOwner, activity.BTo...)
	r.addAddressTargets(usernames, followerHandles, actorOwner, activity.BCC...)

	switch activity.Type {
	case activitypub.FollowType:
		r.addObjectTargets(usernames, followerHandles, activity.Object)
	case activitypub.AcceptType, activitypub.RejectType:
		if err := r.addStoredActivityTargets(ctx, usernames, activity.Object); err != nil {
			return nil, err
		}
	case activitypub.UndoType:
		if err := r.addUndoTargets(ctx, usernames, activity.Object); err != nil {
			return nil, err
		}
	case activitypub.CreateType, activitypub.UpdateType, activitypub.DeleteType:
		r.addObjectTargets(usernames, followerHandles, activity.Object)
	case activitypub.LikeType, activitypub.AnnounceType:
		if err := r.addObjectOwnerTargets(ctx, usernames, activity.Object); err != nil {
			return nil, err
		}
	case activitypub.AddType, activitypub.RemoveType:
		r.addAddressTargets(usernames, followerHandles, actorOwner, activity.Target)
		if err := r.addObjectOwnerTargets(ctx, usernames, activity.Object); err != nil {
			return nil, err
		}
	}

	if err := r.expandFollowerHandles(ctx, usernames, followerHandles); err != nil {
		return nil, err
	}

	return r.loadActors(ctx, usernames)
}

func (r sharedInboxTargetResolver) addUndoTargets(ctx context.Context, usernames map[string]struct{}, object any) error {
	if err := r.addStoredActivityTargets(ctx, usernames, object); err != nil {
		return err
	}
	return r.addObjectOwnerTargets(ctx, usernames, object)
}

func (r sharedInboxTargetResolver) addStoredActivityTargets(ctx context.Context, usernames map[string]struct{}, object any) error {
	activityID := extractObjectID(object)
	if activityID == "" || r.activityRepository == nil {
		return nil
	}

	activity, err := r.activityRepository.GetActivity(ctx, activityID)
	if err != nil || activity == nil {
		return nil
	}

	if username := r.localUsername(activity.Actor); username != "" {
		usernames[username] = struct{}{}
	}
	if username := r.localUsername(activity.Target); username != "" {
		usernames[username] = struct{}{}
	}
	for _, recipient := range explicitLocalRecipients(activity, r.localDomain) {
		usernames[recipient] = struct{}{}
	}

	return nil
}

func (r sharedInboxTargetResolver) addObjectTargets(usernames map[string]struct{}, followerHandles map[string]struct{}, object any) {
	if username := r.localUsername(extractObjectID(object)); username != "" {
		usernames[username] = struct{}{}
	}

	switch typed := object.(type) {
	case map[string]any:
		if actorID, _ := typed["actor"].(string); actorID != "" {
			r.addAddressTargets(usernames, followerHandles, r.localOrRemoteIdentity(actorID), actorID)
		}
		if attributedTo, _ := typed["attributedTo"].(string); attributedTo != "" {
			r.addAddressTargets(usernames, followerHandles, r.localOrRemoteIdentity(attributedTo), attributedTo)
		}
	}
}

func (r sharedInboxTargetResolver) addObjectOwnerTargets(ctx context.Context, usernames map[string]struct{}, object any) error {
	objectID := extractObjectID(object)
	if objectID == "" || r.objectRepository == nil {
		return nil
	}

	storedObject, err := r.objectRepository.GetObject(ctx, objectID)
	if err != nil || storedObject == nil {
		return nil
	}

	if username := r.localUsername(extractAttributedTo(storedObject)); username != "" {
		usernames[username] = struct{}{}
	}
	return nil
}

func (r sharedInboxTargetResolver) expandFollowerHandles(ctx context.Context, usernames map[string]struct{}, followerHandles map[string]struct{}) error {
	if r.relationshipRepository == nil {
		return nil
	}

	for handle := range followerHandles {
		cursor := ""
		for {
			followers, nextCursor, err := r.relationshipRepository.GetFollowers(ctx, handle, sharedInboxFollowersLookupLimit, cursor)
			if err != nil {
				return err
			}
			for _, follower := range followers {
				username := r.localUsername(follower)
				if username == "" {
					continue
				}
				if r.actorRepository != nil {
					actor, err := r.actorRepository.GetActorByUsername(ctx, username)
					if err != nil || actor == nil {
						_ = r.relationshipRepository.DeleteRelationship(ctx, username, handle)
						continue
					}
				}
				usernames[username] = struct{}{}
			}
			if nextCursor == "" {
				break
			}
			cursor = nextCursor
		}
	}

	return nil
}

func (r sharedInboxTargetResolver) loadActors(ctx context.Context, usernames map[string]struct{}) ([]*activitypub.Actor, error) {
	if len(usernames) == 0 || r.actorRepository == nil {
		return nil, nil
	}

	actors := make([]*activitypub.Actor, 0, len(usernames))
	for username := range usernames {
		actor, err := r.actorRepository.GetActorByUsername(ctx, username)
		if err != nil || actor == nil {
			continue
		}
		actors = append(actors, actor)
	}

	return actors, nil
}

func (r sharedInboxTargetResolver) addAddressTargets(usernames map[string]struct{}, followerHandles map[string]struct{}, authorizedFollowerOwner string, addresses ...string) {
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address == "" || address == activitypub.PublicAddress {
			continue
		}

		normalized := models.NormalizeRelationshipIdentity(address, r.localDomain)
		if normalized == "" {
			continue
		}

		if isFollowersCollectionAddress(address) {
			owner := r.followersCollectionOwner(address)
			if owner != "" && owner == authorizedFollowerOwner {
				followerHandles[owner] = struct{}{}
			}
			continue
		}

		if !strings.Contains(normalized, "@") {
			usernames[normalized] = struct{}{}
		}
	}
}

func (r sharedInboxTargetResolver) localUsername(address string) string {
	normalized := r.localOrRemoteIdentity(address)
	if normalized == "" || strings.Contains(normalized, "@") {
		return ""
	}
	return normalized
}

func (r sharedInboxTargetResolver) localOrRemoteIdentity(address string) string {
	return models.NormalizeRelationshipIdentity(address, r.localDomain)
}

func (r sharedInboxTargetResolver) followersCollectionOwner(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	trimmed := strings.TrimSuffix(address, "/")
	idx := strings.LastIndex(trimmed, "/followers")
	if idx < 0 || idx+len("/followers") != len(trimmed) {
		return ""
	}
	return r.localOrRemoteIdentity(trimmed[:idx])
}

func isFollowersCollectionAddress(address string) bool {
	address = strings.TrimSpace(address)
	if address == "" {
		return false
	}
	trimmed := strings.TrimSuffix(address, "/")
	return strings.HasSuffix(trimmed, "/followers")
}

func explicitLocalRecipients(activity *activitypub.Activity, localDomain string) []string {
	results := make([]string, 0)
	seen := make(map[string]struct{})
	for _, address := range append(append(append(activity.To, activity.CC...), activity.BTo...), activity.BCC...) {
		normalized := models.NormalizeRelationshipIdentity(address, localDomain)
		if normalized == "" || strings.Contains(normalized, "@") {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		results = append(results, normalized)
	}
	return results
}

func extractObjectID(object any) string {
	switch typed := object.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if id, ok := typed["id"].(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func extractAttributedTo(object any) string {
	switch typed := object.(type) {
	case *activitypub.Note:
		if typed == nil {
			return ""
		}
		return strings.TrimSpace(typed.AttributedTo)
	case activitypub.Note:
		return strings.TrimSpace(typed.AttributedTo)
	case map[string]any:
		if value, ok := typed["attributedTo"].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
