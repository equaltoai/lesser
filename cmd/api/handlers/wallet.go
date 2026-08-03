package handlers

import (
	"net/http"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

// HandleCreateChallengeLift handles POST /auth/wallet/challenge
func (h *Handler) HandleCreateChallengeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	var req apimodels.WalletChallengeRequest

	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
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
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Create challenge
	challenge, err := authService.CreateWalletChallenge(ctx.Context(), req.Address, req.ChainID, req.Username)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "create challenge")
	}

	return okJSON(challenge)
}

// HandleVerifySignatureLift handles POST /auth/wallet/verify (for registration)
func (h *Handler) HandleVerifySignatureLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	var req auth.WalletVerifyRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
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
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Verify signature only (for registration flow)
	err = authService.VerifyWalletSignature(ctx.Context(), &req)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "verify signature")
	}

	return okJSON(apimodels.WalletVerifyResponse{
		Verified: true,
		Message:  "signature verified successfully",
	})
}

// HandleLoginWalletLift handles POST /auth/wallet/login (for login)
func (h *Handler) HandleLoginWalletLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	var req auth.WalletVerifyRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
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
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Login with wallet (wallet must be linked)
	authResponse, err := authService.LoginWithWallet(ctx.Context(), &req, deviceName, userAgent, ipAddress)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "login with wallet")
	}

	return okJSON(authResponse)
}

// HandleLinkWalletLift handles POST /auth/wallet/link
//
//nolint:gocognit,gocyclo // Wallet linking flow includes many security checks and early exits.
func (h *Handler) HandleLinkWalletLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Try to get authenticated user (for existing users)
	// But allow linking during registration if signature is valid
	username := h.getAuthenticatedUserLift(ctx)
	isAuthenticated := strings.TrimSpace(username) != ""

	var req apimodels.WalletLinkRequest

	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
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
	if !isAuthenticated {
		// No auth token - this is registration flow
		// Use username from request if provided
		if req.Username != "" {
			username = req.Username
		} else {
			return h.respondWithError(ctx, http.StatusUnauthorized, "authentication required or username must be provided")
		}
	}

	// Require signature-based proof for linking (wallet ownership).
	if err := common.ValidateRequiredParam("challengeId", req.ChallengeID); err != nil {
		return h.respondBadRequest(ctx, "challengeId is required")
	}
	if err := common.ValidateRequiredParam("signature", req.Signature); err != nil {
		return h.respondBadRequest(ctx, "signature is required")
	}
	if err := common.ValidateRequiredParam("message", req.Message); err != nil {
		return h.respondBadRequest(ctx, "message is required")
	}

	// Get auth service
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get the challenge to verify username binding
	challenge, err := authService.GetWalletChallenge(ctx.Context(), req.ChallengeID)
	if err != nil {
		h.logger.Error("failed to get challenge for verification",
			zap.String("challengeId", req.ChallengeID),
			zap.Error(err))
		return h.respondWithError(ctx, http.StatusUnauthorized, "invalid or expired challenge")
	}

	// CRITICAL: Verify the challenge username matches the requested username
	// This prevents replay attacks where someone uses your signature for a different username.
	if challenge.Username != username {
		h.logger.Error("username mismatch - signature was bound to different username",
			zap.String("challenge_username", challenge.Username),
			zap.String("requested_username", username),
			zap.String("address", req.Address))
		return h.respondWithError(ctx, http.StatusUnauthorized, "signature was created for a different username - replay attack prevented")
	}

	// Check if challenge is already spent.
	if challenge.Spent {
		h.logger.Error("challenge already spent",
			zap.String("challengeId", req.ChallengeID))
		return h.respondWithError(ctx, http.StatusUnauthorized, "challenge already used")
	}

	// Check that the message matches the challenge (defense-in-depth).
	if strings.TrimSpace(req.Message) != strings.TrimSpace(challenge.Message) {
		return h.respondWithError(ctx, http.StatusUnauthorized, "message mismatch")
	}

	// Enforce that unauthenticated wallet linking is only allowed immediately after registration.
	// The registration handler marks the verified challenge on the typed wallet challenge row.
	accountRepo := h.repos.Account()
	if accountRepo == nil {
		return h.respondWithError(ctx, http.StatusInternalServerError, "internal server error")
	}
	user, err := accountRepo.GetUser(ctx.Context(), username)
	if err != nil || user == nil {
		return h.respondBadRequest(ctx, "user not found")
	}

	if !isAuthenticated && !challenge.RegistrationCompleted {
		return h.respondWithError(ctx, http.StatusUnauthorized, "registration challenge mismatch")
	}

	// Verify the signature directly (single verification).
	// The challenge was already verified once in /auth/wallet/verify; we verify again as defense-in-depth.
	if err := authService.VerifySignatureOnly(ctx.Context(), challenge, req.Signature); err != nil {
		h.logger.Error("signature verification failed for wallet link",
			zap.String("address", req.Address),
			zap.Error(err))
		return h.respondWithError(ctx, http.StatusUnauthorized, "signature verification failed")
	}

	// Mark challenge as spent (second and final use) before linking the wallet.
	// If this write fails, fail closed so a completed registration proof cannot
	// be replayed into a fresh session/JWT.
	if err := authService.MarkWalletChallengeSpent(ctx.Context(), req.ChallengeID); err != nil {
		h.logger.Error("failed to mark challenge as spent",
			zap.String("challengeId", req.ChallengeID),
			zap.String("username", username),
			zap.Error(err))
		return h.respondWithError(ctx, http.StatusInternalServerError, "failed to finalize wallet challenge")
	}

	// Link the wallet
	walletLinkCreated, err := authService.LinkWallet(ctx.Context(), username, req.Address, req.ChainID, req.WalletType)
	if err != nil {
		if resetErr := authService.ResetWalletChallengeSpent(ctx.Context(), req.ChallengeID); resetErr != nil {
			h.logger.Error("failed to restore wallet challenge after wallet-link failure",
				zap.String("challengeId", req.ChallengeID),
				zap.String("username", username),
				zap.String("address", req.Address),
				zap.Error(resetErr))
		}
		h.logger.Error("failed to link wallet",
			zap.String("username", username),
			zap.String("address", req.Address),
			zap.Error(err))
		return h.respondWithError(ctx, http.StatusInternalServerError, "failed to link wallet")
	}

	// If this is a registration flow (signature provided but no auth token), create session
	// This creates a server-side session so /oauth/authorize can authenticate the user
	// Signature was already verified in /auth/wallet/verify, so we can create session directly
	if !isAuthenticated {
		// Get device info for session creation
		userAgent, ipAddress := h.getDeviceInfo(ctx)
		deviceName := userAgent
		if deviceName == "" {
			deviceName = "Unknown Device"
		}

		// Create session (signature already verified, no re-verification needed)
		// This session will be used by /oauth/authorize to generate authorization code
		authResponse, err := authService.LoginWithWalletAfterLinking(ctx.Context(), username, deviceName, userAgent, ipAddress)
		if err != nil {
			if walletLinkCreated {
				if unlinkErr := authService.UnlinkWallet(ctx.Context(), username, req.Address); unlinkErr != nil {
					h.logger.Error("failed to rollback wallet link after session creation failure",
						zap.String("username", username),
						zap.String("address", req.Address),
						zap.String("challengeId", req.ChallengeID),
						zap.Error(unlinkErr))
				} else if resetErr := authService.ResetWalletChallengeSpent(ctx.Context(), req.ChallengeID); resetErr != nil {
					h.logger.Error("failed to restore wallet challenge after session creation failure",
						zap.String("username", username),
						zap.String("address", req.Address),
						zap.String("challengeId", req.ChallengeID),
						zap.Error(resetErr))
					if _, relinkErr := authService.LinkWallet(ctx.Context(), username, req.Address, req.ChainID, req.WalletType); relinkErr != nil {
						h.logger.Error("failed to restore wallet link after challenge reset failure",
							zap.String("username", username),
							zap.String("address", req.Address),
							zap.String("challengeId", req.ChallengeID),
							zap.Error(relinkErr))
					}
				}
			}
			h.logger.Error("failed to create session after wallet linking",
				zap.String("username", username),
				zap.String("address", req.Address),
				zap.Error(err))
			return h.respondWithError(ctx, http.StatusInternalServerError, "failed to create session")
		}

		// Return success with JWT for stateless OAuth flow.
		// Client will use this JWT to authenticate with /oauth/authorize.
		return okJSON(apimodels.WalletLinkResponse{
			Success:     true,
			Message:     "wallet linked successfully",
			Address:     req.Address,
			AccessToken: authResponse.AccessToken,
			TokenType:   authResponse.TokenType,
			ExpiresIn:   authResponse.ExpiresIn,
		})
	}

	return okJSON(apimodels.WalletLinkResponse{
		Success: true,
		Message: "wallet linked successfully",
		Address: req.Address,
	})
}

// HandleUnlinkWalletLift handles DELETE /auth/wallet/unlink/{address}
func (h *Handler) HandleUnlinkWalletLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Check for test mode first
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondWithError(ctx, http.StatusUnauthorized, "authentication required")
	}

	// Get address from path
	address := ctx.Param("address")
	if err := common.ValidateRequiredParam("address", address); err != nil {
		return h.respondBadRequest(ctx, "address is required")
	}

	// Initialize auth service
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Unlink the wallet
	if err := authService.UnlinkWallet(ctx.Context(), username, address); err != nil {
		h.logger.Error("failed to unlink wallet",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return h.respondWithError(ctx, http.StatusInternalServerError, "failed to unlink wallet")
	}

	return okJSON(apimodels.WalletUnlinkResponse{
		Success: true,
		Message: "wallet unlinked successfully",
		Address: address,
	})
}

// HandleGetWalletsLift handles GET /auth/wallet/list
func (h *Handler) HandleGetWalletsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Check for test mode first
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondWithError(ctx, http.StatusUnauthorized, "authentication required")
	}

	// Initialize auth service
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get user's wallets
	wallets, err := authService.GetUserWallets(ctx.Context(), username)
	if err != nil {
		h.logger.Error("failed to get user wallets",
			zap.String("username", username),
			zap.Error(err))
		return h.respondWithError(ctx, http.StatusInternalServerError, "failed to get wallets")
	}

	return okJSON(apimodels.WalletListResponse{
		Wallets: wallets,
		Count:   len(wallets),
	})
}

// getAuthenticatedUserLift gets the authenticated user from the context
func (h *Handler) getAuthenticatedUserLift(ctx *apptheory.Context) string {
	return h.getOptionalAuthenticatedUser(ctx)
}
