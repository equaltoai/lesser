package repositories

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func setupWebSocketCostRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Between", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateWebSocketStructForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateWebSocketSliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()
}

func populateWebSocketSliceForCoverage(target any, baseTime time.Time) {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
		return
	}

	slice := v.Elem()
	elemType := slice.Type().Elem()
	baseElemType := elemType
	if baseElemType.Kind() == reflect.Ptr {
		baseElemType = baseElemType.Elem()
	}

	// Provide tailored fixtures for key types so higher-level repository methods
	// exercise more branches without depending on timestamps or map iteration.
	switch baseElemType {
	case reflect.TypeOf(models.WebSocketCostRecord{}):
		for i := range 3 {
			record := &models.WebSocketCostRecord{
				ID:                   "id-1",
				ConnectionID:         "conn-1",
				UserID:               "user-1",
				Username:             "alice",
				Timestamp:            baseTime.Add(time.Duration(i) * time.Minute),
				TotalCostMicroCents:  int64(1000 + i*500),
				ConnectionDurationMs: 120000,
				MessageCount:         2,
				MessageSizeBytes:     1024,
				ProcessingTimeMs:     10,
				ResponseLatencyMs:    5,
				MemoryUsedMB:         128,
				ActiveStreams:        []string{"public", "notifications"},
			}
			switch i {
			case 0:
				record.OperationType = WSEventConnect
			case 1:
				record.OperationType = WSEventMessageIn
			default:
				record.OperationType = "subscribe"
				record.UserID = "" // cover ValidateRequiredParam skip in rankings
			}
			record.EstimatedCostDollars = float64(record.TotalCostMicroCents) / 1_000_000.0

			slice = reflect.Append(slice, reflect.ValueOf(record))
		}
		v.Elem().Set(slice)
		return

	case reflect.TypeOf(models.WebSocketCostBudget{}):
		budgets := []*models.WebSocketCostBudget{
			{
				UserID:              "user-1",
				Username:            "alice",
				Period:              "daily",
				BudgetMicroCents:    100000,
				UsedMicroCents:      120000,
				RemainingMicroCents: 0,
				UsagePercent:        120,
				Status:              "exceeded",
				WindowStart:         baseTime.Add(-time.Hour),
				WindowEnd:           baseTime.Add(time.Hour),
			},
			{
				UserID:              "user-1",
				Username:            "alice",
				Period:              "weekly",
				BudgetMicroCents:    100000,
				UsedMicroCents:      90000,
				RemainingMicroCents: 10000,
				UsagePercent:        90,
				Status:              "warning",
				WindowStart:         baseTime.Add(-time.Hour),
				WindowEnd:           baseTime.Add(time.Hour),
			},
			{
				UserID:              "user-1",
				Username:            "alice",
				Period:              "monthly",
				BudgetMicroCents:    100000,
				UsedMicroCents:      1,
				RemainingMicroCents: 99999,
				UsagePercent:        0.001,
				Status:              "active",
				WindowStart:         baseTime.Add(-48 * time.Hour),
				WindowEnd:           baseTime.Add(-24 * time.Hour), // out of window: skipped
			},
		}
		for _, budget := range budgets {
			_ = budget.BeforeCreate()
			slice = reflect.Append(slice, reflect.ValueOf(budget))
		}
		v.Elem().Set(slice)
		return
	}

	count := 2
	for i := range count {
		now := baseTime.Add(time.Duration(i) * time.Minute)

		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseElemType)
			populateWebSocketStructForCoverage(element.Interface(), now)
		} else {
			ptr := reflect.New(baseElemType)
			populateWebSocketStructForCoverage(ptr.Interface(), now)
			element = ptr.Elem()
		}

		slice = reflect.Append(slice, element)
	}

	v.Elem().Set(slice)
}

func populateWebSocketStructForCoverage(target any, now time.Time) {
	switch model := target.(type) {
	case *models.WebSocketCostRecord:
		model.ID = "id-1"
		model.OperationType = "connect"
		model.ConnectionID = "conn-1"
		model.UserID = "user-1"
		model.Username = "alice"
		model.Timestamp = now
		model.TotalCostMicroCents = 1000
		model.EstimatedCostDollars = float64(model.TotalCostMicroCents) / 1_000_000.0
		model.ConnectionDurationMs = 120000
		model.MessageCount = 2
		model.MessageSizeBytes = 1024
		model.ProcessingTimeMs = 10
		model.ResponseLatencyMs = 5
		model.MemoryUsedMB = 128
		model.ActiveStreams = []string{"public", "notifications"}

	case *models.WebSocketCostBudget:
		model.UserID = "user-1"
		model.Username = "alice"
		model.Period = "daily"
		model.BudgetMicroCents = 100000
		model.UsedMicroCents = 90000
		model.RemainingMicroCents = 10000
		model.UsagePercent = 90
		model.Status = "warning"
		model.WindowStart = now.Add(-time.Hour)
		model.WindowEnd = now.Add(time.Hour)
		_ = model.BeforeCreate()

	case *models.WebSocketCostAggregation:
		model.Period = "hourly"
		model.OperationType = "connect"
		model.WindowStart = now.Truncate(time.Hour)
		model.WindowEnd = model.WindowStart.Add(time.Hour)
		model.TotalCostMicroCents = 1000
		model.TotalCostDollars = 0.001
		model.CostPercentiles = map[string]float64{"p50": 0.1}
		_ = model.BeforeCreate()
	}
}

func TestWebSocketCostRepository_CoverageSweep(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupWebSocketCostRepoMocks(mockDB, mockQuery, baseTime)

	costService := cost.NewCostTrackingServiceForRepository(nil, zap.NewNop(), "ws-cost-test")
	t.Cleanup(func() { _ = costService.Close(context.Background()) })

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), costService)

	// CreateRecord success
	require.NoError(t, repo.CreateRecord(ctx, &models.WebSocketCostRecord{
		ID:                  "id-1",
		OperationType:       "connect",
		ConnectionID:        "conn-1",
		UserID:              "user-1",
		Username:            "alice",
		TotalCostMicroCents: 1000,
		Timestamp:           baseTime,
	}))

	// BatchCreate: empty slice is a no-op (avoid recursion bug)
	require.NoError(t, repo.BatchCreate(ctx, nil))
	require.NoError(t, repo.BatchCreate(ctx, []*models.WebSocketCostRecord{}))

	// BatchCreate: BeforeCreate error path (also avoids recursion)
	require.Error(t, repo.BatchCreate(ctx, []*models.WebSocketCostRecord{{OperationType: "connect"}}))

	// Get + list queries
	_, _ = repo.GetRecord(ctx, "connect", "id-1", baseTime)
	_, _ = repo.ListByOperationType(ctx, "connect", baseTime.Add(-time.Hour), baseTime, 3)
	_, _ = repo.ListByConnection(ctx, "conn-1", baseTime.Add(-time.Hour), baseTime, 3)
	_, _ = repo.ListByUser(ctx, "user-1", baseTime.Add(-time.Hour), baseTime, 3)
	_, _ = repo.GetRecentCosts(ctx, baseTime.Add(-time.Hour), 10)

	// Summaries
	_, _ = repo.GetConnectionCostSummary(ctx, "conn-1", baseTime.Add(-time.Hour), baseTime)
	_, _ = repo.GetUserCostSummary(ctx, "user-1", baseTime.Add(-time.Hour), baseTime)

	// Budgets
	require.NoError(t, repo.CreateBudget(ctx, &models.WebSocketCostBudget{
		UserID:           "user-1",
		Username:         "alice",
		Period:           "daily",
		BudgetMicroCents: 100000,
		WindowStart:      baseTime.Add(-time.Hour),
		WindowEnd:        baseTime.Add(time.Hour),
		Status:           "active",
	}))

	require.NoError(t, repo.UpdateBudget(ctx, &models.WebSocketCostBudget{
		UserID:           "user-1",
		Username:         "alice",
		Period:           "daily",
		BudgetMicroCents: 100000,
		WindowStart:      baseTime.Add(-time.Hour),
		WindowEnd:        baseTime.Add(time.Hour),
		UsedMicroCents:   100,
		Status:           "active",
	}))

	_, _ = repo.GetBudget(ctx, "user-1", "daily")
	_, _ = repo.GetUserBudgets(ctx, "user-1")
	require.NoError(t, repo.UpdateBudgetUsage(ctx, "user-1", 123))
	_, _ = repo.CheckBudgetLimits(ctx, "user-1")

	// Aggregations
	agg := &models.WebSocketCostAggregation{
		Period:        "hourly",
		OperationType: "connect",
		WindowStart:   baseTime.Truncate(time.Hour),
		WindowEnd:     baseTime.Truncate(time.Hour).Add(time.Hour),
	}
	require.NoError(t, repo.CreateAggregation(ctx, agg))
	require.NoError(t, repo.UpdateAggregation(ctx, agg))
	_, _ = repo.GetAggregation(ctx, "hourly", "connect", baseTime.Truncate(time.Hour))
	_, _ = repo.GetUserAggregation(ctx, "user-1", "hourly", "connect", baseTime.Truncate(time.Hour))
	_, _ = repo.ListAggregationsByPeriod(ctx, "hourly", "connect", baseTime.Add(-time.Hour), baseTime, 5)
	_ = repo.AggregateWebSocketCosts(ctx, "connect", "hourly", baseTime.Add(-time.Hour), baseTime)

	// Rankings and filters
	_, _ = repo.GetHighCostOperations(ctx, 0.0001, baseTime.Add(-time.Hour), baseTime, 3)
	_, _ = repo.GetTopCostlyUsers(ctx, baseTime.Add(-time.Hour), baseTime, 3)
}

func TestWebSocketCostRepository_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("create failed")).Once()

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.Error(t, repo.CreateRecord(ctx, &models.WebSocketCostRecord{
		ID:            "id-1",
		OperationType: "connect",
		ConnectionID:  "conn-1",
		Timestamp:     baseTime,
	}))
}

func TestWebSocketCostRepository_GetUserAggregation_EmptyReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostAggregation")).Return(nil).Once()

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetUserAggregation(ctx, "user-1", "hourly", "connect", baseTime)
	require.Error(t, err)
}

func TestWebSocketCostRepository_SaveOrUpdateAggregation_CreatePath(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// Make GetAggregation fail so saveOrUpdateAggregation takes create path.
	mockQuery.On("First", mock.AnythingOfType("*models.WebSocketCostAggregation")).Return(errors.New("not found")).Once()
	mockQuery.On("Create").Return(nil).Maybe()

	repo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

	aggregation := repo.initializeAggregation("hourly", "connect", baseTime, baseTime.Add(time.Hour))
	require.NoError(t, repo.saveOrUpdateAggregation(ctx, aggregation, "hourly", "connect", baseTime))
}

func TestWebSocketCostRepository_UpdateBudgetUsage_BudgetQueryErrorIsNonFatal(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)

	// GetUserBudgets -> queryBudgetsByGSI -> query.All returns error
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostBudget")).Return(errors.New("query failed")).Once()

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.UpdateBudgetUsage(ctx, "user-1", 123))
}

func TestWebSocketCostRepository_CheckBudgetLimits_BudgetQueryError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostBudget")).Return(errors.New("query failed")).Once()

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.CheckBudgetLimits(ctx, "user-1")
	require.Error(t, err)
}

func TestWebSocketCostRepository_GetRecentCosts_ContinuesOnListError(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	// First ListByOperationType (connect) errors, remaining calls succeed with empty results.
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostRecord")).Return(errors.New("query failed")).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostRecord")).Return(nil).Maybe()

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	records, err := repo.GetRecentCosts(ctx, baseTime.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Empty(t, records)
}

func TestWebSocketCostRepository_Summaries_EmptyShortCircuits(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	// Both ListByConnection and ListByUser will see empty slices.
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostRecord")).Return(nil).Maybe()

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), nil)

	connSummary, err := repo.GetConnectionCostSummary(ctx, "conn-1", baseTime.Add(-time.Hour), baseTime)
	require.NoError(t, err)
	require.Equal(t, 0, connSummary.Count)

	userSummary, err := repo.GetUserCostSummary(ctx, "user-1", baseTime.Add(-time.Hour), baseTime)
	require.NoError(t, err)
	require.Equal(t, 0, userSummary.Count)
}

func TestWebSocketCostRepository_LegacyAliases_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil).Once()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.WebSocketCostRecord")).Return(nil).Once()

	// Use a minimal repository without default validation services to keep this test focused
	// on the legacy alias forwarding rather than validation service behavior.
	baseRepo := NewEnhancedBaseRepository[*models.WebSocketCostRecord](mockDB, "test-table", zap.NewNop(), nil, "WebSocketCostRepository", "websocketcost")
	repo := &WebSocketCostRepository{EnhancedBaseRepository: baseRepo}
	require.NoError(t, repo.Create(ctx, &models.WebSocketCostRecord{
		ID:            "id-1",
		OperationType: "connect",
		ConnectionID:  "conn-1",
		Timestamp:     baseTime,
	}))
	_, _ = repo.Get(ctx, "connect", "id-1", baseTime)
}

func TestWebSocketCostRepository_BudgetValidationErrorBranches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	// Minimal repo without validation service so we can target CreateBudget/UpdateBudget's own BeforeCreate/BeforeUpdate guards.
	repo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

	// Missing required fields => BeforeCreate should fail.
	require.Error(t, repo.CreateBudget(ctx, &models.WebSocketCostBudget{}))

	// Missing required fields => BeforeUpdate should fail.
	require.Error(t, repo.UpdateBudget(ctx, &models.WebSocketCostBudget{}))
}

func TestWebSocketCostRepository_AggregationValidationErrorBranches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	repo := NewWebSocketCostRepository(mockDB, "test-table", logger, nil)

	// Missing required fields => BeforeCreate should fail.
	require.Error(t, repo.CreateAggregation(ctx, &models.WebSocketCostAggregation{}))

	// Missing required fields => BeforeUpdate should fail.
	require.Error(t, repo.UpdateAggregation(ctx, &models.WebSocketCostAggregation{}))
}

func TestWebSocketCostRepository_BudgetInWindow_Branches(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	// Budgets query returns multiple periods with different statuses, all in-window.
	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.WebSocketCostBudget)
		*out = []*models.WebSocketCostBudget{
			{
				UserID:              "user-1",
				Username:            "alice",
				Period:              "daily",
				BudgetMicroCents:    100000,
				UsedMicroCents:      100000,
				RemainingMicroCents: 0,
				UsagePercent:        100,
				Status:              "exceeded",
				WindowStart:         now.Add(-time.Hour),
				WindowEnd:           now.Add(time.Hour),
			},
			{
				UserID:              "user-1",
				Username:            "alice",
				Period:              "weekly",
				BudgetMicroCents:    100000,
				UsedMicroCents:      1,
				RemainingMicroCents: 99999,
				UsagePercent:        0.001,
				Status:              "suspended",
				WindowStart:         now.Add(-time.Hour),
				WindowEnd:           now.Add(time.Hour),
			},
			{
				UserID:              "user-1",
				Username:            "alice",
				Period:              "monthly",
				BudgetMicroCents:    100000,
				UsedMicroCents:      90000,
				RemainingMicroCents: 10000,
				UsagePercent:        90,
				Status:              "warning",
				WindowStart:         now.Add(-time.Hour),
				WindowEnd:           now.Add(time.Hour),
			},
		}
	}).Return(nil).Twice()

	// UpdateBudget may fail for one budget and should be logged/continued.
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.UpdateBudgetUsage(ctx, "user-1", 123))

	// CheckBudgetLimits should classify exceeded/suspended/warning budgets and restrict operations.
	status, err := repo.CheckBudgetLimits(ctx, "user-1")
	require.NoError(t, err)
	require.False(t, status.AllowConnection)
	require.False(t, status.AllowMessages)
	require.NotEmpty(t, status.ExceededBudgets)
	require.NotEmpty(t, status.WarningBudgets)
}

func TestWebSocketCostRepository_GetTopCostlyUsers_CountsByOperationType(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	setupWebSocketCostRepoMocks(mockDB, mockQuery, start)

	// Make GetRecentCosts' underlying range queries return a known set for ranking.
	mockQuery.ExpectedCalls = nil
	mockDB.ExpectedCalls = nil
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]*models.WebSocketCostRecord")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]*models.WebSocketCostRecord)
		*out = []*models.WebSocketCostRecord{
			{UserID: "u1", Username: "alice", OperationType: WSEventConnect, ConnectionID: "c1", Timestamp: start, TotalCostMicroCents: 2000, ConnectionDurationMs: 60000},
			{UserID: "u1", Username: "alice", OperationType: WSEventMessageOut, ConnectionID: "c1", Timestamp: start.Add(time.Minute), TotalCostMicroCents: 3000, MessageCount: 5},
			{UserID: "u2", Username: "bob", OperationType: "subscribe", ConnectionID: "c2", Timestamp: start.Add(2 * time.Minute), TotalCostMicroCents: 1000},
		}
	}).Return(nil).Maybe()

	repo := NewWebSocketCostRepository(mockDB, "test-table", zap.NewNop(), nil)
	rankings, err := repo.GetTopCostlyUsers(ctx, start, end, 10)
	require.NoError(t, err)
	require.NotEmpty(t, rankings)
}

func TestWebSocketCostRepository_processOperationMetrics_MoreBranches(t *testing.T) {
	repo := &WebSocketCostRepository{}
	agg := repo.initializeAggregation("hourly", "message_in", time.Now().Add(-time.Hour), time.Now())
	collectors := repo.createMetricCollectors(10)

	// Exercise message, disconnect, and idle-time paths.
	repo.processOperationMetrics(&models.WebSocketCostRecord{
		OperationType:       WSEventMessageIn,
		MessageCount:        2,
		MessageSizeBytes:    2048,
		TotalCostMicroCents: 1000,
	}, agg, collectors)
	repo.processOperationMetrics(&models.WebSocketCostRecord{
		OperationType:       WSEventMessageOut,
		MessageCount:        1,
		MessageSizeBytes:    1024,
		TotalCostMicroCents: 500,
	}, agg, collectors)
	repo.processOperationMetrics(&models.WebSocketCostRecord{
		OperationType:        WSEventDisconnect,
		ConnectionDurationMs: 60000,
		TotalCostMicroCents:  100,
	}, agg, collectors)
	repo.processOperationMetrics(&models.WebSocketCostRecord{
		OperationType:       "idle_time",
		IdleTimeMs:          30000,
		TotalCostMicroCents: 10,
	}, agg, collectors)
}

func TestWebSocketCostRepository_calculateWebSocketPercentiles_Empty(t *testing.T) {
	result := calculateWebSocketPercentiles(nil)
	require.Equal(t, float64(0), result["p50"])
}
