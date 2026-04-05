package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestConvertStorageUserToAgent_HydratesMetadataAndCapabilities(t *testing.T) {
	resolver := &Resolver{Config: &config.Config{Domain: "example.com"}}
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
		Username:             "lowkey",
		Verified:             true,
		VerifiedAt:           ptrGraphTime(time.Date(2026, 2, 13, 13, 56, 1, 0, time.UTC)),
		DelegatedScopes:      []string{"read"},
		QuarantineStatus:     storage.AgentQuarantineStatusApproved,
		QuarantineStart:      ptrGraphTime(time.Date(2026, 2, 13, 12, 42, 10, 0, time.UTC)),
		QuarantineEnd:        ptrGraphTime(time.Date(2026, 2, 20, 12, 42, 10, 0, time.UTC)),
		QuarantineApprovedBy: "owner",
		QuarantineApprovedAt: ptrGraphTime(time.Date(2026, 2, 14, 8, 0, 0, 0, time.UTC)),
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
	require.NotNil(t, agent.QuarantineStatus)
	require.Equal(t, storage.AgentQuarantineStatusApproved, *agent.QuarantineStatus)
	require.NotNil(t, agent.QuarantineApprovedBy)
	require.Equal(t, "owner", *agent.QuarantineApprovedBy)
	require.NotNil(t, agent.QuarantineApprovedAt)
	require.NotNil(t, agent.QuarantineStart)
	require.NotNil(t, agent.QuarantineEnd)
	require.False(t, agent.QuarantineActive)
	require.NotNil(t, agent.McpAccess)
	bundle := auth.BuildPublicMCPAccessBundle("https://example.com", "lowkey")
	require.Equal(t, bundle.MCPURL, agent.McpAccess.McpURL)
	require.Equal(t, bundle.ProtectedResourceURL, agent.McpAccess.ProtectedResourceURL)
	require.Equal(t, bundle.AuthorizationServerURL, agent.McpAccess.AuthorizationServerURL)
	require.Equal(t, bundle.RegistrationURL, agent.McpAccess.RegistrationURL)
	require.Len(t, agent.McpAccess.Guidance, 5)
}

func TestConvertStorageUserToAgent_HidesVerifiedAtWhenAgentIsNotVerified(t *testing.T) {
	resolver := &Resolver{}
	user := &storage.User{
		Username: "lowkey",
		IsAgent:  true,
	}
	governance := &storage.AgentGovernanceState{
		Username:   "lowkey",
		Verified:   false,
		VerifiedAt: ptrGraphTime(time.Date(2026, 2, 13, 13, 56, 1, 0, time.UTC)),
	}

	agent := resolver.convertStorageUserToAgent(user, governance)
	require.NotNil(t, agent)
	require.False(t, agent.Verified)
	require.Nil(t, agent.VerifiedAt)
}

func TestConvertStorageUserToAgent_NormalizesExpiredQuarantine(t *testing.T) {
	resolver := &Resolver{}
	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	user := &storage.User{
		Username: "lowkey",
		IsAgent:  true,
	}
	governance := &storage.AgentGovernanceState{
		Username:         "lowkey",
		QuarantineStatus: storage.AgentQuarantineStatusQuarantined,
		QuarantineEnd:    &past,
	}

	agent := resolver.convertStorageUserToAgent(user, governance)
	require.NotNil(t, agent)
	require.NotNil(t, agent.QuarantineStatus)
	require.Equal(t, storage.AgentQuarantineStatusExpired, *agent.QuarantineStatus)
	require.False(t, agent.QuarantineActive)
	require.NotNil(t, agent.QuarantineEnd)
	require.Equal(t, past.UTC(), time.Time(*agent.QuarantineEnd))
}

func ptrGraphTime(value time.Time) *time.Time {
	timestamp := value.UTC()
	return &timestamp
}
