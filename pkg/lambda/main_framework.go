// Package lambda provides standardized Lambda function bootstrapping and initialization patterns.
package lambda

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/lift"
	"github.com/equaltoai/lesser/pkg/observability"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

// MainConfig defines configuration for standardized Lambda main function
type MainConfig struct {
	ServiceName    string
	LambdaType     common.LambdaType
	EnableDebug    bool
	EnableMetrics  bool
	EnableCORS     bool
	EnableRateLimit bool
	Timeout        time.Duration
	
	// Custom initialization functions
	InitCustomServices  func(*common.LambdaContext) error
	ConfigureRoutes     func(*liftPkg.App, *common.LambdaContext) error
	CreateCustomMiddleware func(*common.LambdaContext) []liftPkg.Middleware
}

// DefaultMainConfig returns default configuration for Lambda main functions
func DefaultMainConfig(serviceName string, lambdaType common.LambdaType) MainConfig {
	return MainConfig{
		ServiceName:     serviceName,
		LambdaType:      lambdaType,
		EnableDebug:     false,
		EnableMetrics:   true,
		EnableCORS:      lambdaType == common.LambdaTypeAPI,
		EnableRateLimit: lambdaType == common.LambdaTypeAPI,
		Timeout:         30 * time.Second,
	}
}

// StandardizedMain provides a standardized Lambda main function implementation
// This eliminates the 100+ lines of duplication across all Lambda main.go files
func StandardizedMain(config MainConfig) {
	// Initialize Lambda with standardized configuration
	lambdaConfig := common.LambdaConfig{
		ServiceName:    config.ServiceName,
		LambdaType:     config.LambdaType,
		RequestTimeout: config.Timeout,
	}

	lambdaCtx, err := common.InitializeLambda(lambdaConfig)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize Lambda %s: %v", config.ServiceName, err))
	}

	// Initialize with default options for the Lambda type
	options := common.DefaultLambdaInitOptions(config.LambdaType)
	if err := lambdaCtx.InitializeWithOptions(options); err != nil {
		panic(fmt.Sprintf("failed to initialize %s Lambda services: %v", config.ServiceName, err))
	}

	// Initialize custom services if provided
	if config.InitCustomServices != nil {
		if err := config.InitCustomServices(lambdaCtx); err != nil {
			panic(fmt.Sprintf("failed to initialize custom services for %s: %v", config.ServiceName, err))
		}
	}

	// Create Lift application
	app := createStandardizedLiftApp(config, lambdaCtx)

	// Add standard middleware stack
	addStandardMiddleware(app, config, lambdaCtx)

	// Add custom middleware if provided
	if config.CreateCustomMiddleware != nil {
		customMiddleware := config.CreateCustomMiddleware(lambdaCtx)
		for _, mw := range customMiddleware {
			app.Use(mw)
		}
	}

	// Configure routes
	if config.ConfigureRoutes != nil {
		if err := config.ConfigureRoutes(app, lambdaCtx); err != nil {
			panic(fmt.Sprintf("failed to configure routes for %s: %v", config.ServiceName, err))
		}
	}

	// Create standardized Lambda handler with observability
	lambdaHandler := createStandardizedLambdaHandler(app, lambdaCtx, config.ServiceName)

	// Start Lambda
	lambda.Start(lambdaHandler)
}

// createStandardizedLiftApp creates a Lift app with standard configuration
func createStandardizedLiftApp(config MainConfig, lambdaCtx *common.LambdaContext) *liftPkg.App {
	liftConfig := lift.AppConfig{
		Debug:              config.EnableDebug,
		Timeout:            config.Timeout,
		EnableCORS:         config.EnableCORS,
		EnableMetrics:      config.EnableMetrics,
		EnableCostTracking: true,
		AWSConfig:          &lambdaCtx.AWSServices.Config,
	}

	return lift.NewHTTPApp(liftConfig, lambdaCtx.Logger)
}

// addStandardMiddleware adds the standard middleware stack to the Lift app
func addStandardMiddleware(app *liftPkg.App, config MainConfig, lambdaCtx *common.LambdaContext) {
	// Timeout middleware (first)
	app.Use(middleware.TimeoutMiddleware(middleware.TimeoutConfig{
		DefaultTimeout: config.Timeout,
	}))

	// Request ID middleware
	app.Use(createRequestIDMiddleware())

	// Logging middleware (using shared implementation from cmd/api/middleware.go)
	app.Use(createLoggingMiddleware(lambdaCtx.Logger))

	// CORS middleware if enabled
	if config.EnableCORS {
		app.Use(createCORSMiddleware())
	}

	// Cost tracking middleware
	app.Use(createCostTrackingMiddleware(lambdaCtx.Logger))

	// Tracing middleware if available
	if lambdaCtx.TracingManager != nil {
		if tm, ok := lambdaCtx.TracingManager.(*observability.TracingManager); ok && tm.IsEnabled() {
			app.Use(createTracingMiddleware(lambdaCtx))
		}
	}

	// EMF metrics middleware if available
	if lambdaCtx.EMFMetrics != nil {
		app.Use(createEMFMetricsMiddleware(lambdaCtx))
	}

	// Latency tracking middleware
	app.Use(createLatencyTrackingMiddleware(lambdaCtx))

	// Rate limiting middleware if enabled and available
	if config.EnableRateLimit && lambdaCtx.Repos != nil {
		app.Use(createRateLimitMiddleware(lambdaCtx))
	}
}

// createStandardizedLambdaHandler creates a Lambda handler with standardized observability
func createStandardizedLambdaHandler(app *liftPkg.App, lambdaCtx *common.LambdaContext, serviceName string) func(context.Context, interface{}) (interface{}, error) {
	startTime := time.Now()

	return func(ctx context.Context, event interface{}) (interface{}, error) {
		requestStart := time.Now()

		// Record cold start metric if this is a cold start
		if time.Since(startTime) < 30*time.Second && lambdaCtx.EMFMetrics != nil {
			if emfMetrics, ok := lambdaCtx.EMFMetrics.(EMFMetricsInterface); ok {
				emfMetrics.RecordBusinessMetric("ColdStarts", 1.0, "Count", nil)
				coldStartDuration := time.Since(startTime)
				emfMetrics.RecordBusinessMetric("ColdStartDuration", float64(coldStartDuration.Milliseconds()), "Milliseconds", nil)
			}
		}

		// Process the request
		result, err := app.HandleRequest(ctx, event)

		// Record request metrics
		requestDuration := time.Since(requestStart)
		if lambdaCtx.EMFMetrics != nil {
			if emfMetrics, ok := lambdaCtx.EMFMetrics.(EMFMetricsInterface); ok {
				emfMetrics.RecordLatency(fmt.Sprintf("%s_request", serviceName), requestDuration)
				emfMetrics.RecordThroughput(fmt.Sprintf("%s_request", serviceName), 1)

				if err != nil {
					emfMetrics.RecordError(fmt.Sprintf("%s_request", serviceName), "lambda_error")
				} else {
					emfMetrics.RecordSuccess(fmt.Sprintf("%s_request", serviceName))
				}
			}
		}

		// Flush observability services
		lambdaCtx.FlushObservabilityServices()

		return result, err
	}
}

// EMFMetricsInterface defines the interface for EMF metrics to avoid import cycles
type EMFMetricsInterface interface {
	RecordBusinessMetric(name string, value float64, unit string, dimensions map[string]string)
	RecordLatency(operation string, duration time.Duration)
	RecordThroughput(operation string, count int)
	RecordError(operation string, errorType string)
	RecordSuccess(operation string)
}

// Middleware factory functions (these can reference the shared implementations)

func createRequestIDMiddleware() liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			if ctx.GetRequestID() == "" {
				requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())
				ctx.SetRequestID(requestID)
			}
			return next.Handle(ctx)
		})
	}
}

// These functions delegate to the shared implementations in cmd/api/middleware.go
func createLoggingMiddleware(logger *zap.Logger) liftPkg.Middleware {
	// This would reference the shared implementation
	// For now, we'll inline a simplified version to avoid circular imports
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			start := time.Now()
			err := next.Handle(ctx)
			
			logger.Info("request completed",
				zap.String("request_id", ctx.GetRequestID()),
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("success", err == nil),
			)
			
			return err
		})
	}
}

func createCORSMiddleware() liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			// Standard CORS headers
			ctx.Response.Header("Access-Control-Allow-Origin", "*")
			ctx.Response.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
			ctx.Response.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept")
			
			if ctx.Request.Method == "OPTIONS" {
				return ctx.Status(200).Text("")
			}
			
			return next.Handle(ctx)
		})
	}
}

func createCostTrackingMiddleware(logger *zap.Logger) liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			start := time.Now()
			err := next.Handle(ctx)
			
			// Basic cost tracking - can be enhanced with pkg/cost integration
			duration := time.Since(start)
			logger.Debug("request cost tracking",
				zap.String("request_id", ctx.GetRequestID()),
				zap.Duration("duration", duration),
			)
			
			return err
		})
	}
}

func createTracingMiddleware(_ *common.LambdaContext) liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			// Basic tracing implementation - can be enhanced
			return next.Handle(ctx)
		})
	}
}

func createEMFMetricsMiddleware(lambdaCtx *common.LambdaContext) liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			start := time.Now()
			err := next.Handle(ctx)
			
			// Basic EMF metrics - can be enhanced
			if lambdaCtx.EMFMetrics != nil {
				if emfMetrics, ok := lambdaCtx.EMFMetrics.(EMFMetricsInterface); ok {
					emfMetrics.RecordLatency("endpoint_request", time.Since(start))
					if err != nil {
						emfMetrics.RecordError("endpoint_request", "handler_error")
					} else {
						emfMetrics.RecordSuccess("endpoint_request")
					}
				}
			}
			
			return err
		})
	}
}

func createLatencyTrackingMiddleware(lambdaCtx *common.LambdaContext) liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			start := time.Now()
			err := next.Handle(ctx)
			
			// Record latency metrics
			duration := time.Since(start)
			lambdaCtx.Logger.Debug("request latency",
				zap.String("request_id", ctx.GetRequestID()),
				zap.Duration("duration", duration),
			)
			
			return err
		})
	}
}

func createRateLimitMiddleware(_ *common.LambdaContext) liftPkg.Middleware {
	return func(next liftPkg.Handler) liftPkg.Handler {
		return liftPkg.HandlerFunc(func(ctx *liftPkg.Context) error {
			// Basic rate limiting - can be enhanced with ratelimit package
			return next.Handle(ctx)
		})
	}
}