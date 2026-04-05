package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_Accounts_Basics(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	q := resolver.Query()

	ctx := round12AuthContext("alice")
	viewer, err := q.Viewer(ctx)
	require.NoError(t, err)
	require.NotNil(t, viewer)

	_, err = q.Actor(context.Background(), nil, nil)
	require.Error(t, err)

	id := "alice"
	actor, err := q.Actor(context.Background(), &id, nil)
	require.NoError(t, err)
	require.NotNil(t, actor)

	username := "alice"
	actor, err = q.Actor(context.Background(), nil, &username)
	require.NoError(t, err)
	require.NotNil(t, actor)

	fullID := config.Get().ActorURL("alice")
	actor, err = q.Actor(context.Background(), &fullID, nil)
	require.NoError(t, err)
	require.NotNil(t, actor)

	perms, err := q.AccountQuotePermissions(context.Background(), "alice")
	require.NoError(t, err)
	require.NotNil(t, perms)
	require.NotNil(t, perms.BlockList)

	emojis, err := q.CustomEmojis(context.Background())
	require.NoError(t, err)
	require.NotNil(t, emojis)

	first := 10
	directory, err := q.ProfileDirectory(context.Background(), &model.DirectoryFiltersInput{Local: ptrBool(true)}, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, directory)

	suggestions, err := q.Suggestions(context.Background(), &first)
	require.NoError(t, err)
	require.NotNil(t, suggestions)

	ok, err := q.RemoveSuggestion(ctx, "bob")
	require.NoError(t, err)
	require.True(t, ok)

	endorsements, err := q.Endorsements(ctx)
	require.NoError(t, err)
	require.NotNil(t, endorsements)
}

func TestRound12QueryResolvers_Accounts_RemoteActorLookupUsesExactResolution(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	actorRepo, ok := storage.Actor().(*inmemory.ActorRepository)
	require.True(t, ok)

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/steward",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "steward",
		Inbox:             "https://remote.example/users/steward/inbox",
		Outbox:            "https://remote.example/users/steward/outbox",
	}
	actorRepo.SetCachedRemoteActor("steward@remote.example", remoteActor, time.Hour)

	q := resolver.Query()

	username := "steward@remote.example"
	actor, err := q.Actor(context.Background(), nil, &username)
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.Equal(t, remoteActor.ID, actor.ID)

	id := remoteActor.ID
	actor, err = q.Actor(context.Background(), &id, nil)
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.Equal(t, remoteActor.ID, actor.ID)
}
