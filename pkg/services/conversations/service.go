// Package conversations provides the core Conversations Service for the Lesser project's API alignment.
// This service handles direct message conversations including creation, participant management,
// read status tracking, and real-time event emission. It supports federation for remote participants
// and maintains privacy by ensuring only participants can access conversation content.
package conversations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// VisibilityDirect represents direct message visibility
	VisibilityDirect = "direct"

	// ConversationMessageEvent is emitted when a message is sent to a conversation
	ConversationMessageEvent = "conversation.message"

	// ConversationReadEvent is emitted when a conversation is marked as read
	ConversationReadEvent = "conversation.read"

	// ConversationUpdatedEvent is emitted when conversation metadata is updated
	ConversationUpdatedEvent = "conversation.updated"
)

// Service provides conversation operations for direct messages
type Service struct {
	conversationRepo interfaces.ConversationRepository
	noteRepo         interfaces.StatusRepository
	accountRepo      interfaces.AccountRepository
	relationshipRepo interfaces.ConcreteRelationshipRepository
	userRepo         interfaces.UserRepository
	publisher        streaming.Publisher
	federation       FederationService
	logger           *zap.Logger
	domainName       string
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

// NewService creates a new Conversations Service with the required dependencies
func NewService(
	conversationRepo interfaces.ConversationRepository,
	noteRepo interfaces.StatusRepository,
	accountRepo interfaces.AccountRepository,
	relationshipRepo interfaces.ConcreteRelationshipRepository,
	userRepo interfaces.UserRepository,
	publisher streaming.Publisher,
	federation FederationService,
	logger *zap.Logger,
	domainName string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		conversationRepo: conversationRepo,
		noteRepo:         noteRepo,
		accountRepo:      accountRepo,
		relationshipRepo: relationshipRepo,
		userRepo:         userRepo,
		publisher:        publisher,
		federation:       federation,
		logger:           logger,
		domainName:       domainName,
	}
}

// Command structs for operations

// SendDirectMessageCommand contains all data needed to send a direct message
type SendDirectMessageCommand struct {
	SenderID    string   `json:"sender_id" validate:"required"`
	Recipients  []string `json:"recipients" validate:"required,min=1"`
	Content     string   `json:"content" validate:"required,max=5000"`
	Sensitive   bool     `json:"sensitive"`
	Language    string   `json:"language"`
	MediaIDs    []string `json:"media_ids"`
	InReplyToID string   `json:"in_reply_to_id"` // Can reply to messages in the conversation
}

// MarkConversationReadCommand contains data needed to mark a conversation as read
type MarkConversationReadCommand struct {
	ConversationID string `json:"conversation_id" validate:"required"`
	UserID         string `json:"user_id" validate:"required"`
}

// DeleteConversationCommand contains data needed to delete a conversation
type DeleteConversationCommand struct {
	ConversationID string `json:"conversation_id" validate:"required"`
	UserID         string `json:"user_id" validate:"required"`
}

// CreateConversationCommand contains data needed to create a 1:1 conversation.
type CreateConversationCommand struct {
	CreatorID     string `json:"creator_id" validate:"required"`
	ParticipantID string `json:"participant_id" validate:"required"`
}

// GetConversationQuery contains parameters for retrieving a conversation
type GetConversationQuery struct {
	ConversationID string                       `json:"conversation_id" validate:"required"`
	ViewerID       string                       `json:"viewer_id" validate:"required"` // Must be a participant
	Pagination     interfaces.PaginationOptions `json:"pagination"`
}

// SendMessageCommand contains all data needed to send a message to an existing 1:1 conversation.
type SendMessageCommand struct {
	SenderID       string   `json:"sender_id" validate:"required"`
	ConversationID string   `json:"conversation_id" validate:"required"`
	Content        string   `json:"content" validate:"required,max=5000"`
	Sensitive      bool     `json:"sensitive"`
	Language       string   `json:"language"`
	MediaIDs       []string `json:"media_ids"`
	InReplyToID    string   `json:"in_reply_to_id"`
}

// AcceptMessageRequestCommand contains data needed to accept a pending message request.
type AcceptMessageRequestCommand struct {
	ConversationID string `json:"conversation_id" validate:"required"`
	UserID         string `json:"user_id" validate:"required"`
}

// DeclineMessageRequestCommand contains data needed to decline a pending message request.
type DeclineMessageRequestCommand struct {
	ConversationID string `json:"conversation_id" validate:"required"`
	UserID         string `json:"user_id" validate:"required"`
}

// ConversationFolder indicates whether a thread appears in the viewer's inbox or requests list.
type ConversationFolder string

const (
	ConversationFolderInbox    ConversationFolder = "INBOX"
	ConversationFolderRequests ConversationFolder = "REQUESTS"
)

// ListConversationsQuery contains parameters for listing user conversations
type ListConversationsQuery struct {
	UserID     string                       `json:"user_id" validate:"required"`
	Pagination interfaces.PaginationOptions `json:"pagination"`
	Folder     ConversationFolder           `json:"folder"`
	OnlyUnread bool                         `json:"only_unread"`
}

// Result structs for operations

// ConversationResult contains a conversation and associated events
type ConversationResult struct {
	Conversation *models.Conversation `json:"conversation"`
	Events       []*streaming.Event   `json:"events"`
}

// MessageResult contains a direct message and associated events
type MessageResult struct {
	Message      *models.Status       `json:"message"`
	Conversation *models.Conversation `json:"conversation"`
	Events       []*streaming.Event   `json:"events"`
}

// ConversationWithMessages contains a conversation and its message history
type ConversationWithMessages struct {
	Conversation *models.Conversation                        `json:"conversation"`
	Messages     *interfaces.PaginatedResult[*models.Status] `json:"messages"`
	Events       []*streaming.Event                          `json:"events"`
}

// Result contains multiple conversations with pagination
type Result struct {
	Conversations *interfaces.PaginatedResult[*models.Conversation] `json:"conversations"`
	Events        []*streaming.Event                                `json:"events"`
}

// CreateConversation creates (or returns) a 1:1 conversation between the caller and another participant.
func (s *Service) CreateConversation(ctx context.Context, cmd *CreateConversationCommand) (*ConversationResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	creatorID := strings.TrimSpace(cmd.CreatorID)
	participantID := strings.TrimSpace(cmd.ParticipantID)
	if creatorID == "" || participantID == "" || creatorID == participantID {
		return nil, errors.Join(ErrConversationValidationFailed, ErrInvalidRecipient)
	}

	if _, err := s.accountRepo.GetAccount(ctx, creatorID); err != nil {
		return nil, errors.Join(ErrGetSenderAccount, err)
	}
	if _, err := s.accountRepo.GetAccount(ctx, participantID); err != nil {
		return nil, errors.Join(ErrInvalidRecipient, err)
	}

	participants := []string{creatorID, participantID}
	sort.Strings(participants)

	conversation, err := s.conversationRepo.GetConversationByParticipants(ctx, participants)
	if err != nil && !isNotFoundError(err) {
		return nil, errors.Join(ErrLookupExistingConversation, err)
	}

	created := false
	if conversation == nil {
		created = true
		conversationID := uuid.New().String()
		conversation = &models.Conversation{
			ID:           conversationID,
			Participants: participants,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		if err := s.conversationRepo.CreateConversation(ctx, conversation, participants); err != nil {
			return nil, errors.Join(ErrCreateConversation, err)
		}
	}

	// Ensure per-user request state is initialized consistently for a new conversation.
	now := time.Now().UTC()
	if err := s.updateParticipantRecord(ctx, conversation.ID, creatorID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}
		record.RequestState = models.DmRequestStateAccepted
		record.DeclinedAt = nil
		if record.AcceptedAt == nil {
			t := now
			record.AcceptedAt = &t
		}
	}); err != nil {
		s.logger.Warn("failed to update creator participant record for conversation create",
			zap.String("conversation_id", conversation.ID),
			zap.String("creator_id", creatorID),
			zap.Error(err))
	}

	if err := s.updateParticipantRecord(ctx, conversation.ID, participantID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}

		// Creating a conversation (without sending a message) should not surface anything to the other
		// participant. We keep their side hidden until the first inbound DM arrives.
		if !created {
			return
		}

		record.RequestState = models.DmRequestStateDeclined
		record.RequestedAt = nil
		record.AcceptedAt = nil
		t := now
		record.DeclinedAt = &t
	}); err != nil {
		s.logger.Warn("failed to update participant record for conversation create",
			zap.String("conversation_id", conversation.ID),
			zap.String("participant_id", participantID),
			zap.Error(err))
	}

	// Populate per-viewer unread state.
	if record, err := s.conversationRepo.GetConversationParticipantRecord(ctx, conversation.ID, creatorID); err == nil && record != nil {
		conversation.Unread = record.Unread
	}

	return &ConversationResult{
		Conversation: conversation,
		Events:       []*streaming.Event{},
	}, nil
}

// SendDirectMessage creates or updates a conversation, creates a direct message, and emits events
func (s *Service) SendDirectMessage(ctx context.Context, cmd *SendDirectMessageCommand) (*MessageResult, error) {
	s.logger.Info("sending direct message",
		zap.String("sender_id", cmd.SenderID),
		zap.Strings("recipients", cmd.Recipients),
		zap.Int("content_length", len(cmd.Content)))

	// Validate the command (basic validation only - accounts validated below)
	if err := s.validateSendMessageCommandBasic(ctx, cmd); err != nil {
		s.logger.Error("validation failed", zap.Error(err))
		return nil, errors.Join(ErrConversationValidationFailed, err)
	}

	if len(cmd.Recipients) != 1 {
		return nil, errors.Join(ErrConversationValidationFailed, ErrDirectMessageRequiresSingleRecipient)
	}
	recipientID := cmd.Recipients[0]
	if strings.TrimSpace(recipientID) == "" || recipientID == cmd.SenderID {
		return nil, errors.Join(ErrConversationValidationFailed, ErrInvalidRecipient)
	}

	// Get sender account - also validates it exists
	sender, err := s.accountRepo.GetAccount(ctx, cmd.SenderID)
	if err != nil {
		s.logger.Error("failed to get sender account", zap.String("sender_id", cmd.SenderID), zap.Error(err))
		return nil, errors.Join(ErrGetSenderAccount, err)
	}

	// Get recipient account - also validates it exists
	recipientAccounts := make(map[string]*storage.Account, 1)
	recipient, err := s.accountRepo.GetAccount(ctx, recipientID)
	if err != nil {
		s.logger.Error("invalid recipient", zap.String("recipient_id", recipientID), zap.Error(err))
		return nil, errors.Join(ErrInvalidRecipient, err)
	}
	recipientAccounts[recipientID] = recipient

	// Create participant list (sender + recipients)
	allParticipants := append([]string{cmd.SenderID}, cmd.Recipients...)
	sort.Strings(allParticipants) // Ensure consistent ordering for lookup

	// Try to find existing conversation with these exact participants
	conversation, err := s.conversationRepo.GetConversationByParticipants(ctx, allParticipants)
	if err != nil && !isNotFoundError(err) {
		s.logger.Error("failed to lookup existing conversation", zap.Strings("participants", allParticipants), zap.Error(err))
		return nil, errors.Join(ErrLookupExistingConversation, err)
	}

	// Create new conversation if none exists
	if conversation == nil {
		conversationID := uuid.New().String()
		conversation = &models.Conversation{
			ID:           conversationID,
			Participants: allParticipants,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		if err := s.conversationRepo.CreateConversation(ctx, conversation, allParticipants); err != nil {
			s.logger.Error("failed to create conversation", zap.String("conversation_id", conversationID), zap.Error(err))
			return nil, errors.Join(ErrCreateConversation, err)
		}

		s.logger.Info("created new conversation",
			zap.String("conversation_id", conversationID),
			zap.Strings("participants", allParticipants))
	}

	// Generate unique message ID
	messageID := uuid.New().String()

	// Create direct message as a status with direct visibility
	status := &models.Status{
		StatusID:       messageID,
		AuthorID:       cmd.SenderID,
		AuthorUsername: sender.User.Username,
		Content:        cmd.Content,
		Visibility:     VisibilityDirect,
		Sensitive:      cmd.Sensitive,
		Language:       cmd.Language,
		ConversationID: conversation.ID,
		InReplyToID:    cmd.InReplyToID,
		PublishedAt:    time.Now(),
	}

	// Set direct message recipients in To field for ActivityPub
	recipientURLs := make([]string, 0, len(cmd.Recipients))
	for _, recipientID := range cmd.Recipients {
		recipient := recipientAccounts[recipientID]
		recipientURLs = append(recipientURLs, fmt.Sprintf("https://%s/users/%s", s.domainName, recipient.User.Username))
	}
	status.ToRecipients = recipientURLs

	// Create ActivityPub Note
	status.Note = s.buildActivityPubNote(cmd, messageID, sender, conversation.ID, recipientAccounts)

	// Store the message
	if err := s.noteRepo.CreateStatus(ctx, status); err != nil {
		s.logger.Error("failed to create direct message", zap.String("message_id", messageID), zap.Error(err))
		return nil, errors.Join(ErrCreateDirectMessage, err)
	}

	// Update conversation with latest message
	conversation.LastStatusID = messageID
	conversation.UpdatedAt = time.Now()
	if err := s.conversationRepo.UpdateConversation(ctx, conversation); err != nil {
		s.logger.Warn("failed to update conversation", zap.Error(err))
		// Don't fail the whole operation for this
	}

	// Update DM request lifecycle state.
	now := time.Now().UTC()
	if err := s.updateParticipantRecord(ctx, conversation.ID, cmd.SenderID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}
		// Sending (or replying) always places the sender-side view in Inbox.
		record.RequestState = models.DmRequestStateAccepted
		if record.AcceptedAt == nil {
			t := now
			record.AcceptedAt = &t
		}
		record.DeclinedAt = nil
	}); err != nil {
		s.logger.Warn("failed to update sender DM request state",
			zap.String("conversation_id", conversation.ID),
			zap.String("sender_id", cmd.SenderID),
			zap.Error(err))
	}

	if err := s.updateParticipantRecord(ctx, conversation.ID, recipientID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}
		switch record.RequestState {
		case models.DmRequestStateAccepted:
			// No-op: already accepted.
			return
		case models.DmRequestStateDeclined:
			// v1 semantics: declined threads stay hidden; the next inbound message reopens as a new request.
			record.RequestState = models.DmRequestStatePending
			record.AcceptedAt = nil
			record.DeclinedAt = nil
			t := now
			record.RequestedAt = &t
			return
		default:
			// PENDING or unset -> apply inbox policy.
			if s.shouldDeliverToInbox(ctx, recipientID, cmd.SenderID) {
				record.RequestState = models.DmRequestStateAccepted
				record.RequestedAt = nil
				t := now
				record.AcceptedAt = &t
				record.DeclinedAt = nil
				return
			}
			record.RequestState = models.DmRequestStatePending
			if record.RequestedAt == nil {
				t := now
				record.RequestedAt = &t
			}
			return
		}
	}); err != nil {
		s.logger.Warn("failed to update recipient DM request state",
			zap.String("conversation_id", conversation.ID),
			zap.String("recipient_id", recipientID),
			zap.Error(err))
	}

	// Update per-user unread state.
	if err := s.conversationRepo.MarkConversationRead(ctx, conversation.ID, cmd.SenderID); err != nil {
		s.logger.Warn("failed to mark DM conversation read for sender",
			zap.String("conversation_id", conversation.ID),
			zap.String("sender_id", cmd.SenderID),
			zap.Error(err))
	} else {
		conversation.Unread = false
	}

	if err := s.conversationRepo.MarkConversationUnread(ctx, conversation.ID, recipientID); err != nil {
		s.logger.Warn("failed to mark DM conversation unread for recipient",
			zap.String("conversation_id", conversation.ID),
			zap.String("recipient_id", recipientID),
			zap.Error(err))
	}

	s.logger.Info("sent direct message successfully",
		zap.String("message_id", messageID),
		zap.String("conversation_id", conversation.ID))

	// Emit events and queue federation
	events := s.emitMessageSentEvents(ctx, status, conversation)
	s.queueFederationDelivery(ctx, status)

	return &MessageResult{
		Message:      status,
		Conversation: conversation,
		Events:       events,
	}, nil
}

// SendMessage sends a message in an existing 1:1 conversation, enforcing strict participant authz.
func (s *Service) SendMessage(ctx context.Context, cmd *SendMessageCommand) (*MessageResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	// Load conversation and enforce participant access.
	conversation, err := s.conversationRepo.GetConversation(ctx, cmd.ConversationID)
	if err != nil {
		return nil, errors.Join(ErrGetConversation, err)
	}

	if len(conversation.Participants) != 2 {
		return nil, errors.Join(ErrConversationValidationFailed, ErrConversationMustBeOneToOne)
	}
	if !s.isParticipant(cmd.SenderID, conversation.Participants) {
		return nil, ErrNotConversationParticipant
	}

	recipientID := ""
	for _, p := range conversation.Participants {
		if p != cmd.SenderID {
			recipientID = p
			break
		}
	}
	if strings.TrimSpace(recipientID) == "" {
		return nil, errors.Join(ErrConversationValidationFailed, ErrInvalidRecipient)
	}

	sendCmd := &SendDirectMessageCommand{
		SenderID:    cmd.SenderID,
		Recipients:  []string{recipientID},
		Content:     cmd.Content,
		Sensitive:   cmd.Sensitive,
		Language:    cmd.Language,
		MediaIDs:    cmd.MediaIDs,
		InReplyToID: cmd.InReplyToID,
	}
	if err := s.validateSendMessageCommandBasic(ctx, sendCmd); err != nil {
		return nil, errors.Join(ErrConversationValidationFailed, err)
	}

	// If replying, ensure the parent is in the same conversation.
	if cmd.InReplyToID != "" {
		parentMessage, err := s.noteRepo.GetStatus(ctx, cmd.InReplyToID)
		if err != nil {
			return nil, errors.Join(ErrInvalidInReplyToIDConversation, err)
		}
		if parentMessage.ConversationID != "" && parentMessage.ConversationID != conversation.ID {
			return nil, errors.Join(ErrInvalidInReplyToIDConversation, errors.New("reply target is in a different conversation"))
		}
	}

	// Get sender and recipient accounts.
	sender, err := s.accountRepo.GetAccount(ctx, cmd.SenderID)
	if err != nil {
		return nil, errors.Join(ErrGetSenderAccount, err)
	}
	recipient, err := s.accountRepo.GetAccount(ctx, recipientID)
	if err != nil {
		return nil, errors.Join(ErrInvalidRecipient, err)
	}

	messageID := uuid.New().String()
	status := &models.Status{
		StatusID:       messageID,
		AuthorID:       cmd.SenderID,
		AuthorUsername: sender.User.Username,
		Content:        cmd.Content,
		Visibility:     VisibilityDirect,
		Sensitive:      cmd.Sensitive,
		Language:       cmd.Language,
		ConversationID: conversation.ID,
		InReplyToID:    cmd.InReplyToID,
		PublishedAt:    time.Now(),
	}

	recipientURL := fmt.Sprintf("https://%s/users/%s", s.domainName, recipient.User.Username)
	status.ToRecipients = []string{recipientURL}
	status.Note = s.buildActivityPubNote(sendCmd, messageID, sender, conversation.ID, map[string]*storage.Account{
		recipientID: recipient,
	})

	if err := s.noteRepo.CreateStatus(ctx, status); err != nil {
		return nil, errors.Join(ErrCreateDirectMessage, err)
	}

	// Update conversation with latest message.
	conversation.LastStatusID = messageID
	conversation.UpdatedAt = time.Now()
	if err := s.conversationRepo.UpdateConversation(ctx, conversation); err != nil {
		s.logger.Warn("failed to update conversation", zap.Error(err))
	}

	// Update DM request lifecycle state (sender always in inbox).
	now := time.Now().UTC()
	if err := s.updateParticipantRecord(ctx, conversation.ID, cmd.SenderID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}
		record.RequestState = models.DmRequestStateAccepted
		if record.AcceptedAt == nil {
			t := now
			record.AcceptedAt = &t
		}
		record.DeclinedAt = nil
	}); err != nil {
		s.logger.Warn("failed to update sender DM request state",
			zap.String("conversation_id", conversation.ID),
			zap.String("sender_id", cmd.SenderID),
			zap.Error(err))
	}

	if err := s.updateParticipantRecord(ctx, conversation.ID, recipientID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}
		switch record.RequestState {
		case models.DmRequestStateAccepted:
			return
		case models.DmRequestStateDeclined:
			// v1 semantics: declined threads stay hidden; the next inbound message reopens as a new request.
			record.RequestState = models.DmRequestStatePending
			record.AcceptedAt = nil
			record.DeclinedAt = nil
			t := now
			record.RequestedAt = &t
			return
		default:
			if s.shouldDeliverToInbox(ctx, recipientID, cmd.SenderID) {
				record.RequestState = models.DmRequestStateAccepted
				record.RequestedAt = nil
				t := now
				record.AcceptedAt = &t
				record.DeclinedAt = nil
				return
			}
			record.RequestState = models.DmRequestStatePending
			if record.RequestedAt == nil {
				t := now
				record.RequestedAt = &t
			}
			return
		}
	}); err != nil {
		s.logger.Warn("failed to update recipient DM request state",
			zap.String("conversation_id", conversation.ID),
			zap.String("recipient_id", recipientID),
			zap.Error(err))
	}

	// Update per-user unread state.
	if err := s.conversationRepo.MarkConversationRead(ctx, conversation.ID, cmd.SenderID); err != nil {
		s.logger.Warn("failed to mark DM conversation read for sender",
			zap.String("conversation_id", conversation.ID),
			zap.String("sender_id", cmd.SenderID),
			zap.Error(err))
	} else {
		conversation.Unread = false
	}

	if err := s.conversationRepo.MarkConversationUnread(ctx, conversation.ID, recipientID); err != nil {
		s.logger.Warn("failed to mark DM conversation unread for recipient",
			zap.String("conversation_id", conversation.ID),
			zap.String("recipient_id", recipientID),
			zap.Error(err))
	}

	events := s.emitMessageSentEvents(ctx, status, conversation)
	s.queueFederationDelivery(ctx, status)

	return &MessageResult{
		Message:      status,
		Conversation: conversation,
		Events:       events,
	}, nil
}

// AcceptMessageRequest moves a pending request thread into the user's inbox.
func (s *Service) AcceptMessageRequest(ctx context.Context, cmd *AcceptMessageRequestCommand) (*ConversationResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	conversation, err := s.conversationRepo.GetConversation(ctx, cmd.ConversationID)
	if err != nil {
		return nil, errors.Join(ErrGetConversation, err)
	}

	if !s.isParticipant(cmd.UserID, conversation.Participants) {
		return nil, ErrNotConversationParticipant
	}

	now := time.Now().UTC()
	if err := s.updateParticipantRecord(ctx, conversation.ID, cmd.UserID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}
		record.RequestState = models.DmRequestStateAccepted
		record.RequestedAt = nil
		record.DeclinedAt = nil
		t := now
		record.AcceptedAt = &t
	}); err != nil {
		return nil, errors.Join(ErrMarkConversationRead, err)
	}

	if record, err := s.conversationRepo.GetConversationParticipantRecord(ctx, conversation.ID, cmd.UserID); err == nil && record != nil {
		conversation.Unread = record.Unread
	}

	return &ConversationResult{
		Conversation: conversation,
		Events:       []*streaming.Event{},
	}, nil
}

// DeclineMessageRequest hides a request thread from the recipient by setting requestState=DECLINED.
func (s *Service) DeclineMessageRequest(ctx context.Context, cmd *DeclineMessageRequestCommand) (*ConversationResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	conversation, err := s.conversationRepo.GetConversation(ctx, cmd.ConversationID)
	if err != nil {
		return nil, errors.Join(ErrGetConversation, err)
	}

	if !s.isParticipant(cmd.UserID, conversation.Participants) {
		return nil, ErrNotConversationParticipant
	}

	now := time.Now().UTC()
	if err := s.updateParticipantRecord(ctx, conversation.ID, cmd.UserID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}
		record.RequestState = models.DmRequestStateDeclined
		record.RequestedAt = nil
		record.AcceptedAt = nil
		t := now
		record.DeclinedAt = &t
	}); err != nil {
		return nil, errors.Join(ErrMarkConversationRead, err)
	}

	if record, err := s.conversationRepo.GetConversationParticipantRecord(ctx, conversation.ID, cmd.UserID); err == nil && record != nil {
		conversation.Unread = record.Unread
	}

	return &ConversationResult{
		Conversation: conversation,
		Events:       []*streaming.Event{},
	}, nil
}

// MarkConversationRead marks a conversation as read for a specific user
func (s *Service) MarkConversationRead(ctx context.Context, cmd *MarkConversationReadCommand) (*ConversationResult, error) {
	s.logger.Debug("marking conversation as read",
		zap.String("conversation_id", cmd.ConversationID),
		zap.String("user_id", cmd.UserID))

	// Get the conversation
	conversation, err := s.conversationRepo.GetConversation(ctx, cmd.ConversationID)
	if err != nil {
		s.logger.Error("failed to get conversation", zap.String("conversation_id", cmd.ConversationID), zap.Error(err))
		return nil, errors.Join(ErrGetConversation, err)
	}

	// Verify user is a participant
	if !s.isParticipant(cmd.UserID, conversation.Participants) {
		s.logger.Warn("user is not a conversation participant", zap.String("user_id", cmd.UserID), zap.String("conversation_id", cmd.ConversationID))
		return nil, ErrNotConversationParticipant
	}

	// Mark as read
	if err := s.conversationRepo.MarkConversationRead(ctx, cmd.ConversationID, cmd.UserID); err != nil {
		s.logger.Error("failed to mark conversation as read", zap.String("conversation_id", cmd.ConversationID), zap.String("user_id", cmd.UserID), zap.Error(err))
		return nil, errors.Join(ErrMarkConversationRead, err)
	}

	s.logger.Debug("marked conversation as read successfully",
		zap.String("conversation_id", cmd.ConversationID),
		zap.String("user_id", cmd.UserID))

	// Emit read event (only to the user who read it)
	events := s.emitConversationReadEvents(ctx, conversation, cmd.UserID)

	return &ConversationResult{
		Conversation: conversation,
		Events:       events,
	}, nil
}

// ListConversations retrieves conversations for a user with pagination
func (s *Service) ListConversations(ctx context.Context, query *ListConversationsQuery) (*Result, error) {
	s.logger.Debug("listing conversations",
		zap.String("user_id", query.UserID),
		zap.String("folder", string(query.Folder)),
		zap.Bool("only_unread", query.OnlyUnread))

	var result *interfaces.PaginatedResult[*models.Conversation]
	var err error

	if query.Folder != "" {
		requestState := models.DmRequestStateAccepted
		if query.Folder == ConversationFolderRequests {
			requestState = models.DmRequestStatePending
		}
		result, err = s.conversationRepo.GetUserConversationsByRequestState(ctx, query.UserID, requestState, query.Pagination)
	} else if query.OnlyUnread {
		result, err = s.conversationRepo.GetUnreadConversations(ctx, query.UserID, query.Pagination)
	} else {
		result, err = s.conversationRepo.GetUserConversations(ctx, query.UserID, query.Pagination)
	}

	if err != nil {
		s.logger.Error("failed to get user conversations", zap.String("user_id", query.UserID), zap.Bool("only_unread", query.OnlyUnread), zap.Error(err))
		return nil, errors.Join(ErrGetUserConversations, err)
	}

	return &Result{
		Conversations: result,
		Events:        []*streaming.Event{}, // No events for read operations
	}, nil
}

// GetConversation retrieves a conversation with its message history
func (s *Service) GetConversation(ctx context.Context, query *GetConversationQuery) (*ConversationWithMessages, error) {
	s.logger.Debug("getting conversation",
		zap.String("conversation_id", query.ConversationID),
		zap.String("viewer_id", query.ViewerID))

	// Get the conversation
	conversation, err := s.conversationRepo.GetConversation(ctx, query.ConversationID)
	if err != nil {
		s.logger.Error("failed to get conversation", zap.String("conversation_id", query.ConversationID), zap.Error(err))
		return nil, errors.Join(ErrGetConversation, err)
	}

	// Verify user is a participant
	if !s.isParticipant(query.ViewerID, conversation.Participants) {
		s.logger.Warn("viewer is not a conversation participant", zap.String("viewer_id", query.ViewerID), zap.String("conversation_id", query.ConversationID))
		return nil, ErrNotConversationParticipant
	}

	// Populate per-viewer unread state if available.
	if record, err := s.conversationRepo.GetConversationParticipantRecord(ctx, query.ConversationID, query.ViewerID); err == nil && record != nil {
		conversation.Unread = record.Unread
	}

	// Get conversation messages (these are statuses with this conversation ID)
	messages, err := s.noteRepo.GetConversationThread(ctx, query.ConversationID, query.Pagination)
	if err != nil {
		s.logger.Error("failed to get conversation messages", zap.String("conversation_id", query.ConversationID), zap.Error(err))
		return nil, errors.Join(ErrGetConversationMessages, err)
	}

	// Filter messages to ensure they're all direct messages visible to the viewer.
	// NOTE: Status.AuthorID is not consistently an actor URL across the codebase, so we must
	// treat AuthorUsername as the canonical author check here and validate recipient addressing
	// using the viewer's actor URL.
	viewerUsername := strings.TrimSpace(query.ViewerID)
	viewerActorID := s.actorURLForUsername(viewerUsername)
	filteredMessages := make([]*models.Status, 0, len(messages.Items))
	for _, message := range messages.Items {
		if message.Visibility != VisibilityDirect {
			continue
		}
		if message.AuthorUsername != viewerUsername && !message.IsRecipient(viewerActorID) {
			continue
		}
		filteredMessages = append(filteredMessages, message.SanitizeForActor(viewerActorID))
	}

	// Update the messages result with filtered items
	filteredMessagesResult := &interfaces.PaginatedResult[*models.Status]{
		Items:      filteredMessages,
		NextCursor: messages.NextCursor,
		HasMore:    messages.HasMore,
		Total:      int64(len(filteredMessages)),
	}

	return &ConversationWithMessages{
		Conversation: conversation,
		Messages:     filteredMessagesResult,
		Events:       []*streaming.Event{}, // No events for read operations
	}, nil
}

// Private helper methods

func (s *Service) actorURLForUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	if strings.Contains(username, "://") {
		// Already a URL.
		return strings.TrimRight(username, "/")
	}
	return fmt.Sprintf("https://%s/users/%s", s.domainName, username)
}

func (s *Service) directMessagesFromPreference(ctx context.Context, username string) string {
	// Default: following-only
	pref := "FOLLOWING_ONLY"

	if s.userRepo == nil {
		return pref
	}

	prefs, err := s.userRepo.GetUserPreferences(ctx, username)
	if err != nil || prefs == nil {
		return pref
	}

	if v := strings.TrimSpace(prefs.DirectMessagesFrom); v != "" {
		pref = strings.ToUpper(v)
	}

	return pref
}

func (s *Service) shouldDeliverToInbox(ctx context.Context, recipientID, senderID string) bool {
	pref := s.directMessagesFromPreference(ctx, recipientID)
	if pref == "ANYONE" {
		return true
	}

	// Default: FOLLOWING_ONLY.
	if s.relationshipRepo == nil {
		return false
	}

	isFollowing, err := s.relationshipRepo.IsFollowing(ctx, recipientID, senderID)
	if err != nil {
		s.logger.Warn("failed to check follower relationship for DM inbox policy",
			zap.String("recipient_id", recipientID),
			zap.String("sender_id", senderID),
			zap.Error(err))
		return false
	}

	return isFollowing
}

func (s *Service) validateSendMessageCommandBasic(ctx context.Context, cmd *SendDirectMessageCommand) error {
	if err := common.ValidateRequiredParam("cmd.SenderID", cmd.SenderID); err != nil {
		return common.ValidateRequiredParam("sender_id", cmd.SenderID)
	}

	if err := common.ValidateSliceNotEmpty("cmd.Recipients", cmd.Recipients); err != nil {
		return ErrRecipientsRequired
	}

	if err := common.ValidateRequiredParam("content", strings.TrimSpace(cmd.Content)); err != nil {
		return err
	}

	if len(cmd.Content) > 5000 {
		return ErrContentTooLongConversation
	}

	// Account validation is done in SendDirectMessage method to avoid duplicate calls

	// Validate in_reply_to_id if provided
	if cmd.InReplyToID != "" {
		parentMessage, err := s.noteRepo.GetStatus(ctx, cmd.InReplyToID)
		if err != nil {
			return errors.Join(ErrInvalidInReplyToIDConversation, err)
		}
		// Verify it's a direct message in an accessible conversation
		if parentMessage.Visibility != VisibilityDirect {
			return ErrCanOnlyReplyToDirectMessages
		}
	}

	return nil
}

func (s *Service) buildActivityPubNote(cmd *SendDirectMessageCommand, messageID string, sender *storage.Account, conversationID string, recipientAccounts map[string]*storage.Account) *activitypub.Note {
	now := time.Now()

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      "Note",
			ID:        fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, sender.User.Username, messageID),
			Published: &now,
			To:        make([]string, 0, len(cmd.Recipients)),
			Sensitive: cmd.Sensitive,
		},
		Content:        cmd.Content,
		AttributedTo:   fmt.Sprintf("https://%s/users/%s", s.domainName, sender.User.Username),
		Visibility:     VisibilityDirect,
		ConversationID: conversationID,
	}

	// Add recipients to To field - use cached accounts
	for _, recipientID := range cmd.Recipients {
		recipient := recipientAccounts[recipientID]
		if recipient != nil {
			note.To = append(note.To, fmt.Sprintf("https://%s/users/%s", s.domainName, recipient.User.Username))
		}
	}

	// Set in reply to
	if cmd.InReplyToID != "" {
		note.InReplyTo = fmt.Sprintf("https://%s/statuses/%s", s.domainName, cmd.InReplyToID)
	}

	return note
}

func (s *Service) emitMessageSentEvents(ctx context.Context, message *models.Status, conversation *models.Conversation) []*streaming.Event {
	var events []*streaming.Event

	// Create base event
	event := &streaming.Event{
		Type:      ConversationMessageEvent,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"message":      message,
			"conversation": conversation,
		},
	}

	// Emit to all conversation participants
	conversationEvent := *event
	conversationEvent.Stream = fmt.Sprintf("conversation:%s", conversation.ID)
	if err := s.publisher.PublishToConversation(ctx, conversation.ID, &conversationEvent); err != nil {
		s.logger.Error("failed to publish to conversation stream", zap.Error(err))
	} else {
		events = append(events, &conversationEvent)
	}

	// Also emit to each participant's direct stream
	for _, participantID := range conversation.Participants {
		participantEvent := *event
		participantEvent.Stream = fmt.Sprintf("user:%s:direct", participantID)
		if err := s.publisher.PublishToUser(ctx, participantID, &participantEvent); err != nil {
			s.logger.Error("failed to publish to participant direct stream",
				zap.String("participant_id", participantID),
				zap.Error(err))
		} else {
			events = append(events, &participantEvent)
		}
	}

	return events
}

func (s *Service) emitConversationReadEvents(ctx context.Context, conversation *models.Conversation, userID string) []*streaming.Event {
	var events []*streaming.Event

	// Create read event (only sent to the user who read it)
	event := &streaming.Event{
		Type:      ConversationReadEvent,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"conversation_id": conversation.ID,
			"user_id":         userID,
		},
	}

	// Emit to user's direct stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s:direct", userID)
	if err := s.publisher.PublishToUser(ctx, userID, &userEvent); err != nil {
		s.logger.Error("failed to publish read event to user stream", zap.Error(err))
	} else {
		events = append(events, &userEvent)
	}

	return events
}

func (s *Service) queueFederationDelivery(ctx context.Context, message *models.Status) {
	if s.federation == nil {
		s.logger.Debug("federation service not available, skipping delivery")
		return
	}

	// Only federate if there are remote recipients
	hasRemoteRecipients := false
	for _, recipientURL := range message.ToRecipients {
		if !strings.Contains(recipientURL, s.domainName) {
			hasRemoteRecipients = true
			break
		}
	}

	if !hasRemoteRecipients {
		s.logger.Debug("no remote recipients, skipping federation")
		return
	}

	// Create ActivityPub Create activity
	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    "Create",
			ID:      fmt.Sprintf("%s#create", message.Note.ID),
			To:      message.ToRecipients,
			CC:      message.CcRecipients,
		},
		Actor:  message.Note.AttributedTo,
		Object: message.Note,
	}

	if err := s.federation.QueueActivity(ctx, activity); err != nil {
		s.logger.Error("failed to queue federation delivery",
			zap.String("message_id", message.StatusID),
			zap.Error(err))
	}
}

func (s *Service) isParticipant(userID string, participants []string) bool {
	for _, participant := range participants {
		if participant == userID {
			return true
		}
	}
	return false
}

func isNotFoundError(err error) bool {
	// This would typically check for storage-specific "not found" errors
	// The exact implementation depends on how the storage layer reports not found errors
	return err != nil && (strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NotFound"))
}

func (s *Service) updateParticipantRecord(ctx context.Context, conversationID, participantID string, mutator func(record *models.ConversationParticipantRecord)) error {
	if mutator == nil {
		return nil
	}

	record, err := s.conversationRepo.GetConversationParticipantRecord(ctx, conversationID, participantID)
	if err != nil {
		return err
	}
	if record == nil {
		return storage.ErrNotFound
	}

	mutator(record)
	return s.conversationRepo.UpdateConversationParticipantRecord(ctx, record)
}

// DeleteConversation removes a conversation or removes a user from it
func (s *Service) DeleteConversation(ctx context.Context, cmd *DeleteConversationCommand) (*ConversationResult, error) {
	s.logger.Info("deleting conversation",
		zap.String("conversation_id", cmd.ConversationID),
		zap.String("user_id", cmd.UserID))

	// Get conversation to verify it exists and user is a participant
	conversation, err := s.conversationRepo.GetConversation(ctx, cmd.ConversationID)
	if err != nil {
		if isNotFoundError(err) {
			s.logger.Warn("conversation not found", zap.String("conversation_id", cmd.ConversationID))
			return nil, ErrConversationNotFound
		}
		s.logger.Error("failed to get conversation", zap.String("conversation_id", cmd.ConversationID), zap.Error(err))
		return nil, errors.Join(ErrGetConversation, err)
	}

	// Check if user is a participant
	if !s.isParticipant(cmd.UserID, conversation.Participants) {
		// Get user's account to check actor ID
		account, err := s.accountRepo.GetAccount(ctx, cmd.UserID)
		if err != nil {
			s.logger.Error("failed to get account", zap.String("user_id", cmd.UserID), zap.Error(err))
			return nil, errors.Join(ErrGetAccount, err)
		}

		// Check with actor ID as well
		if account.Actor == nil || !s.isParticipant(account.Actor.ID, conversation.Participants) {
			s.logger.Warn("user is not a conversation participant (checked actor ID)", zap.String("user_id", cmd.UserID), zap.String("conversation_id", cmd.ConversationID))
			return nil, ErrNotConversationParticipant
		}
	}

	// Delete the conversation (or remove user from it)
	// The exact behavior depends on the storage implementation
	// Some systems remove the user from participants, others delete the entire conversation
	if err := s.conversationRepo.DeleteConversation(ctx, cmd.ConversationID); err != nil {
		s.logger.Error("failed to delete conversation", zap.String("conversation_id", cmd.ConversationID), zap.String("user_id", cmd.UserID), zap.Error(err))
		return nil, errors.Join(ErrDeleteConversation, err)
	}

	// Emit conversation deleted event
	events := s.emitConversationDeletedEvents(ctx, conversation, cmd.UserID)

	return &ConversationResult{
		Conversation: nil, // Conversation is deleted
		Events:       events,
	}, nil
}

// emitConversationDeletedEvents creates events for conversation deletion
func (s *Service) emitConversationDeletedEvents(ctx context.Context, conversation *models.Conversation, userID string) []*streaming.Event {
	var events []*streaming.Event

	// Create conversation deleted event
	event := &streaming.Event{
		Type:      "conversation.deleted",
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"conversation_id": conversation.ID,
			"user_id":         userID,
		},
	}

	// Emit to user's stream
	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s", userID)
	if err := s.publisher.PublishToUser(ctx, userID, &userEvent); err != nil {
		s.logger.Warn("failed to publish conversation deleted event",
			zap.String("user_id", userID),
			zap.Error(err))
	}
	events = append(events, &userEvent)

	// Emit to conversation stream (for other participants to know)
	conversationEvent := *event
	conversationEvent.Stream = fmt.Sprintf("conversation:%s", conversation.ID)
	if err := s.publisher.PublishToConversation(ctx, conversation.ID, &conversationEvent); err != nil {
		s.logger.Warn("failed to publish conversation deleted event to conversation stream",
			zap.String("conversation_id", conversation.ID),
			zap.Error(err))
	}
	events = append(events, &conversationEvent)

	return events
}
