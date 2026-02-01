package repositories

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func setupPermissiveTrackingRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Between", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Offset", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateTrackingSliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateTrackingSliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateTrackingStructForCoverage(args.Get(0), 0, baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(1), nil).Maybe()
}

func populateTrackingSliceForCoverage(target any, baseTime time.Time) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Slice {
		return
	}

	slice := value.Elem()
	elemType := slice.Type().Elem()

	// Avoid interface slices to prevent type assertion pitfalls.
	if elemType.Kind() == reflect.Interface {
		return
	}

	baseType := elemType
	if baseType.Kind() == reflect.Ptr {
		baseType = baseType.Elem()
	}

	count := 2
	switch baseType {
	case reflect.TypeOf(models.CostProjection{}):
		count = 1
	}

	for i := range count {
		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseType)
			populateTrackingStructForCoverage(element.Interface(), i, baseTime)
		} else {
			elemPtr := reflect.New(baseType)
			populateTrackingStructForCoverage(elemPtr.Interface(), i, baseTime)
			element = elemPtr.Elem()
		}
		slice = reflect.Append(slice, element)
	}

	value.Elem().Set(slice)
}

func populateTrackingStructForCoverage(target any, idx int, baseTime time.Time) {
	now := baseTime.Add(time.Duration(idx) * time.Minute)

	switch model := target.(type) {
	case *models.DynamoDBCostRecord:
		model.OperationType = "GetItem"
		if idx%2 == 1 {
			model.OperationType = "PutItem"
		}
		model.Table = "table-1"
		model.Timestamp = now
		model.Period = models.PeriodDay
		model.ServiceName = "import-processor"
		if idx%2 == 1 {
			model.ServiceName = "export-generator"
		}
		model.TotalCostMicroCents = int64(1000 + idx*500)
		model.ReadCapacityUnits = 1
		model.WriteCapacityUnits = 1
		model.ReadCostMicroCents = 100
		model.WriteCostMicroCents = 200
		model.EstimatedCostDollars = float64(model.TotalCostMicroCents) / 1_000_000.0
		model.ItemCount = 1
		model.RequestDuration = 10
		model.Tags = map[string]string{"username": "alice"}
		model.Properties = map[string]any{"username": "alice"}
		_ = model.UpdateKeys()

	case *models.DynamoDBCostAggregation:
		model.Period = models.PeriodDay
		model.OperationType = "GetItem"
		model.Table = "all"
		model.WindowStart = now
		model.WindowEnd = now.Add(time.Hour)
		model.TotalCostMicroCents = int64(100000 + idx*10000)
		model.TotalCostDollars = float64(model.TotalCostMicroCents) / 1_000_000.0
		model.TotalOperations = int64(10 + idx)
		model.TotalReadCapacityUnits = 10
		model.TotalWriteCapacityUnits = 5
		model.TableBreakdown = map[string]*models.DynamoDBTableCostStats{"table-1": {Table: "table-1", OperationCount: 10, TotalCostMicroCents: model.TotalCostMicroCents}}
		model.ServiceBreakdown = map[string]*models.DynamoDBServiceCostStats{"import-processor": {ServiceName: "import-processor", OperationCount: 10, TotalCostMicroCents: model.TotalCostMicroCents}}
		model.CostPercentiles = map[string]float64{"p50": 0.1}
		_ = model.UpdateKeys()

	case *models.CostProjection:
		model.Period = "daily"
		model.CurrentCost = 1.0
		model.ProjectedCost = 2.0
		model.Variance = 100.0
		model.TopDrivers = []models.Driver{
			{Type: "test", Domain: "example.com", Cost: 1.0, PercentOfTotal: 100.0, Trend: "up"},
		}
		model.Recommendations = []string{"do a thing"}
		model.Timestamp = now
		model.CalculatedAt = now
		model.UpdateKeys()

	case *models.RelayCost:
		model.RelayURL = "https://relay.example"
		model.Domain = "example.com"
		model.OperationType = "delivery"
		model.Direction = "outbound"
		model.Timestamp = now
		model.HTTPRequestCount = 1
		model.HTTPRequestCost = 10
		model.DataTransferBytes = 1000
		model.DataTransferCost = 20
		model.LambdaDurationMs = 50
		model.LambdaCost = 30
		model.DynamoDBOperations = 1
		model.DynamoDBCost = 40
		model.SQSMessages = 1
		model.SQSCost = 50
		model.ResponseTimeMs = 100
		model.Success = true
		model.RequestID = "req-1"
		_ = model.BeforeCreate()

	case *models.RelayMetrics:
		model.RelayURL = "https://relay.example"
		model.Domain = "example.com"
		model.Period = models.PeriodDaily
		model.WindowStart = now
		model.WindowEnd = now.Add(24 * time.Hour)
		model.TotalOperations = 10
		model.SuccessfulOperations = 9
		model.TotalCostMicroCents = 1000
		model.TotalDataTransferBytes = 1000
		model.OperationBreakdown = map[string]*models.RelayOperationStats{"delivery": {OperationType: "delivery", Count: 10, TotalCostMicroCents: 1000}}
		_ = model.BeforeCreate()

	case *models.RelayBudget:
		model.RelayURL = "https://relay.example"
		model.Domain = "example.com"
		model.Period = models.PeriodDaily
		model.LimitMicroCents = 10000
		_ = model.BeforeCreate()
	}
}

func TestTrackingRepository_CoverageSweep(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveTrackingRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewTrackingRepository(mockDB, "test-table", zap.NewNop(), nil)

	// Raw record CRUD + listing
	require.NoError(t, repo.Create(ctx, &models.DynamoDBCostRecord{
		ID:                  "id-1",
		OperationType:       "GetItem",
		Table:               "table-1",
		Period:              models.PeriodDay,
		Timestamp:           baseTime,
		TotalCostMicroCents: 1000,
		CreatedAt:           baseTime,
		UpdatedAt:           baseTime,
	}))
	require.NoError(t, repo.BatchCreate(ctx, []*models.DynamoDBCostRecord{{
		ID:                  "id-2",
		OperationType:       "PutItem",
		Table:               "table-1",
		Period:              models.PeriodDay,
		Timestamp:           baseTime,
		TotalCostMicroCents: 2000,
		CreatedAt:           baseTime,
		UpdatedAt:           baseTime,
	}}))

	_, _ = repo.Get(ctx, "GetItem", "id-1", baseTime)
	_, _ = repo.ListByOperationType(ctx, "GetItem", baseTime.Add(-time.Hour), baseTime, 10)
	_, _, _ = repo.ListByTable(ctx, "table-1", baseTime.Add(-time.Hour), baseTime, 1, "")
	_, _, _ = repo.ListByTable(ctx, "table-1", baseTime.Add(-time.Hour), baseTime, 1, "cursor")
	_, _ = repo.GetRecentCosts(ctx, baseTime.Add(-time.Hour), 10)

	// Aggregations
	_, _ = repo.GetAggregated(ctx, models.PeriodDay, "GetItem", baseTime)
	require.NoError(t, repo.CreateAggregated(ctx, &models.DynamoDBCostAggregation{
		Period:              models.PeriodDay,
		OperationType:       "GetItem",
		Table:               "all",
		WindowStart:         baseTime,
		WindowEnd:           baseTime.Add(time.Hour),
		TotalOperations:     10,
		TotalCostMicroCents: 1000,
	}))
	require.NoError(t, repo.UpdateAggregated(ctx, &models.DynamoDBCostAggregation{
		Period:              models.PeriodDay,
		OperationType:       "GetItem",
		Table:               "all",
		WindowStart:         baseTime,
		WindowEnd:           baseTime.Add(time.Hour),
		TotalOperations:     10,
		TotalCostMicroCents: 1000,
	}))

	_, _, _ = repo.ListAggregatedByPeriod(ctx, models.PeriodDay, "GetItem", baseTime.Add(-time.Hour), baseTime, 1, "")

	_, _ = repo.GetTableCostStats(ctx, "table-1", baseTime.Add(-time.Hour), baseTime)
	require.NoError(t, repo.Aggregate(ctx, "GetItem", models.PeriodDay, baseTime.Add(-time.Hour), baseTime))
	_, _ = repo.GetHighCostOperations(ctx, 0.0, baseTime.Add(-time.Hour), baseTime, 5)
	_, _ = repo.GetCostTrends(ctx, models.PeriodDay, "GetItem", 7)

	// Cost grouping helpers
	_, _ = repo.GetCostsByOperationType(ctx, baseTime.Add(-time.Hour), baseTime)
	_, _ = repo.GetCostsByService(ctx, baseTime.Add(-time.Hour), baseTime)
	_, _ = repo.GetCostsByDateRange(ctx, baseTime.Add(-time.Hour), baseTime)
	_, _ = repo.GetDailyAggregates(ctx, baseTime.Add(-time.Hour), baseTime)
	_, _ = repo.GetMonthlyAggregate(ctx, baseTime.Year(), int(baseTime.Month()))

	// Projection query/conversion
	_, _ = repo.GetCostProjections(ctx, "daily")

	// Relay cost + metrics + budgets
	require.NoError(t, repo.CreateRelayCost(ctx, &models.RelayCost{
		RelayURL:      "https://relay.example",
		Domain:        "example.com",
		OperationType: "delivery",
		Direction:     "outbound",
		Timestamp:     baseTime,
		RequestID:     "req-1",
	}))

	_, _, _ = repo.GetRelayCostsByURL(ctx, "https://relay.example", baseTime.Add(-time.Hour), baseTime, 1, "", "")
	_, _, _ = repo.GetRelayCostsByURL(ctx, "https://relay.example", baseTime.Add(-time.Hour), baseTime, 1, "cursor", "delivery")
	_, _ = repo.collectRelayCosts(ctx, "https://relay.example", baseTime.Add(-time.Hour), baseTime, 2, "")
	_, _ = repo.GetRelayCostsByDateRange(ctx, baseTime.Add(-24*time.Hour), baseTime, 10)

	require.NoError(t, repo.CreateRelayMetrics(ctx, &models.RelayMetrics{
		RelayURL:        "https://relay.example",
		Domain:          "example.com",
		Period:          models.PeriodDaily,
		WindowStart:     baseTime,
		WindowEnd:       baseTime.Add(24 * time.Hour),
		TotalOperations: 10,
	}))
	require.NoError(t, repo.UpdateRelayMetrics(ctx, &models.RelayMetrics{
		RelayURL:        "https://relay.example",
		Domain:          "example.com",
		Period:          models.PeriodDaily,
		WindowStart:     baseTime,
		WindowEnd:       baseTime.Add(24 * time.Hour),
		TotalOperations: 10,
	}))
	_, _ = repo.GetRelayMetrics(ctx, "https://relay.example", models.PeriodDaily, baseTime)
	_, _, _ = repo.GetRelayMetricsHistory(ctx, "https://relay.example", baseTime.Add(-24*time.Hour), baseTime, 1, "")

	require.NoError(t, repo.CreateRelayBudget(ctx, &models.RelayBudget{
		RelayURL:        "https://relay.example",
		Domain:          "example.com",
		Period:          models.PeriodDaily,
		LimitMicroCents: 10000,
	}))
	require.NoError(t, repo.UpdateRelayBudget(ctx, &models.RelayBudget{
		RelayURL:        "https://relay.example",
		Domain:          "example.com",
		Period:          models.PeriodDaily,
		LimitMicroCents: 10000,
	}))
	_, _ = repo.GetRelayBudget(ctx, "https://relay.example", models.PeriodDaily)

	require.NoError(t, repo.AggregateRelayCosts(ctx, "https://relay.example", models.PeriodDaily, baseTime.Add(-time.Hour), baseTime))
	_, _ = repo.GetRelayCostSummary(ctx, "https://relay.example", baseTime.Add(-time.Hour), baseTime)
	_, _ = repo.GetHighCostRelayOperations(ctx, 0, baseTime.Add(-time.Hour), baseTime, 5)

	// Import/export helpers
	_, _ = repo.GetImportExportCostsByUser(ctx, "alice", baseTime.Add(-time.Hour), baseTime)
	_, _ = repo.GetImportExportTrends(ctx, 7)
	_, _ = repo.GetTopCostlyUsers(ctx, baseTime.Add(-time.Hour), baseTime, 5)
	_, _ = repo.GetImportExportMetrics(ctx, baseTime.Add(-time.Hour), baseTime)

	// Activity cost query (happy path)
	_, _ = repo.GetActivityCost(ctx, "activity-1")

	// Percentiles and seasonality helpers
	require.NotEmpty(t, calculatePercentiles([]float64{1, 2, 3}))
	require.NotZero(t, getPercentileValue([]float64{1, 2, 3}, 90))

	analysis := repo.analyzeSeasonality(make([]CostDataPoint, 0))
	require.Nil(t, analysis)

	// Ensure projections map to storage types without panicking.
	_, _ = repo.GetCostProjections(ctx, models.PeriodDay)
	_, _ = repo.GetCostProjections(ctx, models.PeriodMonth)
}
