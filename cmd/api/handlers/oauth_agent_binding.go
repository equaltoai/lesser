package handlers

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
)

func (h *Handler) agentOwnerMatchesLocalPrincipal(owner string, principalUsername string) bool {
	if h == nil || h.cfg == nil {
		return auth.AgentOwnerMatchesLocalPrincipal(owner, principalUsername, nil)
	}
	return auth.AgentOwnerMatchesLocalPrincipal(owner, principalUsername, h.cfg.ActorURL)
}

func (h *Handler) agentOwnedByPrincipal(agentUser *storage.User, principalUsername string) bool {
	if agentUser == nil || !agentUser.IsAgent {
		return false
	}

	principalUsername = strings.TrimSpace(principalUsername)
	if principalUsername == "" {
		return false
	}

	owner := strings.TrimSpace(agentUser.AgentOwner)
	if owner == "" {
		return false
	}

	return h.agentOwnerMatchesLocalPrincipal(owner, principalUsername)
}
