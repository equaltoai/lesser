package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func setupExportRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
}

func TestExportRepository_Round08_CoverageSweep(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 11, 0, 0, 0, time.UTC)
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupExportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()

	mockQuery.On("First", mock.AnythingOfType("*models.Export")).Run(func(args mock.Arguments) {
		exportRecord := args.Get(0).(*models.Export)
		exportRecord.ID = "exp-1"
		exportRecord.Username = "alice"
		exportRecord.Type = "archive"
		exportRecord.Format = "mastodon"
		exportRecord.Status = "processing"
		exportRecord.CreatedAt = baseTime
		exportRecord.UpdatedAt = baseTime
		exportRecord.UpdateKeys()
	}).Return(nil).Maybe()

	mockQuery.On("Scan", mock.AnythingOfType("*[]*models.Export")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]*models.Export)
		*items = []*models.Export{
			{ID: "exp-1", Username: "alice", Type: "archive", Status: StatusCompleted, CreatedAt: baseTime.Add(-2 * time.Hour)},
			{ID: "exp-2", Username: "alice", Type: "followers", Status: StatusFailed, CreatedAt: baseTime.Add(-1 * time.Hour)},
		}
	}).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]*models.Export")).Run(func(args mock.Arguments) {
		items := args.Get(0).(*[]*models.Export)
		*items = []*models.Export{
			{ID: "exp-1", Username: "alice", Status: StatusCompleted, CreatedAt: baseTime.Add(-2 * time.Hour)},
			{ID: "exp-2", Username: "alice", Status: StatusFailed, CreatedAt: baseTime.Add(-1 * time.Hour)},
		}
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.AnythingOfType("*[]*models.ExportCostTracking")).Run(func(args mock.Arguments) {
		costs := args.Get(0).(*[]*models.ExportCostTracking)
		*costs = []*models.ExportCostTracking{
			{
				ExportID:            "exp-1",
				Username:            "alice",
				Type:                "archive",
				Status:              StatusCompleted,
				Timestamp:           baseTime.Add(-30 * time.Minute),
				FileSize:            1000,
				RecordCount:         5,
				MediaFilesIncluded:  1,
				TotalCostMicroCents: 2_000_000,
				LambdaExecutionCost: 1_000_000,
				S3StorageCost:       200_000,
				S3PutRequestCost:    100_000,
				S3GetRequestCost:    100_000,
				S3DataTransferCost:  100_000,
				DynamoDBReadCost:    200_000,
			},
			{
				ExportID:            "exp-2",
				Username:            "alice",
				Type:                "followers",
				Status:              StatusFailed,
				Timestamp:           baseTime.Add(-20 * time.Minute),
				FileSize:            500,
				RecordCount:         2,
				MediaFilesIncluded:  0,
				TotalCostMicroCents: 1_000_000,
				LambdaExecutionCost: 500_000,
			},
		}
	}).Return(nil).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Return(nil).Maybe()

	repo := NewExportRepository(mockDB, "test-table", logger)

	err := repo.CreateExport(ctx, &models.Export{
		ID:       "exp-1",
		Username: "alice",
		Type:     "archive",
		Format:   "mastodon",
		Status:   StatusPending,
	})
	require.NoError(t, err)

	_, err = repo.GetExport(ctx, "exp-1")
	require.NoError(t, err)

	completion := map[string]any{
		"download_url": "https://example.com/download",
		"expires_at":   baseTime.Add(24 * time.Hour),
		"file_size":    123,
		"record_count": 456,
		"s3_key":       "s3/key",
	}
	err = repo.UpdateExportStatus(ctx, "exp-1", StatusCompleted, completion, "done")
	require.NoError(t, err)

	_, _, err = repo.GetExportsForUser(ctx, "alice", 2, "")
	require.NoError(t, err)

	_, err = repo.GetUserExportsByStatus(ctx, "alice", []string{StatusCompleted})
	require.NoError(t, err)

	err = repo.CreateExportCostTracking(ctx, &models.ExportCostTracking{
		ExportID:            "exp-1",
		Username:            "alice",
		Timestamp:           baseTime,
		TotalCostMicroCents: 123,
	})
	require.NoError(t, err)

	_, err = repo.GetExportCostTracking(ctx, "exp-1")
	require.NoError(t, err)

	start := baseTime.Add(-24 * time.Hour)
	end := baseTime
	_, err = repo.GetUserExportCosts(ctx, "alice", start, end, 10)
	require.NoError(t, err)

	_, err = repo.GetExportCostsByDateRange(ctx, start, end, 10)
	require.NoError(t, err)

	summary, err := repo.GetExportCostSummary(ctx, "alice", start, end)
	require.NoError(t, err)
	require.Equal(t, int64(2), summary.TotalExports)
	require.Contains(t, summary.TypeBreakdown, "archive")

	_, err = repo.GetHighCostExports(ctx, 1_000_000, baseTime, baseTime, 1)
	require.NoError(t, err)
}

func TestExportRepository_CreateExport_Error(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupExportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Create").Return(errors.New("create failed")).Once()

	repo := NewExportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.CreateExport(context.Background(), &models.Export{
		ID:       "exp-err",
		Username: "alice",
		Type:     "archive",
		Format:   "mastodon",
		Status:   StatusPending,
	})
	require.Error(t, err)
}

func TestExportRepository_GetExport_NotFound(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupExportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()

	repo := NewExportRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetExport(context.Background(), "missing")
	require.Error(t, err)
}

func TestExportRepository_UpdateExportStatus_InvalidStatus(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	repo := NewExportRepository(new(mocks.MockDB), "test-table", zap.NewNop())
	err := repo.UpdateExportStatus(context.Background(), "exp-1", "not-a-valid-status", nil, "")
	require.Error(t, err)
}

func TestExportRepository_GetExport_OtherError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupExportRepoMocks(mockDB, mockQuery)

	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()

	repo := NewExportRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetExport(context.Background(), "exp-1")
	require.Error(t, err)
}

func TestExportRepository_UpdateExportStatus_NoCompletionData_NoErrorMsg(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 11, 0, 0, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupExportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Update", mock.Anything).Return(nil).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Export")).Run(func(args mock.Arguments) {
		exportRecord := args.Get(0).(*models.Export)
		exportRecord.ID = "exp-1"
		exportRecord.Username = "alice"
		exportRecord.Type = "archive"
		exportRecord.Format = "mastodon"
		exportRecord.Status = StatusPending
		exportRecord.CreatedAt = baseTime
		exportRecord.UpdateKeys()
	}).Return(nil).Once()

	repo := NewExportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.UpdateExportStatus(ctx, "exp-1", StatusFailed, nil, "")
	require.NoError(t, err)
}

func TestExportRepository_GetExportCostTracking_Error(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupExportRepoMocks(mockDB, mockQuery)

	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	repo := NewExportRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetExportCostTracking(context.Background(), "exp-1")
	require.Error(t, err)
}

func TestExportRepository_GetExportCostSummary_EmptyCosts(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupExportRepoMocks(mockDB, mockQuery)

	mockQuery.On("All", mock.AnythingOfType("*[]*models.ExportCostTracking")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ExportCostTracking)
		*dest = nil
	}).Return(nil).Once()

	repo := NewExportRepository(mockDB, "test-table", zap.NewNop())
	summary, err := repo.GetExportCostSummary(ctx, "alice", time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(0), summary.TotalExports)
}

func TestExportRepository_CreateExportCostTracking_CreateError(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "test-table")
	t.Setenv("JWT_SECRET", "dummy")

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupExportRepoMocks(mockDB, mockQuery)

	mockQuery.On("Create").Return(errors.New("create failed")).Once()

	repo := NewExportRepository(mockDB, "test-table", zap.NewNop())
	err := repo.CreateExportCostTracking(context.Background(), &models.ExportCostTracking{
		ExportID:            "exp-1",
		Username:            "alice",
		Timestamp:           time.Now(),
		TotalCostMicroCents: 1,
	})
	require.Error(t, err)
}
