package testing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type round24BatchGetBuilder struct{}

func (round24BatchGetBuilder) Keys(_ []any) core.BatchGetBuilder    { return round24BatchGetBuilder{} }
func (round24BatchGetBuilder) ChunkSize(_ int) core.BatchGetBuilder { return round24BatchGetBuilder{} }
func (round24BatchGetBuilder) ConsistentRead() core.BatchGetBuilder { return round24BatchGetBuilder{} }
func (round24BatchGetBuilder) Parallel(_ int) core.BatchGetBuilder  { return round24BatchGetBuilder{} }
func (round24BatchGetBuilder) WithRetry(_ *core.RetryPolicy) core.BatchGetBuilder {
	return round24BatchGetBuilder{}
}
func (round24BatchGetBuilder) Select(_ ...string) core.BatchGetBuilder {
	return round24BatchGetBuilder{}
}
func (round24BatchGetBuilder) OnProgress(_ core.BatchProgressCallback) core.BatchGetBuilder {
	return round24BatchGetBuilder{}
}
func (round24BatchGetBuilder) OnError(_ core.BatchChunkErrorHandler) core.BatchGetBuilder {
	return round24BatchGetBuilder{}
}
func (round24BatchGetBuilder) Execute(_ any) error { return nil }

type round24UpdateBuilder struct{}

func (round24UpdateBuilder) Set(_ string, _ any) core.UpdateBuilder { return round24UpdateBuilder{} }
func (round24UpdateBuilder) SetIfNotExists(_ string, _ any, _ any) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) Add(_ string, _ any) core.UpdateBuilder    { return round24UpdateBuilder{} }
func (round24UpdateBuilder) Increment(_ string) core.UpdateBuilder     { return round24UpdateBuilder{} }
func (round24UpdateBuilder) Decrement(_ string) core.UpdateBuilder     { return round24UpdateBuilder{} }
func (round24UpdateBuilder) Remove(_ string) core.UpdateBuilder        { return round24UpdateBuilder{} }
func (round24UpdateBuilder) Delete(_ string, _ any) core.UpdateBuilder { return round24UpdateBuilder{} }
func (round24UpdateBuilder) AppendToList(_ string, _ any) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) PrependToList(_ string, _ any) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) RemoveFromListAt(_ string, _ int) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) SetListElement(_ string, _ int, _ any) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) Condition(_ string, _ string, _ any) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) OrCondition(_ string, _ string, _ any) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) ConditionExists(_ string) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) ConditionNotExists(_ string) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) ConditionVersion(_ int64) core.UpdateBuilder {
	return round24UpdateBuilder{}
}
func (round24UpdateBuilder) ReturnValues(_ string) core.UpdateBuilder { return round24UpdateBuilder{} }
func (round24UpdateBuilder) Execute() error                           { return nil }
func (round24UpdateBuilder) ExecuteWithResult(_ any) error            { return nil }

func TestFixtures_LoadIntoTestDB_Round24(t *testing.T) {
	fixtures := NewFixtureBuilder().
		WithUser("alice", "").
		WithStatus("user-1", "hello").
		WithFollow("user-1", "user-2").
		WithNotification("user-1", "user-2", "mention").
		WithMedia("user-1", "file.jpg").
		WithSession("user-1", "127.0.0.1").
		WithProviderAccount("user-1", "github", "gh-1").
		Build()

	testDB := NewTestDB(t, "table")
	fixtures.LoadIntoTestDB(t, testDB)

	require.Equal(t, fixtures.Count(), len(testDB.createdData))
	testDB.Cleanup(t)
	require.Empty(t, testDB.createdData)
}

func TestMockRepository_QueryAndCopyItem_Round24(t *testing.T) {
	repo := NewMockRepository()
	repo.On("Query", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	var dest []string
	require.NoError(t, repo.Query(context.Background(), "query", &dest))
	repo.AssertExpectations(t)

	src := models.User{Username: "bob"}
	var copied models.User
	repo.copyItem(src, &copied)
	require.Equal(t, "bob", copied.Username)

	var mismatched models.Status
	repo.copyItem(src, &mismatched)
	require.Empty(t, mismatched.AuthorID)

	repo.copyItem(src, copied)
}

func TestSetupDefaultMockDB_Round24(t *testing.T) {
	db, query := SetupDefaultMockDB()
	require.NotNil(t, db)
	require.NotNil(t, query)
}

func TestMockDBAndQueryMethods_Round24(t *testing.T) {
	ctx := context.Background()

	mockDB := &MockDB{}
	mockQuery := &MockQuery{}

	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Transaction", mock.Anything).Return(nil).Once()
	mockDB.On("AutoMigrate", mock.Anything).Return(nil).Once()
	mockDB.On("Close").Return(nil).Once()
	mockDB.On("Migrate").Return(nil).Once()

	require.Same(t, mockQuery, mockDB.Model(&models.User{}))
	require.Same(t, mockDB, mockDB.WithContext(ctx))
	require.NoError(t, mockDB.Transaction(func(_ *core.Tx) error { return nil }))
	require.NoError(t, mockDB.AutoMigrate(&models.User{}))
	require.NoError(t, mockDB.Close())
	require.NoError(t, mockDB.Migrate())

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()
	mockQuery.On("Count").Return(int64(7), nil).Once()
	mockQuery.On("Create").Return(nil).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Once()
	mockQuery.On("BatchCreate", mock.Anything).Return(nil).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{}, nil).Once()
	mockQuery.On("BatchDelete", mock.Anything).Return(nil).Once()
	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Once()
	mockQuery.On("BatchGetWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	mockQuery.On("BatchUpdateWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	mockQuery.On("BatchWrite", mock.Anything, mock.Anything).Return(nil).Once()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
	mockQuery.On("OrFilter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
	mockQuery.On("FilterGroup", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("OrFilterGroup", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("WithCondition", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
	mockQuery.On("WithConditionExpression", mock.Anything, mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Offset", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("ConsistentRead").Return(mockQuery).Once()
	mockQuery.On("WithRetry", mock.Anything, mock.Anything).Return(mockQuery).Once()
	mockQuery.On("CreateOrUpdate").Return(nil).Once()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("IfExists").Return(mockQuery).Once()
	mockQuery.On("IfNotExists").Return(mockQuery).Once()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Scan", mock.Anything).Return(nil).Once()
	mockQuery.On("ParallelScan", mock.Anything, mock.Anything).Return(mockQuery).Once()
	mockQuery.On("ScanAllSegments", mock.Anything, mock.Anything).Return(nil).Once()
	mockQuery.On("SetCursor", mock.Anything).Return(nil).Once()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Once()

	require.Same(t, mockQuery, mockQuery.Where("f", "=", "v"))
	require.Same(t, mockQuery, mockQuery.Index("gsi1"))
	require.Same(t, mockQuery, mockQuery.Limit(10))
	require.NoError(t, mockQuery.First(&models.User{}))
	require.NoError(t, mockQuery.All(&[]models.User{}))
	gotCount, err := mockQuery.Count()
	require.NoError(t, err)
	require.Equal(t, int64(7), gotCount)
	require.NoError(t, mockQuery.Create())
	require.NoError(t, mockQuery.Update("field"))
	require.NoError(t, mockQuery.Delete())
	require.NoError(t, mockQuery.BatchCreate([]any{models.User{Username: "u"}}))
	_, err = mockQuery.AllPaginated(&[]models.User{})
	require.NoError(t, err)
	require.NoError(t, mockQuery.BatchDelete([]any{"k"}))
	require.NoError(t, mockQuery.BatchGet([]any{"k"}, &[]models.User{}))
	require.NoError(t, mockQuery.BatchGetWithOptions([]any{"k"}, &[]models.User{}, &core.BatchGetOptions{ChunkSize: 1}))
	require.NoError(t, mockQuery.BatchUpdateWithOptions([]any{"item"}, []string{"field"}, errors.New("ignored")))
	require.NoError(t, mockQuery.BatchWrite([]any{"put"}, []any{"del"}))
	require.Same(t, mockQuery, mockQuery.Filter("f", "=", "v"))
	require.Same(t, mockQuery, mockQuery.OrFilter("f", "=", "v"))
	require.Same(t, mockQuery, mockQuery.FilterGroup(func(core.Query) {}))
	require.Same(t, mockQuery, mockQuery.OrFilterGroup(func(core.Query) {}))
	require.Same(t, mockQuery, mockQuery.WithCondition("f", "=", "v"))
	require.Same(t, mockQuery, mockQuery.WithConditionExpression("expr", map[string]any{}))
	require.Same(t, mockQuery, mockQuery.Offset(10))
	require.Same(t, mockQuery, mockQuery.Select("a", "b"))
	require.Same(t, mockQuery, mockQuery.ConsistentRead())
	require.Same(t, mockQuery, mockQuery.WithRetry(2, time.Millisecond))
	require.NoError(t, mockQuery.CreateOrUpdate())
	require.Same(t, mockQuery, mockQuery.Cursor("cursor"))
	require.Same(t, mockQuery, mockQuery.IfExists())
	require.Same(t, mockQuery, mockQuery.IfNotExists())
	require.Same(t, mockQuery, mockQuery.OrderBy("f", "asc"))
	require.NoError(t, mockQuery.Scan(&[]models.User{}))
	require.Same(t, mockQuery, mockQuery.ParallelScan(0, 2))
	require.NoError(t, mockQuery.ScanAllSegments(&[]models.User{}, 2))
	require.NoError(t, mockQuery.SetCursor("cursor"))
	require.Same(t, mockQuery, mockQuery.WithContext(ctx))

	emptyQuery := &MockQuery{}
	require.Nil(t, emptyQuery.BatchGetBuilder())

	nonNilQuery := &MockQuery{}
	nonNilQuery.On("BatchGetBuilder").Return(round24BatchGetBuilder{}).Once()
	require.NotNil(t, nonNilQuery.BatchGetBuilder())

	updateBuilderNilQuery := &MockQuery{}
	updateBuilderNilQuery.On("UpdateBuilder").Return(nil).Once()
	require.Nil(t, updateBuilderNilQuery.UpdateBuilder())

	updateBuilderQuery := &MockQuery{}
	updateBuilderQuery.On("UpdateBuilder").Return(round24UpdateBuilder{}).Once()
	require.NotNil(t, updateBuilderQuery.UpdateBuilder())

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	nonNilQuery.AssertExpectations(t)
	updateBuilderNilQuery.AssertExpectations(t)
	updateBuilderQuery.AssertExpectations(t)
}

func TestMockUpdateBuilder_SetAndExecute_Round24(t *testing.T) {
	t.Run("returns_nil_builder", func(t *testing.T) {
		builder := &MockUpdateBuilder{}
		builder.On("Set", mock.Anything, mock.Anything).Return(nil).Once()
		builder.On("Execute").Return(nil).Once()

		require.Nil(t, builder.Set("field", "value"))
		require.NoError(t, builder.Execute())
		builder.AssertExpectations(t)
	})

	t.Run("returns_update_builder", func(t *testing.T) {
		builder := &MockUpdateBuilder{}
		builder.On("Set", mock.Anything, mock.Anything).Return(round24UpdateBuilder{}).Once()

		require.NotNil(t, builder.Set("field", "value"))
		builder.AssertExpectations(t)
	})
}

func TestRunBenchmark_Round24(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		result := testing.Benchmark(func(b *testing.B) {
			RunBenchmark(b, BenchmarkConfig{
				ItemCount:     1,
				ConcurrentOps: 1,
				OperationType: "noop",
				SetupFunc:     func() any { return "x" },
				BenchmarkFunc: func(any) error { return nil },
			})
		})
		require.Greater(t, result.N, 0)
	})

	t.Run("parallel", func(t *testing.T) {
		result := testing.Benchmark(func(b *testing.B) {
			RunBenchmark(b, BenchmarkConfig{
				ItemCount:     1,
				ConcurrentOps: 2,
				OperationType: "noop",
				SetupFunc:     func() any { return "x" },
				BenchmarkFunc: func(any) error { return nil },
			})
		})
		require.Greater(t, result.N, 0)
	})
}
