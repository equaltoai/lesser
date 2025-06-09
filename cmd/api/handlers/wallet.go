package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// WalletHandler handles wallet authentication endpoints
type WalletHandler struct {
	authService *auth.AuthService
}

// NewWalletHandler creates a new wallet handler
func NewWalletHandler(authService *auth.AuthService) *WalletHandler {
	return &WalletHandler{
		authService: authService,
	}
}

// CreateChallenge handles POST /auth/wallet/challenge
func (h *WalletHandler) CreateChallenge(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req struct {
		Address  string `json:"address"`
		ChainID  int    `json:"chainId"`
		Username string `json:"username,omitempty"` // Optional, for linking to existing account
	}

	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Validate inputs
	if req.Address == "" {
		return common.BadRequest(errors.New("address is required")), nil
	}
	if req.ChainID == 0 {
		req.ChainID = 1 // Default to Ethereum mainnet
	}

	// If username is provided, verify the user is authenticated
	if req.Username != "" {
		token := getAuthToken(request)
		if token == "" {
			return common.Unauthorized(errors.New("authentication required to link wallet")), nil
		}

		claims, err := h.authService.ValidateAccessToken(token)
		if err != nil {
			return common.Unauthorized(errors.New("invalid token")), nil
		}

		// Ensure the username matches the authenticated user
		if claims.Username != req.Username {
			return common.Forbidden(errors.New("cannot link wallet to another user")), nil
		}
	}

	// Create challenge
	challenge, err := h.authService.CreateWalletChallenge(ctx, req.Address, req.ChainID, req.Username)
	if err != nil {
		common.Logger().Error("failed to create wallet challenge", zap.Error(err))
		return common.InternalServerError(errors.New("failed to create challenge")), nil
	}

	return common.OK(challenge), nil
}

// VerifySignature handles POST /auth/wallet/verify
func (h *WalletHandler) VerifySignature(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	var req auth.WalletVerifyRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Validate inputs
	if req.ChallengeID == "" || req.Address == "" || req.Signature == "" || req.Message == "" {
		return common.BadRequest(errors.New("missing required fields")), nil
	}

	// Get device info
	deviceName := request.Headers["user-agent"]
	if deviceName == "" {
		deviceName = "Unknown Device"
	}
	userAgent := request.Headers["user-agent"]
	ipAddress := request.Headers["x-forwarded-for"]
	if ipAddress == "" {
		ipAddress = request.RequestContext.HTTP.SourceIP
	}

	// Verify signature and create session
	authResponse, err := h.authService.VerifyWalletSignature(ctx, &req, deviceName, userAgent, ipAddress)
	if err != nil {
		common.Logger().Error("wallet signature verification failed",
			zap.String("address", req.Address),
			zap.Error(err))
		return common.Unauthorized(errors.New("signature verification failed")), nil
	}

	// If no access token, wallet is not linked to any account
	if authResponse.AccessToken == "" {
		return common.OK(map[string]interface{}{
			"authenticated": false,
			"message":       "wallet not linked to any account",
			"address":       req.Address,
		}), nil
	}

	return common.OK(authResponse), nil
}

// LinkWallet handles POST /auth/wallet/link
func (h *WalletHandler) LinkWallet(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	token := getAuthToken(request)
	if token == "" {
		return common.Unauthorized(errors.New("authentication required")), nil
	}

	claims, err := h.authService.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(errors.New("invalid token")), nil
	}

	var req struct {
		Address     string `json:"address"`
		ChainID     int    `json:"chainId"`
		WalletType  string `json:"walletType"`
		ChallengeID string `json:"challengeId"`
		Signature   string `json:"signature"`
		Message     string `json:"message"`
	}

	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Validate inputs
	if req.Address == "" || req.ChallengeID == "" || req.Signature == "" || req.Message == "" {
		return common.BadRequest(errors.New("missing required fields")), nil
	}

	if req.ChainID == 0 {
		req.ChainID = 1 // Default to Ethereum mainnet
	}
	if req.WalletType == "" {
		req.WalletType = "ethereum"
	}

	// First verify the signature
	verifyReq := &auth.WalletVerifyRequest{
		ChallengeID: req.ChallengeID,
		Address:     req.Address,
		Signature:   req.Signature,
		Message:     req.Message,
	}

	// Just verify the signature without creating a session
	authResult, err := h.authService.VerifyWalletSignature(ctx, verifyReq, "", "", "")
	if err != nil {
		return common.Unauthorized(errors.New("signature verification failed")), nil
	}

	// Extract username from auth result
	username := authResult.Me

	// If wallet is already linked to another user, fail
	if username != "" && username != claims.Username {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusConflict,
			Body:       `{"error":"wallet already linked to another account"}`,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, nil
	}

	// Link the wallet
	if err := h.authService.LinkWallet(ctx, claims.Username, req.Address, req.ChainID, req.WalletType); err != nil {
		common.Logger().Error("failed to link wallet",
			zap.String("username", claims.Username),
			zap.String("address", req.Address),
			zap.Error(err))
		return common.InternalServerError(errors.New("failed to link wallet")), nil
	}

	return common.OK(map[string]interface{}{
		"success": true,
		"message": "wallet linked successfully",
		"address": req.Address,
	}), nil
}

// UnlinkWallet handles DELETE /auth/wallet/unlink/{address}
func (h *WalletHandler) UnlinkWallet(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	token := getAuthToken(request)
	if token == "" {
		return common.Unauthorized(errors.New("authentication required")), nil
	}

	claims, err := h.authService.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(errors.New("invalid token")), nil
	}

	// Get address from path
	address := request.PathParameters["address"]
	if address == "" {
		return common.BadRequest(errors.New("address is required")), nil
	}

	// Unlink the wallet
	if err := h.authService.UnlinkWallet(ctx, claims.Username, address); err != nil {
		common.Logger().Error("failed to unlink wallet",
			zap.String("username", claims.Username),
			zap.String("address", address),
			zap.Error(err))
		return common.InternalServerError(errors.New("failed to unlink wallet")), nil
	}

	return common.OK(map[string]interface{}{
		"success": true,
		"message": "wallet unlinked successfully",
		"address": address,
	}), nil
}

// GetWallets handles GET /auth/wallet/list
func (h *WalletHandler) GetWallets(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Require authentication
	token := getAuthToken(request)
	if token == "" {
		return common.Unauthorized(errors.New("authentication required")), nil
	}

	claims, err := h.authService.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(errors.New("invalid token")), nil
	}

	// Get user's wallets
	wallets, err := h.authService.GetUserWallets(ctx, claims.Username)
	if err != nil {
		common.Logger().Error("failed to get user wallets",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(errors.New("failed to get wallets")), nil
	}

	return common.OK(map[string]interface{}{
		"wallets": wallets,
		"count":   len(wallets),
	}), nil
}

// Helper function to extract auth token
func getAuthToken(request events.APIGatewayV2HTTPRequest) string {
	auth := request.Headers["authorization"]
	if auth == "" {
		auth = request.Headers["Authorization"]
	}
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
