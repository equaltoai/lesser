package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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
		accountsByKey: map[string]*storage.Account{
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

func TestConversationAPIAccountsBackfillFollowableLocalNumericID(t *testing.T) {
	cfg := round11TestConfig()
	repos := &MockRepositoryStorage{}
	actorRepo := &accountsRound20EnsuringActorRepo{}
	repos.On("Actor").Return(actorRepo).Once()
	handler := &Handler{cfg: cfg, repos: repos, logger: zap.NewNop()}
	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/bob", Type: activitypub.PersonType},
		PreferredUsername: "bob",
	}
	conversation := &storageModels.Conversation{ID: "conv-1", Participants: []string{"alice", "bob"}}
	prefetch := &conversationAPIPrefetch{accountsByKey: map[string]*storage.Account{
		"bob": {User: &storage.User{Username: "bob"}, Actor: actor},
	}}

	accounts := handler.conversationAPIAccounts(context.Background(), conversation, "alice", prefetch)

	require.Len(t, accounts, 1)
	require.Equal(t, []string{"bob"}, actorRepo.usernames)
	repos.AssertExpectations(t)
}

func TestRound11_ConvertConversationToAPIWithPrefetch_FallsBackWhenPrefetchMisses(t *testing.T) {
	now := time.Date(2026, 3, 26, 11, 30, 0, 0, time.UTC)
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{
		statusByID: map[string]storageModels.Status{
			"status-preview": {
				StatusID:       "status-preview",
				AuthorUsername: "bob",
				AuthorID:       "https://example.com/users/bob",
				Content:        "loaded from point lookup",
				CreatedAt:      now,
				UpdatedAt:      now,
				PublishedAt:    now,
			},
		},
	})

	conversation := &storageModels.Conversation{
		ID:           "conv-2",
		Participants: []string{"alice", "bob"},
		Unread:       true,
		ViewerState: &storageModels.UserConversationState{
			ViewerID:        "alice",
			ConversationID:  "conv-2",
			CounterpartID:   "bob",
			PreviewStatusID: "status-preview",
			Unread:          true,
		},
	}

	apiConversation, err := handler.convertConversationToAPIWithPrefetch(context.Background(), conversation, "alice", &conversationAPIPrefetch{})
	require.NoError(t, err)
	require.Len(t, apiConversation.Accounts, 1)
	require.Equal(t, "bob", apiConversation.Accounts[0].Username)
	require.NotNil(t, apiConversation.LastStatus)
	require.Equal(t, "status-preview", apiConversation.LastStatus.ID)
}

func TestRound11_ConversationAPIAccountForParticipant_UsesDerivedFederatedUsername(t *testing.T) {
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})

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

	found := handler.conversationAPIAccountForParticipant(context.Background(), remoteID, &conversationAPIPrefetch{
		accountsByKey: map[string]*storage.Account{
			"bob": account,
		},
	})
	require.Same(t, account, found)
}
