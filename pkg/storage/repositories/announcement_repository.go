package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// AnnouncementRepository handles announcement operations using DynamORM
type AnnouncementRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewAnnouncementRepository creates a new announcement repository
func NewAnnouncementRepository(db core.DB, tableName string, logger *zap.Logger) *AnnouncementRepository {
	return &AnnouncementRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Converter functions for storage <-> model types

func convertStorageReactionsToModel(reactions []storage.Reaction) []models.Reaction {
	if reactions == nil {
		return nil
	}
	result := make([]models.Reaction, len(reactions))
	for i, r := range reactions {
		result[i] = models.Reaction{
			Name:      r.Name,
			Count:     r.Count,
			Me:        r.Me,
			URL:       r.URL,
			StaticURL: r.StaticURL,
		}
	}
	return result
}

func convertModelReactionsToStorage(reactions []models.Reaction) []storage.Reaction {
	if reactions == nil {
		return nil
	}
	result := make([]storage.Reaction, len(reactions))
	for i, r := range reactions {
		result[i] = storage.Reaction{
			Name:      r.Name,
			Count:     r.Count,
			Me:        r.Me,
			URL:       r.URL,
			StaticURL: r.StaticURL,
		}
	}
	return result
}

func convertStorageEmojisToModel(emojis []storage.CustomEmoji) []models.CustomEmoji {
	if emojis == nil {
		return nil
	}
	result := make([]models.CustomEmoji, len(emojis))
	for i, e := range emojis {
		result[i] = models.CustomEmoji{
			Shortcode:       e.Shortcode,
			URL:             e.URL,
			StaticURL:       e.StaticURL,
			VisibleInPicker: e.VisibleInPicker,
			Category:        e.Category,
		}
	}
	return result
}

func convertModelEmojisToStorage(emojis []models.CustomEmoji) []storage.CustomEmoji {
	if emojis == nil {
		return nil
	}
	result := make([]storage.CustomEmoji, len(emojis))
	for i, e := range emojis {
		result[i] = storage.CustomEmoji{
			Shortcode:       e.Shortcode,
			URL:             e.URL,
			StaticURL:       e.StaticURL,
			VisibleInPicker: e.VisibleInPicker,
			Category:        e.Category,
		}
	}
	return result
}

func convertStorageMentionsToModel(mentions []storage.Mention) []models.Mention {
	if mentions == nil {
		return nil
	}
	result := make([]models.Mention, len(mentions))
	for i, m := range mentions {
		result[i] = models.Mention{
			ID:       m.ID,
			Username: m.Username,
			URL:      m.URL,
			Acct:     m.Acct,
		}
	}
	return result
}

func convertModelMentionsToStorage(mentions []models.Mention) []storage.Mention {
	if mentions == nil {
		return nil
	}
	result := make([]storage.Mention, len(mentions))
	for i, m := range mentions {
		result[i] = storage.Mention{
			ID:       m.ID,
			Username: m.Username,
			URL:      m.URL,
			Acct:     m.Acct,
		}
	}
	return result
}

// CreateAnnouncement creates a new announcement
func (r *AnnouncementRepository) CreateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	// Convert storage.Announcement to models.Announcement
	modelAnnouncement := &models.Announcement{
		ID:          announcement.ID,
		Content:     announcement.Content,
		Text:        announcement.Text,
		PublishedAt: announcement.PublishedAt,
		UpdatedAt:   announcement.UpdatedAt,
		AllDay:      announcement.AllDay,
		StartsAt:    announcement.StartsAt,
		EndsAt:      announcement.EndsAt,
		Reactions:   convertStorageReactionsToModel(announcement.Reactions),
		Tags:        announcement.Tags,
		Emojis:      convertStorageEmojisToModel(announcement.Emojis),
		Mentions:    convertStorageMentionsToModel(announcement.Mentions),
		CreatedBy:   announcement.CreatedBy,
	}

	if err := modelAnnouncement.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare announcement for creation: %w", err)
	}

	// Update the original announcement with generated ID and timestamps
	announcement.ID = modelAnnouncement.ID
	announcement.PublishedAt = modelAnnouncement.PublishedAt
	announcement.UpdatedAt = modelAnnouncement.UpdatedAt

	err := r.db.WithContext(ctx).Model(modelAnnouncement).Create()
	if err != nil {
		// Check for conditional check failed (already exists)
		// DynamORM doesn't have this specific check, so we'll skip it for now
		return fmt.Errorf("failed to create announcement: %w", err)
	}

	return nil
}

// GetAnnouncement retrieves a single announcement by ID
func (r *AnnouncementRepository) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	var modelAnnouncement models.Announcement
	err := r.db.WithContext(ctx).Model(&models.Announcement{}).
		Where("PK", "=", fmt.Sprintf("ANNOUNCEMENT#%s", id)).
		Where("SK", "=", "ANNOUNCEMENT").
		First(&modelAnnouncement)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get announcement: %w", err)
	}

	// Convert models.Announcement to storage.Announcement
	announcement := &storage.Announcement{
		ID:          modelAnnouncement.ID,
		Content:     modelAnnouncement.Content,
		Text:        modelAnnouncement.Text,
		PublishedAt: modelAnnouncement.PublishedAt,
		UpdatedAt:   modelAnnouncement.UpdatedAt,
		AllDay:      modelAnnouncement.AllDay,
		StartsAt:    modelAnnouncement.StartsAt,
		EndsAt:      modelAnnouncement.EndsAt,
		Reactions:   convertModelReactionsToStorage(modelAnnouncement.Reactions),
		Tags:        modelAnnouncement.Tags,
		Emojis:      convertModelEmojisToStorage(modelAnnouncement.Emojis),
		Mentions:    convertModelMentionsToStorage(modelAnnouncement.Mentions),
		CreatedBy:   modelAnnouncement.CreatedBy,
	}

	return announcement, nil
}

// GetAnnouncements retrieves all announcements (for backward compatibility)
func (r *AnnouncementRepository) GetAnnouncements(ctx context.Context, active bool) ([]*storage.Announcement, error) {
	// Use paginated version with reasonable default limit
	announcements, _, err := r.GetAnnouncementsPaginated(ctx, active, 100, "")
	return announcements, err
}

// GetAnnouncementsPaginated retrieves announcements with pagination
func (r *AnnouncementRepository) GetAnnouncementsPaginated(ctx context.Context, active bool, limit int, cursor string) ([]*storage.Announcement, string, error) {
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	// Note: In a real implementation, we might want to use a GSI for efficient querying
	// For now, we'll scan with a filter expression matching the legacy behavior
	query := r.db.WithContext(ctx).Model(&models.Announcement{})

	// Handle cursor-based pagination
	if cursor != "" {
		// Parse cursor to get the last announcement ID
		query = query.Where("PK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var modelAnnouncements []*models.Announcement
	err := query.All(&modelAnnouncements)
	
	if err != nil {
		if errors.IsNotFound(err) {
			// Return empty slice when no announcements found
			return []*storage.Announcement{}, "", nil
		}
		return nil, "", fmt.Errorf("failed to scan announcements: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	hasMore := len(modelAnnouncements) > limit
	if hasMore {
		// We got more results than requested, so there are more pages
		nextCursor = modelAnnouncements[limit-1].PK
		// Trim results to requested limit
		modelAnnouncements = modelAnnouncements[:limit]
	}

	announcements := make([]*storage.Announcement, 0, len(modelAnnouncements))
	now := time.Now()

	for _, model := range modelAnnouncements {
		announcement := &storage.Announcement{
			ID:          model.ID,
			Content:     model.Content,
			Text:        model.Text,
			PublishedAt: model.PublishedAt,
			UpdatedAt:   model.UpdatedAt,
			AllDay:      model.AllDay,
			StartsAt:    model.StartsAt,
			EndsAt:      model.EndsAt,
			Reactions:   convertModelReactionsToStorage(model.Reactions),
			Tags:        model.Tags,
			Emojis:      convertModelEmojisToStorage(model.Emojis),
			Mentions:    convertModelMentionsToStorage(model.Mentions),
			CreatedBy:   model.CreatedBy,
		}

		// Filter active announcements if requested
		if active {
			// Skip if not yet started
			if announcement.StartsAt != nil && announcement.StartsAt.After(now) {
				continue
			}
			// Skip if already ended
			if announcement.EndsAt != nil && announcement.EndsAt.Before(now) {
				continue
			}
		}

		announcements = append(announcements, announcement)
	}

	return announcements, nextCursor, nil
}

// UpdateAnnouncement updates an existing announcement
func (r *AnnouncementRepository) UpdateAnnouncement(ctx context.Context, announcement *storage.Announcement) error {
	announcement.UpdatedAt = time.Now()

	// Convert to model
	modelAnnouncement := &models.Announcement{
		ID:          announcement.ID,
		Content:     announcement.Content,
		Text:        announcement.Text,
		PublishedAt: announcement.PublishedAt,
		UpdatedAt:   announcement.UpdatedAt,
		AllDay:      announcement.AllDay,
		StartsAt:    announcement.StartsAt,
		EndsAt:      announcement.EndsAt,
		Reactions:   convertStorageReactionsToModel(announcement.Reactions),
		Tags:        announcement.Tags,
		Emojis:      convertStorageEmojisToModel(announcement.Emojis),
		Mentions:    convertStorageMentionsToModel(announcement.Mentions),
		CreatedBy:   announcement.CreatedBy,
	}

	modelAnnouncement.UpdateKeys()

	// DynamORM doesn't have Condition method, so we'll use regular Update
	err := r.db.WithContext(ctx).Model(modelAnnouncement).Update()
	
	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to update announcement: %w", err)
	}

	return nil
}

// DeleteAnnouncement deletes an announcement
func (r *AnnouncementRepository) DeleteAnnouncement(ctx context.Context, id string) error {
	// Delete the announcement
	modelAnnouncement := &models.Announcement{ID: id}
	modelAnnouncement.UpdateKeys()

	err := r.db.WithContext(ctx).Model(modelAnnouncement).Delete()
	
	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to delete announcement: %w", err)
	}

	// Clean up related dismissals and reactions
	// Note: These are best-effort cleanups - we don't fail the deletion if cleanup fails

	// Clean up reactions
	var reactions []*models.AnnouncementReaction
	err = r.db.WithContext(ctx).Model(&models.AnnouncementReaction{}).
		Where("PK", "=", fmt.Sprintf("ANNOUNCEMENT_REACTION#%s", id)).
		All(&reactions)
	
	if err != nil && !errors.IsNotFound(err) {
		r.logger.Warn("failed to query reactions for cleanup",
			zap.String("announcement_id", id),
			zap.Error(err))
	} else {
		// Delete each reaction
		for _, reaction := range reactions {
			if delErr := r.db.WithContext(ctx).Model(reaction).Delete(); delErr != nil {
				r.logger.Warn("failed to delete reaction during cleanup",
					zap.String("announcement_id", id),
					zap.Error(delErr))
			}
		}
	}

	// Clean up dismissals
	// Since dismissals are stored under user keys, we need to query them differently
	// Note: This is inefficient without a proper GSI
	var dismissals []*models.AnnouncementDismissal
	err = r.db.WithContext(ctx).Model(&models.AnnouncementDismissal{}).
		All(&dismissals)
	
	if err != nil && !errors.IsNotFound(err) {
		r.logger.Warn("failed to scan dismissals for cleanup",
			zap.String("announcement_id", id),
			zap.Error(err))
	} else {
		// Delete each dismissal that matches this announcement
		for _, dismissal := range dismissals {
			if dismissal.AnnouncementID == id {
				if delErr := r.db.WithContext(ctx).Model(dismissal).Delete(); delErr != nil {
					r.logger.Warn("failed to delete dismissal during cleanup",
						zap.String("announcement_id", id),
						zap.Error(delErr))
				}
			}
		}
	}

	return nil
}

// DismissAnnouncement marks an announcement as dismissed by a user
func (r *AnnouncementRepository) DismissAnnouncement(ctx context.Context, username, announcementID string) error {
	dismissal := &models.AnnouncementDismissal{
		Username:       username,
		AnnouncementID: announcementID,
	}

	if err := dismissal.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare dismissal for creation: %w", err)
	}

	err := r.db.WithContext(ctx).Model(dismissal).Create()
	if err != nil {
		return fmt.Errorf("failed to dismiss announcement: %w", err)
	}

	return nil
}

// IsDismissed checks if a user has dismissed an announcement
func (r *AnnouncementRepository) IsDismissed(ctx context.Context, username, announcementID string) (bool, error) {
	var dismissal models.AnnouncementDismissal
	err := r.db.WithContext(ctx).Model(&models.AnnouncementDismissal{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("ANNOUNCEMENT_DISMISSED#%s", announcementID)).
		First(&dismissal)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check dismissal: %w", err)
	}

	return true, nil
}

// GetDismissedAnnouncements gets all announcement IDs dismissed by a user
func (r *AnnouncementRepository) GetDismissedAnnouncements(ctx context.Context, username string) ([]string, error) {
	var dismissals []*models.AnnouncementDismissal
	err := r.db.WithContext(ctx).Model(&models.AnnouncementDismissal{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "begins_with", "ANNOUNCEMENT_DISMISSED#").
		All(&dismissals)
	
	if err != nil {
		if errors.IsNotFound(err) {
			// Return empty slice when no dismissals found
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to query dismissed announcements: %w", err)
	}

	announcementIDs := make([]string, len(dismissals))
	for i, dismissal := range dismissals {
		announcementIDs[i] = dismissal.AnnouncementID
	}

	return announcementIDs, nil
}

// AddAnnouncementReaction adds a user's reaction to an announcement
func (r *AnnouncementRepository) AddAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	reaction := &models.AnnouncementReaction{
		Username:       username,
		AnnouncementID: announcementID,
		EmojiName:      emojiName,
	}

	if err := reaction.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare reaction for creation: %w", err)
	}

	err := r.db.WithContext(ctx).Model(reaction).Create()
	
	if err != nil {
		// If it already exists, that's fine - matches legacy behavior
		return nil
	}

	return nil
}

// RemoveAnnouncementReaction removes a user's reaction from an announcement
func (r *AnnouncementRepository) RemoveAnnouncementReaction(ctx context.Context, username, announcementID, emojiName string) error {
	reaction := &models.AnnouncementReaction{
		Username:       username,
		AnnouncementID: announcementID,
		EmojiName:      emojiName,
	}
	reaction.UpdateKeys()

	err := r.db.WithContext(ctx).Model(reaction).Delete()
	if err != nil {
		if errors.IsNotFound(err) {
			// Reaction not found - not an error
			return nil
		}
		return fmt.Errorf("failed to remove reaction: %w", err)
	}

	return nil
}

// GetAnnouncementReactions gets all reactions for an announcement
func (r *AnnouncementRepository) GetAnnouncementReactions(ctx context.Context, announcementID string) (map[string][]string, error) {
	var reactions []*models.AnnouncementReaction
	err := r.db.WithContext(ctx).Model(&models.AnnouncementReaction{}).
		Where("PK", "=", fmt.Sprintf("ANNOUNCEMENT_REACTION#%s", announcementID)).
		All(&reactions)
	
	if err != nil {
		if errors.IsNotFound(err) {
			// Return empty map when no reactions found
			return make(map[string][]string), nil
		}
		return nil, fmt.Errorf("failed to query reactions: %w", err)
	}

	// Organize reactions by emoji name
	reactionMap := make(map[string][]string)
	for _, reaction := range reactions {
		if _, exists := reactionMap[reaction.EmojiName]; !exists {
			reactionMap[reaction.EmojiName] = make([]string, 0)
		}
		reactionMap[reaction.EmojiName] = append(reactionMap[reaction.EmojiName], reaction.Username)
	}

	return reactionMap, nil
}