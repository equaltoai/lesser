package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// PatternRepository handles moderation pattern storage operations using enhanced patterns
type PatternRepository struct {
	*EnhancedBaseRepository[*models.ModerationPattern]
}

// NewPatternRepository creates a new pattern repository with enhanced functionality
func NewPatternRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *PatternRepository {
	// Create enhanced repository optimized for moderation patterns
	enhancedRepo := NewEnhancedBaseRepository[*models.ModerationPattern](db, tableName, logger, costService, "PatternRepository", "pattern")

	// Set up enhanced services for pattern operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Patterns cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Pattern change events

	return &PatternRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreatePattern creates a new moderation pattern
func (r *PatternRepository) CreatePattern(ctx context.Context, pattern *models.ModerationPattern) error {
	if pattern == nil {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityModerationPattern, "create")
	}

	// Set timestamps
	pattern.CreatedAt = time.Now()
	pattern.UpdatedAt = time.Now()
	pattern.HitCount = 0

	// Use BaseRepository Create method
	err := r.Create(ctx, pattern)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityModerationPattern, pattern.PatternID)
	}

	r.logger.Info("created moderation pattern",
		zap.String("pattern_id", pattern.PatternID),
		zap.String("type", pattern.Type))

	return nil
}

// UpdatePattern updates an existing pattern
func (r *PatternRepository) UpdatePattern(ctx context.Context, patternID string, updates *models.ModerationPattern) error {
	// Get existing pattern using BaseRepository
	existing := &models.ModerationPattern{}
	pk := fmt.Sprintf("PATTERN#%s", patternID)
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, existing)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityModerationPattern, patternID)
	}

	// Apply updates
	if updates.Pattern != "" {
		existing.Pattern = updates.Pattern
	}
	if updates.Type != "" {
		existing.Type = updates.Type
	}
	if updates.Category != "" {
		existing.Category = updates.Category
	}
	if updates.Severity > 0 {
		existing.Severity = updates.Severity
	}
	if updates.Description != "" {
		existing.Description = updates.Description
	}
	if len(updates.Flags) > 0 {
		existing.Flags = updates.Flags
	}
	existing.Active = updates.Active
	existing.UpdatedAt = time.Now()

	// Save updates using BaseRepository
	err = r.Update(ctx, existing)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityModerationPattern, patternID)
	}

	return nil
}

// DeletePattern soft deletes a pattern by marking it inactive
func (r *PatternRepository) DeletePattern(ctx context.Context, patternID string) error {
	pattern := &models.ModerationPattern{}
	pk := fmt.Sprintf("PATTERN#%s", patternID)
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, pattern)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityModerationPattern, patternID)
	}

	// Soft delete by marking inactive
	pattern.Active = false
	pattern.UpdatedAt = time.Now()

	err = r.Update(ctx, pattern)
	if err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityModerationPattern, patternID)
	}

	return nil
}

// GetPattern retrieves a single pattern by ID
func (r *PatternRepository) GetPattern(ctx context.Context, patternID string) (*models.ModerationPattern, error) {
	pattern := &models.ModerationPattern{}
	pk := fmt.Sprintf("PATTERN#%s", patternID)
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, pattern)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityModerationPattern, patternID)
	}

	return pattern, nil
}

// GetPatterns retrieves patterns based on filter criteria
func (r *PatternRepository) GetPatterns(ctx context.Context, category string, activeOnly bool) ([]*models.ModerationPattern, error) {
	patterns := []*models.ModerationPattern{}

	// Build filters for BaseRepository QueryWithFilter method
	filters := make(map[string]interface{})
	if category != "" {
		filters["Category"] = category
	}
	if activeOnly {
		filters["Active"] = true
	}

	// For patterns, we need to scan all patterns (PK prefix "PATTERN#") and apply filters
	// Since patterns don't share a common PK, we'll use the direct DB query approach
	query := r.GetDB().WithContext(ctx).Model(&models.ModerationPattern{}).
		Where("SK", "=", models.SKMetadata)

	// Apply filters
	for field, value := range filters {
		query = query.Filter(field, "=", value)
	}

	err := query.All(&patterns)
	if err != nil {
		r.logger.Error("failed to get patterns",
			zap.Error(err),
			zap.String("category", category),
			zap.Bool("activeOnly", activeOnly))
		return nil, ErrorHandler.HandleQueryError(err, EntityModerationPattern, "patterns by filters")
	}

	// Track cost if available
	if r.costService != nil {
		itemCount := int64(len(patterns))
		estimatedRU := itemCount
		if estimatedRU == 0 {
			estimatedRU = 1
		}
		if trackErr := r.TrackRead(ctx, "Scan", estimatedRU); trackErr != nil {
			r.logger.Warn("failed to track pattern scan cost", zap.Error(trackErr))
		}
	}

	return patterns, nil
}

// IncrementHitCount increments the hit count for a pattern
func (r *PatternRepository) IncrementHitCount(ctx context.Context, patternID string) error {
	pattern := &models.ModerationPattern{}
	pk := fmt.Sprintf("PATTERN#%s", patternID)
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, pattern)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityModerationPattern, patternID)
	}

	// Increment hit count
	pattern.HitCount++
	pattern.LastHit = time.Now()

	err = r.Update(ctx, pattern)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityModerationPattern, patternID)
	}

	return nil
}

// LoadActivePatterns loads all active patterns
func (r *PatternRepository) LoadActivePatterns(ctx context.Context) ([]*models.ModerationPattern, error) {
	return r.GetPatterns(ctx, "", true)
}
