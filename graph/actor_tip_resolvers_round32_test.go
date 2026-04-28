package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
)

func TestActorTipAddressRequiresAuthorizedViewer_Round32(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	state.autoPopulateAll = true
	state.autoPopulateCount = 2
	resolver.Config.TipEnabled = true
	resolver.Config.TipChainID = 1
	resolver.Config.TipContractAddress = "0xfeedfeedfeedfeedfeedfeedfeedfeedfeedfeed"

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://localhost/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
	}
	actorResolver := &actorResolver{resolver}

	publicTip, err := actorResolver.TipAddress(context.Background(), actor)
	require.NoError(t, err)
	require.Nil(t, publicTip)

	otherTip, err := actorResolver.TipAddress(round12AuthContext("bob"), actor)
	require.NoError(t, err)
	require.Nil(t, otherTip)

	ownerTip, err := actorResolver.TipAddress(round12AuthContext("alice"), actor)
	require.NoError(t, err)
	require.NotNil(t, ownerTip)
	require.Equal(t, "0x2222222222222222222222222222222222222222", *ownerTip)

	adminTip, err := actorResolver.TipAddress(round12AuthContext("admin"), actor)
	require.NoError(t, err)
	require.NotNil(t, adminTip)
	require.Equal(t, "0x2222222222222222222222222222222222222222", *adminTip)

	remoteCollisionActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
	}
	remoteTip, err := actorResolver.TipAddress(round12AuthContext("alice"), remoteCollisionActor)
	require.NoError(t, err)
	require.Nil(t, remoteTip)
}
