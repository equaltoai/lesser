package lambda

import "github.com/equaltoai/lesser/pkg/errors"

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// Message processing errors
	ErrInvalidMessageFormat = errors.InvalidFormat("message", "valid federation message format")
	ErrMissingActivity      = errors.RequiredFieldMissing("activity")
	ErrMissingActor         = errors.RequiredFieldMissing("actor")
	ErrMissingTargetInbox   = errors.RequiredFieldMissing("target inbox")

	// Delivery errors
	ErrDeliveryToFollowers  = errors.DeliveryFailed("followers", nil)
	ErrDeliveryToRecipients = errors.DeliveryFailed("recipients", nil)
)
