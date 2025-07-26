package testing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Example tests demonstrating the testing framework usage

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func TestLiftTestingFramework_BasicUsage(t *testing.T) {
	// Create a test app
	app := NewTestApp()

	// Setup a simple endpoint
	app.App().GET("/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "healthy"})
	})

	// Test the endpoint
	response := app.GET("/health")

	assert.Equal(t, 200, response.StatusCode)
	assert.True(t, response.IsSuccess())
	assert.Contains(t, response.Body, "healthy")
}

func TestLiftTestingFramework_WithMocks(t *testing.T) {
	// Setup mocks
	mockAuth := SetupMockAuthService()
	mockStorage := SetupMockStorage()

	// Configure mock expectations
	claims := CreateTestClaims("testuser", "standard")
	ExpectValidToken(mockAuth, "valid-token", claims)

	testActor := BuildTestActor("testuser")
	ExpectActorExists(mockStorage, "testuser", testActor)

	// Create test app with mocks
	app := NewTestApp()

	// Add auth middleware that uses the mock
	app.App().Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			auth := ctx.Header("Authorization")
			if auth == "" {
				return ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
			}

			// Extract token from "Bearer token"
			token := strings.TrimPrefix(auth, "Bearer ")
			claims, err := mockAuth.ValidateAccessToken(token)
			if err != nil {
				return ctx.Status(401).JSON(map[string]string{"error": "invalid token"})
			}

			// Set claims in context
			ctx.Set("claims", claims)
			return next.Handle(ctx)
		})
	})

	// Setup test route that uses mocks
	app.App().GET("/user/:id", func(ctx *lift.Context) error {
		userID := ctx.Param("id")

		// Use the mocked storage service
		actor, err := mockStorage.GetActor(ctx.Context, userID)
		if err != nil {
			return ctx.Status(404).JSON(map[string]string{"error": "not found"})
		}

		return ctx.JSON(actor)
	})

	// Test with valid user
	response := app.
		WithHeader("Authorization", "Bearer valid-token").
		GET("/user/testuser")

	assert.Equal(t, 200, response.StatusCode)
	assert.Contains(t, response.Body, "testuser")

	// Verify mock expectations
	mockAuth.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
}

func TestLiftTestingFramework_AuthenticationFlow(t *testing.T) {
	app := NewTestApp()

	// Setup authenticated endpoint
	app.App().GET("/protected", func(ctx *lift.Context) error {
		// Check if authenticated (in real app, this would be middleware)
		authHeader := ctx.Header("Authorization")
		if authHeader == "" {
			return ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
		}

		return ctx.JSON(map[string]string{"message": "access granted"})
	})

	// Test without authentication
	response := app.GET("/protected")
	assert.Equal(t, 401, response.StatusCode)

	// Test with authentication
	response = app.
		WithHeader("Authorization", "Bearer test-token").
		GET("/protected")
	assert.Equal(t, 200, response.StatusCode)
	assert.Contains(t, response.Body, "access granted")
}

func TestLiftTestingFramework_PostRequest(t *testing.T) {
	app := NewTestApp()

	// Setup POST endpoint
	app.App().POST("/users", func(ctx *lift.Context) error {
		var user CreateUserRequest
		if err := ctx.ParseRequest(&user); err != nil {
			return ctx.Status(400).JSON(map[string]string{"error": "invalid request"})
		}

		// Create user (mocked)
		created := &TestUser{
			ID:       "user-123",
			Username: user.Username,
			Email:    user.Email,
			Name:     user.Name,
		}

		return ctx.Status(201).JSON(created)
	})

	// Test user creation
	request := NewCreateUserRequest("newuser")
	response := app.POST("/users", request)

	assert.Equal(t, 201, response.StatusCode)
	assert.Contains(t, response.Body, "newuser")

	// Verify response structure
	var user TestUser
	err := response.JSON(&user)
	assert.NoError(t, err)
	assert.Equal(t, "newuser", user.Username)
}

func TestLiftTestingFramework_ErrorHandling(t *testing.T) {
	app := NewTestApp()

	// Setup endpoint that can fail
	app.App().GET("/error", func(ctx *lift.Context) error {
		return errors.New("something went wrong")
	})

	// Test error response
	response := app.GET("/error")

	assert.False(t, response.IsSuccess())
	assert.True(t, response.StatusCode >= 400)
}

func TestLiftTestingFramework_IntegrationTest(t *testing.T) {
	// Run integration test
	RunIntegrationTest(t, nil, func(suite *IntegrationTestSuite) {
		// Create test app
		_ = suite.CreateTestApp()

		// Create test data
		ctx := context.Background()
		actor, err := suite.TestData.CreateTestActor(ctx, suite.Storage, "testuser")
		assert.NoError(t, err)
		assert.NotNil(t, actor)

		// Test the integration
		// This would test against real storage in a full integration test
	})
}

func TestLiftTestingFramework_PerformanceTest(t *testing.T) {
	suite := NewPerformanceTestSuite(t)

	// Define performance test
	perfTest := &PerformanceTest{
		Name:        "BasicEndpoint",
		Iterations:  100,
		Concurrency: 10,
		Timeout:     30 * time.Second,
		WarmupRuns:  5,
		TestFunc: func() error {
			app := NewTestApp()
			app.App().GET("/fast", func(ctx *lift.Context) error {
				return ctx.JSON(map[string]string{"result": "fast"})
			})

			response := app.GET("/fast")
			if !response.IsSuccess() {
				return errors.New("request failed")
			}
			return nil
		},
	}

	// Run performance test
	results := suite.RunPerformanceTest(perfTest)

	// Assert performance requirements
	requirements := &PerformanceRequirements{
		MaxAvgTime:    100 * time.Millisecond,
		MaxP95Time:    200 * time.Millisecond,
		MaxMemory:     1024 * 1024, // 1MB
		MinThroughput: 10.0,        // 10 ops/sec
	}

	AssertPerformance(t, results, requirements)

	// Print results for visibility
	t.Logf("Performance Results:")
	t.Logf("  Average time: %v", results.AvgTime)
	t.Logf("  P95 time: %v", results.P95Time)
	t.Logf("  Max memory: %d bytes", results.MaxMemory)
	t.Logf("  Throughput: %.2f ops/sec", results.Throughput)
}

func TestLiftTestingFramework_ColdStartTest(t *testing.T) {
	suite := NewPerformanceTestSuite(t)

	// Define cold start test
	coldStartTest := &ColdStartTest{
		Iterations:   5,
		MemorySize:   128,
		ExpectedTime: 500 * time.Millisecond,
		InitFunc: func() error {
			// Simulate Lambda initialization
			time.Sleep(50 * time.Millisecond)
			return nil
		},
		HandlerFunc: func() error {
			// Simulate handler execution
			app := NewTestApp()
			app.App().GET("/test", func(ctx *lift.Context) error {
				return ctx.JSON(map[string]string{"cold": "start"})
			})

			response := app.GET("/test")
			if !response.IsSuccess() {
				return errors.New("handler failed")
			}
			return nil
		},
	}

	// Run cold start test
	results := suite.MeasureColdStart(coldStartTest)

	// Assert cold start performance
	maxColdStart := 1 * time.Second
	AssertColdStartPerformance(t, results, maxColdStart)

	// Print results
	t.Logf("Cold Start Results:")
	t.Logf("  Average init time: %v", results.AvgInitTime)
	t.Logf("  Average handler time: %v", results.AvgHandlerTime)
	t.Logf("  Average total time: %v", results.AvgTotalTime)
}

func TestLiftTestingFramework_LoadTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	suite := NewPerformanceTestSuite(t)

	// Define load test
	loadTest := &LoadTest{
		Duration:    5 * time.Second,
		Concurrency: 10,
		RampUpTime:  1 * time.Second,
		TestFunc: func() error {
			app := NewTestApp()
			app.App().GET("/load", func(ctx *lift.Context) error {
				// Simulate some work
				time.Sleep(10 * time.Millisecond)
				return ctx.JSON(map[string]string{"load": "test"})
			})

			response := app.GET("/load")
			if !response.IsSuccess() {
				return errors.New("load test request failed")
			}
			return nil
		},
	}

	// Run load test
	results := suite.RunLoadTest(loadTest)

	// Assert load test results
	assert.Greater(t, results.RequestCount, int64(0), "Should have processed requests")
	assert.LessOrEqual(t, results.ErrorRate, 0.05, "Error rate should be < 5%")
	assert.Greater(t, results.RequestsPerSec, 1.0, "Should have decent throughput")

	// Print results
	t.Logf("Load Test Results:")
	t.Logf("  Total requests: %d", results.RequestCount)
	t.Logf("  Error count: %d", results.ErrorCount)
	t.Logf("  Error rate: %.2f%%", results.ErrorRate*100)
	t.Logf("  Requests/sec: %.2f", results.RequestsPerSec)
	t.Logf("  Average response time: %v", results.AvgResponseTime)
}

func TestLiftTestingFramework_CompleteWorkflow(t *testing.T) {
	// This demonstrates a complete test workflow using all framework features

	// 1. Setup mocks
	mockAuth := SetupMockAuthService()
	mockStorage := SetupMockStorage()

	// 2. Configure expectations
	adminClaims := CreateTestClaims("admin", "admin")
	ExpectValidToken(mockAuth, "admin-token", adminClaims)

	testActor := BuildTestActor("testuser")
	mockStorage.On("CreateActor", mock.Anything, mock.AnythingOfType("*models.Actor")).Return(nil)
	mockStorage.On("GetActor", mock.Anything, "testuser").Return(testActor, nil)

	// 3. Create test app
	app := NewTestApp()

	// Add proper auth middleware that uses the mock
	app.App().Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			auth := ctx.Header("Authorization")
			if auth == "" {
				return ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
			}

			// Extract token from "Bearer token"
			token := strings.TrimPrefix(auth, "Bearer ")
			claims, err := mockAuth.ValidateAccessToken(token)
			if err != nil {
				return ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
			}

			// Set claims in context
			ctx.Set("claims", claims)
			return next.Handle(ctx)
		})
	})

	// 4. Setup routes
	app.App().POST("/admin/users", func(ctx *lift.Context) error {
		// Auth is handled by middleware, just check claims
		claims := ctx.Get("claims").(*auth.EnhancedClaims)
		if !contains(claims.Scopes, "admin:write") {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient permissions"})
		}

		var request CreateUserRequest
		if err := ctx.ParseRequest(&request); err != nil {
			return ctx.Status(400).JSON(map[string]string{"error": "bad request"})
		}

		// Create actor using mock
		actor := BuildTestActor(request.Username)
		err := mockStorage.CreateActor(ctx.Context, actor)
		if err != nil {
			return ctx.Status(500).JSON(map[string]string{"error": "creation failed"})
		}

		return ctx.Status(201).JSON(actor)
	})

	app.App().GET("/admin/users/:id", func(ctx *lift.Context) error {
		// Auth is handled by middleware
		userID := ctx.Param("id")
		actor, err := mockStorage.GetActor(ctx.Context, userID)
		if err != nil {
			return ctx.Status(404).JSON(map[string]string{"error": "not found"})
		}

		return ctx.JSON(actor)
	})

	// 5. Test the complete workflow
	t.Run("UnauthorizedAccess", func(t *testing.T) {
		// Test unauthorized access first, before setting up other expectations
		response := app.GET("/admin/users/testuser")
		AssertErrorResponse(t, response, 401, "unauthorized")
	})

	t.Run("CreateUser", func(t *testing.T) {
		request := NewCreateUserRequest("testuser")
		response := app.
			WithHeader("Authorization", "Bearer admin-token").
			POST("/admin/users", request)

		AssertSuccessResponse(t, response)
		assert.Equal(t, 201, response.StatusCode)
	})

	t.Run("GetUser", func(t *testing.T) {
		response := app.
			WithHeader("Authorization", "Bearer admin-token").
			GET("/admin/users/testuser")

		AssertSuccessResponse(t, response)
		assert.Contains(t, response.Body, "testuser")
	})

	// 6. Verify all mocks
	mockAuth.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
}

// Benchmark tests

func BenchmarkTestApp_SimpleGET(b *testing.B) {
	app := NewTestApp()
	app.App().GET("/bench", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"benchmark": "test"})
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		response := app.GET("/bench")
		if !response.IsSuccess() {
			b.Errorf("Request failed: %s", response.Body)
		}
	}
}

func BenchmarkTestApp_WithHeaders(b *testing.B) {
	app := NewTestApp()
	app.App().GET("/bench", func(ctx *lift.Context) error {
		auth := ctx.Header("Authorization")
		tenant := ctx.Header("X-Tenant-ID")

		return ctx.JSON(map[string]interface{}{
			"auth":   auth,
			"tenant": tenant,
			"bench":  "test",
		})
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		response := app.
			WithHeader("Authorization", "Bearer token").
			WithHeader("X-Tenant-ID", "tenant123").
			GET("/bench")

		if !response.IsSuccess() {
			b.Errorf("Request failed: %s", response.Body)
		}
	}
}

func BenchmarkTestApp_POST(b *testing.B) {
	app := NewTestApp()
	app.App().POST("/bench", func(ctx *lift.Context) error {
		var req CreateUserRequest
		if err := ctx.ParseRequest(&req); err != nil {
			return ctx.Status(400).JSON(map[string]string{"error": "bad request"})
		}

		return ctx.Status(201).JSON(map[string]string{
			"created": req.Username,
		})
	})

	request := NewCreateUserRequest("benchuser")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		response := app.POST("/bench", request)
		if response.StatusCode != 201 {
			b.Errorf("Request failed with status %d: %s", response.StatusCode, response.Body)
		}
	}
}
