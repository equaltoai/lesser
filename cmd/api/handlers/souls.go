package handlers

import (
	"context"
	"errors"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

type soulHandlerService interface {
	ListMine(ctx context.Context, username string) ([]soulservice.Soul, error)
	Incorporate(ctx context.Context, username string, agentID string) (*soulservice.Soul, error)
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
		items = append(items, toAPISoulInventoryItem(claims.Username, soul))
	}

	return okJSON(apimodels.SoulsMineResponse{
		Souls: items,
		Count: len(items),
	})
}

// HandleIncorporateSoulLift explicitly binds a soul to the authenticated local body.
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

	svc := h.getSoulService()
	if svc == nil {
		return common.RespondInternalServerError(ctx)
	}

	soul, err := svc.Incorporate(ctx.Context(), claims.Username, agentID)
	if err != nil {
		return h.respondSoulServiceError(ctx, err)
	}

	return okJSON(apimodels.SoulIncorporateResponse{
		Soul: toAPISoulInventoryItem(claims.Username, *soul),
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

func (h *Handler) respondSoulServiceError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
	switch {
	case errors.Is(err, soulservice.ErrTrustNotConfigured):
		return common.RespondUnprocessableEntity(ctx, "trust not configured")
	case errors.Is(err, soulservice.ErrSoulNotAvailable):
		return common.RespondNotFound(ctx, "soul")
	case errors.Is(err, soulservice.ErrSoulAlreadyBound):
		return common.RespondConflict(ctx, "soul already bound to another local body")
	case errors.Is(err, soulservice.ErrBodyAlreadyHasSoul):
		return common.RespondConflict(ctx, "local body already has a soul")
	default:
		return common.RespondInternalServerError(ctx)
	}
}

func toAPISoulInventoryItem(currentUsername string, soul soulservice.Soul) apimodels.SoulInventoryItem {
	var binding *apimodels.SoulBodyBinding
	bindingState := "unbound"
	available := true

	if soul.Bound {
		bindingState = "bound"
		available = strings.EqualFold(strings.TrimSpace(soul.BoundUsername), strings.TrimSpace(currentUsername))
		binding = &apimodels.SoulBodyBinding{
			Username:         soul.BoundUsername,
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
			PrincipalAddress:       soul.PrincipalAddress,
			Status:                 soul.Status,
			LifecycleStatus:        soul.LifecycleStatus,
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
