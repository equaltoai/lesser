// Package middleware provides HTTP middleware for the Lesser application
package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// EnhancedCORSConfig defines enhanced CORS configuration
type EnhancedCORSConfig struct {
	// Allowed origins
	AllowedOrigins  []string
	AllowAllOrigins bool

	// Allowed methods
	AllowedMethods []string

	// Allowed headers
	AllowedHeaders  []string
	AllowAllHeaders bool

	// Exposed headers (accessible to the client)
	ExposedHeaders []string

	// Allow credentials
	AllowCredentials bool

	// Max age for preflight cache (in seconds)
	MaxAge int

	// Whether to pass through OPTIONS requests
	PassthroughOPTIONS bool

	// Custom origin validator function
	ValidateOrigin func(origin string) bool
}

// EnhancedCORS provides Cross-Origin Resource Sharing middleware
type EnhancedCORS struct {
	config *EnhancedCORSConfig
	logger *zap.Logger
}

// NewEnhancedCORS creates a new enhanced CORS middleware
func NewEnhancedCORS(config *EnhancedCORSConfig, logger *zap.Logger) *EnhancedCORS {
	if config == nil {
		config = DefaultEnhancedCORSConfig()
	}

	// Normalize configuration
	config.normalize()

	return &EnhancedCORS{
		config: config,
		logger: logger,
	}
}

// DefaultEnhancedCORSConfig returns default CORS configuration
func DefaultEnhancedCORSConfig() *EnhancedCORSConfig {
	return &EnhancedCORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"X-Requested-With",
			"X-CSRF-Token",
		},
		ExposedHeaders: []string{
			"Link",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
			"X-Total-Count",
			"X-Page",
			"X-Per-Page",
		},
		AllowCredentials:   true,
		MaxAge:             86400, // 24 hours
		PassthroughOPTIONS: false,
	}
}

// StrictEnhancedCORSConfig returns a strict CORS configuration for production
func StrictEnhancedCORSConfig(allowedOrigins []string) *EnhancedCORSConfig {
	return &EnhancedCORSConfig{
		AllowedOrigins:  allowedOrigins,
		AllowAllOrigins: false,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
		},
		AllowedHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
		},
		ExposedHeaders: []string{
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
		},
		AllowCredentials:   true,
		MaxAge:             3600, // 1 hour
		PassthroughOPTIONS: false,
	}
}

// PublicAPIEnhancedCORSConfig returns CORS configuration for public APIs
func PublicAPIEnhancedCORSConfig() *EnhancedCORSConfig {
	return &EnhancedCORSConfig{
		AllowAllOrigins: true,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
		},
		ExposedHeaders: []string{
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
			"Link",
		},
		AllowCredentials:   false, // No credentials for public API
		MaxAge:             86400,
		PassthroughOPTIONS: false,
	}
}

// normalize normalizes the CORS configuration
func (c *EnhancedCORSConfig) normalize() {
	// Normalize methods to uppercase
	for i, method := range c.AllowedMethods {
		c.AllowedMethods[i] = strings.ToUpper(method)
	}

	// Normalize headers to canonical form
	for i, header := range c.AllowedHeaders {
		c.AllowedHeaders[i] = http.CanonicalHeaderKey(header)
	}

	for i, header := range c.ExposedHeaders {
		c.ExposedHeaders[i] = http.CanonicalHeaderKey(header)
	}

	// If allow all origins, set wildcard
	if c.AllowAllOrigins {
		c.AllowedOrigins = []string{"*"}
	}
}

// Middleware returns the CORS middleware for Lift
func (cors *EnhancedCORS) Middleware() func(lift.HandlerFunc) lift.HandlerFunc {
	return func(next lift.HandlerFunc) lift.HandlerFunc {
		return func(ctx *lift.Context) error {
			origin := ctx.Header("Origin")

			// Always set Vary header for Origin
			cors.setVaryHeader(ctx)

			// Check if origin is allowed
			if !cors.isOriginAllowed(origin) {
				// For non-allowed origins, continue without CORS headers
				if ctx.Request.Method == http.MethodOptions {
					ctx.Status(204)
					return nil
				}
				return next(ctx)
			}

			// Handle preflight request
			if ctx.Request.Method == http.MethodOptions {
				return cors.handlePreflight(ctx, origin)
			}

			// Set CORS headers for actual request
			cors.setCORSHeaders(ctx, origin)

			// Continue to next handler
			return next(ctx)
		}
	}
}

// handlePreflight handles CORS preflight requests
func (cors *EnhancedCORS) handlePreflight(ctx *lift.Context, origin string) error {
	requestMethod := ctx.Header("Access-Control-Request-Method")
	requestHeaders := ctx.Header("Access-Control-Request-Headers")

	// Check if requested method is allowed
	if !cors.isMethodAllowed(requestMethod) {
		cors.logger.Debug("CORS preflight rejected - method not allowed",
			zap.String("origin", origin),
			zap.String("method", requestMethod))
		ctx.Status(204)
		return nil
	}

	// Check if requested headers are allowed
	if requestHeaders != "" && !cors.areHeadersAllowed(requestHeaders) {
		cors.logger.Debug("CORS preflight rejected - headers not allowed",
			zap.String("origin", origin),
			zap.String("headers", requestHeaders))
		ctx.Status(204)
		return nil
	}

	// Set preflight response headers
	cors.setPreflightHeaders(ctx, origin, requestMethod, requestHeaders)

	// Handle OPTIONS passthrough if configured
	if cors.config.PassthroughOPTIONS {
		// Continue to next handler
		return nil
	}

	// Return 204 No Content for preflight
	ctx.Status(204)
	return nil
}

// setCORSHeaders sets CORS headers for actual requests
func (cors *EnhancedCORS) setCORSHeaders(ctx *lift.Context, origin string) {
	// Set allowed origin
	if cors.config.AllowAllOrigins {
		ctx.Set("Access-Control-Allow-Origin", "*")
	} else {
		ctx.Set("Access-Control-Allow-Origin", origin)
	}

	// Set credentials if allowed
	if cors.config.AllowCredentials && !cors.config.AllowAllOrigins {
		ctx.Set("Access-Control-Allow-Credentials", "true")
	}

	// Set exposed headers
	if len(cors.config.ExposedHeaders) > 0 {
		ctx.Set("Access-Control-Expose-Headers", strings.Join(cors.config.ExposedHeaders, ", "))
	}
}

// setPreflightHeaders sets headers for preflight responses
func (cors *EnhancedCORS) setPreflightHeaders(ctx *lift.Context, origin, _, requestHeaders string) {
	// Set allowed origin
	if cors.config.AllowAllOrigins {
		ctx.Set("Access-Control-Allow-Origin", "*")
	} else {
		ctx.Set("Access-Control-Allow-Origin", origin)
	}

	// Set allowed methods
	ctx.Set("Access-Control-Allow-Methods", strings.Join(cors.config.AllowedMethods, ", "))

	// Set allowed headers
	if cors.config.AllowAllHeaders && requestHeaders != "" {
		ctx.Set("Access-Control-Allow-Headers", requestHeaders)
	} else if len(cors.config.AllowedHeaders) > 0 {
		ctx.Set("Access-Control-Allow-Headers", strings.Join(cors.config.AllowedHeaders, ", "))
	}

	// Set credentials if allowed
	if cors.config.AllowCredentials && !cors.config.AllowAllOrigins {
		ctx.Set("Access-Control-Allow-Credentials", "true")
	}

	// Set max age
	if cors.config.MaxAge > 0 {
		ctx.Set("Access-Control-Max-Age", strconv.Itoa(cors.config.MaxAge))
	}
}

// setVaryHeader sets the Vary header for CORS
func (cors *EnhancedCORS) setVaryHeader(ctx *lift.Context) {
	vary := ctx.Header("Vary")
	if vary == "" {
		ctx.Set("Vary", "Origin")
	} else if !strings.Contains(strings.ToLower(vary), "origin") {
		ctx.Set("Vary", vary+", Origin")
	}
}

// isOriginAllowed checks if an origin is allowed
func (cors *EnhancedCORS) isOriginAllowed(origin string) bool {
	if origin == "" {
		return true // Allow same-origin requests
	}

	// Check custom validator first
	if cors.config.ValidateOrigin != nil {
		return cors.config.ValidateOrigin(origin)
	}

	// Check if all origins are allowed
	if cors.config.AllowAllOrigins {
		return true
	}

	// Check against allowed origins list
	for _, allowedOrigin := range cors.config.AllowedOrigins {
		if allowedOrigin == "*" {
			return true
		}

		// Exact match
		if allowedOrigin == origin {
			return true
		}

		// Wildcard subdomain match (e.g., https://*.example.com)
		if strings.Contains(allowedOrigin, "*") {
			if cors.matchWildcardOrigin(allowedOrigin, origin) {
				return true
			}
		}
	}

	return false
}

// matchWildcardOrigin matches origin against wildcard pattern
func (cors *EnhancedCORS) matchWildcardOrigin(pattern, origin string) bool {
	// Simple wildcard matching for subdomains
	// e.g., https://*.example.com matches https://api.example.com

	if !strings.Contains(pattern, "*") {
		return pattern == origin
	}

	// Replace * with a regex pattern
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	pattern = "^" + pattern + "$"

	// For simplicity, we'll do prefix/suffix matching
	// In production, you might want to use a proper regex library
	parts := strings.Split(pattern, ".*")
	if len(parts) == 2 {
		return strings.HasPrefix(origin, parts[0]) && strings.HasSuffix(origin, parts[1])
	}

	return false
}

// isMethodAllowed checks if a method is allowed
func (cors *EnhancedCORS) isMethodAllowed(method string) bool {
	if method == "" {
		return false
	}

	method = strings.ToUpper(method)
	for _, allowedMethod := range cors.config.AllowedMethods {
		if allowedMethod == method {
			return true
		}
	}

	return false
}

// areHeadersAllowed checks if requested headers are allowed
func (cors *EnhancedCORS) areHeadersAllowed(requestHeaders string) bool {
	if cors.config.AllowAllHeaders {
		return true
	}

	// Parse requested headers
	headers := strings.Split(requestHeaders, ",")
	for i, header := range headers {
		headers[i] = strings.TrimSpace(header)
		headers[i] = http.CanonicalHeaderKey(headers[i])
	}

	// Check each requested header
	for _, header := range headers {
		if !cors.isHeaderAllowed(header) {
			return false
		}
	}

	return true
}

// isHeaderAllowed checks if a single header is allowed
func (cors *EnhancedCORS) isHeaderAllowed(header string) bool {
	header = http.CanonicalHeaderKey(header)

	// Always allow simple headers
	simpleHeaders := []string{
		"Accept",
		"Accept-Language",
		"Content-Language",
		"Content-Type",
	}

	for _, simpleHeader := range simpleHeaders {
		if header == simpleHeader {
			return true
		}
	}

	// Check against allowed headers
	for _, allowedHeader := range cors.config.AllowedHeaders {
		if allowedHeader == header {
			return true
		}
	}

	return false
}

// FederationEnhancedCORSConfig returns CORS configuration for federation endpoints
func FederationEnhancedCORSConfig() *EnhancedCORSConfig {
	return &EnhancedCORSConfig{
		AllowAllOrigins: true, // Federation requires cross-origin access
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Content-Type",
			"Date",
			"Digest",
			"Signature",
			"Host",
		},
		ExposedHeaders: []string{
			"Content-Type",
			"Date",
			"ETag",
			"Last-Modified",
		},
		AllowCredentials:   false,
		MaxAge:             3600,
		PassthroughOPTIONS: false,
	}
}

// WebSocketEnhancedCORSConfig returns CORS configuration for WebSocket endpoints
func WebSocketEnhancedCORSConfig(allowedOrigins []string) *EnhancedCORSConfig {
	return &EnhancedCORSConfig{
		AllowedOrigins:  allowedOrigins,
		AllowAllOrigins: false,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Origin",
			"Upgrade",
			"Connection",
			"Sec-WebSocket-Key",
			"Sec-WebSocket-Version",
			"Sec-WebSocket-Protocol",
			"Authorization",
		},
		AllowCredentials:   true,
		MaxAge:             3600,
		PassthroughOPTIONS: false,
	}
}
