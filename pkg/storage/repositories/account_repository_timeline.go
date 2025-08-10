package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ===== Timeline Operations =====
// This file contains timeline-related methods for the AccountRepository

// GetHomeTimeline retrieves the home timeline for a user
//
//nolint:dupl // Timeline query patterns are similar by design
func (r *AccountRepository) GetHomeTimeline(ctx context.Context, username string, limit int, maxID, sinceID string) ([]*storage.TimelineEntry, error) {
	var entries []models.TimelineEntry

	// Build query
	query := r.db.WithContext(ctx).Model(&models.TimelineEntry{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "BEGINS_WITH", "HOME#").
		Limit(limit)

	// Apply pagination filters
	if maxID != "" {
		query = query.Where("SK", "<", fmt.Sprintf("HOME#%s", maxID))
	}
	if sinceID != "" {
		query = query.Where("SK", ">", fmt.Sprintf("HOME#%s", sinceID))
	}

	// Execute query
	err := query.All(&entries)
	if err != nil {
		r.logger.Error("failed to get home timeline",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get home timeline: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.TimelineEntry, len(entries))
	for i, entry := range entries {
		result[i] = r.modelToTimelineEntry(&entry)
	}

	return result, nil
}

// GetLocalTimeline retrieves the local public timeline
func (r *AccountRepository) GetLocalTimeline(ctx context.Context, limit int, maxID, sinceID string) ([]*storage.TimelineEntry, error) {
	var entries []models.TimelineEntry

	// Build query using GSI for local timeline
	query := r.db.WithContext(ctx).Model(&models.TimelineEntry{}).
		Index("local-timeline-index").
		Where("GSI1PK", "=", "LOCAL_TIMELINE").
		Limit(limit)

	// Apply pagination filters
	if maxID != "" {
		query = query.Where("GSI1SK", "<", maxID)
	}
	if sinceID != "" {
		query = query.Where("GSI1SK", ">", sinceID)
	}

	// Execute query
	err := query.All(&entries)
	if err != nil {
		r.logger.Error("failed to get local timeline", zap.Error(err))
		return nil, fmt.Errorf("failed to get local timeline: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.TimelineEntry, len(entries))
	for i, entry := range entries {
		result[i] = r.modelToTimelineEntry(&entry)
	}

	return result, nil
}

// GetPublicTimeline retrieves the federated public timeline
func (r *AccountRepository) GetPublicTimeline(ctx context.Context, limit int, maxID, sinceID string, onlyMedia bool) ([]*storage.TimelineEntry, error) {
	var entries []models.TimelineEntry

	// Choose index based on media filter
	indexName := "public-timeline-index"
	gsiPK := "PUBLIC_TIMELINE"
	if onlyMedia {
		indexName = "media-timeline-index"
		gsiPK = "MEDIA_TIMELINE"
	}

	// Build query
	query := r.db.WithContext(ctx).Model(&models.TimelineEntry{}).
		Index(indexName).
		Where("GSI2PK", "=", gsiPK).
		Limit(limit)

	// Apply pagination filters
	if maxID != "" {
		query = query.Where("GSI2SK", "<", maxID)
	}
	if sinceID != "" {
		query = query.Where("GSI2SK", ">", sinceID)
	}

	// Execute query
	err := query.All(&entries)
	if err != nil {
		r.logger.Error("failed to get public timeline",
			zap.Bool("onlyMedia", onlyMedia),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get public timeline: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.TimelineEntry, len(entries))
	for i, entry := range entries {
		result[i] = r.modelToTimelineEntry(&entry)
	}

	return result, nil
}

// GetHashtagTimeline retrieves timeline entries for a hashtag
func (r *AccountRepository) GetHashtagTimeline(ctx context.Context, hashtag string, limit int, maxID, sinceID string) ([]*storage.TimelineEntry, error) {
	var entries []models.TimelineEntry

	// Build query using hashtag index
	query := r.db.WithContext(ctx).Model(&models.TimelineEntry{}).
		Index("hashtag-timeline-index").
		Where("GSI3PK", "=", fmt.Sprintf("HASHTAG#%s", hashtag)).
		Limit(limit)

	// Apply pagination filters
	if maxID != "" {
		query = query.Where("GSI3SK", "<", maxID)
	}
	if sinceID != "" {
		query = query.Where("GSI3SK", ">", sinceID)
	}

	// Execute query
	err := query.All(&entries)
	if err != nil {
		r.logger.Error("failed to get hashtag timeline",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get hashtag timeline: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.TimelineEntry, len(entries))
	for i, entry := range entries {
		result[i] = r.modelToTimelineEntry(&entry)
	}

	return result, nil
}

// GetListTimeline retrieves timeline entries for a list
func (r *AccountRepository) GetListTimeline(ctx context.Context, username, listID string, limit int, maxID, sinceID string) ([]*storage.TimelineEntry, error) {
	// First verify the user owns the list
	list, err := r.getList(ctx, username, listID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return nil, common.ListNotFoundError{ID: listID}
	}

	var entries []models.TimelineEntry

	// Build query using list timeline index
	query := r.db.WithContext(ctx).Model(&models.TimelineEntry{}).
		Index("list-timeline-index").
		Where("GSI4PK", "=", fmt.Sprintf("LIST#%s", listID)).
		Limit(limit)

	// Apply pagination filters
	if maxID != "" {
		query = query.Where("GSI4SK", "<", maxID)
	}
	if sinceID != "" {
		query = query.Where("GSI4SK", ">", sinceID)
	}

	// Execute query
	err = query.All(&entries)
	if err != nil {
		r.logger.Error("failed to get list timeline",
			zap.String("listID", listID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get list timeline: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.TimelineEntry, len(entries))
	for i, entry := range entries {
		result[i] = r.modelToTimelineEntry(&entry)
	}

	return result, nil
}

// AddToTimeline adds an entry to a user's home timeline
func (r *AccountRepository) AddToTimeline(ctx context.Context, username string, entry *storage.TimelineEntry) error {
	// Convert to model
	model := &models.TimelineEntry{
		TimelineType: "HOME",
		TimelineID:   username,
		PostID:       entry.PostID,
		ActorID:      entry.ActorID,
		ActorHandle:  entry.ActorHandle,
		Content:      entry.Content,
		ContentType:  entry.ContentType,
		HasMedia:     entry.HasMedia,
		IsReply:      entry.IsReply,
		InReplyTo:    entry.InReplyTo,
		IsBoost:      entry.IsBoost,
		BoostedBy:    entry.BoostedBy,
		Visibility:   entry.Visibility,
		Language:     entry.Language,
		Sensitive:    entry.Sensitive,
		SpoilerText:  entry.SpoilerText,
		CreatedAt:    entry.CreatedAt,
		TimelineAt:   entry.TimelineAt,
		ExpiresAt: func() time.Time {
			if entry.ExpiresAt != nil {
				return *entry.ExpiresAt
			}
			return time.Time{}
		}(),
	}

	// Create timeline entry
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to add to timeline",
			zap.String("username", username),
			zap.String("postID", entry.PostID),
			zap.Error(err))
		return fmt.Errorf("failed to add to timeline: %w", err)
	}

	return nil
}

// RemoveFromTimeline removes an entry from a user's timeline
func (r *AccountRepository) RemoveFromTimeline(ctx context.Context, username, objectID string) error {
	err := r.db.WithContext(ctx).Model(&models.TimelineEntry{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("HOME#%s", objectID)).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to remove from timeline",
			zap.String("username", username),
			zap.String("objectID", objectID),
			zap.Error(err))
		return fmt.Errorf("failed to remove from timeline: %w", err)
	}

	return nil
}

// GetConversations retrieves conversations for a user
//
//nolint:dupl // Timeline query patterns are similar by design
func (r *AccountRepository) GetConversations(ctx context.Context, username string, limit int, maxID, sinceID string) ([]*storage.Conversation, error) {
	var conversations []models.Conversation

	// Build query
	query := r.db.WithContext(ctx).Model(&models.Conversation{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "BEGINS_WITH", "CONVERSATION#").
		Limit(limit)

	// Apply pagination filters
	if maxID != "" {
		query = query.Where("SK", "<", fmt.Sprintf("CONVERSATION#%s", maxID))
	}
	if sinceID != "" {
		query = query.Where("SK", ">", fmt.Sprintf("CONVERSATION#%s", sinceID))
	}

	// Execute query
	err := query.All(&conversations)
	if err != nil {
		r.logger.Error("failed to get conversations",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get conversations: %w", err)
	}

	// Convert to storage type
	result := make([]*storage.Conversation, len(conversations))
	for i, conv := range conversations {
		result[i] = r.modelToConversation(&conv)
	}

	return result, nil
}

// MuteConversation mutes a conversation for a user
func (r *AccountRepository) MuteConversation(ctx context.Context, username, conversationID string) error {
	mute := &models.ConversationMute{
		Username:       username,
		ConversationID: conversationID,
		CreatedAt:      time.Now(),
	}

	err := r.db.WithContext(ctx).Model(mute).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			// Already muted
			return nil
		}
		r.logger.Error("failed to mute conversation",
			zap.String("username", username),
			zap.String("conversationID", conversationID),
			zap.Error(err))
		return fmt.Errorf("failed to mute conversation: %w", err)
	}

	return nil
}

// UnmuteConversation unmutes a conversation for a user
func (r *AccountRepository) UnmuteConversation(ctx context.Context, username, conversationID string) error {
	err := r.db.WithContext(ctx).Model(&models.ConversationMute{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("CONVERSATION_MUTE#%s", conversationID)).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to unmute conversation",
			zap.String("username", username),
			zap.String("conversationID", conversationID),
			zap.Error(err))
		return fmt.Errorf("failed to unmute conversation: %w", err)
	}

	return nil
}

// IsConversationMuted checks if a conversation is muted
func (r *AccountRepository) IsConversationMuted(ctx context.Context, username, conversationID string) (bool, error) {
	var mute models.ConversationMute

	err := r.db.WithContext(ctx).Model(&mute).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("CONVERSATION_MUTE#%s", conversationID)).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check conversation mute",
			zap.String("username", username),
			zap.String("conversationID", conversationID),
			zap.Error(err))
		return false, fmt.Errorf("failed to check conversation mute: %w", err)
	}

	return true, nil
}

// GetTimelineMarkers retrieves timeline position markers for a user
func (r *AccountRepository) GetTimelineMarkers(ctx context.Context, username string, timelines []string) (map[string]*storage.Marker, error) {
	markers := make(map[string]*storage.Marker)

	for _, timeline := range timelines {
		var marker models.TimelineMarker

		err := r.db.WithContext(ctx).Model(&marker).
			Where("PK", "=", fmt.Sprintf("USER#%s", username)).
			Where("SK", "=", fmt.Sprintf("MARKER#%s", timeline)).
			First(&marker)

		if err != nil {
			if errors.IsNotFound(err) {
				// No marker for this timeline
				continue
			}
			r.logger.Error("failed to get timeline marker",
				zap.String("username", username),
				zap.String("timeline", timeline),
				zap.Error(err))
			return nil, fmt.Errorf("failed to get timeline marker: %w", err)
		}

		markers[timeline] = &storage.Marker{
			LastReadID: marker.LastReadID,
			UpdatedAt:  marker.UpdatedAt,
		}
	}

	return markers, nil
}

// UpdateTimelineMarker updates the position marker for a timeline
func (r *AccountRepository) UpdateTimelineMarker(ctx context.Context, username, timeline, lastReadID string) error {
	marker := &models.TimelineMarker{
		Username:   username,
		Timeline:   timeline,
		LastReadID: lastReadID,
		UpdatedAt:  time.Now(),
	}

	// Use upsert pattern
	// Get existing marker first
	err := r.db.WithContext(ctx).Model(marker).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("MARKER#%s", timeline)).
		First(marker)

	if err == nil {
		// Update existing marker
		marker.LastReadID = lastReadID
		marker.UpdatedAt = time.Now()
		err = r.db.WithContext(ctx).Model(marker).Update()
	}

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new marker
			return r.db.WithContext(ctx).Model(marker).Create()
		}
		r.logger.Error("failed to update timeline marker",
			zap.String("username", username),
			zap.String("timeline", timeline),
			zap.Error(err))
		return fmt.Errorf("failed to update timeline marker: %w", err)
	}

	return nil
}

// Helper methods

// modelToTimelineEntry converts a timeline entry model to storage type
func (r *AccountRepository) modelToTimelineEntry(model *models.TimelineEntry) *storage.TimelineEntry {
	return &storage.TimelineEntry{
		TimelineType: model.TimelineType,
		TimelineID:   model.TimelineID,
		EntryID:      model.EntryID,
		PostID:       model.PostID,
		ActorID:      model.ActorID,
		ActorHandle:  model.ActorHandle,
		Content:      model.Content,
		ContentType:  model.ContentType,
		HasMedia:     model.HasMedia,
		IsReply:      model.IsReply,
		InReplyTo:    model.InReplyTo,
		IsBoost:      model.IsBoost,
		BoostedBy:    model.BoostedBy,
		Visibility:   model.Visibility,
		Language:     model.Language,
		Sensitive:    model.Sensitive,
		SpoilerText:  model.SpoilerText,
		CreatedAt:    model.CreatedAt,
		TimelineAt:   model.TimelineAt,
		ExpiresAt: func() *time.Time {
			if !model.ExpiresAt.IsZero() {
				return &model.ExpiresAt
			}
			return nil
		}(),
	}
}

// modelToConversation converts a conversation model to storage type
func (r *AccountRepository) modelToConversation(model *models.Conversation) *storage.Conversation {
	return &storage.Conversation{
		ID:           model.ID,
		Participants: model.Participants,
		LastStatusID: model.LastStatusID,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}

// getList retrieves a list (helper for list timeline)
func (r *AccountRepository) getList(ctx context.Context, username, listID string) (*models.List, error) {
	var list models.List

	err := r.db.WithContext(ctx).Model(&list).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("LIST#%s", listID)).
		First(&list)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get list: %w", err)
	}

	return &list, nil
}
