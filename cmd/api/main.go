package main

/*
Lesser API Server - Mastodon-compatible ActivityPub implementation

This Lambda function serves the Lesser API using AWS API Gateway v2.
All routing is handled by the Lift framework.

The API Gateway configuration strips the /api/v1 and /api/v2 prefixes
before passing requests to this Lambda, so the router receives clean paths.
*/

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	liftHandlers "github.com/equaltoai/lesser/cmd/api/lift"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	liftAuth "github.com/equaltoai/lesser/pkg/lift"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

var (
	cfg               *config.Config
	repos             core.RepositoryStorage
	logger            *zap.Logger
	liftHandler       *liftHandlers.Handler
	authService       *auth.AuthService
	emfMetrics        *observability.EMFMetrics
	healthChecker     *observability.HealthChecker
	metricsCollector  *observability.MetricsCollector
	tracingManager    *observability.TracingManager
	startTime         time.Time
)

func init() {
	startTime = time.Now()
	cfg = config.Get()
	logger = common.Logger()

	// Initialize DynamORM
	tableName := os.Getenv("DYNAMODB_TABLE")
	if tableName == "" {
		tableName = cfg.DynamoTableName
	}
	if tableName == "" {
		logger.Fatal("DYNAMODB_TABLE environment variable is required")
	}

	// Load AWS configuration
	awsCfg, err := awsConfig.LoadDefaultConfig(context.Background(),
		awsConfig.WithRegion(cfg.Region),
	)
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Create repository storage using new factory pattern with AWS config
	repos, err = factory.NewRepositoryFactory(db, tableName, awsCfg, logger)
	if err != nil {
		logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	// Initialize auth service
	authService, err = auth.NewAuthService(repos)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	// Initialize Lift-native auth service (not currently used but initialized for future use)
	_ = liftAuth.NewLiftAuthService(authService)

	// Validate VAPID keys in production environment
	if err := liftHandlers.ValidateVAPIDKeysForProduction(context.Background(), repos, logger); err != nil {
		logger.Fatal("VAPID keys validation failed in production", zap.Error(err))
	}

	// Initialize observability services
	if os.Getenv("DISABLE_METRICS") != envTrue {
		// EMF Metrics for CloudWatch integration
		emfMetrics = observability.NewEMFMetrics(logger, "Lesser/API", "api")
		emfMetrics.AddDimension(observability.DimensionService, "api")
		emfMetrics.AddDimension(observability.DimensionEnvironment, cfg.Stage)
		emfMetrics.AddDimension(observability.DimensionRegion, cfg.Region)
		
		// Legacy metrics collector (if needed)
		cloudWatchClient := cloudwatch.NewFromConfig(awsCfg)
		metricsCollector = observability.NewMetricsCollector(cloudWatchClient, "Lesser/API", logger)
		
		// Health checker
		healthConfig := &observability.HealthConfig{
			TableName:        tableName,
			CheckTimeout:     5 * time.Second,
			CacheTimeout:     30 * time.Second,
			DependencyChecks: true,
		}
		healthChecker = observability.NewHealthChecker(logger, awsCfg, "api", cfg.Version, healthConfig)
		
		// Distributed tracing
		tracingConfig := &observability.TracingConfig{
			ServiceName:    "lesser-api",
			ServiceVersion: cfg.Version,
			SamplingRate:   observability.TracingSampleRatePercent / 100.0,
			Enabled:        os.Getenv("XRAY_TRACING_ENABLED") != "false",
		}
		tracingManager = observability.NewTracingManager(logger, tracingConfig)
		
		logger.Info("initialized observability services",
			zap.String("emf_enabled", "true"),
			zap.String("health_checks_enabled", "true"),
			zap.String("tracing_enabled", fmt.Sprintf("%t", tracingManager.IsEnabled())),
			zap.String("metrics_namespace", "Lesser/API"))
	}

	// Create handler with all dependencies
	legacyAuthMiddleware, err := auth.GetMiddleware()
	if err != nil {
		logger.Fatal("failed to initialize legacy auth middleware", zap.Error(err))
	}

	// Create Lift handler for all endpoints
	// The handler uses repos which implements RepositoryStorage
	liftHandler = liftHandlers.NewHandler(cfg, repos, logger, legacyAuthMiddleware)
}

func main() {
	// Create a new Lift application
	app := lift.New()
	if os.Getenv("DEBUG") == "true" {
		app = lift.New(lift.WithDebug())
	}

	// Add global middleware
	// Add timeout middleware
	app.Use(middleware.TimeoutMiddleware(middleware.TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
	}))

	// Add custom logging middleware
	app.Use(createLoggingMiddleware(logger))

	// Add CORS middleware
	app.Use(createCORSMiddleware())

	// Add cost tracking middleware
	app.Use(createCostTrackingMiddleware(logger))

	// Add distributed tracing middleware
	if tracingManager != nil {
		app.Use(createTracingMiddleware())
	}

	// Add EMF-based performance monitoring middleware
	if emfMetrics != nil {
		app.Use(createEMFMetricsMiddleware())
	}

	// Add rate limiting middleware (before routes)
	if os.Getenv("DISABLE_RATE_LIMITING") != "true" {
		app.Use(ratelimit.Middleware(repos, nil)) // Use default config
		logger.Info("enabled rate limiting middleware")
	}

	// Configure native Lift routes
	configureLiftRoutes(app)
	
	// Configure health check routes if available
	if healthChecker != nil {
		configureHealthRoutes(app)
	}

	// Start the Lambda handler with observability
	lambdaHandler := func(ctx context.Context, event interface{}) (interface{}, error) {
		requestStart := time.Now()
		
		// Record cold start metric if this is a cold start
		if time.Since(startTime) < 30*time.Second && emfMetrics != nil {
			emfMetrics.RecordBusinessMetric(observability.MetricColdStarts, 1.0, observability.UnitCount, nil)
			coldStartDuration := time.Since(startTime)
			emfMetrics.RecordBusinessMetric(observability.MetricColdStartDuration, float64(coldStartDuration.Milliseconds()), observability.UnitMilliseconds, nil)
		}

		// Process the request
		result, err := app.HandleRequest(ctx, event)
		
		// Record request metrics
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

		// CRITICAL: Flush all metrics before Lambda terminates
		// This ensures all metrics are written to CloudWatch
		if emfMetrics != nil {
			emfMetrics.Flush()
		}
		
		if metricsCollector != nil {
			metricsCollector.Flush()
		}

		return result, err
	}

	lambda.Start(lambdaHandler)
}

// createEMFMetricsMiddleware creates middleware for EMF metrics collection
func createEMFMetricsMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if emfMetrics == nil {
				return next.Handle(ctx)
			}

			// Start latency timer
			timer := emfMetrics.StartLatencyTimer(ctx.Context, "endpoint_request")
			
			// Extract endpoint information
			method := "GET"
			path := "/"
			if ctx.Request != nil && ctx.Request.Request != nil {
				method = ctx.Request.Request.Method
				path = ctx.Request.Request.Path
			}
			endpoint := method + " " + path
			dimensions := map[string]string{
				observability.DimensionEndpoint:    path,
				observability.DimensionMethod:      method,
			}

			// Execute request
			err := next.Handle(ctx)

			// Record metrics based on result
			// Default to 200 if no error
			statusCode := 200
			if err != nil {
				// Infer status from error (this is approximate)
				statusCode = 500
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

			return err
		})
	}
}

// configureHealthRoutes adds health check endpoints to the Lift app
func configureHealthRoutes(app *lift.App) {
	// Liveness endpoint
	_ = app.GET("/health/live", func(ctx *lift.Context) error {
		response := map[string]interface{}{
			"status":    observability.HealthStatusHealthy,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
		}
		return ctx.Status(200).JSON(response)
	})

	// Readiness endpoint  
	_ = app.GET("/health/ready", func(ctx *lift.Context) error {
		// Basic readiness check - can we access our dependencies?
		status := observability.HealthStatusHealthy
		checks := []map[string]interface{}{}

		// Check DynamoDB connectivity
		checkCtx, cancel := context.WithTimeout(ctx.Context, 5*time.Second)
		defer cancel()
		
		start := time.Now()
		_, err := repos.Account().GetUser(checkCtx, "health-check-user")
		duration := time.Since(start)
		
		dbCheck := map[string]interface{}{
			"name":      "dynamodb",
			"status":    observability.HealthStatusHealthy,
			"duration":  duration.Milliseconds(),
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

		return ctx.Status(statusCode).JSON(response)
	})

	// Detailed health endpoint
	_ = app.GET("/health/detailed", func(ctx *lift.Context) error {
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
			"name":         "runtime",
			"status":       observability.HealthStatusHealthy,
			"message":      "Service runtime is healthy",
			"uptime_ms":    time.Since(startTime).Milliseconds(),
			"service":      "api",
			"version":      cfg.Version,
		}
		checks = append(checks, runtimeCheck)
		summary["healthy_checks"] = summary["healthy_checks"].(int) + 1

		// Database detailed check
		checkCtx, cancel := context.WithTimeout(ctx.Context, 10*time.Second)
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

		return ctx.Status(statusCode).JSON(response)
	})
}

// createTracingMiddleware creates middleware for distributed tracing
func createTracingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if tracingManager == nil || !tracingManager.IsEnabled() {
				return next.Handle(ctx)
			}

			// Start tracing segment
			traceCtx, segment := tracingManager.StartSegment(ctx.Context, "api-request")
			defer func() {
				if segment != nil {
					segment.Close(nil)
				}
			}()

			// Extract request info
			method := "GET"
			url := "/"
			userAgent := ""
			clientIP := ""
			if ctx.Request != nil && ctx.Request.Request != nil {
				method = ctx.Request.Request.Method
				url = ctx.Request.Request.Path
				userAgent = ctx.Request.Request.Headers["User-Agent"]
				// Try to get client IP from headers
				if xForwarded := ctx.Request.Request.Headers["X-Forwarded-For"]; xForwarded != "" {
					clientIP = xForwarded
				}
			}

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
			err := next.Handle(ctx)

			// Record response information
			// Default to 200 if no error, 500 if error
			statusCode := 200
			if err != nil {
				statusCode = 500
			}
			tracingManager.SetHTTPResponse(traceCtx, statusCode, 0) // Content length not easily available in Lift
			tracingManager.AddAnnotation(traceCtx, "status_code", statusCode)

			// Record error if any
			if err != nil {
				tracingManager.AddError(traceCtx, err, false)
			}

			return err
		})
	}
}
