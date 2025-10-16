package main

import (
	"context"
	"testing"

	liftapp "github.com/equaltoai/lesser/pkg/lift"
	liftpkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestCreateLoggingMiddleware(t *testing.T) {
	// Create a test logger
	logger, _ := zap.NewDevelopment()

	config := liftapp.DefaultConfig()
	builder := liftapp.NewAppBuilder(config, logger)

	// Create a mock handler
	mockHandler := &mockHandler{}

	// Create logging middleware handler
	handler := builderLoggingMiddleware(builder)(mockHandler)

	// Create a context
	ctx := &liftpkg.Context{
		Context: context.Background(),
		Request: &liftpkg.Request{
			Method: "GET",
			Path:   "/test",
		},
		Response: &liftpkg.Response{},
	}

	// Call the handler
	err := handler.Handle(ctx)
	assert.NoError(t, err)

	// Verify the mock handler was called
	assert.True(t, mockHandler.called)
}

func TestCreateCORSMiddleware(t *testing.T) {
	// Create a test logger
	logger, _ := zap.NewDevelopment()

	config := liftapp.DefaultConfig()
	builder := liftapp.NewAppBuilder(config, logger)

	// Create a mock handler
	mockHandler := &mockHandler{}

	// Create handler with CORS middleware
	handler := builderCORSMiddleware(builder)(mockHandler)

	// Create a context for regular request
	ctx := &liftpkg.Context{
		Context: context.Background(),
		Request: &liftpkg.Request{
			Method: "GET",
			Path:   "/test",
		},
		Response: &liftpkg.Response{Headers: make(map[string]string)},
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

	config := liftapp.DefaultConfig()
	builder := liftapp.NewAppBuilder(config, logger)

	// Create a mock handler
	mockHandler := &mockHandler{}

	// Create handler with cost tracking middleware
	handler := builderCostTrackingMiddleware(builder)(mockHandler)

	// Create a context
	ctx := &liftpkg.Context{
		Context: context.Background(),
		Request: &liftpkg.Request{
			Method: "GET",
			Path:   "/test",
		},
		Response: &liftpkg.Response{},
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

func (m *mockHandler) Handle(_ *liftpkg.Context) error {
	m.called = true
	return nil
}

func builderLoggingMiddleware(builder *liftapp.AppBuilder) liftpkg.Middleware {
	type loggingBuilder interface {
		createLoggingMiddleware() liftpkg.Middleware
	}
	return interface{}(builder).(loggingBuilder).createLoggingMiddleware()
}

func builderCORSMiddleware(builder *liftapp.AppBuilder) liftpkg.Middleware {
	type corsBuilder interface {
		createCORSMiddleware() liftpkg.Middleware
	}
	return interface{}(builder).(corsBuilder).createCORSMiddleware()
}

func builderCostTrackingMiddleware(builder *liftapp.AppBuilder) liftpkg.Middleware {
	type costBuilder interface {
		createCostTrackingMiddleware() liftpkg.Middleware
	}
	return interface{}(builder).(costBuilder).createCostTrackingMiddleware()
}
