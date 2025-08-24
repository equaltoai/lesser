package streaming

import "github.com/equaltoai/lesser/pkg/errors"

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// Connection errors
	ErrConnectionWriteFailed   = errors.FailedToSave("connection", nil)
	ErrConnectionDeleteFailed  = errors.FailedToDelete("connection", nil)
	ErrAPIGatewayClientNotInit = errors.ServiceNotAvailable("API Gateway client")

	// Message errors
	ErrConfirmationSendFailed = errors.ProcessingFailed("confirmation send", nil)
	ErrPongSendFailed         = errors.ProcessingFailed("pong send", nil)
	ErrErrorMessageSendFailed = errors.ProcessingFailed("error message send", nil)

	// Command validation errors
	ErrCommandIDRequired       = errors.ValidationFailedWithField("command id is required")
	ErrCommandIDMustBeString   = errors.ValidationFailedWithField("command id must be a string")
	ErrCommandTypeRequired     = errors.ValidationFailedWithField("command type is required")
	ErrCommandTypeMustBeString = errors.ValidationFailedWithField("command type must be a string")
)
