package main

import (
	"time"

	"github.com/aron23/lesser/pkg/cost"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// createLoggingMiddleware creates a custom logging middleware
func createLoggingMiddleware(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()

			// Process the request
			err := next.Handle(ctx)

			// Log the request after processing
			logger.Info("API request",
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.Int("status", ctx.Response.StatusCode),
				zap.Duration("duration", time.Since(start)),
				zap.String("request_id", ctx.GetRequestID()))

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
