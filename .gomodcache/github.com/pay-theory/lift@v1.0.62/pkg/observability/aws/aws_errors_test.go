package aws

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAPIError implements smithy.APIError for testing
type mockAPIError struct {
	code    string
	message string
	fault   smithy.ErrorFault
}

func (e mockAPIError) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

func (e mockAPIError) ErrorCode() string {
	return e.code
}

func (e mockAPIError) ErrorMessage() string {
	return e.message
}

func (e mockAPIError) ErrorFault() smithy.ErrorFault {
	return e.fault
}

func TestExtractAWSErrorDetails(t *testing.T) {
	tests := []struct {
		err      error
		expected map[string]any
		name     string
	}{
		{
			name:     "nil error returns nil",
			err:      nil,
			expected: nil,
		},
		{
			name: "API error with all details",
			err: &smithy.OperationError{
				ServiceID:     "DynamoDB",
				OperationName: "GetItem",
				Err: mockAPIError{
					code:    "ResourceNotFoundException",
					message: "Table not found",
					fault:   smithy.FaultClient,
				},
			},
			expected: map[string]any{
				"service":       "DynamoDB",
				"operation":     "GetItem",
				"error_code":    "ResourceNotFoundException",
				"error_message": "Table not found",
				"error_fault":   "client",
				"error_chain":   "operation error DynamoDB: GetItem, ResourceNotFoundException: Table not found",
			},
		},
		{
			name: "API error without operation details",
			err: mockAPIError{
				code:    "ValidationException",
				message: "Invalid input",
				fault:   smithy.FaultClient,
			},
			expected: map[string]any{
				"error_code":    "ValidationException",
				"error_message": "Invalid input",
				"error_fault":   "client",
				"error_chain":   "ValidationException: Invalid input",
			},
		},
		{
			name: "non-AWS error",
			err:  errors.New("generic error"),
			expected: map[string]any{
				"error_chain": "generic error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := ExtractAWSErrorDetails(tt.err)
			assert.Equal(t, tt.expected, details)
		})
	}
}

func TestIsAWSErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     string
		expected bool
	}{
		{
			name: "matching error code",
			err: mockAPIError{
				code: "ThrottlingException",
			},
			code:     "ThrottlingException",
			expected: true,
		},
		{
			name: "non-matching error code",
			err: mockAPIError{
				code: "ValidationException",
			},
			code:     "ThrottlingException",
			expected: false,
		},
		{
			name:     "non-AWS error",
			err:      errors.New("generic error"),
			code:     "ThrottlingException",
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			code:     "ThrottlingException",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAWSErrorCode(tt.err, tt.code)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRetryableAWSError(t *testing.T) {
	tests := []struct {
		err      error
		name     string
		expected bool
	}{
		{
			name: "server fault is retryable",
			err: mockAPIError{
				code:  "InternalServerError",
				fault: smithy.FaultServer,
			},
			expected: true,
		},
		{
			name: "client fault is not retryable",
			err: mockAPIError{
				code:  "ValidationException",
				fault: smithy.FaultClient,
			},
			expected: false,
		},
		{
			name: "unknown fault is not retryable",
			err: mockAPIError{
				code:  "UnknownError",
				fault: smithy.FaultUnknown,
			},
			expected: false,
		},
		{
			name:     "non-AWS error is not retryable",
			err:      errors.New("generic error"),
			expected: false,
		},
		{
			name:     "nil error is not retryable",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableAWSError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetAWSErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name: "returns error code for AWS error",
			err: mockAPIError{
				code: "ResourceNotFoundException",
			},
			expected: "ResourceNotFoundException",
		},
		{
			name:     "returns empty string for non-AWS error",
			err:      errors.New("generic error"),
			expected: "",
		},
		{
			name:     "returns empty string for nil error",
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAWSErrorCode(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsThrottlingError(t *testing.T) {
	throttlingCodes := []string{
		"ThrottlingException",
		"ProvisionedThroughputExceededException",
		"RequestLimitExceeded",
		"TooManyRequestsException",
	}

	for _, code := range throttlingCodes {
		t.Run(fmt.Sprintf("recognizes %s as throttling error", code), func(t *testing.T) {
			err := mockAPIError{code: code}
			assert.True(t, IsThrottlingError(err))
		})
	}

	t.Run("non-throttling error returns false", func(t *testing.T) {
		err := mockAPIError{code: "ValidationException"}
		assert.False(t, IsThrottlingError(err))
	})

	t.Run("non-AWS error returns false", func(t *testing.T) {
		err := errors.New("generic error")
		assert.False(t, IsThrottlingError(err))
	})
}

func TestIsAccessDeniedError(t *testing.T) {
	accessDeniedCodes := []string{
		"AccessDeniedException",
		"UnauthorizedException",
		"AccessDenied",
	}

	for _, code := range accessDeniedCodes {
		t.Run(fmt.Sprintf("recognizes %s as access denied error", code), func(t *testing.T) {
			err := mockAPIError{code: code}
			assert.True(t, IsAccessDeniedError(err))
		})
	}

	t.Run("non-access-denied error returns false", func(t *testing.T) {
		err := mockAPIError{code: "ValidationException"}
		assert.False(t, IsAccessDeniedError(err))
	})
}

func TestIsResourceNotFoundError(t *testing.T) {
	notFoundCodes := []string{
		"ResourceNotFoundException",
		"NoSuchKey",
		"NotFound",
		"NotFoundException",
	}

	for _, code := range notFoundCodes {
		t.Run(fmt.Sprintf("recognizes %s as not found error", code), func(t *testing.T) {
			err := mockAPIError{code: code}
			assert.True(t, IsResourceNotFoundError(err))
		})
	}

	t.Run("non-not-found error returns false", func(t *testing.T) {
		err := mockAPIError{code: "ValidationException"}
		assert.False(t, IsResourceNotFoundError(err))
	})
}

func TestErrorWrapping(t *testing.T) {
	t.Run("works with wrapped errors", func(t *testing.T) {
		baseErr := mockAPIError{
			code:    "ThrottlingException",
			message: "Too many requests",
			fault:   smithy.FaultClient,
		}
		wrappedErr := fmt.Errorf("failed to process: %w", baseErr)

		// All functions should work with wrapped errors
		assert.True(t, IsAWSErrorCode(wrappedErr, "ThrottlingException"))
		assert.True(t, IsThrottlingError(wrappedErr))
		assert.Equal(t, "ThrottlingException", GetAWSErrorCode(wrappedErr))

		details := ExtractAWSErrorDetails(wrappedErr)
		require.NotNil(t, details)
		assert.Equal(t, "ThrottlingException", details["error_code"])
		assert.Equal(t, "Too many requests", details["error_message"])
	})
}
