package observability

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Example showing how to replace polling-based metrics with EMF in Lambda handlers

// EMFMetricsService demonstrates proper EMF metrics integration in a Lambda environment
type EMFMetricsService struct {
	collector *EMFMetricsCollector
	logger    *zap.Logger
}

// NewEMFMetricsService creates a new EMF metrics service for Lambda
func NewEMFMetricsService(logger *zap.Logger) *EMFMetricsService {
	collector := NewEMFMetricsCollector("Lesser/API", logger)

	// Set Lambda-specific dimensions
	collector.SetDimension("Runtime", "go1.x")
	collector.SetDimension("Architecture", "arm64")

	return &EMFMetricsService{
		collector: collector,
		logger:    logger,
	}
}

// CreateEMFPerformanceMonitoringMiddleware replaces the polling-based middleware
// This is a drop-in replacement for createPerformanceMonitoringMiddleware
func CreateEMFPerformanceMonitoringMiddleware(emfService *EMFMetricsService) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			startTime := time.Now()

			// Add request-specific dimensions
			emfService.collector.SetDimension("Method", ctx.Request.Method)
			emfService.collector.SetDimension("Path", sanitizePathForMetrics(ctx.Request.Path))

			// Process the request
			err := next.Handle(ctx)

			// Collect performance metrics (no background processing)
			metrics := GetPerformanceMetrics(startTime, time.Time{})

			// Record all performance metrics using EMF
			emfService.RecordRequestMetrics(ctx, metrics, err)

			// CRITICAL: Flush metrics before Lambda terminates
			// This ensures all metrics are written to CloudWatch
			defer func() {
				if flushErr := emfService.collector.Flush(); flushErr != nil {
					emfService.logger.Error("failed to flush EMF metrics", zap.Error(flushErr))
				}
			}()

			return err
		})
	}
}

// RecordRequestMetrics records comprehensive request metrics using EMF
func (ems *EMFMetricsService) RecordRequestMetrics(ctx *lift.Context, perfMetrics *PerformanceMetrics, requestErr error) {
	// Record performance metrics with operation context
	operationDims := map[string]string{
		"Operation": getOperationName(ctx),
		"Method":    ctx.Request.Method,
	}

	// Record latency
	ems.collector.recordMetricWithDimensions(
		"RequestLatency",
		float64(perfMetrics.ExecutionDuration.Milliseconds()),
		"Milliseconds",
		operationDims,
	)

	// Record cold start metrics
	if perfMetrics.ColdStartDuration > 0 {
		ems.collector.recordMetricWithDimensions(
			"ColdStartDuration",
			float64(perfMetrics.ColdStartDuration.Milliseconds()),
			"Milliseconds",
			operationDims,
		)
	}

	// Record memory usage
	ems.collector.recordMetricWithDimensions(
		"MemoryUtilization",
		float64(perfMetrics.MemoryUsed),
		"Bytes",
		operationDims,
	)

	// Record success/error rate
	if requestErr != nil {
		ems.collector.recordMetricWithDimensions("RequestErrors", 1, "Count", operationDims)
		ems.collector.recordMetricWithDimensions("RequestSuccess", 0, "Count", operationDims)
	} else {
		ems.collector.recordMetricWithDimensions("RequestErrors", 0, "Count", operationDims)
		ems.collector.recordMetricWithDimensions("RequestSuccess", 1, "Count", operationDims)
	}

	// Record HTTP status code
	statusDims := make(map[string]string)
	for k, v := range operationDims {
		statusDims[k] = v
	}
	statusDims["StatusCode"] = getStatusCodeRange(ctx.Response.StatusCode)

	ems.collector.recordMetricWithDimensions("HTTPResponses", 1, "Count", statusDims)
}

// RecordDynamoDBMetrics records DynamoDB operation metrics using EMF
func (ems *EMFMetricsService) RecordDynamoDBMetrics(operation, tableName string, duration time.Duration, readUnits, writeUnits float64, err error) {
	dims := map[string]string{
		"Operation": operation,
		"TableName": tableName,
	}

	// Record operation latency
	ems.collector.recordMetricWithDimensions(
		"DynamoDBLatency",
		float64(duration.Milliseconds()),
		"Milliseconds",
		dims,
	)

	// Record consumed capacity
	if readUnits > 0 {
		ems.collector.recordMetricWithDimensions("DynamoDBReadCapacity", readUnits, "Count", dims)
	}
	if writeUnits > 0 {
		ems.collector.recordMetricWithDimensions("DynamoDBWriteCapacity", writeUnits, "Count", dims)
	}

	// Record operation result
	if err != nil {
		errorDims := make(map[string]string)
		for k, v := range dims {
			errorDims[k] = v
		}
		errorDims["ErrorType"] = classifyDynamoDBError(err)
		ems.collector.recordMetricWithDimensions("DynamoDBErrors", 1, "Count", errorDims)
	} else {
		ems.collector.recordMetricWithDimensions("DynamoDBSuccess", 1, "Count", dims)
	}
}

// RecordBusinessMetrics records application-specific business metrics
func (ems *EMFMetricsService) RecordBusinessMetrics(metricName string, value float64, unit string, businessContext map[string]string) {
	dims := map[string]string{
		"MetricType": "Business",
	}
	for k, v := range businessContext {
		dims[k] = v
	}

	ems.collector.recordMetricWithDimensions(metricName, value, unit, dims)
}

// FlushMetrics manually flushes all metrics - call this at the end of Lambda execution
func (ems *EMFMetricsService) FlushMetrics() error {
	if err := ems.collector.Flush(); err != nil {
		return fmt.Errorf("failed to flush metrics: %w", err)
	}
	return nil
}

// Stop gracefully stops the metrics service (no-op for serverless EMF, but maintains interface compatibility)
func (ems *EMFMetricsService) Stop() {
	// No-op: serverless collectors don't need explicit stopping
}

// Helper functions for EMF integration

func getOperationName(ctx *lift.Context) string {
	if ctx.Request.Path != "" && ctx.Request.Method != "" {
		return ctx.Request.Method + "_" + sanitizePathForMetrics(ctx.Request.Path)
	}
	return HealthStatusUnknown
}

func sanitizePathForMetrics(path string) string {
	// Clean path for metric dimension usage
	if path == "" || path == "/" {
		return "root"
	}

	// Replace path parameters and special characters
	result := path
	replacements := map[string]string{
		"/": "_",
		"{": "",
		"}": "",
		"-": "_",
		".": "_",
		":": "_",
	}

	for old, new := range replacements {
		result = replaceAll(result, old, new)
	}

	// Remove leading/trailing underscores
	if len(result) > 0 && result[0] == '_' {
		result = result[1:]
	}
	if len(result) > 0 && result[len(result)-1] == '_' {
		result = result[:len(result)-1]
	}

	if result == "" {
		return "root"
	}

	return result
}

func getStatusCodeRange(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 300 && statusCode < 400:
		return "3xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500:
		return "5xx"
	default:
		return HealthStatusUnknown
	}
}

func replaceAll(s, old, replacement string) string {
	// Simple string replacement without importing strings package
	if old == replacement || len(old) == 0 {
		return s
	}

	result := ""
	for i := 0; i <= len(s)-len(old); {
		if i <= len(s)-len(old) && s[i:i+len(old)] == old {
			result += replacement
			i += len(old)
		} else {
			result += string(s[i])
			i++
		}
	}

	// Add remaining characters
	if len(s) >= len(old) {
		remaining := len(s) - (len(s)/len(old))*len(old)
		if remaining > 0 && len(s) >= len(old) {
			result += s[len(s)-remaining:]
		}
	}

	return result
}

// classifyDynamoDBError provides error classification for metrics
func classifyDynamoDBError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()
	switch {
	case containsString(errStr, "ProvisionedThroughputExceededException"):
		return "throughput_exceeded"
	case containsString(errStr, "ResourceNotFoundException"):
		return "resource_not_found"
	case containsString(errStr, "ConditionalCheckFailedException"):
		return "conditional_check_failed"
	case containsString(errStr, "ValidationException"):
		return "validation"
	case containsString(errStr, "ItemCollectionSizeLimitExceededException"):
		return "item_collection_size_limit"
	case containsString(errStr, "TransactionConflictException"):
		return "transaction_conflict"
	case containsString(errStr, "RequestLimitExceededException"):
		return "request_limit_exceeded"
	case containsString(errStr, "InternalServerError"):
		return "internal_server_error"
	default:
		return HealthStatusUnknown
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Example Lambda handler integration showing EMF usage patterns

// ExampleLambdaHandler demonstrates how to properly use EMF metrics in a Lambda function
func ExampleLambdaHandler() {
	var (
		emfService *EMFMetricsService
		logger     *zap.Logger
		initTime   time.Time
	)

	// Initialize EMF service in init() - this runs once per container
	func() {
		initTime = time.Now()
		logger = zap.L()
		emfService = NewEMFMetricsService(logger)

		// Set any container-level dimensions
		emfService.collector.SetDimension("InitTime", initTime.Format(time.RFC3339))
	}()

	// Lambda handler function
	handler := func(_ context.Context, event interface{}) (interface{}, error) {
		startTime := time.Now()

		// Record cold start if applicable
		if os.Getenv("AWS_LAMBDA_INITIALIZATION_TYPE") != "provisioned-concurrency" {
			coldStartDuration := startTime.Sub(initTime)
			emfService.collector.RecordLatency("cold_start", coldStartDuration)
		}

		// Process request...
		result, err := processRequest(event)

		// Record execution metrics
		executionTime := time.Since(startTime)
		emfService.collector.RecordLatency("execution", executionTime)

		if err != nil {
			emfService.collector.RecordMetric("errors", 1, types.StandardUnitCount)
		} else {
			emfService.collector.RecordMetric("success", 1, types.StandardUnitCount)
		}

		// CRITICAL: Always flush before returning
		// This ensures metrics are written to CloudWatch before Lambda terminates
		if flushErr := emfService.FlushMetrics(); flushErr != nil {
			logger.Error("failed to flush metrics", zap.Error(flushErr))
			// Don't fail the request due to metrics issues
		}

		return result, err
	}

	// Use the handler with AWS Lambda Go runtime
	_ = handler // lambda.Start(handler)
}

func processRequest(_ interface{}) (interface{}, error) {
	// Placeholder for actual request processing
	return map[string]string{"status": "success"}, nil
}

// Migration utility functions

// ConvertPollingMetricsToEMF provides a helper to migrate from polling-based metrics
func ConvertPollingMetricsToEMF(oldCollector *MetricsCollector, logger *zap.Logger) *EMFMetricsService {
	// Flush any remaining metrics from the old collector
	oldCollector.Flush()

	// Create new EMF-based service
	emfService := NewEMFMetricsService(logger)

	logger.Info("migrated from polling-based metrics to EMF",
		zap.String("old_type", "polling"),
		zap.String("new_type", "emf"),
		zap.String("benefit", "no_background_goroutines"),
	)

	return emfService
}
