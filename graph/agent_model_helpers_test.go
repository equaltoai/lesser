package graph

import (
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
		CreatedAt: createdAt,
	}
	governance := &storage.AgentGovernanceState{
		Username:        "lowkey",
		Verified:        true,
		VerifiedAt:      ptrGraphTime(time.Date(2026, 2, 13, 13, 56, 1, 0, time.UTC)),
		DelegatedScopes: []string{"read"},
	}

	agent := resolver.convertStorageUserToAgent(user, governance)
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

func ptrGraphTime(value time.Time) *time.Time {
	timestamp := value.UTC()
	return &timestamp
}
