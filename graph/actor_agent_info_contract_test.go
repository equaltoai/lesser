package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestActorAgentInfoAppliesPrivateFieldPolicy(t *testing.T) {
	user := &storage.User{
		Username:    "review-bot",
		DisplayName: "Review Bot",
		IsAgent:     true,
		AgentOwner:  "@owner",
	}
	userRepo := storagemocks.NewMockUserRepositoryInterface()
	userRepo.On("GetUser", mock.Anything, "review-bot").Return(user, nil).Times(3)
	store := pkgtesting.NewMockRepositoryStorage(pkgtesting.WithUserRepository(userRepo))
	resolver := &Resolver{Storage: store, Config: &config.Config{Domain: "example.com"}}
	actorResolver := &actorResolver{resolver}
	actor := &activitypub.Actor{PreferredUsername: "review-bot"}

	publicAgent, err := actorResolver.AgentInfo(context.Background(), actor)
	require.NoError(t, err)
	require.False(t, publicAgent.ViewerCanSeePrivateFields)
	require.Nil(t, publicAgent.AgentOwner)
	require.Empty(t, publicAgent.DelegatedScopes)

	nonOwnerCtx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{Username: "someone-else"})
	nonOwnerAgent, err := actorResolver.AgentInfo(nonOwnerCtx, actor)
	require.NoError(t, err)
	require.False(t, nonOwnerAgent.ViewerCanSeePrivateFields)
	require.Nil(t, nonOwnerAgent.AgentOwner)

	ownerCtx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{Username: "owner"})
	ownerAgent, err := actorResolver.AgentInfo(ownerCtx, actor)
	require.NoError(t, err)
	require.True(t, ownerAgent.ViewerCanSeePrivateFields)
	require.NotNil(t, ownerAgent.AgentOwner)
	require.Equal(t, "@owner", *ownerAgent.AgentOwner)

	userRepo.AssertExpectations(t)
}
