package auth

import (
	"context"
	"errors"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	// "github.com/equaltoai/lesser/pkg/storage/dynamodb"
	// "go.uber.org/zap"
)

// ContextKey is a type for context keys
type ContextKey string

const (
	// ContextKeyClaims is the context key for JWT claims
	ContextKeyClaims ContextKey = "claims"
)

var (
	// ErrMissingAuthHeader is returned when Authorization header is missing
	ErrMissingAuthHeader = errors.New("missing authorization header")
	// ErrInvalidAuthHeader is returned when Authorization header is malformed
	ErrInvalidAuthHeader = errors.New("invalid authorization header")
)

// Middleware provides authentication middleware functionality
type Middleware struct {
	oauthService *OAuthService
}

// globalMiddleware is reused across Lambda invocations
var globalMiddleware *Middleware

// GetMiddleware returns a singleton middleware instance
func GetMiddleware() (*Middleware, error) {
	if globalMiddleware != nil {
		return globalMiddleware, nil
	}

	// TEMPORARY: Storage initialization needs to be passed in
	// This is part of the migration to Lift
	return nil, errors.New("GetMiddleware needs storage - use handler's auth middleware instead")
}

// NewMiddleware creates a new auth middleware (deprecated - use GetMiddleware)
func NewMiddleware() *Middleware {
	m, err := GetMiddleware()
	if err != nil {
		// Get the JWT secret for fallback
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = "development-secret-change-me"
		}

		// Fallback for backward compatibility, but this won't work properly
		// without storage
		return &Middleware{
			oauthService: &OAuthService{
				jwtSecret: []byte(jwtSecret),
			},
		}
	}
	return m
}

// RequireAuth validates the Bearer token from the request and returns the claims
func (m *Middleware) RequireAuth(_ context.Context, request events.APIGatewayV2HTTPRequest) (*Claims, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if authHeader == "" {
		return nil, ErrMissingAuthHeader
	}

	// Extract bearer token
	token, err := ExtractBearerToken(authHeader)
	if err != nil {
		return nil, err
	}

	// Validate token
	claims, err := m.oauthService.ValidateAccessToken(token)
	if err != nil {
		// Log authentication failure
		ip := request.Headers["X-Forwarded-For"]
		if ip == "" {
			ip = request.RequestContext.HTTP.SourceIP
		}
		userAgent := request.Headers["User-Agent"]

		common.LogAuthFailure(err.Error(), "", ip, userAgent)
		return nil, err
	}

	return claims, nil
}

// RequireScope checks if the claims have the required scope
func (m *Middleware) RequireScope(claims *Claims, scope string) error {
	if !claims.HasScope(scope) {
		return errors.New("insufficient scope")
	}
	return nil
}

// RequireUser checks if the claims match the specified username
func (m *Middleware) RequireUser(claims *Claims, username string) error {
	if claims.Username != username {
		return errors.New("unauthorized for this user")
	}
	return nil
}

// WithClaims adds claims to the context
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, ContextKeyClaims, claims)
}

// GetClaims retrieves claims from the context
func GetClaims(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(ContextKeyClaims).(*Claims)
	return claims, ok
}
