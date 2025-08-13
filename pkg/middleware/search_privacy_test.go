package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/auth"
)

// MockRepos implements the repository interface for testing
type MockRepos struct {
	mock.Mock
}

func (m *MockRepos) RecordSearchAnalytics(ctx context.Context, analytics *SearchAnalytics) error {
	args := m.Called(ctx, analytics)
	return args.Error(0)
}

func (m *MockRepos) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	args := m.Called(ctx, key, limit, window)
	return args.Bool(0), args.Error(1)
}

// Helper to create a test Lift context
func createTestContext(path string, queryParams map[string]string, headers map[string]string) *lift.Context {
	if queryParams == nil {
		queryParams = make(map[string]string)
	}
	if headers == nil {
		headers = make(map[string]string)
	}

	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method:      "GET",
			Path:        path,
			Headers:     headers,
			QueryParams: queryParams,
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
			Body:       []byte{},
		},
	}

	// Initialize internal storage
	ctx.Set("__init", true)
	ctx.Get("__init")

	return ctx
}

func TestSearchPrivacyMiddleware(t *testing.T) {
	// Create a test OAuth service with audit logger
	testJWTSecret := "test_jwt_secret"
	logger := zap.NewNop()
	auditLogger := auth.NewAuditLogger(nil, logger, auth.DefaultAuditConfig())
	oauthService := auth.NewOAuthService(testJWTSecret, nil, auditLogger)

	ctx := context.Background()
	// Create a valid token for testing
	validToken, _, _ := oauthService.GenerateTokens(ctx, "testuser", "test-client", "127.0.0.1", []string{auth.ScopeRead})
	validAuthHeader := "Bearer " + validToken

	// Create an invalid token with wrong signature
	wrongAuditLogger := auth.NewAuditLogger(nil, logger, auth.DefaultAuditConfig())
	wrongOAuthService := auth.NewOAuthService("wrong_secret", nil, wrongAuditLogger)
	invalidToken, _, _ := wrongOAuthService.GenerateTokens(ctx, "testuser", "test-client", "127.0.0.1", []string{auth.ScopeRead})
	invalidAuthHeader := "Bearer " + invalidToken

	// Create a token with insufficient scope (write only)
	writeOnlyToken, _, _ := oauthService.GenerateTokens(ctx, "testuser", "test-client", "127.0.0.1", []string{auth.ScopeWrite})
	writeOnlyAuthHeader := "Bearer " + writeOnlyToken

	tests := []struct {
		name           string
		path           string
		queryParams    map[string]string
		authHeader     string
		requireAuth    bool
		expectedStatus int
		expectedCtx    map[string]interface{}
	}{
		{
			name:           "non_search_endpoint_passes_through",
			path:           "/api/v1/timeline",
			queryParams:    map[string]string{},
			authHeader:     "",
			requireAuth:    false,
			expectedStatus: 200,
			expectedCtx:    map[string]interface{}{},
		},
		{
			name:           "status_search_requires_auth",
			path:           "/api/v1/statuses/search",
			queryParams:    map[string]string{"q": "test"},
			authHeader:     "",
			requireAuth:    false,
			expectedStatus: 401,
			expectedCtx:    nil,
		},
		{
			name:           "status_search_with_valid_token",
			path:           "/api/v1/statuses/search",
			queryParams:    map[string]string{"q": "test"},
			authHeader:     validAuthHeader,
			requireAuth:    false,
			expectedStatus: 200,
			expectedCtx: map[string]interface{}{
				"user_id":               "testuser",
				"authenticated":         true,
				"filter_private":        true,
				"filter_followers_only": true,
				"requesting_user":       "testuser",
			},
		},
		{
			name:           "status_search_with_invalid_token",
			path:           "/api/v1/statuses/search",
			queryParams:    map[string]string{"q": "test"},
			authHeader:     invalidAuthHeader,
			requireAuth:    false,
			expectedStatus: 401,
			expectedCtx:    nil,
		},
		{
			name:           "status_search_with_insufficient_scope",
			path:           "/api/v1/statuses/search",
			queryParams:    map[string]string{"q": "test"},
			authHeader:     writeOnlyAuthHeader,
			requireAuth:    false,
			expectedStatus: 401,
			expectedCtx:    nil,
		},
		{
			name:           "account_search_allows_unauthenticated",
			path:           "/api/v1/accounts/search",
			queryParams:    map[string]string{"q": "user"},
			authHeader:     "",
			requireAuth:    false,
			expectedStatus: 200,
			expectedCtx: map[string]interface{}{
				"public_search": true,
				"limit_results": 20,
			},
		},
		{
			name:           "account_search_with_valid_auth",
			path:           "/api/v1/accounts/search",
			queryParams:    map[string]string{"q": "user"},
			authHeader:     validAuthHeader,
			requireAuth:    false,
			expectedStatus: 200,
			expectedCtx: map[string]interface{}{
				"user_id":       "testuser",
				"authenticated": true,
			},
		},
		{
			name:           "hashtag_search_filters_nsfw_for_unauthenticated",
			path:           "/api/v1/tags/search",
			queryParams:    map[string]string{"q": "trending"},
			authHeader:     "",
			requireAuth:    false,
			expectedStatus: 200,
			expectedCtx: map[string]interface{}{
				"public_search": true,
				"filter_nsfw":   true,
			},
		},
		{
			name:           "search_with_type_parameter",
			path:           "/api/v1/search",
			queryParams:    map[string]string{"q": "test", "type": "statuses"},
			authHeader:     "",
			requireAuth:    false,
			expectedStatus: 401, // Status search requires auth
			expectedCtx:    nil,
		},
		{
			name:           "malformed_bearer_header",
			path:           "/api/v1/statuses/search",
			queryParams:    map[string]string{"q": "test"},
			authHeader:     "InvalidHeader",
			requireAuth:    false,
			expectedStatus: 401,
			expectedCtx:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRepos := new(MockRepos)
			logger := zap.NewNop()
			config := SearchPrivacyConfig{
				Repos:           mockRepos,
				OAuthService:    oauthService,
				Domain:          "test.local",
				Logger:          logger,
				RequireAuth:     tt.requireAuth,
				EnableAnalytics: false,
			}

			// Create middleware
			middleware := SearchPrivacyMiddleware(config)

			// Track if handler was called and capture context
			handlerCalled := false
			var capturedCtx *lift.Context

			handler := func(ctx *lift.Context) error {
				handlerCalled = true
				capturedCtx = ctx
				return nil
			}

			// Create request context
			headers := make(map[string]string)
			if tt.authHeader != "" {
				headers["Authorization"] = tt.authHeader
			}

			ctx := createTestContext(tt.path, tt.queryParams, headers)

			// Execute middleware
			wrappedHandler := middleware(handler)
			err := wrappedHandler(ctx)

			// Assert based on expected status
			if tt.expectedStatus == 401 {
				// For unauthorized requests, handler should not be called
				assert.NoError(t, err)
				assert.False(t, handlerCalled, "Handler should not be called for unauthorized requests")
				// Check that response has 401 status
				assert.Equal(t, 401, ctx.Response.StatusCode)
			} else {
				// For authorized requests, handler should be called
				assert.NoError(t, err)
				assert.True(t, handlerCalled, "Handler should be called for authorized requests")

				// Check expected context values
				if tt.expectedCtx != nil && capturedCtx != nil {
					for key, expectedValue := range tt.expectedCtx {
						actualValue := capturedCtx.Get(key)
						assert.Equal(t, expectedValue, actualValue, "Value mismatch for key %s", key)
					}
				}
			}
		})
	}
}

func TestNewSearchAnalyticsMiddleware(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		queryParams     map[string]string
		userID          string
		expectAnalytics bool
	}{
		{
			name:            "records_analytics_for_authenticated_search",
			path:            "/api/v1/search",
			queryParams:     map[string]string{"q": "test query"},
			userID:          "user123",
			expectAnalytics: true,
		},
		{
			name:            "skips_analytics_for_non_search",
			path:            "/api/v1/timeline",
			queryParams:     map[string]string{},
			userID:          "user123",
			expectAnalytics: false,
		},
		{
			name:            "skips_analytics_for_unauthenticated",
			path:            "/api/v1/search",
			queryParams:     map[string]string{"q": "test"},
			userID:          "",
			expectAnalytics: false,
		},
		{
			name:            "skips_analytics_for_empty_query",
			path:            "/api/v1/search",
			queryParams:     map[string]string{},
			userID:          "user123",
			expectAnalytics: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRepos := new(MockRepos)
			logger := zap.NewNop()

			if tt.expectAnalytics {
				mockRepos.On("RecordSearchAnalytics", mock.Anything, mock.AnythingOfType("*middleware.SearchAnalytics")).Return(nil)
			}

			// Create middleware
			middleware := NewSearchAnalyticsMiddleware(mockRepos, logger)

			// Create test handler
			handlerCalled := false
			handler := func(ctx *lift.Context) error {
				handlerCalled = true
				return nil
			}

			// Create request context
			ctx := createTestContext(tt.path, tt.queryParams, nil)

			// Set user ID if provided
			if tt.userID != "" {
				ctx.Set("user_id", tt.userID)
			}

			// Execute middleware
			wrappedHandler := middleware(handler)
			err := wrappedHandler(ctx)

			// Assert
			assert.NoError(t, err)
			assert.True(t, handlerCalled)

			// Wait briefly for async analytics recording
			if tt.expectAnalytics {
				time.Sleep(100 * time.Millisecond)
				mockRepos.AssertExpectations(t)
			}
		})
	}
}

func TestNewSearchRateLimitMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		queryParams    map[string]string
		userID         string
		rateLimitOK    bool
		expectedStatus int
	}{
		{
			name:           "allows_request_within_rate_limit",
			path:           "/api/v1/search",
			queryParams:    map[string]string{"q": "test"},
			userID:         "user123",
			rateLimitOK:    true,
			expectedStatus: 200,
		},
		{
			name:           "blocks_request_exceeding_rate_limit",
			path:           "/api/v1/search",
			queryParams:    map[string]string{"q": "test"},
			userID:         "user123",
			rateLimitOK:    false,
			expectedStatus: 429,
		},
		{
			name:           "skips_rate_limit_for_non_search",
			path:           "/api/v1/timeline",
			queryParams:    map[string]string{},
			userID:         "user123",
			rateLimitOK:    true,
			expectedStatus: 200,
		},
		{
			name:           "uses_ip_for_unauthenticated",
			path:           "/api/v1/search",
			queryParams:    map[string]string{"q": "test"},
			userID:         "",
			rateLimitOK:    true,
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRepos := new(MockRepos)
			logger := zap.NewNop()
			requestsPerMinute := 60

			// Set up rate limit expectations
			if tt.path == "/api/v1/search" {
				key := "search:"
				if tt.userID != "" {
					key += tt.userID
				} else {
					key += "" // RemoteAddr() returns empty in test context
				}
				mockRepos.On("CheckRateLimit", mock.Anything, key, requestsPerMinute, time.Minute).Return(tt.rateLimitOK, nil)
			}

			// Create middleware
			middleware := NewSearchRateLimitMiddleware(mockRepos, logger, requestsPerMinute)

			// Track if handler was called
			handlerCalled := false
			handler := func(ctx *lift.Context) error {
				handlerCalled = true
				return nil
			}

			// Create request context
			ctx := createTestContext(tt.path, tt.queryParams, nil)

			// Set user ID if provided
			if tt.userID != "" {
				ctx.Set("user_id", tt.userID)
			}

			// Execute middleware
			wrappedHandler := middleware(handler)
			err := wrappedHandler(ctx)

			// Assert
			assert.NoError(t, err)

			if tt.expectedStatus == 429 {
				assert.False(t, handlerCalled, "Handler should not be called when rate limited")
				assert.Equal(t, 429, ctx.Response.StatusCode)
			} else {
				assert.True(t, handlerCalled, "Handler should be called when not rate limited")
			}

			mockRepos.AssertExpectations(t)
		})
	}
}

// Test helper functions
func TestIsSearchEndpoint(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/api/v1/search", true},
		{"/api/v2/search", true},
		{"/api/v1/accounts/search", true},
		{"/api/v1/statuses/search", true},
		{"/api/v1/tags/search", true},
		{"/api/v1/timeline", false},
		{"/api/v1/notifications", false},
		{"/search", false}, // Must have full path
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isSearchEndpoint(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectSearchType(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/api/v1/statuses/search", "statuses"},
		{"/api/v1/accounts/search", "accounts"},
		{"/api/v1/tags/search", "hashtags"},
		{"/api/v1/search", "all"},
		{"/api/v1/timeline", "all"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := detectSearchType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
