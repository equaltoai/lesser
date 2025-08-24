package awsinit

import "errors"

// Error constants for AWS initialization failures
var (
	// ErrLoggerRequired indicates that a logger parameter was nil or not provided
	ErrLoggerRequired = errors.New("logger is required")
)
