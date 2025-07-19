package lift

import (
	"encoding/json"
	"fmt"
)

// Error represents an API error
type Error struct {
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	Details    interface{} `json:"details,omitempty"`
	StatusCode int         `json:"-"`
}

// Error returns the error message
func (e *Error) Error() string {
	return e.Message
}

// JSON returns the JSON representation of the error
func (e *Error) JSON() string {
	data, _ := json.Marshal(e)
	return string(data)
}

// NewError creates a new error with the given status code and message
func NewError(statusCode int, message string, details interface{}) *Error {
	return &Error{
		Code:       fmt.Sprintf("ERROR_%d", statusCode),
		Message:    message,
		Details:    details,
		StatusCode: statusCode,
	}
}

// NewLiftError creates a new error with a custom code
func NewLiftError(code string, message string, statusCode int) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
	}
}

// BadRequest creates a 400 Bad Request error
func BadRequest(message string) *Error {
	return NewError(400, message, nil)
}

// Unauthorized creates a 401 Unauthorized error
func Unauthorized(message string) *Error {
	if message == "" {
		message = "Unauthorized"
	}
	return NewError(401, message, nil)
}

// Forbidden creates a 403 Forbidden error
func Forbidden(message string) *Error {
	if message == "" {
		message = "Forbidden"
	}
	return NewError(403, message, nil)
}

// NotFound creates a 404 Not Found error
func NotFound(message string) *Error {
	if message == "" {
		message = "Resource not found"
	}
	return NewError(404, message, nil)
}

// MethodNotAllowed creates a 405 Method Not Allowed error
func MethodNotAllowed(message string) *Error {
	if message == "" {
		message = "Method not allowed"
	}
	return NewError(405, message, nil)
}

// Conflict creates a 409 Conflict error
func Conflict(message string) *Error {
	if message == "" {
		message = "Resource conflict"
	}
	return NewError(409, message, nil)
}

// ValidationError creates a 422 Unprocessable Entity error
func ValidationError(message string) *Error {
	if message == "" {
		message = "Validation error"
	}
	return NewError(422, message, nil)
}

// InternalServerError creates a 500 Internal Server Error
func InternalServerError(message string) *Error {
	if message == "" {
		message = "Internal server error"
	}
	return NewError(500, message, nil)
}

// ServiceUnavailable creates a 503 Service Unavailable error
func ServiceUnavailable(message string) *Error {
	if message == "" {
		message = "Service unavailable"
	}
	return NewError(503, message, nil)
}
