package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestConvertStorageUserToAgent_HydratesMetadataAndCapabilities(t *testing.T) {
	resolver := &Resolver{}
	createdAt := time.Date(2026, 2, 13, 12, 42, 10, 0, time.UTC)

	user := &storage.User{
		Username:     "lowkey",
		DisplayName:  "Lowkey",
		Note:         "bio",
		IsAgent:      true,
		AgentType:    "assistant",
		AgentVersion: "1.0.0",
		AgentOwner:   "@alice",
		AgentCapabilities: &agents.Capabilities{
			CanPost:          true,
			CanDM:            true,
			MaxPostsPerHour:  0,
			RequiresApproval: true,
		},
		Metadata: map[string]any{
			"agent_verified":         true,
			"agent_verified_at":      "2026-02-13T13:56:01Z",
			"agent_delegated_scopes": []any{"read"},
		},
		CreatedAt: createdAt,
	}

	agent := resolver.convertStorageUserToAgent(context.Background(), user)
	require.NotNil(t, agent)
	require.Equal(t, "lowkey", agent.Username)
	require.True(t, agent.Verified)
	require.Contains(t, agent.DelegatedScopes, "read")
	require.NotNil(t, agent.AgentCapabilities)
	require.True(t, agent.AgentCapabilities.CanPost)
	require.True(t, agent.AgentCapabilities.CanDM)
	require.Equal(t, 0, agent.AgentCapabilities.MaxPostsPerHour)
	require.True(t, agent.AgentCapabilities.RequiresApproval)
}
