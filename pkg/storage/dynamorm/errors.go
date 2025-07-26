package dynamorm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
)

// Common DynamoDB errors
var (
	// ErrConditionalCheckFailed is returned when a conditional write fails
	ErrConditionalCheckFailed = errors.New("conditional check failed")

	// ErrThrottling is returned when DynamoDB throttles the request
	ErrThrottling = errors.New("request throttled")

	// ErrResourceNotFound is returned when a DynamoDB resource is not found
	ErrResourceNotFound = errors.New("resource not found")

	// ErrInternal is returned for internal errors
	ErrInternal = errors.New("internal error")

	// ErrInvalidKey is returned when a key is invalid
	ErrInvalidKey = errors.New("invalid key")

	// ErrBatchOperationFailed is returned when a batch operation fails
	ErrBatchOperationFailed = errors.New("batch operation failed")

	// ErrTransactionCanceled is returned when a transaction is canceled
	ErrTransactionCanceled = errors.New("transaction canceled")

	// ErrValidation is returned when validation fails
	ErrValidation = errors.New("validation failed")
)

// MapError maps DynamORM errors to storage errors
func MapError(err error) error {
	if err == nil {
		return nil
	}

	// Check for common error messages since DynamORM doesn't export error constants
	errMsg := err.Error()

	// Not found errors
	if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "item not found") {
		return storage.ErrNotFound
	}

	// Validation errors
	if strings.Contains(errMsg, "validation failed") {
		return storage.ErrInvalidInput
	}

	// Conditional check errors
	if strings.Contains(errMsg, "conditional check failed") {
		return ErrConditionalCheckFailed
	}

	// Transaction errors
	if strings.Contains(errMsg, "transaction canceled") {
		return ErrTransactionCanceled
	}

	// Key errors
	if strings.Contains(errMsg, "missing key") || strings.Contains(errMsg, "invalid key") {
		return ErrInvalidKey
	}

	// Check for AWS DynamoDB specific errors
	var provisionedThroughputExceeded *types.ProvisionedThroughputExceededException
	if errors.As(err, &provisionedThroughputExceeded) {
		return ErrThrottling
	}

	var resourceNotFound *types.ResourceNotFoundException
	if errors.As(err, &resourceNotFound) {
		return ErrResourceNotFound
	}

	var transactionCanceled *types.TransactionCanceledException
	if errors.As(err, &transactionCanceled) {
		return ErrTransactionCanceled
	}

	// ValidationException is not directly accessible in aws-sdk-go-v2
	// Check for validation error message patterns instead
	if strings.Contains(errMsg, "validation") || strings.Contains(errMsg, "invalid") {
		return ErrValidation
	}

	// Default to internal error with original error message
	return fmt.Errorf("%w: %v", ErrInternal, err)
}

// MapErrorWithContext maps DynamORM errors to storage errors with additional context
func MapErrorWithContext(err error, context string) error {
	if err == nil {
		return nil
	}

	mappedErr := MapError(err)
	return fmt.Errorf("%s: %w", context, mappedErr)
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

// NewDetailedError creates a new DetailedError
func NewDetailedError(err error, operation, entityType, entityID, context string) error {
	if err == nil {
		return nil
	}

	return &DetailedError{
		Err:        err,
		Operation:  operation,
		EntityType: entityType,
		EntityID:   entityID,
		Context:    context,
	}
}

// IsNotFound checks if an error is a not found error
func IsNotFound(err error) bool {
	return errors.Is(err, storage.ErrNotFound)
}

// IsConditionalCheckFailed checks if an error is a conditional check failed error
func IsConditionalCheckFailed(err error) bool {
	return errors.Is(err, ErrConditionalCheckFailed)
}

// IsThrottling checks if an error is a throttling error
func IsThrottling(err error) bool {
	return errors.Is(err, ErrThrottling)
}

// IsTransactionCanceled checks if an error is a transaction canceled error
func IsTransactionCanceled(err error) bool {
	return errors.Is(err, ErrTransactionCanceled)
}

// IsValidation checks if an error is a validation error
func IsValidation(err error) bool {
	return errors.Is(err, ErrValidation) || errors.Is(err, storage.ErrInvalidInput)
}
