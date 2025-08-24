// Package lambda provides standardized error handling patterns for Lambda functions.
package lambda

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// ErrorPattern provides standardized error handling for Lambda functions
type ErrorPattern struct {
	logger *zap.Logger
}

// NewErrorPattern creates a new standardized error handling pattern
func NewErrorPattern(logger *zap.Logger) *ErrorPattern {
	return &ErrorPattern{
		logger: logger,
	}
}

// StandardErrorResponse represents the standard error response format
type StandardErrorResponse struct {
	Error     string                 `json:"error"`
	Message   string                 `json:"message,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Code      string                 `json:"code,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Timestamp string                 `json:"timestamp,omitempty"`
}

// CreateErrorHandlingMiddleware creates standardized error handling middleware
// This eliminates the 20+ line duplication across Lambda error handlers
func (ep *ErrorPattern) CreateErrorHandlingMiddleware() liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			err := next.Handle(ctx)
			if err != nil {
				return ep.handleError(ctx, err)
			}
			return nil
		})
	}
}

// handleError processes errors and returns standardized error responses
func (ep *ErrorPattern) handleError(ctx *liftPkg.Context, err error) error {
	requestID := ctx.GetRequestID()

	// Handle Lift errors (already formatted)
	if liftErr, ok := err.(*liftPkg.LiftError); ok {
		ep.logError(requestID, liftErr.StatusCode, liftErr.Message, liftErr)
		return liftErr // Let Lift handle its own errors
	}

	// Handle common error types
	statusCode, errorCode, message := ep.categorizeError(err)

	// Create standardized error response
	errorResponse := StandardErrorResponse{
		Error:     errorCode,
		Message:   message,
		RequestID: requestID,
		Code:      errorCode,
	}

	// Log the error
	ep.logError(requestID, statusCode, message, err)

	// Return as Lift error
	return ctx.Status(statusCode).JSON(errorResponse)
}

// categorizeError categorizes errors into standard types and status codes
func (ep *ErrorPattern) categorizeError(err error) (statusCode int, errorCode string, message string) {
	errMsg := err.Error()
	errMsgLower := strings.ToLower(errMsg)

	// Authentication errors
	if strings.Contains(errMsgLower, "unauthorized") ||
		strings.Contains(errMsgLower, "invalid token") ||
		strings.Contains(errMsgLower, "authentication required") {
		return http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required"
	}

	// Authorization errors
	if strings.Contains(errMsgLower, "forbidden") ||
		strings.Contains(errMsgLower, "insufficient") ||
		strings.Contains(errMsgLower, "access denied") {
		return http.StatusForbidden, "FORBIDDEN", "Access denied"
	}

	// Validation errors
	if strings.Contains(errMsgLower, "invalid") ||
		strings.Contains(errMsgLower, "required") ||
		strings.Contains(errMsgLower, "validation") ||
		strings.Contains(errMsgLower, "bad request") {
		return http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request data"
	}

	// Not found errors
	if strings.Contains(errMsgLower, "not found") ||
		strings.Contains(errMsgLower, "does not exist") {
		return http.StatusNotFound, "NOT_FOUND", "Resource not found"
	}

	// Conflict errors
	if strings.Contains(errMsgLower, "conflict") ||
		strings.Contains(errMsgLower, "already exists") ||
		strings.Contains(errMsgLower, "duplicate") {
		return http.StatusConflict, "CONFLICT", "Resource conflict"
	}

	// Rate limiting errors
	if strings.Contains(errMsgLower, "rate limit") ||
		strings.Contains(errMsgLower, "too many requests") {
		return http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests"
	}

	// Timeout errors
	if strings.Contains(errMsgLower, "timeout") ||
		strings.Contains(errMsgLower, "deadline exceeded") {
		return http.StatusRequestTimeout, "TIMEOUT", "Request timeout"
	}

	// Default to internal server error
	return http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error"
}

// logError logs errors with appropriate levels and context
func (ep *ErrorPattern) logError(requestID string, statusCode int, message string, err error) {
	fields := []zap.Field{
		zap.String("request_id", requestID),
		zap.Int("status_code", statusCode),
		zap.String("message", message),
		zap.Error(err),
	}

	// Log level based on status code
	if statusCode >= 500 {
		ep.logger.Error("server error", fields...)
	} else if statusCode >= 400 {
		ep.logger.Warn("client error", fields...)
	} else {
		ep.logger.Info("handled error", fields...)
	}
}

// HandleValidationError creates a standardized validation error response
func (ep *ErrorPattern) HandleValidationError(ctx *liftPkg.Context, field string, message string) error {
	errorResponse := StandardErrorResponse{
		Error:     "VALIDATION_ERROR",
		Message:   fmt.Sprintf("Validation failed for field '%s': %s", field, message),
		Code:      "VALIDATION_ERROR",
		RequestID: ctx.GetRequestID(),
		Details: map[string]interface{}{
			"field":              field,
			"validation_message": message,
		},
	}

	ep.logger.Warn("validation error",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("field", field),
		zap.String("message", message),
	)

	return ctx.Status(http.StatusBadRequest).JSON(errorResponse)
}

// HandleAuthenticationError creates a standardized authentication error response
func (ep *ErrorPattern) HandleAuthenticationError(ctx *liftPkg.Context, message string) error {
	errorResponse := StandardErrorResponse{
		Error:     "UNAUTHORIZED",
		Message:   message,
		Code:      "UNAUTHORIZED",
		RequestID: ctx.GetRequestID(),
	}

	ep.logger.Warn("authentication error",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("message", message),
	)

	return ctx.Status(http.StatusUnauthorized).JSON(errorResponse)
}

// HandleAuthorizationError creates a standardized authorization error response
func (ep *ErrorPattern) HandleAuthorizationError(ctx *liftPkg.Context, message string) error {
	errorResponse := StandardErrorResponse{
		Error:     "FORBIDDEN",
		Message:   message,
		Code:      "FORBIDDEN",
		RequestID: ctx.GetRequestID(),
	}

	ep.logger.Warn("authorization error",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("message", message),
	)

	return ctx.Status(http.StatusForbidden).JSON(errorResponse)
}

// HandleNotFoundError creates a standardized not found error response
func (ep *ErrorPattern) HandleNotFoundError(ctx *liftPkg.Context, resource string) error {
	errorResponse := StandardErrorResponse{
		Error:     "NOT_FOUND",
		Message:   fmt.Sprintf("%s not found", resource),
		Code:      "NOT_FOUND",
		RequestID: ctx.GetRequestID(),
		Details: map[string]interface{}{
			"resource": resource,
		},
	}

	ep.logger.Info("resource not found",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("resource", resource),
	)

	return ctx.Status(http.StatusNotFound).JSON(errorResponse)
}

// HandleInternalError creates a standardized internal server error response
func (ep *ErrorPattern) HandleInternalError(ctx *liftPkg.Context, err error, message string) error {
	errorResponse := StandardErrorResponse{
		Error:     "INTERNAL_ERROR",
		Message:   message,
		Code:      "INTERNAL_ERROR",
		RequestID: ctx.GetRequestID(),
	}

	ep.logger.Error("internal server error",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("message", message),
		zap.Error(err),
	)

	return ctx.Status(http.StatusInternalServerError).JSON(errorResponse)
}

// HandleRateLimitError creates a standardized rate limit error response
func (ep *ErrorPattern) HandleRateLimitError(ctx *liftPkg.Context, retryAfter int) error {
	errorResponse := StandardErrorResponse{
		Error:     "RATE_LIMITED",
		Message:   "Too many requests",
		Code:      "RATE_LIMITED",
		RequestID: ctx.GetRequestID(),
		Details: map[string]interface{}{
			"retry_after_seconds": retryAfter,
		},
	}

	// Set Retry-After header
	ctx.Response.Header("Retry-After", fmt.Sprintf("%d", retryAfter))

	ep.logger.Warn("rate limit exceeded",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("retry_after", retryAfter),
	)

	return ctx.Status(http.StatusTooManyRequests).JSON(errorResponse)
}

// WrapWithErrorHandler wraps a handler function with standardized error handling
func (ep *ErrorPattern) WrapWithErrorHandler(handler func(*liftPkg.Context) error) liftPkg.HandlerFunc {
	return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
		err := handler(ctx)
		if err != nil {
			return ep.handleError(ctx, err)
		}
		return nil
	})
}

// CreatePanicRecoveryMiddleware creates middleware to recover from panics
func (ep *ErrorPattern) CreatePanicRecoveryMiddleware() liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					ep.logger.Error("panic recovered",
						zap.String("request_id", ctx.GetRequestID()),
						zap.Any("panic", r),
					)

					errorResponse := StandardErrorResponse{
						Error:     "INTERNAL_ERROR",
						Message:   "Internal server error",
						Code:      "PANIC_RECOVERED",
						RequestID: ctx.GetRequestID(),
					}

					err = ctx.Status(http.StatusInternalServerError).JSON(errorResponse)
				}
			}()

			return next.Handle(ctx)
		})
	}
}

// ValidateRequiredParam validates a required parameter and returns standardized error
func (ep *ErrorPattern) ValidateRequiredParam(ctx *liftPkg.Context, paramName string, paramValue string) error {
	if err := common.ValidateRequiredParam(paramName, paramValue); err != nil {
		return ep.HandleValidationError(ctx, paramName, "parameter is required")
	}
	return nil
}

// ActivityPubErrorResponse represents ActivityPub-specific error responses
type ActivityPubErrorResponse struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// HandleActivityPubError creates ActivityPub-compatible error responses
func (ep *ErrorPattern) HandleActivityPubError(ctx *liftPkg.Context, statusCode int, errorType string, summary string) error {
	// Check if client accepts ActivityPub content type
	acceptHeader := ctx.Header("Accept")
	if strings.Contains(acceptHeader, "application/activity+json") ||
		strings.Contains(acceptHeader, "application/ld+json") {

		// Return ActivityPub-formatted error
		errorResponse := ActivityPubErrorResponse{
			Type:    errorType,
			Name:    http.StatusText(statusCode),
			Summary: summary,
		}

		ctx.Response.Header("Content-Type", "application/activity+json")
		return ctx.Status(statusCode).JSON(errorResponse)
	}

	// Return standard JSON error for non-ActivityPub clients
	errorResponse := StandardErrorResponse{
		Error:     errorType,
		Message:   summary,
		Code:      errorType,
		RequestID: ctx.GetRequestID(),
	}

	return ctx.Status(statusCode).JSON(errorResponse)
}
