// Package observability provides EMF (Embedded Metric Format) metrics for CloudWatch
// This implementation follows AWS EMF specification for serverless environments
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// EMFMetrics handles CloudWatch EMF metric emission
type EMFMetrics struct {
	logger    *zap.Logger
	namespace string
	service   string
	mu        sync.RWMutex
	metrics   map[string]float64
	metadata  map[string]interface{}
	enabled   bool
}

// EMFMetricData represents a single EMF metric
type EMFMetricData struct {
	MetricName string  `json:"metric_name"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit"`
	Timestamp  int64   `json:"timestamp"`
}

// EMFDimension represents metric dimensions
type EMFDimension struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// EMFPayload represents the complete EMF payload structure
type EMFPayload struct {
	Version    string                 `json:"_aws"`
	Timestamp  int64                  `json:"timestamp"`
	LogGroup   string                 `json:"log_group,omitempty"`
	LogStream  string                 `json:"log_stream,omitempty"`
	Namespace  string                 `json:"namespace"`
	Dimensions [][]string             `json:"dimensions"`
	Metrics    []EMFMetricData        `json:"metrics"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// EMFLogEntry represents the EMF log entry structure that CloudWatch expects
type EMFLogEntry struct {
	AWS        EMFMetadata            `json:"_aws"`
	Timestamp  int64                  `json:"timestamp,omitempty"`
	Message    string                 `json:"message,omitempty"`
	Level      string                 `json:"level,omitempty"`
	Service    string                 `json:"service"`
	Namespace  string                 `json:"namespace"`
	Dimensions map[string]string      `json:"dimensions,omitempty"`
	Metrics    map[string]interface{} `json:"metrics"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// EMFMetadata contains EMF-specific metadata
type EMFMetadata struct {
	Timestamp         int64                  `json:"Timestamp"`
	LogGroup          string                 `json:"LogGroup,omitempty"`
	LogStream         string                 `json:"LogStream,omitempty"`
	CloudWatchMetrics []EMFCloudWatchMetrics `json:"CloudWatchMetrics"`
}

// LatencyMetric represents latency tracking
type LatencyMetric struct {
	Operation string
	Start     time.Time
	Context   context.Context
}

// NewEMFMetrics creates a new EMF metrics collector
func NewEMFMetrics(logger *zap.Logger, namespace, service string) *EMFMetrics {
	enabled := os.Getenv("EMF_METRICS_ENABLED") != "false" // Default to enabled unless explicitly disabled

	return &EMFMetrics{
		logger:    logger,
		namespace: namespace,
		service:   service,
		metrics:   make(map[string]float64),
		metadata:  make(map[string]interface{}),
		enabled:   enabled,
	}
}

// PutMetric records a metric value
func (emf *EMFMetrics) PutMetric(name string, value float64, unit string, dimensions map[string]string) {
	if !emf.enabled {
		return
	}

	emf.mu.Lock()
	defer emf.mu.Unlock()

	// Create metric key with dimensions for aggregation
	key := emf.createMetricKey(name, dimensions)
	emf.metrics[key] = value

	// Store additional metadata
	for k, v := range dimensions {
		emf.metadata[k] = v
	}
	emf.metadata[name+"_unit"] = unit
	emf.metadata[name+"_timestamp"] = time.Now().UnixMilli()
}

// RecordLatency records latency metrics
func (emf *EMFMetrics) RecordLatency(operation string, duration time.Duration) {
	if !emf.enabled {
		return
	}

	durationMs := float64(duration.Nanoseconds()) / 1000000.0 // Convert to milliseconds

	dimensions := map[string]string{
		"Operation": operation,
		"Service":   emf.service,
	}

	emf.PutMetric("Latency", durationMs, "Milliseconds", dimensions)

	// Also record percentile-friendly metrics
	emf.PutMetric("LatencyP50", durationMs, "Milliseconds", dimensions)
	emf.PutMetric("LatencyP90", durationMs, "Milliseconds", dimensions)
	emf.PutMetric("LatencyP99", durationMs, "Milliseconds", dimensions)
}

// RecordThroughput records throughput metrics
func (emf *EMFMetrics) RecordThroughput(operation string, count int64) {
	if !emf.enabled {
		return
	}

	dimensions := map[string]string{
		"Operation": operation,
		"Service":   emf.service,
	}

	emf.PutMetric("Throughput", float64(count), "Count", dimensions)
	emf.PutMetric("RequestsPerSecond", float64(count), "Count/Second", dimensions)
}

// RecordError records error metrics
func (emf *EMFMetrics) RecordError(operation string, errorType string) {
	if !emf.enabled {
		return
	}

	dimensions := map[string]string{
		"Operation": operation,
		"Service":   emf.service,
		"ErrorType": errorType,
	}

	emf.PutMetric("Errors", 1.0, "Count", dimensions)
	emf.PutMetric("ErrorRate", 1.0, "Percent", dimensions)
}

// RecordSuccess records successful operations
func (emf *EMFMetrics) RecordSuccess(operation string) {
	if !emf.enabled {
		return
	}

	dimensions := map[string]string{
		"Operation": operation,
		"Service":   emf.service,
	}

	emf.PutMetric("Success", 1.0, "Count", dimensions)
	emf.PutMetric("SuccessRate", 100.0, "Percent", dimensions)
}

// RecordCost records cost metrics
func (emf *EMFMetrics) RecordCost(operation string, costUSD float64) {
	if !emf.enabled {
		return
	}

	dimensions := map[string]string{
		"Operation": operation,
		"Service":   emf.service,
	}

	emf.PutMetric("Cost", costUSD, "None", dimensions)
	emf.PutMetric("CostMicrocents", costUSD*1000000, "None", dimensions) // For compatibility with existing cost tracking
}

// RecordBusinessMetric records business-specific metrics
func (emf *EMFMetrics) RecordBusinessMetric(metricName string, value float64, unit string, dimensions map[string]string) {
	if !emf.enabled {
		return
	}

	if dimensions == nil {
		dimensions = make(map[string]string)
	}
	dimensions["Service"] = emf.service

	emf.PutMetric(metricName, value, unit, dimensions)
}

// RecordFederationMetric records federation-specific metrics
func (emf *EMFMetrics) RecordFederationMetric(operation string, instance string, success bool, latencyMs float64) {
	if !emf.enabled {
		return
	}

	dimensions := map[string]string{
		"Operation": operation,
		"Instance":  instance,
		"Service":   emf.service,
	}

	if success {
		emf.PutMetric("FederationSuccess", 1.0, "Count", dimensions)
	} else {
		emf.PutMetric("FederationError", 1.0, "Count", dimensions)
	}

	if latencyMs > 0 {
		emf.PutMetric("FederationLatency", latencyMs, "Milliseconds", dimensions)
	}
}

// RecordConcurrency records concurrency metrics
func (emf *EMFMetrics) RecordConcurrency(operation string, activeCount int64) {
	if !emf.enabled {
		return
	}

	dimensions := map[string]string{
		"Operation": operation,
		"Service":   emf.service,
	}

	emf.PutMetric("Concurrency", float64(activeCount), "Count", dimensions)
	emf.PutMetric("ActiveConnections", float64(activeCount), "Count", dimensions)
}

// RecordQueueDepth records queue depth metrics
func (emf *EMFMetrics) RecordQueueDepth(queueName string, depth int64) {
	if !emf.enabled {
		return
	}

	dimensions := map[string]string{
		"Queue":   queueName,
		"Service": emf.service,
	}

	emf.PutMetric("QueueDepth", float64(depth), "Count", dimensions)

	// Alert-friendly metrics
	if depth > 10000 {
		emf.PutMetric("QueueDepthCritical", 1.0, "Count", dimensions)
	} else if depth > 1000 {
		emf.PutMetric("QueueDepthWarning", 1.0, "Count", dimensions)
	} else {
		emf.PutMetric("QueueDepthHealthy", 1.0, "Count", dimensions)
	}
}

// StartLatencyTimer starts tracking latency for an operation
func (emf *EMFMetrics) StartLatencyTimer(ctx context.Context, operation string) *LatencyMetric {
	return &LatencyMetric{
		Operation: operation,
		Start:     time.Now(),
		Context:   ctx,
	}
}

// Finish completes latency tracking and records the metric
func (lm *LatencyMetric) Finish(emf *EMFMetrics, success bool) {
	duration := time.Since(lm.Start)

	emf.RecordLatency(lm.Operation, duration)

	if success {
		emf.RecordSuccess(lm.Operation)
	}
}

// FinishWithError completes latency tracking and records error
func (lm *LatencyMetric) FinishWithError(emf *EMFMetrics, errorType string) {
	duration := time.Since(lm.Start)

	emf.RecordLatency(lm.Operation, duration)
	emf.RecordError(lm.Operation, errorType)
}

// Flush emits all accumulated metrics to CloudWatch via structured logs
func (emf *EMFMetrics) Flush() {
	if !emf.enabled {
		return
	}

	emf.mu.Lock()
	defer emf.mu.Unlock()

	if len(emf.metrics) == 0 {
		return
	}

	// Create EMF log entry
	entry := EMFLogEntry{
		AWS: EMFMetadata{
			Timestamp: time.Now().UnixMilli(),
			LogGroup:  os.Getenv("AWS_LAMBDA_LOG_GROUP_NAME"),
			LogStream: os.Getenv("AWS_LAMBDA_LOG_STREAM_NAME"),
			CloudWatchMetrics: []EMFCloudWatchMetrics{
				{
					Namespace:  emf.namespace,
					Dimensions: emf.buildDimensionSets(),
					Metrics:    emf.extractMetricDefinitions(),
				},
			},
		},
		Timestamp:  time.Now().UnixMilli(),
		Service:    emf.service,
		Namespace:  emf.namespace,
		Metrics:    emf.buildMetricsMap(),
		Properties: emf.metadata,
	}

	// Marshal and emit as structured log
	if jsonBytes, err := json.Marshal(entry); err == nil {
		// This will be picked up by CloudWatch and converted to metrics
		emf.logger.Info("EMF_METRICS", zap.String("emf", string(jsonBytes)))
	} else {
		emf.logger.Error("failed to marshal EMF metrics", zap.Error(err))
	}

	// Reset metrics after emission
	emf.metrics = make(map[string]float64)
	emf.metadata = make(map[string]interface{})
}

// Helper methods

func (emf *EMFMetrics) createMetricKey(name string, dimensions map[string]string) string {
	key := name
	for k, v := range dimensions {
		key += fmt.Sprintf(":%s=%s", k, v)
	}
	return key
}

func (emf *EMFMetrics) buildDimensionSets() [][]string {
	// Build dimension sets for EMF
	dimensionSet := []string{"Service"}

	// Add other dimensions found in metadata
	for k := range emf.metadata {
		if k != "Service" && !emf.isMetricMetadata(k) {
			dimensionSet = append(dimensionSet, k)
		}
	}

	return [][]string{dimensionSet}
}

func (emf *EMFMetrics) extractMetricDefinitions() []EMFMetricDefinition {
	var definitions []EMFMetricDefinition
	seen := make(map[string]bool)

	for key := range emf.metrics {
		name := emf.extractMetricNameFromKey(key)
		if !seen[name] {
			definitions = append(definitions, EMFMetricDefinition{
				Name: name,
				Unit: "None", // Default unit, should be set per metric
			})
			seen[name] = true
		}
	}

	return definitions
}

func (emf *EMFMetrics) extractMetricNameFromKey(key string) string {
	// Extract metric name from key (everything before first colon)
	for i, c := range key {
		if c == ':' {
			return key[:i]
		}
	}
	return key
}

func (emf *EMFMetrics) buildMetricsMap() map[string]interface{} {
	metricsMap := make(map[string]interface{})

	for key, value := range emf.metrics {
		name := emf.extractMetricNameFromKey(key)
		metricsMap[name] = value
	}

	return metricsMap
}

func (emf *EMFMetrics) isMetricMetadata(key string) bool {
	return key[len(key)-5:] == "_unit" || key[len(key)-10:] == "_timestamp"
}

// SetProperty adds a property to the EMF output
func (emf *EMFMetrics) SetProperty(key string, value interface{}) {
	if !emf.enabled {
		return
	}

	emf.mu.Lock()
	defer emf.mu.Unlock()

	emf.metadata[key] = value
}

// AddDimension adds a dimension that will be applied to all metrics
func (emf *EMFMetrics) AddDimension(name, value string) {
	if !emf.enabled {
		return
	}

	emf.SetProperty(name, value)
}

// IsEnabled returns whether EMF metrics are enabled
func (emf *EMFMetrics) IsEnabled() bool {
	return emf.enabled
}

// SetEnabled enables or disables EMF metrics
func (emf *EMFMetrics) SetEnabled(enabled bool) {
	emf.mu.Lock()
	defer emf.mu.Unlock()

	emf.enabled = enabled
}
