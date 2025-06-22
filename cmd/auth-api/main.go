package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/aron23/lesser/cmd/api/handlers"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	logger          *zap.Logger
	authHandler     *handlers.AuthHandler
	webAuthnHandler *handlers.WebAuthnHandler
	walletHandler   *handlers.WalletHandler
	authService     *auth.AuthService
)

func init() {
	logger = common.Logger()

	// Initialize storage
	store, err := dynamodb.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Initialize auth service
	authService, err = auth.NewAuthService(store)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	// Initialize handlers
	authHandler, err = handlers.NewAuthHandler()
	if err != nil {
		logger.Fatal("failed to initialize auth handler", zap.Error(err))
	}

	webAuthnHandler = handlers.NewWebAuthnHandler(authService)
	walletHandler = handlers.NewWalletHandler(authService)
}

func lambdaHandler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Wrap the actual handler with cost tracking
	return cost.WrapHandler(handleRequest, logger)(ctx, request)
}

func handleRequest(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get the path
	path := request.RequestContext.HTTP.Path

	// Remove stage prefix if present
	if request.RequestContext.Stage != "" && request.RequestContext.Stage != "$default" {
		stagePrefix := "/" + request.RequestContext.Stage
		if strings.HasPrefix(path, stagePrefix) {
			path = strings.TrimPrefix(path, stagePrefix)
		}
	}

	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Remove trailing slash for consistency (except for root path)
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}

	// Log request
	logger.Info("Auth API request",
		zap.String("path", path),
		zap.String("method", request.RequestContext.HTTP.Method),
		zap.Any("headers", request.Headers))

	// Handle OPTIONS requests for CORS preflight
	if request.RequestContext.HTTP.Method == http.MethodOptions {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]string{
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, PATCH, OPTIONS",
				"Access-Control-Allow-Headers": "Content-Type, Authorization, X-Requested-With",
				"Access-Control-Max-Age":       "86400",
			},
		}, nil
	}

	// Route based on path and method
	method := request.RequestContext.HTTP.Method

	// ==================== BASIC AUTH ====================
	// Password login
	if path == "/auth/login" && method == http.MethodPost {
		return authHandler.HandleLogin(ctx, request)
	}

	// Token refresh
	if path == "/auth/refresh" && method == http.MethodPost {
		return authHandler.HandleRefreshToken(ctx, request)
	}

	// Logout
	if path == "/auth/logout" && method == http.MethodPost {
		return authHandler.HandleLogout(ctx, request)
	}

	// Logout all devices
	if path == "/auth/logout/all" && method == http.MethodPost {
		return authHandler.HandleLogoutAllDevices(ctx, request)
	}

	// ==================== DEVICE MANAGEMENT ====================
	// Get devices
	if path == "/auth/devices" && method == http.MethodGet {
		return authHandler.HandleGetDevices(ctx, request)
	}

	// Trust device
	if strings.HasPrefix(path, "/auth/devices/") && strings.HasSuffix(path, "/trust") && method == http.MethodPost {
		return authHandler.HandleTrustDevice(ctx, request)
	}

	// Delete device/session
	if strings.HasPrefix(path, "/auth/devices/") && method == http.MethodDelete {
		return authHandler.HandleDeleteDevice(ctx, request)
	}

	// ==================== PASSWORD MANAGEMENT ====================
	// Change password
	if path == "/auth/password/change" && method == http.MethodPost {
		return authHandler.HandleChangePassword(ctx, request)
	}

	// ==================== ADMIN ENDPOINTS ====================
	// Get account status
	if strings.HasPrefix(path, "/auth/accounts/") && strings.HasSuffix(path, "/status") && method == http.MethodGet {
		return authHandler.HandleGetAccountStatus(ctx, request)
	}

	// Clear account lockout
	if strings.HasPrefix(path, "/auth/accounts/") && strings.HasSuffix(path, "/unlock") && method == http.MethodPost {
		return authHandler.HandleClearAccountLockout(ctx, request)
	}

	// ==================== WEBAUTHN/PASSKEYS ====================
	// Begin registration
	if path == "/auth/webauthn/register/begin" && method == http.MethodPost {
		return webAuthnHandler.BeginRegistration(ctx, request)
	}

	// Finish registration
	if path == "/auth/webauthn/register/finish" && method == http.MethodPost {
		return webAuthnHandler.FinishRegistration(ctx, request)
	}

	// Begin login
	if path == "/auth/webauthn/login/begin" && method == http.MethodPost {
		return webAuthnHandler.BeginLogin(ctx, request)
	}

	// Finish login
	if path == "/auth/webauthn/login/finish" && method == http.MethodPost {
		return webAuthnHandler.FinishLogin(ctx, request)
	}

	// List credentials
	if path == "/auth/webauthn/credentials" && method == http.MethodGet {
		return webAuthnHandler.ListCredentials(ctx, request)
	}

	// Delete credential
	if strings.HasPrefix(path, "/auth/webauthn/credentials/") && method == http.MethodDelete {
		return webAuthnHandler.DeleteCredential(ctx, request)
	}

	// Update credential name
	if strings.HasPrefix(path, "/auth/webauthn/credentials/") && method == http.MethodPut {
		return webAuthnHandler.UpdateCredentialName(ctx, request)
	}

	// ==================== WALLET AUTHENTICATION ====================
	// Create wallet challenge
	if path == "/auth/wallet/challenge" && method == http.MethodPost {
		return walletHandler.CreateChallenge(ctx, request)
	}

	// Verify wallet signature
	if path == "/auth/wallet/verify" && method == http.MethodPost {
		return walletHandler.VerifySignature(ctx, request)
	}

	// Link wallet to account
	if path == "/auth/wallet/link" && method == http.MethodPost {
		return walletHandler.LinkWallet(ctx, request)
	}

	// Unlink wallet from account
	if strings.HasPrefix(path, "/auth/wallet/unlink/") && method == http.MethodDelete {
		return walletHandler.UnlinkWallet(ctx, request)
	}

	// Get user's wallets
	if path == "/auth/wallet/list" && method == http.MethodGet {
		return walletHandler.GetWallets(ctx, request)
	}

	// Not found
	return common.NotFound(nil), nil
}

func main() {
	lambda.Start(lambdaHandler)
}
