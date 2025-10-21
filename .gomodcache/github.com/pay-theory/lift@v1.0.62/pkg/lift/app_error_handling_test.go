package lift

import (
	"context"
	"testing"

	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRequestLiftErrorStatusCodes(t *testing.T) {
	tests := []struct {
		error          *LiftError
		name           string
		expectedCode   string
		expectedMsg    string
		expectedStatus int
		hasDetails     bool
	}{
		{
			name:           "400 Bad Request",
			error:          NewLiftError("BAD_REQUEST", "Invalid input", 400),
			expectedStatus: 400,
			expectedCode:   "BAD_REQUEST",
			expectedMsg:    "Invalid input",
			hasDetails:     false,
		},
		{
			name:           "401 Unauthorized",
			error:          NewLiftError("UNAUTHORIZED", "No API key provided", 401),
			expectedStatus: 401,
			expectedCode:   "UNAUTHORIZED",
			expectedMsg:    "No API key provided",
			hasDetails:     false,
		},
		{
			name:           "404 Not Found",
			error:          NewLiftError("NOT_FOUND", "Resource not found", 404),
			expectedStatus: 404,
			expectedCode:   "NOT_FOUND",
			expectedMsg:    "Resource not found",
			hasDetails:     false,
		},
		{
			name: "400 with Details",
			error: NewLiftError("VALIDATION_ERROR", "Validation failed", 400).
				WithDetail("field", "email").
				WithDetail("reason", "invalid format"),
			expectedStatus: 400,
			expectedCode:   "VALIDATION_ERROR",
			expectedMsg:    "Validation failed",
			hasDetails:     true,
		},
		{
			name:           "409 Conflict",
			error:          NewLiftError("CONFLICT", "Resource already exists", 409),
			expectedStatus: 409,
			expectedCode:   "CONFLICT",
			expectedMsg:    "Resource already exists",
			hasDetails:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := New()

			// Create a handler that returns the specific error
			if err := app.GET("/test", func(_ *Context) error {
				return tt.error
			}); err != nil {
				t.Fatalf("Failed to register route: %v", err)
			}

			// Create test request
			req := NewRequest(&adapters.Request{
				Method:      "GET",
				Path:        "/test",
				Headers:     make(map[string]string),
				QueryParams: make(map[string]string),
				Body:        []byte{},
				TriggerType: TriggerAPIGateway,
			})

			// Simulate HandleRequest flow
			ctx := NewContext(context.Background(), req)

			// Start the app to transfer middleware
			err := app.Start()
			require.NoError(t, err)

			// Handle the request through the router
			routerErr := app.router.Handle(ctx)
			require.Error(t, routerErr, "Handler should return an error")

			// Call handleError as HandleRequest would
			response, err := app.handleError(ctx, routerErr)
			require.NoError(t, err, "handleError should not return an error")
			require.NotNil(t, response, "Response should not be nil")

			// Verify status code
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode,
				"Status code should match LiftError.StatusCode")

			// Parse response body
			respBody, ok := ctx.Response.Body.(map[string]any)
			require.True(t, ok, "Response body should be map[string]any")
			require.NotNil(t, respBody, "Response body should not be nil")

			// Verify response structure
			assert.Equal(t, tt.expectedCode, respBody["code"], "Error code should match")
			assert.Equal(t, tt.expectedMsg, respBody["message"], "Error message should match")

			// Check details if expected
			if tt.hasDetails {
				assert.Contains(t, respBody, "details", "Response should contain details")
				details, ok := respBody["details"].(map[string]any)
				assert.True(t, ok, "Details should be a map")
				assert.NotEmpty(t, details, "Details should not be empty")
			} else {
				assert.NotContains(t, respBody, "details", "Response should not contain details")
			}
		})
	}
}

func TestHandleRequestNonLiftError(t *testing.T) {
	app := New()

	// Create a handler that returns a regular error
	if err := app.GET("/test", func(_ *Context) error {
		return assert.AnError // A regular error, not LiftError
	}); err != nil {
		t.Fatalf("Failed to register route: %v", err)
	}

	// Create test request
	req := NewRequest(&adapters.Request{
		Method:      "GET",
		Path:        "/test",
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		Body:        []byte{},
		TriggerType: TriggerAPIGateway,
	})

	// Simulate HandleRequest flow
	ctx := NewContext(context.Background(), req)

	// Start the app
	err := app.Start()
	require.NoError(t, err)

	// Handle the request
	routerErr := app.router.Handle(ctx)
	require.Error(t, routerErr, "Handler should return an error")

	// Call handleError
	response, err := app.handleError(ctx, routerErr)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Should return 500 for non-LiftError
	assert.Equal(t, 500, ctx.Response.StatusCode)

	// Parse response
	respBody, ok := ctx.Response.Body.(map[string]string)
	require.True(t, ok)

	// Should contain generic error message
	assert.Equal(t, "Internal server error", respBody["error"])
	assert.NotContains(t, respBody, "code", "Non-LiftError should not have code field")
}

func TestMiddlewareLiftErrorStatusCodes(t *testing.T) {
	app := New()

	// Add middleware that returns a LiftError
	app.Use(func(_ Handler) Handler {
		return HandlerFunc(func(_ *Context) error {
			// Simulate auth middleware that fails
			return NewLiftError("UNAUTHORIZED", "Invalid API key", 401)
		})
	})

	// Add a handler (should not be reached)
	if err := app.GET("/test", func(ctx *Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	}); err != nil {
		t.Fatalf("Failed to register route: %v", err)
	}

	// Create test request
	req := NewRequest(&adapters.Request{
		Method:      "GET",
		Path:        "/test",
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		Body:        []byte{},
		TriggerType: TriggerAPIGateway,
	})

	// Simulate HandleRequest flow
	ctx := NewContext(context.Background(), req)

	// Start the app
	err := app.Start()
	require.NoError(t, err)

	// Handle through router (middleware will fail)
	routerErr := app.router.Handle(ctx)
	require.Error(t, routerErr, "Middleware should return an error")

	// Call handleError
	response, err := app.handleError(ctx, routerErr)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify status code from middleware error
	assert.Equal(t, 401, ctx.Response.StatusCode)

	// Parse response
	respBody, ok := ctx.Response.Body.(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "UNAUTHORIZED", respBody["code"])
	assert.Equal(t, "Invalid API key", respBody["message"])
}
