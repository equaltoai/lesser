package repositories

import (
	"context"
	"fmt"
	"sort"
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
	logger *zap.Logger
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
	}
}

// CreateConversation creates a new conversation with participants (KEEP - Complex conversation business logic)
func (r *ConversationRepository) CreateConversation(ctx context.Context, conversation *models.Conversation, participants []string) error {
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
		conversation.Participants = participants
	}

	if err := conversation.BeforeCreate(); err != nil {
		log.Error("failed to prepare conversation", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversation.ID)
	}

	// Create main conversation record using enhanced validation and creation
	if err := r.ValidateAndCreate(ctx, conversation); err != nil {
		log.Error("failed to create conversation", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversation.ID)
	}

	// Create participant records for each participant (KEEP - Conversation threading logic)
	for _, participantID := range conversation.Participants {
		participantRecord := &models.ConversationParticipantRecord{}

		// Preserve participant metadata across conversation updates by looking up the existing record(s)
		// via the reverse index: GSI1PK=CONVERSATION#<id>, GSI1SK=PARTICIPANT#<username>.
		existingRecords, err := r.findParticipantRecordsByConversationAndParticipant(ctx, conversation.ID, participantID)
		if err != nil && !errors.IsNotFound(err) {
			log.Warn("failed to lookup existing participant record metadata; proceeding with defaults",
				zap.String("participant_id", participantID),
				zap.Error(err))
		}

		if len(existingRecords) > 0 {
			// Pick the most recent record (SK is RFC3339 timestamp + conversation ID).
			sort.Slice(existingRecords, func(i, j int) bool {
				return existingRecords[i].SK > existingRecords[j].SK
			})
			latest := existingRecords[0]
			participantRecord.RequestState = latest.RequestState
			participantRecord.RequestedAt = latest.RequestedAt
			participantRecord.AcceptedAt = latest.AcceptedAt
			participantRecord.DeclinedAt = latest.DeclinedAt
			participantRecord.DeletedAt = latest.DeletedAt
			participantRecord.Unread = latest.Unread
			participantRecord.LastReadAt = latest.LastReadAt
		}

		// Always embed a per-participant conversation copy so Unread can be per-user.
		if conversation != nil {
			convCopy := *conversation
			convCopy.Unread = participantRecord.Unread
			participantRecord.Conversation = &convCopy
		}

		if err := participantRecord.BeforeCreate(participantID); err != nil {
			log.Error("failed to prepare participant record",
				zap.String("participant_id", participantID),
				zap.Error(err))
			continue
		}

		// Remove any stale participant records for this (conversation, participant) pair so we don't
		// accumulate duplicates as the conversation UpdatedAt (and thus SK) changes.
		for _, existing := range existingRecords {
			err := r.GetDB().Model(&models.ConversationParticipantRecord{}).WithContext(ctx).
				Where("PK", "=", existing.PK).
				Where("SK", "=", existing.SK).
				Delete()
			if err != nil && !errors.IsNotFound(err) {
				log.Warn("failed to delete stale participant record",
					zap.String("participant_id", participantID),
					zap.String("pk", existing.PK),
					zap.String("sk", existing.SK),
					zap.Error(err))
			}
		}

		if err := r.GetDB().Model(participantRecord).WithContext(ctx).Create(); err != nil {
			log.Error("failed to create participant record",
				zap.String("participant_id", participantID),
				zap.Error(err))
			continue
		}
	}

	// Create participant lookup key if needed (for GetConversationByParticipants) - KEEP - Conversation search logic
	sortedParticipants := make([]string, len(conversation.Participants))
	copy(sortedParticipants, conversation.Participants)
	sort.Strings(sortedParticipants)
	participantKey := strings.Join(sortedParticipants, ",")

	lookupKey := &models.ConversationParticipantKey{
		PK:             fmt.Sprintf("CONVERSATION_PARTICIPANTS#%s", participantKey),
		SK:             "LOOKUP",
		GSI1PK:         fmt.Sprintf("CONVERSATION_PARTICIPANTS#%s", participantKey),
		ConversationID: conversation.ID,
	}

	if err := r.GetDB().Model(lookupKey).WithContext(ctx).Create(); err != nil {
		log.Warn("failed to create participant lookup key", zap.Error(err))
		// Don't fail the operation if lookup key creation fails
	}

	log.Debug("conversation created successfully")
	return nil
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

	// Delete participant records (KEEP - Conversation cleanup logic)
	if conv != nil {
		for _, participantID := range conv.Participants {
			// Delete participant record
			err = r.GetDB().Model(&models.ConversationParticipantRecord{}).WithContext(ctx).
				Where("PK", "=", fmt.Sprintf("USER_CONVERSATIONS#%s", participantID)).
				Where("SK", "=", fmt.Sprintf("%s#%s", conv.UpdatedAt.Format(time.RFC3339), id)).
				Delete()
			if err != nil {
				log.Warn("failed to delete participant record",
					zap.String("participant_id", participantID),
					zap.Error(err))
				// Continue deleting other records
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

// GetUserConversations retrieves conversations for a user with pagination (KEEP - Complex pagination logic)
func (r *ConversationRepository) GetUserConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	log := r.logger.With(
		zap.String("user_id", userID),
		zap.Int("limit", opts.Limit),
		zap.String("cursor", opts.Cursor),
	)

	query := r.GetDB().WithContext(ctx).Model(&models.ConversationParticipantRecord{}).
		Where("PK", "=", fmt.Sprintf("USER_CONVERSATIONS#%s", userID)).
		OrderBy("SK", "DESC") // Most recent first (timestamp-based sorting)

	// Add cursor condition if provided
	if opts.Cursor != "" {
		query = query.Where("SK", "<", opts.Cursor)
	}

	// Query with limit + 1 to determine if there are more results
	query = query.Limit(opts.Limit + 1)

	var records []models.ConversationParticipantRecord
	err := query.Scan(&records)
	if err != nil {
		log.Error("failed to query user conversations", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "user conversations")
	}

	conversations := make([]*models.Conversation, 0, len(records))
	for _, record := range records {
		if record.DeletedAt != nil && !record.DeletedAt.IsZero() {
			continue
		}
		if record.Conversation != nil {
			// Ensure per-user unread status is populated on the returned Conversation model.
			record.Conversation.Unread = record.Unread
			conversations = append(conversations, record.Conversation)
		}
	}

	// Determine next cursor and pagination
	var nextCursor string
	hasMore := len(conversations) > opts.Limit
	if hasMore {
		conversations = conversations[:opts.Limit]
		if err := common.ValidateSliceNotEmpty("conversations", conversations); err == nil {
			lastConv := conversations[len(conversations)-1]
			nextCursor = fmt.Sprintf("%s#%s", lastConv.UpdatedAt.Format(time.RFC3339), lastConv.ID)
		}
	}

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:      conversations,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1, // Not calculated
	}, nil
}

// GetUserConversationsByRequestState retrieves conversations for a user filtered by the participant
// request state. This powers DM inbox vs. requests listings.
func (r *ConversationRepository) GetUserConversationsByRequestState(ctx context.Context, userID string, requestState models.DmRequestState, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	scanCursor := opts.Cursor
	conversations := make([]*models.Conversation, 0, limit)
	var nextCursor string
	hasMore := false

	// Filtering requires scanning participant records until we have enough matches or the table is exhausted.
	// Keep the scan window bounded to avoid pathological loops in large inboxes.
	const maxIterations = 10
	for iter := 0; iter < maxIterations && len(conversations) < limit; iter++ {
		// Fetch more than we need because we'll filter client-side.
		fetchLimit := limit * 3
		if fetchLimit < 20 {
			fetchLimit = 20
		}
		if fetchLimit > 200 {
			fetchLimit = 200
		}

		query := r.GetDB().WithContext(ctx).Model(&models.ConversationParticipantRecord{}).
			Where("PK", "=", fmt.Sprintf("USER_CONVERSATIONS#%s", userID)).
			OrderBy("SK", "DESC").
			Limit(fetchLimit + 1)
		if scanCursor != "" {
			query = query.Where("SK", "<", scanCursor)
		}

		var records []*models.ConversationParticipantRecord
		if err := query.All(&records); err != nil {
			if errors.IsNotFound(err) {
				break
			}
			return nil, ErrorHandler.HandleQueryError(err, EntityConversation, "user conversations by request state")
		}

		pageHasMore := len(records) > fetchLimit
		if pageHasMore {
			records = records[:fetchLimit]
		}

		if len(records) == 0 {
			break
		}

		for _, record := range records {
			if record == nil {
				continue
			}
			scanCursor = record.SK

			if record.DeletedAt != nil && !record.DeletedAt.IsZero() {
				continue
			}
			matchesRequestState := record.RequestState == requestState
			// Back-compat: conversations created before DM v1 metadata default to inbox.
			if !matchesRequestState && requestState == models.DmRequestStateAccepted && record.RequestState == "" {
				matchesRequestState = true
			}
			if !matchesRequestState {
				continue
			}
			if record.Conversation == nil {
				continue
			}

			record.Conversation.Unread = record.Unread
			conversations = append(conversations, record.Conversation)
			if len(conversations) >= limit {
				break
			}
		}

		if len(conversations) >= limit {
			// There may be more matching items after the last scanned SK.
			hasMore = true
			nextCursor = scanCursor
			break
		}

		if !pageHasMore {
			// Exhausted the user's conversations.
			hasMore = false
			nextCursor = ""
			break
		}

		// Continue scanning older records.
		hasMore = true
		nextCursor = scanCursor
	}

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:      conversations,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      -1,
	}, nil
}

// GetConversationParticipantRecord returns the most recent participant record for a given
// (conversationID, participantID) pair.
func (r *ConversationRepository) GetConversationParticipantRecord(ctx context.Context, conversationID, participantID string) (*models.ConversationParticipantRecord, error) {
	records, err := r.findParticipantRecordsByConversationAndParticipant(ctx, conversationID, participantID)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	if len(records) == 0 {
		return nil, storage.ErrNotFound
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].SK > records[j].SK
	})
	latest := records[0]
	return &latest, nil
}

// UpdateConversationParticipantRecord persists an updated participant record (metadata updates).
func (r *ConversationRepository) UpdateConversationParticipantRecord(ctx context.Context, record *models.ConversationParticipantRecord) error {
	if record == nil {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityConversation, "participant record nil")
	}
	if record.PK == "" || record.SK == "" {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, EntityConversation, "participant record keys missing")
	}
	if record.Conversation != nil {
		record.Conversation.Unread = record.Unread
	}

	if err := r.GetDB().WithContext(ctx).Model(record).Update(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityConversation, record.PK)
	}
	return nil
}

func (r *ConversationRepository) findParticipantRecordsByConversationAndParticipant(ctx context.Context, conversationID, participantID string) ([]models.ConversationParticipantRecord, error) {
	var records []models.ConversationParticipantRecord
	err := r.GetDB().WithContext(ctx).Model(&models.ConversationParticipantRecord{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf(models.KeyPatternConversation, conversationID)).
		Where("gsi1SK", "=", fmt.Sprintf("PARTICIPANT#%s", participantID)).
		All(&records)
	if err != nil {
		return nil, err
	}
	return records, nil
}

// GetConversationByParticipants finds a conversation with exact participants (KEEP - Complex participant search logic)
func (r *ConversationRepository) GetConversationByParticipants(ctx context.Context, participants []string) (*models.Conversation, error) {
	log := r.logger.With(zap.Any("participants", participants))

	// Sort participants to create a consistent lookup key (matching legacy)
	sortedParticipants := make([]string, len(participants))
	copy(sortedParticipants, participants)
	// Simple sort for deterministic order
	for i := 0; i < len(sortedParticipants)-1; i++ {
		for j := i + 1; j < len(sortedParticipants); j++ {
			if sortedParticipants[i] > sortedParticipants[j] {
				sortedParticipants[i], sortedParticipants[j] = sortedParticipants[j], sortedParticipants[i]
			}
		}
	}

	// Create a consistent participant key
	participantKey := strings.Join(sortedParticipants, ",")

	// Query by participant key using GSI1
	var record models.ConversationParticipantKey
	err := r.GetDB().Model(&models.ConversationParticipantKey{}).WithContext(ctx).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("CONVERSATION_PARTICIPANTS#%s", participantKey)).
		Limit(1).
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

	status := &models.ConversationStatus{
		ConversationID: conversationID,
		UserID:         username,
		Unread:         false,
		LastReadAt:     time.Now(),
	}

	if err := status.BeforeCreate(); err != nil {
		log.Error("failed to prepare status", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversationID)
	}

	err := r.GetDB().Model(status).WithContext(ctx).Create()
	if err != nil {
		log.Error("failed to mark conversation as read", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityConversation, conversationID)
	}

	// Best-effort: keep the participant record unread metadata in sync.
	if record, err := r.GetConversationParticipantRecord(ctx, conversationID, username); err == nil && record != nil {
		record.Unread = false
		t := status.LastReadAt
		record.LastReadAt = &t
		if err := r.UpdateConversationParticipantRecord(ctx, record); err != nil {
			log.Warn("failed to sync participant unread metadata on mark read", zap.Error(err))
		}
	}

	return nil
}

// GetUnreadConversationCount gets the count of unread conversations for a user (KEEP - Complex count logic)
func (r *ConversationRepository) GetUnreadConversationCount(ctx context.Context, username string) (int, error) {
	log := r.logger.With(zap.String("username", username))

	// Get all conversations for the user
	opts := interfaces.PaginationOptions{Limit: 1000} // Large limit to get all
	result, err := r.GetUserConversations(ctx, username, opts)
	if err != nil {
		return 0, err
	}

	unreadCount := 0
	for _, conv := range result.Items {
		// Check if conversation has unread status
		var status models.ConversationStatus
		err := r.GetDB().WithContext(ctx).Model(&models.ConversationStatus{}).
			Where("PK", "=", fmt.Sprintf("CONVERSATION_STATUS#%s", conv.ID)).
			Where("SK", "=", fmt.Sprintf("USER#%s", username)).
			First(&status)

		if errors.IsNotFound(err) || status.Unread {
			unreadCount++
		}
	}

	log.Debug("unread conversation count", zap.Int("count", unreadCount))
	return unreadCount, nil
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

// GetConversationStatuses retrieves messages in a conversation with pagination (KEEP - Complex message retrieval)
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

// RemoveStatusFromConversation removes a status from a conversation (KEEP - Complex message removal logic)
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
		Where("SK", "=", fmt.Sprintf("USER#%s", username)).
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
	// Get current conversation
	conv, err := r.GetConversation(ctx, id)
	if err != nil {
		return err
	}

	// Update fields
	conv.LastStatusID = lastStatusID
	conv.UpdatedAt = time.Now()

	// Update the conversation (this will recreate participant records with new timestamps)
	return r.UpdateConversation(ctx, conv)
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

	status := &models.ConversationStatus{
		ConversationID: conversationID,
		UserID:         userID,
		Unread:         true,
		LastReadAt:     time.Unix(0, 0).UTC(), // Epoch sentinel for unread
	}

	if err := status.BeforeCreate(); err != nil {
		log.Error("failed to prepare status", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversationID)
	}

	err := r.GetDB().Model(status).WithContext(ctx).Create()
	if err != nil {
		log.Error("failed to mark conversation as unread", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityConversation, conversationID)
	}

	// Best-effort: keep the participant record unread metadata in sync.
	if record, err := r.GetConversationParticipantRecord(ctx, conversationID, userID); err == nil && record != nil {
		record.Unread = true
		record.LastReadAt = nil
		if err := r.UpdateConversationParticipantRecord(ctx, record); err != nil {
			log.Warn("failed to sync participant unread metadata on mark unread", zap.Error(err))
		}
	}

	return nil
}

// GetUnreadConversations retrieves unread conversations for a user (KEEP - Complex filtering logic)
func (r *ConversationRepository) GetUnreadConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	log := r.logger.With(zap.String("user_id", userID))

	// Get all user conversations
	allResult, err := r.GetUserConversations(ctx, userID, opts)
	if err != nil {
		return nil, err
	}

	// Filter for unread conversations
	unreadConversations := make([]*models.Conversation, 0)
	for _, conv := range allResult.Items {
		// Check if conversation has unread status
		var status models.ConversationStatus
		err := r.GetDB().WithContext(ctx).Model(&models.ConversationStatus{}).
			Where("PK", "=", fmt.Sprintf("CONVERSATION_STATUS#%s", conv.ID)).
			Where("SK", "=", fmt.Sprintf("USER#%s", userID)).
			First(&status)

		if errors.IsNotFound(err) || status.Unread {
			unreadConversations = append(unreadConversations, conv)
		}
	}

	log.Debug("retrieved unread conversations", zap.Int("count", len(unreadConversations)))

	return &interfaces.PaginatedResult[*models.Conversation]{
		Items:      unreadConversations,
		NextCursor: allResult.NextCursor,
		HasMore:    allResult.HasMore,
		Total:      int64(len(unreadConversations)),
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
