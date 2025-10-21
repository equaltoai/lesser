package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Health status constants
const (
	StatusHealthy   = "healthy"
	StatusDegraded  = "degraded"
	StatusUnhealthy = "unhealthy"
	StatusUnknown   = "unknown"
)

// HealthChecker defines the interface for health checking components
type HealthChecker interface {
	// Check performs a health check and returns the status
	Check(ctx context.Context) HealthStatus

	// Name returns the name of this health checker
	Name() string
}

// HealthStatus represents the result of a health check
type HealthStatus struct {
	Timestamp time.Time      `json:"timestamp"`
	Details   map[string]any `json:"details,omitempty"`
	Status    string         `json:"status"`
	Message   string         `json:"message,omitempty"`
	Error     string         `json:"error,omitempty"`
	Duration  time.Duration  `json:"duration"`
}

// HealthManager coordinates multiple health checkers
type HealthManager interface {
	// RegisterChecker registers a health checker
	RegisterChecker(name string, checker HealthChecker) error

	// UnregisterChecker removes a health checker
	UnregisterChecker(name string) error

	// CheckAll performs health checks on all registered checkers
	CheckAll(ctx context.Context) map[string]HealthStatus

	// CheckComponent performs a health check on a specific component
	CheckComponent(ctx context.Context, name string) (HealthStatus, error)

	// OverallHealth returns the overall health status
	OverallHealth(ctx context.Context) HealthStatus

	// ListCheckers returns the names of all registered checkers
	ListCheckers() []string
}

// DefaultHealthManager implements HealthManager
type DefaultHealthManager struct {
	checkers       map[string]HealthChecker
	cache          map[string]cachedResult
	timeout        time.Duration
	cacheDuration  time.Duration
	mu             sync.RWMutex
	cacheMu        sync.RWMutex
	parallelChecks bool
	cacheEnabled   bool
}

// cachedResult stores cached health check results
type cachedResult struct {
	timestamp time.Time
	status    HealthStatus
}

// HealthManagerConfig configures the health manager
type HealthManagerConfig struct {
	// Timeout for individual health checks
	Timeout time.Duration

	// ParallelChecks whether to run checks in parallel
	ParallelChecks bool

	// CacheEnabled whether to cache health check results
	CacheEnabled bool

	// CacheDuration how long to cache results
	CacheDuration time.Duration
}

// NewHealthManager creates a new health manager
func NewHealthManager(config HealthManagerConfig) *DefaultHealthManager {
	return &DefaultHealthManager{
		checkers:       make(map[string]HealthChecker),
		timeout:        config.Timeout,
		parallelChecks: config.ParallelChecks,
		cacheEnabled:   config.CacheEnabled,
		cacheDuration:  config.CacheDuration,
		cache:          make(map[string]cachedResult),
	}
}

// RegisterChecker registers a health checker
func (hm *DefaultHealthManager) RegisterChecker(name string, checker HealthChecker) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if _, exists := hm.checkers[name]; exists {
		return fmt.Errorf("health checker %s already registered", name)
	}

	hm.checkers[name] = checker
	return nil
}

// UnregisterChecker removes a health checker
func (hm *DefaultHealthManager) UnregisterChecker(name string) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if _, exists := hm.checkers[name]; !exists {
		return fmt.Errorf("health checker %s not found", name)
	}

	delete(hm.checkers, name)

	// Clear cache for this checker
	if hm.cacheEnabled {
		hm.cacheMu.Lock()
		delete(hm.cache, name)
		hm.cacheMu.Unlock()
	}

	return nil
}

// CheckAll performs health checks on all registered checkers
func (hm *DefaultHealthManager) CheckAll(ctx context.Context) map[string]HealthStatus {
	hm.mu.RLock()
	checkers := make(map[string]HealthChecker)
	for name, checker := range hm.checkers {
		checkers[name] = checker
	}
	hm.mu.RUnlock()

	results := make(map[string]HealthStatus)

	if hm.parallelChecks {
		// Run checks in parallel
		var wg sync.WaitGroup
		var mu sync.Mutex

		for name, checker := range checkers {
			wg.Add(1)
			go func(n string, c HealthChecker) {
				defer wg.Done()

				status := hm.performCheck(ctx, n, c)

				mu.Lock()
				results[n] = status
				mu.Unlock()
			}(name, checker)
		}

		wg.Wait()
	} else {
		// Run checks sequentially
		for name, checker := range checkers {
			results[name] = hm.performCheck(ctx, name, checker)
		}
	}

	return results
}

// CheckComponent performs a health check on a specific component
func (hm *DefaultHealthManager) CheckComponent(ctx context.Context, name string) (HealthStatus, error) {
	hm.mu.RLock()
	checker, exists := hm.checkers[name]
	hm.mu.RUnlock()

	if !exists {
		return HealthStatus{}, fmt.Errorf("health checker %s not found", name)
	}

	return hm.performCheck(ctx, name, checker), nil
}

// OverallHealth returns the overall health status
func (hm *DefaultHealthManager) OverallHealth(ctx context.Context) HealthStatus {
	start := time.Now()
	results := hm.CheckAll(ctx)
	duration := time.Since(start)

	overall := HealthStatus{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Duration:  duration,
		Details:   make(map[string]any),
	}

	// Aggregate results
	healthyCount := 0
	degradedCount := 0
	unhealthyCount := 0
	unknownCount := 0

	for name, status := range results {
		overall.Details[name] = status

		switch status.Status {
		case StatusHealthy:
			healthyCount++
		case StatusDegraded:
			degradedCount++
		case StatusUnhealthy:
			unhealthyCount++
		case StatusUnknown:
			unknownCount++
		}
	}

	// Determine overall status
	totalChecks := len(results)
	switch {
	case totalChecks == 0:
		overall.Status = StatusUnknown
		overall.Message = "No health checkers registered"
	case unhealthyCount > 0:
		overall.Status = StatusUnhealthy
		overall.Message = fmt.Sprintf("%d unhealthy, %d degraded, %d healthy", unhealthyCount, degradedCount, healthyCount)
	case degradedCount > 0:
		overall.Status = StatusDegraded
		overall.Message = fmt.Sprintf("%d degraded, %d healthy", degradedCount, healthyCount)
	case unknownCount > 0:
		overall.Status = StatusUnknown
		overall.Message = fmt.Sprintf("%d unknown, %d healthy", unknownCount, healthyCount)
	default:
		overall.Status = StatusHealthy
		overall.Message = fmt.Sprintf("All %d components healthy", healthyCount)
	}

	return overall
}

// ListCheckers returns the names of all registered checkers
func (hm *DefaultHealthManager) ListCheckers() []string {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	names := make([]string, 0, len(hm.checkers))
	for name := range hm.checkers {
		names = append(names, name)
	}

	return names
}

// performCheck performs a health check with caching and timeout
func (hm *DefaultHealthManager) performCheck(ctx context.Context, name string, checker HealthChecker) HealthStatus {
	// Check cache first
	if hm.cacheEnabled {
		if cached := hm.getCachedResult(name); cached != nil {
			return *cached
		}
	}

	// Create context with timeout
	checkCtx := ctx
	if hm.timeout > 0 {
		var cancel context.CancelFunc
		checkCtx, cancel = context.WithTimeout(ctx, hm.timeout)
		defer cancel()
	}

	// Perform the check
	start := time.Now()
	var status HealthStatus

	// Handle panics during health checks
	defer func() {
		if r := recover(); r != nil {
			status = HealthStatus{
				Status:    StatusUnhealthy,
				Timestamp: time.Now(),
				Duration:  time.Since(start),
				Message:   "Health check panicked",
				Error:     fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	// Run the health check
	done := make(chan HealthStatus, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- HealthStatus{
					Status:    StatusUnhealthy,
					Timestamp: time.Now(),
					Duration:  time.Since(start),
					Message:   "Health check panicked",
					Error:     fmt.Sprintf("panic: %v", r),
				}
			}
		}()
		done <- checker.Check(checkCtx)
	}()

	select {
	case status = <-done:
		// Health check completed
	case <-checkCtx.Done():
		// Health check timed out
		status = HealthStatus{
			Status:    StatusUnhealthy,
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Message:   "Health check timed out",
			Error:     checkCtx.Err().Error(),
		}
	}

	// Cache the result
	if hm.cacheEnabled {
		hm.cacheResult(name, status)
	}

	return status
}

// getCachedResult retrieves a cached health check result
func (hm *DefaultHealthManager) getCachedResult(name string) *HealthStatus {
	hm.cacheMu.RLock()
	defer hm.cacheMu.RUnlock()

	cached, exists := hm.cache[name]
	if !exists {
		return nil
	}

	// Check if cache is still valid
	if time.Since(cached.timestamp) > hm.cacheDuration {
		return nil
	}

	return &cached.status
}

// cacheResult stores a health check result in cache
func (hm *DefaultHealthManager) cacheResult(name string, status HealthStatus) {
	hm.cacheMu.Lock()
	defer hm.cacheMu.Unlock()

	hm.cache[name] = cachedResult{
		status:    status,
		timestamp: time.Now(),
	}
}

// DefaultHealthManagerConfig returns sensible defaults
func DefaultHealthManagerConfig() HealthManagerConfig {
	return HealthManagerConfig{
		Timeout:        5 * time.Second,
		ParallelChecks: true,
		CacheEnabled:   true,
		CacheDuration:  30 * time.Second,
	}
}
