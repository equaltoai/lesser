// Package observability provides X-Ray middleware for Lambda functions
package observability

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"go.uber.org/zap"
)

// Constants for X-Ray namespaces
const (
	namespaceAWS = "aws"
)

// XRayConfig contains configuration for X-Ray Lambda middleware
type XRayConfig struct {
	ServiceName    string
	ServiceVersion string
	Enabled        bool
	LocalTesting   bool
}

// NewXRayConfig creates X-Ray configuration with defaults
func NewXRayConfig(serviceName, serviceVersion string) *XRayConfig {
	cfg := config.Get()
	return &XRayConfig{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		Enabled:        common.GetXRayTraceID() != "" || cfg.XrayTracingEnabled,
		LocalTesting:   !common.IsRunningInLambda(),
	}
}

// WrapLambdaHandler wraps a Lambda handler with X-Ray tracing
func WrapLambdaHandler(config *XRayConfig, logger *zap.Logger, handler lambda.Handler) lambda.Handler {
	if !config.Enabled || config.LocalTesting {
		logger.Info("X-Ray tracing disabled or running locally")
		return handler
	}

	logger.Info("enabling X-Ray tracing for Lambda",
		zap.String("service", config.ServiceName),
		zap.String("version", config.ServiceVersion))

	// X-Ray is automatically enabled in Lambda environment via _X_AMZN_TRACE_ID
	// We don't need to wrap the handler, just configure X-Ray
	_ = xray.Configure(xray.Config{
		LogLevel:       "info",
		ServiceVersion: config.ServiceVersion,
	})

	return handler
}

// WrapLambdaFunc wraps a Lambda function with X-Ray tracing
func WrapLambdaFunc(config *XRayConfig, logger *zap.Logger, handlerFunc interface{}) interface{} {
	if !config.Enabled || config.LocalTesting {
		logger.Info("X-Ray tracing disabled or running locally")
		return handlerFunc
	}

	logger.Info("enabling X-Ray tracing for Lambda function",
		zap.String("service", config.ServiceName),
		zap.String("version", config.ServiceVersion))

	// Configure X-Ray
	_ = xray.Configure(xray.Config{
		LogLevel:       "info",
		ServiceVersion: config.ServiceVersion,
	})

	// X-Ray is automatically enabled in Lambda environment
	// We return the handler as-is since X-Ray tracing is configured globally
	return handlerFunc
}

// AddServiceAnnotations adds standard service annotations to X-Ray segment
func AddServiceAnnotations(ctx context.Context, serviceName, serviceVersion string, metadata map[string]interface{}) {
	if segment := xray.GetSegment(ctx); segment != nil {
		_ = segment.AddAnnotation("service", serviceName)
		_ = segment.AddAnnotation("version", serviceVersion)

		if metadata != nil {
			_ = segment.AddMetadata("service", metadata)
		}
	}
}

// AddErrorToTrace adds an error to the current X-Ray segment
func AddErrorToTrace(ctx context.Context, err error, remote bool) {
	if segment := xray.GetSegment(ctx); segment != nil {
		_ = segment.AddError(err)
		if !remote {
			segment.Fault = true
		}
	}
}

// TraceSubsegment creates a traced subsegment for operations
func TraceSubsegment(ctx context.Context, name string, operation func(ctx context.Context) error) error {
	subsegmentCtx, subsegment := xray.BeginSubsegment(ctx, name)
	defer func() {
		if subsegment != nil {
			subsegment.Close(nil)
		}
	}()

	err := operation(subsegmentCtx)
	if err != nil && subsegment != nil {
		_ = subsegment.AddError(err)
		subsegment.Fault = true
	}

	return err
}

// TraceDatabaseOperation traces DynamoDB operations
func TraceDatabaseOperation(ctx context.Context, operation, tableName string, fn func(ctx context.Context) error) error {
	return TraceSubsegment(ctx, "DynamoDB", func(segmentCtx context.Context) error {
		if segment := xray.GetSegment(segmentCtx); segment != nil {
			segment.Namespace = namespaceAWS
			_ = segment.AddAnnotation("operation", operation)
			_ = segment.AddAnnotation("table_name", tableName)
		}
		return fn(segmentCtx)
	})
}

// TraceFederationCall traces federation HTTP calls
func TraceFederationCall(ctx context.Context, instance, method, url string, fn func(ctx context.Context) error) error {
	return TraceSubsegment(ctx, "Federation", func(segmentCtx context.Context) error {
		if segment := xray.GetSegment(segmentCtx); segment != nil {
			segment.Namespace = "remote"
			_ = segment.AddAnnotation("instance", instance)
			_ = segment.AddAnnotation("http_method", method)
			_ = segment.AddAnnotation("url", url)
		}
		return fn(segmentCtx)
	})
}

// TraceMediaProcessing traces media processing operations
func TraceMediaProcessing(ctx context.Context, mediaType, operation string, fn func(ctx context.Context) error) error {
	// Validate media type using centralized validation
	if err := common.ValidateMediaType(mediaType); err != nil {
		return fmt.Errorf("invalid media type for tracing: %w", err)
	}

	return TraceSubsegment(ctx, "MediaProcessing", func(segmentCtx context.Context) error {
		if segment := xray.GetSegment(segmentCtx); segment != nil {
			_ = segment.AddAnnotation("media_type", mediaType)
			_ = segment.AddAnnotation("operation", operation)
		}
		return fn(segmentCtx)
	})
}
