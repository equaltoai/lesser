package health

import "github.com/equaltoai/lesser/pkg/errors"

// Error constants for federation health package
var (
	// Event processing errors
	ErrUnknownAction = errors.NewFederationError(errors.CodeInvalidInput, "unknown action")

	// Health check errors
	ErrNoDomains = errors.NewFederationError(errors.CodeRequiredFieldMissing, "no domains specified for health check")

	// Aggregation errors
	ErrAggregationRequired = errors.NewFederationError(errors.CodeRequiredFieldMissing, "domains and windows are required for aggregation")
	ErrUnsupportedWindow   = errors.NewFederationError(errors.CodeInvalidInput, "unsupported window")

	// Event validation errors
	ErrActionRequired                = errors.NewValidationError("action", "required")
	ErrDomainsOrInstanceIDsRequired  = errors.NewValidationError("domains_or_instance_ids", "required")
	ErrBatchSizeMustBePositive       = errors.NewValidationError("batch_size", "must be positive")
	ErrTimeoutMustBePositive         = errors.NewValidationError("timeout", "must be positive")
	ErrDomainsRequiredForAggregation = errors.NewValidationError("domains", "required for aggregation")
	ErrWindowsRequiredForAggregation = errors.NewValidationError("windows", "required for aggregation")
	ErrInvalidWindowFormat           = errors.NewValidationError("window", "invalid format")

	// Serverless health checker errors
	ErrHealthCheckEventValidationFailed = errors.ValidationFailedWithField("health check event")
	ErrServerlessHealthCheckFailed      = errors.ProcessingFailed("serverless health check", nil)
	ErrHealthDataCleanupFailed          = errors.ProcessingFailed("health data cleanup", nil)
)
