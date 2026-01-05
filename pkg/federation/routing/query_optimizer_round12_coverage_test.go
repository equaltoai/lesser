package routing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestQueryCache_SetGet_Evict_Expire_AndInvalidate(t *testing.T) {
	t.Run("set_get_update_and_lru_eviction", func(t *testing.T) {
		c := &queryCache{
			entries: make(map[string]*cacheEntry),
			lru:     &lruList{},
			maxSize: 2,
			ttl:     1 * time.Minute,
		}

		c.set("a", 1, 1)
		c.set("b", 2, 1)

		assert.Equal(t, 1, c.get("a"))
		c.set("c", 3, 1) // should evict "b" (least recently used)

		assert.Nil(t, c.get("b"))
		assert.Equal(t, 1, c.get("a"))
		assert.Equal(t, 3, c.get("c"))

		c.set("a", 4, 1)
		assert.Equal(t, 4, c.get("a"))
	})

	t.Run("expiry_returns_nil", func(t *testing.T) {
		c := &queryCache{
			entries: make(map[string]*cacheEntry),
			lru:     &lruList{},
			maxSize: 10,
			ttl:     1 * time.Millisecond,
		}

		c.set("x", "y", 1)
		time.Sleep(3 * time.Millisecond)
		assert.Nil(t, c.get("x"))
	})

	t.Run("invalidate_pattern_removes_matching_entries", func(t *testing.T) {
		c := &queryCache{
			entries: make(map[string]*cacheEntry),
			lru:     &lruList{},
			maxSize: 10,
			ttl:     1 * time.Minute,
		}

		c.set("instance:1", "a", 1)
		c.set("instance:2", "b", 1)
		c.set("status:active", "c", 1)

		c.invalidatePattern("instance:*")
		assert.Nil(t, c.get("instance:1"))
		assert.Nil(t, c.get("instance:2"))
		assert.Equal(t, "c", c.get("status:active"))
	})
}

func TestLRUListAndPatternMatching(t *testing.T) {
	l := &lruList{}
	n1 := &lruNode{key: "a"}
	n2 := &lruNode{key: "b"}

	l.pushFront(n1)
	assert.Equal(t, n1, l.head)
	assert.Equal(t, n1, l.tail)

	l.pushFront(n2)
	assert.Equal(t, n2, l.head)
	assert.Equal(t, n1, l.tail)

	l.moveToFront(n1)
	assert.Equal(t, n1, l.head)
	assert.Equal(t, n2, l.tail)

	l.remove(n1)
	assert.Equal(t, n2, l.head)
	assert.Equal(t, n2, l.tail)

	ok, err := matchPattern("foo", "foo")
	assert.NoError(t, err)
	assert.True(t, ok)

	ok, err = matchPattern("foo:bar", "foo:*")
	assert.NoError(t, err)
	assert.True(t, ok)

	ok, err = matchPattern("bar", "foo:*")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestBatchQueryCoordinator_BatchingHelpers(t *testing.T) {
	logger := zap.NewNop()
	bc := NewBatchQueryCoordinator(nil, logger)
	bc.maxWaitTime = 50 * time.Millisecond
	bc.batchWindow = 10 * time.Millisecond
	bc.batchSize = 10

	assert.Equal(t, "instances", bc.getBatchKey(QueryTypeInstance, "id"))
	assert.Equal(t, "active", bc.getBatchKey(QueryTypeStatus, "active"))
	assert.Equal(t, "metrics", bc.getBatchKey(QueryTypeMetrics, "route"))
	assert.Equal(t, "weird", bc.getBatchKey("weird", "k"))

	assert.ElementsMatch(t, []string{"a", "b", "c"}, bc.deduplicateStrings([]string{"a", "b", "a", "c", "b"}))

	batch := &batchQuery{
		queryType:        QueryTypeInstance,
		keys:             []string{"k1"},
		deduplicatedKeys: map[string]bool{"k1": true},
		responders:       []chan batchResult{},
		createdAt:        time.Now(),
		lastUpdated:      time.Now(),
		priority:         3,
	}
	assert.True(t, batch.hasKey("k1"))
	assert.False(t, batch.hasKey("k2"))

	batch.addKey("k1")
	assert.Len(t, batch.keys, 1) // deduped
	batch.addKey("k2")
	assert.Len(t, batch.keys, 2)

	// waitForBatchResult success
	go func() {
		for {
			batch.mu.RLock()
			responders := append([]chan batchResult(nil), batch.responders...)
			batch.mu.RUnlock()
			if len(responders) > 0 {
				responders[0] <- batchResult{results: map[string]interface{}{"k1": "ok"}}
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()
	val, err := bc.waitForBatchResult(batch, "k1")
	assert.NoError(t, err)
	assert.Equal(t, "ok", val)

	// waitForBatchResult error
	batch2 := &batchQuery{
		queryType:        QueryTypeInstance,
		keys:             []string{"k1"},
		deduplicatedKeys: map[string]bool{"k1": true},
		responders:       []chan batchResult{},
		createdAt:        time.Now(),
		lastUpdated:      time.Now(),
		priority:         3,
	}
	go func() {
		for {
			batch2.mu.RLock()
			responders := append([]chan batchResult(nil), batch2.responders...)
			batch2.mu.RUnlock()
			if len(responders) > 0 {
				responders[0] <- batchResult{err: ErrBatchQueryFailed}
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()
	_, err = bc.waitForBatchResult(batch2, "k1")
	assert.Error(t, err)
}

func TestBatchQueryCoordinator_addQuery_ExistingBatch_DeduplicatesAndAddsKeys(t *testing.T) {
	logger := zap.NewNop()
	bc := NewBatchQueryCoordinator(nil, logger)
	bc.maxWaitTime = 200 * time.Millisecond
	bc.batchWindow = 10 * time.Millisecond

	batchKey := bc.getBatchKey(QueryTypeInstance, "id-1")
	existing := &batchQuery{
		queryType:        QueryTypeInstance,
		keys:             []string{"id-1"},
		deduplicatedKeys: map[string]bool{"id-1": true},
		responders:       []chan batchResult{},
		createdAt:        time.Now(),
		lastUpdated:      time.Now(),
		priority:         3,
	}
	bc.instanceQueries[batchKey] = existing

	// Query for an already-present key should be deduplicated (no new key added).
	go func() {
		for {
			existing.mu.RLock()
			responders := append([]chan batchResult(nil), existing.responders...)
			existing.mu.RUnlock()
			if len(responders) > 0 {
				responders[len(responders)-1] <- batchResult{results: map[string]interface{}{"id-1": "ok"}}
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()
	val, err := bc.addQuery(context.Background(), QueryTypeInstance, "id-1", 3)
	assert.NoError(t, err)
	assert.Equal(t, "ok", val)
	assert.Equal(t, []string{"id-1"}, existing.keys)

	// Query for a new key should be appended to the existing batch.
	go func() {
		for {
			existing.mu.RLock()
			responders := append([]chan batchResult(nil), existing.responders...)
			existing.mu.RUnlock()
			if len(responders) > 1 {
				responders[len(responders)-1] <- batchResult{results: map[string]interface{}{"id-2": "ok2"}}
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()
	val, err = bc.addQuery(context.Background(), QueryTypeInstance, "id-2", 3)
	assert.NoError(t, err)
	assert.Equal(t, "ok2", val)
	assert.Contains(t, existing.keys, "id-2")
}

func TestBatchQueryCoordinator_getReadyBatches_SortsAndClears(t *testing.T) {
	bc := NewBatchQueryCoordinator(nil, zap.NewNop())
	bc.batchWindow = 1 * time.Millisecond

	now := time.Now()
	b1 := &batchQuery{queryType: QueryTypeInstance, keys: []string{"a"}, deduplicatedKeys: map[string]bool{"a": true}, createdAt: now.Add(-10 * time.Second), priority: 5}
	b2 := &batchQuery{queryType: QueryTypeInstance, keys: []string{"b"}, deduplicatedKeys: map[string]bool{"b": true}, createdAt: now.Add(-10 * time.Second), priority: 1}

	bc.instanceQueries["instances"] = b1
	bc.statusQueries["active"] = b2

	ready := bc.getReadyBatches()
	assert.Len(t, ready, 2)
	assert.Equal(t, 1, ready[0].priority)
	assert.Equal(t, 5, ready[1].priority)
	assert.Empty(t, bc.instanceQueries)
	assert.Empty(t, bc.statusQueries)
}
