package health

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/pay-theory/lift/pkg/lift/resources"
)

// PoolHealthChecker checks the health of a connection pool
type PoolHealthChecker struct {
	pool               resources.ConnectionPool
	name               string
	maxActiveThreshold float64
	errorRateThreshold float64
	minIdleThreshold   int
}

// NewPoolHealthChecker creates a new pool health checker
func NewPoolHealthChecker(name string, pool resources.ConnectionPool) *PoolHealthChecker {
	return &PoolHealthChecker{
		name:               name,
		pool:               pool,
		maxActiveThreshold: 0.8, // 80% of max active
		minIdleThreshold:   1,   // At least 1 idle connection
		errorRateThreshold: 0.1, // 10% error rate
	}
}

// Name returns the name of this health checker
func (p *PoolHealthChecker) Name() string {
	return p.name
}

// Check performs a health check on the connection pool
func (p *PoolHealthChecker) Check(ctx context.Context) HealthStatus {
	start := time.Now()

	// Get pool statistics
	stats := p.pool.Stats()

	status := HealthStatus{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Details: map[string]any{
			"active":   stats.Active,
			"idle":     stats.Idle,
			"total":    stats.Total,
			"gets":     stats.Gets,
			"puts":     stats.Puts,
			"hits":     stats.Hits,
			"misses":   stats.Misses,
			"timeouts": stats.Timeouts,
			"errors":   stats.Errors,
		},
	}

	// Check pool health
	if err := p.pool.HealthCheck(ctx); err != nil {
		status.Status = StatusUnhealthy
		status.Message = "Pool health check failed"
		status.Error = err.Error()
		return status
	}

	// Calculate error rate
	var errorRate float64
	if stats.Gets > 0 {
		errorRate = float64(stats.Errors) / float64(stats.Gets)
	}

	// Check thresholds
	issues := []string{}

	if stats.Idle < p.minIdleThreshold {
		issues = append(issues, fmt.Sprintf("low idle connections (%d < %d)", stats.Idle, p.minIdleThreshold))
	}

	if errorRate > p.errorRateThreshold {
		issues = append(issues, fmt.Sprintf("high error rate (%.2f%% > %.2f%%)", errorRate*100, p.errorRateThreshold*100))
	}

	if len(issues) > 0 {
		status.Status = StatusDegraded
		status.Message = fmt.Sprintf("Pool issues: %v", issues)
	} else {
		status.Message = "Pool is healthy"
	}

	status.Details["error_rate"] = errorRate

	return status
}

// DatabaseHealthChecker checks database connectivity
type DatabaseHealthChecker struct {
	db          *sql.DB
	name        string
	pingTimeout time.Duration
}

// NewDatabaseHealthChecker creates a new database health checker
func NewDatabaseHealthChecker(name string, db *sql.DB) *DatabaseHealthChecker {
	return &DatabaseHealthChecker{
		name:        name,
		db:          db,
		pingTimeout: 5 * time.Second,
	}
}

// Name returns the name of this health checker
func (d *DatabaseHealthChecker) Name() string {
	return d.name
}

// Check performs a health check on the database
func (d *DatabaseHealthChecker) Check(ctx context.Context) HealthStatus {
	start := time.Now()

	// Create context with timeout
	pingCtx, cancel := context.WithTimeout(ctx, d.pingTimeout)
	defer cancel()

	status := HealthStatus{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Details:   make(map[string]any),
	}

	// Ping the database
	if err := d.db.PingContext(pingCtx); err != nil {
		status.Status = StatusUnhealthy
		status.Message = "Database ping failed"
		status.Error = err.Error()
		status.Duration = time.Since(start)
		return status
	}

	// Get database stats
	dbStats := d.db.Stats()
	status.Details["open_connections"] = dbStats.OpenConnections
	status.Details["in_use"] = dbStats.InUse
	status.Details["idle"] = dbStats.Idle
	status.Details["wait_count"] = dbStats.WaitCount
	status.Details["wait_duration"] = dbStats.WaitDuration
	status.Details["max_idle_closed"] = dbStats.MaxIdleClosed
	status.Details["max_lifetime_closed"] = dbStats.MaxLifetimeClosed

	status.Duration = time.Since(start)
	status.Message = "Database is healthy"

	return status
}

// HTTPHealthChecker checks the health of an HTTP service
type HTTPHealthChecker struct {
	client         *http.Client
	name           string
	url            string
	timeout        time.Duration
	expectedStatus int
}

// NewHTTPHealthChecker creates a new HTTP health checker
func NewHTTPHealthChecker(name, url string) *HTTPHealthChecker {
	// Create secure HTTP transport with comprehensive timeouts
	transport := &http.Transport{
		// Connection timeouts (prevent slow connections)
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,  // Connection timeout
			KeepAlive: 30 * time.Second, // Keep-alive timeout
		}).DialContext,

		// TLS security
		TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,            // Verify TLS certificates
			MinVersion:         tls.VersionTLS12, // Minimum TLS 1.2
		},

		// Connection limits (prevent resource exhaustion)
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2, // Limited for health checks
		MaxConnsPerHost:     5, // Limited for health checks
		IdleConnTimeout:     30 * time.Second,

		// Response limits (prevent large response attacks)
		MaxResponseHeaderBytes: 64 * 1024, // 64KB max headers for health checks
		DisableKeepAlives:      true,      // No keep-alive for health checks
		DisableCompression:     false,

		// Response timeout (prevent slow response attacks)
		ResponseHeaderTimeout: 3 * time.Second,
	}

	// Create HTTP client with secure transport and total timeout
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second, // Total request timeout
	}

	return &HTTPHealthChecker{
		name:           name,
		url:            url,
		timeout:        10 * time.Second,
		expectedStatus: http.StatusOK,
		client:         client,
	}
}

// Name returns the name of this health checker
func (h *HTTPHealthChecker) Name() string {
	return h.name
}

// Check performs a health check on the HTTP service
func (h *HTTPHealthChecker) Check(ctx context.Context) HealthStatus {
	start := time.Now()

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", h.url, nil)
	if err != nil {
		return HealthStatus{
			Status:    StatusUnhealthy,
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Message:   "Failed to create HTTP request",
			Error:     err.Error(),
		}
	}

	// Make the request
	resp, err := h.client.Do(req)
	if err != nil {
		return HealthStatus{
			Status:    StatusUnhealthy,
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Message:   "HTTP request failed",
			Error:     err.Error(),
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Silently ignore response body close errors
			_ = err
		}
	}()

	status := HealthStatus{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Details: map[string]any{
			"url":         h.url,
			"status_code": resp.StatusCode,
			"headers":     resp.Header,
		},
	}

	// Check status code
	if resp.StatusCode != h.expectedStatus {
		status.Status = StatusUnhealthy
		status.Message = fmt.Sprintf("Unexpected status code: %d (expected %d)", resp.StatusCode, h.expectedStatus)
	} else {
		status.Message = "HTTP service is healthy"
	}

	return status
}

// MemoryHealthChecker checks system memory usage
type MemoryHealthChecker struct {
	name string

	// Thresholds (in bytes)
	warningThreshold  uint64 // Memory usage warning threshold
	criticalThreshold uint64 // Memory usage critical threshold
}

// NewMemoryHealthChecker creates a new memory health checker
func NewMemoryHealthChecker(name string) *MemoryHealthChecker {
	return &MemoryHealthChecker{
		name:              name,
		warningThreshold:  1024 * 1024 * 1024, // 1GB
		criticalThreshold: 2048 * 1024 * 1024, // 2GB
	}
}

// Name returns the name of this health checker
func (m *MemoryHealthChecker) Name() string {
	return m.name
}

// Check performs a memory health check
func (m *MemoryHealthChecker) Check(_ context.Context) HealthStatus {
	start := time.Now()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	status := HealthStatus{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Details: map[string]any{
			"alloc":           memStats.Alloc,
			"total_alloc":     memStats.TotalAlloc,
			"sys":             memStats.Sys,
			"num_gc":          memStats.NumGC,
			"gc_cpu_fraction": memStats.GCCPUFraction,
			"heap_alloc":      memStats.HeapAlloc,
			"heap_sys":        memStats.HeapSys,
			"heap_idle":       memStats.HeapIdle,
			"heap_inuse":      memStats.HeapInuse,
		},
	}

	// Check memory thresholds
	switch {
	case memStats.Alloc > m.criticalThreshold:
		status.Status = StatusUnhealthy
		status.Message = fmt.Sprintf("Critical memory usage: %d bytes (> %d)", memStats.Alloc, m.criticalThreshold)
	case memStats.Alloc > m.warningThreshold:
		status.Status = StatusDegraded
		status.Message = fmt.Sprintf("High memory usage: %d bytes (> %d)", memStats.Alloc, m.warningThreshold)
	default:
		status.Message = fmt.Sprintf("Memory usage is healthy: %d bytes", memStats.Alloc)
	}

	return status
}

// CustomHealthChecker allows for custom health check functions
type CustomHealthChecker struct {
	checkFn func(ctx context.Context) HealthStatus // function pointer (8 bytes)
	name    string                                 // 16 bytes
}

// NewCustomHealthChecker creates a new custom health checker
func NewCustomHealthChecker(name string, checkFn func(ctx context.Context) HealthStatus) *CustomHealthChecker {
	return &CustomHealthChecker{
		name:    name,
		checkFn: checkFn,
	}
}

// Name returns the name of this health checker
func (c *CustomHealthChecker) Name() string {
	return c.name
}

// Check performs the custom health check
func (c *CustomHealthChecker) Check(ctx context.Context) HealthStatus {
	return c.checkFn(ctx)
}

// AlwaysHealthyChecker always returns healthy status (useful for testing)
type AlwaysHealthyChecker struct {
	name string
}

// NewAlwaysHealthyChecker creates a checker that always returns healthy
func NewAlwaysHealthyChecker(name string) *AlwaysHealthyChecker {
	return &AlwaysHealthyChecker{name: name}
}

// Name returns the name of this health checker
func (a *AlwaysHealthyChecker) Name() string {
	return a.name
}

// Check always returns healthy status
func (a *AlwaysHealthyChecker) Check(_ context.Context) HealthStatus {
	return HealthStatus{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Duration:  time.Microsecond, // Very fast
		Message:   "Always healthy",
	}
}

// AlwaysUnhealthyChecker always returns unhealthy status (useful for testing)
type AlwaysUnhealthyChecker struct {
	name string
}

// NewAlwaysUnhealthyChecker creates a checker that always returns unhealthy
func NewAlwaysUnhealthyChecker(name string) *AlwaysUnhealthyChecker {
	return &AlwaysUnhealthyChecker{name: name}
}

// Name returns the name of this health checker
func (a *AlwaysUnhealthyChecker) Name() string {
	return a.name
}

// Check always returns unhealthy status
func (a *AlwaysUnhealthyChecker) Check(_ context.Context) HealthStatus {
	return HealthStatus{
		Status:    StatusUnhealthy,
		Timestamp: time.Now(),
		Duration:  time.Microsecond,
		Message:   "Always unhealthy",
		Error:     "This checker always fails",
	}
}
