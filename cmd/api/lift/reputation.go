package lift

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/reputation"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetReputationLift handles GET /api/v1/reputation/:actor_id
func (h *Handler) HandleGetReputationLift(ctx *lift.Context) error {
	// Get actorID from path parameter
	actorID := ctx.Param("actor_id")
	if err := common.ValidateRequiredParam("actor_id", actorID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Authentication required - extract and validate token
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = ctx.Header("authorization")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	_, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx)
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
		return common.RespondInternalServerError(ctx)
	}

	// Get reputation
	rep, err := repService.GetReputation(ctx.Context, actorID)
	if err != nil {
		h.logger.Error("Failed to get reputation", zap.Error(err), zap.String("actor", actorID))

		// Check if it's an actor not found error
		if strings.Contains(err.Error(), "actor not found") {
			return common.RespondActorNotFound(ctx)
		}

		return common.RespondInternalServerError(ctx)
	}

	response := apimodels.ReputationResponse{
		ID:              rep.ActorID,
		Instance:        rep.InstanceURL,
		TotalScore:      rep.TotalScore,
		TrustScore:      rep.TrustScore,
		ActivityScore:   rep.ActivityScore,
		ModerationScore: rep.ModerationScore,
		CommunityScore:  rep.CommunityScore,
		CalculatedAt:    rep.CalculatedAt,
		Version:         rep.Version,
		Evidence: apimodels.ReputationEvidence{
			TotalPosts:     rep.TotalPosts,
			TotalFollowers: rep.TotalFollowers,
			AccountAge:     rep.AccountAge,
			VouchCount:     rep.VouchCount,
		},
	}

	return ctx.Status(http.StatusOK).JSON(response)
}

// HandleExportReputationLift handles POST /api/v1/reputation/export
func (h *Handler) HandleExportReputationLift(ctx *lift.Context) error {
	var username string
	var claims *auth.Claims

	// Authentication required - extract and validate token
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = ctx.Header("authorization")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	username = claims.Username

	// Get the actor ID for the authenticated user
	actorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Export reputation
	portableRep, err := repService.ExportReputation(ctx.Context, actorID)
	if err != nil {
		h.logger.Error("Failed to export reputation", zap.Error(err), zap.String("actor", actorID))
		return common.RespondInternalServerError(ctx)
	}

	// Set JSON-LD content type
	ctx.Response.Header("Content-Type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")

	return ctx.Status(http.StatusOK).JSON(portableRep)
}

// HandleImportReputationLift handles POST /api/v1/reputation/import
func (h *Handler) HandleImportReputationLift(ctx *lift.Context) error {
	// Authentication required - extract and validate token
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = ctx.Header("authorization")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	_, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Parse request body
	var importReq apimodels.ReputationDocumentRequest
	if err := ctx.ParseRequest(&importReq); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &importReq); err != nil {
				return common.RespondBadRequest(ctx, "invalid request body")
			}
		} else {
			return common.RespondBadRequest(ctx, "invalid request body")
		}
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Import reputation
	result, err := repService.ImportReputation(ctx.Context, importReq.Document)
	if err != nil {
		h.logger.Error("Failed to import reputation", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return ctx.Status(http.StatusOK).JSON(result)
}

// HandleCreateVouchLift handles POST /api/v1/vouches
func (h *Handler) HandleCreateVouchLift(ctx *lift.Context) error {
	var username string
	var claims *auth.Claims

	// Authentication required - extract and validate token
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = ctx.Header("authorization")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	username = claims.Username

	// Parse request body
	var vouchReq apimodels.CreateVouchRequest
	if err := ctx.ParseRequest(&vouchReq); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &vouchReq); err != nil {
				return common.RespondBadRequest(ctx, "invalid request body")
			}
		} else {
			return common.RespondBadRequest(ctx, "invalid request body")
		}
	}

	// Validate input
	if err := common.ValidateRequiredParam("vouchTo", vouchReq.To); err != nil {
		return common.RespondBadRequest(ctx, "missing 'to' field")
	}
	if err := common.ValidateFloatRange("confidence", vouchReq.Confidence, 0, 1); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Get the actor ID for the authenticated user
	fromActorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Create vouch
	vouch, err := repService.CreateVouch(ctx.Context, fromActorID, vouchReq.To, vouchReq.Confidence, vouchReq.Context)
	if err != nil {
		h.logger.Error("Failed to create vouch", zap.Error(err))
		if strings.Contains(err.Error(), "insufficient reputation") {
			return common.RespondBadRequest(ctx, "insufficient reputation to vouch")
		}
		if strings.Contains(err.Error(), "monthly vouch limit") {
			return common.RespondBadRequest(ctx, "monthly vouch limit reached")
		}
		return common.RespondInternalServerError(ctx)
	}

	response := apimodels.VouchResponse{
		ID:                vouch.ID,
		From:              vouch.From,
		To:                vouch.To,
		Confidence:        vouch.Confidence,
		Context:           vouch.Context,
		CreatedAt:         vouch.CreatedAt,
		ExpiresAt:         vouch.ExpiresAt,
		VoucherReputation: vouch.VoucherReputation,
		Active:            vouch.Active,
		Revoked:           vouch.Revoked,
		RevokedAt:         vouch.RevokedAt,
	}

	return ctx.Status(http.StatusCreated).JSON(response)
}

// HandleGetVouchesLift handles GET /api/v1/vouches/:actor_id
func (h *Handler) HandleGetVouchesLift(ctx *lift.Context) error {
	// Get actorID from path parameter
	actorID := ctx.Param("actor_id")
	if err := common.ValidateRequiredParam("actor_id", actorID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Authentication required - extract and validate token
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = ctx.Header("authorization")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	_, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx)
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
		return common.RespondInternalServerError(ctx)
	}

	// Get vouches
	vouches, err := repService.GetVouches(ctx.Context, actorID)
	if err != nil {
		h.logger.Error("Failed to get vouches", zap.Error(err), zap.String("actor", actorID))

		// Check if it's an actor not found error
		if strings.Contains(err.Error(), "actor not found") {
			return common.RespondActorNotFound(ctx)
		}

		return common.RespondInternalServerError(ctx)
	}

	// Convert to API response
	apiVouches := make([]apimodels.VouchResponse, len(vouches))
	for i := range vouches {
		apiVouches[i] = apimodels.VouchResponse{
			ID:                vouches[i].ID,
			From:              vouches[i].From,
			To:                vouches[i].To,
			Confidence:        vouches[i].Confidence,
			Context:           vouches[i].Context,
			CreatedAt:         vouches[i].CreatedAt,
			ExpiresAt:         vouches[i].ExpiresAt,
			VoucherReputation: vouches[i].VoucherReputation,
			Active:            vouches[i].Active,
			Revoked:           vouches[i].Revoked,
			RevokedAt:         vouches[i].RevokedAt,
		}
	}

	return ctx.Status(http.StatusOK).JSON(apiVouches)
}

// HandleRevokeVouchLift handles DELETE /api/v1/vouches/:vouch_id
func (h *Handler) HandleRevokeVouchLift(ctx *lift.Context) error {
	// Get vouchID from path parameter
	vouchID := ctx.Param("vouch_id")
	if err := common.ValidateRequiredParam("vouchID", vouchID); err != nil {
		return common.RespondBadRequest(ctx, "missing vouch_id parameter")
	}

	var username string
	var claims *auth.Claims

	// Authentication required - extract and validate token
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam("authHeader", authHeader) != nil {
		authHeader = ctx.Header("authorization")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	username = claims.Username

	// Get the actor ID for the authenticated user
	actorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Revoke vouch
	err = repService.RevokeVouch(ctx.Context, vouchID, actorID)
	if err != nil {
		h.logger.Error("Failed to revoke vouch", zap.Error(err))
		if strings.Contains(err.Error(), "only the voucher can revoke") {
			return common.RespondForbidden(ctx, "only the voucher can revoke their vouch")
		}
		return common.RespondInternalServerError(ctx)
	}

	ctx.Status(http.StatusNoContent)
	return nil
}

// HandleVerifyReputationLift handles POST /api/v1/reputation/verify
func (h *Handler) HandleVerifyReputationLift(ctx *lift.Context) error {
	// Parse request body
	var verifyReq apimodels.ReputationDocumentRequest
	if err := ctx.ParseRequest(&verifyReq); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &verifyReq); err != nil {
				return common.RespondBadRequest(ctx, "invalid request body")
			}
		} else {
			return common.RespondBadRequest(ctx, "invalid request body")
		}
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Verify reputation
	result, err := repService.VerifyReputation(ctx.Context, verifyReq.Document)
	if err != nil {
		h.logger.Error("Failed to verify reputation", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return ctx.Status(http.StatusOK).JSON(result)
}

// HandleGetReputationKeysLift handles GET /.well-known/reputation-keys
func (h *Handler) HandleGetReputationKeysLift(ctx *lift.Context) error {
	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	// Get public key
	publicKey := repService.GetPublicKey()

	return ctx.Status(http.StatusOK).JSON(apimodels.ReputationKeysResponse{PublicKey: publicKey})
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
