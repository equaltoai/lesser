package repositories

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type baseRepoPtrModel struct {
	PK string
	SK string

	gsi1SK string

	updateErr error
}

func (m *baseRepoPtrModel) UpdateKeys() error { return m.updateErr }
func (m *baseRepoPtrModel) GetPK() string     { return m.PK }
func (m *baseRepoPtrModel) GetSK() string     { return m.SK }

type baseRepoValModel struct {
	PK string
	SK string
}

func (m baseRepoValModel) UpdateKeys() error { return nil }
func (m baseRepoValModel) GetPK() string     { return m.PK }
func (m baseRepoValModel) GetSK() string     { return m.SK }

type noKeyFieldsModel struct{}

func (noKeyFieldsModel) UpdateKeys() error { return nil }
func (noKeyFieldsModel) GetPK() string     { return "" }
func (noKeyFieldsModel) GetSK() string     { return "" }

func TestBaseRepository_Primitives(t *testing.T) {
	t.Run("modelPrototypeOf handles pointer and value types", func(t *testing.T) {
		ptrProto := modelPrototypeOf[*baseRepoPtrModel]()
		_, ok := ptrProto.(*baseRepoPtrModel)
		require.True(t, ok)

		valProto := modelPrototypeOf[baseRepoValModel]()
		_, ok = valProto.(*baseRepoValModel)
		require.True(t, ok)
	})

	t.Run("clampLimit clamps and defaults", func(t *testing.T) {
		safe, clamped, usedDefault := clampLimit(0, 10, 20)
		assert.Equal(t, 10, safe)
		assert.True(t, clamped)
		assert.True(t, usedDefault)

		safe, clamped, usedDefault = clampLimit(100, 10, 20)
		assert.Equal(t, 20, safe)
		assert.True(t, clamped)
		assert.False(t, usedDefault)

		safe, clamped, usedDefault = clampLimit(5, 10, 20)
		assert.Equal(t, 5, safe)
		assert.False(t, clamped)
		assert.False(t, usedDefault)
	})

	t.Run("setStringField and extractStringField handle common cases", func(t *testing.T) {
		type hasStringField struct {
			PK string
		}

		var model hasStringField
		ok := setStringField(reflectValuePtr(&model), "PK", "PK#1")
		assert.True(t, ok)

		value, ok := extractStringField(model, "PK")
		assert.True(t, ok)
		assert.Equal(t, "PK#1", value)

		ok = setStringField(reflect.ValueOf(model), "PK", "ignored")
		assert.False(t, ok)

		_, ok = extractStringField(model, "Missing")
		assert.False(t, ok)

		var nilModel *hasStringField
		ok = setStringField(reflect.ValueOf(nilModel), "PK", "ignored")
		assert.False(t, ok)

		_, ok = extractStringField(nil, "PK")
		assert.False(t, ok)

		type hasNonStringField struct {
			PK int
		}
		_, ok = extractStringField(hasNonStringField{PK: 1}, "PK")
		assert.False(t, ok)
	})
}

func reflectValuePtr(v any) (rv reflect.Value) {
	return reflect.ValueOf(v)
}

func TestBaseRepository_CrudAndBatchOperations(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("IfNotExists").Return(mockQuery)
	mockQuery.On("Create").Return(nil)
	mockQuery.On("Update", mock.Anything).Return(nil)
	mockQuery.On("Delete").Return(nil)
	mockQuery.On("First", mock.Anything).Return(nil)

	repo := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")

	t.Run("Create success and UpdateKeys error", func(t *testing.T) {
		err := repo.Create(ctx, &baseRepoPtrModel{PK: "PK#1", SK: "SK#1"})
		require.NoError(t, err)

		err = repo.Create(ctx, &baseRepoPtrModel{updateErr: assert.AnError})
		require.Error(t, err)
	})

	t.Run("CreateIfNotExists success", func(t *testing.T) {
		err := repo.CreateIfNotExists(ctx, &baseRepoPtrModel{PK: "PK#1", SK: "SK#1"})
		require.NoError(t, err)
	})

	t.Run("CreateIfNotExists error is wrapped", func(t *testing.T) {
		mockDBErr := new(mocks.MockDB)
		mockQueryErr := new(mocks.MockQuery)
		mockDBErr.On("WithContext", mock.Anything).Return(mockDBErr).Once()
		mockDBErr.On("Model", mock.Anything).Return(mockQueryErr).Once()
		mockQueryErr.On("IfNotExists").Return(mockQueryErr).Once()
		mockQueryErr.On("Create").Return(assert.AnError).Once()

		repoErr := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDBErr, "table", zap.NewNop(), newTestCostService(t), "repo")
		err := repoErr.CreateIfNotExists(ctx, &baseRepoPtrModel{PK: "PK#1", SK: "SK#1"})
		require.Error(t, err)
	})

	t.Run("BatchWriteItems and TransactWrite basic behavior", func(t *testing.T) {
		err := repo.BatchWriteItems(ctx, nil)
		require.NoError(t, err)

		err = repo.BatchWriteItems(ctx, []*baseRepoPtrModel{
			{PK: "PK#1", SK: "SK#1"},
			{PK: "PK#2", SK: "SK#2"},
		})
		require.NoError(t, err)

		err = repo.TransactWrite(ctx, make([]*baseRepoPtrModel, 26))
		require.Error(t, err)

		err = repo.TransactWrite(ctx, []*baseRepoPtrModel{
			{PK: "PK#1", SK: "SK#1"},
		})
		require.NoError(t, err)
	})

	t.Run("BatchWriteItems key update error is wrapped", func(t *testing.T) {
		err := repo.BatchWriteItems(ctx, []*baseRepoPtrModel{
			{PK: "PK#1", SK: "SK#1"},
			{PK: "PK#2", SK: "SK#2", updateErr: assert.AnError},
		})
		require.Error(t, err)
	})

	t.Run("TransactWrite key update error is wrapped", func(t *testing.T) {
		err := repo.TransactWrite(ctx, []*baseRepoPtrModel{
			{PK: "PK#1", SK: "SK#1", updateErr: assert.AnError},
		})
		require.Error(t, err)
	})

	t.Run("Get not found is wrapped; Update and Delete succeed", func(t *testing.T) {
		mockQueryNotFound := new(mocks.MockQuery)
		mockDBNotFound := new(mocks.MockDB)
		mockDBNotFound.On("WithContext", mock.Anything).Return(mockDBNotFound)
		mockDBNotFound.On("Model", mock.Anything).Return(mockQueryNotFound)
		mockQueryNotFound.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryNotFound)
		mockQueryNotFound.On("First", mock.Anything).Return(errors.ErrItemNotFound)

		repoNF := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDBNotFound, "table", zap.NewNop(), newTestCostService(t), "repo")
		err := repoNF.Get(ctx, "PK#missing", "SK#missing", &baseRepoPtrModel{})
		require.Error(t, err)

		err = repo.Update(ctx, &baseRepoPtrModel{PK: "PK#1", SK: "SK#1"})
		require.NoError(t, err)

		err = repo.Delete(ctx, "PK#1", "SK#1")
		require.NoError(t, err)
	})

	t.Run("Update and Delete errors are wrapped", func(t *testing.T) {
		mockDBErr := new(mocks.MockDB)
		mockQueryErr := new(mocks.MockQuery)
		mockDBErr.On("WithContext", mock.Anything).Return(mockDBErr)
		mockDBErr.On("Model", mock.Anything).Return(mockQueryErr)
		mockQueryErr.On("Update", mock.Anything).Return(assert.AnError).Once()
		mockQueryErr.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryErr)
		mockQueryErr.On("Delete").Return(assert.AnError).Once()

		repoErr := NewBaseRepository[*baseRepoPtrModel](mockDBErr, "table", zap.NewNop())
		err := repoErr.Update(ctx, &baseRepoPtrModel{PK: "PK#1", SK: "SK#1"})
		require.Error(t, err)

		err = repoErr.Delete(ctx, "PK#1", "SK#1")
		require.Error(t, err)
	})

	t.Run("Delete value-type model path", func(t *testing.T) {
		mockDBValue := new(mocks.MockDB)
		mockQueryValue := new(mocks.MockQuery)
		mockDBValue.On("WithContext", mock.Anything).Return(mockDBValue)
		mockDBValue.On("Model", mock.Anything).Return(mockQueryValue)
		mockQueryValue.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryValue)
		mockQueryValue.On("Delete").Return(nil)

		repoValue := NewBaseRepository[baseRepoValModel](mockDBValue, "table", zap.NewNop())
		require.NoError(t, repoValue.Delete(ctx, "PK#1", "SK#1"))
	})
}

func TestBaseRepository_QueryAndHelpers(t *testing.T) {
	ctx := context.Background()

	t.Run("Query hasMore trims sentinel", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			items := make([]*baseRepoPtrModel, defaultBaseQueryLimit+1)
			for i := range items {
				items[i] = &baseRepoPtrModel{PK: "PK#1", SK: fmt.Sprintf("SK#%03d", i)}
			}
			*dest = items
		})

		repo := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")
		results, err := repo.Query(ctx, "PK#1", 0)
		require.NoError(t, err)
		require.Len(t, results, defaultBaseQueryLimit)
	})

	t.Run("Query error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError)

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		_, err := repo.Query(ctx, "PK#1", 10)
		require.Error(t, err)
	})

	t.Run("QueryWithSKPrefixPaginated uses cursor and builds next cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{
				{PK: "PK#1", SK: "SK#1"},
				{PK: "PK#1", SK: "SK#2"},
			}
		})

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		out, err := repo.QueryWithSKPrefixPaginated(ctx, "PK#1", "SK#", BasePaginationOptions{
			Limit:  1,
			Cursor: "SK#zzz",
			Order:  SortOrderDesc,
		})
		require.NoError(t, err)
		require.True(t, out.HasMore)
		assert.Equal(t, "SK#1", out.NextCursor)
	})

	t.Run("QueryWithSKPrefixPaginated error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError)

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		_, err := repo.QueryWithSKPrefixPaginated(ctx, "PK#1", "SK#", BasePaginationOptions{Limit: 1})
		require.Error(t, err)
	})

	t.Run("QueryGSIPaginated extracts cursor from gsi field", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "gsi1SK", SortOrderDesc).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{
				{PK: "PK#1", SK: "SK#1", gsi1SK: "CURSOR#next"},
				{PK: "PK#1", SK: "SK#2", gsi1SK: "CURSOR#sentinel"},
			}
		})

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		out, err := repo.QueryGSIPaginated(ctx, "GSI1", "PK#1", BasePaginationOptions{
			Limit:  1,
			Cursor: "CURSOR#prev",
			Order:  SortOrderDesc,
		})
		require.NoError(t, err)
		require.True(t, out.HasMore)
		assert.Equal(t, "CURSOR#next", out.NextCursor)
	})

	t.Run("QueryGSIPaginated error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "gsi1SK", SortOrderAsc).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError)

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		_, err := repo.QueryGSIPaginated(ctx, "GSI1", "PK#1", BasePaginationOptions{Limit: 1})
		require.Error(t, err)
	})

	t.Run("BatchGet errors when model lacks PK/SK fields", func(t *testing.T) {
		repo := NewBaseRepository[noKeyFieldsModel](nil, "table", zap.NewNop())
		_, err := repo.BatchGet(ctx, []struct{ PK, SK string }{{PK: "PK#1", SK: "SK#1"}})
		require.Error(t, err)
	})

	t.Run("BatchGet returns batch results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(1).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#1"}}
		})

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		items, err := repo.BatchGet(ctx, []struct{ PK, SK string }{{PK: "PK#1", SK: "SK#1"}})
		require.NoError(t, err)
		require.Len(t, items, 1)
	})
}

type reportModel struct {
	ID string
}

func TestBaseRepository_ConversionHelpers(t *testing.T) {
	t.Run("ConvertAndPaginateReports trims and returns cursor", func(t *testing.T) {
		models := []reportModel{{ID: "1"}, {ID: "2"}}
		reports, cursor, err := ConvertAndPaginateReports(models, 1, ReportConversionConfig{}, func(m reportModel) *storage.Report {
			return &storage.Report{ID: m.ID}
		}, func(m reportModel) string { return "CURSOR#" + m.ID })
		require.NoError(t, err)
		require.Len(t, reports, 1)
		assert.Equal(t, "CURSOR#1", cursor)
	})

	t.Run("ConvertAndPaginateAuditLogs sets cursor when models present", func(t *testing.T) {
		models := []reportModel{{ID: "1"}}
		logs, cursor := ConvertAndPaginateAuditLogs(models, AuditLogConversionConfig{}, func(m reportModel) *storage.AuditLog {
			return &storage.AuditLog{ID: m.ID}
		}, func(m reportModel) string { return "CURSOR#" + m.ID })
		require.Len(t, logs, 1)
		assert.Equal(t, "CURSOR#1", cursor)
	})
}

type collectionModel struct {
	PK string
	SK string

	gsi1SK string
}

func (m collectionModel) UpdateKeys() error { return nil }
func (m collectionModel) GetPK() string     { return m.PK }
func (m collectionModel) GetSK() string     { return m.SK }

func TestBaseRepository_FindFilterRangeAndCount(t *testing.T) {
	ctx := context.Background()

	t.Run("Count and Exists handle success and errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		// Count walks the keyed partition in bounded pages (wave #1469);
		// Exists still uses the keyed Count() call.
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#1"}, {PK: "PK#1", SK: "SK#2"}}
		}).Return(&core.PaginatedResult{HasMore: false}, nil)
		mockQuery.On("Count").Return(int64(2), nil)

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		count, err := repo.Count(ctx, "PK#1")
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		ok, err := repo.Exists(ctx, "PK#1", "SK#1")
		require.NoError(t, err)
		assert.True(t, ok)

		mockQueryErr := new(mocks.MockQuery)
		mockDBErr := new(mocks.MockDB)
		mockDBErr.On("WithContext", mock.Anything).Return(mockDBErr)
		mockDBErr.On("Model", mock.Anything).Return(mockQueryErr)
		mockQueryErr.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryErr)
		mockQueryErr.On("Limit", mock.Anything).Return(mockQueryErr)
		mockQueryErr.On("AllPaginated", mock.Anything).Return(nil, assert.AnError)
		mockQueryErr.On("Count").Return(int64(0), assert.AnError)

		repoErr := NewBaseRepository[*baseRepoPtrModel](mockDBErr, "table", zap.NewNop())
		_, err = repoErr.Count(ctx, "PK#1")
		require.Error(t, err)
		_, err = repoErr.Exists(ctx, "PK#1", "SK#1")
		require.Error(t, err)
	})

	t.Run("FindByPK returns results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#1"}}
		})

		repo := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")
		items, err := repo.FindByPK(ctx, "PK#1")
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("FindWithPagination supports cursor and hasMore", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", 21).Return(mockQuery)
		mockQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			items := make([]*baseRepoPtrModel, 21)
			for i := range items {
				items[i] = &baseRepoPtrModel{PK: "PK#1", SK: fmt.Sprintf("SK#%02d", i)}
			}
			*dest = items
		})

		repo := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")
		out, err := repo.FindWithPagination(ctx, "PK#1", BasePaginationOptions{})
		require.NoError(t, err)
		require.True(t, out.HasMore)
		assert.Equal(t, "SK#19", out.NextCursor)
	})

	t.Run("QueryWithFilter supports string and complex operators", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{
				{PK: "PK#1", SK: "SK#1"},
				{PK: "PK#1", SK: "SK#2"},
			}
		})

		repo := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")
		_, err := repo.QueryWithFilter(ctx, "PK#1", map[string]interface{}{
			"Status": "active",
			"Score":  map[string]interface{}{"op": ">", "value": 10},
			"Skip":   map[string]interface{}{"op": "="},
		}, 1)
		require.NoError(t, err)
	})

	t.Run("QueryBetweenPaginated supports cursor and nextCursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "SK", SortOrderDesc).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#end"}, {PK: "PK#1", SK: "SK#sentinel"}}
		})

		repo := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")
		out, err := repo.QueryBetweenPaginated(ctx, "PK#1", "SK#start", "SK#end", BasePaginationOptions{
			Limit:  1,
			Cursor: "SK#cursor",
			Order:  SortOrderDesc,
		})
		require.NoError(t, err)
		assert.True(t, out.HasMore)
		assert.Equal(t, "SK#end", out.NextCursor)
	})
}

func TestBaseRepository_BatchGetItems(t *testing.T) {
	ctx := context.Background()

	t.Run("empty keys returns empty", func(t *testing.T) {
		repo := NewBaseRepository[baseRepoValModel](nil, "table", zap.NewNop())
		items, err := repo.BatchGetItems(ctx, nil)
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("skips invalid keys and not-found items", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*baseRepoValModel)
			*dest = baseRepoValModel{PK: "PK#1", SK: "SK#1"}
		}).Once()
		mockQuery.On("First", mock.Anything).Return(errors.ErrItemNotFound).Once()

		repo := NewBaseRepository[baseRepoValModel](mockDB, "table", zap.NewNop())
		items, err := repo.BatchGetItems(ctx, []map[string]interface{}{
			{"PK": "PK#1", "SK": "SK#1"},
			{"PK": "PK#missing", "SK": "SK#missing"},
			{"PK": 123, "SK": "SK#ignored"},
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("returns error on non-not-found failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(assert.AnError).Once()

		repo := NewBaseRepository[baseRepoValModel](mockDB, "table", zap.NewNop())
		_, err := repo.BatchGetItems(ctx, []map[string]interface{}{
			{"PK": "PK#1", "SK": "SK#1"},
		})
		require.Error(t, err)
	})
}

func TestBaseRepository_ConsolidationHelpers(t *testing.T) {
	ctx := context.Background()

	t.Run("QueryCollectionWithConversion main table and gsi paths", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]collectionModel)
			*dest = []collectionModel{
				{PK: "PK#1", SK: "SK#1", gsi1SK: "CURSOR#1"},
				{PK: "PK#1", SK: "SK#2", gsi1SK: "CURSOR#2"},
			}
		})

		repo := NewBaseRepository[collectionModel](mockDB, "table", zap.NewNop())
		cfg := CollectionQueryConfig{PKKey: "OBJ", SKKey: "LIKES", LogName: "likes", ErrorPrefix: "get likes"}
		out, cursor, err := QueryCollectionWithConversion(ctx, repo, cfg, "1", 1, "SK#cursor", func(items []collectionModel) ([]string, error) {
			res := make([]string, 0, len(items))
			for _, item := range items {
				res = append(res, item.GetSK())
			}
			return res, nil
		})
		require.NoError(t, err)
		require.Len(t, out, 1)
		require.Equal(t, "SK#1", out[0])
		require.Equal(t, "SK#1", cursor)

		gsiCfg := CollectionQueryConfig{
			IndexName:   "gsi1",
			LogName:     "likes_gsi",
			ErrorPrefix: "get likes gsi",
			GSIConfig: &GSIQueryConfig{
				PKField:   "gsi1PK",
				SKField:   "gsi1SK",
				PKValue:   "LIKE#%s",
				UseCursor: true,
				OrderBy:   SortOrderDesc,
			},
		}
		_, cursor, err = QueryCollectionWithConversion(ctx, repo, gsiCfg, "1", 1, "CURSOR#prev", func(items []collectionModel) ([]string, error) {
			return []string{"ok"}, nil
		})
		require.NoError(t, err)
		require.Equal(t, "CURSOR#1", cursor)
	})

	t.Run("QueryCollectionWithConversion converter error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]collectionModel)
			*dest = []collectionModel{{PK: "PK#1", SK: "SK#1"}}
		})

		repo := NewBaseRepository[collectionModel](mockDB, "table", zap.NewNop())
		_, _, err := QueryCollectionWithConversion(ctx, repo, CollectionQueryConfig{
			PKKey:       "OBJ",
			SKKey:       "LIKES",
			LogName:     "likes",
			ErrorPrefix: "get likes",
		}, "1", 1, "", func([]collectionModel) ([]string, error) {
			return nil, assert.AnError
		})
		require.Error(t, err)
	})

	t.Run("QueryCollectionWithConversion query error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError)

		repo := NewBaseRepository[collectionModel](mockDB, "table", zap.NewNop())
		_, _, err := QueryCollectionWithConversion(ctx, repo, CollectionQueryConfig{
			PKKey:       "OBJ",
			SKKey:       "LIKES",
			LogName:     "likes",
			ErrorPrefix: "get likes",
		}, "1", 1, "", func([]collectionModel) ([]string, error) { return []string{}, nil })
		require.Error(t, err)
	})

	t.Run("DeleteEntityWithLogging handles not-found and errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Delete").Return(errors.ErrItemNotFound).Once()
		mockQuery.On("Delete").Return(assert.AnError).Once()
		mockQuery.On("Delete").Return(nil).Once()

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		err := DeleteEntityWithLogging(ctx, repo, "PK#1", "SK#1", "entity", map[string]string{"id": "1"})
		require.NoError(t, err)

		err = DeleteEntityWithLogging(ctx, repo, "PK#2", "SK#2", "entity", map[string]string{"id": "2"})
		require.Error(t, err)

		err = DeleteEntityWithLogging(ctx, repo, "PK#3", "SK#3", "entity", map[string]string{"id": "3"})
		require.NoError(t, err)
	})

	t.Run("ListAggregatedByPeriod paginates via cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "SK", SortOrderDesc).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]collectionModel)
			*dest = []collectionModel{
				{PK: "PK#1", SK: "window#end"},
				{PK: "PK#1", SK: "window#sentinel"},
			}
		})

		items, cursor, err := ListAggregatedByPeriod[collectionModel](ctx, mockDB, AggregatedQueryConfig{
			PKPrefix:    "cost_agg",
			LogContext:  "cost",
			ErrorPrefix: "failed list",
		}, "day", "entity", time.Unix(0, 0), time.Unix(10, 0), 1, "window#cursor")
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "window#end", cursor)
	})

	t.Run("ListAggregatedByPeriod error path and defaulted limit", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "SK", SortOrderDesc).Return(mockQuery)
		mockQuery.On("Limit", defaultAggregatedQueryLimit+1).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		_, _, err := ListAggregatedByPeriod[collectionModel](ctx, mockDB, AggregatedQueryConfig{
			PKPrefix:    "cost_agg",
			LogContext:  "cost",
			ErrorPrefix: "failed list",
		}, "day", "entity", time.Unix(0, 0), time.Unix(10, 0), 0, "")
		require.Error(t, err)
	})
}

func TestBaseRepository_BatchCreateAndDelete(t *testing.T) {
	ctx := context.Background()

	t.Run("BatchCreate splits into batches and writes items", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(nil)

		repo := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")
		items := make([]*baseRepoPtrModel, 30)
		for i := range items {
			items[i] = &baseRepoPtrModel{PK: fmt.Sprintf("PK#%d", i), SK: fmt.Sprintf("SK#%d", i)}
		}

		require.NoError(t, repo.BatchCreate(ctx, items))
	})

	t.Run("BatchDelete splits into batches and deletes keys", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Delete").Return(nil)

		repo := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")
		keys := make([]struct{ PK, SK string }, 30)
		for i := range keys {
			keys[i] = struct{ PK, SK string }{PK: fmt.Sprintf("PK#%d", i), SK: fmt.Sprintf("SK#%d", i)}
		}

		require.NoError(t, repo.BatchDelete(ctx, keys))
	})
}

func TestBaseRepository_QueryWrappers(t *testing.T) {
	ctx := context.Background()

	t.Run("QueryWithSKPrefix returns items", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#1"}}
		})

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		items, err := repo.QueryWithSKPrefix(ctx, "PK#1", "SK#", 1)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("QueryGSI returns items", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "gsi1SK", SortOrderAsc).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#1"}}
		})

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		items, err := repo.QueryGSI(ctx, "GSI1", "PK#1", 1)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("QueryBetween returns items", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#start"}}
		})

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		items, err := repo.QueryBetween(ctx, "PK#1", "SK#start", "SK#end", 1)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("QueryWithSKPrefix logs when HasMore is true", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#1"}, {PK: "PK#1", SK: "SK#sentinel"}}
		})

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		items, err := repo.QueryWithSKPrefix(ctx, "PK#1", "SK#", 1)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("QueryGSI logs when HasMore is true", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "gsi1SK", SortOrderAsc).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#1", gsi1SK: "CURSOR#1"}, {PK: "PK#1", SK: "SK#2", gsi1SK: "CURSOR#2"}}
		})

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		items, err := repo.QueryGSI(ctx, "GSI1", "PK#1", 1)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("QueryBetween logs when HasMore is true", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*baseRepoPtrModel)
			*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#end"}, {PK: "PK#1", SK: "SK#sentinel"}}
		})

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		items, err := repo.QueryBetween(ctx, "PK#1", "SK#start", "SK#end", 1)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})
}

func TestBaseRepository_TrackingAndAccessors(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())

	repo.SetRepoName("repo")
	repo.SetCostService(newTestCostService(t))
	require.NotNil(t, repo.GetCostService())
	require.Equal(t, mockDB, repo.GetDB())

	require.NoError(t, repo.TrackRead(ctx, "Query", 1))
	require.NoError(t, repo.TrackWrite(ctx, "PutItem", 1))
	require.NoError(t, repo.TrackCustomOperation(ctx, cost.DynamoOperation{
		Type: "Custom",
	}))
}

func TestBaseRepository_TrackingWithoutCostService(t *testing.T) {
	ctx := context.Background()
	repo := NewBaseRepository[*baseRepoPtrModel](nil, "table", zap.NewNop())
	require.NoError(t, repo.TrackRead(ctx, "Query", 1))
	require.NoError(t, repo.TrackWrite(ctx, "PutItem", 1))
	require.NoError(t, repo.TrackCustomOperation(ctx, cost.DynamoOperation{Type: "Custom"}))
}

func TestBaseRepository_BatchGet_ErrorPath(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(assert.AnError).Once()

	repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
	_, err := repo.BatchGet(ctx, []struct{ PK, SK string }{{PK: "PK#1", SK: "SK#1"}})
	require.Error(t, err)
}

func TestBaseRepository_BatchCreateAndDelete_ErrorAndEmpty(t *testing.T) {
	ctx := context.Background()

	t.Run("BatchCreate empty returns nil", func(t *testing.T) {
		repo := NewBaseRepository[*baseRepoPtrModel](nil, "table", zap.NewNop())
		require.NoError(t, repo.BatchCreate(ctx, nil))
	})

	t.Run("BatchCreate update keys error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		err := repo.BatchCreate(ctx, []*baseRepoPtrModel{{updateErr: assert.AnError}})
		require.Error(t, err)
	})

	t.Run("BatchCreate create error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Create").Return(assert.AnError).Once()

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		err := repo.BatchCreate(ctx, []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#1"}})
		require.Error(t, err)
	})

	t.Run("BatchDelete empty returns nil", func(t *testing.T) {
		repo := NewBaseRepository[*baseRepoPtrModel](nil, "table", zap.NewNop())
		require.NoError(t, repo.BatchDelete(ctx, nil))
	})

	t.Run("BatchDelete delete error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Delete").Return(assert.AnError).Once()

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		err := repo.BatchDelete(ctx, []struct{ PK, SK string }{{PK: "PK#1", SK: "SK#1"}})
		require.Error(t, err)
	})
}

func TestBaseRepository_FindWithPagination_CursorBranches(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "PK#1").Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("OrderBy", "SK", SortOrderDesc).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "<", "SK#cursor").Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*baseRepoPtrModel)
		*dest = []*baseRepoPtrModel{}
	}).Once()

	repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
	_, err := repo.FindWithPagination(ctx, "PK#1", BasePaginationOptions{
		Limit:  1,
		Cursor: "SK#cursor",
		Order:  SortOrderDesc,
	})
	require.NoError(t, err)
}

func TestBaseRepository_QueryBetweenPaginated_CursorAsc(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockQuery)
	mockQuery.On("Limit", 2).Return(mockQuery)
	mockQuery.On("Where", "SK", ">", "SK#cursor").Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*baseRepoPtrModel)
		*dest = []*baseRepoPtrModel{{PK: "PK#1", SK: "SK#end"}, {PK: "PK#1", SK: "SK#sentinel"}}
	})

	repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
	out, err := repo.QueryBetweenPaginated(ctx, "PK#1", "SK#start", "SK#end", BasePaginationOptions{
		Limit:  1,
		Cursor: "SK#cursor",
		Order:  SortOrderAsc,
	})
	require.NoError(t, err)
	require.True(t, out.HasMore)
}

func TestBaseRepository_ErrorPathsMore(t *testing.T) {
	ctx := context.Background()

	t.Run("FindByPK error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Index", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("AllPaginated", mock.Anything).Return(nil, assert.AnError)

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		_, err := repo.FindByPK(ctx, "PK#1")
		require.Error(t, err)
	})

	t.Run("FindWithPagination error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", "SK", SortOrderAsc).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError)

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		_, err := repo.FindWithPagination(ctx, "PK#1", BasePaginationOptions{})
		require.Error(t, err)
	})

	t.Run("QueryWithFilter and QueryBetweenPaginated errors are wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(assert.AnError)

		repo := NewBaseRepository[*baseRepoPtrModel](mockDB, "table", zap.NewNop())
		_, err := repo.QueryWithFilter(ctx, "PK#1", map[string]interface{}{"Status": "active"}, 1)
		require.Error(t, err)

		_, err = repo.QueryBetweenPaginated(ctx, "PK#1", "SK#start", "SK#end", BasePaginationOptions{Limit: 1})
		require.Error(t, err)
	})
}

func TestBaseRepository_CostTrackingWarnBranches(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	mockQuery.On("IfNotExists").Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	mockQuery.On("Create").Return(nil)
	mockQuery.On("Update", mock.Anything).Return(nil)
	mockQuery.On("Delete").Return(nil)
	mockQuery.On("First", mock.Anything).Return(nil)
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*baseRepoPtrModel)
		*dest = []*baseRepoPtrModel{}
	})

	repo := NewBaseRepositoryWithCostTracking[*baseRepoPtrModel](mockDB, "table", zap.NewNop(), newFailingCostService(t), "repo")
	item := &baseRepoPtrModel{PK: "PK#1", SK: "SK#1"}

	require.NoError(t, repo.Create(ctx, item))
	require.NoError(t, repo.CreateIfNotExists(ctx, item))
	require.NoError(t, repo.Get(ctx, "PK#1", "SK#1", item))
	require.NoError(t, repo.Update(ctx, item))
	require.NoError(t, repo.Delete(ctx, "PK#1", "SK#1"))
	_, err := repo.Query(ctx, "PK#1", 1)
	require.NoError(t, err)
}
