package cost

import "github.com/equaltoai/lesser/pkg/errors"

// Error constants for federation cost integration
var (
	// ErrFederationNotAllowed indicates federation is blocked or limited for a domain
	ErrFederationNotAllowed = errors.NewFederationError(errors.CodeForbidden, "federation not allowed")

	// ErrInstanceUnhealthy indicates an instance is marked as unhealthy and should not receive retries
	ErrInstanceUnhealthy = errors.NewFederationError(errors.CodeExternalServiceUnavailable, "instance unhealthy")

	// ErrOperationFailedAfterRetries indicates an operation failed after all retry attempts
	ErrOperationFailedAfterRetries = errors.ProcessingFailed("operation after retries", nil)

	// ErrEmptyInstanceURL indicates an empty or invalid instance URL was provided
	ErrEmptyInstanceURL = errors.NewValidationError("instance_url", "cannot be empty")

	// ErrGetInstanceTier indicates failure to retrieve instance tier
	ErrGetInstanceTier = errors.FailedToGet("instance tier", nil)

	// ErrCheckHealth indicates failure to check instance health
	ErrCheckHealth = errors.ProcessingFailed("health check", nil)

	// ErrGetRemainingBudget indicates failure to get remaining budget
	ErrGetRemainingBudget = errors.FailedToGet("remaining budget", nil)

	// ErrGetInstanceConfig indicates failure to get instance configuration
	ErrGetInstanceConfig = errors.FailedToGet("instance config", nil)

	// ErrGetRetryPolicy indicates failure to get retry policy
	ErrGetRetryPolicy = errors.FailedToGet("retry policy", nil)

	// ErrGetInstanceCost indicates failure to get instance cost data
	ErrGetInstanceCost = errors.FailedToGet("instance cost", nil)

	// ErrRecordCost indicates failure to record cost data
	ErrRecordCost = errors.ProcessingFailed("cost recording", nil)

	// ErrGetInstanceHealth indicates failure to get instance health data
	ErrGetInstanceHealth = errors.FailedToGet("instance health", nil)

	// ErrUpdateInstanceHealth indicates failure to update instance health
	ErrUpdateInstanceHealth = errors.FailedToUpdate("instance health", nil)

	// Cost Integration specific errors

	// ErrDomainExtractionFailed indicates failure to extract domain from URL
	ErrDomainExtractionFailed = errors.ProcessingFailed("domain extraction", nil)

	// ErrFederationCheckFailed indicates failure to check federation status
	ErrFederationCheckFailed = errors.ProcessingFailed("federation check", nil)
)
