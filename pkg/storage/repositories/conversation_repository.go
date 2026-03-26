package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// ConversationRepository handles conversation-related database operations using enhanced patterns
type ConversationRepository struct {
	*EnhancedBaseRepository[*models.Conversation]
	logger          *zap.Logger
	transactWriteFn func(ctx context.Context, fn func(core.TransactionBuilder) error) error
}

const conversationParticipantLookupSK = "LOOKUP"

type conversationTransactionalDB interface {
	core.DB
	TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error
}

func conversationParticipantLookupPK(participants []string) string {
	return fmt.Sprintf("CONVERSATION_PARTICIPANTS#%s", strings.Join(models.CanonicalConversationParticipants(participants), ","))
}

func newConversationParticipantLookup(conversationID string, participants []string) *models.ConversationParticipantKey {
	participantKey := conversationParticipantLookupPK(participants)
	return &models.ConversationParticipantKey{
		PK:             participantKey,
		SK:             conversationParticipantLookupSK,
		GSI1PK:         participantKey,
		ConversationID: conversationID,
	}
}

func canonicalConversationStatusSK(participantID string) string {
	return fmt.Sprintf("USER#%s", models.CanonicalConversationParticipantID(participantID))
}

func newConversationTransactWriteFn(db core.DB) func(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	txDB, ok := db.(conversationTransactionalDB)
	if !ok {
		return nil
	}
	return txDB.TransactWrite
}

// NewConversationRepository creates a new conversation repository with enhanced functionality and cost tracking
func NewConversationRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *ConversationRepository {
	// Create enhanced repository optimized for conversation operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Conversation](db, tableName, logger, costService, "ConversationRepository", "conversation")

	// Set up enhanced services for conversation operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Conversations cached for message threading
	enhancedRepo.SetEventService(NewDefaultEventService())      // Important for conversation events

	return &ConversationRepository{
		EnhancedBaseRepository: enhancedRepo,
		logger:                 logger,
		transactWriteFn:        newConversationTransactWriteFn(db),
	}
}

// CreateConversation creates a new conversation with participants (KEEP - Complex conversation business logic)
func (r *ConversationRepository) CreateConversation(ctx context.Context, conversation *models.Conversation, participants []string) error {
	return r.createConversation(ctx, conversation, participants, nil)
}

func (r *ConversationRepository) createConversation(ctx context.Context, conversation *models.Conversation, participants []string, participantStates []*models.UserConversationState) error {
	if conversation == nil {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityConversation, "nil")
	}

	log := r.logger.With(zap.String("conversation_id", conversation.ID))

	// Generate ID if not provided (matching legacy behavior)
	if err := common.ValidateRequiredParam("conversation_id", conversation.ID); err != nil {
		conversation.ID = r.generateRandomString(12)
	}

	// Set timestamps if not provided
	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = time.Now()
	}
	if conversation.UpdatedAt.IsZero() {
		conversation.UpdatedAt = time.Now()
	}

	// Set participants if provided
	if err := common.ValidateSliceNotEmpty("participants", participants); err == nil {
		conversation.Participants = models.CanonicalConversationParticipants(participants)
	} else {
		conversation.Participants = models.CanonicalConversationParticipants(conversation.Participants)
	}

	if err := conversation.BeforeCreate(); err != nil {
		log.Error("failed to prepare conversation", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversation.ID)
	}

	preparedStates, err := normalizeConversationParticipantStates(conversation, participantStates)
	if err != nil {
		log.Error("failed to prepare participant states", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversation.ID)
	}

	if r.transactWriteFn == nil {
		return r.createConversationLegacy(ctx, log, conversation, preparedStates)
	}

	lookupKey := newConversationParticipantLookup(conversation.ID, conversation.Participants)
	if err := r.transactWriteFn(ctx, func(tx core.TransactionBuilder) error {
		tx = tx.WithContext(ctx)
		tx.Create(conversation)
		for _, state := range preparedStates {
			tx.Create(state)
		}
		tx.Create(lookupKey)
		return nil
	}); err != nil {
		log.Error("failed to create conversation transactionally", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversation.ID)
	}

	log.Debug("conversation created successfully")
	return nil
}

func (r *ConversationRepository) createConversationLegacy(ctx context.Context, log *zap.Logger, conversation *models.Conversation, participantStates []*models.UserConversationState) error {
	if err := r.ValidateAndCreate(ctx, conversation); err != nil {
		log.Error("failed to create conversation", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversation.ID)
	}

	for _, state := range participantStates {
		if err := r.createOrUpdateUserConversationState(ctx, state); err != nil {
			log.Error("failed to initialize canonical user conversation state",
				zap.String("conversation_id", conversation.ID),
				zap.Error(err))
			return ErrorHandler.HandleCreateError(err, EntityConversation, conversation.ID)
		}
	}

	lookupKey := newConversationParticipantLookup(conversation.ID, conversation.Participants)
	if err := r.GetDB().Model(lookupKey).WithContext(ctx).IfNotExists().Create(); err != nil {
		if errors.IsConditionFailed(err) {
			log.Info("participant lookup already exists; cleaning up duplicate conversation create",
				zap.String("participant_key", lookupKey.PK),
				zap.String("conversation_id", conversation.ID))
			if cleanupErr := r.DeleteConversation(ctx, conversation.ID); cleanupErr != nil {
				return ErrorHandler.HandleCreateError(fmt.Errorf("lookup collision cleanup failed: %w", cleanupErr), EntityConversation, conversation.ID)
			}
			return storage.ErrAlreadyExists
		}
		log.Warn("failed to create participant lookup key", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, lookupKey.PK)
	}

	return nil
}

func normalizeConversationParticipantStates(conversation *models.Conversation, participantStates []*models.UserConversationState) ([]*models.UserConversationState, error) {
	if conversation == nil {
		return nil, storage.ErrInvalidInput
	}

	canonicalParticipants := models.CanonicalConversationParticipants(conversation.Participants)
	if len(canonicalParticipants) == 0 {
		return nil, storage.ErrInvalidInput
	}

	if len(participantStates) == 0 {
		states := make([]*models.UserConversationState, 0, len(canonicalParticipants))
		for _, participantID := range canonicalParticipants {
			state := defaultUserConversationState(conversation, participantID)
			if err := state.BeforeCreate(); err != nil {
				return nil, err
			}
			states = append(states, state)
		}
		return states, nil
	}

	stateByViewer := make(map[string]*models.UserConversationState, len(participantStates))
	for _, candidate := range participantStates {
		if candidate == nil {
			return nil, storage.ErrInvalidInput
		}

		state := *candidate
		state.ViewerID = models.CanonicalConversationParticipantID(state.ViewerID)
		if state.ViewerID == "" {
			return nil, storage.ErrInvalidInput
		}
		if _, exists := stateByViewer[state.ViewerID]; exists {
			return nil, storage.ErrInvalidInput
		}
		if state.ConversationID == "" {
			state.ConversationID = conversation.ID
		}
		if state.ConversationID != conversation.ID {
			return nil, storage.ErrInvalidInput
		}
		if state.CounterpartID == "" {
			state.CounterpartID = counterpartForConversation(state.ViewerID, canonicalParticipants)
		}
		if state.CreatedAt.IsZero() {
			state.CreatedAt = conversation.CreatedAt.UTC()
		}
		if state.UpdatedAt.IsZero() {
			state.UpdatedAt = conversation.UpdatedAt.UTC()
		}
		if state.PreviewStatusID == "" {
			state.PreviewStatusID = conversation.LastStatusID
		}
		if state.PreviewStatusPublishedAt.IsZero() && !conversation.LastMessageTime.IsZero() {
			state.PreviewStatusPublishedAt = conversation.LastMessageTime.UTC()
		}
		if state.SortAt.IsZero() {
			switch {
			case !state.PreviewStatusPublishedAt.IsZero():
				state.SortAt = state.PreviewStatusPublishedAt.UTC()
			case !conversation.LastMessageTime.IsZero():
				state.SortAt = conversation.LastMessageTime.UTC()
			default:
				state.SortAt = conversation.UpdatedAt.UTC()
			}
		}
		if err := state.BeforeCreate(); err != nil {
			return nil, err
		}
		stateByViewer[state.ViewerID] = &state
	}

	states := make([]*models.UserConversationState, 0, len(canonicalParticipants))
	for _, participantID := range canonicalParticipants {
		state := stateByViewer[participantID]
		if state == nil {
			return nil, storage.ErrInvalidInput
		}
		states = append(states, state)
	}
	return states, nil
}

// GetConversation retrieves a conversation by ID (REPLACE with BaseRepository)
func (r *ConversationRepository) GetConversation(ctx context.Context, id string) (*models.Conversation, error) {
	log := r.logger.With(zap.String("conversation_id", id))

	var conv models.Conversation
	err := r.Get(ctx, fmt.Sprintf("CONVERSATION#%s", id), "METADATA", &conv)

	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrorHandler.HandleGetError(err, EntityConversation, id)
		}
		log.Error("failed to get conversation", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityConversation, id)
	}

	return &conv, nil
}

// UpdateConversation updates a conversation (KEEP - Complex conversation update logic)
func (r *ConversationRepository) UpdateConversation(ctx context.Context, conversation *models.Conversation) error {
	// For updating, we recreate all records (matching legacy behavior)
	// This ensures participant records are updated with new timestamps
	return r.CreateConversation(ctx, conversation, conversation.Participants)
}

// DeleteConversation deletes a conversation by ID (KEEP - Complex cleanup logic)
func (r *ConversationRepository) DeleteConversation(ctx context.Context, id string) error {
	log := r.logger.With(zap.String("conversation_id", id))

	// Get the conversation first to get participant list
	conv, err := r.GetConversation(ctx, id)
	if err != nil {
		log.Warn("failed to get conversation for cleanup", zap.Error(err))
		// Continue with deletion even if we can't get the conversation
	}

	// Delete main record using BaseRepository
	err = r.Delete(ctx, fmt.Sprintf("CONVERSATION#%s", id), "METADATA")
	if err != nil {
		log.Error("failed to delete conversation", zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityConversation, id)
	}

	// Delete canonical per-user DM state rows.
	if conv != nil {
		for _, participantID := range conv.Participants {
			err = r.GetDB().Model(&models.UserConversationState{}).WithContext(ctx).
				Where("PK", "=", userConversationStatePK(participantID)).
				Where("SK", "=", userConversationStateSK(id)).
				Delete()
			if err != nil && !errors.IsNotFound(err) {
				log.Warn("failed to delete user conversation state",
					zap.String("participant_id", participantID),
					zap.Error(err))
			}
		}
	}

	// Delete all status records for this conversation (KEEP - Conversation cleanup logic)
	var statuses []models.ConversationStatus
	err = r.GetDB().Model(&models.ConversationStatus{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION_STATUS#%s", id)).
		Scan(&statuses)

	if err == nil {
		for _, status := range statuses {
			err := r.GetDB().Model(&status).WithContext(ctx).Delete()
			if err != nil {
				log.Warn("failed to delete status record", zap.Error(err))
			}
		}
	}

	return nil
}

// GetUserConversations retrieves conversations for a user with pagination.
// Legacy note: DM rewrite M5 replaces this snapshot-hydrated list path with a keyed folder query
// over canonical per-user DM state.
func (r *ConversationRepository) GetUserConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	limit := clampListLimit(opts.Limit, 20, 100)

	inboxStates, _, inboxHasMore, err := r.listUserConversationStatesByFolderModels(ctx, userID, models.UserConversationFolderInbox, interfaces.PaginationOptions{
		Limit:  limit,
		Cursor: opts.Cursor,
	})
	if err != nil {
		return nil, err
	}

	requestStates, _, requestHasMore, err := r.listUserConversationStatesByFolderModels(ctx, userID, models.UserConversationFolderRequests, interfaces.PaginationOptions{
		Limit:  limit,
		Cursor: opts.Cursor,
	})
	if err != nil {
		return nil, err
	}

	mergedStates, nextCursor, hasMore := mergeVisibleConversationStatePages(inboxStates, requestStates, limit)
	if !hasMore {
		hasMore = inboxHasMore || requestHasMore
		if hasMore && nextCursor == "" && len(mergedStates) > 0 {
			nextCursor = mergedStates[len(mergedStates)-1].LegacyListCursor()
		}
	}

	conversations, err := r.loadConversationsForStates(ctx, mergedStates)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "user conversations")
	}

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:      conversations,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

// GetUserConversationsByRequestState retrieves conversations for a user filtered by the participant
// request state. This powers DM inbox vs. requests listings.
func (r *ConversationRepository) GetUserConversationsByRequestState(ctx context.Context, userID string, requestState models.DmRequestState, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	folder := folderFromRequestState(requestState)
	states, nextCursor, hasMore, err := r.listUserConversationStatesByFolderModels(ctx, userID, folder, opts)
	if err != nil {
		return nil, err
	}

	conversations, err := r.loadConversationsForStates(ctx, states)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "user conversations by request state")
	}

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:      conversations,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

func clampListLimit(limit int, defaultLimit int, maxLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// GetConversationParticipantRecord returns the most recent participant record for a given
// (conversationID, participantID) pair.
func (r *ConversationRepository) GetConversationParticipantRecord(ctx context.Context, conversationID, participantID string) (*models.ConversationParticipantRecord, error) {
	state, err := r.ensureUserConversationStateModel(ctx, participantID, conversationID)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}

	conversation, err := r.GetConversation(ctx, conversationID)
	if err != nil && !stdErrors.Is(err, storage.ErrNotFound) {
		return nil, err
	}

	return stateRecordFromModel(state, conversation), nil
}

// UpdateConversationParticipantRecord persists an updated participant record (metadata updates).
func (r *ConversationRepository) UpdateConversationParticipantRecord(ctx context.Context, record *models.ConversationParticipantRecord) error {
	if record == nil {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityConversation, "participant record nil")
	}
	if record.PK == "" || record.SK == "" {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityConversation, "participant record keys missing")
	}
	if strings.TrimSpace(record.ViewerID) == "" {
		record.ViewerID = strings.TrimPrefix(record.PK, "USER_CONVERSATIONS#")
	}
	if strings.TrimSpace(record.ConversationID) == "" {
		record.ConversationID = strings.TrimPrefix(record.GSI1PK, "CONVERSATION#")
		if record.ConversationID == record.GSI1PK {
			parts := strings.Split(record.SK, "#")
			if len(parts) > 1 {
				record.ConversationID = parts[len(parts)-1]
			}
		}
	}

	state, err := r.ensureUserConversationStateModel(ctx, record.ViewerID, record.ConversationID)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityConversation, record.PK)
	}

	state.CounterpartID = record.CounterpartID
	if strings.TrimSpace(state.CounterpartID) == "" {
		if conversation, convErr := r.GetConversation(ctx, record.ConversationID); convErr == nil && conversation != nil {
			state.CounterpartID = counterpartForConversation(record.ViewerID, conversation.Participants)
		}
	}
	state.Folder = participantRecordFolder(record)
	state.RequestState = record.RequestState
	state.RequestedAt = record.RequestedAt
	state.AcceptedAt = record.AcceptedAt
	state.DeclinedAt = record.DeclinedAt
	state.DeletedAt = record.DeletedAt
	state.Unread = record.Unread
	state.LastReadAt = record.LastReadAt
	if record.PreviewStatusID != "" {
		state.PreviewStatusID = record.PreviewStatusID
	}
	if !record.PreviewStatusPublishedAt.IsZero() {
		state.PreviewStatusPublishedAt = record.PreviewStatusPublishedAt
	}
	if !record.SortAt.IsZero() {
		state.SortAt = record.SortAt
	}
	state.UpdatedAt = time.Now().UTC()

	if err := state.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityConversation, record.PK)
	}
	if err := r.GetDB().WithContext(ctx).Model(state).Update(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityConversation, record.PK)
	}
	return nil
}

// GetConversationByParticipants finds a conversation with exact participants (KEEP - Complex participant search logic)
func (r *ConversationRepository) GetConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error) {
	log := r.logger.With(zap.Any("participants", participants))

	lookupKey := newConversationParticipantLookup("", participants)

	// Exact participant lookups are deterministic, so use the canonical base-table lookup row.
	var record models.ConversationParticipantKey
	err := r.GetDB().Model(&models.ConversationParticipantKey{}).WithContext(ctx).
		Where("PK", "=", lookupKey.PK).
		Where("SK", "=", lookupKey.SK).
		First(&record)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, EntityConversation, "participants")
		}
		log.Error("failed to query conversation by participants", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "participants")
	}

	// Get the full conversation
	return r.GetConversation(ctx, record.ConversationID)
}

// MarkConversationRead marks a conversation as read for a user (KEEP - Read receipt business logic)
func (r *ConversationRepository) MarkConversationRead(ctx context.Context, conversationID, username string) error {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.String("username", username),
	)

	if strings.TrimSpace(conversationID) == "" || strings.TrimSpace(username) == "" {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityConversation, conversationID)
	}

	now := time.Now().UTC()
	state, err := r.ensureUserConversationStateModel(ctx, username, conversationID)
	if err != nil {
		log.Error("failed to load user conversation state for mark read", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityConversation, conversationID)
	}
	state.Unread = false
	state.LastReadAt = &now
	state.UpdatedAt = now
	if err := state.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityConversation, conversationID)
	}
	if err := r.GetDB().WithContext(ctx).Model(state).Update(); err != nil {
		log.Error("failed to update user conversation state as read", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityConversation, conversationID)
	}

	status := &models.ConversationStatus{
		ConversationID: conversationID,
		UserID:         username,
		Unread:         false,
		LastReadAt:     now,
	}
	if err := status.BeforeCreate(); err == nil {
		if err := r.GetDB().Model(status).WithContext(ctx).Create(); err != nil {
			log.Warn("failed to maintain legacy conversation status row on mark read", zap.Error(err))
		}
	}

	return nil
}

// GetUnreadConversationCount gets the count of unread conversations for a user.
// Legacy note: DM rewrite M4/M5 replaces fan-out unread counting with keyed unread-state queries.
func (r *ConversationRepository) GetUnreadConversationCount(ctx context.Context, username string) (int, error) {
	count := 0
	cursor := ""
	for {
		result, err := r.ListUnreadUserConversationStates(ctx, username, interfaces.PaginationOptions{Limit: 100, Cursor: cursor})
		if err != nil {
			return 0, err
		}
		count += len(result.Items)
		if !result.HasMore || result.NextCursor == "" {
			return count, nil
		}
		cursor = result.NextCursor
	}
}

// AddStatusToConversation adds a status/message to a conversation (KEEP - Complex message threading logic)
func (r *ConversationRepository) AddStatusToConversation(ctx context.Context, conversationID, statusID, senderUsername string) error {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.String("status_id", statusID),
		zap.String("sender_username", senderUsername),
	)

	// Create conversation message record
	message := &models.ConversationMessage{
		ConversationID: conversationID,
		StatusID:       statusID,
		SenderUsername: senderUsername,
		CreatedAt:      time.Now(),
	}

	if err := message.BeforeCreate(); err != nil {
		log.Error("failed to prepare conversation message", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversationID)
	}

	// Create the message record
	if err := r.GetDB().Model(message).WithContext(ctx).Create(); err != nil {
		log.Error("failed to create conversation message", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversationID)
	}

	// Get conversation to update count and last message info
	conv, err := r.GetConversation(ctx, conversationID)
	if err != nil {
		log.Error("failed to get conversation for count update", zap.Error(err))
		// Don't fail the operation if we can't update the count
	} else {
		// Increment message count and update last message time
		conv.TotalMessageCount++
		conv.LastMessageTime = message.CreatedAt
		conv.UpdatedAt = time.Now()

		// Update the conversation's last status and counts
		conv.LastStatusID = statusID

		if err := r.UpdateConversation(ctx, conv); err != nil {
			log.Warn("failed to update conversation counts", zap.Error(err))
			// Don't fail the operation if count update fails
		}
	}

	// Mark conversation as unread for all participants except sender (KEEP - Notification logic)
	if conv != nil {
		for _, participantID := range conv.Participants {
			if participantID != senderUsername {
				if err := r.MarkConversationUnread(ctx, conversationID, participantID); err != nil {
					log.Warn("failed to mark conversation as unread for participant",
						zap.String("participant_id", participantID),
						zap.Error(err))
				}
			}
		}
	}

	log.Debug("successfully added status to conversation")
	return nil
}

// GetConversationStatuses retrieves messages in a conversation with pagination.
// Legacy note: DM rewrite M5 keeps DM thread reads on StatusRepository.GetConversationThread.
func (r *ConversationRepository) GetConversationStatuses(ctx context.Context, conversationID string, limit int, cursor string) ([]*storage.ConversationStatus, string, error) {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.Int("limit", limit),
		zap.String("cursor", cursor),
	)

	// Note: Based on the legacy code, conversation messages/statuses seem to be handled
	// differently than specified in the instructions. The legacy code doesn't show
	// STATUS# records under CONVERSATION# keys. This implementation follows the
	// instructions but may need adjustment based on actual usage.

	query := r.GetDB().Model(&models.ConversationMessage{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID))

	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	query = query.Limit(limit + 1)

	var messages []models.ConversationMessage
	err := query.Scan(&messages)
	if err != nil {
		log.Error("failed to query conversation statuses", zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityConversation, "statuses")
	}

	// Convert to storage.ConversationStatus
	statuses := make([]*storage.ConversationStatus, 0, len(messages))
	for _, msg := range messages {
		// This is a simplified conversion - actual implementation may need adjustment
		statuses = append(statuses, &storage.ConversationStatus{
			ConversationID: msg.ConversationID,
			UserID:         msg.SenderUsername,
			Unread:         false,
			LastReadAt:     &msg.CreatedAt,
		})
	}

	// Determine next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("statuses", statuses, limit); err != nil {
		statuses = statuses[:limit]
		if err := common.ValidateSliceLength("messages", messages, limit); err != nil {
			if err := common.ValidateSliceNotEmpty("messages", messages); err == nil {
				lastMsg := messages[limit]
				nextCursor = lastMsg.SK
			}
		}
	}

	return statuses, nextCursor, nil
}

// RemoveStatusFromConversation removes a status from a conversation.
// Legacy note: DM rewrite M3/M8 removes ConversationMessage-based DM mutation paths.
func (r *ConversationRepository) RemoveStatusFromConversation(ctx context.Context, conversationID, statusID string) error {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.String("status_id", statusID),
	)

	// Find and delete the message record
	var messages []models.ConversationMessage
	err := r.GetDB().Model(&models.ConversationMessage{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID)).
		Scan(&messages)

	if err != nil {
		log.Error("failed to query messages for deletion", zap.Error(err))
		return ErrorHandler.HandleQueryError(err, EntityConversation, "messages")
	}

	var messageDeleted bool
	for _, msg := range messages {
		if msg.StatusID == statusID {
			if err := r.GetDB().Model(&msg).WithContext(ctx).Delete(); err != nil {
				log.Error("failed to delete message", zap.Error(err))
				return ErrorHandler.HandleDeleteError(err, EntityConversation, statusID)
			}
			messageDeleted = true
			log.Debug("deleted conversation message")
			break
		}
	}

	if !messageDeleted {
		log.Debug("message not found in conversation, nothing to delete")
		return nil
	}

	// Update conversation message count
	conv, err := r.GetConversation(ctx, conversationID)
	if err != nil {
		log.Warn("failed to get conversation for count update", zap.Error(err))
		// Don't fail the operation if we can't update the count
		return nil
	}

	// Decrement message count (ensure it doesn't go below 0)
	if conv.TotalMessageCount > 0 {
		conv.TotalMessageCount--
	}
	conv.UpdatedAt = time.Now()

	// If this was the last status, find the new last status
	if conv.LastStatusID == statusID {
		// Get remaining messages to find the most recent one
		var remainingMessages []models.ConversationMessage
		err = r.GetDB().Model(&models.ConversationMessage{}).WithContext(ctx).
			Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID)).
			Limit(1). // Get the most recent
			Scan(&remainingMessages)

		if err == nil && len(remainingMessages) > 0 {
			// Update to the most recent remaining message
			conv.LastStatusID = remainingMessages[0].StatusID
			conv.LastMessageTime = remainingMessages[0].CreatedAt
		} else {
			// No messages remaining
			conv.LastStatusID = ""
			conv.LastMessageTime = time.Time{}
		}
	}

	// Update the conversation with new counts
	if err := r.UpdateConversation(ctx, conv); err != nil {
		log.Warn("failed to update conversation after message removal", zap.Error(err))
		// Don't fail the operation if count update fails
	}

	log.Debug("successfully removed status from conversation")
	return nil
}

// MarkStatusRead marks a specific status as read by a user (KEEP - Read receipt logic)
func (r *ConversationRepository) MarkStatusRead(ctx context.Context, conversationID, _, username string) error {
	// Mark the entire conversation as read (matching typical behavior)
	return r.MarkConversationRead(ctx, conversationID, username)
}

// GetUnreadStatusCount gets the count of unread statuses in a conversation for a user (KEEP - Complex counting logic)
func (r *ConversationRepository) GetUnreadStatusCount(ctx context.Context, conversationID, username string) (int, error) {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.String("username", username),
	)

	// Get the conversation status to find last read time
	var status models.ConversationStatus
	err := r.GetDB().Model(&models.ConversationStatus{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION_STATUS#%s", conversationID)).
		Where("SK", "=", canonicalConversationStatusSK(username)).
		First(&status)

	var lastReadTime time.Time
	if errors.IsNotFound(err) {
		// If no status record exists, user has never read this conversation
		// Count all messages as unread
		lastReadTime = time.Time{} // Zero time
	} else if err != nil {
		log.Error("failed to get conversation status", zap.Error(err))
		return 0, ErrorHandler.HandleGetError(err, EntityConversation, conversationID)
	} else {
		if !status.Unread {
			// Conversation is marked as read
			return 0, nil
		}
		lastReadTime = status.LastReadAt
	}

	// Count messages after the last read time
	return r.countMessagesAfterTime(ctx, conversationID, lastReadTime)
}

// LeaveConversation removes a participant from a conversation (KEEP - Complex participant management)
func (r *ConversationRepository) LeaveConversation(ctx context.Context, conversationID, username string) error {
	// This delegates to RemoveParticipant
	return r.RemoveParticipant(ctx, conversationID, username)
}

// AddParticipant adds a participant to a conversation (KEEP - Complex participant management)
func (r *ConversationRepository) AddParticipant(ctx context.Context, conversationID, participantID string) error {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.String("participant_id", participantID),
	)

	// Get the conversation
	conv, err := r.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}

	// Check if already a participant
	for _, p := range conv.Participants {
		if p == participantID {
			log.Debug("participant already in conversation")
			return nil // Already a participant
		}
	}

	// Add participant
	conv.Participants = append(conv.Participants, participantID)
	conv.UpdatedAt = time.Now()

	// Update the conversation
	return r.CreateConversation(ctx, conv, conv.Participants)
}

// GetConversationParticipants retrieves the list of participants in a conversation (KEEP - Participant retrieval logic)
func (r *ConversationRepository) GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error) {
	conv, err := r.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return conv.Participants, nil
}

// UpdateConversationLastStatus updates the last status in a conversation (KEEP - Conversation state update logic)
func (r *ConversationRepository) UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error {
	var status models.Status
	err := r.GetDB().WithContext(ctx).Model(&models.Status{}).
		Where("PK", "=", "status#"+lastStatusID).
		Where("SK", "=", "status#"+lastStatusID).
		First(&status)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityConversation, id)
	}

	if conversationID := strings.TrimSpace(status.ConversationID); conversationID != "" && conversationID != id {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityConversation, id)
	}

	lastMessageTime := status.PublishedAt.UTC()
	if lastMessageTime.IsZero() {
		lastMessageTime = status.CreatedAt.UTC()
	}
	if lastMessageTime.IsZero() {
		lastMessageTime = time.Now().UTC()
	}

	updateBuilder := r.GetDB().WithContext(ctx).Model(&models.Conversation{}).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", id)).
		Where("SK", "=", "METADATA").
		UpdateBuilder().
		SetIfNotExists("TotalMessageCount", nil, int64(0)).
		Add("TotalMessageCount", 1).
		Set("LastStatusID", lastStatusID).
		Set("LastMessageTime", lastMessageTime).
		Set("UpdatedAt", time.Now().UTC())

	if err := updateBuilder.Execute(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityConversation, id)
	}

	participantStates, err := r.ListConversationParticipantStates(ctx, id)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityConversation, id)
	}
	for _, stateContract := range participantStates {
		state, stateErr := r.getUserConversationStateModel(ctx, stateContract.ViewerID, id)
		if stateErr != nil {
			return ErrorHandler.HandleUpdateError(stateErr, EntityConversation, id)
		}
		state.PreviewStatusID = lastStatusID
		state.PreviewStatusPublishedAt = lastMessageTime
		state.SortAt = lastMessageTime
		state.UpdatedAt = time.Now().UTC()
		if stateErr := state.BeforeUpdate(); stateErr != nil {
			return ErrorHandler.HandleUpdateError(stateErr, EntityConversation, id)
		}
		if stateErr := r.GetDB().WithContext(ctx).Model(state).Update(); stateErr != nil {
			return ErrorHandler.HandleUpdateError(stateErr, EntityConversation, id)
		}
	}

	return nil
}

// RemoveParticipant removes a participant from a conversation (KEEP - Complex participant management)
func (r *ConversationRepository) RemoveParticipant(ctx context.Context, conversationID, participantID string) error {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.String("participant_id", participantID),
	)

	// Get the conversation
	conv, err := r.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}

	// Remove participant
	newParticipants := make([]string, 0, len(conv.Participants))
	for _, p := range conv.Participants {
		if p != participantID {
			newParticipants = append(newParticipants, p)
		}
	}

	if len(newParticipants) == len(conv.Participants) {
		log.Debug("participant not found in conversation")
		return nil // Participant not found
	}

	conv.Participants = newParticipants
	conv.UpdatedAt = time.Now()

	// Update the conversation
	return r.CreateConversation(ctx, conv, conv.Participants)
}

// CreateConversationMute creates a new conversation mute (KEEP - Mute business logic)
func (r *ConversationRepository) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	model := &models.ConversationMute{
		Username:       mute.Username,
		ConversationID: mute.ConversationID,
		CreatedAt:      mute.CreatedAt,
		ExpiresAt:      mute.ExpiresAt,
	}

	if err := model.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityConversation, mute.ConversationID)
	}

	if err := r.GetDB().Model(model).WithContext(ctx).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityConversation, model.ConversationID)
	}

	return nil
}

// DeleteConversationMute removes a conversation mute (REPLACE with BaseRepository pattern)
func (r *ConversationRepository) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	err := r.GetDB().Model(&models.ConversationMute{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("CONVERSATION_MUTE#%s", conversationID)).
		Delete()

	if err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityConversation, conversationID)
	}

	return nil
}

// IsConversationMuted checks if a conversation is muted by a user (KEEP - Mute check logic with expiration)
func (r *ConversationRepository) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	var mute models.ConversationMute
	err := r.GetDB().Model(&models.ConversationMute{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("CONVERSATION_MUTE#%s", conversationID)).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, ErrorHandler.HandleGetError(err, EntityConversation, conversationID)
	}

	// Check if mute has expired
	if !mute.ExpiresAt.IsZero() && mute.ExpiresAt.Before(time.Now()) {
		// Delete expired mute
		_ = r.DeleteConversationMute(ctx, username, conversationID)
		return false, nil
	}

	return true, nil
}

// GetMutedConversations retrieves all muted conversations for a user (KEEP - Complex mute retrieval with expiration cleanup)
func (r *ConversationRepository) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	var mutes []models.ConversationMute
	err := r.GetDB().Model(&models.ConversationMute{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Scan(&mutes)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "muted")
	}

	conversationIDs := make([]string, 0, len(mutes))
	now := time.Now()

	for _, mute := range mutes {
		// Skip expired mutes
		if !mute.ExpiresAt.IsZero() && mute.ExpiresAt.Before(now) {
			// Delete expired mute
			_ = r.DeleteConversationMute(ctx, username, mute.ConversationID)
			continue
		}
		conversationIDs = append(conversationIDs, mute.ConversationID)
	}

	return conversationIDs, nil
}

// MarkConversationUnread marks a conversation as unread for a user (KEEP - Unread notification logic)
func (r *ConversationRepository) MarkConversationUnread(ctx context.Context, conversationID, userID string) error {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.String("user_id", userID),
	)

	if strings.TrimSpace(conversationID) == "" || strings.TrimSpace(userID) == "" {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityConversation, conversationID)
	}

	state, err := r.ensureUserConversationStateModel(ctx, userID, conversationID)
	if err != nil {
		log.Error("failed to load user conversation state for mark unread", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityConversation, conversationID)
	}
	state.Unread = true
	state.LastReadAt = nil
	state.UpdatedAt = time.Now().UTC()
	if err := state.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityConversation, conversationID)
	}
	if err := r.GetDB().WithContext(ctx).Model(state).Update(); err != nil {
		log.Error("failed to update user conversation state as unread", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityConversation, conversationID)
	}

	status := &models.ConversationStatus{
		ConversationID: conversationID,
		UserID:         userID,
		Unread:         true,
		LastReadAt:     time.Unix(0, 0).UTC(),
	}
	if err := status.BeforeCreate(); err == nil {
		if err := r.GetDB().Model(status).WithContext(ctx).Create(); err != nil {
			log.Warn("failed to maintain legacy conversation status row on mark unread", zap.Error(err))
		}
	}

	return nil
}

// GetUnreadConversations retrieves unread conversations for a user.
// Legacy note: DM rewrite M4/M5 replaces unread list fan-out with a keyed sparse unread query
// over canonical per-user DM state.
func (r *ConversationRepository) GetUnreadConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	stateResult, err := r.ListUnreadUserConversationStates(ctx, userID, opts)
	if err != nil {
		return nil, err
	}

	states := make([]*models.UserConversationState, 0, len(stateResult.Items))
	for _, item := range stateResult.Items {
		states = append(states, &models.UserConversationState{
			ViewerID:                 item.ViewerID,
			ConversationID:           item.ConversationID,
			CounterpartID:            item.CounterpartID,
			Folder:                   item.Folder,
			RequestState:             item.RequestState,
			PreviewStatusID:          item.PreviewStatusID,
			PreviewStatusPublishedAt: item.PreviewStatusPublishedAt,
			SortAt:                   item.SortAt,
			Unread:                   item.Unread,
			LastReadAt:               item.LastReadAt,
			DeletedAt:                item.DeletedAt,
			RequestedAt:              item.RequestedAt,
			AcceptedAt:               item.AcceptedAt,
			DeclinedAt:               item.DeclinedAt,
			CreatedAt:                item.CreatedAt,
			UpdatedAt:                item.UpdatedAt,
		})
	}
	unreadConversations, err := r.loadConversationsForStates(ctx, states)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "unread conversations")
	}

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:      unreadConversations,
		NextCursor: stateResult.NextCursor,
		HasMore:    stateResult.HasMore,
		Total:      stateResult.Total,
	}, nil
}

// SearchConversations searches conversations for a user by query (KEEP - Search logic with filtering)
func (r *ConversationRepository) SearchConversations(ctx context.Context, userID, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	log := r.logger.With(
		zap.String("user_id", userID),
		zap.String("query", query),
	)

	// Use client-side filtering for simplicity in serverless environment
	// For high-scale deployments, consider ElasticSearch or DynamoDB full-text search
	allResult, err := r.GetUserConversations(ctx, userID, opts)
	if err != nil {
		return nil, err
	}

	// Simple string matching on participant IDs
	// In production, this should use proper search indexing
	matchingConversations := make([]*models.Conversation, 0)
	queryLower := strings.ToLower(query)

	for _, conv := range allResult.Items {
		for _, participant := range conv.Participants {
			if strings.Contains(strings.ToLower(participant), queryLower) {
				matchingConversations = append(matchingConversations, conv)
				break
			}
		}
	}

	log.Debug("search completed", zap.Int("matches", len(matchingConversations)))

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:      matchingConversations,
		NextCursor: allResult.NextCursor,
		HasMore:    len(matchingConversations) >= opts.Limit,
		Total:      int64(len(matchingConversations)),
	}, nil
}

// GetConversationMessageCount gets the total number of messages in a conversation (KEEP - Complex counting with caching)
func (r *ConversationRepository) GetConversationMessageCount(ctx context.Context, conversationID string) (int64, error) {
	log := r.logger.With(zap.String("conversation_id", conversationID))

	// Try to get cached count from conversation record first
	conv, err := r.GetConversation(ctx, conversationID)
	if err == nil && conv.TotalMessageCount > 0 {
		// Return cached count if available and non-zero
		return conv.TotalMessageCount, nil
	}

	// If no cached count, count messages directly
	var messages []models.ConversationMessage
	err = r.GetDB().Model(&models.ConversationMessage{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID)).
		Scan(&messages)

	if err != nil {
		log.Error("failed to count conversation messages", zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityConversation, "message count")
	}

	count := int64(len(messages))
	log.Debug("counted conversation messages", zap.Int64("count", count))

	// Update the cached count in the conversation record
	if conv != nil {
		conv.TotalMessageCount = count
		if err := r.UpdateConversation(ctx, conv); err != nil {
			log.Warn("failed to update cached message count", zap.Error(err))
			// Don't fail the operation if caching fails
		}
	}

	return count, nil
}

// GetUnreadMessageCount gets the count of unread messages for a user across all conversations (KEEP - Complex aggregation logic)
func (r *ConversationRepository) GetUnreadMessageCount(ctx context.Context, username string) (int64, error) {
	log := r.logger.With(zap.String("username", username))

	// Get all conversations for the user
	opts := interfaces.PaginationOptions{Limit: 1000} // Large limit to get all
	result, err := r.GetUserConversations(ctx, username, opts)
	if err != nil {
		return 0, err
	}

	var totalUnreadCount int64
	for _, conv := range result.Items {
		// Get unread count for each conversation
		unreadCount, err := r.GetUnreadStatusCount(ctx, conv.ID, username)
		if err != nil {
			log.Warn("failed to get unread count for conversation",
				zap.String("conversation_id", conv.ID),
				zap.Error(err))
			continue
		}
		totalUnreadCount += int64(unreadCount)
	}

	log.Debug("calculated total unread message count", zap.Int64("count", totalUnreadCount))
	return totalUnreadCount, nil
}

// countMessagesAfterTime counts messages in a conversation after a specific time (KEEP - Complex time-based counting)
func (r *ConversationRepository) countMessagesAfterTime(ctx context.Context, conversationID string, afterTime time.Time) (int, error) {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.Time("after_time", afterTime),
	)

	query := r.GetDB().Model(&models.ConversationMessage{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID))

	// If afterTime is not zero, filter for messages after that time
	if !afterTime.IsZero() {
		// SK format is STATUS#timestamp#statusID, so we need to filter by SK
		timePrefix := fmt.Sprintf("STATUS#%s", afterTime.Format(time.RFC3339Nano))
		query = query.Where("SK", ">", timePrefix)
	}

	var messages []models.ConversationMessage
	err := query.Scan(&messages)
	if err != nil {
		log.Error("failed to count messages after time", zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityConversation, "messages after time")
	}

	count := len(messages)
	log.Debug("counted messages after time", zap.Int("count", count))
	return count, nil
}

// GetConversationMessagesByTimeRange gets messages in a conversation within a time range (KEEP - Complex time range queries)
func (r *ConversationRepository) GetConversationMessagesByTimeRange(ctx context.Context, conversationID string, startTime, endTime time.Time, limit int) ([]*models.ConversationMessage, error) {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime),
		zap.Int("limit", limit),
	)

	query := r.GetDB().Model(&models.ConversationMessage{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID))

	// Add time range filters using SK (STATUS#timestamp#statusID format)
	if !startTime.IsZero() {
		startPrefix := fmt.Sprintf("STATUS#%s", startTime.Format(time.RFC3339Nano))
		query = query.Where("SK", ">=", startPrefix)
	}
	if !endTime.IsZero() {
		endPrefix := fmt.Sprintf("STATUS#%s", endTime.Format(time.RFC3339Nano))
		query = query.Where("SK", "<=", endPrefix)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	var messages []models.ConversationMessage
	err := query.Scan(&messages)
	if err != nil {
		log.Error("failed to get messages by time range", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "messages by time range")
	}

	// Convert to pointers
	result := make([]*models.ConversationMessage, len(messages))
	for i := range messages {
		result[i] = &messages[i]
	}

	log.Debug("retrieved messages by time range", zap.Int("count", len(result)))
	return result, nil
}

// generateRandomString generates a random string of specified length (KEEP - Utility function)
func (r *ConversationRepository) generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
