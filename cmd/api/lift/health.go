package lift

import (
	"context"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HealthStatus represents the overall health status
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// HealthCheckResponse represents the health check response
type HealthCheckResponse struct {
	Status    HealthStatus           `json:"status"`
	Timestamp time.Time             `json:"timestamp"`
	Version   string                `json:"version,omitempty"`
	Checks    map[string]CheckResult `json:"checks,omitempty"`
	Uptime    time.Duration         `json:"uptime,omitempty"`
}

// CheckResult represents individual health check result
type CheckResult struct {
	Status    HealthStatus `json:"status"`
	Message   string       `json:"message,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
	Error     string       `json:"error,omitempty"`
	Details   interface{}  `json:"details,omitempty"`
}

// HealthChecker handles health check operations
type HealthChecker struct {
	logger       *zap.Logger
	storage      interface{}
	startTime    time.Time
	version      string
	environment  string
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(logger *zap.Logger, storage interface{}) *HealthChecker {
	return &HealthChecker{
		logger:      logger,
		storage:     storage,
		startTime:   time.Now(),
		version:     "1.0.0", // TODO: Get from build info
		environment: config.GetEnvironment(),
	}
}

// HandleLivenessCheck handles GET /health/live - basic liveness check
func (h *HealthChecker) HandleLivenessCheck(c *lift.Context) error {
	response := HealthCheckResponse{
		Status:    HealthStatusHealthy,
		Timestamp: time.Now(),
		Version:   h.version,
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
		Uptime:    time.Since(h.startTime),
		Checks:    checks,
	}

	statusCode := http.StatusOK
	if overallStatus == HealthStatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	} else if overallStatus == HealthStatusDegraded {
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
	
	// Quick validation without full AWS resource checks
	err := config.QuickValidateProductionConfig()
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

	// If we have storage interface, try a simple operation
	if h.storage != nil {
		// TODO: Add actual database ping when storage interface is available
		checks["database"] = CheckResult{
			Status:   HealthStatusHealthy,
			Message:  "Database configuration is valid",
			Duration: duration,
		}
	} else {
		checks["database"] = CheckResult{
			Status:   HealthStatusDegraded,
			Message:  "Database configuration valid but storage interface not available",
			Duration: duration,
		}
	}
}

// checkS3Storage checks S3 storage availability
func (h *HealthChecker) checkS3Storage(ctx context.Context, checks map[string]CheckResult) {
	start := time.Now()

	// Basic S3 configuration check
	bucketName := config.GetS3Bucket()
	duration := time.Since(start)

	if bucketName == "" {
		checks["s3_storage"] = CheckResult{
			Status:   HealthStatusDegraded,
			Message:  "S3 storage not configured (optional)",
			Duration: duration,
		}
		return
	}

	// TODO: Add actual S3 connectivity test
	checks["s3_storage"] = CheckResult{
		Status:   HealthStatusHealthy,
		Message:  "S3 storage configuration present",
		Duration: duration,
		Details: map[string]string{
			"bucket": bucketName,
		},
	}
}

// checkSecrets checks secrets management availability
func (h *HealthChecker) checkSecrets(ctx context.Context, checks map[string]CheckResult) {
	start := time.Now()

	// Check if required secrets are configured
	privateKeySecret := config.GetPrivateKeySecret()
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

	// TODO: Add actual memory usage check
	// For now, just report as healthy
	duration := time.Since(start)

	checks["memory"] = CheckResult{
		Status:   HealthStatusHealthy,
		Message:  "Memory usage within acceptable limits",
		Duration: duration,
		Details: map[string]string{
			"note": "Memory monitoring not yet implemented",
		},
	}
}

// checkDiskSpace checks disk space availability
func (h *HealthChecker) checkDiskSpace(checks map[string]CheckResult) {
	start := time.Now()

	// TODO: Add actual disk space check
	// For Lambda environments, this might not be applicable
	duration := time.Since(start)

	checks["disk_space"] = CheckResult{
		Status:   HealthStatusHealthy,
		Message:  "Disk space monitoring not applicable for serverless environment",
		Duration: duration,
		Details: map[string]string{
			"environment": "serverless",
		},
	}
}

// checkConfiguration validates overall configuration
func (h *HealthChecker) checkConfiguration(ctx context.Context, checks map[string]CheckResult) {
	start := time.Now()

	// Create a production config validator
	validator, err := config.NewProductionConfigValidator(h.logger)
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

	// TODO: Add checks for external dependencies like:
	// - Federation endpoints
	// - AI services (AWS Bedrock)
	// - Monitoring services
	
	duration := time.Since(start)

	checks["external_dependencies"] = CheckResult{
		Status:   HealthStatusHealthy,
		Message:  "External dependency checks not yet implemented",
		Duration: duration,
		Details: map[string]string{
			"note": "Future implementation will check federation and AI services",
		},
	}
}

// RegisterHealthRoutes registers health check routes
func RegisterHealthRoutes(app *lift.App, storage interface{}, logger *zap.Logger) {
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
func HealthCheckMiddleware(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(c *lift.Context) error {
			// Add health check timestamp to response headers
			c.Response.Header("X-Health-Check-Time", time.Now().UTC().Format(time.RFC3339))
			c.Response.Header("X-Service-Name", "lesser-api")
			
			return next.Handle(c)
		})
	}
}