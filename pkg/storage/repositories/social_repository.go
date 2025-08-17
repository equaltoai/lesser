package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// SocialRepository handles all social interaction operations
type SocialRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewSocialRepository creates a new social repository
func NewSocialRepository(db core.DB, logger *zap.Logger) *SocialRepository {
	return &SocialRepository{
		db:     db,
		logger: logger,
	}
}

// ================== Block Methods ==================

// CreateBlock creates a new block relationship
func (r *SocialRepository) CreateBlock(ctx context.Context, block *storage.Block) error {
	// Convert to model
	model := &models.Block{
		Actor:     block.Actor,
		Object:    block.Object,
		ID:        block.ID,
		Published: block.Published,
		CreatedAt: block.CreatedAt,
	}

	// BeforeCreate will set timestamps and keys
	if err := model.BeforeCreate(); err != nil {
		return err
	}

	// Create the block
	err := r.db.WithContext(ctx).Model(model).Create()

	if err != nil {
		if errors.IsConditionFailed(err) {
			r.logger.Info("block already exists",
				zap.String("actor", block.Actor),
				zap.String("blocked", block.Object))
			return fmt.Errorf("block already exists")
		}
		return fmt.Errorf("failed to create block: %w", err)
	}

	r.logger.Info("block created",
		zap.String("actor", block.Actor),
		zap.String("blocked", block.Object))

	return nil
}

// DeleteBlock removes a block relationship
func (r *SocialRepository) DeleteBlock(ctx context.Context, actor, blockedActor string) error {
	// Extract usernames to match key pattern
	blockerUsername := extractUsername(actor)
	blockedUsername := extractUsername(blockedActor)

	err := r.db.WithContext(ctx).Model(&models.Block{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)).
		Where("SK", "=", fmt.Sprintf("BLOCKED#%s", blockedUsername)).
		Delete()

	if err != nil {
		return fmt.Errorf("failed to delete block: %w", err)
	}

	return nil
}

// GetBlock retrieves a specific block relationship
func (r *SocialRepository) GetBlock(ctx context.Context, actor, blockedActor string) (*storage.Block, error) {
	// Extract usernames to match key pattern
	blockerUsername := extractUsername(actor)
	blockedUsername := extractUsername(blockedActor)

	var block models.Block
	err := r.db.WithContext(ctx).Model(&models.Block{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)).
		Where("SK", "=", fmt.Sprintf("BLOCKED#%s", blockedUsername)).
		First(&block)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("block not found")
		}
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return &storage.Block{
		Actor:     block.Actor,
		Object:    block.Object,
		ID:        block.ID,
		Published: block.Published,
		CreatedAt: block.CreatedAt,
	}, nil
}

// IsBlocked checks if targetActor is blocked by actor
func (r *SocialRepository) IsBlocked(ctx context.Context, actor, targetActor string) (bool, error) {
	// Extract usernames to match key pattern
	blockerUsername := extractUsername(actor)
	blockedUsername := extractUsername(targetActor)

	var block models.Block
	err := r.db.WithContext(ctx).Model(&models.Block{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)).
		Where("SK", "=", fmt.Sprintf("BLOCKED#%s", blockedUsername)).
		First(&block)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// GetBlockedUsers returns a paginated list of actors blocked by the given actor
func (r *SocialRepository) GetBlockedUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	blockerUsername := extractUsername(actor)

	query := r.db.WithContext(ctx).Model(&models.Block{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)).
		Limit(limit + 1) // Get one extra to check if there are more results

	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	var blocks []models.Block
	err := query.All(&blocks)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query blocks: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("blocks", blocks, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = blocks[limit-1].SK
		blocks = blocks[:limit] // Trim to requested limit
	}

	// Convert to storage blocks
	result := make([]*storage.Block, len(blocks))
	for i, b := range blocks {
		result[i] = &storage.Block{
			Actor:     b.Actor,
			Object:    b.Object,
			ID:        b.ID,
			Published: b.Published,
			CreatedAt: b.CreatedAt,
		}
	}

	r.logger.Info("retrieved blocked actors",
		zap.String("actor", actor),
		zap.Int("count", len(result)),
		zap.Bool("has_more", nextCursor != ""))

	return result, nextCursor, nil
}

// GetBlockedByUsers returns a paginated list of actors who have blocked the given actor
func (r *SocialRepository) GetBlockedByUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Block, string, error) {
	blockedUsername := extractUsername(actor)

	query := r.db.WithContext(ctx).Model(&models.Block{}).
		Index("GSI5").
		Where("GSI5PK", "=", fmt.Sprintf("BLOCKED#%s", blockedUsername)).
		Limit(limit + 1) // Get one extra to check if there are more results

	if cursor != "" {
		query = query.Where("GSI5SK", ">", cursor)
	}

	var blocks []models.Block
	err := query.All(&blocks)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query blocks: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("blocks", blocks, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = blocks[limit-1].GSI5SK
		blocks = blocks[:limit] // Trim to requested limit
	}

	// Convert to storage blocks
	result := make([]*storage.Block, len(blocks))
	for i, b := range blocks {
		result[i] = &storage.Block{
			Actor:     b.Actor,
			Object:    b.Object,
			ID:        b.ID,
			Published: b.Published,
			CreatedAt: b.CreatedAt,
		}
	}

	r.logger.Info("retrieved actors who blocked",
		zap.String("blocked_actor", actor),
		zap.Int("count", len(result)),
		zap.Bool("has_more", nextCursor != ""))

	return result, nextCursor, nil
}

// ================== Mute Methods ==================

// CreateMute creates a new mute relationship
func (r *SocialRepository) CreateMute(ctx context.Context, mute *storage.Mute) error {
	r.logger.Info("creating mute", zap.String("actor", mute.Actor), zap.String("muted", mute.Object))

	// Convert to model
	model := &models.Mute{
		Actor:             mute.Actor,
		Object:            mute.Object,
		ID:                mute.ID,
		HideNotifications: mute.HideNotifications,
		Published:         mute.Published,
		CreatedAt:         mute.CreatedAt,
	}

	// BeforeCreate will set timestamps and keys
	if err := model.BeforeCreate(); err != nil {
		return err
	}

	// Create the mute
	err := r.db.WithContext(ctx).Model(model).Create()

	if err != nil {
		if errors.IsConditionFailed(err) {
			return fmt.Errorf("mute already exists")
		}
		return fmt.Errorf("failed to create mute: %w", err)
	}

	return nil
}

// DeleteMute removes a mute relationship
func (r *SocialRepository) DeleteMute(ctx context.Context, actor, mutedActor string) error {
	err := r.db.WithContext(ctx).Model(&models.Mute{}).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", actor)).
		Where("SK", "=", fmt.Sprintf("MUTED#%s", mutedActor)).
		Delete()

	if err != nil {
		return fmt.Errorf("failed to delete mute: %w", err)
	}

	return nil
}

// GetMute retrieves a specific mute relationship
func (r *SocialRepository) GetMute(ctx context.Context, actor, mutedActor string) (*storage.Mute, error) {
	var mute models.Mute
	err := r.db.WithContext(ctx).Model(&models.Mute{}).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", actor)).
		Where("SK", "=", fmt.Sprintf("MUTED#%s", mutedActor)).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get mute: %w", err)
	}

	return &storage.Mute{
		Actor:             mute.Actor,
		Object:            mute.Object,
		ID:                mute.ID,
		HideNotifications: mute.HideNotifications,
		Published:         mute.Published,
		CreatedAt:         mute.CreatedAt,
	}, nil
}

// IsMuted checks if targetActor is muted by actor
func (r *SocialRepository) IsMuted(ctx context.Context, actor, targetActor string) (bool, error) {
	var mute models.Mute
	err := r.db.WithContext(ctx).Model(&models.Mute{}).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", actor)).
		Where("SK", "=", fmt.Sprintf("MUTED#%s", targetActor)).
		First(&mute)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// GetMutedUsers returns all actors muted by the given actor
func (r *SocialRepository) GetMutedUsers(ctx context.Context, actor string, limit int, cursor string) ([]*storage.Mute, string, error) {
	if limit == 0 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Model(&models.Mute{}).
		Where("PK", "=", fmt.Sprintf("MUTE#%s", actor)).
		Limit(limit+1).       // Get one extra to check if there are more results
		OrderBy("SK", "DESC") // Newest first

	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	var mutes []models.Mute
	err := query.All(&mutes)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query muted actors: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("mutes", mutes, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = mutes[limit-1].SK
		mutes = mutes[:limit] // Trim to requested limit
	}

	// Convert to storage mutes
	result := make([]*storage.Mute, len(mutes))
	for i, m := range mutes {
		result[i] = &storage.Mute{
			Actor:             m.Actor,
			Object:            m.Object,
			ID:                m.ID,
			HideNotifications: m.HideNotifications,
			Published:         m.Published,
			CreatedAt:         m.CreatedAt,
		}
	}

	return result, nextCursor, nil
}

// ================== Announce Methods ==================

// CreateAnnounce creates a new Announce activity
func (r *SocialRepository) CreateAnnounce(ctx context.Context, announce *storage.Announce) error {
	if err := common.ValidateRequiredParam("actor", announce.Actor); err != nil {
		return fmt.Errorf("actor and object are required")
	}
	if err := common.ValidateRequiredParam("object", announce.Object); err != nil {
		return fmt.Errorf("actor and object are required")
	}

	// Convert to model
	model := &models.Announce{
		Actor:     announce.Actor,
		Object:    announce.Object,
		ID:        announce.ID,
		Published: announce.Published,
		CreatedAt: announce.CreatedAt,
		To:        announce.To,
		CC:        announce.CC,
	}

	// BeforeCreate will generate ID and set timestamps/keys
	if err := model.BeforeCreate(); err != nil {
		return err
	}

	// Create the announce
	err := r.db.WithContext(ctx).Model(model).Create()

	if err != nil {
		if errors.IsConditionFailed(err) {
			return fmt.Errorf("actor %s already announced object %s", announce.Actor, announce.Object)
		}
		return fmt.Errorf("failed to create announce: %w", err)
	}

	// Copy generated values back
	announce.ID = model.ID
	announce.Published = model.Published
	announce.CreatedAt = model.CreatedAt

	return nil
}

// DeleteAnnounce removes an Announce activity
func (r *SocialRepository) DeleteAnnounce(ctx context.Context, actor, object string) error {
	err := r.db.WithContext(ctx).Model(&models.Announce{}).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s#ANNOUNCES", object)).
		Where("SK", "=", fmt.Sprintf("ACTOR#%s", actor)).
		Delete()

	if err != nil {
		return fmt.Errorf("failed to delete announce: %w", err)
	}

	return nil
}

// GetAnnounce retrieves a specific Announce by actor and object
func (r *SocialRepository) GetAnnounce(ctx context.Context, actor, object string) (*storage.Announce, error) {
	var model models.Announce
	err := r.db.WithContext(ctx).Model(&models.Announce{}).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s#ANNOUNCES", object)).
		Where("SK", "=", fmt.Sprintf("ACTOR#%s", actor)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("announce not found for actor %s on object %s", actor, object)
		}
		return nil, fmt.Errorf("failed to get announce: %w", err)
	}

	return &storage.Announce{
		Actor:     model.Actor,
		Object:    model.Object,
		ID:        model.ID,
		Published: model.Published,
		CreatedAt: model.CreatedAt,
		To:        model.To,
		CC:        model.CC,
	}, nil
}

// GetStatusAnnounces retrieves all announces for a specific object
func (r *SocialRepository) GetStatusAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Model(&models.Announce{}).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s#ANNOUNCES", objectID)).
		Limit(limit+1).       // Get one extra to check if there are more results
		OrderBy("SK", "DESC") // Most recent first

	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	var announces []models.Announce
	err := query.All(&announces)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query object announces: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("announces", announces, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = announces[limit-1].SK
		announces = announces[:limit] // Trim to requested limit
	}

	// Convert to storage announces
	result := make([]*storage.Announce, len(announces))
	for i, a := range announces {
		result[i] = &storage.Announce{
			Actor:     a.Actor,
			Object:    a.Object,
			ID:        a.ID,
			Published: a.Published,
			CreatedAt: a.CreatedAt,
			To:        a.To,
			CC:        a.CC,
		}
	}

	return result, nextCursor, nil
}

// HasUserAnnounced checks if a user has announced a specific object
func (r *SocialRepository) HasUserAnnounced(ctx context.Context, actor, object string) (bool, error) {
	var announce models.Announce
	err := r.db.WithContext(ctx).Model(&models.Announce{}).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s#ANNOUNCES", object)).
		Where("SK", "=", fmt.Sprintf("ACTOR#%s", actor)).
		First(&announce)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// GetActorAnnounces retrieves all objects announced by a specific actor with pagination
func (r *SocialRepository) GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Announce, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Model(&models.Announce{}).
		Index("GSI4").
		Where("GSI4PK", "=", fmt.Sprintf("ACTOR#%s#ANNOUNCES", actorID)).
		Limit(limit+1).           // Get one extra to check if there are more results
		OrderBy("GSI4SK", "DESC") // Most recent first

	if cursor != "" {
		query = query.Where("GSI4SK", "<", cursor)
	}

	var announces []models.Announce
	err := query.All(&announces)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query actor announces: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("announces", announces, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = announces[limit-1].GSI4SK
		announces = announces[:limit] // Trim to requested limit
	}

	// Convert to storage announces
	result := make([]*storage.Announce, len(announces))
	for i, a := range announces {
		result[i] = &storage.Announce{
			Actor:     a.Actor,
			Object:    a.Object,
			ID:        a.ID,
			Published: a.Published,
			CreatedAt: a.CreatedAt,
			To:        a.To,
			CC:        a.CC,
		}
	}

	return result, nextCursor, nil
}

// CountObjectAnnounces returns the total number of announces for an object
func (r *SocialRepository) CountObjectAnnounces(ctx context.Context, objectID string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.Announce{}).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s#ANNOUNCES", objectID)).
		Count()

	if err != nil {
		return 0, fmt.Errorf("failed to count object announces: %w", err)
	}

	return int(count), nil
}

// CascadeDeleteAnnounces deletes all announces for an object
func (r *SocialRepository) CascadeDeleteAnnounces(ctx context.Context, objectID string) error {
	// Query all announces for the object
	var announces []models.Announce
	err := r.db.WithContext(ctx).Model(&models.Announce{}).
		Where("PK", "=", fmt.Sprintf("OBJECT#%s#ANNOUNCES", objectID)).
		Scan(&announces)

	if err != nil {
		return fmt.Errorf("failed to query announces for deletion: %w", err)
	}

	// Delete each announce
	for _, announce := range announces {
		err := r.db.WithContext(ctx).Model(&models.Announce{}).
			Where("PK", "=", announce.PK).
			Where("SK", "=", announce.SK).
			Delete()

		if err != nil {
			r.logger.Error("failed to delete announce",
				zap.String("object_id", objectID),
				zap.String("actor", announce.Actor),
				zap.Error(err))
			// Continue deleting others even if one fails
		}
	}

	return nil
}

// ================== Account Pin Methods ==================

// CreateAccountPin creates a new account pin (endorsed account)
func (r *SocialRepository) CreateAccountPin(ctx context.Context, pin *storage.AccountPin) error {
	// Check if already pinned
	exists, err := r.IsAccountPinned(ctx, pin.Username, pin.PinnedActorID)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("account already pinned")
	}

	// Convert to model
	model := &models.AccountPin{
		Username:       pin.Username,
		PinnedActorID:  pin.PinnedActorID,
		PinnedUsername: pin.PinnedUsername,
		CreatedAt:      pin.CreatedAt,
	}

	// BeforeCreate will set timestamp and keys
	if err := model.BeforeCreate(); err != nil {
		return err
	}

	// Create the pin
	err = r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create account pin", zap.Error(err))
		return err
	}

	return nil
}

// DeleteAccountPin deletes an account pin
func (r *SocialRepository) DeleteAccountPin(ctx context.Context, username, pinnedActorID string) error {
	err := r.db.WithContext(ctx).Model(&models.AccountPin{}).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_PIN#%s", username)).
		Where("SK", "=", fmt.Sprintf("PIN#%s", pinnedActorID)).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete account pin", zap.Error(err))
		return err
	}

	return nil
}

// GetAccountPins retrieves all pinned accounts for a user (for backward compatibility)
func (r *SocialRepository) GetAccountPins(ctx context.Context, username string) ([]*storage.AccountPin, error) {
	// Use paginated version with reasonable default limit (typically 5 for pinned accounts)
	pins, _, err := r.GetAccountPinsPaginated(ctx, username, 10, "")
	return pins, err
}

// GetAccountPinsPaginated retrieves pinned accounts for a user with pagination
func (r *SocialRepository) GetAccountPinsPaginated(ctx context.Context, username string, limit int, cursor string) ([]*storage.AccountPin, string, error) {
	if limit <= 0 {
		limit = 5 // Default limit for pins
	}
	if limit > 10 {
		limit = 10 // Max limit for pins (Mastodon typically allows 5)
	}

	query := r.db.WithContext(ctx).Model(&models.AccountPin{}).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_PIN#%s", username)).
		Filter("SK", "BEGINS_WITH", "PIN#").
		OrderBy("SK", "ASC")

	// Handle cursor-based pagination
	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var pins []models.AccountPin
	err := query.Scan(&pins)
	if err != nil {
		r.logger.Error("failed to query account pins", zap.Error(err))
		return nil, "", err
	}

	// Generate next cursor
	var nextCursor string
	hasMore := len(pins) > limit
	if hasMore {
		// We got more results than requested, so there are more pages
		nextCursor = pins[limit-1].SK
		// Trim results to requested limit
		pins = pins[:limit]
	}

	// Convert to storage pins
	result := make([]*storage.AccountPin, len(pins))
	for i, p := range pins {
		result[i] = &storage.AccountPin{
			Username:       p.Username,
			PinnedActorID:  p.PinnedActorID,
			PinnedUsername: p.PinnedUsername,
			CreatedAt:      p.CreatedAt,
		}
	}

	return result, nextCursor, nil
}

// IsAccountPinned checks if an account is pinned
func (r *SocialRepository) IsAccountPinned(ctx context.Context, username, pinnedActorID string) (bool, error) {
	var pin models.AccountPin
	err := r.db.WithContext(ctx).Model(&models.AccountPin{}).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_PIN#%s", username)).
		Where("SK", "=", fmt.Sprintf("PIN#%s", pinnedActorID)).
		First(&pin)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		r.logger.Error("failed to check account pin", zap.Error(err))
		return false, err
	}

	return true, nil
}

// ================== Account Note Methods ==================

// CreateAccountNote creates a new private note on an account
func (r *SocialRepository) CreateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	// Convert to model
	model := &models.AccountNote{
		Username:      note.Username,
		TargetActorID: note.TargetActorID,
		Note:          note.Note,
		CreatedAt:     note.CreatedAt,
		UpdatedAt:     note.UpdatedAt,
	}

	// BeforeCreate will set timestamps and keys
	if err := model.BeforeCreate(); err != nil {
		return err
	}

	// Create or update the note
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create account note", zap.Error(err))
		return err
	}

	return nil
}

// UpdateAccountNote updates an existing private note on an account
func (r *SocialRepository) UpdateAccountNote(ctx context.Context, note *storage.AccountNote) error {
	note.UpdatedAt = time.Now()

	// Convert to model
	model := &models.AccountNote{
		Username:      note.Username,
		TargetActorID: note.TargetActorID,
		Note:          note.Note,
		CreatedAt:     note.CreatedAt,
		UpdatedAt:     note.UpdatedAt,
	}

	// Update keys
	model.UpdateKeys()

	// Update the note (overwrites existing)
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to update account note", zap.Error(err))
		return err
	}

	return nil
}

// DeleteAccountNote deletes a private note on an account
func (r *SocialRepository) DeleteAccountNote(ctx context.Context, username, targetActorID string) error {
	err := r.db.WithContext(ctx).Model(&models.AccountNote{}).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_NOTE#%s", username)).
		Where("SK", "=", fmt.Sprintf("NOTE#%s", targetActorID)).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete account note", zap.Error(err))
		return err
	}

	return nil
}

// GetAccountNote retrieves a private note on an account
func (r *SocialRepository) GetAccountNote(ctx context.Context, username, targetActorID string) (*storage.AccountNote, error) {
	var model models.AccountNote
	err := r.db.WithContext(ctx).Model(&models.AccountNote{}).
		Where("PK", "=", fmt.Sprintf("ACCOUNT_NOTE#%s", username)).
		Where("SK", "=", fmt.Sprintf("NOTE#%s", targetActorID)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		r.logger.Error("failed to get account note", zap.Error(err))
		return nil, err
	}

	return &storage.AccountNote{
		Username:      model.Username,
		TargetActorID: model.TargetActorID,
		Note:          model.Note,
		CreatedAt:     model.CreatedAt,
		UpdatedAt:     model.UpdatedAt,
	}, nil
}

// ================== Status Pin Methods ==================

// CreateStatusPin creates a new status pin
func (r *SocialRepository) CreateStatusPin(ctx context.Context, pin *storage.StatusPin) error {
	r.logger.Info("creating status pin",
		zap.String("username", pin.Username),
		zap.String("status_id", pin.StatusID))

	// Check if user already has too many pinned statuses (Mastodon limit is typically 5)
	count, err := r.CountUserPinnedStatuses(ctx, pin.Username)
	if err != nil {
		return fmt.Errorf("failed to count pinned statuses: %w", err)
	}
	if count >= 5 {
		return fmt.Errorf("too many pinned statuses (maximum 5)")
	}

	// Convert to model
	model := &models.StatusPin{
		Username:  pin.Username,
		StatusID:  pin.StatusID,
		CreatedAt: pin.CreatedAt,
	}

	// BeforeCreate will set timestamp and keys
	if err := model.BeforeCreate(); err != nil {
		return err
	}

	// Create the status pin
	err = r.db.WithContext(ctx).Model(model).Create()

	if err != nil {
		if errors.IsConditionFailed(err) {
			return fmt.Errorf("status already pinned")
		}
		return fmt.Errorf("failed to create status pin: %w", err)
	}

	return nil
}

// DeleteStatusPin removes a status pin
func (r *SocialRepository) DeleteStatusPin(ctx context.Context, username, statusID string) error {
	err := r.db.WithContext(ctx).Model(&models.StatusPin{}).
		Where("PK", "=", fmt.Sprintf(storage.UserPinsKey, username)).
		Where("SK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		Delete()

	if err != nil {
		return fmt.Errorf("failed to delete status pin: %w", err)
	}

	return nil
}

// GetStatusPins retrieves all pinned statuses for a user (for backward compatibility)
func (r *SocialRepository) GetStatusPins(ctx context.Context, username string) ([]*storage.StatusPin, error) {
	// Use paginated version with reasonable default limit (typically 5 for pinned statuses)
	pins, _, err := r.GetStatusPinsPaginated(ctx, username, 10, "")
	return pins, err
}

// GetStatusPinsPaginated retrieves pinned statuses for a user with pagination
func (r *SocialRepository) GetStatusPinsPaginated(ctx context.Context, username string, limit int, cursor string) ([]*storage.StatusPin, string, error) {
	if limit <= 0 {
		limit = 5 // Default limit for pins
	}
	if limit > 10 {
		limit = 10 // Max limit for pins (Mastodon typically allows 5)
	}

	query := r.db.WithContext(ctx).Model(&models.StatusPin{}).
		Where("PK", "=", fmt.Sprintf(storage.UserPinsKey, username)).
		OrderBy("SK", "ASC")

	// Handle cursor-based pagination
	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var pins []models.StatusPin
	err := query.Scan(&pins)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query status pins: %w", err)
	}

	// Generate next cursor
	var nextCursor string
	hasMore := len(pins) > limit
	if hasMore {
		// We got more results than requested, so there are more pages
		nextCursor = pins[limit-1].SK
		// Trim results to requested limit
		pins = pins[:limit]
	}

	// Convert to storage pins
	result := make([]*storage.StatusPin, len(pins))
	for i, p := range pins {
		result[i] = &storage.StatusPin{
			Username:  p.Username,
			StatusID:  p.StatusID,
			CreatedAt: p.CreatedAt,
		}
	}

	return result, nextCursor, nil
}

// IsStatusPinned checks if a status is pinned by a user
func (r *SocialRepository) IsStatusPinned(ctx context.Context, username, statusID string) (bool, error) {
	var pin models.StatusPin
	err := r.db.WithContext(ctx).Model(&models.StatusPin{}).
		Where("PK", "=", fmt.Sprintf(storage.UserPinsKey, username)).
		Where("SK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		First(&pin)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check status pin: %w", err)
	}

	return true, nil
}

// ReorderStatusPins reorders pinned statuses by re-creating them with new timestamps
// Since the legacy system doesn't have a PinOrder field, we use creation timestamps for ordering
func (r *SocialRepository) ReorderStatusPins(ctx context.Context, username string, statusIDs []string) error {
	// Get existing pins to validate all statusIDs are currently pinned
	existing, err := r.GetStatusPins(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to get existing pins: %w", err)
	}

	// Create a map of existing pins for validation
	existingMap := make(map[string]*storage.StatusPin)
	for _, pin := range existing {
		existingMap[pin.StatusID] = pin
	}

	// Validate that all provided statusIDs are currently pinned
	for _, statusID := range statusIDs {
		if _, exists := existingMap[statusID]; !exists {
			return fmt.Errorf("status %s is not currently pinned", statusID)
		}
	}

	// Validate we're not missing any pinned statuses
	if len(statusIDs) != len(existing) {
		return fmt.Errorf("must provide all pinned status IDs for reordering")
	}

	// Re-create pins with new timestamps to establish order
	baseTime := time.Now()
	for i, statusID := range statusIDs {
		// Delete the existing pin
		err := r.db.WithContext(ctx).Model(&models.StatusPin{}).
			Where("PK", "=", fmt.Sprintf(storage.UserPinsKey, username)).
			Where("SK", "=", fmt.Sprintf("STATUS#%s", statusID)).
			Delete()
		if err != nil {
			r.logger.Error("Failed to delete existing pin for reordering",
				zap.String("username", username),
				zap.String("status_id", statusID),
				zap.Error(err))
			return fmt.Errorf("failed to delete existing pin: %w", err)
		}

		// Create new pin with incremental timestamp
		newPin := &models.StatusPin{
			Username:  username,
			StatusID:  statusID,
			CreatedAt: baseTime.Add(time.Duration(i) * time.Second),
		}

		// BeforeCreate will set the keys
		if err := newPin.BeforeCreate(); err != nil {
			r.logger.Error("Failed to prepare pin for creation",
				zap.String("username", username),
				zap.String("status_id", statusID),
				zap.Error(err))
			return fmt.Errorf("failed to prepare pin: %w", err)
		}

		err = r.db.WithContext(ctx).Model(newPin).Create()
		if err != nil {
			r.logger.Error("Failed to create reordered pin",
				zap.String("username", username),
				zap.String("status_id", statusID),
				zap.Error(err))
			return fmt.Errorf("failed to create reordered pin: %w", err)
		}
	}

	r.logger.Info("Successfully reordered status pins",
		zap.String("username", username),
		zap.Int("count", len(statusIDs)))

	return nil
}

// CountUserPinnedStatuses counts how many statuses a user has pinned
func (r *SocialRepository) CountUserPinnedStatuses(ctx context.Context, username string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.StatusPin{}).
		Where("PK", "=", fmt.Sprintf(storage.UserPinsKey, username)).
		Count()

	if err != nil {
		return 0, fmt.Errorf("failed to count pinned statuses: %w", err)
	}

	return int(count), nil
}

// ================== Helper Functions ==================

// extractUsername extracts the username from an actor ID
// e.g., "https://example.com/users/alice" -> "alice"
func extractUsername(actorID string) string {
	parts := strings.Split(actorID, "/")
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil {
		return parts[len(parts)-1]
	}
	return actorID
}
