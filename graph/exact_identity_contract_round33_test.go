package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound33QueryResolvers_Accounts_ExactIdentityContract(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	actorRepo, ok := storage.Actor().(*inmemory.ActorRepository)
	require.True(t, ok)

	localActorID := "https://localhost/users/alice"
	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}
	actorRepo.SetCachedRemoteActor("alice@remote.example", remoteActor, time.Hour)

	localUsername := "alice"
	localActorURL := localActorID
	remoteHandle := "alice@remote.example"
	remoteActorURL := remoteActor.ID

	tests := []struct {
		name      string
		id        *string
		username  *string
		wantID    string
		wantLocal bool
	}{
		{name: "local username", username: &localUsername, wantID: localActorID, wantLocal: true},
		{name: "local actor URL", id: &localActorURL, wantID: localActorID, wantLocal: true},
		{name: "remote handle", username: &remoteHandle, wantID: remoteActor.ID, wantLocal: false},
		{name: "remote actor URL", id: &remoteActorURL, wantID: remoteActor.ID, wantLocal: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actor, err := resolver.Query().Actor(context.Background(), tc.id, tc.username)
			require.NoError(t, err)
			require.NotNil(t, actor)
			require.Equal(t, tc.wantID, actor.ID)
			require.Equal(t, "alice", actor.PreferredUsername)
			if tc.wantLocal {
				require.Equal(t, localActorID, actor.ID)
			} else {
				require.NotEqual(t, localActorID, actor.ID)
			}
		})
	}
}

func TestRound33QueryResolvers_Search_ExactIdentityContract(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	actorRepo, ok := storage.Actor().(*inmemory.ActorRepository)
	require.True(t, ok)

	localActorID := "https://localhost/users/alice"
	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}
	actorRepo.SetCachedRemoteActor("alice@remote.example", remoteActor, time.Hour)

	searchType := "accounts"
	tests := []struct {
		name      string
		query     string
		wantID    string
		wantLocal bool
	}{
		{name: "local actor URL", query: localActorID, wantID: localActorID, wantLocal: true},
		{name: "remote handle", query: "alice@remote.example", wantID: remoteActor.ID, wantLocal: false},
		{name: "remote actor URL", query: remoteActor.ID, wantID: remoteActor.ID, wantLocal: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := resolver.Query().Search(context.Background(), tc.query, &searchType, nil, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, result.Accounts, 1)
			require.Equal(t, tc.wantID, result.Accounts[0].ID)
			require.Equal(t, "alice", result.Accounts[0].PreferredUsername)
			if tc.wantLocal {
				require.Equal(t, localActorID, result.Accounts[0].ID)
			} else {
				require.NotEqual(t, localActorID, result.Accounts[0].ID)
			}
		})
	}
}
