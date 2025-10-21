package middleware

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/observability"
)

// ObservabilityConfig holds configuration for observability middleware
type ObservabilityConfig struct {
	Logger  observability.StructuredLogger
	Metrics observability.MetricsCollector
	// Optional: custom operation name extractor
	OperationNameFunc func(*lift.Context) string
}

// ObservabilityMiddleware provides comprehensive logging and metrics collection
func ObservabilityMiddleware(config ObservabilityConfig) lift.Middleware {
	// Set defaults
	config = setObservabilityMiddlewareDefaults(config)

	// Create coordinated handler
	handler := newBasicObservabilityHandler(config)

	return lift.MarkGlobalMiddleware(lift.Middleware(eventAwareMiddleware(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return handler.handle(ctx, next)
		})
	})))
}

// setObservabilityMiddlewareDefaults applies default configuration values
func setObservabilityMiddlewareDefaults(config ObservabilityConfig) ObservabilityConfig {
	if config.OperationNameFunc == nil {
		config.OperationNameFunc = func(ctx *lift.Context) string {
			return fmt.Sprintf("%s_%s", ctx.Request.Method, ctx.Request.Path)
		}
	}
	return config
}

// basicObservabilityHandler coordinates basic logging and metrics
type basicObservabilityHandler struct {
	config  ObservabilityConfig
	logger  *basicLogHandler
	metrics *basicMetricsHandler
}

// newBasicObservabilityHandler creates a new basic observability handler
func newBasicObservabilityHandler(config ObservabilityConfig) *basicObservabilityHandler {
	return &basicObservabilityHandler{
		config:  config,
		logger:  newBasicLogHandler(config),
		metrics: newBasicMetricsHandler(config),
	}
}

// handle processes the request with coordinated observability
func (h *basicObservabilityHandler) handle(ctx *lift.Context, next lift.Handler) error {
	start := time.Now()
	operation := h.config.OperationNameFunc(ctx)

	// Setup logging and metrics
	h.logger.before(ctx)
	h.metrics.before(ctx, operation)

	// Execute handler
	err := next.Handle(ctx)

	// Complete logging and metrics
	duration := time.Since(start)
	h.logger.after(ctx, duration, err)
	h.metrics.after(ctx, operation, duration, err)

	return err
}

// basicLogHandler handles the logging aspect
type basicLogHandler struct {
	config ObservabilityConfig
}

// newBasicLogHandler creates a new basic log handler
func newBasicLogHandler(config ObservabilityConfig) *basicLogHandler {
	return &basicLogHandler{config: config}
}

// before sets up logging context before request processing
func (l *basicLogHandler) before(ctx *lift.Context) {
	if l.config.Logger == nil {
		return
	}

	// Create context logger
	contextLogger := l.config.Logger.
		WithRequestID(ctx.RequestID).
		WithTenantID(ctx.TenantID()).
		WithUserID(ctx.UserID())

	// Add trace context if available
	if traceID := ctx.Request.Headers["X-Trace-Id"]; traceID != "" {
		contextLogger = contextLogger.WithTraceID(traceID)
	}
	if spanID := ctx.Request.Headers["X-Span-Id"]; spanID != "" {
		contextLogger = contextLogger.WithSpanID(spanID)
	}

	ctx.Logger = contextLogger

	// Log request start
	ctx.Logger.Info("Request started", map[string]any{
		"method":     ctx.Request.Method,
		"path":       ctx.Request.Path,
		"query":      "[SANITIZED_QUERY_PARAMS]", // Sanitized for security
		"source_ip":  ctx.Request.Headers["X-Forwarded-For"],
		"user_agent": ctx.Request.Headers["User-Agent"],
	})
}

// after handles logging after request processing
func (l *basicLogHandler) after(ctx *lift.Context, duration time.Duration, err error) {
	if ctx.Logger == nil {
		return
	}

	logFields := map[string]any{
		"duration": duration.String(),
		"status":   ctx.Response.StatusCode,
	}

	if err != nil {
		logFields["error"] = "[SANITIZED_ERROR]" // Sanitized for security
		ctx.Logger.Error("Request failed", logFields)
	} else {
		ctx.Logger.Info("Request completed", logFields)
	}
}

// basicMetricsHandler handles the metrics aspect
type basicMetricsHandler struct {
	config ObservabilityConfig
}

// newBasicMetricsHandler creates a new basic metrics handler
func newBasicMetricsHandler(config ObservabilityConfig) *basicMetricsHandler {
	return &basicMetricsHandler{config: config}
}

// before handles metrics setup before request processing
func (m *basicMetricsHandler) before(ctx *lift.Context, _ string) {
	if m.config.Metrics == nil {
		return
	}

	// Record request count
	tenantMetrics := m.config.Metrics.WithTags(map[string]string{
		"tenant_id": ctx.TenantID(),
		"method":    ctx.Request.Method,
		"path":      ctx.Request.Path,
	})

	counter := tenantMetrics.Counter("requests.total")
	counter.Inc()
}

// after handles metrics after request processing
func (m *basicMetricsHandler) after(ctx *lift.Context, operation string, duration time.Duration, err error) {
	if m.config.Metrics == nil {
		return
	}

	// Record response metrics based on success/error
	if err != nil {
		m.recordErrorMetrics(ctx, duration)
	} else {
		m.recordSuccessMetrics(ctx, duration)
	}

	// Record operation-specific metrics
	m.recordOperationMetrics(operation, duration)
}

// recordErrorMetrics records metrics for error responses
func (m *basicMetricsHandler) recordErrorMetrics(ctx *lift.Context, duration time.Duration) {
	errorMetrics := m.config.Metrics.WithTags(map[string]string{
		"tenant_id": ctx.TenantID(),
		"method":    ctx.Request.Method,
		"path":      ctx.Request.Path,
		"error":     "true",
	})

	errorCounter := errorMetrics.Counter("requests.errors")
	errorCounter.Inc()

	// Record latency even for errors
	histogram := errorMetrics.Histogram("requests.duration")
	histogram.Observe(float64(duration.Milliseconds()))
}

// recordSuccessMetrics records metrics for successful responses
func (m *basicMetricsHandler) recordSuccessMetrics(ctx *lift.Context, duration time.Duration) {
	successMetrics := m.config.Metrics.WithTags(map[string]string{
		"tenant_id": ctx.TenantID(),
		"method":    ctx.Request.Method,
		"path":      ctx.Request.Path,
		"status":    fmt.Sprintf("%d", ctx.Response.StatusCode),
	})

	// Record latency
	histogram := successMetrics.Histogram("requests.duration")
	histogram.Observe(float64(duration.Milliseconds()))

	// Record response size if available
	m.recordResponseSize(ctx, successMetrics)
}

// recordResponseSize calculates and records response size metrics
func (m *basicMetricsHandler) recordResponseSize(ctx *lift.Context, metrics observability.MetricsCollector) {
	if ctx.Response.Body == nil {
		return
	}

	var size int
	switch v := ctx.Response.Body.(type) {
	case string:
		size = len(v)
	case []byte:
		size = len(v)
	default:
		// For other types, marshal to JSON to get size estimate
		if data, marshalErr := json.Marshal(v); marshalErr == nil {
			size = len(data)
		}
	}

	if size > 0 {
		gauge := metrics.Gauge("response.size")
		gauge.Set(float64(size))
	}
}

// recordOperationMetrics records operation-specific metrics
func (m *basicMetricsHandler) recordOperationMetrics(operation string, duration time.Duration) {
	// Record operation-specific counters
	operationCounter := m.config.Metrics.Counter(fmt.Sprintf("operation.%s", operation))
	operationCounter.Inc()

	// Record operation duration
	operationHistogram := m.config.Metrics.Histogram(fmt.Sprintf("operation.%s.duration", operation))
	operationHistogram.Observe(float64(duration.Milliseconds()))
}

// MetricsOnlyMiddleware provides lightweight metrics collection without logging
func MetricsOnlyMiddleware(metrics lift.MetricsCollector) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()

			// Record request
			counter := metrics.Counter("http.requests", map[string]string{
				"method": ctx.Request.Method,
				"path":   ctx.Request.Path,
			})
			counter.Inc()

			// Execute handler
			err := next.Handle(ctx)

			// Record duration
			duration := time.Since(start)
			histogram := metrics.Histogram("http.duration", map[string]string{
				"method": ctx.Request.Method,
				"path":   ctx.Request.Path,
				"status": fmt.Sprintf("%d", ctx.Response.StatusCode),
			})
			histogram.Observe(float64(duration.Milliseconds()))

			// Record errors
			if err != nil {
				errorCounter := metrics.Counter("http.errors", map[string]string{
					"method": ctx.Request.Method,
					"path":   ctx.Request.Path,
				})
				errorCounter.Inc()
			}

			return err
		})
	}
}
