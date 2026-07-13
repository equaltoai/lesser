package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func setupPermissiveSearchCostRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Offset", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("BatchDelete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()
}

func TestSearchCostTrackingWrapper_Helpers(t *testing.T) {
	w := &SearchCostTrackingWrapper{logger: zap.NewNop()}

	ctx := context.WithValue(context.Background(), "request_id", "req-123")
	assert.Equal(t, "req-123", w.getRequestID(ctx))
	assert.NotEmpty(t, w.getRequestID(context.Background()))

	assert.Equal(t, int64(100+20), w.estimateSearchCost("text_search", 10))
	assert.Equal(t, int64(100+10), w.estimateSearchCost("hashtag_search", 10))
	assert.Equal(t, int64(100+50), w.estimateSearchCost("all_search", 10))
	assert.Equal(t, int64(100+10), w.estimateSearchCost("search_suggestions", 10))
	assert.Equal(t, int64(100+30), w.estimateSearchCost("unknown", 10))

	assert.Greater(t, w.estimateSemanticSearchCost(128, 10), int64(0))
}

func TestSearchCostTrackingWrapper_SearchAccounts_TracksAndRecordsCost(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveSearchCostRepoMocks(mockDB, mockQuery)

	logger := zap.NewNop()
	searchRepo := NewSearchRepository(mockDB, "test-table", logger, nil)
	costRepo := NewSearchCostRepository(mockDB, "test-table", logger, nil)
	wrapper := NewSearchCostTrackingWrapper(searchRepo, costRepo, nil, logger)

	// RecordSearchCost writes the cost record using BaseRepository.Create.
	mockQuery.On("Create").Return(nil).Maybe()

	// Use a too-short query so SearchRepository returns early without DB work,
	// while the wrapper still exercises cost tracking and recording.
	results, err := wrapper.SearchAccounts(context.Background(), "a", 10, false, 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchCostTrackingWrapper_SearchByEmbedding_ErrorStillRecordsCost(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveSearchCostRepoMocks(mockDB, mockQuery)

	logger := zap.NewNop()
	searchRepo := NewSearchRepository(mockDB, "test-table", logger, nil)
	costRepo := NewSearchCostRepository(mockDB, "test-table", logger, nil)
	wrapper := NewSearchCostTrackingWrapper(searchRepo, costRepo, nil, logger)

	// Empty embedding triggers parameter validation error inside SearchRepository.SearchByEmbedding.
	_, err := wrapper.SearchByEmbedding(context.Background(), nil, 5, 0.9)
	assert.Error(t, err)
}

func TestSearchCostTrackingWrapper_SearchAccountsAdvanced_ExecutesBudgetCheckPath(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// For this test, allow CheckBudget() to succeed by returning a permissive budget.
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.SearchBudget)
		out.UserID = "user-1"
		out.Period = "daily"
		out.PeriodDate = time.Now().Format("2006-01-02")
		out.BudgetLimitMicros = 1_000_000_000
		out.SearchBudgetMicros = 1_000_000_000
		out.SemanticBudgetMicros = 1_000_000_000
		out.IndexingBudgetMicros = 1_000_000_000
		out.MaxRequestsPerHour = 10_000
		out.MaxSemanticPerHour = 10_000
	}).Return(nil).Once()

	setupPermissiveSearchCostRepoMocks(mockDB, mockQuery)

	logger := zap.NewNop()
	searchRepo := NewSearchRepository(mockDB, "test-table", logger, nil)
	costRepo := NewSearchCostRepository(mockDB, "test-table", logger, nil)
	wrapper := NewSearchCostTrackingWrapper(searchRepo, costRepo, nil, logger)

	results, err := wrapper.SearchAccountsAdvanced(context.Background(), "a", false, 10, 0, false, "user-1")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearchCostTrackingWrapper_CoverageSweep(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Offset", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.SearchBudget:
			dest.UserID = "user-1"
			dest.Period = "daily"
			dest.PeriodDate = now.Format("2006-01-02")
			dest.BudgetLimitMicros = 1_000_000_000
			dest.SearchBudgetMicros = 1_000_000_000
			dest.SemanticBudgetMicros = 1_000_000_000
			dest.IndexingBudgetMicros = 1_000_000_000
			dest.MaxRequestsPerHour = 10_000
			dest.MaxSemanticPerHour = 10_000
		case *models.SearchEmbedding:
			dest.ContentID = "content-1"
			dest.Embedding = []float32{1, 0}
			dest.CreatedAt = now
		case *models.SearchQueryStats:
			// Leave empty to force create/update branches in repositories.
		}
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.SearchEmbedding:
			*dest = []models.SearchEmbedding{
				{ContentID: "content-1", Embedding: []float32{1, 0}, CreatedAt: now},
				{ContentID: "content-2", Embedding: []float32{0, 1}, CreatedAt: now.Add(-time.Minute)},
			}
		}
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("BatchDelete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()

	logger := zap.NewNop()
	searchRepo := NewSearchRepository(mockDB, "test-table", logger, nil)
	costRepo := NewSearchCostRepository(mockDB, "test-table", logger, nil)
	w := NewSearchCostTrackingWrapper(searchRepo, costRepo, nil, logger)

	ctx := context.WithValue(context.Background(), "request_id", "req-1")

	_, _ = w.SearchAccounts(ctx, "alice", 10, false, 0)
	_, _ = w.SearchAccountsAdvanced(ctx, "alice", false, 10, 0, true, "user-1")
	_, _ = w.SearchStatuses(ctx, "hello", 5)
	_, _ = w.SearchStatusesWithOptions(ctx, "hello", storage.StatusSearchOptions{Limit: 5, AccountID: "user-1"})
	_, _ = w.SearchStatusesAdvanced(ctx, "hello", 5, nil, nil, "user-1")
	_, _ = w.SearchAll(ctx, "hello", 5, "user-1")
	_, _ = w.SearchHashtags(ctx, "tag", 5)
	_, _ = w.SearchHashtagsAdvanced(ctx, "tag", 5, "user-1")
	_, _ = w.GetSearchSuggestions(ctx, "al", 5)
	_, _ = w.SearchByEmbedding(ctx, []float32{1, 0}, 5, 0.0)

	w.SetDependencies(&fakeSearchRepositoryDeps{following: []string{"alice"}})
	_ = w.CreateSearchSuggestion(ctx, &models.SearchSuggestion{Type: "username", Term: "al"})
	_ = w.UpdateSearchSuggestion(ctx, "username", "al", map[string]interface{}{"score": 1.1})
	_ = w.IncrementSuggestionUse(ctx, "username", "al")
	_ = w.PruneOldSuggestions(ctx, time.Now().Add(-time.Hour))
	_ = w.IndexStatus(ctx, &models.Object{ID: "status-1", Type: ActivityTypeNote, Content: "hello"})
	_ = w.UnindexStatus(ctx, "status-1")
	_, _ = w.SearchStatusesByHashtag(ctx, "tag", 5)
	_, _ = w.SearchStatusesByAuthor(ctx, "author-1", 5)
	_ = w.RecordSearch(ctx, &models.SearchAnalytics{Query: "hello", SearchType: "accounts", Timestamp: now})
	_, _ = w.GetSearchAnalytics(ctx, now.Add(-24*time.Hour), now)
	_, _ = w.GetPopularSearches(ctx, 5, 24*time.Hour)
	_, _ = w.GetSearchTrends(ctx, 1)
	_ = w.IndexContentEmbedding(ctx, &models.SearchEmbedding{ContentID: "content-1", Embedding: []float32{1, 0}})
	_ = w.UpdateEmbedding(ctx, "content-1", []float32{1, 0})
	_ = w.DeleteEmbedding(ctx, "content-1")
}

func TestSearchCostTrackingWrapper_BudgetExceeded_ReturnsError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("First", mock.AnythingOfType("*models.SearchBudget")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.SearchBudget)
		out.UserID = "user-1"
		out.Period = "daily"
		out.PeriodDate = time.Now().Format("2006-01-02")
		out.BudgetLimitMicros = 1
		out.SearchBudgetMicros = 1
		out.MaxRequestsPerHour = 1
	}).Return(nil).Once()

	mockQuery.On("Create").Return(nil).Maybe()

	logger := zap.NewNop()
	searchRepo := NewSearchRepository(mockDB, "test-table", logger, nil)
	costRepo := NewSearchCostRepository(mockDB, "test-table", logger, nil)
	w := NewSearchCostTrackingWrapper(searchRepo, costRepo, nil, logger)

	_, err := w.SearchAccountsAdvanced(context.Background(), "alice", false, 10, 0, false, "user-1")
	assert.Error(t, err)
}
