package lift

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetInstanceMetricsLift(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		checkResponse  func(*testing.T, interface{})
	}{
		{
			name: "successful metrics retrieval",
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetActiveUserCount", mock.Anything, 30).Return(int64(100), nil)
				m.On("GetActiveUserCount", mock.Anything, 1).Return(int64(50), nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				metrics, ok := resp.(map[string]any)
				assert.True(t, ok)
				
				current, ok := metrics["current"].(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, int64(100), current["active_users"])
				assert.NotNil(t, current["requests_per_minute"])
				assert.NotNil(t, current["avg_latency_ms"])
				assert.NotNil(t, current["timestamp"])
				
				system, ok := metrics["system"].(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "2.0.0", system["version"])
				assert.Equal(t, 30, system["uptime_days"])
				assert.Equal(t, "us-east-1", system["region"])
			},
		},
		{
			name: "handles storage errors gracefully",
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetActiveUserCount", mock.Anything, 30).Return(int64(0), assert.AnError)
				m.On("GetActiveUserCount", mock.Anything, 1).Return(int64(0), assert.AnError)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				metrics, ok := resp.(map[string]any)
				assert.True(t, ok)
				
				current, ok := metrics["current"].(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, int64(0), current["active_users"])
				assert.Equal(t, 0.0, current["requests_per_minute"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			tt.setupMocks(mockStore)

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
					Region:    "us-east-1",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method: "GET",
					Path:   "/api/v1/metrics/instance",
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleGetInstanceMetricsLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx.Response.Body)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetDailyAggregatesLift(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		checkResponse  func(*testing.T, interface{})
	}{
		{
			name:        "default 7 days",
			queryParams: "",
			setupMocks:  func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				response, ok := resp.(map[string]any)
				assert.True(t, ok)
				
				period, ok := response["period"].(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, 7, period["days"])
				
				aggregates, ok := response["daily_aggregates"].([]map[string]any)
				assert.True(t, ok)
				assert.Equal(t, 7, len(aggregates))
			},
		},
		{
			name:        "custom days parameter",
			queryParams: "?days=14",
			setupMocks:  func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				response, ok := resp.(map[string]any)
				assert.True(t, ok)
				
				period, ok := response["period"].(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, 14, period["days"])
				
				aggregates, ok := response["daily_aggregates"].([]map[string]any)
				assert.True(t, ok)
				assert.Equal(t, 14, len(aggregates))
			},
		},
		{
			name:        "caps at 30 days",
			queryParams: "?days=50",
			setupMocks:  func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				response, ok := resp.(map[string]any)
				assert.True(t, ok)
				
				period, ok := response["period"].(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, 30, period["days"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			tt.setupMocks(mockStore)

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
					Region:    "us-east-1",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method: "GET",
					Path:   "/api/v1/metrics/daily",
					QueryParams: make(map[string]string),
				},
			}
			
			// Parse query params from test case
			if tt.queryParams != "" && strings.HasPrefix(tt.queryParams, "?") {
				params := strings.TrimPrefix(tt.queryParams, "?")
				for _, param := range strings.Split(params, "&") {
					kv := strings.Split(param, "=")
					if len(kv) == 2 {
						ctx.Request.QueryParams[kv[0]] = kv[1]
					}
				}
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleGetDailyAggregatesLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx.Response.Body)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetPredictiveAnalyticsLift(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		checkResponse  func(*testing.T, interface{})
	}{
		{
			name: "successful predictive analytics",
			setupMocks: func(m *MockStorageAdapter) {
				// For calculateStorageProjectionLift
				m.On("GetStorageUsage", mock.Anything).Return(5.0, nil)
				m.On("GetStorageHistory", mock.Anything, 60).Return([]any{
					map[string]any{"UsageGB": 4.0},
					map[string]any{"UsageGB": 5.0},
				}, nil)
				// For calculateUserProjectionLift
				m.On("GetActiveUserCount", mock.Anything, 30).Return(int64(100), nil)
				m.On("GetUserGrowthHistory", mock.Anything, 60).Return([]any{
					map[string]any{"NewRegistrations": 10},
					map[string]any{"NewRegistrations": 15},
				}, nil)
				m.On("GetTotalUserCount", mock.Anything).Return(int64(500), nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				analytics, ok := resp.(map[string]any)
				assert.True(t, ok)
				
				projections, ok := analytics["projections"].(map[string]any)
				assert.True(t, ok)
				assert.NotNil(t, projections["monthly_cost"])
				assert.NotNil(t, projections["storage_growth"])
				assert.NotNil(t, projections["user_growth"])
				
				recommendations, ok := analytics["recommendations"].([]map[string]any)
				assert.True(t, ok)
				assert.Len(t, recommendations, 2)
				
				assert.NotNil(t, analytics["generated_at"])
			},
		},
		{
			name: "handles missing storage history",
			setupMocks: func(m *MockStorageAdapter) {
				// For calculateStorageProjectionLift - will use fallback
				m.On("GetStorageUsage", mock.Anything).Return(nil, assert.AnError)
				m.On("GetActiveUserCount", mock.Anything, 30).Return(int64(100), nil).Maybe()
				m.On("GetStorageHistory", mock.Anything, 60).Return([]any{}, assert.AnError)
				// For calculateUserProjectionLift
				m.On("GetUserGrowthHistory", mock.Anything, 60).Return([]any{}, assert.AnError)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				analytics, ok := resp.(map[string]any)
				assert.True(t, ok)
				
				// Should still return analytics with values
				projections, ok := analytics["projections"].(map[string]any)
				assert.True(t, ok)
				assert.NotNil(t, projections["monthly_cost"])
				assert.NotNil(t, projections["storage_growth"])
				assert.NotNil(t, projections["user_growth"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			tt.setupMocks(mockStore)

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
					Region:    "us-east-1",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method: "GET",
					Path:   "/api/v1/metrics/predictive",
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleGetPredictiveAnalyticsLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx.Response.Body)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestMetricsHelpers(t *testing.T) {
	t.Run("calculateRequestRateLift", func(t *testing.T) {
		mockStore := new(MockStorageAdapter)
		mockStore.On("GetActiveUserCount", mock.Anything, 1).Return(int64(10), nil)

		handler := &Handler{
			cfg: &config.Config{
				Region: "us-east-1",
			},
			store:  mockStore,
			logger: zap.NewNop(),
		}

		rate := handler.calculateRequestRateLift(context.Background())
		// 10 users * 10 requests/hour / 60 minutes
		assert.InDelta(t, 1.67, rate, 0.01)

		mockStore.AssertExpectations(t)
	})

	t.Run("calculateStorageProjectionLift", func(t *testing.T) {
		mockStore := new(MockStorageAdapter)
		mockStore.On("GetStorageUsage", mock.Anything).Return(10.0, nil)
		mockStore.On("GetStorageHistory", mock.Anything, 60).Return([]any{
			map[string]any{"UsageGB": 9.0},
			map[string]any{"UsageGB": 10.0},
		}, nil)

		handler := &Handler{
			cfg:    &config.Config{},
			store:  mockStore,
			logger: zap.NewNop(),
		}

		projection := handler.calculateStorageProjectionLift(context.Background(), 30)
		assert.Greater(t, projection, 10.0) // Should be growing

		mockStore.AssertExpectations(t)
	})

	t.Run("calculateUserProjectionLift", func(t *testing.T) {
		mockStore := new(MockStorageAdapter)
		mockStore.On("GetActiveUserCount", mock.Anything, 30).Return(int64(100), nil)
		mockStore.On("GetUserGrowthHistory", mock.Anything, 60).Return([]any{
			map[string]any{"NewRegistrations": 5},
			map[string]any{"NewRegistrations": 10},
		}, nil)
		mockStore.On("GetTotalUserCount", mock.Anything).Return(int64(100), nil)

		handler := &Handler{
			cfg:    &config.Config{},
			store:  mockStore,
			logger: zap.NewNop(),
		}

		projection := handler.calculateUserProjectionLift(context.Background(), 30)
		assert.Greater(t, projection, 100) // Should be growing

		mockStore.AssertExpectations(t)
	})
}

func TestCostStorageIntegration(t *testing.T) {
	t.Run("getCostStorageLift without env var", func(t *testing.T) {
		handler := &Handler{
			cfg:    &config.Config{},
			logger: zap.NewNop(),
		}

		storage := handler.getCostStorageLift()
		assert.Nil(t, storage)
	})
}

