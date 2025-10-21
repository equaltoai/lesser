package resources

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Mock resource for testing
type mockResource struct {
	lastUsed time.Time
	id       int
	mu       sync.Mutex
	valid    bool
}

func (r *mockResource) Initialize(_ context.Context) error {
	return nil
}

func (r *mockResource) HealthCheck(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.valid {
		return errors.New("resource is invalid")
	}
	return nil
}

func (r *mockResource) Cleanup() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.valid = false
	return nil
}

func (r *mockResource) IsValid() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.valid
}

func (r *mockResource) LastUsed() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastUsed
}

func (r *mockResource) MarkUsed() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastUsed = time.Now()
}

// Mock factory for testing
type mockFactory struct {
	counter int
	mu      sync.Mutex
}

func (f *mockFactory) Create(_ context.Context) (Resource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.counter++
	return &mockResource{
		id:       f.counter,
		valid:    true,
		lastUsed: time.Now(),
	}, nil
}

func (f *mockFactory) Validate(resource Resource) bool {
	return resource.IsValid()
}

func TestConnectionPool_BasicOperations(t *testing.T) {
	config := PoolConfig{
		MinIdle:   1,
		MaxActive: 5,
		MaxIdle:   3,
	}

	factory := &mockFactory{}
	pool := NewConnectionPool(config, factory)
	defer func() {
		if err := pool.Close(); err != nil {
			t.Logf("Warning: failed to close pool: %v", err)
		}
	}()

	ctx := context.Background()

	// Test Get
	resource1, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get resource: %v", err)
	}

	// Test Put
	err = pool.Put(resource1)
	if err != nil {
		t.Fatalf("failed to put resource: %v", err)
	}

	// Test Stats
	stats := pool.Stats()
	if stats.Total != 1 {
		t.Errorf("expected 1 total connection, got %d", stats.Total)
	}

	if stats.Idle != 1 {
		t.Errorf("expected 1 idle connection, got %d", stats.Idle)
	}
}

func TestConnectionPool_MaxActive(t *testing.T) {
	config := PoolConfig{
		MinIdle:   0,
		MaxActive: 2,
		MaxIdle:   2,
	}

	factory := &mockFactory{}
	pool := NewConnectionPool(config, factory)
	defer func() {
		if err := pool.Close(); err != nil {
			t.Logf("Warning: failed to close pool: %v", err)
		}
	}()

	ctx := context.Background()

	// Get max active connections
	resource1, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get resource 1: %v", err)
	}

	resource2, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get resource 2: %v", err)
	}

	// Try to get one more (should timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = pool.Get(ctx)
	if err == nil {
		t.Error("expected timeout error when exceeding max active")
	}

	// Return one resource
	if err := pool.Put(resource1); err != nil {
		t.Logf("Warning: failed to put resource: %v", err)
	}

	// Now we should be able to get another
	ctx = context.Background()
	resource3, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get resource after put: %v", err)
	}

	if err := pool.Put(resource2); err != nil {
		t.Logf("Warning: failed to put resource: %v", err)
	}
	if err := pool.Put(resource3); err != nil {
		t.Logf("Warning: failed to put resource: %v", err)
	}
}

func TestConnectionPool_HealthCheck(t *testing.T) {
	config := PoolConfig{
		MinIdle:   2,
		MaxActive: 10,
		MaxIdle:   5,
	}

	factory := &mockFactory{}
	pool := NewConnectionPool(config, factory)
	defer func() {
		if err := pool.Close(); err != nil {
			t.Logf("Warning: failed to close pool: %v", err)
		}
	}()

	ctx := context.Background()

	// Pool should fail health check initially (no idle connections)
	err := pool.HealthCheck(ctx)
	if err == nil {
		t.Error("expected health check to fail with insufficient idle connections")
	}

	// Add some connections
	resource1, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get resource1: %v", err)
	}
	resource2, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get resource2: %v", err)
	}
	if err := pool.Put(resource1); err != nil {
		t.Logf("Warning: failed to put resource: %v", err)
	}
	if err := pool.Put(resource2); err != nil {
		t.Logf("Warning: failed to put resource: %v", err)
	}

	// Now health check should pass
	err = pool.HealthCheck(ctx)
	if err != nil {
		t.Errorf("health check failed: %v", err)
	}
}

func TestConnectionPool_ConcurrentAccess(t *testing.T) {
	config := PoolConfig{
		MinIdle:   0,
		MaxActive: 25, // Increase to handle 20 concurrent goroutines
		MaxIdle:   10,
	}

	factory := &mockFactory{}
	pool := NewConnectionPool(config, factory)
	defer func() {
		if err := pool.Close(); err != nil {
			t.Logf("Warning: failed to close pool: %v", err)
		}
	}()

	ctx := context.Background()

	// Test concurrent get/put operations
	var wg sync.WaitGroup
	numGoroutines := 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			resource, err := pool.Get(ctx)
			if err != nil {
				t.Errorf("failed to get resource: %v", err)
				return
			}

			// Simulate some work
			time.Sleep(10 * time.Millisecond)

			err = pool.Put(resource)
			if err != nil {
				t.Errorf("failed to put resource: %v", err)
			}
		}()
	}

	wg.Wait()

	stats := pool.Stats()
	if stats.Gets != int64(numGoroutines) {
		t.Errorf("expected %d gets, got %d", numGoroutines, stats.Gets)
	}

	if stats.Puts != int64(numGoroutines) {
		t.Errorf("expected %d puts, got %d", numGoroutines, stats.Puts)
	}
}

func TestConnectionPool_ResourceValidation(t *testing.T) {
	config := PoolConfig{
		MinIdle:   0,
		MaxActive: 10,
		MaxIdle:   3,
	}

	factory := &mockFactory{}
	pool := NewConnectionPool(config, factory)
	defer func() {
		if err := pool.Close(); err != nil {
			t.Logf("Warning: failed to close pool: %v", err)
		}
	}()

	ctx := context.Background()

	// Get a resource and invalidate it
	resource, err := pool.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get resource: %v", err)
	}

	mockRes, ok := resource.(*mockResource)
	if !ok {
		t.Fatalf("Expected resource to be *mockResource, got %T", resource)
	}
	if err := mockRes.Cleanup(); err != nil {
		t.Logf("Warning: cleanup failed: %v", err)
	} // Invalidate the resource

	// Put it back - should be cleaned up
	err = pool.Put(resource)
	if err != nil {
		t.Fatalf("failed to put invalid resource: %v", err)
	}

	stats := pool.Stats()
	if stats.Idle != 0 {
		t.Errorf("expected 0 idle connections after invalid resource, got %d", stats.Idle)
	}
}

func TestResourceManager_BasicOperations(t *testing.T) {
	config := DefaultResourceManagerConfig()
	manager := NewResourceManager(config)
	defer func() {
		if err := manager.Close(); err != nil {
			t.Logf("Warning: failed to close manager: %v", err)
		}
	}()

	// Create and register a pool
	poolConfig := PoolConfig{
		MinIdle:   1,
		MaxActive: 5,
		MaxIdle:   3,
	}

	factory := &mockFactory{}
	pool := NewConnectionPool(poolConfig, factory)

	err := manager.RegisterPool("test-pool", pool)
	if err != nil {
		t.Fatalf("failed to register pool: %v", err)
	}

	// Get the pool back
	retrievedPool, err := manager.GetPool("test-pool")
	if err != nil {
		t.Fatalf("failed to get pool: %v", err)
	}

	if retrievedPool != pool {
		t.Error("retrieved pool is not the same as registered pool")
	}

	// Test health check
	ctx := context.Background()
	healthResults := manager.HealthCheck(ctx)

	if len(healthResults) != 1 {
		t.Errorf("expected 1 health result, got %d", len(healthResults))
	}

	// Test stats
	stats := manager.Stats()
	if len(stats) != 1 {
		t.Errorf("expected 1 pool stats, got %d", len(stats))
	}
}

func TestResourceManager_PreWarming(t *testing.T) {
	config := DefaultResourceManagerConfig()
	manager := NewResourceManager(config)
	defer func() {
		if err := manager.Close(); err != nil {
			t.Logf("Warning: failed to close manager: %v", err)
		}
	}()

	// Create and register a pool
	poolConfig := PoolConfig{
		MinIdle:   0,
		MaxActive: 10,
		MaxIdle:   5,
	}

	factory := &mockFactory{}
	pool := NewConnectionPool(poolConfig, factory)

	err := manager.RegisterPool("test-pool", pool)
	if err != nil {
		t.Fatalf("failed to register pool: %v", err)
	}

	// Register a pre-warmer
	preWarmer := NewDefaultPreWarmer("test-warmer", 3, 5*time.Second)
	err = manager.RegisterPreWarmer("test-pool", preWarmer)
	if err != nil {
		t.Fatalf("failed to register pre-warmer: %v", err)
	}

	// Pre-warm all pools
	ctx := context.Background()
	err = manager.PreWarmAll(ctx)
	if err != nil {
		t.Fatalf("failed to pre-warm pools: %v", err)
	}

	// Check that the pool has idle connections
	stats := pool.Stats()
	if stats.Idle < 3 {
		t.Errorf("expected at least 3 idle connections after pre-warming, got %d", stats.Idle)
	}
}

func TestResourceManager_GracefulShutdown(t *testing.T) {
	config := ResourceManagerConfig{
		ShutdownTimeout: 1 * time.Second,
	}
	manager := NewResourceManager(config)

	// Create and register multiple pools
	for i := 0; i < 3; i++ {
		poolConfig := PoolConfig{
			MinIdle:   1,
			MaxActive: 5,
			MaxIdle:   3,
		}

		factory := &mockFactory{}
		pool := NewConnectionPool(poolConfig, factory)

		err := manager.RegisterPool(fmt.Sprintf("pool-%d", i), pool)
		if err != nil {
			t.Fatalf("failed to register pool %d: %v", i, err)
		}
	}

	// Close should shut down all pools
	err := manager.Close()
	if err != nil {
		t.Fatalf("failed to close manager: %v", err)
	}

	// Verify manager is closed
	_, err = manager.GetPool("pool-0")
	if err == nil {
		t.Error("expected error when getting pool from closed manager")
	}
}

func BenchmarkConnectionPool_GetPut(b *testing.B) {
	config := PoolConfig{
		MinIdle:   2,
		MaxActive: 10,
		MaxIdle:   5,
	}

	factory := &mockFactory{}
	pool := NewConnectionPool(config, factory)
	defer func() {
		if err := pool.Close(); err != nil {
			b.Logf("Warning: failed to close pool: %v", err)
		}
	}()

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resource, err := pool.Get(ctx)
			if err != nil {
				b.Fatalf("failed to get resource: %v", err)
			}

			err = pool.Put(resource)
			if err != nil {
				b.Fatalf("failed to put resource: %v", err)
			}
		}
	})
}

func BenchmarkResourceManager_HealthCheck(b *testing.B) {
	config := DefaultResourceManagerConfig()
	manager := NewResourceManager(config)
	defer func() {
		if err := manager.Close(); err != nil {
			b.Logf("Warning: failed to close manager: %v", err)
		}
	}()

	// Register multiple pools
	for i := 0; i < 5; i++ {
		poolConfig := PoolConfig{
			MinIdle:   2,
			MaxActive: 10,
			MaxIdle:   5,
		}

		factory := &mockFactory{}
		pool := NewConnectionPool(poolConfig, factory)

		if err := manager.RegisterPool(fmt.Sprintf("pool-%d", i), pool); err != nil {
			b.Fatalf("Failed to register pool-%d: %v", i, err)
		}
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.HealthCheck(ctx)
	}
}
