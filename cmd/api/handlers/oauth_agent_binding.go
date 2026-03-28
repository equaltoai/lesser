package handlers

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
)

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

	if h != nil && h.cfg != nil {
		normalizedOwner := h.normalizeDelegatedByActorURI(owner)
		if strings.EqualFold(normalizedOwner, h.cfg.ActorURL(principalUsername)) {
			return true
		}
	}

	owner = strings.TrimPrefix(owner, "@")
	if idx := strings.LastIndex(owner, "/users/"); idx >= 0 {
		owner = owner[idx+len("/users/"):]
	}
	return strings.EqualFold(strings.TrimSpace(owner), principalUsername)
}
