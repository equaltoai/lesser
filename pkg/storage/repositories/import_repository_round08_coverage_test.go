package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func setupImportRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
}

func TestImportRepository_Round08_CoverageSweep(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()

	mockQuery.On("First", mock.AnythingOfType("*models.Import")).Run(func(args mock.Arguments) {
		importRecord := args.Get(0).(*models.Import)
		importRecord.ID = "imp-1"
		importRecord.Username = "alice"
		importRecord.Type = "followers"
		importRecord.Mode = "merge"
		importRecord.Status = "processing"
		importRecord.CreatedAt = baseTime
		importRecord.UpdatedAt = baseTime
		importRecord.UpdateKeys()
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]*models.Import")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]*models.Import)
		*items = []*models.Import{
			{ID: "imp-1", Username: "alice", Type: "followers", Status: StatusCompleted, CreatedAt: baseTime.Add(-2 * time.Hour)},
			{ID: "imp-2", Username: "alice", Type: "archive", Status: StatusFailed, CreatedAt: baseTime.Add(-1 * time.Hour)},
		}
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]*models.Import")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]*models.Import)
		*items = []*models.Import{
			{ID: "imp-1", Username: "alice", Status: StatusCompleted, CreatedAt: baseTime.Add(-2 * time.Hour)},
			{ID: "imp-2", Username: "alice", Status: StatusFailed, CreatedAt: baseTime.Add(-1 * time.Hour)},
		}
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]*models.ImportCostTracking")).Run(func(args mock.Arguments) {
		costs := args.Get(0).(*[]*models.ImportCostTracking)
		*costs = []*models.ImportCostTracking{
			{
				ImportID:            "imp-1",
				Username:            "alice",
				Type:                "followers",
				Status:              StatusCompleted,
				Timestamp:           baseTime.Add(-30 * time.Minute),
				ProcessedCount:      10,
				SuccessCount:        9,
				ErrorCount:          1,
				RecordCount:         10,
				TotalCostMicroCents: 2_000_000,
				LambdaExecutionCost: 1_000_000,
				S3StorageCost:       200_000,
				S3GetRequestCost:    100_000,
				S3DataTransferCost:  100_000,
				DynamoDBWriteCost:   300_000,
				DynamoDBReadCost:    200_000,
				ExternalAPICallCost: 100_000,
			},
			{
				ImportID:            "imp-2",
				Username:            "alice",
				Type:                "archive",
				Status:              StatusFailed,
				Timestamp:           baseTime.Add(-20 * time.Minute),
				ProcessedCount:      5,
				SuccessCount:        3,
				ErrorCount:          2,
				RecordCount:         5,
				TotalCostMicroCents: 1_000_000,
				LambdaExecutionCost: 500_000,
			},
		}
	}).Return(nil).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()

	repo := NewImportRepository(mockDB, "test-table", logger)

	err := repo.CreateImport(ctx, &models.Import{
		ID:       "imp-1",
		Username: "alice",
		Type:     "followers",
		Mode:     "merge",
		Status:   StatusPending,
	})
	require.NoError(t, err)

	_, err = repo.GetImport(ctx, "imp-1")
	require.NoError(t, err)

	err = repo.UpdateImportProgress(ctx, "imp-1", 42)
	require.NoError(t, err)

	completion := map[string]any{
		"total":   10,
		"success": 9,
		"skipped": 0,
		"failed":  1,
		"errors":  []string{"row 5 failed"},
	}
	err = repo.UpdateImportStatus(ctx, "imp-1", StatusCompleted, completion, "partial errors")
	require.NoError(t, err)

	_, _, err = repo.GetImportsForUser(ctx, "alice", 2, "")
	require.NoError(t, err)

	_, err = repo.GetUserImportsByStatus(ctx, "alice", []string{StatusCompleted, StatusFailed})
	require.NoError(t, err)

	err = repo.CreateImportCostTracking(ctx, &models.ImportCostTracking{
		ImportID:            "imp-1",
		Username:            "alice",
		Timestamp:           baseTime,
		TotalCostMicroCents: 123,
	})
	require.NoError(t, err)

	_, err = repo.GetImportCostTracking(ctx, "imp-1")
	require.NoError(t, err)

	start := baseTime.Add(-24 * time.Hour)
	end := baseTime
	_, err = repo.GetUserImportCosts(ctx, "alice", start, end, 10)
	require.NoError(t, err)

	_, err = repo.GetImportCostsByDateRange(ctx, start, end, 10)
	require.NoError(t, err)

	summary, err := repo.GetImportCostSummary(ctx, "alice", start, end)
	require.NoError(t, err)
	require.Equal(t, int64(2), summary.TotalImports)
	require.Greater(t, summary.TotalCostMicroCents, int64(0))
	require.Contains(t, summary.TypeBreakdown, "followers")

	_, err = repo.GetHighCostImports(ctx, 1_000_000, baseTime, baseTime, 1)
	require.NoError(t, err)
}

func TestImportRepository_CreateImport_Error(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Create").Return(errors.New("create failed")).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.CreateImport(context.Background(), &models.Import{
		ID:       "imp-err",
		Username: "alice",
		Type:     "followers",
		Mode:     "merge",
		Status:   StatusPending,
	})
	require.Error(t, err)
}

func TestImportRepository_GetImport_NotFound(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetImport(context.Background(), "missing")
	require.Error(t, err)
}

func TestImportRepository_CheckBudgetLimits_ExceedsLimit(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.AnythingOfType("*models.ImportBudget")).Return(dynamormErrors.ErrItemNotFound).Twice()
	mockQuery.On("First", mock.AnythingOfType("*models.ImportBudget")).Run(func(args mock.Arguments) {
		budget := args.Get(0).(*models.ImportBudget)
		budget.Username = "alice"
		budget.Period = "monthly"
		budget.IsActive = true
		budget.ImportLimitMicroCents = 100
		budget.CurrentImportCost = 90
		budget.UpdateKeys()
	}).Return(nil).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	budget, ok, err := repo.CheckBudgetLimits(context.Background(), "alice", 20, 0)
	require.NoError(t, err)
	require.NotNil(t, budget)
	require.False(t, ok)
}

func TestImportRepository_UpdateBudgetUsage_CreatesDefaultOnMissing(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()

	mockQuery.On("First", mock.AnythingOfType("*models.ImportBudget")).Return(dynamormErrors.ErrItemNotFound).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.UpdateBudgetUsage(context.Background(), "alice", "daily", 10, 20)
	require.NoError(t, err)
}

func TestImportRepository_GetImport_OtherError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetImport(context.Background(), "imp-1")
	require.Error(t, err)
}

func TestImportRepository_UpdateImportProgress_UpdateError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.AnythingOfType("*models.Import")).Run(func(args mock.Arguments) {
		importRecord := args.Get(0).(*models.Import)
		importRecord.ID = "imp-1"
		importRecord.Username = "alice"
		importRecord.CreatedAt = baseTime
		importRecord.UpdateKeys()
	}).Return(nil).Once()

	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.UpdateImportProgress(ctx, "imp-1", 1)
	require.Error(t, err)
}

func TestImportRepository_GetImportCostTracking_Error(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetImportCostTracking(context.Background(), "imp-1")
	require.Error(t, err)
}

func TestImportRepository_GetImportCostSummary_EmptyCosts(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("All", mock.AnythingOfType("*[]*models.ImportCostTracking")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ImportCostTracking)
		*dest = nil
	}).Return(nil).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	summary, err := repo.GetImportCostSummary(ctx, "alice", time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(0), summary.TotalImports)
}

func TestImportRepository_CheckBudgetLimits_NoBudgetFound(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.AnythingOfType("*models.ImportBudget")).Return(dynamormErrors.ErrItemNotFound).Times(3)

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	budget, ok, err := repo.CheckBudgetLimits(context.Background(), "alice", 1, 1)
	require.NoError(t, err)
	require.Nil(t, budget)
	require.True(t, ok)
}

func TestImportRepository_UpdateBudgetUsage_ExistingBudget(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.AnythingOfType("*models.ImportBudget")).Run(func(args mock.Arguments) {
		budget := args.Get(0).(*models.ImportBudget)
		budget.Username = "alice"
		budget.Period = "daily"
		budget.IsActive = true
		budget.UpdateKeys()
	}).Return(nil).Once()

	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.UpdateBudgetUsage(ctx, "alice", "daily", 10, 20)
	require.NoError(t, err)
}

func TestImportRepository_UpdateImportStatus_NoCompletionData_WithErrorMsg(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.AnythingOfType("*models.Import")).Run(func(args mock.Arguments) {
		importRecord := args.Get(0).(*models.Import)
		importRecord.ID = "imp-1"
		importRecord.Username = "alice"
		importRecord.CreatedAt = baseTime
		importRecord.UpdateKeys()
	}).Return(nil).Once()

	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.UpdateImportStatus(ctx, "imp-1", StatusFailed, nil, "failed")
	require.NoError(t, err)
}

func TestImportRepository_CreateImportCostTracking_CreateError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Create").Return(errors.New("create failed")).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.CreateImportCostTracking(context.Background(), &models.ImportCostTracking{
		ImportID:            "imp-1",
		Username:            "alice",
		Timestamp:           time.Now(),
		TotalCostMicroCents: 1,
	})
	require.Error(t, err)
}

func TestImportRepository_GetImportBudget_NotFound(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.AnythingOfType("*models.ImportBudget")).Return(dynamormErrors.ErrItemNotFound).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetImportBudget(context.Background(), "alice", "daily")
	require.Error(t, err)
}

func TestImportRepository_CheckBudgetLimits_CombinedLimitExceeded(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.AnythingOfType("*models.ImportBudget")).Run(func(args mock.Arguments) {
		budget := args.Get(0).(*models.ImportBudget)
		budget.Username = "alice"
		budget.Period = "daily"
		budget.IsActive = true
		budget.CombinedLimitMicroCents = 100
		budget.CurrentCombinedCost = 95
		budget.UpdateKeys()
	}).Return(nil).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	budget, ok, err := repo.CheckBudgetLimits(context.Background(), "alice", 10, 10)
	require.NoError(t, err)
	require.NotNil(t, budget)
	require.False(t, ok)
}

func TestImportRepository_CreateImportBudget_CreateError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Create").Return(errors.New("create failed")).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.CreateImportBudget(context.Background(), &models.ImportBudget{
		Username: "alice",
		Period:   "daily",
	})
	require.Error(t, err)
}

func TestImportRepository_UpdateImportBudget_UpdateError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.UpdateImportBudget(context.Background(), &models.ImportBudget{
		Username: "alice",
		Period:   "daily",
	})
	require.Error(t, err)
}

func TestImportRepository_GetImportBudget_OtherError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupImportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.AnythingOfType("*models.ImportBudget")).Return(errors.New("boom")).Once()

	repo := NewImportRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetImportBudget(context.Background(), "alice", "daily")
	require.Error(t, err)
}
