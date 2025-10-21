package analytics

import (
	"context"
	"testing"
	"time"
)

type noopAnalyticsStore struct{}

func (noopAnalyticsStore) StoreMetric(context.Context, *PerformanceMetric) error { return nil }
func (noopAnalyticsStore) GetMetrics(context.Context, MetricQuery) ([]PerformanceMetric, error) {
	return nil, nil
}
func (noopAnalyticsStore) GetAggregatedMetrics(context.Context, AggregateQuery) ([]AggregatedMetric, error) {
	return nil, nil
}
func (noopAnalyticsStore) DeleteOldMetrics(context.Context, time.Time) error { return nil }
func (noopAnalyticsStore) GetMetricNames() []string                          { return nil }

func TestStartDefaultsAnalysisInterval(t *testing.T) {
	engine := NewPerformanceAnalyticsEngine(PerformanceAnalyticsConfig{Enabled: true})
	engine.SetDataStore(noopAnalyticsStore{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("unexpected error starting engine: %v", err)
	}
	defer func() {
		if err := engine.Stop(); err != nil {
			t.Fatalf("unexpected error stopping engine: %v", err)
		}
	}()

	if engine.config.AnalysisInterval != defaultAnalysisInterval {
		t.Fatalf("expected analysis interval to default to %v, got %v", defaultAnalysisInterval, engine.config.AnalysisInterval)
	}
}

func TestStartValidatesAnalysisInterval(t *testing.T) {
	engine := NewPerformanceAnalyticsEngine(PerformanceAnalyticsConfig{
		Enabled:          true,
		AnalysisInterval: -1 * time.Second,
	})
	engine.SetDataStore(noopAnalyticsStore{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err == nil {
		t.Fatal("expected error for negative analysis interval")
	}
}

func TestPerformanceAnalyticsEngineRestart(t *testing.T) {
	engine := NewPerformanceAnalyticsEngine(PerformanceAnalyticsConfig{
		Enabled:          true,
		AnalysisInterval: time.Millisecond,
	})
	engine.SetDataStore(noopAnalyticsStore{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("unexpected error starting engine: %v", err)
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("unexpected error stopping engine: %v", err)
	}

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("unexpected error restarting engine: %v", err)
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("unexpected error stopping engine second time: %v", err)
	}
}

func TestPerformanceAnalyticsEngineStopIdempotent(t *testing.T) {
	engine := NewPerformanceAnalyticsEngine(PerformanceAnalyticsConfig{Enabled: true})
	engine.SetDataStore(noopAnalyticsStore{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("unexpected error starting engine: %v", err)
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("unexpected error stopping engine: %v", err)
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("unexpected error on idempotent stop: %v", err)
	}
}
