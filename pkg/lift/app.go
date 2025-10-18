// Package lift provides application configuration and initialization utilities for the Lift serverless framework.
package lift

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	appconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

// AppConfig defines configuration options for the Lift application
type AppConfig struct {
	Debug              bool
	Timeout            time.Duration
	EnableCORS         bool
	EnableMetrics      bool
	EnableCostTracking bool
	AuthRequired       bool
	TenantRequired     bool
	MetricsConfig      *MetricsConfig
	AWSConfig          *aws.Config
}

// DefaultConfig returns a default configuration suitable for most HTTP APIs
func DefaultConfig() AppConfig {
	globalCfg := appconfig.Get()
	return AppConfig{
		Debug:              globalCfg.DebugMode,
		Timeout:            30 * time.Second,
		EnableCORS:         true,
		EnableMetrics:      true,
		EnableCostTracking: true,
		AuthRequired:       false,
		TenantRequired:     false,
	}
}

// AppBuilder provides a fluent interface for building Lift applications
type AppBuilder struct {
	config            AppConfig
	app               *lift.App
	logger            *zap.Logger
	metricsMiddleware *MetricsMiddleware
}

// NewAppBuilder creates a new application builder with the given configuration and logger
func NewAppBuilder(config AppConfig, logger *zap.Logger) *AppBuilder {
	var app *lift.App
	if config.Debug {
		app = lift.New(lift.WithDebug())
	} else {
		app = lift.New()
	}

	builder := &AppBuilder{
		config: config,
		app:    app,
		logger: logger,
	}

	// Initialize metrics middleware if enabled and AWS config is provided
	if config.EnableMetrics && config.AWSConfig != nil {
		metricsConfig := DefaultMetricsConfig()
		if config.MetricsConfig != nil {
			metricsConfig = *config.MetricsConfig
		}
		builder.metricsMiddleware = NewMetricsMiddleware(*config.AWSConfig, metricsConfig, logger)
	}

	return builder
}

// WithStandardMiddleware adds the standard middleware stack in the correct order
// This matches the existing cmd/api/main.go implementation
func (ab *AppBuilder) WithStandardMiddleware() *AppBuilder {
	// Timeout middleware (first - to wrap all other middleware)
	if ab.config.Timeout > 0 {
		ab.app.Use(middleware.TimeoutMiddleware(middleware.TimeoutConfig{
			DefaultTimeout: ab.config.Timeout,
		}))
	}

	// Metrics middleware (early in stack to capture all metrics)
	if ab.config.EnableMetrics && ab.metricsMiddleware != nil {
		ab.app.Use(ab.metricsMiddleware.Middleware())
	}

	// Custom logging middleware (matches existing pattern)
	ab.app.Use(ab.createLoggingMiddleware())

	// CORS middleware (matches existing pattern)
	if ab.config.EnableCORS {
		ab.app.Use(ab.createCORSMiddleware())
	}

	// Cost tracking middleware (if enabled)
	if ab.config.EnableCostTracking {
		ab.app.Use(ab.createCostTrackingMiddleware())
	}

	return ab
}

// Build returns the configured Lift application
func (ab *AppBuilder) Build() *lift.App {
	return ab.app
}

// GetMetricsMiddleware returns the metrics middleware instance for manual control
func (ab *AppBuilder) GetMetricsMiddleware() *MetricsMiddleware {
	return ab.metricsMiddleware
}

// createLoggingMiddleware creates a logging middleware that matches the existing pattern
func (ab *AppBuilder) createLoggingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()

			// Process the request
			err := next.Handle(ctx)

			// Log the request after processing (matches existing pattern)
			ab.logger.Info("API request",
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.Int("status", ctx.Response.StatusCode),
				zap.Duration("duration", time.Since(start)),
				zap.String("request_id", ctx.GetRequestID()))

			return err
		})
	}
}

// createCORSMiddleware creates a CORS middleware that matches the existing pattern
func (ab *AppBuilder) createCORSMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers (matches existing pattern)
			ctx.Response.Header("Access-Control-Allow-Origin", "*")
			ctx.Response.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
			ctx.Response.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto, X-CSRF-Token")

			// Handle OPTIONS requests
			if ctx.Request.Method == "OPTIONS" {
				return ctx.Status(200).Text("")
			}

			// Process the request
			return next.Handle(ctx)
		})
	}
}

// createCostTrackingMiddleware creates a cost tracking middleware that integrates with existing pkg/cost infrastructure
func (ab *AppBuilder) createCostTrackingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Initialize cost tracking context
			tracker := cost.NewWithRequest(ctx.GetRequestID(), "api_request")

			// Store the tracker in the Lift context for easy access
			ctx.Set("cost_tracker", tracker)

			// Track Lambda invocation
			start := time.Now()

			// Process request
			err := next.Handle(ctx)

			// Calculate Lambda execution cost
			duration := time.Since(start)
			memoryMB := int64(128) // Default Lambda memory, could be configurable
			tracker.TrackLambdaInvocation(duration.Milliseconds(), memoryMB)

			// Calculate and log costs
			operationCost := tracker.CalculateCost()
			if operationCost.TotalCostMicroCents > 0 {
				ab.logger.Info("request_costs",
					zap.String("request_id", ctx.GetRequestID()),
					zap.Int64("total_cost_microcents", operationCost.TotalCostMicroCents),
					zap.Int64("dynamodb_reads", operationCost.DynamoDBReads),
					zap.Int64("dynamodb_writes", operationCost.DynamoDBWrites),
					zap.Int64("lambda_invocations", operationCost.LambdaInvocations),
					zap.Int64("lambda_duration_ms", operationCost.LambdaDurationMs),
					zap.Int64("s3_gets", operationCost.S3Gets),
					zap.Int64("s3_puts", operationCost.S3Puts),
				)
			}

			return err
		})
	}
}

// Convenience functions for common Lambda patterns

// NewHTTPApp creates a new HTTP API application with standard middleware
func NewHTTPApp(config AppConfig, logger *zap.Logger) *lift.App {
	return NewAppBuilder(config, logger).
		WithStandardMiddleware().
		Build()
}

// NewHTTPAppWithBuilder creates a new HTTP API application and returns both app and builder
func NewHTTPAppWithBuilder(config AppConfig, logger *zap.Logger) (*lift.App, *AppBuilder) {
	builder := NewAppBuilder(config, logger).WithStandardMiddleware()
	return builder.Build(), builder
}

// NewSQSApp creates a new SQS application with appropriate middleware (no CORS needed)
func NewSQSApp(config AppConfig, logger *zap.Logger) *lift.App {
	config.EnableCORS = false // Not needed for SQS
	return NewAppBuilder(config, logger).
		WithStandardMiddleware().
		Build()
}

// NewSQSAppWithBuilder creates a new SQS application and returns both app and builder
func NewSQSAppWithBuilder(config AppConfig, logger *zap.Logger) (*lift.App, *AppBuilder) {
	config.EnableCORS = false // Not needed for SQS
	builder := NewAppBuilder(config, logger).WithStandardMiddleware()
	return builder.Build(), builder
}

// NewDynamoDBStreamApp creates a new DynamoDB stream application with appropriate middleware
func NewDynamoDBStreamApp(config AppConfig, logger *zap.Logger) *lift.App {
	config.EnableCORS = false   // Not needed for DynamoDB streams
	config.AuthRequired = false // Streams don't need auth
	return NewAppBuilder(config, logger).
		WithStandardMiddleware().
		Build()
}

// NewDynamoDBStreamAppWithBuilder creates a new DynamoDB stream application and returns both app and builder
func NewDynamoDBStreamAppWithBuilder(config AppConfig, logger *zap.Logger) (*lift.App, *AppBuilder) {
	config.EnableCORS = false   // Not needed for DynamoDB streams
	config.AuthRequired = false // Streams don't need auth
	builder := NewAppBuilder(config, logger).WithStandardMiddleware()
	return builder.Build(), builder
}
