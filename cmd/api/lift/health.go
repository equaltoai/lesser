package lift

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	lconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/version"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HealthStatus represents the overall health status
type HealthStatus string

// HealthStatus values
const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheckResponse represents the health check response
type HealthCheckResponse struct {
	Status    HealthStatus           `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Version   string                 `json:"version,omitempty"`
	BuildInfo map[string]string      `json:"build_info,omitempty"`
	Checks    map[string]CheckResult `json:"checks,omitempty"`
	Uptime    time.Duration          `json:"uptime,omitempty"`
}

// CheckResult represents individual health check result
type CheckResult struct {
	Status   HealthStatus  `json:"status"`
	Message  string        `json:"message,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
	Error    string        `json:"error,omitempty"`
	Details  interface{}   `json:"details,omitempty"`
}

// HealthChecker handles health check operations
type HealthChecker struct {
	logger      *zap.Logger
	repos       core.RepositoryStorage
	httpClient  *http.Client
	startTime   time.Time
	version     string
	environment string
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(logger *zap.Logger, repos core.RepositoryStorage) *HealthChecker {
	return &HealthChecker{
		logger:      logger,
		repos:       repos,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		startTime:   time.Now(),
		version:     version.GetVersion(),
		environment: lconfig.GetEnvironment(),
	}
}

func (h *HealthChecker) httpClientOrDefault() *http.Client {
	if h != nil && h.httpClient != nil {
		return h.httpClient
	}
	return &http.Client{Timeout: 5 * time.Second}
}

// HandleLivenessCheck handles GET /health/live - basic liveness check
func (h *HealthChecker) HandleLivenessCheck(c *lift.Context) error {
	response := HealthCheckResponse{
		Status:    HealthStatusHealthy,
		Timestamp: time.Now(),
		Version:   h.version,
		BuildInfo: version.GetBuildInfo(),
		Uptime:    time.Since(h.startTime),
	}

	return c.Status(http.StatusOK).JSON(response)
}

// HandleReadinessCheck handles GET /health/ready - full readiness check
func (h *HealthChecker) HandleReadinessCheck(c *lift.Context) error {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	checks := make(map[string]CheckResult)
	overallStatus := HealthStatusHealthy

	// Run all health checks
	h.checkDatabase(ctx, checks)
	h.checkS3Storage(ctx, checks)
	h.checkSecrets(ctx, checks)
	h.checkMemory(checks)
	h.checkDiskSpace(checks)

	// Determine overall status
	for _, check := range checks {
		if check.Status == HealthStatusUnhealthy {
			overallStatus = HealthStatusUnhealthy
			break
		} else if check.Status == HealthStatusDegraded {
			overallStatus = HealthStatusDegraded
		}
	}

	response := HealthCheckResponse{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Version:   h.version,
		BuildInfo: version.GetBuildInfo(),
		Uptime:    time.Since(h.startTime),
		Checks:    checks,
	}

	statusCode := http.StatusOK
	switch overallStatus {
	case HealthStatusUnhealthy:
		statusCode = http.StatusServiceUnavailable
	case HealthStatusDegraded:
		statusCode = http.StatusOK // Still serve traffic but indicate degraded state
	}

	return c.Status(statusCode).JSON(response)
}

// HandleDetailedHealthCheck handles GET /health/detailed - comprehensive health check
func (h *HealthChecker) HandleDetailedHealthCheck(c *lift.Context) error {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	checks := make(map[string]CheckResult)
	overallStatus := HealthStatusHealthy

	// Run all health checks with detailed information
	h.checkDatabase(ctx, checks)
	h.checkS3Storage(ctx, checks)
	h.checkSecrets(ctx, checks)
	h.checkMemory(checks)
	h.checkDiskSpace(checks)
	h.checkConfiguration(ctx, checks)
	h.checkExternalDependencies(ctx, checks)

	// Determine overall status
	for _, check := range checks {
		if check.Status == HealthStatusUnhealthy {
			overallStatus = HealthStatusUnhealthy
			break
		} else if check.Status == HealthStatusDegraded {
			overallStatus = HealthStatusDegraded
		}
	}

	response := HealthCheckResponse{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Version:   h.version,
		BuildInfo: version.GetBuildInfo(),
		Uptime:    time.Since(h.startTime),
		Checks:    checks,
	}

	statusCode := http.StatusOK
	if overallStatus == HealthStatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}

	return c.Status(statusCode).JSON(response)
}

// checkDatabase checks database connectivity and health
func (h *HealthChecker) checkDatabase(ctx context.Context, checks map[string]CheckResult) {
	start := time.Now()

	// If we have repos interface, try a basic read operation
	if h.repos != nil {
		// Test basic read operation with a non-existent user (expected behavior)
		_, err := h.repos.Account().GetUser(ctx, "health-check-nonexistent-user")

		duration := time.Since(start)

		// "user not found" is expected and indicates DB is responding properly
		if err != nil && strings.Contains(err.Error(), "not found") {
			checks["database"] = CheckResult{
				Status:   HealthStatusHealthy,
				Message:  "Database responding correctly",
				Duration: duration,
				Details: map[string]interface{}{
					"type": "dynamodb",
					"test": "user_lookup_response",
				},
			}
			return
		}

		// Real errors indicate connectivity issues
		if err != nil {
			checks["database"] = CheckResult{
				Status:   HealthStatusUnhealthy,
				Message:  "Database connectivity issue",
				Duration: duration,
				Error:    err.Error(),
				Details: map[string]interface{}{
					"type": "dynamodb",
					"test": "user_lookup_failed",
				},
			}
			return
		}

		// Unexpected success (should not find non-existent user)
		checks["database"] = CheckResult{
			Status:   HealthStatusHealthy,
			Message:  "Database responding correctly",
			Duration: duration,
			Details: map[string]interface{}{
				"type": "dynamodb",
				"test": "user_lookup_success",
			},
		}
	} else {
		// Quick validation without full AWS resource checks as fallback
		err := lconfig.QuickValidateProductionConfig()
		duration := time.Since(start)

		if err != nil {
			checks["database"] = CheckResult{
				Status:   HealthStatusUnhealthy,
				Message:  "Database configuration validation failed",
				Duration: duration,
				Error:    err.Error(),
			}
			return
		}

		checks["database"] = CheckResult{
			Status:   HealthStatusDegraded,
			Message:  "Database configuration valid but repository interface not available",
			Duration: duration,
		}
	}
}

// checkS3Storage checks S3 storage availability
func (h *HealthChecker) checkS3Storage(ctx context.Context, checks map[string]CheckResult) {
	start := time.Now()

	// Get S3 bucket name from Lesser config/env (supports all bucket env aliases)
	bucketName := strings.TrimSpace(lconfig.GetS3Bucket())
	if bucketName == "" {
		checks["s3_storage"] = CheckResult{
			Status:   HealthStatusDegraded,
			Message:  "S3 storage not configured (optional)",
			Duration: time.Since(start),
			Details: map[string]interface{}{
				"note": "S3 bucket env not configured",
			},
		}
		return
	}

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithHTTPClient(h.httpClientOrDefault()))
	if err != nil {
		checks["s3_storage"] = CheckResult{
			Status:   HealthStatusUnhealthy,
			Message:  "Failed to load AWS config",
			Duration: time.Since(start),
			Error:    err.Error(),
			Details: map[string]interface{}{
				"bucket": bucketName,
			},
		}
		return
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(cfg)

	// Test with HEAD bucket operation (minimal cost, no data transfer)
	_, err = s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})

	duration := time.Since(start)

	if err != nil {
		checks["s3_storage"] = CheckResult{
			Status:   HealthStatusUnhealthy,
			Message:  "S3 bucket accessibility test failed",
			Duration: duration,
			Error:    err.Error(),
			Details: map[string]interface{}{
				"bucket": bucketName,
				"test":   "head_bucket",
			},
		}
		return
	}

	checks["s3_storage"] = CheckResult{
		Status:   HealthStatusHealthy,
		Message:  "S3 storage accessible",
		Duration: duration,
		Details: map[string]interface{}{
			"bucket": bucketName,
			"test":   "head_bucket",
		},
	}
}

// checkSecrets checks secrets management availability
func (h *HealthChecker) checkSecrets(_ context.Context, checks map[string]CheckResult) {
	start := time.Now()

	// Check if required secrets are configured
	privateKeySecret := lconfig.GetPrivateKeySecret()
	duration := time.Since(start)

	if privateKeySecret == "" {
		checks["secrets"] = CheckResult{
			Status:   HealthStatusUnhealthy,
			Message:  "Private key secret not configured",
			Duration: duration,
			Error:    "PRIVATE_KEY_SECRET environment variable not set",
		}
		return
	}

	checks["secrets"] = CheckResult{
		Status:   HealthStatusHealthy,
		Message:  "Secrets configuration is valid",
		Duration: duration,
	}
}

// checkMemory checks memory usage
func (h *HealthChecker) checkMemory(checks map[string]CheckResult) {
	start := time.Now()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Convert bytes to MB for easier reading
	allocMB := float64(m.Alloc) / 1024 / 1024
	sysMB := float64(m.Sys) / 1024 / 1024

	// Lambda has up to 10GB (10240MB) memory maximum
	maxMemoryMB := 10240.0
	memoryUsagePercent := (sysMB / maxMemoryMB) * 100

	duration := time.Since(start)

	status := HealthStatusHealthy
	message := "Memory usage within acceptable limits"

	if memoryUsagePercent > 80 {
		status = HealthStatusDegraded
		message = "Memory usage approaching limits"
	}
	if memoryUsagePercent > 95 {
		status = HealthStatusUnhealthy
		message = "Memory usage critically high"
	}

	checks["memory"] = CheckResult{
		Status:   status,
		Message:  message,
		Duration: duration,
		Details: map[string]interface{}{
			"alloc_mb":      fmt.Sprintf("%.2f", allocMB),
			"sys_mb":        fmt.Sprintf("%.2f", sysMB),
			"max_mb":        fmt.Sprintf("%.2f", maxMemoryMB),
			"usage_percent": fmt.Sprintf("%.2f", memoryUsagePercent),
			"gc_count":      m.NumGC,
			"heap_objects":  m.HeapObjects,
			"goroutines":    runtime.NumGoroutine(),
			"last_gc_time":  time.Unix(0, int64(m.LastGC)).Format(time.RFC3339), // #nosec G115
		},
	}
}

// checkDiskSpace checks disk space availability
func (h *HealthChecker) checkDiskSpace(checks map[string]CheckResult) {
	start := time.Now()

	// In Lambda, check /tmp directory (only writable space available)
	tmpDir := "/tmp"

	var stat syscall.Statfs_t
	err := syscall.Statfs(tmpDir, &stat)
	if err != nil {
		checks["disk_space"] = CheckResult{
			Status:   HealthStatusUnhealthy,
			Message:  "Failed to get disk statistics",
			Duration: time.Since(start),
			Error:    err.Error(),
			Details: map[string]interface{}{
				"path": tmpDir,
			},
		}
		return
	}

	// Calculate disk usage
	totalMB := float64(stat.Blocks*uint64(stat.Bsize)) / 1024 / 1024 // #nosec G115
	freeMB := float64(stat.Bavail*uint64(stat.Bsize)) / 1024 / 1024  // #nosec G115
	usedMB := totalMB - freeMB
	usagePercent := (usedMB / totalMB) * 100

	duration := time.Since(start)

	status := HealthStatusHealthy
	message := "Disk space usage within acceptable limits"

	if usagePercent > 80 {
		status = HealthStatusDegraded
		message = "Disk space usage approaching limits"
	}
	if usagePercent > 95 {
		status = HealthStatusUnhealthy
		message = "Disk space usage critically high"
	}

	checks["disk_space"] = CheckResult{
		Status:   status,
		Message:  message,
		Duration: duration,
		Details: map[string]interface{}{
			"path":          tmpDir,
			"total_mb":      fmt.Sprintf("%.2f", totalMB),
			"used_mb":       fmt.Sprintf("%.2f", usedMB),
			"free_mb":       fmt.Sprintf("%.2f", freeMB),
			"usage_percent": fmt.Sprintf("%.2f", usagePercent),
			"environment":   "lambda",
		},
	}
}

// checkConfiguration validates overall configuration
func (h *HealthChecker) checkConfiguration(ctx context.Context, checks map[string]CheckResult) {
	start := time.Now()

	// Create a production config validator
	validator, err := lconfig.NewProductionConfigValidator(h.logger)
	if err != nil {
		checks["configuration"] = CheckResult{
			Status:   HealthStatusDegraded,
			Message:  "Could not create configuration validator",
			Duration: time.Since(start),
			Error:    err.Error(),
		}
		return
	}

	// Validate configuration
	result, err := validator.ValidateProductionConfig(ctx)
	duration := time.Since(start)

	if err != nil {
		checks["configuration"] = CheckResult{
			Status:   HealthStatusUnhealthy,
			Message:  "Configuration validation failed",
			Duration: duration,
			Error:    err.Error(),
		}
		return
	}

	status := HealthStatusHealthy
	if !result.Valid {
		if result.Summary.CriticalErrors > 0 {
			status = HealthStatusUnhealthy
		} else {
			status = HealthStatusDegraded
		}
	}

	checks["configuration"] = CheckResult{
		Status:   status,
		Message:  "Configuration validation completed",
		Duration: duration,
		Details:  result.Summary,
	}
}

// checkExternalDependencies checks external service dependencies
func (h *HealthChecker) checkExternalDependencies(ctx context.Context, checks map[string]CheckResult) {
	start := time.Now()

	externalChecks := []CheckResult{}

	// Check ActivityPub well-known endpoints
	if domainName := os.Getenv("DOMAIN_NAME"); domainName != "" {
		wellKnownCheck := h.checkWellKnownEndpoint(ctx, domainName)
		externalChecks = append(externalChecks, wellKnownCheck)
	}

	// Check federation connectivity
	federationCheck := h.checkFederationConnectivity(ctx)
	externalChecks = append(externalChecks, federationCheck)

	duration := time.Since(start)

	// Determine overall external dependencies status
	overallStatus := HealthStatusHealthy
	failedChecks := 0
	for _, check := range externalChecks {
		if check.Status == HealthStatusUnhealthy {
			failedChecks++
		}
	}

	if failedChecks > 0 {
		if failedChecks == len(externalChecks) {
			overallStatus = HealthStatusUnhealthy
		} else {
			overallStatus = HealthStatusDegraded
		}
	}

	checks["external_dependencies"] = CheckResult{
		Status:   overallStatus,
		Message:  fmt.Sprintf("External dependency checks completed (%d/%d healthy)", len(externalChecks)-failedChecks, len(externalChecks)),
		Duration: duration,
		Details: map[string]interface{}{
			"checks": externalChecks,
		},
	}
}

// checkWellKnownEndpoint checks if ActivityPub well-known endpoints are accessible
func (h *HealthChecker) checkWellKnownEndpoint(_ context.Context, domain string) CheckResult {
	start := time.Now()

	resp, err := h.httpClientOrDefault().Get(fmt.Sprintf("https://%s/.well-known/nodeinfo", domain))

	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Status:   HealthStatusUnhealthy,
			Message:  "Well-known nodeinfo endpoint not accessible",
			Duration: duration,
			Error:    err.Error(),
			Details: map[string]interface{}{
				"endpoint": fmt.Sprintf("https://%s/.well-known/nodeinfo", domain),
			},
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			h.logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	status := HealthStatusHealthy
	message := "Well-known nodeinfo endpoint accessible"
	if resp.StatusCode != 200 {
		status = HealthStatusUnhealthy
		message = "Well-known nodeinfo endpoint returned non-200 status"
	}

	return CheckResult{
		Status:   status,
		Message:  message,
		Duration: duration,
		Details: map[string]interface{}{
			"endpoint":    fmt.Sprintf("https://%s/.well-known/nodeinfo", domain),
			"status_code": resp.StatusCode,
		},
	}
}

// checkFederationConnectivity checks if federation services are reachable
func (h *HealthChecker) checkFederationConnectivity(_ context.Context) CheckResult {
	start := time.Now()

	// Check if we can connect to a well-known ActivityPub instance
	resp, err := h.httpClientOrDefault().Get("https://mastodon.social/.well-known/nodeinfo")

	duration := time.Since(start)

	if err != nil {
		return CheckResult{
			Status:   HealthStatusDegraded,
			Message:  "Federation connectivity test failed",
			Duration: duration,
			Error:    err.Error(),
			Details: map[string]interface{}{
				"test_endpoint": "https://mastodon.social/.well-known/nodeinfo",
				"note":          "This may indicate network connectivity issues",
			},
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			h.logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	status := HealthStatusHealthy
	message := "Federation connectivity test successful"
	if resp.StatusCode != 200 {
		status = HealthStatusDegraded
		message = "Federation connectivity test returned unexpected status"
	}

	return CheckResult{
		Status:   status,
		Message:  message,
		Duration: duration,
		Details: map[string]interface{}{
			"test_endpoint": "https://mastodon.social/.well-known/nodeinfo",
			"status_code":   resp.StatusCode,
		},
	}
}

// RegisterHealthRoutes registers health check routes
func RegisterHealthRoutes(app *lift.App, storage core.RepositoryStorage, logger *zap.Logger) {
	healthChecker := NewHealthChecker(logger, storage)

	// Liveness probe - simple check that service is running
	_ = app.GET("/health/live", lift.HandlerFunc(healthChecker.HandleLivenessCheck))

	// Readiness probe - checks that service is ready to handle traffic
	_ = app.GET("/health/ready", lift.HandlerFunc(healthChecker.HandleReadinessCheck))

	// Detailed health check - comprehensive status information
	_ = app.GET("/health/detailed", lift.HandlerFunc(healthChecker.HandleDetailedHealthCheck))

	// Legacy health endpoint for backward compatibility
	_ = app.GET("/health", lift.HandlerFunc(healthChecker.HandleLivenessCheck))
}

// HealthCheckMiddleware adds health check information to request context
func HealthCheckMiddleware(_ *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(c *lift.Context) error {
			// Add health check timestamp to response headers
			c.Response.Header("X-Health-Check-Time", time.Now().UTC().Format(time.RFC3339))
			c.Response.Header("X-Service-Name", "lesser-api")

			return next.Handle(c)
		})
	}
}
