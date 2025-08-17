package patterns

import (
	"fmt"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// RequestIDMiddleware creates middleware that ensures every request has a unique ID
func RequestIDMiddleware(serviceName string) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := ctx.GetRequestID()
			if err := common.ValidateRequiredParam("requestID", requestID); err != nil {
				requestID = fmt.Sprintf("%s-%d", serviceName, time.Now().UnixNano())
				ctx.Set("requestID", requestID)
			}
			return next.Handle(ctx)
		})
	}
}

// LoggingMiddleware creates middleware that logs request start and completion
func LoggingMiddleware(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.GetRequestID()

			// Log request start
			logger.Debug("request started",
				zap.String("request_id", requestID),
				zap.String("path", ctx.Request.Path),
				zap.String("method", ctx.Request.Method),
			)

			// Execute the handler
			err := next.Handle(ctx)

			// Calculate duration
			duration := time.Since(start)

			// Log completion
			if err != nil {
				logger.Error("request failed",
					zap.String("request_id", requestID),
					zap.Error(err),
					zap.Duration("duration", duration),
				)
			} else {
				logger.Info("request completed",
					zap.String("request_id", requestID),
					zap.Duration("duration", duration),
				)
			}

			return err
		})
	}
}

// RecoveryMiddleware creates middleware that recovers from panics
func RecoveryMiddleware(logger *zap.Logger) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					requestID := ctx.GetRequestID()

					logger.Error("panic in request handler",
						zap.String("request_id", requestID),
						zap.Any("panic", r),
						zap.Stack("stack"),
					)

					// Convert panic to error
					if err := ctx.Status(500).JSON(map[string]interface{}{
						"error":      "Internal server error",
						"request_id": requestID,
					}); err != nil {
						logger.Error("failed to send panic error response", zap.Error(err))
					}
				}
			}()

			return next.Handle(ctx)
		})
	}
}

// MetricsMiddleware creates middleware that tracks request metrics
func MetricsMiddleware(serviceName string, metricsSender MetricsSender) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()

			// Execute the handler
			err := next.Handle(ctx)

			// Calculate duration
			duration := time.Since(start)

			// Send metrics
			if metricsSender != nil {
				status := "success"
				if err != nil {
					status = "error"
				}

				if err := metricsSender.SendMetric(MetricData{
					Name:      fmt.Sprintf("%s.request", serviceName),
					Value:     float64(duration.Milliseconds()),
					Unit:      "milliseconds",
					Timestamp: time.Now(),
					Tags: map[string]string{
						"service": serviceName,
						"status":  status,
					},
				}); err != nil {
					// Log metric send failure but don't affect request
					zap.L().Warn("failed to send request metrics", zap.Error(err))
				}
			}

			return err
		})
	}
}

// MetricsSender is an interface for sending metrics
type MetricsSender interface {
	SendMetric(metric MetricData) error
}

// MetricData represents a single metric
type MetricData struct {
	Name      string
	Value     float64
	Unit      string
	Timestamp time.Time
	Tags      map[string]string
}
