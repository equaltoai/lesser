package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_Relationships_Mainline(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)

	actorRepo := storageRepo.Actor()
	require.NoError(t, actorRepo.CreateActor(context.Background(), &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   config.Get().ActorURL("alice"),
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
	}, ""))
	require.NoError(t, actorRepo.CreateActor(context.Background(), &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   config.Get().ActorURL("bob"),
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
	}, ""))

	mut := resolver.Mutation()
	ctx := round12AuthContext("alice")

	act, err := mut.FollowActor(ctx, "bob")
	require.NoError(t, err)
	require.NotNil(t, act)

	relationship, err := mut.BlockActor(ctx, "bob")
	require.NoError(t, err)
	require.NotNil(t, relationship)

	notifications := false
	relationship, err = mut.MuteActor(ctx, "bob", &notifications)
	require.NoError(t, err)
	require.True(t, relationship.Muting)
	require.Equal(t, notifications, relationship.MutingNotifications)

	updated, err := mut.UpdateRelationship(ctx, "bob", model.UpdateRelationshipInput{
		Notify:      ptrBool(true),
		ShowReblogs: ptrBool(false),
		Note:        ptrString("note"),
		Languages:   []string{"en"},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	ok, err := mut.AddDomainBlock(ctx, "example.com")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = mut.RemoveDomainBlock(ctx, "example.com")
	require.NoError(t, err)
	require.True(t, ok)

	_, err = mut.AcceptFollowRequest(ctx, "bob")
	require.Error(t, err)

	_, err = mut.RejectFollowRequest(ctx, "bob")
	require.Error(t, err)
}
