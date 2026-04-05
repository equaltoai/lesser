package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAccountsRound22_ExactIdentityContract_ResolveAndSerialize(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	localActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.ActorURL("alice"),
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Local Alice",
		URL:               cfg.BaseURL() + "/@alice",
	}
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

	cached := storagemodels.RemoteActor{
		Handle:    "alice@remote.example",
		Actor:     remoteActor,
		ExpiresAt: now.Add(time.Hour),
		CachedAt:  now,
		UpdatedAt: now,
	}
	cached.UpdateKeys()

	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username:  "alice",
				Actor:     localActor,
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
		},
		remoteActorsByPK: map[string]storagemodels.RemoteActor{
			cached.PK: cached,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	tests := []struct {
		name                    string
		input                   string
		wantResolvedActorID     string
		wantSerializedAccountID string
		wantAcct                string
		wantLocal               bool
	}{
		{
			name:                    "local username",
			input:                   "alice",
			wantResolvedActorID:     localActor.ID,
			wantSerializedAccountID: common.GenerateNumericID("alice"),
			wantAcct:                "alice",
			wantLocal:               true,
		},
		{
			name:                    "local actor URL",
			input:                   localActor.ID,
			wantResolvedActorID:     localActor.ID,
			wantSerializedAccountID: common.GenerateNumericID("alice"),
			wantAcct:                "alice",
			wantLocal:               true,
		},
		{
			name:                    "remote handle",
			input:                   "alice@remote.example",
			wantResolvedActorID:     remoteActor.ID,
			wantSerializedAccountID: remoteActor.ID,
			wantAcct:                "alice@remote.example",
			wantLocal:               false,
		},
		{
			name:                    "remote actor URL",
			input:                   remoteActor.ID,
			wantResolvedActorID:     remoteActor.ID,
			wantSerializedAccountID: remoteActor.ID,
			wantAcct:                "alice@remote.example",
			wantLocal:               false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actor, err := h.resolveAccountID(context.Background(), tc.input)
			require.NoError(t, err)
			require.NotNil(t, actor)
			require.Equal(t, tc.wantResolvedActorID, actor.ID)

			account := h.publicAccountFromActor(context.Background(), actor)
			require.Equal(t, tc.wantSerializedAccountID, account.ID)
			require.Equal(t, "alice", account.Username)
			require.Equal(t, tc.wantAcct, account.Acct)

			if tc.wantLocal {
				require.True(t, h.actorAppearsLocal(actor))
				require.Equal(t, cfg.BaseURL()+"/@alice", account.URL)
			} else {
				require.False(t, h.actorAppearsLocal(actor))
				require.Equal(t, remoteActor.URL, account.URL)
			}
		})
	}
}
