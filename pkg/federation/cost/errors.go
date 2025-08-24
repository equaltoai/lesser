package cost

import "errors"

// Error constants for federation cost integration
var (
	// ErrFederationNotAllowed indicates federation is blocked or limited for a domain
	ErrFederationNotAllowed = errors.New("federation not allowed")

	// ErrInstanceUnhealthy indicates an instance is marked as unhealthy and should not receive retries
	ErrInstanceUnhealthy = errors.New("instance unhealthy")

	// ErrOperationFailedAfterRetries indicates an operation failed after all retry attempts
	ErrOperationFailedAfterRetries = errors.New("operation failed after retries")

	// ErrEmptyInstanceURL indicates an empty or invalid instance URL was provided
	ErrEmptyInstanceURL = errors.New("empty instance URL")

	// ErrGetInstanceTier indicates failure to retrieve instance tier
	ErrGetInstanceTier = errors.New("failed to get instance tier")

	// ErrCheckHealth indicates failure to check instance health
	ErrCheckHealth = errors.New("failed to check health")

	// ErrGetRemainingBudget indicates failure to get remaining budget
	ErrGetRemainingBudget = errors.New("failed to get remaining budget")

	// ErrGetInstanceConfig indicates failure to get instance configuration
	ErrGetInstanceConfig = errors.New("failed to get instance config")

	// ErrGetRetryPolicy indicates failure to get retry policy
	ErrGetRetryPolicy = errors.New("failed to get retry policy")

	// ErrGetInstanceCost indicates failure to get instance cost data
	ErrGetInstanceCost = errors.New("failed to get instance cost")

	// ErrRecordCost indicates failure to record cost data
	ErrRecordCost = errors.New("failed to record cost")

	// ErrGetInstanceHealth indicates failure to get instance health data
	ErrGetInstanceHealth = errors.New("failed to get instance health")

	// ErrUpdateInstanceHealth indicates failure to update instance health
	ErrUpdateInstanceHealth = errors.New("failed to update instance health")

	// Cost Integration specific errors

	// ErrDomainExtractionFailed indicates failure to extract domain from URL
	ErrDomainExtractionFailed = errors.New("domain extraction failed")

	// ErrFederationCheckFailed indicates failure to check federation status
	ErrFederationCheckFailed = errors.New("federation check failed")
)
