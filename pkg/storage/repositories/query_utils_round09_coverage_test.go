package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestQueryUtils_paginateResults_and_mapExtractKeys(t *testing.T) {
	type item struct{ pk, sk string }
	extract := func(i item) (string, string) { return i.pk, i.sk }

	out, next, more := paginateResults([]item{{pk: "p", sk: "s"}}, nil, extract)
	assert.Len(t, out, 1)
	assert.Empty(t, next)
	assert.False(t, more)

	out, next, more = paginateResults([]item{{pk: "p1", sk: "s1"}, {pk: "p2", sk: "s2"}}, &QueryOptions{Limit: 1}, extract)
	assert.Len(t, out, 1)
	assert.NotEmpty(t, next)
	assert.True(t, more)

	pk, sk := mapExtractKeys(map[string]interface{}{"PK": "a", "SK": "b"})
	assert.Equal(t, "a", pk)
	assert.Equal(t, "b", sk)
}

func TestQueryUtils_db_queries_and_helpers(t *testing.T) {
	ctx := context.Background()

	t.Run("user_relationship_query_and_time_range_query", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		q := NewQueryUtils(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Where", "PK", "=", Utils.Keys.UserKey("u1")).Return(mockQuery)
		mockQuery.On("Where", "SK", "BEGINS_WITH", "FOLLOWING#").Return(mockQuery)
		mockQuery.On("Limit", 3).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]map[string]interface{})
			*dest = []map[string]interface{}{
				{"PK": "p1", "SK": "s1"},
				{"PK": "p2", "SK": "s2"},
				{"PK": "p3", "SK": "s3"},
			}
		}).Return(nil).Once()

		res, err := q.UserRelationshipQuery(ctx, "u1", "FOLLOWING", &QueryOptions{Limit: 2})
		require.NoError(t, err)
		assert.True(t, res.HasMore)
		assert.NotEmpty(t, res.NextCursor)
		assert.Len(t, res.Items, 2)

		mockQuery.On("Where", "PK", "=", "PK#time").Return(mockQuery)
		mockQuery.On("Where", "SK", "BETWEEN", mock.AnythingOfType("[]interface {}")).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("Index", "gsi1").Return(mockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]map[string]interface{})
			*dest = []map[string]interface{}{{"PK": "p", "SK": "s"}}
		}).Return(nil).Once()

		res, err = q.TimeRangeQuery(ctx, "PK#time", 1, 2, &QueryOptions{Limit: 1, IndexName: "gsi1"})
		require.NoError(t, err)
		assert.False(t, res.HasMore)

		mockDB.AssertExpectations(t)
	})

	t.Run("gsi_status_query_count_exists_and_filter_active", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		q := NewQueryUtils(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Index", "gsi1").Return(mockQuery)
		mockQuery.On("Where", "gsi1PK", "=", Utils.GSI.StatusIndexKey("x")).Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]map[string]interface{})
			*dest = []map[string]interface{}{
				{"PK": "p1", "SK": "s1"},
				{"PK": "p2", "SK": "s2"},
			}
		}).Return(nil).Once()

		out, err := q.GSIStatusQuery(ctx, "gsi1", "x", &QueryOptions{Limit: 1})
		require.NoError(t, err)
		assert.True(t, out.HasMore)

		mockQuery.On("Where", "PK", "=", "pk1").Return(mockQuery)
		mockQuery.On("Index", "gsi2").Return(mockQuery)
		// CountQuery is now a page-capped walk (wave #1469): Limit(500)/page
		// via AllPaginated; count = walked rows.
		mockQuery.On("Limit", 500).Return(mockQuery)
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]map[string]interface{})
			*dest = make([]map[string]interface{}, 7)
		}).Return(&core.PaginatedResult{}, nil).Once()
		n, err := q.CountQuery(ctx, "pk1", "gsi2")
		require.NoError(t, err)
		assert.Equal(t, 7, n)

		mockQuery.On("Where", "PK", "=", "pk2").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "sk2").Return(mockQuery)
		mockQuery.On("Count").Return(int64(0), nil).Once()
		exists, err := q.ExistsQuery(ctx, "pk2", "sk2")
		require.NoError(t, err)
		assert.False(t, exists)

		active := q.FilterActiveItems([]map[string]interface{}{
			{"PK": "p1", "ExpiresAt": int64(0), "Active": true},
			{"PK": "p2", "ExpiresAt": time.Now().Add(-time.Hour).Unix(), "Active": true},
			{"PK": "p3", "Revoked": true, "Active": true},
			{"PK": "p4", "Active": false},
		}, time.Now().Unix())
		assert.Len(t, active, 1)
	})

	t.Run("batch_delete_get_update_delete_and_not_found_handling", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		q := NewQueryUtils(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		// empty batch is a no-op
		require.NoError(t, q.BatchDeleteQuery(ctx, nil))

		mockQuery.On("Where", "PK", "=", "p1").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "s1").Return(mockQuery)
		mockQuery.On("Delete").Return(nil).Once()
		require.NoError(t, q.BatchDeleteQuery(ctx, []struct{ PK, SK string }{{PK: "p1", SK: "s1"}}))

		var got map[string]interface{}
		mockQuery.On("Where", "PK", "=", "p2").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "s2").Return(mockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*map[string]interface{})
			*dest = map[string]interface{}{"PK": "p2"}
		}).Return(nil)
		require.NoError(t, q.GetItemByPK(ctx, "p2", "s2", &got))

		mockQuery.On("Update", mock.Anything).Return(nil)
		require.NoError(t, q.UpdateItem(ctx, &got))

		mockQuery.On("Where", "PK", "=", "p3").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "s3").Return(mockQuery)
		mockQuery.On("Delete").Return(nil).Once()
		require.NoError(t, q.DeleteItem(ctx, "p3", "s3", &map[string]interface{}{}))

		// not found delete treated as success
		mockQuery.On("Where", "PK", "=", "p4").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "s4").Return(mockQuery)
		mockQuery.On("Delete").Return(dynamormerrors.ErrItemNotFound).Once()
		require.NoError(t, q.DeleteWithNotFoundHandling(ctx, "p4", "s4", &map[string]interface{}{}, "delete", "a", "b"))

		// other errors are wrapped
		mockQuery.On("Where", "PK", "=", "p5").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "s5").Return(mockQuery)
		mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
		assert.Error(t, q.DeleteWithNotFoundHandling(ctx, "p5", "s5", &map[string]interface{}{}, "delete", "a", "b"))
	})

	t.Run("query_by_gsi_prefix_and_create_with_condition", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		q := NewQueryUtils(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Index", "gsi2").Return(mockQuery)
		mockQuery.On("Where", "gsi2PK", "=", "pk").Return(mockQuery)
		mockQuery.On("Where", "gsi2SK", "=", "sk").Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]map[string]interface{})
			*dest = []map[string]interface{}{{"PK": "p", "SK": "s"}}
		}).Return(nil).Once()
		res, err := q.QueryByGSI(ctx, "gsi2", "pk", "sk", &QueryOptions{Limit: 1})
		require.NoError(t, err)
		assert.False(t, res.HasMore)

		mockQuery.On("Where", "PK", "=", "pk2").Return(mockQuery)
		mockQuery.On("Where", "SK", "BEGINS_WITH", "pref#").Return(mockQuery)
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err = q.QueryWithPrefix(ctx, "pk2", "pref#", &QueryOptions{Limit: 1})
		assert.Error(t, err)

		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
		require.NoError(t, q.CreateWithCondition(ctx, &map[string]interface{}{}))
	})

	t.Run("generic_query_list_batchget_and_query_builder", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		q := NewQueryUtils(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		type titem struct {
			PK string
			SK string
		}

		mockQuery.On("Where", "PK", "=", "p").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "s").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		got, err := GenericQuery[titem](ctx, q, "p", "s")
		assert.ErrorIs(t, err, storage.ErrNotFound)
		assert.Nil(t, got)

		mockQuery.On("Where", "PK", "=", "p2").Return(mockQuery)
		mockQuery.On("Where", "SK", "=", "s2").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
		_, err = GenericQuery[titem](ctx, q, "p2", "s2")
		assert.Error(t, err)

		mockQuery.On("Where", "PK", "=", "lp").Return(mockQuery)
		mockQuery.On("Where", "SK", "BEGINS_WITH", "pref").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]titem)
			*dest = []titem{{PK: "a", SK: "b"}, {PK: "c", SK: "d"}}
		}).Return(nil).Once()
		l, err := GenericList[titem](ctx, q, "lp", "pref", &QueryOptions{Limit: 1})
		require.NoError(t, err)
		assert.True(t, l.HasMore)

		_, err = BatchGet[titem](ctx, q, nil)
		assert.Error(t, err)

		mockQuery.On("Where", "PK", "=", "bp").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "bs").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*titem)
			*dest = titem{PK: "bp", SK: "bs"}
		}).Return(nil).Once()
		mockQuery.On("Where", "PK", "=", "bp2").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "bs2").Return(mockQuery).Once()
		mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
		items, err := BatchGet[titem](ctx, q, []struct{ PK, SK string }{{PK: "bp", SK: "bs"}, {PK: "bp2", SK: "bs2"}})
		require.NoError(t, err)
		assert.Len(t, items, 1)

		// QueryBuilder happy-path and error-path.
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "PK#x").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "BEGINS_WITH", "SK#").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]titem)
			*dest = []titem{{PK: "1", SK: "1"}, {PK: "2", SK: "2"}}
		}).Return(nil).Once()

		qb := NewQueryBuilder[titem](ctx, q).WithIndex("gsi1").WithPK("PK#x").WithSKPrefix("SK#").WithLimit(1)
		qres, err := qb.Execute()
		require.NoError(t, err)
		assert.True(t, qres.HasMore)

		mockQuery.On("Where", "PK", "=", "PK#x").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "SK#y").Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(fmt.Errorf("boom")).Once()
		_, err = NewQueryBuilder[titem](ctx, q).WithPK("PK#x").WithSK("SK#y").WithLimit(1).Execute()
		assert.Error(t, err)
	})
}

func TestQueryUtils_collection_and_convert_helpers(t *testing.T) {
	ctx := context.Background()

	t.Run("add_to_collection_helper", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		q := NewQueryUtils(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
		err := q.AddToCollectionHelper(ctx, "col", &storage.CollectionItem{ItemID: "1", ItemType: "t", AddedBy: "u"}, mockDB)
		require.NoError(t, err)

		mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
		err = q.AddToCollectionHelper(ctx, "col", &storage.CollectionItem{ItemID: "2", ItemType: "t", AddedBy: "u"}, mockDB)
		assert.Error(t, err)
	})

	t.Run("query_and_convert_and_pk_sk_prefix", func(t *testing.T) {
		q := NewQueryUtils(new(mocks.MockDB), zap.NewNop())

		items, err := QueryAndConvert[int, string](ctx, q, func() ([]int, error) {
			return []int{1, 2}, nil
		}, func(v int) string { return fmt.Sprintf("v=%d", v) }, "op", "param")
		require.NoError(t, err)
		assert.Equal(t, []string{"v=1", "v=2"}, items)

		_, err = QueryAndConvert[int, string](ctx, q, func() ([]int, error) {
			return nil, fmt.Errorf("fail")
		}, func(v int) string { return fmt.Sprintf("v=%d", v) }, "op", "param")
		assert.Error(t, err)

		type m struct{ PK, SK string }
		convert := func(in m) string { return in.PK + "#" + in.SK }

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		q = NewQueryUtils(mockDB, zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)

		mockQuery.On("Where", "PK", "=", "pk").Return(mockQuery)
		mockQuery.On("Where", "SK", "BEGINS_WITH", "p").Return(mockQuery)
		// Both flags now iterate via a bounded page walk (wave #1469): a clamped
		// Limit(500) page read via AllPaginated, not a bare All or Scan.
		mockQuery.On("Limit", 500).Return(mockQuery)
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]m)
			*dest = []m{{PK: "pk", SK: "p1"}}
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

		out, err := QueryWithPKAndSKPrefix[m, string](ctx, q, func() *m { return &m{} }, "pk", "p", false, convert, "op", "param")
		require.NoError(t, err)
		assert.Equal(t, []string{"pk#p1"}, out)

		mockQuery.On("Where", "PK", "=", "pk2").Return(mockQuery)
		mockQuery.On("Where", "SK", "BEGINS_WITH", "p").Return(mockQuery)
		mockQuery.On("Limit", 500).Return(mockQuery)
		mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]m)
			*dest = []m{{PK: "pk2", SK: "p2"}}
		}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

		out, err = QueryWithPKAndSKPrefix[m, string](ctx, q, func() *m { return &m{} }, "pk2", "p", true, convert, "op", "param")
		require.NoError(t, err)
		assert.Equal(t, []string{"pk2#p2"}, out)
	})
}

func TestCommonQueries_round09_wrappers_and_more_branches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	c := NewCommonQueries(mockDB, zap.NewNop())

	// GetUserFollows delegates to UserRelationshipQuery.
	mockQuery.On("Where", "PK", "=", Utils.Keys.UserKey("u1")).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "FOLLOWING#").Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]map[string]interface{})
		*dest = []map[string]interface{}{
			{"PK": "p1", "SK": "s1"},
			{"PK": "p2", "SK": "s2"},
		}
	}).Return(nil).Once()
	out, err := c.GetUserFollows(ctx, "u1", 1, "")
	require.NoError(t, err)
	assert.True(t, out.HasMore)

	// GetUserFollowers delegates to GSIStatusQuery.
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", Utils.GSI.StatusIndexKey("follow#u1")).Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]map[string]interface{})
		*dest = []map[string]interface{}{
			{"PK": "p1", "SK": "s1"},
			{"PK": "p2", "SK": "s2"},
		}
	}).Return(nil).Once()
	out, err = c.GetUserFollowers(ctx, "u1", 1, "")
	require.NoError(t, err)
	assert.True(t, out.HasMore)

	// GetActiveTokensForUser filters expired/revoked.
	mockQuery.On("Where", "PK", "=", Utils.Keys.UserKey("u1")).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "TOKEN#").Return(mockQuery).Once()
	mockQuery.On("Limit", 101).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]map[string]interface{})
		*dest = []map[string]interface{}{
			{"PK": "p1", "SK": "TOKEN#1", "ExpiresAt": time.Now().Add(-time.Hour).Unix()},
			{"PK": "p2", "SK": "TOKEN#2", "Revoked": true},
			{"PK": "p3", "SK": "TOKEN#3", "Active": true},
		}
	}).Return(nil).Once()

	tokens, err := c.GetActiveTokensForUser(ctx, "u1")
	require.NoError(t, err)
	assert.Len(t, tokens.Items, 1)

	// DeleteWithNotFoundHandling success branch.
	q := NewQueryUtils(mockDB, zap.NewNop())
	mockQuery.On("Where", "PK", "=", "pk").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "sk").Return(mockQuery).Once()
	mockQuery.On("Delete").Return(nil).Once()
	require.NoError(t, q.DeleteWithNotFoundHandling(ctx, "pk", "sk", &map[string]interface{}{}, "delete", "a", "b"))

	// QueryWithPrefix success branch.
	mockQuery.On("Where", "PK", "=", "pfx").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "BEGINS_WITH", "pref#").Return(mockQuery).Once()
	mockQuery.On("Limit", 2).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]map[string]interface{})
		*dest = []map[string]interface{}{
			{"PK": "pfx", "SK": "pref#1"},
			{"PK": "pfx", "SK": "pref#2"},
		}
	}).Return(nil).Once()
	res, err := q.QueryWithPrefix(ctx, "pfx", "pref#", &QueryOptions{Limit: 1})
	require.NoError(t, err)
	assert.True(t, res.HasMore)

	// CreateWithCondition error branch.
	mockQuery.On("Create").Return(fmt.Errorf("boom")).Once()
	assert.Error(t, q.CreateWithCondition(ctx, &map[string]interface{}{}))
}

func TestQueryUtils_round09_more_error_and_success_branches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	q := NewQueryUtils(mockDB, zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	// ExistsQuery error branch
	mockQuery.On("Count").Return(int64(0), fmt.Errorf("boom")).Once()
	_, err := q.ExistsQuery(ctx, "pk", "sk")
	assert.Error(t, err)

	// GetItemByPK error branch
	mockQuery.On("First", mock.Anything).Return(fmt.Errorf("boom")).Once()
	err = q.GetItemByPK(ctx, "pk", "sk", &map[string]interface{}{})
	assert.Error(t, err)

	// UpdateItem error branch
	mockQuery.On("Update", mock.Anything).Return(fmt.Errorf("boom")).Once()
	assert.Error(t, q.UpdateItem(ctx, &map[string]interface{}{}))

	// DeleteItem error branch
	mockQuery.On("Delete").Return(fmt.Errorf("boom")).Once()
	assert.Error(t, q.DeleteItem(ctx, "pk", "sk", &map[string]interface{}{}))

	// AddToCollectionHelper success branch
	mockQuery.On("Create").Return(nil).Once()
	require.NoError(t, q.AddToCollectionHelper(ctx, "col", &storage.CollectionItem{ItemID: "1", ItemType: "t", AddedBy: "u"}, mockDB))

	// QueryWithPKAndSKPrefix error branches (AllPaginated walk; both flags walk)
	type m struct{ PK, SK string }
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, fmt.Errorf("boom")).Once()
	_, err = QueryWithPKAndSKPrefix[m, string](ctx, q, func() *m { return &m{} }, "pk", "p", false, func(in m) string { return in.PK }, "op", "param")
	assert.Error(t, err)

	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, fmt.Errorf("boom")).Once()
	_, err = QueryWithPKAndSKPrefix[m, string](ctx, q, func() *m { return &m{} }, "pk", "p", true, func(in m) string { return in.PK }, "op", "param")
	assert.Error(t, err)
}
