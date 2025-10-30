package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// AnnouncementRepository handles announcement operations using enhanced DynamORM patterns
type AnnouncementRepository struct {
	// Use EnhancedBaseRepository for Announcement model
	*EnhancedBaseRepository[*models.Announcement]
	db        core.DB
	logger    *zap.Logger
	tableName string
}

// NewAnnouncementRepository creates a new announcement repository with enhanced functionality
func NewAnnouncementRepository(db core.DB, tableName string, logger *zap.Logger) *AnnouncementRepository {
	// Create enhanced repository optimized for announcement operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Announcement](db, tableName, logger, nil, "AnnouncementRepository", "announcement")

	// Set up enhanced services for announcement operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Admin-only operations
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Announcements cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())           // Important for admin notifications

	return &AnnouncementRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
		logger:                 logger,
		tableName:              tableName,
	}
}

// NewAnnouncementRepositoryWithCostTracking creates a new announcement repository with cost tracking
func NewAnnouncementRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *AnnouncementRepository {
	// Create enhanced repository with cost tracking
	enhancedRepo := NewEnhancedBaseRepository[*models.Announcement](db, tableName, logger, costService, "AnnouncementRepository", "announcement")

	// Set up enhanced services for announcement operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Admin-only operations
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Announcements cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())           // Important for admin notifications

	return &AnnouncementRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
		logger:                 logger,
		tableName:              tableName,
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
		return ErrorHandler.HandleCreateError(err, "announcement", announcement.ID)
	}

	// Update the original announcement with generated ID and timestamps
	announcement.ID = modelAnnouncement.ID
	announcement.PublishedAt = modelAnnouncement.PublishedAt
	announcement.UpdatedAt = modelAnnouncement.UpdatedAt

	// Use enhanced validation and creation with automatic permission checking and event emission
	err := r.ValidateAndCreate(ctx, modelAnnouncement)
	if err != nil {
		r.logger.Error("failed to create announcement with enhanced validation",
			zap.String("announcement_id", announcement.ID),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "announcement", announcement.ID)
	}

	r.logger.Info("created announcement with enhanced patterns",
		zap.String("announcement_id", announcement.ID),
		zap.String("created_by", announcement.CreatedBy))

	return nil
}

// GetAnnouncement retrieves a single announcement by ID
func (r *AnnouncementRepository) GetAnnouncement(ctx context.Context, id string) (*storage.Announcement, error) {
	var modelAnnouncement models.Announcement
	pk := fmt.Sprintf("ANNOUNCEMENT#%s", id)
	sk := "ANNOUNCEMENT"

	// Use BaseRepository Get method for cost tracking
	err := r.Get(ctx, pk, sk, &modelAnnouncement)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, ErrorHandler.HandleGetError(err, "announcement", id)
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

// GetAnnouncementsPaginated retrieves announcements with pagination using optimized GSI queries
func (r *AnnouncementRepository) GetAnnouncementsPaginated(ctx context.Context, active bool, limit int, cursor string) ([]*storage.Announcement, string, error) {
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	// Use GSI1 for efficient status-based queries with chronological ordering
	status := "active"
	if !active {
		status = "inactive"
	}

	query := r.db.WithContext(ctx).Model(&models.Announcement{}).
		Index("status-date-index").
		Where("GSI1PK", "=", fmt.Sprintf("ANNOUNCEMENT#%s", status)).
		OrderBy("GSI1SK", "ASC") // ASC because we use reverse timestamps (newest first)

	// Resume results from the provided cursor position when present
	if cursor != "" {
		query = query.Where("GSI1SK", ">", cursor)
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
		return nil, "", ErrorHandler.HandleQueryError(err, "announcement", "status query")
	}

	// Generate next cursor
	var nextCursor string
	hasMore := len(modelAnnouncements) > limit
	if hasMore {
		// We got more results than requested, so there are more pages
		nextCursor = modelAnnouncements[limit-1].GSI1SK
		// Trim results to requested limit
		modelAnnouncements = modelAnnouncements[:limit]
	}

	announcements := make([]*storage.Announcement, 0, len(modelAnnouncements))

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

		announcements = append(announcements, announcement)
	}

	r.logger.Debug("retrieved announcements using GSI1",
		zap.String("status", status),
		zap.Int("returned_count", len(announcements)),
		zap.Bool("has_more", hasMore),
		zap.String("cursor", cursor),
		zap.String("next_cursor", nextCursor),
	)

	return announcements, nextCursor, nil
}

// GetAnnouncementsByAdmin retrieves announcements created by a specific admin using GSI2
func (r *AnnouncementRepository) GetAnnouncementsByAdmin(ctx context.Context, adminUsername string, limit int, cursor string) ([]*storage.Announcement, string, error) {
	if limit <= 0 {
		limit = 20 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	// Use GSI2 for efficient admin-based queries
	query := r.db.WithContext(ctx).Model(&models.Announcement{}).
		Index("admin-index").
		Where("GSI2PK", "=", "ADMIN#"+adminUsername).
		OrderBy("GSI2SK", "DESC") // Most recent first

	// Resume results for the admin index when a cursor is provided
	if cursor != "" {
		query = query.Where("GSI2SK", "<", cursor)
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
		return nil, "", ErrorHandler.HandleQueryError(err, "announcement", "admin query")
	}

	// Generate next cursor
	var nextCursor string
	hasMore := len(modelAnnouncements) > limit
	if hasMore {
		// We got more results than requested, so there are more pages
		nextCursor = modelAnnouncements[limit-1].GSI2SK
		// Trim results to requested limit
		modelAnnouncements = modelAnnouncements[:limit]
	}

	announcements := make([]*storage.Announcement, 0, len(modelAnnouncements))

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

		announcements = append(announcements, announcement)
	}

	r.logger.Debug("retrieved announcements by admin using GSI2",
		zap.String("admin", adminUsername),
		zap.Int("returned_count", len(announcements)),
		zap.Bool("has_more", hasMore),
		zap.String("cursor", cursor),
		zap.String("next_cursor", nextCursor),
	)

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

	if err := modelAnnouncement.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "announcement", announcement.ID)
	}

	// Use enhanced validation and update with automatic permission checking and event emission
	err := r.ValidateAndUpdate(ctx, modelAnnouncement)
	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		r.logger.Error("failed to update announcement with enhanced validation",
			zap.String("announcement_id", announcement.ID),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "announcement", announcement.ID)
	}

	r.logger.Info("updated announcement with enhanced patterns",
		zap.String("announcement_id", announcement.ID))

	return nil
}

// DeleteAnnouncement deletes an announcement
func (r *AnnouncementRepository) DeleteAnnouncement(ctx context.Context, id string) error {
	// Delete the announcement using BaseRepository
	pk := fmt.Sprintf("ANNOUNCEMENT#%s", id)
	sk := "ANNOUNCEMENT"

	// Use BaseRepository Delete method for cost tracking
	err := r.Delete(ctx, pk, sk)
	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		return ErrorHandler.HandleDeleteError(err, "announcement", id)
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
		return ErrorHandler.HandleCreateError(err, "announcement dismissal", announcementID)
	}

	err := r.db.WithContext(ctx).Model(dismissal).Create()
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "announcement dismissal", announcementID)
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
		return false, ErrorHandler.HandleGetError(err, "announcement dismissal", announcementID)
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
		return nil, ErrorHandler.HandleQueryError(err, "announcement dismissal", "user dismissed")
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
		return ErrorHandler.HandleCreateError(err, "announcement reaction", emojiName)
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
	if err := reaction.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "announcement reaction", emojiName)
	}

	err := r.db.WithContext(ctx).Model(reaction).Delete()
	if err != nil {
		if errors.IsNotFound(err) {
			// Reaction not found - not an error
			return nil
		}
		return ErrorHandler.HandleDeleteError(err, "announcement reaction", emojiName)
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
		return nil, ErrorHandler.HandleQueryError(err, "announcement reaction", "reactions query")
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
