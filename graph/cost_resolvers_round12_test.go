package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/stretchr/testify/require"
)

func TestRound12DriverResolver_Behavior(t *testing.T) {
	t.Parallel()

	r := &driverResolver{&Resolver{}}
	ctx := context.Background()

	typ, err := r.Type(ctx, &cost.Driver{Service: "dynamo", Operation: "read"})
	require.NoError(t, err)
	require.Equal(t, "dynamo read", typ)

	typ, err = r.Type(ctx, &cost.Driver{Service: "dynamo", Operation: allOperationsValue})
	require.NoError(t, err)
	require.Equal(t, "dynamo", typ)

	typ, err = r.Type(ctx, &cost.Driver{Service: "dynamo"})
	require.NoError(t, err)
	require.Equal(t, "dynamo", typ)

	domain, err := r.Domain(ctx, &cost.Driver{})
	require.NoError(t, err)
	require.Nil(t, domain)

	costValue, err := r.Cost(ctx, &cost.Driver{CostMicroCents: 1234567})
	require.NoError(t, err)
	require.InDelta(t, 1.234567, costValue, 0.0000001)

	pct, err := r.PercentOfTotal(ctx, &cost.Driver{PercentageOfTotal: 12.5})
	require.NoError(t, err)
	require.Equal(t, 12.5, pct)

	trend, err := r.Trend(ctx, &cost.Driver{Trend: "INCREASING"})
	require.NoError(t, err)
	require.Equal(t, model.TrendIncreasing, trend)

	trend, err = r.Trend(ctx, &cost.Driver{Trend: "decreasing"})
	require.NoError(t, err)
	require.Equal(t, model.TrendDecreasing, trend)

	trend, err = r.Trend(ctx, &cost.Driver{Trend: "unknown"})
	require.NoError(t, err)
	require.Equal(t, model.TrendStable, trend)
}

func TestRound12QueryResolvers_CostQueries(t *testing.T) {
	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	query := &queryResolver{resolver}
	adminCtx := round12AuthContext("admin")

	tracker := cost.New()
	require.NoError(t, tracker.TrackDynamoRead(1))
	require.NoError(t, tracker.TrackDynamoWrite(1))
	tracker.TrackS3Get(25)
	tracker.TrackS3Put(2)
	tracker.TrackLambdaInvocation(100, 128)
	tracker.TrackDataTransfer(1024 * 1024) // 1 MiB

	resolver.CostTracker = tracker

	_, err := query.CostBreakdown(round12AuthContext("alice"), nil)
	require.Error(t, err)

	breakdown, err := query.CostBreakdown(adminCtx, nil)
	require.NoError(t, err)
	require.Equal(t, model.PeriodDay, breakdown.Period)
	require.Greater(t, breakdown.TotalCost, 0.0)
	require.NotEmpty(t, breakdown.Breakdown)

	// Infrastructure health uses real adapter-backed logic; just ensure it runs.
	health, err := query.InfrastructureHealth(adminCtx)
	require.NoError(t, err)
	require.NotNil(t, health)

	// Seed the query tracker with a slow query and assert it appears.
	qTracker := resolver.Registry.QueryTracker()
	qTracker.RecordQuery(context.Background(), "Query1", 2*time.Second, false)

	slow, err := query.SlowQueries(adminCtx, model.Duration(1*time.Second))
	require.NoError(t, err)
	require.NotEmpty(t, slow)

	// Bandwidth usage returns empty report when service unavailable.
	_, err = query.BandwidthUsage(context.Background(), model.TimePeriodDay)
	require.Error(t, err)

	report, err := query.BandwidthUsage(adminCtx, model.TimePeriodDay)
	require.NoError(t, err)
	require.NotNil(t, report)
	require.Equal(t, model.TimePeriodDay, report.Period)

	// Cost projections may rely on storage state; ensure it doesn't panic in unit mode.
	_, _ = query.CostProjections(adminCtx, model.PeriodDay)
}
