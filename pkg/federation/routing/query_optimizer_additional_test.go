package routing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type fakeQueryCacheRepo struct {
	mu sync.Mutex

	// Persistent cache
	getInstance map[string]*fedTypes.Instance
	setInstance map[string]*fedTypes.Instance

	// Batch-get source
	batchInstances map[string]*fedTypes.Instance

	instancesByStatus map[fedTypes.InstanceStatus][]*fedTypes.Instance
	metricsByRoute    map[string][]*fedTypes.DeliveryResult

	getInstanceCalls int
	setInstanceCalls int

	setInstanceErr error

	getStatusErr  error
	batchGetErr   error
	getMetricsErr error

	prewarmErr    error
	invalidateErr error
}

func newFakeQueryCacheRepo() *fakeQueryCacheRepo {
	return &fakeQueryCacheRepo{
		getInstance:       make(map[string]*fedTypes.Instance),
		setInstance:       make(map[string]*fedTypes.Instance),
		batchInstances:    make(map[string]*fedTypes.Instance),
		instancesByStatus: make(map[fedTypes.InstanceStatus][]*fedTypes.Instance),
		metricsByRoute:    make(map[string][]*fedTypes.DeliveryResult),
	}
}

func (r *fakeQueryCacheRepo) GetInstance(_ context.Context, instanceID string) (*fedTypes.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getInstanceCalls++
	return r.getInstance[instanceID], nil
}

func (r *fakeQueryCacheRepo) SetInstance(_ context.Context, instance *fedTypes.Instance, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setInstanceCalls++
	if r.setInstanceErr != nil {
		return r.setInstanceErr
	}
	r.setInstance[instance.ID] = instance
	return nil
}

func (r *fakeQueryCacheRepo) GetInstancesByStatus(_ context.Context, status fedTypes.InstanceStatus) ([]*fedTypes.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getStatusErr != nil {
		return nil, r.getStatusErr
	}
	return r.instancesByStatus[status], nil
}

func (r *fakeQueryCacheRepo) BatchGetInstances(_ context.Context, instanceIDs []string) ([]*fedTypes.Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.batchGetErr != nil {
		return nil, r.batchGetErr
	}
	out := make([]*fedTypes.Instance, 0, len(instanceIDs))
	for _, id := range instanceIDs {
		if inst := r.batchInstances[id]; inst != nil {
			out = append(out, inst)
		}
	}
	return out, nil
}

func (r *fakeQueryCacheRepo) GetMetricsInRange(_ context.Context, routeID string, _ time.Time, _ time.Time, limit int) ([]*fedTypes.DeliveryResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getMetricsErr != nil {
		return nil, r.getMetricsErr
	}
	results := r.metricsByRoute[routeID]
	if limit > 0 && len(results) > limit {
		return results[:limit], nil
	}
	return results, nil
}

func (r *fakeQueryCacheRepo) PrewarmActiveInstances(_ context.Context) error {
	return r.prewarmErr
}

func (r *fakeQueryCacheRepo) InvalidateCachePattern(_ context.Context, _ string) error {
	return r.invalidateErr
}

func newTestQueryOptimizer(t *testing.T, repo *fakeQueryCacheRepo) (*QueryOptimizer, func()) {
	t.Helper()

	qo := newQueryOptimizer(repo, zap.NewNop())
	qo.batchCoordinator.batchSize = 1
	qo.batchCoordinator.maxWaitTime = 20 * time.Millisecond

	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				// Avoid racing batch execution before waitForBatchResult registers responders.
				if batchCoordinatorHasResponder(qo.batchCoordinator) {
					qo.batchCoordinator.processPendingBatches()
				}
			}
		}
	}()

	cleanup := func() {
		close(stop)
		qo.Shutdown()
	}

	return qo, cleanup
}

func batchCoordinatorHasResponder(bc *BatchQueryCoordinator) bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	for _, batch := range bc.instanceQueries {
		batch.mu.RLock()
		has := len(batch.responders) > 0
		batch.mu.RUnlock()
		if has {
			return true
		}
	}
	for _, batch := range bc.statusQueries {
		batch.mu.RLock()
		has := len(batch.responders) > 0
		batch.mu.RUnlock()
		if has {
			return true
		}
	}
	for _, batch := range bc.metricsQueries {
		batch.mu.RLock()
		has := len(batch.responders) > 0
		batch.mu.RUnlock()
		if has {
			return true
		}
	}

	return false
}

func TestQueryOptimizer_OptimizedGetInstance_Branches(t *testing.T) {
	repo := newFakeQueryCacheRepo()
	qo, cleanup := newTestQueryOptimizer(t, repo)
	defer cleanup()

	// In-memory hit.
	cached := &fedTypes.Instance{ID: "cached", Domain: "cached.example"}
	qo.cache.set("instance:cached", cached, 1)
	inst, err := qo.OptimizedGetInstance(context.Background(), "cached")
	assert.NoError(t, err)
	assert.Same(t, cached, inst)

	// Persistent cache hit.
	repo.getInstance["persisted"] = &fedTypes.Instance{ID: "persisted", Domain: "persisted.example"}
	inst, err = qo.OptimizedGetInstance(context.Background(), "persisted")
	assert.NoError(t, err)
	assert.Equal(t, "persisted", inst.ID)

	// Batch query success + persistent set.
	repo.batchInstances["batched"] = &fedTypes.Instance{ID: "batched", Domain: "batched.example"}
	inst, err = qo.OptimizedGetInstance(context.Background(), "batched")
	assert.NoError(t, err)
	assert.Equal(t, "batched", inst.ID)
	assert.GreaterOrEqual(t, repo.setInstanceCalls, 1)

	// SetInstance failures should not fail the request.
	repo.setInstanceErr = errors.New("set failed")
	repo.batchInstances["batched2"] = &fedTypes.Instance{ID: "batched2", Domain: "batched2.example"}
	inst, err = qo.OptimizedGetInstance(context.Background(), "batched2")
	assert.NoError(t, err)
	assert.Equal(t, "batched2", inst.ID)
	repo.setInstanceErr = nil

	// Instance not found in batch results returns ErrInstanceNotFound.
	_, err = qo.OptimizedGetInstance(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrInstanceNotFound)
}

func TestQueryOptimizer_OptimizedGetInstance_InvalidResultTypeAndBatchError(t *testing.T) {
	repo := newFakeQueryCacheRepo()
	qo := newQueryOptimizer(repo, zap.NewNop())
	qo.batchCoordinator.maxWaitTime = 20 * time.Millisecond
	defer qo.Shutdown()

	// Manually inject a batch result with the wrong type.
	go func() {
		for {
			qo.batchCoordinator.mu.RLock()
			batch := qo.batchCoordinator.instanceQueries["instances"]
			qo.batchCoordinator.mu.RUnlock()
			if batch == nil {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			batch.mu.RLock()
			responders := append([]chan batchResult(nil), batch.responders...)
			batch.mu.RUnlock()
			if len(responders) == 0 {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			responders[0] <- batchResult{results: map[string]interface{}{"weird": "not-an-instance"}}
			return
		}
	}()

	_, err := qo.OptimizedGetInstance(context.Background(), "weird")
	assert.ErrorIs(t, err, ErrInvalidResultType)

	// Batch coordinator error is wrapped.
	go func() {
		for {
			qo.batchCoordinator.mu.RLock()
			batch := qo.batchCoordinator.instanceQueries["instances"]
			qo.batchCoordinator.mu.RUnlock()
			if batch == nil {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			batch.mu.RLock()
			responders := append([]chan batchResult(nil), batch.responders...)
			batch.mu.RUnlock()
			if len(responders) == 0 {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			responders[0] <- batchResult{err: errors.New("batch failed")}
			return
		}
	}()

	_, err = qo.OptimizedGetInstance(context.Background(), "errorcase")
	assert.ErrorIs(t, err, ErrBatchQueryFailed)
}

func TestQueryOptimizer_StatusAndMetricsQueries(t *testing.T) {
	repo := newFakeQueryCacheRepo()
	repo.instancesByStatus[fedTypes.InstanceStatusActive] = []*fedTypes.Instance{
		{ID: "a", Domain: "a.example", Status: fedTypes.InstanceStatusActive},
	}
	repo.metricsByRoute["route-1"] = []*fedTypes.DeliveryResult{
		{RouteID: "route-1", Timestamp: time.Now().Add(-1 * time.Minute)},
		{RouteID: "route-1", Timestamp: time.Now()},
	}

	qo, cleanup := newTestQueryOptimizer(t, repo)
	defer cleanup()

	instances, err := qo.OptimizedQueryByStatus(context.Background(), fedTypes.InstanceStatusActive)
	assert.NoError(t, err)
	assert.Len(t, instances, 1)

	metrics, err := qo.OptimizedQueryRecentMetrics(context.Background(), "route-1", 1)
	assert.NoError(t, err)
	assert.Len(t, metrics, 1)

	// Metrics fallback path aggregates across ranges.
	repo.getMetricsErr = errors.New("metrics read failed")
	metrics, err = qo.OptimizedQueryRecentMetrics(context.Background(), "route-1", 5)
	assert.NoError(t, err)
	assert.Empty(t, metrics)
	repo.getMetricsErr = nil
}

func TestQueryOptimizer_OptimizedBatchGetInstances_Paths(t *testing.T) {
	repo := newFakeQueryCacheRepo()
	repo.batchInstances["a"] = &fedTypes.Instance{ID: "a"}
	repo.batchInstances["b"] = &fedTypes.Instance{ID: "b"}

	qo, cleanup := newTestQueryOptimizer(t, repo)
	defer cleanup()

	// All cached in memory.
	qo.cache.set("instance:a", repo.batchInstances["a"], 1)
	qo.cache.set("instance:b", repo.batchInstances["b"], 1)
	instances, err := qo.OptimizedBatchGetInstances(context.Background(), []string{"a", "b"})
	assert.NoError(t, err)
	assert.Len(t, instances, 2)

	// Mixed cache/batch.
	qo.cache.invalidatePattern("instance:*")
	instances, err = qo.OptimizedBatchGetInstances(context.Background(), []string{"a", "b"})
	assert.NoError(t, err)
	assert.Len(t, instances, 2)
}

func TestQueryOptimizer_PrewarmInvalidateAndShutdown(t *testing.T) {
	repo := newFakeQueryCacheRepo()
	repo.instancesByStatus[fedTypes.InstanceStatusActive] = []*fedTypes.Instance{{ID: "active"}}

	qo, cleanup := newTestQueryOptimizer(t, repo)
	defer cleanup()

	assert.NoError(t, qo.PrewarmCache(context.Background()))

	repo.prewarmErr = errors.New("prewarm failed")
	assert.ErrorIs(t, qo.PrewarmCache(context.Background()), ErrPrewarmActiveInstancesFailed)

	repo.invalidateErr = errors.New("invalidate failed")
	qo.InvalidateCache("instance:*")
}

func TestBatchQueryCoordinator_executeBatch_UsesRepository(t *testing.T) {
	repo := newFakeQueryCacheRepo()
	repo.batchInstances["a"] = &fedTypes.Instance{ID: "a"}
	repo.batchInstances["b"] = &fedTypes.Instance{ID: "b"}
	repo.instancesByStatus[fedTypes.InstanceStatusActive] = []*fedTypes.Instance{{ID: "active"}}
	repo.metricsByRoute["route-1"] = []*fedTypes.DeliveryResult{{RouteID: "route-1"}}

	bc := newBatchQueryCoordinator(repo, zap.NewNop())
	bc.batchSize = 1
	bc.maxWaitTime = 20 * time.Millisecond

	// Instance batch executes and returns per-key result.
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if batchCoordinatorHasResponder(bc) {
					bc.processPendingBatches()
				}
			}
		}
	}()
	defer close(stop)

	v, err := bc.AddInstanceQuery(ctx, "a", 1)
	assert.NoError(t, err)
	assert.Equal(t, "a", v.(*fedTypes.Instance).ID)

	// Status query executes.
	v, err = bc.AddStatusQuery(ctx, string(fedTypes.InstanceStatusActive), 1)
	assert.NoError(t, err)
	assert.NotNil(t, v)

	// Metrics query executes.
	v, err = bc.AddMetricsQuery(ctx, "route-1", 1)
	assert.NoError(t, err)
	assert.NotNil(t, v)
}
