package advanced

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
)

// PatternRepositoryBridge adapts the storage.Storage interface to the PatternRepository interface
// This allows the PatternMatcher to work with the existing DynamORM-based storage layer
type PatternRepositoryBridge struct {
	storage storage.Storage
}

// NewPatternRepositoryBridge creates a new bridge to adapt storage.Storage to PatternRepository
func NewPatternRepositoryBridge(storage storage.Storage) PatternRepository {
	return &PatternRepositoryBridge{
		storage: storage,
	}
}

// CreatePattern creates a new moderation pattern
func (b *PatternRepositoryBridge) CreatePattern(ctx context.Context, pattern *ModerationPattern) error {
	// Convert to storage format
	storagePattern := &storage.ModerationPattern{
		ID:          pattern.ID,
		Name:        pattern.Name,
		Description: pattern.Description,
		Type:        pattern.PatternType,
		Content:     pattern.Pattern,
		Severity:    string(pattern.Severity),
		Active:      pattern.Active,
		CreatedAt:   pattern.CreatedAt,
		UpdatedAt:   pattern.UpdatedAt,
	}

	return b.storage.CreateModerationPattern(ctx, storagePattern)
}

// UpdatePattern updates an existing moderation pattern
func (b *PatternRepositoryBridge) UpdatePattern(ctx context.Context, patternID string, pattern *ModerationPattern) error {
	// Get existing pattern first
	existing, err := b.storage.GetModerationPattern(ctx, patternID)
	if err != nil {
		return err
	}

	// Apply updates
	if pattern.Name != "" {
		existing.Name = pattern.Name
	}
	if pattern.Description != "" {
		existing.Description = pattern.Description
	}
	if pattern.Pattern != "" {
		existing.Content = pattern.Pattern
	}
	if pattern.PatternType != "" {
		existing.Type = pattern.PatternType
	}
	if pattern.Severity != "" {
		existing.Severity = string(pattern.Severity)
	}

	existing.UpdatedAt = time.Now()

	return b.storage.UpdateModerationPattern(ctx, existing)
}

// DeletePattern deletes a moderation pattern (soft delete by marking inactive)
func (b *PatternRepositoryBridge) DeletePattern(ctx context.Context, patternID string) error {
	// Get existing pattern
	existing, err := b.storage.GetModerationPattern(ctx, patternID)
	if err != nil {
		return err
	}

	// Mark as inactive
	existing.Active = false
	existing.UpdatedAt = time.Now()

	return b.storage.UpdateModerationPattern(ctx, existing)
}

// GetPattern retrieves a moderation pattern by ID
func (b *PatternRepositoryBridge) GetPattern(ctx context.Context, patternID string) (*ModerationPattern, error) {
	storagePattern, err := b.storage.GetModerationPattern(ctx, patternID)
	if err != nil {
		return nil, err
	}

	// Convert from storage format
	pattern := &ModerationPattern{
		ID:          storagePattern.ID,
		Name:        storagePattern.Name,
		Description: storagePattern.Description,
		Pattern:     storagePattern.Content,
		PatternType: storagePattern.Type,
		Severity:    Severity(storagePattern.Severity),
		Active:      storagePattern.Active,
		CreatedAt:   storagePattern.CreatedAt,
		UpdatedAt:   storagePattern.UpdatedAt,
		HitCount:    0, // Will be tracked separately
	}

	return pattern, nil
}

// GetPatterns retrieves patterns based on filter criteria
func (b *PatternRepositoryBridge) GetPatterns(ctx context.Context, filter PatternFilter) ([]*ModerationPattern, error) {
	// Use the storage layer to get patterns
	storagePatterns, err := b.storage.GetModerationPatterns(ctx, 
		filter.Active != nil && *filter.Active, 
		string(filter.Severity), 
		100) // Default limit
	if err != nil {
		return nil, err
	}

	// Convert to advanced format
	patterns := make([]*ModerationPattern, 0, len(storagePatterns))
	for _, sp := range storagePatterns {
		pattern := &ModerationPattern{
			ID:          sp.ID,
			Name:        sp.Name,
			Description: sp.Description,
			Pattern:     sp.Content,
			PatternType: sp.Type,
			Severity:    Severity(sp.Severity),
			Active:      sp.Active,
			CreatedAt:   sp.CreatedAt,
			UpdatedAt:   sp.UpdatedAt,
			HitCount:    0, // Will be tracked separately
		}

		// Apply additional filters
		if b.matchesFilter(pattern, filter) {
			patterns = append(patterns, pattern)
		}
	}

	return patterns, nil
}

// IncrementHitCount increments the hit count for a pattern
func (b *PatternRepositoryBridge) IncrementHitCount(ctx context.Context, patternID string) error {
	// For now, just record the match in analytics
	// In the future, this could be implemented with the RecordPatternMatch method
	return b.storage.RecordPatternMatch(ctx, patternID, true, time.Now())
}

// LoadActivePatterns loads all active patterns for caching
func (b *PatternRepositoryBridge) LoadActivePatterns(ctx context.Context) ([]*ModerationPattern, error) {
	active := true
	filter := PatternFilter{
		Active: &active,
	}
	return b.GetPatterns(ctx, filter)
}

// matchesFilter checks if a pattern matches the given filter
func (b *PatternRepositoryBridge) matchesFilter(pattern *ModerationPattern, filter PatternFilter) bool {
	// Check severity
	if filter.Severity != "" && pattern.Severity != filter.Severity {
		return false
	}

	// Check active status
	if filter.Active != nil && pattern.Active != *filter.Active {
		return false
	}

	// Check categories (if pattern had categories)
	if len(filter.Categories) > 0 {
		hasCategory := false
		for _, filterCat := range filter.Categories {
			for _, patternCat := range pattern.Categories {
				if filterCat == patternCat {
					hasCategory = true
					break
				}
			}
		}
		if !hasCategory {
			return false
		}
	}

	// Check created by
	if filter.CreatedBy != "" && pattern.CreatedBy != filter.CreatedBy {
		return false
	}

	return true
}