package observability

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// EMFMetricsCollector implements CloudWatch Embedded Metrics Format for serverless environments
// It eliminates polling patterns and writes metrics directly to stdout for Lambda integration
type EMFMetricsCollector struct {
	namespace  string
	dimensions map[string]string
	buffer     *EMFBuffer
	logger     *zap.Logger
	mu         sync.RWMutex
}

// EMFBuffer holds metrics before flushing - no background goroutines
type EMFBuffer struct {
	metrics []EMFMetric
	maxSize int
	mu      sync.Mutex
}

// EMFMetric represents a metric in EMF format
type EMFMetric struct {
	Name       string
	Value      float64
	Unit       string
	Dimensions map[string]string
	Timestamp  int64
}

// EMFLog represents the complete EMF log structure written to stdout
type EMFLog struct {
	AWS        EMFMetadata            `json:"_aws"`
	Timestamp  int64                  `json:"Timestamp,omitempty"`
	Dimensions map[string]string      `json:",inline"`
	Metrics    map[string]interface{} `json:",inline"`
}

// EMFCloudWatchMetrics defines the metrics structure for CloudWatch
type EMFCloudWatchMetrics struct {
	Namespace  string                `json:"Namespace"`
	Dimensions [][]string            `json:"Dimensions"`
	Metrics    []EMFMetricDefinition `json:"Metrics"`
}

// EMFMetricDefinition defines a single metric
type EMFMetricDefinition struct {
	Name string `json:"Name"`
	Unit string `json:"Unit"`
}

// NewEMFMetricsCollector creates a new EMF-based metrics collector optimized for Lambda
func NewEMFMetricsCollector(namespace string, logger *zap.Logger) *EMFMetricsCollector {
	return &EMFMetricsCollector{
		namespace: namespace,
		dimensions: map[string]string{
			"FunctionName": getEnvWithDefault("AWS_LAMBDA_FUNCTION_NAME", "unknown"),
			"Environment":  getEnvironment(),
			"Version":      getEnvWithDefault("AWS_LAMBDA_FUNCTION_VERSION", "$LATEST"),
		},
		buffer: &EMFBuffer{
			metrics: make([]EMFMetric, 0, 100), // Start with reasonable capacity
			maxSize: 100,                       // CloudWatch EMF recommended batch size
		},
		logger: logger,
	}
}

// RecordMetric records a custom metric with optional additional dimensions
// This is thread-safe and does not use background goroutines
func (emc *EMFMetricsCollector) RecordMetric(name string, value float64, unit types.StandardUnit, dimensions ...types.Dimension) {
	// Convert CloudWatch types to EMF format
	dimMap := make(map[string]string)
	for _, dim := range dimensions {
		if dim.Name != nil && dim.Value != nil {
			dimMap[*dim.Name] = *dim.Value
		}
	}

	emc.recordMetricWithDimensions(name, value, convertUnit(unit), dimMap)
}

// RecordLatency records operation latency - optimized for Lambda use
func (emc *EMFMetricsCollector) RecordLatency(operation string, duration time.Duration) {
	emc.recordMetricWithDimensions(
		"OperationLatency",
		float64(duration.Milliseconds()),
		"Milliseconds",
		map[string]string{"Operation": operation},
	)
}

// RecordThroughput records operation throughput
func (emc *EMFMetricsCollector) RecordThroughput(operation string, count int64) {
	emc.recordMetricWithDimensions(
		"OperationThroughput",
		float64(count),
		"Count",
		map[string]string{"Operation": operation},
	)
}

// RecordCost records operation cost in USD
func (emc *EMFMetricsCollector) RecordCost(operation string, costUSD float64) {
	emc.recordMetricWithDimensions(
		"OperationCost",
		costUSD,
		"None",
		map[string]string{"Operation": operation},
	)
}

// RecordErrorRate records error rate as a percentage
func (emc *EMFMetricsCollector) RecordErrorRate(operation string, errorCount, totalCount int64) {
	errorRate := 0.0
	if totalCount > 0 {
		errorRate = float64(errorCount) / float64(totalCount) * 100.0
	}

	emc.recordMetricWithDimensions(
		"ErrorRate",
		errorRate,
		"Percent",
		map[string]string{"Operation": operation},
	)
}

// RecordPerformanceMetrics records Lambda runtime performance metrics
func (emc *EMFMetricsCollector) RecordPerformanceMetrics(metrics *PerformanceMetrics) {
	// Record all performance metrics with consistent dimensions
	perfDims := map[string]string{"MetricType": "Performance"}

	emc.recordMetricWithDimensions("ColdStartDuration", float64(metrics.ColdStartDuration.Milliseconds()), "Milliseconds", perfDims)
	emc.recordMetricWithDimensions("ExecutionDuration", float64(metrics.ExecutionDuration.Milliseconds()), "Milliseconds", perfDims)
	emc.recordMetricWithDimensions("MemoryUsed", float64(metrics.MemoryUsed), "Bytes", perfDims)
	emc.recordMetricWithDimensions("MemoryAllocated", float64(metrics.MemoryAllocated), "Bytes", perfDims)
	emc.recordMetricWithDimensions("CPUUtilization", metrics.CPUUtilization, "Percent", perfDims)
	emc.recordMetricWithDimensions("GoroutineCount", float64(metrics.GoroutineCount), "Count", perfDims)
	emc.recordMetricWithDimensions("GCPauseTime", float64(metrics.GCPauseTime.Microseconds()), "Microseconds", perfDims)
}

// recordMetricWithDimensions is the internal method for recording metrics
func (emc *EMFMetricsCollector) recordMetricWithDimensions(name string, value float64, unit string, extraDims map[string]string) {
	// Merge base dimensions with extra dimensions
	allDims := make(map[string]string)

	emc.mu.RLock()
	for k, v := range emc.dimensions {
		allDims[k] = v
	}
	emc.mu.RUnlock()

	for k, v := range extraDims {
		if v != "" { // Only add non-empty values
			allDims[k] = v
		}
	}

	metric := EMFMetric{
		Name:       name,
		Value:      value,
		Unit:       unit,
		Dimensions: allDims,
		Timestamp:  time.Now().UnixMilli(),
	}

	// Add to buffer - thread safe
	emc.buffer.Add(metric)

	// Auto-flush if buffer is approaching capacity (no background goroutines)
	if emc.buffer.ShouldFlush() {
		if err := emc.Flush(); err != nil {
			emc.logger.Error("failed to flush metrics", zap.Error(err))
		}
	}
}

// Flush writes all buffered metrics to stdout in EMF format
// This method is synchronous and safe to call from Lambda handlers
func (emc *EMFMetricsCollector) Flush() error {
	metrics := emc.buffer.GetAndClear()
	if err := common.ValidateSliceNotEmpty("metrics", metrics); err != nil {
		return nil
	}

	// Group metrics by dimension combinations for efficient EMF logs
	groups := emc.groupMetricsByDimensions(metrics)

	// Write each group as a separate EMF log entry
	for _, group := range groups {
		if err := emc.writeEMFLog(group); err != nil {
			emc.logger.Error("failed to write EMF log", zap.Error(err))
			return err
		}
	}

	emc.logger.Debug("flushed metrics to EMF", zap.Int("metric_count", len(metrics)))
	return nil
}

// groupMetricsByDimensions groups metrics by their dimension combinations
func (emc *EMFMetricsCollector) groupMetricsByDimensions(metrics []EMFMetric) [][]EMFMetric {
	groups := make(map[string][]EMFMetric)

	for _, metric := range metrics {
		// Create a key from dimensions
		key := emc.dimensionsKey(metric.Dimensions)
		groups[key] = append(groups[key], metric)
	}

	// Convert map to slice
	result := make([][]EMFMetric, 0, len(groups))
	for _, group := range groups {
		result = append(result, group)
	}

	return result
}

// writeEMFLog writes a group of metrics as a single EMF log entry to stdout
func (emc *EMFMetricsCollector) writeEMFLog(metrics []EMFMetric) error {
	if err := common.ValidateSliceNotEmpty("metrics", metrics); err != nil {
		return nil
	}

	// Use the first metric's dimensions as the base (they should all be the same in a group)
	baseDims := metrics[0].Dimensions
	timestamp := time.Now().UnixMilli()

	// Build dimension arrays for EMF
	dimensionKeys := make([]string, 0, len(baseDims))
	for key := range baseDims {
		dimensionKeys = append(dimensionKeys, key)
	}

	// Build metric definitions
	metricDefs := make([]EMFMetricDefinition, len(metrics))
	metricValues := make(map[string]interface{})

	for i, metric := range metrics {
		metricDefs[i] = EMFMetricDefinition{
			Name: metric.Name,
			Unit: metric.Unit,
		}
		metricValues[metric.Name] = metric.Value
	}

	// Create EMF log structure
	emfLog := EMFLog{
		AWS: EMFMetadata{
			Timestamp: timestamp,
			CloudWatchMetrics: []EMFCloudWatchMetrics{
				{
					Namespace:  emc.namespace,
					Dimensions: [][]string{dimensionKeys},
					Metrics:    metricDefs,
				},
			},
		},
		Timestamp:  timestamp,
		Dimensions: baseDims,
		Metrics:    metricValues,
	}

	// Marshal to JSON and write to stdout
	jsonData, err := json.Marshal(emfLog)
	if err != nil {
		return fmt.Errorf("failed to marshal EMF log: %w", err)
	}

	// Write to stdout - CloudWatch Lambda integration captures this automatically
	fmt.Println(string(jsonData))

	return nil
}

// dimensionsKey creates a consistent key from dimensions for grouping
func (emc *EMFMetricsCollector) dimensionsKey(dims map[string]string) string {
	if len(dims) == 0 {
		return ""
	}

	// Create a consistent key by sorting dimension names
	keys := make([]string, 0, len(dims))
	for k := range dims {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var key string
	for _, k := range keys {
		key += fmt.Sprintf("%s=%s;", k, dims[k])
	}
	return key
}

// SetDimension adds or updates a default dimension
func (emc *EMFMetricsCollector) SetDimension(name, value string) {
	emc.mu.Lock()
	defer emc.mu.Unlock()
	emc.dimensions[name] = value
}

// RemoveDimension removes a default dimension
func (emc *EMFMetricsCollector) RemoveDimension(name string) {
	emc.mu.Lock()
	defer emc.mu.Unlock()
	delete(emc.dimensions, name)
}

// GetBufferSize returns the current buffer size (for monitoring)
func (emc *EMFMetricsCollector) GetBufferSize() int {
	return emc.buffer.Size()
}

// Stop is a no-op for EMF collector since there are no background goroutines
// This maintains interface compatibility with the polling-based collectors
func (emc *EMFMetricsCollector) Stop() {
	// Final flush to ensure all metrics are sent
	if err := emc.Flush(); err != nil {
		emc.logger.Error("error during final metrics flush", zap.Error(err))
	}
}

// EMFBuffer methods

// Add adds a metric to the buffer (thread-safe)
func (eb *EMFBuffer) Add(metric EMFMetric) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.metrics = append(eb.metrics, metric)
}

// ShouldFlush returns true if the buffer should be flushed
func (eb *EMFBuffer) ShouldFlush() bool {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return len(eb.metrics) >= eb.maxSize
}

// GetAndClear returns all metrics and clears the buffer (thread-safe)
func (eb *EMFBuffer) GetAndClear() []EMFMetric {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if err := common.ValidateSliceNotEmpty("eb.metrics", eb.metrics); err != nil {
		return nil
	}

	// Copy metrics
	result := make([]EMFMetric, len(eb.metrics))
	copy(result, eb.metrics)

	// Clear buffer
	eb.metrics = eb.metrics[:0]

	return result
}

// Size returns the current buffer size (thread-safe)
func (eb *EMFBuffer) Size() int {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return len(eb.metrics)
}

// Helper functions

// convertUnit converts CloudWatch StandardUnit to EMF string format
func convertUnit(unit types.StandardUnit) string {
	switch unit {
	case types.StandardUnitSeconds:
		return "Seconds"
	case types.StandardUnitMicroseconds:
		return "Microseconds"
	case types.StandardUnitMilliseconds:
		return "Milliseconds"
	case types.StandardUnitBytes:
		return "Bytes"
	case types.StandardUnitKilobytes:
		return "Kilobytes"
	case types.StandardUnitMegabytes:
		return "Megabytes"
	case types.StandardUnitGigabytes:
		return "Gigabytes"
	case types.StandardUnitTerabytes:
		return "Terabytes"
	case types.StandardUnitBits:
		return "Bits"
	case types.StandardUnitKilobits:
		return "Kilobits"
	case types.StandardUnitMegabits:
		return "Megabits"
	case types.StandardUnitGigabits:
		return "Gigabits"
	case types.StandardUnitTerabits:
		return "Terabits"
	case types.StandardUnitPercent:
		return "Percent"
	case types.StandardUnitCount:
		return "Count"
	case types.StandardUnitBytesSecond:
		return "Bytes/Second"
	case types.StandardUnitKilobytesSecond:
		return "Kilobytes/Second"
	case types.StandardUnitMegabytesSecond:
		return "Megabytes/Second"
	case types.StandardUnitGigabytesSecond:
		return "Gigabytes/Second"
	case types.StandardUnitTerabytesSecond:
		return "Terabytes/Second"
	case types.StandardUnitBitsSecond:
		return "Bits/Second"
	case types.StandardUnitKilobitsSecond:
		return "Kilobits/Second"
	case types.StandardUnitMegabitsSecond:
		return "Megabits/Second"
	case types.StandardUnitGigabitsSecond:
		return "Gigabits/Second"
	case types.StandardUnitTerabitsSecond:
		return "Terabits/Second"
	case types.StandardUnitCountSecond:
		return "Count/Second"
	case types.StandardUnitNone:
		return "None"
	default:
		return "None"
	}
}

// getEnvWithDefault gets an environment variable with a default value
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// EMF-specific helper functions use the existing implementations from metrics.go
// to avoid code duplication
