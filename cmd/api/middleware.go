package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/logging"
	"github.com/equaltoai/lesser/pkg/observability"
	browsercors "github.com/equaltoai/lesser/pkg/security/cors"
	"github.com/equaltoai/lesser/pkg/storage/core"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

// createLoggingMiddleware creates a custom logging middleware with structured correlation
func createLoggingMiddleware(logger *zap.Logger) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			start := time.Now()
			requestID := ctx.RequestID

			// Extract user and tenant context for correlation
			userID := ""
			if claims := auth.GetJWTClaims(ctx); claims != nil {
				userID = claims.GetUsername()
			}
			tenantID := ctx.TenantID

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
			logPath := logging.SanitizeLogPath(ctx.Request.Path)
			contextLogger.Info("request_start",
				zap.String("method", ctx.Request.Method),
				zap.String("path", logPath),
				zap.String("user_agent", headerValue(ctx, "User-Agent")),
				zap.String("remote_addr", headerValue(ctx, "X-Forwarded-For")),
			)

			// Process the request
			resp, err := next(ctx)

			// Calculate execution metrics
			duration := time.Since(start)
			statusCode := http.StatusOK
			if err != nil {
				statusCode = http.StatusInternalServerError
			} else if resp != nil && resp.Status != 0 {
				statusCode = resp.Status
			}

			// Log request completion with metrics
			logLevel := zap.InfoLevel
			if err != nil {
				logLevel = zap.ErrorLevel
			} else if statusCode >= 400 {
				logLevel = zap.WarnLevel
			}

			contextLogger.Log(logLevel, "request_complete",
				zap.String("method", ctx.Request.Method),
				zap.String("path", logPath),
				zap.Int("status", statusCode),
				zap.Duration("duration", duration),
				zap.Bool("success", err == nil && statusCode < 400),
				zap.Error(err),
			)

			return resp, err
		}
	}
}

func apiSecurityHeaders() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			resp, err := next(ctx)
			if resp == nil {
				return resp, err
			}
			if resp.Headers == nil {
				resp.Headers = map[string][]string{}
			}

			setDefault := func(key string, value []string) {
				if existing := resp.Headers[key]; len(existing) > 0 {
					return
				}
				resp.Headers[key] = value
			}

			setDefault("x-content-type-options", []string{"nosniff"})
			setDefault("x-frame-options", []string{"DENY"})
			setDefault("content-security-policy", []string{"default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; object-src 'none'"})
			setDefault("strict-transport-security", []string{"max-age=31536000; includeSubDomains"})
			setDefault("referrer-policy", []string{"strict-origin-when-cross-origin"})
			setDefault("cross-origin-resource-policy", []string{"same-origin"})
			setDefault("cross-origin-opener-policy", []string{"same-origin"})
			setDefault("permissions-policy", []string{"camera=(), geolocation=(), microphone=(), payment=(), usb=()"})
			setDefault("x-permitted-cross-domain-policies", []string{"none"})
			setDefault("x-robots-tag", []string{"noindex, nofollow"})
			return resp, err
		}
	}
}

func createOAuthOriginRestrictionMiddleware(cfg *config.Config) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if !isOAuthSensitivePath(strings.TrimSpace(ctx.Request.Path)) {
				return next(ctx)
			}

			origin := strings.TrimSpace(headerValue(ctx, "Origin"))
			if origin == "" || isAllowedOAuthOrigin(origin, cfg) {
				return next(ctx)
			}

			return common.RespondForbidden(ctx, "origin not allowed for oauth endpoint")
		}
	}
}

func isOAuthSensitivePath(path string) bool {
	switch {
	case path == "":
		return false
	case strings.HasPrefix(path, "/oauth/"):
		return true
	case path == "/.well-known/oauth-authorization-server":
		return true
	case path == apiV1AppsPath:
		return true
	case strings.HasPrefix(path, apiV1AppsPathPrefix):
		return true
	default:
		return false
	}
}

func isAllowedOAuthOrigin(origin string, cfg *config.Config) bool {
	normalizedOrigin, parsedOrigin, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}

	switch normalizedOrigin {
	case "https://claude.ai", "https://claude.com":
		return true
	}

	if isAllowedLocalDevelopmentOrigin(parsedOrigin) {
		return true
	}

	if cfg == nil {
		return false
	}

	instanceOrigin, _, ok := normalizeOrigin(cfg.BaseURL())
	return ok && normalizedOrigin == instanceOrigin
}

func normalizeOrigin(raw string) (string, *url.URL, bool) {
	return browsercors.NormalizeOrigin(raw)
}

func isAllowedLocalDevelopmentOrigin(origin *url.URL) bool {
	if origin == nil || !strings.EqualFold(origin.Scheme, "http") {
		return false
	}

	switch strings.ToLower(origin.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// createInstanceLockMiddleware blocks publishing and signups until the instance is activated.
func createInstanceLockMiddleware(repos core.RepositoryStorage, logger *zap.Logger) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			method := ctx.Request.Method
			path := ctx.Request.Path

			if method == http.MethodOptions {
				return next(ctx)
			}
			if method == http.MethodGet || method == http.MethodHead {
				return handleLockedReadRequest(ctx, repos, logger, path, method, next)
			}
			return handleLockedWriteRequest(ctx, repos, logger, path, method, next)
		}
	}
}

func handleLockedReadRequest(ctx *apptheory.Context, repos core.RepositoryStorage, logger *zap.Logger, path string, method string, next apptheory.Handler) (*apptheory.Response, error) {
	if !shouldSuppressContentReadWhileLocked(path) {
		return next(ctx)
	}

	locked, err := instanceLocked(ctx, repos)
	if err != nil {
		logger.Warn("failed to get instance lock state; defaulting to locked",
			zap.Error(err),
			zap.String("method", method),
			zap.String("path", logging.SanitizeLogPath(path)))
	}
	if !locked {
		return next(ctx)
	}

	return respondLockedContentRead(ctx, path)
}

func handleLockedWriteRequest(ctx *apptheory.Context, repos core.RepositoryStorage, logger *zap.Logger, path string, method string, next apptheory.Handler) (*apptheory.Response, error) {
	locked, err := instanceLocked(ctx, repos)
	if err != nil {
		logger.Warn("failed to get instance lock state; defaulting to locked",
			zap.Error(err),
			zap.String("method", method),
			zap.String("path", logging.SanitizeLogPath(path)))
		return common.RespondForbidden(ctx, "instance is locked")
	}
	if !locked {
		return next(ctx)
	}
	if isWriteAllowedWhileLocked(path) {
		return next(ctx)
	}
	return common.RespondForbidden(ctx, "instance is locked")
}

func instanceLocked(ctx *apptheory.Context, repos core.RepositoryStorage) (bool, error) {
	state, err := repos.Instance().GetInstanceState(ctx.Context())
	if err != nil {
		return true, err
	}
	return state.Locked, nil
}

func isWriteAllowedWhileLocked(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	return strings.HasPrefix(path, "/setup/") ||
		strings.HasPrefix(path, "/auth/") ||
		strings.HasPrefix(path, "/oauth/") ||
		strings.HasPrefix(path, "/api/v1/auth/") ||
		path == apiV1AppsPath
}

func shouldSuppressContentReadWhileLocked(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}

	// Mastodon content surfaces (read-only).
	if strings.HasPrefix(path, "/api/v1/timelines/") ||
		strings.HasPrefix(path, "/api/v1/statuses/") ||
		strings.HasPrefix(path, "/api/v1/trends") ||
		strings.HasPrefix(path, "/api/v1/bookmarks") ||
		strings.HasPrefix(path, "/api/v1/favourites") ||
		strings.HasPrefix(path, "/api/v2/search") ||
		strings.HasPrefix(path, "/api/v1/search/statuses") ||
		strings.HasPrefix(path, "/api/oembed") {
		return true
	}

	// Account status listings (e.g. /api/v1/accounts/{id}/statuses).
	if strings.HasPrefix(path, "/api/v1/accounts/") && strings.Contains(path, "/statuses") {
		return true
	}

	return false
}

func respondLockedContentRead(ctx *apptheory.Context, path string) (*apptheory.Response, error) {
	// For locked instances, lists become empty and individual objects are 404.
	switch {
	case strings.HasPrefix(path, "/api/v1/statuses/"):
		return common.RespondNotFound(ctx, "status not found")
	case strings.HasPrefix(path, "/api/oembed"):
		return common.RespondNotFound(ctx, "resource not found")
	case strings.HasPrefix(path, "/api/v2/search"):
		return apptheory.JSON(http.StatusOK, map[string]any{
			"accounts": []any{},
			"statuses": []any{},
			"hashtags": []any{},
		})
	default:
		return apptheory.JSON(http.StatusOK, []any{})
	}
}

//nolint:unused // Used in tests and infrastructure patterns - part of complete middleware infrastructure
func createCostTrackingMiddleware(logger *zap.Logger) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			// Initialize unified cost tracking system
			userID := ""
			if claims := auth.GetJWTClaims(ctx); claims != nil {
				userID = claims.GetUsername()
			}
			unifiedTracker := cost.NewUnifiedTracker(nil, logger, userID, ctx.RequestID)

			// Store the unified tracker in context for all cost tracking
			ctx.Set("cost_tracker", unifiedTracker)
			ctx.Set("unified_cost_tracker", unifiedTracker)

			// Track Lambda invocation
			start := time.Now()

			// Process request
			resp, err := next(ctx)

			// Calculate Lambda execution cost using centralized tracking
			duration := time.Since(start)
			memoryMB := int64(128) // Default Lambda memory, could be configurable
			if trackErr := unifiedTracker.TrackLambdaInvocation(ctx.Context(), "api", duration, memoryMB); trackErr != nil {
				logger.Warn("failed to track lambda cost", zap.Error(trackErr))
			}

			// Calculate and log costs
			totalCost := unifiedTracker.GetCurrentCostMicroCents()
			if totalCost > 0 {
				logger.Info("request_costs",
					zap.String("request_id", ctx.RequestID),
					zap.Int64("total_cost_microcents", totalCost),
				)
			}

			return resp, err
		}
	}
}

// Helper functions for cost tracking

func GetCostTracker(ctx *apptheory.Context) *cost.Tracker {
	if tracker, ok := ctx.Get("cost_tracker").(*cost.Tracker); ok {
		return tracker
	}
	return nil
}

func TrackCost(ctx *apptheory.Context, fn func(*cost.Tracker)) {
	if tracker := GetCostTracker(ctx); tracker != nil {
		fn(tracker)
	}
}

// createPerformanceMonitoringMiddleware is deprecated in favor of EMF-based metrics
// Use observability.CreateEMFPerformanceMonitoringMiddleware instead
// This function is kept for backwards compatibility but should not be used in new code
//
//nolint:unused // Used in tests (infrastructure_test.go)
func createPerformanceMonitoringMiddleware(_ *observability.MetricsCollector) apptheory.Middleware {
	// This is now a no-op since we've migrated to EMF
	// The EMF middleware handles all performance monitoring without polling
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			// Just pass through - EMF middleware handles metrics collection
			return next(ctx)
		}
	}
}

func GetLogger(ctx *apptheory.Context) *zap.Logger {
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
func createRBACMiddleware(requiredPermission string, repos core.RepositoryStorage) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			// Check if the user has the required permissions
			if err := checkUserPermissions(ctx, requiredPermission, repos); err != nil {
				return common.RespondForbidden(ctx, "You do not have permission to access this resource")
			}

			return next(ctx)
		}
	}
}

// checkUserPermissions checks if the current user has the required permission level
func checkUserPermissions(ctx *apptheory.Context, requiredPermission string, repos core.RepositoryStorage) error {
	// Get user claims from context (set by auth middleware)
	claims := auth.GetJWTClaims(ctx)
	if claims == nil {
		return errors.New(common.ErrorUnauthorizedNoValidClaims)
	}

	// Get user role from storage
	userRole := "user" // Default role
	if repos != nil && repos.User() != nil {
		if user, err := repos.User().GetUser(ctx.Context(), claims.GetUsername()); err == nil && user != nil {
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
func AdminOnlyMiddleware(repos core.RepositoryStorage) apptheory.Middleware {
	return createRBACMiddleware(PermissionAdmin, repos)
}

// ModeratorOrHigherMiddleware creates middleware that requires moderator or admin permissions
func ModeratorOrHigherMiddleware(repos core.RepositoryStorage) apptheory.Middleware {
	return createRBACMiddleware(PermissionModerator, repos)
}

// ViewerOrHigherMiddleware creates middleware that requires viewer, moderator, or admin permissions
func ViewerOrHigherMiddleware(repos core.RepositoryStorage) apptheory.Middleware {
	return createRBACMiddleware(PermissionViewer, repos)
}
