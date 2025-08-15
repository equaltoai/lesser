package repositories

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ConversationRepository handles conversation-related database operations
type ConversationRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewConversationRepository creates a new conversation repository
func NewConversationRepository(db core.DB, logger *zap.Logger) *ConversationRepository {
	return &ConversationRepository{
		db:     db,
		logger: logger,
	}
}

// CreateConversation creates a new conversation with participants
func (r *ConversationRepository) CreateConversation(ctx context.Context, conversation *models.Conversation, participants []string) error {
	log := r.logger.With(zap.String("conversation_id", conversation.ID))

	// Generate ID if not provided (matching legacy behavior)
	if conversation.ID == "" {
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
	if len(participants) > 0 {
		conversation.Participants = participants
	}

	if err := conversation.BeforeCreate(); err != nil {
		log.Error("failed to prepare conversation", zap.Error(err))
		return fmt.Errorf("failed to prepare conversation: %w", err)
	}

	// Create main conversation record
	if err := r.db.Model(conversation).WithContext(ctx).Create(); err != nil {
		log.Error("failed to create conversation", zap.Error(err))
		return fmt.Errorf("failed to create conversation: %w", err)
	}

	// Create participant records for each participant
	for _, participantID := range conversation.Participants {
		participantRecord := &models.ConversationParticipantRecord{
			Conversation: conversation,
		}

		if err := participantRecord.BeforeCreate(participantID); err != nil {
			log.Error("failed to prepare participant record",
				zap.String("participant_id", participantID),
				zap.Error(err))
			continue
		}

		if err := r.db.Model(participantRecord).WithContext(ctx).Create(); err != nil {
			log.Error("failed to create participant record",
				zap.String("participant_id", participantID),
				zap.Error(err))
			continue
		}
	}

	// Create participant lookup key if needed (for GetConversationByParticipants)
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

	if err := r.db.Model(lookupKey).WithContext(ctx).Create(); err != nil {
		log.Warn("failed to create participant lookup key", zap.Error(err))
		// Don't fail the operation if lookup key creation fails
	}

	log.Debug("conversation created successfully")
	return nil
}

// GetConversation retrieves a conversation by ID
func (r *ConversationRepository) GetConversation(ctx context.Context, id string) (*models.Conversation, error) {
	log := r.logger.With(zap.String("conversation_id", id))

	var conv models.Conversation
	err := r.db.Model(&models.Conversation{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", id)).
		Where("SK", "=", "METADATA").
		First(&conv)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("conversation not found")
		}
		log.Error("failed to get conversation", zap.Error(err))
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	return &conv, nil
}

// UpdateConversation updates a conversation
func (r *ConversationRepository) UpdateConversation(ctx context.Context, conversation *models.Conversation) error {
	// For updating, we recreate all records (matching legacy behavior)
	// This ensures participant records are updated with new timestamps
	return r.CreateConversation(ctx, conversation, conversation.Participants)
}

// DeleteConversation deletes a conversation by ID
func (r *ConversationRepository) DeleteConversation(ctx context.Context, id string) error {
	log := r.logger.With(zap.String("conversation_id", id))

	// Get the conversation first to get participant list
	conv, err := r.GetConversation(ctx, id)
	if err != nil {
		log.Warn("failed to get conversation for cleanup", zap.Error(err))
		// Continue with deletion even if we can't get the conversation
	}

	// Delete main record
	err = r.db.Model(&models.Conversation{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", id)).
		Where("SK", "=", "METADATA").
		Delete()
	if err != nil {
		log.Error("failed to delete conversation", zap.Error(err))
		return fmt.Errorf("failed to delete conversation: %w", err)
	}

	// Delete participant records
	if conv != nil {
		for _, participantID := range conv.Participants {
			// Delete participant record
			err = r.db.Model(&models.ConversationParticipantRecord{}).WithContext(ctx).
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

	// Delete all status records for this conversation
	var statuses []models.ConversationStatus
	err = r.db.Model(&models.ConversationStatus{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION_STATUS#%s", id)).
		Scan(&statuses)

	if err == nil {
		for _, status := range statuses {
			err := r.db.Model(&status).WithContext(ctx).Delete()
			if err != nil {
				log.Warn("failed to delete status record", zap.Error(err))
			}
		}
	}

	return nil
}

// GetUserConversations retrieves conversations for a user with pagination
func (r *ConversationRepository) GetUserConversations(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Conversation], error) {
	log := r.logger.With(
		zap.String("user_id", userID),
		zap.Int("limit", opts.Limit),
		zap.String("cursor", opts.Cursor),
	)

	query := r.db.WithContext(ctx).Model(&models.ConversationParticipantRecord{}).
		Where("PK", "=", fmt.Sprintf("USER_CONVERSATIONS#%s", userID))

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
		return nil, fmt.Errorf("failed to query user conversations: %w", err)
	}

	conversations := make([]*models.Conversation, 0, len(records))
	for _, record := range records {
		if record.Conversation != nil {
			conversations = append(conversations, record.Conversation)
		}
	}

	// Determine next cursor and pagination
	var nextCursor string
	hasMore := len(conversations) > opts.Limit
	if hasMore {
		conversations = conversations[:opts.Limit]
		if len(conversations) > 0 {
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

// GetConversationByParticipants finds a conversation with exact participants
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
	err := r.db.Model(&models.ConversationParticipantKey{}).WithContext(ctx).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("CONVERSATION_PARTICIPANTS#%s", participantKey)).
		Limit(1).
		First(&record)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("conversation not found")
		}
		log.Error("failed to query conversation by participants", zap.Error(err))
		return nil, fmt.Errorf("failed to query conversation: %w", err)
	}

	// Get the full conversation
	return r.GetConversation(ctx, record.ConversationID)
}

// MarkConversationRead marks a conversation as read for a user
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
		return fmt.Errorf("failed to prepare status: %w", err)
	}

	err := r.db.Model(status).WithContext(ctx).Create()
	if err != nil {
		log.Error("failed to mark conversation as read", zap.Error(err))
		return fmt.Errorf("failed to mark conversation as read: %w", err)
	}

	return nil
}

// GetUnreadConversationCount gets the count of unread conversations for a user
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
		err := r.db.WithContext(ctx).Model(&models.ConversationStatus{}).
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

// AddStatusToConversation adds a status/message to a conversation
func (r *ConversationRepository) AddStatusToConversation(ctx context.Context, conversationID, statusID, _ string) error {
	// Update the conversation's last status
	return r.UpdateConversationLastStatus(ctx, conversationID, statusID)
}

// GetConversationStatuses retrieves messages in a conversation with pagination
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

	query := r.db.Model(&models.ConversationMessage{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID))

	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	query = query.Limit(limit + 1)

	var messages []models.ConversationMessage
	err := query.Scan(&messages)
	if err != nil {
		log.Error("failed to query conversation statuses", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query conversation statuses: %w", err)
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
	if len(statuses) > limit {
		statuses = statuses[:limit]
		if len(messages) > limit && len(messages) > 0 {
			lastMsg := messages[limit]
			nextCursor = lastMsg.SK
		}
	}

	return statuses, nextCursor, nil
}

// RemoveStatusFromConversation removes a status from a conversation
func (r *ConversationRepository) RemoveStatusFromConversation(ctx context.Context, conversationID, statusID string) error {
	// Find and delete the message record
	var messages []models.ConversationMessage
	err := r.db.Model(&models.ConversationMessage{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION#%s", conversationID)).
		Scan(&messages)

	if err != nil {
		return fmt.Errorf("failed to query messages: %w", err)
	}

	for _, msg := range messages {
		if msg.StatusID == statusID {
			if err := r.db.Model(&msg).WithContext(ctx).Delete(); err != nil {
				return fmt.Errorf("failed to delete message: %w", err)
			}
			break
		}
	}

	return nil
}

// MarkStatusRead marks a specific status as read by a user
func (r *ConversationRepository) MarkStatusRead(ctx context.Context, conversationID, _, username string) error {
	// Mark the entire conversation as read (matching typical behavior)
	return r.MarkConversationRead(ctx, conversationID, username)
}

// GetUnreadStatusCount gets the count of unread statuses in a conversation for a user
func (r *ConversationRepository) GetUnreadStatusCount(ctx context.Context, conversationID, username string) (int, error) {
	// Check if the conversation is marked as read
	var status models.ConversationStatus
	err := r.db.Model(&models.ConversationStatus{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("CONVERSATION_STATUS#%s", conversationID)).
		Where("SK", "=", fmt.Sprintf("USER#%s", username)).
		First(&status)

	if errors.IsNotFound(err) || status.Unread {
		// If no status record or marked unread, count all messages after last read
		// For simplicity, returning 1 if unread (can be enhanced to count actual messages)
		return 1, nil
	}

	return 0, nil
}

// LeaveConversation removes a participant from a conversation
func (r *ConversationRepository) LeaveConversation(ctx context.Context, conversationID, username string) error {
	// This delegates to RemoveParticipant
	return r.RemoveParticipant(ctx, conversationID, username)
}

// AddParticipant adds a participant to a conversation
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

// GetConversationParticipants retrieves the list of participants in a conversation
func (r *ConversationRepository) GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error) {
	conv, err := r.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return conv.Participants, nil
}

// UpdateConversationLastStatus updates the last status in a conversation
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

// RemoveParticipant removes a participant from a conversation
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

// CreateConversationMute creates a new conversation mute
func (r *ConversationRepository) CreateConversationMute(ctx context.Context, mute *storage.ConversationMute) error {
	model := &models.ConversationMute{
		Username:       mute.Username,
		ConversationID: mute.ConversationID,
		CreatedAt:      mute.CreatedAt,
		ExpiresAt:      mute.ExpiresAt,
	}

	if err := model.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare mute: %w", err)
	}

	if err := r.db.Model(model).WithContext(ctx).Create(); err != nil {
		return fmt.Errorf("failed to create conversation mute: %w", err)
	}

	return nil
}

// DeleteConversationMute removes a conversation mute
func (r *ConversationRepository) DeleteConversationMute(ctx context.Context, username, conversationID string) error {
	err := r.db.Model(&models.ConversationMute{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("CONVERSATION_MUTE#%s", conversationID)).
		Delete()

	if err != nil {
		return fmt.Errorf("failed to delete conversation mute: %w", err)
	}

	return nil
}

// IsConversationMuted checks if a conversation is muted by a user
func (r *ConversationRepository) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	var mute models.ConversationMute
	err := r.db.Model(&models.ConversationMute{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("CONVERSATION_MUTE#%s", conversationID)).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check conversation mute: %w", err)
	}

	// Check if mute has expired
	if !mute.ExpiresAt.IsZero() && mute.ExpiresAt.Before(time.Now()) {
		// Delete expired mute
		_ = r.DeleteConversationMute(ctx, username, conversationID)
		return false, nil
	}

	return true, nil
}

// GetMutedConversations retrieves all muted conversations for a user
func (r *ConversationRepository) GetMutedConversations(ctx context.Context, username string) ([]string, error) {
	var mutes []models.ConversationMute
	err := r.db.Model(&models.ConversationMute{}).WithContext(ctx).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Scan(&mutes)

	if err != nil {
		return nil, fmt.Errorf("failed to query muted conversations: %w", err)
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

// MarkConversationUnread marks a conversation as unread for a user
func (r *ConversationRepository) MarkConversationUnread(ctx context.Context, conversationID, userID string) error {
	log := r.logger.With(
		zap.String("conversation_id", conversationID),
		zap.String("user_id", userID),
	)

	status := &models.ConversationStatus{
		ConversationID: conversationID,
		UserID:         userID,
		Unread:         true,
		LastReadAt:     time.Time{}, // Zero time for unread
	}

	if err := status.BeforeCreate(); err != nil {
		log.Error("failed to prepare status", zap.Error(err))
		return fmt.Errorf("failed to prepare status: %w", err)
	}

	err := r.db.Model(status).WithContext(ctx).Create()
	if err != nil {
		log.Error("failed to mark conversation as unread", zap.Error(err))
		return fmt.Errorf("failed to mark conversation as unread: %w", err)
	}

	return nil
}

// GetUnreadConversations retrieves unread conversations for a user
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
		err := r.db.WithContext(ctx).Model(&models.ConversationStatus{}).
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

// SearchConversations searches conversations for a user by query
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

// generateRandomString generates a random string of specified length
func (r *ConversationRepository) generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
