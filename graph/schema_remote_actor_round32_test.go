package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestRound32Resolver_ConvertAccountToActor_PreservesRemoteIdentity(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		URL:               "https://remote.example/@alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}

	actor := resolver.convertAccountToActor(&storage.Account{
		User:  &storage.User{Username: "alice@remote.example", DisplayName: "Wrapped Alice"},
		Actor: remoteActor,
	})
	require.NotNil(t, actor)
	require.Equal(t, remoteActor.ID, actor.ID)
	require.Equal(t, remoteActor.PreferredUsername, actor.PreferredUsername)
	require.Equal(t, remoteActor.URL, actor.URL)
}

func TestRound32ActivityResolver_ActorPreservesRemoteIdentifiers(t *testing.T) {
	resolver, graphStorage := newRound12GraphResolver(t)
	graphStorage.SeedAccountUser(&storage.User{
		Username:    "alice",
		DisplayName: "Local Alice",
		Approved:    true,
	})

	activityResolver := resolver.Activity()
	ctx := context.Background()

	remoteActorURL := "https://remote.example/users/alice"
	urlActor, err := activityResolver.Actor(ctx, &activitypub.Activity{Actor: remoteActorURL})
	require.NoError(t, err)
	require.NotNil(t, urlActor)
	require.Equal(t, remoteActorURL, urlActor.ID)
	require.Equal(t, "alice", urlActor.PreferredUsername)

	remoteActorHandle := "alice@remote.example"
	handleActor, err := activityResolver.Actor(ctx, &activitypub.Activity{Actor: remoteActorHandle})
	require.NoError(t, err)
	require.NotNil(t, handleActor)
	require.Equal(t, remoteActorHandle, handleActor.ID)
	require.Equal(t, "alice", handleActor.PreferredUsername)
}
