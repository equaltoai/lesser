package services

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestRegistryAgentShareActorURL(t *testing.T) {
	require.Empty(t, (*Registry)(nil).agentShareActorURL("alice"))
	require.Empty(t, (&Registry{}).agentShareActorURL("alice"))

	withConfig := &Registry{config: &ServiceConfig{Config: &config.Config{Domain: "example.com"}}}
	require.Equal(t, "https://example.com/users/alice", withConfig.agentShareActorURL("alice"))

	withBaseURL := &Registry{config: &ServiceConfig{BaseURL: "https://example.net/"}}
	require.Equal(t, "https://example.net/users/alice", withBaseURL.agentShareActorURL("alice"))
}
