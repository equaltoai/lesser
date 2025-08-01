package lift

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHandleGetAIAnalysisLift_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		pathParams     map[string]string
		expectedStatus int
	}{
		{
			name: "missing authentication returns 401",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			pathParams: map[string]string{
				"object_id": "test-object-123",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "missing object_id returns 401 (invalid token)",
			headers: map[string]string{
				"Authorization": "Bearer test-token",
			},
			pathParams:     map[string]string{},
			expectedStatus: http.StatusUnauthorized, // Token validation will fail
		},
		{
			name: "invalid bearer token returns 401",
			headers: map[string]string{
				"Authorization": "InvalidFormat token",
			},
			pathParams: map[string]string{
				"object_id": "test-object-123",
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "GET",
					Path:    "/api/v1/ai/analysis/test-object-123",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			// For path parameters, we need to implement a mock Param method
			// In real usage, Lift framework extracts these from the URL
			// For testing, we'll need to adjust the handler to accept test mode

			err := handler.HandleGetAIAnalysisLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleRequestAIAnalysisLift_Validation(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		body           string
		expectedStatus int
	}{
		{
			name: "missing authentication returns 401",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:           `{"object_id":"test-123"}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "missing object_id returns 401 (invalid token)",
			headers: map[string]string{
				"Authorization": "Bearer test-token",
				"Content-Type":  "application/json",
			},
			body:           `{"object_type":"status"}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "invalid JSON returns 401 (invalid token)",
			headers: map[string]string{
				"Authorization": "Bearer test-token",
				"Content-Type":  "application/json",
			},
			body:           `{invalid json}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "empty body returns 401 (invalid token)",
			headers: map[string]string{
				"Authorization": "Bearer test-token",
				"Content-Type":  "application/json",
			},
			body:           ``,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/api/v1/ai/analyze",
					Headers: tt.headers,
					Body:    []byte(tt.body),
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleRequestAIAnalysisLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleGetAIStatsLift(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    map[string]string
		expectedStatus int
	}{
		{
			name:           "no period parameter uses default",
			queryParams:    map[string]string{},
			expectedStatus: http.StatusOK, // Public endpoint - stats will be empty but OK
		},
		{
			name: "with period parameter",
			queryParams: map[string]string{
				"period": "week",
			},
			expectedStatus: http.StatusOK, // Public endpoint - stats will be empty but OK
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "GET",
					Path:    "/api/v1/ai/stats",
					Headers: map[string]string{},
				},
			}

			// For query parameters, set them in the request URL
			if period, ok := tt.queryParams["period"]; ok {
				ctx.Request.Path = ctx.Request.Path + "?period=" + period
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleGetAIStatsLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleGetAISummaryLift(t *testing.T) {
	mockStore := new(MockStorageAdapter)

	handler := &Handler{
		cfg: &config.Config{
			JWTSecret: "test-secret",
			Domain:    "test.example.com",
		},
		store:  mockStore,
		logger: zap.NewNop(),
	}

	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method:  "GET",
			Path:    "/api/v1/ai/capabilities",
			Headers: map[string]string{},
		},
	}
	ctx.Response = &lift.Response{
		Headers:    make(map[string]string),
		StatusCode: 200,
	}

	err := handler.HandleGetAISummaryLift(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	
	// Verify response structure
	// Verify response structure by checking the body exists and has content
	body, ok := ctx.Response.Body.(map[string]interface{})
	assert.True(t, ok, "Response body should be a map")
	assert.NotNil(t, body["text_analysis"])
	assert.NotNil(t, body["image_analysis"])
	assert.NotNil(t, body["ai_detection"])
	assert.NotNil(t, body["moderation_actions"])
	assert.NotNil(t, body["cost_per_analysis"])
}

func TestGetBearerTokenLift(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		expectToken string
	}{
		{
			name: "valid Bearer token",
			headers: map[string]string{
				"Authorization": "Bearer test-token-12345",
			},
			expectToken: "test-token-12345",
		},
		{
			name: "lowercase authorization header",
			headers: map[string]string{
				"authorization": "Bearer test-token-12345",
			},
			expectToken: "test-token-12345",
		},
		{
			name: "no authorization header",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			expectToken: "",
		},
		{
			name: "invalid format",
			headers: map[string]string{
				"Authorization": "InvalidFormat token",
			},
			expectToken: "",
		},
		{
			name: "empty Bearer token",
			headers: map[string]string{
				"Authorization": "Bearer",
			},
			expectToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
				},
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Headers: tt.headers,
				},
			}

			result := handler.getBearerTokenLift(ctx)
			assert.Equal(t, tt.expectToken, result)
		})
	}
}

func TestAIHandlers_JSONParsing(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "empty body returns 401 (invalid token)",
			body:           "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "null body returns 401 (invalid token)",
			body:           "null",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "valid but missing object_id returns 401 (invalid token)",
			body:           `{"force": true}`,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method: "POST",
					Path:   "/api/v1/ai/analyze",
					Headers: map[string]string{
						"Content-Type":  "application/json",
						"Authorization": "Bearer test-token",
					},
					Body: []byte(tt.body),
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleRequestAIAnalysisLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}