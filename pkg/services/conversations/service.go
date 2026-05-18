// Package conversations provides the core Conversations Service for the Lesser project's API alignment.
// This service handles direct message conversations including creation, participant management,
// read status tracking, and real-time event emission. It supports federation for remote participants
// and maintains privacy by ensuring only participants can access conversation content.
package conversations

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/security/htmlsafe"
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

type remoteActorResolver interface {
	ResolveActor(ctx context.Context, handle string) (*activitypub.Actor, error)
}

type directMessageTombstoneRepository interface {
	CreateTombstone(ctx context.Context, viewerUsername, statusID string) error
	TombstonesByStatusID(ctx context.Context, viewerUsername string, statusIDs []string) (map[string]bool, error)
}

type directMessageSendCapability interface {
	TransactionalDirectMessageSendEnabled() bool
}

type directMessageWriteFreezeChecker interface {
	DirectMessageWritesFrozen(ctx context.Context) (bool, error)
}

type fixedWindowRateLimiter interface {
	CheckFixedWindowRateLimit(ctx context.Context, identifier, bucket string, limit int, window time.Duration) (allowed bool, remaining int, resetTime time.Time, err error)
}

func (s *Service) logDirectMessageFailure(message, phase string, status *models.Status, err error, requestErr error) {
	if s == nil || s.logger == nil || requestErr == nil {
		return
	}

	fields := []zap.Field{
		zap.String("phase", phase),
		zap.Strings("root_causes", common.ErrorLeafMessages(err)),
		zap.Error(requestErr),
	}
	if status != nil {
		fields = append(fields,
			zap.String("status_id", status.StatusID),
			zap.String("conversation_id", status.ConversationID))
	}

	s.logger.Error(message, fields...)
}

type directMessageSendAttempt struct {
	conversation  *models.Conversation
	status        *models.Status
	messageID     string
	willBeRequest bool
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

func (s *Service) ensureDirectMessageWritesAllowed(ctx context.Context) error {
	if s == nil || s.conversationRepo == nil {
		return nil
	}

	checker, ok := s.conversationRepo.(directMessageWriteFreezeChecker)
	if !ok {
		return nil
	}

	frozen, err := checker.DirectMessageWritesFrozen(ctx)
	if err != nil {
		return err
	}
	if frozen {
		return ErrDirectMessageWritesFrozen
	}
	return nil
}

// Command structs for operations

const (
	dmSendTotalLimit            = 60
	dmSendTotalWindow           = time.Minute
	dmRequestTotalLimit         = 20
	dmRequestTotalWindow        = time.Hour
	dmRequestPerRecipientLimit  = 3
	dmRequestPerRecipientWindow = 24 * time.Hour
	directMessageSendRetryLimit = 3
)

var (
	errDirectMessageRemoteRecipientActorRequired = errors.New("remote recipient actor id required")
	errDirectMessageStatusContractRequired       = errors.New("canonical status create contract required for transactional direct message send")
)

// SendDirectMessageCommand contains all data needed to send a direct message
type SendDirectMessageCommand struct {
	SenderID               string                             `json:"sender_id" validate:"required"`
	Recipients             []string                           `json:"recipients" validate:"required,min=1"`
	Content                string                             `json:"content" validate:"required,max=5000"`
	Sensitive              bool                               `json:"sensitive"`
	SpoilerText            string                             `json:"spoiler_text"`
	Language               string                             `json:"language"`
	MediaIDs               []string                           `json:"media_ids"`
	InReplyToID            string                             `json:"in_reply_to_id"` // Can reply to messages in the conversation
	AgentAttribution       *activitypub.AgentPostAttribution  `json:"agent_attribution,omitempty"`
	ResolvedRecipientActor *activitypub.Actor                 `json:"-"`
	ResolvedRecipientRef   *models.ConversationParticipantRef `json:"-"`
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
	if err := s.ensureDirectMessageWritesAllowed(ctx); err != nil {
		return nil, err
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

	if conversation == nil {
		conversationID := uuid.New().String()
		conversation = &models.Conversation{
			ID:           conversationID,
			Participants: participants,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		initialStates := buildCreateConversationParticipantStates(conversation, creatorID, participantID)
		if err := s.conversationRepo.CreateConversationWithParticipantStates(ctx, conversation, participants, initialStates); err != nil {
			if errors.Is(err, storage.ErrAlreadyExists) {
				existingConversation, reloadErr := s.conversationRepo.GetConversationByParticipants(ctx, participants)
				if reloadErr == nil && existingConversation != nil {
					conversation = existingConversation
				} else {
					s.logger.Warn("conversation create lost a lookup race and canonical reload failed",
						zap.Strings("participants", participants),
						zap.Error(reloadErr))
					return nil, errors.Join(ErrCreateConversation, err)
				}
			} else {
				return nil, errors.Join(ErrCreateConversation, err)
			}
		}
	}

	// Populate per-viewer unread state.
	if state, err := s.conversationRepo.GetUserConversationState(ctx, creatorID, conversation.ID); err == nil && state != nil {
		conversation.Unread = state.Unread
	}

	return &ConversationResult{
		Conversation: conversation,
		Events:       []*streaming.Event{},
	}, nil
}

func buildCreateConversationParticipantStates(conversation *models.Conversation, creatorID, participantID string) []*models.UserConversationState {
	if conversation == nil {
		return nil
	}

	updatedAt := conversation.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	createdAt := conversation.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	acceptedAt := updatedAt

	return []*models.UserConversationState{
		{
			ViewerID:       creatorID,
			ConversationID: conversation.ID,
			CounterpartID:  participantID,
			Folder:         models.UserConversationFolderInbox,
			RequestState:   models.DmRequestStateAccepted,
			AcceptedAt:     &acceptedAt,
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
			SortAt:         updatedAt,
		},
		{
			ViewerID:       participantID,
			ConversationID: conversation.ID,
			CounterpartID:  creatorID,
			Folder:         models.UserConversationFolderHidden,
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
			SortAt:         updatedAt,
		},
	}
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
	if strings.TrimSpace(recipientID) == "" {
		s.auditDMEvent(ctx, cmd, "", false, "invalid_recipient", map[string]any{
			"recipient_id": recipientID,
		})
		return "", errors.Join(ErrConversationValidationFailed, ErrInvalidRecipient)
	}
	if isDirectMessageSelfRecipient(cmd.SenderID, recipientID, s.domainName) {
		s.auditDMEvent(ctx, cmd, "", false, "direct_self_post_not_allowed", map[string]any{
			"recipient_id": recipientID,
		})
		return "", errors.Join(ErrDirectSelfPostNotAllowed, ErrConversationValidationFailed, ErrInvalidRecipient)
	}

	return recipientID, nil
}

func isDirectMessageSelfRecipient(senderID, recipientID, localDomain string) bool {
	sender := models.CanonicalConversationParticipantID(senderID)
	recipient := strings.TrimSpace(recipientID)
	if sender == "" || recipient == "" {
		return false
	}

	if models.CanonicalConversationParticipantID(recipient) == sender {
		return true
	}

	normalizedLocalDomain := normalizeDirectMessageMentionDomain(localDomain)
	if username, domain := directMessageMentionHandleParts(recipient); username != "" && domain != "" &&
		normalizedLocalDomain != "" && normalizeDirectMessageMentionDomain(domain) == normalizedLocalDomain &&
		models.CanonicalConversationParticipantID(username) == sender {
		return true
	}

	parsed, err := url.Parse(recipient)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" ||
		normalizedLocalDomain == "" || normalizeDirectMessageMentionDomain(parsed.Hostname()) != normalizedLocalDomain {
		return false
	}

	path := strings.Trim(parsed.EscapedPath(), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] != "users" {
		return false
	}
	username, err := url.PathUnescape(parts[1])
	if err != nil {
		username = parts[1]
	}
	return models.CanonicalConversationParticipantID(username) == sender
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
	return s.consumeDirectMessageRateLimit(ctx, cmd, "", recipientID, "dm_send_total", dmSendTotalLimit, dmSendTotalWindow, "rate_limited_send_total")
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
	if isRemoteDirectMessageRecipientIdentifier(recipientID, s.domainName) {
		recipient, recipientRef, err := s.resolveRemoteDirectMessageRecipient(ctx, recipientID)
		if err != nil {
			s.logger.Error("invalid remote recipient", zap.String("recipient_id", recipientID), zap.Error(err))
			s.auditDMEvent(ctx, cmd, "", false, "resolve_remote_recipient_failed", map[string]any{
				"recipient_id": recipientID,
			})
			return nil, nil, nil, errors.Join(ErrInvalidRecipient, err)
		}
		cmd.ResolvedRecipientActor = recipient.Actor
		cmd.ResolvedRecipientRef = recipientRef
		recipientAccounts[recipientRef.ParticipantID] = recipient
		return sender, recipient, recipientAccounts, nil
	}

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

func isRemoteDirectMessageRecipientIdentifier(recipientID, localDomain string) bool {
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return false
	}

	normalizedLocalDomain := normalizeDirectMessageMentionDomain(localDomain)
	if parsed, err := url.Parse(recipientID); err == nil && parsed.Scheme != "" && parsed.Hostname() != "" {
		return normalizeDirectMessageMentionDomain(parsed.Hostname()) != normalizedLocalDomain
	}

	_, domain := directMessageMentionHandleParts(recipientID)
	return domain != "" && normalizeDirectMessageMentionDomain(domain) != normalizedLocalDomain
}

func (s *Service) resolveRemoteDirectMessageRecipient(ctx context.Context, recipientID string) (*storage.Account, *models.ConversationParticipantRef, error) {
	resolver, ok := s.federation.(remoteActorResolver)
	if !ok || resolver == nil {
		return nil, nil, errDirectMessageRemoteRecipientActorRequired
	}

	actor, err := resolver.ResolveActor(ctx, recipientID)
	if err != nil {
		return nil, nil, err
	}
	if actor == nil || strings.TrimSpace(actor.ID) == "" {
		return nil, nil, errDirectMessageRemoteRecipientActorRequired
	}

	username, domain := normalizeDirectMessageMentionAccount(recipientID, nil, s.domainName)
	if username == "" || domain == "" {
		identity := federationActorIdentityForDirectMessage(actor, s.domainName)
		username = identity.username
		domain = identity.domain
	}
	acct := strings.TrimSpace(username)
	if domain != "" && !strings.Contains(acct, "@") {
		acct += "@" + domain
	}
	now := time.Now().UTC()
	ref := models.NormalizeConversationParticipantRef(models.ConversationParticipantRef{
		ParticipantType: models.ConversationParticipantTypeRemoteActor,
		ParticipantID:   actor.ID,
		Acct:            acct,
		Domain:          domain,
		ResolvedAt:      &now,
	})

	displayName := actor.Name
	if displayName == "" {
		displayName = username
	}
	account := &storage.Account{
		User: &storage.User{
			ID:          actor.ID,
			Username:    actor.ID,
			DisplayName: displayName,
			URL:         actor.URL,
		},
		Actor: actor,
	}
	return account, &ref, nil
}

type directMessageActorIdentity struct {
	username string
	domain   string
}

func federationActorIdentityForDirectMessage(actor *activitypub.Actor, localDomain string) directMessageActorIdentity {
	if actor == nil {
		return directMessageActorIdentity{}
	}
	username := strings.TrimSpace(actor.PreferredUsername)
	domain := ""
	if parsed, err := url.Parse(strings.TrimSpace(actor.ID)); err == nil && parsed.Hostname() != "" {
		domain = normalizeDirectMessageMentionDomain(parsed.Hostname())
	}
	if username == "" {
		username = extractUsernameFromActorIdentifier(actor.ID)
	}
	if domain == normalizeDirectMessageMentionDomain(localDomain) {
		domain = ""
	}
	return directMessageActorIdentity{username: username, domain: domain}
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
		if cmd.ResolvedRecipientRef != nil && cmd.ResolvedRecipientRef.ParticipantID != "" {
			cloned.Recipients[0] = cmd.ResolvedRecipientRef.ParticipantID
		} else {
			cloned.Recipients[0] = resolvedLegacyLocalAccountID(cloned.Recipients[0], recipient)
		}
	}
	cloned.ResolvedRecipientActor = cmd.ResolvedRecipientActor
	cloned.ResolvedRecipientRef = cmd.ResolvedRecipientRef

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

func (s *Service) enforceDirectMessageRequestRateLimit(ctx context.Context, cmd *SendDirectMessageCommand, conversationID, recipientID string) error {
	if err := s.consumeDirectMessageRateLimit(ctx, cmd, conversationID, recipientID, "dm_request_total", dmRequestTotalLimit, dmRequestTotalWindow, "rate_limited_request_total"); err != nil {
		return err
	}
	return s.consumeDirectMessageRateLimit(ctx, cmd, conversationID, recipientID, fmt.Sprintf("dm_request_to:%s", recipientID), dmRequestPerRecipientLimit, dmRequestPerRecipientWindow, "rate_limited_request_to_recipient")
}

func (s *Service) consumeDirectMessageRateLimit(ctx context.Context, cmd *SendDirectMessageCommand, conversationID, recipientID, bucket string, limit int, window time.Duration, auditReason string) error {
	if s.rateLimitRepo == nil || directMessageRateLimitingDisabled() {
		return nil
	}

	identifier := fmt.Sprintf("dm:%s", cmd.SenderID)
	if limiter, ok := s.rateLimitRepo.(fixedWindowRateLimiter); ok {
		allowed, _, _, err := limiter.CheckFixedWindowRateLimit(ctx, identifier, bucket, limit, window)
		if err != nil {
			s.logger.Warn("failed to consume direct message rate limit; allowing send",
				zap.String("sender_id", cmd.SenderID),
				zap.String("recipient_id", recipientID),
				zap.String("bucket", bucket),
				zap.Error(err))
			s.auditDMEvent(ctx, cmd, conversationID, true, auditReason+"_storage_error_fail_open", map[string]any{
				"recipient_id": recipientID,
				"bucket":       bucket,
			})
			return nil
		}
		if !allowed {
			s.auditDMEvent(ctx, cmd, conversationID, false, auditReason, map[string]any{
				"recipient_id": recipientID,
				"bucket":       bucket,
			})
			return storage.ErrRateLimited
		}
		return nil
	}

	if err := s.rateLimitRepo.CheckAPIRateLimit(ctx, identifier, bucket, limit, window); err != nil {
		s.auditDMEvent(ctx, cmd, conversationID, false, auditReason, map[string]any{
			"recipient_id": recipientID,
			"bucket":       bucket,
		})
		return err
	}

	return nil
}

func directMessageRateLimitingDisabled() bool {
	cfg := config.Get()
	return cfg != nil && cfg.DisableRateLimiting
}

func (s *Service) createDirectMessageStatus(_ context.Context, cmd *SendDirectMessageCommand, sender *storage.Account, recipientAccounts map[string]*storage.Account, conversationID, _ string) (*models.Status, string, error) {
	messageID := uuid.New().String()
	now := time.Now().UTC()
	noteCmd := sanitizedDirectMessageCommand(cmd)

	status := &models.Status{
		StatusID:       messageID,
		AuthorID:       cmd.SenderID,
		AuthorUsername: sender.User.Username,
		Content:        noteCmd.Content,
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

	var err error
	status.Note, err = s.buildActivityPubNote(noteCmd, messageID, sender, conversationID, recipientAccounts)
	if err != nil {
		return nil, "", err
	}
	status.ToRecipients = append([]string(nil), status.Note.To...)
	status.SyncTagFieldsFromNote()

	return status, messageID, nil
}

func (s *Service) resolveDirectMessageConversationForCommand(ctx context.Context, cmd *SendDirectMessageCommand, recipientID string) (*models.Conversation, bool, error) {
	senderID := cmd.SenderID
	participantRefs := buildDirectMessageParticipantRefs(senderID, recipientID, cmd.ResolvedRecipientRef)
	participants := models.ConversationParticipantIDsFromRefs(participantRefs)
	if len(participants) == 0 {
		participants = models.CanonicalConversationParticipants([]string{senderID, recipientID})
	}

	conversation, err := s.lookupDirectMessageConversationForSend(ctx, participants, participantRefs)
	if err == nil && conversation != nil {
		return conversation, false, nil
	}
	if err != nil && !isNotFoundError(err) {
		return nil, false, errors.Join(ErrLookupExistingConversation, err)
	}

	now := time.Now().UTC()
	return &models.Conversation{
		ID:              uuid.New().String(),
		Participants:    participants,
		ParticipantRefs: participantRefs,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true, nil
}

type typedConversationLookupRepository interface {
	GetConversationByParticipantRefs(ctx context.Context, refs []models.ConversationParticipantRef) (*models.Conversation, error)
}

func (s *Service) lookupDirectMessageConversationForSend(ctx context.Context, participants []string, participantRefs []models.ConversationParticipantRef) (*models.Conversation, error) {
	if len(participantRefs) > 0 {
		for _, ref := range participantRefs {
			if ref.ParticipantType == models.ConversationParticipantTypeRemoteActor {
				if typedRepo, ok := s.conversationRepo.(typedConversationLookupRepository); ok {
					return typedRepo.GetConversationByParticipantRefs(ctx, participantRefs)
				}
				break
			}
		}
	}
	return s.conversationRepo.GetConversationByParticipants(ctx, participants)
}

func buildDirectMessageParticipantRefs(senderID, recipientID string, recipientRef *models.ConversationParticipantRef) []models.ConversationParticipantRef {
	refs := []models.ConversationParticipantRef{{
		ParticipantType: models.ConversationParticipantTypeLocalUser,
		ParticipantID:   senderID,
	}}
	if recipientRef != nil {
		refs = append(refs, *recipientRef)
	} else {
		refs = append(refs, models.ConversationParticipantRef{
			ParticipantType: models.ConversationParticipantTypeLocalUser,
			ParticipantID:   recipientID,
		})
	}
	return models.NormalizeConversationParticipantRefs(refs)
}

func (s *Service) getUserConversationStateForSend(ctx context.Context, conversationID, participantID string) (*models.UserConversationState, error) {
	stateContract, err := s.conversationRepo.GetUserConversationState(ctx, participantID, conversationID)
	switch {
	case err == nil && stateContract == nil:
		return nil, nil
	case err == nil:
		return userConversationStateFromContract(nil, participantID, "", stateContract), nil
	case errors.Is(err, storage.ErrNotFound), isNotFoundError(err):
		return nil, nil
	default:
		return nil, err
	}
}

func defaultSendConversationState(conversation *models.Conversation, viewerID, counterpartID string) *models.UserConversationState {
	now := time.Now().UTC()
	state := &models.UserConversationState{
		ViewerID:      viewerID,
		CounterpartID: counterpartID,
		Folder:        models.UserConversationFolderHidden,
		SortAt:        now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if conversation != nil {
		state.ConversationID = conversation.ID
		state.CreatedAt = conversation.CreatedAt.UTC()
		state.UpdatedAt = conversation.UpdatedAt.UTC()
		if state.CreatedAt.IsZero() {
			state.CreatedAt = now
		}
		if state.UpdatedAt.IsZero() {
			state.UpdatedAt = now
		}
		if !conversation.LastMessageTime.IsZero() {
			state.SortAt = conversation.LastMessageTime.UTC()
			state.PreviewStatusPublishedAt = conversation.LastMessageTime.UTC()
		} else if !conversation.UpdatedAt.IsZero() {
			state.SortAt = conversation.UpdatedAt.UTC()
		}
		state.PreviewStatusID = conversation.LastStatusID
	}
	return state
}

func userConversationStateFromContract(conversation *models.Conversation, viewerID, counterpartID string, stateContract *interfaces.UserConversationStateContract) *models.UserConversationState {
	state := defaultSendConversationState(conversation, viewerID, counterpartID)
	if stateContract == nil {
		return state
	}

	if stateContract.ViewerID != "" {
		state.ViewerID = stateContract.ViewerID
	}
	if stateContract.ConversationID != "" {
		state.ConversationID = stateContract.ConversationID
	}
	if stateContract.CounterpartID != "" {
		state.CounterpartID = stateContract.CounterpartID
	}
	if stateContract.CounterpartType != "" {
		state.CounterpartType = stateContract.CounterpartType
	}
	if stateContract.CounterpartAcct != "" {
		state.CounterpartAcct = stateContract.CounterpartAcct
	}
	if stateContract.CounterpartDomain != "" {
		state.CounterpartDomain = stateContract.CounterpartDomain
	}
	state.CounterpartResolvedAt = stateContract.CounterpartResolvedAt
	if stateContract.Folder != "" {
		state.Folder = stateContract.Folder
	}
	if stateContract.RequestState != "" {
		state.RequestState = stateContract.RequestState
	}
	state.RequestedAt = stateContract.RequestedAt
	state.AcceptedAt = stateContract.AcceptedAt
	state.DeclinedAt = stateContract.DeclinedAt
	state.DeletedAt = stateContract.DeletedAt
	state.Unread = stateContract.Unread
	state.LastReadAt = stateContract.LastReadAt
	if stateContract.PreviewStatusID != "" {
		state.PreviewStatusID = stateContract.PreviewStatusID
	}
	if !stateContract.PreviewStatusPublishedAt.IsZero() {
		state.PreviewStatusPublishedAt = stateContract.PreviewStatusPublishedAt.UTC()
	}
	if !stateContract.SortAt.IsZero() {
		state.SortAt = stateContract.SortAt.UTC()
	}
	if !stateContract.CreatedAt.IsZero() {
		state.CreatedAt = stateContract.CreatedAt.UTC()
	}
	if !stateContract.UpdatedAt.IsZero() {
		state.UpdatedAt = stateContract.UpdatedAt.UTC()
	}
	return state
}

func userConversationStateContractFromModel(state *models.UserConversationState) *interfaces.UserConversationStateContract {
	if state == nil {
		return nil
	}
	return &interfaces.UserConversationStateContract{
		ViewerID:                 state.ViewerID,
		ConversationID:           state.ConversationID,
		CounterpartID:            state.CounterpartID,
		CounterpartType:          state.CounterpartType,
		CounterpartAcct:          state.CounterpartAcct,
		CounterpartDomain:        state.CounterpartDomain,
		CounterpartResolvedAt:    state.CounterpartResolvedAt,
		Folder:                   state.Folder,
		RequestState:             state.RequestState,
		PreviewStatusID:          state.PreviewStatusID,
		PreviewStatusPublishedAt: state.PreviewStatusPublishedAt,
		SortAt:                   state.SortAt,
		Unread:                   state.Unread,
		LastReadAt:               state.LastReadAt,
		DeletedAt:                state.DeletedAt,
		RequestedAt:              state.RequestedAt,
		AcceptedAt:               state.AcceptedAt,
		DeclinedAt:               state.DeclinedAt,
		CreatedAt:                state.CreatedAt,
		UpdatedAt:                state.UpdatedAt,
	}
}

func userConversationStateForSend(conversation *models.Conversation, viewerID, counterpartID string, state *models.UserConversationState) *models.UserConversationState {
	return userConversationStateFromContract(conversation, viewerID, counterpartID, userConversationStateContractFromModel(state))
}

func conversationParticipantRefByID(conversation *models.Conversation, participantID string) *models.ConversationParticipantRef {
	if conversation == nil || participantID == "" {
		return nil
	}
	canonicalParticipantID := models.CanonicalConversationParticipantID(participantID)
	for _, ref := range models.NormalizeConversationParticipantRefs(conversation.ParticipantRefs) {
		if models.CanonicalConversationParticipantID(ref.ParticipantID) == canonicalParticipantID {
			refCopy := ref
			return &refCopy
		}
	}
	return nil
}

func applyConversationCounterpartRef(state *models.UserConversationState, ref *models.ConversationParticipantRef) {
	if state == nil || ref == nil {
		return
	}
	state.CounterpartType = ref.ParticipantType
	state.CounterpartAcct = ref.Acct
	state.CounterpartDomain = ref.Domain
	state.CounterpartResolvedAt = ref.ResolvedAt
}

// applyDirectMessageStatusPreview applies the status projection fields to a
// UserConversationState for a newly created direct message. This is a narrow
// private helper that keeps sender/recipient status-projection branches from
// drifting — the same four-field assignment is used for both local and remote
// recipient paths.
func applyDirectMessageStatusPreview(state *models.UserConversationState, statusID string, publishedAt time.Time) {
	if state == nil {
		return
	}
	state.PreviewStatusID = statusID
	state.PreviewStatusPublishedAt = publishedAt
	state.SortAt = publishedAt
	state.UpdatedAt = publishedAt
}

func isRemoteConversationParticipant(conversation *models.Conversation, participantID string) bool {
	if ref := conversationParticipantRefByID(conversation, participantID); ref != nil {
		return ref.ParticipantType == models.ConversationParticipantTypeRemoteActor
	}
	return strings.Contains(strings.TrimSpace(participantID), "://")
}

func (s *Service) evaluateDirectMessageRequestPolicyForState(ctx context.Context, cmd *SendDirectMessageCommand, conversationID, recipientID string, recipientRequestState models.DmRequestState) (willBeRequest bool, deliversToInbox bool, _ error) {
	if cmd != nil && cmd.ResolvedRecipientRef != nil && cmd.ResolvedRecipientRef.ParticipantType == models.ConversationParticipantTypeRemoteActor {
		return false, false, nil
	}

	deliversToInbox = s.shouldDeliverToInbox(ctx, recipientID, cmd.SenderID)

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
		return false, deliversToInbox, ErrMessageRequestPending
	}

	if willBeRequest && len(cmd.MediaIDs) > 0 {
		s.auditDMEvent(ctx, cmd, conversationID, false, "media_not_allowed_in_request", map[string]any{
			"recipient_id": recipientID,
			"media_count":  len(cmd.MediaIDs),
		})
		return false, deliversToInbox, ErrMessageRequestMediaNotAllowed
	}

	return willBeRequest, deliversToInbox, nil
}

func buildDirectMessageParticipantStatesForSend(
	conversation *models.Conversation,
	status *models.Status,
	senderID string,
	recipientID string,
	senderState *models.UserConversationState,
	recipientState *models.UserConversationState,
	deliversToInbox bool,
) []*models.UserConversationState {
	now := time.Now().UTC()
	if status != nil && !status.PublishedAt.IsZero() {
		now = status.PublishedAt.UTC()
	}

	senderState = userConversationStateForSend(conversation, senderID, recipientID, senderState)
	senderState.CounterpartID = recipientID
	applyConversationCounterpartRef(senderState, conversationParticipantRefByID(conversation, recipientID))
	senderState.Folder = models.UserConversationFolderInbox
	senderState.RequestState = models.DmRequestStateAccepted
	senderState.RequestedAt = nil
	senderState.DeclinedAt = nil
	senderState.DeletedAt = nil
	senderState.Unread = false
	senderState.LastReadAt = &now
	if senderState.AcceptedAt == nil {
		t := now
		senderState.AcceptedAt = &t
	}

	if status != nil {
		applyDirectMessageStatusPreview(senderState, status.StatusID, now)
	}
	if isRemoteConversationParticipant(conversation, recipientID) {
		return []*models.UserConversationState{senderState}
	}

	recipientState = userConversationStateForSend(conversation, recipientID, senderID, recipientState)
	recipientState.CounterpartID = senderID
	applyConversationCounterpartRef(recipientState, conversationParticipantRefByID(conversation, senderID))
	recipientState.DeletedAt = nil
	recipientState.Unread = true
	recipientState.LastReadAt = nil

	switch recipientState.RequestState {
	case models.DmRequestStateAccepted:
		recipientState.Folder = models.UserConversationFolderInbox
		recipientState.RequestedAt = nil
		recipientState.DeclinedAt = nil
		if recipientState.AcceptedAt == nil {
			t := now
			recipientState.AcceptedAt = &t
		}
	case models.DmRequestStateDeclined:
		recipientState.Folder = models.UserConversationFolderRequests
		recipientState.RequestState = models.DmRequestStatePending
		recipientState.AcceptedAt = nil
		recipientState.DeclinedAt = nil
		t := now
		recipientState.RequestedAt = &t
	default:
		if deliversToInbox {
			recipientState.Folder = models.UserConversationFolderInbox
			recipientState.RequestState = models.DmRequestStateAccepted
			recipientState.RequestedAt = nil
			recipientState.DeclinedAt = nil
			t := now
			recipientState.AcceptedAt = &t
		} else {
			recipientState.Folder = models.UserConversationFolderRequests
			recipientState.RequestState = models.DmRequestStatePending
			recipientState.AcceptedAt = nil
			recipientState.DeclinedAt = nil
			if recipientState.RequestedAt == nil {
				t := now
				recipientState.RequestedAt = &t
			}
		}
	}

	if status != nil {
		applyDirectMessageStatusPreview(recipientState, status.StatusID, now)
	}

	return []*models.UserConversationState{senderState, recipientState}
}

func buildExpectedDirectMessageParticipantStates(
	conversation *models.Conversation,
	senderID string,
	recipientID string,
	senderState *models.UserConversationState,
	recipientState *models.UserConversationState,
) []*models.UserConversationState {
	if isRemoteConversationParticipant(conversation, recipientID) {
		return []*models.UserConversationState{
			userConversationStateForSend(conversation, senderID, recipientID, senderState),
		}
	}
	return []*models.UserConversationState{
		userConversationStateForSend(conversation, senderID, recipientID, senderState),
		userConversationStateForSend(conversation, recipientID, senderID, recipientState),
	}
}

func (s *Service) prepareTransactionalDirectMessageStatusWrite(status *models.Status) (interfaces.DirectMessageStatusStageFn, error) {
	if s.noteRepo == nil {
		return nil, nil
	}

	capability, ok := s.conversationRepo.(directMessageSendCapability)
	if !ok || !capability.TransactionalDirectMessageSendEnabled() {
		return nil, nil
	}

	contract, ok := s.noteRepo.(interfaces.CanonicalStatusCreateRepository)
	if !ok {
		return nil, errors.Join(ErrCreateDirectMessage, errDirectMessageStatusContractRequired)
	}

	if err := contract.PrepareStatusCreate(status); err != nil {
		requestErr := errors.Join(ErrCreateDirectMessage, err)
		s.logDirectMessageFailure("failed to prepare direct message status for persistence", "prepare_status", status, err, requestErr)
		return nil, requestErr
	}

	return contract.StageStatusCreate, nil
}

func (s *Service) finalizeDirectMessageStatusWrite(ctx context.Context, status *models.Status) error {
	if s.noteRepo == nil {
		return nil
	}

	capability, ok := s.conversationRepo.(directMessageSendCapability)
	if !ok || !capability.TransactionalDirectMessageSendEnabled() {
		if err := s.noteRepo.CreateStatus(ctx, status); err != nil {
			requestErr := errors.Join(ErrCreateDirectMessage, err)
			s.logDirectMessageFailure("failed to persist direct message status", "create_status", status, err, requestErr)
			return requestErr
		}
		return nil
	}

	contract, ok := s.noteRepo.(interfaces.CanonicalStatusCreateRepository)
	if !ok {
		return errors.Join(ErrCreateDirectMessage, errDirectMessageStatusContractRequired)
	}

	if err := contract.FinalizeCreatedStatus(ctx, status); err != nil {
		requestErr := errors.Join(ErrCreateDirectMessage, err)
		s.logDirectMessageFailure("failed to finalize direct message status persistence", "finalize_status", status, err, requestErr)
		return requestErr
	}

	return nil
}

func (s *Service) applyDirectMessageSendTransition(
	ctx context.Context,
	conversation *models.Conversation,
	createConversation bool,
	senderID string,
	recipientID string,
	senderState *models.UserConversationState,
	recipientState *models.UserConversationState,
	status *models.Status,
	deliversToInbox bool,
) error {
	stageStatusCreate, err := s.prepareTransactionalDirectMessageStatusWrite(status)
	if err != nil {
		return err
	}

	transition := &models.DirectMessageSendTransition{
		Conversation:       conversation,
		Status:             status,
		ParticipantStates:  buildDirectMessageParticipantStatesForSend(conversation, status, senderID, recipientID, senderState, recipientState, deliversToInbox),
		CreateConversation: createConversation,
	}
	if !createConversation {
		transition.ExpectedParticipantStates = buildExpectedDirectMessageParticipantStates(conversation, senderID, recipientID, senderState, recipientState)
	}

	if err := s.conversationRepo.ApplyDirectMessageSend(ctx, transition, stageStatusCreate); err != nil {
		requestErr := errors.Join(ErrCreateDirectMessage, err)
		s.logDirectMessageFailure("failed to apply direct message send transition", "apply_transition", status, err, requestErr)
		return requestErr
	}
	if err := s.finalizeDirectMessageStatusWrite(ctx, status); err != nil {
		return err
	}

	if conversation != nil {
		conversation.LastStatusID = status.StatusID
		conversation.LastMessageTime = status.PublishedAt.UTC()
		if conversation.LastMessageTime.IsZero() {
			conversation.LastMessageTime = time.Now().UTC()
		}
		conversation.TotalMessageCount++
		conversation.UpdatedAt = conversation.LastMessageTime
		conversation.Unread = false
	}

	return nil
}

func (s *Service) executeDirectMessageSendAttempt(
	ctx context.Context,
	cmd *SendDirectMessageCommand,
	sender *storage.Account,
	recipientAccounts map[string]*storage.Account,
	recipientID string,
	requestRateLimitConsumed *bool,
) (*directMessageSendAttempt, bool, error) {
	conversation, createConversation, err := s.resolveDirectMessageConversationForCommand(ctx, cmd, recipientID)
	if err != nil {
		return nil, false, err
	}

	var recipientRequestState models.DmRequestState
	var recipientState *models.UserConversationState
	recipientIsRemote := isRemoteConversationParticipant(conversation, recipientID)
	if !createConversation && !recipientIsRemote {
		recipientState, err = s.getUserConversationStateForSend(ctx, conversation.ID, recipientID)
		if err != nil {
			return nil, false, errors.Join(ErrCreateDirectMessage, err)
		}
		if recipientState != nil {
			recipientRequestState = recipientState.RequestState
		}
	}

	willBeRequest, deliversToInbox, err := s.evaluateDirectMessageRequestPolicyForState(ctx, cmd, conversation.ID, recipientID, recipientRequestState)
	if err != nil {
		return nil, false, err
	}
	if willBeRequest && (requestRateLimitConsumed == nil || !*requestRateLimitConsumed) {
		if err := s.enforceDirectMessageRequestRateLimit(ctx, cmd, conversation.ID, recipientID); err != nil {
			return nil, false, err
		}
		if requestRateLimitConsumed != nil {
			*requestRateLimitConsumed = true
		}
	}

	status, messageID, err := s.createDirectMessageStatus(ctx, cmd, sender, recipientAccounts, conversation.ID, recipientID)
	if err != nil {
		return nil, false, err
	}

	var senderState *models.UserConversationState
	if !createConversation {
		senderState, err = s.getUserConversationStateForSend(ctx, conversation.ID, cmd.SenderID)
		if err != nil {
			return nil, false, errors.Join(ErrCreateDirectMessage, err)
		}
	}

	if err := s.applyDirectMessageSendTransition(ctx, conversation, createConversation, cmd.SenderID, recipientID, senderState, recipientState, status, deliversToInbox); err != nil {
		if createConversation && errors.Is(err, storage.ErrAlreadyExists) {
			return nil, true, storage.ErrAlreadyExists
		}
		if !createConversation && errors.Is(err, storage.ErrVersionConflict) {
			return nil, true, storage.ErrVersionConflict
		}
		return nil, false, err
	}

	return &directMessageSendAttempt{
		conversation:  conversation,
		status:        status,
		messageID:     messageID,
		willBeRequest: willBeRequest,
	}, false, nil
}

// SendDirectMessage creates or updates a conversation, creates a direct message, and emits events
func (s *Service) SendDirectMessage(ctx context.Context, cmd *SendDirectMessageCommand) (*MessageResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}
	if err := s.ensureDirectMessageWritesAllowed(ctx); err != nil {
		return nil, err
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

	if err := s.enforceDirectMessageTotalRateLimit(ctx, cmd, recipientID); err != nil {
		return nil, err
	}

	sender, recipient, recipientAccounts, err := s.getDirectMessageAccounts(ctx, cmd, recipientID)
	if err != nil {
		return nil, err
	}

	cmd = cloneDirectMessageCommandWithResolvedParticipants(cmd, sender, recipient)
	recipientID = cmd.Recipients[0]

	var attemptResult *directMessageSendAttempt
	var retryErr error
	requestRateLimitConsumed := false
	for attempt := 0; attempt < directMessageSendRetryLimit; attempt++ {
		var retry bool
		attemptResult, retry, err = s.executeDirectMessageSendAttempt(ctx, cmd, sender, recipientAccounts, recipientID, &requestRateLimitConsumed)
		if err != nil {
			if retry {
				retryErr = err
				continue
			}
			return nil, err
		}
		break
	}
	if attemptResult == nil || attemptResult.conversation == nil || attemptResult.status == nil {
		if retryErr != nil {
			return nil, errors.Join(ErrCreateDirectMessage, retryErr)
		}
		return nil, errors.Join(ErrCreateDirectMessage, storage.ErrAlreadyExists)
	}

	conversation := attemptResult.conversation
	status := attemptResult.status
	messageID := attemptResult.messageID
	willBeRequest := attemptResult.willBeRequest

	s.logger.Info("sent direct message successfully",
		zap.String("message_id", messageID),
		zap.String("conversation_id", conversation.ID))

	// Emit events and queue federation
	events := s.emitMessageSentEvents(ctx, status, conversation)
	s.queueFederationDelivery(ctx, status)

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

func (s *Service) getSendMessageAccountsForSend(ctx context.Context, sendCmd *SendDirectMessageCommand, conversation *models.Conversation, senderID, recipientID string) (*storage.Account, *storage.Account, error) {
	sender, err := s.accountRepo.GetAccount(ctx, senderID)
	if err != nil {
		return nil, nil, errors.Join(ErrGetSenderAccount, err)
	}

	if ref := conversationParticipantRefByID(conversation, recipientID); ref != nil && ref.ParticipantType == models.ConversationParticipantTypeRemoteActor {
		actor := sendCmd.ResolvedRecipientActor
		if actor == nil {
			actor = &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   ref.ParticipantID,
					Type: activitypub.PersonType,
				},
				PreferredUsername: remoteUsernameFromParticipantRef(ref),
			}
		}
		sendCmd.ResolvedRecipientActor = actor
		sendCmd.ResolvedRecipientRef = ref
		return sender, &storage.Account{
			User: &storage.User{
				ID:          ref.ParticipantID,
				Username:    ref.ParticipantID,
				DisplayName: remoteUsernameFromParticipantRef(ref),
			},
			Actor: actor,
		}, nil
	}

	recipient, err := s.accountRepo.GetAccount(ctx, recipientID)
	if err != nil {
		return nil, nil, errors.Join(ErrInvalidRecipient, err)
	}

	return sender, recipient, nil
}

func remoteUsernameFromParticipantRef(ref *models.ConversationParticipantRef) string {
	if ref == nil {
		return ""
	}
	if ref.Acct != "" {
		if username, _ := directMessageMentionHandleParts(ref.Acct); username != "" {
			return username
		}
	}
	return extractUsernameFromActorIdentifier(ref.ParticipantID)
}

func extractUsernameFromActorIdentifier(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return ""
	}
	if username, _ := directMessageMentionHandleParts(identifier); username != "" && !strings.Contains(identifier, "://") {
		return username
	}
	if parsed, err := url.Parse(identifier); err == nil && parsed.Scheme != "" {
		path := strings.Trim(parsed.Path, "/")
		if path == "" {
			return ""
		}
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if (part == "users" || part == "actors" || part == "profiles") && i+1 < len(parts) {
				return strings.TrimPrefix(parts[i+1], "@")
			}
		}
		return strings.TrimPrefix(parts[len(parts)-1], "@")
	}
	return strings.TrimPrefix(identifier, "@")
}

func (s *Service) createSendMessageStatus(_ context.Context, cmd *SendMessageCommand, sendCmd *SendDirectMessageCommand, sender *storage.Account, recipient *storage.Account, conversationID, recipientID string) (*models.Status, string, error) {
	messageID := uuid.New().String()
	now := time.Now().UTC()
	noteCmd := sanitizedDirectMessageCommand(sendCmd)

	status := &models.Status{
		StatusID:       messageID,
		AuthorID:       cmd.SenderID,
		AuthorUsername: sender.User.Username,
		Content:        noteCmd.Content,
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

	var err error
	status.Note, err = s.buildActivityPubNote(noteCmd, messageID, sender, conversationID, map[string]*storage.Account{
		recipientID: recipient,
	})
	if err != nil {
		return nil, "", err
	}
	status.ToRecipients = append([]string(nil), status.Note.To...)
	status.SyncTagFieldsFromNote()

	return status, messageID, nil
}

func (s *Service) executeSendMessageAttempt(
	ctx context.Context,
	cmd *SendMessageCommand,
	totalRateLimitConsumed *bool,
	requestRateLimitConsumed *bool,
) (*MessageResult, bool, error) {
	conversation, recipientID, err := s.loadConversationAndRecipientForSendMessage(ctx, cmd)
	if err != nil {
		return nil, false, err
	}

	sendCmd := sendDirectMessageCommandFromSendMessage(cmd, recipientID)
	if err := s.validateSendMessageCommandBasic(ctx, sendCmd); err != nil {
		return nil, false, errors.Join(ErrConversationValidationFailed, err)
	}

	if err := s.validateSendMessageReplyTarget(ctx, cmd.InReplyToID, conversation.ID); err != nil {
		return nil, false, err
	}

	sender, recipient, err := s.getSendMessageAccountsForSend(ctx, sendCmd, conversation, cmd.SenderID, recipientID)
	if err != nil {
		return nil, false, err
	}

	if totalRateLimitConsumed == nil || !*totalRateLimitConsumed {
		if err := s.enforceDirectMessageTotalRateLimit(ctx, sendCmd, recipientID); err != nil {
			return nil, false, err
		}
		if totalRateLimitConsumed != nil {
			*totalRateLimitConsumed = true
		}
	}

	recipientRequestState := models.DmRequestState("")
	var recipientState *models.UserConversationState
	if !isRemoteConversationParticipant(conversation, recipientID) {
		recipientState, err = s.getUserConversationStateForSend(ctx, conversation.ID, recipientID)
		if err != nil {
			return nil, false, errors.Join(ErrCreateDirectMessage, err)
		}
		if recipientState != nil {
			recipientRequestState = recipientState.RequestState
		}
	}

	willBeRequest, deliversToInbox, err := s.evaluateDirectMessageRequestPolicyForState(ctx, sendCmd, conversation.ID, recipientID, recipientRequestState)
	if err != nil {
		return nil, false, err
	}
	if willBeRequest && (requestRateLimitConsumed == nil || !*requestRateLimitConsumed) {
		if err := s.enforceDirectMessageRequestRateLimit(ctx, sendCmd, conversation.ID, recipientID); err != nil {
			return nil, false, err
		}
		if requestRateLimitConsumed != nil {
			*requestRateLimitConsumed = true
		}
	}

	status, _, err := s.createSendMessageStatus(ctx, cmd, sendCmd, sender, recipient, conversation.ID, recipientID)
	if err != nil {
		return nil, false, err
	}
	senderState, err := s.getUserConversationStateForSend(ctx, conversation.ID, cmd.SenderID)
	if err != nil {
		return nil, false, errors.Join(ErrCreateDirectMessage, err)
	}

	if err := s.applyDirectMessageSendTransition(ctx, conversation, false, cmd.SenderID, recipientID, senderState, recipientState, status, deliversToInbox); err != nil {
		if errors.Is(err, storage.ErrVersionConflict) {
			return nil, true, storage.ErrVersionConflict
		}
		return nil, false, err
	}

	events := s.emitMessageSentEvents(ctx, status, conversation)
	s.queueFederationDelivery(ctx, status)

	return &MessageResult{
		Message:      status,
		Conversation: conversation,
		Events:       events,
	}, false, nil
}

// SendMessage sends a message in an existing 1:1 conversation, enforcing strict participant authz.
func (s *Service) SendMessage(ctx context.Context, cmd *SendMessageCommand) (*MessageResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}
	if err := s.ensureDirectMessageWritesAllowed(ctx); err != nil {
		return nil, err
	}

	var retryErr error
	totalRateLimitConsumed := false
	requestRateLimitConsumed := false
	for attempt := 0; attempt < directMessageSendRetryLimit; attempt++ {
		result, retry, err := s.executeSendMessageAttempt(ctx, cmd, &totalRateLimitConsumed, &requestRateLimitConsumed)
		if err != nil {
			if retry {
				retryErr = err
				continue
			}
			return nil, err
		}
		return result, nil
	}

	if retryErr != nil {
		return nil, errors.Join(ErrCreateDirectMessage, retryErr)
	}
	return nil, errors.Join(ErrCreateDirectMessage, storage.ErrVersionConflict)
}

// AcceptMessageRequest moves a pending request thread into the user's inbox.
func (s *Service) AcceptMessageRequest(ctx context.Context, cmd *AcceptMessageRequestCommand) (*ConversationResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}
	if err := s.ensureDirectMessageWritesAllowed(ctx); err != nil {
		return nil, err
	}

	return s.applyMessageRequestDecision(ctx, cmd.ConversationID, cmd.UserID, models.DmRequestStateAccepted, "dm.request.accept")
}

// DeclineMessageRequest hides a request thread from the recipient by setting requestState=DECLINED.
func (s *Service) DeclineMessageRequest(ctx context.Context, cmd *DeclineMessageRequestCommand) (*ConversationResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}
	if err := s.ensureDirectMessageWritesAllowed(ctx); err != nil {
		return nil, err
	}

	return s.applyMessageRequestDecision(ctx, cmd.ConversationID, cmd.UserID, models.DmRequestStateDeclined, "dm.request.decline")
}

func (s *Service) applyMessageRequestDecision(ctx context.Context, conversationID, userID string, decision models.DmRequestState, auditEvent string) (*ConversationResult, error) {
	conversation, err := s.conversationRepo.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, errors.Join(ErrGetConversation, err)
	}

	if !s.isParticipant(userID, conversation.Participants) {
		return nil, ErrNotConversationParticipant
	}

	now := time.Now().UTC()
	if err := s.updateUserConversationState(ctx, conversation.ID, userID, func(participantState *models.UserConversationState) {
		if participantState == nil {
			return
		}

		switch decision {
		case models.DmRequestStateAccepted:
			participantState.Folder = models.UserConversationFolderInbox
			participantState.RequestState = models.DmRequestStateAccepted
			participantState.DeletedAt = nil
			participantState.RequestedAt = nil
			participantState.DeclinedAt = nil
			t := now
			participantState.AcceptedAt = &t
		case models.DmRequestStateDeclined:
			participantState.Folder = models.UserConversationFolderDeclined
			participantState.RequestState = models.DmRequestStateDeclined
			participantState.DeletedAt = nil
			participantState.RequestedAt = nil
			participantState.AcceptedAt = nil
			t := now
			participantState.DeclinedAt = &t
		}
	}); err != nil {
		return nil, errors.Join(ErrMarkConversationRead, err)
	}

	if state, err := s.conversationRepo.GetUserConversationState(ctx, userID, conversation.ID); err == nil && state != nil {
		conversation.Unread = state.Unread
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
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}
	if err := s.ensureDirectMessageWritesAllowed(ctx); err != nil {
		return nil, err
	}

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

	conversation.Unread = false

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
		folder := models.UserConversationFolderInbox
		if query.Folder == ConversationFolderRequests {
			folder = models.UserConversationFolderRequests
		}
		result, err = s.conversationRepo.GetUserConversationsByFolder(ctx, query.UserID, folder, query.Pagination)
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
	if state, err := s.conversationRepo.GetUserConversationState(ctx, query.ViewerID, query.ConversationID); err == nil && state != nil {
		if (state.DeletedAt != nil && !state.DeletedAt.IsZero()) ||
			state.Folder == models.UserConversationFolderHidden {
			return nil, ErrConversationNotFound
		}

		conversation.Unread = state.Unread
		conversation.ViewerState = userConversationStateFromContract(conversation, query.ViewerID, "", state)
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

func sanitizedDirectMessageCommand(cmd *SendDirectMessageCommand) *SendDirectMessageCommand {
	if cmd == nil {
		return nil
	}

	sanitized := *cmd
	sanitized.Content = strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(cmd.Content))
	sanitized.SpoilerText = strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(cmd.SpoilerText))
	sanitized.Recipients = append([]string(nil), cmd.Recipients...)
	sanitized.MediaIDs = append([]string(nil), cmd.MediaIDs...)
	return &sanitized
}

func (s *Service) buildActivityPubNote(cmd *SendDirectMessageCommand, messageID string, sender *storage.Account, conversationID string, recipientAccounts map[string]*storage.Account) (*activitypub.Note, error) {
	now := time.Now().UTC()
	recipients, mentionTags, err := s.buildDirectMessageRecipientAudience(cmd.Recipients, recipientAccounts)
	if err != nil {
		return nil, err
	}

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      "Note",
			ID:        fmt.Sprintf("https://%s/users/%s/statuses/%s", s.domainName, sender.User.Username, messageID),
			Published: &now,
			To:        recipients,
			Sensitive: cmd.Sensitive,
			Summary:   cmd.SpoilerText,
		},
		Content:          cmd.Content,
		AttributedTo:     fmt.Sprintf("https://%s/users/%s", s.domainName, sender.User.Username),
		Tag:              mentionTags,
		Visibility:       VisibilityDirect,
		ConversationID:   conversationID,
		AgentAttribution: cmd.AgentAttribution,
	}

	// Set in reply to
	if cmd.InReplyToID != "" {
		note.InReplyTo = fmt.Sprintf("https://%s/statuses/%s", s.domainName, cmd.InReplyToID)
	}

	return note, nil
}

func (s *Service) buildDirectMessageRecipientAudience(recipientIDs []string, recipientAccounts map[string]*storage.Account) ([]string, []activitypub.Tag, error) {
	localDomain := strings.TrimSpace(s.domainName)
	recipients := make([]string, 0, len(recipientIDs))
	mentionTags := make([]activitypub.Tag, 0, len(recipientIDs))
	seenActors := make(map[string]struct{}, len(recipientIDs))

	for _, recipientID := range recipientIDs {
		recipient := recipientAccounts[recipientID]
		actorID, err := directMessageRecipientActorID(recipientID, recipient, localDomain)
		if err != nil {
			return nil, nil, err
		}

		actorKey := strings.ToLower(strings.TrimSpace(actorID))
		if actorKey == "" {
			continue
		}
		if _, exists := seenActors[actorKey]; exists {
			continue
		}
		seenActors[actorKey] = struct{}{}

		username, domain := normalizeDirectMessageMentionAccount(recipientID, recipient, localDomain)
		recipients = append(recipients, actorID)
		mentionTags = append(mentionTags, activitypub.Tag{
			Type: "Mention",
			Href: actorID,
			Name: formatDirectMessageMentionTagName(username, domain, localDomain),
		})
	}

	return recipients, mentionTags, nil
}

func directMessageRecipientActorID(recipientID string, account *storage.Account, localDomain string) (string, error) {
	if account != nil && account.Actor != nil {
		actorID := strings.TrimSpace(account.Actor.ID)
		if actorID != "" {
			return actorID, nil
		}
	}

	trimmedRecipientID := strings.TrimSpace(recipientID)
	if strings.HasPrefix(trimmedRecipientID, "http://") || strings.HasPrefix(trimmedRecipientID, "https://") {
		return trimmedRecipientID, nil
	}

	username, domain := normalizeDirectMessageMentionAccount(trimmedRecipientID, account, localDomain)
	normalizedLocalDomain := normalizeDirectMessageMentionDomain(localDomain)
	if domain != "" && domain != normalizedLocalDomain {
		return "", errors.Join(ErrInvalidRecipient, errDirectMessageRemoteRecipientActorRequired)
	}
	if username == "" || normalizedLocalDomain == "" {
		return "", ErrInvalidRecipient
	}

	return fmt.Sprintf("https://%s/users/%s", localDomain, username), nil
}

func normalizeDirectMessageMentionAccount(recipientID string, account *storage.Account, localDomain string) (string, string) {
	resolvedUsername, resolvedDomain := directMessageMentionHandleParts(recipientID)
	normalizedLocalDomain := normalizeDirectMessageMentionDomain(localDomain)

	if account != nil && account.User != nil {
		storedUsername := strings.TrimSpace(account.User.Username)
		if storedUsername != "" {
			if parsedUsername, parsedDomain := directMessageMentionHandleParts(storedUsername); parsedDomain != "" {
				resolvedUsername = parsedUsername
				resolvedDomain = parsedDomain
			} else {
				resolvedUsername = storedUsername
			}
		}
	}

	if account != nil && account.Actor != nil {
		if preferred := strings.TrimSpace(account.Actor.PreferredUsername); preferred != "" {
			resolvedUsername = preferred
		}
		if actorDomain := directMessageMentionActorDomain(account.Actor.ID); actorDomain != "" && actorDomain != normalizedLocalDomain {
			resolvedDomain = actorDomain
		}
	}

	if normalizeDirectMessageMentionDomain(resolvedDomain) == normalizedLocalDomain {
		resolvedDomain = ""
	}

	return strings.TrimSpace(resolvedUsername), strings.TrimSpace(resolvedDomain)
}

func directMessageMentionHandleParts(recipientID string) (string, string) {
	trimmed := strings.TrimSpace(recipientID)
	if trimmed == "" {
		return "", ""
	}

	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", ""
		}

		path := strings.Trim(parsed.Path, "/")
		if path == "" {
			return "", directMessageMentionActorDomain(trimmed)
		}

		segments := strings.Split(path, "/")
		username := strings.TrimPrefix(segments[len(segments)-1], "@")
		return strings.TrimSpace(username), directMessageMentionActorDomain(trimmed)
	}

	username, domain, found := strings.Cut(trimmed, "@")
	if !found {
		return strings.TrimSpace(username), ""
	}

	return strings.TrimSpace(username), strings.TrimSpace(domain)
}

func directMessageMentionActorDomain(actorID string) string {
	parsed, err := url.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return ""
	}

	return normalizeDirectMessageMentionDomain(parsed.Host)
}

func normalizeDirectMessageMentionDomain(domain string) string {
	normalized := strings.ToLower(strings.TrimSpace(domain))
	normalized = strings.TrimPrefix(normalized, "https://")
	normalized = strings.TrimPrefix(normalized, "http://")
	normalized = strings.TrimSuffix(normalized, "/")
	return normalized
}

func formatDirectMessageMentionTagName(username, domain, localDomain string) string {
	username = strings.TrimSpace(username)
	domain = strings.TrimSpace(domain)
	if username == "" {
		return ""
	}
	if domain == "" || strings.EqualFold(domain, strings.TrimSpace(localDomain)) {
		return "@" + username
	}
	return "@" + username + "@" + domain
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
	for _, participantID := range models.ConversationLocalParticipantIDs(conversation) {
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
		if isRemoteDirectMessageActorID(recipientURL, s.domainName) {
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

func isRemoteDirectMessageActorID(actorID, localDomain string) bool {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return false
	}

	actorDomain := directMessageMentionActorDomain(actorID)
	if actorDomain == "" {
		return false
	}
	return actorDomain != normalizeDirectMessageMentionDomain(localDomain)
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

func (s *Service) updateUserConversationState(ctx context.Context, conversationID, participantID string, mutator func(state *models.UserConversationState)) error {
	if mutator == nil {
		return nil
	}

	stateContract, err := s.conversationRepo.GetUserConversationState(ctx, participantID, conversationID)
	if err != nil {
		return err
	}
	if stateContract == nil {
		return storage.ErrNotFound
	}

	state := userConversationStateFromContract(nil, participantID, "", stateContract)
	mutator(state)
	return s.conversationRepo.PutUserConversationState(ctx, state)
}

// DeleteConversation implements delete-for-me semantics for a DM conversation.
// It marks the viewer's participant record DeletedAt without deleting shared conversation data.
func (s *Service) DeleteConversation(ctx context.Context, cmd *DeleteConversationCommand) (*ConversationResult, error) {
	if cmd == nil {
		return nil, errors.Join(ErrConversationValidationFailed, storage.ErrInvalidInput)
	}
	if err := s.ensureDirectMessageWritesAllowed(ctx); err != nil {
		return nil, err
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
	if err := s.updateUserConversationState(ctx, conversationID, participantIDForRecord, func(state *models.UserConversationState) {
		if state == nil {
			return
		}
		t := now
		state.Folder = models.UserConversationFolderHidden
		state.DeletedAt = &t
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
	if err := s.ensureDirectMessageWritesAllowed(ctx); err != nil {
		return false, err
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
