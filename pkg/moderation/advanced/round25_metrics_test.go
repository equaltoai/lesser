package advanced

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeModerationMetricsRepo struct {
	recordMetricsEntryFn    func(context.Context, *models.ModerationMetricsEntry) error
	recordMetricsEntriesFn  func(context.Context, []*models.ModerationMetricsEntry) error
	recordFalsePositiveFn   func(context.Context, *models.ModerationFalsePositive) error
	getFalsePositivesFn     func(context.Context, models.ModerationMetricsTimeRange) ([]*models.ModerationFalsePositive, error)
	recordDecisionSampleFn  func(context.Context, *models.ModerationDecisionSample) error
	getDecisionSamplesFn    func(context.Context, models.ModerationMetricsTimeRange, string) ([]*models.ModerationDecisionSample, error)
	updatePatternStatsFn    func(context.Context, *models.ModerationPatternStats) error
	getTopPatternsFn        func(context.Context, int) ([]*models.ModerationPatternStats, error)
	incrementPatternHitFn   func(context.Context, string, string) error
	getMetricsEntriesFn     func(context.Context, models.ModerationMetricsTimeRange, []string) ([]*models.ModerationMetricsEntry, error)
	getAggregatedStatsFn    func(context.Context, models.ModerationMetricsTimeRange) (*models.ModerationMetricsStats, error)
	defaultAggregatedResult *models.ModerationMetricsStats
}

func (f *fakeModerationMetricsRepo) RecordMetricsEntry(ctx context.Context, entry *models.ModerationMetricsEntry) error {
	if f.recordMetricsEntryFn != nil {
		return f.recordMetricsEntryFn(ctx, entry)
	}
	return nil
}

func (f *fakeModerationMetricsRepo) RecordMetricsEntries(ctx context.Context, entries []*models.ModerationMetricsEntry) error {
	if f.recordMetricsEntriesFn != nil {
		return f.recordMetricsEntriesFn(ctx, entries)
	}
	return nil
}

func (f *fakeModerationMetricsRepo) RecordFalsePositive(ctx context.Context, fp *models.ModerationFalsePositive) error {
	if f.recordFalsePositiveFn != nil {
		return f.recordFalsePositiveFn(ctx, fp)
	}
	return nil
}

func (f *fakeModerationMetricsRepo) GetFalsePositives(ctx context.Context, timeRange models.ModerationMetricsTimeRange) ([]*models.ModerationFalsePositive, error) {
	if f.getFalsePositivesFn != nil {
		return f.getFalsePositivesFn(ctx, timeRange)
	}
	return nil, nil
}

func (f *fakeModerationMetricsRepo) RecordDecisionSample(ctx context.Context, sample *models.ModerationDecisionSample) error {
	if f.recordDecisionSampleFn != nil {
		return f.recordDecisionSampleFn(ctx, sample)
	}
	return nil
}

func (f *fakeModerationMetricsRepo) GetDecisionSamples(ctx context.Context, timeRange models.ModerationMetricsTimeRange, decision string) ([]*models.ModerationDecisionSample, error) {
	if f.getDecisionSamplesFn != nil {
		return f.getDecisionSamplesFn(ctx, timeRange, decision)
	}
	return nil, nil
}

func (f *fakeModerationMetricsRepo) UpdatePatternStats(ctx context.Context, stats *models.ModerationPatternStats) error {
	if f.updatePatternStatsFn != nil {
		return f.updatePatternStatsFn(ctx, stats)
	}
	return nil
}

func (f *fakeModerationMetricsRepo) GetTopPatterns(ctx context.Context, limit int) ([]*models.ModerationPatternStats, error) {
	if f.getTopPatternsFn != nil {
		return f.getTopPatternsFn(ctx, limit)
	}
	return nil, nil
}

func (f *fakeModerationMetricsRepo) IncrementPatternHit(ctx context.Context, patternID, patternName string) error {
	if f.incrementPatternHitFn != nil {
		return f.incrementPatternHitFn(ctx, patternID, patternName)
	}
	return nil
}

func (f *fakeModerationMetricsRepo) GetMetricsEntries(ctx context.Context, timeRange models.ModerationMetricsTimeRange, metricTypes []string) ([]*models.ModerationMetricsEntry, error) {
	if f.getMetricsEntriesFn != nil {
		return f.getMetricsEntriesFn(ctx, timeRange, metricTypes)
	}
	return nil, nil
}

func (f *fakeModerationMetricsRepo) GetAggregatedStats(ctx context.Context, timeRange models.ModerationMetricsTimeRange) (*models.ModerationMetricsStats, error) {
	if f.getAggregatedStatsFn != nil {
		return f.getAggregatedStatsFn(ctx, timeRange)
	}
	if f.defaultAggregatedResult != nil {
		return f.defaultAggregatedResult, nil
	}
	return &models.ModerationMetricsStats{
		TimeRange:      timeRange,
		ActionCounts:   map[models.AdvancedModerationAction]int64{},
		CategoryCounts: map[string]int64{},
		SeverityCounts: map[models.AdvancedSeverity]int64{},
	}, nil
}

func TestModerationMetrics_RecordAnalysis_IncrementsCountersAndSamples(t *testing.T) {
	ctx := context.Background()

	sampleCh := make(chan *models.ModerationDecisionSample, 1)
	repo := &fakeModerationMetricsRepo{
		recordDecisionSampleFn: func(_ context.Context, sample *models.ModerationDecisionSample) error {
			sampleCh <- sample
			return nil
		},
	}

	mm := NewModerationMetrics(repo, zap.NewNop())
	mm.startTime = time.Now().Add(-2 * time.Minute)

	decision := &ModerationDecision{
		ContentID:      "content-1",
		Decision:       ActionFlag,
		Confidence:     0.4,
		RequiresReview: true,
		ReviewPriority: 2,
		Reasons: []DecisionReason{
			{Type: "toxicity", Severity: SeverityHigh},
			{Type: "pii", Severity: SeverityLow},
		},
	}

	mm.RecordAnalysis(ctx, "note", 120*time.Millisecond, decision)

	select {
	case sample := <-sampleCh:
		require.NotNil(t, sample)
		assert.Equal(t, decision.ContentID, sample.ContentID)
		assert.Equal(t, string(decision.Decision), sample.Decision)
		assert.Equal(t, decision.Confidence, sample.Confidence)
		assert.Equal(t, int64(120), sample.ProcessingTime)
		assert.True(t, sample.RequiresReview)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for decision sample to be recorded")
	}

	stats := mm.GetRealtimeStats()
	assert.Equal(t, int64(1), stats.TotalAnalyzed)
	assert.Greater(t, stats.AnalysisRate, 0.0)
	assert.Greater(t, mm.getCounter("content_type:note"), int64(0))
	assert.Greater(t, mm.getCounter("decision:flag"), int64(0))
	assert.Greater(t, mm.getCounter("requires_review"), int64(0))
	assert.Greater(t, mm.getCounter("severity:high"), int64(0))
	assert.Greater(t, mm.getCounter("reason_type:toxicity"), int64(0))
}

func TestModerationMetrics_RecordFalsePositive_RecordsAndContinuesOnError(t *testing.T) {
	ctx := context.Background()
	repo := &fakeModerationMetricsRepo{
		recordFalsePositiveFn: func(context.Context, *models.ModerationFalsePositive) error {
			return errors.New("store failed")
		},
	}

	mm := NewModerationMetrics(repo, zap.NewNop())
	mm.RecordFalsePositive(ctx, "content-1", &ModerationDecision{Decision: ActionRemove, Confidence: 0.9})

	assert.Equal(t, int64(1), mm.getCounter("false_positives"))
	assert.Equal(t, int64(1), mm.getCounter("false_positive:remove"))
}

func TestModerationMetrics_GetStats_ConvertsAndAddsCurrentPeriod(t *testing.T) {
	ctx := context.Background()

	repo := &fakeModerationMetricsRepo{
		defaultAggregatedResult: &models.ModerationMetricsStats{
			TotalAnalyzed: 10,
			ActionCounts: map[models.AdvancedModerationAction]int64{
				models.AdvancedModerationActionAllow: 7,
				models.AdvancedModerationActionFlag:  3,
			},
			CategoryCounts: map[string]int64{"toxicity": 2},
			SeverityCounts: map[models.AdvancedSeverity]int64{
				models.AdvancedSeverityHigh: 2,
			},
			AverageConfidence: 0.77,
			FalsePositives:    1,
			TruePositives:     2,
			ResponseTime:      250 * time.Millisecond,
		},
	}

	mm := NewModerationMetrics(repo, zap.NewNop())
	mm.startTime = time.Now().Add(-1 * time.Minute)

	mm.RecordTruePositive(ctx, "content-1", &ModerationDecision{Decision: ActionRemove})
	mm.RecordAnalysis(ctx, "note", 100*time.Millisecond, &ModerationDecision{Decision: ActionAllow, Confidence: 0.9})
	mm.RecordAnalysis(ctx, "note", 200*time.Millisecond, &ModerationDecision{Decision: ActionFlag, Confidence: 0.4})

	tr := TimeRange{Start: time.Now().Add(-5 * time.Minute), End: time.Now().Add(5 * time.Minute)}
	stats, err := mm.GetStats(ctx, tr)
	require.NoError(t, err)

	assert.Equal(t, int64(12), stats.TotalAnalyzed)
	assert.Equal(t, int64(8), stats.ActionCounts[ActionAllow])
	assert.Equal(t, int64(4), stats.ActionCounts[ActionFlag])
	assert.Equal(t, int64(3), stats.TruePositives)
	assert.Equal(t, int64(1), stats.FalsePositives)
	assert.Equal(t, int64(2), stats.SeverityCounts[SeverityHigh])
	assert.Equal(t, 150*time.Millisecond, stats.ResponseTime)
}

func TestModerationMetrics_GetRealtimeStats_ComputesP95(t *testing.T) {
	repo := &fakeModerationMetricsRepo{}
	mm := NewModerationMetrics(repo, zap.NewNop())

	for i := 0; i < 20; i++ {
		// Deliberately shuffle values so the p95 path sorts.
		mm.RecordAnalysis(context.Background(), "note", time.Duration((20-i))*time.Millisecond, &ModerationDecision{Decision: ActionAllow, Confidence: 0.9})
	}

	stats := mm.GetRealtimeStats()
	assert.Equal(t, int64(20), stats.TotalAnalyzed)
	assert.Equal(t, 20*time.Millisecond, stats.P95ResponseTime)
}

func TestModerationMetrics_GetTopPatterns_ConvertsResults(t *testing.T) {
	repo := &fakeModerationMetricsRepo{
		getTopPatternsFn: func(context.Context, int) ([]*models.ModerationPatternStats, error) {
			return []*models.ModerationPatternStats{
				{PatternID: "p1", PatternName: "bad", HitCount: 3, LastHit: time.Now().Add(-time.Hour)},
			}, nil
		},
	}

	mm := NewModerationMetrics(repo, zap.NewNop())
	patterns, err := mm.GetTopPatterns(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, patterns, 1)
	assert.Equal(t, "p1", patterns[0].PatternID)
	assert.Equal(t, "bad", patterns[0].PatternName)
	assert.Equal(t, int64(3), patterns[0].HitCount)
}

func TestModerationMetrics_FlushMetrics_FlushesAndResetsCounters(t *testing.T) {
	ctx := context.Background()

	var (
		mu      sync.Mutex
		entries []*models.ModerationMetricsEntry
	)
	repo := &fakeModerationMetricsRepo{
		recordMetricsEntriesFn: func(_ context.Context, got []*models.ModerationMetricsEntry) error {
			mu.Lock()
			defer mu.Unlock()
			entries = append(entries, got...)
			return nil
		},
	}

	mm := NewModerationMetrics(repo, zap.NewNop())
	mm.incrementCounter("decision:allow")
	mm.incrementCounter("decision:allow")
	mm.incrementCounter("decision:flag")

	require.NoError(t, mm.FlushMetrics(ctx))

	mu.Lock()
	require.Len(t, entries, 2)
	mu.Unlock()

	assert.Equal(t, int64(0), mm.getCounter("decision:allow"))
	assert.Equal(t, int64(0), mm.getCounter("decision:flag"))
}

func TestModerationMetrics_FlushMetrics_ReturnsErrorWhenRepositoryFails(t *testing.T) {
	repo := &fakeModerationMetricsRepo{
		recordMetricsEntriesFn: func(context.Context, []*models.ModerationMetricsEntry) error {
			return errors.New("write failed")
		},
	}

	mm := NewModerationMetrics(repo, zap.NewNop())
	mm.incrementCounter("decision:allow")

	err := mm.FlushMetrics(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flush metrics")
}
