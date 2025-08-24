package conversations

import "github.com/equaltoai/lesser/pkg/errors"

// Conversation service errors
var (
	// ErrConversationValidationFailed is returned when conversation validation fails
	ErrConversationValidationFailed = errors.ValidationFailedWithField("conversation")

	// ErrGetSenderAccount is returned when getting sender account fails
	ErrGetSenderAccount = errors.FailedToGet("sender account", nil)

	// ErrInvalidRecipient is returned when recipient validation fails
	ErrInvalidRecipient = errors.NewValidationError("recipient", "invalid")

	// ErrLookupExistingConversation is returned when looking up existing conversation fails
	ErrLookupExistingConversation = errors.FailedToQuery("existing conversation", nil)

	// ErrCreateConversation is returned when conversation creation fails
	ErrCreateConversation = errors.FailedToCreate("conversation", nil)

	// ErrCreateDirectMessage is returned when direct message creation fails
	ErrCreateDirectMessage = errors.FailedToCreate("direct message", nil)

	// ErrGetConversation is returned when conversation retrieval fails
	ErrGetConversation = errors.FailedToGet("conversation", nil)

	// ErrNotConversationParticipant is returned when user is not a participant in conversation
	ErrNotConversationParticipant = errors.AccessDeniedForResource("conversation", "participant")

	// ErrMarkConversationRead is returned when marking conversation as read fails
	ErrMarkConversationRead = errors.FailedToUpdate("conversation read status", nil)

	// ErrGetUserConversations is returned when getting user conversations fails
	ErrGetUserConversations = errors.FailedToList("user conversations", nil)

	// ErrGetConversationMessages is returned when getting conversation messages fails
	ErrGetConversationMessages = errors.FailedToList("conversation messages", nil)

	// ErrRecipientsRequired is returned when recipients list is required but empty
	ErrRecipientsRequired = errors.NewValidationError("recipients", "required")

	// ErrContentTooLongConversation is returned when conversation content is too long
	ErrContentTooLongConversation = errors.NewValidationError("content", "too long (max 5000 characters)")

	// ErrInvalidInReplyToIDConversation is returned when in_reply_to_id is invalid for conversation
	ErrInvalidInReplyToIDConversation = errors.NewValidationError("in_reply_to_id", "invalid")

	// ErrCanOnlyReplyToDirectMessages is returned when attempting to reply to non-direct message
	ErrCanOnlyReplyToDirectMessages = errors.NewValidationError("reply_target", "can only reply to direct messages")

	// ErrConversationNotFound is returned when conversation is not found
	ErrConversationNotFound = errors.NewAppError(errors.CodeNotFound, errors.CategoryBusiness, "conversation not found")

	// ErrGetAccount is returned when account retrieval fails
	ErrGetAccount = errors.FailedToGet("account", nil)

	// ErrDeleteConversation is returned when conversation deletion fails
	ErrDeleteConversation = errors.FailedToDelete("conversation", nil)
)
