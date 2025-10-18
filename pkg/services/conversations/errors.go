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
)
