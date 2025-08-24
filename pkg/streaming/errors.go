package streaming

import "errors"

// Streaming-specific errors
var (
	// Connection errors
	ErrConnectionWriteFailed   = errors.New("failed to write connection")
	ErrConnectionDeleteFailed  = errors.New("failed to delete connection")
	ErrAPIGatewayClientNotInit = errors.New("API Gateway client not initialized")

	// Message errors
	ErrConfirmationSendFailed = errors.New("failed to send confirmation")
	ErrPongSendFailed         = errors.New("failed to send pong")
	ErrErrorMessageSendFailed = errors.New("failed to send error message")

	// Command validation errors
	ErrCommandIDRequired       = errors.New("command id is required")
	ErrCommandIDMustBeString   = errors.New("command id must be a string")
	ErrCommandTypeRequired     = errors.New("command type is required")
	ErrCommandTypeMustBeString = errors.New("command type must be a string")
)
