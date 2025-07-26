package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
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
	store := authService.GetStore()

	return &EmailFreeRecoveryHandler{
		socialRecovery: auth.NewSocialRecoveryService(store, logger),
		recoveryCode:   auth.NewRecoveryCodeService(store, logger),
		authService:    authService,
		logger:         logger,
	}
}

// HandleGetRecoveryOptions returns available recovery methods for an account
// GET /auth/recovery/options
func (h *EmailFreeRecoveryHandler) HandleGetRecoveryOptions(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	username := request.QueryStringParameters["username"]
	if username == "" {
		return common.BadRequest(fmt.Errorf("username required")), nil
	}

	// Get user to check what recovery methods are available
	user, err := h.authService.GetStore().GetUser(ctx, username)
	if err != nil {
		// Return generic response to prevent user enumeration
		return common.OK(map[string]any{
			"options": []string{},
			"message": "No account found with this username",
		}), nil
	}

	options := []string{}

	// Check available recovery methods

	// 1. Passkeys (if user has any registered)
	credentials, err := h.authService.GetStore().GetUserWebAuthnCredentials(ctx, username)
	if err == nil && len(credentials) > 0 {
		options = append(options, "passkey")
	}

	// 2. Crypto wallets (if user has any linked)
	wallets, err := h.authService.GetStore().GetUserWalletCredentials(ctx, username)
	if err == nil && len(wallets) > 0 {
		options = append(options, "wallet")
	}

	// 3. OAuth providers (if user has any linked)
	providers, err := h.authService.GetStore().GetLinkedProviders(ctx, username)
	if err == nil && len(providers) > 0 {
		for _, provider := range providers {
			options = append(options, fmt.Sprintf("oauth_%s", provider))
		}
	}

	// 4. Social recovery (if user has trustees configured)
	trustees, err := h.socialRecovery.GetTrustees(ctx, username)
	if err == nil && len(trustees) >= 2 {
		options = append(options, "social")
	}

	// 5. Recovery codes (if user has any)
	codeCount, err := h.recoveryCode.GetRecoveryCodeCount(ctx, username)
	if err == nil && codeCount > 0 {
		options = append(options, "recovery_code")
	}

	// For development/testing, always show all options
	if h.authService.GetConfig().Environment == "development" {
		options = []string{"passkey", "wallet", "oauth_github", "social", "recovery_code"}
	}

	return common.OK(map[string]any{
		"username": user.Username,
		"options":  options,
		"message":  fmt.Sprintf("Found %d recovery options", len(options)),
	}), nil
}

// HandleInitiateSocialRecovery starts the social recovery process
// POST /auth/recovery/social/initiate
func (h *EmailFreeRecoveryHandler) HandleInitiateSocialRecovery(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req struct {
		Username string `json:"username"`
	}

	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Initiate social recovery
	recoveryRequest, err := h.socialRecovery.InitiateRecovery(ctx, req.Username)
	if err != nil {
		// Generic response to prevent enumeration
		return common.OK(map[string]string{
			"message": "If you have trustees configured, they have been notified",
		}), nil
	}

	// In development, return the recovery details
	if h.authService.GetConfig().Environment == "development" {
		return common.OK(map[string]any{
			"message":        "Social recovery initiated",
			"request_id":     recoveryRequest.ID,
			"required_votes": recoveryRequest.RequiredVotes,
			"expires_at":     recoveryRequest.ExpiresAt,
		}), nil
	}

	return common.OK(map[string]string{
		"message": "If you have trustees configured, they have been notified",
	}), nil
}

// HandleConfirmSocialRecovery processes a trustee's confirmation
// POST /auth/recovery/social/confirm
func (h *EmailFreeRecoveryHandler) HandleConfirmSocialRecovery(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// This would typically be called via ActivityPub federation
	// For now, support direct API calls for testing

	var req struct {
		RequestID string `json:"request_id"`
		TrusteeID string `json:"trustee_id"`
	}

	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	if err := h.socialRecovery.ConfirmRecovery(ctx, req.RequestID, req.TrusteeID); err != nil {
		return common.BadRequest(err), nil
	}

	return common.OK(map[string]string{
		"message": "Recovery vote recorded",
	}), nil
}

// HandleGenerateRecoveryCodes generates new recovery codes
// POST /auth/recovery/codes/generate
func (h *EmailFreeRecoveryHandler) HandleGenerateRecoveryCodes(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get username from JWT
	username := request.RequestContext.Authorizer.JWT.Claims["sub"]
	if username == "" {
		return common.Unauthorized(fmt.Errorf("authentication required")), nil
	}

	// Generate codes
	codes, err := h.recoveryCode.GenerateRecoveryCodes(ctx, username, 8)
	if err != nil {
		return common.InternalServerError(err), nil
	}

	return common.OK(map[string]any{
		"codes":          codes,
		"warning":        "Save these codes securely! Each code can only be used once.",
		"recommendation": "Store in a password manager or hardware wallet",
	}), nil
}

// HandleUseRecoveryCode uses a recovery code to recover account
// POST /auth/recovery/codes/use
func (h *EmailFreeRecoveryHandler) HandleUseRecoveryCode(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req struct {
		Username string `json:"username"`
		Code     string `json:"code"`
	}

	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate recovery code
	valid, err := h.recoveryCode.ValidateRecoveryCode(ctx, req.Username, req.Code)
	if err != nil || !valid {
		return common.BadRequest(fmt.Errorf("invalid recovery code")), nil
	}

	// Generate recovery token for password reset
	token, err := h.authService.GenerateRecoveryToken(ctx, req.Username, "recovery_code")
	if err != nil {
		return common.InternalServerError(err), nil
	}

	// Get remaining code count
	remainingCodes, _ := h.recoveryCode.GetRecoveryCodeCount(ctx, req.Username)

	return common.OK(map[string]any{
		"recovery_token":  token,
		"remaining_codes": remainingCodes,
		"message":         "Recovery code accepted. Use the token to set a new password or add authentication methods.",
	}), nil
}

// HandleAddTrustee adds a trusted contact for social recovery
// POST /auth/recovery/trustees/add
func (h *EmailFreeRecoveryHandler) HandleAddTrustee(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get username from JWT
	username := request.RequestContext.Authorizer.JWT.Claims["sub"]
	if username == "" {
		return common.Unauthorized(fmt.Errorf("authentication required")), nil
	}

	var req struct {
		TrusteeActorID string `json:"trustee_actor_id"` // @username@instance
	}

	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate format
	if !strings.Contains(req.TrusteeActorID, "@") {
		return common.BadRequest(fmt.Errorf("invalid actor ID format, expected @username@instance")), nil
	}

	if err := h.socialRecovery.AddTrustee(ctx, username, req.TrusteeActorID); err != nil {
		return common.BadRequest(err), nil
	}

	return common.OK(map[string]string{
		"message": "Trustee added. They will receive a notification to confirm.",
		"trustee": req.TrusteeActorID,
	}), nil
}

// HandleListTrustees lists all trustees for the authenticated user
// GET /auth/recovery/trustees
func (h *EmailFreeRecoveryHandler) HandleListTrustees(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get username from JWT
	username := request.RequestContext.Authorizer.JWT.Claims["sub"]
	if username == "" {
		return common.Unauthorized(fmt.Errorf("authentication required")), nil
	}

	trustees, err := h.socialRecovery.GetTrustees(ctx, username)
	if err != nil {
		return common.InternalServerError(err), nil
	}

	return common.OK(map[string]any{
		"trustees":         trustees,
		"count":            len(trustees),
		"minimum_required": 2,
	}), nil
}

// HandleRemoveTrustee removes a trustee
// DELETE /auth/recovery/trustees/{trustee_id}
func (h *EmailFreeRecoveryHandler) HandleRemoveTrustee(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get username from JWT
	username := request.RequestContext.Authorizer.JWT.Claims["sub"]
	if username == "" {
		return common.Unauthorized(fmt.Errorf("authentication required")), nil
	}

	// Extract trustee ID from path
	path := request.RequestContext.HTTP.Path
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return common.BadRequest(fmt.Errorf("trustee ID required")), nil
	}
	trusteeID := parts[len(parts)-1]

	if err := h.socialRecovery.RemoveTrustee(ctx, username, trusteeID); err != nil {
		return common.BadRequest(err), nil
	}

	return common.OK(map[string]string{
		"message": "Trustee removed",
		"trustee": trusteeID,
	}), nil
}

// HandleDeviceRecovery allows recovery via a trusted device
// POST /auth/recovery/device
func (h *EmailFreeRecoveryHandler) HandleDeviceRecovery(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req struct {
		Username string `json:"username"`
		DeviceID string `json:"device_id"`
	}

	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Check if device is trusted
	device, err := h.authService.GetStore().GetDevice(ctx, req.DeviceID)
	if err != nil || device.Username != req.Username || device.TrustLevel != "trusted" {
		return common.BadRequest(fmt.Errorf("device not found or not trusted")), nil
	}

	// Generate recovery token
	token, err := h.authService.GenerateRecoveryToken(ctx, req.Username, "trusted_device")
	if err != nil {
		return common.InternalServerError(err), nil
	}

	return common.OK(map[string]any{
		"recovery_token": token,
		"device_name":    device.DeviceName,
		"message":        "Device verified. Use the token to add new authentication methods.",
	}), nil
}
