package storagecost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeTrackingRepo struct {
	createFn                     func(context.Context, *models.DynamoDBCostRecord) error
	getAggregatedCostsByPeriodFn func(context.Context, string, time.Time, time.Time) ([]*models.DynamoDBCostAggregation, error)
	getCostsByOperationTypeFn    func(context.Context, time.Time, time.Time) (map[string]*models.DynamoDBServiceCostStats, error)
	getCostsByServiceFn          func(context.Context, time.Time, time.Time) (map[string]*models.DynamoDBServiceCostStats, error)
}

func (f *fakeTrackingRepo) Create(ctx context.Context, record *models.DynamoDBCostRecord) error {
	if f.createFn != nil {
		return f.createFn(ctx, record)
	}
	return nil
}

func (f *fakeTrackingRepo) GetAggregatedCostsByPeriod(ctx context.Context, period string, startDate, endDate time.Time) ([]*models.DynamoDBCostAggregation, error) {
	if f.getAggregatedCostsByPeriodFn != nil {
		return f.getAggregatedCostsByPeriodFn(ctx, period, startDate, endDate)
	}
	return nil, nil
}

func (f *fakeTrackingRepo) GetCostsByOperationType(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
	if f.getCostsByOperationTypeFn != nil {
		return f.getCostsByOperationTypeFn(ctx, startDate, endDate)
	}
	return nil, nil
}

func (f *fakeTrackingRepo) GetCostsByService(ctx context.Context, startDate, endDate time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
	if f.getCostsByServiceFn != nil {
		return f.getCostsByServiceFn(ctx, startDate, endDate)
	}
	return nil, nil
}

func TestDynamORMStorage_SaveOperationCost_MapsFields_Round23(t *testing.T) {
	t.Parallel()

	var captured *models.DynamoDBCostRecord
	repo := &fakeTrackingRepo{
		createFn: func(_ context.Context, record *models.DynamoDBCostRecord) error {
			captured = record
			return nil
		},
	}

	storage := &DynamORMStorage{repo: repo, logger: zap.NewNop()}

	opCost := &cost.OperationCost{
		DynamoDBReads:       2,
		DynamoDBWrites:      3,
		DynamoDBStorage:     99,
		LambdaInvocations:   5,
		LambdaDurationMs:    123,
		LambdaMemoryMB:      256,
		S3Gets:              7,
		S3Puts:              11,
		S3Storage:           12,
		DataTransferBytes:   13,
		TotalCostMicroCents: 42,
		Timestamp:           time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		OperationType:       "GetItem",
		RequestID:           "req-1",
	}

	require.NoError(t, storage.SaveOperationCost(context.Background(), opCost))
	require.NotNil(t, captured)
	require.Equal(t, "GetItem", captured.OperationType)
	require.Equal(t, "main", captured.Table)
	require.Equal(t, "raw", captured.Period)
	require.Equal(t, opCost.Timestamp, captured.Timestamp)
	require.Equal(t, float64(2), captured.ReadCapacityUnits)
	require.Equal(t, float64(3), captured.WriteCapacityUnits)
	require.Equal(t, int64(42), captured.TotalCostMicroCents)
	require.Equal(t, "req-1", captured.RequestID)
	require.Equal(t, "GetItem", captured.FunctionName)
	require.Equal(t, int64(5), captured.Properties["lambda_invocations"])
	require.Equal(t, int64(256), captured.Properties["lambda_memory_mb"])
	require.Equal(t, int64(7), captured.Properties["s3_gets"])
	require.Equal(t, int64(11), captured.Properties["s3_puts"])
	require.Equal(t, int64(12), captured.Properties["s3_storage_bytes"])
	require.Equal(t, int64(13), captured.Properties["data_transfer_bytes"])
	require.Equal(t, int64(99), captured.Properties["dynamodb_storage_bytes"])
}

func TestDynamORMStorage_GetDailyCosts_ConvertsAndPropagatesErrors_Round23(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	storage := &DynamORMStorage{
		repo: &fakeTrackingRepo{
			getAggregatedCostsByPeriodFn: func(_ context.Context, period string, _, _ time.Time) ([]*models.DynamoDBCostAggregation, error) {
				require.Equal(t, "day", period)
				return []*models.DynamoDBCostAggregation{
					{
						WindowStart:         start,
						TotalCostMicroCents: 100,
						TotalOperations:     2,
					},
				}, nil
			},
		},
		logger: zap.NewNop(),
	}

	daily, err := storage.GetDailyCosts(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, daily, 1)
	require.Equal(t, int64(100), daily[0].TotalCostMicrocents)

	storage.repo = &fakeTrackingRepo{
		getAggregatedCostsByPeriodFn: func(context.Context, string, time.Time, time.Time) ([]*models.DynamoDBCostAggregation, error) {
			return nil, errors.New("boom")
		},
	}
	_, err = storage.GetDailyCosts(context.Background(), start, end)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get daily costs")
}

func TestDynamORMStorage_GetMonthlyCost_EmptyAndPresent_Round23(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, -1)

	repo := &fakeTrackingRepo{
		getAggregatedCostsByPeriodFn: func(_ context.Context, period string, startDate, endDate time.Time) ([]*models.DynamoDBCostAggregation, error) {
			require.Equal(t, "month", period)
			require.Equal(t, start, startDate)
			require.Equal(t, end, endDate)
			return nil, nil
		},
	}
	storage := &DynamORMStorage{repo: repo, logger: zap.NewNop()}

	empty, err := storage.GetMonthlyCost(context.Background(), 2025, time.February)
	require.NoError(t, err)
	require.Equal(t, 2025, empty.Year)
	require.Equal(t, 2, empty.Month)
	require.Zero(t, empty.TotalCostMicrocents)
	require.Zero(t, empty.ProjectedCostMicrocents)

	storage.repo = &fakeTrackingRepo{
		getAggregatedCostsByPeriodFn: func(context.Context, string, time.Time, time.Time) ([]*models.DynamoDBCostAggregation, error) {
			return []*models.DynamoDBCostAggregation{
				{
					WindowStart:         start,
					TotalCostMicroCents: 123,
					TotalCostDollars:    1.0,
					TotalOperations:     3,
				},
			}, nil
		},
	}
	got, err := storage.GetMonthlyCost(context.Background(), 2025, time.February)
	require.NoError(t, err)
	require.Equal(t, int64(123), got.TotalCostMicrocents)
}

func TestDynamORMStorage_GetMonthlyCosts_Converts_Round23(t *testing.T) {
	t.Parallel()

	storage := &DynamORMStorage{
		repo: &fakeTrackingRepo{
			getAggregatedCostsByPeriodFn: func(context.Context, string, time.Time, time.Time) ([]*models.DynamoDBCostAggregation, error) {
				return []*models.DynamoDBCostAggregation{
					{WindowStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), TotalCostMicroCents: 10, TotalCostDollars: 0.1},
					{WindowStart: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), TotalCostMicroCents: 20, TotalCostDollars: 0.2},
				}, nil
			},
		},
		logger: zap.NewNop(),
	}

	got, err := storage.GetMonthlyCosts(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestDynamORMStorage_GetCostByOperationAndService_Converts_Round23(t *testing.T) {
	t.Parallel()

	storage := &DynamORMStorage{
		repo: &fakeTrackingRepo{
			getCostsByOperationTypeFn: func(context.Context, time.Time, time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
				return map[string]*models.DynamoDBServiceCostStats{
					"GetItem": {TotalCostMicroCents: 10},
				}, nil
			},
			getCostsByServiceFn: func(context.Context, time.Time, time.Time) (map[string]*models.DynamoDBServiceCostStats, error) {
				return map[string]*models.DynamoDBServiceCostStats{
					"s3": {TotalCostMicroCents: 20},
				}, nil
			},
		},
		logger: zap.NewNop(),
	}

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	byOp, err := storage.GetCostByOperation(context.Background(), start, end)
	require.NoError(t, err)
	require.Equal(t, int64(10), byOp["GetItem"])

	bySvc, err := storage.GetCostByService(context.Background(), start, end)
	require.NoError(t, err)
	require.Equal(t, int64(20), bySvc["s3"])
}

func TestDynamORMStorage_GetCurrentMonthProjection_EmptyDailyCosts_Round23(t *testing.T) {
	t.Parallel()

	now := time.Now()
	storage := &DynamORMStorage{
		repo: &fakeTrackingRepo{
			getAggregatedCostsByPeriodFn: func(context.Context, string, time.Time, time.Time) ([]*models.DynamoDBCostAggregation, error) {
				return nil, nil
			},
		},
		logger: zap.NewNop(),
	}

	proj, err := storage.GetCurrentMonthProjection(context.Background())
	require.NoError(t, err)
	require.Equal(t, now.Year(), proj.Year)
	require.Equal(t, int(now.Month()), proj.Month)
	require.Zero(t, proj.TotalCostMicrocents)
	require.Zero(t, proj.ProjectedCostMicrocents)
}

func TestDynamORMStorage_GetCurrentMonthProjection_WithDailyCosts_Round23(t *testing.T) {
	t.Parallel()

	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	storage := &DynamORMStorage{
		repo: &fakeTrackingRepo{
			getAggregatedCostsByPeriodFn: func(context.Context, string, time.Time, time.Time) ([]*models.DynamoDBCostAggregation, error) {
				return []*models.DynamoDBCostAggregation{
					{
						WindowStart:             startOfMonth,
						TotalCostMicroCents:     100,
						TotalOperations:         2,
						TotalReadCapacityUnits:  1,
						TotalWriteCapacityUnits: 2,
						AverageDuration:         10,
						TableBreakdown: map[string]*models.DynamoDBTableCostStats{
							"users": {UniqueUsers: 5},
						},
					},
				}, nil
			},
		},
		logger: zap.NewNop(),
	}

	proj, err := storage.GetCurrentMonthProjection(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(100), proj.TotalCostMicrocents)
	require.GreaterOrEqual(t, proj.ProjectedCostMicrocents, proj.TotalCostMicrocents)
	require.Equal(t, int64(2), proj.RequestCount)
	require.Equal(t, int64(2), proj.LambdaInvocations)
}

func TestDataTransferBytesHelpersAndDaysInMonth_Round23(t *testing.T) {
	t.Parallel()

	require.Equal(t, int64(0), dataTransferBytesFromServiceBreakdownDaily(nil))
	require.Equal(t, int64(0), dataTransferBytesFromServiceBreakdownAll(nil))

	require.Equal(t, int64(150), dataTransferBytesFromServiceBreakdownDaily(map[string]*models.DynamoDBServiceCostStats{
		"s3":         {DataTransferBytes: 100},
		"cloudfront": {DataTransferBytes: 50},
		"other":      {DataTransferBytes: 999},
	}))
	require.Equal(t, int64(1149), dataTransferBytesFromServiceBreakdownAll(map[string]*models.DynamoDBServiceCostStats{
		"s3":         {DataTransferBytes: 100},
		"cloudfront": {DataTransferBytes: 50},
		"other":      {DataTransferBytes: 999},
	}))

	require.Equal(t, 29, daysInMonth(2024, 2))
	require.Equal(t, 28, daysInMonth(2025, 2))
}
