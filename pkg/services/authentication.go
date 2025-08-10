package services

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/core"
)

// authenticationService implements AuthenticationService
type authenticationService struct {
	jwtSecret string
	storage   StorageAdapter
	repos     interface{}
}

// NewAuthenticationService creates a new authentication service
func NewAuthenticationService(jwtSecret string, repos interface{}) AuthenticationService {
	storage := CreateStorageAdapter(repos)
	return &authenticationService{
		jwtSecret: jwtSecret,
		storage:   storage,
		repos:     repos,
	}
}

// AuthenticateUser validates a token and returns user context
func (a *authenticationService) AuthenticateUser(_ context.Context, token string) (*UserContext, error) {
	if token == "" {
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
		oauthSvc = auth.NewOAuthService(a.jwtSecret, r)
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
	if authHeader == "" {
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