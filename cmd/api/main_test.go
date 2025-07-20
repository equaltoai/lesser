package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
)

func TestLiftToLambdaRequest(t *testing.T) {
	// Create a Lift context
	ctx := &lift.Context{
		Context: context.Background(),
		Request: lift.Request{
			Method: "GET",
			Path:   "/test",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			QueryStringParameters: map[string]string{
				"param1": "value1",
			},
			Body:            `{"test": "data"}`,
			IsBase64Encoded: false,
		},
	}

	// Convert to Lambda request
	lambdaReq := liftToLambdaRequest(ctx)

	// Verify conversion
	assert.Equal(t, "GET", lambdaReq.RequestContext.HTTP.Method)
	assert.Equal(t, "/test", lambdaReq.RequestContext.HTTP.Path)
	assert.Equal(t, "application/json", lambdaReq.Headers["Content-Type"])
	assert.Equal(t, "value1", lambdaReq.QueryStringParameters["param1"])
	assert.Equal(t, `{"test": "data"}`, lambdaReq.Body)
	assert.False(t, lambdaReq.IsBase64Encoded)
}

func TestLambdaResponseToLift(t *testing.T) {
	// Create a Lift context
	ctx := &lift.Context{
		Context:  context.Background(),
		Response: lift.Response{},
	}

	// Create a Lambda response
	lambdaResp := &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: `{"message": "success"}`,
	}

	// Convert Lambda response to Lift response
	err := lambdaResponseToLift(ctx, lambdaResp)
	assert.NoError(t, err)

	// Verify conversion
	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	assert.Equal(t, "application/json", ctx.Response.Headers["Content-Type"])

	// Verify body was parsed as JSON
	var responseBody map[string]interface{}
	err = json.Unmarshal([]byte(ctx.Response.Body), &responseBody)
	assert.NoError(t, err)
	assert.Equal(t, "success", responseBody["message"])
}

func TestCreateLoggingMiddleware(t *testing.T) {
	// Create a mock logger
	logger := &mockLogger{}

	// Create the middleware
	middleware := createLoggingMiddleware(logger)

	// Create a mock handler
	mockHandler := &mockHandler{}

	// Create a handler with the middleware
	handler := middleware(mockHandler)

	// Create a context
	ctx := &lift.Context{
		Context: context.Background(),
		Request: lift.Request{
			Method: "GET",
			Path:   "/test",
		},
		Response: lift.Response{
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
type mockLogger struct{}

func (m *mockLogger) Info(msg string, fields ...interface{})  {}
func (m *mockLogger) Error(msg string, fields ...interface{}) {}
func (m *mockLogger) Debug(msg string, fields ...interface{}) {}
func (m *mockLogger) Warn(msg string, fields ...interface{})  {}
func (m *mockLogger) Fatal(msg string, fields ...interface{}) {}

type mockHandler struct {
	called bool
}

func (m *mockHandler) Handle(ctx *lift.Context) error {
	m.called = true
	return nil
}
