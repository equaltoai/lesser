// Package main provides standardized error constants for the enhanced federation processor
package main

import "errors"

// AWS-related errors
var (
	// ErrDynamORMInit indicates failure to initialize DynamORM client
	ErrDynamORMInit = errors.New("failed to initialize DynamORM")
)

// Processing-related errors
var (
	// ErrUnmarshalRetryMessage indicates failure to unmarshal retry message from SQS
	ErrUnmarshalRetryMessage = errors.New("failed to unmarshal retry message")

	// ErrProcessEnhancedRetry indicates failure to process enhanced retry operation
	ErrProcessEnhancedRetry = errors.New("failed to process enhanced retry")
)
