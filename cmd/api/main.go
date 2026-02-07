package main

/*
Lesser API Server - Mastodon-compatible ActivityPub implementation

This Lambda function serves the Lesser API using AWS API Gateway v2.
All routing is handled by AppTheory.

The API Gateway configuration forwards the full request path including /api/v1
and /api/v2 prefixes to this Lambda. All routes must include the complete
path with prefix to match Mastodon API specifications.
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/crawler"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/streaming"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// Context key types for type-safe context values
type contextKey string

const (
	latencyStartKey contextKey = "latency_start"
	endpointKey     contextKey = "endpoint"
	methodKey       contextKey = "method"
	pathKey         contextKey = "path"
)

var (
	lambdaCtx           *common.LambdaContext
	cfg                 *config.Config
	repos               core.RepositoryStorage
	logger              *zap.Logger
	apiHandler          *apiHandlers.Handler
	authService         *auth.AuthService
	emfMetrics          *observability.EMFMetrics
	healthChecker       *observability.HealthChecker
	metricsCollector    *observability.MetricsCollector //nolint:unused // Extracted from standardized initialization, used in different configurations
	tracingManager      *observability.TracingManager
	latencyAggregator   *observability.LatencyAggregator
	latencyAlerter      *observability.LatencyAlerter
	costTrackingService *cost.TrackingService
	startTime           time.Time
)

var (
	lambdaStart = lambda.Start

	newLambdaOptimizedClient = theorydb.NewLambdaOptimizedClient
	newRepositoryFactory     = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (core.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
	newAuthService = auth.NewAuthService
	newStreamQueue = streaming.NewDynamoStreamQueue

	createAPIAuthMiddlewareFromAuthService = auth.CreateAPIAuthMiddlewareFromAuthService
	newAPIHandler                          = apiHandlers.NewHandler

	createInstanceLockMiddlewareFn = createInstanceLockMiddleware
	configureRoutesFn              = configureRoutes

	trackLambdaInvocation = func(ctx context.Context, service *cost.TrackingService, operation cost.LambdaOperation) error {
		return service.TrackLambdaInvocation(ctx, operation)
	}
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	startTime = time.Now()

	// Initialize Lambda with standardized configuration
	lambdaConfig := common.LambdaConfig{
		ServiceName: "api",
		LambdaType:  common.LambdaTypeAPI,
		// Feature flags will be auto-detected from environment
		RequestTimeout: 30 * time.Second,
	}

	// Use standardized Lambda initialization
	lambdaCtx = common.MustInitializeLambda(lambdaConfig)

	// Initialize with default options for API Lambda type
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		// Fallback to manual initialization for services requiring it
		initializeManualServices()
	} else {
		// Extract standardized services
		extractStandardizedServices()
	}

	// Initialize API-specific services
	initializeAPISpecificServices()
}

// extractStandardizedServices extracts services from standardized initialization
func extractStandardizedServices() {
	// Extract basic services
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger

	// Extract storage services
	if lambdaCtx.Repos != nil {
		repos = lambdaCtx.Repos.(core.RepositoryStorage)
	}

	// Extract auth services
	if lambdaCtx.AuthService != nil {
		authService = lambdaCtx.AuthService.(*auth.AuthService)
	}

	// Extract observability services
	if lambdaCtx.EMFMetrics != nil {
		emfMetrics = lambdaCtx.EMFMetrics.(*observability.EMFMetrics)
	}
	if lambdaCtx.HealthChecker != nil {
		healthChecker = lambdaCtx.HealthChecker.(*observability.HealthChecker)
	}
	if lambdaCtx.TracingManager != nil {
		tracingManager = lambdaCtx.TracingManager.(*observability.TracingManager)
	}
	if lambdaCtx.MetricsCollector != nil {
		metricsCollector = lambdaCtx.MetricsCollector.(*observability.MetricsCollector)
	}
	if lambdaCtx.LatencyAggregator != nil {
		latencyAggregator = lambdaCtx.LatencyAggregator.(*observability.LatencyAggregator)
	}
	if lambdaCtx.LatencyAlerter != nil {
		latencyAlerter = lambdaCtx.LatencyAlerter.(*observability.LatencyAlerter)
	}

	// Initialize centralized cost tracking service if CloudWatch client is available
	if lambdaCtx.AWSServices != nil && lambdaCtx.AWSServices.CloudWatch != nil {
		costTrackingService = cost.NewCostTrackingServiceForLambda(lambdaCtx.AWSServices.CloudWatch, logger, "api")
		logger.Info("initialized centralized cost tracking service from standardized initialization")
	}

	logger.Info("extracted services from standardized initialization")
}

// initializeManualServices provides fallback manual initialization
func initializeManualServices() {
	logger = lambdaCtx.Logger
	cfg = lambdaCtx.Config

	logger.Info("falling back to manual service initialization")

	// Manual DynamORM initialization
	tableName := cfg.DynamoTableName
	logger.Info("manual initialization: configured DynamoDB table",
		zap.String("table_name", tableName))
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main" // fallback
		logger.Warn("manual initialization: missing table name, falling back to default",
			zap.String("table_name", tableName),
			zap.Error(err))
	}
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		logger.Fatal("DYNAMODB_TABLE environment variable is required")
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := newLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Create repository storage using factory pattern
	repos, err = newRepositoryFactory(db, tableName, logger)
	if err != nil {
		logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	// Initialize auth service
	authService, err = newAuthService(cfg, repos)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	// Initialize observability services manually
	if !cfg.DisableMetrics {
		// EMF Metrics for CloudWatch integration
		emfMetrics = observability.NewEMFMetrics(logger, "Lesser/API", "api")
		emfMetrics.AddDimension(observability.DimensionService, "api")
		emfMetrics.AddDimension(observability.DimensionEnvironment, cfg.Stage)
		emfMetrics.AddDimension(observability.DimensionRegion, cfg.Region)

		// Legacy metrics collector
		cloudWatchClient := cloudwatch.NewFromConfig(lambdaCtx.AWSServices.Config)
		metricsCollector = observability.NewMetricsCollector(cloudWatchClient, "Lesser/API", logger)

		// Health checker
		healthConfig := &observability.HealthConfig{
			TableName:        tableName,
			CheckTimeout:     5 * time.Second,
			CacheTimeout:     30 * time.Second,
			DependencyChecks: true,
		}
		healthChecker = observability.NewHealthChecker(logger, lambdaCtx.AWSServices.Config, "api", cfg.Version, healthConfig)

		// Distributed tracing
		tracingConfig := &observability.TracingConfig{
			ServiceName:    "lesser-api",
			ServiceVersion: cfg.Version,
			SamplingRate:   observability.TracingSampleRatePercent / 100.0,
			Enabled:        cfg.XRayTracingEnabled,
		}
		tracingManager = observability.NewTracingManager(logger, tracingConfig)

		// Latency aggregation and alerting
		metricsRecorder := observability.NewDefaultMetricsRecorder(
			func(ctx context.Context, metric *models.MetricRecord) error {
				return repos.MetricRecord().CreateMetricRecord(ctx, metric)
			},
			"api",
		)

		// Initialize latency aggregator
		latencyAggregator = observability.NewLatencyAggregator(
			logger,
			metricsRecorder,
			observability.WithAggregateInterval(5*time.Minute),
			observability.WithRetentionPeriod(24*time.Hour),
			observability.WithMaxBuckets(1000),
		)
		latencyAggregator.Start()

		// Initialize latency alerter
		latencyAlerter = observability.NewLatencyAlerter(logger, metricsRecorder, nil)

		// Initialize centralized cost tracking service
		if cloudWatchClient != nil {
			costTrackingService = cost.NewCostTrackingServiceForLambda(cloudWatchClient, logger, "api")
			logger.Info("initialized centralized cost tracking service")
		}

		logger.Info("initialized observability services manually")
	}
}

// initializeAPISpecificServices initializes API-specific services
func initializeAPISpecificServices() {
	// Validate VAPID keys in production environment
	if err := apiHandlers.ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger); err != nil {
		logger.Fatal("VAPID keys validation failed in production", zap.Error(err))
	}

	// Create stream queue service with proper fallback
	var streamQueue streaming.StreamQueueService
	if lambdaCtx.StreamQueue != nil {
		// Try to cast from standardized initialization
		if sq, ok := lambdaCtx.StreamQueue.(streaming.StreamQueueService); ok {
			streamQueue = sq
		}
	}

	if streamQueue == nil {
		// Fallback to manual stream queue creation
		// Get or create DynamORM client with proper type
		var coreDB dynamormCore.DB
		if lambdaCtx.DynamoDB != nil {
			if db, ok := lambdaCtx.DynamoDB.(dynamormCore.DB); ok {
				coreDB = db
			}
		}

		if coreDB == nil {
			// Last resort: create DynamORM client
			manualDB, dbErr := newLambdaOptimizedClient(context.Background(), cfg.Region)
			if dbErr != nil {
				logger.Fatal("Failed to initialize DynamORM for stream queue", zap.Error(dbErr))
			}
			coreDB = manualDB
		}
		streamQueue = newStreamQueue(coreDB, cfg.DynamoTableName, logger)
	}

	// Create API handler for all endpoints
	apiHandler = newAPIHandler(cfg, repos, logger, streamQueue)

	logger.Info("initialized API-specific services")
}

func main() {
	app := buildApp(logger)

	lambdaHandler := func(ctx context.Context, event json.RawMessage) (any, error) {
		requestStart := time.Now()

		// Record cold start metric if this is a cold start.
		if time.Since(startTime) < 30*time.Second && emfMetrics != nil {
			emfMetrics.RecordBusinessMetric(observability.MetricColdStarts, 1.0, observability.UnitCount, nil)
			coldStartDuration := time.Since(startTime)
			emfMetrics.RecordBusinessMetric(observability.MetricColdStartDuration, float64(coldStartDuration.Milliseconds()), observability.UnitMilliseconds, nil)
		}

		result, err := app.HandleLambda(ctx, event)

		requestDuration := time.Since(requestStart)
		if emfMetrics != nil {
			emfMetrics.RecordLatency("api_request", requestDuration)
			emfMetrics.RecordThroughput("api_request", 1)

			if err != nil {
				emfMetrics.RecordError("api_request", "lambda_error")
			} else {
				emfMetrics.RecordSuccess("api_request")
			}
		}

		lambdaCtx.FlushObservabilityServices()

		return result, err
	}

	lambdaStart(lambdaHandler)
}

func buildApp(lambdaLogger *zap.Logger) *apptheory.App {
	app := apptheory.New(
		apptheory.WithCORS(apptheory.CORSConfig{
			AllowedOrigins:   []string{"*"},
			AllowCredentials: false,
			AllowHeaders: []string{
				"Accept",
				"Authorization",
				"Content-Type",
				"User-Agent",
				"X-Forwarded-For",
				"X-Forwarded-Proto",
				"X-Request-Id",
				"X-Tenant-Id",
			},
		}),
		apptheory.WithLimits(apptheory.Limits{
			MaxRequestBytes:  512 * 1024,
			MaxResponseBytes: 0,
		}),
	)

	// Timeout middleware (app-tier).
	app.Use(apptheory.TimeoutMiddleware(apptheory.TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
	}))

	// Crawler classification middleware (observe-only; configurable via CRAWLER_PROTECTION_MODE).
	app.Use(crawler.NewMiddleware(lambdaLogger))

	// Block publishing/signups until instance activation completes.
	app.Use(createInstanceLockMiddlewareFn(repos, lambdaLogger))

	// Optional auth (enables user context for public endpoints).
	if authService != nil {
		app.Use(createAPIAuthMiddlewareFromAuthService(authService, lambdaLogger))
	}
	// Optional OAuth auth fallback (enables user context for OAuth/agent tokens too).
	// This allows downstream middleware (rate limits, logging) to key by username for agents.
	app.Use(createOptionalOAuthAuthMiddleware(cfg, repos, lambdaLogger))

	// Enforce default-deny public surface policy.
	app.Use(createPublicSurfaceMiddleware())

	// Agent safety rails middleware (Phase 1).
	app.Use(createAgentSafetyRailsMiddleware(cfg, repos, lambdaLogger))

	// Request logging with correlation fields.
	app.Use(createLoggingMiddleware(lambdaLogger))

	// Security headers (web-friendly).
	app.Use(apiSecurityHeaders())

	// Standardized API error mapping.
	app.Use(common.CreateAPIErrorMiddleware(lambdaLogger))

	// Centralized cost tracking middleware.
	if costTrackingService != nil {
		app.Use(createCentralizedCostTrackingMiddleware())
	}

	// Distributed tracing middleware.
	if tracingManager != nil {
		app.Use(createTracingMiddleware())
	}

	// EMF metrics middleware.
	if emfMetrics != nil {
		app.Use(createEMFMetricsMiddleware())
	}

	// Comprehensive latency tracking middleware.
	app.Use(createLatencyTrackingMiddleware())

	// Configure API routes.
	configureRoutesFn(app)

	// Health check routes.
	if healthChecker != nil {
		configureHealthRoutes(app)
	}

	return app
}

// requestInfo holds extracted request information
type requestInfo struct {
	method    string
	path      string
	endpoint  string
	userAgent string
	clientIP  string
}

func extractRequestInfo(ctx *apptheory.Context) requestInfo {
	info := requestInfo{
		method: "GET",
		path:   "/",
	}

	if ctx == nil {
		info.endpoint = info.method + " " + info.path
		return info
	}

	if method := strings.TrimSpace(ctx.Request.Method); method != "" {
		info.method = method
	}
	if path := strings.TrimSpace(ctx.Request.Path); path != "" {
		info.path = path
	}
	info.userAgent = headerValue(ctx, "User-Agent")
	info.clientIP = headerValue(ctx, "X-Forwarded-For")

	info.endpoint = info.method + " " + info.path
	return info
}

func headerValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	values := ctx.Request.Headers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// addLatencyContextValues adds latency tracking values to context
func addLatencyContextValues(ctx *apptheory.Context, info requestInfo, startTime time.Time) {
	if ctx == nil {
		return
	}

	ctx.Set(string(latencyStartKey), startTime)
	ctx.Set(string(endpointKey), info.endpoint)
	ctx.Set(string(methodKey), info.method)
	ctx.Set(string(pathKey), info.path)
}

// recordDynamoDBLatencyMetric records latency metric to DynamoDB asynchronously
func recordDynamoDBLatencyMetric(ctx context.Context, info requestInfo, statusCode int, totalLatency time.Duration) {
	go func() {
		if err := recordLatencyMetric(ctx, "api_endpoint", info.endpoint, totalLatency, map[string]string{
			"endpoint":    info.path,
			"method":      info.method,
			"status_code": fmt.Sprintf("%d", statusCode),
			"user_agent":  info.userAgent,
			"client_ip":   info.clientIP,
		}); err != nil {
			logger.Warn("failed to record latency metric", zap.Error(err))
		}
	}()
}

// recordAggregatorLatency records latency to the aggregator if available
func recordAggregatorLatency(info requestInfo, totalLatency time.Duration) {
	if latencyAggregator == nil {
		return
	}
	latencyAggregator.RecordLatency(info.endpoint, "api", totalLatency)
}

// calculatePercentiles extracts percentile values from stats
func calculatePercentiles(stats interface{}, totalLatency time.Duration) (float64, float64) {
	defaultMs := float64(totalLatency.Milliseconds())
	p95Ms, p99Ms := defaultMs, defaultMs

	if stats == nil {
		return p95Ms, p99Ms
	}

	// Try type assertion for stats with Percentiles field
	if statsType := stats; statsType != nil {
		// Use reflection-like approach to safely access percentiles
		if statsMap, ok := stats.(map[string]interface{}); ok {
			if percentiles, exists := statsMap["Percentiles"]; exists {
				if percMap, ok := percentiles.(map[string]float64); ok {
					if p95Val, exists := percMap["p95"]; exists {
						p95Ms = p95Val
					}
					if p99Val, exists := percMap["p99"]; exists {
						p99Ms = p99Val
					}
				}
			}
		}
	}

	return p95Ms, p99Ms
}

// checkLatencyAlerting performs latency alerting if alerter is available
func checkLatencyAlerting(ctx context.Context, info requestInfo, totalLatency time.Duration) {
	if latencyAlerter == nil {
		return
	}

	stats, _ := latencyAggregator.GetCurrentStats(info.endpoint, "api")
	p95Ms, p99Ms := calculatePercentiles(stats, totalLatency)

	latencyAlerter.CheckLatency(ctx, "api_endpoint", "api",
		float64(totalLatency.Milliseconds()), p95Ms, p99Ms)
}

// determineErrorType maps status code to error type
func determineErrorType(statusCode int) string {
	if statusCode < 400 || statusCode >= 500 {
		return observability.ErrorTypeInternal
	}

	switch statusCode {
	case 401:
		return observability.ErrorTypeAuthentication
	case 403:
		return observability.ErrorTypeAuthorization
	case 404:
		return observability.ErrorTypeNotFound
	case 409:
		return observability.ErrorTypeConflict
	case 429:
		return observability.ErrorTypeRateLimit
	default:
		return observability.ErrorTypeValidation
	}
}

// recordEMFMetrics records metrics to EMF if available
func recordEMFMetrics(info requestInfo, statusCode int, totalLatency time.Duration, err error) {
	if emfMetrics == nil {
		return
	}

	dimensions := map[string]string{
		observability.DimensionEndpoint:   info.path,
		observability.DimensionMethod:     info.method,
		observability.DimensionStatusCode: fmt.Sprintf("%d", statusCode),
	}

	emfMetrics.RecordLatency(info.endpoint, totalLatency)

	if err != nil {
		errorType := determineErrorType(statusCode)
		dimensions[observability.DimensionErrorType] = errorType
		emfMetrics.RecordError(info.endpoint, errorType)
	} else {
		emfMetrics.RecordSuccess(info.endpoint)
	}

	emfMetrics.RecordThroughput(info.endpoint, 1)
}

// createLatencyTrackingMiddleware creates comprehensive latency tracking middleware
func createLatencyTrackingMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			startTime := time.Now()
			info := extractRequestInfo(ctx)

			addLatencyContextValues(ctx, info, startTime)

			// Execute request
			resp, err := next(ctx)

			// Calculate latency and determine status
			totalLatency := time.Since(startTime)
			statusCode := http.StatusOK
			if err != nil {
				statusCode = http.StatusInternalServerError
			} else if resp != nil && resp.Status != 0 {
				statusCode = resp.Status
			}

			// Record metrics to all available systems
			recordDynamoDBLatencyMetric(ctx.Context(), info, statusCode, totalLatency)
			recordAggregatorLatency(info, totalLatency)
			checkLatencyAlerting(ctx.Context(), info, totalLatency)
			recordEMFMetrics(info, statusCode, totalLatency, err)

			return resp, err
		}
	}
}

// createEMFMetricsMiddleware creates middleware for EMF metrics collection
func createEMFMetricsMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if emfMetrics == nil {
				return next(ctx)
			}

			// Start latency timer
			timer := emfMetrics.StartLatencyTimer(ctx.Context(), "endpoint_request")

			// Extract endpoint information
			method := ctx.Request.Method
			path := ctx.Request.Path
			endpoint := method + " " + path
			dimensions := map[string]string{
				observability.DimensionEndpoint: path,
				observability.DimensionMethod:   method,
			}

			// Execute request
			resp, err := next(ctx)

			// Record metrics based on result
			// Default to 200 if no error
			statusCode := http.StatusOK
			if err != nil {
				statusCode = http.StatusInternalServerError
			} else if resp != nil && resp.Status != 0 {
				statusCode = resp.Status
			}
			dimensions[observability.DimensionStatusCode] = fmt.Sprintf("%d", statusCode)

			if err != nil {
				errorType := observability.ErrorTypeInternal
				if statusCode >= 400 && statusCode < 500 {
					switch statusCode {
					case 401:
						errorType = observability.ErrorTypeAuthentication
					case 403:
						errorType = observability.ErrorTypeAuthorization
					case 404:
						errorType = observability.ErrorTypeNotFound
					case 409:
						errorType = observability.ErrorTypeConflict
					case 429:
						errorType = observability.ErrorTypeRateLimit
					default:
						errorType = observability.ErrorTypeValidation
					}
				}

				dimensions[observability.DimensionErrorType] = errorType
				timer.FinishWithError(emfMetrics, errorType)
				emfMetrics.RecordError(endpoint, errorType)
			} else {
				timer.Finish(emfMetrics, true)
				emfMetrics.RecordSuccess(endpoint)
			}

			// Record status code specific metrics
			if statusCode >= 200 && statusCode < 300 {
				emfMetrics.RecordBusinessMetric("SuccessfulRequests", 1.0, observability.UnitCount, dimensions)
			} else if statusCode >= 400 {
				emfMetrics.RecordBusinessMetric("ErrorRequests", 1.0, observability.UnitCount, dimensions)
			}

			// Record throughput
			emfMetrics.RecordThroughput(endpoint, 1)

			return resp, err
		}
	}
}

func configureHealthRoutes(app *apptheory.App) {
	// Liveness endpoint
	app.Get("/health/live", func(_ *apptheory.Context) (*apptheory.Response, error) {
		response := map[string]interface{}{
			"status":    observability.HealthStatusHealthy,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
		}
		return apptheory.JSON(200, response)
	})

	// Legacy health endpoint (infra + backwards compatibility)
	app.Get("/health", func(_ *apptheory.Context) (*apptheory.Response, error) {
		response := map[string]interface{}{
			"status":    observability.HealthStatusHealthy,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
		}
		return apptheory.JSON(200, response)
	})

	// Readiness endpoint
	app.Get("/health/ready", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		// Basic readiness check - can we access our dependencies?
		status := observability.HealthStatusHealthy
		checks := []map[string]interface{}{}

		// Check DynamoDB connectivity
		checkCtx, cancel := context.WithTimeout(ctx.Context(), 5*time.Second)
		defer cancel()

		start := time.Now()
		_, err := repos.Account().GetUser(checkCtx, "health-check-user")
		duration := time.Since(start)

		dbCheck := map[string]interface{}{
			"name":     "dynamodb",
			"status":   observability.HealthStatusHealthy,
			"duration": duration.Milliseconds(),
		}

		if err != nil && err.Error() != "user not found" { // "not found" is expected
			dbCheck["status"] = observability.HealthStatusCritical
			dbCheck["message"] = "Database connectivity issue"
			status = observability.HealthStatusCritical
		}

		checks = append(checks, dbCheck)

		response := map[string]interface{}{
			"status":    status,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
			"checks":    checks,
		}

		statusCode := 200
		if status == observability.HealthStatusCritical {
			statusCode = 503
		}

		return apptheory.JSON(statusCode, response)
	})

	// Detailed health endpoint
	app.Get("/health/detailed", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		// Comprehensive health check with all components
		status := observability.HealthStatusHealthy
		checks := []map[string]interface{}{}
		summary := map[string]interface{}{
			"total_checks":    0,
			"healthy_checks":  0,
			"warning_checks":  0,
			"critical_checks": 0,
		}

		// Runtime check
		runtimeCheck := map[string]interface{}{
			"name":      "runtime",
			"status":    observability.HealthStatusHealthy,
			"message":   "Service runtime is healthy",
			"uptime_ms": time.Since(startTime).Milliseconds(),
			"service":   "api",
			"version":   cfg.Version,
		}
		checks = append(checks, runtimeCheck)
		summary["healthy_checks"] = summary["healthy_checks"].(int) + 1

		// Database detailed check
		checkCtx, cancel := context.WithTimeout(ctx.Context(), 10*time.Second)
		defer cancel()

		start := time.Now()
		_, err := repos.Account().GetUser(checkCtx, "health-check-user")
		duration := time.Since(start)

		dbCheck := map[string]interface{}{
			"name":       "dynamodb_detailed",
			"status":     observability.HealthStatusHealthy,
			"duration":   duration.Milliseconds(),
			"table_name": cfg.DynamoTableName,
		}

		if err != nil && err.Error() != "user not found" {
			dbCheck["status"] = observability.HealthStatusCritical
			dbCheck["message"] = fmt.Sprintf("Database error: %v", err)
			status = observability.HealthStatusCritical
			summary["critical_checks"] = summary["critical_checks"].(int) + 1
		} else {
			summary["healthy_checks"] = summary["healthy_checks"].(int) + 1
		}

		checks = append(checks, dbCheck)
		summary["total_checks"] = len(checks)

		response := map[string]interface{}{
			"status":    status,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
			"region":    cfg.Region,
			"checks":    checks,
			"summary":   summary,
		}

		statusCode := 200
		if status == observability.HealthStatusCritical {
			statusCode = 503
		}

		return apptheory.JSON(statusCode, response)
	})
}

// createTracingMiddleware creates middleware for distributed tracing
func createTracingMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if tracingManager == nil || !tracingManager.IsEnabled() {
				return next(ctx)
			}

			// Start tracing segment
			traceCtx, segment := tracingManager.StartSegment(ctx.Context(), "api-request")
			defer func() {
				if segment != nil {
					segment.Close(nil)
				}
			}()

			// Extract request info
			method := ctx.Request.Method
			url := ctx.Request.Path
			userAgent := headerValue(ctx, "User-Agent")
			clientIP := headerValue(ctx, "X-Forwarded-For")

			// Add HTTP request information
			tracingManager.SetHTTPRequest(traceCtx,
				method,
				url,
				userAgent,
				clientIP)

			// Add request metadata
			tracingManager.AddAnnotation(traceCtx, "endpoint", url)
			tracingManager.AddAnnotation(traceCtx, "method", method)

			// Add trace ID to response headers for debugging
			// Note: Lift doesn't provide direct header access for responses in middleware
			// This would need to be set in individual handlers if needed
			_ = tracingManager.GetTraceContext(traceCtx)

			// Execute request
			resp, err := next(ctx)

			// Record response information
			// Default to 200 if no error, 500 if error
			statusCode := http.StatusOK
			if err != nil {
				statusCode = http.StatusInternalServerError
			} else if resp != nil && resp.Status != 0 {
				statusCode = resp.Status
			}
			tracingManager.SetHTTPResponse(traceCtx, statusCode, 0) // Content length not easily available in Lift
			tracingManager.AddAnnotation(traceCtx, "status_code", statusCode)

			// Record error if any
			if err != nil {
				tracingManager.AddError(traceCtx, err, false)
			}

			return resp, err
		}
	}
}

// recordLatencyMetric records latency metrics to DynamoDB using DynamORM
func recordLatencyMetric(ctx context.Context, metricType, operation string, duration time.Duration, dimensions map[string]string) error {
	if repos == nil {
		return errors.ServiceInitializationFailed("repositories", nil)
	}

	// Create metric record
	metric := &models.MetricRecord{
		MetricType:       metricType,
		ServiceName:      "api",
		Timestamp:        time.Now(),
		AggregationLevel: "raw",
		Unit:             "ms",
		Dimensions:       dimensions,
		// Statistical values for single measurement
		Count: 1,
		Sum:   float64(duration.Milliseconds()),
		Min:   float64(duration.Milliseconds()),
		Max:   float64(duration.Milliseconds()),
		P50:   float64(duration.Milliseconds()),
		P95:   float64(duration.Milliseconds()),
		P99:   float64(duration.Milliseconds()),
	}

	// Add operation to dimensions
	metric.AddDimension("operation", operation)

	// Store to DynamoDB via metrics repository
	return repos.MetricRecord().CreateMetricRecord(ctx, metric)
}

// createCentralizedCostTrackingMiddleware creates centralized cost tracking middleware using the TrackingService
func createCentralizedCostTrackingMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if costTrackingService == nil {
				return next(ctx)
			}

			startTime := time.Now()

			// Execute request
			resp, err := next(ctx)

			// Track Lambda execution cost
			duration := time.Since(startTime)
			memoryMB := int64(128) // Default Lambda memory, could be extracted from env
			if mem, parseErr := common.GetLambdaMemorySizeInt(); parseErr == nil && mem > 0 {
				memoryMB = int64(mem)
			}

			// Create Lambda operation for cost tracking
			lambdaOp := cost.LambdaOperation{
				FunctionName: "api",
				Duration:     duration,
				MemoryMB:     memoryMB,
				Timestamp:    startTime,
			}

			// Track the operation asynchronously to avoid blocking the response
			go func() {
				if trackErr := trackLambdaInvocation(context.Background(), costTrackingService, lambdaOp); trackErr != nil {
					logger.Warn("failed to track Lambda cost", zap.Error(trackErr))
				}
			}()

			return resp, err
		}
	}
}
