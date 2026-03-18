package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
)

var (
	errOAuthAgentUsernameRequired = errors.New("agent_username is required for agent clients")
	errOAuthAgentNotFound         = errors.New("agent_username must reference an existing agent")
	errOAuthAgentForbidden        = errors.New("not authorized to bind this agent")
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

func (h *Handler) getAgentUserForOAuthClient(ctx context.Context, client *storage.OAuthClient, principalUsername string) (*storage.User, error) {
	if client == nil || strings.ToLower(strings.TrimSpace(client.ClientClass)) != "agent" {
		return nil, nil
	}

	agentUsername := strings.TrimSpace(client.AgentUsername)
	if err := common.ValidateRequiredParam("agent_username", agentUsername); err != nil {
		return nil, errOAuthAgentUsernameRequired
	}
	if err := common.ValidateUsernameParamID(agentUsername); err != nil {
		return nil, errOAuthAgentNotFound
	}

	agentUser, err := h.repos.Account().GetUser(ctx, agentUsername)
	if err != nil || agentUser == nil || !agentUser.IsAgent {
		return nil, errOAuthAgentNotFound
	}
	if !h.agentOwnedByPrincipal(agentUser, principalUsername) {
		return nil, errOAuthAgentForbidden
	}

	return agentUser, nil
}
