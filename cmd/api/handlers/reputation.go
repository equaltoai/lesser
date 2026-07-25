package handlers

import (
	"errors"
	"fmt"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/reputation"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

// HandleGetReputationLift handles GET /api/v1/reputation/:actor_id
func (h *Handler) HandleGetReputationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get actorID from path parameter
	actorID := ctx.Param("actor_id")
	if err := common.ValidateRequiredParam("actor_id", actorID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	if _, authResp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	); authResp != nil || err != nil {
		return authResp, err
	}

	// Normalize actor ID
	// If it's not already a URI, resolve numeric account ID / handle / username to the canonical actor URI.
	if !strings.Contains(actorID, "://") {
		actor, resolveErr := h.resolveAccountID(ctx.Context(), actorID)
		if resolveErr != nil {
			if errors.Is(resolveErr, storage.ErrNotFound) || errorChainContains(resolveErr, "not found") || errorChainContains(resolveErr, "actor not found") {
				return common.RespondActorNotFound(ctx)
			}
			h.logger.Error("Failed to resolve actor for reputation lookup", zap.Error(resolveErr), zap.String("actor_id", actorID))
			return common.RespondInternalServerError(ctx)
		}

		if actor == nil || strings.TrimSpace(actor.ID) == "" {
			return common.RespondActorNotFound(ctx)
		}
		actorID = strings.TrimSpace(actor.ID)
	}

	if err := reputation.ValidateActorID(actorID); err != nil {
		return common.RespondBadRequest(ctx, "invalid actor_id")
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Get reputation
	rep, err := repService.GetReputation(ctx.Context(), actorID)
	if err != nil {
		h.logger.Error("Failed to get reputation", zap.Error(err), zap.String("actor", actorID))

		// Check if it's an actor not found error
		if errorChainContains(err, "actor not found") {
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

	return okJSON(response)
}

// HandleExportReputationLift handles POST /api/v1/reputation/export
func (h *Handler) HandleExportReputationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, authResp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if authResp != nil || err != nil {
		return authResp, err
	}

	// Get the actor ID for the authenticated user
	actorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Export reputation
	portableRep, err := repService.ExportReputation(ctx.Context(), actorID)
	if err != nil {
		h.logger.Error("Failed to export reputation", zap.Error(err), zap.String("actor", actorID))
		return common.RespondInternalServerError(ctx)
	}

	resp, err := okJSON(portableRep)
	if err != nil {
		return nil, err
	}
	setHeader(resp, "content-type", "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	return resp, nil
}

// HandleImportReputationLift handles POST /api/v1/reputation/import
func (h *Handler) HandleImportReputationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if _, authResp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	); authResp != nil || err != nil {
		return authResp, err
	}

	// Parse request body
	var importReq apimodels.ReputationDocumentRequest
	if err := common.ParseRequestWithFallback(ctx, &importReq); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Import reputation
	result, err := repService.ImportReputation(ctx.Context(), importReq.Document)
	if err != nil {
		h.logger.Error("Failed to import reputation", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(result)
}

// HandleCreateVouchLift handles POST /api/v1/vouches
func (h *Handler) HandleCreateVouchLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, authResp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if authResp != nil || err != nil {
		return authResp, err
	}

	// Parse request body
	var vouchReq apimodels.CreateVouchRequest
	if err := common.ParseRequestWithFallback(ctx, &vouchReq); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Validate input
	if err := common.ValidateRequiredParam("vouchTo", vouchReq.To); err != nil {
		return common.RespondBadRequest(ctx, "missing 'to' field")
	}
	if err := common.ValidateFloatRange("confidence", vouchReq.Confidence, 0, 1); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Get the actor ID for the authenticated user
	fromActorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Create vouch
	vouch, err := repService.CreateVouch(ctx.Context(), fromActorID, vouchReq.To, vouchReq.Confidence, vouchReq.Context)
	if err != nil {
		h.logger.Error("Failed to create vouch", zap.Error(err))
		if errorChainContains(err, "insufficient reputation") {
			return common.RespondBadRequest(ctx, "insufficient reputation to vouch")
		}
		if errorChainContains(err, "monthly vouch limit") {
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

	return createdJSON(response)
}

// HandleGetVouchesLift handles GET /api/v1/vouches/:actor_id
func (h *Handler) HandleGetVouchesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get actorID from path parameter
	actorID := ctx.Param("actor_id")
	if err := common.ValidateRequiredParam("actor_id", actorID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	if _, authResp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	); authResp != nil || err != nil {
		return authResp, err
	}

	// Normalize actor ID
	// If it's just a username, convert to full actor URI
	if !strings.Contains(actorID, "://") {
		actorID = fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, actorID)
	}
	if err := reputation.ValidateActorID(actorID); err != nil {
		return common.RespondBadRequest(ctx, "invalid actor_id")
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Get vouches
	vouches, err := repService.GetVouches(ctx.Context(), actorID)
	if err != nil {
		h.logger.Error("Failed to get vouches", zap.Error(err), zap.String("actor", actorID))

		// Check if it's an actor not found error
		if errorChainContains(err, "actor not found") {
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

	return okJSON(apiVouches)
}

// HandleRevokeVouchLift handles DELETE /api/v1/vouches/:vouch_id
func (h *Handler) HandleRevokeVouchLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get vouchID from path parameter
	vouchID := ctx.Param("vouch_id")
	if err := common.ValidateRequiredParam("vouchID", vouchID); err != nil {
		return common.RespondBadRequest(ctx, "missing vouch_id parameter")
	}

	claims, authResp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if authResp != nil || err != nil {
		return authResp, err
	}

	// Get the actor ID for the authenticated user
	actorID := fmt.Sprintf("https://%s/users/%s", h.cfg.Domain, claims.Username)

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Revoke vouch
	err = repService.RevokeVouch(ctx.Context(), vouchID, actorID)
	if err != nil {
		h.logger.Error("Failed to revoke vouch", zap.Error(err))
		if errorChainContains(err, "only the voucher can revoke") {
			return common.RespondForbidden(ctx, "only the voucher can revoke their vouch")
		}
		return common.RespondInternalServerError(ctx)
	}

	return noContent(), nil
}

// HandleVerifyReputationLift handles POST /api/v1/reputation/verify
func (h *Handler) HandleVerifyReputationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse request body
	var verifyReq apimodels.ReputationDocumentRequest
	if err := common.ParseRequestWithFallback(ctx, &verifyReq); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		h.logger.Error("Failed to initialize reputation service", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Verify reputation
	result, err := repService.VerifyReputation(ctx.Context(), verifyReq.Document)
	if err != nil {
		h.logger.Error("Failed to verify reputation", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return okJSON(result)
}

// HandleGetReputationKeysLift handles GET /.well-known/reputation-keys
func (h *Handler) HandleGetReputationKeysLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Initialize reputation service
	repService, err := h.getReputationService()
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	// Get public key
	publicKey := repService.GetPublicKey()

	return okJSON(apimodels.ReputationKeysResponse{PublicKey: publicKey})
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

func errorChainContains(err error, substring string) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), substring) {
		return true
	}

	type multiUnwrapper interface {
		Unwrap() []error
	}
	if u, ok := err.(multiUnwrapper); ok {
		for _, nested := range u.Unwrap() {
			if errorChainContains(nested, substring) {
				return true
			}
		}
	}

	type unwrapper interface {
		Unwrap() error
	}
	if u, ok := err.(unwrapper); ok {
		return errorChainContains(u.Unwrap(), substring)
	}

	return false
}
