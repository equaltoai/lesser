package lift

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
)

// MetricsConfig configures the metrics middleware
type MetricsConfig struct {
	Namespace         string
	Environment       string
	BufferSize        int
	FlushInterval     time.Duration
	EnableXRayTracing bool
	EnableColdStart   bool
	MetricDimensions  map[string]string
}

// DefaultMetricsConfig returns sensible defaults for metrics collection
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Namespace:         "Lesser/Lambda",
		Environment:       getEnvironment(),
		BufferSize:        100,
		FlushInterval:     60 * time.Second,
		EnableXRayTracing: true,
		EnableColdStart:   true,
		MetricDimensions:  make(map[string]string),
	}
}

// MetricsMiddleware collects comprehensive performance metrics for Lift handlers
type MetricsMiddleware struct {
	config     MetricsConfig
	cloudwatch *cloudwatch.Client
	logger     *zap.Logger
	buffer     *MetricBuffer
	initTime   time.Time
	isFirstRun bool
}

// MetricBuffer buffers metrics for efficient batching
type MetricBuffer struct {
	metrics []types.MetricDatum
	maxSize int
}

// NewMetricsMiddleware creates a new metrics middleware following Lift patterns
func NewMetricsMiddleware(awsConfig aws.Config, config MetricsConfig, logger *zap.Logger) *MetricsMiddleware {
	return &MetricsMiddleware{
		config:     config,
		cloudwatch: cloudwatch.NewFromConfig(awsConfig),
		logger:     logger,
		buffer: &MetricBuffer{
			metrics: make([]types.MetricDatum, 0, config.BufferSize),
			maxSize: config.BufferSize,
		},
		initTime:   time.Now(),
		isFirstRun: true,
	}
}

// Middleware returns the Lift middleware function following canonical pattern
func (mm *MetricsMiddleware) Middleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			startTime := time.Now()

			// Get Lambda context for additional metadata
			lambdaCtx, _ := lambdacontext.FromContext(ctx.Context)

			// Detect cold start
			isColdStart := mm.detectColdStart()

			// Set up tracing context
			operationName := mm.getOperationName(ctx)

			// Record cold start metrics if enabled
			if mm.config.EnableColdStart && isColdStart {
				mm.recordColdStartMetrics(ctx.Context, operationName, startTime)
			}

			// Process the request
			err := next.Handle(ctx)

			// Calculate execution time
			duration := time.Since(startTime)

			// Record performance metrics
			mm.recordRequestMetrics(ctx.Context, RequestMetrics{
				Operation:   operationName,
				Duration:    duration,
				StatusCode:  ctx.Response.StatusCode,
				Method:      ctx.Request.Method,
				Path:        ctx.Request.URL().Path,
				RequestID:   ctx.RequestID,
				TenantID:    ctx.TenantID(),
				Error:       err,
				IsColdStart: isColdStart,
				LambdaCtx:   lambdaCtx,
			})

			// Record runtime metrics
			mm.recordRuntimeMetrics(ctx.Context, operationName)

			// Flush metrics if buffer is full
			if len(mm.buffer.metrics) >= mm.buffer.maxSize {
				mm.flushMetrics(ctx.Context)
			}

			return err
		})
	}
}

// RequestMetrics contains data about a single request
type RequestMetrics struct {
	Operation   string
	Duration    time.Duration
	StatusCode  int
	Method      string
	Path        string
	RequestID   string
	TenantID    string
	Error       error
	IsColdStart bool
	LambdaCtx   *lambdacontext.LambdaContext
}

// recordRequestMetrics records metrics for a single request
func (mm *MetricsMiddleware) recordRequestMetrics(_ context.Context, metrics RequestMetrics) {
	baseDimensions := mm.getBaseDimensions(metrics.Operation)

	// Add request-specific dimensions
	requestDimensions := append(baseDimensions,
		types.Dimension{Name: aws.String("Method"), Value: aws.String(metrics.Method)},
		types.Dimension{Name: aws.String("StatusCode"), Value: aws.String(strconv.Itoa(metrics.StatusCode))},
	)

	// Add tenant dimension if available
	if metrics.TenantID != "" {
		requestDimensions = append(requestDimensions,
			types.Dimension{Name: aws.String("TenantID"), Value: aws.String(metrics.TenantID)})
	}

	// Record latency
	mm.addMetric("OperationLatency", float64(metrics.Duration.Milliseconds()),
		types.StandardUnitMilliseconds, requestDimensions)

	// Record throughput
	mm.addMetric("RequestCount", 1, types.StandardUnitCount, requestDimensions)

	// Record errors
	if metrics.Error != nil {
		errorDimensions := append(baseDimensions,
			types.Dimension{Name: aws.String("ErrorType"), Value: aws.String(getErrorType(metrics.Error))},
		)
		mm.addMetric("ErrorCount", 1, types.StandardUnitCount, errorDimensions)
	}

	// Record success rate
	successValue := 1.0
	if metrics.Error != nil {
		successValue = 0.0
	}
	mm.addMetric("SuccessRate", successValue, types.StandardUnitCount, baseDimensions)

	// Record cold start
	if metrics.IsColdStart {
		mm.addMetric("ColdStartCount", 1, types.StandardUnitCount, baseDimensions)
	}

	// Record Lambda-specific metrics
	if metrics.LambdaCtx != nil {
		lambdaDimensions := append(baseDimensions,
			types.Dimension{Name: aws.String("RequestID"), Value: aws.String(metrics.LambdaCtx.AwsRequestID)},
		)

		// Record Lambda invocation
		mm.addMetric("LambdaInvocations", 1,
			types.StandardUnitCount, lambdaDimensions)

		// Record memory utilization
		if memorySize := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE"); memorySize != "" {
			if size, err := strconv.Atoi(memorySize); err == nil {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				utilization := float64(m.Alloc) / float64(size*1024*1024) * 100
				mm.addMetric("MemoryUtilization", utilization,
					types.StandardUnitPercent, lambdaDimensions)
			}
		}
	}
}

// recordColdStartMetrics records cold start specific metrics
func (mm *MetricsMiddleware) recordColdStartMetrics(_ context.Context, operation string, startTime time.Time) {
	initDuration := startTime.Sub(mm.initTime)
	baseDimensions := mm.getBaseDimensions(operation)

	mm.addMetric("ColdStartDuration", float64(initDuration.Milliseconds()),
		types.StandardUnitMilliseconds, baseDimensions)

	mm.logger.Info("cold_start_detected",
		zap.String("operation", operation),
		zap.Duration("init_duration", initDuration),
		zap.String("function_name", os.Getenv("AWS_LAMBDA_FUNCTION_NAME")),
	)
}

// recordRuntimeMetrics records Go runtime metrics
func (mm *MetricsMiddleware) recordRuntimeMetrics(_ context.Context, operation string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	baseDimensions := mm.getBaseDimensions(operation)

	// Memory metrics
	mm.addMetric("MemoryAllocated", float64(m.Alloc), types.StandardUnitBytes, baseDimensions)
	mm.addMetric("MemoryTotalAlloc", float64(m.TotalAlloc), types.StandardUnitBytes, baseDimensions)
	mm.addMetric("MemorySystem", float64(m.Sys), types.StandardUnitBytes, baseDimensions)

	// GC metrics
	mm.addMetric("GCCount", float64(m.NumGC), types.StandardUnitCount, baseDimensions)
	mm.addMetric("GCPauseTime", float64(m.PauseTotalNs/1000000), types.StandardUnitMilliseconds, baseDimensions)

	// Goroutine metrics
	mm.addMetric("GoroutineCount", float64(runtime.NumGoroutine()), types.StandardUnitCount, baseDimensions)

	// CPU metrics (simplified)
	mm.addMetric("CPUCount", float64(runtime.NumCPU()), types.StandardUnitCount, baseDimensions)
}

// addMetric adds a metric to the buffer
func (mm *MetricsMiddleware) addMetric(name string, value float64, unit types.StandardUnit, dimensions []types.Dimension) {
	mm.buffer.metrics = append(mm.buffer.metrics, types.MetricDatum{
		MetricName: aws.String(name),
		Value:      aws.Float64(value),
		Unit:       unit,
		Dimensions: dimensions,
		Timestamp:  aws.Time(time.Now()),
	})
}

// flushMetrics sends all buffered metrics to CloudWatch
func (mm *MetricsMiddleware) flushMetrics(ctx context.Context) {
	if err := common.ValidateSliceNotEmpty("mm.buffer.metrics", mm.buffer.metrics); err != nil {
		return
	}

	// CloudWatch has a limit of 20 metrics per request
	for i := 0; i < len(mm.buffer.metrics); i += 20 {
		end := i + 20
		if end > len(mm.buffer.metrics) {
			end = len(mm.buffer.metrics)
		}

		input := &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(mm.config.Namespace),
			MetricData: mm.buffer.metrics[i:end],
		}

		if _, err := mm.cloudwatch.PutMetricData(ctx, input); err != nil {
			mm.logger.Error("failed to flush metrics",
				zap.Error(err),
				zap.Int("metric_count", end-i),
			)
		} else {
			mm.logger.Debug("flushed metrics",
				zap.Int("metric_count", end-i),
				zap.String("namespace", mm.config.Namespace),
			)
		}
	}

	// Clear buffer
	mm.buffer.metrics = mm.buffer.metrics[:0]
}

// FlushOnExit flushes any remaining metrics - call this in a defer
func (mm *MetricsMiddleware) FlushOnExit(ctx context.Context) {
	mm.flushMetrics(ctx)
}

// getBaseDimensions returns the base dimensions for all metrics
func (mm *MetricsMiddleware) getBaseDimensions(operation string) []types.Dimension {
	dimensions := []types.Dimension{
		{Name: aws.String("Environment"), Value: aws.String(mm.config.Environment)},
		{Name: aws.String("Operation"), Value: aws.String(operation)},
	}

	// Add function name if available
	if functionName := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); functionName != "" {
		dimensions = append(dimensions,
			types.Dimension{Name: aws.String("FunctionName"), Value: aws.String(functionName)})
	}

	// Add version if available
	if version := os.Getenv("AWS_LAMBDA_FUNCTION_VERSION"); version != "" {
		dimensions = append(dimensions,
			types.Dimension{Name: aws.String("Version"), Value: aws.String(version)})
	}

	// Add custom dimensions
	for key, value := range mm.config.MetricDimensions {
		dimensions = append(dimensions,
			types.Dimension{Name: aws.String(key), Value: aws.String(value)})
	}

	return dimensions
}

// getOperationName extracts the operation name from the Lift context
func (mm *MetricsMiddleware) getOperationName(ctx *lift.Context) string {
	// Try to get operation name from route pattern
	reqPath := ctx.Request.URL().Path
	if reqPath != "" {
		// Clean the path for use as metric dimension
		path := strings.ReplaceAll(reqPath, "/", "_")
		path = strings.ReplaceAll(path, "{", "")
		path = strings.ReplaceAll(path, "}", "")
		path = strings.TrimPrefix(path, "_")
		return fmt.Sprintf("%s_%s", ctx.Request.Method, path)
	}

	// Fallback to function name
	if functionName := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); functionName != "" {
		return functionName
	}

	return "unknown_operation"
}

// detectColdStart determines if this is a cold start
func (mm *MetricsMiddleware) detectColdStart() bool {
	if mm.isFirstRun {
		mm.isFirstRun = false
		return true
	}
	return false
}

// getErrorType extracts a classification from an error
func getErrorType(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()

	// Common error patterns
	switch {
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "not found"):
		return "not_found"
	case strings.Contains(errStr, "unauthorized"):
		return "unauthorized"
	case strings.Contains(errStr, "forbidden"):
		return "forbidden"
	case strings.Contains(errStr, "validation"):
		return "validation"
	case strings.Contains(errStr, "database"):
		return "database"
	case strings.Contains(errStr, "network"):
		return "network"
	default:
		return "unknown"
	}
}

// getEnvironment determines the current environment
func getEnvironment() string {
	cfg := config.Get()
	
	// Use centralized config first (Environment field has priority over Stage)
	if cfg.Environment != "" {
		return cfg.Environment
	}
	if cfg.Stage != "" {
		return cfg.Stage
	}
	
	// Fallback to AWS Lambda function name pattern
	if env := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); env != "" {
		// Extract environment from function name if it follows pattern
		parts := strings.Split(env, "-")
		if len(parts) > 1 {
			return parts[len(parts)-1]
		}
	}
	return "unknown"
}
