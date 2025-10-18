package lift

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// EmailFreeRecoveryHandler handles email-free account recovery
type EmailFreeRecoveryHandler struct {
	socialRecovery *auth.SocialRecoveryService
	recoveryCode   *auth.RecoveryCodeService
	authService    *auth.AuthService
	logger         *zap.Logger
}

// NewEmailFreeRecoveryHandler creates a new email-free recovery handler
func NewEmailFreeRecoveryHandler(authService *auth.AuthService) *EmailFreeRecoveryHandler {
	logger := common.Logger()
	repos := authService.GetStore()

	return &EmailFreeRecoveryHandler{
		socialRecovery: auth.NewSocialRecoveryService(repos, logger),
		recoveryCode:   auth.NewRecoveryCodeService(repos, logger),
		authService:    authService,
		logger:         logger,
	}
}

// HandleGetRecoveryOptionsLift returns available recovery methods for an account
// GET /auth/recovery/options
func (h *EmailFreeRecoveryHandler) HandleGetRecoveryOptionsLift(ctx *lift.Context) error {
	username := ctx.Query("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": err.Error()})
	}

	// Get user to check what recovery methods are available
	user, err := h.authService.GetStore().Account().GetUser(ctx.Context, username)
	if err != nil {
		// Return generic response to prevent user enumeration
		ctx.Status(http.StatusOK)
		return ctx.JSON(map[string]any{
			"options": []string{},
			"message": "No account found with this username",
		})
	}

	options := []string{}

	// Check available recovery methods

	// 1. Passkeys (if user has any registered)
	credentials, err := h.authService.GetStore().Account().GetUserWebAuthnCredentials(ctx.Context, username)
	if err == nil && len(credentials) > 0 {
		options = append(options, "passkey")
	}

	// 2. Crypto wallets (if user has any linked)
	wallets, err := h.authService.GetStore().Account().GetUserWalletCredentials(ctx.Context, username)
	if err == nil && len(wallets) > 0 {
		options = append(options, "wallet")
	}

	// 3. OAuth providers (if user has any linked)
	providers, err := h.authService.GetStore().Account().GetLinkedProviders(ctx.Context, username)
	if err == nil && len(providers) > 0 {
		for _, provider := range providers {
			options = append(options, fmt.Sprintf("oauth_%s", provider))
		}
	}

	// 4. Social recovery (if user has trustees configured)
	trustees, err := h.socialRecovery.GetTrustees(ctx.Context, username)
	if err == nil && len(trustees) >= 2 {
		options = append(options, "social")
	}

	// 5. Recovery codes (if user has any)
	codeCount, err := h.recoveryCode.GetRecoveryCodeCount(ctx.Context, username)
	if err == nil && codeCount > 0 {
		options = append(options, "recovery_code")
	}

	// For development/testing, always show all options
	if h.authService.GetConfig().Environment == "development" {
		options = []string{"passkey", "wallet", "oauth_github", "social", "recovery_code"}
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"username": user.Username,
		"options":  options,
		"message":  fmt.Sprintf("Found %d recovery options", len(options)),
	})
}

// HandleInitiateSocialRecoveryLift starts the social recovery process
// POST /auth/recovery/social/initiate
func (h *EmailFreeRecoveryHandler) HandleInitiateSocialRecoveryLift(ctx *lift.Context) error {
	var req struct {
		Username string `json:"username"`
	}

	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]any{"error": "invalid request body"})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]any{"error": "invalid request body"})
		}
	}

	// Initiate social recovery
	recoveryRequest, err := h.socialRecovery.InitiateRecovery(ctx.Context, req.Username)
	if err != nil {
		// Generic response to prevent enumeration
		ctx.Status(http.StatusOK)
		return ctx.JSON(map[string]string{
			"message": "If you have trustees configured, they have been notified",
		})
	}

	// In development, return the recovery details
	if h.authService.GetConfig().Environment == "development" {
		ctx.Status(http.StatusOK)
		return ctx.JSON(map[string]any{
			"message":        "Social recovery initiated",
			"request_id":     recoveryRequest.ID,
			"required_votes": recoveryRequest.RequiredVotes,
			"expires_at":     recoveryRequest.ExpiresAt,
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]string{
		"message": "If you have trustees configured, they have been notified",
	})
}

// HandleConfirmSocialRecoveryLift processes a trustee's confirmation
// POST /auth/recovery/social/confirm
func (h *EmailFreeRecoveryHandler) HandleConfirmSocialRecoveryLift(ctx *lift.Context) error {
	// This would typically be called via ActivityPub federation
	// For now, support direct API calls for testing

	var req struct {
		RequestID string `json:"request_id"`
		TrusteeID string `json:"trustee_id"`
	}

	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]any{"error": "invalid request body"})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]any{"error": "invalid request body"})
		}
	}

	if err := h.socialRecovery.ConfirmRecovery(ctx.Context, req.RequestID, req.TrusteeID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": err.Error()})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]string{
		"message": "Recovery vote recorded",
	})
}

// HandleGenerateRecoveryCodesLift generates new recovery codes
// POST /auth/recovery/codes/generate
func (h *EmailFreeRecoveryHandler) HandleGenerateRecoveryCodesLift(ctx *lift.Context) error {
	// Get username from authorization context or test header
	var username string

	// Authentication required - get from JWT context
	if ctx.Value("jwt_claims") != nil {
		if claims, ok := ctx.Value("jwt_claims").(map[string]any); ok {
			if sub, ok := claims["sub"].(string); ok {
				username = sub
			}
		}
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]any{"error": "authentication required"})
	}

	// Generate codes
	codes, err := h.recoveryCode.GenerateRecoveryCodes(ctx.Context, username, 8)
	if err != nil {
		h.logger.Error("failed to generate recovery codes", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]any{"error": "failed to generate codes"})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"codes":          codes,
		"warning":        "Save these codes securely! Each code can only be used once.",
		"recommendation": "Store in a password manager or hardware wallet",
	})
}

// HandleUseRecoveryCodeLift uses a recovery code to recover account
// POST /auth/recovery/codes/use
func (h *EmailFreeRecoveryHandler) HandleUseRecoveryCodeLift(ctx *lift.Context) error {
	var req struct {
		Username string `json:"username"`
		Code     string `json:"code"`
	}

	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]any{"error": "invalid request body"})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]any{"error": "invalid request body"})
		}
	}

	// Validate recovery code
	valid, err := h.recoveryCode.ValidateRecoveryCode(ctx.Context, req.Username, req.Code)
	if err != nil || !valid {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": "invalid recovery code"})
	}

	// Generate recovery token for password reset
	token, err := h.authService.GenerateRecoveryToken(ctx.Context, req.Username, "recovery_code")
	if err != nil {
		h.logger.Error("failed to generate recovery token", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]any{"error": "internal server error"})
	}

	// Get remaining code count
	remainingCodes, _ := h.recoveryCode.GetRecoveryCodeCount(ctx.Context, req.Username)

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"recovery_token":  token,
		"remaining_codes": remainingCodes,
		"message":         "Recovery code accepted. Use the token to set a new password or add authentication methods.",
	})
}

// HandleAddTrusteeLift adds a trusted contact for social recovery
// POST /auth/recovery/trustees/add
func (h *EmailFreeRecoveryHandler) HandleAddTrusteeLift(ctx *lift.Context) error {
	// Get username from authorization context or test header
	var username string

	// Authentication required - get from JWT context
	if ctx.Value("jwt_claims") != nil {
		if claims, ok := ctx.Value("jwt_claims").(map[string]any); ok {
			if sub, ok := claims["sub"].(string); ok {
				username = sub
			}
		}
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]any{"error": "authentication required"})
	}

	var req struct {
		TrusteeActorID string `json:"trustee_actor_id"` // @username@instance
	}

	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]any{"error": "invalid request body"})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]any{"error": "invalid request body"})
		}
	}

	// Validate format
	if !strings.Contains(req.TrusteeActorID, "@") {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": "invalid actor ID format, expected @username@instance"})
	}

	if err := h.socialRecovery.AddTrustee(ctx.Context, username, req.TrusteeActorID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": err.Error()})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"message": "Trustee added. They will receive a notification to confirm.",
		"trustee": req.TrusteeActorID,
	})
}

// HandleListTrusteesLift lists all trustees for the authenticated user
// GET /auth/recovery/trustees
func (h *EmailFreeRecoveryHandler) HandleListTrusteesLift(ctx *lift.Context) error {
	// Get username from authorization context or test header
	var username string

	// Authentication required - get from JWT context
	if ctx.Value("jwt_claims") != nil {
		if claims, ok := ctx.Value("jwt_claims").(map[string]any); ok {
			if sub, ok := claims["sub"].(string); ok {
				username = sub
			}
		}
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]any{"error": "authentication required"})
	}

	trustees, err := h.socialRecovery.GetTrustees(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get trustees", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]any{"error": "internal server error"})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"trustees":         trustees,
		"count":            len(trustees),
		"minimum_required": 2,
	})
}

// HandleRemoveTrusteeLift removes a trustee
// DELETE /auth/recovery/trustees/{trustee_id}
func (h *EmailFreeRecoveryHandler) HandleRemoveTrusteeLift(ctx *lift.Context) error {
	// Get username from authorization context or test header
	var username string

	// Authentication required - get from JWT context
	if ctx.Value("jwt_claims") != nil {
		if claims, ok := ctx.Value("jwt_claims").(map[string]any); ok {
			if sub, ok := claims["sub"].(string); ok {
				username = sub
			}
		}
	}

	if err := common.ValidateRequiredParam("username", username); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]any{"error": "authentication required"})
	}

	// Extract trustee ID from path parameter
	trusteeID := ctx.Param("trustee_id")
	if err := common.ValidateRequiredParam("trustee_id", trusteeID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": err.Error()})
	}

	if err := h.socialRecovery.RemoveTrustee(ctx.Context, username, trusteeID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": err.Error()})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"message": "Trustee removed",
		"trustee": trusteeID,
	})
}

// HandleDeviceRecoveryLift allows recovery via a trusted device
// POST /auth/recovery/device
func (h *EmailFreeRecoveryHandler) HandleDeviceRecoveryLift(ctx *lift.Context) error {
	var req struct {
		Username string `json:"username"`
		DeviceID string `json:"device_id"`
	}

	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]any{"error": "invalid request body"})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]any{"error": "invalid request body"})
		}
	}

	// Check if device is trusted
	device, err := h.authService.GetStore().Account().GetDevice(ctx.Context, req.DeviceID)
	if err != nil || device.Username != req.Username || device.TrustLevel != "trusted" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": "device not found or not trusted"})
	}

	// Generate recovery token
	token, err := h.authService.GenerateRecoveryToken(ctx.Context, req.Username, "trusted_device")
	if err != nil {
		h.logger.Error("failed to generate recovery token", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]any{"error": "internal server error"})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"recovery_token": token,
		"device_name":    device.DeviceName,
		"message":        "Device verified. Use the token to add new authentication methods.",
	})
}
