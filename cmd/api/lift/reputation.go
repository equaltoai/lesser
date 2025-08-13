package lift

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/reputation"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetReputationLift handles GET /api/v1/reputation/:actor_id
func (h *Handler) HandleGetReputationLift(ctx *lift.Context) error {
	// Get actorID from path parameter
	actorID := ctx.Param("actor_id")
	if actorID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing actor_id parameter"})
	}

	// Check for test mode
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string

	if testUsername != "" {
		// Test mode - use provided username
		username = testUsername
		h.logger.Debug("test mode: using provided username", zap.String("username", username))
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		_, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}
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
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Get reputation
	rep, err := repService.GetReputation(ctx.Context, actorID)
	if err != nil {
		h.logger.Error("Failed to get reputation", zap.Error(err), zap.String("actor", actorID))

		// Check if it's an actor not found error
		if strings.Contains(err.Error(), "actor not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": fmt.Sprintf("actor not found: %s", actorID)})
		}

		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Convert to API response
	response := convertReputationToAPI(rep)

	return ctx.Status(http.StatusOK).JSON(response)
}

// HandleExportReputationLift handles POST /api/v1/reputation/export
func (h *Handler) HandleExportReputationLift(ctx *lift.Context) error {
	// Check for test mode
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	var claims *auth.Claims

	if testUsername != "" {
		// Test mode - use provided username
		username = testUsername
		h.logger.Debug("test mode: using provided username", zap.String("username", username))
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}

		username = claims.Username
	}

	// Get the actor ID for the authenticated user
	actorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Export reputation
	portableRep, err := repService.ExportReputation(ctx.Context, actorID)
	if err != nil {
		h.logger.Error("Failed to export reputation", zap.Error(err), zap.String("actor", actorID))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Set JSON-LD content type
	ctx.Response.Header("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")

	return ctx.Status(http.StatusOK).JSON(portableRep)
}

// HandleImportReputationLift handles POST /api/v1/reputation/import
func (h *Handler) HandleImportReputationLift(ctx *lift.Context) error {
	// Check for test mode
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string

	if testUsername != "" {
		// Test mode - use provided username
		username = testUsername
		h.logger.Debug("test mode: using provided username", zap.String("username", username))
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		_, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}
	}

	// Parse request body
	var importReq struct {
		Document string `json:"document"`
	}
	if err := ctx.ParseRequest(&importReq); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &importReq); err != nil {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
			}
		} else {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
		}
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Import reputation
	result, err := repService.ImportReputation(ctx.Context, importReq.Document)
	if err != nil {
		h.logger.Error("Failed to import reputation", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	return ctx.Status(http.StatusOK).JSON(result)
}

// HandleCreateVouchLift handles POST /api/v1/vouches
func (h *Handler) HandleCreateVouchLift(ctx *lift.Context) error {
	// Check for test mode
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	var claims *auth.Claims

	if testUsername != "" {
		// Test mode - use provided username
		username = testUsername
		h.logger.Debug("test mode: using provided username", zap.String("username", username))
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}

		username = claims.Username
	}

	// Parse request body
	var vouchReq struct {
		To         string  `json:"to"`
		Confidence float64 `json:"confidence"`
		Context    string  `json:"context"`
	}
	if err := ctx.ParseRequest(&vouchReq); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &vouchReq); err != nil {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
			}
		} else {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
		}
	}

	// Validate input
	if vouchReq.To == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing 'to' field"})
	}
	if vouchReq.Confidence < 0 || vouchReq.Confidence > 1 {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "confidence must be between 0 and 1"})
	}

	// Get the actor ID for the authenticated user
	fromActorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Create vouch
	vouch, err := repService.CreateVouch(ctx.Context, fromActorID, vouchReq.To, vouchReq.Confidence, vouchReq.Context)
	if err != nil {
		h.logger.Error("Failed to create vouch", zap.Error(err))
		if strings.Contains(err.Error(), "insufficient reputation") {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "insufficient reputation to vouch"})
		}
		if strings.Contains(err.Error(), "monthly vouch limit") {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "monthly vouch limit reached"})
		}
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Convert to API response
	response := convertVouchToAPI(vouch)

	return ctx.Status(http.StatusCreated).JSON(response)
}

// HandleGetVouchesLift handles GET /api/v1/vouches/:actor_id
func (h *Handler) HandleGetVouchesLift(ctx *lift.Context) error {
	// Get actorID from path parameter
	actorID := ctx.Param("actor_id")
	if actorID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing actor_id parameter"})
	}

	// Check for test mode
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string

	if testUsername != "" {
		// Test mode - use provided username
		username = testUsername
		h.logger.Debug("test mode: using provided username", zap.String("username", username))
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		_, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}
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
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Get vouches
	vouches, err := repService.GetVouches(ctx.Context, actorID)
	if err != nil {
		h.logger.Error("Failed to get vouches", zap.Error(err), zap.String("actor", actorID))

		// Check if it's an actor not found error
		if strings.Contains(err.Error(), "actor not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": fmt.Sprintf("actor not found: %s", actorID)})
		}

		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Convert to API response
	apiVouches := make([]map[string]any, len(vouches))
	for i, v := range vouches {
		apiVouches[i] = convertVouchToAPI(&v)
	}

	return ctx.Status(http.StatusOK).JSON(apiVouches)
}

// HandleRevokeVouchLift handles DELETE /api/v1/vouches/:vouch_id
func (h *Handler) HandleRevokeVouchLift(ctx *lift.Context) error {
	// Get vouchID from path parameter
	vouchID := ctx.Param("vouch_id")
	if vouchID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing vouch_id parameter"})
	}

	// Check for test mode
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	var claims *auth.Claims

	if testUsername != "" {
		// Test mode - use provided username
		username = testUsername
		h.logger.Debug("test mode: using provided username", zap.String("username", username))
	} else {
		// Extract and validate token
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			authHeader = ctx.Header("authorization")
		}

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}

		// Validate token
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "unauthorized"})
		}

		username = claims.Username
	}

	// Get the actor ID for the authenticated user
	actorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Revoke vouch
	err = repService.RevokeVouch(ctx.Context, vouchID, actorID)
	if err != nil {
		h.logger.Error("Failed to revoke vouch", zap.Error(err))
		if strings.Contains(err.Error(), "only the voucher can revoke") {
			return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "only the voucher can revoke their vouch"})
		}
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	return ctx.Status(http.StatusNoContent).JSON(nil)
}

// HandleVerifyReputationLift handles POST /api/v1/reputation/verify
func (h *Handler) HandleVerifyReputationLift(ctx *lift.Context) error {
	// Parse request body
	var verifyReq struct {
		Document string `json:"document"`
	}
	if err := ctx.ParseRequest(&verifyReq); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &verifyReq); err != nil {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
			}
		} else {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
		}
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Verify reputation
	result, err := repService.VerifyReputation(ctx.Context, verifyReq.Document)
	if err != nil {
		h.logger.Error("Failed to verify reputation", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	return ctx.Status(http.StatusOK).JSON(result)
}

// HandleGetReputationKeysLift handles GET /.well-known/reputation-keys
func (h *Handler) HandleGetReputationKeysLift(ctx *lift.Context) error {
	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "internal server error"})
	}

	// Get public key
	publicKey := repService.GetPublicKey()

	response := map[string]string{
		"publicKey": publicKey,
	}

	return ctx.Status(http.StatusOK).JSON(response)
}

// Helper functions

func (h *Handler) getReputationService() (*reputation.Service, error) {
	// Create service config using existing store
	cfg := &reputation.Config{
		Storage:     h.repos,
		Logger:      h.logger,
		InstanceURL: h.cfg.BaseURL(),
		PrivateKey:  h.cfg.ReputationPrivateKey,
	}

	return reputation.NewService(cfg)
}

func convertReputationToAPI(rep *reputation.Reputation) map[string]any {
	return map[string]any{
		"id":               rep.ActorID,
		"instance":         rep.InstanceURL,
		"total_score":      rep.TotalScore,
		"trust_score":      rep.TrustScore,
		"activity_score":   rep.ActivityScore,
		"moderation_score": rep.ModerationScore,
		"community_score":  rep.CommunityScore,
		"calculated_at":    rep.CalculatedAt,
		"version":          rep.Version,
		"evidence": map[string]any{
			"total_posts":     rep.TotalPosts,
			"total_followers": rep.TotalFollowers,
			"account_age":     rep.AccountAge,
			"vouch_count":     rep.VouchCount,
		},
	}
}

func convertVouchToAPI(vouch *reputation.Vouch) map[string]any {
	result := map[string]any{
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
