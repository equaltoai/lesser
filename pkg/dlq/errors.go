package dlq

import "errors"

// Error constants for DLQ package
var (
	// ErrBatchProcessingFailed indicates that batch processing failed with some messages failing
	ErrBatchProcessingFailed = errors.New("batch processing failed")
	
	// ErrNoDLQMessagesProcessed indicates that all DLQ messages failed to process
	ErrNoDLQMessagesProcessed = errors.New("failed to process any DLQ messages")

	// Validation errors
	ErrMissingRequiredField       = errors.New("missing required field")
	ErrChannelsMustBeArray        = errors.New("channels must be an array")
	ErrMissingActivityPubType     = errors.New("missing ActivityPub type field")
	ErrActivityPubTypeMustBeString = errors.New("ActivityPub type must be a string")
	ErrMissingActivityPubActor    = errors.New("missing ActivityPub actor field")
	ErrActivityPubActorMustBeString = errors.New("ActivityPub actor must be a string")
	ErrInvalidAction              = errors.New("invalid action")

	// URL validation errors
	ErrInvalidMediaURL        = errors.New("invalid media URL")
	ErrInvalidMediaURLFormat  = errors.New("invalid media URL format")
	ErrInvalidInboxURL        = errors.New("invalid inbox URL")
	ErrInvalidInboxURLFormat  = errors.New("invalid inbox URL format")

	// Media accessibility errors
	ErrMediaPermanentlyUnavailable = errors.New("media permanently unavailable")
	ErrMediaAccessDenied          = errors.New("media access denied")
	ErrMediaValidationFailed      = errors.New("media validation failed with non-retryable error")
)