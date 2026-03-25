// Package conversations provides the core Conversations Service for the Lesser project's API alignment.
// This service handles direct message conversations including creation, participant management,
// read status tracking, and real-time event emission. It supports federation for remote participants
// and maintains privacy by ensuring only participants can access conversation content.
package conversations

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
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
	dmTombstoneRepo  directMessageTombstoneRepository
	accountRepo      interfaces.AccountRepository
	relationshipRepo interfaces.ConcreteRelationshipRepository
	userRepo         interfaces.UserRepository
	rateLimitRepo    interfaces.RateLimitRepository
	auditRepo        interfaces.AuditRepository
	publisher        streaming.Publisher
	federation       FederationService
	logger           *zap.Logger
	domainName       string
}

// FederationService defines the interface for federation operations
type FederationService interface {
	QueueActivity(ctx context.Context, activity *activitypub.Activity) error
}

type directMessageTombstoneRepository interface {
	CreateTombstone(ctx context.Context, viewerUsername, statusID string) error
	TombstonesByStatusID(ctx context.Context, viewerUsername string, statusIDs []string) (map[string]bool, error)
}

// NewService creates a new Conversations Service with the required dependencies
func NewService(
	conversationRepo interfaces.ConversationRepository,
	noteRepo interfaces.StatusRepository,
	dmTombstoneRepo directMessageTombstoneRepository,
	accountRepo interfaces.AccountRepository,
	relationshipRepo interfaces.ConcreteRelationshipRepository,
	userRepo interfaces.UserRepository,
	rateLimitRepo interfaces.RateLimitRepository,
	auditRepo interfaces.AuditRepository,
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
		dmTombstoneRepo:  dmTombstoneRepo,
		accountRepo:      accountRepo,
		relationshipRepo: relationshipRepo,
		userRepo:         userRepo,
		rateLimitRepo:    rateLimitRepo,
		auditRepo:        auditRepo,
		publisher:        publisher,
		federation:       federation,
		logger:           logger,
		domainName:       domainName,
	}
}

// Command structs for operations

const (
	dmSendTotalLimit            = 60
	dmSendTotalWindow           = time.Minute
	dmRequestTotalLimit         = 20
	dmRequestTotalWindow        = time.Hour
	dmRequestPerRecipientLimit  = 3
	dmRequestPerRecipientWindow = 24 * time.Hour
)

type apiRateLimitInfoReader interface {
	GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error)
}

// SendDirectMessageCommand contains all data needed to send a direct message
type SendDirectMessageCommand struct {
	SenderID         string                            `json:"sender_id" validate:"required"`
	Recipients       []string                          `json:"recipients" validate:"required,min=1"`
	Content          string                            `json:"content" validate:"required,max=5000"`
	Sensitive        bool                              `json:"sensitive"`
	SpoilerText      string                            `json:"spoiler_text"`
	Language         string                            `json:"language"`
	MediaIDs         []string                          `json:"media_ids"`
	InReplyToID      string                            `json:"in_reply_to_id"` // Can reply to messages in the conversation
	AgentAttribution *activitypub.AgentPostAttribution `json:"agent_attribution,omitempty"`
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

// DeleteMessageCommand contains data needed to delete a direct message for the viewer.
type DeleteMessageCommand struct {
	MessageID string `json:"message_id" validate:"required"`
	UserID    string `json:"user_id" validate:"required"`
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
	// ConversationFolderInbox is the user's accepted DM inbox.
	ConversationFolderInbox ConversationFolder = "INBOX"
	// ConversationFolderRequests is the user's pending message requests folder.
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
	if creatorID == "" || participantID == "" ||
		models.CanonicalConversationParticipantID(creatorID) == models.CanonicalConversationParticipantID(participantID) {
		return nil, errors.Join(ErrConversationValidationFailed, ErrInvalidRecipient)
	}

	if _, err := s.accountRepo.GetAccount(ctx, creatorID); err != nil {
		return nil, errors.Join(ErrGetSenderAccount, err)
	}
	if _, err := s.accountRepo.GetAccount(ctx, participantID); err != nil {
		return nil, errors.Join(ErrInvalidRecipient, err)
	}

	// Block enforcement: do not allow creating DM threads between blocked users.
	if s.relationshipRepo != nil {
		blocked, err := s.relationshipRepo.IsBlockedBidirectional(ctx, creatorID, participantID)
		if err != nil {
			return nil, errors.Join(ErrConversationValidationFailed, err)
		}
		if blocked {
			return nil, ErrDirectMessageBlocked
		}
	}

	participants := models.CanonicalConversationParticipants([]string{creatorID, participantID})

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

		record.RequestState = ""
		record.RequestedAt = nil
		record.AcceptedAt = nil
		record.DeclinedAt = nil
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

func (s *Service) validateSendDirectMessageCommand(ctx context.Context, cmd *SendDirectMessageCommand) (string, error) {
	if err := s.validateSendMessageCommandBasic(ctx, cmd); err != nil {
		s.logger.Error("validation failed", zap.Error(err))
		s.auditDMEvent(ctx, cmd, "", false, "validation_failed", map[string]any{
			"content_length": len(cmd.Content),
		})
		return "", errors.Join(ErrConversationValidationFailed, err)
	}

	if len(cmd.Recipients) != 1 {
		s.auditDMEvent(ctx, cmd, "", false, "invalid_recipient_count", nil)
		return "", errors.Join(ErrConversationValidationFailed, ErrDirectMessageRequiresSingleRecipient)
	}

	recipientID := cmd.Recipients[0]
	if strings.TrimSpace(recipientID) == "" ||
		models.CanonicalConversationParticipantID(recipientID) == models.CanonicalConversationParticipantID(cmd.SenderID) {
		s.auditDMEvent(ctx, cmd, "", false, "invalid_recipient", map[string]any{
			"recipient_id": recipientID,
		})
		return "", errors.Join(ErrConversationValidationFailed, ErrInvalidRecipient)
	}

	return recipientID, nil
}

func (s *Service) enforceDirectMessageNotBlocked(ctx context.Context, cmd *SendDirectMessageCommand, recipientID string) error {
	if s.relationshipRepo == nil {
		return nil
	}

	blocked, err := s.relationshipRepo.IsBlockedBidirectional(ctx, cmd.SenderID, recipientID)
	if err != nil {
		s.logger.Warn("failed to check block status for DM send",
			zap.String("sender_id", cmd.SenderID),
			zap.String("recipient_id", recipientID),
			zap.Error(err))
		s.auditDMEvent(ctx, cmd, "", false, "block_check_failed", map[string]any{
			"recipient_id": recipientID,
		})
		return errors.Join(ErrConversationValidationFailed, err)
	}
	if blocked {
		s.auditDMEvent(ctx, cmd, "", false, "blocked", map[string]any{
			"recipient_id": recipientID,
		})
		return ErrDirectMessageBlocked
	}

	return nil
}

func (s *Service) enforceDirectMessageTotalRateLimit(ctx context.Context, cmd *SendDirectMessageCommand, recipientID string) error {
	if s.rateLimitRepo == nil || directMessageRateLimitingDisabled() {
		return nil
	}

	if err := s.rateLimitRepo.CheckAPIRateLimit(ctx, fmt.Sprintf("dm:%s", cmd.SenderID), "dm_send_total", dmSendTotalLimit, dmSendTotalWindow); err != nil {
		s.auditDMEvent(ctx, cmd, "", false, "rate_limited_send_total", map[string]any{
			"recipient_id": recipientID,
		})
		return err
	}

	return nil
}

func (s *Service) getDirectMessageAccounts(ctx context.Context, cmd *SendDirectMessageCommand, recipientID string) (*storage.Account, *storage.Account, map[string]*storage.Account, error) {
	sender, err := s.accountRepo.GetAccount(ctx, cmd.SenderID)
	if err != nil {
		s.logger.Error("failed to get sender account", zap.String("sender_id", cmd.SenderID), zap.Error(err))
		s.auditDMEvent(ctx, cmd, "", false, "get_sender_account_failed", map[string]any{
			"recipient_id": recipientID,
		})
		return nil, nil, nil, errors.Join(ErrGetSenderAccount, err)
	}

	recipientAccounts := make(map[string]*storage.Account, 1)
	recipient, err := s.accountRepo.GetAccount(ctx, recipientID)
	if err != nil {
		s.logger.Error("invalid recipient", zap.String("recipient_id", recipientID), zap.Error(err))
		s.auditDMEvent(ctx, cmd, "", false, "get_recipient_account_failed", map[string]any{
			"recipient_id": recipientID,
		})
		return nil, nil, nil, errors.Join(ErrInvalidRecipient, err)
	}
	resolvedRecipientID := resolvedLegacyLocalAccountID(recipientID, recipient)
	recipientAccounts[resolvedRecipientID] = recipient

	return sender, recipient, recipientAccounts, nil
}

func cloneDirectMessageCommandWithResolvedParticipants(
	cmd *SendDirectMessageCommand,
	sender *storage.Account,
	recipient *storage.Account,
) *SendDirectMessageCommand {
	if cmd == nil {
		return nil
	}

	cloned := *cmd
	cloned.SenderID = resolvedLegacyLocalAccountID(cmd.SenderID, sender)
	cloned.Recipients = append([]string(nil), cmd.Recipients...)
	if len(cloned.Recipients) > 0 {
		cloned.Recipients[0] = resolvedLegacyLocalAccountID(cloned.Recipients[0], recipient)
	}

	return &cloned
}

func resolvedLegacyLocalAccountID(requestedID string, account *storage.Account) string {
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" || account == nil || account.User == nil {
		return requestedID
	}

	storedUsername := strings.TrimSpace(account.User.Username)
	if storedUsername == "" {
		return requestedID
	}

	if requestedID == storedUsername || !strings.EqualFold(requestedID, storedUsername) {
		return requestedID
	}

	return storedUsername
}

func (s *Service) getOrCreateDirectMessageConversation(ctx context.Context, cmd *SendDirectMessageCommand, recipientID string) (*models.Conversation, error) {
	allParticipants := append([]string{cmd.SenderID}, cmd.Recipients...)
	lookupParticipants := models.CanonicalConversationParticipants(allParticipants)

	conversation, err := s.conversationRepo.GetConversationByParticipants(ctx, lookupParticipants)
	if err != nil && !isNotFoundError(err) {
		s.logger.Error("failed to lookup existing conversation", zap.Strings("participants", lookupParticipants), zap.Error(err))
		s.auditDMEvent(ctx, cmd, "", false, "lookup_conversation_failed", map[string]any{
			"recipient_id": recipientID,
		})
		return nil, errors.Join(ErrLookupExistingConversation, err)
	}

	if conversation != nil {
		return conversation, nil
	}

	conversationID := uuid.New().String()
	conversation = &models.Conversation{
		ID:           conversationID,
		Participants: lookupParticipants,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.conversationRepo.CreateConversation(ctx, conversation, lookupParticipants); err != nil {
		s.logger.Error("failed to create conversation", zap.String("conversation_id", conversationID), zap.Error(err))
		s.auditDMEvent(ctx, cmd, conversationID, false, "create_conversation_failed", map[string]any{
			"recipient_id": recipientID,
		})
		return nil, errors.Join(ErrCreateConversation, err)
	}

	s.logger.Info("created new conversation",
		zap.String("conversation_id", conversationID),
		zap.Strings("participants", lookupParticipants))

	return conversation, nil
}

func (s *Service) directMessageRecipientRequestState(ctx context.Context, conversationID, recipientID string) models.DmRequestState {
	record, err := s.conversationRepo.GetConversationParticipantRecord(ctx, conversationID, recipientID)
	if err != nil || record == nil {
		return ""
	}
	return record.RequestState
}

func (s *Service) enforceDirectMessageRequestRateLimit(ctx context.Context, cmd *SendDirectMessageCommand, conversationID, recipientID string) error {
	if s.rateLimitRepo == nil || directMessageRateLimitingDisabled() {
		return nil
	}

	if err := s.rateLimitRepo.CheckAPIRateLimit(ctx, fmt.Sprintf("dm:%s", cmd.SenderID), "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow); err != nil {
		s.auditDMEvent(ctx, cmd, conversationID, false, "rate_limited_request_total", map[string]any{
			"recipient_id": recipientID,
		})
		return err
	}

	if err := s.rateLimitRepo.CheckAPIRateLimit(ctx, fmt.Sprintf("dm:%s", cmd.SenderID), fmt.Sprintf("dm_request_to:%s", recipientID), dmRequestPerRecipientLimit, dmRequestPerRecipientWindow); err != nil {
		s.auditDMEvent(ctx, cmd, conversationID, false, "rate_limited_request_to_recipient", map[string]any{
			"recipient_id": recipientID,
		})
		return err
	}

	return nil
}

func (s *Service) evaluateDirectMessageRequestPolicy(ctx context.Context, cmd *SendDirectMessageCommand, conversationID, recipientID string) (willBeRequest bool, deliversToInbox bool, recipientRequestState models.DmRequestState, _ error) {
	deliversToInbox = s.shouldDeliverToInbox(ctx, recipientID, cmd.SenderID)
	recipientRequestState = s.directMessageRecipientRequestState(ctx, conversationID, recipientID)

	switch recipientRequestState {
	case models.DmRequestStateAccepted:
		willBeRequest = false
	case models.DmRequestStateDeclined:
		willBeRequest = true
	default:
		willBeRequest = !deliversToInbox
	}

	if recipientRequestState == models.DmRequestStatePending && !deliversToInbox {
		s.auditDMEvent(ctx, cmd, conversationID, false, "request_pending", map[string]any{
			"recipient_id": recipientID,
		})
		return false, deliversToInbox, recipientRequestState, ErrMessageRequestPending
	}

	if willBeRequest && len(cmd.MediaIDs) > 0 {
		s.auditDMEvent(ctx, cmd, conversationID, false, "media_not_allowed_in_request", map[string]any{
			"recipient_id": recipientID,
			"media_count":  len(cmd.MediaIDs),
		})
		return false, deliversToInbox, recipientRequestState, ErrMessageRequestMediaNotAllowed
	}

	return willBeRequest, deliversToInbox, recipientRequestState, nil
}

func directMessageRateLimitingDisabled() bool {
	cfg := config.Get()
	return cfg != nil && cfg.DisableRateLimiting
}

func (s *Service) previewDirectMessageRateLimit(ctx context.Context, cmd *SendDirectMessageCommand, conversationID, recipientID, endpoint string, limit int, window time.Duration, auditReason string) error {
	if s.rateLimitRepo == nil || directMessageRateLimitingDisabled() {
		return nil
	}

	reader, ok := s.rateLimitRepo.(apiRateLimitInfoReader)
	if !ok {
		return nil
	}

	remaining, _, err := reader.GetAPIRateLimitInfo(ctx, fmt.Sprintf("dm:%s", cmd.SenderID), endpoint, limit, window)
	if err != nil {
		s.logger.Warn("failed to preview direct message rate limit",
			zap.String("sender_id", cmd.SenderID),
			zap.String("recipient_id", recipientID),
			zap.String("endpoint", endpoint),
			zap.Error(err))
		return nil
	}

	if remaining <= 0 {
		s.auditDMEvent(ctx, cmd, conversationID, false, auditReason, map[string]any{
			"recipient_id": recipientID,
		})
		return storage.ErrRateLimited
	}

	return nil
}

func (s *Service) previewDirectMessageTotalRateLimit(ctx context.Context, cmd *SendDirectMessageCommand, recipientID string) error {
	return s.previewDirectMessageRateLimit(ctx, cmd, "", recipientID, "dm_send_total", dmSendTotalLimit, dmSendTotalWindow, "rate_limited_send_total")
}

func (s *Service) previewDirectMessageRequestRateLimit(ctx context.Context, cmd *SendDirectMessageCommand, conversationID, recipientID string) error {
	if err := s.previewDirectMessageRateLimit(ctx, cmd, conversationID, recipientID, "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow, "rate_limited_request_total"); err != nil {
		return err
	}
	return s.previewDirectMessageRateLimit(ctx, cmd, conversationID, recipientID, fmt.Sprintf("dm_request_to:%s", recipientID), dmRequestPerRecipientLimit, dmRequestPerRecipientWindow, "rate_limited_request_to_recipient")
}

func (s *Service) createDirectMessageStatus(ctx context.Context, cmd *SendDirectMessageCommand, sender *storage.Account, recipientAccounts map[string]*storage.Account, conversationID, recipientID string) (*models.Status, string, error) {
	messageID := uuid.New().String()
	now := time.Now().UTC()

	status := &models.Status{
		StatusID:       messageID,
		AuthorID:       cmd.SenderID,
		AuthorUsername: sender.User.Username,
		Content:        cmd.Content,
		Visibility:     VisibilityDirect,
		Sensitive:      cmd.Sensitive,
		Language:       cmd.Language,
		ConversationID: conversationID,
		InReplyToID:    cmd.InReplyToID,
		PublishedAt:    now,
		CreatedAt:      now,
		ModifiedAt:     now,
		UpdatedAt:      now,
	}

	recipientURLs := make([]string, 0, len(cmd.Recipients))
	for _, recipientID := range cmd.Recipients {
		recipient := recipientAccounts[recipientID]
		recipientURLs = append(recipientURLs, fmt.Sprintf("https://%s/users/%s", s.domainName, recipient.User.Username))
	}
	status.ToRecipients = recipientURLs

	status.Note = s.buildActivityPubNote(cmd, messageID, sender, conversationID, recipientAccounts)

	if err := s.noteRepo.CreateStatus(ctx, status); err != nil {
		s.logger.Error("failed to create direct message", zap.String("message_id", messageID), zap.Error(err))
		s.auditDMEvent(ctx, cmd, conversationID, false, "create_status_failed", map[string]any{
			"recipient_id":   recipientID,
			"message_id":     messageID,
			"content_length": len(cmd.Content),
		})
		return nil, "", errors.Join(ErrCreateDirectMessage, err)
	}

	return status, messageID, nil
}

func (s *Service) updateConversationLastStatus(ctx context.Context, conversation *models.Conversation, messageID string) {
	conversation.LastStatusID = messageID
	conversation.UpdatedAt = time.Now()
	if err := s.conversationRepo.UpdateConversation(ctx, conversation); err != nil {
		s.logger.Warn("failed to update conversation", zap.Error(err))
	}
}

func (s *Service) updateDirectMessageParticipantStateAfterSend(ctx context.Context, conversationID, senderID, recipientID string, deliversToInbox bool) {
	now := time.Now().UTC()

	if err := s.updateParticipantRecord(ctx, conversationID, senderID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}

		record.DeletedAt = nil
		record.RequestState = models.DmRequestStateAccepted
		if record.AcceptedAt == nil {
			t := now
			record.AcceptedAt = &t
		}
		record.DeclinedAt = nil
	}); err != nil {
		s.logger.Warn("failed to update sender DM request state",
			zap.String("conversation_id", conversationID),
			zap.String("sender_id", senderID),
			zap.Error(err))
	}

	if err := s.updateParticipantRecord(ctx, conversationID, recipientID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}

		record.DeletedAt = nil
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
			// PENDING or unset -> apply inbox policy.
			if deliversToInbox {
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
			zap.String("conversation_id", conversationID),
			zap.String("recipient_id", recipientID),
			zap.Error(err))
	}
}

func (s *Service) updateConversationUnreadAfterSend(ctx context.Context, conversation *models.Conversation, senderID, recipientID string) {
	if err := s.conversationRepo.MarkConversationRead(ctx, conversation.ID, senderID); err != nil {
		s.logger.Warn("failed to mark DM conversation read for sender",
			zap.String("conversation_id", conversation.ID),
			zap.String("sender_id", senderID),
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
}

// SendDirectMessage creates or updates a conversation, creates a direct message, and emits events
func (s *Service) SendDirectMessage(ctx context.Context, cmd *SendDirectMessageCommand) (*MessageResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	s.logger.Info("sending direct message",
		zap.String("sender_id", cmd.SenderID),
		zap.Strings("recipients", cmd.Recipients),
		zap.Int("content_length", len(cmd.Content)))

	recipientID, err := s.validateSendDirectMessageCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}

	if err := s.enforceDirectMessageNotBlocked(ctx, cmd, recipientID); err != nil {
		return nil, err
	}

	if err := s.previewDirectMessageTotalRateLimit(ctx, cmd, recipientID); err != nil {
		return nil, err
	}

	sender, recipient, recipientAccounts, err := s.getDirectMessageAccounts(ctx, cmd, recipientID)
	if err != nil {
		return nil, err
	}

	cmd = cloneDirectMessageCommandWithResolvedParticipants(cmd, sender, recipient)
	recipientID = cmd.Recipients[0]

	conversation, err := s.getOrCreateDirectMessageConversation(ctx, cmd, recipientID)
	if err != nil {
		return nil, err
	}

	willBeRequest, deliversToInbox, _, err := s.evaluateDirectMessageRequestPolicy(ctx, cmd, conversation.ID, recipientID)
	if err != nil {
		return nil, err
	}
	if willBeRequest {
		if err := s.previewDirectMessageRequestRateLimit(ctx, cmd, conversation.ID, recipientID); err != nil {
			return nil, err
		}
	}

	status, messageID, err := s.createDirectMessageStatus(ctx, cmd, sender, recipientAccounts, conversation.ID, recipientID)
	if err != nil {
		return nil, err
	}

	s.updateConversationLastStatus(ctx, conversation, messageID)
	s.updateDirectMessageParticipantStateAfterSend(ctx, conversation.ID, cmd.SenderID, recipientID, deliversToInbox)
	s.updateConversationUnreadAfterSend(ctx, conversation, cmd.SenderID, recipientID)

	s.logger.Info("sent direct message successfully",
		zap.String("message_id", messageID),
		zap.String("conversation_id", conversation.ID))

	// Emit events and queue federation
	events := s.emitMessageSentEvents(ctx, status, conversation)
	s.queueFederationDelivery(ctx, status)

	if err := s.enforceDirectMessageTotalRateLimit(ctx, cmd, recipientID); err != nil {
		s.logger.Warn("failed to record direct message total rate limit after successful send",
			zap.String("conversation_id", conversation.ID),
			zap.String("recipient_id", recipientID),
			zap.Error(err))
	}
	if willBeRequest {
		if err := s.enforceDirectMessageRequestRateLimit(ctx, cmd, conversation.ID, recipientID); err != nil {
			s.logger.Warn("failed to record direct message request rate limit after successful send",
				zap.String("conversation_id", conversation.ID),
				zap.String("recipient_id", recipientID),
				zap.Error(err))
		}
	}

	s.auditDMEvent(ctx, cmd, conversation.ID, true, "", map[string]any{
		"recipient_id":    recipientID,
		"message_id":      messageID,
		"content_length":  len(cmd.Content),
		"request_message": willBeRequest,
		"media_count":     len(cmd.MediaIDs),
	})

	return &MessageResult{
		Message:      status,
		Conversation: conversation,
		Events:       events,
	}, nil
}

func (s *Service) loadConversationAndRecipientForSendMessage(ctx context.Context, cmd *SendMessageCommand) (*models.Conversation, string, error) {
	conversation, err := s.conversationRepo.GetConversation(ctx, cmd.ConversationID)
	if err != nil {
		return nil, "", errors.Join(ErrGetConversation, err)
	}

	if len(conversation.Participants) != 2 {
		return nil, "", errors.Join(ErrConversationValidationFailed, ErrConversationMustBeOneToOne)
	}
	if !s.isParticipant(cmd.SenderID, conversation.Participants) {
		return nil, "", ErrNotConversationParticipant
	}

	recipientID := ""
	for _, participantID := range conversation.Participants {
		if participantID != cmd.SenderID {
			recipientID = participantID
			break
		}
	}
	if strings.TrimSpace(recipientID) == "" {
		return nil, "", errors.Join(ErrConversationValidationFailed, ErrInvalidRecipient)
	}

	return conversation, recipientID, nil
}

func sendDirectMessageCommandFromSendMessage(cmd *SendMessageCommand, recipientID string) *SendDirectMessageCommand {
	return &SendDirectMessageCommand{
		SenderID:    cmd.SenderID,
		Recipients:  []string{recipientID},
		Content:     cmd.Content,
		Sensitive:   cmd.Sensitive,
		Language:    cmd.Language,
		MediaIDs:    cmd.MediaIDs,
		InReplyToID: cmd.InReplyToID,
	}
}

func (s *Service) validateSendMessageReplyTarget(ctx context.Context, inReplyToID, conversationID string) error {
	if inReplyToID == "" {
		return nil
	}

	parentMessage, err := s.noteRepo.GetStatus(ctx, inReplyToID)
	if err != nil {
		return errors.Join(ErrInvalidInReplyToIDConversation, err)
	}
	if parentMessage.ConversationID != "" && parentMessage.ConversationID != conversationID {
		return errors.Join(ErrInvalidInReplyToIDConversation, errors.New("reply target is in a different conversation"))
	}

	return nil
}

func (s *Service) getSendMessageAccounts(ctx context.Context, senderID, recipientID string) (*storage.Account, *storage.Account, error) {
	sender, err := s.accountRepo.GetAccount(ctx, senderID)
	if err != nil {
		return nil, nil, errors.Join(ErrGetSenderAccount, err)
	}

	recipient, err := s.accountRepo.GetAccount(ctx, recipientID)
	if err != nil {
		return nil, nil, errors.Join(ErrInvalidRecipient, err)
	}

	return sender, recipient, nil
}

func (s *Service) createSendMessageStatus(ctx context.Context, cmd *SendMessageCommand, sendCmd *SendDirectMessageCommand, sender *storage.Account, recipient *storage.Account, conversationID, recipientID string) (*models.Status, string, error) {
	messageID := uuid.New().String()
	now := time.Now().UTC()
	status := &models.Status{
		StatusID:       messageID,
		AuthorID:       cmd.SenderID,
		AuthorUsername: sender.User.Username,
		Content:        cmd.Content,
		Visibility:     VisibilityDirect,
		Sensitive:      cmd.Sensitive,
		Language:       cmd.Language,
		ConversationID: conversationID,
		InReplyToID:    cmd.InReplyToID,
		PublishedAt:    now,
		CreatedAt:      now,
		ModifiedAt:     now,
		UpdatedAt:      now,
	}

	recipientURL := fmt.Sprintf("https://%s/users/%s", s.domainName, recipient.User.Username)
	status.ToRecipients = []string{recipientURL}
	status.Note = s.buildActivityPubNote(sendCmd, messageID, sender, conversationID, map[string]*storage.Account{
		recipientID: recipient,
	})

	if err := s.noteRepo.CreateStatus(ctx, status); err != nil {
		return nil, "", errors.Join(ErrCreateDirectMessage, err)
	}

	return status, messageID, nil
}

func (s *Service) updateSendMessageParticipantStateAfterSend(ctx context.Context, conversationID, senderID, recipientID string) {
	now := time.Now().UTC()

	if err := s.updateParticipantRecord(ctx, conversationID, senderID, func(record *models.ConversationParticipantRecord) {
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
			zap.String("conversation_id", conversationID),
			zap.String("sender_id", senderID),
			zap.Error(err))
	}

	if err := s.updateParticipantRecord(ctx, conversationID, recipientID, func(record *models.ConversationParticipantRecord) {
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
			if s.shouldDeliverToInbox(ctx, recipientID, senderID) {
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
			zap.String("conversation_id", conversationID),
			zap.String("recipient_id", recipientID),
			zap.Error(err))
	}
}

// SendMessage sends a message in an existing 1:1 conversation, enforcing strict participant authz.
func (s *Service) SendMessage(ctx context.Context, cmd *SendMessageCommand) (*MessageResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	conversation, recipientID, err := s.loadConversationAndRecipientForSendMessage(ctx, cmd)
	if err != nil {
		return nil, err
	}

	sendCmd := sendDirectMessageCommandFromSendMessage(cmd, recipientID)
	if err := s.validateSendMessageCommandBasic(ctx, sendCmd); err != nil {
		return nil, errors.Join(ErrConversationValidationFailed, err)
	}

	if err := s.validateSendMessageReplyTarget(ctx, cmd.InReplyToID, conversation.ID); err != nil {
		return nil, err
	}

	sender, recipient, err := s.getSendMessageAccounts(ctx, cmd.SenderID, recipientID)
	if err != nil {
		return nil, err
	}

	status, _, err := s.createSendMessageStatus(ctx, cmd, sendCmd, sender, recipient, conversation.ID, recipientID)
	if err != nil {
		return nil, err
	}

	s.updateConversationLastStatus(ctx, conversation, status)
	s.updateSendMessageParticipantStateAfterSend(ctx, conversation.ID, cmd.SenderID, recipientID)
	s.updateConversationUnreadAfterSend(ctx, conversation, cmd.SenderID, recipientID)

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

	return s.applyMessageRequestDecision(ctx, cmd.ConversationID, cmd.UserID, models.DmRequestStateAccepted, "dm.request.accept")
}

// DeclineMessageRequest hides a request thread from the recipient by setting requestState=DECLINED.
func (s *Service) DeclineMessageRequest(ctx context.Context, cmd *DeclineMessageRequestCommand) (*ConversationResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	return s.applyMessageRequestDecision(ctx, cmd.ConversationID, cmd.UserID, models.DmRequestStateDeclined, "dm.request.decline")
}

func (s *Service) applyMessageRequestDecision(ctx context.Context, conversationID, userID string, state models.DmRequestState, auditEvent string) (*ConversationResult, error) {
	conversation, err := s.conversationRepo.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, errors.Join(ErrGetConversation, err)
	}

	if !s.isParticipant(userID, conversation.Participants) {
		return nil, ErrNotConversationParticipant
	}

	now := time.Now().UTC()
	if err := s.updateParticipantRecord(ctx, conversation.ID, userID, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}

		switch state {
		case models.DmRequestStateAccepted:
			record.RequestState = models.DmRequestStateAccepted
			record.RequestedAt = nil
			record.DeclinedAt = nil
			t := now
			record.AcceptedAt = &t
		case models.DmRequestStateDeclined:
			record.RequestState = models.DmRequestStateDeclined
			record.RequestedAt = nil
			record.AcceptedAt = nil
			t := now
			record.DeclinedAt = &t
		}
	}); err != nil {
		return nil, errors.Join(ErrMarkConversationRead, err)
	}

	if record, err := s.conversationRepo.GetConversationParticipantRecord(ctx, conversation.ID, userID); err == nil && record != nil {
		conversation.Unread = record.Unread
	}

	s.auditDMRequestEvent(ctx, auditEvent, userID, conversation.ID, true, "", map[string]any{
		"conversation_id": conversation.ID,
	})

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
		if record.DeletedAt != nil && !record.DeletedAt.IsZero() {
			return nil, ErrConversationNotFound
		}

		conversation.Unread = record.Unread
	}

	// Get conversation messages (these are statuses with this conversation ID).
	messages, err := s.noteRepo.GetConversationThread(ctx, query.ConversationID, query.Pagination)
	if err != nil {
		s.logger.Error("failed to get conversation messages", zap.String("conversation_id", query.ConversationID), zap.Error(err))
		return nil, errors.Join(ErrGetConversationMessages, err)
	}

	viewerUsername := strings.TrimSpace(query.ViewerID)
	viewerActorID := s.actorURLForUsername(viewerUsername)
	filteredMessages := s.filterConversationMessagesForViewer(messages.Items, viewerUsername, viewerActorID)
	filteredMessages, err = s.filterTombstonedConversationMessages(ctx, viewerUsername, filteredMessages)
	if err != nil {
		return nil, errors.Join(ErrGetConversationMessages, err)
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

// filterConversationMessagesForViewer filters a conversation thread to direct messages that the viewer is allowed to see.
// NOTE: Status.AuthorID is not consistently an actor URL across the codebase, so we must treat AuthorUsername as the
// canonical author check and validate recipient addressing using the viewer's actor URL.
func (s *Service) filterConversationMessagesForViewer(messages []*models.Status, viewerUsername, viewerActorID string) []*models.Status {
	filteredMessages := make([]*models.Status, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		if message.Visibility != VisibilityDirect {
			continue
		}
		if message.AuthorUsername != viewerUsername && !message.IsRecipient(viewerActorID) {
			continue
		}
		filteredMessages = append(filteredMessages, message.SanitizeForActor(viewerActorID))
	}
	return filteredMessages
}

func (s *Service) filterTombstonedConversationMessages(ctx context.Context, viewerUsername string, messages []*models.Status) ([]*models.Status, error) {
	if s.dmTombstoneRepo == nil || viewerUsername == "" || len(messages) == 0 {
		return messages, nil
	}

	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		if id := strings.TrimSpace(message.StatusID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return messages, nil
	}

	tombstoned, err := s.dmTombstoneRepo.TombstonesByStatusID(ctx, viewerUsername, ids)
	if err != nil {
		return nil, err
	}

	kept := messages[:0]
	for _, message := range messages {
		if message == nil {
			continue
		}
		if tombstoned[message.StatusID] {
			continue
		}
		kept = append(kept, message)
	}
	return kept, nil
}

func (s *Service) auditEvent(ctx context.Context, eventType, severity, username, userID string, success bool, failureReason string, metadata map[string]any) {
	if s == nil || s.auditRepo == nil {
		return
	}

	if metadata == nil {
		metadata = map[string]any{}
	}

	if err := s.auditRepo.StoreAuditEvent(ctx, eventType, severity, username, userID, "", "", "", "", "", success, failureReason, metadata); err != nil {
		s.logger.Debug("failed to store audit event",
			zap.String("event_type", eventType),
			zap.Error(err))
	}
}

func (s *Service) auditDMEvent(ctx context.Context, cmd *SendDirectMessageCommand, conversationID string, success bool, failureReason string, metadata map[string]any) {
	if s == nil || s.auditRepo == nil || cmd == nil {
		return
	}

	severity := "LOW"
	if !success {
		severity = "MEDIUM"
		if strings.Contains(failureReason, "rate_limited") || strings.Contains(failureReason, "blocked") {
			severity = "HIGH"
		}
	}

	merged := map[string]any{
		"conversation_id": strings.TrimSpace(conversationID),
		"sender_id":       strings.TrimSpace(cmd.SenderID),
	}
	for k, v := range metadata {
		merged[k] = v
	}

	s.auditEvent(ctx, "dm.send", severity, cmd.SenderID, cmd.SenderID, success, failureReason, merged)
}

func (s *Service) auditDMRequestEvent(ctx context.Context, eventType, username, conversationID string, success bool, failureReason string, metadata map[string]any) {
	if s == nil || s.auditRepo == nil {
		return
	}

	severity := "LOW"
	if !success {
		severity = "MEDIUM"
	}

	merged := map[string]any{
		"conversation_id": strings.TrimSpace(conversationID),
	}
	for k, v := range metadata {
		merged[k] = v
	}

	s.auditEvent(ctx, eventType, severity, username, username, success, failureReason, merged)
}

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
			Summary:   cmd.SpoilerText,
		},
		Content:          cmd.Content,
		AttributedTo:     fmt.Sprintf("https://%s/users/%s", s.domainName, sender.User.Username),
		Visibility:       VisibilityDirect,
		ConversationID:   conversationID,
		AgentAttribution: cmd.AgentAttribution,
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

var dmMentionHandleRegex = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_])@([a-zA-Z0-9_]+(?:@[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?)?)`)

// ExtractMentionHandles exposes the DM-safe mention parsing used by the conversation service so
// REST handlers can resolve direct-message recipients consistently with note mention handling.
func ExtractMentionHandles(content string) []string {
	matches := dmMentionHandleRegex.FindAllStringSubmatch(content, -1)
	mentions := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			mentions = append(mentions, match[1])
		}
	}
	return mentions
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
	canonicalUserID := models.CanonicalConversationParticipantID(userID)
	for _, participant := range participants {
		if models.CanonicalConversationParticipantID(participant) == canonicalUserID {
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
	if participantRecordSnapshotCorrupt(record) {
		s.logger.Warn("participant record snapshot missing canonical conversation identity",
			zap.String("conversation_id", conversationID),
			zap.String("participant_id", participantID))
	}

	mutator(record)
	return s.conversationRepo.UpdateConversationParticipantRecord(ctx, record)
}

func participantRecordSnapshotCorrupt(record *models.ConversationParticipantRecord) bool {
	if record == nil || record.ConversationData == nil {
		return true
	}
	return strings.TrimSpace(record.ConversationData.ID) == "" || len(record.ConversationData.Participants) == 0
}

// DeleteConversation implements delete-for-me semantics for a DM conversation.
// It marks the viewer's participant record DeletedAt without deleting shared conversation data.
func (s *Service) DeleteConversation(ctx context.Context, cmd *DeleteConversationCommand) (*ConversationResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	conversationID := strings.TrimSpace(cmd.ConversationID)
	userID := strings.TrimSpace(cmd.UserID)
	if conversationID == "" || userID == "" {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	s.logger.Info("deleting conversation",
		zap.String("conversation_id", conversationID),
		zap.String("user_id", userID))

	// Get conversation to verify it exists and user is a participant
	conversation, err := s.conversationRepo.GetConversation(ctx, conversationID)
	if err != nil {
		if isNotFoundError(err) {
			s.logger.Warn("conversation not found", zap.String("conversation_id", conversationID))
			return nil, ErrConversationNotFound
		}
		s.logger.Error("failed to get conversation", zap.String("conversation_id", conversationID), zap.Error(err))
		return nil, errors.Join(ErrGetConversation, err)
	}

	participantIDForRecord := userID

	// Check if user is a participant
	if !s.isParticipant(userID, conversation.Participants) {
		// Get user's account to check actor ID
		account, err := s.accountRepo.GetAccount(ctx, userID)
		if err != nil {
			s.logger.Error("failed to get account", zap.String("user_id", userID), zap.Error(err))
			return nil, errors.Join(ErrGetAccount, err)
		}

		// Check with actor ID as well
		if account.Actor == nil || !s.isParticipant(account.Actor.ID, conversation.Participants) {
			s.logger.Warn("user is not a conversation participant (checked actor ID)", zap.String("user_id", userID), zap.String("conversation_id", conversationID))
			return nil, ErrNotConversationParticipant
		}

		participantIDForRecord = account.Actor.ID
	}

	now := time.Now().UTC()
	if err := s.updateParticipantRecord(ctx, conversationID, participantIDForRecord, func(record *models.ConversationParticipantRecord) {
		if record == nil {
			return
		}
		t := now
		record.DeletedAt = &t
	}); err != nil {
		if errors.Is(err, storage.ErrNotFound) || isNotFoundError(err) {
			// If the participant record doesn't exist, the conversation is already effectively hidden
			// for this user. Treat as an idempotent delete-for-me.
			events := s.emitConversationDeletedEvents(ctx, conversation, userID)
			return &ConversationResult{Conversation: conversation, Events: events}, nil
		}

		s.logger.Error("failed to mark conversation deleted for viewer",
			zap.String("conversation_id", conversationID),
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, errors.Join(ErrDeleteConversation, err)
	}

	events := s.emitConversationDeletedEvents(ctx, conversation, userID)

	return &ConversationResult{
		Conversation: conversation,
		Events:       events,
	}, nil
}

// DeleteMessage implements delete-for-me semantics for a direct message (Status).
// It creates a per-viewer tombstone keyed by (viewerUsername, statusID).
func (s *Service) DeleteMessage(ctx context.Context, cmd *DeleteMessageCommand) (bool, error) {
	if cmd == nil {
		return false, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	viewerUsername := strings.TrimSpace(cmd.UserID)
	messageID := strings.TrimSpace(cmd.MessageID)
	if viewerUsername == "" || messageID == "" {
		return false, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	if s.dmTombstoneRepo == nil {
		return false, errors.Join(ErrDeleteMessage, errors.New("direct message tombstone repository is not configured"))
	}

	status, err := s.noteRepo.GetStatus(ctx, messageID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || isNotFoundError(err) {
			// Idempotent delete semantics; do not reveal whether the message exists.
			return true, nil
		}
		return false, err
	}
	if status == nil {
		return true, nil
	}
	if status.Visibility != models.VisibilityDirect {
		return false, errors.Join(ErrConversationValidationFailed, errors.New("only direct messages can be deleted via deleteMessage"))
	}
	if strings.TrimSpace(status.ConversationID) == "" {
		return false, errors.Join(ErrConversationValidationFailed, errors.New("direct message is missing conversation id"))
	}

	conversation, err := s.conversationRepo.GetConversation(ctx, status.ConversationID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || isNotFoundError(err) {
			return true, nil
		}
		return false, errors.Join(ErrGetConversation, err)
	}
	if conversation == nil {
		return true, nil
	}

	isParticipant := s.isParticipant(viewerUsername, conversation.Participants)
	if !isParticipant {
		account, err := s.accountRepo.GetAccount(ctx, viewerUsername)
		if err != nil {
			return false, errors.Join(ErrGetAccount, err)
		}
		if account.Actor == nil || !s.isParticipant(account.Actor.ID, conversation.Participants) {
			return false, ErrNotConversationParticipant
		}
	}

	if err := s.dmTombstoneRepo.CreateTombstone(ctx, viewerUsername, status.StatusID); err != nil {
		return false, errors.Join(ErrDeleteMessage, err)
	}

	return true, nil
}

func (s *Service) getConversationLastStatusFromPage(ctx context.Context, viewerUsername, viewerActorID string, items []*models.Status) (*models.Status, error) {
	if len(items) == 0 {
		return nil, nil
	}

	candidates := make([]*models.Status, 0, len(items))
	ids := make([]string, 0, len(items))
	for _, message := range items {
		if message == nil {
			continue
		}
		if message.Visibility != models.VisibilityDirect {
			continue
		}
		if message.AuthorUsername != viewerUsername && !message.IsRecipient(viewerActorID) {
			continue
		}
		candidates = append(candidates, message)
		ids = append(ids, message.StatusID)
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	tombstoned := map[string]bool{}
	if s.dmTombstoneRepo != nil && len(ids) > 0 {
		var err error
		tombstoned, err = s.dmTombstoneRepo.TombstonesByStatusID(ctx, viewerUsername, ids)
		if err != nil {
			return nil, err
		}
	}

	for _, message := range candidates {
		if tombstoned[message.StatusID] {
			continue
		}
		return message.SanitizeForActor(viewerActorID), nil
	}

	return nil, nil
}

// GetConversationLastStatus returns the latest direct message in the conversation that is visible
// to the viewer and is not deleted-for-viewer.
func (s *Service) GetConversationLastStatus(ctx context.Context, conversationID, viewerID string) (*models.Status, error) {
	viewerUsername := strings.TrimSpace(viewerID)
	conversationID = strings.TrimSpace(conversationID)
	if viewerUsername == "" || conversationID == "" {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}

	viewerActorID := s.actorURLForUsername(viewerUsername)
	if viewerActorID == "" {
		return nil, nil
	}

	cursor := ""
	const maxIterations = 10
	for iter := 0; iter < maxIterations; iter++ {
		page, err := s.noteRepo.GetConversationThreadReverse(ctx, conversationID, interfaces.PaginationOptions{
			Limit:  50,
			Cursor: cursor,
		})
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) || isNotFoundError(err) {
				return nil, nil
			}
			return nil, err
		}
		if page == nil || len(page.Items) == 0 {
			return nil, nil
		}

		if message, err := s.getConversationLastStatusFromPage(ctx, viewerUsername, viewerActorID, page.Items); err != nil {
			return nil, err
		} else if message != nil {
			return message, nil
		}

		if !page.HasMore || strings.TrimSpace(page.NextCursor) == "" {
			return nil, nil
		}
		cursor = page.NextCursor
	}

	return nil, nil
}

// emitConversationDeletedEvents creates events for viewer-only conversation deletion.
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

	return events
}
