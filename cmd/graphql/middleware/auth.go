package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

type contextKey string

const (
	// UserContextKey is the key for storing user claims in context
	UserContextKey = contextKey("user")
	// RequestIDKey is the key for storing request ID in context
	RequestIDKey = contextKey("request_id")
)

var (
	// ErrInsufficientScope is returned when the user doesn't have the required scope
	ErrInsufficientScope = errors.New("insufficient scope")
	// ErrUnauthorized is returned when the user is not authorized for the requested resource
	ErrUnauthorized = errors.New("unauthorized for this resource")
)

// AuthMiddleware wraps an HTTP handler with authentication
func AuthMiddleware(authService *auth.Middleware, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Convert http.Request to Lambda-style request for auth middleware
			lambdaRequest := events.APIGatewayV2HTTPRequest{
				Headers: make(map[string]string),
			}

			// Copy headers
			for key, values := range r.Header {
				if len(values) > 0 {
					lambdaRequest.Headers[key] = values[0]
				}
			}

			// Use the auth middleware's RequireAuth method
			claims, err := authService.RequireAuth(r.Context(), lambdaRequest)
			if err != nil {
				logger.Warn("authentication failed",
					zap.String("error", err.Error()),
					zap.String("path", r.URL.Path))

				// Return GraphQL-formatted error
				errorMessage := "Authentication required"
				if err == auth.ErrInvalidAuthHeader {
					errorMessage = "Invalid authorization header"
				} else if err.Error() == "token has expired" {
					errorMessage = "Token has expired"
				}

				http.Error(w, `{"errors":[{"message":"`+errorMessage+`","extensions":{"code":"UNAUTHENTICATED"}}]}`, http.StatusUnauthorized)
				return
			}

			// Add user claims to context
			ctx := context.WithValue(r.Context(), UserContextKey, claims)

			// Add request ID to context for tracing
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = generateRequestID()
			}
			ctx = context.WithValue(ctx, RequestIDKey, requestID)

			// Log authenticated request
			logger.Info("authenticated GraphQL request",
				zap.String("username", claims.Username),
				zap.Strings("scopes", claims.Scopes),
				zap.String("path", r.URL.Path),
				zap.String("request_id", requestID))

			// Pass to next handler with updated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthMiddlewareForLambda wraps the Lambda handler with authentication
// This is used when we need to authenticate at the Lambda level before HTTP conversion
func AuthMiddlewareForLambda(authService *auth.Middleware, logger *zap.Logger) func(context.Context, events.APIGatewayProxyRequest) (*auth.Claims, error) {
	return func(ctx context.Context, request events.APIGatewayProxyRequest) (*auth.Claims, error) {
		// Create a v2 request from v1 for compatibility
		v2Request := events.APIGatewayV2HTTPRequest{
			Headers: request.Headers,
		}

		// Use the auth middleware's RequireAuth method
		claims, err := authService.RequireAuth(ctx, v2Request)
		if err != nil {
			logger.Warn("authentication failed",
				zap.String("error", err.Error()),
				zap.String("path", request.Path))
			return nil, err
		}

		return claims, nil
	}
}

// GetUserFromContext retrieves the authenticated user from the context
func GetUserFromContext(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(UserContextKey).(*auth.Claims)
	return claims, ok
}

// RequireScope checks if the user has the required scope
func RequireScope(ctx context.Context, scope string) error {
	claims, ok := GetUserFromContext(ctx)
	if !ok {
		return auth.ErrMissingAuthHeader
	}

	if !claims.HasScope(scope) {
		return ErrInsufficientScope
	}

	return nil
}

// RequireUser checks if the authenticated user matches the specified username
func RequireUser(ctx context.Context, username string) error {
	claims, ok := GetUserFromContext(ctx)
	if !ok {
		return auth.ErrMissingAuthHeader
	}

	if claims.Username != username {
		return ErrUnauthorized
	}

	return nil
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(b)
}
