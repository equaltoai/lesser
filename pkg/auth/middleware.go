package auth

import (
	"context"
	"errors"
	"os"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
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

var (
	// globalMiddleware is reused across Lambda invocations
	globalMiddleware *Middleware
)

// GetMiddleware returns a singleton middleware instance
func GetMiddleware() (*Middleware, error) {
	if globalMiddleware != nil {
		return globalMiddleware, nil
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "development-secret-change-me"
	}

	// Initialize storage
	store, err := dynamodb.New()
	if err != nil {
		common.Logger().Error("failed to initialize storage for middleware", zap.Error(err))
		return nil, err
	}

	globalMiddleware = &Middleware{
		oauthService: NewOAuthService(jwtSecret, store),
	}

	return globalMiddleware, nil
}

// NewMiddleware creates a new auth middleware (deprecated - use GetMiddleware)
func NewMiddleware() *Middleware {
	m, err := GetMiddleware()
	if err != nil {
		// Fallback for backward compatibility, but this won't work properly
		// without storage
		return &Middleware{
			oauthService: &OAuthService{
				jwtSecret: []byte("development-secret-change-me"),
			},
		}
	}
	return m
}

// RequireAuth validates the Bearer token from the request and returns the claims
func (m *Middleware) RequireAuth(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*Claims, error) {
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
