package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

type soulHandlerService interface {
	ListMine(ctx context.Context, username string) ([]soulservice.Soul, error)
	Incorporate(ctx context.Context, principalUsername string, targetAgentUsername string, soulAgentID string) (*soulservice.Soul, error)
	ResolveBoundAgent(ctx context.Context, agentUsername string) (*soulservice.Soul, error)
}

// HandleGetMySoulsLift returns souls owned by the authenticated user's linked wallet(s) for this instance.
func (h *Handler) HandleGetMySoulsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithAnyScope(ctx, auth.ScopeRead, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	svc := h.getSoulService()
	if svc == nil {
		return common.RespondInternalServerError(ctx)
	}

	souls, err := svc.ListMine(ctx.Context(), claims.Username)
	if err != nil {
		return h.respondSoulServiceError(ctx, err)
	}

	items := make([]apimodels.SoulInventoryItem, 0, len(souls))
	for _, soul := range souls {
		items = append(items, toAPISoulInventoryItem(soul))
	}

	return okJSON(apimodels.SoulsMineResponse{
		Souls: items,
		Count: len(items),
	})
}

// HandleGetBoundSoulMeLift returns the active soul bound to the authenticated local runtime-agent principal.
func (h *Handler) HandleGetBoundSoulMeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithAnyScope(ctx, auth.ScopeRead, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	svc := h.getSoulService()
	if svc == nil {
		return common.RespondInternalServerError(ctx)
	}

	soul, err := svc.ResolveBoundAgent(ctx.Context(), claims.Username)
	if err != nil {
		if errors.Is(err, soulservice.ErrSoulNotAvailable) {
			return respondBoundSoulNotFound("soul_not_available", "bound soul is not available")
		}
		return h.respondSoulServiceError(ctx, err)
	}
	if soul == nil || !soul.Bound {
		return respondBoundSoulNotFound("soul_not_bound", "no bound soul for authenticated principal")
	}

	item := toAPISoulInventoryItem(*soul)
	if item.Binding == nil || item.BindingState != "bound" {
		return respondBoundSoulNotFound("soul_not_bound", "no bound soul for authenticated principal")
	}

	return okJSON(apimodels.BoundSoulResponse{
		Agent:        item.Agent,
		BindingState: item.BindingState,
		Binding:      *item.Binding,
	})
}

// HandleIncorporateSoulLift explicitly binds a soul to a local agent chosen by the authenticated principal.
func (h *Handler) HandleIncorporateSoulLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	agentID := strings.TrimSpace(ctx.Param("agentId"))
	if agentID == "" {
		return common.RespondBadRequest(ctx, "agentId is required")
	}

	var req apimodels.SoulIncorporateRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
	}
	targetAgentUsername := strings.TrimSpace(req.TargetAgentUsername)
	if err := common.ValidateUsernameParamID(targetAgentUsername); err != nil {
		return h.respondBadRequest(ctx, "target_agent_username is required and must be a valid username")
	}

	svc := h.getSoulService()
	if svc == nil {
		return common.RespondInternalServerError(ctx)
	}

	soul, err := svc.Incorporate(ctx.Context(), claims.Username, targetAgentUsername, agentID)
	if err != nil {
		return h.respondSoulServiceError(ctx, err)
	}

	return okJSON(apimodels.SoulIncorporateResponse{
		Soul: toAPISoulInventoryItem(*soul),
	})
}

func (h *Handler) getSoulService() soulHandlerService {
	if h == nil {
		return nil
	}
	if h.soulsService != nil {
		return h.soulsService
	}
	if h.repos == nil || h.cfg == nil {
		return nil
	}
	return soulservice.NewService(h.repos.Account(), h.repos.Instance(), h.cfg, h.logger)
}

func respondBoundSoulNotFound(code string, message string) (*apptheory.Response, error) {
	return apptheory.JSON(http.StatusNotFound, common.StandardErrorResponse{
		Error:       message,
		Description: message,
		Code:        code,
	})
}

func (h *Handler) respondSoulServiceError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
	switch {
	case errors.Is(err, soulservice.ErrTrustNotConfigured):
		return common.RespondUnprocessableEntity(ctx, "trust not configured")
	case errors.Is(err, soulservice.ErrSoulNotAvailable):
		return common.RespondNotFound(ctx, "soul")
	case errors.Is(err, soulservice.ErrTargetAgentRequired):
		return common.RespondUnprocessableEntity(ctx, "target agent is required")
	case errors.Is(err, soulservice.ErrTargetAgentNotFound):
		return common.RespondNotFound(ctx, "target agent")
	case errors.Is(err, soulservice.ErrTargetAgentNotOwned):
		return common.RespondForbidden(ctx, "target agent is not owned by authenticated principal")
	case errors.Is(err, soulservice.ErrTargetAgentMustBeAgent):
		return common.RespondUnprocessableEntity(ctx, "target account must be an agent")
	case errors.Is(err, soulservice.ErrSoulAlreadyBound):
		return common.RespondConflict(ctx, "soul already bound to another local agent")
	case errors.Is(err, soulservice.ErrTargetAgentAlreadyHasSoul):
		return common.RespondConflict(ctx, "target agent already has a soul")
	default:
		return common.RespondInternalServerError(ctx)
	}
}

func toAPISoulInventoryItem(soul soulservice.Soul) apimodels.SoulInventoryItem {
	var binding *apimodels.SoulAgentBinding
	bindingState := "unbound"
	available := !soul.Bound

	if soul.Bound {
		bindingState = "bound"
		binding = &apimodels.SoulAgentBinding{
			AgentUsername:    soul.BoundAgentUsername,
			PrincipalAddress: soul.BoundPrincipalAddress,
			BoundAt:          soul.BoundAt,
			UpdatedAt:        soul.BoundUpdatedAt,
		}
	}

	return apimodels.SoulInventoryItem{
		Agent: apimodels.SoulAgentIdentity{
			AgentID:                soul.AgentID,
			Domain:                 soul.Domain,
			LocalID:                soul.LocalID,
			ENSName:                soul.ENSName,
			Wallet:                 soul.Wallet,
			TokenID:                soul.TokenID,
			MetaURI:                soul.MetaURI,
			Avatar:                 toAPISoulAvatar(soul.Avatar),
			PrincipalAddress:       soul.PrincipalAddress,
			PrincipalSignature:     soul.PrincipalSignature,
			PrincipalDeclaration:   soul.PrincipalDeclaration,
			PrincipalDeclaredAt:    soul.PrincipalDeclaredAt,
			Status:                 soul.Status,
			LifecycleStatus:        soul.LifecycleStatus,
			LifecycleReason:        soul.LifecycleReason,
			SuccessorAgentID:       soul.SuccessorAgentID,
			PredecessorAgentID:     soul.PredecessorAgentID,
			SelfDescriptionVersion: soul.SelfDescriptionVersion,
			Capabilities:           append([]string(nil), soul.Capabilities...),
			MintTxHash:             soul.MintTxHash,
			MintedAt:               soul.MintedAt,
			UpdatedAt:              soul.UpdatedAt,
		},
		BindingState:              bindingState,
		AvailableForIncorporation: available,
		Binding:                   binding,
	}
}

func toAPISoulAvatar(value *soulservice.SoulAvatar) *apimodels.SoulAgentAvatar {
	if value == nil {
		return nil
	}

	styles := make([]apimodels.SoulAgentAvatarStyle, 0, len(value.Styles))
	for _, style := range value.Styles {
		styles = append(styles, apimodels.SoulAgentAvatarStyle{
			StyleID:         style.StyleID,
			StyleName:       style.StyleName,
			RendererAddress: style.RendererAddress,
			Image:           style.Image,
			Selected:        style.Selected,
		})
	}

	return &apimodels.SoulAgentAvatar{
		TokenURI:               value.TokenURI,
		Image:                  value.Image,
		CurrentStyleID:         value.CurrentStyleID,
		CurrentStyleName:       value.CurrentStyleName,
		CurrentRendererAddress: value.CurrentRendererAddress,
		Styles:                 styles,
	}
}
