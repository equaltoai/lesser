package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMyAgentsViewerIsOwnerUsesCanonicalOwnerMatchAndExcludesSharedAgents(t *testing.T) {
	resolver, graphStorage := newRound12GraphResolver(t)
	resolver.Config.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true
	require.NoError(t, graphStorage.Instance().SetAgentInstanceConfig(context.Background(), policy))

	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	ownedAtForm := &storage.User{
		Username:     "alpha",
		DisplayName:  "Alpha",
		IsAgent:      true,
		AgentType:    "ASSISTANT",
		AgentVersion: "1.0.0",
		AgentOwner:   "@Alice",
		CreatedAt:    now,
	}
	seedGraphAgentContractUser(graphStorage, ownedAtForm)
	ownedPlainForm := &storage.User{
		Username:     "beta",
		DisplayName:  "Beta",
		IsAgent:      true,
		AgentType:    "ASSISTANT",
		AgentVersion: "1.0.0",
		AgentOwner:   "ALICE",
		CreatedAt:    now.Add(time.Minute),
	}
	seedGraphAgentContractUser(graphStorage, ownedPlainForm)
	sharedWithViewer := &storage.User{
		Username:     "shared-helper",
		DisplayName:  "Shared Helper",
		IsAgent:      true,
		AgentType:    "ASSISTANT",
		AgentVersion: "1.0.0",
		AgentOwner:   "@Bob",
		CreatedAt:    now.Add(2 * time.Minute),
	}
	seedGraphAgentContractUser(graphStorage, sharedWithViewer)

	userRepo := storagemocks.NewMockUserRepositoryInterface()
	userRepo.On("ListAgents", mock.Anything, int32(100), "").Return([]*storage.User{
		ownedAtForm,
		ownedPlainForm,
		sharedWithViewer,
	}, "", nil).Once()
	userRepo.On("GetUser", mock.Anything, "shared-helper").Return(sharedWithViewer, nil).Once()
	resolver.Storage = &agentOwnershipTestStorage{
		round12GraphStorage: graphStorage,
		userRepo:            userRepo,
	}

	grant := &storagemodels.AgentShareGrant{
		AgentUsername:   "shared-helper",
		GranteeUsername: "alice",
		GrantedBy:       "bob",
		GrantedAt:       now,
	}
	require.NoError(t, grant.UpdateKeys())
	graphStorage.SeedAgentShareGrant(grant)

	ctx := delegatedAgentAuthContext("alice", auth.ScopeRead)
	ownedAgents, err := resolver.Query().MyAgents(ctx)
	require.NoError(t, err)
	require.Len(t, ownedAgents, 2)

	gotUsernames := make([]string, 0, len(ownedAgents))
	for _, agent := range ownedAgents {
		require.NotNil(t, agent)
		require.True(t, agent.ViewerCanSeePrivateFields)
		require.True(t, agent.ViewerIsOwner)
		gotUsernames = append(gotUsernames, agent.Username)
	}
	require.ElementsMatch(t, []string{"alpha", "beta"}, gotUsernames)

	sharedAgent, err := resolver.Query().Agent(ctx, "shared-helper")
	require.NoError(t, err)
	require.NotNil(t, sharedAgent)
	require.False(t, sharedAgent.ViewerIsOwner)
	require.False(t, sharedAgent.ViewerCanSeePrivateFields)
	require.Nil(t, sharedAgent.AgentOwner)
	require.NotNil(t, sharedAgent.McpAccess)

	userRepo.AssertExpectations(t)
}

type agentOwnershipTestStorage struct {
	*round12GraphStorage
	userRepo interfaces.UserRepository
}

func (s *agentOwnershipTestStorage) User() interfaces.UserRepository {
	return s.userRepo
}

func seedGraphAgentContractUser(graphStorage *round12GraphStorage, user *storage.User) {
	graphStorage.SeedAgentGovernanceState(&storage.AgentGovernanceState{
		Username:  user.Username,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.CreatedAt,
	})
}
