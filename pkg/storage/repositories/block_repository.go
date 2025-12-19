package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// BlockRepository implements block operations using enhanced DynamORM patterns
type BlockRepository struct {
	*EnhancedBaseRepository[*models.Block]
	// Keep direct access to fields that domain methods need
	logger *zap.Logger
	db     core.DB
}

// NewBlockRepository creates a new block repository with enhanced functionality
func NewBlockRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *BlockRepository {
	// Create enhanced repository optimized for block operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Block](db, tableName, logger, costService, "BlockRepository", "block")

	// Set up enhanced services for block operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Critical for moderation security
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Block status frequently checked
	enhancedRepo.SetEventService(NewDefaultEventService())           // Important for moderation events

	return &BlockRepository{
		EnhancedBaseRepository: enhancedRepo,
		logger:                 logger,
		db:                     db,
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
		return ErrorHandler.HandleCreateError(err, EntityBlock, "prepare block")
	}

	// Use enhanced validation and creation with automatic permission checking and event emission
	if err := r.ValidateAndCreate(ctx, block); err != nil {
		// Check if it's a duplicate block
		if errors.IsConditionFailed(err) {
			r.logger.Debug("block relationship already exists",
				zap.String("blocker", blockerActor),
				zap.String("blocked", blockedActor),
				zap.Bool("validation_enabled", r.HasValidation()),
				zap.Bool("events_enabled", r.HasEvents()))
			return nil // Idempotent - don't fail if block already exists
		}
		r.logger.Error("failed to create block with enhanced validation",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.Bool("validation_enabled", r.HasValidation()),
			zap.Bool("events_enabled", r.HasEvents()),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityBlock, activityID)
	}

	r.logger.Info("created block relationship with enhanced patterns",
		zap.String("block_id", fmt.Sprintf("%s:%s", blockerActor, blockedActor)),
		zap.String("blocker", blockerActor),
		zap.String("blocked", blockedActor),
		zap.String("activity_id", activityID))

	return nil
}

// DeleteBlock removes a block relationship (for Undo Block)
func (r *BlockRepository) DeleteBlock(ctx context.Context, blockerActor, blockedActor string) error {
	// Extract usernames for key generation
	blockerUsername := extractUsernameFromActor(blockerActor)
	blockedUsername := extractUsernameFromActor(blockedActor)

	pk := fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)
	sk := fmt.Sprintf("BLOCKED#%s", blockedUsername)

	err := r.Delete(ctx, pk, sk)
	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("block not found for deletion",
				zap.String("blocker", blockerActor),
				zap.String("blocked", blockedActor))
			return nil // Idempotent - don't fail if block doesn't exist
		}
		r.logger.Error("failed to delete block",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityBlock, blockerActor)
	}

	r.logger.Info("deleted block relationship",
		zap.String("blocker", blockerActor),
		zap.String("blocked", blockedActor))

	return nil
}

// IsBlocked checks if one actor has blocked another
func (r *BlockRepository) IsBlocked(ctx context.Context, blockerActor, blockedActor string) (bool, error) {
	// Extract usernames for key generation
	blockerUsername := extractUsernameFromActor(blockerActor)
	blockedUsername := extractUsernameFromActor(blockedActor)

	pk := fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)
	sk := fmt.Sprintf("BLOCKED#%s", blockedUsername)

	var block models.Block
	err := r.Get(ctx, pk, sk, &block)
	if err != nil {
		if errors.IsNotFound(err) ||
			pkgErrors.HasCode(err, pkgErrors.CodeNotFound) ||
			strings.Contains(strings.ToLower(err.Error()), "not found") {
			return false, nil
		}
		r.logger.Error("failed to check block status",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.Error(err))
		return false, ErrorHandler.HandleGetError(err, EntityBlock, blockerActor)
	}

	return true, nil
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
		IndexName:   "",                // Use main table
		PKFormat:    "ACTOR#%s#BLOCKS", // PK format
		SKField:     "SK",              // Sort key field
		ActorField:  "Object",          // Extract blocked users (Object field)
		ErrorPrefix: "blocked users",   // Error message prefix
	}

	return getPaginatedBlockList(ctx, r.db, r.logger, blockerUsername, limit, cursor, config)
}

// GetUsersWhoBlocked returns a list of users who have blocked the given actor
func (r *BlockRepository) GetUsersWhoBlocked(ctx context.Context, blockedActor string, limit int, cursor string) ([]string, string, error) {
	blockedUsername := extractUsernameFromActor(blockedActor)

	config := RelationshipPaginationConfig{
		IndexName:   "gsi5",                    // Use gsi5 for reverse lookup
		PKFormat:    "BLOCKED#%s",              // gsi5PK format
		SKField:     "gsi5SK",                  // Sort key field for gsi5
		ActorField:  "Actor",                   // Extract blocker users (Actor field)
		ErrorPrefix: "users who blocked actor", // Error message prefix
	}

	return getPaginatedBlockList(ctx, r.db, r.logger, blockedUsername, limit, cursor, config)
}

// GetBlock retrieves a specific block relationship
func (r *BlockRepository) GetBlock(ctx context.Context, blockerActor, blockedActor string) (*storage.Block, error) {
	// Extract usernames for key generation
	blockerUsername := extractUsernameFromActor(blockerActor)
	blockedUsername := extractUsernameFromActor(blockedActor)

	pk := fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)
	sk := fmt.Sprintf("BLOCKED#%s", blockedUsername)

	var block models.Block
	err := r.Get(ctx, pk, sk, &block)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityBlock, "not found")
		}
		r.logger.Error("failed to get block",
			zap.String("blocker", blockerActor),
			zap.String("blocked", blockedActor),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityBlock, blockerActor)
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
	pk := fmt.Sprintf("ACTOR#%s#BLOCKS", blockerUsername)

	count, err := r.Count(ctx, pk)
	if err != nil {
		r.logger.Error("failed to count blocked users",
			zap.String("blocker", blockerActor),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityBlock, "count blocked users")
	}

	return count, nil
}

// CountUsersWhoBlocked returns the number of users who have blocked the given actor
func (r *BlockRepository) CountUsersWhoBlocked(ctx context.Context, blockedActor string) (int, error) {
	blockedUsername := extractUsernameFromActor(blockedActor)

	count, err := r.db.WithContext(ctx).Model(&models.Block{}).
		Index("gsi5").
		Where("gsi5PK", "=", fmt.Sprintf("BLOCKED#%s", blockedUsername)).
		Count()
	if err != nil {
		r.logger.Error("failed to count users who blocked actor",
			zap.String("blocked_actor", blockedActor),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityBlock, "count blockers")
	}

	return int(count), nil
}
