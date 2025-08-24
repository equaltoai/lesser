// Package circuit provides error constants for circuit breaker operations.
package circuit

import "github.com/equaltoai/lesser/pkg/errors"

// Circuit breaker error constants
var (
	// ErrInvalidStateTransition is returned when attempting an invalid state transition
	ErrInvalidStateTransition = errors.NewFederationError(errors.CodeInvalidStateTransition, "invalid circuit breaker state transition")
)
