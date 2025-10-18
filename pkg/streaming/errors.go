package streaming

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// Connection errors
	ErrConnectionWriteFailed   = errors.FailedToSave("connection", stdErrors.New("failed to write to connection"))
	ErrConnectionDeleteFailed  = errors.FailedToDelete("connection", stdErrors.New("failed to delete connection"))
	ErrAPIGatewayClientNotInit = errors.ServiceUnavailable("API Gateway client")

	// Message errors
	ErrConfirmationSendFailed = errors.ProcessingFailed("confirmation send", stdErrors.New("failed to send confirmation"))
	ErrPongSendFailed         = errors.ProcessingFailed("pong send", stdErrors.New("failed to send pong"))
	ErrErrorMessageSendFailed = errors.ProcessingFailed("error message send", stdErrors.New("failed to send error message"))

	// Command validation errors
	ErrCommandIDRequired       = errors.ValidationFailedWithField("command id is required")
	ErrCommandIDMustBeString   = errors.ValidationFailedWithField("command id must be a string")
	ErrCommandTypeRequired     = errors.ValidationFailedWithField("command type is required")
	ErrCommandTypeMustBeString = errors.ValidationFailedWithField("command type must be a string")
)
