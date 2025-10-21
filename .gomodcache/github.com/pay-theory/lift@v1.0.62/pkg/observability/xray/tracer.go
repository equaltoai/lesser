package xray

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/aws/aws-xray-sdk-go/xray"

	"github.com/pay-theory/lift/pkg/lift"
)

// XRayConfig holds configuration for X-Ray tracing
type XRayConfig struct {
	Annotations       map[string]string `json:"annotations"`
	Metadata          map[string]string `json:"metadata"`
	ServiceName       string            `json:"service_name"`
	ServiceVersion    string            `json:"service_version"`
	Environment       string            `json:"environment"`
	SamplingRate      float64           `json:"sampling_rate"`
	EnableSubsegments bool              `json:"enable_subsegments"`
	RecoverPanics     bool              `json:"recover_panics"`
}

// XRayTracer provides X-Ray distributed tracing capabilities
type XRayTracer struct {
	config XRayConfig
}

// NewXRayTracer creates a new X-Ray tracer with the given configuration
func NewXRayTracer(config XRayConfig) *XRayTracer {
	// Set defaults
	if config.ServiceName == "" {
		config.ServiceName = "lift-service"
	}
	if config.SamplingRate == 0 {
		config.SamplingRate = 0.1 // 10% sampling by default
	}
	if config.Annotations == nil {
		config.Annotations = make(map[string]string)
	}
	if config.Metadata == nil {
		config.Metadata = make(map[string]string)
	}

	return &XRayTracer{
		config: config,
	}
}

// XRayMiddleware creates middleware for automatic X-Ray tracing
func XRayMiddleware(config XRayConfig) lift.Middleware {
	tracer := NewXRayTracer(config)
	segmentMgr := newSegmentManager(config)
	panicHandler := newPanicHandler(config)
	annotationMgr := newAnnotationManager(config, tracer)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return segmentMgr.withSegment(ctx, func(segment *xray.Segment) error {
				// Add request annotations and metadata
				annotationMgr.addRequestAnnotations(segment, ctx)
				annotationMgr.addRequestMetadata(segment, ctx)
				annotationMgr.addTraceHeaders(ctx, segment)

				// Execute handler with panic recovery and timing
				start := time.Now()
				err := panicHandler.safeExecute(ctx, segment, next)
				duration := time.Since(start)

				// Add response data
				annotationMgr.addResponseData(segment, ctx, duration, err)

				return err
			})
		})
	}
}

// addStandardAnnotations adds standard annotations to the segment
func (t *XRayTracer) addStandardAnnotations(segment *xray.Segment, ctx *lift.Context) {
	// HTTP information (only if request is not nil)
	if ctx.Request != nil {
		if err := segment.AddAnnotation("http.method", ctx.Request.Method); err != nil {
			// Silently ignore XRay annotation errors
			_ = err
		}
		if err := segment.AddAnnotation("http.url", ctx.Request.Path); err != nil {
			// Silently ignore XRay annotation errors
			_ = err
		}
	}

	// Multi-tenant information
	if tenantID := ctx.TenantID(); tenantID != "" {
		if err := segment.AddAnnotation("tenant_id", tenantID); err != nil {
			// Silently ignore XRay annotation errors
			_ = err
		}
	}
	if userID := ctx.UserID(); userID != "" {
		if err := segment.AddAnnotation("user_id", userID); err != nil {
			// Silently ignore XRay annotation errors
			_ = err
		}
	}

	// Request information
	if ctx.RequestID != "" {
		if err := segment.AddAnnotation("request_id", ctx.RequestID); err != nil {
			// Silently ignore XRay annotation errors
			_ = err
		}
	}

	// Service information
	if err := segment.AddAnnotation("service.name", t.config.ServiceName); err != nil {
		// Silently ignore XRay annotation errors
		_ = err
	}
	if t.config.ServiceVersion != "" {
		if err := segment.AddAnnotation("service.version", t.config.ServiceVersion); err != nil {
			// Silently ignore XRay annotation errors
			_ = err
		}
	}
	if t.config.Environment != "" {
		if err := segment.AddAnnotation("environment", t.config.Environment); err != nil {
			// Silently ignore XRay annotation errors
			_ = err
		}
	}
}

// addStandardMetadata adds standard metadata to the segment
func (t *XRayTracer) addStandardMetadata(segment *xray.Segment, ctx *lift.Context) {
	if ctx.Request == nil {
		return // Skip if request is nil
	}

	// HTTP metadata
	httpMetadata := map[string]any{
		"method": ctx.Request.Method,
		"path":   ctx.Request.Path,
	}

	// Add query params if not nil
	if ctx.Request.QueryParams != nil {
		httpMetadata["query_params"] = ctx.Request.QueryParams
	}

	// Add filtered headers if headers exist
	if ctx.Request.Headers != nil {
		httpMetadata["headers"] = filterSensitiveHeaders(ctx.Request.Headers)
	}

	if err := segment.AddMetadata("http", httpMetadata); err != nil {
		// Silently ignore XRay metadata errors
		_ = err
	}

	// Request metadata
	requestMetadata := map[string]any{
		"request_id":   ctx.RequestID,
		"tenant_id":    ctx.TenantID(),
		"user_id":      ctx.UserID(),
		"trigger_type": ctx.Request.TriggerType,
	}
	if err := segment.AddMetadata("lift", requestMetadata); err != nil {
		// Silently ignore XRay metadata errors
		_ = err
	}

	// Service metadata
	serviceMetadata := map[string]any{
		"name":        t.config.ServiceName,
		"version":     t.config.ServiceVersion,
		"environment": t.config.Environment,
	}
	if err := segment.AddMetadata("service", serviceMetadata); err != nil {
		// Silently ignore XRay metadata errors
		_ = err
	}
}

// filterSensitiveHeaders removes sensitive headers from tracing
func filterSensitiveHeaders(headers map[string]string) map[string]string {
	filtered := make(map[string]string)
	sensitiveHeaders := map[string]bool{
		"authorization": true,
		"cookie":        true,
		"x-api-key":     true,
		"x-auth-token":  true,
	}

	for key, value := range headers {
		if sensitiveHeaders[key] {
			filtered[key] = "[REDACTED]"
		} else {
			filtered[key] = value
		}
	}

	return filtered
}

// TraceDynamoDBOperation creates a subsegment for DynamoDB operations
func TraceDynamoDBOperation(ctx context.Context, operation, tableName string) (context.Context, func()) {
	newCtx, subsegment := xray.BeginSubsegment(ctx, fmt.Sprintf("DynamoDB.%s", operation))
	if subsegment == nil {
		// X-Ray not available, return no-op
		return ctx, func() {}
	}

	// Add DynamoDB-specific annotations
	if err := subsegment.AddAnnotation("aws.operation", operation); err != nil {
		// Silently ignore XRay annotation errors
		_ = err
	}
	if err := subsegment.AddAnnotation("aws.table_name", tableName); err != nil {
		// Silently ignore XRay annotation errors
		_ = err
	}
	if err := subsegment.AddAnnotation("aws.service", "DynamoDB"); err != nil {
		// Silently ignore XRay annotation errors
		_ = err
	}

	// Add metadata
	if err := subsegment.AddMetadata("aws", map[string]any{
		"operation":  operation,
		"table_name": tableName,
		"service":    "DynamoDB",
	}); err != nil {
		// Silently ignore XRay metadata errors
		_ = err
	}

	return newCtx, func() {
		subsegment.Close(nil)
	}
}

// TraceHTTPCall creates a subsegment for HTTP calls to other services
func TraceHTTPCall(ctx context.Context, method, url string) (context.Context, func(statusCode int, err error)) {
	newCtx, subsegment := xray.BeginSubsegment(ctx, fmt.Sprintf("HTTP.%s", method))
	if subsegment == nil {
		// X-Ray not available, return no-op
		return ctx, func(int, error) {}
	}

	// Add HTTP-specific annotations
	if err := subsegment.AddAnnotation("http.method", method); err != nil {
		// Silently ignore XRay annotation errors
		_ = err
	}
	if err := subsegment.AddAnnotation("http.url", url); err != nil {
		// Silently ignore XRay annotation errors
		_ = err
	}

	// Add metadata
	if err := subsegment.AddMetadata("http", map[string]any{
		"method": method,
		"url":    url,
	}); err != nil {
		// Silently ignore XRay metadata errors
		_ = err
	}

	return newCtx, func(statusCode int, err error) {
		if statusCode > 0 {
			if annoErr := subsegment.AddAnnotation("http.status_code", statusCode); annoErr != nil {
				// Silently ignore XRay annotation errors
				_ = annoErr
			}
			if metaErr := subsegment.AddMetadata("http", map[string]any{
				"status_code": statusCode,
			}); metaErr != nil {
				// Silently ignore XRay metadata errors
				_ = metaErr
			}
		}

		if err != nil {
			if addErr := subsegment.AddError(err); addErr != nil {
				// Silently ignore XRay errors
				_ = addErr
			}
		}

		subsegment.Close(err)
	}
}

// TraceCustomOperation creates a subsegment for custom operations
func TraceCustomOperation(ctx context.Context, operationName string, metadata map[string]any) (context.Context, func(error)) {
	newCtx, subsegment := xray.BeginSubsegment(ctx, operationName)
	if subsegment == nil {
		// X-Ray not available, return no-op
		return ctx, func(error) {}
	}

	// Add operation name
	if err := subsegment.AddAnnotation("operation", operationName); err != nil {
		// Silently ignore XRay annotation errors
		_ = err
	}

	// Add custom metadata
	if len(metadata) > 0 {
		if err := subsegment.AddMetadata("custom", metadata); err != nil {
			// Silently ignore XRay metadata errors
			_ = err
		}
	}

	return newCtx, func(err error) {
		if err != nil {
			if addErr := subsegment.AddError(err); addErr != nil {
				// Silently ignore XRay errors
				_ = addErr
			}
			if annoErr := subsegment.AddAnnotation("error", "true"); annoErr != nil {
				// Silently ignore XRay annotation errors
				_ = annoErr
			}
		} else {
			if annoErr := subsegment.AddAnnotation("error", "false"); annoErr != nil {
				// Silently ignore XRay annotation errors
				_ = annoErr
			}
		}

		subsegment.Close(err)
	}
}

// GetTraceID extracts the trace ID from the context
func GetTraceID(ctx context.Context) string {
	if segment := xray.GetSegment(ctx); segment != nil {
		return segment.TraceID
	}
	return ""
}

// GetSegmentID extracts the segment ID from the context
func GetSegmentID(ctx context.Context) string {
	if segment := xray.GetSegment(ctx); segment != nil {
		return segment.ID
	}
	return ""
}

// AddAnnotation adds an annotation to the current segment
func AddAnnotation(ctx context.Context, key string, value any) {
	if segment := xray.GetSegment(ctx); segment != nil {
		if err := segment.AddAnnotation(key, value); err != nil {
			// Silently ignore XRay annotation errors
			_ = err
		}
	}
}

// AddMetadata adds metadata to the current segment
func AddMetadata(ctx context.Context, namespace, key string, value any) {
	if segment := xray.GetSegment(ctx); segment != nil {
		if err := segment.AddMetadata(namespace, map[string]any{key: value}); err != nil {
			// Silently ignore XRay metadata errors
			_ = err
		}
	}
}

// SetError marks the current segment as having an error
func SetError(ctx context.Context, err error) {
	if segment := xray.GetSegment(ctx); segment != nil {
		if addErr := segment.AddError(err); addErr != nil {
			// Silently ignore XRay errors
			_ = addErr
		}
	}
}

// xraySegmentManager handles X-Ray segment lifecycle
type xraySegmentManager struct {
	config XRayConfig
}

// newSegmentManager creates a new segment manager
func newSegmentManager(config XRayConfig) *xraySegmentManager {
	return &xraySegmentManager{
		config: config,
	}
}

// withSegment executes a function within an X-Ray segment
func (sm *xraySegmentManager) withSegment(ctx *lift.Context, fn func(*xray.Segment) error) error {
	newCtx, segment := xray.BeginSegment(ctx.Context, sm.config.ServiceName)
	ctx.Context = newCtx

	defer func() {
		if segment != nil {
			segment.Close(nil)
		}
	}()

	return fn(segment)
}

// xrayPanicHandler handles panic recovery for X-Ray middleware
type xrayPanicHandler struct {
	config XRayConfig
}

// newPanicHandler creates a new panic handler
func newPanicHandler(config XRayConfig) *xrayPanicHandler {
	return &xrayPanicHandler{
		config: config,
	}
}

// safeExecute executes a function with panic recovery
func (ph *xrayPanicHandler) safeExecute(ctx *lift.Context, segment *xray.Segment, next lift.Handler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			panicErr := fmt.Errorf("panic in request handler: %v", r)

			// Log to X-Ray if possible
			if segment != nil {
				if xrayErr := segment.AddError(panicErr); xrayErr != nil {
					_ = xrayErr // Silently ignore XRay errors
				}
				segment.Close(panicErr)
			}

			if ph.config.RecoverPanics {
				ph.handlePanicRecovery(ctx, r)
				err = panicErr
			} else {
				panic(r) // Re-panic in development
			}
		}
	}()

	return next.Handle(ctx)
}

// handlePanicRecovery handles panic recovery by setting error response
func (ph *xrayPanicHandler) handlePanicRecovery(ctx *lift.Context, panicValue any) {
	ctx.Response.StatusCode = http.StatusInternalServerError
	ctx.Response.Body = []byte(`{"error":"internal server error"}`)
	ctx.Response.Headers[lift.HeaderContentType] = lift.ContentTypeJSON

	if ctx.Logger != nil {
		ctx.Logger.Error("Recovered from panic", map[string]any{
			"panic": panicValue,
			"stack": string(debug.Stack()),
		})
	}
}

// xrayAnnotationManager handles adding annotations and metadata
type xrayAnnotationManager struct {
	tracer *XRayTracer
	config XRayConfig
}

// newAnnotationManager creates a new annotation manager
func newAnnotationManager(config XRayConfig, tracer *XRayTracer) *xrayAnnotationManager {
	return &xrayAnnotationManager{
		config: config,
		tracer: tracer,
	}
}

// addRequestAnnotations adds standard request annotations
func (am *xrayAnnotationManager) addRequestAnnotations(segment *xray.Segment, ctx *lift.Context) {
	// Add standard annotations
	am.tracer.addStandardAnnotations(segment, ctx)

	// Add custom annotations from config
	if am.config.Annotations != nil {
		for key, value := range am.config.Annotations {
			if err := segment.AddAnnotation(key, value); err != nil {
				_ = err // Silently ignore XRay errors
			}
		}
	}
}

// addRequestMetadata adds standard request metadata
func (am *xrayAnnotationManager) addRequestMetadata(segment *xray.Segment, ctx *lift.Context) {
	// Add standard metadata
	am.tracer.addStandardMetadata(segment, ctx)

	// Add custom metadata from config
	if am.config.Metadata != nil {
		for key, value := range am.config.Metadata {
			if err := segment.AddMetadata("custom", map[string]any{key: value}); err != nil {
				_ = err // Silently ignore XRay errors
			}
		}
	}
}

// addTraceHeaders adds trace information to request headers
func (am *xrayAnnotationManager) addTraceHeaders(ctx *lift.Context, segment *xray.Segment) {
	if ctx.Request != nil {
		if ctx.Request.Headers == nil {
			ctx.Request.Headers = make(map[string]string)
		}

		if traceID := segment.TraceID; traceID != "" {
			ctx.Request.Headers["X-Trace-Id"] = traceID
		}
		if segmentID := segment.ID; segmentID != "" {
			ctx.Request.Headers["X-Span-Id"] = segmentID
		}
	}
}

// addResponseData adds response timing and status information
func (am *xrayAnnotationManager) addResponseData(segment *xray.Segment, ctx *lift.Context, duration time.Duration, err error) {
	// Record timing
	if addErr := segment.AddMetadata("timing", map[string]any{
		"duration_ms": duration.Milliseconds(),
	}); addErr != nil {
		_ = addErr // Silently ignore XRay errors
	}

	// Handle errors
	if err != nil {
		am.addErrorData(segment, err)
	} else {
		if annoErr := segment.AddAnnotation("error", "false"); annoErr != nil {
			_ = annoErr // Silently ignore XRay errors
		}
	}

	// Add response information
	if annoErr := segment.AddAnnotation("http.status_code", ctx.Response.StatusCode); annoErr != nil {
		_ = annoErr // Silently ignore XRay errors
	}
	if metaErr := segment.AddMetadata("response", map[string]any{
		"status_code": ctx.Response.StatusCode,
	}); metaErr != nil {
		_ = metaErr // Silently ignore XRay errors
	}
}

// addErrorData adds error-specific annotations and metadata
func (am *xrayAnnotationManager) addErrorData(segment *xray.Segment, err error) {
	if addErr := segment.AddError(err); addErr != nil {
		_ = addErr // Silently ignore XRay errors
	}
	if annoErr := segment.AddAnnotation("error", "true"); annoErr != nil {
		_ = annoErr // Silently ignore XRay errors
	}
	if metaErr := segment.AddMetadata("error", map[string]any{
		"message": err.Error(),
	}); metaErr != nil {
		_ = metaErr // Silently ignore XRay errors
	}
}
