package lambda

import "errors"

// Federation delivery error constants
var (
	// Message processing errors
	ErrInvalidMessageFormat = errors.New("invalid message format")
	ErrMissingActivity      = errors.New("missing activity in message")
	ErrMissingActor         = errors.New("missing actor in message")
	ErrMissingTargetInbox   = errors.New("missing target inbox in message")

	// Delivery errors
	ErrDeliveryToFollowers  = errors.New("failed to deliver to followers")
	ErrDeliveryToRecipients = errors.New("failed to deliver to recipients")
)
