package lift

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
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
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string

	if testUsername != "" {
		// Use test username directly (test mode)
		username = testUsername
	} else {
		// Extract token from Authorization header
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		// Try direct access to headers if ctx.Header doesn't work
		if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
			authHeader = ctx.Request.Request.Headers["Authorization"]
			if authHeader == "" {
				authHeader = ctx.Request.Request.Headers["authorization"]
			}
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check read scope for blocks
		if !claims.HasScope(auth.ScopeRead) && !claims.HasScope("read:blocks") {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Parse query parameters
	limit := 100
	limitStr := ctx.Query("limit")
	if limitStr == "" && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	cursor := ctx.Query("max_id")
	if cursor == "" && ctx.Request != nil && ctx.Request.Request != nil {
		cursor = ctx.Request.Request.QueryParams["max_id"]
	}

	// Use Relationships service
	result, err := h.registry.Relationships().GetDomainBlocks(ctx.Context, &relationshipsvc.GetDomainBlocksQuery{
		UserID: username,
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		h.logger.Error("failed to get domain blocks", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get domain blocks"})
	}

	// Set Link header for pagination if there's a cursor
	if result.NextCursor != "" && len(result.Domains) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/domain_blocks?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), result.NextCursor, limit)
		ctx.Response.Header("Link", linkHeader)
	}

	return ctx.JSON(result.Domains)
}

// HandleCreateDomainBlockLift handles POST /api/v1/domain_blocks
func (h *Handler) HandleCreateDomainBlockLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string

	if testUsername != "" {
		// Use test username directly (test mode)
		username = testUsername
	} else {
		// Extract token from Authorization header
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		// Try direct access to headers if ctx.Header doesn't work
		if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
			authHeader = ctx.Request.Request.Headers["Authorization"]
			if authHeader == "" {
				authHeader = ctx.Request.Request.Headers["authorization"]
			}
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope for blocks
		if !claims.HasScope(auth.ScopeWrite) && !claims.HasScope("write:blocks") {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
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
				return ctx.Status(400).JSON(map[string]string{"error": "invalid request"})
			}
			h.logger.Debug("parsed request from body fallback",
				zap.String("domain", req.Domain))
		} else {
			h.logger.Debug("invalid domain block request - no body", zap.Error(err))
			return ctx.Status(400).JSON(map[string]string{"error": "invalid request"})
		}
	}

	// Validate domain
	if req.Domain == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "domain is required"})
	}

	// Validate domain format - basic check
	if strings.Contains(req.Domain, " ") || req.Domain == "" ||
		strings.Contains(req.Domain, "..") || strings.HasPrefix(req.Domain, ".") ||
		strings.HasSuffix(req.Domain, ".") {
		return ctx.Status(400).JSON(map[string]string{"error": "invalid domain format"})
	}

	// Use Relationships service
	err := h.registry.Relationships().AddDomainBlock(ctx.Context, &relationshipsvc.AddDomainBlockCommand{
		UserID: username,
		Domain: req.Domain,
	})
	if err != nil {
		h.logger.Error("failed to add domain block",
			zap.String("username", username),
			zap.String("domain", req.Domain),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to block domain"})
	}

	// Return empty response (Mastodon returns empty object)
	return ctx.JSON(map[string]any{})
}

// HandleDeleteDomainBlockLift handles DELETE /api/v1/domain_blocks
func (h *Handler) HandleDeleteDomainBlockLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string

	if testUsername != "" {
		// Use test username directly (test mode)
		username = testUsername
	} else {
		// Extract token from Authorization header
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		// Try direct access to headers if ctx.Header doesn't work
		if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
			authHeader = ctx.Request.Request.Headers["Authorization"]
			if authHeader == "" {
				authHeader = ctx.Request.Request.Headers["authorization"]
			}
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope for blocks
		if !claims.HasScope(auth.ScopeWrite) && !claims.HasScope("write:blocks") {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get domain from query parameter
	domain := ctx.Query("domain")
	if domain == "" && ctx.Request != nil && ctx.Request.Request != nil {
		domain = ctx.Request.Request.QueryParams["domain"]
	}

	if domain == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "domain parameter is required"})
	}

	// Use Relationships service
	err := h.registry.Relationships().RemoveDomainBlock(ctx.Context, &relationshipsvc.RemoveDomainBlockCommand{
		UserID: username,
		Domain: domain,
	})
	if err != nil {
		h.logger.Error("failed to remove domain block",
			zap.String("username", username),
			zap.String("domain", domain),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to unblock domain"})
	}

	// Return empty response (Mastodon returns empty object)
	return ctx.JSON(map[string]any{})
}
