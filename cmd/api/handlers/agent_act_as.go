package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

// resolveActAs resolves an optional X-Lesser-Act-As indicator into an authorized
// act-as resolution for the authenticated caller. A (nil, nil, nil) result means the
// request carries no indicator and must behave exactly like an ordinary owner-scoped
// request. Malformed or unknown indicators fail with 400, a missing or inactive share
// grant fails with 403, and any storage or grant-check error fails closed with 500.
func (h *Handler) resolveActAs(ctx *apptheory.Context, claims *auth.Claims) (*auth.ActAsResolution, *apptheory.Response, error) {
	indicator := headerValue(ctx, auth.ActAsAgentHeader)
	if strings.TrimSpace(indicator) == "" {
		return nil, nil, nil
	}

	resolution, err := auth.ResolveActAs(ctx.Context(), claims, indicator, h.actAsAccountLookup(), h.actAsGrantCheck())
	if err == nil {
		return resolution, nil, nil
	}

	switch {
	case errors.Is(err, auth.ErrActAsMalformedIndicator):
		resp, respErr := common.RespondBadRequest(ctx, "invalid "+auth.ActAsAgentHeader+" agent indicator")
		return nil, resp, respErr
	case errors.Is(err, auth.ErrActAsUnknownAgent):
		resp, respErr := common.RespondBadRequest(ctx, "act-as agent not found")
		return nil, resp, respErr
	case errors.Is(err, auth.ErrActAsNotGranted):
		resp, respErr := common.RespondForbidden(ctx, "no active agent share grant authorizes this caller")
		return nil, resp, respErr
	default:
		h.logger.Error("act-as resolution failed", zap.Error(err))
		resp, respErr := common.RespondInternalServerError(ctx)
		return nil, resp, respErr
	}
}

// effectiveActingUsername returns the account the request acts on: the shared agent
// under act-as, otherwise the authenticated caller. Owner-path output is identical to
// reading claims.Username directly.
func effectiveActingUsername(claims *auth.Claims, acting *auth.ActAsResolution) string {
	if acting != nil {
		return acting.AgentUsername
	}
	if claims == nil {
		return ""
	}
	return claims.Username
}

func (h *Handler) actAsAccountLookup() auth.ActAsAccountLookup {
	if h == nil || h.repos == nil || h.repos.Account() == nil {
		return nil
	}
	accounts := h.repos.Account()
	return func(ctx context.Context, username string) (*storage.Account, error) {
		return accounts.GetAccount(ctx, username)
	}
}

func (h *Handler) actAsGrantCheck() auth.ActAsGrantCheck {
	service := h.agentShareService()
	if service == nil {
		return nil
	}
	return service.IsActive
}

// recordActAsAuditEvent writes the mandatory caller-attribution audit record for an
// act-as mutation. It fires only when an act-as resolution is present, so owner-path
// behavior is byte-identical. The record names both identities: the agent in Username
// (matching the per-agent audit stream used by share-grant management events) and the
// real caller in the acted_by metadata field.
func (h *Handler) recordActAsAuditEvent(ctx *apptheory.Context, claims *auth.Claims, acting *auth.ActAsResolution, action, targetID string, metadata map[string]any) {
	if h == nil || h.repos == nil || h.repos.Audit() == nil || claims == nil || acting == nil {
		return
	}

	action = strings.TrimSpace(action)
	if action == "" {
		return
	}

	now := time.Now().UTC()
	entry := &storageModels.AuthAuditLog{
		ID:        common.GenerateOperationIDULID(),
		Timestamp: now,
		EventType: action,
		Severity:  "INFO",
		Username:  acting.AgentUsername,
		SessionID: claims.SessionID,
		IPAddress: headerValue(ctx, "x-forwarded-for"),
		UserAgent: headerValue(ctx, "user-agent"),
		Success:   true,
		CreatedAt: now,
	}

	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["acted_by"] = acting.ActedBy
	metadata["agent_username"] = acting.AgentUsername
	if targetID != "" {
		metadata["target_id"] = targetID
	}
	if raw, err := json.Marshal(metadata); err == nil {
		entry.Metadata = string(raw)
	} else {
		h.logger.Debug("failed to marshal act-as audit metadata", zap.Error(err))
	}

	if err := h.repos.Audit().StoreAuditLog(ctx.Context(), entry); err != nil {
		h.logger.Debug("failed to store act-as audit log", zap.Error(err))
	}
}
