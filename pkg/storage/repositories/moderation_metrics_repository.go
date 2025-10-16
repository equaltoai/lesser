package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// ModerationMetricsRepository interface defines methods for moderation metrics operations
type ModerationMetricsRepository interface {
	// Metrics recording
	RecordMetricsEntry(ctx context.Context, entry *models.ModerationMetricsEntry) error
	RecordMetricsEntries(ctx context.Context, entries []*models.ModerationMetricsEntry) error

	// False positive tracking
	RecordFalsePositive(ctx context.Context, fp *models.ModerationFalsePositive) error
	GetFalsePositives(ctx context.Context, timeRange models.ModerationMetricsTimeRange) ([]*models.ModerationFalsePositive, error)

	// Decision sampling
	RecordDecisionSample(ctx context.Context, sample *models.ModerationDecisionSample) error
	GetDecisionSamples(ctx context.Context, timeRange models.ModerationMetricsTimeRange, decision string) ([]*models.ModerationDecisionSample, error)

	// Pattern statistics
	UpdatePatternStats(ctx context.Context, stats *models.ModerationPatternStats) error
	GetTopPatterns(ctx context.Context, limit int) ([]*models.ModerationPatternStats, error)
	IncrementPatternHit(ctx context.Context, patternID, patternName string) error

	// Statistics retrieval
	GetMetricsEntries(ctx context.Context, timeRange models.ModerationMetricsTimeRange, metricTypes []string) ([]*models.ModerationMetricsEntry, error)
	GetAggregatedStats(ctx context.Context, timeRange models.ModerationMetricsTimeRange) (*models.ModerationMetricsStats, error)
}

// moderationMetricsRepository implements moderation metrics operations using enhanced patterns
type moderationMetricsRepository struct {
	// Embed EnhancedBaseRepository for different model types - we'll use ModerationMetricsEntry as the primary type
	*EnhancedBaseRepository[*models.ModerationMetricsEntry]

	// Additional enhanced repositories for other model types
	falsePositiveRepo  *EnhancedBaseRepository[*models.ModerationFalsePositive]
	decisionSampleRepo *EnhancedBaseRepository[*models.ModerationDecisionSample]
	patternStatsRepo   *EnhancedBaseRepository[*models.ModerationPatternStats]
}

// NewModerationMetricsRepository creates a new moderation metrics repository with optional cost tracking
// For backward compatibility, supports both old signature (db, logger) and new signature (db, tableName, logger, costService)
func NewModerationMetricsRepository(args ...interface{}) ModerationMetricsRepository {
	var db core.DB
	var tableName string
	var logger *zap.Logger
	var costService *cost.TrackingService

	// Handle different argument signatures for backward compatibility
	switch len(args) {
	case 2:
		// Old signature: (db, logger)
		db = args[0].(core.DB)
		logger = args[1].(*zap.Logger)
		tableName = models.MainTableName // Use default table name
		costService = nil                // No cost tracking
	case 4:
		// New signature: (db, tableName, logger, costService)
		db = args[0].(core.DB)
		tableName = args[1].(string)
		logger = args[2].(*zap.Logger)
		costService = args[3].(*cost.TrackingService)
	default:
		// Return nil and let caller handle the error
		if len(args) > 0 {
			if logger, ok := args[2].(*zap.Logger); ok {
				logger.Error("NewModerationMetricsRepository: invalid number of arguments",
					zap.Int("arg_count", len(args)))
			}
		}
		return nil
	}

	// Create enhanced repositories for moderation metrics operations
	metricsRepo := NewEnhancedBaseRepository[*models.ModerationMetricsEntry](db, tableName, logger, costService, "ModerationMetricsRepository", "moderationmetrics")
	metricsRepo.SetValidationService(NewDefaultValidationService())
	metricsRepo.SetPermissionService(NewDefaultPermissionService())
	metricsRepo.SetCachingService(NewInMemoryCachingService())
	metricsRepo.SetEventService(NewDefaultEventService())

	falsePositiveRepo := NewEnhancedBaseRepository[*models.ModerationFalsePositive](db, tableName, logger, costService, "ModerationFalsePositiveRepository", "moderationfalsepositive")
	falsePositiveRepo.SetValidationService(NewDefaultValidationService())
	falsePositiveRepo.SetPermissionService(NewDefaultPermissionService())
	falsePositiveRepo.SetCachingService(NewInMemoryCachingService())
	falsePositiveRepo.SetEventService(NewDefaultEventService())

	decisionSampleRepo := NewEnhancedBaseRepository[*models.ModerationDecisionSample](db, tableName, logger, costService, "ModerationDecisionSampleRepository", "moderationdecisionsample")
	decisionSampleRepo.SetValidationService(NewDefaultValidationService())
	decisionSampleRepo.SetPermissionService(NewDefaultPermissionService())
	decisionSampleRepo.SetCachingService(NewInMemoryCachingService())
	decisionSampleRepo.SetEventService(NewDefaultEventService())

	patternStatsRepo := NewEnhancedBaseRepository[*models.ModerationPatternStats](db, tableName, logger, costService, "ModerationPatternStatsRepository", "moderationpatternstats")
	patternStatsRepo.SetValidationService(NewDefaultValidationService())
	patternStatsRepo.SetPermissionService(NewDefaultPermissionService())
	patternStatsRepo.SetCachingService(NewInMemoryCachingService())
	patternStatsRepo.SetEventService(NewDefaultEventService())

	return &moderationMetricsRepository{
		EnhancedBaseRepository: metricsRepo,
		falsePositiveRepo:      falsePositiveRepo,
		decisionSampleRepo:     decisionSampleRepo,
		patternStatsRepo:       patternStatsRepo,
	}
}

// RecordMetricsEntry records a single metrics entry
func (r *moderationMetricsRepository) RecordMetricsEntry(ctx context.Context, entry *models.ModerationMetricsEntry) error {
	entry.CreatedAt = time.Now()
	return r.ValidateAndCreate(ctx, entry)
}

// RecordMetricsEntries records multiple metrics entries in batch
func (r *moderationMetricsRepository) RecordMetricsEntries(ctx context.Context, entries []*models.ModerationMetricsEntry) error {
	if err := common.ValidateSliceNotEmpty("entries", entries); err != nil {
		return nil
	}

	// Update timestamps for all entries
	for _, entry := range entries {
		entry.CreatedAt = time.Now()
	}

	return r.BatchCreate(ctx, entries)
}

// RecordFalsePositive records a false positive using dedicated repository
func (r *moderationMetricsRepository) RecordFalsePositive(ctx context.Context, fp *models.ModerationFalsePositive) error {
	fp.Timestamp = time.Now()
	return r.falsePositiveRepo.ValidateAndCreate(ctx, fp)
}

// GetFalsePositives retrieves false positives within a time range
func (r *moderationMetricsRepository) GetFalsePositives(ctx context.Context, timeRange models.ModerationMetricsTimeRange) ([]*models.ModerationFalsePositive, error) {
	var results []*models.ModerationFalsePositive

	// Query by GSI1 for efficient time range queries
	err := r.falsePositiveRepo.GetDB().WithContext(ctx).Model(&models.ModerationFalsePositive{}).
		Index("gsi1").
		Where("GSI1PK", "=", "FALSE_POSITIVES").
		Where("GSI1SK", ">=", fmt.Sprintf("DATE#%s", timeRange.Start.Format(common.DateFormat))).
		Where("GSI1SK", "<=", fmt.Sprintf("DATE#%s#Z", timeRange.End.Format(common.DateFormat))).
		All(&results)

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrModerationMetricsFalsePositivesQueryFailed, err)
	}

	return results, nil
}

// RecordDecisionSample records a decision sample using dedicated repository
func (r *moderationMetricsRepository) RecordDecisionSample(ctx context.Context, sample *models.ModerationDecisionSample) error {
	sample.Timestamp = time.Now()
	return r.decisionSampleRepo.ValidateAndCreate(ctx, sample)
}

// GetDecisionSamples retrieves decision samples within a time range
func (r *moderationMetricsRepository) GetDecisionSamples(ctx context.Context, timeRange models.ModerationMetricsTimeRange, decision string) ([]*models.ModerationDecisionSample, error) {
	var results []*models.ModerationDecisionSample

	if decision != "" {
		// Query by specific decision type using GSI1
		err := r.decisionSampleRepo.GetDB().WithContext(ctx).Model(&models.ModerationDecisionSample{}).
			Index("gsi1").
			Where("GSI1PK", "=", fmt.Sprintf("DECISION#%s", decision)).
			Where("GSI1SK", ">=", fmt.Sprintf("DATE#%s", timeRange.Start.Format(common.DateFormat))).
			Where("GSI1SK", "<=", fmt.Sprintf("DATE#%s#Z", timeRange.End.Format(common.DateFormat))).
			All(&results)

		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrModerationMetricsDecisionSamplesQueryFailed, err)
		}
	} else {
		// Query all decisions by date range using primary key
		// We need to iterate through each date since we can't query ranges across different partition keys
		current := timeRange.Start
		for current.Before(timeRange.End) || current.Equal(timeRange.End) {
			dateStr := current.Format(common.DateFormat)
			var dayResults []*models.ModerationDecisionSample

			err := r.decisionSampleRepo.GetDB().WithContext(ctx).Model(&models.ModerationDecisionSample{}).
				Where("PK", "=", fmt.Sprintf("SAMPLES#%s", dateStr)).
				All(&dayResults)

			if err == nil {
				results = append(results, dayResults...)
			}

			current = current.AddDate(0, 0, 1)
		}
	}

	return results, nil
}

// UpdatePatternStats updates pattern statistics using dedicated repository
func (r *moderationMetricsRepository) UpdatePatternStats(ctx context.Context, stats *models.ModerationPatternStats) error {
	stats.UpdatedAt = time.Now()
	return r.patternStatsRepo.Update(ctx, stats)
}

// GetTopPatterns retrieves the top patterns by hit count
func (r *moderationMetricsRepository) GetTopPatterns(ctx context.Context, limit int) ([]*models.ModerationPatternStats, error) {
	var results []*models.ModerationPatternStats

	// Query by GSI1 ordered by hit count (descending)
	err := r.patternStatsRepo.GetDB().WithContext(ctx).Model(&models.ModerationPatternStats{}).
		Index("gsi1").
		Where("GSI1PK", "=", "PATTERN_HITS").
		Limit(limit).
		All(&results)

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrModerationMetricsTopPatternsQueryFailed, err)
	}

	return results, nil
}

// IncrementPatternHit increments the hit count for a pattern - preserved business logic
func (r *moderationMetricsRepository) IncrementPatternHit(ctx context.Context, patternID, patternName string) error {
	// Try to get existing stats first
	pk := fmt.Sprintf("PATTERN_STATS#%s", patternID)
	sk := "STATS"

	var existing models.ModerationPatternStats
	err := r.patternStatsRepo.GetDB().WithContext(ctx).Model(&models.ModerationPatternStats{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&existing)

	if err != nil {
		// Create new stats record using BaseRepository
		stats := &models.ModerationPatternStats{
			PatternID:   patternID,
			PatternName: patternName,
			HitCount:    1,
			LastHit:     time.Now(),
			CreatedAt:   time.Now(),
		}

		return r.patternStatsRepo.ValidateAndCreate(ctx, stats)
	}
	// Update existing record using BaseRepository
	existing.HitCount++
	existing.LastHit = time.Now()
	existing.UpdatedAt = time.Now()

	return r.patternStatsRepo.Update(ctx, &existing)
}

// GetMetricsEntries retrieves metrics entries within a time range - preserved complex business logic
func (r *moderationMetricsRepository) GetMetricsEntries(ctx context.Context, timeRange models.ModerationMetricsTimeRange, metricTypes []string) ([]*models.ModerationMetricsEntry, error) {
	var allResults []*models.ModerationMetricsEntry

	if len(metricTypes) > 0 {
		// Query by specific metric types using GSI1
		for _, metricType := range metricTypes {
			var results []*models.ModerationMetricsEntry
			err := r.GetDB().WithContext(ctx).Model(&models.ModerationMetricsEntry{}).
				Index("gsi1").
				Where("GSI1PK", "=", fmt.Sprintf("METRIC_TYPE#%s", metricType)).
				Where("GSI1SK", ">=", fmt.Sprintf("DATE#%s", timeRange.Start.Format(common.DateFormat))).
				Where("GSI1SK", "<=", fmt.Sprintf("DATE#%s#Z", timeRange.End.Format(common.DateFormat))).
				All(&results)

			if err == nil {
				allResults = append(allResults, results...)
			}
		}
	} else {
		// Query all metrics by date range using primary key
		current := timeRange.Start
		for current.Before(timeRange.End) || current.Equal(timeRange.End) {
			dateStr := current.Format(common.DateFormat)
			var dayResults []*models.ModerationMetricsEntry

			err := r.GetDB().WithContext(ctx).Model(&models.ModerationMetricsEntry{}).
				Where("PK", "=", fmt.Sprintf("METRICS#%s", dateStr)).
				Where("SK", "begins_with", "STATS#").
				All(&dayResults)

			if err == nil {
				allResults = append(allResults, dayResults...)
			}

			current = current.AddDate(0, 0, 1)
		}
	}

	return allResults, nil
}

// GetAggregatedStats retrieves and aggregates statistics within a time range - preserved complex business logic
func (r *moderationMetricsRepository) GetAggregatedStats(ctx context.Context, timeRange models.ModerationMetricsTimeRange) (*models.ModerationMetricsStats, error) {
	// Get all metrics entries for the time range
	entries, err := r.GetMetricsEntries(ctx, timeRange, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrModerationMetricsEntriesQueryFailed, err)
	}

	// Get false positives
	falsePositives, err := r.GetFalsePositives(ctx, timeRange)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrModerationMetricsFalsePositivesQueryFailed, err)
	}

	// Initialize stats - preserved exact logic
	stats := &models.ModerationMetricsStats{
		TimeRange:      timeRange,
		ActionCounts:   make(map[models.AdvancedModerationAction]int64),
		CategoryCounts: make(map[string]int64),
		SeverityCounts: make(map[models.AdvancedSeverity]int64),
	}

	// Aggregate metrics entries - preserved exact aggregation logic
	var totalAnalyzed int64
	var totalConfidence float64
	var confidenceCount int64

	for _, entry := range entries {
		switch {
		case entry.MetricType == "content_type:text":
			totalAnalyzed += entry.Count

		case len(entry.MetricType) > 9 && entry.MetricType[:9] == "decision:":
			action := models.AdvancedModerationAction(entry.MetricType[9:])
			stats.ActionCounts[action] += entry.Count

		case len(entry.MetricType) > 9 && entry.MetricType[:9] == "severity:":
			severity := models.AdvancedSeverity(entry.MetricType[9:])
			stats.SeverityCounts[severity] += entry.Count

		case len(entry.MetricType) > 12 && entry.MetricType[:12] == "reason_type:":
			category := entry.MetricType[12:]
			stats.CategoryCounts[category] += entry.Count

		case len(entry.MetricType) > 11 && entry.MetricType[:11] == "confidence:":
			// Parse confidence value and weight by count
			// This is a simplified aggregation - in practice you'd parse the confidence value
			totalConfidence += 0.8 * float64(entry.Count) // Placeholder
			confidenceCount += entry.Count
		}
	}

	stats.TotalAnalyzed = totalAnalyzed
	stats.FalsePositives = int64(len(falsePositives))

	// Calculate average confidence
	if confidenceCount > 0 {
		stats.AverageConfidence = totalConfidence / float64(confidenceCount)
	}

	return stats, nil
}
