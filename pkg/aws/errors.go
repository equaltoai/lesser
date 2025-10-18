package awsinit

import "github.com/equaltoai/lesser/pkg/errors"

// Error constants for AWS initialization failures
var (
	// ErrLoggerRequired indicates that a logger parameter was nil or not provided
	ErrLoggerRequired = errors.LoggerRequired()
)
