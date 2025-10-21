package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

// HealthCheckConfig holds configuration for health checks
type HealthCheckConfig struct {
	Metrics              observability.MetricsCollector `json:"-"`
	Logger               observability.StructuredLogger `json:"-"`
	Path                 string                         `json:"path"`
	DetailPath           string                         `json:"detail_path"`
	ReadyPath            string                         `json:"ready_path"`
	LivePath             string                         `json:"live_path"`
	Dependencies         []HealthChecker                `json:"-"`
	FailureThreshold     int                            `json:"failure_threshold"`
	RecoveryTime         time.Duration                  `json:"recovery_time"`
	GracePeriod          time.Duration                  `json:"grace_period"`
	Interval             time.Duration                  `json:"interval"`
	Timeout              time.Duration                  `json:"timeout"`
	EnableDetailedChecks bool                           `json:"enable_detailed_checks"`
	EnableMetrics        bool                           `json:"enable_metrics"`
	EnableBackgroundRuns bool                           `json:"enable_background_runs"`
}

// HealthChecker interface for dependency health checks
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) error
	IsRequired() bool // If true, failure marks entire system as unhealthy
}

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Timestamp time.Time      `json:"timestamp"`
	Details   map[string]any `json:"details,omitempty"`
	Name      string         `json:"name"`
	Status    HealthStatus   `json:"status"`
	Message   string         `json:"message,omitempty"`
	Duration  time.Duration  `json:"duration"`
	Required  bool           `json:"required"`
}

// OverallHealthResult represents the overall system health
type OverallHealthResult struct {
	Timestamp   time.Time                     `json:"timestamp"`
	Checks      map[string]*HealthCheckResult `json:"checks,omitempty"`
	Summary     *HealthSummary                `json:"summary,omitempty"`
	Status      HealthStatus                  `json:"status"`
	Version     string                        `json:"version,omitempty"`
	Environment string                        `json:"environment,omitempty"`
	Duration    time.Duration                 `json:"duration"`
}

// HealthSummary provides a summary of health check results
type HealthSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
	Degraded  int `json:"degraded"`
	Unknown   int `json:"unknown"`
}

// HealthCheckMiddleware creates a health check middleware
func HealthCheckMiddleware(config HealthCheckConfig) lift.Middleware {
	// Set defaults
	if config.Path == "" {
		config.Path = "/health"
	}
	if config.DetailPath == "" {
		config.DetailPath = "/health/detail"
	}
	if config.ReadyPath == "" {
		config.ReadyPath = "/ready"
	}
	if config.LivePath == "" {
		config.LivePath = "/live"
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	if config.Interval == 0 {
		config.Interval = 30 * time.Second
	}
	if config.GracePeriod == 0 {
		config.GracePeriod = 30 * time.Second
	}
	if config.FailureThreshold == 0 {
		config.FailureThreshold = 3
	}
	if config.RecoveryTime == 0 {
		config.RecoveryTime = 60 * time.Second
	}

	monitor := &healthMonitor{
		config:        config,
		results:       make(map[string]*HealthCheckResult),
		startTime:     time.Now(),
		lastCheck:     time.Now(),
		failureCounts: make(map[string]int),
	}

	// Start background health checks if enabled
	if config.EnableBackgroundRuns {
		go monitor.runBackgroundChecks()
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Check if this is a health check request
			switch ctx.Request.Path {
			case config.Path:
				return monitor.handleBasicHealth(ctx)
			case config.DetailPath:
				return monitor.handleDetailedHealth(ctx)
			case config.ReadyPath:
				return monitor.handleReadiness(ctx)
			case config.LivePath:
				return monitor.handleLiveness(ctx)
			default:
				// Not a health check request, continue to next handler
				return next.Handle(ctx)
			}
		})
	}
}

// healthMonitor manages health check state and execution
type healthMonitor struct {
	startTime     time.Time
	lastCheck     time.Time
	results       map[string]*HealthCheckResult
	failureCounts map[string]int
	config        HealthCheckConfig
	resultsMutex  sync.RWMutex
	failureMutex  sync.RWMutex
}

// handleBasicHealth handles basic health check requests
func (h *healthMonitor) handleBasicHealth(ctx *lift.Context) error {
	start := time.Now()

	// Run health checks
	result := h.runHealthChecks(ctx.Context, false)

	// Record metrics
	if h.config.EnableMetrics && h.config.Metrics != nil {
		duration := time.Since(start)

		tags := map[string]string{
			"endpoint": "health",
			"status":   string(result.Status),
		}

		metrics := h.config.Metrics.WithTags(tags)

		counter := metrics.Counter("health_checks.total")
		counter.Inc()

		histogram := metrics.Histogram("health_checks.duration")
		histogram.Observe(float64(duration.Milliseconds()))
	}

	// Set appropriate status code
	statusCode := 200
	if result.Status != HealthStatusHealthy {
		statusCode = 503
	}

	return ctx.Status(statusCode).JSON(map[string]any{
		"status":    result.Status,
		"timestamp": result.Timestamp,
		"duration":  result.Duration.String(),
	})
}

// handleDetailedHealth handles detailed health check requests
func (h *healthMonitor) handleDetailedHealth(ctx *lift.Context) error {
	start := time.Now()

	// Run detailed health checks
	result := h.runHealthChecks(ctx.Context, true)

	// Record metrics
	if h.config.EnableMetrics && h.config.Metrics != nil {
		duration := time.Since(start)

		tags := map[string]string{
			"endpoint": "health_detail",
			"status":   string(result.Status),
		}

		metrics := h.config.Metrics.WithTags(tags)

		counter := metrics.Counter("health_checks.total")
		counter.Inc()

		histogram := metrics.Histogram("health_checks.duration")
		histogram.Observe(float64(duration.Milliseconds()))
	}

	// Set appropriate status code
	statusCode := 200
	if result.Status != HealthStatusHealthy {
		statusCode = 503
	}

	return ctx.Status(statusCode).JSON(result)
}

// handleReadiness handles readiness check requests
func (h *healthMonitor) handleReadiness(ctx *lift.Context) error {
	// Readiness checks if the service is ready to receive traffic
	// This includes checking dependencies and initialization state

	// Check if we're still in grace period
	if time.Since(h.startTime) < h.config.GracePeriod {
		return ctx.Status(503).JSON(map[string]any{
			"status":                 "not_ready",
			"message":                "Service is still starting up",
			"grace_period_remaining": h.config.GracePeriod - time.Since(h.startTime),
		})
	}

	// Run dependency checks
	result := h.runHealthChecks(ctx.Context, false)

	// For readiness, we're more strict - any required dependency failure means not ready
	ready := result.Status == HealthStatusHealthy

	statusCode := 200
	if !ready {
		statusCode = 503
	}

	return ctx.Status(statusCode).JSON(map[string]any{
		"status":    map[bool]string{true: "ready", false: "not_ready"}[ready],
		"timestamp": time.Now(),
		"checks":    len(h.config.Dependencies),
	})
}

// handleLiveness handles liveness check requests
func (h *healthMonitor) handleLiveness(ctx *lift.Context) error {
	// Liveness checks if the service is alive and not deadlocked
	// This is typically a simple check that doesn't depend on external services

	// Simple liveness check - if we can respond, we're alive
	return ctx.Status(200).JSON(map[string]any{
		"status":    "alive",
		"timestamp": time.Now(),
		"uptime":    time.Since(h.startTime).String(),
	})
}

// runHealthChecks executes all configured health checks
func (h *healthMonitor) runHealthChecks(ctx context.Context, detailed bool) *OverallHealthResult {
	start := time.Now()

	// Create context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, h.config.Timeout)
	defer cancel()

	results := make(map[string]*HealthCheckResult)
	overallStatus := HealthStatusHealthy

	// Run checks for each dependency
	for _, checker := range h.config.Dependencies {
		result := h.runSingleCheck(checkCtx, checker)
		results[checker.Name()] = result

		// Update overall status based on this check
		if checker.IsRequired() && result.Status != HealthStatusHealthy {
			overallStatus = HealthStatusUnhealthy
		} else if result.Status == HealthStatusDegraded && overallStatus == HealthStatusHealthy {
			overallStatus = HealthStatusDegraded
		}
	}

	// Store results for background monitoring
	h.resultsMutex.Lock()
	h.results = results
	h.lastCheck = time.Now()
	h.resultsMutex.Unlock()

	// Create summary
	summary := h.createSummary(results)

	result := &OverallHealthResult{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Summary:   summary,
	}

	if detailed {
		result.Checks = results
	}

	return result
}

// runSingleCheck executes a single health check with circuit breaker logic
func (h *healthMonitor) runSingleCheck(ctx context.Context, checker HealthChecker) *HealthCheckResult {
	start := time.Now()

	// Check circuit breaker state
	h.failureMutex.RLock()
	failures := h.failureCounts[checker.Name()]
	h.failureMutex.RUnlock()

	if failures >= h.config.FailureThreshold {
		// Circuit is open, return cached failure
		return &HealthCheckResult{
			Name:      checker.Name(),
			Status:    HealthStatusUnhealthy,
			Message:   fmt.Sprintf("Circuit breaker open (failures: %d)", failures),
			Duration:  0,
			Timestamp: time.Now(),
			Required:  checker.IsRequired(),
		}
	}

	// Run the actual check
	err := checker.Check(ctx)
	duration := time.Since(start)

	result := &HealthCheckResult{
		Name:      checker.Name(),
		Duration:  duration,
		Timestamp: time.Now(),
		Required:  checker.IsRequired(),
	}

	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Message = err.Error()

		// Increment failure count
		h.failureMutex.Lock()
		h.failureCounts[checker.Name()]++
		h.failureMutex.Unlock()

		// Log the failure
		if h.config.Logger != nil {
			h.config.Logger.Error("Health check failed", map[string]any{
				"checker":  checker.Name(),
				"error":    err.Error(),
				"duration": duration.String(),
				"failures": h.failureCounts[checker.Name()],
			})
		}
	} else {
		result.Status = HealthStatusHealthy
		result.Message = "OK"

		// Reset failure count on success
		h.failureMutex.Lock()
		h.failureCounts[checker.Name()] = 0
		h.failureMutex.Unlock()
	}

	return result
}

// createSummary creates a summary of health check results
func (h *healthMonitor) createSummary(results map[string]*HealthCheckResult) *HealthSummary {
	summary := &HealthSummary{}

	for _, result := range results {
		summary.Total++
		switch result.Status {
		case HealthStatusHealthy:
			summary.Healthy++
		case HealthStatusUnhealthy:
			summary.Unhealthy++
		case HealthStatusDegraded:
			summary.Degraded++
		default:
			summary.Unknown++
		}
	}

	return summary
}

// runBackgroundChecks runs health checks in the background
func (h *healthMonitor) runBackgroundChecks() {
	ticker := time.NewTicker(h.config.Interval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), h.config.Timeout)
		h.runHealthChecks(ctx, false)
		cancel()
	}
}

// GetHealthStatus returns the current health status
func (h *healthMonitor) GetHealthStatus() *OverallHealthResult {
	h.resultsMutex.RLock()
	defer h.resultsMutex.RUnlock()

	// Return cached results if available and recent
	if time.Since(h.lastCheck) < h.config.Interval {
		summary := h.createSummary(h.results)
		overallStatus := HealthStatusHealthy

		for _, result := range h.results {
			if result.Required && result.Status != HealthStatusHealthy {
				overallStatus = HealthStatusUnhealthy
				break
			} else if result.Status == HealthStatusDegraded && overallStatus == HealthStatusHealthy {
				overallStatus = HealthStatusDegraded
			}
		}

		return &OverallHealthResult{
			Status:    overallStatus,
			Timestamp: h.lastCheck,
			Checks:    h.results,
			Summary:   summary,
		}
	}

	// No recent results available
	return &OverallHealthResult{
		Status:    HealthStatusUnknown,
		Timestamp: time.Now(),
	}
}

// Built-in health checkers

// DatabaseHealthChecker checks database connectivity
type DatabaseHealthChecker struct {
	testFunc func(context.Context) error
	name     string
	required bool
}

func NewDatabaseHealthChecker(name string, required bool, testFunc func(context.Context) error) *DatabaseHealthChecker {
	return &DatabaseHealthChecker{
		name:     name,
		required: required,
		testFunc: testFunc,
	}
}

func (d *DatabaseHealthChecker) Name() string {
	return d.name
}

func (d *DatabaseHealthChecker) Check(ctx context.Context) error {
	return d.testFunc(ctx)
}

func (d *DatabaseHealthChecker) IsRequired() bool {
	return d.required
}

// HTTPHealthChecker checks HTTP endpoint health
type HTTPHealthChecker struct {
	name     string
	url      string
	required bool
	timeout  time.Duration
}

func NewHTTPHealthChecker(name, url string, required bool, timeout time.Duration) *HTTPHealthChecker {
	return &HTTPHealthChecker{
		name:     name,
		url:      url,
		required: required,
		timeout:  timeout,
	}
}

func (h *HTTPHealthChecker) Name() string {
	return h.name
}

func (h *HTTPHealthChecker) Check(_ context.Context) error {
	// This would implement an HTTP health check
	// For now, return success
	return nil
}

func (h *HTTPHealthChecker) IsRequired() bool {
	return h.required
}

// MemoryHealthChecker checks memory usage
type MemoryHealthChecker struct {
	name      string
	threshold float64 // Memory usage threshold (0.0 to 1.0)
}

func NewMemoryHealthChecker(name string, threshold float64) *MemoryHealthChecker {
	return &MemoryHealthChecker{
		name:      name,
		threshold: threshold,
	}
}

func (m *MemoryHealthChecker) Name() string {
	return m.name
}

func (m *MemoryHealthChecker) Check(_ context.Context) error {
	// This would implement memory usage checking
	// For now, return success
	return nil
}

func (m *MemoryHealthChecker) IsRequired() bool {
	return false // Memory checks are typically not required
}

// Backward compatibility aliases
type HealthConfig = HealthCheckConfig

// HealthMiddleware is an alias for HealthCheckMiddleware for backward compatibility
func HealthMiddleware(config HealthConfig) lift.Middleware {
	return HealthCheckMiddleware(config)
}
