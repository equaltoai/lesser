package lift

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	relationshipsvc "github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// CreateDomainBlockRequest represents the request to block a domain
type CreateDomainBlockRequest struct {
	Domain string `json:"domain"`
}

// HandleGetDomainBlocksLift handles GET /api/v1/domain_blocks
func (h *Handler) HandleGetDomainBlocksLift(ctx *lift.Context) error {
	// Authenticate user with read:blocks scope or general read scope
	username, err := h.authenticateUser(ctx, []string{"read:blocks", auth.ScopeRead})
	if err != nil {
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
	result, err := h.registry.Relationships().GetDomainBlocks(ctx.Context, &relationshipsvc.GetDomainBlocksQuery{
		UserID: username,
		Limit:  params.Limit,
		Cursor: params.MaxID,
	})
	if err != nil {
		h.logger.Error("failed to get domain blocks", zap.Error(err))
		return h.respondWithError(ctx, 500, "failed to get domain blocks")
	}

	// Set pagination headers
	if result.NextCursor != "" && len(result.Domains) > 0 {
		params.MaxID = result.NextCursor
		h.withPaginationHeaders(ctx, fmt.Sprintf("%s/api/v1/domain_blocks", h.cfg.BaseURL()),
			params, true, false)
	}

	return ctx.JSON(result.Domains)
}

// HandleCreateDomainBlockLift handles POST /api/v1/domain_blocks
func (h *Handler) HandleCreateDomainBlockLift(ctx *lift.Context) error {
	// Authenticate user with write:blocks scope or general write scope
	username, err := h.authenticateUser(ctx, []string{"write:blocks", auth.ScopeWrite})
	if err != nil {
		return h.respondUnauthorized(ctx)
	}

	// Parse request body
	var req CreateDomainBlockRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environment - try parsing directly from request body
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
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
	if err = h.registry.Relationships().AddDomainBlock(ctx.Context, &relationshipsvc.AddDomainBlockCommand{
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
	return ctx.JSON(map[string]any{})
}

// HandleDeleteDomainBlockLift handles DELETE /api/v1/domain_blocks
func (h *Handler) HandleDeleteDomainBlockLift(ctx *lift.Context) error {
	var username string

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("auth_header", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Check write scope for blocks
	if !claims.HasScope(auth.ScopeWrite) && !claims.HasScope("write:blocks") {
		return common.RespondInsufficientScope(ctx)
	}

	username = claims.Username

	// Get domain from query parameter
	domain := ctx.Query("domain")
	if err := common.ValidateRequiredParam("domain", domain); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		domain = ctx.Request.Request.QueryParams["domain"]
	}

	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		return common.RespondBadRequest(ctx, "domain parameter is required")
	}

	// Use Relationships service
	err = h.registry.Relationships().RemoveDomainBlock(ctx.Context, &relationshipsvc.RemoveDomainBlockCommand{
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
	return ctx.JSON(map[string]any{})
}
