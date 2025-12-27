package routing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// FakeFederationInstanceRepository implements FederationInstanceRepository for testing
type FakeFederationInstanceRepository struct {
	mu sync.Mutex

	// Storage
	instances map[string]*types.Instance

	// Function hooks
	CreateInstanceFunc             func(ctx context.Context, instance *types.Instance) error
	GetInstanceFunc                func(ctx context.Context, instanceID string) (*types.Instance, error)
	GetInstanceByDomainFunc        func(ctx context.Context, domain string) (*types.Instance, error)
	UpdateInstanceFunc             func(ctx context.Context, instance *types.Instance) error
	DeleteInstanceFunc             func(ctx context.Context, instanceID string) error
	ListInstancesByStatusFunc      func(ctx context.Context, status types.InstanceStatus, limit int) ([]*types.Instance, error)
	ListHealthyInstancesFunc       func(ctx context.Context) ([]*types.Instance, error)
	GetInstancesByTierFunc         func(ctx context.Context, tier types.TierLevel, limit int) ([]*types.Instance, error)
	BatchGetInstancesFunc          func(ctx context.Context, instanceIDs []string) ([]*types.Instance, error)
	SearchInstancesFunc            func(ctx context.Context, domainPattern string, limit int) ([]*types.Instance, error)
	ListAllInstancesFunc           func(ctx context.Context, limit int, startKey map[string]interface{}) ([]*types.Instance, map[string]interface{}, error)
	BatchCreateInstancesFunc       func(ctx context.Context, instances []*types.Instance) error
	BatchUpdateInstancesHealthFunc func(ctx context.Context, healthUpdates map[string]*types.HealthStatus) error
	BatchUpdateInstancesUsageFunc  func(ctx context.Context, usageUpdates map[string]int64) error
	UpdateInstanceHealthFunc       func(ctx context.Context, instanceID string, health *types.HealthStatus) error
	UpdateInstanceUsageFunc        func(ctx context.Context, instanceID string, bytesUsed int64) error
	GetHealthHistoryFunc           func(ctx context.Context, instanceID string, duration time.Duration) ([]*types.HealthStatus, error)

	// Call counters
	CallCounts map[string]int
}

func NewFakeFederationInstanceRepository() *FakeFederationInstanceRepository {
	return &FakeFederationInstanceRepository{
		instances:  make(map[string]*types.Instance),
		CallCounts: make(map[string]int),
	}
}

func (f *FakeFederationInstanceRepository) incrementCall(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CallCounts[method]++
}

func (f *FakeFederationInstanceRepository) GetCallCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.CallCounts[method]
}

func (f *FakeFederationInstanceRepository) AddInstance(instance *types.Instance) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.instances[instance.ID] = instance
}

func (f *FakeFederationInstanceRepository) CreateInstance(ctx context.Context, instance *types.Instance) error {
	f.incrementCall("CreateInstance")
	if f.CreateInstanceFunc != nil {
		return f.CreateInstanceFunc(ctx, instance)
	}
	f.mu.Lock()
	f.instances[instance.ID] = instance
	f.mu.Unlock()
	return nil
}

func (f *FakeFederationInstanceRepository) GetInstance(ctx context.Context, instanceID string) (*types.Instance, error) {
	f.incrementCall("GetInstance")
	if f.GetInstanceFunc != nil {
		return f.GetInstanceFunc(ctx, instanceID)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if instance, ok := f.instances[instanceID]; ok {
		return instance, nil
	}
	return nil, errors.New("instance not found")
}

func (f *FakeFederationInstanceRepository) GetInstanceByDomain(ctx context.Context, domain string) (*types.Instance, error) {
	f.incrementCall("GetInstanceByDomain")
	if f.GetInstanceByDomainFunc != nil {
		return f.GetInstanceByDomainFunc(ctx, domain)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, instance := range f.instances {
		if instance.Domain == domain {
			return instance, nil
		}
	}
	return nil, errors.New("instance not found")
}

func (f *FakeFederationInstanceRepository) UpdateInstance(ctx context.Context, instance *types.Instance) error {
	f.incrementCall("UpdateInstance")
	if f.UpdateInstanceFunc != nil {
		return f.UpdateInstanceFunc(ctx, instance)
	}
	return nil
}

func (f *FakeFederationInstanceRepository) DeleteInstance(ctx context.Context, instanceID string) error {
	f.incrementCall("DeleteInstance")
	if f.DeleteInstanceFunc != nil {
		return f.DeleteInstanceFunc(ctx, instanceID)
	}
	f.mu.Lock()
	delete(f.instances, instanceID)
	f.mu.Unlock()
	return nil
}

func (f *FakeFederationInstanceRepository) ListInstancesByStatus(ctx context.Context, status types.InstanceStatus, limit int) ([]*types.Instance, error) {
	f.incrementCall("ListInstancesByStatus")
	if f.ListInstancesByStatusFunc != nil {
		return f.ListInstancesByStatusFunc(ctx, status, limit)
	}
	return []*types.Instance{}, nil
}

func (f *FakeFederationInstanceRepository) ListHealthyInstances(ctx context.Context) ([]*types.Instance, error) {
	f.incrementCall("ListHealthyInstances")
	if f.ListHealthyInstancesFunc != nil {
		return f.ListHealthyInstancesFunc(ctx)
	}
	return []*types.Instance{}, nil
}

func (f *FakeFederationInstanceRepository) GetInstancesByTier(ctx context.Context, tier types.TierLevel, limit int) ([]*types.Instance, error) {
	f.incrementCall("GetInstancesByTier")
	if f.GetInstancesByTierFunc != nil {
		return f.GetInstancesByTierFunc(ctx, tier, limit)
	}
	return []*types.Instance{}, nil
}

func (f *FakeFederationInstanceRepository) BatchGetInstances(ctx context.Context, instanceIDs []string) ([]*types.Instance, error) {
	f.incrementCall("BatchGetInstances")
	if f.BatchGetInstancesFunc != nil {
		return f.BatchGetInstancesFunc(ctx, instanceIDs)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	result := []*types.Instance{}
	for _, id := range instanceIDs {
		if instance, ok := f.instances[id]; ok {
			result = append(result, instance)
		}
	}
	return result, nil
}

func (f *FakeFederationInstanceRepository) SearchInstances(ctx context.Context, domainPattern string, limit int) ([]*types.Instance, error) {
	f.incrementCall("SearchInstances")
	if f.SearchInstancesFunc != nil {
		return f.SearchInstancesFunc(ctx, domainPattern, limit)
	}
	return []*types.Instance{}, nil
}

func (f *FakeFederationInstanceRepository) ListAllInstances(ctx context.Context, limit int, startKey map[string]interface{}) ([]*types.Instance, map[string]interface{}, error) {
	f.incrementCall("ListAllInstances")
	if f.ListAllInstancesFunc != nil {
		return f.ListAllInstancesFunc(ctx, limit, startKey)
	}
	return []*types.Instance{}, nil, nil
}

func (f *FakeFederationInstanceRepository) BatchCreateInstances(ctx context.Context, instances []*types.Instance) error {
	f.incrementCall("BatchCreateInstances")
	if f.BatchCreateInstancesFunc != nil {
		return f.BatchCreateInstancesFunc(ctx, instances)
	}
	return nil
}

func (f *FakeFederationInstanceRepository) BatchUpdateInstancesHealth(ctx context.Context, healthUpdates map[string]*types.HealthStatus) error {
	f.incrementCall("BatchUpdateInstancesHealth")
	if f.BatchUpdateInstancesHealthFunc != nil {
		return f.BatchUpdateInstancesHealthFunc(ctx, healthUpdates)
	}
	return nil
}

func (f *FakeFederationInstanceRepository) BatchUpdateInstancesUsage(ctx context.Context, usageUpdates map[string]int64) error {
	f.incrementCall("BatchUpdateInstancesUsage")
	if f.BatchUpdateInstancesUsageFunc != nil {
		return f.BatchUpdateInstancesUsageFunc(ctx, usageUpdates)
	}
	return nil
}

func (f *FakeFederationInstanceRepository) UpdateInstanceHealth(ctx context.Context, instanceID string, health *types.HealthStatus) error {
	f.incrementCall("UpdateInstanceHealth")
	if f.UpdateInstanceHealthFunc != nil {
		return f.UpdateInstanceHealthFunc(ctx, instanceID, health)
	}
	return nil
}

func (f *FakeFederationInstanceRepository) UpdateInstanceUsage(ctx context.Context, instanceID string, bytesUsed int64) error {
	f.incrementCall("UpdateInstanceUsage")
	if f.UpdateInstanceUsageFunc != nil {
		return f.UpdateInstanceUsageFunc(ctx, instanceID, bytesUsed)
	}
	return nil
}

func (f *FakeFederationInstanceRepository) GetHealthHistory(ctx context.Context, instanceID string, duration time.Duration) ([]*types.HealthStatus, error) {
	f.incrementCall("GetHealthHistory")
	if f.GetHealthHistoryFunc != nil {
		return f.GetHealthHistoryFunc(ctx, instanceID, duration)
	}
	return []*types.HealthStatus{}, nil
}

// === Helper functions ===

func createTestInstance(id, domain string) *types.Instance {
	return &types.Instance{
		ID:             id,
		Domain:         domain,
		InboxURL:       "https://" + domain + "/inbox",
		SharedInboxURL: "https://" + domain + "/shared-inbox",
		Status:         types.InstanceStatusActive,
		TierLevel:      types.TierStandard,
	}
}

// === RegisterInstance Tests ===

func TestRegisterInstance(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("generates_id_when_not_provided", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		instance := &types.Instance{
			Domain:   "example.com",
			InboxURL: "https://example.com/inbox",
		}

		err := registry.RegisterInstance(ctx, instance)

		require.NoError(t, err)
		assert.NotEmpty(t, instance.ID)
		assert.Contains(t, instance.ID, "example.com")
	})

	t.Run("sets_timestamps_and_status", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		instance := createTestInstance("test-id", "example.com")
		instance.RegisteredAt = time.Time{}
		instance.LastSeen = time.Time{}
		instance.Status = ""

		before := time.Now()
		err := registry.RegisterInstance(ctx, instance)
		after := time.Now()

		require.NoError(t, err)
		assert.Equal(t, types.InstanceStatusActive, instance.Status)
		assert.True(t, instance.RegisteredAt.After(before.Add(-time.Second)))
		assert.True(t, instance.RegisteredAt.Before(after.Add(time.Second)))
		assert.True(t, instance.LastSeen.After(before.Add(-time.Second)))
	})

	t.Run("updates_cache_after_registration", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		instance := createTestInstance("cached-instance", "example.com")

		err := registry.RegisterInstance(ctx, instance)
		require.NoError(t, err)

		// Should be in cache
		cached, ok := registry.cache.Load("cached-instance")
		assert.True(t, ok)
		assert.NotNil(t, cached)
	})

	t.Run("returns_wrapped_error_on_repo_failure", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		fakeRepo.CreateInstanceFunc = func(ctx context.Context, instance *types.Instance) error {
			return errors.New("database error")
		}
		registry := NewInstanceRegistry(fakeRepo, logger)

		instance := createTestInstance("test-id", "example.com")

		err := registry.RegisterInstance(ctx, instance)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInstanceRegistrationFailed))
	})

	t.Run("calls_repository", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		instance := createTestInstance("test-id", "example.com")

		err := registry.RegisterInstance(ctx, instance)

		require.NoError(t, err)
		assert.Equal(t, 1, fakeRepo.GetCallCount("CreateInstance"))
	})
}

// === GetInstance Tests ===

func TestGetInstance(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("returns_cached_value_no_repo_call", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		// Pre-populate cache
		instance := createTestInstance("cached-id", "example.com")
		registry.cache.Store("cached-id", &cachedInstance{
			instance: instance,
			cachedAt: time.Now(),
		})

		result, err := registry.GetInstance(ctx, "cached-id")

		require.NoError(t, err)
		assert.Equal(t, "cached-id", result.ID)
		assert.Equal(t, 0, fakeRepo.GetCallCount("GetInstance"), "should not call repo")
	})

	t.Run("cache_miss_calls_repo", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		fakeRepo.AddInstance(createTestInstance("uncached-id", "example.com"))
		registry := NewInstanceRegistry(fakeRepo, logger)

		result, err := registry.GetInstance(ctx, "uncached-id")

		require.NoError(t, err)
		assert.Equal(t, "uncached-id", result.ID)
		assert.Equal(t, 1, fakeRepo.GetCallCount("GetInstance"), "should call repo once")
	})

	t.Run("expired_cache_calls_repo", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		fakeRepo.AddInstance(createTestInstance("expired-id", "example.com"))
		registry := NewInstanceRegistry(fakeRepo, logger)

		// Pre-populate cache with expired entry
		instance := createTestInstance("expired-id", "example.com")
		registry.cache.Store("expired-id", &cachedInstance{
			instance: instance,
			cachedAt: time.Now().Add(-10 * time.Minute), // Expired (TTL is 5 minutes)
		})

		result, err := registry.GetInstance(ctx, "expired-id")

		require.NoError(t, err)
		assert.Equal(t, "expired-id", result.ID)
		assert.Equal(t, 1, fakeRepo.GetCallCount("GetInstance"), "should call repo for expired cache")
	})

	t.Run("updates_cache_after_repo_call", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		fakeRepo.AddInstance(createTestInstance("new-id", "new.example.com"))
		registry := NewInstanceRegistry(fakeRepo, logger)

		_, err := registry.GetInstance(ctx, "new-id")
		require.NoError(t, err)

		// Should now be cached
		cached, ok := registry.cache.Load("new-id")
		assert.True(t, ok)
		ci := cached.(*cachedInstance)
		assert.Equal(t, "new-id", ci.instance.ID)
	})
}

// === BatchGetInstances Tests ===

func TestBatchGetInstances(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("empty_ids_returns_empty", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		result, err := registry.BatchGetInstances(ctx, []string{})

		require.NoError(t, err)
		assert.Empty(t, result)
		assert.Equal(t, 0, fakeRepo.GetCallCount("BatchGetInstances"))
	})

	t.Run("all_cached_no_repo_call", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		registry := NewInstanceRegistry(fakeRepo, logger)

		// Pre-populate cache
		for i := 1; i <= 3; i++ {
			id := "cached-" + string(rune('0'+i))
			registry.cache.Store(id, &cachedInstance{
				instance: createTestInstance(id, "example.com"),
				cachedAt: time.Now(),
			})
		}

		result, err := registry.BatchGetInstances(ctx, []string{"cached-1", "cached-2", "cached-3"})

		require.NoError(t, err)
		assert.Len(t, result, 3)
		assert.Equal(t, 0, fakeRepo.GetCallCount("BatchGetInstances"), "should not call repo")
	})

	t.Run("all_uncached_single_repo_call", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		fakeRepo.AddInstance(createTestInstance("uncached-1", "example.com"))
		fakeRepo.AddInstance(createTestInstance("uncached-2", "example.com"))
		fakeRepo.AddInstance(createTestInstance("uncached-3", "example.com"))
		registry := NewInstanceRegistry(fakeRepo, logger)

		result, err := registry.BatchGetInstances(ctx, []string{"uncached-1", "uncached-2", "uncached-3"})

		require.NoError(t, err)
		assert.Len(t, result, 3)
		assert.Equal(t, 1, fakeRepo.GetCallCount("BatchGetInstances"), "should make single batch call")
	})

	t.Run("partial_cache_only_fetches_uncached", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		fakeRepo.AddInstance(createTestInstance("uncached-a", "example.com"))
		fakeRepo.AddInstance(createTestInstance("uncached-b", "example.com"))
		registry := NewInstanceRegistry(fakeRepo, logger)

		// Pre-populate cache for one item
		registry.cache.Store("cached-a", &cachedInstance{
			instance: createTestInstance("cached-a", "example.com"),
			cachedAt: time.Now(),
		})

		// Track which IDs were requested
		var requestedIDs []string
		fakeRepo.BatchGetInstancesFunc = func(ctx context.Context, instanceIDs []string) ([]*types.Instance, error) {
			requestedIDs = instanceIDs
			result := []*types.Instance{}
			for _, id := range instanceIDs {
				if instance, ok := fakeRepo.instances[id]; ok {
					result = append(result, instance)
				}
			}
			return result, nil
		}

		result, err := registry.BatchGetInstances(ctx, []string{"cached-a", "uncached-a", "uncached-b"})

		require.NoError(t, err)
		assert.Len(t, result, 3)
		// Should only request the uncached IDs
		assert.NotContains(t, requestedIDs, "cached-a")
		assert.Contains(t, requestedIDs, "uncached-a")
		assert.Contains(t, requestedIDs, "uncached-b")
	})

	t.Run("updates_cache_for_fetched_instances", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		fakeRepo.AddInstance(createTestInstance("to-cache", "example.com"))
		registry := NewInstanceRegistry(fakeRepo, logger)

		_, err := registry.BatchGetInstances(ctx, []string{"to-cache"})
		require.NoError(t, err)

		// Should now be cached
		_, ok := registry.cache.Load("to-cache")
		assert.True(t, ok)
	})

	t.Run("returns_wrapped_error_on_repo_failure", func(t *testing.T) {
		fakeRepo := NewFakeFederationInstanceRepository()
		fakeRepo.BatchGetInstancesFunc = func(ctx context.Context, instanceIDs []string) ([]*types.Instance, error) {
			return nil, errors.New("batch get failed")
		}
		registry := NewInstanceRegistry(fakeRepo, logger)

		_, err := registry.BatchGetInstances(ctx, []string{"test-id"})

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInstanceBatchGetFailed))
	})
}

// === ClearCache Tests ===

func TestClearCache(t *testing.T) {
	logger := zaptest.NewLogger(t)
	fakeRepo := NewFakeFederationInstanceRepository()
	registry := NewInstanceRegistry(fakeRepo, logger)

	// Pre-populate cache
	registry.cache.Store("id-1", &cachedInstance{instance: createTestInstance("id-1", "a.com"), cachedAt: time.Now()})
	registry.cache.Store("id-2", &cachedInstance{instance: createTestInstance("id-2", "b.com"), cachedAt: time.Now()})
	registry.cache.Store("id-3", &cachedInstance{instance: createTestInstance("id-3", "c.com"), cachedAt: time.Now()})

	registry.ClearCache()

	// All entries should be gone
	_, ok1 := registry.cache.Load("id-1")
	_, ok2 := registry.cache.Load("id-2")
	_, ok3 := registry.cache.Load("id-3")

	assert.False(t, ok1)
	assert.False(t, ok2)
	assert.False(t, ok3)
}

// === GetCacheStats Tests ===

func TestGetCacheStats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	fakeRepo := NewFakeFederationInstanceRepository()
	registry := NewInstanceRegistry(fakeRepo, logger)

	t.Run("empty_cache", func(t *testing.T) {
		stats := registry.GetCacheStats()

		assert.Equal(t, 0, stats["cached_instances"])
		assert.Equal(t, 300.0, stats["cache_ttl_seconds"]) // 5 minutes
	})

	t.Run("with_cached_instances", func(t *testing.T) {
		registry.cache.Store("id-1", &cachedInstance{instance: createTestInstance("id-1", "a.com"), cachedAt: time.Now()})
		registry.cache.Store("id-2", &cachedInstance{instance: createTestInstance("id-2", "b.com"), cachedAt: time.Now()})

		stats := registry.GetCacheStats()

		assert.Equal(t, 2, stats["cached_instances"])
	})
}
