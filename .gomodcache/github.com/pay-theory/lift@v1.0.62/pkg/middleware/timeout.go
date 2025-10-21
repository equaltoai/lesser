package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

// TimeoutConfig holds configuration for request timeouts
type TimeoutConfig struct {
	Logger               observability.StructuredLogger    `json:"-"`
	Metrics              observability.MetricsCollector    `json:"-"`
	OperationTimeouts    map[string]time.Duration          `json:"operation_timeouts"`
	TenantTimeouts       map[string]time.Duration          `json:"tenant_timeouts"`
	TimeoutCalculator    func(*lift.Context) time.Duration `json:"-"`
	TimeoutHandler       func(*lift.Context) error         `json:"-"`
	TimeoutMessage       string                            `json:"timeout_message"`
	Name                 string                            `json:"name"`
	DefaultTimeout       time.Duration                     `json:"default_timeout"`
	ReadTimeout          time.Duration                     `json:"read_timeout"`
	WriteTimeout         time.Duration                     `json:"write_timeout"`
	IdleTimeout          time.Duration                     `json:"idle_timeout"`
	ShutdownTimeout      time.Duration                     `json:"shutdown_timeout"`
	TimeoutStatusCode    int                               `json:"timeout_status_code"`
	EnableDynamicTimeout bool                              `json:"enable_dynamic_timeout"`
	GracefulShutdown     bool                              `json:"graceful_shutdown"`
	EnableMetrics        bool                              `json:"enable_metrics"`
}

// TimeoutStats provides statistics about timeout performance
type TimeoutStats struct {
	Name            string        `json:"name"`
	TotalRequests   int64         `json:"total_requests"`
	TimeoutRequests int64         `json:"timeout_requests"`
	TimeoutRatio    float64       `json:"timeout_ratio"`
	AverageTimeout  time.Duration `json:"average_timeout"`
	MaxTimeout      time.Duration `json:"max_timeout"`
	MinTimeout      time.Duration `json:"min_timeout"`
	AverageDuration time.Duration `json:"average_duration"`
}

// TimeoutMiddleware creates a timeout middleware
func TimeoutMiddleware(config TimeoutConfig) lift.Middleware {
	// Set defaults
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 30 * time.Second
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 10 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 10 * time.Second
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 60 * time.Second
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 30 * time.Second
	}
	if config.TimeoutStatusCode == 0 {
		config.TimeoutStatusCode = 408 // Request Timeout
	}
	if config.TimeoutMessage == "" {
		config.TimeoutMessage = "Request timeout"
	}
	if config.Name == "" {
		config.Name = defaultName
	}
	if config.TimeoutHandler == nil {
		config.TimeoutHandler = defaultTimeoutHandler(config.TimeoutStatusCode, config.TimeoutMessage)
	}

	manager := &timeoutManager{
		config: config,
		stats: &TimeoutStats{
			Name:       config.Name,
			MinTimeout: config.DefaultTimeout,
			MaxTimeout: config.DefaultTimeout,
		},
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()

			// Calculate timeout for this request
			timeout := manager.calculateTimeout(ctx)

			// Create timeout context
			timeoutCtx, cancel := context.WithTimeout(ctx.Context, timeout)
			defer cancel()

			// Update context
			originalCtx := ctx.Context
			ctx.Context = timeoutCtx

			// Channel to receive result
			done := make(chan error, 1)

			// Execute handler in goroutine
			go func() {
				defer func() {
					// Recover from panics
					if r := recover(); r != nil {
						done <- fmt.Errorf("panic in handler: %v", r)
					}
				}()

				err := next.Handle(ctx)
				done <- err
			}()

			// Wait for completion or timeout
			select {
			case err := <-done:
				// Request completed
				duration := time.Since(start)

				// Restore original context
				ctx.Context = originalCtx

				// Record success metrics
				manager.recordSuccess(ctx, timeout, duration)

				if config.Logger != nil {
					config.Logger.Debug("Request completed within timeout", map[string]any{
						"timeout_name": config.Name,
						"timeout":      timeout.String(),
						"duration":     duration.String(),
						"tenant_id":    ctx.TenantID(),
						"operation":    fmt.Sprintf("%s:%s", ctx.Request.Method, ctx.Request.Path),
					})
				}

				return err

			case <-timeoutCtx.Done():
				// Request timed out
				duration := time.Since(start)

				// Restore original context
				ctx.Context = originalCtx

				// Record timeout metrics
				manager.recordTimeout(ctx, timeout, duration)

				if config.Logger != nil {
					config.Logger.Warn("Request timed out", map[string]any{
						"timeout_name": config.Name,
						"timeout":      timeout.String(),
						"duration":     duration.String(),
						"tenant_id":    ctx.TenantID(),
						"operation":    fmt.Sprintf("%s:%s", ctx.Request.Method, ctx.Request.Path),
					})
				}

				return config.TimeoutHandler(ctx)
			}
		})
	}
}

// timeoutManager manages timeout logic and statistics
// Memory optimized: 160 → 136 bytes (24 bytes saved)
type timeoutManager struct {
	stats  *TimeoutStats
	config TimeoutConfig
	mutex  sync.RWMutex
}

// calculateTimeout determines the timeout for a specific request
func (tm *timeoutManager) calculateTimeout(ctx *lift.Context) time.Duration {
	// Check for custom timeout calculator
	if tm.config.EnableDynamicTimeout && tm.config.TimeoutCalculator != nil {
		return tm.config.TimeoutCalculator(ctx)
	}

	// Check tenant-specific timeout
	if tenantID := ctx.TenantID(); tenantID != "" {
		if timeout, exists := tm.config.TenantTimeouts[tenantID]; exists {
			return timeout
		}
	}

	// Check operation-specific timeout
	operation := fmt.Sprintf("%s:%s", ctx.Request.Method, ctx.Request.Path)
	if timeout, exists := tm.config.OperationTimeouts[operation]; exists {
		return timeout
	}

	// Use default timeout
	return tm.config.DefaultTimeout
}

// recordSuccess records metrics for successful requests
func (tm *timeoutManager) recordSuccess(ctx *lift.Context, timeout, duration time.Duration) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.stats.TotalRequests++

	// Update timeout statistics
	tm.updateTimeoutStats(timeout, duration)

	// Update timeout ratio
	tm.stats.TimeoutRatio = float64(tm.stats.TimeoutRequests) / float64(tm.stats.TotalRequests)

	// Record metrics
	if tm.config.EnableMetrics && tm.config.Metrics != nil {
		tm.recordMetrics(ctx, "success", timeout, duration)
	}
}

// recordTimeout records metrics for timed out requests
func (tm *timeoutManager) recordTimeout(ctx *lift.Context, timeout, duration time.Duration) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.stats.TotalRequests++
	tm.stats.TimeoutRequests++

	// Update timeout statistics
	tm.updateTimeoutStats(timeout, duration)

	// Update timeout ratio
	tm.stats.TimeoutRatio = float64(tm.stats.TimeoutRequests) / float64(tm.stats.TotalRequests)

	// Record metrics
	if tm.config.EnableMetrics && tm.config.Metrics != nil {
		tm.recordMetrics(ctx, "timeout", timeout, duration)
	}
}

// updateTimeoutStats updates min, max, and average timeout statistics
func (tm *timeoutManager) updateTimeoutStats(timeout, duration time.Duration) {
	if timeout < tm.stats.MinTimeout {
		tm.stats.MinTimeout = timeout
	}
	if timeout > tm.stats.MaxTimeout {
		tm.stats.MaxTimeout = timeout
	}

	// Update average timeout
	if tm.stats.TotalRequests == 1 {
		tm.stats.AverageTimeout = timeout
		tm.stats.AverageDuration = duration
	} else {
		// Running average
		tm.stats.AverageTimeout = time.Duration(
			(int64(tm.stats.AverageTimeout)*(tm.stats.TotalRequests-1) + int64(timeout)) / tm.stats.TotalRequests,
		)
		tm.stats.AverageDuration = time.Duration(
			(int64(tm.stats.AverageDuration)*(tm.stats.TotalRequests-1) + int64(duration)) / tm.stats.TotalRequests,
		)
	}
}

// recordMetrics records observability metrics
func (tm *timeoutManager) recordMetrics(ctx *lift.Context, result string, timeout, duration time.Duration) {
	tenantID := ctx.TenantID()
	operation := fmt.Sprintf("%s:%s", ctx.Request.Method, ctx.Request.Path)

	tags := map[string]string{
		"timeout_name": tm.config.Name,
		"result":       result,
		"operation":    operation,
	}

	if tenantID != "" {
		tags["tenant_id"] = tenantID
	}

	metrics := tm.config.Metrics.WithTags(tags)

	// Record request
	counter := metrics.Counter("timeout.requests.total")
	counter.Inc()

	// Record timeout value
	histogram := metrics.Histogram("timeout.configured")
	histogram.Observe(float64(timeout.Milliseconds()))

	// Record actual duration
	durationHistogram := metrics.Histogram("timeout.duration")
	durationHistogram.Observe(float64(duration.Milliseconds()))

	// Record timeout ratio
	gauge := metrics.Gauge("timeout.ratio")
	gauge.Set(tm.stats.TimeoutRatio)
}

// GetStats returns current timeout statistics
func (tm *timeoutManager) GetStats() TimeoutStats {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	return *tm.stats
}

// Default implementations

// defaultTimeoutHandler creates a default timeout response handler
func defaultTimeoutHandler(statusCode int, message string) func(*lift.Context) error {
	return func(ctx *lift.Context) error {
		return ctx.Status(statusCode).JSON(map[string]any{
			"error":   "Request Timeout",
			"message": message,
			"code":    "TIMEOUT",
		})
	}
}

// Utility functions for common timeout configurations

// NewBasicTimeout creates a basic timeout configuration
func NewBasicTimeout(name string, defaultTimeout time.Duration) TimeoutConfig {
	return TimeoutConfig{
		Name:              name,
		DefaultTimeout:    defaultTimeout,
		ReadTimeout:       defaultTimeout / 3,
		WriteTimeout:      defaultTimeout / 3,
		IdleTimeout:       defaultTimeout * 2,
		GracefulShutdown:  true,
		ShutdownTimeout:   30 * time.Second,
		TimeoutStatusCode: 408,
		TimeoutMessage:    "Request timeout",
		EnableMetrics:     true,
	}
}

// NewOperationTimeout creates a timeout configuration with per-operation timeouts
func NewOperationTimeout(name string, defaultTimeout time.Duration, operationTimeouts map[string]time.Duration) TimeoutConfig {
	config := NewBasicTimeout(name, defaultTimeout)
	config.OperationTimeouts = operationTimeouts
	return config
}

// NewTenantTimeout creates a timeout configuration with per-tenant timeouts
func NewTenantTimeout(name string, defaultTimeout time.Duration, tenantTimeouts map[string]time.Duration) TimeoutConfig {
	config := NewBasicTimeout(name, defaultTimeout)
	config.TenantTimeouts = tenantTimeouts
	return config
}

// NewDynamicTimeout creates a timeout configuration with dynamic timeout calculation
func NewDynamicTimeout(name string, defaultTimeout time.Duration, calculator func(*lift.Context) time.Duration) TimeoutConfig {
	config := NewBasicTimeout(name, defaultTimeout)
	config.EnableDynamicTimeout = true
	config.TimeoutCalculator = calculator
	return config
}

// Common timeout calculators

// AdaptiveTimeoutCalculator creates a timeout calculator that adapts based on request complexity
func AdaptiveTimeoutCalculator(baseTimeout time.Duration) func(*lift.Context) time.Duration {
	return func(ctx *lift.Context) time.Duration {
		timeout := baseTimeout

		// Adjust based on request method
		switch ctx.Request.Method {
		case "GET":
			timeout /= 2 // GET requests should be faster
		case "POST", "PUT":
			timeout *= 2 // Write operations may take longer
		case "DELETE":
			timeout *= 3 // Delete operations may be complex
		}

		// Adjust based on query parameters (more params = more complex)
		if len(ctx.Request.QueryParams) > 5 {
			timeout *= 2
		}

		// Adjust based on body size
		if len(ctx.Request.Body) > 1024*1024 { // > 1MB
			timeout *= 3
		}

		return timeout
	}
}

// PriorityTimeoutCalculator creates a timeout calculator based on request priority
func PriorityTimeoutCalculator(baseTimeout time.Duration) func(*lift.Context) time.Duration {
	return func(ctx *lift.Context) time.Duration {
		priority := ctx.Request.Headers["X-Priority"]

		switch priority {
		case "critical":
			return baseTimeout * 5 // Critical requests get more time
		case "high":
			return baseTimeout * 2
		case "normal":
			return baseTimeout
		case "low":
			return baseTimeout / 2
		case "background":
			return baseTimeout / 4
		default:
			return baseTimeout
		}
	}
}

// LoadBasedTimeoutCalculator creates a timeout calculator that adjusts based on system load
func LoadBasedTimeoutCalculator(baseTimeout time.Duration, loadMetrics *LoadMetrics) func(*lift.Context) time.Duration {
	return func(_ *lift.Context) time.Duration {
		if loadMetrics == nil {
			return baseTimeout
		}

		// Adjust timeout based on current system load
		loadFactor := 1.0

		// CPU load adjustment
		if loadMetrics.CPUUsage > 0.8 {
			loadFactor += 0.5 // 50% more time under high CPU load
		}

		// Memory load adjustment
		if loadMetrics.MemoryUsage > 0.8 {
			loadFactor += 0.3 // 30% more time under high memory load
		}

		// Active requests adjustment
		if loadMetrics.ActiveRequests > 100 {
			loadFactor += 0.2 // 20% more time under high request load
		}

		// Error rate adjustment
		if loadMetrics.ErrorRate > 0.1 {
			loadFactor += 0.4 // 40% more time when error rate is high
		}

		adjustedTimeout := time.Duration(float64(baseTimeout) * loadFactor)

		// Cap the timeout to prevent excessive delays
		maxTimeout := baseTimeout * 5
		if adjustedTimeout > maxTimeout {
			adjustedTimeout = maxTimeout
		}

		return adjustedTimeout
	}
}
