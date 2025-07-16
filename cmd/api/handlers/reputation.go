package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/reputation"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetReputation handles GET /api/v1/reputation/:actor_id
func (h *Handler) HandleGetReputation(ctx context.Context, request events.APIGatewayV2HTTPRequest, actorID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	_, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Normalize actor ID
	// If it's just a username, convert to full actor URI
	if !strings.Contains(actorID, "://") {
		actorID = fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, actorID)
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get reputation
	rep, err := repService.GetReputation(ctx, actorID)
	if err != nil {
		h.logger.Error("Failed to get reputation", zap.Error(err), zap.String("actor", actorID))

		// Check if it's an actor not found error
		if strings.Contains(err.Error(), "actor not found") {
			return common.NotFound(fmt.Errorf("actor not found: %s", actorID)), nil
		}

		return common.InternalServerError(err), nil
	}

	// Convert to API response
	response := convertReputationToAPI(rep)

	return common.OK(response), nil
}

// HandleExportReputation handles POST /api/v1/reputation/export
func (h *Handler) HandleExportReputation(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get the actor ID for the authenticated user
	actorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Export reputation
	portableRep, err := repService.ExportReputation(ctx, actorID)
	if err != nil {
		h.logger.Error("Failed to export reputation", zap.Error(err), zap.String("actor", actorID))
		return common.InternalServerError(err), nil
	}

	// Return with JSON-LD content type
	resp := common.OK(portableRep)
	resp.Headers["Content-Type"] = "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\""

	return resp, nil
}

// HandleImportReputation handles POST /api/v1/reputation/import
func (h *Handler) HandleImportReputation(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	_, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Parse request body
	var importReq struct {
		Document string `json:"document"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &importReq); err != nil {
		return common.BadRequest(fmt.Errorf("invalid request body: %w", err)), nil
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Import reputation
	result, err := repService.ImportReputation(ctx, importReq.Document)
	if err != nil {
		h.logger.Error("Failed to import reputation", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return common.OK(result), nil
}

// HandleCreateVouch handles POST /api/v1/vouches
func (h *Handler) HandleCreateVouch(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Parse request body
	var vouchReq struct {
		To         string  `json:"to"`
		Confidence float64 `json:"confidence"`
		Context    string  `json:"context"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &vouchReq); err != nil {
		return common.BadRequest(fmt.Errorf("invalid request body: %w", err)), nil
	}

	// Validate input
	if vouchReq.To == "" {
		return common.BadRequest(fmt.Errorf("missing 'to' field")), nil
	}
	if vouchReq.Confidence < 0 || vouchReq.Confidence > 1 {
		return common.BadRequest(fmt.Errorf("confidence must be between 0 and 1")), nil
	}

	// Get the actor ID for the authenticated user
	fromActorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Create vouch
	vouch, err := repService.CreateVouch(ctx, fromActorID, vouchReq.To, vouchReq.Confidence, vouchReq.Context)
	if err != nil {
		h.logger.Error("Failed to create vouch", zap.Error(err))
		if strings.Contains(err.Error(), "insufficient reputation") {
			return common.BadRequest(fmt.Errorf("insufficient reputation to vouch")), nil
		}
		if strings.Contains(err.Error(), "monthly vouch limit") {
			return common.BadRequest(fmt.Errorf("monthly vouch limit reached")), nil
		}
		return common.InternalServerError(err), nil
	}

	// Convert to API response
	response := convertVouchToAPI(vouch)

	return common.Created(response), nil
}

// HandleGetVouches handles GET /api/v1/vouches/:actor_id
func (h *Handler) HandleGetVouches(ctx context.Context, request events.APIGatewayV2HTTPRequest, actorID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	_, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Normalize actor ID
	// If it's just a username, convert to full actor URI
	if !strings.Contains(actorID, "://") {
		actorID = fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, actorID)
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get vouches
	vouches, err := repService.GetVouches(ctx, actorID)
	if err != nil {
		h.logger.Error("Failed to get vouches", zap.Error(err), zap.String("actor", actorID))

		// Check if it's an actor not found error
		if strings.Contains(err.Error(), "actor not found") {
			return common.NotFound(fmt.Errorf("actor not found: %s", actorID)), nil
		}

		return common.InternalServerError(err), nil
	}

	// Convert to API response
	apiVouches := make([]map[string]interface{}, len(vouches))
	for i, v := range vouches {
		apiVouches[i] = convertVouchToAPI(&v)
	}

	return common.OK(apiVouches), nil
}

// HandleRevokeVouch handles DELETE /api/v1/vouches/:vouch_id
func (h *Handler) HandleRevokeVouch(ctx context.Context, request events.APIGatewayV2HTTPRequest, vouchID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get the actor ID for the authenticated user
	actorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Revoke vouch
	err = repService.RevokeVouch(ctx, vouchID, actorID)
	if err != nil {
		h.logger.Error("Failed to revoke vouch", zap.Error(err))
		if strings.Contains(err.Error(), "only the voucher can revoke") {
			return common.Forbidden(fmt.Errorf("only the voucher can revoke their vouch")), nil
		}
		return common.InternalServerError(err), nil
	}

	return common.NoContent(), nil
}

// HandleVerifyReputation handles POST /api/v1/reputation/verify
func (h *Handler) HandleVerifyReputation(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Parse request body
	var verifyReq struct {
		Document string `json:"document"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &verifyReq); err != nil {
		return common.BadRequest(fmt.Errorf("invalid request body: %w", err)), nil
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Verify reputation
	result, err := repService.VerifyReputation(ctx, verifyReq.Document)
	if err != nil {
		h.logger.Error("Failed to verify reputation", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return common.OK(result), nil
}

// HandleGetReputationKeys handles GET /.well-known/reputation-keys
func (h *Handler) HandleGetReputationKeys(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		return common.InternalServerError(err), nil
	}

	// Get public key
	publicKey := repService.GetPublicKey()

	response := map[string]string{
		"publicKey": publicKey,
	}

	return common.OK(response), nil
}

// Helper functions

func (h *Handler) getReputationService() (*reputation.Service, error) {
	// Create service config using existing store
	cfg := &reputation.Config{
		Storage:     h.store,
		Logger:      h.logger,
		InstanceURL: h.cfg.BaseURL(),
		PrivateKey:  h.cfg.ReputationPrivateKey,
	}

	return reputation.NewService(cfg)
}

func convertReputationToAPI(rep *reputation.Reputation) map[string]interface{} {
	return map[string]interface{}{
		"id":               rep.ActorID,
		"instance":         rep.InstanceURL,
		"total_score":      rep.TotalScore,
		"trust_score":      rep.TrustScore,
		"activity_score":   rep.ActivityScore,
		"moderation_score": rep.ModerationScore,
		"community_score":  rep.CommunityScore,
		"calculated_at":    rep.CalculatedAt,
		"version":          rep.Version,
		"evidence": map[string]interface{}{
			"total_posts":     rep.TotalPosts,
			"total_followers": rep.TotalFollowers,
			"account_age":     rep.AccountAge,
			"vouch_count":     rep.VouchCount,
		},
	}
}

func convertVouchToAPI(vouch *reputation.Vouch) map[string]interface{} {
	result := map[string]interface{}{
		"id":                 vouch.ID,
		"from":               vouch.From,
		"to":                 vouch.To,
		"confidence":         vouch.Confidence,
		"context":            vouch.Context,
		"created_at":         vouch.CreatedAt,
		"expires_at":         vouch.ExpiresAt,
		"voucher_reputation": vouch.VoucherReputation,
		"active":             vouch.Active,
		"revoked":            vouch.Revoked,
	}

	if vouch.RevokedAt != nil {
		result["revoked_at"] = vouch.RevokedAt
	}

	return result
}
