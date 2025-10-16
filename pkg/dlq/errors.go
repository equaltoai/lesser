package dlq

import (
	stdErrors "errors"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Legacy error variables for backwards compatibility
// These are now wrappers around the centralized error system
var (
	// ErrBatchProcessingFailed indicates that batch processing failed with some messages failing
	ErrBatchProcessingFailed = errors.BatchOperationFailed("DLQ processing", nil)

	// ErrNoDLQMessagesProcessed indicates that all DLQ messages failed to process
	ErrNoDLQMessagesProcessed = errors.ProcessingFailed("DLQ messages", stdErrors.New("no DLQ messages processed"))

	// Validation errors
	ErrMissingRequiredField         = errors.RequiredFieldMissing("field")
	ErrChannelsMustBeArray          = errors.NewValidationError("channels", "Channels must be an array")
	ErrMissingActivityPubType       = errors.RequiredFieldMissing("ActivityPub type")
	ErrActivityPubTypeMustBeString  = errors.NewValidationError("type", "ActivityPub type must be a string")
	ErrMissingActivityPubActor      = errors.RequiredFieldMissing("ActivityPub actor")
	ErrActivityPubActorMustBeString = errors.NewValidationError("actor", "ActivityPub actor must be a string")
	ErrInvalidAction                = errors.NewValidationError("action", "Invalid action")

	// URL validation errors
	ErrInvalidMediaURL       = errors.InvalidFormat("media_url", "valid URL format")
	ErrInvalidMediaURLFormat = errors.InvalidFormat("media_url", "valid URL format")
	ErrInvalidInboxURL       = errors.InvalidFormat("inbox_url", "valid URL format")
	ErrInvalidInboxURLFormat = errors.InvalidFormat("inbox_url", "valid URL format")

	// Media accessibility errors
	ErrMediaPermanentlyUnavailable = errors.ResourceUnavailable("media")
	ErrMediaAccessDenied           = errors.AccessDeniedForResource("media", "")
	ErrMediaValidationFailed       = errors.MediaAttachmentValidationFailed("non-retryable validation error")
)
