package dynamorm

import (
	stdErrors "errors"
	"strings"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
)

// Legacy error variables for backward compatibility - using centralized error system
var (
	// ErrConditionalCheckFailed is returned when a conditional write fails
	ErrConditionalCheckFailed = errors.DynamoDBConditionalCheckFailed("")

	// ErrThrottling is returned when DynamoDB throttles the request
	ErrThrottling = errors.DynamoDBProvisionedThroughputExceeded()

	// ErrResourceNotFound is returned when a DynamoDB resource is not found
	ErrResourceNotFound = errors.DatabaseUnavailable(stdErrors.New("database resource not found"))

	// ErrInternal is returned for internal errors
	ErrInternal = errors.NewStorageError(errors.CodeInternal, "internal error")

	// ErrInvalidKey is returned when a key is invalid
	ErrInvalidKey = errors.InvalidFormat("key", "valid key format")

	// ErrBatchOperationFailed is returned when a batch operation fails
	ErrBatchOperationFailed = errors.BatchOperationFailed("batch operation", stdErrors.New("batch operation failed"))

	// ErrTransactionCanceled is returned when a transaction is canceled
	ErrTransactionCanceled = errors.TransactionFailed(stdErrors.New("transaction was canceled"))

	// ErrValidation is returned when validation fails
	ErrValidation = errors.NewValidationError("", "validation failed")
)

// MapError maps DynamORM errors to storage errors using centralized error system
func MapError(err error) error {
	if err == nil {
		return nil
	}

	// Check for DynamORM specific errors first
	if isDynamORMNotFoundError(err) {
		return storage.ErrNotFound
	}

	// Check for common error messages
	errMsg := err.Error()

	// Not found errors
	if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "item not found") ||
		strings.Contains(errMsg, "record not found") {
		return storage.ErrNotFound
	}

	// Validation errors
	if strings.Contains(errMsg, "validation failed") || strings.Contains(errMsg, "validation error") {
		return storage.ErrInvalidInput
	}

	// Conditional check errors
	if strings.Contains(errMsg, "conditional check failed") ||
		strings.Contains(errMsg, "condition failed") {
		return ErrConditionalCheckFailed
	}

	// Transaction errors
	if strings.Contains(errMsg, "transaction canceled") ||
		strings.Contains(errMsg, "transaction failed") {
		return ErrTransactionCanceled
	}

	// Key errors
	if strings.Contains(errMsg, "missing key") || strings.Contains(errMsg, "invalid key") ||
		strings.Contains(errMsg, "key required") {
		return ErrInvalidKey
	}

	// Throttling errors
	if strings.Contains(errMsg, "throttl") || strings.Contains(errMsg, "rate limit") ||
		strings.Contains(errMsg, "capacity exceed") {
		return ErrThrottling
	}

	// Resource not found errors
	if strings.Contains(errMsg, "resource not found") || strings.Contains(errMsg, "table not found") {
		return ErrResourceNotFound
	}

	// Batch operation errors
	if strings.Contains(errMsg, "batch") && strings.Contains(errMsg, "failed") {
		return ErrBatchOperationFailed
	}

	// General validation errors
	if strings.Contains(errMsg, "validation") || strings.Contains(errMsg, "invalid") {
		return ErrValidation
	}

	// Default to internal error with original error message
	return errors.NewStorageInternalError(errors.CodeInternal, "DynamORM error", err)
}

// isDynamORMNotFoundError checks if the error is a DynamORM not found error
func isDynamORMNotFoundError(err error) bool {
	// Check for DynamORM error patterns that indicate not found
	// Note: core.ErrNotFound may not be exported, so check error strings

	// Check error message patterns that indicate not found
	errMsg := strings.ToLower(err.Error())
	notFoundPatterns := []string{
		"record not found",
		"item not found",
		"no rows",
		"no items found",
		"does not exist",
	}

	for _, pattern := range notFoundPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// MapErrorWithContext maps DynamORM errors to storage errors with additional context using centralized error system
func MapErrorWithContext(err error, context string) error {
	if err == nil {
		return nil
	}

	mappedErr := MapError(err)
	// If mapped error is already an AppError, add context as metadata
	if appErr, ok := mappedErr.(*errors.AppError); ok {
		return appErr.WithMetadata("context", context)
	}

	return errors.NewStorageInternalError(errors.CodeInternal, context, mappedErr)
}

// DetailedError provides detailed error information
type DetailedError struct {
	// Original error
	Err error

	// Operation that caused the error
	Operation string

	// Entity type that was being operated on
	EntityType string

	// Entity ID that was being operated on
	EntityID string

	// Additional context
	Context string
}

// Error implements the error interface
func (e *DetailedError) Error() string {
	var builder strings.Builder

	if e.Operation != "" {
		builder.WriteString(e.Operation)
		builder.WriteString(" ")
	}

	if e.EntityType != "" {
		builder.WriteString(e.EntityType)

		if e.EntityID != "" {
			builder.WriteString(" (")
			builder.WriteString(e.EntityID)
			builder.WriteString(")")
		}

		builder.WriteString(": ")
	}

	if e.Context != "" {
		builder.WriteString(e.Context)
		builder.WriteString(": ")
	}

	if e.Err != nil {
		builder.WriteString(e.Err.Error())
	} else {
		builder.WriteString("unknown error")
	}

	return builder.String()
}

// Unwrap returns the original error
func (e *DetailedError) Unwrap() error {
	return e.Err
}

// NewDetailedError creates a new DetailedError using centralized error system
func NewDetailedError(err error, operation, entityType, entityID, context string) error {
	if err == nil {
		return nil
	}

	appErr := errors.NewStorageInternalError(errors.CodeInternal, "Detailed error", err).
		WithMetadata("operation", operation).
		WithMetadata("entity_type", entityType).
		WithMetadata("entity_id", entityID).
		WithMetadata("context", context)

	return appErr
}

// IsNotFound checks if an error is a not found error using centralized error system
func IsNotFound(err error) bool {
	return errors.HasCode(err, errors.CodeNotFound)
}

// IsConditionalCheckFailed checks if an error is a conditional check failed error using centralized error system
func IsConditionalCheckFailed(err error) bool {
	return errors.HasCode(err, errors.CodeConflict)
}

// IsThrottling checks if an error is a throttling error using centralized error system
func IsThrottling(err error) bool {
	return errors.HasCode(err, errors.CodeRateLimited)
}

// IsTransactionCanceled checks if an error is a transaction canceled error using centralized error system
func IsTransactionCanceled(err error) bool {
	return errors.HasCode(err, errors.CodeTransactionFailed)
}

// IsValidation checks if an error is a validation error using centralized error system
func IsValidation(err error) bool {
	return errors.HasCode(err, errors.CodeValidationFailed) || errors.HasCode(err, errors.CodeInvalidInput)
}

// MapRepositoryError maps repository errors with context using centralized error system
func MapRepositoryError(err error, operation, entityType, entityID string) error {
	if err == nil {
		return nil
	}

	// Map the error first
	mappedErr := MapError(err)

	// If already an AppError, add metadata
	if appErr, ok := mappedErr.(*errors.AppError); ok {
		return appErr.
			WithMetadata("operation", operation).
			WithMetadata("entity_type", entityType).
			WithMetadata("entity_id", entityID)
	}

	// Create detailed error using centralized system
	return errors.NewStorageInternalError(errors.CodeInternal, "Repository error", mappedErr).
		WithMetadata("operation", operation).
		WithMetadata("entity_type", entityType).
		WithMetadata("entity_id", entityID)
}
