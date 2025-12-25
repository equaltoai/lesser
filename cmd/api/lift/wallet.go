package lift

import (
	"net/http"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleCreateChallengeLift handles POST /auth/wallet/challenge
func (h *Handler) HandleCreateChallengeLift(ctx *lift.Context) error {
	var req apimodels.WalletChallengeRequest

	if err := h.parseRequestBody(ctx, &req); err != nil {
		return err // Error response already set by parseRequestBody
	}

	// Validate inputs
	if err := common.ValidateRequiredParam("address", req.Address); err != nil {
		return h.respondBadRequest(ctx, "address is required")
	}
	if err := common.ValidateRequiredParam("username", req.Username); err != nil {
		return h.respondBadRequest(ctx, "username is required for security (binds signature to username)")
	}
	if req.ChainID == 0 {
		req.ChainID = 1 // Default to Ethereum mainnet
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

// HandleVerifySignatureLift handles POST /auth/wallet/verify (for registration)
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

	// Get auth service
	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err // Error response already set
	}

	// Verify signature only (for registration flow)
	err = authService.VerifyWalletSignature(ctx.Context, &req)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "verify signature")
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(apimodels.WalletVerifyResponse{
		Verified: true,
		Message:  "signature verified successfully",
	})
}

// HandleLoginWalletLift handles POST /auth/wallet/login (for login)
func (h *Handler) HandleLoginWalletLift(ctx *lift.Context) error {
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

	// Login with wallet (wallet must be linked)
	authResponse, err := authService.LoginWithWallet(ctx.Context, &req, deviceName, userAgent, ipAddress)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "login with wallet")
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(authResponse)
}

// HandleLinkWalletLift handles POST /auth/wallet/link
func (h *Handler) HandleLinkWalletLift(ctx *lift.Context) error {
	// Try to get authenticated user (for existing users)
	// But allow linking during registration if signature is valid
	username := h.getAuthenticatedUserLift(ctx)

	var req apimodels.WalletLinkRequest

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
	if err := common.ValidateRequiredParam("wallet_type", req.WalletType); err != nil {
		req.WalletType = "ethereum"
	}

	// Determine username
	if username == "" {
		// No auth token - this is registration flow
		// Use username from request if provided
		if req.Username != "" {
			username = req.Username
		} else {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required or username must be provided",
			})
		}
	}

	// Get auth service
	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err // Error response already set
	}

	// If signature provided, verify it (for registration flow or manual verification)
	if req.ChallengeID != "" && req.Signature != "" && req.Message != "" {
		// Get the challenge to verify username binding
		challenge, err := authService.GetWalletChallenge(ctx.Context, req.ChallengeID)
		if err != nil {
			h.logger.Error("failed to get challenge for verification",
				zap.String("challengeId", req.ChallengeID),
				zap.Error(err))
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid or expired challenge",
			})
		}

		// CRITICAL: Verify the challenge username matches the requested username
		// This prevents replay attacks where someone uses your signature for a different username
		if challenge.Username != username {
			h.logger.Error("username mismatch - signature was bound to different username",
				zap.String("challenge_username", challenge.Username),
				zap.String("requested_username", username),
				zap.String("address", req.Address))
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "signature was created for a different username - replay attack prevented",
			})
		}

		// Check if challenge is already spent
		if challenge.Spent {
			h.logger.Error("challenge already spent",
				zap.String("challengeId", req.ChallengeID))
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "challenge already used",
			})
		}

		// Verify the signature directly (single verification)
		// The challenge was already verified once in /auth/wallet/verify
		// We just need to verify the signature matches
		if err := authService.VerifySignatureOnly(ctx.Context, challenge, req.Signature); err != nil {
			h.logger.Error("signature verification failed for wallet link",
				zap.String("address", req.Address),
				zap.Error(err))
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "signature verification failed",
			})
		}

		// Mark challenge as spent (second and final use)
		if err := authService.MarkWalletChallengeSpent(ctx.Context, req.ChallengeID); err != nil {
			h.logger.Warn("failed to mark challenge as spent", zap.Error(err))
			// Non-fatal - continue
		}
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

	// If this is a registration flow (signature provided but no auth token), create session
	// This creates a server-side session so /oauth/authorize can authenticate the user
	// Signature was already verified in /auth/wallet/verify, so we can create session directly
	if req.ChallengeID != "" && req.Signature != "" && req.Message != "" {
		// Get device info for session creation
		userAgent, ipAddress := h.getDeviceInfo(ctx)
		deviceName := userAgent
		if deviceName == "" {
			deviceName = "Unknown Device"
		}

		// Create session (signature already verified, no re-verification needed)
		// This session will be used by /oauth/authorize to generate authorization code
		authResponse, err := authService.LoginWithWalletAfterLinking(ctx.Context, username, deviceName, userAgent, ipAddress)
		if err != nil {
			h.logger.Error("failed to create session after wallet linking",
				zap.String("username", username),
				zap.String("address", req.Address),
				zap.Error(err))
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{
				"error": "failed to create session",
			})
		}

		// Return success with JWT for stateless OAuth flow
		// Client will use this JWT to authenticate with /oauth/authorize
		ctx.Status(http.StatusOK)
		return ctx.JSON(apimodels.WalletLinkResponse{
			Success:     true,
			Message:     "wallet linked successfully",
			Address:     req.Address,
			AccessToken: authResponse.AccessToken,
			TokenType:   authResponse.TokenType,
			ExpiresIn:   authResponse.ExpiresIn,
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(apimodels.WalletLinkResponse{
		Success: true,
		Message: "wallet linked successfully",
		Address: req.Address,
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
	authService, err := auth.NewAuthService(h.cfg, h.repos)
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
	return ctx.JSON(apimodels.WalletUnlinkResponse{
		Success: true,
		Message: "wallet unlinked successfully",
		Address: address,
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
	authService, err := auth.NewAuthService(h.cfg, h.repos)
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
	return ctx.JSON(apimodels.WalletListResponse{
		Wallets: wallets,
		Count:   len(wallets),
	})
}

// getAuthenticatedUserLift gets the authenticated user from the context
func (h *Handler) getAuthenticatedUserLift(ctx *lift.Context) string {
	// Get bearer token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return ""
	}

	// Create auth service and validate token
	authService, err := auth.NewAuthService(h.cfg, h.repos)
	if err != nil {
		return ""
	}

	claims, err := authService.ValidateAccessToken(token)
	if err != nil {
		return ""
	}

	return claims.Username
}
