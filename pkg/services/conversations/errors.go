package conversations

import "errors"

// Conversation service errors
var (
	// ErrConversationValidationFailed is returned when conversation validation fails
	ErrConversationValidationFailed = errors.New("validation failed")

	// ErrGetSenderAccount is returned when getting sender account fails
	ErrGetSenderAccount = errors.New("failed to get sender account")

	// ErrInvalidRecipient is returned when recipient validation fails
	ErrInvalidRecipient = errors.New("invalid recipient")

	// ErrLookupExistingConversation is returned when looking up existing conversation fails
	ErrLookupExistingConversation = errors.New("failed to lookup existing conversation")

	// ErrCreateConversation is returned when conversation creation fails
	ErrCreateConversation = errors.New("failed to create conversation")

	// ErrCreateDirectMessage is returned when direct message creation fails
	ErrCreateDirectMessage = errors.New("failed to create direct message")

	// ErrGetConversation is returned when conversation retrieval fails
	ErrGetConversation = errors.New("failed to get conversation")

	// ErrNotConversationParticipant is returned when user is not a participant in conversation
	ErrNotConversationParticipant = errors.New("user is not a participant in this conversation")

	// ErrMarkConversationRead is returned when marking conversation as read fails
	ErrMarkConversationRead = errors.New("failed to mark conversation as read")

	// ErrGetUserConversations is returned when getting user conversations fails
	ErrGetUserConversations = errors.New("failed to get user conversations")

	// ErrGetConversationMessages is returned when getting conversation messages fails
	ErrGetConversationMessages = errors.New("failed to get conversation messages")

	// ErrRecipientsRequired is returned when recipients list is required but empty
	ErrRecipientsRequired = errors.New("recipients is required")

	// ErrContentTooLongConversation is returned when conversation content is too long
	ErrContentTooLongConversation = errors.New("content too long (max 5000 characters)")

	// ErrInvalidInReplyToIDConversation is returned when in_reply_to_id is invalid for conversation
	ErrInvalidInReplyToIDConversation = errors.New("invalid in_reply_to_id")

	// ErrCanOnlyReplyToDirectMessages is returned when attempting to reply to non-direct message
	ErrCanOnlyReplyToDirectMessages = errors.New("can only reply to direct messages")

	// ErrConversationNotFound is returned when conversation is not found
	ErrConversationNotFound = errors.New("conversation not found")

	// ErrGetAccount is returned when account retrieval fails
	ErrGetAccount = errors.New("failed to get account")

	// ErrDeleteConversation is returned when conversation deletion fails
	ErrDeleteConversation = errors.New("failed to delete conversation")
)