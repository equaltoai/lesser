package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// EnhancedPatternRepository handles enhanced moderation pattern storage operations
type EnhancedPatternRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewEnhancedPatternRepository creates a new enhanced pattern repository
func NewEnhancedPatternRepository(db core.DB, tableName string, logger *zap.Logger) *EnhancedPatternRepository {
	return &EnhancedPatternRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreatePattern creates a new enhanced moderation pattern
func (r *EnhancedPatternRepository) CreatePattern(ctx context.Context, pattern *models.EnhancedModerationPattern) error {
	if pattern == nil {
		return fmt.Errorf("pattern cannot be nil")
	}

	// Set timestamps and defaults
	now := time.Now()
	pattern.CreatedAt = now
	pattern.UpdatedAt = now
	pattern.Version = 1
	pattern.MatchCount = 0
	pattern.FalsePositiveCount = 0
	pattern.TruePositiveCount = 0
	pattern.ConfidenceScore = 0.5 // Default neutral confidence
	pattern.ValidationScore = 0.0 // Will be set after validation

	// Calculate initial effectiveness
	pattern.CalculateEffectiveness()

	// Update keys for DynamoDB
	pattern.UpdateKeys()

	// Save to DynamoDB
	err := r.db.WithContext(ctx).Model(pattern).Create()
	if err != nil {
		return fmt.Errorf("failed to create enhanced pattern: %w", err)
	}

	r.logger.Info("created enhanced moderation pattern",
		zap.String("pattern_id", pattern.PatternID),
		zap.String("pattern_type", pattern.PatternType),
		zap.String("category", pattern.Category),
		zap.Int("priority", pattern.Priority))

	return nil
}

// GetPattern retrieves an enhanced pattern by ID
func (r *EnhancedPatternRepository) GetPattern(ctx context.Context, patternID string) (*models.EnhancedModerationPattern, error) {
	pattern := &models.EnhancedModerationPattern{}
	pattern.PK = fmt.Sprintf("ENHANCED_PATTERN#%s", patternID)
	pattern.SK = "METADATA"

	err := r.db.WithContext(ctx).Model(pattern).
		Where("PK", "=", pattern.PK).
		Where("SK", "=", pattern.SK).
		First(pattern)

	if err != nil {
		return nil, fmt.Errorf("failed to get enhanced pattern: %w", err)
	}

	return pattern, nil
}

// UpdatePattern updates an existing enhanced pattern
func (r *EnhancedPatternRepository) UpdatePattern(ctx context.Context, pattern *models.EnhancedModerationPattern) error {
	if pattern == nil {
		return fmt.Errorf("pattern cannot be nil")
	}

	// Update timestamp and recalculate effectiveness
	pattern.UpdatedAt = time.Now()
	pattern.CalculateEffectiveness()
	pattern.UpdateKeys()

	err := r.db.WithContext(ctx).Model(pattern).Update()
	if err != nil {
		return fmt.Errorf("failed to update enhanced pattern: %w", err)
	}

	r.logger.Info("updated enhanced moderation pattern",
		zap.String("pattern_id", pattern.PatternID),
		zap.Float64("effectiveness", pattern.Effectiveness))

	return nil
}

// DeletePattern soft deletes a pattern by marking it inactive
func (r *EnhancedPatternRepository) DeletePattern(ctx context.Context, patternID string) error {
	pattern, err := r.GetPattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("failed to get pattern for deletion: %w", err)
	}

	// Soft delete by marking inactive
	pattern.Active = false
	pattern.UpdatedAt = time.Now()
	pattern.UpdateKeys()

	err = r.db.WithContext(ctx).Model(pattern).Update()
	if err != nil {
		return fmt.Errorf("failed to delete enhanced pattern: %w", err)
	}

	r.logger.Info("deleted enhanced moderation pattern",
		zap.String("pattern_id", patternID))

	return nil
}

// GetActivePatterns retrieves all active patterns ordered by priority
func (r *EnhancedPatternRepository) GetActivePatterns(ctx context.Context, limit int) ([]*models.EnhancedModerationPattern, error) {
	patterns := []*models.EnhancedModerationPattern{}

	query := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("GSI1PK", "=", "ENHANCED_PATTERNS#ACTIVE")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to get active enhanced patterns: %w", err)
	}

	return patterns, nil
}

// GetPatternsByType retrieves patterns by type ordered by effectiveness
func (r *EnhancedPatternRepository) GetPatternsByType(ctx context.Context, patternType string, limit int) ([]*models.EnhancedModerationPattern, error) {
	patterns := []*models.EnhancedModerationPattern{}

	query := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("GSI2PK", "=", fmt.Sprintf("ENHANCED_PATTERNS#%s", patternType)).
		OrderBy("GSI2SK", "DESC") // Descending order for best effectiveness first

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns by type: %w", err)
	}

	return patterns, nil
}

// GetPatternsByCategory retrieves patterns by category ordered by effectiveness
func (r *EnhancedPatternRepository) GetPatternsByCategory(ctx context.Context, category string, limit int) ([]*models.EnhancedModerationPattern, error) {
	patterns := []*models.EnhancedModerationPattern{}

	query := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("GSI3PK", "=", fmt.Sprintf("PATTERN_METRICS#%s", category)).
		OrderBy("GSI3SK", "DESC") // Descending order for best effectiveness first

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns by category: %w", err)
	}

	return patterns, nil
}

// RecordMatch records a pattern match and updates statistics
func (r *EnhancedPatternRepository) RecordMatch(ctx context.Context, patternID string, isMatch bool, isTruePositive bool, matchTime float64) error {
	pattern, err := r.GetPattern(ctx, patternID)
	if err != nil {
		return fmt.Errorf("failed to get pattern for match recording: %w", err)
	}

	// Update match statistics
	if isMatch {
		pattern.MatchCount++
		pattern.LastMatch = time.Now()
		pattern.LastUsed = time.Now()

		if isTruePositive {
			pattern.TruePositiveCount++
		} else {
			pattern.FalsePositiveCount++
		}

		// Update average match time
		if pattern.AverageMatchTime == 0 {
			pattern.AverageMatchTime = matchTime
		} else {
			// Simple moving average
			pattern.AverageMatchTime = (pattern.AverageMatchTime + matchTime) / 2
		}
	}

	// Recalculate effectiveness and update
	pattern.CalculateEffectiveness()
	return r.UpdatePattern(ctx, pattern)
}

// GetPatternCache retrieves cached pattern data
func (r *EnhancedPatternRepository) GetPatternCache(ctx context.Context, patternID, patternType string) (*models.PatternCache, error) {
	cache := &models.PatternCache{}
	cache.PK = fmt.Sprintf("PATTERN_CACHE#%s", patternType)
	cache.SK = fmt.Sprintf("COMPILED#%s", patternID)

	err := r.db.WithContext(ctx).Model(cache).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		First(cache)

	if err != nil {
		return nil, fmt.Errorf("pattern cache not found: %w", err)
	}

	// Update last used and cache hits
	cache.LastUsed = time.Now()
	cache.CacheHits++
	cache.UpdateKeys()

	// Update cache statistics (fire and forget)
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.db.WithContext(updateCtx).Model(cache).Update()
	}()

	return cache, nil
}

// SetPatternCache stores compiled pattern data in cache
func (r *EnhancedPatternRepository) SetPatternCache(ctx context.Context, cache *models.PatternCache) error {
	if cache == nil {
		return fmt.Errorf("cache cannot be nil")
	}

	now := time.Now()
	cache.CreatedAt = now
	cache.UpdatedAt = now
	cache.LastUsed = now
	cache.CacheHits = 0

	cache.UpdateKeys()

	err := r.db.WithContext(ctx).Model(cache).Create()
	if err != nil {
		// Try update if create fails (cache entry might exist)
		cache.UpdatedAt = now
		err = r.db.WithContext(ctx).Model(cache).Update()
		if err != nil {
			return fmt.Errorf("failed to set pattern cache: %w", err)
		}
	}

	return nil
}

// InvalidatePatternCache removes cached pattern data
func (r *EnhancedPatternRepository) InvalidatePatternCache(ctx context.Context, patternID, patternType string) error {
	cache := &models.PatternCache{}
	cache.PK = fmt.Sprintf("PATTERN_CACHE#%s", patternType)
	cache.SK = fmt.Sprintf("COMPILED#%s", patternID)

	err := r.db.WithContext(ctx).Model(cache).
		Where("PK", "=", cache.PK).
		Where("SK", "=", cache.SK).
		Delete()

	if err != nil {
		r.logger.Warn("failed to invalidate pattern cache",
			zap.String("pattern_id", patternID),
			zap.String("pattern_type", patternType),
			zap.Error(err))
	}

	return nil
}

// RecordPerformanceMetric records detailed performance metrics
func (r *EnhancedPatternRepository) RecordPerformanceMetric(ctx context.Context, metric *models.PatternPerformanceMetric) error {
	if metric == nil {
		return fmt.Errorf("metric cannot be nil")
	}

	now := time.Now()
	metric.CreatedAt = now
	metric.UpdatedAt = now

	// Calculate quality metrics
	metric.CalculateQualityMetrics()

	metric.UpdateKeys()

	// Try to get existing metric for this hour
	existing := &models.PatternPerformanceMetric{}
	existing.PK = metric.PK
	existing.SK = metric.SK

	err := r.db.WithContext(ctx).Model(existing).
		Where("PK", "=", existing.PK).
		Where("SK", "=", existing.SK).
		First(existing)

	if err != nil {
		// Create new metric
		err = r.db.WithContext(ctx).Model(metric).Create()
		if err != nil {
			return fmt.Errorf("failed to create performance metric: %w", err)
		}
	} else {
		// Update existing metric
		existing.MatchAttempts += metric.MatchAttempts
		existing.SuccessfulMatches += metric.SuccessfulMatches
		existing.FalsePositives += metric.FalsePositives
		existing.TruePositives += metric.TruePositives
		existing.TotalMatchTime += metric.TotalMatchTime
		existing.MemoryUsage = metric.MemoryUsage // Use latest
		existing.CPUTime += metric.CPUTime

		// Recalculate averages
		if existing.MatchAttempts > 0 {
			existing.AverageMatchTime = existing.TotalMatchTime / float64(existing.MatchAttempts)
		}

		// Update min/max
		if metric.MaxMatchTime > existing.MaxMatchTime {
			existing.MaxMatchTime = metric.MaxMatchTime
		}
		if existing.MinMatchTime == 0 || (metric.MinMatchTime > 0 && metric.MinMatchTime < existing.MinMatchTime) {
			existing.MinMatchTime = metric.MinMatchTime
		}

		existing.UpdatedAt = now
		existing.CalculateQualityMetrics()
		existing.UpdateKeys()

		err = r.db.WithContext(ctx).Model(existing).Update()
		if err != nil {
			return fmt.Errorf("failed to update performance metric: %w", err)
		}
	}

	return nil
}

// CreateTestResult records pattern test results
func (r *EnhancedPatternRepository) CreateTestResult(ctx context.Context, result *models.PatternTestResult) error {
	if result == nil {
		return fmt.Errorf("test result cannot be nil")
	}

	now := time.Now()
	result.CreatedAt = now
	result.RunAt = now

	result.UpdateKeys()

	err := r.db.WithContext(ctx).Model(result).Create()
	if err != nil {
		return fmt.Errorf("failed to create test result: %w", err)
	}

	r.logger.Info("recorded pattern test result",
		zap.String("pattern_id", result.PatternID),
		zap.String("test_type", result.TestType),
		zap.Bool("passed", result.Passed),
		zap.Float64("score", result.Score))

	return nil
}

// GetTestResults retrieves test results for a pattern
func (r *EnhancedPatternRepository) GetTestResults(ctx context.Context, patternID string, testType string, limit int) ([]*models.PatternTestResult, error) {
	results := []*models.PatternTestResult{}

	query := r.db.WithContext(ctx).Model(&models.PatternTestResult{}).
		Where("PK", "=", fmt.Sprintf("PATTERN_TEST#%s", patternID))

	if testType != "" {
		query = query.Filter("TestType", "=", testType)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&results)
	if err != nil {
		return nil, fmt.Errorf("failed to get test results: %w", err)
	}

	return results, nil
}

// GetLatestTestResult gets the most recent test result for a pattern and test type
func (r *EnhancedPatternRepository) GetLatestTestResult(ctx context.Context, patternID, testType string) (*models.PatternTestResult, error) {
	results, err := r.GetTestResults(ctx, patternID, testType, 1)
	if err != nil {
		return nil, err
	}

	if err := common.ValidateSliceNotEmpty("results", results); err != nil {
		return nil, fmt.Errorf("no test results found for pattern %s and type %s", patternID, testType)
	}

	return results[0], nil
}

// GetPerformanceMetrics retrieves performance metrics for a pattern and date range
func (r *EnhancedPatternRepository) GetPerformanceMetrics(ctx context.Context, patternID, startDate, endDate string) ([]*models.PatternPerformanceMetric, error) {
	metrics := []*models.PatternPerformanceMetric{}

	query := r.db.WithContext(ctx).Model(&models.PatternPerformanceMetric{}).
		Where("PK", "=", fmt.Sprintf("PATTERN_METRICS#%s", patternID))

	if startDate != "" {
		query = query.Filter("SK", ">=", fmt.Sprintf("TIME#%s#00", startDate))
	}
	if endDate != "" {
		query = query.Filter("SK", "<=", fmt.Sprintf("TIME#%s#23", endDate))
	}

	err := query.All(&metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to get performance metrics: %w", err)
	}

	return metrics, nil
}

// CleanupExpiredPatterns removes patterns that have expired
func (r *EnhancedPatternRepository) CleanupExpiredPatterns(ctx context.Context) (int, error) {
	// Get all patterns to check expiration
	patterns := []*models.EnhancedModerationPattern{}
	err := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("SK", "=", "METADATA").
		All(&patterns)

	if err != nil {
		return 0, fmt.Errorf("failed to get patterns for cleanup: %w", err)
	}

	cleanedCount := 0
	for _, pattern := range patterns {
		if pattern.IsExpired() {
			err := r.DeletePattern(ctx, pattern.PatternID)
			if err != nil {
				r.logger.Warn("failed to cleanup expired pattern",
					zap.String("pattern_id", pattern.PatternID),
					zap.Error(err))
			} else {
				cleanedCount++
			}
		}
	}

	if cleanedCount > 0 {
		r.logger.Info("cleaned up expired patterns",
			zap.Int("count", cleanedCount))
	}

	return cleanedCount, nil
}

// GetPatternStatistics returns aggregate statistics for patterns
func (r *EnhancedPatternRepository) GetPatternStatistics(ctx context.Context) (map[string]interface{}, error) {
	patterns := []*models.EnhancedModerationPattern{}
	err := r.db.WithContext(ctx).Model(&models.EnhancedModerationPattern{}).
		Where("SK", "=", "METADATA").
		All(&patterns)

	if err != nil {
		return nil, fmt.Errorf("failed to get patterns for statistics: %w", err)
	}

	stats := map[string]interface{}{
		"total_patterns":          len(patterns),
		"active_patterns":         0,
		"average_effectiveness":   0.0,
		"total_matches":           int64(0),
		"total_false_positives":   int64(0),
		"total_true_positives":    int64(0),
		"patterns_by_type":        make(map[string]int),
		"patterns_by_category":    make(map[string]int),
		"patterns_by_severity":    make(map[string]int),
		"patterns_needing_review": 0,
	}

	var totalEffectiveness float64
	activeCount := 0

	for _, pattern := range patterns {
		if pattern.Active {
			activeCount++
			stats["active_patterns"] = activeCount
		}

		totalEffectiveness += pattern.Effectiveness
		stats["total_matches"] = stats["total_matches"].(int64) + pattern.MatchCount
		stats["total_false_positives"] = stats["total_false_positives"].(int64) + pattern.FalsePositiveCount
		stats["total_true_positives"] = stats["total_true_positives"].(int64) + pattern.TruePositiveCount

		// Count by type
		typeMap := stats["patterns_by_type"].(map[string]int)
		typeMap[pattern.PatternType]++

		// Count by category
		categoryMap := stats["patterns_by_category"].(map[string]int)
		categoryMap[pattern.Category]++

		// Count by severity
		severityMap := stats["patterns_by_severity"].(map[string]int)
		severityMap[pattern.Severity]++

		// Check if needs review (low effectiveness)
		if pattern.Effectiveness < 0.3 && pattern.MatchCount > 0 {
			stats["patterns_needing_review"] = stats["patterns_needing_review"].(int) + 1
		}
	}

	if len(patterns) > 0 {
		stats["average_effectiveness"] = totalEffectiveness / float64(len(patterns))
	}

	return stats, nil
}