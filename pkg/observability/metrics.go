// Package observability provides serverless-friendly metrics collection for AWS Lambda.
//
// Usage in Lambda functions:
//  1. Create a collector during init() or at handler start
//  2. Record metrics during execution
//  3. Call Flush() before handler returns to send metrics to CloudWatch
//
// This implementation avoids background goroutines and polling to be compatible
// with serverless environments where Lambda containers can be frozen.
package observability

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"go.uber.org/zap"
)

// MetricsCollector aggregates and publishes custom metrics
type MetricsCollector struct {
	client     cloudWatchPutMetricDataAPI
	namespace  string
	dimensions []types.Dimension
	metrics    map[string]*MetricBuffer
	logger     *zap.Logger
	mu         sync.RWMutex
}

type cloudWatchPutMetricDataAPI interface {
	PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error)
}

// MetricBuffer represents a buffer for metrics collection
type MetricBuffer struct {
	values    []float64
	unit      types.StandardUnit
	timestamp time.Time
	mu        sync.Mutex
}

// PerformanceMetrics contains runtime performance data
type PerformanceMetrics struct {
	ColdStartDuration time.Duration
	ExecutionDuration time.Duration
	MemoryUsed        int64
	MemoryAllocated   int64
	CPUUtilization    float64
	GoroutineCount    int
	GCPauseTime       time.Duration
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(client cloudWatchPutMetricDataAPI, namespace string, logger *zap.Logger) *MetricsCollector {
	return &MetricsCollector{
		client:    client,
		namespace: namespace,
		metrics:   make(map[string]*MetricBuffer),
		logger:    logger,
		dimensions: []types.Dimension{
			{
				Name:  aws.String("FunctionName"),
				Value: aws.String(os.Getenv("AWS_LAMBDA_FUNCTION_NAME")),
			},
			{
				Name:  aws.String("Environment"),
				Value: aws.String(getEnvironment()),
			},
		},
	}
}

// RecordMetric records a custom metric
func (mc *MetricsCollector) RecordMetric(name string, value float64, unit types.StandardUnit, dimensions ...types.Dimension) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := mc.getMetricKey(name, dimensions)

	if buffer, exists := mc.metrics[key]; exists {
		buffer.mu.Lock()
		buffer.values = append(buffer.values, value)
		buffer.mu.Unlock()
	} else {
		mc.metrics[key] = &MetricBuffer{
			values:    []float64{value},
			unit:      unit,
			timestamp: time.Now(),
		}
	}
}

// RecordLatency records operation latency
func (mc *MetricsCollector) RecordLatency(operation string, duration time.Duration) {
	mc.RecordMetric(
		"OperationLatency",
		float64(duration.Milliseconds()),
		types.StandardUnitMilliseconds,
		types.Dimension{
			Name:  aws.String("Operation"),
			Value: aws.String(operation),
		},
	)
}

// RecordThroughput records operation throughput
func (mc *MetricsCollector) RecordThroughput(operation string, count int64) {
	mc.RecordMetric(
		"OperationThroughput",
		float64(count),
		types.StandardUnitCount,
		types.Dimension{
			Name:  aws.String("Operation"),
			Value: aws.String(operation),
		},
	)
}

// RecordCost records operation cost
func (mc *MetricsCollector) RecordCost(operation string, costUSD float64) {
	mc.RecordMetric(
		"OperationCost",
		costUSD,
		types.StandardUnitNone,
		types.Dimension{
			Name:  aws.String("Operation"),
			Value: aws.String(operation),
		},
	)
}

// RecordErrorRate records error rate
func (mc *MetricsCollector) RecordErrorRate(operation string, errorCount, totalCount int64) {
	errorRate := 0.0
	if totalCount > 0 {
		errorRate = float64(errorCount) / float64(totalCount) * 100.0
	}

	mc.RecordMetric(
		"ErrorRate",
		errorRate,
		types.StandardUnitPercent,
		types.Dimension{
			Name:  aws.String("Operation"),
			Value: aws.String(operation),
		},
	)
}

// RecordPerformanceMetrics records runtime performance metrics
func (mc *MetricsCollector) RecordPerformanceMetrics(metrics *PerformanceMetrics) {
	mc.RecordMetric("ColdStartDuration", float64(metrics.ColdStartDuration.Milliseconds()), types.StandardUnitMilliseconds)
	mc.RecordMetric("ExecutionDuration", float64(metrics.ExecutionDuration.Milliseconds()), types.StandardUnitMilliseconds)
	mc.RecordMetric("MemoryUsed", float64(metrics.MemoryUsed), types.StandardUnitBytes)
	mc.RecordMetric("MemoryAllocated", float64(metrics.MemoryAllocated), types.StandardUnitBytes)
	mc.RecordMetric("CPUUtilization", metrics.CPUUtilization, types.StandardUnitPercent)
	mc.RecordMetric("GoroutineCount", float64(metrics.GoroutineCount), types.StandardUnitCount)
	mc.RecordMetric("GCPauseTime", float64(metrics.GCPauseTime.Microseconds()), types.StandardUnitMicroseconds)
}

// GetPerformanceMetrics collects current runtime performance metrics
func GetPerformanceMetrics(startTime time.Time, initTime time.Time) *PerformanceMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Determine if this is a cold start
	coldStartDuration := time.Duration(0)
	if os.Getenv("AWS_LAMBDA_INITIALIZATION_TYPE") != "provisioned-concurrency" {
		coldStartDuration = time.Since(initTime)
	}

	// Safe uint64 to int64 conversions with overflow checks
	const maxInt64AsUint64 = uint64(9223372036854775807) // math.MaxInt64
	var memUsed int64
	if m.Alloc > maxInt64AsUint64 {
		memUsed = ^int64(0) // Max int64
	} else {
		memUsed = int64(m.Alloc)
	}

	var memAllocated int64
	if m.TotalAlloc > maxInt64AsUint64 {
		memAllocated = ^int64(0) // Max int64
	} else {
		memAllocated = int64(m.TotalAlloc)
	}

	// Safe uint64 to Duration conversion
	var gcPauseTime time.Duration
	if m.PauseTotalNs > maxInt64AsUint64 {
		gcPauseTime = time.Duration(^int64(0)) // Max duration
	} else {
		gcPauseTime = time.Duration(m.PauseTotalNs)
	}

	return &PerformanceMetrics{
		ColdStartDuration: coldStartDuration,
		ExecutionDuration: time.Since(startTime),
		MemoryUsed:        memUsed,
		MemoryAllocated:   memAllocated,
		CPUUtilization:    calculateCPUUtilization(),
		GoroutineCount:    runtime.NumGoroutine(),
		GCPauseTime:       gcPauseTime,
	}
}

// Flush manually flushes all accumulated metrics to CloudWatch.
// This should be called before Lambda function returns to ensure metrics are sent.
// In serverless environments, call this at the end of each request handler.
func (mc *MetricsCollector) Flush() {
	mc.flushMetrics()
}

// flushMetrics sends accumulated metrics to CloudWatch
func (mc *MetricsCollector) flushMetrics() {
	mc.mu.Lock()
	metricsToFlush := make(map[string]*MetricBuffer)
	for k, v := range mc.metrics {
		metricsToFlush[k] = v
	}
	mc.metrics = make(map[string]*MetricBuffer) // Reset
	mc.mu.Unlock()

	if len(metricsToFlush) == 0 {
		return
	}

	// Prepare metric data
	var metricData []types.MetricDatum
	for key, buffer := range metricsToFlush {
		buffer.mu.Lock()
		if len(buffer.values) > 0 {
			// Calculate statistics
			sum, minVal, maxVal := calculateStats(buffer.values)
			count := float64(len(buffer.values))

			metricDatum := types.MetricDatum{
				MetricName: aws.String(mc.extractMetricName(key)),
				Dimensions: mc.extractDimensions(key),
				Timestamp:  aws.Time(buffer.timestamp),
				Unit:       buffer.unit,
				StatisticValues: &types.StatisticSet{
					Sum:         aws.Float64(sum),
					SampleCount: aws.Float64(count),
					Minimum:     aws.Float64(minVal),
					Maximum:     aws.Float64(maxVal),
				},
			}
			metricData = append(metricData, metricDatum)
		}
		buffer.mu.Unlock()
	}

	// Send to CloudWatch
	if len(metricData) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err := mc.client.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(mc.namespace),
			MetricData: metricData,
		})
		if err != nil {
			mc.logger.Error("failed to publish metrics", zap.Error(err))
		} else {
			mc.logger.Debug("published metrics", zap.Int("count", len(metricData)))
		}
	}
}

// Helper functions

func (mc *MetricsCollector) getMetricKey(name string, dimensions []types.Dimension) string {
	key := name
	for _, dim := range dimensions {
		key += fmt.Sprintf(":%s=%s", *dim.Name, *dim.Value)
	}
	return key
}

func (mc *MetricsCollector) extractMetricName(key string) string {
	// Simple extraction - just return everything before the first colon
	for i, c := range key {
		if c == ':' {
			return key[:i]
		}
	}
	return key
}

func (mc *MetricsCollector) extractDimensions(_ string) []types.Dimension {
	// Add base dimensions
	dimensions := make([]types.Dimension, len(mc.dimensions))
	copy(dimensions, mc.dimensions)

	// Extract additional dimensions from key
	// This is a simplified implementation
	return dimensions
}

func calculateStats(values []float64) (sum, minVal, maxVal float64) {
	if err := common.ValidateSliceNotEmpty("values", values); err != nil {
		return 0, 0, 0
	}

	sum = values[0]
	minVal = values[0]
	maxVal = values[0]

	for i := 1; i < len(values); i++ {
		sum += values[i]
		if values[i] < minVal {
			minVal = values[i]
		}
		if values[i] > maxVal {
			maxVal = values[i]
		}
	}

	return sum, minVal, maxVal
}

func calculateCPUUtilization() float64 {
	// In AWS Lambda, true CPU utilization is not directly available
	// Use a composite metric based on runtime stats as a proxy

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Normalize metrics to 0-1 range and combine them
	goroutineLoad := mathMin(float64(runtime.NumGoroutine())/1000.0, 1.0) // Cap at 1000 goroutines = 100%
	gcLoad := mathMin(memStats.GCCPUFraction, 1.0)                        // GC CPU fraction
	heapLoad := 0.0
	if memStats.HeapSys > 0 {
		heapLoad = mathMin(float64(memStats.HeapInuse)/float64(memStats.HeapSys), 1.0)
	}

	// Weighted combination: GC 50%, goroutines 30%, memory 20%
	utilization := (gcLoad * 0.5) + (goroutineLoad * 0.3) + (heapLoad * 0.2)
	return utilization
}

func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func getEnvironment() string {
	cfg := config.Get()
	if cfg.Environment != "" {
		return cfg.Environment
	}
	if cfg.Stage != "" {
		return cfg.Stage
	}
	return StatusUnknown
}
