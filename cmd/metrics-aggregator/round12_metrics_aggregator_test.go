package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	dynamormmocks "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeMetricsRepository struct {
	createAggregatedCalls int
	cleanupCalls          int
	getStatsCalls         int
	aggregateCalls        int

	createAggregatedErr error
	cleanupErr          error
	stats               *repositories.ServiceStats
	statsErr            error
	aggregateErr        error
}

func (f *fakeMetricsRepository) CreateAggregated(_ context.Context, _ *models.AggregatedMetrics) error {
	f.createAggregatedCalls++
	return f.createAggregatedErr
}

func (f *fakeMetricsRepository) CleanupOldMetrics(_ context.Context, _ string, _ time.Time) (int, error) {
	f.cleanupCalls++
	if f.cleanupErr != nil {
		return 0, f.cleanupErr
	}
	return 1, nil
}

func (f *fakeMetricsRepository) GetServiceStats(_ context.Context, _, _ string, _ time.Time, _ time.Time) (*repositories.ServiceStats, error) {
	f.getStatsCalls++
	if f.statsErr != nil {
		return nil, f.statsErr
	}
	if f.stats != nil {
		return f.stats, nil
	}
	return &repositories.ServiceStats{}, nil
}

func (f *fakeMetricsRepository) Aggregate(_ context.Context, _, _ string, _ time.Time, _ time.Time) error {
	f.aggregateCalls++
	return f.aggregateErr
}

func TestMetricsAggregator_isMetricsRecord_Round12(t *testing.T) {
	ma := &MetricsAggregator{}
	require.True(t, ma.isMetricsRecord("metrics#request"))
	require.False(t, ma.isMetricsRecord("metrics"))
	require.False(t, ma.isMetricsRecord("other#metrics"))
}

func TestMetricsAggregator_extractMetricFromRecord_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	ma := &MetricsAggregator{logger: zap.NewNop()}

	unmarshalItemFn = func(events.DynamoDBEventRecord, any) error { return errors.New("boom") }
	_, err := ma.extractMetricFromRecord(events.DynamoDBEventRecord{})
	require.Error(t, err)

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, out any) error {
		m := out.(*models.Metrics)
		m.Type = ""
		m.Service = "api"
		return nil
	}
	_, err = ma.extractMetricFromRecord(events.DynamoDBEventRecord{})
	require.Error(t, err)
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeBadRequest))

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, out any) error {
		m := out.(*models.Metrics)
		m.Type = "request"
		m.Service = "api"
		m.Count = 1
		m.Sum = 10
		m.Min = 10
		m.Max = 10
		return nil
	}
	metric, err := ma.extractMetricFromRecord(events.DynamoDBEventRecord{})
	require.NoError(t, err)
	require.Equal(t, "request", metric.Type)
	require.Equal(t, "api", metric.Service)
}

func TestMetricsAggregator_HandleStreamWithContext_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	repo := &fakeMetricsRepository{}
	ma := &MetricsAggregator{
		logger:            zap.NewNop(),
		metricsRepository: repo,
	}

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, out any) error {
		m := out.(*models.Metrics)
		m.Type = "request"
		m.Service = "api"
		m.Count = 2
		m.Sum = 20
		m.Min = 5
		m.Max = 15
		return nil
	}

	liftCtx := lift.NewContext(context.Background(), lift.NewRequest(nil))

	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{EventName: "REMOVE"},
			{
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("metrics#request"),
					},
				},
			},
			{
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("not-metrics"),
					},
				},
			},
		},
	}

	require.NoError(t, ma.HandleStreamWithContext(context.Background(), liftCtx, event))
	require.Equal(t, 1, repo.createAggregatedCalls)
}

func TestMetricsAggregator_AggregationAndCleanup_Round12(t *testing.T) {
	repo := &fakeMetricsRepository{
		stats: &repositories.ServiceStats{Count: 1, Average: 2.5},
	}
	ma := &MetricsAggregator{
		logger:            zap.NewNop(),
		metricsRepository: repo,
	}

	now := time.Now()
	require.NoError(t, ma.aggregateMetrics(context.Background(), "api", "request", "hour", now.Add(-time.Hour), now))
	require.Equal(t, 1, repo.aggregateCalls)

	repo.stats = &repositories.ServiceStats{Count: 0}
	require.NoError(t, ma.aggregateMetrics(context.Background(), "api", "request", "hour", now.Add(-time.Hour), now))

	repo.statsErr = errors.New("boom")
	require.Error(t, ma.aggregateMetrics(context.Background(), "api", "request", "hour", now.Add(-time.Hour), now))

	repo.statsErr = nil
	repo.aggregateErr = errors.New("boom")
	repo.stats = &repositories.ServiceStats{Count: 1}
	require.Error(t, ma.aggregateMetrics(context.Background(), "api", "request", "hour", now.Add(-time.Hour), now))

	repo.aggregateErr = nil
	require.NoError(t, ma.handleAggregationEvent(context.Background(), AggregationEvent{
		Type:      "hour",
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
	}))
	require.Greater(t, repo.cleanupCalls, 0)
}

func TestMetricsAggregator_cleanupMetricsByGranularity_Round12(t *testing.T) {
	repo := &fakeMetricsRepository{cleanupErr: errors.New("boom")}
	ma := &MetricsAggregator{
		logger:            zap.NewNop(),
		metricsRepository: repo,
	}

	_, err := ma.cleanupMetricsByGranularity(context.Background(), "minute", time.Now().Add(-24*time.Hour))
	require.Error(t, err)
}

func TestMetricsAggregator_HandleCloudWatchEvent_Round12(t *testing.T) {
	repo := &fakeMetricsRepository{}
	ma := &MetricsAggregator{
		logger:            zap.NewNop(),
		metricsRepository: repo,
	}

	// Invalid JSON should fall back to hourly aggregation defaults.
	require.NoError(t, ma.HandleCloudWatchEvent(context.Background(), events.CloudWatchEvent{
		DetailType: "Scheduled Event",
		Source:     "aws.events",
		Detail:     []byte("not-json"),
	}))
	require.GreaterOrEqual(t, repo.getStatsCalls, 1)

	// Valid JSON should use the provided aggregation configuration.
	repo.getStatsCalls = 0
	require.NoError(t, ma.HandleCloudWatchEvent(context.Background(), events.CloudWatchEvent{
		DetailType: "Scheduled Event",
		Source:     "aws.events",
		Detail:     []byte(`{"type":"minute","startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-01T00:01:00Z","services":["api"],"metrics":["request"]}`),
	}))
	require.GreaterOrEqual(t, repo.getStatsCalls, 1)
}

func TestMetricsAggregator_cleanupOldMetrics_SkipsTooNew_Round12(t *testing.T) {
	repo := &fakeMetricsRepository{}
	ma := &MetricsAggregator{
		logger:            zap.NewNop(),
		metricsRepository: repo,
	}

	// Using an old "beforeTime" should skip cleanup for retention periods.
	before := time.Now().Add(-365 * 24 * time.Hour)
	require.NoError(t, ma.cleanupOldMetrics(context.Background(), before))
	require.Equal(t, 0, repo.cleanupCalls)
}

func TestMetricsAggregator_HandleStreamWithContext_SkipsNonStringPK_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	repo := &fakeMetricsRepository{}
	ma := &MetricsAggregator{
		logger:            zap.NewNop(),
		metricsRepository: repo,
	}

	unmarshalItemFn = func(events.DynamoDBEventRecord, any) error { return nil }

	liftCtx := lift.NewContext(context.Background(), lift.NewRequest(nil))
	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewNumberAttribute("123"),
					},
				},
			},
			{
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{},
				},
			},
		},
	}

	require.NoError(t, ma.HandleStreamWithContext(context.Background(), liftCtx, event))
	require.Equal(t, 0, repo.createAggregatedCalls)
}

func TestMetricsAggregator_processRealtimeMetricsWithContext_CreateError_Round12(t *testing.T) {
	repo := &fakeMetricsRepository{createAggregatedErr: errors.New("boom")}
	ma := &MetricsAggregator{
		logger:            zap.NewNop(),
		metricsRepository: repo,
	}

	liftCtx := lift.NewContext(context.Background(), lift.NewRequest(nil))
	metrics := []*models.Metrics{
		{Service: "api", Type: "request", Count: 0, Sum: 0, Min: 0, Max: 0},
		{Service: "api", Type: "request", Count: 0, Sum: 0, Min: 0, Max: 0},
	}

	require.NoError(t, ma.processRealtimeMetricsWithContext(context.Background(), liftCtx, metrics))
	require.Equal(t, 1, repo.createAggregatedCalls)
}

func TestMetricsAggregator_HandleStreamWithContext_SkipsUnmarshalError_Round12(t *testing.T) {
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() { unmarshalItemFn = origUnmarshal })

	unmarshalItemFn = func(events.DynamoDBEventRecord, any) error { return errors.New("boom") }

	repo := &fakeMetricsRepository{}
	ma := &MetricsAggregator{
		logger:            zap.NewNop(),
		metricsRepository: repo,
	}

	liftCtx := lift.NewContext(context.Background(), lift.NewRequest(nil))
	event := events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{
			{
				EventName: "INSERT",
				Change: events.DynamoDBStreamRecord{
					NewImage: map[string]events.DynamoDBAttributeValue{
						"PK": events.NewStringAttribute("metrics#request"),
					},
				},
			},
		},
	}

	require.NoError(t, ma.HandleStreamWithContext(context.Background(), liftCtx, event))
	require.Equal(t, 0, repo.createAggregatedCalls)
}

func TestRunMetricsAggregator_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origGetClient := dynamormGetClientFn
	origStart := lambdaStartFn
	origUnmarshal := unmarshalItemFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		dynamormGetClientFn = origGetClient
		lambdaStartFn = origStart
		unmarshalItemFn = origUnmarshal
	})

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				Region:          "us-east-1",
				DynamoTableName: "test-table",
			},
			Logger: zap.NewNop(),
		}
	}

	dynamormGetClientFn = func(context.Context) (dynamormCore.DB, error) {
		return new(dynamormmocks.MockDB), nil
	}

	unmarshalItemFn = func(_ events.DynamoDBEventRecord, out any) error {
		m := out.(*models.Metrics)
		m.Type = "request"
		m.Service = "api"
		m.Count = 1
		m.Sum = 10
		m.Min = 10
		m.Max = 10
		return nil
	}

	repo := &fakeMetricsRepository{}

	called := false
	lambdaStartFn = func(handler any) {
		called = true
		fn, ok := handler.(func(context.Context, any) (any, error))
		require.True(t, ok)

		// Ensure we don't hit the real metrics repository implementation.
		processor.metricsRepository = repo

		event := map[string]any{
			"Records": []any{
				map[string]any{
					"eventID":     "1",
					"eventName":   "INSERT",
					"eventSource": "aws:dynamodb",
					"dynamodb": map[string]any{
						"NewImage": map[string]any{
							"PK": map[string]any{"S": "metrics#request"},
						},
					},
				},
			},
		}
		_, err := fn(context.Background(), event)
		require.NoError(t, err)
	}

	main()
	require.True(t, called)
	require.Equal(t, 1, repo.createAggregatedCalls)
}
