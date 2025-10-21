package aws

import (
	"errors"

	"github.com/aws/smithy-go"
)

// ExtractAWSErrorDetails extracts detailed error information from AWS SDK errors
func ExtractAWSErrorDetails(err error) map[string]any {
	if err == nil {
		return nil
	}

	details := make(map[string]any)

	// Extract operation details if available
	var oe *smithy.OperationError
	if errors.As(err, &oe) {
		details["service"] = oe.Service()
		details["operation"] = oe.Operation()
	}

	// Extract API error details
	var ae smithy.APIError
	if errors.As(err, &ae) {
		details["error_code"] = ae.ErrorCode()
		details["error_message"] = ae.ErrorMessage()
		details["error_fault"] = ae.ErrorFault().String()
	}

	// Add the full error chain for debugging
	details["error_chain"] = err.Error()

	return details
}

// IsAWSErrorCode checks if the error matches a specific AWS error code
func IsAWSErrorCode(err error, code string) bool {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode() == code
	}
	return false
}

// IsRetryableAWSError determines if an AWS error should be retried
func IsRetryableAWSError(err error) bool {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		// Server errors (5xx) are typically retryable
		return ae.ErrorFault() == smithy.FaultServer
	}
	return false
}

// GetAWSErrorCode extracts the AWS error code from an error, returns empty string if not an AWS error
func GetAWSErrorCode(err error) string {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode()
	}
	return ""
}

// IsThrottlingError checks if the error is an AWS throttling error
func IsThrottlingError(err error) bool {
	code := GetAWSErrorCode(err)
	return code == "ThrottlingException" ||
		code == "ProvisionedThroughputExceededException" ||
		code == "RequestLimitExceeded" ||
		code == "TooManyRequestsException"
}

// IsAccessDeniedError checks if the error is an AWS access denied error
func IsAccessDeniedError(err error) bool {
	code := GetAWSErrorCode(err)
	return code == "AccessDeniedException" ||
		code == "UnauthorizedException" ||
		code == "AccessDenied"
}

// IsResourceNotFoundError checks if the error is an AWS resource not found error
func IsResourceNotFoundError(err error) bool {
	code := GetAWSErrorCode(err)
	return code == "ResourceNotFoundException" ||
		code == "NoSuchKey" ||
		code == "NotFound" ||
		code == "NotFoundException"
}
