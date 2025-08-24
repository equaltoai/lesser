// Package lambda provides standardized health check patterns for Lambda functions.
package lambda

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage/core"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HealthCheckPattern provides standardized health check endpoints for Lambda functions
type HealthCheckPattern struct {
	lambdaCtx *common.LambdaContext
	repos     core.RepositoryStorage
	logger    *zap.Logger
	startTime time.Time
}

// NewHealthCheckPattern creates a new standardized health check pattern
func NewHealthCheckPattern(lambdaCtx *common.LambdaContext, startTime time.Time) *HealthCheckPattern {
	return &HealthCheckPattern{
		lambdaCtx: lambdaCtx,
		repos:     lambdaCtx.Repos.(core.RepositoryStorage),
		logger:    lambdaCtx.Logger,
		startTime: startTime,
	}
}

// ConfigureHealthRoutes adds standardized health check endpoints to the Lift app
// This eliminates the 80+ line duplication across Lambda functions
func (hcp *HealthCheckPattern) ConfigureHealthRoutes(app *liftPkg.App) {
	// Liveness endpoint - basic service availability
	_ = app.GET("/health/live", hcp.handleLivenessCheck)

	// Readiness endpoint - dependency checks
	_ = app.GET("/health/ready", hcp.handleReadinessCheck)

	// Detailed health endpoint - comprehensive diagnostics
	_ = app.GET("/health/detailed", hcp.handleDetailedHealthCheck)

	hcp.logger.Info("health check routes configured",
		zap.String("service", "lambda-service"),
	)
}

// handleLivenessCheck handles the basic liveness probe
func (hcp *HealthCheckPattern) handleLivenessCheck(ctx *liftPkg.Context) error {
	response := map[string]interface{}{
		"status":    observability.HealthStatusHealthy,
		"timestamp": time.Now(),
		"service":   "lambda-service",
		"version":   hcp.lambdaCtx.Config.Version,
		"uptime":    time.Since(hcp.startTime).String(),
	}
	return ctx.Status(200).JSON(response)
}

// handleReadinessCheck handles the readiness probe with dependency checks
func (hcp *HealthCheckPattern) handleReadinessCheck(ctx *liftPkg.Context) error {
	status := observability.HealthStatusHealthy
	checks := []map[string]interface{}{}

	// Check DynamoDB connectivity
	dbCheck := hcp.checkDynamoDBConnectivity(ctx.Request.Context())
	checks = append(checks, dbCheck)

	if dbCheck["status"] == observability.HealthStatusCritical {
		status = observability.HealthStatusCritical
	}

	response := map[string]interface{}{
		"status":    status,
		"timestamp": time.Now(),
		"service":   "lambda-service",
		"version":   hcp.lambdaCtx.Config.Version,
		"checks":    checks,
	}

	statusCode := 200
	if status == observability.HealthStatusCritical {
		statusCode = 503
	}

	return ctx.Status(statusCode).JSON(response)
}

// handleDetailedHealthCheck handles comprehensive health diagnostics
func (hcp *HealthCheckPattern) handleDetailedHealthCheck(ctx *liftPkg.Context) error {
	status := observability.HealthStatusHealthy
	checks := []map[string]interface{}{}
	summary := map[string]interface{}{
		"total_checks":    0,
		"healthy_checks":  0,
		"warning_checks":  0,
		"critical_checks": 0,
	}

	// Runtime health check
	runtimeCheck := hcp.createRuntimeHealthCheck()
	checks = append(checks, runtimeCheck)
	hcp.updateSummary(summary, runtimeCheck)

	// Database detailed check
	dbCheck := hcp.checkDynamoDBConnectivity(ctx.Request.Context())
	checks = append(checks, dbCheck)
	hcp.updateSummary(summary, dbCheck)

	// AWS services check
	awsCheck := hcp.checkAWSServices(ctx.Request.Context())
	checks = append(checks, awsCheck)
	hcp.updateSummary(summary, awsCheck)

	// Memory and performance check
	perfCheck := hcp.checkPerformanceMetrics()
	checks = append(checks, perfCheck)
	hcp.updateSummary(summary, perfCheck)

	// Determine overall status
	if summary["critical_checks"].(int) > 0 {
		status = observability.HealthStatusCritical
	} else if summary["warning_checks"].(int) > 0 {
		status = observability.HealthStatusWarning
	}

	summary["total_checks"] = len(checks)

	response := map[string]interface{}{
		"status":    status,
		"timestamp": time.Now(),
		"service":   "lambda-service",
		"version":   hcp.lambdaCtx.Config.Version,
		"region":    hcp.lambdaCtx.Config.Region,
		"uptime":    time.Since(hcp.startTime).String(),
		"checks":    checks,
		"summary":   summary,
	}

	statusCode := 200
	switch status {
	case observability.HealthStatusCritical:
		statusCode = 503
	case observability.HealthStatusWarning:
		statusCode = 200 // Warnings don't fail health checks
	}

	return ctx.Status(statusCode).JSON(response)
}

// checkDynamoDBConnectivity checks DynamoDB connectivity and performance
func (hcp *HealthCheckPattern) checkDynamoDBConnectivity(ctx context.Context) map[string]interface{} {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()
	_, err := hcp.repos.User().GetUser(checkCtx, "health-check-user")
	duration := time.Since(start)

	check := map[string]interface{}{
		"name":       "dynamodb_connectivity",
		"status":     observability.HealthStatusHealthy,
		"duration":   duration.Milliseconds(),
		"table_name": hcp.lambdaCtx.Config.DynamoTableName,
	}

	// "user not found" is expected and indicates healthy connectivity
	if err != nil && err.Error() != "user not found" {
		check["status"] = observability.HealthStatusCritical
		check["message"] = fmt.Sprintf("Database connectivity issue: %v", err)
		hcp.logger.Error("DynamoDB health check failed", zap.Error(err))
	} else {
		// Check for performance issues
		if duration > 5*time.Second {
			check["status"] = observability.HealthStatusWarning
			check["message"] = "Database response time is slow"
		}
	}

	return check
}

// checkAWSServices checks AWS service connectivity
func (hcp *HealthCheckPattern) checkAWSServices(_ context.Context) map[string]interface{} {
	check := map[string]interface{}{
		"name":   "aws_services",
		"status": observability.HealthStatusHealthy,
	}

	// Check AWS config availability
	if hcp.lambdaCtx.AWSServices.Config.Region == "" {
		check["status"] = observability.HealthStatusWarning
		check["message"] = "AWS region not configured"
	}

	// Additional AWS service checks could be added here
	// - S3 connectivity
	// - SQS connectivity
	// - CloudWatch connectivity

	return check
}

// createRuntimeHealthCheck creates a runtime health check
func (hcp *HealthCheckPattern) createRuntimeHealthCheck() map[string]interface{} {
	return map[string]interface{}{
		"name":        "runtime",
		"status":      observability.HealthStatusHealthy,
		"message":     "Service runtime is healthy",
		"uptime_ms":   time.Since(hcp.startTime).Milliseconds(),
		"service":     "lambda-service",
		"version":     hcp.lambdaCtx.Config.Version,
		"go_version":  "go1.21", // Could be determined dynamically
		"lambda_type": string("lambda"),
	}
}

// checkPerformanceMetrics checks performance and resource usage
func (hcp *HealthCheckPattern) checkPerformanceMetrics() map[string]interface{} {
	check := map[string]interface{}{
		"name":   "performance",
		"status": observability.HealthStatusHealthy,
	}

	// Basic performance metrics
	uptime := time.Since(hcp.startTime)

	check["uptime_ms"] = uptime.Milliseconds()
	check["uptime_human"] = uptime.String()

	// Memory usage could be checked here if needed
	// CPU usage could be checked here if needed

	// Check for long-running processes (potential memory leaks)
	if uptime > 15*time.Minute {
		check["status"] = observability.HealthStatusWarning
		check["message"] = "Lambda has been running for an extended period"
	}

	return check
}

// updateSummary updates the health check summary based on individual check results
func (hcp *HealthCheckPattern) updateSummary(summary map[string]interface{}, check map[string]interface{}) {
	status := check["status"].(string)

	switch status {
	case observability.HealthStatusHealthy:
		summary["healthy_checks"] = summary["healthy_checks"].(int) + 1
	case observability.HealthStatusWarning:
		summary["warning_checks"] = summary["warning_checks"].(int) + 1
	case observability.HealthStatusCritical:
		summary["critical_checks"] = summary["critical_checks"].(int) + 1
	}
}

// CreateHealthCheckMiddleware creates middleware for health check request logging
func (hcp *HealthCheckPattern) CreateHealthCheckMiddleware() liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			// Skip detailed logging for health checks to reduce noise
			if isHealthCheckPath(ctx.Request.Path) {
				return next.Handle(ctx)
			}

			start := time.Now()
			err := next.Handle(ctx)

			hcp.logger.Debug("health check request",
				zap.String("path", ctx.Request.Path),
				zap.String("method", ctx.Request.Method),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("success", err == nil),
			)

			return err
		})
	}
}

// isHealthCheckPath determines if a path is a health check endpoint
func isHealthCheckPath(path string) bool {
	healthPaths := []string{
		"/health/live",
		"/health/ready",
		"/health/detailed",
		"/health",
	}

	for _, healthPath := range healthPaths {
		if path == healthPath {
			return true
		}
	}

	return false
}

// HealthCheckConfig provides configuration for health check behavior
type HealthCheckConfig struct {
	EnableDetailedChecks          bool
	DBTimeoutSeconds              int
	PerformanceWarningThresholdMs int
}

// DefaultHealthCheckConfig returns default health check configuration
func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		EnableDetailedChecks:          true,
		DBTimeoutSeconds:              10,
		PerformanceWarningThresholdMs: 5000,
	}
}

// ConfigureMinimalHealthRoutes adds only basic health endpoints (for services that don't need full checks)
func (hcp *HealthCheckPattern) ConfigureMinimalHealthRoutes(app *liftPkg.App) {
	// Only liveness endpoint for minimal setup
	_ = app.GET("/health/live", hcp.handleLivenessCheck)
	_ = app.GET("/health", hcp.handleLivenessCheck) // Alias

	hcp.logger.Info("minimal health check routes configured",
		zap.String("service", "lambda-service"),
	)
}
