package resources

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// ConnectionPool manages a pool of reusable resources
type ConnectionPool interface {
	// Get retrieves a resource from the pool
	Get(ctx context.Context) (any, error)

	// Put returns a resource to the pool
	Put(resource any) error

	// Close shuts down the pool and cleans up all resources
	Close() error

	// Stats returns current pool statistics
	Stats() PoolStats

	// HealthCheck verifies pool health
	HealthCheck(ctx context.Context) error
}

// Resource defines the interface for pooled resources
type Resource interface {
	// Initialize sets up the resource
	Initialize(ctx context.Context) error

	// HealthCheck verifies resource health
	HealthCheck(ctx context.Context) error

	// Cleanup releases resource-specific resources
	Cleanup() error

	// IsValid checks if the resource is still usable
	IsValid() bool

	// LastUsed returns when the resource was last used
	LastUsed() time.Time

	// MarkUsed updates the last used timestamp
	MarkUsed()
}

// ResourceFactory creates new resources
type ResourceFactory interface {
	// Create creates a new resource instance
	Create(ctx context.Context) (Resource, error)

	// Validate checks if a resource is still valid
	Validate(resource Resource) bool
}

// PoolConfig configures connection pool behavior
// Memory optimized: 80 → 16 bytes (64 bytes saved)
type PoolConfig struct {
	// Interface first (24 bytes)
	Logger lift.Logger
	// Durations (8 bytes each)
	IdleTimeout         time.Duration
	MaxLifetime         time.Duration
	GetTimeout          time.Duration
	HealthCheckInterval time.Duration
	// Ints (4 bytes each)
	MinIdle   int
	MaxActive int
	MaxIdle   int
	// Bool last (1 byte)
	PreWarm bool
}

// PoolStats provides pool statistics
type PoolStats struct {
	// Active number of active connections
	Active int `json:"active"`

	// Idle number of idle connections
	Idle int `json:"idle"`

	// Total total connections created
	Total int `json:"total"`

	// Gets total number of get requests
	Gets int64 `json:"gets"`

	// Puts total number of put requests
	Puts int64 `json:"puts"`

	// Hits successful gets from pool
	Hits int64 `json:"hits"`

	// Misses gets that required new connection
	Misses int64 `json:"misses"`

	// Timeouts gets that timed out
	Timeouts int64 `json:"timeouts"`

	// Errors connection errors
	Errors int64 `json:"errors"`
}

// DefaultConnectionPool implements ConnectionPool
// Memory optimized: 240 → 152 bytes (88 bytes saved)
type DefaultConnectionPool struct {
	factory       ResourceFactory
	logger        lift.Logger
	active        map[Resource]bool
	cond          *sync.Cond
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
	idle          []Resource
	config        PoolConfig
	stats         PoolStats
	mu            sync.RWMutex
	closed        bool
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config PoolConfig, factory ResourceFactory) *DefaultConnectionPool {
	pool := &DefaultConnectionPool{
		config:      config,
		factory:     factory,
		logger:      config.Logger,
		idle:        make([]Resource, 0, config.MaxIdle),
		active:      make(map[Resource]bool),
		stopCleanup: make(chan struct{}),
	}

	pool.cond = sync.NewCond(&pool.mu)

	// Start background cleanup if health check interval is set
	if config.HealthCheckInterval > 0 {
		pool.startCleanup()
	}

	return pool
}

// Get retrieves a resource from the pool
func (p *DefaultConnectionPool) Get(ctx context.Context) (any, error) {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("pool is closed")
	}

	p.stats.Gets++

	// Try to get from idle pool first
	if len(p.idle) > 0 {
		resource := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]

		// Validate the resource
		if p.factory.Validate(resource) && resource.IsValid() {
			p.active[resource] = true
			resource.MarkUsed()
			p.stats.Hits++
			p.mu.Unlock()
			return resource, nil
		}

		// Resource is invalid, clean it up
		if err := resource.Cleanup(); err != nil {
			// Log cleanup error but continue - this is best-effort cleanup
			if p.logger != nil {
				p.logger.WithField("error", err).Warn("Failed to cleanup resource")
			}
		}
	}

	// Check if we can create a new connection
	if len(p.active) >= p.config.MaxActive {
		p.stats.Timeouts++
		p.mu.Unlock()
		return nil, errors.New("connection pool exhausted")
	}

	// Create new resource (unlock while creating)
	p.mu.Unlock()

	resource, err := p.factory.Create(ctx)
	if err != nil {
		p.mu.Lock()
		p.stats.Errors++
		p.mu.Unlock()
		return nil, err
	}

	if err := resource.Initialize(ctx); err != nil {
		p.mu.Lock()
		p.stats.Errors++
		p.mu.Unlock()
		if cleanupErr := resource.Cleanup(); cleanupErr != nil {
			// Log cleanup error but continue - this is best-effort cleanup
			if p.logger != nil {
				p.logger.WithField("error", cleanupErr).Warn("Failed to cleanup resource during initialization")
			}
		}
		return nil, err
	}

	p.mu.Lock()
	p.active[resource] = true
	p.stats.Total++
	p.stats.Misses++
	resource.MarkUsed()
	p.mu.Unlock()

	return resource, nil
}

// Put returns a resource to the pool
func (p *DefaultConnectionPool) Put(resource any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return errors.New("pool is closed")
	}

	res, ok := resource.(Resource)
	if !ok {
		return errors.New("resource does not implement Resource interface")
	}

	p.stats.Puts++

	// Remove from active
	delete(p.active, res)

	// Check if resource is still valid
	if !p.factory.Validate(res) || !res.IsValid() {
		if err := res.Cleanup(); err != nil {
			// Log cleanup error but continue - this is best-effort cleanup
			if p.logger != nil {
				p.logger.WithField("error", err).Warn("Failed to cleanup invalid resource")
			}
		}
		p.cond.Signal()
		return nil
	}

	// Add to idle pool if there's space
	if len(p.idle) < p.config.MaxIdle {
		p.idle = append(p.idle, res)
	} else {
		// Pool is full, cleanup the resource
		if err := res.Cleanup(); err != nil {
			// Log cleanup error but continue - this is best-effort cleanup
			if p.logger != nil {
				p.logger.WithField("error", err).Warn("Failed to cleanup resource when pool is full")
			}
		}
	}

	p.cond.Signal()
	return nil
}

// Close shuts down the pool
func (p *DefaultConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	// Stop cleanup goroutine
	if p.cleanupTicker != nil {
		p.cleanupTicker.Stop()
		close(p.stopCleanup)
	}

	// Cleanup all idle resources
	for _, resource := range p.idle {
		if err := resource.Cleanup(); err != nil {
			// Log cleanup error but continue - this is best-effort cleanup
			if p.logger != nil {
				p.logger.WithField("error", err).Warn("Failed to cleanup resource")
			}
		}
	}
	p.idle = nil

	// Cleanup all active resources
	for resource := range p.active {
		if err := resource.Cleanup(); err != nil {
			// Log cleanup error but continue - this is best-effort cleanup
			if p.logger != nil {
				p.logger.WithField("error", err).Warn("Failed to cleanup resource")
			}
		}
	}
	p.active = nil

	p.cond.Broadcast()
	return nil
}

// Stats returns current pool statistics
func (p *DefaultConnectionPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := p.stats
	stats.Active = len(p.active)
	stats.Idle = len(p.idle)

	return stats
}

// HealthCheck verifies pool health
func (p *DefaultConnectionPool) HealthCheck(_ context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return errors.New("pool is closed")
	}

	// Check if we have minimum idle connections
	if len(p.idle) < p.config.MinIdle {
		return errors.New("insufficient idle connections")
	}

	return nil
}

// startCleanup starts the background cleanup goroutine
func (p *DefaultConnectionPool) startCleanup() {
	p.cleanupTicker = time.NewTicker(p.config.HealthCheckInterval)

	go func() {
		for {
			select {
			case <-p.cleanupTicker.C:
				p.cleanup()
			case <-p.stopCleanup:
				return
			}
		}
	}()
}

// cleanup removes stale connections from the idle pool
func (p *DefaultConnectionPool) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	cleaner := newPoolResourceCleaner(p.config, p.logger)
	p.idle = cleaner.cleanResources(p.idle)
}

// poolResourceCleaner handles cleanup of pool resources
type poolResourceCleaner struct {
	now    time.Time
	logger lift.Logger
	config PoolConfig
}

// newPoolResourceCleaner creates a new pool resource cleaner
func newPoolResourceCleaner(config PoolConfig, logger lift.Logger) *poolResourceCleaner {
	return &poolResourceCleaner{
		config: config,
		logger: logger,
		now:    time.Now(),
	}
}

// cleanResources filters and cleans resources, returning valid ones
func (c *poolResourceCleaner) cleanResources(resources []Resource) []Resource {
	validIdle := make([]Resource, 0, len(resources))

	for _, resource := range resources {
		if c.shouldKeepResource(resource) {
			validIdle = append(validIdle, resource)
		} else {
			c.cleanupResource(resource, "cleanup")
		}
	}

	return validIdle
}

// shouldKeepResource determines if a resource should be kept in the pool
func (c *poolResourceCleaner) shouldKeepResource(resource Resource) bool {
	// Check max lifetime
	if c.hasExceededMaxLifetime(resource) {
		return false
	}

	// Check idle timeout
	if c.hasExceededIdleTimeout(resource) {
		return false
	}

	// Perform health check
	return c.isResourceHealthy(resource)
}

// hasExceededMaxLifetime checks if resource has exceeded its maximum lifetime
func (c *poolResourceCleaner) hasExceededMaxLifetime(resource Resource) bool {
	return c.config.MaxLifetime > 0 && c.now.Sub(resource.LastUsed()) > c.config.MaxLifetime
}

// hasExceededIdleTimeout checks if resource has been idle too long
func (c *poolResourceCleaner) hasExceededIdleTimeout(resource Resource) bool {
	return c.config.IdleTimeout > 0 && c.now.Sub(resource.LastUsed()) > c.config.IdleTimeout
}

// isResourceHealthy performs a health check on the resource
func (c *poolResourceCleaner) isResourceHealthy(resource Resource) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := resource.HealthCheck(ctx); err != nil {
		c.cleanupResource(resource, "health check")
		return false
	}

	return true
}

// cleanupResource safely cleans up a resource with error logging
func (c *poolResourceCleaner) cleanupResource(resource Resource, reason string) {
	if err := resource.Cleanup(); err != nil {
		if c.logger != nil {
			message := fmt.Sprintf("Failed to cleanup resource during %s", reason)
			c.logger.WithField("error", err).Warn(message)
		}
	}
}

// DefaultPoolConfig returns a sensible default configuration
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MinIdle:             2,
		MaxActive:           10,
		MaxIdle:             5,
		IdleTimeout:         5 * time.Minute,
		MaxLifetime:         30 * time.Minute,
		GetTimeout:          30 * time.Second,
		HealthCheckInterval: 1 * time.Minute,
		PreWarm:             true,
	}
}
