// Package middleware provides common middleware for Lambda functions
package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// PanicRecovery creates a middleware that recovers from panics and returns a proper error response
func PanicRecovery(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					// Get stack trace
					stack := debug.Stack()

					// Generate request ID for tracking
					requestID := ctx.Header("X-Request-Id")
					if requestID == "" {
						requestID = common.GenerateRequestIDULID()
					}

					// Log the panic with full context
					logger.Error("panic recovered",
						zap.Any("panic", r),
						zap.String("request_id", requestID),
						zap.String("path", ctx.Request.Path),
						zap.String("method", ctx.Request.Method),
						zap.ByteString("stack", stack),
					)

					// Return a proper error response
					err = ctx.Status(500).JSON(map[string]interface{}{
						"error":             "internal_server_error",
						"error_description": "An unexpected error occurred",
						"request_id":        requestID,
					})
				}
			}()

			// Call the next handler
			return next.Handle(ctx)
		})
	}
}

// PanicRecoveryWithMetrics creates a panic recovery middleware that also records metrics
func PanicRecoveryWithMetrics(logger *zap.Logger, metrics MetricsRecorder) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					// Get stack trace
					stack := debug.Stack()

					// Generate request ID for tracking
					requestID := ctx.Header("X-Request-Id")
					if requestID == "" {
						requestID = common.GenerateRequestIDULID()
					}

					// Log the panic with full context
					logger.Error("panic recovered",
						zap.Any("panic", r),
						zap.String("request_id", requestID),
						zap.String("path", ctx.Request.Path),
						zap.String("method", ctx.Request.Method),
						zap.ByteString("stack", stack),
					)

					// Record panic metric
					if metrics != nil {
						metrics.RecordPanic(ctx.Request.Path, fmt.Sprintf("%v", r))
					}

					// Send alert for critical panics
					if shouldAlert(r) {
						sendPanicAlert(logger, requestID, r, stack)
					}

					// Return a proper error response
					err = ctx.Status(500).JSON(map[string]interface{}{
						"error":             "internal_server_error",
						"error_description": "An unexpected error occurred",
						"request_id":        requestID,
					})
				}
			}()

			// Call the next handler
			return next.Handle(ctx)
		})
	}
}

// MetricsRecorder interface for recording panic metrics
type MetricsRecorder interface {
	RecordPanic(path string, panicValue string)
}

// shouldAlert determines if a panic should trigger an alert
func shouldAlert(panicValue interface{}) bool {
	// Alert on all panics except known recoverable ones
	panicStr := fmt.Sprintf("%v", panicValue)

	// Don't alert on context cancellation panics
	if panicStr == "context canceled" {
		return false
	}

	// Alert on all other panics
	return true
}

// sendPanicAlert sends an alert for critical panics
func sendPanicAlert(logger *zap.Logger, requestID string, panicValue interface{}, stack []byte) {
	// In a production system, this would send to PagerDuty, Slack, etc.
	logger.Error("CRITICAL: Panic alert",
		zap.String("severity", "critical"),
		zap.String("request_id", requestID),
		zap.Any("panic_value", panicValue),
		zap.ByteString("stack_trace", stack),
	)
}
