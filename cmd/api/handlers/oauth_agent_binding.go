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

// agentRefreshGrantAuthorized reports whether an actor-scoped agent-subject refresh
// token's stored authorizing principal may still obtain new tokens. Owners are
// authorized by ownership and skip the grant check (preserving the owner path);
// non-owners must hold an active share grant, read uncached and strongly-consistent
// so a revocation blocks the next refresh rather than the next authorization.
func (h *Handler) agentRefreshGrantAuthorized(ctx context.Context, agentUsername, principalUsername string) (bool, error) {
	agentUser, err := h.repos.Account().GetUser(ctx, agentUsername)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return false, nil
		}
		return false, errors.Join(errors.New("agent refresh subject lookup failed"), err)
	}
	if agentUser == nil || !agentUser.IsAgent {
		return false, nil
	}
	if h.agentOwnedByPrincipal(agentUser, principalUsername) {
		return true, nil
	}
	return h.agentShareGrantActive(ctx, agentUsername, principalUsername)
}
