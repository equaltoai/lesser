// Package middleware provides security middleware application utilities
package middleware

import (
	"context"
	"strconv"
	"strings"

	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

type contextKey string

const securityTypeKey contextKey = "security_type"

// SecurityMiddlewareType defines the type of security middleware to apply
type SecurityMiddlewareType int

// SecurityMiddlewareType values
const (
	// SecurityTypeAPI for web client API endpoints
	SecurityTypeAPI SecurityMiddlewareType = iota
	// SecurityTypeFederation for federation-compatible security
	SecurityTypeFederation
	SecurityTypeMedia
	SecurityTypeWebSocket
)

// ApplySecurityMiddleware applies appropriate security middleware based on service type
func ApplySecurityMiddleware(app *lift.App, securityType SecurityMiddlewareType, logger *zap.Logger) {
	switch securityType {
	case SecurityTypeAPI:
		applyAPISecurityMiddleware(app, logger)
	case SecurityTypeFederation:
		applyFederationSecurityMiddleware(app, logger)
	case SecurityTypeMedia:
		applyMediaSecurityMiddleware(app, logger)
	case SecurityTypeWebSocket:
		applyWebSocketSecurityMiddleware(app, logger)
	}
}

// applyAPISecurityMiddleware applies strict security for web client API endpoints
func applyAPISecurityMiddleware(app *lift.App, logger *zap.Logger) {
	// Apply strict CORS for web clients
	corsConfig := GetWebClientCORSConfig()
	app.Use(createLiftCORSMiddleware(&corsConfig, logger))

	// Apply strict security headers
	securityConfig := WebClientSecurityHeaders()
	app.Use(createLiftSecurityMiddleware(securityConfig, logger))

	// Apply body size limits for API endpoints
	app.Use(createLiftBodyLimitMiddleware(512*1024, logger)) // 512KB

	logger.Info("applied API security middleware")
}

// applyFederationSecurityMiddleware applies federation-compatible security
func applyFederationSecurityMiddleware(app *lift.App, logger *zap.Logger) {
	// Apply permissive CORS for federation
	corsConfig := GetFederationCORSConfig()
	app.Use(createLiftCORSMiddleware(&corsConfig, logger))

	// Apply federation-friendly security headers
	securityConfig := ActivityPubSecurityHeaders()
	app.Use(createLiftSecurityMiddleware(securityConfig, logger))

	// Apply body size limits for federation endpoints
	app.Use(createLiftBodyLimitMiddleware(1024*1024, logger)) // 1MB

	logger.Info("applied federation security middleware")
}

// applyMediaSecurityMiddleware applies security for media endpoints
func applyMediaSecurityMiddleware(app *lift.App, logger *zap.Logger) {
	// Apply media-friendly CORS (use web client CORS for now)
	corsConfig := GetWebClientCORSConfig()
	app.Use(createLiftCORSMiddleware(&corsConfig, logger))

	// Apply media-specific security headers
	securityConfig := MediaSecurityHeaders()
	app.Use(createLiftSecurityMiddleware(securityConfig, logger))

	// Apply larger body size limits for media
	app.Use(createLiftBodyLimitMiddleware(40*1024*1024, logger)) // 40MB

	logger.Info("applied media security middleware")
}

// applyWebSocketSecurityMiddleware applies security for WebSocket endpoints
func applyWebSocketSecurityMiddleware(app *lift.App, logger *zap.Logger) {
	// WebSocket endpoints use strict CORS like API
	corsConfig := GetWebClientCORSConfig()
	app.Use(createLiftCORSMiddleware(&corsConfig, logger))

	// Apply WebSocket-specific security headers
	securityConfig := WebSocketSecurityHeaders()
	app.Use(createLiftSecurityMiddleware(securityConfig, logger))

	// Small body limits for WebSocket
	app.Use(createLiftBodyLimitMiddleware(64*1024, logger)) // 64KB

	logger.Info("applied WebSocket security middleware")
}

// createLiftCORSMiddleware creates a Lift-compatible CORS middleware
func createLiftCORSMiddleware(config *CORSConfig, _ *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			origin := ctx.Header("Origin")
			if origin == "" {
				origin = ctx.Header("origin")
			}

			allowed, useWildcard := checkOriginAllowed(origin, config.AllowedOrigins)

			// Handle preflight requests
			if ctx.Request.Method == "OPTIONS" {
				return handlePreflightRequest(ctx, config, allowed, useWildcard, origin)
			}

			// Set CORS headers for actual requests and continue
			setActualRequestHeaders(ctx, config, allowed, useWildcard, origin)

			return next.Handle(ctx)
		})
	}
}

// handlePreflightRequest handles CORS preflight (OPTIONS) requests
func handlePreflightRequest(ctx *lift.Context, config *CORSConfig, allowed, useWildcard bool, origin string) error {
	// Set origin header
	if allowed {
		if useWildcard {
			ctx.Response.Header("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			ctx.Response.Header("Access-Control-Allow-Origin", origin)
		}
	}

	// Set preflight headers
	ctx.Response.Header("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
	ctx.Response.Header("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
	ctx.Response.Header("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))

	if config.AllowCredentials {
		ctx.Response.Header("Access-Control-Allow-Credentials", "true")
	}

	ctx.Response.Header("Vary", "Origin")
	return ctx.Status(204).Text("")
}

// setActualRequestHeaders sets CORS headers for non-preflight requests
func setActualRequestHeaders(ctx *lift.Context, config *CORSConfig, allowed, useWildcard bool, origin string) {
	if allowed {
		if useWildcard {
			ctx.Response.Header("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			ctx.Response.Header("Access-Control-Allow-Origin", origin)
		}
	}

	if len(config.ExposedHeaders) > 0 {
		exposedHeaders := strings.Join(config.ExposedHeaders, ", ")
		ctx.Response.Header("Access-Control-Expose-Headers", exposedHeaders)
		// Debug: log exposed headers for OAuth endpoints
		if strings.Contains(ctx.Request.Path, "/oauth/") {
			ctx.Set("_cors_exposed_headers", exposedHeaders)
		}
	}

	if config.AllowCredentials {
		ctx.Response.Header("Access-Control-Allow-Credentials", "true")
	}

	existingVary := ctx.Response.Headers["Vary"]
	if existingVary == "" {
		ctx.Response.Header("Vary", "Origin")
	} else if !strings.Contains(existingVary, "Origin") {
		ctx.Response.Header("Vary", existingVary+", Origin")
	}
}

// createLiftSecurityMiddleware creates a Lift-compatible security headers middleware
func createLiftSecurityMiddleware(config *SecurityHeadersConfig, logger *zap.Logger) lift.Middleware {
	// Use the existing enhanced security headers middleware
	enhancedHeaders := NewEnhancedSecurityHeaders(config, logger)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Generate nonce if configured
			var nonce string
			if config.GenerateNonce {
				nonce = enhancedHeaders.generateNonce()
				ctx.Set("csp-nonce", nonce)
			}

			// Set security headers using the enhanced headers implementation
			enhancedHeaders.setSecurityHeaders(ctx, nonce)

			// Continue to next handler
			return next.Handle(ctx)
		})
	}
}

// createLiftBodyLimitMiddleware creates a Lift-compatible body size limit middleware
func createLiftBodyLimitMiddleware(maxSize int64, logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Check content-length header if present
			if contentLength := ctx.Header("Content-Length"); contentLength != "" {
				if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
					if size > maxSize {
						logger.Warn("request body too large",
							zap.Int64("size", size),
							zap.Int64("max_size", maxSize),
							zap.String("path", ctx.Request.Path))

						return ctx.Status(413).JSON(map[string]interface{}{
							"error":     "payload_too_large",
							"message":   "Request body too large",
							"max_size":  maxSize,
							"your_size": size,
						})
					}
				}
			}

			// Check actual body size if available
			if ctx.Request != nil && ctx.Request.Body != nil {
				bodySize := len(ctx.Request.Body)
				if int64(bodySize) > maxSize {
					logger.Warn("request body too large",
						zap.Int("body_size", bodySize),
						zap.Int64("max_size", maxSize),
						zap.String("path", ctx.Request.Path))

					return ctx.Status(413).JSON(map[string]interface{}{
						"error":     "payload_too_large",
						"message":   "Request body too large",
						"max_size":  maxSize,
						"your_size": bodySize,
					})
				}
			}

			return next.Handle(ctx)
		})
	}
}

// ApplyInputValidation applies ActivityPub input validation to federation endpoints
func ApplyInputValidation(app *lift.App, logger *zap.Logger) {
	// This would be applied specifically to routes that need it
	// For now, create a middleware that can be selectively applied
	app.Use(createInputValidationMiddleware(logger))

	logger.Info("applied input validation middleware")
}

// createInputValidationMiddleware creates input validation middleware
func createInputValidationMiddleware(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			path := ctx.Request.Path

			// Only validate on federation POST endpoints
			if ctx.Request.Method == "POST" && isFederationEndpointForValidation(path) {
				// Basic size validation (detailed validation would be in handlers)
				if ctx.Request.Body != nil && len(ctx.Request.Body) > 1024*1024 {
					logger.Warn("ActivityPub object too large",
						zap.String("path", path),
						zap.Int("size", len(ctx.Request.Body)))

					return ctx.Status(413).JSON(map[string]string{
						"error":   "payload_too_large",
						"message": "ActivityPub object too large (max 1MB)",
					})
				}

				// Content-Type validation for ActivityPub
				contentType := ctx.Header("Content-Type")
				if contentType != "" && !isValidActivityPubContentType(contentType) {
					logger.Warn("invalid content type for ActivityPub",
						zap.String("content_type", contentType),
						zap.String("path", path))

					return ctx.Status(400).JSON(map[string]string{
						"error":   "invalid_content_type",
						"message": "Invalid content type for ActivityPub endpoint",
					})
				}
			}

			return next.Handle(ctx)
		})
	}
}

// isFederationEndpointForValidation checks if path needs ActivityPub validation
func isFederationEndpointForValidation(path string) bool {
	return strings.Contains(path, "/inbox") || strings.Contains(path, "/outbox")
}

// isValidActivityPubContentType checks if content type is valid for ActivityPub
func isValidActivityPubContentType(contentType string) bool {
	validTypes := []string{
		"application/activity+json",
		"application/ld+json",
		"application/json",
	}

	contentTypeLower := strings.ToLower(contentType)
	for _, validType := range validTypes {
		if strings.Contains(contentTypeLower, validType) {
			return true
		}
	}

	return false
}

// GetSecurityTypeForService determines security type based on service name
func GetSecurityTypeForService(serviceName string) SecurityMiddlewareType {
	switch serviceName {
	case "api", "auth", "auth-api", "search", "graphql":
		return SecurityTypeAPI
	case "inbox", "outbox", "actor", "objects", "webfinger", "nodeinfo":
		return SecurityTypeFederation
	case "media", "media-processor":
		return SecurityTypeMedia
	case "streaming", "websocket":
		return SecurityTypeWebSocket
	default:
		// Default to API security for unknown services
		return SecurityTypeAPI
	}
}

// CreateSecurityContext creates a context with security information
func CreateSecurityContext(ctx context.Context, securityType SecurityMiddlewareType, _ *zap.Logger) context.Context {
	// Add security type to context for use by handlers
	return context.WithValue(ctx, securityTypeKey, securityType)
}
