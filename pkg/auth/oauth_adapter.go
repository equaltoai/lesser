package auth

import (
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

// OAuthServiceAdapter adapts AuthService to implement common.OAuthServiceInterface
type OAuthServiceAdapter struct {
	authService *AuthService
}

// NewOAuthServiceAdapter creates a new adapter for AuthService
func NewOAuthServiceAdapter(authService *AuthService) *OAuthServiceAdapter {
	return &OAuthServiceAdapter{
		authService: authService,
	}
}

// ValidateAccessToken validates an access token and returns Claims interface
func (a *OAuthServiceAdapter) ValidateAccessToken(token string) (common.Claims, error) {
	enhancedClaims, err := a.authService.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}

	// EnhancedClaims already implements the Claims interface
	return enhancedClaims, nil
}

// CreateAPIAuthMiddlewareFromAuthService creates API auth middleware from AuthService.
func CreateAPIAuthMiddlewareFromAuthService(authService *AuthService, logger *zap.Logger) apptheory.Middleware {
	return CreatePrincipalContextBridgeFromAuthService(authService, logger, "api")
}

// CreateAPIAuthMiddlewareFromAuthAndOAuthServices creates API auth middleware that preserves
// native session validation while also accepting OAuth-issued tokens.
func CreateAPIAuthMiddlewareFromAuthAndOAuthServices(authService *AuthService, oauthService *OAuthService, logger *zap.Logger) apptheory.Middleware {
	return CreatePrincipalContextBridgeFromAuthAndOAuthServices(authService, oauthService, logger, "api")
}

// CreateGraphQLAuthMiddlewareFromAuthService creates GraphQL auth middleware from AuthService.
func CreateGraphQLAuthMiddlewareFromAuthService(authService *AuthService, logger *zap.Logger) apptheory.Middleware {
	return CreatePrincipalContextBridgeFromAuthService(authService, logger, "graphql")
}

// CreateFederationAuthMiddlewareFromAuthService creates federation auth middleware from AuthService.
func CreateFederationAuthMiddlewareFromAuthService(authService *AuthService, logger *zap.Logger) apptheory.Middleware {
	return CreatePrincipalContextBridgeFromAuthService(authService, logger, "federation")
}
