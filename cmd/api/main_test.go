package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestCreateLoggingMiddleware(t *testing.T) {
	// Create a test logger
	logger, _ := zap.NewDevelopment()

	// Create the middleware
	middleware := createLoggingMiddleware(logger)

	// Create a mock handler
	mockHandler := &mockHandler{}

	// Create a handler with the middleware
	handler := middleware(mockHandler)

	// Create a context
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method: "GET",
			Path:   "/test",
		},
		Response: &lift.Response{
			StatusCode: http.StatusOK,
		},
	}

	// Call the handler
	err := handler.Handle(ctx)
	assert.NoError(t, err)

	// Verify the mock handler was called
	assert.True(t, mockHandler.called)
}

func TestCreateCORSMiddleware(t *testing.T) {
	// Create the middleware
	middleware := createCORSMiddleware()

	// Create a mock handler
	mockHandler := &mockHandler{}

	// Create a handler with the middleware
	handler := middleware(mockHandler)

	// Create a context for regular request
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method: "GET",
			Path:   "/test",
		},
		Response: &lift.Response{
			StatusCode: http.StatusOK,
		},
	}

	// Call the handler
	err := handler.Handle(ctx)
	assert.NoError(t, err)

	// Verify the mock handler was called
	assert.True(t, mockHandler.called)
}

func TestCreateCostTrackingMiddleware(t *testing.T) {
	// Create a test logger
	logger, _ := zap.NewDevelopment()

	// Create the middleware
	middleware := createCostTrackingMiddleware(logger)

	// Create a mock handler
	mockHandler := &mockHandler{}

	// Create a handler with the middleware
	handler := middleware(mockHandler)

	// Create a context
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method: "GET",
			Path:   "/test",
		},
		Response: &lift.Response{
			StatusCode: http.StatusOK,
		},
	}

	// Call the handler
	err := handler.Handle(ctx)
	assert.NoError(t, err)

	// Verify the mock handler was called
	assert.True(t, mockHandler.called)
}

// Mock types for testing
type mockHandler struct {
	called bool
}

func (m *mockHandler) Handle(_ *lift.Context) error {
	m.called = true
	return nil
}
