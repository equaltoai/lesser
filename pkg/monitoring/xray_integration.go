package monitoring

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-xray-sdk-go/instrumentation/awsv2"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// XRayTracer provides comprehensive X-Ray tracing for the Lesser application
type XRayTracer struct {
	serviceName string
	environment string
	logger      *zap.Logger
	enabled     bool
}

// NewXRayTracer creates a new X-Ray tracer
func NewXRayTracer(serviceName, environment string, logger *zap.Logger) *XRayTracer {
	// Check if X-Ray is enabled via environment variable
	enabled := os.Getenv("_X_AMZN_TRACE_ID") != ""
	
	if enabled {
		// Configure X-Ray
		xray.Configure(xray.Config{
			LogLevel:       "info",
			ServiceVersion: "1.0.0",
		})
		
		logger.Info("X-Ray tracing enabled",
			zap.String("service", serviceName),
			zap.String("environment", environment))
	} else {
		logger.Info("X-Ray tracing disabled (not running in AWS Lambda)")
	}
	
	return &XRayTracer{
		serviceName: serviceName,
		environment: environment,
		logger:      logger,
		enabled:     enabled,
	}
}

// InstrumentAWSConfig instruments AWS SDK v2 configuration for X-Ray tracing
func (xt *XRayTracer) InstrumentAWSConfig(cfg aws.Config) aws.Config {
	if !xt.enabled {
		return cfg
	}
	
	// Instrument AWS SDK v2 with X-Ray
	awsv2.AWSV2Instrumentor(&cfg.APIOptions)
	
	return cfg
}

// TraceLiftHandler wraps a Lift handler with X-Ray tracing
func (xt *XRayTracer) TraceLiftHandler(handlerName string, handler func(*lift.Context) error) func(*lift.Context) error {
	if !xt.enabled {
		return handler
	}
	
	return func(ctx *lift.Context) error {
		// Start X-Ray segment for the handler
		_, seg := xray.BeginSegment(context.Background(), xt.serviceName)
		defer seg.Close(nil)
		
		// Add annotations for searchability
		seg.AddAnnotation("handler", handlerName)
		seg.AddAnnotation("environment", xt.environment)
		seg.AddAnnotation("method", ctx.Request.Method)
		seg.AddAnnotation("path", ctx.Request.Path)
		
		// Add metadata for detailed tracing
		seg.AddMetadata("request", map[string]interface{}{
			"headers":    ctx.Request.Headers,
			"tenant_id":  ctx.TenantID,
			"request_id": ctx.RequestID,
		})
		
		// Track cold start
		if lambdaCtx, ok := lambdacontext.FromContext(context.Background()); ok {
			seg.AddAnnotation("cold_start", true)
			seg.AddMetadata("lambda", map[string]interface{}{
				"request_id":     lambdaCtx.AwsRequestID,
				"function_name":  lambdaCtx.InvokedFunctionArn,
			})
		}
		
		// Execute handler with X-Ray context
		start := time.Now()
		err := handler(ctx)
		duration := time.Since(start)
		
		// Record error if present
		if err != nil {
			seg.AddError(err)
			seg.AddMetadata("error", map[string]interface{}{
				"message": err.Error(),
				"type":    fmt.Sprintf("%T", err),
			})
		}
		
		// Add response metadata
		seg.AddMetadata("response", map[string]interface{}{
			"duration_ms": duration.Milliseconds(),
			"success":     err == nil,
		})
		
		return err
	}
}

// TraceDynamoDBOperation traces a DynamoDB operation
func (xt *XRayTracer) TraceDynamoDBOperation(ctx context.Context, operation, tableName string, fn func(context.Context) error) error {
	if !xt.enabled {
		return fn(ctx)
	}
	
	ctx, subseg := xray.BeginSubsegment(ctx, "DynamoDB."+operation)
	defer subseg.Close(nil)
	
	// Set AWS namespace for proper service map
	subseg.Namespace = "aws"
	
	// Add annotations
	subseg.AddAnnotation("operation", operation)
	subseg.AddAnnotation("table_name", tableName)
	
	// Add metadata
	subseg.AddMetadata("dynamodb", map[string]interface{}{
		"table_name": tableName,
		"operation":  operation,
	})
	
	// Execute operation
	err := fn(ctx)
	
	if err != nil {
		subseg.AddError(err)
	}
	
	return err
}

// TraceS3Operation traces an S3 operation
func (xt *XRayTracer) TraceS3Operation(ctx context.Context, operation, bucket, key string, fn func(context.Context) error) error {
	if !xt.enabled {
		return fn(ctx)
	}
	
	ctx, subseg := xray.BeginSubsegment(ctx, "S3."+operation)
	defer subseg.Close(nil)
	
	// Set AWS namespace
	subseg.Namespace = "aws"
	
	// Add annotations
	subseg.AddAnnotation("operation", operation)
	subseg.AddAnnotation("bucket", bucket)
	
	// Add metadata
	subseg.AddMetadata("s3", map[string]interface{}{
		"bucket":    bucket,
		"key":       key,
		"operation": operation,
	})
	
	// Execute operation
	err := fn(ctx)
	
	if err != nil {
		subseg.AddError(err)
	}
	
	return err
}

// TraceSQSOperation traces an SQS operation
func (xt *XRayTracer) TraceSQSOperation(ctx context.Context, operation, queueName string, messageCount int, fn func(context.Context) error) error {
	if !xt.enabled {
		return fn(ctx)
	}
	
	ctx, subseg := xray.BeginSubsegment(ctx, "SQS."+operation)
	defer subseg.Close(nil)
	
	// Set AWS namespace
	subseg.Namespace = "aws"
	
	// Add annotations
	subseg.AddAnnotation("operation", operation)
	subseg.AddAnnotation("queue_name", queueName)
	
	// Add metadata
	subseg.AddMetadata("sqs", map[string]interface{}{
		"queue_name":    queueName,
		"operation":     operation,
		"message_count": messageCount,
	})
	
	// Execute operation
	err := fn(ctx)
	
	if err != nil {
		subseg.AddError(err)
	}
	
	return err
}

// TraceFederationCall traces federation ActivityPub calls
func (xt *XRayTracer) TraceFederationCall(ctx context.Context, domain, operation, activityType string, fn func(context.Context) error) error {
	if !xt.enabled {
		return fn(ctx)
	}
	
	ctx, subseg := xray.BeginSubsegment(ctx, "Federation."+operation)
	defer subseg.Close(nil)
	
	// Set remote namespace for external calls
	subseg.Namespace = "remote"
	
	// Add annotations
	subseg.AddAnnotation("domain", domain)
	subseg.AddAnnotation("operation", operation)
	subseg.AddAnnotation("activity_type", activityType)
	
	// Add metadata
	subseg.AddMetadata("federation", map[string]interface{}{
		"domain":        domain,
		"operation":     operation,
		"activity_type": activityType,
	})
	
	// Execute operation
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)
	
	// Add performance metadata
	subseg.AddMetadata("performance", map[string]interface{}{
		"duration_ms": duration.Milliseconds(),
		"success":     err == nil,
	})
	
	if err != nil {
		subseg.AddError(err)
	}
	
	return err
}

// TraceGraphQLResolver traces GraphQL resolver execution
func (xt *XRayTracer) TraceGraphQLResolver(ctx context.Context, resolver, fieldName string, fn func(context.Context) error) error {
	if !xt.enabled {
		return fn(ctx)
	}
	
	ctx, subseg := xray.BeginSubsegment(ctx, "GraphQL."+resolver)
	defer subseg.Close(nil)
	
	// Set custom namespace
	subseg.Namespace = "graphql"
	
	// Add annotations
	subseg.AddAnnotation("resolver", resolver)
	subseg.AddAnnotation("field", fieldName)
	
	// Execute resolver
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)
	
	// Add metadata
	subseg.AddMetadata("graphql", map[string]interface{}{
		"resolver":    resolver,
		"field":       fieldName,
		"duration_ms": duration.Milliseconds(),
		"success":     err == nil,
	})
	
	if err != nil {
		subseg.AddError(err)
	}
	
	return err
}

// TraceMediaProcessing traces media processing operations
func (xt *XRayTracer) TraceMediaProcessing(ctx context.Context, operation, mediaType string, size int64, fn func(context.Context) error) error {
	if !xt.enabled {
		return fn(ctx)
	}
	
	ctx, subseg := xray.BeginSubsegment(ctx, "MediaProcessing."+operation)
	defer subseg.Close(nil)
	
	// Add annotations
	subseg.AddAnnotation("operation", operation)
	subseg.AddAnnotation("media_type", mediaType)
	
	// Execute operation
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)
	
	// Add metadata
	subseg.AddMetadata("media", map[string]interface{}{
		"operation":   operation,
		"media_type":  mediaType,
		"size_bytes":  size,
		"duration_ms": duration.Milliseconds(),
		"success":     err == nil,
	})
	
	if err != nil {
		subseg.AddError(err)
	}
	
	return err
}

// TraceStreamProcessor traces DynamoDB stream processing
func (xt *XRayTracer) TraceStreamProcessor(ctx context.Context, processorName string, recordCount int, fn func(context.Context) error) error {
	if !xt.enabled {
		return fn(ctx)
	}
	
	ctx, seg := xray.BeginSegment(ctx, processorName)
	defer seg.Close(nil)
	
	// Add annotations
	seg.AddAnnotation("processor", processorName)
	seg.AddAnnotation("record_count", recordCount)
	seg.AddAnnotation("environment", xt.environment)
	
	// Execute processing
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)
	
	// Add metadata
	seg.AddMetadata("stream", map[string]interface{}{
		"processor":     processorName,
		"record_count":  recordCount,
		"duration_ms":   duration.Milliseconds(),
		"success":       err == nil,
		"records_per_ms": float64(recordCount) / float64(duration.Milliseconds()),
	})
	
	if err != nil {
		seg.AddError(err)
	}
	
	return err
}

// TraceAuthOperation traces authentication operations
func (xt *XRayTracer) TraceAuthOperation(ctx context.Context, operation, authType string, fn func(context.Context) (bool, error)) (bool, error) {
	if !xt.enabled {
		return fn(ctx)
	}
	
	ctx, subseg := xray.BeginSubsegment(ctx, "Auth."+operation)
	defer subseg.Close(nil)
	
	// Add annotations
	subseg.AddAnnotation("operation", operation)
	subseg.AddAnnotation("auth_type", authType)
	
	// Execute operation
	start := time.Now()
	success, err := fn(ctx)
	duration := time.Since(start)
	
	// Add metadata
	subseg.AddMetadata("auth", map[string]interface{}{
		"operation":   operation,
		"auth_type":   authType,
		"duration_ms": duration.Milliseconds(),
		"success":     success,
		"error":       err != nil,
	})
	
	if err != nil {
		subseg.AddError(err)
	}
	
	return success, err
}

// TraceCostOperation traces cost tracking operations
func (xt *XRayTracer) TraceCostOperation(ctx context.Context, service string, costMicroCents int64, fn func(context.Context) error) error {
	if !xt.enabled {
		return fn(ctx)
	}
	
	ctx, subseg := xray.BeginSubsegment(ctx, "CostTracking."+service)
	defer subseg.Close(nil)
	
	// Add annotations
	subseg.AddAnnotation("service", service)
	subseg.AddAnnotation("cost_micro_cents", costMicroCents)
	
	// Execute operation
	err := fn(ctx)
	
	// Add metadata
	subseg.AddMetadata("cost", map[string]interface{}{
		"service":          service,
		"cost_micro_cents": costMicroCents,
		"cost_dollars":     float64(costMicroCents) / 1000000.0,
		"success":          err == nil,
	})
	
	if err != nil {
		subseg.AddError(err)
	}
	
	return err
}

// InstrumentDynamoDBClient instruments a DynamoDB client for X-Ray tracing
func (xt *XRayTracer) InstrumentDynamoDBClient(client *dynamodb.Client) *dynamodb.Client {
	if !xt.enabled {
		return client
	}
	
	// Get the client's config
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		xt.logger.Warn("failed to load config for X-Ray instrumentation", zap.Error(err))
		return client
	}
	
	// Instrument the config
	cfg = xt.InstrumentAWSConfig(cfg)
	
	// Return new instrumented client
	return dynamodb.NewFromConfig(cfg)
}

// InstrumentS3Client instruments an S3 client for X-Ray tracing
func (xt *XRayTracer) InstrumentS3Client(client *s3.Client) *s3.Client {
	if !xt.enabled {
		return client
	}
	
	// Get the client's config
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		xt.logger.Warn("failed to load config for X-Ray instrumentation", zap.Error(err))
		return client
	}
	
	// Instrument the config
	cfg = xt.InstrumentAWSConfig(cfg)
	
	// Return new instrumented client
	return s3.NewFromConfig(cfg)
}

// InstrumentSQSClient instruments an SQS client for X-Ray tracing
func (xt *XRayTracer) InstrumentSQSClient(client *sqs.Client) *sqs.Client {
	if !xt.enabled {
		return client
	}
	
	// Get the client's config
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		xt.logger.Warn("failed to load config for X-Ray instrumentation", zap.Error(err))
		return client
	}
	
	// Instrument the config
	cfg = xt.InstrumentAWSConfig(cfg)
	
	// Return new instrumented client
	return sqs.NewFromConfig(cfg)
}

// AddAnnotation adds an annotation to the current segment
func (xt *XRayTracer) AddAnnotation(ctx context.Context, key string, value interface{}) error {
	if !xt.enabled {
		return nil
	}
	
	return xray.AddAnnotation(ctx, key, value)
}

// AddMetadata adds metadata to the current segment
func (xt *XRayTracer) AddMetadata(ctx context.Context, namespace string, data map[string]interface{}) error {
	if !xt.enabled {
		return nil
	}
	
	return xray.AddMetadata(ctx, namespace, data)
}

// RecordError records an error in the current segment
func (xt *XRayTracer) RecordError(ctx context.Context, err error) {
	if !xt.enabled || err == nil {
		return
	}
	
	if xrayErr := xray.AddError(ctx, err); xrayErr != nil {
		xt.logger.Warn("failed to record X-Ray error", 
			zap.Error(xrayErr),
			zap.Error(err))
	}
}

// GetTraceID returns the current X-Ray trace ID
func (xt *XRayTracer) GetTraceID(ctx context.Context) string {
	if !xt.enabled {
		return ""
	}
	
	if seg := xray.GetSegment(ctx); seg != nil {
		return seg.TraceID
	}
	
	return ""
}

// IsEnabled returns whether X-Ray tracing is enabled
func (xt *XRayTracer) IsEnabled() bool {
	return xt.enabled
}