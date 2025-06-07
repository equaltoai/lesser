package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// WebAuthnHandler handles WebAuthn/passkey operations
type WebAuthnHandler struct {
	authService *auth.AuthService
}

// NewWebAuthnHandler creates a new WebAuthn handler
func NewWebAuthnHandler(authService *auth.AuthService) *WebAuthnHandler {
	return &WebAuthnHandler{
		authService: authService,
	}
}

// BeginRegistration starts the WebAuthn registration process
// POST /api/v1/auth/webauthn/register/begin
func (h *WebAuthnHandler) BeginRegistration(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get authenticated user from context
	username := request.RequestContext.Authorizer.JWT.Claims["sub"]
	if username == "" {
		return common.Unauthorized(errors.New("authentication required")), nil
	}

	// Begin registration
	options, challenge, err := h.authService.BeginWebAuthnRegistration(ctx, username)
	if err != nil {
		if err == auth.ErrWebAuthnNotConfigured {
			return common.InternalServerError(errors.New("WebAuthn not configured")), nil
		}
		common.Logger().Error("failed to begin WebAuthn registration", zap.Error(err))
		return common.InternalServerError(errors.New("failed to begin registration")), nil
	}

	// Return options and challenge
	response := map[string]interface{}{
		"publicKey": options,
		"challenge": challenge,
	}

	return common.OK(response), nil
}

// FinishRegistration completes the WebAuthn registration process
// POST /api/v1/auth/webauthn/register/finish
func (h *WebAuthnHandler) FinishRegistration(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get authenticated user from context
	username := request.RequestContext.Authorizer.JWT.Claims["sub"]
	if username == "" {
		return common.Unauthorized(errors.New("authentication required")), nil
	}

	// Parse request body
	var req struct {
		Challenge      string          `json:"challenge"`
		Response       json.RawMessage `json:"response"`
		CredentialName string          `json:"credential_name"`
	}
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Finish registration
	err := h.authService.FinishWebAuthnRegistration(ctx, username, req.Challenge, req.Response, req.CredentialName)
	if err != nil {
		if err == auth.ErrChallengeNotFound {
			return common.BadRequest(errors.New("invalid or expired challenge")), nil
		}
		common.Logger().Error("failed to finish WebAuthn registration", zap.Error(err))
		return common.InternalServerError(errors.New("failed to complete registration")), nil
	}

	return common.OK(map[string]string{
		"message": "passkey registered successfully",
	}), nil
}

// BeginLogin starts the WebAuthn login process
// POST /api/v1/auth/webauthn/login/begin
func (h *WebAuthnHandler) BeginLogin(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Parse request body
	var req struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	if req.Username == "" {
		return common.BadRequest(errors.New("username required")), nil
	}

	// Begin login
	options, challenge, err := h.authService.BeginWebAuthnLogin(ctx, req.Username)
	if err != nil {
		if err == auth.ErrUserHasNoCredentials {
			return common.BadRequest(errors.New("no passkeys registered for this user")), nil
		}
		if err == auth.ErrWebAuthnNotConfigured {
			return common.InternalServerError(errors.New("WebAuthn not configured")), nil
		}
		common.Logger().Error("failed to begin WebAuthn login", zap.Error(err))
		return common.InternalServerError(errors.New("failed to begin login")), nil
	}

	// Return options and challenge
	response := map[string]interface{}{
		"publicKey": options,
		"challenge": challenge,
	}

	return common.OK(response), nil
}

// FinishLogin completes the WebAuthn login process
// POST /api/v1/auth/webauthn/login/finish
func (h *WebAuthnHandler) FinishLogin(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Parse request body
	var req struct {
		Username   string          `json:"username"`
		Challenge  string          `json:"challenge"`
		Response   json.RawMessage `json:"response"`
		DeviceName string          `json:"device_name"`
	}
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	if req.Username == "" || req.Challenge == "" {
		return common.BadRequest(errors.New("username and challenge required")), nil
	}

	// Get device info
	userAgent := request.Headers["user-agent"]
	ipAddress := request.RequestContext.HTTP.SourceIP
	if req.DeviceName == "" {
		req.DeviceName = "WebAuthn Device"
	}

	// Finish login and create session
	authResponse, err := h.authService.FinishWebAuthnLogin(ctx, req.Username, req.Challenge, req.Response, req.DeviceName, userAgent, ipAddress)
	if err != nil {
		if err == auth.ErrChallengeNotFound {
			return common.BadRequest(errors.New("invalid or expired challenge")), nil
		}
		if err == auth.ErrInvalidCredential {
			return common.Unauthorized(errors.New("invalid credential")), nil
		}
		common.Logger().Error("failed to finish WebAuthn login", zap.Error(err))
		return common.InternalServerError(errors.New("failed to complete login")), nil
	}

	return common.OK(authResponse), nil
}

// ListCredentials returns all WebAuthn credentials for the authenticated user
// GET /api/v1/auth/webauthn/credentials
func (h *WebAuthnHandler) ListCredentials(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get authenticated user from context
	username := request.RequestContext.Authorizer.JWT.Claims["sub"]
	if username == "" {
		return common.Unauthorized(errors.New("authentication required")), nil
	}

	// Get credentials
	credentials, err := h.authService.GetWebAuthnCredentials(ctx, username)
	if err != nil {
		common.Logger().Error("failed to get WebAuthn credentials", zap.Error(err))
		return common.InternalServerError(errors.New("failed to get credentials")), nil
	}

	// Format response
	var response []map[string]interface{}
	for _, cred := range credentials {
		response = append(response, map[string]interface{}{
			"id":           cred.ID,
			"name":         cred.Name,
			"created_at":   cred.CreatedAt,
			"last_used_at": cred.LastUsedAt,
		})
	}

	return common.OK(map[string]interface{}{
		"credentials": response,
	}), nil
}

// DeleteCredential removes a WebAuthn credential
// DELETE /api/v1/auth/webauthn/credentials/{credentialId}
func (h *WebAuthnHandler) DeleteCredential(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get authenticated user from context
	username := request.RequestContext.Authorizer.JWT.Claims["sub"]
	if username == "" {
		return common.Unauthorized(errors.New("authentication required")), nil
	}

	// Get credential ID from path
	credentialID := request.PathParameters["credentialId"]
	if credentialID == "" {
		return common.BadRequest(errors.New("credential ID required")), nil
	}

	// Delete credential
	err := h.authService.DeleteWebAuthnCredential(ctx, username, credentialID)
	if err != nil {
		if err == auth.ErrCredentialNotFound {
			return common.NotFound(errors.New("credential not found")), nil
		}
		if err.Error() == "cannot delete last authentication method" {
			return common.BadRequest(errors.New("cannot delete last authentication method")), nil
		}
		common.Logger().Error("failed to delete WebAuthn credential", zap.Error(err))
		return common.InternalServerError(errors.New("failed to delete credential")), nil
	}

	return common.OK(map[string]string{
		"message": "credential deleted successfully",
	}), nil
}

// UpdateCredentialName updates the display name of a WebAuthn credential
// PUT /api/v1/auth/webauthn/credentials/{credentialId}
func (h *WebAuthnHandler) UpdateCredentialName(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get authenticated user from context
	username := request.RequestContext.Authorizer.JWT.Claims["sub"]
	if username == "" {
		return common.Unauthorized(errors.New("authentication required")), nil
	}

	// Get credential ID from path
	credentialID := request.PathParameters["credentialId"]
	if credentialID == "" {
		return common.BadRequest(errors.New("credential ID required")), nil
	}

	// Parse request body
	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	if req.Name == "" {
		return common.BadRequest(errors.New("name required")), nil
	}

	// Update credential name
	err := h.authService.UpdateWebAuthnCredentialName(ctx, username, credentialID, req.Name)
	if err != nil {
		if err == auth.ErrCredentialNotFound {
			return common.NotFound(errors.New("credential not found")), nil
		}
		common.Logger().Error("failed to update WebAuthn credential name", zap.Error(err))
		return common.InternalServerError(errors.New("failed to update credential")), nil
	}

	return common.OK(map[string]string{
		"message": "credential updated successfully",
	}), nil
}
