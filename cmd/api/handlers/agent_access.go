package handlers

import (
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

const (
	agentAccessRelationshipOwner   = "owner"
	agentAccessRelationshipGrantee = "grantee"
)

// HandleGetAgentAccessLift handles GET /api/v1/agents/{username}/access.
//
// It resolves whether the authorizing human carried in the presented token's
// DelegatedBy claim is the owner of, or an active grantee on, the named agent.
// The token subject must be the named agent itself (a caller may only ask about
// itself), and the principal is derived solely from DelegatedBy, never from
// client input. Every negative path — unknown actor, non-agent target, blank
// DelegatedBy, revoked grant, or storage error — returns a uniform 403 with no
// ownership or grant detail, so an unauthorized caller cannot probe the share
// list or learn the agent's owner.
func (h *Handler) HandleGetAgentAccessLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if resp != nil || err != nil {
		return resp, err
	}

	agentUsername := strings.ToLower(strings.TrimSpace(ctx.Param("username")))
	if err := common.ValidateUsernameParamID(agentUsername); err != nil {
		return common.RespondForbidden(ctx, "not authorized")
	}

	// A caller may only ask about the agent that is the token subject.
	if !strings.EqualFold(strings.TrimSpace(claims.Username), agentUsername) {
		return common.RespondForbidden(ctx, "not authorized")
	}

	// The authorizing human is carried in DelegatedBy, never the subject.
	principal := strings.TrimSpace(claims.DelegatedBy)
	if principal == "" {
		return common.RespondForbidden(ctx, "not authorized")
	}
	principalUsername := strings.TrimPrefix(principal, "@")
	if strings.TrimSpace(principalUsername) == "" {
		return common.RespondForbidden(ctx, "not authorized")
	}

	agentUser, err := h.repos.Account().GetUser(ctx.Context(), agentUsername)
	if err != nil || agentUser == nil || !agentUser.IsAgent || agentUser.Suspended {
		return common.RespondForbidden(ctx, "not authorized")
	}

	if h.agentOwnedByPrincipal(agentUser, principalUsername) {
		return okJSON(apimodels.AgentAccessResponse{
			Actor:        agentUsername,
			Relationship: agentAccessRelationshipOwner,
			Authorized:   true,
			ActedBy:      principalUsername,
		})
	}

	// Non-owners must hold an active share grant. The read is the uncached,
	// strongly consistent base-table check so revocation takes effect on the
	// very next request. Any error fails closed.
	active, err := h.agentShareGrantActive(ctx.Context(), agentUsername, principalUsername)
	if err != nil || !active {
		return common.RespondForbidden(ctx, "not authorized")
	}

	return okJSON(apimodels.AgentAccessResponse{
		Actor:        agentUsername,
		Relationship: agentAccessRelationshipGrantee,
		Authorized:   true,
		ActedBy:      principalUsername,
	})
}
