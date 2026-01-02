package routing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestInstanceRegistry_WrapperMethodsAndCacheInvalidation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("GetInstanceByDomain_and_list_methods_defer_to_repo", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		expected := createTestInstance("id-1", "example.com")
		fakeRepo.GetInstanceByDomainFunc = func(_ context.Context, domain string) (*types.Instance, error) {
			assert.Equal(t, "example.com", domain)
			return expected, nil
		}

		got, err := registry.GetInstanceByDomain(ctx, "example.com")
		require.NoError(t, err)
		assert.Equal(t, expected, got)
		assert.Equal(t, 1, fakeRepo.GetCallCount("GetInstanceByDomain"))

		fakeRepo.ListHealthyInstancesFunc = func(_ context.Context) ([]*types.Instance, error) {
			return []*types.Instance{expected}, nil
		}
		healthy, err := registry.ListHealthyInstances(ctx)
		require.NoError(t, err)
		assert.Len(t, healthy, 1)
		assert.Equal(t, 1, fakeRepo.GetCallCount("ListHealthyInstances"))

		fakeRepo.ListAllInstancesFunc = func(_ context.Context, limit int, startKey map[string]interface{}) ([]*types.Instance, map[string]interface{}, error) {
			assert.Equal(t, 25, limit)
			assert.Equal(t, map[string]interface{}{"k": "v"}, startKey)
			return []*types.Instance{expected}, map[string]interface{}{"next": "key"}, nil
		}
		all, nextKey, err := registry.ListInstances(ctx, 25, map[string]interface{}{"k": "v"})
		require.NoError(t, err)
		assert.Len(t, all, 1)
		assert.Equal(t, map[string]interface{}{"next": "key"}, nextKey)
		assert.Equal(t, 1, fakeRepo.GetCallCount("ListAllInstances"))

		fakeRepo.ListInstancesByStatusFunc = func(_ context.Context, status types.InstanceStatus, limit int) ([]*types.Instance, error) {
			assert.Equal(t, types.InstanceStatusActive, status)
			assert.Equal(t, 10, limit)
			return []*types.Instance{expected}, nil
		}
		byStatus, err := registry.GetInstancesByStatus(ctx, types.InstanceStatusActive, 10)
		require.NoError(t, err)
		assert.Len(t, byStatus, 1)
		assert.Equal(t, 1, fakeRepo.GetCallCount("ListInstancesByStatus"))

		fakeRepo.GetInstancesByTierFunc = func(_ context.Context, tier types.TierLevel, limit int) ([]*types.Instance, error) {
			assert.Equal(t, types.TierStandard, tier)
			assert.Equal(t, 7, limit)
			return []*types.Instance{expected}, nil
		}
		byTier, err := registry.GetInstancesByTier(ctx, types.TierStandard, 7)
		require.NoError(t, err)
		assert.Len(t, byTier, 1)
		assert.Equal(t, 1, fakeRepo.GetCallCount("GetInstancesByTier"))

		fakeRepo.SearchInstancesFunc = func(_ context.Context, domainPattern string, limit int) ([]*types.Instance, error) {
			assert.Equal(t, "*.example.com", domainPattern)
			assert.Equal(t, 5, limit)
			return []*types.Instance{expected}, nil
		}
		found, err := registry.SearchInstances(ctx, "*.example.com", 5)
		require.NoError(t, err)
		assert.Len(t, found, 1)
		assert.Equal(t, 1, fakeRepo.GetCallCount("SearchInstances"))

		fakeRepo.GetHealthHistoryFunc = func(_ context.Context, instanceID string, duration time.Duration) ([]*types.HealthStatus, error) {
			assert.Equal(t, "id-1", instanceID)
			assert.Equal(t, 1*time.Hour, duration)
			return []*types.HealthStatus{{Reachable: true}}, nil
		}
		history, err := registry.GetHealthHistory(ctx, "id-1", 1*time.Hour)
		require.NoError(t, err)
		assert.Len(t, history, 1)
		assert.Equal(t, 1, fakeRepo.GetCallCount("GetHealthHistory"))
	})

	t.Run("UpdateInstance_invalidates_cache_on_success_and_wraps_on_error", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)
		registry.cache.Store("id-1", &cachedInstance{instance: createTestInstance("id-1", "example.com"), cachedAt: time.Now()})

		instance := createTestInstance("id-1", "example.com")
		require.NoError(t, registry.UpdateInstance(ctx, instance))
		_, ok := registry.cache.Load("id-1")
		assert.False(t, ok)

		registry.cache.Store("id-1", &cachedInstance{instance: createTestInstance("id-1", "example.com"), cachedAt: time.Now()})
		fakeRepo.UpdateInstanceFunc = func(_ context.Context, _ *types.Instance) error { return errors.New("update failed") }
		err := registry.UpdateInstance(ctx, instance)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInstanceUpdateFailed)
		_, ok = registry.cache.Load("id-1")
		assert.True(t, ok)
	})

	t.Run("UnregisterInstance_removes_cache_on_success_and_wraps_on_error", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		registry.cache.Store("id-1", &cachedInstance{instance: createTestInstance("id-1", "example.com"), cachedAt: time.Now()})
		require.NoError(t, registry.UnregisterInstance(ctx, "id-1"))
		_, ok := registry.cache.Load("id-1")
		assert.False(t, ok)

		registry.cache.Store("id-2", &cachedInstance{instance: createTestInstance("id-2", "example.com"), cachedAt: time.Now()})
		fakeRepo.DeleteInstanceFunc = func(_ context.Context, _ string) error { return errors.New("delete failed") }
		err := registry.UnregisterInstance(ctx, "id-2")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInstanceUnregistrationFailed)
		_, ok = registry.cache.Load("id-2")
		assert.True(t, ok)
	})

	t.Run("UpdateInstanceHealth_and_Usage_invalidate_cache_and_wrap_errors", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		registry.cache.Store("id-1", &cachedInstance{instance: createTestInstance("id-1", "example.com"), cachedAt: time.Now()})
		require.NoError(t, registry.UpdateInstanceHealth(ctx, "id-1", &types.HealthStatus{Reachable: true}))
		_, ok := registry.cache.Load("id-1")
		assert.False(t, ok)

		registry.cache.Store("id-2", &cachedInstance{instance: createTestInstance("id-2", "example.com"), cachedAt: time.Now()})
		require.NoError(t, registry.UpdateInstanceUsage(ctx, "id-2", 123))
		_, ok = registry.cache.Load("id-2")
		assert.False(t, ok)

		fakeRepo.UpdateInstanceHealthFunc = func(_ context.Context, _ string, _ *types.HealthStatus) error { return errors.New("health failed") }
		err := registry.UpdateInstanceHealth(ctx, "id-3", &types.HealthStatus{Reachable: false})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInstanceHealthUpdateFailed)

		fakeRepo.UpdateInstanceUsageFunc = func(_ context.Context, _ string, _ int64) error { return errors.New("usage failed") }
		err = registry.UpdateInstanceUsage(ctx, "id-4", 1)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInstanceUsageUpdateFailed)
	})

	t.Run("BatchCreate_and_BatchUpdate_methods_update_and_invalidate_cache", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		instances := []*types.Instance{
			createTestInstance("id-1", "a.example.com"),
			createTestInstance("id-2", "b.example.com"),
		}
		require.NoError(t, registry.BatchCreateInstances(ctx, instances))
		_, ok1 := registry.cache.Load("id-1")
		_, ok2 := registry.cache.Load("id-2")
		assert.True(t, ok1)
		assert.True(t, ok2)

		registry.cache.Store("id-1", &cachedInstance{instance: createTestInstance("id-1", "a.example.com"), cachedAt: time.Now()})
		registry.cache.Store("id-2", &cachedInstance{instance: createTestInstance("id-2", "b.example.com"), cachedAt: time.Now()})
		require.NoError(t, registry.BatchUpdateInstancesHealth(ctx, map[string]*types.HealthStatus{
			"id-1": {Reachable: true},
			"id-2": {Reachable: false},
		}))
		_, ok1 = registry.cache.Load("id-1")
		_, ok2 = registry.cache.Load("id-2")
		assert.False(t, ok1)
		assert.False(t, ok2)

		registry.cache.Store("id-1", &cachedInstance{instance: createTestInstance("id-1", "a.example.com"), cachedAt: time.Now()})
		require.NoError(t, registry.BatchUpdateInstancesUsage(ctx, map[string]int64{"id-1": 42}))
		_, ok1 = registry.cache.Load("id-1")
		assert.False(t, ok1)

		fakeRepo.BatchCreateInstancesFunc = func(_ context.Context, _ []*types.Instance) error { return errors.New("batch create failed") }
		err := registry.BatchCreateInstances(ctx, instances)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInstanceBatchCreateFailed)

		fakeRepo.BatchUpdateInstancesHealthFunc = func(_ context.Context, _ map[string]*types.HealthStatus) error {
			return errors.New("batch health failed")
		}
		err = registry.BatchUpdateInstancesHealth(ctx, map[string]*types.HealthStatus{"id-x": {Reachable: true}})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInstanceBatchHealthUpdateFailed)

		fakeRepo.BatchUpdateInstancesUsageFunc = func(_ context.Context, _ map[string]int64) error { return errors.New("batch usage failed") }
		err = registry.BatchUpdateInstancesUsage(ctx, map[string]int64{"id-x": 1})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInstanceBatchUsageUpdateFailed)
	})
}
