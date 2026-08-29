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
	"go.uber.org/zap"
)

type failingCreatable struct{}

func (failingCreatable) GetID() string          { return "" }
func (failingCreatable) SetID(string)           {}
func (failingCreatable) SetCreatedAt(time.Time) {}
func (failingCreatable) UpdateKeys() error      { return errors.New("keys") }

func TestFilterRepository_CreateUpdateGetDeleteAndMatching(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewFilterRepository(mockDB, "tbl", zap.NewNop(), nil)

	// createFilterItem UpdateKeys error via fake adapter
	err := repo.createFilterItem(context.Background(), &struct{}{}, failingCreatable{}, EntityFilterKeyword, nil)
	require.Error(t, err)

	// CreateFilter generates ID, updates keys, and creates
	mockQuery.On("Create").Return(nil).Once()
	filter := &models.Filter{Username: "alice", Title: "t", Context: []string{"home"}}
	err = repo.CreateFilter(context.Background(), filter)
	require.NoError(t, err)
	require.NotEmpty(t, filter.ID)

	// GetFilter query returns a single match via the GSI
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.Filter); ok {
			*slice = []*models.Filter{
				{ID: filter.ID, Username: "alice"},
			}
		}
	}).Return(nil).Once()
	got, err := repo.GetFilter(context.Background(), filter.ID)
	require.NoError(t, err)
	require.Equal(t, filter.ID, got.ID)

	// GetFilter not found
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.Filter); ok {
			*slice = []*models.Filter{}
		}
	}).Return(nil).Once()
	_, err = repo.GetFilter(context.Background(), "missing")
	require.Error(t, err)

	// UpdateFilter error path
	mockQuery.On("Update", mock.Anything).Return(errors.New("boom")).Once()
	err = repo.UpdateFilter(context.Background(), filter)
	require.Error(t, err)

	// UpdateFilter success
	mockQuery.On("Update", mock.Anything).Return(nil).Once()
	err = repo.UpdateFilter(context.Background(), filter)
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestFilterRepository_KeywordsStatusesAndEvaluate(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewFilterRepository(mockDB, "tbl", zap.NewNop(), nil)

	// createFilterItem create error
	mockQuery.On("Create").Return(errors.New("boom")).Once()
	err := repo.AddFilterKeyword(context.Background(), &models.FilterKeyword{FilterID: "f1", Keyword: "bad"})
	require.Error(t, err)

	// createFilterItem success
	mockQuery.On("Create").Return(nil).Once()
	err = repo.AddFilterKeyword(context.Background(), &models.FilterKeyword{FilterID: "f1", Keyword: "bad", WholeWord: true})
	require.NoError(t, err)

	// EvaluateFilters: GetUserFilters then GetFilterKeywords
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]models.Filter); ok {
			*d = []models.Filter{{ID: "f1", Username: "alice", CaseSensitive: false}}
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]models.FilterKeyword); ok {
			*d = []models.FilterKeyword{
				{ID: "kw1", FilterID: "f1", Keyword: "bad", WholeWord: true},
				{ID: "kw2", FilterID: "f1", Keyword: "(", IsRegex: true}, // invalid regex branch
			}
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	matches, err := repo.EvaluateFilters(context.Background(), "alice", "this is bad", []string{"home"})
	require.NoError(t, err)
	require.Len(t, matches, 1)

	// CheckContentFiltered: GetUserFilters then GetFilterStatuses
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]models.Filter); ok {
			*d = []models.Filter{{ID: "f1", Username: "alice"}}
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]models.FilterStatus); ok {
			*d = []models.FilterStatus{{ID: "fs1", FilterID: "f1", StatusID: "s1"}}
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	ok, _, err := repo.CheckContentFiltered(context.Background(), "alice", "s1", []string{"home"})
	require.NoError(t, err)
	require.True(t, ok)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestFilterRepository_GetActiveFiltersAndMatchingHelpers(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewFilterRepository(mockDB, "tbl", zap.NewNop(), nil)

	// GetUserFilters success
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if filters, ok := dst.(*[]models.Filter); ok {
			expired := time.Now().Add(-time.Hour)
			*filters = []models.Filter{
				{ID: "f1", Username: "alice", ExpiresAt: &expired, Context: []string{"home"}},
				{ID: "f2", Username: "alice", Context: []string{"notifications"}},
				{ID: "f3", Username: "alice", Context: []string{"home", "public"}},
			}
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	active, err := repo.GetActiveFilters(context.Background(), "alice", []string{"home"})
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, "f3", active[0].ID)

	// matchesKeyword branches
	require.True(t, repo.matchesKeyword("Hello World", &models.FilterKeyword{Keyword: "Hello"}, true))
	require.True(t, repo.matchesKeyword("Hello World", &models.FilterKeyword{Keyword: "hello"}, false))
	require.True(t, repo.matchesKeyword("hello world", &models.FilterKeyword{Keyword: "world", WholeWord: true}, false))
	require.False(t, repo.matchesKeyword("helloworld", &models.FilterKeyword{Keyword: "world", WholeWord: true}, false))

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestFilterRepository_StatusItemAdaptersAndRemoval(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewFilterRepository(mockDB, "tbl", zap.NewNop(), nil)

	// AddFilterStatus exercises filterStatusAdapter methods
	mockQuery.On("Create").Return(nil).Once()
	err := repo.AddFilterStatus(context.Background(), &models.FilterStatus{FilterID: "f1", StatusID: "s1"})
	require.NoError(t, err)

	// matchesKeyword regex valid branch
	require.True(t, repo.matchesKeyword("abc-123", &models.FilterKeyword{Keyword: `[0-9]+`, IsRegex: true}, false))

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestFilterRepository_AdapterAccessorsAndQueryErrors(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewFilterRepository(mockDB, "tbl", zap.NewNop(), nil)

	mockQuery.On("AllPaginated", mock.Anything).Return(nil, errors.New("boom")).Twice()
	_, err := repo.GetFilterKeywords(context.Background(), "f1")
	require.Error(t, err)
	_, err = repo.GetFilterStatuses(context.Background(), "f1")
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestFilterRepository_EvaluateAndCheckContent_ErrorBranches(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewFilterRepository(mockDB, "tbl", zap.NewNop(), nil)

	// GetActiveFilters -> GetUserFilters error
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, errors.New("boom")).Once()
	_, err := repo.EvaluateFilters(context.Background(), "alice", "x", []string{"home"})
	require.Error(t, err)

	// EvaluateFilters continues when keyword query fails
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]models.Filter); ok {
			*d = []models.Filter{{ID: "f1", Username: "alice"}}
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, errors.New("boom")).Once()
	filters, err := repo.EvaluateFilters(context.Background(), "alice", "x", []string{"home"})
	require.NoError(t, err)
	require.Len(t, filters, 0)

	// CheckContentFiltered continues when status query fails
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]models.Filter); ok {
			*d = []models.Filter{{ID: "f1", Username: "alice"}}
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, errors.New("boom")).Once()
	ok, _, err := repo.CheckContentFiltered(context.Background(), "alice", "s1", []string{"home"})
	require.NoError(t, err)
	require.False(t, ok)

	requireNoMockExpectations(t, mockDB, mockQuery)
}
