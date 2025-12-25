package lift

import (
	"encoding/json"
	"net/http"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleBeginWebAuthnRegistrationLift starts the WebAuthn registration process
// POST /api/v1/auth/webauthn/register/begin
func (h *Handler) HandleBeginWebAuthnRegistrationLift(ctx *lift.Context) error {
	// Get authenticated user from context
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondUnauthorized(ctx)
	}

	// Get auth service
	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err // Error response already set
	}

	// Begin registration
	options, challenge, err := authService.BeginWebAuthnRegistration(ctx.Context, username)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "begin registration")
	}

	publicKey := options
	switch typed := options.(type) {
	case *protocol.CredentialCreation:
		publicKey = typed.Response
	case protocol.CredentialCreation:
		publicKey = typed.Response
	}

	// Return options and challenge
	response := map[string]any{
		"publicKey": publicKey,
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
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondUnauthorized(ctx)
	}

	// Parse request body
	var req struct {
		Challenge      string          `json:"challenge"`
		Response       json.RawMessage `json:"response"`
		CredentialName string          `json:"credential_name"`
	}
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return err // Error response already set by parseRequestBody
	}

	// Get auth service
	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err // Error response already set
	}

	// Finish registration
	err = authService.FinishWebAuthnRegistration(ctx.Context, username, req.Challenge, req.Response, req.CredentialName)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "complete registration")
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
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return err // Error response already set by parseRequestBody
	}

	if err := common.ValidateRequiredParam("username", req.Username); err != nil {
		return h.respondBadRequest(ctx, "username required")
	}

	// Get auth service
	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err // Error response already set
	}

	// Begin login
	options, challenge, err := authService.BeginWebAuthnLogin(ctx.Context, req.Username)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "begin login")
	}

	publicKey := options
	switch typed := options.(type) {
	case *protocol.CredentialAssertion:
		publicKey = typed.Response
	case protocol.CredentialAssertion:
		publicKey = typed.Response
	}

	// Return options and challenge
	response := map[string]any{
		"publicKey": publicKey,
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
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return err // Error response already set by parseRequestBody
	}

	if err := common.ValidateRequiredParam("username", req.Username); err != nil {
		return h.respondBadRequest(ctx, "username required")
	}
	if err := common.ValidateRequiredParam("challenge", req.Challenge); err != nil {
		return h.respondBadRequest(ctx, "challenge required")
	}

	// Get device info
	userAgent, ipAddress := h.getDeviceInfo(ctx)
	if err := common.ValidateRequiredParam("device_name", req.DeviceName); err != nil {
		req.DeviceName = "WebAuthn Device"
	}

	// Get auth service
	authService, err := h.requireAuthService(ctx)
	if err != nil {
		return err // Error response already set
	}

	// Finish login and create session
	authResponse, err := authService.FinishWebAuthnLogin(ctx.Context, req.Username, req.Challenge, req.Response, req.DeviceName, userAgent, ipAddress)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "complete login")
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(authResponse)
}

// HandleListWebAuthnCredentialsLift returns all WebAuthn credentials for the authenticated user
// GET /api/v1/auth/webauthn/credentials
func (h *Handler) HandleListWebAuthnCredentialsLift(ctx *lift.Context) error {
	// Get authenticated user from context
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
	response := make([]map[string]any, 0, len(credentials))
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
	if err := common.ValidateRequiredParam("username", username); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Get credential ID from path
	credentialID := ctx.Param("credentialId")
	if err := common.ValidateRequiredParam("credentialId", credentialID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "credential ID required",
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
	if err := common.ValidateRequiredParam("username", username); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Get credential ID from path
	credentialID := ctx.Param("credentialId")
	if err := common.ValidateRequiredParam("credentialId", credentialID); err != nil {
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

	if err := common.ValidateRequiredParam("name", req.Name); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "name required",
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
