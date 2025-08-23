package cost

import "errors"

// Circuit breaker related errors
var (
	ErrCircuitBreakerOpen     = errors.New("circuit breaker open: cost limit exceeded")
	ErrCircuitBreakerReopened = errors.New("circuit breaker reopened: cost still too high")
	ErrHourlyCostLimitExceeded = errors.New("hourly cost limit would be exceeded")
)