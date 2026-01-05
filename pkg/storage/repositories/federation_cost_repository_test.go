package repositories

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newFederationCostRepoForTest(mockDB *mocks.MockDB, tableName string) *FederationCostRepository {
	logger := zap.NewNop()
	mainRepo := NewEnhancedBaseRepository[*models.FederationCostTracking](mockDB, tableName, logger, nil, "FederationCostRepository", "federation_cost")
	budgetRepo := NewEnhancedBaseRepository[*models.FederationBudget](mockDB, tableName, logger, nil, "FederationBudgetRepository", "federation_budget")
	return NewFederationCostRepository(mainRepo, budgetRepo)
}

func TestFederationCostRepository_Constructors(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()

	enhancedMain := NewEnhancedBaseRepository[*models.FederationCostTracking](mockDB, "test-table", logger, nil, "FederationCostRepository", "federation_cost")
	enhancedBudget := NewEnhancedBaseRepository[*models.FederationBudget](mockDB, "test-table", logger, nil, "FederationBudgetRepository", "federation_budget")

	repo := NewFederationCostRepository(enhancedMain, enhancedBudget)
	require.NotNil(t, repo)

	// FromBase path exercises convertBaseToEnhanced helper.
	baseMain := NewBaseRepositoryWithCostTracking[*models.FederationCostTracking](mockDB, "test-table", logger, nil, "FederationCostRepository")
	baseBudget := NewBaseRepositoryWithCostTracking[*models.FederationBudget](mockDB, "test-table", logger, nil, "FederationBudgetRepository")

	repo2 := NewFederationCostRepositoryFromBase(baseMain, baseBudget, nil)
	require.NotNil(t, repo2)

	converted := convertBaseToEnhanced(baseMain, nil, "x", "y")
	require.NotNil(t, converted)
	require.Equal(t, "y", converted.entityName)
}

func TestFederationCostRepository_RecordFederationCost_SuccessAndError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := newFederationCostRepoForTest(mockDB, "test-table")

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil).Once()

	record := &models.FederationCostTracking{
		Domain:             "example.com",
		ActivityType:       "Create",
		ActivityID:         "act-1",
		Timestamp:          time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		Success:            true,
		TotalCostMicroCents: 123,
		DataTransferBytes:  100,
		ResponseTimeMs:     50,
	}
	require.NoError(t, repo.RecordFederationCost(ctx, record))

	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	repo2 := newFederationCostRepoForTest(mockDB2, "test-table")

	mockDB2.On("WithContext", ctx).Return(mockDB2)
	mockDB2.On("Model", mock.Anything).Return(mockQuery2)
	mockQuery2.On("Create").Return(errors.New("create failed")).Once()

	require.Error(t, repo2.RecordFederationCost(ctx, record))
}

func TestFederationCostRepository_GetFederationCosts_SuccessAndError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := newFederationCostRepoForTest(mockDB, "test-table")

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationCostTracking")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.FederationCostTracking)
			*out = []*models.FederationCostTracking{
				{Domain: "example.com", ActivityType: "Create", ActivityID: "a1", Timestamp: start, TotalCostMicroCents: 10},
				{Domain: "example.com", ActivityType: "Follow", ActivityID: "a2", Timestamp: start.Add(time.Minute), TotalCostMicroCents: 20},
			}
		}).
		Return(nil).
		Once()

	costs, err := repo.GetFederationCosts(ctx, "example.com", start, end, 10)
	require.NoError(t, err)
	require.Len(t, costs, 2)

	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	repo2 := newFederationCostRepoForTest(mockDB2, "test-table")

	mockDB2.On("WithContext", ctx).Return(mockDB2)
	mockDB2.On("Model", mock.Anything).Return(mockQuery2)
	mockQuery2.On("Index", "gsi1").Return(mockQuery2)
	mockQuery2.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery2)
	mockQuery2.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery2)
	mockQuery2.On("Limit", mock.Anything).Return(mockQuery2)
	mockQuery2.On("All", mock.Anything).Return(errors.New("scan failed")).Once()

	_, err = repo2.GetFederationCosts(ctx, "example.com", start, end, 10)
	require.Error(t, err)
}

func TestFederationCostRepository_GetFederationCostsByActivityType_SuccessAndError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := newFederationCostRepoForTest(mockDB, "test-table")

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationCostTracking")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.FederationCostTracking)
			*out = []*models.FederationCostTracking{{ActivityType: "Follow"}}
		}).
		Return(nil).
		Once()

	costs, err := repo.GetFederationCostsByActivityType(ctx, "Follow", start, end, 10)
	require.NoError(t, err)
	require.Len(t, costs, 1)

	mockDB2 := new(mocks.MockDB)
	mockQuery2 := new(mocks.MockQuery)
	repo2 := newFederationCostRepoForTest(mockDB2, "test-table")

	mockDB2.On("WithContext", ctx).Return(mockDB2)
	mockDB2.On("Model", mock.Anything).Return(mockQuery2)
	mockQuery2.On("Index", "gsi2").Return(mockQuery2)
	mockQuery2.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery2)
	mockQuery2.On("Limit", mock.Anything).Return(mockQuery2)
	mockQuery2.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	_, err = repo2.GetFederationCostsByActivityType(ctx, "Follow", start, end, 10)
	require.Error(t, err)
}

func TestFederationCostRepository_GetDailyCostSummary_ComputesBreakdowns(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := newFederationCostRepoForTest(mockDB, "test-table")

	day := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationCostTracking")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.FederationCostTracking)
			*out = []*models.FederationCostTracking{
				{Domain: "example.com", ActivityType: "Create", ActivityID: "a1", Success: true, TotalCostMicroCents: 100, DataTransferBytes: 1000, ResponseTimeMs: 50, Timestamp: day.Add(time.Hour)},
				{Domain: "example.com", ActivityType: "Create", ActivityID: "a2", Success: false, TotalCostMicroCents: 50, DataTransferBytes: 500, ResponseTimeMs: 100, Timestamp: day.Add(2 * time.Hour)},
				{Domain: "example.com", ActivityType: "Follow", ActivityID: "a3", Success: true, TotalCostMicroCents: 25, DataTransferBytes: 250, ResponseTimeMs: 10, Timestamp: day.Add(3 * time.Hour)},
			}
		}).
		Return(nil).
		Once()

	summary, err := repo.GetDailyCostSummary(ctx, "example.com", day)
	require.NoError(t, err)
	require.Equal(t, int64(3), summary.TotalActivities)
	require.Equal(t, int64(175), summary.TotalCostMicroCents)
	require.Len(t, summary.ActivityTypeBreakdown, 2)
}

func TestFederationCostRepository_BudgetsAndChecks_Coverage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	// CreateOrUpdateBudget success
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mainRepo := NewEnhancedBaseRepository[*models.FederationCostTracking](mockDB, "test-table", logger, nil, "FederationCostRepository", "federation_cost")
	budgetRepo := NewEnhancedBaseRepository[*models.FederationBudget](mockDB, "test-table", logger, nil, "FederationBudgetRepository", "federation_budget")
	repo := NewFederationCostRepository(mainRepo, budgetRepo)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Create").Return(nil).Maybe()

	budget := &models.FederationBudget{
		Domain:                  "example.com",
		Period:                  PeriodDaily,
		CombinedLimitMicroCents: 100,
		AlertThresholdPercent:   75.0,
		AlertSendingEnabled:     true,
		IsActive:                true,
		ActivityTypeLimits:      map[string]int64{"Create": 60},
		ActivityTypeUsage:       map[string]int64{"Create": 0},
	}
	require.NoError(t, repo.CreateOrUpdateBudget(ctx, budget))

	// GetBudget success path uses BaseRepository.Get (Where+First)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*models.FederationBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.FederationBudget)
		*out = *budget
	}).Return(nil).Once()

	got, err := repo.GetBudget(ctx, "example.com", PeriodDaily)
	require.NoError(t, err)
	require.Equal(t, "example.com", got.Domain)

	// UpdateBudgetUsage covers status transitions; uses GetBudget then CreateOrUpdateBudget.
	mockQuery.On("First", mock.AnythingOfType("*models.FederationBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.FederationBudget)
		*out = models.FederationBudget{
			Domain:                  "example.com",
			Period:                  PeriodDaily,
			CombinedLimitMicroCents: 100,
			AlertThresholdPercent:   75.0,
			AlertSendingEnabled:     true,
			IsActive:                true,
			ActivityTypeLimits:      map[string]int64{},
			ActivityTypeUsage:       map[string]int64{},
			CurrentCombinedCost:     0,
		}
	}).Return(nil).Once()
	require.NoError(t, repo.UpdateBudgetUsage(ctx, "example.com", PeriodDaily, "Create", "outbound", 10))

	// GetActiveBudgets + derived over-limit/alerts methods
	mockQuery.On("Index", "gsi1").Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationBudget")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.FederationBudget)
			// budget1 over combined limit
			b1 := &models.FederationBudget{
				Domain:                  "a.example",
				Period:                  PeriodDaily,
				CombinedLimitMicroCents: 100,
				CurrentCombinedCost:     120,
				AlertThresholdPercent:   75,
				AlertSendingEnabled:     true,
				IsActive:                true,
			}
			// budget2 needs alert
			b2 := &models.FederationBudget{
				Domain:                  "b.example",
				Period:                  PeriodDaily,
				CombinedLimitMicroCents: 100,
				CurrentCombinedCost:     80,
				AlertThresholdPercent:   75,
				AlertSendingEnabled:     true,
				IsActive:                true,
			}
			*out = []*models.FederationBudget{b1, b2}
		}).
		Return(nil).
		Once()

	over, err := repo.GetBudgetsOverLimit(ctx, 10)
	require.NoError(t, err)
	require.Len(t, over, 1)

	// call again with fresh All result
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationBudget")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.FederationBudget)
			b2 := &models.FederationBudget{
				Domain:                  "b.example",
				Period:                  PeriodDaily,
				CombinedLimitMicroCents: 100,
				CurrentCombinedCost:     80,
				AlertThresholdPercent:   75,
				AlertSendingEnabled:     true,
				IsActive:                true,
			}
			*out = []*models.FederationBudget{b2}
		}).
		Return(nil).
		Once()

	alerts, err := repo.GetBudgetsNeedingAlerts(ctx, 10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	// CheckBudgetLimits core branches (activity type limit exceeded, combined exceeded, rate limit, warning, normal)
	testBudget := &models.FederationBudget{
		Domain:                  "example.com",
		Period:                  PeriodDaily,
		CombinedLimitMicroCents: 100,
		CurrentCombinedCost:     50,
		AlertThresholdPercent:   75,
		RateLimitOnThreshold:    true,
		ActivityTypeLimits:      map[string]int64{"Create": 60},
		ActivityTypeUsage:       map[string]int64{"Create": 59},
	}

	mockQuery.On("First", mock.AnythingOfType("*models.FederationBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.FederationBudget)
		*out = *testBudget
	}).Return(nil).Once()

	result, err := repo.CheckBudgetLimits(ctx, "example.com", PeriodDaily, "Create", "outbound", 2)
	require.NoError(t, err)
	require.False(t, result.Allowed)

	testBudget.ActivityTypeUsage["Create"] = 0
	testBudget.CurrentCombinedCost = 99
	mockQuery.On("First", mock.AnythingOfType("*models.FederationBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.FederationBudget)
		*out = *testBudget
	}).Return(nil).Once()
	result, err = repo.CheckBudgetLimits(ctx, "example.com", PeriodDaily, "Create", "outbound", 2)
	require.NoError(t, err)
	require.False(t, result.Allowed)

	testBudget.CurrentCombinedCost = 74
	mockQuery.On("First", mock.AnythingOfType("*models.FederationBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.FederationBudget)
		*out = *testBudget
	}).Return(nil).Once()
	result, err = repo.CheckBudgetLimits(ctx, "example.com", PeriodDaily, "Create", "outbound", 2) // 76% => rate limit
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.True(t, result.ShouldRateLimit)

	testBudget.RateLimitOnThreshold = false
	mockQuery.On("First", mock.AnythingOfType("*models.FederationBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.FederationBudget)
		*out = *testBudget
	}).Return(nil).Once()
	result, err = repo.CheckBudgetLimits(ctx, "example.com", PeriodDaily, "Create", "outbound", 2) // warning without rate limit
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, StatusWarning, result.WarningLevel)

	testBudget.CurrentCombinedCost = 10
	testBudget.AlertThresholdPercent = 75
	mockQuery.On("First", mock.AnythingOfType("*models.FederationBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.FederationBudget)
		*out = *testBudget
	}).Return(nil).Once()
	result, err = repo.CheckBudgetLimits(ctx, "example.com", PeriodDaily, "Create", "outbound", 2)
	require.NoError(t, err)
	require.True(t, result.Allowed)
	require.Equal(t, "info", result.WarningLevel)

	// ResetPeriodBudgets exercises per-budget reset and save loop.
	newStart := time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC)
	newEnd := newStart.Add(24 * time.Hour)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.FederationBudget")).
		Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]*models.FederationBudget)
			*out = []*models.FederationBudget{{Domain: "example.com", Period: PeriodDaily, IsActive: true, CombinedLimitMicroCents: 100}}
		}).
		Return(nil).
		Once()
	mockQuery.On("Create").Return(nil).Maybe()
	require.NoError(t, repo.ResetPeriodBudgets(ctx, PeriodDaily, newStart, newEnd))
}

func TestFederationCostRepository_GetBudget_ErrorPaths(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := newFederationCostRepoForTest(mockDB, "test-table")

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(errors.New("db error")).Once()

	_, err := repo.GetBudget(ctx, "example.com", PeriodDaily)
	require.Error(t, err)

	// Note: the "not found" branch uses dynamormErrors.IsNotFound on the result of BaseRepository.Get.
	// BaseRepository.Get wraps not-found errors into app errors, so this branch is effectively unreachable here.
	_ = dynamormErrors.ErrItemNotFound
}

func TestFederationCostRepository_createDefaultBudget_SwitchCoverage(t *testing.T) {
	repo := &FederationCostRepository{}
	require.NotNil(t, repo.createDefaultBudget("example.com", PeriodDaily))
	require.NotNil(t, repo.createDefaultBudget("example.com", PeriodWeekly))
	require.NotNil(t, repo.createDefaultBudget("example.com", PeriodMonthly))
	require.NotNil(t, repo.createDefaultBudget("example.com", "unknown"))
}

func TestFederationCostRepository_GetActiveBudgets_Error(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := newFederationCostRepoForTest(mockDB, "test-table")

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(fmt.Errorf("scan failed")).Once()

	_, err := repo.GetActiveBudgets(ctx, 10)
	require.Error(t, err)
}

