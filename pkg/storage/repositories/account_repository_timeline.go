package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// ===== Timeline Operations =====
// This file contains timeline-related methods for the AccountRepository.
//
// The six legacy timeline READS (GetHomeTimeline, GetLocalTimeline,
// GetPublicTimeline, GetHashtagTimeline, GetListTimeline, GetConversations)
// were structurally dead and are deleted (issue #1506): their key shapes never
// matched the writer. models.TimelineEntry.UpdateKeys writes PK
// "TIMELINE#{type}#{id}" / SK "{ts}#{entryID}" (only GSI1 exists, for PUBLIC),
// while those reads keyed "USER#{u}"/"HOME#..." and gsi2/gsi3/gsi4 attributes
// the model does not declare — they could never return rows. The live
// TimelineRepository family (models.Timeline) is the serving timeline path.

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
		return ErrorHandler.HandleCreateError(err, EntityTimelineEntry, entry.PostID)
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
		return ErrorHandler.HandleDeleteError(err, EntityTimelineEntry, objectID)
	}

	return nil
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
		return ErrorHandler.HandleCreateError(err, EntityConversation, conversationID)
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
		return ErrorHandler.HandleDeleteError(err, EntityConversation, conversationID)
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
		return false, ErrorHandler.HandleGetError(err, EntityConversation, conversationID)
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
			return nil, ErrorHandler.HandleGetError(err, "timeline marker", timeline)
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
		return ErrorHandler.HandleUpdateError(err, "timeline marker", timeline)
	}

	return nil
}
