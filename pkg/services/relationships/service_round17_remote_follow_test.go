package relationships

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestService_Follow_AllowsRemoteFolloweeByHandle(t *testing.T) {
	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)

	actorRepo := inmemory.NewActorRepository()
	relationshipRepo := inmemory.NewRelationshipRepository()
	storageHarness.actorRepo = actorRepo
	storageHarness.relationshipRepo = relationshipRepo

	require.NoError(t, actorRepo.CreateActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}, ""))

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.social/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             "https://remote.social/users/bob/inbox",
		Outbox:            "https://remote.social/users/bob/outbox",
	}
	actorRepo.SetCachedRemoteActor("bob@remote.social", remoteActor, time.Hour)

	result, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsFollowing)
	require.NotNil(t, result.Relationship)
	require.False(t, result.Relationship.Following)
	require.True(t, result.Relationship.Requested)
	require.NotNil(t, result.Activity)
	require.True(t, strings.Contains(result.Activity.ID, url.PathEscape("bob@remote.social")))

	record, err := relationshipRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, "pending", record.State)

	switch object := result.Activity.Object.(type) {
	case *activitypub.Actor:
		require.Equal(t, remoteActor.ID, object.ID)
	default:
		require.Equal(t, remoteActor.ID, object)
	}
}

func TestService_Follow_AllowsRemoteFolloweeByActorURL(t *testing.T) {
	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)

	actorRepo := inmemory.NewActorRepository()
	relationshipRepo := inmemory.NewRelationshipRepository()
	storageHarness.actorRepo = actorRepo
	storageHarness.relationshipRepo = relationshipRepo

	require.NoError(t, actorRepo.CreateActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}, ""))

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.social/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             "https://remote.social/users/bob/inbox",
		Outbox:            "https://remote.social/users/bob/outbox",
	}
	actorRepo.SetCachedRemoteActor("bob@remote.social", remoteActor, time.Hour)

	result, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: remoteActor.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsFollowing)
	require.NotNil(t, result.Relationship)
	require.True(t, result.Relationship.Requested)
	require.NotNil(t, result.Activity)

	record, err := relationshipRepo.GetRelationship(ctx, "alice", "bob@remote.social")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, "pending", record.State)

	switch object := result.Activity.Object.(type) {
	case *activitypub.Actor:
		require.Equal(t, remoteActor.ID, object.ID)
	default:
		require.Equal(t, remoteActor.ID, object)
	}
}

func TestService_Follow_RemotePendingRequestIsIdempotent(t *testing.T) {
	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)

	actorRepo := inmemory.NewActorRepository()
	relationshipRepo := inmemory.NewRelationshipRepository()
	storageHarness.actorRepo = actorRepo
	storageHarness.relationshipRepo = relationshipRepo

	require.NoError(t, actorRepo.CreateActor(ctx, &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}, ""))

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.social/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
		Inbox:             "https://remote.social/users/bob/inbox",
		Outbox:            "https://remote.social/users/bob/outbox",
	}
	actorRepo.SetCachedRemoteActor("bob@remote.social", remoteActor, time.Hour)

	first, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	require.False(t, first.IsFollowing)
	require.True(t, first.Relationship.Requested)
	require.NotEmpty(t, first.RequestID)

	second, err := service.Follow(ctx, &FollowCommand{
		FollowerID:  "alice",
		FollowingID: "bob@remote.social",
	})
	require.NoError(t, err)
	require.NotNil(t, second)
	require.False(t, second.IsFollowing)
	require.True(t, second.Relationship.Requested)
	require.Equal(t, first.RequestID, second.RequestID)
}

func TestService_BuildAccountFromActor_PreservesRemoteIdentity(t *testing.T) {
	ctx := context.Background()
	service, _ := newServiceWithStorageHarness(t)

	now := time.Now().UTC()
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.social/users/bob",
			Type:      activitypub.PersonType,
			Published: &now,
			Updated:   &now,
		},
		PreferredUsername: "bob",
		URL:               "https://remote.social/@bob",
	}

	account := service.buildAccountFromActor(ctx, actor, "bob@remote.social")
	require.NotNil(t, account)
	require.NotNil(t, account.User)
	require.NotNil(t, account.Actor)
	require.Equal(t, "bob@remote.social", account.User.Username)
	require.Equal(t, actor.ID, account.User.ID)
	require.Equal(t, actor.ID, account.Actor.ID)
	require.Equal(t, actor.URL, account.Actor.URL)
}
