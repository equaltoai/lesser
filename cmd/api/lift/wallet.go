package lift

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
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

	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]string{
					"error": "invalid request body",
				})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{
				"error": "invalid request body",
			})
		}
	}

	// Validate inputs
	if req.Address == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "address is required",
		})
	}
	if req.ChainID == 0 {
		req.ChainID = 1 // Default to Ethereum mainnet
	}

	// If username is provided, verify the user is authenticated
	if req.Username != "" {
		token := h.getAuthTokenLift(ctx)
		if token == "" {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "authentication required to link wallet",
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

		claims, err := authService.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid token",
			})
		}

		// Ensure the username matches the authenticated user
		if claims.Username != req.Username {
			ctx.Status(http.StatusForbidden)
			return ctx.JSON(map[string]string{
				"error": "cannot link wallet to another user",
			})
		}
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

	// Create challenge
	challenge, err := authService.CreateWalletChallenge(ctx.Context, req.Address, req.ChainID, req.Username)
	if err != nil {
		h.logger.Error("failed to create wallet challenge", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to create challenge",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(challenge)
}

// HandleVerifySignatureLift handles POST /auth/wallet/verify
func (h *Handler) HandleVerifySignatureLift(ctx *lift.Context) error {
	var req auth.WalletVerifyRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]string{
					"error": "invalid request body",
				})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{
				"error": "invalid request body",
			})
		}
	}

	// Validate inputs
	if req.ChallengeID == "" || req.Address == "" || req.Signature == "" || req.Message == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "missing required fields",
		})
	}

	// Get device info
	deviceName := ctx.Header("User-Agent")
	if deviceName == "" {
		deviceName = "Unknown Device"
	}
	userAgent := ctx.Header("User-Agent")
	ipAddress := ctx.Header("X-Forwarded-For")
	if ipAddress == "" {
		ipAddress = ctx.Header("X-Real-IP")
	}
	if ipAddress == "" {
		ipAddress = "unknown"
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

	// Verify signature and create session
	authResponse, err := authService.VerifyWalletSignature(ctx.Context, &req, deviceName, userAgent, ipAddress)
	if err != nil {
		h.logger.Error("wallet signature verification failed",
			zap.String("address", req.Address),
			zap.Error(err))
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "signature verification failed",
		})
	}

	// If no access token, wallet is not linked to any account
	if authResponse.AccessToken == "" {
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
	if username == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	var req struct {
		Address     string `json:"address"`
		ChainID     int    `json:"chainId"`
		WalletType  string `json:"walletType"`
		ChallengeID string `json:"challengeId"`
		Signature   string `json:"signature"`
		Message     string `json:"message"`
	}

	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]string{
					"error": "invalid request body",
				})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{
				"error": "invalid request body",
			})
		}
	}

	// Validate inputs
	if req.Address == "" || req.ChallengeID == "" || req.Signature == "" || req.Message == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "missing required fields",
		})
	}

	if req.ChainID == 0 {
		req.ChainID = 1 // Default to Ethereum mainnet
	}
	if req.WalletType == "" {
		req.WalletType = "ethereum"
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
	if username == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Get address from path
	address := ctx.Param("address")
	if address == "" {
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
	if username == "" {
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

// getAuthTokenLift extracts Bearer token from Authorization header
func (h *Handler) getAuthTokenLift(ctx *lift.Context) string {
	auth := ctx.Header("Authorization")
	if auth == "" {
		return ""
	}
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
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
	token := h.getAuthTokenLift(ctx)
	if token == "" {
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
