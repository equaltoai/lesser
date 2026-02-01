package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	relationshipsvc "github.com/equaltoai/lesser/pkg/services/relationships"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleGetDomainBlocksLift handles GET /api/v1/domain_blocks
func (h *Handler) HandleGetDomainBlocksLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user with read:blocks scope or general read scope
	username, err := h.authenticateUser(ctx, []string{"read:blocks", auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Parse pagination parameters
	params := h.parsePaginationParams(ctx)
	if err := common.ValidateIntRange("limit", params.Limit, 1, 200); err != nil {
		params.Limit = 200 // Cap at 200 for domain blocks
	}
	if params.Limit == 0 {
		params.Limit = 100 // Default
	}

	// Use Relationships service
	result, err := h.registry.Relationships().GetDomainBlocks(ctx.Context(), &relationshipsvc.GetDomainBlocksQuery{
		UserID: username,
		Limit:  params.Limit,
		Cursor: params.MaxID,
	})
	if err != nil {
		h.logger.Error("failed to get domain blocks", zap.Error(err))
		return h.respondWithError(ctx, 500, "failed to get domain blocks")
	}

	// Set pagination headers
	resp, err := okJSON(result.Domains)
	if err != nil {
		return nil, err
	}

	if result.NextCursor != "" && len(result.Domains) > 0 {
		params.MaxID = result.NextCursor
		h.withPaginationHeaders(resp, fmt.Sprintf("%s/api/v1/domain_blocks", h.cfg.BaseURL()),
			params, true, false)
	}

	return resp, nil
}

// HandleCreateDomainBlockLift handles POST /api/v1/domain_blocks
func (h *Handler) HandleCreateDomainBlockLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user with write:blocks scope or general write scope
	username, err := h.authenticateUser(ctx, []string{"write:blocks", auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Parse request body
	var req apimodels.CreateDomainBlockRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		// Fallback for test environment - try parsing directly from request body
		if len(ctx.Request.Body) > 0 {
			if jsonErr := json.Unmarshal(ctx.Request.Body, &req); jsonErr != nil {
				h.logger.Debug("invalid domain block request",
					zap.Error(err),
					zap.Error(jsonErr),
					zap.String("body", string(ctx.Request.Body)))
				return common.RespondInvalidRequest(ctx)
			}
			h.logger.Debug("parsed request from body fallback",
				zap.String("domain", req.Domain))
		} else {
			h.logger.Debug("invalid domain block request - no body", zap.Error(err))
			return common.RespondInvalidRequest(ctx)
		}
	}

	// Validate domain
	if err := common.ValidateRequiredParam("domain", req.Domain); err != nil {
		return common.RespondBadRequest(ctx, "domain is required")
	}

	// Validate domain format - basic check
	if err := common.ValidateRequiredParam("domain", req.Domain); err != nil {
		return common.RespondBadRequest(ctx, "invalid domain format")
	}
	if strings.Contains(req.Domain, " ") ||
		strings.Contains(req.Domain, "..") || strings.HasPrefix(req.Domain, ".") ||
		strings.HasSuffix(req.Domain, ".") {
		return common.RespondBadRequest(ctx, "invalid domain format")
	}

	// Use Relationships service
	if err = h.registry.Relationships().AddDomainBlock(ctx.Context(), &relationshipsvc.AddDomainBlockCommand{
		UserID: username,
		Domain: req.Domain,
	}); err != nil {
		h.logger.Error("failed to add domain block",
			zap.String("username", username),
			zap.String("domain", req.Domain),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to block domain")
	}

	// Return empty response (Mastodon returns empty object)
	return okJSON(apimodels.EmptyObject{})
}

// HandleDeleteDomainBlockLift handles DELETE /api/v1/domain_blocks
func (h *Handler) HandleDeleteDomainBlockLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user with write:blocks scope or general write scope
	username, err := h.authenticateUser(ctx, []string{"write:blocks", auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondInsufficientScope(ctx)
		}
		return common.RespondUnauthorized(ctx)
	}

	// Get domain from query parameter
	domain := queryValue(ctx, "domain")

	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return common.RespondBadRequest(ctx, "domain parameter is required")
	}

	// Use Relationships service
	err = h.registry.Relationships().RemoveDomainBlock(ctx.Context(), &relationshipsvc.RemoveDomainBlockCommand{
		UserID: username,
		Domain: domain,
	})
	if err != nil {
		h.logger.Error("failed to remove domain block",
			zap.String("username", username),
			zap.String("domain", domain),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to unblock domain")
	}

	// Return empty response (Mastodon returns empty object)
	return okJSON(apimodels.EmptyObject{})
}
