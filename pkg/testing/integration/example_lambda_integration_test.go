//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/stretchr/testify/assert"
	"github.com/equaltoai/lesser/pkg/testing/integration"
)

// Example Lambda handler for testing
func exampleLambdaHandler(_ context.Context, _ interface{}) (interface{}, error) {
	// Simulate a simple Lambda that returns success
	return map[string]interface{}{
		"statusCode": 200,
		"body":       `{"message": "Hello from Lambda"}`,
		"headers": map[string]string{
			"Content-Type": "application/json",
		},
	}, nil
}

// ExampleLambdaIntegrationTestSuite demonstrates how to use the comprehensive Lambda integration testing
func ExampleLambdaIntegrationTestSuite() {
	// This example shows how to use the comprehensive Lambda integration test suite
	// In actual tests, this would be in a *testing.T function

	// Create a Lambda handler from the function
	handler := lambda.NewHandler(exampleLambdaHandler)

	// Create the comprehensive test suite with options
	suite := integration.NewLambdaIntegrationTestSuite(nil, handler) // nil for *testing.T in example

	// Define comprehensive test cases
	testCases := []integration.LambdaIntegrationTestCase{
		{
			Name:        "Basic_API_Request",
			Description: "Test basic API Gateway request processing",
			Event: integration.BuildAPIGatewayEvent(
				"GET", 
				"/health", 
				nil, 
				map[string]string{"Content-Type": "application/json"},
			),
			Timeout: 10 * time.Second,
			PerformanceThresholds: &integration.PerformanceThresholds{
				MaxColdStart:   5 * time.Second,
				MaxWarmStart:   2 * time.Second,
				MaxMemoryMB:    256,
				MinSuccessRate: 99.0,
				MaxErrorRate:   1.0,
			},
		},
		{
			Name:        "Authenticated_Request",
			Description: "Test authenticated API request",
			RequiredAuth: true,
			RequiredScopes: []string{"read", "write"},
			DataRequirements: &integration.TestDataRequirements{
				Users:  1,
				Actors: 1,
			},
			Event: integration.BuildAPIGatewayEvent(
				"GET", 
				"/api/v1/accounts/verify_credentials", 
				nil, 
				nil,
			),
			Timeout: 15 * time.Second,
		},
		{
			Name:        "SQS_Background_Job",
			Description: "Test SQS message processing",
			Event: integration.BuildSQSEvent(
				`{"task": "process_notifications", "user_id": "test_user"}`,
			),
			DataRequirements: &integration.TestDataRequirements{
				Users: 1,
			},
			Timeout: 30 * time.Second,
		},
	}

	// Run all integration tests
	suite.RunIntegrationTests(testCases)

	// Output:
	// Comprehensive test metrics and performance analysis will be logged
	// Test quality assessment: EXCELLENT/GOOD/ACCEPTABLE/NEEDS_IMPROVEMENT/POOR
	// Detailed performance breakdown with percentiles, throughput, and concurrency analysis
}

// TestAPILambdaIntegration demonstrates API-specific Lambda integration testing
func TestAPILambdaIntegration(t *testing.T) {
	// Skip if integration tests disabled
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create handler and test suite
	handler := lambda.NewHandler(exampleLambdaHandler)
	suite := integration.NewLambdaIntegrationTestSuite(t, handler)
	defer suite.AddCleanup(func() error {
		return nil // Custom cleanup if needed
	})

	// Use pre-built API test scenarios
	apiTestCases := suite.APILambdaTestScenarios()
	
	// Run API-specific tests
	suite.RunIntegrationTests(apiTestCases)
}

// TestConcurrentLambdaExecution demonstrates concurrent Lambda execution testing
func TestConcurrentLambdaExecution(t *testing.T) {
	handler := lambda.NewHandler(exampleLambdaHandler)
	suite := integration.NewLambdaIntegrationTestSuite(t, handler)

	// Define concurrent execution test
	concurrencyTest := integration.LambdaConcurrencyTest{
		Name:               "High_Concurrency_API_Load",
		ConcurrentRequests: 50,
		RequestBuilder: func(index int) interface{} {
			return integration.BuildAPIGatewayEvent(
				"GET",
				"/health",
				nil,
				map[string]string{"X-Request-ID": fmt.Sprintf("req-%d", index)},
			)
		},
		ValidateResponse: func(t *testing.T, response interface{}, err error) {
			assert.NoError(t, err)
			assert.NotNil(t, response)
		},
		MaxDuration: 30 * time.Second,
	}

	// Execute concurrency test
	suite.RunConcurrencyTest(concurrencyTest)
}

// TestPerformanceBenchmarking demonstrates performance benchmarking
func TestPerformanceBenchmarking(t *testing.T) {
	handler := lambda.NewHandler(exampleLambdaHandler)
	suite := integration.NewLambdaIntegrationTestSuite(t, handler)

	// Define performance benchmark test
	performanceTest := integration.LambdaIntegrationTestCase{
		Name:        "Performance_Benchmark",
		Description: "Comprehensive performance benchmarking of Lambda function",
		Event: integration.BuildAPIGatewayEvent("GET", "/api/v1/statuses", nil, nil),
		DataRequirements: &integration.TestDataRequirements{
			Users:    10,
			Actors:   10,
			Statuses: 100,
		},
		PerformanceThresholds: &integration.PerformanceThresholds{
			MaxColdStart:   3 * time.Second,
			MaxWarmStart:   1 * time.Second,
			MaxMemoryMB:    512,
			MinSuccessRate: 99.5,
			MaxErrorRate:   0.5,
		},
		RequiredAuth: true,
		Timeout:      60 * time.Second,
	}

	// Execute multiple times for statistical significance
	performanceTests := make([]integration.LambdaIntegrationTestCase, 10)
	for i := range performanceTests {
		performanceTests[i] = performanceTest
		performanceTests[i].Name = fmt.Sprintf("Performance_Benchmark_%d", i+1)
	}

	suite.RunIntegrationTests(performanceTests)
}