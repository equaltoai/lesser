package relationships

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestService_Round18_ResolveFollowStorageAccount_ExactIdentityContract(t *testing.T) {
	ctx := context.Background()
	service, storageHarness := newServiceWithStorageHarness(t)

	actorRepo := inmemory.NewActorRepository()
	storageHarness.actorRepo = actorRepo

	localActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Inbox:             "https://example.com/users/alice/inbox",
		Outbox:            "https://example.com/users/alice/outbox",
	}
	require.NoError(t, actorRepo.CreateActor(ctx, localActor, ""))

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.social/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		URL:               "https://remote.social/@alice",
		Inbox:             "https://remote.social/users/alice/inbox",
		Outbox:            "https://remote.social/users/alice/outbox",
	}
	actorRepo.SetCachedRemoteActor("alice@remote.social", remoteActor, time.Hour)

	tests := []struct {
		name            string
		input           string
		requireLocal    bool
		wantID          string
		wantUsername    string
		wantErr         bool
		wantRemoteActor bool
	}{
		{name: "local username control", input: "alice", requireLocal: true, wantID: localActor.ID, wantUsername: "alice"},
		{name: "local actor URL control", input: localActor.ID, requireLocal: true, wantID: localActor.ID, wantUsername: "alice"},
		{name: "remote handle", input: "alice@remote.social", wantID: remoteActor.ID, wantUsername: "alice@remote.social", wantRemoteActor: true},
		{name: "remote actor URL", input: remoteActor.ID, wantID: remoteActor.ID, wantUsername: "alice@remote.social", wantRemoteActor: true},
		{name: "remote handle rejected for local-only path", input: "alice@remote.social", requireLocal: true, wantErr: true},
		{name: "remote actor URL rejected for local-only path", input: remoteActor.ID, requireLocal: true, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			account, err := service.resolveFollowStorageAccount(ctx, tc.input, tc.requireLocal)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, account)
			require.NotNil(t, account.User)
			require.NotNil(t, account.Actor)
			require.Equal(t, tc.wantID, account.Actor.ID)
			require.Equal(t, tc.wantUsername, account.User.Username)
			if tc.wantRemoteActor {
				require.NotEqual(t, localActor.ID, account.Actor.ID)
			} else {
				require.Equal(t, localActor.ID, account.Actor.ID)
			}
		})
	}
}
