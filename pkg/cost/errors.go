package cost

import "github.com/equaltoai/lesser/pkg/errors"

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	ErrCircuitBreakerOpen      = errors.CircuitBreakerOpen()
	ErrCircuitBreakerReopened  = errors.CircuitBreakerReopened()
	ErrHourlyCostLimitExceeded = errors.HourlyCostLimitExceeded()
)
