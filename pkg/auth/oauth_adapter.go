package auth

import (
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
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

// CreateAPIAuthMiddlewareFromAuthService creates API auth middleware from AuthService
func CreateAPIAuthMiddlewareFromAuthService(authService *AuthService, logger *zap.Logger) lift.Middleware {
	adapter := NewOAuthServiceAdapter(authService)
	return CreateAPIAuthMiddleware(adapter, logger)
}

// CreateGraphQLAuthMiddlewareFromAuthService creates GraphQL auth middleware from AuthService  
func CreateGraphQLAuthMiddlewareFromAuthService(authService *AuthService, logger *zap.Logger) lift.Middleware {
	adapter := NewOAuthServiceAdapter(authService)
	return CreateGraphQLAuthMiddleware(adapter, logger)
}

// CreateFederationAuthMiddlewareFromAuthService creates federation auth middleware from AuthService
func CreateFederationAuthMiddlewareFromAuthService(authService *AuthService, logger *zap.Logger) lift.Middleware {
	adapter := NewOAuthServiceAdapter(authService)
	return CreateFederationAuthMiddleware(adapter, logger)
}