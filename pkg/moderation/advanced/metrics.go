package advanced

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// ModerationMetrics tracks moderation system performance using DynamORM
type ModerationMetrics struct {
	repo   repositories.ModerationMetricsRepository
	logger *zap.Logger

	// In-memory counters for current period
	counters  sync.Map
	startTime time.Time

	// Atomic counters for real-time metrics
	totalAnalyzed    atomic.Int64
	totalAllowed     atomic.Int64
	totalFlagged     atomic.Int64
	totalRemoved     atomic.Int64
	totalQuarantined atomic.Int64

	// Response time tracking
	responseTimes  []time.Duration
	responseTimeMu sync.Mutex
}

// NewModerationMetrics creates a new metrics tracker
func NewModerationMetrics(repo repositories.ModerationMetricsRepository, logger *zap.Logger) *ModerationMetrics {
	return &ModerationMetrics{
		repo:          repo,
		logger:        logger,
		startTime:     time.Now(),
		responseTimes: make([]time.Duration, 0, 1000),
	}
}

// RecordAnalysis records an analysis event
func (mm *ModerationMetrics) RecordAnalysis(ctx context.Context, contentType string, processingTime time.Duration, decision *ModerationDecision) {
	// Update atomic counters
	mm.totalAnalyzed.Add(1)

	switch decision.Decision {
	case ActionAllow:
		mm.totalAllowed.Add(1)
	case ActionFlag:
		mm.totalFlagged.Add(1)
	case ActionRemove:
		mm.totalRemoved.Add(1)
	case ActionQuarantine:
		mm.totalQuarantined.Add(1)
	}

	// Record response time
	mm.responseTimeMu.Lock()
	mm.responseTimes = append(mm.responseTimes, processingTime)
	if len(mm.responseTimes) > 10000 {
		// Keep only last 10k measurements
		mm.responseTimes = mm.responseTimes[5000:]
	}
	mm.responseTimeMu.Unlock()

	// Update detailed counters
	mm.incrementCounter(fmt.Sprintf("content_type:%s", contentType))
	mm.incrementCounter(fmt.Sprintf("decision:%s", decision.Decision))
	mm.incrementCounter(fmt.Sprintf("confidence:%.1f", roundToNearest(decision.Confidence, 0.1)))

	// Track severity distribution
	for _, reason := range decision.Reasons {
		mm.incrementCounter(fmt.Sprintf("severity:%s", reason.Severity))
		mm.incrementCounter(fmt.Sprintf("reason_type:%s", reason.Type))
	}

	// Track review requirements
	if decision.RequiresReview {
		mm.incrementCounter("requires_review")
		mm.incrementCounter(fmt.Sprintf("review_priority:%d", decision.ReviewPriority))
	}

	// Store decision for later analysis if significant
	if decision.Decision != ActionAllow || decision.Confidence < 0.5 {
		mm.storeDecisionSampleAsync(ctx, decision, processingTime)
	}
}

// RecordFalsePositive records a false positive
func (mm *ModerationMetrics) RecordFalsePositive(ctx context.Context, contentID string, originalDecision *ModerationDecision) {
	mm.incrementCounter("false_positives")
	mm.incrementCounter(fmt.Sprintf("false_positive:%s", originalDecision.Decision))

	// Store for analysis
	fp := &models.ModerationFalsePositive{
		ContentID:        contentID,
		OriginalDecision: string(originalDecision.Decision),
		Confidence:       originalDecision.Confidence,
		Date:             time.Now().Format(common.DateFormat),
	}

	err := mm.repo.RecordFalsePositive(ctx, fp)
	if err != nil {
		mm.logger.Warn("failed to store false positive", zap.Error(err))
	}
}

// RecordTruePositive records a true positive (confirmed violation)
func (mm *ModerationMetrics) RecordTruePositive(_ context.Context, _ string, decision *ModerationDecision) {
	mm.incrementCounter("true_positives")
	mm.incrementCounter(fmt.Sprintf("true_positive:%s", decision.Decision))
}

// GetStats retrieves moderation statistics for a time range
func (mm *ModerationMetrics) GetStats(ctx context.Context, timeRange TimeRange) (*ModerationStats, error) {
	// Get aggregated stats from repository
	stats, err := mm.repo.GetAggregatedStats(ctx, models.ModerationMetricsTimeRange{
		Start: timeRange.Start,
		End:   timeRange.End,
	})
	if err != nil {
		return nil, fmt.Errorf("get aggregated stats: %w", err)
	}

	// Convert back to advanced.ModerationStats
	result := &ModerationStats{
		TimeRange:         timeRange,
		TotalAnalyzed:     stats.TotalAnalyzed,
		ActionCounts:      make(map[ModerationAction]int64),
		CategoryCounts:    stats.CategoryCounts,
		SeverityCounts:    make(map[Severity]int64),
		AverageConfidence: stats.AverageConfidence,
		FalsePositives:    stats.FalsePositives,
		TruePositives:     stats.TruePositives,
		ResponseTime:      stats.ResponseTime,
	}

	// Convert action counts
	for action, count := range stats.ActionCounts {
		result.ActionCounts[ModerationAction(action)] = count
	}

	// Convert severity counts
	for severity, count := range stats.SeverityCounts {
		result.SeverityCounts[Severity(severity)] = count
	}

	// Add current period stats if within range
	if mm.startTime.After(timeRange.Start) && mm.startTime.Before(timeRange.End) {
		mm.addCurrentPeriodStats(result)
	}

	// Calculate average response time
	mm.responseTimeMu.Lock()
	if len(mm.responseTimes) > 0 {
		var total time.Duration
		for _, rt := range mm.responseTimes {
			total += rt
		}
		result.ResponseTime = total / time.Duration(len(mm.responseTimes))
	}
	mm.responseTimeMu.Unlock()

	return result, nil
}

// GetRealtimeStats returns current real-time statistics
func (mm *ModerationMetrics) GetRealtimeStats() *RealtimeStats {
	uptime := time.Since(mm.startTime)

	total := mm.totalAnalyzed.Load()
	if total == 0 {
		total = 1 // Prevent division by zero
	}

	stats := &RealtimeStats{
		Uptime:         uptime,
		TotalAnalyzed:  total,
		AnalysisRate:   float64(total) / uptime.Seconds(),
		AllowRate:      float64(mm.totalAllowed.Load()) / float64(total),
		FlagRate:       float64(mm.totalFlagged.Load()) / float64(total),
		RemoveRate:     float64(mm.totalRemoved.Load()) / float64(total),
		QuarantineRate: float64(mm.totalQuarantined.Load()) / float64(total),
	}

	// Calculate current response time
	mm.responseTimeMu.Lock()
	if len(mm.responseTimes) > 0 {
		// Get last 100 response times
		start := len(mm.responseTimes) - 100
		if start < 0 {
			start = 0
		}
		recent := mm.responseTimes[start:]

		var total time.Duration
		for _, rt := range recent {
			total += rt
		}
		stats.AvgResponseTime = total / time.Duration(len(recent))

		// Find p95
		if len(recent) >= 20 {
			sorted := make([]time.Duration, len(recent))
			copy(sorted, recent)
			// Simple bubble sort for small dataset
			for i := 0; i < len(sorted)-1; i++ {
				for j := i + 1; j < len(sorted); j++ {
					if sorted[i] > sorted[j] {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}
			p95Index := int(float64(len(sorted)) * 0.95)
			stats.P95ResponseTime = sorted[p95Index]
		}
	}
	mm.responseTimeMu.Unlock()

	return stats
}

// GetTopPatterns returns the most frequently matched patterns
func (mm *ModerationMetrics) GetTopPatterns(ctx context.Context, limit int) ([]PatternStats, error) {
	patterns, err := mm.repo.GetTopPatterns(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("get top patterns: %w", err)
	}

	result := make([]PatternStats, len(patterns))
	for i, p := range patterns {
		result[i] = PatternStats{
			PatternID:   p.PatternID,
			PatternName: p.PatternName,
			HitCount:    p.HitCount,
			LastHit:     p.LastHit,
		}
	}

	return result, nil
}

// FlushMetrics flushes accumulated counters to persistent storage
// This replaces the background goroutine approach for Lambda compatibility
func (mm *ModerationMetrics) FlushMetrics(ctx context.Context) error {
	// Get current date and hour for partition key
	now := time.Now()
	date := now.Format(common.DateFormat)
	hour := now.Format("15")

	var entries []*models.ModerationMetricsEntry

	// Flush all counters
	mm.counters.Range(func(key, value any) bool {
		counter := value.(*atomic.Int64)
		count := counter.Swap(0) // Reset counter

		if count == 0 {
			return true
		}

		keyStr := key.(string)
		entry := &models.ModerationMetricsEntry{
			MetricType: keyStr,
			Count:      count,
			Hour:       hour,
			Date:       date,
		}

		entries = append(entries, entry)
		return true
	})

	if err := common.ValidateSliceNotEmpty("entries", entries); err != nil {
		return nil
	}

	// Write in batch
	err := mm.repo.RecordMetricsEntries(ctx, entries)
	if err != nil {
		mm.logger.Error("failed to flush metrics", zap.Error(err))
		return fmt.Errorf("flush metrics: %w", err)
	}

	mm.logger.Info("flushed metrics",
		zap.Int("counters", len(entries)),
		zap.String("date", date),
		zap.String("hour", hour))

	return nil
}

// Helper methods

func (mm *ModerationMetrics) incrementCounter(key string) {
	val, _ := mm.counters.LoadOrStore(key, &atomic.Int64{})
	counter := val.(*atomic.Int64)
	counter.Add(1)
}

func (mm *ModerationMetrics) getCounter(key string) int64 {
	val, ok := mm.counters.Load(key)
	if !ok {
		return 0
	}
	return val.(*atomic.Int64).Load()
}

func (mm *ModerationMetrics) storeDecisionSampleAsync(ctx context.Context, decision *ModerationDecision, processingTime time.Duration) {
	// Store a sample of decisions for analysis
	sample := &models.ModerationDecisionSample{
		ContentID:      decision.ContentID,
		Decision:       string(decision.Decision),
		Confidence:     decision.Confidence,
		ProcessingTime: processingTime.Milliseconds(),
		ReasonCount:    len(decision.Reasons),
		RequiresReview: decision.RequiresReview,
		Date:           time.Now().Format(common.DateFormat),
	}

	// Use goroutine for async storage (acceptable for non-critical data)
	go func() {
		err := mm.repo.RecordDecisionSample(ctx, sample)
		if err != nil {
			mm.logger.Warn("failed to store decision sample", zap.Error(err))
		}
	}()
}

func (mm *ModerationMetrics) addCurrentPeriodStats(stats *ModerationStats) {
	// Add current in-memory stats
	stats.TotalAnalyzed += mm.totalAnalyzed.Load()
	stats.ActionCounts[ActionAllow] += mm.totalAllowed.Load()
	stats.ActionCounts[ActionFlag] += mm.totalFlagged.Load()
	stats.ActionCounts[ActionRemove] += mm.totalRemoved.Load()
	stats.ActionCounts[ActionQuarantine] += mm.totalQuarantined.Load()

	// Add other counters
	stats.FalsePositives += mm.getCounter("false_positives")
	stats.TruePositives += mm.getCounter("true_positives")
}

func roundToNearest(value, nearest float64) float64 {
	return float64(int(value/nearest+0.5)) * nearest
}
