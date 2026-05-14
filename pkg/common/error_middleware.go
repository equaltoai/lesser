package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"context"
	stdErrors "errors"
	"net"
	"runtime"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/logging"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// ErrorMiddlewareConfig holds configuration for error handling middleware
type ErrorMiddlewareConfig struct {
	Logger              *zap.Logger
	ServiceName         string
	EnableStackTrace    bool
	EnablePanicRecovery bool
	EnableErrorMetrics  bool
	MaxErrorLogLength   int
}

// DefaultErrorConfig returns a default error middleware configuration
func DefaultErrorConfig(serviceName string, logger *zap.Logger) ErrorMiddlewareConfig {
	cfg := config.Get()
	return ErrorMiddlewareConfig{
		Logger:              logger,
		ServiceName:         serviceName,
		EnableStackTrace:    cfg.DebugMode,
		EnablePanicRecovery: true,
		EnableErrorMetrics:  true,
		MaxErrorLogLength:   2000,
	}
}

// ErrorHandlingMiddleware creates centralized error handling middleware for Lift
func ErrorHandlingMiddleware(config ErrorMiddlewareConfig) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (resp *apptheory.Response, err error) {
			// Add panic recovery if enabled
			if config.EnablePanicRecovery {
				defer func() {
					if r := recover(); r != nil {
						// Capture stack trace for panics
						buf := make([]byte, 4096)
						buf = buf[:runtime.Stack(buf, false)]

						config.Logger.Error("panic recovered in error middleware",
							zap.String("service", config.ServiceName),
							zap.String("path", sanitizedRequestLogPath(ctx)),
							zap.String("method", ctx.Request.Method),
							zap.Any("panic", r),
							zap.String("stack", string(buf)))

						// Return generic internal server error
						resp, err = RespondInternalServerError(ctx, "Internal server error")
					}
				}()
			}

			// Process the request
			resp, err = next(ctx)

			// Handle any errors returned by handlers
			if err != nil {
				handled, handleErr := handleRequestError(ctx, err, config)
				if handleErr != nil {
					return nil, handleErr
				}
				return handled, nil
			}

			return resp, nil
		}
	}
}

// handleRequestError processes errors from request handlers using centralized patterns
func handleRequestError(ctx *apptheory.Context, err error, config ErrorMiddlewareConfig) (*apptheory.Response, error) {
	if portableErr, ok := apptheory.AsAppTheoryError(err); ok {
		status := portableErr.StatusCode
		if status == 0 {
			status = appTheoryStatusForErrorCode(portableErr.Code)
		}
		return apptheory.JSON(status, StandardErrorResponse{
			Error: portableErr.Message,
			Code:  errorCodeForHTTPStatus(status),
		})
	}

	// AppTheory runtime errors (timeouts, rate limits, etc.) should be preserved.
	var atErr *apptheory.AppError
	if stdErrors.As(err, &atErr) {
		status := appTheoryStatusForErrorCode(atErr.Code)
		return apptheory.JSON(status, StandardErrorResponse{
			Error: atErr.Message,
			Code:  errorCodeForHTTPStatus(status),
		})
	}

	// Check if it's already a centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		return handleAppError(ctx, appErr, config)
	}

	// Check for common error patterns and convert to centralized AppError
	var appErr *errors.AppError
	switch {
	case IsNotFound(err):
		appErr = errors.NotFound("resource")
	case IsValidation(err):
		appErr = errors.ValidationFailed("input", err.Error())
	case IsAuthentication(err):
		appErr = errors.Unauthorized(err.Error())
	case IsAuthorization(err):
		appErr = errors.Forbidden(err.Error())
	case IsConflict(err):
		appErr = errors.NewAppError(errors.CodeConflict, errors.CategoryBusiness, err.Error())
	case IsFederation(err):
		appErr = errors.NewFederationInternalError(errors.CodeExternalServiceUnavailable, "Federation service unavailable", err)
	default:
		// Unknown error - wrap as internal
		appErr = errors.InternalWithCause(err, "An error occurred processing your request")
	}

	return handleAppError(ctx, appErr, config)
}

func appTheoryStatusForErrorCode(code string) int {
	switch code {
	case "app.bad_request", "app.validation_failed":
		return 400
	case "app.unauthorized":
		return 401
	case "app.forbidden":
		return 403
	case "app.not_found":
		return 404
	case "app.method_not_allowed":
		return 405
	case "app.conflict":
		return 409
	case "app.too_large":
		return 413
	case "app.timeout":
		return 408
	case "app.rate_limited":
		return 429
	case "app.overloaded":
		return 503
	case "app.internal":
		return 500
	default:
		return 500
	}
}

func sanitizedRequestLogPath(ctx *apptheory.Context) string {
	if ctx == nil {
		return ""
	}
	return logging.SanitizeLogPath(ctx.Request.Path)
}

// handleAppError processes AppError instances with safe user message handling
func handleAppError(ctx *apptheory.Context, appErr *errors.AppError, config ErrorMiddlewareConfig) (*apptheory.Response, error) {
	path := ""
	method := ""
	requestID := ""
	var username any
	if ctx != nil {
		path = sanitizedRequestLogPath(ctx)
		method = ctx.Request.Method
		requestID = ctx.RequestID
		username = ctx.Get("username")
	}

	// Log the internal error details
	logFields := []zap.Field{
		zap.String("service", config.ServiceName),
		zap.String("error_code", string(appErr.Code)),
		zap.String("path", path),
		zap.String("method", method),
		zap.Int("status_code", appErr.HTTPStatusCode),
	}

	// Add user context if available
	if username != nil {
		logFields = append(logFields, zap.Any("username", username))
	}

	if requestID != "" {
		logFields = append(logFields, zap.String("request_id", requestID))
	}

	// Log internal error (truncated if too long)
	internalMsg := ""
	if appErr.InternalError != nil {
		internalMsg = appErr.InternalError.Error()
	} else if appErr.InternalMessage != "" {
		internalMsg = appErr.InternalMessage
	}

	if config.MaxErrorLogLength > 0 && len(internalMsg) > config.MaxErrorLogLength {
		internalMsg = internalMsg[:config.MaxErrorLogLength] + "... (truncated)"
	}
	logFields = append(logFields, zap.String("internal_error", internalMsg))

	// Add stack trace in debug mode
	if config.EnableStackTrace {
		buf := make([]byte, 4096)
		buf = buf[:runtime.Stack(buf, false)]
		logFields = append(logFields, zap.String("stack_trace", string(buf)))
	}

	// Log at appropriate level based on status code
	switch {
	case appErr.HTTPStatusCode >= 500:
		config.Logger.Error("server error", logFields...)
	case appErr.HTTPStatusCode >= 400:
		config.Logger.Warn("client error", logFields...)
	default:
		config.Logger.Info("handled error", logFields...)
	}

	// Emit error metrics if enabled
	if config.EnableErrorMetrics {
		emitErrorMetrics(appErr, config)
	}

	// Return safe response to client
	return apptheory.JSON(appErr.HTTPStatusCode, StandardErrorResponse{
		Error: appErr.Message,
		Code:  string(appErr.Code),
	})
}

// emitErrorMetrics emits error metrics for monitoring and alerting
func emitErrorMetrics(appErr *errors.AppError, config ErrorMiddlewareConfig) {
	// This would integrate with your metrics system (EMF, CloudWatch, etc.)
	// For now, we'll log structured metrics that can be picked up by log processors
	config.Logger.Info("error_metric",
		zap.String("metric_type", "error_count"),
		zap.String("service", config.ServiceName),
		zap.String("error_code", string(appErr.Code)),
		zap.String("error_category", string(appErr.Category)),
		zap.Int("status_code", appErr.HTTPStatusCode),
		zap.Bool("retryable", appErr.Retryable),
		zap.Time("timestamp", time.Now()),
		zap.String("metric_namespace", "lesser/errors"))
}

// ValidationErrorMiddleware creates middleware specifically for validation error handling
func ValidationErrorMiddleware(serviceName string, logger *zap.Logger) apptheory.Middleware {
	config := DefaultErrorConfig(serviceName, logger)
	config.EnableStackTrace = false // Don't need stack traces for validation errors

	return ErrorHandlingMiddleware(config)
}

// ProductionErrorMiddleware creates error middleware optimized for production
func ProductionErrorMiddleware(serviceName string, logger *zap.Logger) apptheory.Middleware {
	config := DefaultErrorConfig(serviceName, logger)
	config.EnableStackTrace = false
	config.EnablePanicRecovery = true
	config.EnableErrorMetrics = true
	config.MaxErrorLogLength = 1000 // Shorter logs in production

	return ErrorHandlingMiddleware(config)
}

// DevelopmentErrorMiddleware creates error middleware optimized for development
func DevelopmentErrorMiddleware(serviceName string, logger *zap.Logger) apptheory.Middleware {
	config := DefaultErrorConfig(serviceName, logger)
	config.EnableStackTrace = true
	config.EnablePanicRecovery = true
	config.EnableErrorMetrics = false
	config.MaxErrorLogLength = 5000 // Longer logs in development

	return ErrorHandlingMiddleware(config)
}

// Helper middleware for specific error handling patterns

// NotFoundMiddleware creates middleware that handles 404 errors gracefully
func NotFoundMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			resp, err := next(ctx)
			if err == nil && resp == nil {
				return RespondNotFound(ctx)
			}
			return resp, err
		}
	}
}

// TimeoutErrorMiddleware creates middleware that handles timeout errors
func TimeoutErrorMiddleware(serviceName string, logger *zap.Logger) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			resp, err := next(ctx)
			if err != nil && isTimeoutError(err) {
				logger.Warn("request timeout",
					zap.String("service", serviceName),
					zap.String("path", sanitizedRequestLogPath(ctx)),
					zap.Error(err))

				return RespondServiceUnavailable(ctx, "request timeout")
			}

			return resp, err
		}
	}
}

// isTimeoutError checks if an error is a timeout error
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Standard library context timeouts.
	if stdErrors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Network timeouts (e.g., net/http).
	var netErr net.Error
	if stdErrors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Centralized error codes.
	return errors.HasCode(err, errors.CodeTimeout) ||
		errors.HasCode(err, errors.CodeLambdaTimeout) ||
		errors.HasCode(err, errors.CodeStreamingTimeout) ||
		errors.HasCode(err, errors.CodeExternalServiceTimeout)
}

// ErrorRecoveryMiddleware provides graceful degradation for critical errors
func ErrorRecoveryMiddleware(serviceName string, logger *zap.Logger) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			resp, err := next(ctx)
			if err == nil {
				return resp, nil
			}

			recovered, recoveredResp, recoveredErr := attemptErrorRecovery(ctx, err, serviceName, logger)
			if recoveredErr != nil {
				return nil, recoveredErr
			}
			if recovered {
				return recoveredResp, nil
			}

			return resp, err
		}
	}
}

// attemptErrorRecovery attempts to recover from specific types of errors
func attemptErrorRecovery(ctx *apptheory.Context, err error, serviceName string, logger *zap.Logger) (bool, *apptheory.Response, error) {
	switch {
	case isTemporaryError(err):
		logger.Info("temporary error - returning degraded response",
			zap.String("service", serviceName),
			zap.Error(err))
		resp, respErr := RespondServiceUnavailable(ctx, "temporary service issue")
		return true, resp, respErr

	case isRetryableError(err):
		logger.Info("retryable error detected",
			zap.String("service", serviceName),
			zap.Error(err))
		// Could implement retry logic here
		resp, respErr := RespondServiceUnavailable(ctx, "please retry")
		return true, resp, respErr

	default:
		return false, nil, nil // No recovery possible
	}
}

// isTemporaryError checks if an error is temporary
func isTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	// Common temporary pattern used by net.Error and similar interfaces.
	type temporary interface {
		Temporary() bool
	}
	var te temporary
	if stdErrors.As(err, &te) {
		return te.Temporary()
	}
	return false
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Centralized retryable flag for AppError.
	if errors.IsRetryable(err) {
		return true
	}

	// Generic retryable interface.
	type retryable interface {
		Retryable() bool
	}
	var re retryable
	if stdErrors.As(err, &re) {
		return re.Retryable()
	}
	return false
}

// CreateStandardErrorMiddleware creates the standard error handling middleware stack
func CreateStandardErrorMiddleware(serviceName string, logger *zap.Logger) apptheory.Middleware {
	cfg := config.Get()
	if cfg.Environment == "production" {
		return ProductionErrorMiddleware(serviceName, logger)
	}
	return DevelopmentErrorMiddleware(serviceName, logger)
}

// CreateAPIErrorMiddleware creates error middleware specifically for API services
func CreateAPIErrorMiddleware(logger *zap.Logger) apptheory.Middleware {
	return CreateStandardErrorMiddleware("api", logger)
}

// CreateGraphQLErrorMiddleware creates error middleware specifically for GraphQL services
func CreateGraphQLErrorMiddleware(logger *zap.Logger) apptheory.Middleware {
	return CreateStandardErrorMiddleware("graphql", logger)
}

// CreateFederationErrorMiddleware creates error middleware specifically for federation services
func CreateFederationErrorMiddleware(logger *zap.Logger) apptheory.Middleware {
	return CreateStandardErrorMiddleware("federation", logger)
}
