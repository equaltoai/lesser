package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// RecoveryRequest represents a password recovery request
type RecoveryRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
}

// ResetPasswordRequest represents a password reset request
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// HandleInitiateRecovery initiates password recovery process
// POST /auth/recovery/initiate
func (h *Handler) HandleInitiateRecovery(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req RecoveryRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Find user by email or username
	var user *storage.User
	var err error

	if req.Email != "" {
		user, err = h.store.GetUserByEmail(ctx, req.Email)
	} else if req.Username != "" {
		user, err = h.store.GetUser(ctx, req.Username)
	} else {
		return common.BadRequest(fmt.Errorf("email or username required")), nil
	}

	// Always return success to prevent user enumeration
	successResponse := common.OK(map[string]string{
		"message": "If the account exists, a recovery email has been sent",
	})

	if err != nil || user == nil {
		h.logger.Info("recovery requested for non-existent account",
			zap.String("email", req.Email),
			zap.String("username", req.Username))
		return successResponse, nil
	}

	// Check if user has email
	if user.Email == "" {
		h.logger.Info("recovery requested for account without email",
			zap.String("username", user.Username))
		return successResponse, nil
	}

	// Generate recovery token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		h.logger.Error("failed to generate recovery token", zap.Error(err))
		return successResponse, nil
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Store recovery token with expiration (1 hour)
	recoveryData := map[string]interface{}{
		"username":  user.Username,
		"token":     token,
		"expiresAt": time.Now().Add(time.Hour).Unix(),
		"used":      false,
	}

	// Store in DynamoDB with TTL
	recoveryKey := fmt.Sprintf("RECOVERY#%s", token)
	if err := h.store.StoreRecoveryToken(ctx, recoveryKey, recoveryData); err != nil {
		h.logger.Error("failed to store recovery token", zap.Error(err))
		return successResponse, nil
	}

	// Send recovery email
	recoveryURL := fmt.Sprintf("%s/auth/recovery/verify?token=%s", h.cfg.BaseURL(), token)

	// TODO: Implement email sending
	// For now, log the recovery URL
	h.logger.Info("recovery email would be sent",
		zap.String("email", user.Email),
		zap.String("recovery_url", recoveryURL))

	// In development, include the token in response
	if os.Getenv("ENVIRONMENT") == "development" {
		return common.OK(map[string]interface{}{
			"message": "Recovery email sent (dev mode)",
			"token":   token,
			"url":     recoveryURL,
		}), nil
	}

	return successResponse, nil
}

// HandleVerifyRecoveryToken verifies a recovery token
// GET /auth/recovery/verify
func (h *Handler) HandleVerifyRecoveryToken(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	token := request.QueryStringParameters["token"]
	if token == "" {
		return common.BadRequest(fmt.Errorf("token required")), nil
	}

	// Get recovery data
	recoveryKey := fmt.Sprintf("RECOVERY#%s", token)
	recoveryData, err := h.store.GetRecoveryToken(ctx, recoveryKey)
	if err != nil {
		return common.BadRequest(fmt.Errorf("invalid or expired token")), nil
	}

	// Check if token is expired
	expiresAt := int64(recoveryData["expiresAt"].(float64))
	if time.Now().Unix() > expiresAt {
		h.store.DeleteRecoveryToken(ctx, recoveryKey)
		return common.BadRequest(fmt.Errorf("token expired")), nil
	}

	// Check if token was already used
	if used, ok := recoveryData["used"].(bool); ok && used {
		return common.BadRequest(fmt.Errorf("token already used")), nil
	}

	// Return success with masked username
	username := recoveryData["username"].(string)
	maskedUsername := maskUsername(username)

	return common.OK(map[string]interface{}{
		"valid":    true,
		"username": maskedUsername,
		"token":    token,
	}), nil
}

// HandleCompleteRecovery completes the recovery process by setting new password
// POST /auth/recovery/complete
func (h *Handler) HandleCompleteRecovery(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req ResetPasswordRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate new password
	if len(req.NewPassword) < 8 {
		return common.BadRequest(fmt.Errorf("password must be at least 8 characters")), nil
	}

	// Get recovery data
	recoveryKey := fmt.Sprintf("RECOVERY#%s", req.Token)
	recoveryData, err := h.store.GetRecoveryToken(ctx, recoveryKey)
	if err != nil {
		return common.BadRequest(fmt.Errorf("invalid or expired token")), nil
	}

	// Check if token is expired
	expiresAt := int64(recoveryData["expiresAt"].(float64))
	if time.Now().Unix() > expiresAt {
		h.store.DeleteRecoveryToken(ctx, recoveryKey)
		return common.BadRequest(fmt.Errorf("token expired")), nil
	}

	// Check if token was already used
	if used, ok := recoveryData["used"].(bool); ok && used {
		return common.BadRequest(fmt.Errorf("token already used")), nil
	}

	// Get username
	username := recoveryData["username"].(string)

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return common.InternalServerError(err), nil
	}

	// Update user password
	updates := map[string]interface{}{
		"password_hash": string(hashedPassword),
		"updated_at":    time.Now(),
	}

	if err := h.store.UpdateUser(ctx, username, updates); err != nil {
		h.logger.Error("failed to update password", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Mark token as used
	recoveryData["used"] = true
	h.store.StoreRecoveryToken(ctx, recoveryKey, recoveryData)

	// Revoke all existing sessions for security
	sessions, err := h.store.GetUserSessions(ctx, username)
	if err == nil {
		for _, session := range sessions {
			h.store.DeleteSession(ctx, session.SessionID)
		}
	}

	h.logger.Info("password reset completed",
		zap.String("username", username))

	return common.OK(map[string]string{
		"message": "Password reset successful. Please login with your new password.",
	}), nil
}

// HandleAccountRecoveryOptions returns available recovery options
// GET /auth/recovery/options
func (h *Handler) HandleAccountRecoveryOptions(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	identifier := request.QueryStringParameters["identifier"]
	if identifier == "" {
		return common.BadRequest(fmt.Errorf("identifier required")), nil
	}

	// Try to find user
	var user *storage.User
	var err error

	// Check if it's an email
	if strings.Contains(identifier, "@") {
		user, err = h.store.GetUserByEmail(ctx, identifier)
	} else {
		user, err = h.store.GetUser(ctx, identifier)
	}

	// Always return some options to prevent user enumeration
	defaultOptions := map[string]interface{}{
		"options": []string{},
		"message": "No recovery options available for this account",
	}

	if err != nil || user == nil {
		return common.OK(defaultOptions), nil
	}

	options := []string{}

	// Check available recovery methods
	if user.Email != "" {
		options = append(options, "email")
	}

	// Check linked OAuth providers
	linkedProviders, err := h.store.GetLinkedProviders(ctx, user.Username)
	if err == nil && len(linkedProviders) > 0 {
		for _, provider := range linkedProviders {
			options = append(options, fmt.Sprintf("oauth_%s", provider))
		}
	}

	// Check if user has trusted contacts (social recovery)
	// TODO: Implement social recovery

	if len(options) == 0 {
		return common.OK(defaultOptions), nil
	}

	return common.OK(map[string]interface{}{
		"options": options,
		"message": "Select a recovery method",
	}), nil
}

// HandleSendRecoveryCode sends a recovery code via selected method
// POST /auth/recovery/send-code
func (h *Handler) HandleSendRecoveryCode(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req struct {
		Identifier string `json:"identifier"`
		Method     string `json:"method"`
	}

	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Find user
	var user *storage.User
	var err error

	if strings.Contains(req.Identifier, "@") {
		user, err = h.store.GetUserByEmail(ctx, req.Identifier)
	} else {
		user, err = h.store.GetUser(ctx, req.Identifier)
	}

	// Always return success to prevent enumeration
	successResponse := common.OK(map[string]string{
		"message": "Recovery code sent if the account exists",
	})

	if err != nil || user == nil {
		return successResponse, nil
	}

	// Generate 6-digit recovery code
	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)

	// Store recovery code with 15 minute expiration
	codeKey := fmt.Sprintf("RECOVERY_CODE#%s#%s", user.Username, code)
	codeData := map[string]interface{}{
		"username":  user.Username,
		"code":      code,
		"method":    req.Method,
		"expiresAt": time.Now().Add(15 * time.Minute).Unix(),
		"attempts":  0,
	}

	if err := h.store.StoreRecoveryToken(ctx, codeKey, codeData); err != nil {
		h.logger.Error("failed to store recovery code", zap.Error(err))
		return successResponse, nil
	}

	switch req.Method {
	case "email":
		if user.Email != "" {
			// TODO: Send email with code
			h.logger.Info("would send recovery code via email",
				zap.String("email", user.Email),
				zap.String("code", code))
		}
	case "sms":
		// TODO: Implement SMS recovery
	default:
		h.logger.Warn("unsupported recovery method",
			zap.String("method", req.Method))
	}

	// In development, return the code
	if os.Getenv("ENVIRONMENT") == "development" {
		return common.OK(map[string]interface{}{
			"message": "Recovery code sent (dev mode)",
			"code":    code,
		}), nil
	}

	return successResponse, nil
}

// maskUsername masks part of the username for privacy
func maskUsername(username string) string {
	if len(username) <= 3 {
		return strings.Repeat("*", len(username))
	}

	if len(username) <= 6 {
		return username[:1] + strings.Repeat("*", len(username)-2) + username[len(username)-1:]
	}

	return username[:2] + strings.Repeat("*", len(username)-4) + username[len(username)-2:]
}
