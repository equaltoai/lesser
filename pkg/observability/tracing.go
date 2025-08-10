// Package observability provides distributed tracing support with AWS X-Ray integration
package observability

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-xray-sdk-go/xray"
	"go.uber.org/zap"
)

// TracingConfig contains configuration for distributed tracing
type TracingConfig struct {
	ServiceName     string
	ServiceVersion  string
	SamplingRate    float64
	DaemonAddress   string
	UseECS          bool
	LocalTesting    bool
	Enabled         bool
}

// TraceContext represents a distributed trace context
type TraceContext struct {
	TraceID    string
	SegmentID  string
	ParentID   string
	Sampled    bool
	RequestID  string
	UserID     string
	TenantID   string
	metadata   map[string]interface{}
}

// TracingManager manages distributed tracing operations
type TracingManager struct {
	config *TracingConfig
	logger *zap.Logger
}

// NewTracingManager creates a new tracing manager
func NewTracingManager(logger *zap.Logger, config *TracingConfig) *TracingManager {
	if config == nil {
		config = &TracingConfig{
			ServiceName:    "lesser-service",
			ServiceVersion: "1.0.0",
			SamplingRate:   TracingSampleRatePercent / 100.0, // Convert percentage to decimal
			Enabled:        os.Getenv("XRAY_TRACING_ENABLED") != "false",
			LocalTesting:   os.Getenv("AWS_LAMBDA_FUNCTION_NAME") == "",
		}
	}

	tm := &TracingManager{
		config: config,
		logger: logger,
	}

	// Initialize X-Ray if enabled
	if config.Enabled && !config.LocalTesting {
		tm.initializeXRay()
	}

	return tm
}

// initializeXRay initializes AWS X-Ray tracing
func (tm *TracingManager) initializeXRay() {
	// Configure X-Ray daemon address if specified
	if tm.config.DaemonAddress != "" {
		err := xray.Configure(xray.Config{
			DaemonAddr: tm.config.DaemonAddress,
		})
		if err != nil {
			tm.logger.Error("failed to configure X-Ray daemon address", zap.Error(err))
		}
	}

	// X-Ray logger is configured through AWS SDK
	
	tm.logger.Info("initialized X-Ray tracing",
		zap.String("service", tm.config.ServiceName),
		zap.String("version", tm.config.ServiceVersion),
		zap.Float64("sampling_rate", tm.config.SamplingRate),
		zap.String("daemon_address", tm.config.DaemonAddress))
}

// StartSegment starts a new X-Ray segment
func (tm *TracingManager) StartSegment(ctx context.Context, name string) (context.Context, *xray.Segment) {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return ctx, nil
	}

	// Start segment with proper service name
	segmentCtx, segment := xray.BeginSegment(ctx, name)
	
	if segment != nil {
		// Add service information
		_ = segment.AddAnnotation("service", tm.config.ServiceName)
		_ = segment.AddAnnotation("version", tm.config.ServiceVersion)
		_ = segment.AddMetadata("service_info", map[string]interface{}{
			"service_name":    tm.config.ServiceName,
			"service_version": tm.config.ServiceVersion,
			"timestamp":       time.Now().Format(time.RFC3339),
		})
	}

	return segmentCtx, segment
}

// StartSubsegment starts a new X-Ray subsegment
func (tm *TracingManager) StartSubsegment(ctx context.Context, name string) (context.Context, *xray.Segment) {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return ctx, nil
	}

	subsegmentCtx, subsegment := xray.BeginSubsegment(ctx, name)
	return subsegmentCtx, subsegment
}

// AddAnnotation adds an annotation to the current segment
func (tm *TracingManager) AddAnnotation(ctx context.Context, key string, value interface{}) {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return
	}

	if segment := xray.GetSegment(ctx); segment != nil {
		_ = segment.AddAnnotation(key, value)
	}
}

// AddMetadata adds metadata to the current segment
func (tm *TracingManager) AddMetadata(ctx context.Context, namespace string, data map[string]interface{}) {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return
	}

	if segment := xray.GetSegment(ctx); segment != nil {
		_ = segment.AddMetadata(namespace, data)
	}
}

// AddError adds an error to the current segment
func (tm *TracingManager) AddError(ctx context.Context, err error, remote bool) {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return
	}

	if segment := xray.GetSegment(ctx); segment != nil {
		_ = segment.AddError(err)
		if !remote {
			// Mark as fault (non-remote error)
			segment.Fault = true
		}
	}
}

// SetHTTPRequest adds HTTP request information to the segment
func (tm *TracingManager) SetHTTPRequest(ctx context.Context, method, url string, userAgent string, clientIP string) {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return
	}

	if segment := xray.GetSegment(ctx); segment != nil {
		segment.GetHTTP().GetRequest().Method = method
		segment.GetHTTP().GetRequest().URL = url
		segment.GetHTTP().GetRequest().UserAgent = userAgent
		segment.GetHTTP().GetRequest().ClientIP = clientIP
	}
}

// SetHTTPResponse adds HTTP response information to the segment
func (tm *TracingManager) SetHTTPResponse(ctx context.Context, statusCode int, contentLength int64) {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return
	}

	if segment := xray.GetSegment(ctx); segment != nil {
		segment.GetHTTP().GetResponse().Status = statusCode
		segment.GetHTTP().GetResponse().ContentLength = int(contentLength)
	}
}

// SetUser adds user information to the segment
func (tm *TracingManager) SetUser(ctx context.Context, userID string) {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return
	}

	if segment := xray.GetSegment(ctx); segment != nil {
		segment.User = userID
	}
}

// TraceDatabase traces a database operation
func (tm *TracingManager) TraceDatabase(ctx context.Context, operation string, tableName string, queryFunc func(ctx context.Context) error) error {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return queryFunc(ctx)
	}

	subsegmentCtx, subsegment := tm.StartSubsegment(ctx, fmt.Sprintf("DynamoDB.%s", operation))
	defer func() {
		if subsegment != nil {
			subsegment.Close(nil)
		}
	}()

	if subsegment != nil {
		subsegment.Namespace = namespaceAWS
		_ = subsegment.AddAnnotation("operation", operation)
		_ = subsegment.AddAnnotation("table_name", tableName)
		_ = subsegment.AddMetadata("dynamodb", map[string]interface{}{
			"operation":  operation,
			"table_name": tableName,
			"timestamp":  time.Now().Format(time.RFC3339),
		})
	}

	err := queryFunc(subsegmentCtx)
	if err != nil && subsegment != nil {
		_ = subsegment.AddError(err)
	}

	return err
}

// TraceExternalCall traces an external HTTP call
func (tm *TracingManager) TraceExternalCall(ctx context.Context, serviceName string, method string, url string, callFunc func(ctx context.Context) error) error {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return callFunc(ctx)
	}

	subsegmentCtx, subsegment := tm.StartSubsegment(ctx, serviceName)
	defer func() {
		if subsegment != nil {
			subsegment.Close(nil)
		}
	}()

	if subsegment != nil {
		subsegment.Namespace = "remote"
		_ = subsegment.AddAnnotation("http_method", method)
		_ = subsegment.AddAnnotation("url", url)
		subsegment.GetHTTP().GetRequest().Method = method
		subsegment.GetHTTP().GetRequest().URL = url
	}

	err := callFunc(subsegmentCtx)
	if err != nil && subsegment != nil {
		_ = subsegment.AddError(err)
	}

	return err
}

// TraceLambdaFunction traces a Lambda function execution
func (tm *TracingManager) TraceLambdaFunction(ctx context.Context, functionName string, execFunc func(ctx context.Context) error) error {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return execFunc(ctx)
	}

	subsegmentCtx, subsegment := tm.StartSubsegment(ctx, fmt.Sprintf("Lambda.%s", functionName))
	defer func() {
		if subsegment != nil {
			subsegment.Close(nil)
		}
	}()

	if subsegment != nil {
		subsegment.Namespace = namespaceAWS
		_ = subsegment.AddAnnotation("function_name", functionName)
		_ = subsegment.AddMetadata("lambda", map[string]interface{}{
			"function_name": functionName,
			"timestamp":     time.Now().Format(time.RFC3339),
		})
	}

	err := execFunc(subsegmentCtx)
	if err != nil && subsegment != nil {
		_ = subsegment.AddError(err)
		subsegment.Fault = true
	}

	return err
}

// GetTraceContext extracts trace context from the current segment
func (tm *TracingManager) GetTraceContext(ctx context.Context) *TraceContext {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return &TraceContext{
			TraceID:   "local-testing",
			SegmentID: "local-segment",
			Sampled:   false,
		}
	}

	segment := xray.GetSegment(ctx)
	if segment == nil {
		return &TraceContext{
			TraceID:   "no-segment",
			SegmentID: "no-segment",
			Sampled:   false,
		}
	}

	return &TraceContext{
		TraceID:   segment.TraceID,
		SegmentID: segment.ID,
		ParentID:  segment.ParentID,
		Sampled:   segment.Sampled,
		metadata:  make(map[string]interface{}),
	}
}

// InjectTraceHeaders injects trace headers into a map for propagation
func (tm *TracingManager) InjectTraceHeaders(ctx context.Context, headers map[string]string) {
	if !tm.config.Enabled || tm.config.LocalTesting {
		return
	}

	traceCtx := tm.GetTraceContext(ctx)
	if traceCtx != nil && traceCtx.TraceID != "" {
		// X-Ray trace header format
		headers["X-Amzn-Trace-Id"] = fmt.Sprintf("Root=%s;Parent=%s;Sampled=%d",
			traceCtx.TraceID,
			traceCtx.SegmentID,
			boolToInt(traceCtx.Sampled))
	}
}

// ExtractTraceHeaders extracts trace information from headers
func (tm *TracingManager) ExtractTraceHeaders(headers map[string]string) *TraceContext {
	traceHeader := headers["X-Amzn-Trace-Id"]
	if traceHeader == "" {
		return nil
	}

	// Parse X-Ray trace header (simplified)
	// Real implementation would need proper parsing
	return &TraceContext{
		TraceID:  traceHeader,
		metadata: make(map[string]interface{}),
	}
}

// CreateTracingMiddleware creates middleware for automatic tracing
func (tm *TracingManager) CreateTracingMiddleware() func(next func(ctx context.Context) error) func(ctx context.Context) error {
	return func(next func(ctx context.Context) error) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			if !tm.config.Enabled {
				return next(ctx)
			}

			// Start segment for the request
			segmentCtx, segment := tm.StartSegment(ctx, tm.config.ServiceName)
			defer func() {
				if segment != nil {
					segment.Close(nil)
				}
			}()

			// Execute the handler
			err := next(segmentCtx)
			
			// Record error if any
			if err != nil && segment != nil {
				_ = segment.AddError(err)
				segment.Fault = true
			}

			return err
		}
	}
}

// Helper functions

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetProperty adds a property to the trace context
func (tc *TraceContext) SetProperty(key string, value interface{}) {
	if tc.metadata == nil {
		tc.metadata = make(map[string]interface{})
	}
	tc.metadata[key] = value
}

// GetProperty gets a property from the trace context
func (tc *TraceContext) GetProperty(key string) (interface{}, bool) {
	if tc.metadata == nil {
		return nil, false
	}
	value, exists := tc.metadata[key]
	return value, exists
}

// IsEnabled returns whether tracing is enabled
func (tm *TracingManager) IsEnabled() bool {
	return tm.config.Enabled && !tm.config.LocalTesting
}