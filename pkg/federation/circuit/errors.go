// Package circuit provides error constants for circuit breaker operations.
package circuit

import "errors"

// Circuit breaker error constants
var (
	// ErrInvalidStateTransition is returned when attempting an invalid state transition
	ErrInvalidStateTransition = errors.New("invalid circuit breaker state transition")
)