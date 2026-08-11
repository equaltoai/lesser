package handlers

import (
	"errors"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/agentshare"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	tabletheoryerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

// HandleGrantAgentShareLift handles PUT /api/v1/agents/{username}/share/{grantee}.
func (h *Handler) HandleGrantAgentShareLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}
	claims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return resp, err
	}
	agentUsername, granteeUsername, resp, err := validateAgentSharePath(ctx)
	if resp != nil || err != nil {
		return resp, err
	}
	service := h.agentShareService()
	if service == nil {
		return common.RespondInternalServerError(ctx)
	}
	grant, err := service.Grant(ctx.Context(), agentshare.ManageInput{
		AgentUsername:   agentUsername,
		GranteeUsername: granteeUsername,
		ActorUsername:   claims.Username,
		ActorIsAdmin:    agentShareAdmin(claims),
	})
	if err != nil {
		return respondAgentShareError(ctx, err)
	}
	return okJSON(agentShareGrantFromStorage(grant))
}

// HandleRevokeAgentShareLift handles DELETE /api/v1/agents/{username}/share/{grantee}.
func (h *Handler) HandleRevokeAgentShareLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}
	claims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return resp, err
	}
	agentUsername, granteeUsername, resp, err := validateAgentSharePath(ctx)
	if resp != nil || err != nil {
		return resp, err
	}
	service := h.agentShareService()
	if service == nil {
		return common.RespondInternalServerError(ctx)
	}
	grant, err := service.Revoke(ctx.Context(), agentshare.ManageInput{
		AgentUsername:   agentUsername,
		GranteeUsername: granteeUsername,
		ActorUsername:   claims.Username,
		ActorIsAdmin:    agentShareAdmin(claims),
	})
	if err != nil {
		return respondAgentShareError(ctx, err)
	}
	return okJSON(agentShareGrantFromStorage(grant))
}

// HandleListAgentSharesLift handles GET /api/v1/agents/{username}/share.
func (h *Handler) HandleListAgentSharesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}
	claims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return resp, err
	}
	agentUsername := strings.ToLower(strings.TrimSpace(ctx.Param("username")))
	if err := common.ValidateUsernameParamID(agentUsername); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	service := h.agentShareService()
	if service == nil {
		return common.RespondInternalServerError(ctx)
	}
	grants, err := service.ListByAgent(ctx.Context(), agentUsername, claims.Username, agentShareAdmin(claims))
	if err != nil {
		return respondAgentShareError(ctx, err)
	}
	return okJSON(agentShareGrantListFromStorage(grants))
}

// HandleListAgentsSharedWithMeLift handles GET /api/v1/agents/shared-with-me.
func (h *Handler) HandleListAgentsSharedWithMeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeRead)
		}
		return common.RespondUnauthorized(ctx)
	}
	service := h.agentShareService()
	if service == nil {
		return common.RespondInternalServerError(ctx)
	}
	grants, err := service.ListSharedWith(ctx.Context(), claims.Username)
	if err != nil {
		return respondAgentShareError(ctx, err)
	}
	return okJSON(agentShareGrantListFromStorage(grants))
}

func (h *Handler) agentShareService() AgentShareService {
	if h == nil || h.registry == nil {
		return nil
	}
	return h.registry.AgentShares()
}

func validateAgentSharePath(ctx *apptheory.Context) (string, string, *apptheory.Response, error) {
	agentUsername := strings.ToLower(strings.TrimSpace(ctx.Param("username")))
	if err := common.ValidateUsernameParamID(agentUsername); err != nil {
		resp, respErr := common.RespondValidationError(ctx, err)
		return "", "", resp, respErr
	}
	granteeUsername := strings.ToLower(strings.TrimSpace(ctx.Param("grantee")))
	if err := common.ValidateUsernameParamID(granteeUsername); err != nil {
		resp, respErr := common.RespondValidationError(ctx, common.ValidationError{Field: "grantee", Message: err.Error()})
		return "", "", resp, respErr
	}
	return agentUsername, granteeUsername, nil, nil
}

func agentShareAdmin(claims *auth.Claims) bool {
	return claims != nil && (claims.HasScope(auth.ScopeAdmin) || claims.HasScope("admin:write") || claims.HasScope("admin:all"))
}

func respondAgentShareError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
	switch {
	case errors.Is(err, agentshare.ErrNotAuthorized):
		return common.RespondForbidden(ctx, "not authorized to manage agent share list")
	case errors.Is(err, agentshare.ErrAgentNotFound):
		return common.RespondNotFound(ctx, "agent")
	case errors.Is(err, agentshare.ErrGranteeNotFound):
		return common.RespondNotFound(ctx, "grantee account")
	case errors.Is(err, agentshare.ErrGrantNotFound):
		return common.RespondNotFound(ctx, "agent share grant")
	case errors.Is(err, agentshare.ErrInvalidGrantee), errors.Is(err, agentshare.ErrSelfGrant), errors.Is(err, agentshare.ErrAgentSelfGrant):
		return common.RespondUnprocessableEntity(ctx, err.Error())
	case tabletheoryerrors.IsConditionFailed(err):
		return common.RespondConflict(ctx, "agent share grant changed concurrently")
	default:
		return common.RespondInternalServerError(ctx)
	}
}

func agentShareGrantFromStorage(grant *storagemodels.AgentShareGrant) apimodels.AgentShareGrant {
	if grant == nil {
		return apimodels.AgentShareGrant{}
	}
	return apimodels.AgentShareGrant{
		AgentUsername:   grant.AgentUsername,
		GranteeUsername: grant.GranteeUsername,
		GrantedBy:       grant.GrantedBy,
		GrantedAt:       grant.GrantedAt,
		RevokedAt:       grant.RevokedAt,
		RevokedBy:       grant.RevokedBy,
		Active:          grant.RevokedAt == nil,
	}
}

func agentShareGrantListFromStorage(grants []*storagemodels.AgentShareGrant) apimodels.AgentShareGrantListResponse {
	items := make([]apimodels.AgentShareGrant, 0, len(grants))
	for _, grant := range grants {
		if grant != nil {
			items = append(items, agentShareGrantFromStorage(grant))
		}
	}
	return apimodels.AgentShareGrantListResponse{Grants: items}
}
