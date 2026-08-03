package handlers

import (
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

const soulWellKnownProofPrefix = "lesser-soul-agent="

// HandleWellKnownLesserSoulAgentLift serves the current soul HTTPS proof value at:
// GET /.well-known/lesser-soul-agent
//
// lesser-host verifies this endpoint by checking that the response body matches the expected JSON payload.
func (h *Handler) HandleWellKnownLesserSoulAgentLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h == nil || h.repos == nil || h.repos.Instance() == nil {
		return common.RespondNotFound(ctx, "proof")
	}

	cfg, err := h.repos.Instance().GetWellKnownLesserSoulAgent(ctx.Context())
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}
	if cfg == nil || strings.TrimSpace(cfg.ProofValue) == "" {
		return common.RespondNotFound(ctx, "proof")
	}

	if !cfg.ExpiresAt.IsZero() && time.Now().After(cfg.ExpiresAt) {
		return common.RespondNotFound(ctx, "proof")
	}

	token := strings.TrimSpace(cfg.ProofValue)
	token = strings.TrimPrefix(token, soulWellKnownProofPrefix)
	token = strings.TrimSpace(token)
	if token == "" {
		return common.RespondNotFound(ctx, "proof")
	}

	resp, err := okJSON(map[string]any{"lesser-soul-agent": token})
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}
	setHeader(resp, "cache-control", "no-store")
	return resp, nil
}

type adminSetSoulWellKnownProofRequest struct {
	ProofValue       string `json:"proof_value"`
	ExpiresInSeconds *int   `json:"expires_in_seconds,omitempty"`
}

// HandleAdminSetSoulWellKnownProofLift sets the soul HTTPS proof value served at:
// GET /.well-known/lesser-soul-agent
//
// This is intended to let instance admins quickly satisfy lesser-host's domain proof checks without redeploys.
func (h *Handler) HandleAdminSetSoulWellKnownProofLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	_, err := h.authenticateWithScope(ctx, auth.ScopeAdmin)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx, auth.ScopeAdmin)
		}
		return common.RespondUnauthorized(ctx)
	}

	if h == nil || h.repos == nil || h.repos.Instance() == nil {
		return common.RespondInternalServerError(ctx)
	}

	var req adminSetSoulWellKnownProofRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	proofValue := strings.TrimSpace(req.ProofValue)
	if proofValue != "" {
		token := proofValue
		token = strings.TrimPrefix(token, soulWellKnownProofPrefix)
		token = strings.TrimSpace(token)
		if token == "" {
			return common.RespondValidationError(ctx, common.ValidationError{Field: "proof_value", Message: "must not be empty"})
		}
		if strings.ContainsAny(token, " \t\r\n") {
			return common.RespondValidationError(ctx, common.ValidationError{Field: "proof_value", Message: "must not include whitespace"})
		}
		if len(token) > 512 {
			return common.RespondValidationError(ctx, common.ValidationError{Field: "proof_value", Message: "must be 512 characters or fewer"})
		}

		proofValue = token
	}

	now := time.Now().UTC()
	expiresAt := time.Time{}
	if proofValue != "" {
		ttl := 45 * time.Minute
		if req.ExpiresInSeconds != nil {
			if *req.ExpiresInSeconds < 1 || *req.ExpiresInSeconds > 24*60*60 {
				return common.RespondValidationError(ctx, common.ValidationError{Field: "expires_in_seconds", Message: "must be between 1 and 86400"})
			}
			ttl = time.Duration(*req.ExpiresInSeconds) * time.Second
		}
		expiresAt = now.Add(ttl)
	}

	cfg := &storageModels.InstanceWellKnownLesserSoulAgent{
		ProofValue: proofValue,
		ExpiresAt:  expiresAt,
		UpdatedAt:  now,
	}

	if err := h.repos.Instance().SetWellKnownLesserSoulAgent(ctx.Context(), cfg); err != nil {
		return common.RespondInternalServerError(ctx)
	}

	out := map[string]any{
		"proof_value": cfg.ProofValue,
	}
	if !cfg.ExpiresAt.IsZero() {
		out["expires_at"] = cfg.ExpiresAt.UTC().Format(time.RFC3339)
	}

	return okJSON(out)
}
