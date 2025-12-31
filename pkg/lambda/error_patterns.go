// Package lambda provides standardized error handling patterns for Lambda functions.
package lambda

import (
	stdErrors "errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/errors"
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
	ErrorCode string                 `json:"error_code,omitempty"`
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

	// Handle centralized AppError
	if appErr, ok := errors.AsAppError(err); ok {
		errorResponse := StandardErrorResponse{
			Error:     string(appErr.Code),
			Message:   appErr.Message,
			RequestID: requestID,
			Code:      string(appErr.Code),
			ErrorCode: string(appErr.Code),
		}
		ep.logError(requestID, appErr.HTTPStatusCode, appErr.Message, err)
		return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
	}

	// Handle legacy error types by converting to centralized system
	appErr := ep.convertLegacyError(err)
	errorResponse := StandardErrorResponse{
		Error:     string(appErr.Code),
		Message:   appErr.Message,
		RequestID: requestID,
		Code:      string(appErr.Code),
		ErrorCode: string(appErr.Code),
	}

	// Log the error
	ep.logError(requestID, appErr.HTTPStatusCode, appErr.Message, err)

	// Return as Lift error
	return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
}

// convertLegacyError converts legacy errors to centralized AppError
func (ep *ErrorPattern) convertLegacyError(err error) *errors.AppError {
	var legacy common.AppError
	if stdErrors.As(err, &legacy) {
		return ep.convertCommonAppError(legacy)
	}

	errMsg := err.Error()
	errMsgLower := strings.ToLower(errMsg)

	// Authentication errors
	if strings.Contains(errMsgLower, "unauthorized") ||
		strings.Contains(errMsgLower, "invalid token") ||
		strings.Contains(errMsgLower, "authentication required") {
		return errors.WrapError(err, errors.CodeUnauthorized, errors.CategoryAuth, "Authentication required")
	}

	// Authorization errors
	if strings.Contains(errMsgLower, "forbidden") ||
		strings.Contains(errMsgLower, "insufficient") ||
		strings.Contains(errMsgLower, "access denied") {
		return errors.WrapError(err, errors.CodeForbidden, errors.CategoryAuth, "Access denied")
	}

	// Validation errors
	if strings.Contains(errMsgLower, "invalid") ||
		strings.Contains(errMsgLower, "required") ||
		strings.Contains(errMsgLower, "validation") ||
		strings.Contains(errMsgLower, "bad request") {
		return errors.WrapError(err, errors.CodeValidationFailed, errors.CategoryValidation, "Invalid request data")
	}

	// Not found errors
	if strings.Contains(errMsgLower, "not found") ||
		strings.Contains(errMsgLower, "does not exist") {
		return errors.WrapError(err, errors.CodeNotFound, errors.CategoryAPI, "Resource not found")
	}

	// Conflict errors
	if strings.Contains(errMsgLower, "conflict") ||
		strings.Contains(errMsgLower, "already exists") ||
		strings.Contains(errMsgLower, "duplicate") {
		return errors.WrapError(err, errors.CodeConflict, errors.CategoryBusiness, "Resource conflict")
	}

	// Rate limiting errors
	if strings.Contains(errMsgLower, "rate limit") ||
		strings.Contains(errMsgLower, "too many requests") {
		return errors.WrapError(err, errors.CodeRateLimited, errors.CategoryAPI, "Too many requests").AsRetryable()
	}

	// Timeout errors
	if strings.Contains(errMsgLower, "timeout") ||
		strings.Contains(errMsgLower, "deadline exceeded") {
		return errors.WrapError(err, errors.CodeTimeout, errors.CategoryLambda, "Request timeout").AsRetryable()
	}

	// Lambda-specific errors
	if strings.Contains(errMsgLower, "lambda") {
		return errors.WrapError(err, errors.CodeLambdaTimeout, errors.CategoryLambda, "Lambda function error").AsRetryable()
	}

	// Default to internal server error
	return errors.WrapError(err, errors.CodeInternal, errors.CategoryInternal, "Internal server error")
}

func (ep *ErrorPattern) convertCommonAppError(err common.AppError) *errors.AppError {
	code := errors.ErrorCode(err.Code)
	category := errors.CategoryInternal

	switch err.StatusCode {
	case http.StatusUnauthorized:
		category = errors.CategoryAuth
		if err.Code == "" {
			code = errors.CodeUnauthorized
		}
	case http.StatusForbidden:
		category = errors.CategoryAuth
		if err.Code == "" {
			code = errors.CodeForbidden
		}
	case http.StatusNotFound:
		category = errors.CategoryAPI
		if err.Code == "" {
			code = errors.CodeNotFound
		}
	case http.StatusBadRequest:
		category = errors.CategoryValidation
		// common.AppError uses VALIDATION_ERROR which isn't a canonical errors.ErrorCode.
		if err.Code == "VALIDATION_ERROR" || err.Code == "" {
			code = errors.CodeValidationFailed
		}
	}

	// Fall back to internal if we can't map to a stable status code.
	if code.GetHTTPStatusCode() != err.StatusCode {
		code = errors.CodeInternal
		category = errors.CategoryInternal
	}

	appErr := errors.NewAppError(code, category, err.UserMessage)
	if err.InternalError != nil {
		appErr = appErr.WithInternalError(err.InternalError)
	}
	return appErr
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
	appErr := errors.ValidationFailed(field, message)
	errorResponse := StandardErrorResponse{
		Error:     string(appErr.Code),
		Message:   appErr.Message,
		Code:      string(appErr.Code),
		ErrorCode: string(appErr.Code),
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

	return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
}

// HandleAuthenticationError creates a standardized authentication error response
func (ep *ErrorPattern) HandleAuthenticationError(ctx *liftPkg.Context, message string) error {
	appErr := errors.Unauthorized(message)
	errorResponse := StandardErrorResponse{
		Error:     string(appErr.Code),
		Message:   appErr.Message,
		Code:      string(appErr.Code),
		ErrorCode: string(appErr.Code),
		RequestID: ctx.GetRequestID(),
	}

	ep.logger.Warn("authentication error",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("message", message),
	)

	return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
}

// HandleAuthorizationError creates a standardized authorization error response
func (ep *ErrorPattern) HandleAuthorizationError(ctx *liftPkg.Context, message string) error {
	appErr := errors.Forbidden(message)
	errorResponse := StandardErrorResponse{
		Error:     string(appErr.Code),
		Message:   appErr.Message,
		Code:      string(appErr.Code),
		ErrorCode: string(appErr.Code),
		RequestID: ctx.GetRequestID(),
	}

	ep.logger.Warn("authorization error",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("message", message),
	)

	return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
}

// HandleNotFoundError creates a standardized not found error response
func (ep *ErrorPattern) HandleNotFoundError(ctx *liftPkg.Context, resource string) error {
	appErr := errors.NotFound(resource)
	errorResponse := StandardErrorResponse{
		Error:     string(appErr.Code),
		Message:   appErr.Message,
		Code:      string(appErr.Code),
		ErrorCode: string(appErr.Code),
		RequestID: ctx.GetRequestID(),
		Details: map[string]interface{}{
			"resource": resource,
		},
	}

	ep.logger.Info("resource not found",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("resource", resource),
	)

	return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
}

// HandleInternalError creates a standardized internal server error response
func (ep *ErrorPattern) HandleInternalError(ctx *liftPkg.Context, err error, message string) error {
	appErr := errors.InternalWithCause(err, message)
	errorResponse := StandardErrorResponse{
		Error:     string(appErr.Code),
		Message:   appErr.Message,
		Code:      string(appErr.Code),
		ErrorCode: string(appErr.Code),
		RequestID: ctx.GetRequestID(),
	}

	ep.logger.Error("internal server error",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("message", message),
		zap.Error(err),
	)

	return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
}

// HandleRateLimitError creates a standardized rate limit error response
func (ep *ErrorPattern) HandleRateLimitError(ctx *liftPkg.Context, retryAfter int) error {
	appErr := errors.NewAppError(errors.CodeRateLimited, errors.CategoryAuth, "Too many requests").
		WithMetadata("retry_after_seconds", retryAfter)
	errorResponse := StandardErrorResponse{
		Error:     string(appErr.Code),
		Message:   appErr.Message,
		Code:      string(appErr.Code),
		ErrorCode: string(appErr.Code),
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

	return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
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
						ErrorCode: "PANIC_RECOVERED",
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
	if paramValue == "" {
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
func (ep *ErrorPattern) HandleActivityPubError(ctx *liftPkg.Context, appErr *errors.AppError, summary string) error {
	// Check if client accepts ActivityPub content type
	acceptHeader := ctx.Header("Accept")
	if strings.Contains(acceptHeader, "application/activity+json") ||
		strings.Contains(acceptHeader, "application/ld+json") {

		// Return ActivityPub-formatted error
		errorResponse := ActivityPubErrorResponse{
			Type:    string(appErr.Code),
			Name:    http.StatusText(appErr.HTTPStatusCode),
			Summary: summary,
		}

		ctx.Response.Header("Content-Type", "application/activity+json")
		return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
	}

	// Return standard JSON error for non-ActivityPub clients
	errorResponse := StandardErrorResponse{
		Error:     string(appErr.Code),
		Message:   appErr.Message,
		Code:      string(appErr.Code),
		ErrorCode: string(appErr.Code),
		RequestID: ctx.GetRequestID(),
	}

	return ctx.Status(appErr.HTTPStatusCode).JSON(errorResponse)
}
