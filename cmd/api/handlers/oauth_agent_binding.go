package handlers

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
)

func (h *Handler) agentOwnerMatchesLocalPrincipal(owner string, principalUsername string) bool {
	principalUsername = strings.TrimSpace(principalUsername)
	owner = strings.TrimSpace(owner)
	if principalUsername == "" || owner == "" {
		return false
	}

	lowerOwner := strings.ToLower(owner)
	if strings.HasPrefix(lowerOwner, "http://") || strings.HasPrefix(lowerOwner, "https://") {
		return h != nil && h.cfg != nil && strings.EqualFold(owner, h.cfg.ActorURL(principalUsername))
	}

	owner = strings.TrimPrefix(owner, "@")
	if strings.Contains(owner, "/") {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(owner), principalUsername)
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
