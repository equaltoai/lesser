package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// authenticationService implements AuthenticationService
type authenticationService struct {
	jwtSecret string
	storage   StorageAdapter
	repos     interface{}
	logger    *zap.Logger
	config    *config.Config
}

// NewAuthenticationService creates a new authentication service
func NewAuthenticationService(jwtSecret string, cfg *config.Config, repos interface{}) AuthenticationService {
	storage := CreateStorageAdapter(repos)
	return &authenticationService{
		jwtSecret: jwtSecret,
		storage:   storage,
		repos:     repos,
		logger:    zap.NewNop(), // Use nop logger by default
		config:    cfg,
	}
}

// AuthenticateUser validates a token and returns user context
func (a *authenticationService) AuthenticateUser(_ context.Context, token string) (*UserContext, error) {
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return nil, NewUnauthorizedError("Missing authentication token")
	}

	// Extract bearer token if needed
	if bearerToken, err := auth.ExtractBearerToken("Bearer " + token); err == nil {
		token = bearerToken
	}

	// Validate the token using the appropriate storage type
	var oauthSvc *auth.OAuthService
	switch r := a.repos.(type) {
	case core.RepositoryStorage:
		jwtSecret, err := a.jwtSigningSecret()
		if err != nil {
			return nil, NewInternalError("JWT secret configuration is required", err)
		}
		// Create a minimal audit logger for OAuth service
		auditLogger := auth.NewAuditLogger(r, a.logger, auth.DefaultAuditConfig())
		oauthSvc = auth.NewOAuthService(jwtSecret, a.config, r, auditLogger)
	default:
		// For legacy storage, we can't use the OAuth service directly
		// Would need to implement a different validation approach
		return nil, NewInternalError("OAuth validation not supported for legacy storage", nil)
	}
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, NewUnauthorizedError("Invalid authentication token")
	}

	// Build actor ID
	// Note: This assumes a specific actor ID format - adjust as needed for your system
	actorID := fmt.Sprintf("https://your-domain/users/%s", claims.Username)

	return &UserContext{
		Username: claims.Username,
		ActorID:  actorID,
		Claims:   claims,
	}, nil
}

func (a *authenticationService) jwtSigningSecret() (string, error) {
	if a == nil {
		return "", fmt.Errorf("authentication service is nil")
	}
	if secret := strings.TrimSpace(a.jwtSecret); secret != "" {
		return secret, nil
	}
	if a.config == nil {
		return "", fmt.Errorf("config is nil")
	}
	secret, err := a.config.ResolveJWTSecret()
	if err != nil {
		return "", fmt.Errorf("resolve JWT secret: %w", err)
	}
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("JWT secret is empty")
	}
	a.jwtSecret = secret
	return secret, nil
}

// ValidateScope checks if the user has the required scope
func (a *authenticationService) ValidateScope(user *UserContext, requiredScope string) error {
	if user == nil || user.Claims == nil {
		return NewUnauthorizedError("No authentication provided")
	}

	if !user.Claims.HasScope(requiredScope) {
		return NewForbiddenError("Insufficient permissions")
	}

	return nil
}

// AuthenticateUserFromHeader extracts and validates token from authorization header
func AuthenticateUserFromHeader(authHeader string, authService AuthenticationService) (*UserContext, error) {
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		return nil, NewUnauthorizedError("Missing authorization header")
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return nil, NewUnauthorizedError("Invalid authorization header format")
	}

	return authService.AuthenticateUser(context.Background(), token)
}

// ValidateWriteScope is a convenience function for write operations
func ValidateWriteScope(user *UserContext) error {
	authService := &authenticationService{}
	return authService.ValidateScope(user, auth.ScopeWrite)
}

// ValidateReadScope is a convenience function for read operations
func ValidateReadScope(user *UserContext) error {
	authService := &authenticationService{}
	return authService.ValidateScope(user, auth.ScopeRead)
}

// ValidateFollowScope is a convenience function for follow operations
func ValidateFollowScope(user *UserContext) error {
	authService := &authenticationService{}
	if err := authService.ValidateScope(user, "write:follows"); err == nil {
		return nil
	}
	// Fall back to general write scope
	return authService.ValidateScope(user, auth.ScopeWrite)
}
