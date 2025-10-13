package auth

import (
	"os"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// UnifiedAuthMiddleware provides centralized authentication middleware for all services
type UnifiedAuthMiddleware struct {
	oauthService common.OAuthServiceInterface
	logger       *zap.Logger
	serviceName  string
}

// MiddlewareConfig holds configuration for unified authentication middleware
type MiddlewareConfig struct {
	OAuthService  common.OAuthServiceInterface
	Logger        *zap.Logger
	ServiceName   string
	Required      bool     // Whether authentication is required or optional
	RequiredScope string   // Required scope for access
	AllowedScopes []string // Multiple allowed scopes (alternative to RequiredScope)
}

// NewUnifiedAuthMiddleware creates a new unified authentication middleware
func NewUnifiedAuthMiddleware(config MiddlewareConfig) *UnifiedAuthMiddleware {
	return &UnifiedAuthMiddleware{
		oauthService: config.OAuthService,
		logger:       config.Logger,
		serviceName:  config.ServiceName,
	}
}

// RequiredAuth creates middleware that requires authentication with a specific scope
func RequiredAuth(config MiddlewareConfig) lift.Middleware {

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Use consolidated authentication pattern
			authResult := common.ExtractAndValidateAuth(ctx, config.RequiredScope, config.OAuthService)

			if authResult.Error != nil {
				// Use consolidated error response
				return common.RespondAuthErrorWithCode(ctx, authResult.ErrorCode, authResult.Error)
			}

			// Store authentication context for handlers
			ctx.Set("auth_context", authResult.Context)
			ctx.Set("username", authResult.Context.Username)
			ctx.Set("claims", authResult.Context.Claims)

			if config.Logger != nil {
				config.Logger.Debug("authentication successful",
					zap.String("username", authResult.Context.Username),
					zap.String("service", config.ServiceName))
			}

			return next.Handle(ctx)
		})
	}
}

// RequiredAuthWithMultipleScopes creates middleware that requires one of multiple scopes
func RequiredAuthWithMultipleScopes(config MiddlewareConfig) lift.Middleware {

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Use consolidated authentication pattern with multiple scopes
			authResult := common.ExtractAndValidateAuthWithMultipleScopes(ctx, config.AllowedScopes, config.OAuthService)

			if authResult.Error != nil {
				// Use consolidated error response
				return common.RespondAuthErrorWithCode(ctx, authResult.ErrorCode, authResult.Error)
			}

			// Store authentication context for handlers
			ctx.Set("auth_context", authResult.Context)
			ctx.Set("username", authResult.Context.Username)
			ctx.Set("claims", authResult.Context.Claims)

			if config.Logger != nil {
				config.Logger.Debug("authentication successful (multiple scopes)",
					zap.String("username", authResult.Context.Username),
					zap.String("service", config.ServiceName),
					zap.Strings("allowed_scopes", config.AllowedScopes),
				)
			}

			return next.Handle(ctx)
		})
	}
}

// OptionalAuth creates middleware that provides optional authentication
func OptionalAuth(config MiddlewareConfig) lift.Middleware {

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Use consolidated optional authentication pattern
			authResult := common.ExtractOptionalAuth(ctx, config.OAuthService)

			// Store authentication context for handlers (may be empty for unauthenticated)
			ctx.Set("auth_context", authResult.Context)
			if authResult.Context.Username != "" {
				ctx.Set("username", authResult.Context.Username)
				ctx.Set("claims", authResult.Context.Claims)
				ctx.Set("is_authenticated", true)
			} else {
				ctx.Set("is_authenticated", false)
			}

			if config.Logger != nil && authResult.Context.Username != "" {
				config.Logger.Debug("optional authentication successful",
					zap.String("username", authResult.Context.Username),
					zap.String("service", config.ServiceName),
				)
			}

			return next.Handle(ctx)
		})
	}
}

// WriteAuth creates middleware specifically for write operations
func WriteAuth(config MiddlewareConfig) lift.Middleware {
	// Set required scope for write operations
	config.RequiredScope = common.ScopeWrite
	return RequiredAuth(config)
}

// ReadAuth creates middleware specifically for read operations
func ReadAuth(config MiddlewareConfig) lift.Middleware {
	// Set required scope for read operations
	config.RequiredScope = common.ScopeRead
	return RequiredAuth(config)
}

// AdminAuth creates middleware specifically for admin operations
func AdminAuth(config MiddlewareConfig) lift.Middleware {
	// Set allowed scopes for admin operations
	config.AllowedScopes = common.AdminScopes
	return RequiredAuthWithMultipleScopes(config)
}

// FollowAuth creates middleware specifically for follow operations
func FollowAuth(config MiddlewareConfig) lift.Middleware {
	// Set allowed scopes for follow operations
	config.AllowedScopes = common.FollowScopes
	return RequiredAuthWithMultipleScopes(config)
}

// Helper functions for common middleware patterns

// CreateAPIAuthMiddleware creates standard API authentication middleware
func CreateAPIAuthMiddleware(oauthService common.OAuthServiceInterface, logger *zap.Logger) lift.Middleware {
	return OptionalAuth(MiddlewareConfig{
		OAuthService: oauthService,
		Logger:       logger,
		ServiceName:  "api",
	})
}

// CreateGraphQLAuthMiddleware creates standard GraphQL authentication middleware
func CreateGraphQLAuthMiddleware(oauthService common.OAuthServiceInterface, logger *zap.Logger) lift.Middleware {
	return OptionalAuth(MiddlewareConfig{
		OAuthService: oauthService,
		Logger:       logger,
		ServiceName:  "graphql",
	})
}

// CreateFederationAuthMiddleware creates federation-specific authentication middleware
func CreateFederationAuthMiddleware(oauthService common.OAuthServiceInterface, logger *zap.Logger) lift.Middleware {
	return OptionalAuth(MiddlewareConfig{
		OAuthService: oauthService,
		Logger:       logger,
		ServiceName:  "federation",
	})
}

// Utility functions for accessing authentication context in handlers

// GetLegacyAuthContext retrieves the legacy authentication context from the Lift context
func GetLegacyAuthContext(ctx *lift.Context) *common.AuthContext {
	if authCtx, ok := ctx.Get("auth_context").(*common.AuthContext); ok {
		return authCtx
	}
	return &common.AuthContext{} // Return empty context if not found
}

// GetAuthenticatedUsername retrieves the authenticated username or empty string
func GetAuthenticatedUsername(ctx *lift.Context) string {
	if username, ok := ctx.Get("username").(string); ok {
		return username
	}
	return ""
}

// GetJWTClaims retrieves the JWT claims from context
func GetJWTClaims(ctx *lift.Context) common.Claims {
	if claims, ok := ctx.Get("claims").(common.Claims); ok {
		return claims
	}
	return nil
}

// IsAuthenticated returns true if the request is authenticated
func IsAuthenticated(ctx *lift.Context) bool {
	if authenticated, ok := ctx.Get("is_authenticated").(bool); ok {
		return authenticated
	}
	return GetAuthenticatedUsername(ctx) != ""
}

// IsTestMode determines if the current request is in test mode
func IsTestMode(_ *lift.Context) bool {
	// For now, test mode is determined by environment variable
	return os.Getenv("TEST_MODE") == "true"
}

// RequireWriteAccess validates that the current request has write access
func RequireWriteAccess(ctx *lift.Context) error {
	authCtx := GetLegacyAuthContext(ctx)
	authResult := &common.AuthenticationResult{Context: authCtx}
	return common.ValidateWriteAccess(authResult)
}

// RequireReadAccess validates that the current request has read access
func RequireReadAccess(ctx *lift.Context) error {
	authCtx := GetLegacyAuthContext(ctx)
	authResult := &common.AuthenticationResult{Context: authCtx}
	return common.ValidateReadAccess(authResult)
}
