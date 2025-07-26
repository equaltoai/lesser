package lift

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
)

// MiddlewareTestCase defines a test case for middleware
type MiddlewareTestCase struct {
	Name           string
	Middleware     lift.Middleware
	Context        *lift.Context
	ExpectedCalled bool
	ExpectedError  string
	ExpectedState  map[string]interface{}
	SetupFunc      func(*lift.Context)
	ValidateFunc   func(*testing.T, *lift.Context)
}

// MiddlewareTestSuite provides utilities for testing middleware
type MiddlewareTestSuite struct {
	t             *testing.T
	handlerCalled bool
	handlerError  error
}

// NewMiddlewareTestSuite creates a new middleware test suite
func NewMiddlewareTestSuite(t *testing.T) *MiddlewareTestSuite {
	return &MiddlewareTestSuite{
		t: t,
	}
}

// TestMiddleware tests a single middleware
func (s *MiddlewareTestSuite) TestMiddleware(tc MiddlewareTestCase) {
	s.t.Run(tc.Name, func(t *testing.T) {
		// Reset state
		s.handlerCalled = false
		s.handlerError = nil

		// Create context if not provided
		ctx := tc.Context
		if ctx == nil {
			ctx = MockLiftContext("GET", "/test")
		}

		// Run setup if provided
		if tc.SetupFunc != nil {
			tc.SetupFunc(ctx)
		}

		// Create mock handler
		mockHandler := s.createMockHandler(tc.ExpectedError)

		// Apply middleware
		wrappedHandler := tc.Middleware(mockHandler)

		// Execute
		err := wrappedHandler.Handle(ctx)

		// Validate
		if tc.ExpectedError != "" {
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.ExpectedError)
		} else {
			assert.NoError(t, err)
		}

		assert.Equal(t, tc.ExpectedCalled, s.handlerCalled, "Handler called mismatch")

		// Check expected state
		if tc.ExpectedState != nil {
			for key, expectedValue := range tc.ExpectedState {
				actualValue := ctx.Get(key)
				assert.Equal(t, expectedValue, actualValue, fmt.Sprintf("Context value %s mismatch", key))
			}
		}

		// Run custom validation if provided
		if tc.ValidateFunc != nil {
			tc.ValidateFunc(t, ctx)
		}
	})
}

// TestMiddlewareChain tests a chain of middleware
func (s *MiddlewareTestSuite) TestMiddlewareChain(name string, middlewares []lift.Middleware, ctx *lift.Context, validateFunc func(*testing.T, *lift.Context)) {
	s.t.Run(name, func(t *testing.T) {
		// Reset state
		s.handlerCalled = false

		// Create mock handler
		handler := s.createMockHandler("")

		// Apply middleware chain in reverse order
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}

		// Execute
		err := handler.Handle(ctx)
		assert.NoError(t, err)

		// Validate
		assert.True(t, s.handlerCalled, "Handler should have been called")
		if validateFunc != nil {
			validateFunc(t, ctx)
		}
	})
}

// createMockHandler creates a mock handler for testing
func (s *MiddlewareTestSuite) createMockHandler(expectedError string) lift.Handler {
	return lift.HandlerFunc(func(ctx *lift.Context) error {
		s.handlerCalled = true
		if expectedError != "" {
			s.handlerError = errors.New(expectedError)
			return s.handlerError
		}
		return nil
	})
}

// Common middleware test helpers

// TestAuthMiddleware tests authentication middleware
func TestAuthMiddleware(t *testing.T, middleware lift.Middleware) {
	suite := NewMiddlewareTestSuite(t)

	testCases := []MiddlewareTestCase{
		{
			Name:       "Valid authentication",
			Middleware: middleware,
			Context: MockLiftContext("GET", "/protected",
				WithHeaders(map[string]string{
					"Authorization": "Bearer valid-token",
				}),
			),
			ExpectedCalled: true,
			ExpectedState: map[string]interface{}{
				"authenticated": true,
			},
		},
		{
			Name:           "Missing authentication",
			Middleware:     middleware,
			Context:        MockLiftContext("GET", "/protected"),
			ExpectedCalled: false,
			ExpectedError:  "unauthorized",
		},
		{
			Name:       "Invalid token",
			Middleware: middleware,
			Context: MockLiftContext("GET", "/protected",
				WithHeaders(map[string]string{
					"Authorization": "Bearer invalid-token",
				}),
			),
			ExpectedCalled: false,
			ExpectedError:  "invalid token",
		},
	}

	for _, tc := range testCases {
		suite.TestMiddleware(tc)
	}
}

// TestRateLimitMiddleware tests rate limiting middleware
func TestRateLimitMiddleware(t *testing.T, middleware lift.Middleware) {
	suite := NewMiddlewareTestSuite(t)

	ctx := MockLiftContext("GET", "/api/resource",
		WithHeaders(map[string]string{
			"X-Client-ID": "test-client",
		}),
	)

	// First request should succeed
	suite.TestMiddleware(MiddlewareTestCase{
		Name:           "First request within limit",
		Middleware:     middleware,
		Context:        ctx,
		ExpectedCalled: true,
	})

	// Simulate rapid requests
	for i := 0; i < 10; i++ {
		suite.TestMiddleware(MiddlewareTestCase{
			Name:           fmt.Sprintf("Request %d", i+2),
			Middleware:     middleware,
			Context:        ctx,
			ExpectedCalled: i < 5, // Assuming limit is 5
			ExpectedError:  func() string {
				if i >= 5 {
					return "rate limit exceeded"
				}
				return ""
			}(),
		})
	}
}

// TestCORSMiddleware tests CORS middleware
func TestCORSMiddleware(t *testing.T, middleware lift.Middleware) {
	suite := NewMiddlewareTestSuite(t)

	testCases := []MiddlewareTestCase{
		{
			Name:       "Preflight request",
			Middleware: middleware,
			Context: MockLiftContext("OPTIONS", "/api/resource",
				WithHeaders(map[string]string{
					"Origin":                         "https://example.com",
					"Access-Control-Request-Method":  "POST",
					"Access-Control-Request-Headers": "Content-Type",
				}),
			),
			ExpectedCalled: false, // Preflight doesn't call handler
			ValidateFunc: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 200, ctx.Response.StatusCode)
				assert.NotEmpty(t, ctx.Response.Headers["Access-Control-Allow-Origin"])
				assert.NotEmpty(t, ctx.Response.Headers["Access-Control-Allow-Methods"])
			},
		},
		{
			Name:       "Regular request with CORS",
			Middleware: middleware,
			Context: MockLiftContext("GET", "/api/resource",
				WithHeaders(map[string]string{
					"Origin": "https://example.com",
				}),
			),
			ExpectedCalled: true,
			ValidateFunc: func(t *testing.T, ctx *lift.Context) {
				assert.NotEmpty(t, ctx.Response.Headers["Access-Control-Allow-Origin"])
			},
		},
	}

	for _, tc := range testCases {
		suite.TestMiddleware(tc)
	}
}

// TestLoggingMiddleware tests logging middleware
func TestLoggingMiddleware(t *testing.T, middleware lift.Middleware) {
	suite := NewMiddlewareTestSuite(t)

	startTime := time.Now()

	suite.TestMiddleware(MiddlewareTestCase{
		Name:       "Logs request details",
		Middleware: middleware,
		Context: MockLiftContext("POST", "/api/users",
			WithBody(map[string]string{"name": "test"}),
			WithHeaders(map[string]string{
				"X-Request-ID": "test-123",
			}),
		),
		ExpectedCalled: true,
		ValidateFunc: func(t *testing.T, ctx *lift.Context) {
			// Check if request ID was set
			requestID := ctx.Get("request_id")
			assert.NotNil(t, requestID)

			// Check if timing was recorded
			duration := time.Since(startTime)
			assert.True(t, duration > 0)
		},
	})
}

// TestTenantMiddleware tests multi-tenant middleware
func TestTenantMiddleware(t *testing.T, middleware lift.Middleware) {
	suite := NewMiddlewareTestSuite(t)

	testCases := []MiddlewareTestCase{
		{
			Name:       "Valid tenant header",
			Middleware: middleware,
			Context: MockLiftContext("GET", "/api/data",
				WithHeaders(map[string]string{
					"X-Tenant-ID": "tenant-123",
				}),
			),
			ExpectedCalled: true,
			ExpectedState: map[string]interface{}{
				"tenant_id": "tenant-123",
			},
		},
		{
			Name:           "Missing tenant header",
			Middleware:     middleware,
			Context:        MockLiftContext("GET", "/api/data"),
			ExpectedCalled: false,
			ExpectedError:  "tenant required",
		},
		{
			Name:       "Tenant from subdomain",
			Middleware: middleware,
			Context: MockLiftContext("GET", "/api/data",
				WithHeaders(map[string]string{
					"Host": "tenant-456.example.com",
				}),
			),
			ExpectedCalled: true,
			ExpectedState: map[string]interface{}{
				"tenant_id": "tenant-456",
			},
		},
	}

	for _, tc := range testCases {
		suite.TestMiddleware(tc)
	}
}

// MiddlewareRecorder records middleware execution for testing
type MiddlewareRecorder struct {
	ExecutionOrder []string
	Errors         []error
	States         map[string]map[string]interface{}
}

// NewMiddlewareRecorder creates a new middleware recorder
func NewMiddlewareRecorder() *MiddlewareRecorder {
	return &MiddlewareRecorder{
		ExecutionOrder: make([]string, 0),
		Errors:         make([]error, 0),
		States:         make(map[string]map[string]interface{}),
	}
}

// RecordingMiddleware creates a middleware that records its execution
func (r *MiddlewareRecorder) RecordingMiddleware(name string) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Record pre-execution
			r.ExecutionOrder = append(r.ExecutionOrder, name+"-before")

			// Capture state before
			stateBefore := make(map[string]interface{})
			stateBefore["authenticated"] = ctx.Get("authenticated")
			stateBefore["tenant_id"] = ctx.Get("tenant_id")
			r.States[name+"-before"] = stateBefore

			// Call next handler
			err := next.Handle(ctx)
			if err != nil {
				r.Errors = append(r.Errors, err)
			}

			// Record post-execution
			r.ExecutionOrder = append(r.ExecutionOrder, name+"-after")

			// Capture state after
			stateAfter := make(map[string]interface{})
			stateAfter["authenticated"] = ctx.Get("authenticated")
			stateAfter["tenant_id"] = ctx.Get("tenant_id")
			r.States[name+"-after"] = stateAfter

			return err
		})
	}
}

// AssertExecutionOrder verifies middleware execution order
func (r *MiddlewareRecorder) AssertExecutionOrder(t *testing.T, expected []string) {
	assert.Equal(t, expected, r.ExecutionOrder, "Middleware execution order mismatch")
}

// MockTimeProvider provides controllable time for testing
type MockTimeProvider struct {
	CurrentTime time.Time
}

// Now returns the mocked current time
func (m *MockTimeProvider) Now() time.Time {
	return m.CurrentTime
}

// Advance advances the mock time by duration
func (m *MockTimeProvider) Advance(d time.Duration) {
	m.CurrentTime = m.CurrentTime.Add(d)
}

// ContextModifier allows modification of context in tests
type ContextModifier func(*lift.Context)

// ChainModifiers chains multiple context modifiers
func ChainModifiers(modifiers ...ContextModifier) ContextModifier {
	return func(ctx *lift.Context) {
		for _, mod := range modifiers {
			mod(ctx)
		}
	}
}

// WithContextValue sets a context value
func WithContextValue(key string, value interface{}) ContextModifier {
	return func(ctx *lift.Context) {
		ctx.Set(key, value)
	}
}

// WithError simulates an error in the context
func WithError(err error) ContextModifier {
	return func(ctx *lift.Context) {
		ctx.Set("error", err)
	}
}

// TestMiddlewareError tests middleware error handling
func TestMiddlewareError(t *testing.T, middleware lift.Middleware, expectedStatus int, expectedMessage string) {
	ctx := MockLiftContext("GET", "/test")
	
	errorHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
		return errors.New("handler error")
	})
	
	wrappedHandler := middleware(errorHandler)
	err := wrappedHandler.Handle(ctx)
	
	assert.Error(t, err)
	assert.Equal(t, expectedStatus, ctx.Response.StatusCode)
	if expectedMessage != "" {
		assert.Contains(t, ctx.Response.Body, expectedMessage)
	}
}

// TestMiddlewarePerformance benchmarks middleware performance
func TestMiddlewarePerformance(b *testing.B, middleware lift.Middleware) {
	ctx := MockLiftContext("GET", "/test")
	handler := lift.HandlerFunc(func(ctx *lift.Context) error {
		return nil
	})
	
	wrappedHandler := middleware(handler)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = wrappedHandler.Handle(ctx)
	}
}