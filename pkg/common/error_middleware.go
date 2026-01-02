package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"context"
	stdErrors "errors"
	"net"
	"runtime"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/pay-theory/lift/pkg/lift"
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
func ErrorHandlingMiddleware(config ErrorMiddlewareConfig) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) (err error) {
			// Add panic recovery if enabled
			if config.EnablePanicRecovery {
				defer func() {
					if r := recover(); r != nil {
						// Capture stack trace for panics
						buf := make([]byte, 4096)
						buf = buf[:runtime.Stack(buf, false)]

						config.Logger.Error("panic recovered in error middleware",
							zap.String("service", config.ServiceName),
							zap.String("path", ctx.Request.Path),
							zap.String("method", ctx.Request.Method),
							zap.Any("panic", r),
							zap.String("stack", string(buf)))

						// Return generic internal server error
						err = RespondInternalServerError(ctx, "Internal server error")
					}
				}()
			}

			// Process the request
			err = next.Handle(ctx)

			// Handle any errors returned by handlers
			if err != nil {
				handleRequestError(ctx, err, config)
				return nil
			}

			return err
		})
	}
}

// handleRequestError processes errors from request handlers using centralized patterns
func handleRequestError(ctx *lift.Context, err error, config ErrorMiddlewareConfig) {
	// Check if it's already a centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		handleAppError(ctx, appErr, config)
		return
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

	handleAppError(ctx, appErr, config)
}

// handleAppError processes AppError instances with safe user message handling
func handleAppError(ctx *lift.Context, appErr *errors.AppError, config ErrorMiddlewareConfig) {
	// Log the internal error details
	logFields := []zap.Field{
		zap.String("service", config.ServiceName),
		zap.String("error_code", string(appErr.Code)),
		zap.String("path", ctx.Request.Path),
		zap.String("method", ctx.Request.Method),
		zap.Int("status_code", appErr.HTTPStatusCode),
	}

	// Add user context if available
	if username := ctx.Get("username"); username != nil {
		logFields = append(logFields, zap.Any("username", username))
	}

	if requestID := ctx.Get("request_id"); requestID != nil {
		logFields = append(logFields, zap.Any("request_id", requestID))
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
	_ = ctx.Status(appErr.HTTPStatusCode).JSON(StandardErrorResponse{
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
func ValidationErrorMiddleware(serviceName string, logger *zap.Logger) lift.Middleware {
	config := DefaultErrorConfig(serviceName, logger)
	config.EnableStackTrace = false // Don't need stack traces for validation errors

	return ErrorHandlingMiddleware(config)
}

// ProductionErrorMiddleware creates error middleware optimized for production
func ProductionErrorMiddleware(serviceName string, logger *zap.Logger) lift.Middleware {
	config := DefaultErrorConfig(serviceName, logger)
	config.EnableStackTrace = false
	config.EnablePanicRecovery = true
	config.EnableErrorMetrics = true
	config.MaxErrorLogLength = 1000 // Shorter logs in production

	return ErrorHandlingMiddleware(config)
}

// DevelopmentErrorMiddleware creates error middleware optimized for development
func DevelopmentErrorMiddleware(serviceName string, logger *zap.Logger) lift.Middleware {
	config := DefaultErrorConfig(serviceName, logger)
	config.EnableStackTrace = true
	config.EnablePanicRecovery = true
	config.EnableErrorMetrics = false
	config.MaxErrorLogLength = 5000 // Longer logs in development

	return ErrorHandlingMiddleware(config)
}

// Helper middleware for specific error handling patterns

// NotFoundMiddleware creates middleware that handles 404 errors gracefully
func NotFoundMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			err := next.Handle(ctx)

			// If no error but no response was set, it's likely a 404
			if err == nil && ctx.Response.StatusCode == 0 {
				return RespondNotFound(ctx)
			}

			return err
		})
	}
}

// TimeoutErrorMiddleware creates middleware that handles timeout errors
func TimeoutErrorMiddleware(serviceName string, logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			err := next.Handle(ctx)

			if err != nil && isTimeoutError(err) {
				logger.Warn("request timeout",
					zap.String("service", serviceName),
					zap.String("path", ctx.Request.Path),
					zap.Error(err))

				return RespondServiceUnavailable(ctx, "request timeout")
			}

			return err
		})
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
func ErrorRecoveryMiddleware(serviceName string, logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			err := next.Handle(ctx)

			if err != nil {
				// Attempt graceful recovery for specific error types
				recoveredErr := attemptErrorRecovery(ctx, err, serviceName, logger)
				if recoveredErr == nil {
					return nil
				}
				return recoveredErr
			}

			return err
		})
	}
}

// attemptErrorRecovery attempts to recover from specific types of errors
func attemptErrorRecovery(ctx *lift.Context, err error, serviceName string, logger *zap.Logger) error {
	switch {
	case isTemporaryError(err):
		logger.Info("temporary error - returning degraded response",
			zap.String("service", serviceName),
			zap.Error(err))
		return RespondServiceUnavailable(ctx, "temporary service issue")

	case isRetryableError(err):
		logger.Info("retryable error detected",
			zap.String("service", serviceName),
			zap.Error(err))
		// Could implement retry logic here
		return RespondServiceUnavailable(ctx, "please retry")

	default:
		return err // No recovery possible
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
func CreateStandardErrorMiddleware(serviceName string, logger *zap.Logger) lift.Middleware {
	cfg := config.Get()
	if cfg.Environment == "production" {
		return ProductionErrorMiddleware(serviceName, logger)
	}
	return DevelopmentErrorMiddleware(serviceName, logger)
}

// CreateAPIErrorMiddleware creates error middleware specifically for API services
func CreateAPIErrorMiddleware(logger *zap.Logger) lift.Middleware {
	return CreateStandardErrorMiddleware("api", logger)
}

// CreateGraphQLErrorMiddleware creates error middleware specifically for GraphQL services
func CreateGraphQLErrorMiddleware(logger *zap.Logger) lift.Middleware {
	return CreateStandardErrorMiddleware("graphql", logger)
}

// CreateFederationErrorMiddleware creates error middleware specifically for federation services
func CreateFederationErrorMiddleware(logger *zap.Logger) lift.Middleware {
	return CreateStandardErrorMiddleware("federation", logger)
}
