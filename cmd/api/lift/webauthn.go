package lift

import (
	"encoding/json"
	"net/http"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleBeginWebAuthnRegistrationLift starts the WebAuthn registration process
// POST /api/v1/auth/webauthn/register/begin
func (h *Handler) HandleBeginWebAuthnRegistrationLift(ctx *lift.Context) error {
	// Get authenticated user from context
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

	// Begin registration
	options, challenge, err := authService.BeginWebAuthnRegistration(ctx.Context, username)
	if err != nil {
		if err == auth.ErrWebAuthnNotConfigured {
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{
				"error": "WebAuthn not configured",
			})
		}
		h.logger.Error("failed to begin WebAuthn registration", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to begin registration",
		})
	}

	// Return options and challenge
	response := map[string]any{
		"publicKey": options,
		"challenge": challenge,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleFinishWebAuthnRegistrationLift completes the WebAuthn registration process
// POST /api/v1/auth/webauthn/register/finish
func (h *Handler) HandleFinishWebAuthnRegistrationLift(ctx *lift.Context) error {
	// Get authenticated user from context
	username := h.getAuthenticatedUserLift(ctx)
	if username == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Parse request body
	var req struct {
		Challenge      string          `json:"challenge"`
		Response       json.RawMessage `json:"response"`
		CredentialName string          `json:"credential_name"`
	}
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "invalid request body",
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

	// Finish registration
	err = authService.FinishWebAuthnRegistration(ctx.Context, username, req.Challenge, req.Response, req.CredentialName)
	if err != nil {
		if err == auth.ErrChallengeNotFound {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{
				"error": "invalid or expired challenge",
			})
		}
		h.logger.Error("failed to finish WebAuthn registration", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to complete registration",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]string{
		"message": "passkey registered successfully",
	})
}

// HandleBeginWebAuthnLoginLift starts the WebAuthn login process
// POST /api/v1/auth/webauthn/login/begin
func (h *Handler) HandleBeginWebAuthnLoginLift(ctx *lift.Context) error {
	// Parse request body
	var req struct {
		Username string `json:"username"`
	}
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "invalid request body",
		})
	}

	if req.Username == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "username required",
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

	// Begin login
	options, challenge, err := authService.BeginWebAuthnLogin(ctx.Context, req.Username)
	if err != nil {
		if err == auth.ErrUserHasNoCredentials {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{
				"error": "no passkeys registered for this user",
			})
		}
		if err == auth.ErrWebAuthnNotConfigured {
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{
				"error": "WebAuthn not configured",
			})
		}
		h.logger.Error("failed to begin WebAuthn login", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to begin login",
		})
	}

	// Return options and challenge
	response := map[string]any{
		"publicKey": options,
		"challenge": challenge,
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleFinishWebAuthnLoginLift completes the WebAuthn login process
// POST /api/v1/auth/webauthn/login/finish
func (h *Handler) HandleFinishWebAuthnLoginLift(ctx *lift.Context) error {
	// Parse request body
	var req struct {
		Username   string          `json:"username"`
		Challenge  string          `json:"challenge"`
		Response   json.RawMessage `json:"response"`
		DeviceName string          `json:"device_name"`
	}
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "invalid request body",
		})
	}

	if req.Username == "" || req.Challenge == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "username and challenge required",
		})
	}

	// Get device info
	userAgent := ctx.Header("User-Agent")
	ipAddress := ctx.Header("X-Forwarded-For")
	if ipAddress == "" {
		ipAddress = ctx.Header("X-Real-IP")
	}
	if ipAddress == "" {
		ipAddress = "unknown"
	}
	if req.DeviceName == "" {
		req.DeviceName = "WebAuthn Device"
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

	// Finish login and create session
	authResponse, err := authService.FinishWebAuthnLogin(ctx.Context, req.Username, req.Challenge, req.Response, req.DeviceName, userAgent, ipAddress)
	if err != nil {
		if err == auth.ErrChallengeNotFound {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{
				"error": "invalid or expired challenge",
			})
		}
		if err == auth.ErrInvalidCredential {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]string{
				"error": "invalid credential",
			})
		}
		h.logger.Error("failed to finish WebAuthn login", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to complete login",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(authResponse)
}

// HandleListWebAuthnCredentialsLift returns all WebAuthn credentials for the authenticated user
// GET /api/v1/auth/webauthn/credentials
func (h *Handler) HandleListWebAuthnCredentialsLift(ctx *lift.Context) error {
	// Get authenticated user from context
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

	// Get credentials
	credentials, err := authService.GetWebAuthnCredentials(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get WebAuthn credentials", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to get credentials",
		})
	}

	// Format response
	var response []map[string]any
	for _, cred := range credentials {
		response = append(response, map[string]any{
			"id":           cred.ID,
			"name":         cred.Name,
			"created_at":   cred.CreatedAt,
			"last_used_at": cred.LastUsedAt,
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"credentials": response,
	})
}

// HandleDeleteWebAuthnCredentialLift removes a WebAuthn credential
// DELETE /api/v1/auth/webauthn/credentials/{credentialId}
func (h *Handler) HandleDeleteWebAuthnCredentialLift(ctx *lift.Context) error {
	// Get authenticated user from context
	username := h.getAuthenticatedUserLift(ctx)
	if username == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Get credential ID from path
	credentialID := ctx.Param("credentialId")
	if credentialID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "credential ID required",
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

	// Delete credential
	err = authService.DeleteWebAuthnCredential(ctx.Context, username, credentialID)
	if err != nil {
		if err == auth.ErrCredentialNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{
				"error": "credential not found",
			})
		}
		if err.Error() == "cannot delete last authentication method" {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]string{
				"error": "cannot delete last authentication method",
			})
		}
		h.logger.Error("failed to delete WebAuthn credential", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to delete credential",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]string{
		"message": "credential deleted successfully",
	})
}

// HandleUpdateWebAuthnCredentialNameLift updates the display name of a WebAuthn credential
// PUT /api/v1/auth/webauthn/credentials/{credentialId}
func (h *Handler) HandleUpdateWebAuthnCredentialNameLift(ctx *lift.Context) error {
	// Get authenticated user from context
	username := h.getAuthenticatedUserLift(ctx)
	if username == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Get credential ID from path
	credentialID := ctx.Param("credentialId")
	if credentialID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "credential ID required",
		})
	}

	// Parse request body
	var req struct {
		Name string `json:"name"`
	}
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "invalid request body",
		})
	}

	if req.Name == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "name required",
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

	// Update credential name
	err = authService.UpdateWebAuthnCredentialName(ctx.Context, username, credentialID, req.Name)
	if err != nil {
		if err == auth.ErrCredentialNotFound {
			ctx.Status(http.StatusNotFound)
			return ctx.JSON(map[string]string{
				"error": "credential not found",
			})
		}
		h.logger.Error("failed to update WebAuthn credential name", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "failed to update credential",
		})
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]string{
		"message": "credential updated successfully",
	})
}

