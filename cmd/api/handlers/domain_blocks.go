package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// CreateDomainBlockRequest represents the request to block a domain
type CreateDomainBlockRequest struct {
	Domain string `json:"domain"`
}

// HandleGetDomainBlocks handles GET /api/v1/domain_blocks
func (h *Handler) HandleGetDomainBlocks(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope for blocks
	if !claims.HasScope(auth.ScopeRead) && !claims.HasScope("read:blocks") {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse query parameters
	limit := 100
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["max_id"]

	// Get blocked domains
	domains, nextCursor, err := h.store.GetUserDomainBlocks(ctx, claims.Username, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get domain blocks",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get domain blocks")), nil
	}

	// Create response with Link header for pagination
	response := common.OK(domains)
	if nextCursor != "" && len(domains) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/domain_blocks?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), nextCursor, limit)
		response.Headers["Link"] = linkHeader
	}

	return response, nil
}

// HandleCreateDomainBlock handles POST /api/v1/domain_blocks
func (h *Handler) HandleCreateDomainBlock(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope for blocks
	if !claims.HasScope(auth.ScopeWrite) && !claims.HasScope("write:blocks") {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request body
	var req CreateDomainBlockRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		h.logger.Debug("invalid domain block request", zap.Error(err))
		return common.BadRequest(errors.New("invalid request")), nil
	}

	// Validate domain
	if req.Domain == "" {
		return common.BadRequest(errors.New("domain is required")), nil
	}

	// Validate domain format
	if _, err := url.Parse("https://" + req.Domain); err != nil {
		return common.BadRequest(errors.New("invalid domain format")), nil
	}

	// Add domain block
	if err := h.store.AddDomainBlock(ctx, claims.Username, req.Domain); err != nil {
		h.logger.Error("failed to add domain block",
			zap.String("username", claims.Username),
			zap.String("domain", req.Domain),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to block domain")), nil
	}

	// Return empty response (Mastodon returns empty object)
	return common.OK(map[string]interface{}{}), nil
}

// HandleDeleteDomainBlock handles DELETE /api/v1/domain_blocks
func (h *Handler) HandleDeleteDomainBlock(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope for blocks
	if !claims.HasScope(auth.ScopeWrite) && !claims.HasScope("write:blocks") {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get domain from query parameter
	domain := request.QueryStringParameters["domain"]
	if domain == "" {
		return common.BadRequest(errors.New("domain parameter is required")), nil
	}

	// Remove domain block
	if err := h.store.RemoveDomainBlock(ctx, claims.Username, domain); err != nil {
		h.logger.Error("failed to remove domain block",
			zap.String("username", claims.Username),
			zap.String("domain", domain),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to unblock domain")), nil
	}

	// Return empty response (Mastodon returns empty object)
	return common.OK(map[string]interface{}{}), nil
}
