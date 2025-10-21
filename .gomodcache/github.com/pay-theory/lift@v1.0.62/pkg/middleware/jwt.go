package middleware

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/pay-theory/lift/pkg/lift"
)

const (
	algorithmHS256 = "HS256"
	algorithmRS256 = "RS256"
)

// JWTConfig holds configuration for JWT middleware
type JWTConfig struct {
	PublicKey    any
	Claims       jwt.Claims
	Validator    func(claims jwt.MapClaims) error
	ErrorHandler func(ctx *lift.Context, err error) error
	Extractor    func(ctx *lift.Context) (string, error)
	Secret       string
	Algorithm    string
	TokenLookup  string
	SkipPaths    []string
}

// DefaultJWTConfig returns a default JWT configuration
func DefaultJWTConfig() JWTConfig {
	return JWTConfig{
		Algorithm:   algorithmHS256,
		TokenLookup: "header:Authorization",
		ErrorHandler: func(ctx *lift.Context, err error) error {
			return ctx.Unauthorized("Invalid or missing token", err)
		},
	}
}

// JWTAuth creates a JWT authentication middleware
func JWTAuth(config JWTConfig) lift.Middleware {
	// Apply default configuration
	config = applyJWTDefaults(config)

	processor := newJWTProcessor(config)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return processor.process(ctx, next)
		})
	}
}

// applyJWTDefaults applies default values to JWT configuration
func applyJWTDefaults(config JWTConfig) JWTConfig {
	if config.Algorithm == "" {
		config.Algorithm = algorithmHS256
	}
	if config.TokenLookup == "" {
		config.TokenLookup = "header:Authorization"
	}
	if config.ErrorHandler == nil {
		config.ErrorHandler = DefaultJWTConfig().ErrorHandler
	}
	if config.Extractor == nil {
		config.Extractor = createExtractor(config.TokenLookup)
	}
	return config
}

// jwtProcessor handles JWT processing logic
type jwtProcessor struct {
	pathSkipper   *jwtPathSkipper
	tokenHandler  *jwtTokenHandler
	claimsHandler *jwtClaimsHandler
	config        JWTConfig
}

// newJWTProcessor creates a new JWT processor
func newJWTProcessor(config JWTConfig) *jwtProcessor {
	return &jwtProcessor{
		config:        config,
		pathSkipper:   newJWTPathSkipper(config.SkipPaths),
		tokenHandler:  newJWTTokenHandler(config),
		claimsHandler: newJWTClaimsHandler(config),
	}
}

// process handles the JWT authentication process
func (p *jwtProcessor) process(ctx *lift.Context, next lift.Handler) error {
	// Check if path should be skipped
	if p.pathSkipper.shouldSkip(ctx.Request.Path) {
		return next.Handle(ctx)
	}

	// Process token
	token, err := p.tokenHandler.processToken(ctx)
	if err != nil {
		return p.config.ErrorHandler(ctx, err)
	}

	// Process claims
	if err := p.claimsHandler.processClaims(ctx, token); err != nil {
		return p.config.ErrorHandler(ctx, err)
	}

	// Continue to next handler
	return next.Handle(ctx)
}

// jwtPathSkipper handles path skipping logic
type jwtPathSkipper struct {
	skipPaths []string
}

// newJWTPathSkipper creates a new path skipper
func newJWTPathSkipper(skipPaths []string) *jwtPathSkipper {
	return &jwtPathSkipper{skipPaths: skipPaths}
}

// shouldSkip checks if a path should skip JWT authentication
func (ps *jwtPathSkipper) shouldSkip(path string) bool {
	for _, skipPath := range ps.skipPaths {
		if path == skipPath || strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// jwtTokenHandler handles token extraction and parsing
type jwtTokenHandler struct {
	config JWTConfig
}

// newJWTTokenHandler creates a new token handler
func newJWTTokenHandler(config JWTConfig) *jwtTokenHandler {
	return &jwtTokenHandler{config: config}
}

// processToken extracts and parses the JWT token
func (th *jwtTokenHandler) processToken(ctx *lift.Context) (*jwt.Token, error) {
	// Extract token
	tokenString, err := th.config.Extractor(ctx)
	if err != nil {
		return nil, err
	}

	// Parse token
	token, err := parseToken(tokenString, th.config)
	if err != nil {
		return nil, err
	}

	// Validate token
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return token, nil
}

// jwtClaimsHandler handles claims extraction and validation
type jwtClaimsHandler struct {
	config JWTConfig
}

// newJWTClaimsHandler creates a new claims handler
func newJWTClaimsHandler(config JWTConfig) *jwtClaimsHandler {
	return &jwtClaimsHandler{config: config}
}

// processClaims extracts and validates JWT claims
func (ch *jwtClaimsHandler) processClaims(ctx *lift.Context, token *jwt.Token) error {
	// Extract claims
	claims, err := ch.extractClaims(token)
	if err != nil {
		return err
	}

	// Validate claims if validator provided
	if ch.config.Validator != nil {
		if err := ch.config.Validator(claims); err != nil {
			return err
		}
	}

	// Set claims in context
	ctx.SetClaims(claims)
	return nil
}

// extractClaims extracts claims from the token
func (ch *jwtClaimsHandler) extractClaims(token *jwt.Token) (jwt.MapClaims, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		// Try to handle custom claims
		if ch.config.Claims != nil {
			// Create empty MapClaims for custom claims handling
			// This is a simplified version - in production you'd want more robust conversion
			return jwt.MapClaims{}, nil
		}
		return nil, fmt.Errorf("invalid claims type")
	}
	return claims, nil
}

// createExtractor creates a token extractor based on the lookup string
func createExtractor(lookup string) func(*lift.Context) (string, error) {
	parts := strings.Split(lookup, ":")
	if len(parts) != 2 {
		// Return an extractor that always returns an error
		return func(_ *lift.Context) (string, error) {
			return "", fmt.Errorf("invalid token lookup format: expected 'type:field', got '%s'", lookup)
		}
	}

	switch parts[0] {
	case "header":
		return func(ctx *lift.Context) (string, error) {
			auth := ctx.Header(parts[1])
			if auth == "" {
				return "", fmt.Errorf("missing %s header", parts[1])
			}
			// Handle Bearer token
			if parts[1] == "Authorization" && strings.HasPrefix(auth, "Bearer ") {
				return strings.TrimPrefix(auth, "Bearer "), nil
			}
			return auth, nil
		}
	case "query":
		return func(ctx *lift.Context) (string, error) {
			token := ctx.Query(parts[1])
			if token == "" {
				return "", fmt.Errorf("missing %s query parameter", parts[1])
			}
			return token, nil
		}
	case "cookie":
		return func(ctx *lift.Context) (string, error) {
			return extractJWTFromCookie(ctx, parts[1])
		}
	default:
		// Return an extractor that always returns an error
		return func(_ *lift.Context) (string, error) {
			return "", fmt.Errorf("unsupported token lookup type: %s (supported: header, query, cookie)", parts[0])
		}
	}
}

// parseToken parses and validates a JWT token
func parseToken(tokenString string, config JWTConfig) (*jwt.Token, error) {
	// Parse with appropriate method based on algorithm
	switch config.Algorithm {
	case algorithmHS256, "HS384", "HS512":
		return jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
			// Validate algorithm
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(config.Secret), nil
		})
	case algorithmRS256, "RS384", "RS512":
		return jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
			// Validate algorithm
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return config.PublicKey, nil
		})
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", config.Algorithm)
	}
}

// WithJWTAuth is a convenience function for creating JWT middleware with minimal config
func WithJWTAuth(secret string) lift.Middleware {
	return JWTAuth(JWTConfig{
		Secret:    secret,
		Algorithm: algorithmHS256,
	})
}

// extractJWTFromCookie extracts JWT token from HTTP cookies with security validation
func extractJWTFromCookie(ctx *lift.Context, cookieName string) (string, error) {
	// Parse cookies from the Cookie header
	cookieHeader := ctx.Header("Cookie")
	if cookieHeader == "" {
		return "", fmt.Errorf("no cookies found in request")
	}

	// Parse individual cookies
	cookies := parseCookies(cookieHeader)

	// Find the JWT cookie
	tokenCookie, exists := cookies[cookieName]
	if !exists {
		return "", fmt.Errorf("JWT cookie '%s' not found", cookieName)
	}

	// Validate the cookie token
	if err := validateJWTCookie(tokenCookie); err != nil {
		return "", fmt.Errorf("invalid JWT cookie: %w", err)
	}

	return tokenCookie.Value, nil
}

// CookieToken represents a parsed HTTP cookie
type CookieToken struct {
	Name     string
	Value    string
	SameSite string
	Path     string
	Domain   string
	MaxAge   int
	HttpOnly bool
	Secure   bool
}

// parseCookies parses the Cookie header value into individual cookies
func parseCookies(cookieHeader string) map[string]*CookieToken {
	cookies := make(map[string]*CookieToken)

	// Split by semicolon to get individual cookies
	parts := strings.Split(cookieHeader, ";")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Find the first equals sign to separate name from value
		equalIndex := strings.Index(part, "=")
		if equalIndex == -1 {
			continue // Skip malformed cookies
		}

		name := strings.TrimSpace(part[:equalIndex])
		value := strings.TrimSpace(part[equalIndex+1:])

		// Remove quotes if present
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}

		cookies[name] = &CookieToken{
			Name:  name,
			Value: value,
			// Note: Cookie header only contains name=value pairs
			// Attributes like HttpOnly, Secure, etc. are only in Set-Cookie response headers
		}
	}

	return cookies
}

// validateJWTCookie performs security validation on the JWT cookie
func validateJWTCookie(cookie *CookieToken) error {
	// Validate cookie name
	if cookie.Name == "" {
		return fmt.Errorf("cookie name cannot be empty")
	}

	// Validate token value is not empty
	if cookie.Value == "" {
		return fmt.Errorf("JWT token value cannot be empty")
	}

	// Basic JWT format validation (header.payload.signature)
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid JWT format: expected 3 parts separated by dots, got %d", len(parts))
	}

	// Validate each part is base64-encoded (basic check)
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("JWT part %d is empty", i+1)
		}

		// Check for valid base64url characters
		for _, char := range part {
			if !isValidBase64URLChar(char) {
				return fmt.Errorf("JWT part %d contains invalid base64url character: %c", i+1, char)
			}
		}
	}

	// Validate token length (prevent extremely long tokens)
	if len(cookie.Value) > 8192 { // 8KB limit
		return fmt.Errorf("JWT token too long: %d bytes (max 8192)", len(cookie.Value))
	}

	return nil
}

// isValidBase64URLChar checks if a character is valid in base64url encoding
func isValidBase64URLChar(c rune) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' // base64url uses - and _ instead of + and /
}
