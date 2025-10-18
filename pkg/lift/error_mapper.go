package lift

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
)

// MapCommonError converts common package errors to Lift errors
func MapCommonError(err error) error {
	switch e := err.(type) {
	case *pkgerrors.AppError:
		// Map AppError based on status code
		switch e.HTTPStatusCode {
		case 400:
			return ValidationError(e.Message)
		case 401:
			return UnauthorizedError(e.Message)
		case 403:
			return ForbiddenError("", "")
		case 404:
			return NotFoundError("resource")
		case 409:
			return ConflictError("resource", e.Message)
		case 429:
			return RateLimitError(e.Message)
		case 500:
			return InternalError(e.Message)
		case 502:
			return FederationError("request", "remote", e.InternalError)
		case 503:
			return ServiceUnavailableError("")
		case 504:
			return TimeoutError("")
		default:
			return InternalError(e.Message)
		}
	default:
		return err
	}
}

// MapStorageError converts storage errors to appropriate Lift errors
func MapStorageError(err error) error {
	if err == nil {
		return nil
	}

	// Check for storage-specific errors
	switch err {
	case storage.ErrNotFound:
		return NotFoundError("resource")
	case storage.ErrAlreadyExists:
		return ConflictError("resource", "already exists")
	case storage.ErrInvalidInput:
		return ValidationError("Invalid input")
	// Note: condition failed is handled by DynamoDB-specific errors below
	default:
		// Check for DynamoDB-specific errors
		var rnf *types.ResourceNotFoundException
		if errors.As(err, &rnf) {
			return NotFoundError("resource")
		}

		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return ConflictError("resource", "condition check failed")
		}

		// Note: ValidationException is not a separate type in AWS SDK v2

		var rle *types.RequestLimitExceeded
		if errors.As(err, &rle) {
			return RateLimitError("Request throttled, please try again")
		}

		var pte *types.ProvisionedThroughputExceededException
		if errors.As(err, &pte) {
			return RateLimitError("Request throttled, please try again")
		}

		// For any other storage error, return internal error
		return InternalError("Database operation failed")
	}
}

// MapAWSError maps AWS SDK errors to appropriate Lift errors
func MapAWSError(err error) error {
	if err == nil {
		return nil
	}

	// Check for smithy API errors
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		message := apiErr.ErrorMessage()

		switch code {
		// Authentication/Authorization errors
		case "UnauthorizedException", "InvalidUserPool", "NotAuthorizedException":
			return UnauthorizedError(message)
		case "AccessDeniedException", "AccessDenied", "Forbidden":
			return ForbiddenError("", "AWS resource")

		// Not found errors
		case "ResourceNotFoundException", "NotFoundException", "NoSuchEntity":
			return NotFoundError("resource")

		// Validation errors
		case "ValidationException", "InvalidParameterException", "InvalidRequestException":
			return ValidationError(message)

		// Conflict errors
		case "ResourceConflictException", "ConflictException", "DuplicateResourceException":
			return ConflictError("resource", message)

		// Rate limiting
		case "ThrottlingException", "TooManyRequestsException", "RequestLimitExceeded":
			return RateLimitError(message)

		// Service errors
		case "ServiceUnavailableException", "ServiceUnavailable":
			return ServiceUnavailableError("AWS")

		// Timeout errors
		case "RequestTimeoutException", "RequestTimeout":
			return TimeoutError("AWS request")

		default:
			// Return generic AWS error
			return InternalError("AWS operation failed")
		}
	}

	// Check error message for common patterns
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "AccessDenied"):
		return ForbiddenError("", "AWS resource")
	case strings.Contains(errMsg, "NoSuchKey"):
		return NotFoundError("object")
	case strings.Contains(errMsg, "BucketNotFound"):
		return NotFoundError("bucket")
	case strings.Contains(errMsg, "RequestTimeout"):
		return TimeoutError("AWS request")
	case strings.Contains(errMsg, "ServiceUnavailable"):
		return ServiceUnavailableError("AWS")
	case strings.Contains(errMsg, "ThrottlingException"):
		return RateLimitError("AWS request throttled")
	default:
		return InternalError("AWS operation failed")
	}
}
