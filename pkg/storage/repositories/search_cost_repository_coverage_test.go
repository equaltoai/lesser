package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// RecordSearchCost Tests
// ============================================================================

func TestSearchCostRepository_RecordSearchCost_NilInput(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	err := repo.RecordSearchCost(ctx, nil)

	require.Error(t, err)
}

func TestSearchCostRepository_RecordSearchCost_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	costData := &models.SearchCostTracking{
		UserID:        "user-123",
		OperationType: "search",
		SearchType:    "full_text",
		Query:         "test query",
		ResultCount:   10,
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

	err := repo.RecordSearchCost(ctx, costData)

	require.NoError(t, err)
	require.GreaterOrEqual(t, costData.TotalCostMicros, int64(0))
}

func TestSearchCostRepository_RecordSearchCost_ZeroResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	costData := &models.SearchCostTracking{
		UserID:        "user-123",
		OperationType: "search",
		SearchType:    "full_text",
		Query:         "no results query",
		ResultCount:   0,
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Maybe()

	err := repo.RecordSearchCost(ctx, costData)

	require.NoError(t, err)
	require.Equal(t, int64(0), costData.CostPerResult)
}

func TestSearchCostRepository_RecordSearchCost_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	costData := &models.SearchCostTracking{
		UserID:        "user-123",
		OperationType: "search",
		SearchType:    "full_text",
		Query:         "test query",
		ResultCount:   10,
	}

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("create failed"))

	err := repo.RecordSearchCost(ctx, costData)

	require.Error(t, err)
}

// ============================================================================
// CheckBudget Tests
// ============================================================================

func TestSearchCostRepository_CheckBudget_WithinLimit(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Run(func(args mock.Arguments) {
		budget := args.Get(0).(*models.SearchBudget)
		budget.UserID = "user-123"
		budget.Period = "daily"
		budget.BudgetLimitMicros = 100000
		budget.UsedBudgetMicros = 10000
		budget.SearchBudgetMicros = 50000
		budget.SearchUsedMicros = 5000
		budget.MaxRequestsPerHour = 1000
		budget.CurrentRequests = 5
	}).Return(nil)

	err := repo.CheckBudget(ctx, "user-123", "text_search", 1000)

	require.NoError(t, err)
}

func TestSearchCostRepository_CheckBudget_Exceeded(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Run(func(args mock.Arguments) {
		budget := args.Get(0).(*models.SearchBudget)
		budget.UserID = "user-123"
		budget.Period = "daily"
		budget.BudgetLimitMicros = 100000
		budget.UsedBudgetMicros = 99000 // Almost at limit
	}).Return(nil)

	err := repo.CheckBudget(ctx, "user-123", "text_search", 100000)

	require.Error(t, err)
	require.Contains(t, err.Error(), "budget")
}

func TestSearchCostRepository_CheckBudget_NoBudgetCreatesDefault(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockCreateQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	// First lookup: not found
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Return(dynamormerrors.ErrItemNotFound).Once()

	// Create default budget
	mockDB.On("Model", mock.Anything).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(nil)

	// Retry lookup
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Run(func(args mock.Arguments) {
		budget := args.Get(0).(*models.SearchBudget)
		budget.UserID = "user-123"
		budget.Period = "daily"
		budget.BudgetLimitMicros = 10000000
		budget.SearchBudgetMicros = 5000000
		budget.MaxRequestsPerHour = 1000
	}).Return(nil).Once()

	err := repo.CheckBudget(ctx, "user-123", "text_search", 1000)

	require.NoError(t, err)
}

// ============================================================================
// GetSearchCosts Tests
// ============================================================================

func TestSearchCostRepository_GetSearchCosts_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // Same day

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	// GetSearchCosts is now a per-day page-capped walk (wave #1469):
	// Limit(500)/page via AllPaginated.
	mockQuery.On("Limit", 500).Return(mockQuery)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchCostTracking")).Run(func(args mock.Arguments) {
		costs := args.Get(0).(*[]models.SearchCostTracking)
		*costs = []models.SearchCostTracking{
			{UserID: "user-123", TotalCostMicros: 100},
			{UserID: "user-123", TotalCostMicros: 200},
		}
	}).Return(&core.PaginatedResult{}, nil)

	results, err := repo.GetSearchCosts(ctx, "user-123", startTime, endTime)

	require.NoError(t, err)
	require.NotNil(t, results)
	require.Len(t, results, 2)
}

func TestSearchCostRepository_GetSearchCosts_NotFoundReturnsEmpty(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	// Page-capped walk (wave #1469): NotFound from AllPaginated → empty.
	mockQuery.On("Limit", 500).Return(mockQuery)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchCostTracking")).Return(nil, dynamormerrors.ErrItemNotFound)

	results, err := repo.GetSearchCosts(ctx, "user-123", startTime, endTime)

	require.NoError(t, err)
	require.Empty(t, results)
}

// ============================================================================
// GetSearchCostSummary Tests
// ============================================================================

func TestSearchCostRepository_GetSearchCostSummary_Daily(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	// GetSearchCostSummary → GetSearchCosts is a page-capped walk (wave #1469).
	mockQuery.On("Limit", 500).Return(mockQuery)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchCostTracking")).Run(func(args mock.Arguments) {
		costs := args.Get(0).(*[]models.SearchCostTracking)
		*costs = []models.SearchCostTracking{
			{TotalCostMicros: 100, ResultCount: 5, CacheHit: true},
			{TotalCostMicros: 200, ResultCount: 10, CacheHit: false},
		}
	}).Return(&core.PaginatedResult{}, nil)

	summary, err := repo.GetSearchCostSummary(ctx, "user-123", "daily")

	require.NoError(t, err)
	require.NotNil(t, summary)
}

func TestSearchCostRepository_GetSearchCostSummary_Weekly(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	// Page-capped walk (wave #1469): Limit(500)/page via AllPaginated.
	mockQuery.On("Limit", 500).Return(mockQuery)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchCostTracking")).Run(func(args mock.Arguments) {
		costs := args.Get(0).(*[]models.SearchCostTracking)
		*costs = []models.SearchCostTracking{}
	}).Return(&core.PaginatedResult{}, nil)

	summary, err := repo.GetSearchCostSummary(ctx, "user-123", "weekly")

	require.NoError(t, err)
	require.NotNil(t, summary)
}

func TestSearchCostRepository_GetSearchCostSummary_Monthly(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	// Page-capped walk (wave #1469): Limit(500)/page via AllPaginated.
	mockQuery.On("Limit", 500).Return(mockQuery)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.SearchCostTracking")).Run(func(args mock.Arguments) {
		costs := args.Get(0).(*[]models.SearchCostTracking)
		*costs = []models.SearchCostTracking{}
	}).Return(&core.PaginatedResult{}, nil)

	summary, err := repo.GetSearchCostSummary(ctx, "user-123", "monthly")

	require.NoError(t, err)
	require.NotNil(t, summary)
}

// ============================================================================
// RecordBudgetUsage Tests
// ============================================================================

func TestSearchCostRepository_RecordBudgetUsage_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Run(func(args mock.Arguments) {
		budget := args.Get(0).(*models.SearchBudget)
		budget.UserID = "user-123"
		budget.Period = "daily"
		budget.BudgetLimitMicros = 100000
		budget.UsedBudgetMicros = 10000
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.RecordBudgetUsage(ctx, "user-123", "search", 5000)

	require.NoError(t, err)
}

func TestSearchCostRepository_RecordBudgetUsage_BudgetNotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Return(dynamormerrors.ErrItemNotFound)

	err := repo.RecordBudgetUsage(ctx, "user-123", "search", 5000)

	require.Error(t, err)
}

func TestSearchCostRepository_RecordBudgetUsage_UpdateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Run(func(args mock.Arguments) {
		budget := args.Get(0).(*models.SearchBudget)
		budget.UserID = "user-123"
		budget.Period = "daily"
		budget.BudgetLimitMicros = 100000
		budget.UsedBudgetMicros = 10000
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed"))

	err := repo.RecordBudgetUsage(ctx, "user-123", "search", 5000)

	require.Error(t, err)
}

// ============================================================================
// GetUserBudget Tests
// ============================================================================

func TestSearchCostRepository_GetUserBudget_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Run(func(args mock.Arguments) {
		budget := args.Get(0).(*models.SearchBudget)
		budget.UserID = "user-123"
		budget.Period = "daily"
		budget.BudgetLimitMicros = 100000
	}).Return(nil)

	budget, err := repo.GetUserBudget(ctx, "user-123", "daily")

	require.NoError(t, err)
	require.NotNil(t, budget)
	require.Equal(t, "user-123", budget.UserID)
}

func TestSearchCostRepository_GetUserBudget_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewSearchCostRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Return(dynamormerrors.ErrItemNotFound)

	budget, err := repo.GetUserBudget(ctx, "user-123", "daily")

	require.Error(t, err)
	require.Nil(t, budget)
}
