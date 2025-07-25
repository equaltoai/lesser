package lift

import (
	"fmt"

	"github.com/pay-theory/lift/pkg/lift"
)

// LogAndReturnError logs an error with context and returns an appropriate user-facing error
func LogAndReturnError(ctx *lift.Context, err error, message string, fields map[string]any) error {
	if err == nil {
		return nil
	}

	// Add request ID to fields
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["request_id"] = ctx.GetRequestID()
	fields["error"] = err.Error()

	// If it's already a LiftError, just log and return it
	if _, ok := err.(*lift.LiftError); ok {
		ctx.Logger.Error(message, fields)
		return err
	}

	// For other errors, log the internal error and return a safe error
	ctx.Logger.Error(message, fields)
	return InternalError("An error occurred processing your request")
}

// WrapDatabaseError wraps a database error with context and returns appropriate error
func WrapDatabaseError(ctx *lift.Context, err error, operation string, resource string) error {
	if err == nil {
		return nil
	}

	// Map to appropriate error type
	mappedErr := MapStorageError(err)

	// Log with context
	ctx.Logger.Error("Database operation failed", map[string]any{
		"operation":  operation,
		"resource":   resource,
		"request_id": ctx.GetRequestID(),
		"error":      err.Error(),
	})

	// Add operation context to the error if it's a LiftError
	if liftErr, ok := mappedErr.(*lift.LiftError); ok {
		return liftErr.WithDetail("operation", operation).WithDetail("resource", resource)
	}

	return mappedErr
}

// WrapExternalServiceError wraps errors from external services
func WrapExternalServiceError(ctx *lift.Context, err error, service string, operation string) error {
	if err == nil {
		return nil
	}

	// Log the actual error
	ctx.Logger.Error("External service error", map[string]any{
		"service":    service,
		"operation":  operation,
		"request_id": ctx.GetRequestID(),
		"error":      err.Error(),
	})

	// Return a generic error to avoid leaking service details
	return ServiceUnavailableError(service).WithDetail("operation", operation)
}

// ResourceNotFound creates a not found error with logging
func ResourceNotFound(ctx *lift.Context, resourceType string, id string) error {
	ctx.Logger.Info("Resource not found", map[string]any{
		"resource_type": resourceType,
		"resource_id":   id,
		"request_id":    ctx.GetRequestID(),
	})
	return NotFoundError(resourceType).WithDetail("id", id)
}

// ValidationFailed creates a validation error with logging
func ValidationFailed(ctx *lift.Context, errors map[string]string) error {
	ctx.Logger.Info("Validation failed", map[string]any{
		"validation_errors": errors,
		"request_id":        ctx.GetRequestID(),
	})

	// Create error with first validation error as main message
	var firstError string
	var firstField string
	for field, err := range errors {
		firstField = field
		firstError = err
		break
	}

	liftErr := ValidationErrorWithField(firstField, firstError)
	
	// Add all validation errors as details
	for field, err := range errors {
		liftErr = liftErr.WithDetail(fmt.Sprintf("validation.%s", field), err)
	}

	return liftErr
}

// AccessDenied creates an authorization error with logging
func AccessDenied(ctx *lift.Context, action string, resource string, userID string) error {
	ctx.Logger.Warn("Access denied", map[string]any{
		"action":     action,
		"resource":   resource,
		"user_id":    userID,
		"request_id": ctx.GetRequestID(),
	})
	return ForbiddenError(action, resource)
}