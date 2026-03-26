package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound11_ConvertConversationToAPIWithPrefetch_UsesViewerStatePreview(t *testing.T) {
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})

	now := time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC)
	conversation := &storageModels.Conversation{
		ID:           "conv-1",
		Participants: []string{"alice", "bob"},
		LastStatusID: "legacy-status",
		Unread:       true,
		ViewerState: &storageModels.UserConversationState{
			ViewerID:        "alice",
			ConversationID:  "conv-1",
			CounterpartID:   "bob",
			PreviewStatusID: "status-preview",
			Unread:          true,
		},
	}

	prefetch := &conversationAPIPrefetch{
		accountsByUsername: map[string]*storage.Account{
			"bob": {
				User: &storage.User{
					Username:    "bob",
					DisplayName: "Bob",
				},
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/bob", Type: activitypub.PersonType},
					PreferredUsername: "bob",
					Name:              "Bob",
				},
			},
		},
		statusesByID: map[string]*storageModels.Status{
			"status-preview": {
				StatusID:       "status-preview",
				AuthorUsername: "bob",
				AuthorID:       "https://example.com/users/bob",
				Content:        "batched preview",
				CreatedAt:      now,
				UpdatedAt:      now,
				PublishedAt:    now,
			},
		},
	}

	apiConversation, err := handler.convertConversationToAPIWithPrefetch(context.Background(), conversation, "alice", prefetch)
	require.NoError(t, err)
	require.Equal(t, "conv-1", apiConversation.ID)
	require.True(t, apiConversation.Unread)
	require.Len(t, apiConversation.Accounts, 1)
	require.Equal(t, "bob", apiConversation.Accounts[0].Username)
	require.NotNil(t, apiConversation.LastStatus)
	require.Equal(t, "status-preview", apiConversation.LastStatus.ID)
}
