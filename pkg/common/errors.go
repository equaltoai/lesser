package common

import (
	"fmt"

	"go.uber.org/zap"
)

// Domain-specific error types

// ActorNotFoundError indicates an actor was not found
type ActorNotFoundError struct {
	Username string
}

func (e ActorNotFoundError) Error() string {
	return fmt.Sprintf("actor not found: %s", e.Username)
}

// ActivityNotFoundError indicates an activity was not found
type ActivityNotFoundError struct {
	ID string
}

func (e ActivityNotFoundError) Error() string {
	return fmt.Sprintf("activity not found: %s", e.ID)
}

// ValidationError indicates input validation failed
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

// AuthenticationError indicates authentication failed
type AuthenticationError struct {
	Message string
}

func (e AuthenticationError) Error() string {
	return fmt.Sprintf("authentication failed: %s", e.Message)
}

// AuthorizationError indicates authorization failed
type AuthorizationError struct {
	Action   string
	Resource string
}

func (e AuthorizationError) Error() string {
	return fmt.Sprintf("not authorized to %s %s", e.Action, e.Resource)
}

// ConflictError indicates a resource conflict
type ConflictError struct {
	Resource string
	Message  string
}

func (e ConflictError) Error() string {
	return fmt.Sprintf("conflict with %s: %s", e.Resource, e.Message)
}

// FederationError indicates a federation operation failed
type FederationError struct {
	Operation string
	Remote    string
	Err       error
}

func (e FederationError) Error() string {
	return fmt.Sprintf("federation %s failed for %s: %v", e.Operation, e.Remote, e.Err)
}

func (e FederationError) Unwrap() error {
	return e.Err
}

// AppError represents a safe application error that separates internal details from user messages
type AppError struct {
	Code          string // Internal error code for logging/monitoring
	UserMessage   string // Safe message for users
	InternalError error  // Detailed error for logging
	StatusCode    int    // HTTP status code
}

// Error implements the error interface
func (e AppError) Error() string {
	return e.UserMessage
}

// ErrUnauthorized creates an unauthorized error
func ErrUnauthorized(internal error) AppError {
	return AppError{
		Code:          "AUTH_FAILED",
		UserMessage:   "Authentication failed",
		InternalError: internal,
		StatusCode:    401,
	}
}

// ErrNotFound creates a not found error
func ErrNotFound(resource string) AppError {
	return AppError{
		Code:          "NOT_FOUND",
		UserMessage:   "Resource not found",
		InternalError: fmt.Errorf("%s not found", resource),
		StatusCode:    404,
	}
}

// ErrForbidden creates a forbidden error
func ErrForbidden(internal error) AppError {
	return AppError{
		Code:          "FORBIDDEN",
		UserMessage:   "Access denied",
		InternalError: internal,
		StatusCode:    403,
	}
}

// ErrBadRequest creates a bad request error
func ErrBadRequest(userMessage string, internal error) AppError {
	return AppError{
		Code:          "BAD_REQUEST",
		UserMessage:   userMessage,
		InternalError: internal,
		StatusCode:    400,
	}
}

// ErrInternal creates an internal server error
func ErrInternal(internal error) AppError {
	return AppError{
		Code:          "INTERNAL_ERROR",
		UserMessage:   "An error occurred processing your request",
		InternalError: internal,
		StatusCode:    500,
	}
}

// ErrValidation creates a validation error
func ErrValidation(field string, message string) AppError {
	return AppError{
		Code:          "VALIDATION_ERROR",
		UserMessage:   message,
		InternalError: fmt.Errorf("validation failed for field %s: %s", field, message),
		StatusCode:    400,
	}
}

// HandleError processes an error and returns safe HTTP response values
func HandleError(logger *zap.Logger, err error) (int, string) {
	if appErr, ok := err.(AppError); ok {
		// Log the internal details
		logger.Error("Request failed",
			zap.String("code", appErr.Code),
			zap.Error(appErr.InternalError),
			zap.Int("status", appErr.StatusCode))

		// Return safe user message
		return appErr.StatusCode, fmt.Sprintf(`{"error": "%s", "code": "%s"}`, appErr.UserMessage, appErr.Code)
	}

	// Unknown errors get generic message
	logger.Error("Unexpected error", zap.Error(err))
	return 500, `{"error": "An error occurred processing your request", "code": "INTERNAL_ERROR"}`
}

// WrapError wraps an error with context while keeping it safe for users
func WrapError(err error, context string) error {
	if appErr, ok := err.(AppError); ok {
		// Already an AppError, add context to internal error
		appErr.InternalError = fmt.Errorf("%s: %w", context, appErr.InternalError)
		return appErr
	}

	// Regular error, wrap as internal
	return ErrInternal(fmt.Errorf("%s: %w", context, err))
}

// Error checking functions

// IsNotFound returns true if the error is a not found error
func IsNotFound(err error) bool {
	switch err.(type) {
	case ActorNotFoundError, ActivityNotFoundError:
		return true
	}
	return false
}

// IsValidation returns true if the error is a validation error
func IsValidation(err error) bool {
	_, ok := err.(ValidationError)
	return ok
}

// IsAuthentication returns true if the error is an authentication error
func IsAuthentication(err error) bool {
	_, ok := err.(AuthenticationError)
	return ok
}

// IsAuthorization returns true if the error is an authorization error
func IsAuthorization(err error) bool {
	_, ok := err.(AuthorizationError)
	return ok
}

// IsConflict returns true if the error is a conflict error
func IsConflict(err error) bool {
	_, ok := err.(ConflictError)
	return ok
}

// IsFederation returns true if the error is a federation error
func IsFederation(err error) bool {
	_, ok := err.(FederationError)
	return ok
}
