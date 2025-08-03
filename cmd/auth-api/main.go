package main

/*
Auth API Service - Authentication Management

This Lambda function handles authentication operations outside of the main API.
It provides endpoints for login, logout, password management, WebAuthn/passkeys,
and wallet authentication.

This is a separate service from the main API to handle authentication concerns
independently and provide better security isolation.
*/

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

// AuthAPIHandler handles all auth API endpoints
type AuthAPIHandler struct {
	authService *auth.AuthService
	store       storage.Storage
	logger      *zap.Logger
	cfg         *config.Config
}

// NewAuthAPIHandler creates a new auth API handler
func NewAuthAPIHandler(authService *auth.AuthService, store storage.Storage, logger *zap.Logger, cfg *config.Config) *AuthAPIHandler {
	return &AuthAPIHandler{
		authService: authService,
		store:       store,
		logger:      logger,
		cfg:         cfg,
	}
}

// HandleLogin handles user login
func (h *AuthAPIHandler) HandleLogin(ctx *lift.Context) error {
	// TODO: Implement login handler
	return ctx.JSON(map[string]string{
		"status": "login endpoint - implementation pending",
	})
}

// HandleRefreshToken handles token refresh
func (h *AuthAPIHandler) HandleRefreshToken(ctx *lift.Context) error {
	// TODO: Implement refresh token handler
	return ctx.JSON(map[string]string{
		"status": "refresh token endpoint - implementation pending",
	})
}

// HandleLogout handles user logout
func (h *AuthAPIHandler) HandleLogout(ctx *lift.Context) error {
	// TODO: Implement logout handler
	return ctx.JSON(map[string]string{
		"status": "logout endpoint - implementation pending",
	})
}

// HandleLogoutAllDevices handles logout from all devices
func (h *AuthAPIHandler) HandleLogoutAllDevices(ctx *lift.Context) error {
	// TODO: Implement logout all devices handler
	return ctx.JSON(map[string]string{
		"status": "logout all devices endpoint - implementation pending",
	})
}

// HandleGetDevices returns user's devices/sessions
func (h *AuthAPIHandler) HandleGetDevices(ctx *lift.Context) error {
	// TODO: Implement get devices handler
	return ctx.JSON(map[string]string{
		"status": "get devices endpoint - implementation pending",
	})
}

// HandleTrustDevice marks a device as trusted
func (h *AuthAPIHandler) HandleTrustDevice(ctx *lift.Context) error {
	// TODO: Implement trust device handler
	deviceID := ctx.Param("deviceID")
	return ctx.JSON(map[string]string{
		"status":   "trust device endpoint - implementation pending",
		"deviceID": deviceID,
	})
}

// HandleDeleteDevice removes a device/session
func (h *AuthAPIHandler) HandleDeleteDevice(ctx *lift.Context) error {
	// TODO: Implement delete device handler
	deviceID := ctx.Param("deviceID")
	return ctx.JSON(map[string]string{
		"status":   "delete device endpoint - implementation pending",
		"deviceID": deviceID,
	})
}

// HandleChangePassword handles password change
func (h *AuthAPIHandler) HandleChangePassword(ctx *lift.Context) error {
	// TODO: Implement change password handler
	return ctx.JSON(map[string]string{
		"status": "change password endpoint - implementation pending",
	})
}

// HandleGetAccountStatus returns account status (admin)
func (h *AuthAPIHandler) HandleGetAccountStatus(ctx *lift.Context) error {
	// TODO: Implement get account status handler
	username := ctx.Param("username")
	return ctx.JSON(map[string]string{
		"status":   "get account status endpoint - implementation pending",
		"username": username,
	})
}

// HandleClearAccountLockout clears account lockout (admin)
func (h *AuthAPIHandler) HandleClearAccountLockout(ctx *lift.Context) error {
	// TODO: Implement clear lockout handler
	username := ctx.Param("username")
	return ctx.JSON(map[string]string{
		"status":   "clear lockout endpoint - implementation pending",
		"username": username,
	})
}

// WebAuthn handlers

// HandleBeginWebAuthnRegistration starts WebAuthn registration
func (h *AuthAPIHandler) HandleBeginWebAuthnRegistration(ctx *lift.Context) error {
	// TODO: Implement WebAuthn registration begin
	return ctx.JSON(map[string]string{
		"status": "begin WebAuthn registration endpoint - implementation pending",
	})
}

// HandleFinishWebAuthnRegistration completes WebAuthn registration
func (h *AuthAPIHandler) HandleFinishWebAuthnRegistration(ctx *lift.Context) error {
	// TODO: Implement WebAuthn registration finish
	return ctx.JSON(map[string]string{
		"status": "finish WebAuthn registration endpoint - implementation pending",
	})
}

// HandleBeginWebAuthnLogin starts WebAuthn login
func (h *AuthAPIHandler) HandleBeginWebAuthnLogin(ctx *lift.Context) error {
	// TODO: Implement WebAuthn login begin
	return ctx.JSON(map[string]string{
		"status": "begin WebAuthn login endpoint - implementation pending",
	})
}

// HandleFinishWebAuthnLogin completes WebAuthn login
func (h *AuthAPIHandler) HandleFinishWebAuthnLogin(ctx *lift.Context) error {
	// TODO: Implement WebAuthn login finish
	return ctx.JSON(map[string]string{
		"status": "finish WebAuthn login endpoint - implementation pending",
	})
}

// HandleListCredentials lists WebAuthn credentials
func (h *AuthAPIHandler) HandleListCredentials(ctx *lift.Context) error {
	// TODO: Implement list credentials handler
	return ctx.JSON(map[string]string{
		"status": "list credentials endpoint - implementation pending",
	})
}

// HandleDeleteCredential removes a WebAuthn credential
func (h *AuthAPIHandler) HandleDeleteCredential(ctx *lift.Context) error {
	// TODO: Implement delete credential handler
	credentialID := ctx.Param("credentialID")
	return ctx.JSON(map[string]string{
		"status":       "delete credential endpoint - implementation pending",
		"credentialID": credentialID,
	})
}

// HandleUpdateCredentialName updates credential name
func (h *AuthAPIHandler) HandleUpdateCredentialName(ctx *lift.Context) error {
	// TODO: Implement update credential name handler
	credentialID := ctx.Param("credentialID")
	return ctx.JSON(map[string]string{
		"status":       "update credential name endpoint - implementation pending",
		"credentialID": credentialID,
	})
}

// Wallet auth handlers

// HandleCreateWalletChallenge creates wallet challenge
func (h *AuthAPIHandler) HandleCreateWalletChallenge(ctx *lift.Context) error {
	// TODO: Implement create wallet challenge handler
	return ctx.JSON(map[string]string{
		"status": "create wallet challenge endpoint - implementation pending",
	})
}

// HandleVerifyWalletSignature verifies wallet signature
func (h *AuthAPIHandler) HandleVerifyWalletSignature(ctx *lift.Context) error {
	// TODO: Implement verify wallet signature handler
	return ctx.JSON(map[string]string{
		"status": "verify wallet signature endpoint - implementation pending",
	})
}

// HandleLinkWallet links wallet to account
func (h *AuthAPIHandler) HandleLinkWallet(ctx *lift.Context) error {
	// TODO: Implement link wallet handler
	return ctx.JSON(map[string]string{
		"status": "link wallet endpoint - implementation pending",
	})
}

// HandleUnlinkWallet unlinks wallet from account
func (h *AuthAPIHandler) HandleUnlinkWallet(ctx *lift.Context) error {
	// TODO: Implement unlink wallet handler
	walletAddress := ctx.Param("walletAddress")
	return ctx.JSON(map[string]string{
		"status":        "unlink wallet endpoint - implementation pending",
		"walletAddress": walletAddress,
	})
}

// HandleGetWallets returns user's wallets
func (h *AuthAPIHandler) HandleGetWallets(ctx *lift.Context) error {
	// TODO: Implement get wallets handler
	return ctx.JSON(map[string]string{
		"status": "get wallets endpoint - implementation pending",
	})
}

func main() {
	// Initialize configuration
	cfg := config.Get()
	logger := common.Logger()

	// Initialize DynamORM
	tableName := os.Getenv("DYNAMODB_TABLE")
	if tableName == "" {
		tableName = cfg.DynamoTableName
	}
	if tableName == "" {
		logger.Fatal("DYNAMODB_TABLE environment variable is required")
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Create storage adapter
	store := dynamorm.NewStorageAdapter(db, tableName, logger)
	
	// Initialize repositories
	store.SetUserRepository(repositories.NewUserRepository(db, tableName, logger))
	store.SetAuthRepository(repositories.NewAuthRepository(db, tableName, logger))
	store.SetRelationshipRepository(repositories.NewRelationshipRepository(db, tableName, logger))

	// Initialize auth service
	authService, err := auth.NewAuthService(store)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	// Create handler
	handler := NewAuthAPIHandler(authService, store, logger, cfg)

	// Create Lift app
	app := lift.New()
	if os.Getenv("DEBUG") == "true" {
		app = lift.New(lift.WithDebug())
	}

	// Add global middleware
	app.Use(middleware.TimeoutMiddleware(middleware.TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
	}))

	// Add logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			
			err := next.Handle(ctx)
			
			logger.Info("auth api request",
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("error", err != nil),
			)
			
			return err
		})
	})

	// Add CORS middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers
			ctx.Response.Headers["Access-Control-Allow-Origin"] = "*"
			ctx.Response.Headers["Access-Control-Allow-Methods"] = "GET, POST, PUT, DELETE, PATCH, OPTIONS"
			ctx.Response.Headers["Access-Control-Allow-Headers"] = "Content-Type, Authorization, X-Requested-With"
			ctx.Response.Headers["Access-Control-Max-Age"] = "86400"
			
			// Handle preflight
			if ctx.Request.Method == http.MethodOptions {
				return ctx.Status(http.StatusNoContent).Text("")
			}
			
			return next.Handle(ctx)
		})
	})

	// Configure routes

	// Basic auth
	app.POST("/auth/login", handler.HandleLogin)
	app.POST("/auth/refresh", handler.HandleRefreshToken)
	app.POST("/auth/logout", handler.HandleLogout)
	app.POST("/auth/logout/all", handler.HandleLogoutAllDevices)

	// Device management
	app.GET("/auth/devices", handler.HandleGetDevices)
	app.POST("/auth/devices/:deviceID/trust", handler.HandleTrustDevice)
	app.DELETE("/auth/devices/:deviceID", handler.HandleDeleteDevice)

	// Password management
	app.POST("/auth/password/change", handler.HandleChangePassword)

	// Admin endpoints
	app.GET("/auth/accounts/:username/status", handler.HandleGetAccountStatus)
	app.POST("/auth/accounts/:username/unlock", handler.HandleClearAccountLockout)

	// WebAuthn/Passkeys
	app.POST("/auth/webauthn/register/begin", handler.HandleBeginWebAuthnRegistration)
	app.POST("/auth/webauthn/register/finish", handler.HandleFinishWebAuthnRegistration)
	app.POST("/auth/webauthn/login/begin", handler.HandleBeginWebAuthnLogin)
	app.POST("/auth/webauthn/login/finish", handler.HandleFinishWebAuthnLogin)
	app.GET("/auth/webauthn/credentials", handler.HandleListCredentials)
	app.DELETE("/auth/webauthn/credentials/:credentialID", handler.HandleDeleteCredential)
	app.PUT("/auth/webauthn/credentials/:credentialID", handler.HandleUpdateCredentialName)

	// Wallet authentication
	app.POST("/auth/wallet/challenge", handler.HandleCreateWalletChallenge)
	app.POST("/auth/wallet/verify", handler.HandleVerifyWalletSignature)
	app.POST("/auth/wallet/link", handler.HandleLinkWallet)
	app.DELETE("/auth/wallet/unlink/:walletAddress", handler.HandleUnlinkWallet)
	app.GET("/auth/wallet/list", handler.HandleGetWallets)

	// Start Lambda handler
	lambda.Start(app.HandleRequest)
}