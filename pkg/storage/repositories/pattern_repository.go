package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// PatternRepository handles moderation pattern storage operations
type PatternRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewPatternRepository creates a new pattern repository
func NewPatternRepository(db core.DB, tableName string, logger *zap.Logger) *PatternRepository {
	return &PatternRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreatePattern creates a new moderation pattern
func (r *PatternRepository) CreatePattern(ctx context.Context, pattern *models.ModerationPattern) error {
	if pattern == nil {
		return fmt.Errorf("pattern cannot be nil")
	}

	// Set timestamps
	pattern.CreatedAt = time.Now()
	pattern.UpdatedAt = time.Now()
	pattern.HitCount = 0

	// Update keys for DynamoDB
	pattern.UpdateKeys()

	// Save to DynamoDB
	err := r.db.WithContext(ctx).Model(pattern).Create()
	if err != nil {
		return fmt.Errorf("failed to create pattern: %w", err)
	}

	r.logger.Info("created moderation pattern",
		zap.String("pattern_id", pattern.PatternID),
		zap.String("type", pattern.Type))

	return nil
}

// UpdatePattern updates an existing pattern
func (r *PatternRepository) UpdatePattern(ctx context.Context, patternID string, updates *models.ModerationPattern) error {
	// Get existing pattern
	existing := &models.ModerationPattern{}
	existing.PK = fmt.Sprintf("PATTERN#%s", patternID)
	existing.SK = SKMetadata

	err := r.db.WithContext(ctx).Model(existing).
		Where("PK", "=", existing.PK).
		Where("SK", "=", existing.SK).
		First(existing)

	if err != nil {
		return fmt.Errorf("failed to get pattern for update: %w", err)
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

	// Save updates
	err = r.db.WithContext(ctx).Model(existing).Update()
	if err != nil {
		return fmt.Errorf("failed to update pattern: %w", err)
	}

	return nil
}

// DeletePattern soft deletes a pattern by marking it inactive
func (r *PatternRepository) DeletePattern(ctx context.Context, patternID string) error {
	pattern := &models.ModerationPattern{}
	pattern.PK = fmt.Sprintf("PATTERN#%s", patternID)
	pattern.SK = SKMetadata

	err := r.db.WithContext(ctx).Model(pattern).
		Where("PK", "=", pattern.PK).
		Where("SK", "=", pattern.SK).
		First(pattern)

	if err != nil {
		return fmt.Errorf("failed to get pattern for deletion: %w", err)
	}

	// Soft delete by marking inactive
	pattern.Active = false
	pattern.UpdatedAt = time.Now()

	err = r.db.WithContext(ctx).Model(pattern).Update()
	if err != nil {
		return fmt.Errorf("failed to delete pattern: %w", err)
	}

	return nil
}

// GetPattern retrieves a single pattern by ID
func (r *PatternRepository) GetPattern(ctx context.Context, patternID string) (*models.ModerationPattern, error) {
	pattern := &models.ModerationPattern{}
	pattern.PK = fmt.Sprintf("PATTERN#%s", patternID)
	pattern.SK = SKMetadata

	err := r.db.WithContext(ctx).Model(pattern).
		Where("PK", "=", pattern.PK).
		Where("SK", "=", pattern.SK).
		First(pattern)

	if err != nil {
		return nil, fmt.Errorf("failed to get pattern: %w", err)
	}

	return pattern, nil
}

// GetPatterns retrieves patterns based on filter criteria
func (r *PatternRepository) GetPatterns(ctx context.Context, category string, activeOnly bool) ([]*models.ModerationPattern, error) {
	patterns := []*models.ModerationPattern{}

	query := r.db.WithContext(ctx).Model(&models.ModerationPattern{}).
		Where("SK", "=", "METADATA")

	// Apply filters
	if category != "" {
		query = query.Filter("Category", "=", category)
	}
	if activeOnly {
		query = query.Filter("Active", "=", true)
	}

	err := query.All(&patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns: %w", err)
	}

	return patterns, nil
}

// IncrementHitCount increments the hit count for a pattern
func (r *PatternRepository) IncrementHitCount(ctx context.Context, patternID string) error {
	pattern := &models.ModerationPattern{}
	pattern.PK = fmt.Sprintf("PATTERN#%s", patternID)
	pattern.SK = SKMetadata

	err := r.db.WithContext(ctx).Model(pattern).
		Where("PK", "=", pattern.PK).
		Where("SK", "=", pattern.SK).
		First(pattern)

	if err != nil {
		return fmt.Errorf("failed to get pattern for hit count update: %w", err)
	}

	// Increment hit count
	pattern.HitCount++
	pattern.LastHit = time.Now()

	err = r.db.WithContext(ctx).Model(pattern).Update()
	if err != nil {
		return fmt.Errorf("failed to update hit count: %w", err)
	}

	return nil
}

// LoadActivePatterns loads all active patterns
func (r *PatternRepository) LoadActivePatterns(ctx context.Context) ([]*models.ModerationPattern, error) {
	return r.GetPatterns(ctx, "", true)
}
