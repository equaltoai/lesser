package lift

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-xray-sdk-go/v2/xray"
	"github.com/equaltoai/lesser/pkg/monitoring"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Namespace constants
const (
	namespaceAWS = "aws"
)

// XRayMiddleware provides X-Ray tracing middleware for Lift handlers
type XRayMiddleware struct {
	tracer      *monitoring.XRayTracer
	serviceName string
	logger      *zap.Logger
}

// NewXRayMiddleware creates a new X-Ray middleware
func NewXRayMiddleware(serviceName, environment string, logger *zap.Logger) *XRayMiddleware {
	return &XRayMiddleware{
		tracer:      monitoring.NewXRayTracer(serviceName, environment, logger),
		serviceName: serviceName,
		logger:      logger,
	}
}

// Middleware returns the Lift middleware function for X-Ray tracing
func (xm *XRayMiddleware) Middleware() func(lift.HandlerFunc) lift.HandlerFunc {
	return func(next lift.HandlerFunc) lift.HandlerFunc {
		return func(ctx *lift.Context) error {
			// Skip if X-Ray is not enabled
			if !xm.tracer.IsEnabled() {
				return next(ctx)
			}

			// Start X-Ray segment
			segmentName := fmt.Sprintf("%s.%s", xm.serviceName, ctx.Request.Method)
			xrayCtx, seg := xray.BeginSegment(context.Background(), segmentName)
			defer seg.Close(nil)

			// Add standard annotations for searchability
			xm.addStandardAnnotations(seg, ctx)

			// Add Lambda-specific metadata if available
			xm.addLambdaMetadata(xrayCtx, seg)

			// Add request metadata
			xm.addRequestMetadata(seg, ctx)

			// Track execution time
			start := time.Now()

			// Execute next handler
			err := next(ctx)

			// Calculate duration
			duration := time.Since(start)

			// Handle error
			if err != nil {
				_ = seg.AddError(err)
				xm.addErrorMetadata(seg, err)
			}

			// Add response metadata
			xm.addResponseMetadata(seg, ctx, duration, err)

			return err
		}
	}
}

// addStandardAnnotations adds standard X-Ray annotations
func (xm *XRayMiddleware) addStandardAnnotations(seg *xray.Segment, ctx *lift.Context) {
	annotations := map[string]interface{}{
		"service":    xm.serviceName,
		"method":     ctx.Request.Method,
		"path":       ctx.Request.Path,
		"tenant_id":  ctx.TenantID(),
		"request_id": ctx.RequestID,
	}

	// Add user ID if authenticated
	if userID := ctx.UserID(); userID != "" {
		annotations["user_id"] = userID
	}

	// Add annotations
	for key, value := range annotations {
		if err := seg.AddAnnotation(key, value); err != nil {
			xm.logger.Warn("failed to add X-Ray annotation",
				zap.String("key", key),
				zap.Error(err))
		}
	}
}

// addLambdaMetadata adds Lambda-specific metadata
func (xm *XRayMiddleware) addLambdaMetadata(ctx context.Context, seg *xray.Segment) {
	lambdaCtx, ok := lambdacontext.FromContext(ctx)
	if !ok {
		return
	}

	metadata := map[string]interface{}{
		"request_id":   lambdaCtx.AwsRequestID,
		"function_arn": lambdaCtx.InvokedFunctionArn,
	}

	// Mark as potential cold start (simplified detection)
	metadata["cold_start"] = true
	_ = seg.AddAnnotation("cold_start", true)

	if err := seg.AddMetadata("lambda", metadata); err != nil {
		xm.logger.Warn("failed to add Lambda metadata to X-Ray", zap.Error(err))
	}
}

// addRequestMetadata adds request metadata
func (xm *XRayMiddleware) addRequestMetadata(seg *xray.Segment, ctx *lift.Context) {
	metadata := map[string]interface{}{
		"headers":   ctx.Request.Headers,
		"body_size": len(ctx.Request.Body),
	}

	// Add content type if present
	if contentType := ctx.Request.Headers["Content-Type"]; contentType != "" {
		metadata["content_type"] = contentType
	}

	// Add user agent if present
	if userAgent := ctx.Request.Headers["User-Agent"]; userAgent != "" {
		metadata["user_agent"] = userAgent
	}

	if err := seg.AddMetadata("request", metadata); err != nil {
		xm.logger.Warn("failed to add request metadata to X-Ray", zap.Error(err))
	}
}

// addResponseMetadata adds response metadata
func (xm *XRayMiddleware) addResponseMetadata(seg *xray.Segment, ctx *lift.Context, duration time.Duration, err error) {
	metadata := map[string]interface{}{
		"duration_ms": duration.Milliseconds(),
		"success":     err == nil,
	}

	// Add status code if set
	if ctx.Response != nil && ctx.Response.StatusCode > 0 {
		metadata["status_code"] = ctx.Response.StatusCode
		_ = seg.AddAnnotation("status_code", ctx.Response.StatusCode)
	}

	// Add response size if available (body is interface{}, so we can't get size directly)
	if ctx.Response != nil && ctx.Response.Body != nil {
		metadata["has_body"] = true
	}

	if err := seg.AddMetadata("response", metadata); err != nil {
		xm.logger.Warn("failed to add response metadata to X-Ray", zap.Error(err))
	}
}

// addErrorMetadata adds error metadata
func (xm *XRayMiddleware) addErrorMetadata(seg *xray.Segment, err error) {
	if err == nil {
		return
	}

	metadata := map[string]interface{}{
		"message": err.Error(),
		"type":    fmt.Sprintf("%T", err),
	}

	// Add error type information
	metadata["error_type"] = fmt.Sprintf("%T", err)

	if err := seg.AddMetadata("error", metadata); err != nil {
		xm.logger.Warn("failed to add error metadata to X-Ray", zap.Error(err))
	}
}

// TraceDynamoDB creates a subsegment for DynamoDB operations
func TraceDynamoDB(ctx context.Context, operation, tableName string, fn func(context.Context) error) error {
	// Check if X-Ray is enabled
	if xray.GetSegment(ctx) == nil {
		return fn(ctx)
	}

	ctx, subseg := xray.BeginSubsegment(ctx, "DynamoDB."+operation)
	defer subseg.Close(nil)

	// Set AWS namespace
	subseg.Namespace = namespaceAWS

	// Add annotations
	_ = subseg.AddAnnotation("operation", operation)
	_ = subseg.AddAnnotation("table_name", tableName)

	// Execute operation
	err := fn(ctx)

	if err != nil {
		_ = subseg.AddError(err)
	}

	return err
}

// TraceFederation creates a subsegment for federation calls
func TraceFederation(ctx context.Context, domain, operation string, fn func(context.Context) error) error {
	// Check if X-Ray is enabled
	if xray.GetSegment(ctx) == nil {
		return fn(ctx)
	}

	ctx, subseg := xray.BeginSubsegment(ctx, "Federation."+operation)
	defer subseg.Close(nil)

	// Set remote namespace
	subseg.Namespace = "remote"

	// Add annotations
	_ = subseg.AddAnnotation("domain", domain)
	_ = subseg.AddAnnotation("operation", operation)

	// Execute operation
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	// Add metadata
	_ = subseg.AddMetadata("federation", map[string]interface{}{
		"domain":      domain,
		"operation":   operation,
		"duration_ms": duration.Milliseconds(),
		"success":     err == nil,
	})

	if err != nil {
		_ = subseg.AddError(err)
	}

	return err
}

// TraceS3 creates a subsegment for S3 operations
func TraceS3(ctx context.Context, operation, bucket, key string, fn func(context.Context) error) error {
	// Check if X-Ray is enabled
	if xray.GetSegment(ctx) == nil {
		return fn(ctx)
	}

	ctx, subseg := xray.BeginSubsegment(ctx, "S3."+operation)
	defer subseg.Close(nil)

	// Set AWS namespace
	subseg.Namespace = namespaceAWS

	// Add annotations
	_ = subseg.AddAnnotation("operation", operation)
	_ = subseg.AddAnnotation("bucket", bucket)

	// Add metadata
	_ = subseg.AddMetadata("s3", map[string]interface{}{
		"bucket":    bucket,
		"key":       key,
		"operation": operation,
	})

	// Execute operation
	err := fn(ctx)

	if err != nil {
		_ = subseg.AddError(err)
	}

	return err
}

// TraceSQS creates a subsegment for SQS operations
func TraceSQS(ctx context.Context, operation, queueName string, messageCount int, fn func(context.Context) error) error {
	// Check if X-Ray is enabled
	if xray.GetSegment(ctx) == nil {
		return fn(ctx)
	}

	ctx, subseg := xray.BeginSubsegment(ctx, "SQS."+operation)
	defer subseg.Close(nil)

	// Set AWS namespace
	subseg.Namespace = namespaceAWS

	// Add annotations
	_ = subseg.AddAnnotation("operation", operation)
	_ = subseg.AddAnnotation("queue_name", queueName)

	// Add metadata
	_ = subseg.AddMetadata("sqs", map[string]interface{}{
		"queue_name":    queueName,
		"operation":     operation,
		"message_count": messageCount,
	})

	// Execute operation
	err := fn(ctx)

	if err != nil {
		_ = subseg.AddError(err)
	}

	return err
}
