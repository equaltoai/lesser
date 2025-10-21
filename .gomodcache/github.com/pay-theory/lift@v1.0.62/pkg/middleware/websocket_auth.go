package middleware

import (
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/security"
)

// WebSocketAuthConfig configures WebSocket authentication
// Memory optimized: 160 → 136 bytes (24 bytes saved)
type WebSocketAuthConfig struct {
	// 8-byte aligned fields (functions, slices)
	TokenExtractor func(ctx *lift.Context) string           // 8 bytes (function pointer)
	OnError        func(ctx *lift.Context, err error) error // 8 bytes (function pointer)
	SkipRoutes     []string                                 // 24 bytes (slice)

	// Struct field
	JWTConfig security.JWTConfig // struct
}

// WebSocketAuth creates authentication middleware for WebSocket connections
func WebSocketAuth(config WebSocketAuthConfig) lift.Middleware {
	handler, err := newWebSocketAuthHandler(config)
	if err != nil {
		// Return middleware that returns the initialization error
		return func(_ lift.Handler) lift.Handler {
			return lift.HandlerFunc(func(_ *lift.Context) error {
				return err
			})
		}
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return handler.handle(ctx, next)
		})
	}
}

// WebSocketAuthFromQuery is a simple token extractor that gets the token from query parameters
func WebSocketAuthFromQuery(paramName string) func(ctx *lift.Context) string {
	return func(ctx *lift.Context) string {
		return ctx.Query(paramName)
	}
}

// WebSocketAuthFromHeader is a token extractor that gets the token from headers
func WebSocketAuthFromHeader(headerName string) func(ctx *lift.Context) string {
	return func(ctx *lift.Context) string {
		return ctx.Header(headerName)
	}
}

// webSocketAuthHandler handles WebSocket authentication workflow
type webSocketAuthHandler struct {
	validator      *JWTValidator
	contextManager *securityContextManager
	tokenExtractor tokenExtractor
	config         WebSocketAuthConfig
}

// newWebSocketAuthHandler creates a new WebSocket auth handler
func newWebSocketAuthHandler(config WebSocketAuthConfig) (*webSocketAuthHandler, error) {
	validator, err := NewJWTValidator(config.JWTConfig)
	if err != nil {
		return nil, err
	}

	return &webSocketAuthHandler{
		config:         config,
		tokenExtractor: newTokenExtractor(config),
		validator:      validator,
		contextManager: newSecurityContextManager(),
	}, nil
}

// handle processes WebSocket authentication
func (h *webSocketAuthHandler) handle(ctx *lift.Context, next lift.Handler) error {
	// Check if this is a WebSocket context
	wsCtx, err := ctx.AsWebSocket()
	if err != nil {
		return next.Handle(ctx) // Not a WebSocket context
	}

	// Check route filtering
	if h.shouldSkipRoute(wsCtx.RouteKey()) {
		return next.Handle(ctx)
	}

	// Handle based on event type
	if wsCtx.IsConnectEvent() {
		return h.handleConnectEvent(ctx, wsCtx, next)
	}

	return h.handleNonConnectEvent(ctx, next)
}

// shouldSkipRoute checks if the route should skip authentication
func (h *webSocketAuthHandler) shouldSkipRoute(routeKey string) bool {
	for _, skip := range h.config.SkipRoutes {
		if routeKey == skip {
			return true
		}
	}
	return false
}

// handleConnectEvent processes WebSocket connect events with full authentication
func (h *webSocketAuthHandler) handleConnectEvent(ctx *lift.Context, wsCtx *lift.WebSocketContext, next lift.Handler) error {
	// Extract and validate token
	token, err := h.tokenExtractor.extractToken(ctx)
	if err != nil {
		return h.handleError(ctx, err)
	}

	// Validate JWT token
	claims, err := h.validator.ValidateToken(token)
	if err != nil {
		return h.handleError(ctx, err)
	}

	// Set up security context
	h.contextManager.setupSecurityContext(ctx, claims)

	// Log successful authentication
	h.logAuthentication(ctx, wsCtx, claims)

	return next.Handle(ctx)
}

// handleNonConnectEvent processes non-connect WebSocket events
func (h *webSocketAuthHandler) handleNonConnectEvent(ctx *lift.Context, next lift.Handler) error {
	if ctx.UserID() == "" {
		err := lift.NewLiftError("UNAUTHORIZED", "User not authenticated", 401)
		return h.handleError(ctx, err)
	}
	return next.Handle(ctx)
}

// handleError handles authentication errors
func (h *webSocketAuthHandler) handleError(ctx *lift.Context, err error) error {
	if h.config.OnError != nil {
		return h.config.OnError(ctx, err)
	}

	// Default error handling
	if liftErr, ok := err.(*lift.LiftError); ok {
		return ctx.Status(liftErr.StatusCode).JSON(map[string]string{
			"error": liftErr.Message,
		})
	}

	return ctx.Status(500).JSON(map[string]string{
		"error": err.Error(),
	})
}

// logAuthentication logs successful WebSocket authentication
func (h *webSocketAuthHandler) logAuthentication(ctx *lift.Context, wsCtx *lift.WebSocketContext, claims *JWTClaims) {
	if ctx.Logger != nil {
		ctx.Logger.Info("WebSocket authenticated", map[string]any{
			"user_id":       claims.Subject,
			"connection_id": wsCtx.ConnectionID(),
			"route_key":     wsCtx.RouteKey(),
		})
	}
}

// tokenExtractor handles token extraction from WebSocket requests
type tokenExtractor struct {
	config WebSocketAuthConfig
}

// newTokenExtractor creates a new token extractor
func newTokenExtractor(config WebSocketAuthConfig) tokenExtractor {
	return tokenExtractor{config: config}
}

// extractToken extracts the authentication token from the request
func (te *tokenExtractor) extractToken(ctx *lift.Context) (string, error) {
	var token string

	if te.config.TokenExtractor != nil {
		token = te.config.TokenExtractor(ctx)
	} else {
		token = te.extractTokenDefault(ctx)
	}

	// Clean token format
	token = te.cleanToken(token)

	if token == "" {
		return "", lift.NewLiftError("MISSING_TOKEN", "Missing authentication token", 401)
	}

	return token, nil
}

// extractTokenDefault uses default token extraction logic
func (te *tokenExtractor) extractTokenDefault(ctx *lift.Context) string {
	// Try query parameters first (common for WebSocket)
	if token := ctx.Query("Authorization"); token != "" {
		return token
	}
	if token := ctx.Query("authorization"); token != "" {
		return token
	}
	if token := ctx.Query("token"); token != "" {
		return token
	}

	// Try headers as fallback
	if token := ctx.Header("Authorization"); token != "" {
		return token
	}
	if token := ctx.Header("authorization"); token != "" {
		return token
	}

	return ""
}

// cleanToken removes Bearer prefix and whitespace
func (te *tokenExtractor) cleanToken(token string) string {
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	return strings.TrimSpace(token)
}

// securityContextManager handles security context setup
type securityContextManager struct{}

// newSecurityContextManager creates a new security context manager
func newSecurityContextManager() *securityContextManager {
	return &securityContextManager{}
}

// setupSecurityContext sets up security context from JWT claims
func (scm *securityContextManager) setupSecurityContext(ctx *lift.Context, claims *JWTClaims) {
	// Create security context
	secCtx := lift.WithSecurity(ctx)

	// Create principal from claims
	principal := &security.Principal{
		UserID:     claims.Subject,
		TenantID:   claims.TenantID,
		AccountID:  claims.AccountID,
		Roles:      claims.Roles,
		Scopes:     claims.Scopes,
		AuthMethod: "jwt",
		IssuedAt:   time.Now(),
		IPAddress:  ctx.Header("X-Real-IP"),
		UserAgent:  ctx.Header("User-Agent"),
		RequestID:  ctx.RequestID,
	}

	// Set principal in security context
	secCtx.SetPrincipal(principal)

	// Set backward compatibility context
	scm.setBackwardCompatibility(ctx, claims)
}

// setBackwardCompatibility sets legacy context values for backward compatibility
func (scm *securityContextManager) setBackwardCompatibility(ctx *lift.Context, claims *JWTClaims) {
	ctx.SetUserID(claims.Subject)
	ctx.Set("claims", claims)
	ctx.Set("tenant_id", claims.TenantID)
	ctx.Set("roles", claims.Roles)
}
