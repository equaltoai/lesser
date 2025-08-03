package main

import (
	"os"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// createLoggingMiddleware creates a custom logging middleware with structured correlation
func createLoggingMiddleware(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.GetRequestID()
			
			// Extract user and tenant context for correlation
			userID := ""
			tenantID := ""
			if claims, ok := ctx.Get("claims").(*auth.Claims); ok && claims != nil {
				userID = claims.Username
			}
			if tenant, ok := ctx.Get("tenantID").(string); ok {
				tenantID = tenant
			}
			
			// Create contextual logger with correlation fields
			contextLogger := logger.With(
				zap.String("request_id", requestID),
				zap.String("user_id", userID),
				zap.String("tenant_id", tenantID),
				zap.String("function_name", os.Getenv("AWS_LAMBDA_FUNCTION_NAME")),
				zap.String("function_version", os.Getenv("AWS_LAMBDA_FUNCTION_VERSION")),
				zap.String("cold_start", os.Getenv("AWS_LAMBDA_INITIALIZATION_TYPE")),
			)
			
			// Store contextual logger in context
			ctx.Set("logger", contextLogger)
			
			// Log request start
			contextLogger.Info("request_start",
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.String("user_agent", ctx.Header("User-Agent")),
				zap.String("remote_addr", ctx.Header("X-Forwarded-For")),
			)

			// Process the request
			err := next.Handle(ctx)
			
			// Calculate execution metrics
			duration := time.Since(start)
			statusCode := ctx.Response.StatusCode
			
			// Log request completion with metrics
			logLevel := zap.InfoLevel
			if err != nil {
				logLevel = zap.ErrorLevel
			} else if statusCode >= 400 {
				logLevel = zap.WarnLevel
			}
			
			contextLogger.Log(logLevel, "request_complete",
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.Int("status", statusCode),
				zap.Duration("duration", duration),
				zap.Bool("success", err == nil && statusCode < 400),
				zap.Error(err),
			)

			return err
		})
	}
}

// createCORSMiddleware creates a CORS middleware
func createCORSMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers
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
func createCostTrackingMiddleware(logger *zap.Logger) lift.Middleware {
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
				logger.Info("request_costs",
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

// createAuthMiddleware creates Lift-native authentication middleware
func createAuthMiddleware() lift.Middleware {
	return liftAuthSvc.RequireAuth()
}

// createAdminMiddleware creates Lift-native admin middleware that checks for admin scope
func createAdminMiddleware() lift.Middleware {
	return liftAuthSvc.RequireScope("admin")
}

// Helper functions for cost tracking

// GetCostTracker retrieves the cost tracker from the Lift context
func GetCostTracker(ctx *lift.Context) *cost.Tracker {
	if tracker, ok := ctx.Get("cost_tracker").(*cost.Tracker); ok {
		return tracker
	}
	return nil
}

// TrackCost is a convenience function to track costs from a Lift context
func TrackCost(ctx *lift.Context, fn func(*cost.Tracker)) {
	if tracker := GetCostTracker(ctx); tracker != nil {
		fn(tracker)
	}
}

// createPerformanceMonitoringMiddleware is deprecated in favor of EMF-based metrics
// Use observability.CreateEMFPerformanceMonitoringMiddleware instead
// This function is kept for backwards compatibility but should not be used in new code
func createPerformanceMonitoringMiddleware(metricsCollector *observability.MetricsCollector) lift.Middleware {
	// This is now a no-op since we've migrated to EMF
	// The EMF middleware handles all performance monitoring without polling
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Just pass through - EMF middleware handles metrics collection
			return next.Handle(ctx)
		})
	}
}

// GetLogger retrieves the contextual logger from the Lift context
func GetLogger(ctx *lift.Context) *zap.Logger {
	if logger, ok := ctx.Get("logger").(*zap.Logger); ok {
		return logger
	}
	return zap.L() // fallback to global logger
}

// createEnhancedLoggingMiddleware is an alias for createLoggingMiddleware to maintain API compatibility
func createEnhancedLoggingMiddleware(logger *zap.Logger) lift.Middleware {
	return createLoggingMiddleware(logger)
}
