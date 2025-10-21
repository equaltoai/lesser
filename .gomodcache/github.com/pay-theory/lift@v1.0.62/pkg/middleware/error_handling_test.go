package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
)

func TestErrorHandlingImprovements(t *testing.T) {
	t.Run("Recovery middleware handles JSON response errors gracefully", func(t *testing.T) {
		middleware := Recover()

		panicHandler := lift.HandlerFunc(func(_ *lift.Context) error {
			panic("test panic")
		})

		handler := middleware(panicHandler)
		ctx := createErrorTestContext("GET", "/test", nil)

		// This should not panic and should handle the error gracefully
		err := handler.Handle(ctx)
		assert.NoError(t, err, "Recovery middleware should handle panic gracefully")
		assert.Equal(t, 500, ctx.Response.StatusCode, "Should set 500 status code")
	})

	t.Run("Error handler middleware processes LiftErrors correctly", func(t *testing.T) {
		middleware := ErrorHandler()

		errorHandler := lift.HandlerFunc(func(_ *lift.Context) error {
			return &lift.LiftError{
				Code:       "TEST_ERROR",
				Message:    "Test error message",
				StatusCode: 400,
			}
		})

		handler := middleware(errorHandler)
		ctx := createErrorTestContext("GET", "/test", nil)

		err := handler.Handle(ctx)
		assert.NoError(t, err, "Error handler should process LiftError without propagating")
		assert.Equal(t, 400, ctx.Response.StatusCode, "Should set correct status code from LiftError")
	})

	t.Run("Error handler middleware processes generic errors correctly", func(t *testing.T) {
		middleware := ErrorHandler()

		errorHandler := lift.HandlerFunc(func(_ *lift.Context) error {
			return errors.New("generic test error")
		})

		handler := middleware(errorHandler)
		ctx := createErrorTestContext("GET", "/test", nil)

		err := handler.Handle(ctx)
		assert.NoError(t, err, "Error handler should process generic error without propagating")
		assert.Equal(t, 500, ctx.Response.StatusCode, "Should set 500 status code for generic errors")
	})

	t.Run("Middleware chain works correctly with error handling", func(t *testing.T) {
		// Chain multiple middleware including our improved error handlers
		middlewareChain := Chain(
			RequestID(),
			Recover(),
			ErrorHandler(),
		)

		successHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"status": "ok"})
		})

		handler := middlewareChain(successHandler)
		ctx := createErrorTestContext("GET", "/test", nil)

		err := handler.Handle(ctx)
		assert.NoError(t, err, "Middleware chain should work correctly")
		assert.Equal(t, 200, ctx.Response.StatusCode, "Should maintain 200 status for successful requests")
		assert.NotEmpty(t, ctx.RequestID, "Should set request ID")
	})
}

func TestSecurityErrorHandling(t *testing.T) {
	t.Run("Security middleware continues to work with error improvements", func(t *testing.T) {
		middlewareChain := Chain(
			func(next lift.Handler) lift.Handler {
				return SecurityHeaders(DefaultSecurityHeadersConfig())(next)
			},
			ErrorHandler(),
		)

		handler := middlewareChain(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.JSON(map[string]string{"message": "secure"})
		}))

		ctx := createErrorTestContext("GET", "/api/auth/login", nil) // Sensitive path

		err := handler.Handle(ctx)
		assert.NoError(t, err)

		// Check that security headers are still applied
		assert.NotEmpty(t, ctx.Response.Headers["X-Frame-Options"])
		assert.NotEmpty(t, ctx.Response.Headers["X-Content-Type-Options"])

		// Check that cache control headers are applied for sensitive paths
		assert.NotEmpty(t, ctx.Response.Headers["Cache-Control"])
	})
}

// Helper function to create test context for error handling tests
func createErrorTestContext(method, path string, body []byte) *lift.Context {
	adapterReq := &adapters.Request{
		Method:      method,
		Path:        path,
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		PathParams:  make(map[string]string),
		Body:        body,
	}
	req := lift.NewRequest(adapterReq)
	ctx := lift.NewContext(context.Background(), req)

	// Initialize response headers if not already done
	if ctx.Response.Headers == nil {
		ctx.Response.Headers = make(map[string]string)
	}

	return ctx
}
