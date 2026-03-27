package conversations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type dmHydrationFixture struct {
	User dmHydrationFixtureUser `json:"user"`
}

type dmHydrationFixtureUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Note        string `json:"note"`
	URL         string `json:"url"`
	IsAgent     bool   `json:"isAgent"`
	AgentType   string `json:"agentType"`
}

func TestService_SendDirectMessage_LiveShapedCoreAccountsDoNotNeedMetadata(t *testing.T) {
	service, conversationRepo, _, accountRepo, _, federation := createTestService()
	ctx := context.Background()
	fixtures := loadDirectMessageHydrationFixtures(t)

	sender := fixtures["medic"].toAccount()
	recipient := fixtures["pilot"].toAccount()

	accountRepo.On("GetAccount", ctx, "medic").Return(sender, nil).Once()
	accountRepo.On("GetAccount", ctx, "pilot").Return(recipient, nil).Once()
	conversationRepo.On("GetConversationByParticipants", ctx, []string{"medic", "pilot"}).
		Return((*models.Conversation)(nil), fmt.Errorf("not found")).
		Once()
	conversationRepo.On("ApplyDirectMessageSend", ctx, mock.MatchedBy(func(transition *models.DirectMessageSendTransition) bool {
		if transition == nil || transition.Status == nil || len(transition.ParticipantStates) != 2 {
			return false
		}
		return transition.Status.AuthorID == "medic" && transition.Status.Visibility == VisibilityDirect
	}), mock.Anything).Return(nil).Once()

	result, err := service.SendDirectMessage(ctx, &SendDirectMessageCommand{
		SenderID:   "medic",
		Recipients: []string{"pilot"},
		Content:    "fixture-backed direct message",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Message)
	require.Equal(t, "medic", result.Message.AuthorID)
	require.Nil(t, sender.User.Metadata)
	require.Nil(t, recipient.User.Metadata)
	require.Empty(t, federation.GetQueuedActivities())

	accountRepo.AssertExpectations(t)
	conversationRepo.AssertExpectations(t)
}

func loadDirectMessageHydrationFixtures(t *testing.T) map[string]dmHydrationFixture {
	t.Helper()

	path := filepath.Join("..", "..", "..", "testdata", "account_hydration", "live_agents.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var fixtures map[string]dmHydrationFixture
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	require.Contains(t, fixtures, "medic")
	require.Contains(t, fixtures, "pilot")
	return fixtures
}

func (f dmHydrationFixture) toAccount() *storage.Account {
	return &storage.Account{
		User: &storage.User{
			Username:    f.User.Username,
			DisplayName: f.User.DisplayName,
			Note:        f.User.Note,
			URL:         f.User.URL,
			IsAgent:     f.User.IsAgent,
			AgentType:   f.User.AgentType,
			Metadata:    nil,
		},
	}
}
