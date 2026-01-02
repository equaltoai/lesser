package federation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type fakeAnalyticsFederationRepo struct {
	storeErr error

	stored []*models.FederationAnalyticsTimeSeries

	healthScore float64
	healthErr   error

	periodMetrics []*models.FederationAnalyticsTimeSeries
	periodErr     error

	unhealthy    []*models.FederationAnalyticsTimeSeries
	unhealthyErr error

	aggregateCalls []string
	aggregateErr   error
}

func (f *fakeAnalyticsFederationRepo) StoreDetailedFederationMetrics(_ context.Context, metrics *models.FederationAnalyticsTimeSeries) error {
	if f.storeErr != nil {
		return f.storeErr
	}
	f.stored = append(f.stored, metrics)
	return nil
}

func (f *fakeAnalyticsFederationRepo) AggregateFederationMetrics(_ context.Context, domain, fromPeriod, toPeriod string, timestamp time.Time) error {
	f.aggregateCalls = append(f.aggregateCalls, domain+":"+fromPeriod+"->"+toPeriod+":"+timestamp.Format(time.RFC3339))
	return f.aggregateErr
}

func (f *fakeAnalyticsFederationRepo) GetDomainHealthScore(_ context.Context, _ string) (float64, error) {
	if f.healthErr != nil {
		return 0, f.healthErr
	}
	return f.healthScore, nil
}

func (f *fakeAnalyticsFederationRepo) GetDetailedMetricsByPeriod(_ context.Context, _ string, _ time.Time, _ time.Time, _ int) ([]*models.FederationAnalyticsTimeSeries, error) {
	if f.periodErr != nil {
		return nil, f.periodErr
	}
	return f.periodMetrics, nil
}

func (f *fakeAnalyticsFederationRepo) GetUnhealthyDomains(_ context.Context, _ float64) ([]*models.FederationAnalyticsTimeSeries, error) {
	if f.unhealthyErr != nil {
		return nil, f.unhealthyErr
	}
	return f.unhealthy, nil
}

func TestAnalyticsAggregator_RecordMetric(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("stores_success_metric_and_sets_error_type_fields", func(t *testing.T) {
		repo := &fakeAnalyticsFederationRepo{}
		agg := &AnalyticsAggregator{federationRepo: repo, logger: logger}

		err := agg.RecordMetric(ctx, "example.com", &Metric{
			InboundBytes:    10,
			OutboundBytes:   20,
			ResponseTimeMs:  123,
			SignatureTimeMs: 4,
			Success:         true,
			ErrorType:       "signature_failure",
			ActivityType:    "follow",
		})
		require.NoError(t, err)
		require.Len(t, repo.stored, 1)

		stored := repo.stored[0]
		assert.Equal(t, "example.com", stored.Domain)
		assert.Equal(t, "raw", stored.Period)
		assert.Equal(t, int64(1), stored.ActivityCount)
		assert.Equal(t, int64(1), stored.SuccessfulActivities)
		assert.Equal(t, int64(0), stored.FailedActivities)
		assert.Equal(t, int64(1), stored.SignatureFailures)
		assert.InDelta(t, 1.0, stored.InstanceReachability, 0.0001)
		assert.InDelta(t, 0.0, stored.ErrorRate, 0.0001)
	})

	t.Run("wraps_store_error", func(t *testing.T) {
		repo := &fakeAnalyticsFederationRepo{storeErr: errors.New("boom")}
		agg := &AnalyticsAggregator{federationRepo: repo, logger: logger}

		err := agg.RecordMetric(ctx, "example.com", &Metric{Success: false, ActivityType: "follow"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrFederationMetricStoreFailed)
	})
}

func TestAnalyticsAggregator_triggerAggregation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := &fakeAnalyticsFederationRepo{}
	agg := &AnalyticsAggregator{federationRepo: repo, logger: logger}

	// Midnight on first day should trigger all aggregation levels.
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	agg.triggerAggregation(context.Background(), "example.com", ts)

	assert.GreaterOrEqual(t, len(repo.aggregateCalls), 4)
}

func TestAnalyticsAggregator_GetDomainHealthStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	now := time.Now()

	repo := &fakeAnalyticsFederationRepo{
		healthScore: 35,
		periodMetrics: []*models.FederationAnalyticsTimeSeries{
			{Domain: "example.com", ActivityCount: 10, FailedActivities: 5, InboxDeliveryP95: 6000, InstanceReachability: 0.4},
			{Domain: "example.com", ActivityCount: 20, FailedActivities: 0, InboxDeliveryP95: 1000, InstanceReachability: 1.0, Timestamp: now},
		},
	}
	agg := &AnalyticsAggregator{federationRepo: repo, logger: logger}

	status, err := agg.GetDomainHealthStatus(context.Background(), "example.com")
	require.NoError(t, err)
	assert.Equal(t, "CRITICAL", status.Status)
	assert.Equal(t, int64(30), status.RecentActivities)
	assert.Equal(t, int64(5), status.RecentErrors)
	assert.True(t, status.ShouldAlert)
	assert.NotEmpty(t, status.AlertMessage)
}

func TestAnalyticsAggregator_GetUnhealthyDomains(t *testing.T) {
	logger := zaptest.NewLogger(t)
	repo := &fakeAnalyticsFederationRepo{
		unhealthy: []*models.FederationAnalyticsTimeSeries{
			{Domain: "a.example", HealthScore: 55, Timestamp: time.Now(), ActivityCount: 100, FailedActivities: 20, ErrorRate: 0.2, InboxDeliveryP95: 600, InstanceReachability: 0.9},
			{Domain: "b.example", HealthScore: 10, Timestamp: time.Now(), ActivityCount: 10, FailedActivities: 9, ErrorRate: 0.9, InboxDeliveryP95: 9000, InstanceReachability: 0.1},
		},
	}
	agg := &AnalyticsAggregator{federationRepo: repo, logger: logger}

	statuses, err := agg.GetUnhealthyDomains(context.Background(), 0)
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	assert.Equal(t, "UNHEALTHY", statuses[0].Status)
	assert.Equal(t, "CRITICAL", statuses[1].Status)
	assert.True(t, statuses[1].ShouldAlert)
}
