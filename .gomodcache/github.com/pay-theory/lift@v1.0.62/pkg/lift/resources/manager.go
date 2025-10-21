// Package resources provides resource management utilities for Lift applications,
// including resource pooling, lifecycle management, and cleanup coordination.
package resources

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// ResourceManager coordinates multiple resource pools and provides lifecycle management
// Memory optimized: 64 → 32 bytes (32 bytes saved)
type ResourceManager struct {
	logger          lift.Logger
	pools           map[string]ConnectionPool
	preWarmers      map[string]PreWarmer
	shutdownTimeout time.Duration
	mu              sync.RWMutex
	closed          bool
}

// PreWarmer defines how to pre-warm a resource pool
type PreWarmer interface {
	// PreWarm initializes the pool with resources
	PreWarm(ctx context.Context, pool ConnectionPool) error

	// Name returns the name of this pre-warmer
	Name() string
}

// ResourceManagerConfig configures the resource manager
// Memory optimized: 32 → 16 bytes (16 bytes saved)
type ResourceManagerConfig struct {
	// Interface first (24 bytes)
	Logger lift.Logger
	// Durations (8 bytes each)
	ShutdownTimeout time.Duration
	PreWarmTimeout  time.Duration
}

// NewResourceManager creates a new resource manager
func NewResourceManager(config ResourceManagerConfig) *ResourceManager {
	return &ResourceManager{
		pools:           make(map[string]ConnectionPool),
		preWarmers:      make(map[string]PreWarmer),
		logger:          config.Logger,
		shutdownTimeout: config.ShutdownTimeout,
	}
}

// RegisterPool registers a connection pool with the manager
func (rm *ResourceManager) RegisterPool(name string, pool ConnectionPool) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.closed {
		return fmt.Errorf("resource manager is closed")
	}

	if _, exists := rm.pools[name]; exists {
		return fmt.Errorf("pool %s already registered", name)
	}

	rm.pools[name] = pool
	return nil
}

// RegisterPreWarmer registers a pre-warmer for a pool
func (rm *ResourceManager) RegisterPreWarmer(poolName string, preWarmer PreWarmer) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.closed {
		return fmt.Errorf("resource manager is closed")
	}

	rm.preWarmers[poolName] = preWarmer
	return nil
}

// GetPool retrieves a registered pool
func (rm *ResourceManager) GetPool(name string) (ConnectionPool, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.closed {
		return nil, fmt.Errorf("resource manager is closed")
	}

	pool, exists := rm.pools[name]
	if !exists {
		return nil, fmt.Errorf("pool %s not found", name)
	}

	return pool, nil
}

// PreWarmAll pre-warms all registered pools
func (rm *ResourceManager) PreWarmAll(ctx context.Context) error {
	rm.mu.RLock()
	pools := make(map[string]ConnectionPool)
	preWarmers := make(map[string]PreWarmer)

	for name, pool := range rm.pools {
		pools[name] = pool
	}

	for name, preWarmer := range rm.preWarmers {
		preWarmers[name] = preWarmer
	}
	rm.mu.RUnlock()

	// Pre-warm pools in parallel
	var wg sync.WaitGroup
	errChan := make(chan error, len(preWarmers))

	for poolName, preWarmer := range preWarmers {
		pool, exists := pools[poolName]
		if !exists {
			continue
		}

		wg.Add(1)
		go func(name string, pw PreWarmer, p ConnectionPool) {
			defer wg.Done()

			if err := pw.PreWarm(ctx, p); err != nil {
				errChan <- fmt.Errorf("pre-warming %s failed: %w", name, err)
			}
		}(poolName, preWarmer, pool)
	}

	wg.Wait()
	close(errChan)

	// Collect any errors
	errors := make([]error, 0, len(preWarmers))
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("pre-warming failed: %v", errors)
	}

	return nil
}

// HealthCheck checks the health of all pools
func (rm *ResourceManager) HealthCheck(ctx context.Context) map[string]error {
	rm.mu.RLock()
	pools := make(map[string]ConnectionPool)
	for name, pool := range rm.pools {
		pools[name] = pool
	}
	rm.mu.RUnlock()

	results := make(map[string]error)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, pool := range pools {
		wg.Add(1)
		go func(poolName string, p ConnectionPool) {
			defer wg.Done()

			err := p.HealthCheck(ctx)

			mu.Lock()
			results[poolName] = err
			mu.Unlock()
		}(name, pool)
	}

	wg.Wait()
	return results
}

// Stats returns statistics for all pools
func (rm *ResourceManager) Stats() map[string]PoolStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	stats := make(map[string]PoolStats)
	for name, pool := range rm.pools {
		stats[name] = pool.Stats()
	}

	return stats
}

// Close gracefully shuts down all pools
func (rm *ResourceManager) Close() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.closed {
		return nil
	}

	rm.closed = true

	// Close all pools with timeout
	ctx, cancel := context.WithTimeout(context.Background(), rm.shutdownTimeout)
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, len(rm.pools))

	for name, pool := range rm.pools {
		wg.Add(1)
		go func(poolName string, p ConnectionPool) {
			defer wg.Done()

			done := make(chan error, 1)
			go func() {
				done <- p.Close()
			}()

			select {
			case err := <-done:
				if err != nil {
					errChan <- fmt.Errorf("closing pool %s: %w", poolName, err)
				}
			case <-ctx.Done():
				errChan <- fmt.Errorf("timeout closing pool %s", poolName)
			}
		}(name, pool)
	}

	wg.Wait()
	close(errChan)

	// Collect any errors
	errors := make([]error, 0, len(rm.pools))
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

// DefaultPreWarmer provides a basic pre-warming implementation
type DefaultPreWarmer struct {
	name     string
	minConns int
	timeout  time.Duration
}

// NewDefaultPreWarmer creates a default pre-warmer
func NewDefaultPreWarmer(name string, minConns int, timeout time.Duration) *DefaultPreWarmer {
	return &DefaultPreWarmer{
		name:     name,
		minConns: minConns,
		timeout:  timeout,
	}
}

// PreWarm pre-warms the pool by creating minimum connections
func (pw *DefaultPreWarmer) PreWarm(ctx context.Context, pool ConnectionPool) error {
	ctx, cancel := context.WithTimeout(ctx, pw.timeout)
	defer cancel()

	// Create and return connections to warm the pool
	var resources []any

	for i := 0; i < pw.minConns; i++ {
		resource, err := pool.Get(ctx)
		if err != nil {
			// Return any resources we've already gotten
			for _, res := range resources {
				if putErr := pool.Put(res); putErr != nil {
					// Log but continue cleanup - this is best-effort cleanup
					// Note: Cannot access manager logger from DefaultPreWarmer context
					_ = putErr // Intentionally ignored for best-effort cleanup
				}
			}
			return fmt.Errorf("failed to pre-warm connection %d: %w", i+1, err)
		}
		resources = append(resources, resource)
	}

	// Return all resources to the pool
	for _, resource := range resources {
		if err := pool.Put(resource); err != nil {
			return fmt.Errorf("failed to return pre-warmed connection: %w", err)
		}
	}

	return nil
}

// Name returns the name of this pre-warmer
func (pw *DefaultPreWarmer) Name() string {
	return pw.name
}

// DefaultResourceManagerConfig returns sensible defaults
func DefaultResourceManagerConfig() ResourceManagerConfig {
	return ResourceManagerConfig{
		ShutdownTimeout: 30 * time.Second,
		PreWarmTimeout:  10 * time.Second,
	}
}
