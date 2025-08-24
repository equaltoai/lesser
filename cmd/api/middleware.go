package main

import (
	"errors"
	"os"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
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
				zap.String("function_name", os.Getenv("AWS_LAMBDA_FUNCTION_NAME")),
				zap.String("function_version", os.Getenv("AWS_LAMBDA_FUNCTION_VERSION")),
				zap.String("cold_start", os.Getenv("AWS_LAMBDA_INITIALIZATION_TYPE")),
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

// createCORSMiddleware creates a CORS middleware with security headers
func createCORSMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// CORS headers for Mastodon client compatibility
			ctx.Response.Header("Access-Control-Allow-Origin", "*")
			ctx.Response.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
			ctx.Response.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto, X-CSRF-Token")
			ctx.Response.Header("Access-Control-Expose-Headers", "Link, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset")
			ctx.Response.Header("Access-Control-Max-Age", "86400") // 24 hours

			// Security headers
			ctx.Response.Header("X-Frame-Options", "DENY")
			ctx.Response.Header("X-Content-Type-Options", "nosniff")
			ctx.Response.Header("X-XSS-Protection", "1; mode=block")
			ctx.Response.Header("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content Security Policy - strict but allows Mastodon client functionality
			csp := "default-src 'self'; " +
				"script-src 'self' 'unsafe-inline'; " +
				"style-src 'self' 'unsafe-inline'; " +
				"img-src 'self' data: https:; " +
				"media-src 'self' https:; " +
				"connect-src 'self' wss: https:; " +
				"font-src 'self' data:; " +
				"object-src 'none'; " +
				"base-uri 'self'; " +
				"form-action 'self'; " +
				"frame-ancestors 'none'"
			ctx.Response.Header("Content-Security-Policy", csp)

			// HTTPS enforcement (HSTS) - only if over HTTPS
			if ctx.Header("X-Forwarded-Proto") == "https" || ctx.Header("CloudFront-Forwarded-Proto") == "https" {
				ctx.Response.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
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
