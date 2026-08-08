package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAgentsFiltersBeforePaginationAndMarksPrivateRedaction(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	users := []*storage.User{
		{Username: "a", DisplayName: "A", IsAgent: true, AgentType: "ASSISTANT", AgentOwner: "@owner", CreatedAt: base},
		{Username: "b", DisplayName: "B", IsAgent: true, AgentType: "CUSTOM", CreatedAt: base.Add(time.Minute)},
		{Username: "c", DisplayName: "C", IsAgent: true, AgentType: "ASSISTANT", Suspended: true, CreatedAt: base.Add(2 * time.Minute)},
		{Username: "d", DisplayName: "D", IsAgent: true, AgentType: "ASSISTANT", CreatedAt: base.Add(3 * time.Minute)},
		{Username: "e", DisplayName: "E", IsAgent: true, AgentType: "ASSISTANT", CreatedAt: base.Add(4 * time.Minute)},
	}
	userRepo := storagemocks.NewMockUserRepositoryInterface()
	userRepo.On("ListAgents", mock.Anything, int32(100), "").Return(users, "", nil).Twice()
	store := pkgtesting.NewMockRepositoryStorage(pkgtesting.WithUserRepository(userRepo))
	resolver := &Resolver{Storage: store, Config: &config.Config{AllowAgents: true}}
	typeFilter := model.AgentTypeAssistant
	first := 2

	page, err := resolver.Query().Agents(ctx, &first, nil, &typeFilter, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 3, page.TotalCount)
	require.Len(t, page.Edges, 2)
	require.Equal(t, "a", page.Edges[0].Node.Username)
	require.Equal(t, "d", page.Edges[1].Node.Username)
	require.True(t, page.PageInfo.HasNextPage)
	require.False(t, page.Edges[0].Node.ViewerCanSeePrivateFields)
	require.Nil(t, page.Edges[0].Node.AgentOwner)

	after := page.Edges[1].Cursor
	next, err := resolver.Query().Agents(ctx, &first, &after, &typeFilter, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 3, next.TotalCount)
	require.Len(t, next.Edges, 1)
	require.Equal(t, "e", next.Edges[0].Node.Username)
	require.False(t, next.PageInfo.HasNextPage)
	userRepo.AssertExpectations(t)
}
