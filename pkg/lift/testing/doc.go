// Package testing provides comprehensive testing utilities for Lift applications.
//
// This package implements the standard testing patterns from the Lift testing guide,
// providing tools for unit testing, integration testing, mocking, and performance
// validation of Lift-based Lambda functions.
//
// # Core Components
//
// TestApp: A test harness that simulates HTTP requests to Lift applications
// without requiring Lambda or API Gateway. Supports all HTTP methods, headers,
// and request/response validation.
//
// Mocking Framework: Comprehensive mocks for auth services, storage layers,
// and external dependencies following testify/mock patterns.
//
// Integration Testing: Tools for testing against real services with proper
// setup, teardown, and data management.
//
// Performance Testing: Utilities for measuring cold start times, throughput,
// memory usage, and other performance metrics.
//
// # Basic Usage
//
//	func TestMyHandler(t *testing.T) {
//	    // Create test app
//	    app := lifttesting.NewTestApp()
//	    
//	    // Setup routes
//	    app.App().GET("/health", func(ctx *lift.Context) error {
//	        return ctx.JSON(map[string]string{"status": "healthy"})
//	    })
//	    
//	    // Test the endpoint
//	    response := app.GET("/health")
//	    
//	    assert.Equal(t, 200, response.StatusCode)
//	    assert.Contains(t, response.Body, "healthy")
//	}
//
// # Testing with Mocks
//
//	func TestWithMocks(t *testing.T) {
//	    // Setup mocks
//	    mockAuth := SetupMockAuthService()
//	    mockStorage := SetupMockStorage()
//	    
//	    // Configure expectations
//	    claims := CreateTestClaims("user", "standard")
//	    ExpectValidToken(mockAuth, "token", claims)
//	    
//	    // Test with mocked dependencies
//	    app := NewTestApp()
//	    // ... setup routes with mocks
//	    
//	    response := app.WithHeader("Authorization", "Bearer token").GET("/protected")
//	    assert.True(t, response.IsSuccess())
//	    
//	    // Verify mock expectations
//	    mockAuth.AssertExpectations(t)
//	    mockStorage.AssertExpectations(t)
//	}
//
// # Integration Testing
//
//	func TestIntegration(t *testing.T) {
//	    RunIntegrationTest(t, nil, func(suite *IntegrationTestSuite) {
//	        // Create test data
//	        ctx := context.Background()
//	        actor, err := suite.TestData.CreateTestActor(ctx, suite.Storage, "testuser")
//	        require.NoError(t, err)
//	        
//	        // Test against real storage
//	        app := suite.CreateTestApp()
//	        response := app.GET("/users/testuser")
//	        assert.True(t, response.IsSuccess())
//	    })
//	}
//
// # Performance Testing
//
//	func TestPerformance(t *testing.T) {
//	    suite := NewPerformanceTestSuite(t)
//	    
//	    test := &PerformanceTest{
//	        Name:        "EndpointPerformance",
//	        Iterations:  100,
//	        Concurrency: 10,
//	        TestFunc:    func() error { /* test logic */ },
//	    }
//	    
//	    results := suite.RunPerformanceTest(test)
//	    
//	    requirements := &PerformanceRequirements{
//	        MaxAvgTime:    100 * time.Millisecond,
//	        MinThroughput: 10.0,
//	    }
//	    
//	    AssertPerformance(t, results, requirements)
//	}
//
// # Authentication Testing
//
//	func TestAuth(t *testing.T) {
//	    app := NewTestApp()
//	    
//	    // Test unauthenticated access
//	    response := app.GET("/protected")
//	    AssertErrorResponse(t, response, 401, "unauthorized")
//	    
//	    // Test authenticated access
//	    token := GenerateTestToken("user", "standard")
//	    response = app.WithHeader("Authorization", "Bearer "+token).GET("/protected")
//	    AssertSuccessResponse(t, response)
//	}
//
// # Best Practices
//
// 1. Always use NewTestApp() for isolated test environments
// 2. Mock external dependencies using the provided mock framework
// 3. Test both success and error scenarios comprehensively
// 4. Verify mock expectations to ensure proper interaction
// 5. Use integration tests for critical user workflows
// 6. Validate performance requirements for Lambda functions
// 7. Test authentication and authorization flows thoroughly
//
// # Testing Patterns
//
// This package follows the standard testing patterns from the Lift testing guide:
//
// - Test Structure: Organized with Given/When/Then patterns
// - Mock Setup: Comprehensive mocking of external dependencies
// - Error Testing: Validation of error conditions and edge cases
// - Integration Testing: End-to-end testing with real services
// - Performance Testing: Cold start and throughput validation
//
// For detailed examples and patterns, see examples_test.go and the individual
// test files for each component.
package testing