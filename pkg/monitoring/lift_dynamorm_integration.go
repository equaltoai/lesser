package monitoring

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
)

// ProductionMetricsConfig configures comprehensive production monitoring
type ProductionMetricsConfig struct {
	Namespace            string
	Environment          string
	ServiceName          string
	EnableBusinessMetrics bool
	EnableCostTracking   bool
	EnablePerformanceMetrics bool
	BufferSize           int
	FlushInterval        time.Duration
	Dimensions          map[string]string
}

// DefaultProductionConfig returns production-ready configuration
func DefaultProductionConfig() ProductionMetricsConfig {
	return ProductionMetricsConfig{
		Namespace:            "Lesser/Production",
		Environment:          getEnvironment(),
		ServiceName:          getFunctionName(),
		EnableBusinessMetrics: true,
		EnableCostTracking:   true,
		EnablePerformanceMetrics: true,
		BufferSize:           200,
		FlushInterval:        30 * time.Second,
		Dimensions:          make(map[string]string),
	}
}

// ProductionMonitor provides comprehensive monitoring following Lift/DynamORM patterns
type ProductionMonitor struct {
	config       ProductionMetricsConfig
	cloudwatch   *cloudwatch.Client
	logger       *zap.Logger
	buffer       *MetricBuffer
	costTracker  *cost.Tracker
	initTime     time.Time
	isFirstRun   bool
	mu           sync.RWMutex
}

// MetricBuffer buffers metrics for efficient CloudWatch publishing
type MetricBuffer struct {
	metrics []types.MetricDatum
	maxSize int
	mu      sync.Mutex
}

// NewProductionMonitor creates a comprehensive monitoring solution
func NewProductionMonitor(awsConfig aws.Config, config ProductionMetricsConfig, logger *zap.Logger) *ProductionMonitor {
	return &ProductionMonitor{
		config:     config,
		cloudwatch: cloudwatch.NewFromConfig(awsConfig),
		logger:     logger,
		buffer: &MetricBuffer{
			metrics: make([]types.MetricDatum, 0, config.BufferSize),
			maxSize: config.BufferSize,
		},
		costTracker: cost.New(),
		initTime:    time.Now(),
		isFirstRun:  true,
	}
}

// LiftMiddleware returns Lift middleware that integrates with DynamORM cost tracking
func (pm *ProductionMonitor) LiftMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Attach cost tracker to context for DynamORM integration
			requestID := ctx.RequestID
			ctxWithCost := cost.WithTracker(ctx.Context, cost.NewWithRequest(requestID, "api"))
			ctx.Context = ctxWithCost
			
			startTime := time.Now()
			lambdaCtx, _ := lambdacontext.FromContext(ctx.Context)
			isColdStart := pm.detectColdStart()
			operationName := pm.getOperationName(ctx)
			
			// Record cold start if this is the first invocation
			if pm.config.EnablePerformanceMetrics && isColdStart {
				pm.recordColdStartMetrics(ctx.Context, operationName, startTime)
			}
			
			// Execute the request
			err := next.Handle(ctx)
			duration := time.Since(startTime)
			
			// Collect request metrics
			pm.recordRequestMetrics(ctx.Context, RequestMetrics{
				Operation:   operationName,
				Duration:    duration,
				StatusCode:  ctx.Response.StatusCode,
				Method:      ctx.Request.Method,
				Path:        ctx.Request.URL().Path,
				RequestID:   requestID,
				TenantID:    ctx.TenantID(),
				Error:       err,
				IsColdStart: isColdStart,
				LambdaCtx:   lambdaCtx,
			})
			
			// Record DynamORM cost metrics
			if pm.config.EnableCostTracking {
				pm.recordCostMetrics(ctx.Context, operationName)
			}
			
			// Record business metrics
			if pm.config.EnableBusinessMetrics {
				pm.recordBusinessMetrics(ctx.Context, operationName, ctx, err)
			}
			
			// Flush buffer if needed
			if pm.shouldFlush() {
				go pm.flushMetrics(context.Background())
			}
			
			return err
		})
	}
}

// WrapDynamORMClient wraps a DynamORM client with cost tracking
func (pm *ProductionMonitor) WrapDynamORMClient(client core.DB) core.DB {
	return cost.NewTrackingDB(client, pm.costTracker, pm.logger)
}

// RequestMetrics contains comprehensive request information
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

// recordRequestMetrics records comprehensive request metrics
func (pm *ProductionMonitor) recordRequestMetrics(ctx context.Context, metrics RequestMetrics) {
	baseDimensions := pm.getBaseDimensions(metrics.Operation)
	
	// Add request-specific dimensions
	requestDimensions := append(baseDimensions,
		types.Dimension{Name: aws.String("Method"), Value: aws.String(metrics.Method)},
		types.Dimension{Name: aws.String("StatusCode"), Value: aws.String(fmt.Sprintf("%d", metrics.StatusCode))},
	)
	
	// Add tenant dimension if available
	if metrics.TenantID != "" {
		requestDimensions = append(requestDimensions,
			types.Dimension{Name: aws.String("TenantID"), Value: aws.String(metrics.TenantID)})
	}
	
	// Core performance metrics
	pm.addMetric("RequestLatency", float64(metrics.Duration.Milliseconds()),
		types.StandardUnitMilliseconds, requestDimensions)
	pm.addMetric("RequestCount", 1, types.StandardUnitCount, requestDimensions)
	
	// Error tracking
	if metrics.Error != nil {
		errorType := classifyError(metrics.Error)
		errorDimensions := append(baseDimensions,
			types.Dimension{Name: aws.String("ErrorType"), Value: aws.String(errorType)})
		pm.addMetric("ErrorCount", 1, types.StandardUnitCount, errorDimensions)
		pm.addMetric("ErrorRate", 1, types.StandardUnitCount, baseDimensions)
	} else {
		pm.addMetric("SuccessRate", 1, types.StandardUnitCount, baseDimensions)
	}
	
	// Lambda-specific metrics
	if metrics.LambdaCtx != nil {
		lambdaDimensions := append(baseDimensions,
			types.Dimension{Name: aws.String("RequestID"), Value: aws.String(metrics.LambdaCtx.AwsRequestID)})
		
		// Memory utilization
		if memSize := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE"); memSize != "" {
			if memoryMB, err := parseMemorySize(memSize); err == nil {
				memUsageMB := getCurrentMemoryUsage()
				utilization := (memUsageMB / memoryMB) * 100
				pm.addMetric("MemoryUtilization", utilization,
					types.StandardUnitPercent, lambdaDimensions)
			}
		}
		
		// Lambda request ID for tracking
		pm.addMetric("LambdaInvocations", 1,
			types.StandardUnitCount, lambdaDimensions)
	}
	
	// Cold start tracking
	if metrics.IsColdStart {
		pm.addMetric("ColdStartCount", 1, types.StandardUnitCount, baseDimensions)
	}
}

// recordColdStartMetrics records detailed cold start information
func (pm *ProductionMonitor) recordColdStartMetrics(ctx context.Context, operation string, startTime time.Time) {
	initDuration := startTime.Sub(pm.initTime)
	baseDimensions := pm.getBaseDimensions(operation)
	
	pm.addMetric("ColdStartDuration", float64(initDuration.Milliseconds()),
		types.StandardUnitMilliseconds, baseDimensions)
	
	// Log cold start for analysis
	pm.logger.Info("cold_start_detected",
		zap.String("operation", operation),
		zap.Duration("init_duration", initDuration),
		zap.String("function_name", pm.config.ServiceName),
		zap.Time("init_time", pm.initTime),
	)
}

// recordCostMetrics records DynamORM cost tracking metrics
func (pm *ProductionMonitor) recordCostMetrics(ctx context.Context, operation string) {
	costTracker := cost.FromContext(ctx)
	if costTracker == nil {
		return
	}
	
	costSummary := costTracker.CalculateCost()
	baseDimensions := pm.getBaseDimensions(operation)
	
	// DynamoDB capacity unit tracking
	pm.addMetric("ConsumedReadUnits", float64(costSummary.DynamoDBReads),
		types.StandardUnitCount, baseDimensions)
	pm.addMetric("ConsumedWriteUnits", float64(costSummary.DynamoDBWrites),
		types.StandardUnitCount, baseDimensions)
	
	// Lambda invocation tracking
	pm.addMetric("LambdaInvocations", float64(costSummary.LambdaInvocations),
		types.StandardUnitCount, baseDimensions)
	pm.addMetric("LambdaDurationMs", float64(costSummary.LambdaDurationMs),
		types.StandardUnitMilliseconds, baseDimensions)
	
	// Total cost tracking (in micro cents)
	pm.addMetric("TotalRequestCostMicroCents", float64(costSummary.TotalCostMicroCents),
		types.StandardUnitCount, baseDimensions)
}

// recordBusinessMetrics records application-specific business metrics
func (pm *ProductionMonitor) recordBusinessMetrics(ctx context.Context, operation string, liftCtx *lift.Context, err error) {
	baseDimensions := pm.getBaseDimensions(operation)
	
	// Extract business context from the request
	userID := extractUserID(liftCtx)
	if userID != "" {
		userDimensions := append(baseDimensions,
			types.Dimension{Name: aws.String("UserType"), Value: aws.String(classifyUser(userID))})
		pm.addMetric("ActiveUsers", 1, types.StandardUnitCount, userDimensions)
	}
	
	// Track API endpoint usage
	if endpoint := classifyEndpoint(liftCtx.Request.URL().Path); endpoint != "" {
		endpointDimensions := append(baseDimensions,
			types.Dimension{Name: aws.String("Endpoint"), Value: aws.String(endpoint)})
		pm.addMetric("EndpointUsage", 1, types.StandardUnitCount, endpointDimensions)
	}
	
	// Track federation activity
	if isFederationRequest(liftCtx) {
		fedDimensions := append(baseDimensions,
			types.Dimension{Name: aws.String("ActivityType"), Value: aws.String("federation")})
		pm.addMetric("FederationActivity", 1, types.StandardUnitCount, fedDimensions)
	}
}

// addMetric adds a metric to the buffer
func (pm *ProductionMonitor) addMetric(name string, value float64, unit types.StandardUnit, dimensions []types.Dimension) {
	pm.buffer.mu.Lock()
	defer pm.buffer.mu.Unlock()
	
	pm.buffer.metrics = append(pm.buffer.metrics, types.MetricDatum{
		MetricName: aws.String(name),
		Value:      aws.Float64(value),
		Unit:       unit,
		Dimensions: dimensions,
		Timestamp:  aws.Time(time.Now()),
	})
}

// shouldFlush determines if metrics should be flushed
func (pm *ProductionMonitor) shouldFlush() bool {
	pm.buffer.mu.Lock()
	defer pm.buffer.mu.Unlock()
	return len(pm.buffer.metrics) >= pm.buffer.maxSize
}

// flushMetrics sends all buffered metrics to CloudWatch
func (pm *ProductionMonitor) flushMetrics(ctx context.Context) {
	pm.buffer.mu.Lock()
	if len(pm.buffer.metrics) == 0 {
		pm.buffer.mu.Unlock()
		return
	}
	
	// Copy metrics and clear buffer
	metricsToFlush := make([]types.MetricDatum, len(pm.buffer.metrics))
	copy(metricsToFlush, pm.buffer.metrics)
	pm.buffer.metrics = pm.buffer.metrics[:0]
	pm.buffer.mu.Unlock()
	
	// Send metrics in batches of 20 (CloudWatch limit)
	for i := 0; i < len(metricsToFlush); i += 20 {
		end := i + 20
		if end > len(metricsToFlush) {
			end = len(metricsToFlush)
		}
		
		input := &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(pm.config.Namespace),
			MetricData: metricsToFlush[i:end],
		}
		
		if _, err := pm.cloudwatch.PutMetricData(ctx, input); err != nil {
			pm.logger.Error("failed to flush metrics",
				zap.Error(err),
				zap.Int("metric_count", end-i),
				zap.String("namespace", pm.config.Namespace),
			)
		} else {
			pm.logger.Debug("flushed metrics",
				zap.Int("metric_count", end-i),
				zap.String("namespace", pm.config.Namespace),
			)
		}
	}
}

// FlushOnExit flushes any remaining metrics - call this in a defer statement
func (pm *ProductionMonitor) FlushOnExit(ctx context.Context) {
	pm.flushMetrics(ctx)
}

// getBaseDimensions returns base dimensions for all metrics
func (pm *ProductionMonitor) getBaseDimensions(operation string) []types.Dimension {
	dimensions := []types.Dimension{
		{Name: aws.String("Environment"), Value: aws.String(pm.config.Environment)},
		{Name: aws.String("Service"), Value: aws.String(pm.config.ServiceName)},
		{Name: aws.String("Operation"), Value: aws.String(operation)},
	}
	
	// Add function name and version if available
	if functionName := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); functionName != "" {
		dimensions = append(dimensions,
			types.Dimension{Name: aws.String("FunctionName"), Value: aws.String(functionName)})
	}
	
	if version := os.Getenv("AWS_LAMBDA_FUNCTION_VERSION"); version != "" {
		dimensions = append(dimensions,
			types.Dimension{Name: aws.String("Version"), Value: aws.String(version)})
	}
	
	// Add custom dimensions
	for key, value := range pm.config.Dimensions {
		dimensions = append(dimensions,
			types.Dimension{Name: aws.String(key), Value: aws.String(value)})
	}
	
	return dimensions
}

// Helper functions

func (pm *ProductionMonitor) detectColdStart() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if pm.isFirstRun {
		pm.isFirstRun = false
		return true
	}
	return false
}

func (pm *ProductionMonitor) getOperationName(ctx *lift.Context) string {
	reqPath := ctx.Request.URL().Path
	if reqPath != "" {
		// Create a clean operation name from path and method
		path := sanitizePath(reqPath)
		return fmt.Sprintf("%s_%s", ctx.Request.Method, path)
	}
	return pm.config.ServiceName
}

// Production monitoring factory functions

// NewProductionLiftApp creates a Lift app with comprehensive monitoring
func NewProductionLiftApp(awsConfig aws.Config, logger *zap.Logger) (*lift.App, *ProductionMonitor) {
	monitor := NewProductionMonitor(awsConfig, DefaultProductionConfig(), logger)
	
	app := lift.New()
	app.Use(monitor.LiftMiddleware())
	
	return app, monitor
}

// NewProductionDynamORMClient creates a DynamORM client with monitoring
func NewProductionDynamORMClient(baseClient core.DB, monitor *ProductionMonitor) core.DB {
	return monitor.WrapDynamORMClient(baseClient)
}

// Helper utility functions

func getEnvironment() string {
	if env := os.Getenv("ENVIRONMENT"); env != "" {
		return env
	}
	if env := os.Getenv("STAGE"); env != "" {
		return env
	}
	return "production"
}

func getFunctionName() string {
	if name := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); name != "" {
		return name
	}
	return "unknown"
}


func classifyError(err error) string {
	if err == nil {
		return "none"
	}
	
	errorStr := err.Error()
	switch {
	case strings.Contains(errorStr, "timeout"):
		return "timeout"
	case strings.Contains(errorStr, "not found"):
		return "not_found"
	case strings.Contains(errorStr, "unauthorized"):
		return "unauthorized"
	case strings.Contains(errorStr, "forbidden"):
		return "forbidden"
	case strings.Contains(errorStr, "validation"):
		return "validation"
	case strings.Contains(errorStr, "throttl"):
		return "throttling"
	default:
		return "unknown"
	}
}

func extractUserID(ctx *lift.Context) string {
	// Extract user ID from context - implementation depends on auth system
	if userID := ctx.Get("user_id"); userID != nil {
		if id, ok := userID.(string); ok {
			return id
		}
	}
	return ""
}

func classifyUser(userID string) string {
	// Classify user type based on ID pattern or database lookup
	// This is a simplified implementation
	if strings.HasPrefix(userID, "admin_") {
		return "admin"
	}
	if strings.HasPrefix(userID, "bot_") {
		return "bot"
	}
	return "user"
}

func classifyEndpoint(path string) string {
	// Classify API endpoints for business metrics
	switch {
	case strings.Contains(path, "/statuses"):
		return "statuses"
	case strings.Contains(path, "/accounts"):
		return "accounts"
	case strings.Contains(path, "/timelines"):
		return "timelines"
	case strings.Contains(path, "/notifications"):
		return "notifications"
	case strings.Contains(path, "/inbox") || strings.Contains(path, "/outbox"):
		return "federation"
	default:
		return "other"
	}
}

func isFederationRequest(ctx *lift.Context) bool {
	// Check if this is a federation-related request
	path := ctx.Request.URL().Path
	return strings.Contains(path, "/inbox") ||
		strings.Contains(path, "/outbox") ||
		strings.Contains(path, "/.well-known")
}

func parseMemorySize(memSize string) (float64, error) {
	// Parse memory size from Lambda environment variable
	var size float64
	_, err := fmt.Sscanf(memSize, "%f", &size)
	return size, err
}

func getCurrentMemoryUsage() float64 {
	// Get current memory usage in MB
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.Alloc) / 1024 / 1024
}