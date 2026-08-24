package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/go-webauthn/webauthn/protocol"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

type webAuthnSignupFinishRequest struct {
	Username  string         `json:"username"`
	Challenge string         `json:"challenge"`
	Response  map[string]any `json:"response"`
}

type webAuthnSignupFinishResponse struct {
	PasskeyRegistrationProof string `json:"passkey_registration_proof"`
}

func webAuthnPublicKeyMap(options any) (map[string]any, error) {
	if options == nil {
		return nil, nil
	}

	publicKey := options
	switch typed := options.(type) {
	case *protocol.CredentialCreation:
		publicKey = typed.Response
	case protocol.CredentialCreation:
		publicKey = typed.Response
	case *protocol.CredentialAssertion:
		publicKey = typed.Response
	case protocol.CredentialAssertion:
		publicKey = typed.Response
	}

	raw, err := json.Marshal(publicKey)
	if err != nil {
		return nil, err
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// HandleBeginWebAuthnSignupLift starts the public WebAuthn signup process.
// POST /api/v1/auth/webauthn/signup/begin
func (h *Handler) HandleBeginWebAuthnSignupLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	var req apimodels.WebAuthnBeginLoginRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
	}

	if err := common.ValidateRequiredParam("username", req.Username); err != nil {
		return h.respondBadRequest(ctx, "username required")
	}

	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	options, challenge, err := authService.BeginWebAuthnSignup(ctx.Context(), req.Username)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "begin signup")
	}

	publicKey, err := webAuthnPublicKeyMap(options)
	if err != nil {
		h.logger.Error("failed to serialize webauthn signup options", zap.Error(err))
		return h.respondInternalError(ctx, "failed to begin signup")
	}

	return okJSON(apimodels.WebAuthnBeginResponse{
		PublicKey: publicKey,
		Challenge: challenge,
	})
}

// HandleFinishWebAuthnSignupLift completes the public WebAuthn signup process
// and returns a single-use registration proof.
// POST /api/v1/auth/webauthn/signup/finish
func (h *Handler) HandleFinishWebAuthnSignupLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	var req webAuthnSignupFinishRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
	}

	if err := common.ValidateRequiredParam("username", req.Username); err != nil {
		return h.respondBadRequest(ctx, "username required")
	}
	if err := common.ValidateRequiredParam("challenge", req.Challenge); err != nil {
		return h.respondBadRequest(ctx, "challenge required")
	}

	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	rawResponse, err := json.Marshal(req.Response)
	if err != nil {
		return h.respondBadRequest(ctx, "invalid response payload")
	}

	proofID, err := authService.FinishWebAuthnSignup(ctx.Context(), req.Username, req.Challenge, rawResponse)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "complete signup")
	}

	return okJSON(webAuthnSignupFinishResponse{
		PasskeyRegistrationProof: proofID,
	})
}

// HandleBeginWebAuthnRegistrationLift starts the WebAuthn registration process
// POST /api/v1/auth/webauthn/register/begin
func (h *Handler) HandleBeginWebAuthnRegistrationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get authenticated user from context
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondWithError(ctx, http.StatusUnauthorized, "authentication required")
	}

	// Get auth service
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Begin registration
	options, challenge, err := authService.BeginWebAuthnRegistration(ctx.Context(), username)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "begin registration")
	}

	publicKey, err := webAuthnPublicKeyMap(options)
	if err != nil {
		h.logger.Error("failed to serialize webauthn options", zap.Error(err))
		return h.respondInternalError(ctx, "failed to begin registration")
	}

	// Return options and challenge
	response := apimodels.WebAuthnBeginResponse{
		PublicKey: publicKey,
		Challenge: challenge,
	}

	return okJSON(response)
}

// HandleFinishWebAuthnRegistrationLift completes the WebAuthn registration process
// POST /api/v1/auth/webauthn/register/finish
func (h *Handler) HandleFinishWebAuthnRegistrationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get authenticated user from context
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondWithError(ctx, http.StatusUnauthorized, "authentication required")
	}

	// Parse request body
	var req apimodels.WebAuthnFinishRegistrationRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
	}

	if err := common.ValidateRequiredParam("challenge", req.Challenge); err != nil {
		return h.respondBadRequest(ctx, "challenge required")
	}

	// Get auth service
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Finish registration
	rawResponse, err := json.Marshal(req.Response)
	if err != nil {
		return h.respondBadRequest(ctx, "invalid response payload")
	}

	userAgent, ipAddress := h.getDeviceInfo(ctx)
	requestCtx := auth.WithAuditRequestMetadata(ctx.Context(), ipAddress, userAgent)

	err = authService.FinishWebAuthnRegistration(requestCtx, username, req.Challenge, rawResponse, req.CredentialName)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "complete registration")
	}

	return okJSON(apimodels.MessageResponse{
		Message: "passkey registered successfully",
	})
}

// HandleBeginWebAuthnLoginLift starts the WebAuthn login process
// POST /api/v1/auth/webauthn/login/begin
func (h *Handler) HandleBeginWebAuthnLoginLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse request body
	var req apimodels.WebAuthnBeginLoginRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
	}

	if err := common.ValidateRequiredParam("username", req.Username); err != nil {
		return h.respondBadRequest(ctx, "username required")
	}

	// Get auth service
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Begin login
	options, challenge, err := authService.BeginWebAuthnLogin(ctx.Context(), req.Username)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "begin login")
	}

	publicKey, err := webAuthnPublicKeyMap(options)
	if err != nil {
		h.logger.Error("failed to serialize webauthn options", zap.Error(err))
		return h.respondInternalError(ctx, "failed to begin login")
	}

	// Return options and challenge
	response := apimodels.WebAuthnBeginResponse{
		PublicKey: publicKey,
		Challenge: challenge,
	}

	return okJSON(response)
}

// HandleFinishWebAuthnLoginLift completes the WebAuthn login process
// POST /api/v1/auth/webauthn/login/finish
func (h *Handler) HandleFinishWebAuthnLoginLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse request body
	var req apimodels.WebAuthnFinishLoginRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
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
	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Finish login and create session
	rawResponse, err := json.Marshal(req.Response)
	if err != nil {
		return h.respondBadRequest(ctx, "invalid response payload")
	}

	authResponse, err := authService.FinishWebAuthnLogin(ctx.Context(), req.Username, req.Challenge, rawResponse, req.DeviceName, userAgent, ipAddress)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "complete login")
	}

	return okJSON(authResponse)
}

// HandleListWebAuthnCredentialsLift returns all WebAuthn credentials for the authenticated user
// GET /api/v1/auth/webauthn/credentials
func (h *Handler) HandleListWebAuthnCredentialsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get authenticated user from context
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondWithError(ctx, http.StatusUnauthorized, "authentication required")
	}

	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get credentials
	credentials, err := authService.GetWebAuthnCredentials(ctx.Context(), username)
	if err != nil {
		h.logger.Error("failed to get WebAuthn credentials", zap.Error(err))
		return h.respondWithError(ctx, http.StatusInternalServerError, "failed to get credentials")
	}

	// Format response
	response := make([]apimodels.WebAuthnCredentialSummary, 0, len(credentials))
	for _, cred := range credentials {
		response = append(response, apimodels.WebAuthnCredentialSummary{
			ID:         cred.ID,
			Name:       cred.Name,
			CreatedAt:  cred.CreatedAt,
			LastUsedAt: cred.LastUsedAt,
		})
	}

	return okJSON(apimodels.WebAuthnCredentialsResponse{
		Credentials: response,
	})
}

// HandleDeleteWebAuthnCredentialLift removes a WebAuthn credential
// DELETE /api/v1/auth/webauthn/credentials/{credentialId}
func (h *Handler) HandleDeleteWebAuthnCredentialLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get authenticated user from context
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondWithError(ctx, http.StatusUnauthorized, "authentication required")
	}

	// Get credential ID from path
	credentialID := ctx.Param("credentialId")
	if err := common.ValidateRequiredParam("credentialId", credentialID); err != nil {
		return h.respondBadRequest(ctx, "credential ID required")
	}

	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	userAgent, ipAddress := h.getDeviceInfo(ctx)
	requestCtx := auth.WithAuditRequestMetadata(ctx.Context(), ipAddress, userAgent)

	// Delete credential
	err = authService.DeleteWebAuthnCredential(requestCtx, username, credentialID)
	if err != nil {
		if errors.Is(err, auth.ErrCredentialNotFound) {
			return h.respondWithError(ctx, http.StatusNotFound, "credential not found")
		}
		if errors.Is(err, auth.ErrLastAuthMethodDelete) {
			return h.respondBadRequest(ctx, "cannot delete last authentication method")
		}
		h.logger.Error("failed to delete WebAuthn credential", zap.Error(err))
		return h.respondWithError(ctx, http.StatusInternalServerError, "failed to delete credential")
	}

	return okJSON(apimodels.MessageResponse{
		Message: "credential deleted successfully",
	})
}

// HandleUpdateWebAuthnCredentialNameLift updates the display name of a WebAuthn credential
// PUT /api/v1/auth/webauthn/credentials/{credentialId}
func (h *Handler) HandleUpdateWebAuthnCredentialNameLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get authenticated user from context
	username := h.getAuthenticatedUserLift(ctx)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return h.respondWithError(ctx, http.StatusUnauthorized, "authentication required")
	}

	// Get credential ID from path
	credentialID := ctx.Param("credentialId")
	if err := common.ValidateRequiredParam("credentialId", credentialID); err != nil {
		return h.respondBadRequest(ctx, "credential ID required")
	}

	// Parse request body
	var req apimodels.WebAuthnUpdateCredentialRequest
	if err := h.parseRequestBody(ctx, &req); err != nil {
		return h.respondBadRequest(ctx, "invalid request body")
	}

	if err := common.ValidateRequiredParam("name", req.Name); err != nil {
		return h.respondBadRequest(ctx, "name required")
	}

	authService, resp, err := h.requireAuthService(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Update credential name
	err = authService.UpdateWebAuthnCredentialName(ctx.Context(), username, credentialID, req.Name)
	if err != nil {
		if err == auth.ErrCredentialNotFound {
			return h.respondWithError(ctx, http.StatusNotFound, "credential not found")
		}
		h.logger.Error("failed to update WebAuthn credential name", zap.Error(err))
		return h.respondWithError(ctx, http.StatusInternalServerError, "failed to update credential")
	}

	return okJSON(apimodels.MessageResponse{
		Message: "credential updated successfully",
	})
}
