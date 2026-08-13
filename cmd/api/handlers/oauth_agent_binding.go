package handlers

import (
	"context"
	"errors"
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

// agentShareGrantActive reports whether an active (non-revoked) share grant
// authorizes principalUsername on agentUsername. The read is the uncached,
// strongly-consistent direct base-table check from the agentshare service; a
// service or storage error is returned so callers fail closed rather than
// authorizing on an error path.
func (h *Handler) agentShareGrantActive(ctx context.Context, agentUsername, principalUsername string) (bool, error) {
	service := h.agentShareService()
	if service == nil {
		return false, errors.New("agent share service unavailable")
	}
	return service.IsActive(ctx, agentUsername, principalUsername)
}
