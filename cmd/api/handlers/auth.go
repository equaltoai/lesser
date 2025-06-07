package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authService *auth.AuthService
	middleware  *auth.Middleware
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler() (*AuthHandler, error) {
	store, err := dynamodb.New()
	if err != nil {
		return nil, err
	}

	authService, err := auth.NewAuthService(store)
	if err != nil {
		return nil, err
	}

	middleware, err := auth.GetMiddleware()
	if err != nil {
		return nil, err
	}

	return &AuthHandler{
		authService: authService,
		middleware:  middleware,
	}, nil
}

// HandleLogin handles password-based login
// POST /api/v1/auth/login
func (h *AuthHandler) HandleLogin(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Parse request body
	var loginReq LoginRequest
	if err := json.Unmarshal([]byte(request.Body), &loginReq); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Validate required fields
	if loginReq.Username == "" || loginReq.Password == "" {
		return common.BadRequest(errors.New("username and password are required")), nil
	}

	// Extract device info
	deviceName := loginReq.DeviceName
	if deviceName == "" {
		deviceName = "Web Browser"
	}

	userAgent := request.Headers["User-Agent"]
	if userAgent == "" {
		userAgent = request.Headers["user-agent"]
	}

	// Get client IP
	ipAddress := request.RequestContext.HTTP.SourceIP

	// Authenticate
	response, err := h.authService.AuthenticateWithPassword(
		ctx,
		loginReq.Username,
		loginReq.Password,
		deviceName,
		userAgent,
		ipAddress,
	)

	if err != nil {
		switch err {
		case auth.ErrAccountLocked, auth.ErrIPRateLimited:
			return common.TooManyRequests(err), nil
		case auth.ErrUserSuspended:
			return common.Forbidden(errors.New("account suspended")), nil
		case auth.ErrUserNotApproved:
			return common.Forbidden(errors.New("account not approved")), nil
		default:
			// Generic error to avoid leaking information
			return common.Unauthorized(errors.New("invalid credentials")), nil
		}
	}

	return common.OK(response), nil
}

// HandleRefreshToken handles token refresh
// POST /api/v1/auth/refresh
func (h *AuthHandler) HandleRefreshToken(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Parse request body
	var refreshReq RefreshTokenRequest
	if err := json.Unmarshal([]byte(request.Body), &refreshReq); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Validate refresh token
	if refreshReq.RefreshToken == "" {
		return common.BadRequest(errors.New("refresh_token is required")), nil
	}

	// Get client IP
	ipAddress := request.RequestContext.HTTP.SourceIP

	// Refresh token
	response, err := h.authService.RefreshAccessToken(ctx, refreshReq.RefreshToken, ipAddress)
	if err != nil {
		return common.Unauthorized(errors.New("invalid refresh token")), nil
	}

	return common.OK(response), nil
}

// HandleLogout handles session logout
// POST /api/v1/auth/logout
func (h *AuthHandler) HandleLogout(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	_, err := h.middleware.RequireAuth(ctx, request)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get enhanced claims to access session ID
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, _ := auth.ExtractBearerToken(authHeader)
	enhancedClaims, err := h.authService.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Logout current session
	if err := h.authService.Logout(ctx, enhancedClaims.SessionID); err != nil {
		common.Logger().Error("failed to logout session", zap.Error(err))
		// Don't fail the request - consider it successful anyway
	}

	return common.OK(map[string]string{
		"message": "logged out successfully",
	}), nil
}

// HandleLogoutAllDevices handles logout from all devices
// POST /api/v1/auth/logout/all
func (h *AuthHandler) HandleLogoutAllDevices(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	claims, err := h.middleware.RequireAuth(ctx, request)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Logout all sessions
	if err := h.authService.LogoutAllDevices(ctx, claims.Username); err != nil {
		common.Logger().Error("failed to logout all devices", zap.Error(err))
		return common.InternalServerError(errors.New("failed to logout all devices")), nil
	}

	return common.OK(map[string]string{
		"message": "logged out from all devices successfully",
	}), nil
}

// HandleGetDevices returns user's devices
// GET /api/v1/auth/devices
func (h *AuthHandler) HandleGetDevices(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	claims, err := h.middleware.RequireAuth(ctx, request)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get devices
	devices, err := h.authService.GetUserDevices(ctx, claims.Username)
	if err != nil {
		common.Logger().Error("failed to get devices", zap.Error(err))
		return common.InternalServerError(errors.New("failed to get devices")), nil
	}

	// Convert to API response format
	var deviceResponses []DeviceResponse
	for _, device := range devices {
		deviceResponses = append(deviceResponses, DeviceResponse{
			ID:         device.DeviceID,
			Name:       device.DeviceName,
			Type:       device.DeviceType,
			LastSeenAt: device.LastSeenAt.Format("2006-01-02T15:04:05Z"),
			LastIP:     device.LastIPAddress,
			TrustLevel: device.TrustLevel,
			CreatedAt:  device.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return common.OK(deviceResponses), nil
}

// HandleTrustDevice marks a device as trusted
// POST /api/v1/auth/devices/{deviceId}/trust
func (h *AuthHandler) HandleTrustDevice(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	claims, err := h.middleware.RequireAuth(ctx, request)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Get device ID from path
	deviceID := request.PathParameters["deviceId"]
	if deviceID == "" {
		return common.BadRequest(errors.New("device ID is required")), nil
	}

	// Trust device
	if err := h.authService.TrustDevice(ctx, claims.Username, deviceID); err != nil {
		if err.Error() == "device does not belong to user" {
			return common.Forbidden(errors.New("device does not belong to user")), nil
		}
		common.Logger().Error("failed to trust device", zap.Error(err))
		return common.InternalServerError(errors.New("failed to trust device")), nil
	}

	return common.OK(map[string]string{
		"message": "device trusted successfully",
	}), nil
}

// HandleDeleteDevice removes a device/session
// DELETE /api/v1/auth/devices/{deviceId}
func (h *AuthHandler) HandleDeleteDevice(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	claims, err := h.middleware.RequireAuth(ctx, request)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Extract device ID from path
	path := request.RequestContext.HTTP.Path
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return common.BadRequest(errors.New("device ID required")), nil
	}
	deviceID := parts[len(parts)-1]

	// Get device to verify ownership
	device, err := h.authService.GetStore().GetDevice(ctx, deviceID)
	if err != nil {
		return common.NotFound(errors.New("device not found")), nil
	}

	// Verify device belongs to user
	if device.Username != claims.Username {
		return common.Forbidden(errors.New("device does not belong to user")), nil
	}

	// Delete all sessions associated with this device
	sessions, err := h.authService.GetStore().GetUserSessions(ctx, claims.Username)
	if err == nil {
		for _, session := range sessions {
			if session.DeviceID == deviceID {
				_ = h.authService.Logout(ctx, session.SessionID)
			}
		}
	}

	return common.OK(map[string]string{
		"message":   "device removed successfully",
		"device_id": deviceID,
	}), nil
}

// HandleChangePassword handles password change
// POST /api/v1/auth/password/change
func (h *AuthHandler) HandleChangePassword(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	claims, err := h.middleware.RequireAuth(ctx, request)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Parse request body
	var changeReq ChangePasswordRequest
	if err := json.Unmarshal([]byte(request.Body), &changeReq); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Validate fields
	if changeReq.OldPassword == "" || changeReq.NewPassword == "" {
		return common.BadRequest(errors.New("old_password and new_password are required")), nil
	}

	// Change password
	if err := h.authService.ChangePassword(ctx, claims.Username, changeReq.OldPassword, changeReq.NewPassword); err != nil {
		switch err {
		case auth.ErrInvalidCredentials:
			return common.Unauthorized(errors.New("invalid old password")), nil
		case auth.ErrPasswordTooShort, auth.ErrPasswordTooLong:
			return common.BadRequest(err), nil
		default:
			common.Logger().Error("failed to change password", zap.Error(err))
			return common.InternalServerError(errors.New("failed to change password")), nil
		}
	}

	return common.OK(map[string]string{
		"message": "password changed successfully",
	}), nil
}

// HandleGetAccountStatus returns rate limit status (admin endpoint)
// GET /api/v1/auth/accounts/{username}/status
func (h *AuthHandler) HandleGetAccountStatus(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	claims, err := h.middleware.RequireAuth(ctx, request)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check admin permission (simplified for now)
	// TODO: Implement proper role checking
	if !strings.HasSuffix(claims.Username, "-admin") {
		return common.Forbidden(errors.New("admin access required")), nil
	}

	// Get username from path
	username := request.PathParameters["username"]
	if username == "" {
		return common.BadRequest(errors.New("username is required")), nil
	}

	// Get account status
	status, err := h.authService.GetAccountStatus(ctx, username)
	if err != nil {
		common.Logger().Error("failed to get account status", zap.Error(err))
		return common.InternalServerError(errors.New("failed to get account status")), nil
	}

	return common.OK(status), nil
}

// HandleClearAccountLockout clears account lockout (admin endpoint)
// POST /api/v1/auth/accounts/{username}/unlock
func (h *AuthHandler) HandleClearAccountLockout(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	claims, err := h.middleware.RequireAuth(ctx, request)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check admin permission
	if !strings.HasSuffix(claims.Username, "-admin") {
		return common.Forbidden(errors.New("admin access required")), nil
	}

	// Get username from path
	username := request.PathParameters["username"]
	if username == "" {
		return common.BadRequest(errors.New("username is required")), nil
	}

	// Clear lockout
	if err := h.authService.ClearAccountLockout(ctx, username); err != nil {
		common.Logger().Error("failed to clear account lockout", zap.Error(err))
		return common.InternalServerError(errors.New("failed to clear account lockout")), nil
	}

	return common.OK(map[string]string{
		"message": "account unlocked successfully",
	}), nil
}

// Request/Response types

type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	DeviceName string `json:"device_name,omitempty"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type DeviceResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	LastSeenAt string `json:"last_seen_at"`
	LastIP     string `json:"last_ip"`
	TrustLevel string `json:"trust_level"`
	CreatedAt  string `json:"created_at"`
}
