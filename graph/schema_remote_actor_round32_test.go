package graph

import (
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
