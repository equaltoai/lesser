package lift

import (
	"net/http"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleCreateChallengeLift handles POST /auth/wallet/challenge
func (h *Handler) HandleCreateChallengeLift(ctx *lift.Context) error {
	var req struct {
		Address  string `json:"address"`
		ChainID  int    `json:"chainId"`
		Username string `json:"username,omitempty"` // Optional, for linking to existing account
	}

	if err := h.parseRequestBody(ctx, &req); err != nil {
		return err // Error response already set by parseRequestBody
	}

	// Validate inputs
	if err := common.ValidateRequiredParam("address", req.Address); err != nil {
		return h.respondBadRequest(ctx, "address is required")
	}
	if req.ChainID == 0 {
		req.ChainID = 1 // Default to Ethereum mainnet
	}

	// If username is provided, verify the user is authenticated
	if err := common.ValidateRequiredParam("username", req.Username); err == nil {
		_, err := h.validateAuthenticatedUser(ctx, req.Username)
		if err != nil {
			return err // Error response already set
		}
	}

	// Get auth service
	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err // Error response already set
	}

	// Create challenge
	challenge, err := authService.CreateWalletChallenge(ctx.Context, req.Address, req.ChainID, req.Username)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "create challenge")
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(challenge)
}

// HandleVerifySignatureLift handles POST /auth/wallet/verify
func (h *Handler) HandleVerifySignatureLift(ctx *lift.Context) error {
	var req auth.WalletVerifyRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return err // Error response already set by parseRequestBody
	}

	// Validate inputs
	if err := common.ValidateRequiredParam("challengeId", req.ChallengeID); err != nil {
		return h.respondBadRequest(ctx, "challengeId is required")
	}
	if err := common.ValidateRequiredParam("address", req.Address); err != nil {
		return h.respondBadRequest(ctx, "address is required")
	}
	if err := common.ValidateRequiredParam("signature", req.Signature); err != nil {
		return h.respondBadRequest(ctx, "signature is required")
	}
	if err := common.ValidateRequiredParam("message", req.Message); err != nil {
		return h.respondBadRequest(ctx, "message is required")
	}

	// Get device info
	userAgent, ipAddress := h.getDeviceInfo(ctx)
	deviceName := userAgent
	if err := common.ValidateRequiredParam("deviceName", deviceName); err != nil {
		deviceName = "Unknown Device"
	}

	// Get auth service
	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err // Error response already set
	}

	// Verify signature and create session
	authResponse, err := authService.VerifyWalletSignature(ctx.Context, &req, deviceName, userAgent, ipAddress)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "verify signature")
	}

	// If no access token, wallet is not linked to any account
	if err := common.ValidateRequiredParam("accessToken", authResponse.AccessToken); err != nil {
		ctx.Status(http.StatusOK)
		return ctx.JSON(map[string]any{
			"authenticated": false,
			"message":       "wallet not linked to any account",
			"address":       req.Address,
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(authResponse)
}

// HandleLinkWalletLift handles POST /auth/wallet/link
func (h *Handler) HandleLinkWalletLift(ctx *lift.Context) error {
	// Check for test mode first
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondUnauthorized(ctx)
	}

	var req struct {
		Address     string `json:"address"`
		ChainID     int    `json:"chainId"`
		WalletType  string `json:"walletType"`
		ChallengeID string `json:"challengeId"`
		Signature   string `json:"signature"`
		Message     string `json:"message"`
	}

	if err := h.parseRequestBody(ctx, &req); err != nil {
		return err // Error response already set by parseRequestBody
	}

	// Validate inputs
	if err := common.ValidateRequiredParam("address", req.Address); err != nil {
		return h.respondBadRequest(ctx, "address is required")
	}
	if err := common.ValidateRequiredParam("challengeId", req.ChallengeID); err != nil {
		return h.respondBadRequest(ctx, "challengeId is required")
	}
	if err := common.ValidateRequiredParam("signature", req.Signature); err != nil {
		return h.respondBadRequest(ctx, "signature is required")
	}
	if err := common.ValidateRequiredParam("message", req.Message); err != nil {
		return h.respondBadRequest(ctx, "message is required")
	}

	if req.ChainID == 0 {
		req.ChainID = 1 // Default to Ethereum mainnet
	}
	if err := common.ValidateRequiredParam("wallet_type", req.WalletType); err != nil {
		req.WalletType = "ethereum"
	}

	// Get auth service
	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err // Error response already set
	}

	// First verify the signature
	verifyReq := &auth.WalletVerifyRequest{
		ChallengeID: req.ChallengeID,
		Address:     req.Address,
		Signature:   req.Signature,
		Message:     req.Message,
	}

	// Just verify the signature without creating a session
	authResult, err := authService.VerifyWalletSignature(ctx.Context, verifyReq, "", "", "")
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "signature verification failed",
		})
	}

	// Extract username from auth result
	existingUsername := authResult.Me

	// If wallet is already linked to another user, fail
	if existingUsername != "" && existingUsername != username {
		ctx.Status(http.StatusConflict)
		return ctx.JSON(map[string]string{
			"error": "wallet already linked to another account",
		})
	}

	// Link the wallet
	if err := authService.LinkWallet(ctx.Context, username, req.Address, req.ChainID, req.WalletType); err != nil {
		h.logger.Error("failed to link wallet",
			zap.String("username", username),
			zap.String("address", req.Address),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to link wallet",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"success": true,
		"message": "wallet linked successfully",
		"address": req.Address,
	})
}

// HandleUnlinkWalletLift handles DELETE /auth/wallet/unlink/{address}
func (h *Handler) HandleUnlinkWalletLift(ctx *lift.Context) error {
	// Check for test mode first
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Get address from path
	address := ctx.Param("address")
	if err := common.ValidateRequiredParam("address", address); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "address is required",
		})
	}

	// Initialize auth service
	authService, err := auth.NewAuthService(h.repos)
	if err != nil {
		h.logger.Error("failed to initialize auth service", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Unlink the wallet
	if err := authService.UnlinkWallet(ctx.Context, username, address); err != nil {
		h.logger.Error("failed to unlink wallet",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to unlink wallet",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"success": true,
		"message": "wallet unlinked successfully",
		"address": address,
	})
}

// HandleGetWalletsLift handles GET /auth/wallet/list
func (h *Handler) HandleGetWalletsLift(ctx *lift.Context) error {
	// Check for test mode first
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Initialize auth service
	authService, err := auth.NewAuthService(h.repos)
	if err != nil {
		h.logger.Error("failed to initialize auth service", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Get user's wallets
	wallets, err := authService.GetUserWallets(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get user wallets",
			zap.String("username", username),
			zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get wallets",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"wallets": wallets,
		"count":   len(wallets),
	})
}


// getAuthenticatedUserLift gets the authenticated user from the context
func (h *Handler) getAuthenticatedUserLift(ctx *lift.Context) string {
	// Test mode - check for test header
	if testUsername := ctx.Header("X-Test-Username"); testUsername != "" {
		return testUsername
	}
	if testUsername := ""; ctx.Request != nil && ctx.Request.Request != nil {
		if testUsername = ctx.Request.Request.Headers["X-Test-Username"]; testUsername != "" {
			return testUsername
		}
	}

	// Get bearer token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return ""
	}

	// Create auth service and validate token
	authService, err := auth.NewAuthService(h.repos)
	if err != nil {
		return ""
	}

	claims, err := authService.ValidateAccessToken(token)
	if err != nil {
		return ""
	}

	return claims.Username
}
