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

// BlockRepository implements block operations using DynamORM
type BlockRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewBlockRepository creates a new block repository
func NewBlockRepository(db core.DB, tableName string, logger *zap.Logger) *BlockRepository {
	return &BlockRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateBlock creates a new block relationship
func (r *BlockRepository) CreateBlock(ctx context.Context, blockerActor, blockedActor, activityID string) error {
	block := &models.Block{
		Actor:     blockerActor,
		Object:    blockedActor,
		ID:        activityID,
		Published: time.Now().UTC(),
		CreatedAt: time.Now().UTC(),
	}

	// BeforeCreate will set keys and other fields
	if err := block.BeforeCreate(); err != nil {
		r.logger.Error("failed to prepare block for creation",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.Error(err))
		return fmt.Errorf("failed to prepare block: %w", err)
	}

	if err := r.db.WithContext(ctx).Model(block).Create(); err != nil {
		// Check if it's a duplicate block
		if errors.IsConditionFailed(err) {
			r.logger.Debug("block relationship already exists",
				zap.String("blocker", blockerActor),
				zap.String("blocked", blockedActor))
			return nil // Idempotent - don't fail if block already exists
		}
		r.logger.Error("failed to create block",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.Error(err))
		return fmt.Errorf("failed to create block: %w", err)
	}

	r.logger.Info("created block relationship",
		zap.String("blocker", blockerActor),
		zap.String("blocked", blockedActor),
		zap.String("activity_id", activityID))

	return nil
}

// DeleteBlock removes a block relationship (for Undo Block)
func (r *BlockRepository) DeleteBlock(ctx context.Context, blockerActor, blockedActor string) error {
	helper := NewRelationshipHelper(r.db, r.logger, "block")
	return helper.DeleteRelationship(ctx, blockerActor, blockedActor, 
		"ACTOR#%s#BLOCKS", "BLOCKED#%s", &models.Block{})
}

// IsBlocked checks if one actor has blocked another
func (r *BlockRepository) IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error) {
	helper := NewRelationshipHelper(r.db, r.logger, "block")
	return helper.CheckRelationship(ctx, blockerActor, blockedActor, 
		"ACTOR#%s#BLOCKS", "BLOCKED#%s", &models.Block{})
}

// IsBlockedBidirectional checks if either actor has blocked the other
func (r *BlockRepository) IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error) {
	// Check both directions
	blocked1, err := r.IsBlocked(ctx, actor1, actor2)
	if err != nil {
		return false, err
	}
	if blocked1 {
		return true, nil
	}

	blocked2, err := r.IsBlocked(ctx, actor2, actor1)
	if err != nil {
		return false, err
	}

	return blocked2, nil
}

// GetBlockedUsers returns a list of users blocked by the given actor
func (r *BlockRepository) GetBlockedUsers(ctx context.Context, blockerActor string, limit int, cursor string) ([]string, string, error) {
	blockerUsername := extractUsernameFromActor(blockerActor)
	
	config := RelationshipPaginationConfig{
		IndexName:   "",                       // Use main table
		PKFormat:    "ACTOR#%s#BLOCKS",       // PK format
		SKField:     "SK",                    // Sort key field
		ActorField:  "Object",                // Extract blocked users (Object field)
		ErrorPrefix: "blocked users",         // Error message prefix
	}
	
	return getPaginatedBlockList(ctx, r.db, r.logger, blockerUsername, limit, cursor, config)
}

// GetUsersWhoBlocked returns a list of users who have blocked the given actor
func (r *BlockRepository) GetUsersWhoBlocked(ctx context.Context, blockedActor string, limit int, cursor string) ([]string, string, error) {
	blockedUsername := extractUsernameFromActor(blockedActor)
	
	config := RelationshipPaginationConfig{
		IndexName:   "GSI5",                     // Use GSI5 for reverse lookup
		PKFormat:    "BLOCKED#%s",               // GSI5PK format
		SKField:     "GSI5SK",                   // Sort key field for GSI5
		ActorField:  "Actor",                    // Extract blocker users (Actor field)
		ErrorPrefix: "users who blocked actor", // Error message prefix
	}
	
	return getPaginatedBlockList(ctx, r.db, r.logger, blockedUsername, limit, cursor, config)
}

// GetBlock retrieves a specific block relationship
func (r *BlockRepository) GetBlock(ctx context.Context, blockerActor, blockedActor string) (*storage.Block, error) {
	// Extract usernames for key generation
	blockerUsername := extractUsernameFromActor(blockerActor)
	blockedUsername := extractUsernameFromActor(blockedActor)

	var block models.Block

	err := r.db.WithContext(ctx).Model(&block).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)).
		Where("SK", "=", fmt.Sprintf("BLOCKED#%s", blockedUsername)).
		First(&block)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("block not found")
		}
		r.logger.Error("failed to get block",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	// Convert to storage.Block
	return &storage.Block{
		Actor:     block.Actor,
		Object:    block.Object,
		ID:        block.ID,
		Published: block.Published,
		CreatedAt: block.CreatedAt,
	}, nil
}

// CountBlockedUsers returns the number of users blocked by the given actor
func (r *BlockRepository) CountBlockedUsers(ctx context.Context, blockerActor string) (int, error) {
	blockerUsername := extractUsernameFromActor(blockerActor)

	count, err := r.db.WithContext(ctx).Model(&models.Block{}).
		Where("PK", "=", fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)).
		Count()

	if err != nil {
		r.logger.Error("failed to count blocked users",
			zap.String("blocker", blockerActor),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count blocked users: %w", err)
	}

	return int(count), nil
}

// CountUsersWhoBlocked returns the number of users who have blocked the given actor
func (r *BlockRepository) CountUsersWhoBlocked(ctx context.Context, blockedActor string) (int, error) {
	blockedUsername := extractUsernameFromActor(blockedActor)

	count, err := r.db.WithContext(ctx).Model(&models.Block{}).
		Index("GSI5").
		Where("GSI5PK", "=", fmt.Sprintf("BLOCKED#%s", blockedUsername)).
		Count()

	if err != nil {
		r.logger.Error("failed to count users who blocked actor",
			zap.String("blocked_actor", blockedActor),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count users who blocked actor: %w", err)
	}

	return int(count), nil
}

