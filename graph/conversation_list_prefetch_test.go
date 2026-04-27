package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12_ConvertConversationListToGraphQL_UsesCanonicalViewerStateAndPrefetch(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)

	previousViewerBoostResolver := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(_ context.Context, _ *Resolver, _, _ string) (bool, error) {
		return false, nil
	}
	defer func() {
		viewerBoostStateResolverFunc = previousViewerBoostResolver
	}()

	now := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	requestedAt := now.Add(-time.Hour)
	conversation := &storagemodels.Conversation{
		ID:           "conv-1",
		Participants: []string{"alice", "bob"},
		LastStatusID: "legacy-status",
		Unread:       true,
		CreatedAt:    now.Add(-2 * time.Hour),
		UpdatedAt:    now,
		ViewerState: &storagemodels.UserConversationState{
			ViewerID:        "alice",
			ConversationID:  "conv-1",
			CounterpartID:   "bob",
			Folder:          storagemodels.UserConversationFolderRequests,
			RequestState:    storagemodels.DmRequestStatePending,
			RequestedAt:     &requestedAt,
			PreviewStatusID: "status-preview",
			Unread:          true,
		},
	}

	prefetch := &conversationListPrefetch{
		accountsByKey: map[string]*storage.Account{
			"bob": {
				User: &storage.User{
					Username:    "bob",
					DisplayName: "Bob",
				},
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://localhost/users/bob", Type: activitypub.PersonType},
					PreferredUsername: "bob",
					Name:              "Bob",
				},
			},
		},
		statusesByID: map[string]*storagemodels.Status{
			"status-preview": {
				StatusID:       "status-preview",
				AuthorUsername: "bob",
				AuthorID:       "https://localhost/users/bob",
				Content:        "hello from preview",
				Visibility:     storagemodels.VisibilityPublic,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		statusesReady: true,
	}

	gqlConversation := resolver.convertConversationListToGraphQL(round12AuthContext("alice"), conversation, prefetch)
	require.NotNil(t, gqlConversation)
	require.Equal(t, "conv-1", gqlConversation.ID)
	require.NotNil(t, gqlConversation.ViewerMetadata)
	require.Equal(t, "PENDING", string(gqlConversation.ViewerMetadata.RequestState))
	require.NotNil(t, gqlConversation.ViewerMetadata.RequestedAt)
	require.Len(t, gqlConversation.Accounts, 1)
	require.Equal(t, "bob", gqlConversation.Accounts[0].PreferredUsername)
	require.NotNil(t, gqlConversation.LastStatus)
	require.Equal(t, "status-preview", gqlConversation.LastStatus.ID)
}

func TestRound12_ConvertConversationListToGraphQL_FallsBackWhenPrefetchMisses(t *testing.T) {
	resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)

	previousViewerBoostResolver := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(_ context.Context, _ *Resolver, _, _ string) (bool, error) {
		return false, nil
	}
	defer func() {
		viewerBoostStateResolverFunc = previousViewerBoostResolver
	}()

	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, storage.Status().CreateStatus(context.Background(), &storagemodels.Status{
		StatusID:       "status-preview",
		AuthorUsername: "bob",
		AuthorID:       "https://localhost/users/bob",
		Content:        "loaded from point lookup",
		Visibility:     storagemodels.VisibilityPublic,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	conversation := &storagemodels.Conversation{
		ID:           "conv-2",
		Participants: []string{"alice", "bob"},
		CreatedAt:    now.Add(-2 * time.Hour),
		UpdatedAt:    now,
		ViewerState: &storagemodels.UserConversationState{
			ViewerID:        "alice",
			ConversationID:  "conv-2",
			CounterpartID:   "bob",
			PreviewStatusID: "status-preview",
			Unread:          true,
		},
	}

	gqlConversation := resolver.convertConversationListToGraphQL(round12AuthContext("alice"), conversation, &conversationListPrefetch{})
	require.NotNil(t, gqlConversation)
	require.Len(t, gqlConversation.Accounts, 1)
	require.Equal(t, "bob", gqlConversation.Accounts[0].PreferredUsername)
	require.NotNil(t, gqlConversation.LastStatus)
	require.Equal(t, "status-preview", gqlConversation.LastStatus.ID)
}

func TestRound12_ConversationAccountsFromPrefetch_UsesFederatedParticipantKeys(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)

	remoteID := "https://remote.example/users/bob"
	account := &storage.Account{
		User: &storage.User{
			Username:    "bob",
			DisplayName: "Bob",
		},
		Actor: &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: remoteID, Type: activitypub.PersonType},
			PreferredUsername: "bob",
			Name:              "Bob",
		},
	}

	prefetch := &conversationListPrefetch{
		accountsByKey: buildConversationAccountMap([]*storage.Account{account}),
		statusesReady: true,
	}

	actors := resolver.conversationAccountsFromParticipantRefs(round12AuthContext("alice"), []storagemodels.ConversationParticipantRef{
		{
			ParticipantType: storagemodels.ConversationParticipantTypeLocalUser,
			ParticipantID:   "alice",
		},
		{
			ParticipantType: storagemodels.ConversationParticipantTypeRemoteActor,
			ParticipantID:   remoteID,
		},
	}, "alice", prefetch)
	require.Len(t, actors, 1)
	require.Equal(t, remoteID, actors[0].ID)
	require.Equal(t, "bob", actors[0].PreferredUsername)
}

func TestConversationAccountsFromParticipantRefs_UsesSyntheticRemoteFallback(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	remoteID := "https://remote.example/users/bob"

	actors := resolver.conversationAccountsFromParticipantRefs(round12AuthContext("alice"), []storagemodels.ConversationParticipantRef{
		{
			ParticipantType: storagemodels.ConversationParticipantTypeLocalUser,
			ParticipantID:   "alice",
		},
		{
			ParticipantType: storagemodels.ConversationParticipantTypeRemoteActor,
			ParticipantID:   remoteID,
			Acct:            "bob@remote.example",
			Domain:          "remote.example",
		},
	}, "alice", &conversationListPrefetch{accountsByKey: map[string]*storage.Account{}})

	require.Len(t, actors, 1)
	require.Equal(t, remoteID, actors[0].ID)
	require.Equal(t, "bob", actors[0].PreferredUsername)
}
