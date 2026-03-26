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
		accountsByUsername: map[string]*storage.Account{
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
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
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
