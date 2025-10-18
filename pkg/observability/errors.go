// Package observability provides error constants for the observability package
package observability

import "github.com/equaltoai/lesser/pkg/errors"

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// ErrLoggerRequired indicates that a logger instance is required
	ErrLoggerRequired = errors.LoggerRequired()

	// ErrDatabaseRequired indicates that a database connection is required
	ErrDatabaseRequired = errors.DatabaseRequired()

	// ErrSNSPublishFailed indicates that publishing to SNS failed
	ErrSNSPublishFailed = errors.SNSPublishFailed(nil)
)
