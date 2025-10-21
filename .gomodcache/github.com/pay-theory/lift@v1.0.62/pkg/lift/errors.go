package lift

import (
	"crypto/rand"
	"fmt"
	"runtime/debug"
	"time"
)

// LiftError represents a structured error in the Lift framework
type LiftError struct {
	Cause      error          `json:"-"`
	Details    map[string]any `json:"details,omitempty"`
	ErrorData  map[string]any `json:"errorData,omitempty"`
	ErrorInfo  map[string]any `json:"errorInfo,omitempty"`
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	RequestID  string         `json:"request_id,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	Timestamp  string         `json:"timestamp"`
	StackTrace string         `json:"stack_trace,omitempty"`
	StatusCode int            `json:"-"`
	LogError   bool           `json:"-"`
}

// Error implements the error interface
func (e *LiftError) Error() string {
	errStr := fmt.Sprintf("[%s] %s", e.Code, e.Message)

	if e.Cause != nil {
		errStr += fmt.Sprintf("\ncaused by: %v", e.Cause)
	}

	if len(e.Details) > 0 {
		errStr += fmt.Sprintf("\ndetails: %v", e.Details)
	}

	return errStr
}

// Unwrap implements the unwrap interface for error chaining
func (e *LiftError) Unwrap() error {
	return e.Cause
}

// NewLiftError creates a new LiftError
func NewLiftError(code, message string, statusCode int) *LiftError {
	return &LiftError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
}

// WithDetails adds details to the error
func (e *LiftError) WithDetails(details map[string]any) *LiftError {
	e.Details = details
	return e
}

// WithDetail adds a single detail to the error
func (e *LiftError) WithDetail(key string, value any) *LiftError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = value
	return e
}

// WithCause adds a cause error
func (e *LiftError) WithCause(cause error) *LiftError {
	e.Cause = cause
	return e
}

// WithRequestID adds a request ID to the error
func (e *LiftError) WithRequestID(requestID string) *LiftError {
	e.RequestID = requestID
	return e
}

// WithTraceID adds a trace ID to the error
func (e *LiftError) WithTraceID(traceID string) *LiftError {
	if traceID == "" {
		// Generate a simple UUID v4
		uuid := make([]byte, 16)
		if _, err := rand.Read(uuid); err != nil {
			// Fallback to time-based ID if crypto/rand fails
			e.TraceID = fmt.Sprintf("fallback-%d", time.Now().UnixNano())
			return e
		}
		uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
		uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant RFC 4122
		e.TraceID = fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
	} else {
		e.TraceID = traceID
	}
	return e
}

// WithStackTrace adds a stack trace to the error
func (e *LiftError) WithStackTrace() *LiftError {
	e.StackTrace = string(debug.Stack())
	return e
}

// WithErrorData adds AppSync error data
func (e *LiftError) WithErrorData(data map[string]any) *LiftError {
	e.ErrorData = data
	return e
}

// WithErrorInfo adds AppSync error info
func (e *LiftError) WithErrorInfo(info map[string]any) *LiftError {
	e.ErrorInfo = info
	return e
}

// WithLogging marks the error for logging
func (e *LiftError) WithLogging() *LiftError {
	e.LogError = true
	return e
}

// HTTP error constructors

// ParameterError creates a parameter validation error with field details
func ParameterError(field, message string) *LiftError {
	return NewLiftError(ErrorCodeParameterError, message, 400).WithDetails(map[string]any{
		"field":   field,
		"message": message,
	})
}

func Unauthorized(message string) *LiftError {
	return NewLiftError(ErrorCodeUnauthorized, message, 401)
}

// AuthorizationError creates a 403 Forbidden error
func AuthorizationError(message string) *LiftError {
	return NewLiftError(ErrorCodeAuthorizationError, message, 403)
}

func NotFound(message string) *LiftError {
	return NewLiftError(ErrorCodeNotFound, message, 404)
}

// ValidationError creates a 422 Unprocessable Entity error for validation failures
func ValidationError(message string) *LiftError {
	return NewLiftError(ErrorCodeValidationError, message, 422)
}

func SystemError(message string) *LiftError {
	return NewLiftError(ErrorCodeSystemError, message, 500).WithStackTrace().WithTraceID("")
}

// NetworkError creates a network-related error
func NetworkError(message string) *LiftError {
	return NewLiftError(ErrorCodeNetworkError, message, 500)
}

// ProcessingError creates a processing error
func ProcessingError(message string) *LiftError {
	return NewLiftError(ErrorCodeProcessingError, message, 500)
}

// TokenizationFailure creates a tokenization error
func TokenizationFailure(message string) *LiftError {
	return NewLiftError(ErrorCodeTokenizationError, message, 500)
}

// ErrorResponse formats an error for JSON response
func ErrorResponse(e *LiftError) map[string]any {
	response := map[string]any{
		"error": map[string]any{
			"code":    e.Code,
			"message": e.Message,
		},
	}

	if e.RequestID != "" {
		response["request_id"] = e.RequestID
	}

	if len(e.Details) > 0 {
		if errorMap, ok := response["error"].(map[string]any); ok {
			errorMap["details"] = e.Details
		}
	}

	return response
}
