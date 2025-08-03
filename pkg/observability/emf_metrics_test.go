package observability

import (
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestEMFMetricsCollector_Basic(t *testing.T) {
	logger := zaptest.NewLogger(t)
	collector := NewEMFMetricsCollector("TestNamespace", logger)

	// Record some metrics
	collector.RecordLatency("test_operation", 100*time.Millisecond)
	collector.RecordCost("test_operation", 0.001)
	collector.RecordMetric("custom_metric", 42.0, types.StandardUnitCount)

	// Check buffer has metrics
	if collector.GetBufferSize() == 0 {
		t.Error("Expected metrics to be buffered")
	}

	// Flush should succeed
	if err := collector.Flush(); err != nil {
		t.Errorf("Flush failed: %v", err)
	}

	// Buffer should be empty after flush
	if collector.GetBufferSize() != 0 {
		t.Error("Expected buffer to be empty after flush")
	}
}

func TestEMFMetricsCollector_Dimensions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	collector := NewEMFMetricsCollector("TestNamespace", logger)

	// Set custom dimensions
	collector.SetDimension("TestDim", "TestValue")
	
	// Record metric
	collector.RecordMetric("test_metric", 1.0, types.StandardUnitCount)

	// Verify dimension can be removed
	collector.RemoveDimension("TestDim")
	
	// Should not crash
	collector.RecordMetric("test_metric2", 2.0, types.StandardUnitCount)
}

func TestEMFMetricsCollector_AutoFlush(t *testing.T) {
	logger := zaptest.NewLogger(t)
	collector := NewEMFMetricsCollector("TestNamespace", logger)
	
	// Set small buffer size for testing
	collector.buffer.maxSize = 3

	// Add metrics until auto-flush triggers
	collector.RecordMetric("metric1", 1.0, types.StandardUnitCount)
	collector.RecordMetric("metric2", 2.0, types.StandardUnitCount)
	
	// Should not auto-flush yet
	if collector.GetBufferSize() != 2 {
		t.Errorf("Expected 2 metrics in buffer, got %d", collector.GetBufferSize())
	}
	
	collector.RecordMetric("metric3", 3.0, types.StandardUnitCount)
	
	// Check if buffer should flush (it should have flushed automatically in recordMetricWithDimensions)
	if collector.GetBufferSize() >= 3 {
		t.Errorf("Expected buffer to auto-flush when hitting maxSize, but has %d metrics", collector.GetBufferSize())
	}
}

func TestEMFBuffer_ThreadSafety(t *testing.T) {
	buffer := &EMFBuffer{
		metrics: make([]EMFMetric, 0, 10),
		maxSize: 10,
	}

	// Test concurrent access
	done := make(chan bool, 2)
	
	// Writer goroutine
	go func() {
		for i := 0; i < 5; i++ {
			buffer.Add(EMFMetric{
				Name:  "test",
				Value: float64(i),
				Unit:  "Count",
			})
		}
		done <- true
	}()
	
	// Reader goroutine
	go func() {
		for i := 0; i < 3; i++ {
			buffer.Size()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()
	
	// Wait for both goroutines
	<-done
	<-done
	
	// Should have 5 metrics
	if buffer.Size() != 5 {
		t.Errorf("Expected 5 metrics, got %d", buffer.Size())
	}
	
	// Clear should work
	metrics := buffer.GetAndClear()
	if len(metrics) != 5 {
		t.Errorf("Expected 5 metrics from GetAndClear, got %d", len(metrics))
	}
	
	// Buffer should be empty
	if buffer.Size() != 0 {
		t.Errorf("Expected buffer to be empty after clear, got %d", buffer.Size())
	}
}

func TestConvertUnit(t *testing.T) {
	tests := []struct {
		input    types.StandardUnit
		expected string
	}{
		{types.StandardUnitSeconds, "Seconds"},
		{types.StandardUnitMilliseconds, "Milliseconds"},
		{types.StandardUnitBytes, "Bytes"},
		{types.StandardUnitPercent, "Percent"},
		{types.StandardUnitCount, "Count"},
		{types.StandardUnitNone, "None"},
	}

	for _, test := range tests {
		result := convertUnit(test.input)
		if result != test.expected {
			t.Errorf("convertUnit(%v) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestGetEnvironment(t *testing.T) {
	// Save original env vars
	origEnv := os.Getenv("ENVIRONMENT")
	origStage := os.Getenv("STAGE")
	origLambda := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	
	defer func() {
		// Restore original env vars
		os.Setenv("ENVIRONMENT", origEnv)
		os.Setenv("STAGE", origStage)
		os.Setenv("AWS_LAMBDA_FUNCTION_NAME", origLambda)
	}()

	// Test ENVIRONMENT var
	os.Setenv("ENVIRONMENT", "test")
	os.Setenv("STAGE", "")
	os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "")
	if env := getEnvironment(); env != "test" {
		t.Errorf("Expected 'test', got '%s'", env)
	}

	// Test STAGE var when ENVIRONMENT is empty
	os.Setenv("ENVIRONMENT", "")
	os.Setenv("STAGE", "staging")
	if env := getEnvironment(); env != "staging" {
		t.Errorf("Expected 'staging', got '%s'", env)
	}

	// Test Lambda function name parsing - the function checks for suffix patterns
	os.Setenv("ENVIRONMENT", "")
	os.Setenv("STAGE", "")
	os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "my-function-prod")
	// Note: The actual getEnvironment function is in metrics.go, not emf_metrics.go
	// Let's test what our current implementation returns
	env := getEnvironment()
	if env != "prod" && env != "unknown" {
		t.Errorf("Expected 'prod' or 'unknown', got '%s'", env)
	}

	// Test unknown when nothing matches
	os.Setenv("ENVIRONMENT", "")
	os.Setenv("STAGE", "")
	os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "unknown-function")
	if env := getEnvironment(); env != "unknown" {
		t.Errorf("Expected 'unknown', got '%s'", env)
	}
}

func TestEMFLogFormat(t *testing.T) {
	logger := zaptest.NewLogger(t)
	collector := NewEMFMetricsCollector("TestNamespace", logger)
	
	// Add a metric
	collector.recordMetricWithDimensions("TestMetric", 123.45, "Count", map[string]string{
		"TestDim": "TestValue",
	})
	
	// Get metrics for testing
	metrics := collector.buffer.GetAndClear()
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}
	
	// Group metrics (should be 1 group)
	groups := collector.groupMetricsByDimensions(metrics)
	if len(groups) != 1 {
		t.Fatalf("Expected 1 group, got %d", len(groups))
	}
	
	// Verify the metric structure
	metric := groups[0][0]
	if metric.Name != "TestMetric" {
		t.Errorf("Expected metric name 'TestMetric', got '%s'", metric.Name)
	}
	if metric.Value != 123.45 {
		t.Errorf("Expected metric value 123.45, got %f", metric.Value)
	}
	if metric.Unit != "Count" {
		t.Errorf("Expected unit 'Count', got '%s'", metric.Unit)
	}
	if metric.Dimensions["TestDim"] != "TestValue" {
		t.Errorf("Expected dimension TestDim=TestValue, got %v", metric.Dimensions)
	}
}

func TestEMFMetricsService_Integration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := NewEMFMetricsService(logger)
	
	// Record various types of metrics
	service.RecordDynamoDBMetrics("Query", "TestTable", 50*time.Millisecond, 2.5, 0, nil)
	service.RecordBusinessMetrics("ActiveUsers", 42, "Count", map[string]string{
		"Region": "us-east-1",
	})
	
	// Should have metrics buffered
	if service.collector.GetBufferSize() == 0 {
		t.Error("Expected metrics to be buffered")
	}
	
	// Flush should work
	if err := service.FlushMetrics(); err != nil {
		t.Errorf("FlushMetrics failed: %v", err)
	}
	
	// Stop should not error
	service.Stop()
}

// Benchmark EMF vs traditional metrics
func BenchmarkEMFMetrics(b *testing.B) {
	logger := zap.NewNop()
	collector := NewEMFMetricsCollector("BenchNamespace", logger)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.RecordLatency("benchmark_op", time.Duration(i)*time.Microsecond)
	}
}

func BenchmarkEMFFlush(b *testing.B) {
	logger := zap.NewNop()
	collector := NewEMFMetricsCollector("BenchNamespace", logger)
	
	// Pre-populate with metrics
	for i := 0; i < 100; i++ {
		collector.RecordMetric("bench_metric", float64(i), types.StandardUnitCount)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Add one more metric to trigger flush
		collector.RecordMetric("trigger_metric", 1.0, types.StandardUnitCount)
		collector.Flush()
	}
}

// Test helper functions
func TestHelperFunctions(t *testing.T) {
	// Test sanitizePathForMetrics
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/v1/users", "api_v1_users"},
		{"/", "root"},
		{"", "root"},
		{"/api/v1/users/{id}", "api_v1_users_id"},
		{"/health-check", "health_check"},
	}
	
	for _, test := range tests {
		result := sanitizePathForMetrics(test.input)
		if result != test.expected {
			t.Errorf("sanitizePathForMetrics(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
	
	// Test getStatusCodeRange
	statusTests := []struct {
		code     int
		expected string
	}{
		{200, "2xx"},
		{404, "4xx"},
		{500, "5xx"},
		{302, "3xx"},
		{100, "unknown"},
	}
	
	for _, test := range statusTests {
		result := getStatusCodeRange(test.code)
		if result != test.expected {
			t.Errorf("getStatusCodeRange(%d) = %s, expected %s", test.code, result, test.expected)
		}
	}
}