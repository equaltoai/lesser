package lift

import (
	"context"
	"encoding/json"
	"testing"
)

func TestHandleRequestExecutesMiddleware(t *testing.T) {
	// Create app
	app := New()

	// Track middleware execution
	middlewareExecuted := false

	// Add middleware that sets a value in context
	app.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			middlewareExecuted = true
			ctx.Set("middleware_value", "middleware_ran")
			return next.Handle(ctx)
		})
	})

	// Add handler that checks for the middleware value
	if err := app.GET("/test", func(ctx *Context) error {
		value := ctx.Get("middleware_value")
		if value != "middleware_ran" {
			t.Error("Middleware did not set expected value")
		}
		return ctx.JSON(map[string]string{"status": "ok"})
	}); err != nil {
		t.Fatalf("Failed to register route: %v", err)
	}

	// Create API Gateway v1 event (using map format like other tests)
	event := map[string]any{
		"resource":   "/test",
		"httpMethod": "GET",
		"path":       "/test",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
		},
		"headers": map[string]any{
			"Content-Type": "application/json",
		},
	}

	// Call HandleRequest directly (simulating Lambda)
	response, err := app.HandleRequest(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleRequest failed: %v", err)
	}

	// Verify middleware was executed
	if !middlewareExecuted {
		t.Error("Middleware was not executed")
	}

	// Verify response
	resp, ok := response.(*Response)
	if !ok {
		t.Fatalf("Expected *Response, got %T", response)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestHandleRequestMiddlewareOrder(t *testing.T) {
	// Create app
	app := New()

	// Track execution order
	var executionOrder []string

	// Add multiple middleware
	app.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			executionOrder = append(executionOrder, "middleware1_before")
			err := next.Handle(ctx)
			executionOrder = append(executionOrder, "middleware1_after")
			return err
		})
	})

	app.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			executionOrder = append(executionOrder, "middleware2_before")
			err := next.Handle(ctx)
			executionOrder = append(executionOrder, "middleware2_after")
			return err
		})
	})

	// Add handler
	if err := app.GET("/test", func(ctx *Context) error {
		executionOrder = append(executionOrder, "handler")
		return ctx.JSON(map[string]string{"status": "ok"})
	}); err != nil {
		t.Fatalf("Failed to register route: %v", err)
	}

	// Create API Gateway v1 event
	event := map[string]any{
		"resource":   "/test",
		"httpMethod": "GET",
		"path":       "/test",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
		},
	}

	// Call HandleRequest
	_, err := app.HandleRequest(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleRequest failed: %v", err)
	}

	// Verify execution order
	expected := []string{
		"middleware1_before",
		"middleware2_before",
		"handler",
		"middleware2_after",
		"middleware1_after",
	}

	if len(executionOrder) != len(expected) {
		t.Fatalf("Expected %d executions, got %d", len(expected), len(executionOrder))
	}

	for i, exp := range expected {
		if executionOrder[i] != exp {
			t.Errorf("Expected execution[%d] to be %s, got %s", i, exp, executionOrder[i])
		}
	}
}

func TestHandleRequestWithDependencyInjection(t *testing.T) {
	// Create app
	app := New()

	// Mock service
	type TestService struct {
		Name string
	}

	// Add middleware that injects service
	app.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			service := &TestService{Name: "test_service"}
			ctx.Set("service", service)
			return next.Handle(ctx)
		})
	})

	// Add handler that uses the service
	if err := app.GET("/test", func(ctx *Context) error {
		serviceInterface := ctx.Get("service")
		if serviceInterface == nil {
			t.Error("Service not found in context")
			return NewLiftError("SERVICE_NOT_FOUND", "Service not found", 500)
		}

		service, ok := serviceInterface.(*TestService)
		if !ok {
			t.Error("Service has wrong type")
			return NewLiftError("INVALID_SERVICE_TYPE", "Invalid service type", 500)
		}

		return ctx.JSON(map[string]string{"service_name": service.Name})
	}); err != nil {
		t.Fatalf("Failed to register route: %v", err)
	}

	// Create API Gateway v1 event
	event := map[string]any{
		"resource":   "/test",
		"httpMethod": "GET",
		"path":       "/test",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
		},
	}

	// Call HandleRequest
	response, err := app.HandleRequest(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleRequest failed: %v", err)
	}

	// Verify response
	resp, ok := response.(*Response)
	if !ok {
		t.Fatalf("Expected *Response, got %T", response)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	expectedBody := `{"service_name":"test_service"}`
	bodyBytes, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("Failed to marshal response body: %v", err)
	}
	if string(bodyBytes) != expectedBody {
		t.Errorf("Expected body %s, got %s", expectedBody, string(bodyBytes))
	}
}
