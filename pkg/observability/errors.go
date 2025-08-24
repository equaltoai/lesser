// Package observability provides error constants for the observability package
package observability

import "errors"

// Error constants for alerting system
var (
	// ErrLoggerRequired indicates that a logger instance is required
	ErrLoggerRequired = errors.New("logger is required")

	// ErrDatabaseRequired indicates that a database connection is required
	ErrDatabaseRequired = errors.New("database connection is required")

	// ErrSNSPublishFailed indicates that publishing to SNS failed
	ErrSNSPublishFailed = errors.New("failed to publish to SNS")
)
