package monitoring

import (
	"context"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-xray-sdk-go/v2/instrumentation/awsv2"
	"github.com/aws/aws-xray-sdk-go/v2/xray"
	"go.uber.org/zap"
)

var loadDefaultAWSConfigFn = config.LoadDefaultConfig

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
		if err := xray.Configure(xray.Config{
			LogLevel:       "info",
			ServiceVersion: "1.0.0",
		}); err != nil {
			logger.Warn("failed to configure X-Ray", zap.Error(err))
		}

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

// AWSConfigLoadOptions returns AWS SDK v2 load options that enable X-Ray middleware.
//
// This is useful for libraries (including TableTheory) that internally call config.LoadDefaultConfig and
// accept `[]func(*config.LoadOptions) error` as configuration hooks.
func (xt *XRayTracer) AWSConfigLoadOptions() []func(*config.LoadOptions) error {
	if !xt.enabled {
		return nil
	}

	return []func(*config.LoadOptions) error{
		func(o *config.LoadOptions) error {
			if o != nil {
				awsv2.AWSV2Instrumentor(&o.APIOptions)
			}
			return nil
		},
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
	subseg.Namespace = ProviderAWS

	// Add annotations
	_ = subseg.AddAnnotation("operation", operation)
	_ = subseg.AddAnnotation("table_name", tableName)

	// Add metadata
	_ = subseg.AddMetadata("dynamodb", map[string]interface{}{
		"table_name": tableName,
		"operation":  operation,
	})

	// Execute operation
	err := fn(ctx)

	if err != nil {
		_ = subseg.AddError(err)
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
	_ = subseg.AddAnnotation("domain", domain)
	_ = subseg.AddAnnotation("operation", operation)
	_ = subseg.AddAnnotation("activity_type", activityType)

	// Add metadata
	_ = subseg.AddMetadata("federation", map[string]interface{}{
		"domain":        domain,
		"operation":     operation,
		"activity_type": activityType,
	})

	// Execute operation
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	// Add performance metadata
	_ = subseg.AddMetadata("performance", map[string]interface{}{
		"duration_ms": duration.Milliseconds(),
		"success":     err == nil,
	})

	if err != nil {
		_ = subseg.AddError(err)
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
	_ = subseg.AddAnnotation("resolver", resolver)
	_ = subseg.AddAnnotation("field", fieldName)

	// Execute resolver
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	// Add metadata
	_ = subseg.AddMetadata("graphql", map[string]interface{}{
		"resolver":    resolver,
		"field":       fieldName,
		"duration_ms": duration.Milliseconds(),
		"success":     err == nil,
	})

	if err != nil {
		_ = subseg.AddError(err)
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
	_ = subseg.AddAnnotation("operation", operation)
	_ = subseg.AddAnnotation("media_type", mediaType)

	// Execute operation
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	// Add metadata
	_ = subseg.AddMetadata("media", map[string]interface{}{
		"operation":   operation,
		"media_type":  mediaType,
		"size_bytes":  size,
		"duration_ms": duration.Milliseconds(),
		"success":     err == nil,
	})

	if err != nil {
		_ = subseg.AddError(err)
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
	_ = seg.AddAnnotation("processor", processorName)
	_ = seg.AddAnnotation("record_count", recordCount)
	_ = seg.AddAnnotation("environment", xt.environment)

	// Execute processing
	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	// Add metadata
	_ = seg.AddMetadata("stream", map[string]interface{}{
		"processor":      processorName,
		"record_count":   recordCount,
		"duration_ms":    duration.Milliseconds(),
		"success":        err == nil,
		"records_per_ms": float64(recordCount) / float64(duration.Milliseconds()),
	})

	if err != nil {
		_ = seg.AddError(err)
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
	_ = subseg.AddAnnotation("operation", operation)
	_ = subseg.AddAnnotation("auth_type", authType)

	// Execute operation
	start := time.Now()
	success, err := fn(ctx)
	duration := time.Since(start)

	// Add metadata
	_ = subseg.AddMetadata("auth", map[string]interface{}{
		"operation":   operation,
		"auth_type":   authType,
		"duration_ms": duration.Milliseconds(),
		"success":     success,
		"error":       err != nil,
	})

	if err != nil {
		_ = subseg.AddError(err)
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
	_ = subseg.AddAnnotation("service", service)
	_ = subseg.AddAnnotation("cost_micro_cents", costMicroCents)

	// Execute operation
	err := fn(ctx)

	// Add metadata
	_ = subseg.AddMetadata("cost", map[string]interface{}{
		"service":          service,
		"cost_micro_cents": costMicroCents,
		"cost_dollars":     float64(costMicroCents) / 1000000.0,
		"success":          err == nil,
	})

	if err != nil {
		_ = subseg.AddError(err)
	}

	return err
}

// InstrumentS3Client instruments an S3 client for X-Ray tracing
func (xt *XRayTracer) InstrumentS3Client(client *s3.Client) *s3.Client {
	if !xt.enabled {
		return client
	}

	// Get the client's config
	cfg, err := loadDefaultAWSConfigFn(context.Background())
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
	cfg, err := loadDefaultAWSConfigFn(context.Background())
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
