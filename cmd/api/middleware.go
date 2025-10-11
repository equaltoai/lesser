package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// createLoggingMiddleware creates a custom logging middleware with structured correlation
func createLoggingMiddleware(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.GetRequestID()

			// Extract user and tenant context for correlation
			userID := ""
			tenantID := ""
			if claims, ok := ctx.Get("claims").(*auth.Claims); ok && claims != nil {
				userID = claims.Username
			}
			if tenant, ok := ctx.Get("tenantID").(string); ok {
				tenantID = tenant
			}

			// Create contextual logger with correlation fields
			contextLogger := logger.With(
				zap.String("request_id", requestID),
				zap.String("user_id", userID),
				zap.String("tenant_id", tenantID),
				zap.String("function_name", common.GetLambdaFunctionName()),
				zap.String("function_version", common.GetLambdaFunctionVersion()),
				zap.String("cold_start", common.GetLambdaInitializationType()),
			)

			// Store contextual logger in context
			ctx.Set("logger", contextLogger)

			// Log request start
			contextLogger.Info("request_start",
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.String("user_agent", ctx.Header("User-Agent")),
				zap.String("remote_addr", ctx.Header("X-Forwarded-For")),
			)

			// Process the request
			err := next.Handle(ctx)

			// Calculate execution metrics
			duration := time.Since(start)
			statusCode := ctx.Response.StatusCode

			// Log request completion with metrics
			logLevel := zap.InfoLevel
			if err != nil {
				logLevel = zap.ErrorLevel
			} else if statusCode >= 400 {
				logLevel = zap.WarnLevel
			}

			contextLogger.Log(logLevel, "request_complete",
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.Int("status", statusCode),
				zap.Duration("duration", duration),
				zap.Bool("success", err == nil && statusCode < 400),
				zap.Error(err),
			)

			return err
		})
	}
}

// createCORSMiddleware creates a CORS middleware with enhanced security headers
func createCORSMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Use the new middleware system for enhanced security
			path := ctx.Request.Path
			
			// Determine if this is a federation request
			isFederation := isFederationEndpoint(path)
			
			if isFederation {
				// Apply ActivityPub-friendly CORS and security headers
				applyCORSHeaders(ctx, middleware.ActivityPubCORSConfig)
				applySecurityHeaders(ctx, middleware.ActivityPubSecurityHeaders())
			} else {
				// Apply strict CORS and security headers for web client API
				applyCORSHeaders(ctx, middleware.DefaultCORSConfig)  
				applySecurityHeaders(ctx, middleware.WebClientSecurityHeaders())
			}
			
			// Apply body size limits
			if err := applyBodyLimits(ctx); err != nil {
				return err
			}

			// Handle OPTIONS requests (CORS preflight)
			if ctx.Request.Method == "OPTIONS" {
				return ctx.Status(200).Text("")
			}

			// Process the request
			return next.Handle(ctx)
		})
	}
}

// isFederationEndpoint checks if a path is a federation endpoint
func isFederationEndpoint(path string) bool {
	federationPaths := []string{
		"/inbox", "/outbox", "/users/", "/.well-known/", "/nodeinfo",
	}
	
	for _, fedPath := range federationPaths {
		if strings.HasPrefix(path, fedPath) || strings.Contains(path, fedPath) {
			return true
		}
	}
	
	return false
}

// applyCORSHeaders applies CORS headers from a configuration
func applyCORSHeaders(ctx *lift.Context, config middleware.CORSConfig) {
	// Apply origin handling
	origin := ctx.Header("Origin")
	if len(config.AllowedOrigins) > 0 && config.AllowedOrigins[0] == "*" {
		ctx.Response.Header("Access-Control-Allow-Origin", "*")
	} else {
		for _, allowedOrigin := range config.AllowedOrigins {
			if allowedOrigin == origin {
				ctx.Response.Header("Access-Control-Allow-Origin", origin)
				break
			}
		}
	}
	
	// Apply other CORS headers
	ctx.Response.Header("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
	ctx.Response.Header("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
	ctx.Response.Header("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
	ctx.Response.Header("Access-Control-Max-Age", fmt.Sprintf("%d", config.MaxAge))
	
	if config.AllowCredentials {
		ctx.Response.Header("Access-Control-Allow-Credentials", "true")
	}
	
	// Add Vary header for caching
	ctx.Response.Header("Vary", "Origin")
}

// applySecurityHeaders applies security headers from a configuration
func applySecurityHeaders(ctx *lift.Context, config *middleware.SecurityHeadersConfig) {
	// X-Frame-Options
	if config.XFrameOptions != "" {
		ctx.Response.Header("X-Frame-Options", config.XFrameOptions)
	}
	
	// X-Content-Type-Options
	if config.XContentTypeOptions != "" {
		ctx.Response.Header("X-Content-Type-Options", config.XContentTypeOptions)
	}
	
	// X-XSS-Protection
	if config.XXSSProtection != "" {
		ctx.Response.Header("X-XSS-Protection", config.XXSSProtection)
	}
	
	// Referrer Policy
	if config.ReferrerPolicy != "" {
		ctx.Response.Header("Referrer-Policy", config.ReferrerPolicy)
	}
	
	// CSP
	if config.EnableCSP && len(config.CSPDirectives) > 0 {
		var directives []string
		for directive, sources := range config.CSPDirectives {
			if len(sources) > 0 {
				directives = append(directives, fmt.Sprintf("%s %s", directive, strings.Join(sources, " ")))
			} else {
				directives = append(directives, directive)
			}
		}
		csp := strings.Join(directives, "; ")
		if config.CSPReportURI != "" {
			csp += fmt.Sprintf("; report-uri %s", config.CSPReportURI)
		}
		ctx.Response.Header("Content-Security-Policy", csp)
	}
	
	// HSTS (only in production)
	if config.EnableHSTS && os.Getenv("ENVIRONMENT") == "production" {
		hsts := fmt.Sprintf("max-age=%d", config.HSTSMaxAge)
		if config.HSTSIncludeSubDomains {
			hsts += "; includeSubDomains"
		}
		if config.HSTSPreload {
			hsts += "; preload"
		}
		ctx.Response.Header("Strict-Transport-Security", hsts)
	}
	
	// Custom headers
	for key, value := range config.CustomHeaders {
		ctx.Response.Header(key, value)
	}
	
	// Remove potentially dangerous headers
	ctx.Response.Header("X-Powered-By", "")
	ctx.Response.Header("Server", "")
}

// applyBodyLimits applies request body size limits based on endpoint
func applyBodyLimits(ctx *lift.Context) error {
	path := ctx.Request.Path
	
	var maxSize int64
	switch {
	case strings.Contains(path, "/inbox"):
		maxSize = 1024 * 1024 // 1MB for ActivityPub inbox
	case strings.Contains(path, "/outbox"):
		maxSize = 1024 * 1024 // 1MB for ActivityPub outbox  
	case strings.Contains(path, "/api/v1/media"):
		maxSize = 40 * 1024 * 1024 // 40MB for media uploads
	case strings.Contains(path, "/api/v1/statuses"):
		maxSize = 512 * 1024 // 512KB for posts
	case strings.Contains(path, "/oauth/"):
		maxSize = 16 * 1024 // 16KB for OAuth
	case strings.HasPrefix(path, "/api/"):
		maxSize = 256 * 1024 // 256KB for other API endpoints
	case strings.Contains(path, "/.well-known/"):
		maxSize = 64 * 1024 // 64KB for well-known endpoints
	default:
		maxSize = 128 * 1024 // 128KB default
	}
	
	// Check content-length header if present
	if contentLength := ctx.Header("Content-Length"); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			if size > maxSize {
				return ctx.Status(413).JSON(map[string]interface{}{
					"error": "payload_too_large",
					"message": fmt.Sprintf("Request body too large: %d bytes (max %d)", size, maxSize),
					"max_size": maxSize,
				})
			}
		}
	}
	
	// For requests with body, check actual size
	if ctx.Request != nil && ctx.Request.Body != nil {
		bodySize := len(ctx.Request.Body)
		if int64(bodySize) > maxSize {
			return ctx.Status(413).JSON(map[string]interface{}{
				"error": "payload_too_large", 
				"message": fmt.Sprintf("Request body too large: %d bytes (max %d)", bodySize, maxSize),
				"max_size": maxSize,
			})
		}
	}
	
	return nil
}

//nolint:unused // Used in tests and infrastructure patterns - part of complete middleware infrastructure
func createCostTrackingMiddleware(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Initialize unified cost tracking system
			unifiedTracker := cost.NewUnifiedTracker(nil, logger, ctx.GetUserID(), ctx.GetRequestID())

			// Store the unified tracker in context for all cost tracking
			ctx.Set("cost_tracker", unifiedTracker)
			ctx.Set("unified_cost_tracker", unifiedTracker)

			// Track Lambda invocation
			start := time.Now()

			// Process request
			err := next.Handle(ctx)

			// Calculate Lambda execution cost using centralized tracking
			duration := time.Since(start)
			memoryMB := int64(128) // Default Lambda memory, could be configurable
			if trackErr := unifiedTracker.TrackLambdaInvocation(ctx.Request.Context(), "api", duration, memoryMB); trackErr != nil {
				logger.Warn("failed to track lambda cost", zap.Error(trackErr))
			}

			// Calculate and log costs
			totalCost := unifiedTracker.GetCurrentCostMicroCents()
			if totalCost > 0 {
				logger.Info("request_costs",
					zap.String("request_id", ctx.GetRequestID()),
					zap.Int64("total_cost_microcents", totalCost),
				)
			}

			return err
		})
	}
}

// Helper functions for cost tracking

// GetCostTracker retrieves the cost tracker from the Lift context
func GetCostTracker(ctx *lift.Context) *cost.Tracker {
	if tracker, ok := ctx.Get("cost_tracker").(*cost.Tracker); ok {
		return tracker
	}
	return nil
}

// TrackCost is a convenience function to track costs from a Lift context
func TrackCost(ctx *lift.Context, fn func(*cost.Tracker)) {
	if tracker := GetCostTracker(ctx); tracker != nil {
		fn(tracker)
	}
}

// createPerformanceMonitoringMiddleware is deprecated in favor of EMF-based metrics
// Use observability.CreateEMFPerformanceMonitoringMiddleware instead
// This function is kept for backwards compatibility but should not be used in new code
//
//nolint:unused // Used in tests (infrastructure_test.go)
func createPerformanceMonitoringMiddleware(_ *observability.MetricsCollector) lift.Middleware {
	// This is now a no-op since we've migrated to EMF
	// The EMF middleware handles all performance monitoring without polling
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Just pass through - EMF middleware handles metrics collection
			return next.Handle(ctx)
		})
	}
}

// GetLogger retrieves the contextual logger from the Lift context
func GetLogger(ctx *lift.Context) *zap.Logger {
	if logger, ok := ctx.Get("logger").(*zap.Logger); ok {
		return logger
	}
	return zap.L() // fallback to global logger
}

// Permission levels for RBAC
const (
	PermissionAdmin     = "admin"
	PermissionModerator = "moderator"
	PermissionViewer    = "viewer"
)

// createRBACMiddleware creates middleware for role-based access control
// This middleware should be used after authentication middleware
func createRBACMiddleware(requiredPermission string, repos core.RepositoryStorage) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Check if the user has the required permissions
			if err := checkUserPermissions(ctx, requiredPermission, repos); err != nil {
				return common.RespondForbidden(ctx, "You do not have permission to access this resource")
			}

			return next.Handle(ctx)
		})
	}
}

// checkUserPermissions checks if the current user has the required permission level
func checkUserPermissions(ctx *lift.Context, requiredPermission string, repos core.RepositoryStorage) error {
	// Get user claims from context (set by auth middleware)
	claims, ok := ctx.Get("claims").(*auth.Claims)
	if !ok || claims == nil {
		return errors.New(common.ErrorUnauthorizedNoValidClaims)
	}

	// Get user role from storage
	userRole := "user" // Default role
	if repos != nil && repos.User() != nil {
		if user, err := repos.User().GetUser(ctx.Request.Context(), claims.Username); err == nil && user != nil {
			userRole = user.Role
		}
		// If user lookup fails, we fall back to default "user" role for security
	}

	// Check permission hierarchy
	switch requiredPermission {
	case PermissionAdmin:
		// Only admins can access admin resources
		if userRole != PermissionAdmin {
			return errors.New(common.ErrorForbiddenAdminRequired)
		}
	case PermissionModerator:
		// Admins and moderators can access moderator resources
		if userRole != "admin" && userRole != "moderator" {
			return errors.New(common.ErrorForbiddenModeratorRequired)
		}
	case PermissionViewer:
		// Admins, moderators, and viewers can access viewer resources
		if userRole != "admin" && userRole != "moderator" && userRole != "viewer" {
			return errors.New(common.ErrorForbiddenViewerRequired)
		}
	default:
		// Unknown permission level
		return errors.New(common.ErrorForbiddenUnknownPermission)
	}

	return nil
}

// AdminOnlyMiddleware creates middleware that requires admin permissions
func AdminOnlyMiddleware(repos core.RepositoryStorage) lift.Middleware {
	return createRBACMiddleware(PermissionAdmin, repos)
}

// ModeratorOrHigherMiddleware creates middleware that requires moderator or admin permissions
func ModeratorOrHigherMiddleware(repos core.RepositoryStorage) lift.Middleware {
	return createRBACMiddleware(PermissionModerator, repos)
}

// ViewerOrHigherMiddleware creates middleware that requires viewer, moderator, or admin permissions
func ViewerOrHigherMiddleware(repos core.RepositoryStorage) lift.Middleware {
	return createRBACMiddleware(PermissionViewer, repos)
}
