package health

import "errors"

// Error constants for federation health package
var (
	// Event processing errors
	ErrUnknownAction = errors.New("unknown action")
	
	// Health check errors
	ErrNoDomains = errors.New("no domains specified for health check")
	
	// Aggregation errors
	ErrAggregationRequired = errors.New("domains and windows are required for aggregation")
	ErrUnsupportedWindow   = errors.New("unsupported window")
	
	// Event validation errors
	ErrActionRequired                 = errors.New("action is required")
	ErrDomainsOrInstanceIDsRequired   = errors.New("domains or instance IDs are required")
	ErrBatchSizeMustBePositive        = errors.New("batch size must be positive")
	ErrTimeoutMustBePositive          = errors.New("timeout must be positive")
	ErrDomainsRequiredForAggregation  = errors.New("domains are required for aggregation")
	ErrWindowsRequiredForAggregation  = errors.New("windows are required for aggregation")
	ErrInvalidWindowFormat            = errors.New("invalid window format")
	
	// Serverless health checker errors
	ErrHealthCheckEventValidationFailed = errors.New("health check event validation failed")
	ErrServerlessHealthCheckFailed      = errors.New("serverless health check failed")
	ErrHealthDataCleanupFailed          = errors.New("health data cleanup failed")
)