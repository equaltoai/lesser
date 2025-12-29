package errors

import (
	stdErrors "errors"
	"fmt"
	"time"
)

// AppError represents a comprehensive application error that separates
// internal details from user-facing messages and provides structured
// information for logging, monitoring, and debugging.
type AppError struct {
	// Code is the standardized error code for programmatic handling
	Code ErrorCode `json:"code"`

	// Category is the domain/category this error belongs to
	Category ErrorCategory `json:"category"`

	// Message is the user-friendly error message
	Message string `json:"message"`

	// InternalMessage contains detailed technical information for debugging
	InternalMessage string `json:"-"`

	// InternalError is the underlying error that caused this AppError
	InternalError error `json:"-"`

	// Metadata contains additional structured information about the error
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// HTTPStatusCode is the appropriate HTTP status code for this error
	HTTPStatusCode int `json:"-"`

	// Timestamp records when this error was created
	Timestamp time.Time `json:"timestamp"`

	// Retryable indicates whether this error represents a condition that might succeed on retry
	Retryable bool `json:"retryable"`

	// Stack contains the stack trace for debugging (only in development)
	Stack string `json:"-"`
}

// Error implements the standard error interface
func (e *AppError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("[%s:%s] %s", e.Category, e.Code, e.InternalMessage)
}

// Unwrap allows errors.Is and errors.As to work with the underlying error
func (e *AppError) Unwrap() error {
	return e.InternalError
}

// Clone creates a shallow copy of the error, including a copy of Metadata map,
// so callers can safely add context without mutating shared instances.
func (e *AppError) Clone() *AppError {
	if e == nil {
		return nil
	}

	clone := *e
	if e.Metadata != nil {
		clone.Metadata = make(map[string]interface{}, len(e.Metadata))
		for k, v := range e.Metadata {
			clone.Metadata[k] = v
		}
	}

	return &clone
}

// WithMetadata adds metadata to the error
func (e *AppError) WithMetadata(key string, value interface{}) *AppError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// WithInternalMessage adds or updates the internal message
func (e *AppError) WithInternalMessage(msg string) *AppError {
	e.InternalMessage = msg
	return e
}

// WithInternalError wraps another error as the internal cause
func (e *AppError) WithInternalError(err error) *AppError {
	e.InternalError = err
	return e
}

// AsRetryable marks this error as retryable
func (e *AppError) AsRetryable() *AppError {
	e.Retryable = true
	return e
}

// AsNonRetryable marks this error as non-retryable
func (e *AppError) AsNonRetryable() *AppError {
	e.Retryable = false
	return e
}

// NewAppError creates a new AppError with the specified code and category
func NewAppError(code ErrorCode, category ErrorCategory, message string) *AppError {
	return &AppError{
		Code:           code,
		Category:       category,
		Message:        message,
		HTTPStatusCode: code.GetHTTPStatusCode(),
		Timestamp:      time.Now(),
		Retryable:      false,
		Metadata:       make(map[string]interface{}),
	}
}

// NewAppErrorf creates a new AppError with formatted message
func NewAppErrorf(code ErrorCode, category ErrorCategory, format string, args ...interface{}) *AppError {
	return NewAppError(code, category, fmt.Sprintf(format, args...))
}

// WrapError wraps an existing error as an AppError
func WrapError(err error, code ErrorCode, category ErrorCategory, message string) *AppError {
	if err == nil {
		return &AppError{
			Code:           code,
			Category:       category,
			Message:        message,
			HTTPStatusCode: code.GetHTTPStatusCode(),
			Timestamp:      time.Now(),
			Retryable:      false,
			Metadata:       make(map[string]interface{}),
		}
	}

	// If it's already an AppError, wrap it with additional context
	if appErr, ok := err.(*AppError); ok {
		return &AppError{
			Code:            code,
			Category:        category,
			Message:         message,
			InternalMessage: fmt.Sprintf("wrapped: %s", appErr.InternalMessage),
			InternalError:   appErr,
			HTTPStatusCode:  code.GetHTTPStatusCode(),
			Timestamp:       time.Now(),
			Retryable:       appErr.Retryable,
			Metadata:        make(map[string]interface{}),
		}
	}

	// Wrap a regular error
	return &AppError{
		Code:            code,
		Category:        category,
		Message:         message,
		InternalMessage: err.Error(),
		InternalError:   err,
		HTTPStatusCode:  code.GetHTTPStatusCode(),
		Timestamp:       time.Now(),
		Retryable:       false,
		Metadata:        make(map[string]interface{}),
	}
}

// WrapErrorf wraps an existing error as an AppError with formatted message
func WrapErrorf(err error, code ErrorCode, category ErrorCategory, format string, args ...interface{}) *AppError {
	return WrapError(err, code, category, fmt.Sprintf(format, args...))
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) bool {
	_, ok := AsAppError(err)
	return ok
}

// AsAppError attempts to convert an error to AppError
func AsAppError(err error) (*AppError, bool) {
	if err == nil {
		return nil, false
	}

	var appErr *AppError
	if stdErrors.As(err, &appErr) {
		return appErr, true
	}

	return nil, false
}

// HasCode checks if the error has the specified code
func HasCode(err error, code ErrorCode) bool {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Code == code
	}
	return false
}

// HasCategory checks if the error has the specified category
func HasCategory(err error, category ErrorCategory) bool {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Category == category
	}
	return false
}

// IsRetryable checks if the error is marked as retryable
func IsRetryable(err error) bool {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Retryable
	}
	return false
}

// GetHTTPStatus extracts the HTTP status code from an error
func GetHTTPStatus(err error) int {
	if appErr, ok := AsAppError(err); ok {
		return appErr.HTTPStatusCode
	}
	return 500
}

// GetErrorCode extracts the error code from an error
func GetErrorCode(err error) ErrorCode {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Code
	}
	return CodeInternal
}

// GetErrorCategory extracts the error category from an error
func GetErrorCategory(err error) ErrorCategory {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Category
	}
	return CategoryInternal
}

// Common error creation helpers

// NotFound creates a not found error
func NotFound(resource string) *AppError {
	return NewAppError(CodeNotFound, CategoryAPI, fmt.Sprintf("%s not found", resource))
}

// NotFoundWithID creates a not found error with specific ID
func NotFoundWithID(resource, id string) *AppError {
	return NewAppError(CodeNotFound, CategoryAPI, fmt.Sprintf("%s not found", resource)).
		WithMetadata("resource", resource).
		WithMetadata("id", id)
}

// Unauthorized creates an unauthorized error
func Unauthorized(message string) *AppError {
	if message == "" {
		message = "Authentication required"
	}
	return NewAppError(CodeUnauthorized, CategoryAuth, message)
}

// Forbidden creates a forbidden error
func Forbidden(message string) *AppError {
	if message == "" {
		message = "Access denied"
	}
	return NewAppError(CodeForbidden, CategoryAuth, message)
}

// BadRequest creates a bad request error
func BadRequest(message string) *AppError {
	return NewAppError(CodeBadRequest, CategoryAPI, message)
}

// ValidationFailed creates a validation error
func ValidationFailed(field, message string) *AppError {
	return NewAppError(CodeValidationFailed, CategoryValidation, message).
		WithMetadata("field", field)
}

// Internal creates an internal server error
func Internal(message string) *AppError {
	if message == "" {
		message = "An internal error occurred"
	}
	return NewAppError(CodeInternal, CategoryInternal, message)
}

// InternalWithCause creates an internal error wrapping another error
func InternalWithCause(err error, message string) *AppError {
	return WrapError(err, CodeInternal, CategoryInternal, message)
}
