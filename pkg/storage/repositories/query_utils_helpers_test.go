package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPaginateResults(t *testing.T) {
	// Helper to create test items
	makeItems := func(n int) []map[string]interface{} {
		items := make([]map[string]interface{}, n)
		for i := 0; i < n; i++ {
			items[i] = map[string]interface{}{
				"PK": "PK#" + string(rune('A'+i)),
				"SK": "SK#" + string(rune('A'+i)),
			}
		}
		return items
	}

	t.Run("nil opts returns all results without cursor", func(t *testing.T) {
		items := makeItems(5)

		result, cursor, hasMore := paginateResults(items, nil, mapExtractKeys)

		assert.Len(t, result, 5)
		assert.Empty(t, cursor)
		assert.False(t, hasMore)
	})

	t.Run("zero limit returns all results", func(t *testing.T) {
		items := makeItems(5)
		opts := &QueryOptions{Limit: 0}

		result, cursor, hasMore := paginateResults(items, opts, mapExtractKeys)

		assert.Len(t, result, 5)
		assert.Empty(t, cursor)
		assert.False(t, hasMore)
	})

	t.Run("limit larger than results returns all without hasMore", func(t *testing.T) {
		items := makeItems(3)
		opts := &QueryOptions{Limit: 10}

		result, cursor, hasMore := paginateResults(items, opts, mapExtractKeys)

		assert.Len(t, result, 3)
		assert.Empty(t, cursor)
		assert.False(t, hasMore)
	})

	t.Run("limit smaller than results truncates with hasMore", func(t *testing.T) {
		items := makeItems(5)
		opts := &QueryOptions{Limit: 3}

		result, cursor, hasMore := paginateResults(items, opts, mapExtractKeys)

		assert.Len(t, result, 3)
		assert.True(t, hasMore)
		assert.NotEmpty(t, cursor, "should have cursor for pagination")
	})

	t.Run("limit equal to results has no hasMore", func(t *testing.T) {
		items := makeItems(5)
		opts := &QueryOptions{Limit: 5}

		result, cursor, hasMore := paginateResults(items, opts, mapExtractKeys)

		assert.Len(t, result, 5)
		assert.False(t, hasMore)
		assert.Empty(t, cursor)
	})

	t.Run("nil extractKeys still paginates correctly", func(t *testing.T) {
		items := makeItems(5)
		opts := &QueryOptions{Limit: 3}

		result, cursor, hasMore := paginateResults(items, opts, nil)

		assert.Len(t, result, 3)
		assert.True(t, hasMore)
		assert.Empty(t, cursor, "no cursor when extractKeys is nil")
	})

	t.Run("empty results returns empty", func(t *testing.T) {
		items := []map[string]interface{}{}
		opts := &QueryOptions{Limit: 10}

		result, cursor, hasMore := paginateResults(items, opts, mapExtractKeys)

		assert.Empty(t, result)
		assert.Empty(t, cursor)
		assert.False(t, hasMore)
	})

	t.Run("negative limit treated as no limit", func(t *testing.T) {
		items := makeItems(5)
		opts := &QueryOptions{Limit: -1}

		result, cursor, hasMore := paginateResults(items, opts, mapExtractKeys)

		assert.Len(t, result, 5)
		assert.Empty(t, cursor)
		assert.False(t, hasMore)
	})
}

func TestMapExtractKeys(t *testing.T) {
	t.Run("extracts PK and SK when present", func(t *testing.T) {
		item := map[string]interface{}{
			"PK":   "USER#123",
			"SK":   "PROFILE",
			"name": "John",
		}

		pk, sk := mapExtractKeys(item)

		assert.Equal(t, "USER#123", pk)
		assert.Equal(t, "PROFILE", sk)
	})

	t.Run("returns empty strings when PK missing", func(t *testing.T) {
		item := map[string]interface{}{
			"SK":   "PROFILE",
			"name": "John",
		}

		pk, sk := mapExtractKeys(item)

		assert.Empty(t, pk)
		assert.Equal(t, "PROFILE", sk)
	})

	t.Run("returns empty strings when SK missing", func(t *testing.T) {
		item := map[string]interface{}{
			"PK":   "USER#123",
			"name": "John",
		}

		pk, sk := mapExtractKeys(item)

		assert.Equal(t, "USER#123", pk)
		assert.Empty(t, sk)
	})

	t.Run("returns empty strings for empty map", func(t *testing.T) {
		item := map[string]interface{}{}

		pk, sk := mapExtractKeys(item)

		assert.Empty(t, pk)
		assert.Empty(t, sk)
	})

	t.Run("returns empty when PK is not a string", func(t *testing.T) {
		item := map[string]interface{}{
			"PK": 123,
			"SK": "PROFILE",
		}

		pk, sk := mapExtractKeys(item)

		assert.Empty(t, pk)
		assert.Equal(t, "PROFILE", sk)
	})

	t.Run("returns empty when SK is not a string", func(t *testing.T) {
		item := map[string]interface{}{
			"PK": "USER#123",
			"SK": []string{"invalid"},
		}

		pk, sk := mapExtractKeys(item)

		assert.Equal(t, "USER#123", pk)
		assert.Empty(t, sk)
	})
}

func TestFilterActiveItems(t *testing.T) {
	q := &QueryUtils{} // logger not needed for this pure function

	now := int64(1000000)

	t.Run("active item passes through", func(t *testing.T) {
		items := []map[string]interface{}{
			{"id": "1", "Active": true},
		}

		result := q.FilterActiveItems(items, now)

		assert.Len(t, result, 1)
	})

	t.Run("expired item by ExpiresAt is filtered", func(t *testing.T) {
		items := []map[string]interface{}{
			{"id": "1", "ExpiresAt": int64(500000)},  // Expired
			{"id": "2", "ExpiresAt": int64(2000000)}, // Not expired
		}

		result := q.FilterActiveItems(items, now)

		require.Len(t, result, 1)
		assert.Equal(t, "2", result[0]["id"])
	})

	t.Run("revoked item is filtered", func(t *testing.T) {
		items := []map[string]interface{}{
			{"id": "1", "Revoked": true},
			{"id": "2", "Revoked": false},
		}

		result := q.FilterActiveItems(items, now)

		require.Len(t, result, 1)
		assert.Equal(t, "2", result[0]["id"])
	})

	t.Run("inactive item (Active=false) is filtered", func(t *testing.T) {
		items := []map[string]interface{}{
			{"id": "1", "Active": false},
			{"id": "2", "Active": true},
		}

		result := q.FilterActiveItems(items, now)

		require.Len(t, result, 1)
		assert.Equal(t, "2", result[0]["id"])
	})

	t.Run("item without Active field passes through", func(t *testing.T) {
		items := []map[string]interface{}{
			{"id": "1"}, // No Active field
		}

		result := q.FilterActiveItems(items, now)

		assert.Len(t, result, 1)
	})

	t.Run("ExpiresAt of 0 is not treated as expired", func(t *testing.T) {
		items := []map[string]interface{}{
			{"id": "1", "ExpiresAt": int64(0)}, // Zero means no expiry
		}

		result := q.FilterActiveItems(items, now)

		assert.Len(t, result, 1)
	})

	t.Run("combination: expired AND revoked is filtered", func(t *testing.T) {
		items := []map[string]interface{}{
			{"id": "1", "ExpiresAt": int64(500), "Revoked": true},
		}

		result := q.FilterActiveItems(items, now)

		assert.Empty(t, result)
	})

	t.Run("combination: not expired but inactive is filtered", func(t *testing.T) {
		items := []map[string]interface{}{
			{"id": "1", "ExpiresAt": int64(2000000), "Active": false},
		}

		result := q.FilterActiveItems(items, now)

		assert.Empty(t, result)
	})

	t.Run("empty input returns empty output", func(t *testing.T) {
		result := q.FilterActiveItems([]map[string]interface{}{}, now)
		assert.Empty(t, result)
	})

	t.Run("nil slice returns empty", func(t *testing.T) {
		result := q.FilterActiveItems(nil, now)
		assert.Empty(t, result)
	})

	t.Run("multiple filters applied correctly", func(t *testing.T) {
		items := []map[string]interface{}{
			{"id": "expired", "ExpiresAt": int64(100)},
			{"id": "revoked", "Revoked": true},
			{"id": "inactive", "Active": false},
			{"id": "valid1", "Active": true, "ExpiresAt": int64(2000000)},
			{"id": "valid2"}, // No restrictions
			{"id": "valid3", "Revoked": false, "Active": true},
		}

		result := q.FilterActiveItems(items, now)

		assert.Len(t, result, 3)
		ids := []string{}
		for _, item := range result {
			ids = append(ids, item["id"].(string))
		}
		assert.Contains(t, ids, "valid1")
		assert.Contains(t, ids, "valid2")
		assert.Contains(t, ids, "valid3")
	})
}

func TestQueryAndConvert(t *testing.T) {
	q := &QueryUtils{logger: zap.NewNop()}

	type TestModel struct {
		ID   int
		Name string
	}
	type TestStorage struct {
		Identifier int
		Label      string
	}

	converter := func(m TestModel) TestStorage {
		return TestStorage{
			Identifier: m.ID,
			Label:      m.Name,
		}
	}

	t.Run("success path converts models to storage types", func(t *testing.T) {
		queryFunc := func() ([]TestModel, error) {
			return []TestModel{
				{ID: 1, Name: "Alice"},
				{ID: 2, Name: "Bob"},
			}, nil
		}

		result, err := QueryAndConvert(context.Background(), q, queryFunc, converter, "test query", "param1")

		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, 1, result[0].Identifier)
		assert.Equal(t, "Alice", result[0].Label)
		assert.Equal(t, 2, result[1].Identifier)
		assert.Equal(t, "Bob", result[1].Label)
	})

	t.Run("empty result returns empty slice", func(t *testing.T) {
		queryFunc := func() ([]TestModel, error) {
			return []TestModel{}, nil
		}

		result, err := QueryAndConvert(context.Background(), q, queryFunc, converter, "test query", "param1")

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("error path returns wrapped error", func(t *testing.T) {
		queryErr := errors.New("database connection failed")
		queryFunc := func() ([]TestModel, error) {
			return nil, queryErr
		}

		result, err := QueryAndConvert(context.Background(), q, queryFunc, converter, "test query", "param1")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "test query")
		assert.True(t, errors.Is(err, ErrQueryExecutionFailed))
	})
}
