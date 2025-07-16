package dynamorm

import (
	"errors"
	"fmt"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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
)

// MapError maps DynamORM errors to storage errors
func MapError(err error) error {
	if err == nil {
		return nil
	}

	// Check for common error messages since DynamORM doesn't export error constants
	if err.Error() == "not found" || err.Error() == "item not found" {
		return storage.ErrNotFound
	}

	if err.Error() == "validation failed" {
		return storage.ErrInvalidInput
	}

	if err.Error() == "conditional check failed" {
		return ErrConditionalCheckFailed
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
