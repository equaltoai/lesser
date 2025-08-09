package lift

import (
	"fmt"

	"github.com/pay-theory/lift/pkg/lift"
)

// Domain-specific error constructors using Lift patterns

// NotFoundError creates a Lift error for not found resources
func NotFoundError(resource string) *lift.LiftError {
	return lift.NotFound(fmt.Sprintf("%s not found", resource))
}

// ValidationError creates a Lift error for validation failures
func ValidationError(message string) *lift.LiftError {
	return lift.ValidationError(message)
}

// ValidationErrorWithField creates a validation error with field details
func ValidationErrorWithField(field, message string) *lift.LiftError {
	return lift.NewLiftError(
		"VALIDATION_ERROR",
		message,
		422,
	).WithDetail("field", field)
}

// UnauthorizedError creates a Lift error for authentication failures
func UnauthorizedError(message string) *lift.LiftError {
	if message == "" {
		message = "Authentication required"
	}
	return lift.Unauthorized(message)
}

// ForbiddenError creates a Lift error for authorization failures
func ForbiddenError(action, resource string) *lift.LiftError {
	message := "Access denied"
	if action != "" && resource != "" {
		message = fmt.Sprintf("Not authorized to %s %s", action, resource)
	}
	return lift.AuthorizationError(message)
}

// ConflictError creates a Lift error for resource conflicts
func ConflictError(resource, message string) *lift.LiftError {
	fullMessage := fmt.Sprintf("Conflict with %s", resource)
	if message != "" {
		fullMessage = fmt.Sprintf("%s: %s", fullMessage, message)
	}
	return lift.NewLiftError(
		"CONFLICT",
		fullMessage,
		409,
	).WithDetail("resource", resource)
}

// RateLimitError creates a Lift error for rate limiting
func RateLimitError(message string) *lift.LiftError {
	if message == "" {
		message = "Rate limit exceeded"
	}
	return lift.NewLiftError(
		"RATE_LIMITED",
		message,
		429,
	)
}

// FederationError creates a Lift error for federation failures
func FederationError(operation, remote string, err error) *lift.LiftError {
	return lift.NewLiftError(
		"FEDERATION_ERROR",
		fmt.Sprintf("Federation %s failed for %s", operation, remote),
		502,
	).WithDetail("operation", operation).
		WithDetail("remote", remote).
		WithDetail("error", err.Error())
}

// InternalError creates a generic internal server error
// This should be used when we want to hide internal details from users
func InternalError(message string) *lift.LiftError {
	if message == "" {
		message = "An internal error occurred"
	}
	return lift.NewLiftError(
		"INTERNAL_ERROR",
		message,
		500,
	)
}

// ServiceUnavailableError creates a service unavailable error
func ServiceUnavailableError(service string) *lift.LiftError {
	message := "Service temporarily unavailable"
	if service != "" {
		message = fmt.Sprintf("%s service temporarily unavailable", service)
	}
	return lift.NewLiftError(
		"SERVICE_UNAVAILABLE",
		message,
		503,
	).WithDetail("service", service)
}

// TimeoutError creates a timeout error
func TimeoutError(operation string) *lift.LiftError {
	message := "Request timed out"
	if operation != "" {
		message = fmt.Sprintf("%s timed out", operation)
	}
	return lift.NewLiftError(
		"TIMEOUT",
		message,
		504,
	).WithDetail("operation", operation)
}
