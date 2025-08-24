package main

import "errors"

// CMD layer error constants for streaming service
// These are used for client-facing error messages and command-specific errors
var (
	// Client message format errors
	ErrInvalidMessageFormat = errors.New("invalid message format")
	ErrConnectionNotFound   = errors.New("connection not found")
	ErrUnknownMessageType   = errors.New("unknown message type")

	// Stream validation errors
	ErrInvalidStream          = errors.New("invalid stream")
	ErrAuthenticationRequired = errors.New("authentication required for stream")
	ErrFailedToSubscribe      = errors.New("failed to subscribe")
	ErrFailedToUnsubscribe    = errors.New("failed to unsubscribe")

	// Command processing errors
	ErrInvalidCommandFormat   = errors.New("invalid command format")
	ErrCommandExecutionFailed = errors.New("command execution failed")

	// WebSocket route errors
	ErrUnknownRoute = errors.New("unknown WebSocket route")
)
