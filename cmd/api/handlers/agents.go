package handlers

import (
	"net/http"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

type agentDelegationRequest struct {
	AgentUsername string   `json:"agent_username"`
	DisplayName   string   `json:"display_name"`
	Bio           string   `json:"bio"`
	Scopes        []string `json:"scopes"`
	ExpiresIn     int      `json:"expires_in"`
	AgentInfo     any      `json:"agent_info,omitempty"`
}

func (h *Handler) ensureAgentsEnabled(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h == nil || h.cfg == nil || !h.cfg.AllowAgents {
		return common.RespondForbidden(ctx, "agents are disabled")
	}

	if h.repos != nil && h.repos.Instance() != nil {
		policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context())
		if err != nil {
			return common.RespondInternalServerError(ctx)
		}
		if policy == nil || !policy.AllowAgents {
			return common.RespondForbidden(ctx, "agents are disabled by instance policy")
		}
	}

	return nil, nil
}

func (h *Handler) ensureAgentRegistrationEnabled(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}
	if h.cfg == nil || !h.cfg.AllowAgentRegistration {
		return common.RespondForbidden(ctx, "agent registration is disabled")
	}

	if h.repos != nil && h.repos.Instance() != nil {
		policy, err := h.repos.Instance().GetAgentInstanceConfig(ctx.Context())
		if err != nil {
			return common.RespondInternalServerError(ctx)
		}
		if policy == nil || !policy.AllowAgentRegistration {
			return common.RespondForbidden(ctx, "agent registration is disabled by instance policy")
		}
	}

	return nil, nil
}

// HandleDelegateAgentLift handles POST /api/v1/agents/delegate.
//
// M0: contract stub. Full implementation lands in M2.
func (h *Handler) HandleDelegateAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentRegistrationEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	_, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeWrite)
		}
		return common.RespondUnauthorized(ctx)
	}

	var req agentDelegationRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Email is intentionally not accepted anywhere in this flow (Lesser is email-free).
	return apptheory.JSON(http.StatusNotImplemented, map[string]any{
		"error":             "not implemented",
		"error_description": "agent delegation is not implemented yet",
	})
}

// HandleListAgentsLift handles GET /api/v1/agents.
//
// M0: contract stub. Full implementation lands in M2.
func (h *Handler) HandleListAgentsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	return okJSON([]any{})
}

// HandleGetAgentLift handles GET /api/v1/agents/:username.
//
// M0: contract stub. Full implementation lands in M2.
func (h *Handler) HandleGetAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := ctx.Param("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	return apptheory.JSON(http.StatusNotImplemented, map[string]any{
		"error":             "not implemented",
		"error_description": "agent lookup is not implemented yet",
	})
}

// HandleUpdateAgentLift handles PATCH /api/v1/agents/:username.
//
// M0: contract stub. Full implementation lands in M2.
func (h *Handler) HandleUpdateAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	_, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeWrite)
		}
		return common.RespondUnauthorized(ctx)
	}

	return apptheory.JSON(http.StatusNotImplemented, map[string]any{
		"error":             "not implemented",
		"error_description": "agent update is not implemented yet",
	})
}

// HandleDeleteAgentLift handles DELETE /api/v1/agents/:username.
//
// M0: contract stub. Full implementation lands in M2.
func (h *Handler) HandleDeleteAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	_, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeWrite)
		}
		return common.RespondUnauthorized(ctx)
	}

	return apptheory.JSON(http.StatusNotImplemented, map[string]any{
		"error":             "not implemented",
		"error_description": "agent deletion is not implemented yet",
	})
}

// HandleGetAgentActivityLift handles GET /api/v1/agents/:username/activity.
//
// M0: contract stub. Full implementation lands in M3.
func (h *Handler) HandleGetAgentActivityLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	_, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeRead)
		}
		return common.RespondUnauthorized(ctx)
	}

	return apptheory.JSON(http.StatusNotImplemented, map[string]any{
		"error":             "not implemented",
		"error_description": "agent activity log is not implemented yet",
	})
}

// HandleSuspendAgentLift handles POST /api/v1/agents/:username/suspend.
//
// M0: contract stub. Full implementation lands in M3.
func (h *Handler) HandleSuspendAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	_, err := h.authenticateWithScope(ctx, auth.ScopeAdmin)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeAdmin)
		}
		return common.RespondUnauthorized(ctx)
	}

	return apptheory.JSON(http.StatusNotImplemented, map[string]any{
		"error":             "not implemented",
		"error_description": "agent suspension is not implemented yet",
	})
}
