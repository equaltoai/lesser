package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LambdaTestCase defines an end-to-end Lambda test case
type LambdaTestCase struct {
	Name          string
	Event         interface{}
	SetupFunc     func() error
	CleanupFunc   func() error
	ValidateFunc  func(*testing.T, interface{}, error)
	ExpectedError bool
	Timeout       time.Duration
}

// LambdaTestSuite provides utilities for end-to-end Lambda testing
type LambdaTestSuite struct {
	t         *testing.T
	handler   lambda.Handler
	metrics   *TestMetrics
	startTime time.Time
}

// TestMetrics tracks Lambda execution metrics
type TestMetrics struct {
	Invocations      int
	ColdStarts       int
	TotalDuration    time.Duration
	AverageDuration  time.Duration
	MaxDuration      time.Duration
	MinDuration      time.Duration
	Errors           int
	Timeouts         int
	MemoryUsed       []int
	ConcurrentExecs  int
}

// NewLambdaTestSuite creates a new Lambda test suite
func NewLambdaTestSuite(t *testing.T, handler lambda.Handler) *LambdaTestSuite {
	return &LambdaTestSuite{
		t:         t,
		handler:   handler,
		metrics:   &TestMetrics{MinDuration: time.Hour}, // Initialize with large value
		startTime: time.Now(),
	}
}

// RunTest executes a single Lambda test case
func (s *LambdaTestSuite) RunTest(tc LambdaTestCase) {
	s.t.Run(tc.Name, func(t *testing.T) {
		// Setup
		if tc.SetupFunc != nil {
			require.NoError(t, tc.SetupFunc(), "Setup failed")
		}

		// Cleanup
		if tc.CleanupFunc != nil {
			defer func() {
				if err := tc.CleanupFunc(); err != nil {
					t.Logf("Cleanup error: %v", err)
				}
			}()
		}

		// Set timeout
		timeout := tc.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// Execute Lambda
		start := time.Now()
		result, err := s.invokeLambda(ctx, tc.Event)
		duration := time.Since(start)

		// Update metrics
		s.updateMetrics(duration, err)

		// Validate
		if tc.ValidateFunc != nil {
			tc.ValidateFunc(t, result, err)
		} else {
			if tc.ExpectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		}
	})
}

// RunTests executes multiple Lambda test cases
func (s *LambdaTestSuite) RunTests(testCases []LambdaTestCase) {
	for _, tc := range testCases {
		s.RunTest(tc)
	}

	// Print metrics summary
	s.PrintMetrics()
}

// invokeLambda invokes the Lambda handler
func (s *LambdaTestSuite) invokeLambda(ctx context.Context, event interface{}) (interface{}, error) {
	// Marshal event to JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	// Invoke handler
	result, err := s.handler.Invoke(ctx, eventJSON)
	if err != nil {
		return nil, err
	}

	// Unmarshal result
	var response interface{}
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

// updateMetrics updates test metrics
func (s *LambdaTestSuite) updateMetrics(duration time.Duration, err error) {
	s.metrics.Invocations++
	s.metrics.TotalDuration += duration

	if duration > s.metrics.MaxDuration {
		s.metrics.MaxDuration = duration
	}
	if duration < s.metrics.MinDuration {
		s.metrics.MinDuration = duration
	}

	if err != nil {
		s.metrics.Errors++
		if ctx, ok := err.(interface{ Timeout() bool }); ok && ctx.Timeout() {
			s.metrics.Timeouts++
		}
	}

	// Check for cold start (simplified - first invocation or long gap)
	if s.metrics.Invocations == 1 || time.Since(s.startTime) > 5*time.Minute {
		s.metrics.ColdStarts++
	}
}

// PrintMetrics prints test metrics summary
func (s *LambdaTestSuite) PrintMetrics() {
	if s.metrics.Invocations == 0 {
		return
	}

	s.metrics.AverageDuration = s.metrics.TotalDuration / time.Duration(s.metrics.Invocations)

	s.t.Logf("\n=== Lambda Test Metrics ===")
	s.t.Logf("Total Invocations: %d", s.metrics.Invocations)
	s.t.Logf("Cold Starts: %d", s.metrics.ColdStarts)
	s.t.Logf("Errors: %d", s.metrics.Errors)
	s.t.Logf("Timeouts: %d", s.metrics.Timeouts)
	s.t.Logf("Average Duration: %v", s.metrics.AverageDuration)
	s.t.Logf("Min Duration: %v", s.metrics.MinDuration)
	s.t.Logf("Max Duration: %v", s.metrics.MaxDuration)
	s.t.Logf("Success Rate: %.2f%%", float64(s.metrics.Invocations-s.metrics.Errors)/float64(s.metrics.Invocations)*100)
}

// Common Lambda Event Builders

// BuildAPIGatewayEvent creates an API Gateway event for testing
func BuildAPIGatewayEvent(method, path string, body interface{}, headers map[string]string) events.APIGatewayV2HTTPRequest {
	var bodyStr string
	switch v := body.(type) {
	case string:
		bodyStr = v
	case []byte:
		bodyStr = string(v)
	default:
		data, _ := json.Marshal(body)
		bodyStr = string(data)
	}

	if headers == nil {
		headers = make(map[string]string)
	}

	return events.APIGatewayV2HTTPRequest{
		RouteKey: fmt.Sprintf("%s %s", method, path),
		RawPath:  path,
		Headers:  headers,
		Body:     bodyStr,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: method,
				Path:   path,
			},
			RequestID: fmt.Sprintf("test-%d", time.Now().UnixNano()),
		},
	}
}

// BuildSQSEvent creates an SQS event for testing
func BuildSQSEvent(messages ...string) events.SQSEvent {
	records := make([]events.SQSMessage, len(messages))
	for i, msg := range messages {
		records[i] = events.SQSMessage{
			MessageId:     fmt.Sprintf("msg-%d", i),
			ReceiptHandle: fmt.Sprintf("receipt-%d", i),
			Body:          msg,
			EventSource:   "aws:sqs",
			EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:test-queue",
		}
	}

	return events.SQSEvent{Records: records}
}

// BuildDynamoDBStreamEvent creates a DynamoDB stream event for testing
func BuildDynamoDBStreamEvent(eventName string, newImage, oldImage map[string]interface{}) events.DynamoDBEvent {
	record := events.DynamoDBEventRecord{
		EventName: eventName,
		Change: events.DynamoDBStreamRecord{
			NewImage: buildAttributeMap(newImage),
			OldImage: buildAttributeMap(oldImage),
		},
		EventSource: "aws:dynamodb",
	}

	return events.DynamoDBEvent{Records: []events.DynamoDBEventRecord{record}}
}

// buildAttributeMap converts a map to DynamoDB attribute values
func buildAttributeMap(m map[string]interface{}) map[string]events.DynamoDBAttributeValue {
	if m == nil {
		return nil
	}

	result := make(map[string]events.DynamoDBAttributeValue)
	for k, v := range m {
		switch val := v.(type) {
		case string:
			result[k] = events.NewStringAttribute(val)
		case float64:
			result[k] = events.NewNumberAttribute(fmt.Sprintf("%f", val))
		case int:
			result[k] = events.NewNumberAttribute(fmt.Sprintf("%d", val))
		case bool:
			result[k] = events.NewBooleanAttribute(val)
		case []string:
			result[k] = events.NewStringSetAttribute(val)
		case map[string]interface{}:
			result[k] = events.NewMapAttribute(buildAttributeMap(val))
		}
	}
	return result
}

// BuildS3Event creates an S3 event for testing
func BuildS3Event(bucket, key, eventName string) events.S3Event {
	return events.S3Event{
		Records: []events.S3EventRecord{
			{
				EventName: eventName,
				S3: events.S3Entity{
					Bucket: events.S3Bucket{
						Name: bucket,
					},
					Object: events.S3Object{
						Key:  key,
						Size: 1024,
					},
				},
			},
		},
	}
}

// BuildEventBridgeEvent creates an EventBridge event for testing
func BuildEventBridgeEvent(source, detailType string, detail interface{}) events.CloudWatchEvent {
	detailJSON, _ := json.Marshal(detail)

	return events.CloudWatchEvent{
		Source:     source,
		DetailType: detailType,
		Detail:     json.RawMessage(detailJSON),
		Time:       time.Now(),
		ID:         fmt.Sprintf("event-%d", time.Now().UnixNano()),
	}
}

// Test Helpers

// AssertLambdaResponse validates Lambda response structure
func AssertLambdaResponse(t *testing.T, response interface{}, expectedStatus int, expectedBody interface{}) {
	t.Helper()

	// For API Gateway responses
	if apiResp, ok := response.(map[string]interface{}); ok {
		// Check status code
		if statusCode, exists := apiResp["statusCode"]; exists {
			assert.Equal(t, float64(expectedStatus), statusCode)
		}

		// Check body
		if body, exists := apiResp["body"]; exists && expectedBody != nil {
			var actualBody interface{}
			if err := json.Unmarshal([]byte(body.(string)), &actualBody); err == nil {
				assert.Equal(t, expectedBody, actualBody)
			}
		}
	}
}

// AssertColdStartTime verifies cold start performance
func AssertColdStartTime(t *testing.T, duration time.Duration, maxAllowed time.Duration) {
	t.Helper()
	assert.LessOrEqual(t, duration.Milliseconds(), maxAllowed.Milliseconds(),
		"Cold start time %v exceeds maximum %v", duration, maxAllowed)
}

// AssertWarmStartTime verifies warm start performance
func AssertWarmStartTime(t *testing.T, duration time.Duration, maxAllowed time.Duration) {
	t.Helper()
	assert.LessOrEqual(t, duration.Milliseconds(), maxAllowed.Milliseconds(),
		"Warm start time %v exceeds maximum %v", duration, maxAllowed)
}

// LambdaConcurrencyTest tests Lambda under concurrent load
type LambdaConcurrencyTest struct {
	Name              string
	ConcurrentRequests int
	RequestBuilder    func(int) interface{}
	ValidateResponse  func(*testing.T, interface{}, error)
	MaxDuration       time.Duration
}

// RunConcurrencyTest executes Lambda concurrency test
func RunConcurrencyTest(t *testing.T, handler lambda.Handler, test LambdaConcurrencyTest) {
	results := make(chan struct {
		response interface{}
		err      error
		duration time.Duration
	}, test.ConcurrentRequests)

	// Launch concurrent requests
	start := time.Now()
	for i := 0; i < test.ConcurrentRequests; i++ {
		go func(index int) {
			event := test.RequestBuilder(index)
			eventJSON, _ := json.Marshal(event)

			reqStart := time.Now()
			result, err := handler.Invoke(context.Background(), eventJSON)
			duration := time.Since(reqStart)

			var response interface{}
			if err == nil {
				json.Unmarshal(result, &response)
			}

			results <- struct {
				response interface{}
				err      error
				duration time.Duration
			}{response, err, duration}
		}(i)
	}

	// Collect results
	var successCount int
	var totalDuration time.Duration
	var maxDuration time.Duration

	for i := 0; i < test.ConcurrentRequests; i++ {
		result := <-results
		if result.err == nil {
			successCount++
		}
		totalDuration += result.duration
		if result.duration > maxDuration {
			maxDuration = result.duration
		}

		// Validate individual response
		if test.ValidateResponse != nil {
			test.ValidateResponse(t, result.response, result.err)
		}
	}

	totalTime := time.Since(start)

	// Assert performance
	assert.GreaterOrEqual(t, successCount, test.ConcurrentRequests*90/100, 
		"Less than 90%% success rate")
	assert.LessOrEqual(t, totalTime, test.MaxDuration,
		"Total execution time exceeds limit")

	// Log metrics
	t.Logf("Concurrency Test Results:")
	t.Logf("  Total Requests: %d", test.ConcurrentRequests)
	t.Logf("  Successful: %d (%.2f%%)", successCount, float64(successCount)/float64(test.ConcurrentRequests)*100)
	t.Logf("  Average Duration: %v", totalDuration/time.Duration(test.ConcurrentRequests))
	t.Logf("  Max Duration: %v", maxDuration)
	t.Logf("  Total Time: %v", totalTime)
}

// MemoryProfiler tracks Lambda memory usage
type MemoryProfiler struct {
	samples []int
}

// NewMemoryProfiler creates a memory profiler
func NewMemoryProfiler() *MemoryProfiler {
	return &MemoryProfiler{
		samples: make([]int, 0),
	}
}

// Sample records current memory usage
func (m *MemoryProfiler) Sample() {
	// In real implementation, would read from /proc/meminfo or runtime.MemStats
	// For testing, we simulate
	m.samples = append(m.samples, 128) // MB
}

// GetPeakMemory returns peak memory usage
func (m *MemoryProfiler) GetPeakMemory() int {
	peak := 0
	for _, sample := range m.samples {
		if sample > peak {
			peak = sample
		}
	}
	return peak
}