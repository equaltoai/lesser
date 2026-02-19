package conversations

import (
	"errors"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
)

// Conversation service errors
var (
	// ErrConversationValidationFailed is returned when conversation validation fails
	ErrConversationValidationFailed = apperrors.ValidationFailedWithField("conversation")

	// ErrGetSenderAccount is returned when getting sender account fails
	ErrGetSenderAccount = apperrors.FailedToGet("sender account", errors.New("failed to get sender account"))

	// ErrInvalidRecipient is returned when recipient validation fails
	ErrInvalidRecipient = apperrors.NewValidationError("recipient", "invalid")

	// ErrLookupExistingConversation is returned when looking up existing conversation fails
	ErrLookupExistingConversation = apperrors.FailedToQuery("existing conversation", errors.New("failed to lookup existing conversation"))

	// ErrCreateConversation is returned when conversation creation fails
	ErrCreateConversation = apperrors.FailedToCreate("conversation", errors.New("failed to create conversation"))

	// ErrCreateDirectMessage is returned when direct message creation fails
	ErrCreateDirectMessage = apperrors.FailedToCreate("direct message", errors.New("failed to create direct message"))

	// ErrGetConversation is returned when conversation retrieval fails
	ErrGetConversation = apperrors.FailedToGet("conversation", errors.New("failed to get conversation"))

	// ErrNotConversationParticipant is returned when user is not a participant in conversation
	ErrNotConversationParticipant = apperrors.AccessDeniedForResource("conversation", "participant")

	// ErrMarkConversationRead is returned when marking conversation as read fails
	ErrMarkConversationRead = apperrors.FailedToUpdate("conversation read status", errors.New("failed to mark conversation as read"))

	// ErrGetUserConversations is returned when getting user conversations fails
	ErrGetUserConversations = apperrors.FailedToList("user conversations", errors.New("failed to get user conversations"))

	// ErrGetConversationMessages is returned when getting conversation messages fails
	ErrGetConversationMessages = apperrors.FailedToList("conversation messages", errors.New("failed to get conversation messages"))

	// ErrRecipientsRequired is returned when recipients list is required but empty
	ErrRecipientsRequired = apperrors.NewValidationError("recipients", "required")

	// ErrDirectMessageRequiresSingleRecipient is returned when the DM command contains multiple recipients (group chats not supported in v1).
	ErrDirectMessageRequiresSingleRecipient = apperrors.NewValidationError("recipients", "must contain exactly 1 recipient")

	// ErrConversationMustBeOneToOne is returned when a conversation has an unexpected participant count.
	ErrConversationMustBeOneToOne = apperrors.NewValidationError("conversation", "must be 1:1")

	// ErrContentTooLongConversation is returned when conversation content is too long
	ErrContentTooLongConversation = apperrors.NewValidationError("content", "too long (max 5000 characters)")

	// ErrInvalidInReplyToIDConversation is returned when in_reply_to_id is invalid for conversation
	ErrInvalidInReplyToIDConversation = apperrors.NewValidationError("in_reply_to_id", "invalid")

	// ErrCanOnlyReplyToDirectMessages is returned when attempting to reply to non-direct message
	ErrCanOnlyReplyToDirectMessages = apperrors.NewValidationError("reply_target", "can only reply to direct messages")

	// ErrConversationNotFound is returned when conversation is not found
	ErrConversationNotFound = apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "conversation not found")

	// ErrGetAccount is returned when account retrieval fails
	ErrGetAccount = apperrors.FailedToGet("account", errors.New("failed to get account"))

	// ErrDeleteConversation is returned when conversation deletion fails
	ErrDeleteConversation = apperrors.FailedToDelete("conversation", errors.New("failed to delete conversation"))

	// ErrDeleteMessage is returned when message deletion (delete-for-me tombstone) fails.
	ErrDeleteMessage = apperrors.FailedToDelete("direct message", errors.New("failed to delete message"))

	// ErrDirectMessageBlocked is returned when a DM cannot be sent due to a block.
	//
	// Note: intentionally generic to avoid leaking block direction.
	ErrDirectMessageBlocked = apperrors.AccessDeniedForResource("direct message", "blocked")

	// ErrMessageRequestPending is returned when a sender attempts to send additional messages
	// before a message request has been accepted.
	ErrMessageRequestPending = apperrors.NewAppError(apperrors.CodeForbidden, apperrors.CategoryBusiness, "Message request pending")

	// ErrMessageRequestMediaNotAllowed is returned when a DM request includes attachments.
	ErrMessageRequestMediaNotAllowed = apperrors.NewValidationError("media", "not allowed in message requests")
)
