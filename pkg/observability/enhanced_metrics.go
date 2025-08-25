package observability

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MetricType represents different types of metrics
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// MetricLevel represents the importance level of a metric
type MetricLevel string

const (
	MetricLevelCritical MetricLevel = "critical"
	MetricLevelHigh     MetricLevel = "high"
	MetricLevelMedium   MetricLevel = "medium"
	MetricLevelLow      MetricLevel = "low"
)

// EnhancedMetric represents a comprehensive metric with metadata
type EnhancedMetric struct {
	Name        string                 `json:"name"`
	Type        MetricType            `json:"type"`
	Level       MetricLevel           `json:"level"`
	Value       float64               `json:"value"`
	Unit        string                `json:"unit"`
	Description string                `json:"description"`
	Labels      map[string]string     `json:"labels"`
	Timestamp   time.Time             `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// PercentileMetric represents percentile-based metrics
type PercentileMetric struct {
	P50 float64 `json:"p50"`
	P75 float64 `json:"p75"`
	P90 float64 `json:"p90"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

// ErrorRateMetric represents error rate metrics
type ErrorRateMetric struct {
	Total      int64   `json:"total_requests"`
	Errors     int64   `json:"error_count"`
	ErrorRate  float64 `json:"error_rate"`
	By4xx      int64   `json:"4xx_errors"`
	By5xx      int64   `json:"5xx_errors"`
	ByType     map[string]int64 `json:"errors_by_type"`
}

// CapacityMetrics represents DynamoDB capacity consumption
type CapacityMetrics struct {
	ReadCapacityUnits     float64 `json:"read_capacity_units"`
	WriteCapacityUnits    float64 `json:"write_capacity_units"`
	ReadThrottles         int64   `json:"read_throttles"`
	WriteThrottles        int64   `json:"write_throttles"`
	ConsumedReadCapacity  float64 `json:"consumed_read_capacity"`
	ConsumedWriteCapacity float64 `json:"consumed_write_capacity"`
}

// CacheMetrics represents cache performance metrics
type CacheMetrics struct {
	HitRate     float64 `json:"hit_rate"`
	MissRate    float64 `json:"miss_rate"`
	TotalHits   int64   `json:"total_hits"`
	TotalMisses int64   `json:"total_misses"`
	EvictionRate float64 `json:"eviction_rate"`
}

// EnhancedMetricsCollector collects and aggregates metrics
type EnhancedMetricsCollector struct {
	mu                sync.RWMutex
	metrics           map[string]*EnhancedMetric
	logger            *zap.Logger
	requestLatencies  []float64
	errorCounts       map[string]int64
	capacityMetrics   *CapacityMetrics
	cacheMetrics      *CacheMetrics
	startTime         time.Time
	lastFlush         time.Time
	flushInterval     time.Duration
}

// NewEnhancedMetricsCollector creates a new enhanced metrics collector
func NewEnhancedMetricsCollector(logger *zap.Logger) *EnhancedMetricsCollector {
	return &EnhancedMetricsCollector{
		metrics:          make(map[string]*EnhancedMetric),
		logger:           logger,
		requestLatencies: make([]float64, 0, 1000),
		errorCounts:      make(map[string]int64),
		capacityMetrics:  &CapacityMetrics{},
		cacheMetrics:     &CacheMetrics{},
		startTime:        time.Now(),
		lastFlush:        time.Now(),
		flushInterval:    time.Minute * 5, // Flush every 5 minutes
	}
}

// RecordLatency records request latency
func (c *EnhancedMetricsCollector) RecordLatency(operation string, latency time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	latencyMs := float64(latency.Nanoseconds()) / 1e6
	c.requestLatencies = append(c.requestLatencies, latencyMs)

	// Keep only last 1000 latencies to prevent memory growth
	if len(c.requestLatencies) > 1000 {
		c.requestLatencies = c.requestLatencies[len(c.requestLatencies)-1000:]
	}

	// Update latency metric
	c.recordMetric(&EnhancedMetric{
		Name:        "request_latency",
		Type:        MetricTypeHistogram,
		Level:       MetricLevelCritical,
		Value:       latencyMs,
		Unit:        "ms",
		Description: "Request latency in milliseconds",
		Labels: map[string]string{
			"operation": operation,
		},
		Timestamp: time.Now(),
	})
}

// RecordError records error occurrences
func (c *EnhancedMetricsCollector) RecordError(errorType string, statusCode int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.errorCounts[errorType]++
	
	// Categorize by status code
	category := "other"
	if statusCode >= 400 && statusCode < 500 {
		category = "4xx"
	} else if statusCode >= 500 {
		category = "5xx"
	}

	c.errorCounts[category]++

	c.recordMetric(&EnhancedMetric{
		Name:        "error_count",
		Type:        MetricTypeCounter,
		Level:       MetricLevelHigh,
		Value:       1,
		Unit:        "count",
		Description: "Error occurrences",
		Labels: map[string]string{
			"error_type":  errorType,
			"status_code": fmt.Sprintf("%d", statusCode),
			"category":    category,
		},
		Timestamp: time.Now(),
	})
}

// RecordDynamoDBCapacity records DynamoDB capacity consumption
func (c *EnhancedMetricsCollector) RecordDynamoDBCapacity(operation string, consumedRCU, consumedWCU float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.capacityMetrics.ConsumedReadCapacity += consumedRCU
	c.capacityMetrics.ConsumedWriteCapacity += consumedWCU

	c.recordMetric(&EnhancedMetric{
		Name:        "dynamodb_consumed_capacity",
		Type:        MetricTypeGauge,
		Level:       MetricLevelCritical,
		Value:       consumedRCU + consumedWCU,
		Unit:        "units",
		Description: "DynamoDB consumed capacity units",
		Labels: map[string]string{
			"operation": operation,
			"type":      "total",
		},
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"read_capacity":  consumedRCU,
			"write_capacity": consumedWCU,
		},
	})
}

// RecordCacheHit records cache hit/miss
func (c *EnhancedMetricsCollector) RecordCacheHit(hit bool, cacheType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if hit {
		c.cacheMetrics.TotalHits++
	} else {
		c.cacheMetrics.TotalMisses++
	}

	// Recalculate hit rate
	total := c.cacheMetrics.TotalHits + c.cacheMetrics.TotalMisses
	if total > 0 {
		c.cacheMetrics.HitRate = float64(c.cacheMetrics.TotalHits) / float64(total)
		c.cacheMetrics.MissRate = float64(c.cacheMetrics.TotalMisses) / float64(total)
	}

	hitValue := 0.0
	if hit {
		hitValue = 1.0
	}

	c.recordMetric(&EnhancedMetric{
		Name:        "cache_hit_rate",
		Type:        MetricTypeGauge,
		Level:       MetricLevelMedium,
		Value:       hitValue,
		Unit:        "ratio",
		Description: "Cache hit rate",
		Labels: map[string]string{
			"cache_type": cacheType,
		},
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"total_hits":   c.cacheMetrics.TotalHits,
			"total_misses": c.cacheMetrics.TotalMisses,
			"hit_rate":     c.cacheMetrics.HitRate,
		},
	})
}

// GetLatencyPercentiles calculates latency percentiles
func (c *EnhancedMetricsCollector) GetLatencyPercentiles() PercentileMetric {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.requestLatencies) == 0 {
		return PercentileMetric{}
	}

	// Sort latencies for percentile calculation
	latencies := make([]float64, len(c.requestLatencies))
	copy(latencies, c.requestLatencies)
	
	// Simple sort for percentile calculation
	for i := 0; i < len(latencies); i++ {
		for j := i + 1; j < len(latencies); j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	return PercentileMetric{
		P50: c.getPercentile(latencies, 0.5),
		P75: c.getPercentile(latencies, 0.75),
		P90: c.getPercentile(latencies, 0.9),
		P95: c.getPercentile(latencies, 0.95),
		P99: c.getPercentile(latencies, 0.99),
	}
}

// GetErrorRates calculates error rates
func (c *EnhancedMetricsCollector) GetErrorRates() ErrorRateMetric {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := int64(0)
	errors := int64(0)
	by4xx := c.errorCounts["4xx"]
	by5xx := c.errorCounts["5xx"]

	for _, count := range c.errorCounts {
		total += count
	}

	errors = by4xx + by5xx

	errorRate := 0.0
	if total > 0 {
		errorRate = float64(errors) / float64(total)
	}

	return ErrorRateMetric{
		Total:     total,
		Errors:    errors,
		ErrorRate: errorRate,
		By4xx:     by4xx,
		By5xx:     by5xx,
		ByType:    c.copyErrorCounts(),
	}
}

// GetCurrentMetrics returns all current metrics
func (c *EnhancedMetricsCollector) GetCurrentMetrics() map[string]*EnhancedMetric {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*EnhancedMetric)
	for k, v := range c.metrics {
		// Create a copy to prevent concurrent access issues
		metricCopy := *v
		result[k] = &metricCopy
	}

	// Add computed metrics
	c.addComputedMetrics(result)

	return result
}

// GetMetricsJSON returns metrics as JSON
func (c *EnhancedMetricsCollector) GetMetricsJSON() ([]byte, error) {
	metrics := c.GetCurrentMetrics()
	return json.Marshal(metrics)
}

// ShouldFlush returns true if metrics should be flushed to storage
func (c *EnhancedMetricsCollector) ShouldFlush() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return time.Since(c.lastFlush) >= c.flushInterval
}

// MarkFlushed marks metrics as flushed
func (c *EnhancedMetricsCollector) MarkFlushed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.lastFlush = time.Now()
}

// Reset clears all metrics (call after flushing to persistent storage)
func (c *EnhancedMetricsCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Keep critical metrics, clear others
	newMetrics := make(map[string]*EnhancedMetric)
	for k, v := range c.metrics {
		if v.Level == MetricLevelCritical {
			newMetrics[k] = v
		}
	}
	
	c.metrics = newMetrics
	c.requestLatencies = c.requestLatencies[:0]
}

// Helper methods

// recordMetric records a metric internally
func (c *EnhancedMetricsCollector) recordMetric(metric *EnhancedMetric) {
	key := c.generateMetricKey(metric)
	c.metrics[key] = metric
}

// generateMetricKey generates a unique key for a metric
func (c *EnhancedMetricsCollector) generateMetricKey(metric *EnhancedMetric) string {
	key := metric.Name
	for k, v := range metric.Labels {
		key += fmt.Sprintf("_%s_%s", k, v)
	}
	return key
}

// getPercentile calculates the specified percentile from sorted data
func (c *EnhancedMetricsCollector) getPercentile(sortedData []float64, percentile float64) float64 {
	if len(sortedData) == 0 {
		return 0
	}

	index := percentile * float64(len(sortedData)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sortedData[lower]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return sortedData[lower]*(1-weight) + sortedData[upper]*weight
}

// copyErrorCounts creates a copy of error counts
func (c *EnhancedMetricsCollector) copyErrorCounts() map[string]int64 {
	result := make(map[string]int64)
	for k, v := range c.errorCounts {
		result[k] = v
	}
	return result
}

// addComputedMetrics adds computed metrics to the result
func (c *EnhancedMetricsCollector) addComputedMetrics(result map[string]*EnhancedMetric) {
	// Add latency percentiles
	percentiles := c.GetLatencyPercentiles()
	result["latency_p95"] = &EnhancedMetric{
		Name:        "latency_p95",
		Type:        MetricTypeGauge,
		Level:       MetricLevelCritical,
		Value:       percentiles.P95,
		Unit:        "ms",
		Description: "95th percentile latency",
		Timestamp:   time.Now(),
	}

	// Add error rates
	errorRates := c.GetErrorRates()
	result["error_rate"] = &EnhancedMetric{
		Name:        "error_rate",
		Type:        MetricTypeGauge,
		Level:       MetricLevelCritical,
		Value:       errorRates.ErrorRate,
		Unit:        "ratio",
		Description: "Overall error rate",
		Timestamp:   time.Now(),
		Metadata: map[string]interface{}{
			"total_requests": errorRates.Total,
			"error_count":    errorRates.Errors,
			"4xx_errors":     errorRates.By4xx,
			"5xx_errors":     errorRates.By5xx,
		},
	}

	// Add cache hit rate
	result["cache_hit_rate_overall"] = &EnhancedMetric{
		Name:        "cache_hit_rate_overall",
		Type:        MetricTypeGauge,
		Level:       MetricLevelMedium,
		Value:       c.cacheMetrics.HitRate,
		Unit:        "ratio",
		Description: "Overall cache hit rate",
		Timestamp:   time.Now(),
	}

	// Add uptime
	uptime := time.Since(c.startTime).Seconds()
	result["uptime"] = &EnhancedMetric{
		Name:        "uptime",
		Type:        MetricTypeGauge,
		Level:       MetricLevelLow,
		Value:       uptime,
		Unit:        "seconds",
		Description: "Application uptime",
		Timestamp:   time.Now(),
	}
}

// MetricsMiddleware creates a middleware that records request metrics
func MetricsMiddleware(collector *EnhancedMetricsCollector) func(next func()) func() {
	return func(next func()) func() {
		return func() {
			start := time.Now()
			
			// Call next handler
			next()
			
			// Record latency
			latency := time.Since(start)
			collector.RecordLatency("http_request", latency)
		}
	}
}

// Global metrics collector instance
var globalMetricsCollector *EnhancedMetricsCollector
var once sync.Once

// GetGlobalMetricsCollector returns the global metrics collector
func GetGlobalMetricsCollector(logger *zap.Logger) *EnhancedMetricsCollector {
	once.Do(func() {
		globalMetricsCollector = NewEnhancedMetricsCollector(logger)
	})
	return globalMetricsCollector
}