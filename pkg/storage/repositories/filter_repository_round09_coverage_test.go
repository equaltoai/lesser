package repositories

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

	// removeFilterItem query error
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	err = repo.RemoveFilterKeyword(context.Background(), "kw1")
	require.Error(t, err)

	// removeFilterItem not found
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Slice {
			v.Elem().Set(reflect.MakeSlice(v.Elem().Type(), 0, 0))
		}
	}).Return(nil).Once()
	err = repo.RemoveFilterKeyword(context.Background(), "kw1")
	require.Error(t, err)

	// removeFilterItem delete error
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.FilterKeyword); ok {
			*slice = []*models.FilterKeyword{{ID: "kw1", FilterID: "f1", PK: "FILTER#f1", SK: "KEYWORD#kw1"}}
		}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(errors.New("boom")).Once()
	err = repo.RemoveFilterKeyword(context.Background(), "kw1")
	require.Error(t, err)

	// removeFilterItem success
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.FilterKeyword); ok {
			*slice = []*models.FilterKeyword{{ID: "kw1", FilterID: "f1", PK: "FILTER#f1", SK: "KEYWORD#kw1"}}
		}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Once()
	err = repo.RemoveFilterKeyword(context.Background(), "kw1")
	require.NoError(t, err)

	// EvaluateFilters: GetUserFilters then GetFilterKeywords
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]*models.Filter); ok {
			*d = []*models.Filter{{ID: "f1", Username: "alice", CaseSensitive: false}}
		}
	}).Return(nil).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]*models.FilterKeyword); ok {
			*d = []*models.FilterKeyword{
				{ID: "kw1", FilterID: "f1", Keyword: "bad", WholeWord: true},
				{ID: "kw2", FilterID: "f1", Keyword: "(", IsRegex: true}, // invalid regex branch
			}
		}
	}).Return(nil).Once()

	matches, err := repo.EvaluateFilters(context.Background(), "alice", "this is bad", []string{"home"})
	require.NoError(t, err)
	require.Len(t, matches, 1)

	// CheckContentFiltered: GetUserFilters then GetFilterStatuses
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]*models.Filter); ok {
			*d = []*models.Filter{{ID: "f1", Username: "alice"}}
		}
	}).Return(nil).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]*models.FilterStatus); ok {
			*d = []*models.FilterStatus{{ID: "fs1", FilterID: "f1", StatusID: "s1"}}
		}
	}).Return(nil).Once()

	ok, _, err := repo.CheckContentFiltered(context.Background(), "alice", "s1", []string{"home"})
	require.NoError(t, err)
	require.True(t, ok)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestFilterRepository_DeleteFilter_ContinuesOnChildDeleteErrors(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewFilterRepository(mockDB, "tbl", zap.NewNop(), nil)

	// GetFilter returns filter
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.Filter); ok {
			*slice = []*models.Filter{{ID: "f1", Username: "alice", PK: "USER#alice", SK: "FILTER#f1"}}
		}
	}).Return(nil).Once()

	// Keywords list returns one keyword, item lookup fails but continues
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.FilterKeyword); ok {
			*slice = []*models.FilterKeyword{{ID: "kw1", FilterID: "f1", PK: "FILTER#f1", SK: "KEYWORD#kw1"}}
		}
	}).Return(nil).Once()
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()

	// Status list returns one status, item lookup fails but continues
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.FilterStatus); ok {
			*slice = []*models.FilterStatus{{ID: "fs1", FilterID: "f1", StatusID: "s1", PK: "FILTER#f1", SK: "STATUS#s1"}}
		}
	}).Return(nil).Once()
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()

	// Delete filter itself
	mockQuery.On("Delete").Return(nil).Once()

	err := repo.DeleteFilter(context.Background(), "f1")
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestFilterRepository_GetActiveFiltersAndMatchingHelpers(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewFilterRepository(mockDB, "tbl", zap.NewNop(), nil)

	// GetUserFilters success
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if filters, ok := dst.(*[]*models.Filter); ok {
			expired := time.Now().Add(-time.Hour)
			*filters = []*models.Filter{
				{ID: "f1", Username: "alice", ExpiresAt: &expired, Context: []string{"home"}},
				{ID: "f2", Username: "alice", Context: []string{"notifications"}},
				{ID: "f3", Username: "alice", Context: []string{"home", "public"}},
			}
		}
	}).Return(nil).Once()

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

	// RemoveFilterStatus not found
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if slice, ok := args.Get(0).(*[]*models.FilterStatus); ok {
			*slice = []*models.FilterStatus{{ID: "fs1", FilterID: "f1", StatusID: "other"}}
		}
	}).Return(nil).Once()
	err = repo.RemoveFilterStatus(context.Background(), "s1")
	require.Error(t, err)

	// RemoveFilterStatus success (exercises filterStatusDeletable methods)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if slice, ok := args.Get(0).(*[]*models.FilterStatus); ok {
			*slice = []*models.FilterStatus{{ID: "fs1", FilterID: "f1", StatusID: "s1", PK: "FILTER#f1", SK: "STATUS#s1"}}
		}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Once()
	err = repo.RemoveFilterStatus(context.Background(), "s1")
	require.NoError(t, err)

	// matchesKeyword regex valid branch
	require.True(t, repo.matchesKeyword("abc-123", &models.FilterKeyword{Keyword: `[0-9]+`, IsRegex: true}, false))

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestFilterRepository_AdapterAccessorsAndQueryErrors(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewFilterRepository(mockDB, "tbl", zap.NewNop(), nil)

	kw := &models.FilterKeyword{ID: "kw1", FilterID: "f1", PK: "FILTER#f1", SK: "KEYWORD#kw1"}
	kwDel := &filterKeywordDeletable{FilterKeyword: kw}
	require.Equal(t, "kw1", kwDel.GetItemID())

	st := &models.FilterStatus{ID: "fs1", FilterID: "f1", StatusID: "s1", PK: "FILTER#f1", SK: "STATUS#s1"}
	stDel := &filterStatusDeletable{FilterStatus: st}
	require.Equal(t, "fs1", stDel.GetItemID())

	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Twice()
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
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	_, err := repo.EvaluateFilters(context.Background(), "alice", "x", []string{"home"})
	require.Error(t, err)

	// EvaluateFilters continues when keyword query fails
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]*models.Filter); ok {
			*d = []*models.Filter{{ID: "f1", Username: "alice"}}
		}
	}).Return(nil).Once()
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	filters, err := repo.EvaluateFilters(context.Background(), "alice", "x", []string{"home"})
	require.NoError(t, err)
	require.Len(t, filters, 0)

	// CheckContentFiltered continues when status query fails
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if d, ok := dst.(*[]*models.Filter); ok {
			*d = []*models.Filter{{ID: "f1", Username: "alice"}}
		}
	}).Return(nil).Once()
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	ok, _, err := repo.CheckContentFiltered(context.Background(), "alice", "s1", []string{"home"})
	require.NoError(t, err)
	require.False(t, ok)

	requireNoMockExpectations(t, mockDB, mockQuery)
}
