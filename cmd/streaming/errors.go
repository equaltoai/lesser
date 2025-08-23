package main

import "errors"

// CMD layer error constants for streaming service
// These are used for client-facing error messages and command-specific errors
var (
	// Client message format errors
	ErrInvalidMessageFormat = errors.New("Invalid message format")
	ErrConnectionNotFound   = errors.New("Connection not found")
	ErrUnknownMessageType   = errors.New("Unknown message type")
	
	// Stream validation errors  
	ErrInvalidStream           = errors.New("Invalid stream")
	ErrAuthenticationRequired  = errors.New("Authentication required for stream")
	ErrFailedToSubscribe      = errors.New("Failed to subscribe")
	ErrFailedToUnsubscribe    = errors.New("Failed to unsubscribe")
	
	// Command processing errors
	ErrInvalidCommandFormat   = errors.New("Invalid command format")
	ErrCommandExecutionFailed = errors.New("Command execution failed")
	
	// WebSocket route errors
	ErrUnknownRoute = errors.New("Unknown WebSocket route")
)