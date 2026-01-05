package advanced

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

type repositoryPatternAdapter struct {
	repo *repositories.PatternRepository
}

// NewPatternRepositoryAdapter adapts repositories.PatternRepository to the advanced PatternRepository interface.
func NewPatternRepositoryAdapter(repo *repositories.PatternRepository) PatternRepository {
	if repo == nil {
		return nil
	}
	return &repositoryPatternAdapter{repo: repo}
}

func (a *repositoryPatternAdapter) CreatePattern(ctx context.Context, pattern *ModerationPattern) error {
	return a.repo.CreatePattern(ctx, toModelPattern(pattern))
}

func (a *repositoryPatternAdapter) UpdatePattern(ctx context.Context, patternID string, pattern *ModerationPattern) error {
	return a.repo.UpdatePattern(ctx, patternID, toModelPattern(pattern))
}

func (a *repositoryPatternAdapter) DeletePattern(ctx context.Context, patternID string) error {
	return a.repo.DeletePattern(ctx, patternID)
}

func (a *repositoryPatternAdapter) GetPattern(ctx context.Context, patternID string) (*ModerationPattern, error) {
	modelPattern, err := a.repo.GetPattern(ctx, patternID)
	if err != nil {
		return nil, err
	}
	return fromModelPattern(modelPattern), nil
}

func (a *repositoryPatternAdapter) GetPatterns(ctx context.Context, filter PatternFilter) ([]*ModerationPattern, error) {
	activeOnly := false
	if filter.Active != nil {
		activeOnly = *filter.Active
	}

	modelPatterns, err := a.repo.GetPatterns(ctx, filter.Category, activeOnly)
	if err != nil {
		return nil, err
	}

	patterns := make([]*ModerationPattern, 0, len(modelPatterns))
	for _, mp := range modelPatterns {
		pattern := fromModelPattern(mp)
		if filter.Type != "" && pattern.Type != filter.Type {
			continue
		}
		if filter.MinSeverity > 0 && pattern.Severity < filter.MinSeverity {
			continue
		}
		patterns = append(patterns, pattern)
		if filter.Limit > 0 && len(patterns) >= filter.Limit {
			break
		}
	}

	return patterns, nil
}

func (a *repositoryPatternAdapter) IncrementHitCount(ctx context.Context, patternID string) error {
	return a.repo.IncrementHitCount(ctx, patternID)
}

func (a *repositoryPatternAdapter) LoadActivePatterns(ctx context.Context) ([]*ModerationPattern, error) {
	modelPatterns, err := a.repo.LoadActivePatterns(ctx)
	if err != nil {
		return nil, err
	}

	patterns := make([]*ModerationPattern, 0, len(modelPatterns))
	for _, mp := range modelPatterns {
		patterns = append(patterns, fromModelPattern(mp))
	}
	return patterns, nil
}

func toModelPattern(pattern *ModerationPattern) *models.ModerationPattern {
	if pattern == nil {
		return nil
	}
	return &models.ModerationPattern{
		PatternID:   pattern.ID,
		Pattern:     pattern.Pattern,
		Type:        pattern.Type,
		Category:    pattern.Category,
		Name:        pattern.Name,
		Severity:    pattern.Severity,
		Description: pattern.Description,
		Active:      pattern.Active,
		Flags:       pattern.Flags,
		CreatedAt:   pattern.CreatedAt,
		UpdatedAt:   pattern.UpdatedAt,
		HitCount:    pattern.HitCount,
		LastHit:     pattern.LastHit,
	}
}

func fromModelPattern(modelPattern *models.ModerationPattern) *ModerationPattern {
	if modelPattern == nil {
		return nil
	}
	return &ModerationPattern{
		ID:          modelPattern.PatternID,
		Pattern:     modelPattern.Pattern,
		Type:        modelPattern.Type,
		Category:    modelPattern.Category,
		Name:        modelPattern.Name,
		Severity:    modelPattern.Severity,
		Description: modelPattern.Description,
		Active:      modelPattern.Active,
		Flags:       modelPattern.Flags,
		CreatedAt:   modelPattern.CreatedAt,
		UpdatedAt:   modelPattern.UpdatedAt,
		HitCount:    modelPattern.HitCount,
		LastHit:     modelPattern.LastHit,
	}
}
