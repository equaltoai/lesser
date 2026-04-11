package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestAgentOwnedByPrincipalRound14(t *testing.T) {
	h := &Handler{cfg: round11TestConfig()}

	require.False(t, h.agentOwnedByPrincipal(nil, "alice"))
	require.False(t, h.agentOwnedByPrincipal(&storage.User{IsAgent: false}, "alice"))
	require.False(t, h.agentOwnedByPrincipal(&storage.User{IsAgent: true}, ""))
	require.False(t, h.agentOwnedByPrincipal(&storage.User{IsAgent: true, AgentOwner: ""}, "alice"))

	require.True(t, h.agentOwnedByPrincipal(&storage.User{
		IsAgent:    true,
		AgentOwner: h.cfg.ActorURL("alice"),
	}, "alice"))

	require.True(t, h.agentOwnedByPrincipal(&storage.User{
		IsAgent:    true,
		AgentOwner: "@alice",
	}, "alice"))

	require.True(t, h.agentOwnedByPrincipal(&storage.User{
		IsAgent:    true,
		AgentOwner: "https://example.com/users/alice",
	}, "alice"))

	require.False(t, h.agentOwnedByPrincipal(&storage.User{
		IsAgent:    true,
		AgentOwner: "@bob",
	}, "alice"))
}
