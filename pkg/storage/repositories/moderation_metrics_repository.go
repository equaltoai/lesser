package repositories

import (
	"context"
	"fmt"
	"time"

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

// moderationMetricsRepository implements ModerationMetricsRepository
type moderationMetricsRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewModerationMetricsRepository creates a new moderation metrics repository
func NewModerationMetricsRepository(db core.DB, logger *zap.Logger) ModerationMetricsRepository {
	return &moderationMetricsRepository{
		db:     db,
		logger: logger,
	}
}

// RecordMetricsEntry records a single metrics entry
func (r *moderationMetricsRepository) RecordMetricsEntry(ctx context.Context, entry *models.ModerationMetricsEntry) error {
	entry.UpdateKeys()
	entry.CreatedAt = time.Now()
	
	err := r.db.WithContext(ctx).Model(entry).Create()
	if err != nil {
		r.logger.Error("failed to record metrics entry", 
			zap.String("metric_type", entry.MetricType),
			zap.Int64("count", entry.Count),
			zap.Error(err))
		return fmt.Errorf("record metrics entry: %w", err)
	}
	
	r.logger.Debug("recorded metrics entry",
		zap.String("metric_type", entry.MetricType),
		zap.Int64("count", entry.Count),
		zap.String("date", entry.Date),
		zap.String("hour", entry.Hour))
	
	return nil
}

// RecordMetricsEntries records multiple metrics entries in batch
func (r *moderationMetricsRepository) RecordMetricsEntries(ctx context.Context, entries []*models.ModerationMetricsEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Update keys and timestamps for all entries
	for _, entry := range entries {
		entry.UpdateKeys()
		entry.CreatedAt = time.Now()
	}

	// Use batch create for efficiency - process in batch
	for _, entry := range entries {
		err := r.db.WithContext(ctx).Model(entry).Create()
		if err != nil {
			r.logger.Error("failed to record metrics entry in batch",
				zap.String("metric_type", entry.MetricType),
				zap.Int64("count", entry.Count),
				zap.Error(err))
			return fmt.Errorf("record metrics entry in batch: %w", err)
		}
	}

	r.logger.Info("recorded metrics entries batch",
		zap.Int("count", len(entries)))

	return nil
}

// RecordFalsePositive records a false positive
func (r *moderationMetricsRepository) RecordFalsePositive(ctx context.Context, fp *models.ModerationFalsePositive) error {
	fp.UpdateKeys()
	fp.Timestamp = time.Now()

	err := r.db.WithContext(ctx).Model(fp).Create()
	if err != nil {
		r.logger.Error("failed to record false positive",
			zap.String("content_id", fp.ContentID),
			zap.String("decision", fp.OriginalDecision),
			zap.Error(err))
		return fmt.Errorf("record false positive: %w", err)
	}

	r.logger.Info("recorded false positive",
		zap.String("content_id", fp.ContentID),
		zap.String("decision", fp.OriginalDecision),
		zap.Float64("confidence", fp.Confidence))

	return nil
}

// GetFalsePositives retrieves false positives within a time range
func (r *moderationMetricsRepository) GetFalsePositives(ctx context.Context, timeRange models.ModerationMetricsTimeRange) ([]*models.ModerationFalsePositive, error) {
	var results []*models.ModerationFalsePositive

	// Query by GSI1 for efficient time range queries
	err := r.db.WithContext(ctx).Model(&models.ModerationFalsePositive{}).
		Where("GSI1PK", "=", "FALSE_POSITIVES").
		Where("GSI1SK", ">=", fmt.Sprintf("DATE#%s", timeRange.Start.Format("2006-01-02"))).
		Where("GSI1SK", "<=", fmt.Sprintf("DATE#%s#Z", timeRange.End.Format("2006-01-02"))).
		All(&results)

	if err != nil {
		r.logger.Error("failed to get false positives",
			zap.Time("start", timeRange.Start),
			zap.Time("end", timeRange.End),
			zap.Error(err))
		return nil, fmt.Errorf("get false positives: %w", err)
	}

	r.logger.Debug("retrieved false positives",
		zap.Int("count", len(results)),
		zap.Time("start", timeRange.Start),
		zap.Time("end", timeRange.End))

	return results, nil
}

// RecordDecisionSample records a decision sample
func (r *moderationMetricsRepository) RecordDecisionSample(ctx context.Context, sample *models.ModerationDecisionSample) error {
	sample.UpdateKeys()
	sample.Timestamp = time.Now()

	err := r.db.WithContext(ctx).Model(sample).Create()
	if err != nil {
		r.logger.Error("failed to record decision sample",
			zap.String("content_id", sample.ContentID),
			zap.String("decision", sample.Decision),
			zap.Error(err))
		return fmt.Errorf("record decision sample: %w", err)
	}

	r.logger.Debug("recorded decision sample",
		zap.String("content_id", sample.ContentID),
		zap.String("decision", sample.Decision),
		zap.Float64("confidence", sample.Confidence))

	return nil
}

// GetDecisionSamples retrieves decision samples within a time range
func (r *moderationMetricsRepository) GetDecisionSamples(ctx context.Context, timeRange models.ModerationMetricsTimeRange, decision string) ([]*models.ModerationDecisionSample, error) {
	var results []*models.ModerationDecisionSample

	if decision != "" {
		// Query by specific decision type using GSI1
		err := r.db.WithContext(ctx).Model(&models.ModerationDecisionSample{}).
			Where("GSI1PK", "=", fmt.Sprintf("DECISION#%s", decision)).
			Where("GSI1SK", ">=", fmt.Sprintf("DATE#%s", timeRange.Start.Format("2006-01-02"))).
			Where("GSI1SK", "<=", fmt.Sprintf("DATE#%s#Z", timeRange.End.Format("2006-01-02"))).
			All(&results)

		if err != nil {
			r.logger.Error("failed to get decision samples by decision type",
				zap.String("decision", decision),
				zap.Time("start", timeRange.Start),
				zap.Time("end", timeRange.End),
				zap.Error(err))
			return nil, fmt.Errorf("get decision samples: %w", err)
		}
	} else {
		// Query all decisions by date range using primary key
		// We need to iterate through each date since we can't query ranges across different partition keys
		current := timeRange.Start
		for current.Before(timeRange.End) || current.Equal(timeRange.End) {
			dateStr := current.Format("2006-01-02")
			var dayResults []*models.ModerationDecisionSample

			err := r.db.WithContext(ctx).Model(&models.ModerationDecisionSample{}).
				Where("PK", "=", fmt.Sprintf("SAMPLES#%s", dateStr)).
				All(&dayResults)

			if err != nil {
				r.logger.Warn("failed to get decision samples for date",
					zap.String("date", dateStr),
					zap.Error(err))
			} else {
				results = append(results, dayResults...)
			}

			current = current.AddDate(0, 0, 1)
		}
	}

	r.logger.Debug("retrieved decision samples",
		zap.Int("count", len(results)),
		zap.String("decision", decision),
		zap.Time("start", timeRange.Start),
		zap.Time("end", timeRange.End))

	return results, nil
}

// UpdatePatternStats updates pattern statistics
func (r *moderationMetricsRepository) UpdatePatternStats(ctx context.Context, stats *models.ModerationPatternStats) error {
	stats.UpdateKeys()
	stats.UpdatedAt = time.Now()

	err := r.db.WithContext(ctx).Model(stats).Update()

	if err != nil {
		r.logger.Error("failed to update pattern stats",
			zap.String("pattern_id", stats.PatternID),
			zap.Int64("hit_count", stats.HitCount),
			zap.Error(err))
		return fmt.Errorf("update pattern stats: %w", err)
	}

	r.logger.Debug("updated pattern stats",
		zap.String("pattern_id", stats.PatternID),
		zap.Int64("hit_count", stats.HitCount))

	return nil
}

// GetTopPatterns retrieves the top patterns by hit count
func (r *moderationMetricsRepository) GetTopPatterns(ctx context.Context, limit int) ([]*models.ModerationPatternStats, error) {
	var results []*models.ModerationPatternStats

	// Query by GSI1 ordered by hit count (descending)
	err := r.db.WithContext(ctx).Model(&models.ModerationPatternStats{}).
		Where("GSI1PK", "=", "PATTERN_HITS").
		Limit(limit).
		All(&results)

	if err != nil {
		r.logger.Error("failed to get top patterns",
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, fmt.Errorf("get top patterns: %w", err)
	}

	r.logger.Debug("retrieved top patterns",
		zap.Int("count", len(results)),
		zap.Int("limit", limit))

	return results, nil
}

// IncrementPatternHit increments the hit count for a pattern
func (r *moderationMetricsRepository) IncrementPatternHit(ctx context.Context, patternID, patternName string) error {
	// Try to increment existing record
	pk := fmt.Sprintf("PATTERN_STATS#%s", patternID)
	sk := "STATS"

	// First try to get existing stats
	var existing models.ModerationPatternStats
	err := r.db.WithContext(ctx).Model(&models.ModerationPatternStats{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&existing)

	if err != nil {
		// Create new stats record
		stats := &models.ModerationPatternStats{
			PatternID:   patternID,
			PatternName: patternName,
			HitCount:    1,
			LastHit:     time.Now(),
			CreatedAt:   time.Now(),
		}
		stats.UpdateKeys()

		err = r.db.WithContext(ctx).Model(stats).Create()
		if err != nil {
			r.logger.Error("failed to create pattern stats",
				zap.String("pattern_id", patternID),
				zap.Error(err))
			return fmt.Errorf("create pattern stats: %w", err)
		}

		r.logger.Debug("created new pattern stats",
			zap.String("pattern_id", patternID),
			zap.String("pattern_name", patternName))
	} else {
		// Update existing record
		existing.HitCount++
		existing.LastHit = time.Now()
		existing.UpdatedAt = time.Now()
		existing.UpdateKeys() // Recalculate GSI keys

		err = r.db.WithContext(ctx).Model(&existing).Update()

		if err != nil {
			r.logger.Error("failed to increment pattern hit count",
				zap.String("pattern_id", patternID),
				zap.Int64("new_count", existing.HitCount),
				zap.Error(err))
			return fmt.Errorf("increment pattern hit: %w", err)
		}

		r.logger.Debug("incremented pattern hit count",
			zap.String("pattern_id", patternID),
			zap.Int64("new_count", existing.HitCount))
	}

	return nil
}

// GetMetricsEntries retrieves metrics entries within a time range
func (r *moderationMetricsRepository) GetMetricsEntries(ctx context.Context, timeRange models.ModerationMetricsTimeRange, metricTypes []string) ([]*models.ModerationMetricsEntry, error) {
	var allResults []*models.ModerationMetricsEntry

	if len(metricTypes) > 0 {
		// Query by specific metric types using GSI1
		for _, metricType := range metricTypes {
			var results []*models.ModerationMetricsEntry
			err := r.db.WithContext(ctx).Model(&models.ModerationMetricsEntry{}).
				Where("GSI1PK", "=", fmt.Sprintf("METRIC_TYPE#%s", metricType)).
				Where("GSI1SK", ">=", fmt.Sprintf("DATE#%s", timeRange.Start.Format("2006-01-02"))).
				Where("GSI1SK", "<=", fmt.Sprintf("DATE#%s#Z", timeRange.End.Format("2006-01-02"))).
				All(&results)

			if err != nil {
				r.logger.Warn("failed to get metrics entries for type",
					zap.String("metric_type", metricType),
					zap.Error(err))
				continue
			}

			allResults = append(allResults, results...)
		}
	} else {
		// Query all metrics by date range using primary key
		current := timeRange.Start
		for current.Before(timeRange.End) || current.Equal(timeRange.End) {
			dateStr := current.Format("2006-01-02")
			var dayResults []*models.ModerationMetricsEntry

			err := r.db.WithContext(ctx).Model(&models.ModerationMetricsEntry{}).
				Where("PK", "=", fmt.Sprintf("METRICS#%s", dateStr)).
				Where("SK", "begins_with", "STATS#").
				All(&dayResults)

			if err != nil {
				r.logger.Warn("failed to get metrics entries for date",
					zap.String("date", dateStr),
					zap.Error(err))
			} else {
				allResults = append(allResults, dayResults...)
			}

			current = current.AddDate(0, 0, 1)
		}
	}

	r.logger.Debug("retrieved metrics entries",
		zap.Int("count", len(allResults)),
		zap.Strings("metric_types", metricTypes),
		zap.Time("start", timeRange.Start),
		zap.Time("end", timeRange.End))

	return allResults, nil
}

// GetAggregatedStats retrieves and aggregates statistics within a time range
func (r *moderationMetricsRepository) GetAggregatedStats(ctx context.Context, timeRange models.ModerationMetricsTimeRange) (*models.ModerationMetricsStats, error) {
	// Get all metrics entries for the time range
	entries, err := r.GetMetricsEntries(ctx, timeRange, nil)
	if err != nil {
		return nil, fmt.Errorf("get metrics entries: %w", err)
	}

	// Get false positives
	falsePositives, err := r.GetFalsePositives(ctx, timeRange)
	if err != nil {
		return nil, fmt.Errorf("get false positives: %w", err)
	}

	// Initialize stats
	stats := &models.ModerationMetricsStats{
		TimeRange:      timeRange,
		ActionCounts:   make(map[models.AdvancedModerationAction]int64),
		CategoryCounts: make(map[string]int64),
		SeverityCounts: make(map[models.AdvancedSeverity]int64),
	}

	// Aggregate metrics entries
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

	r.logger.Debug("aggregated stats",
		zap.Int64("total_analyzed", stats.TotalAnalyzed),
		zap.Int64("false_positives", stats.FalsePositives),
		zap.Float64("avg_confidence", stats.AverageConfidence),
		zap.Int("action_types", len(stats.ActionCounts)),
		zap.Int("categories", len(stats.CategoryCounts)))

	return stats, nil
}