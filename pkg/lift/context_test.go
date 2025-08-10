package lift

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
)

// Helper functions for creating test contexts (following auth_test.go patterns)

func createTestContextWithQuery(queryParams map[string]string) *lift.Context {
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method:      "GET",
			Path:        "/test",
			Headers:     make(map[string]string),
			QueryParams: queryParams,
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}
	// Initialize internal maps that Lift uses
	ctx.Set("__test", "init") // This initializes the internal storage
	ctx.Get("__test")         // Clean up the test key

	return ctx
}

func createTestContextWithHeaders(headers map[string]string) *lift.Context {
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method:  "GET",
			Path:    "/test",
			Headers: headers,
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}
	// Initialize internal maps that Lift uses
	ctx.Set("__test", "init") // This initializes the internal storage
	ctx.Get("__test")         // Clean up the test key

	return ctx
}

func TestGetPaginationParams(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    map[string]string
		expectedLimit  int
		expectedOffset int
	}{
		{
			name:           "default values",
			queryParams:    map[string]string{},
			expectedLimit:  20,
			expectedOffset: 0,
		},
		{
			name: "custom limit and offset",
			queryParams: map[string]string{
				"limit":  "50",
				"offset": "100",
			},
			expectedLimit:  50,
			expectedOffset: 100,
		},
		{
			name: "page-based pagination",
			queryParams: map[string]string{
				"limit": "10",
				"page":  "3",
			},
			expectedLimit:  10,
			expectedOffset: 20, // (3-1) * 10
		},
		{
			name: "invalid limit uses default",
			queryParams: map[string]string{
				"limit": "invalid",
			},
			expectedLimit:  20,
			expectedOffset: 0,
		},
		{
			name: "limit too high uses default",
			queryParams: map[string]string{
				"limit": "200",
			},
			expectedLimit:  20,
			expectedOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := createTestContextWithQuery(tt.queryParams)

			pagination := GetPaginationParams(ctx)
			assert.Equal(t, tt.expectedLimit, pagination.Limit)
			assert.Equal(t, tt.expectedOffset, pagination.Offset)
		})
	}
}

func TestRespondWithPagination(t *testing.T) {
	ctx := createTestContext()

	data := []string{"item1", "item2", "item3"}
	pagination := Pagination{
		Limit:  10,
		Offset: 0,
		Total:  3,
	}

	err := RespondWithPagination(ctx, data, pagination)
	assert.NoError(t, err)
}

func TestRespondWithSuccess(t *testing.T) {
	data := map[string]string{"key": "value"}

	// Test without message
	ctx1 := createTestContext()
	err := RespondWithSuccess(ctx1, data)
	assert.NoError(t, err)

	// Test with message
	ctx2 := createTestContext()
	err = RespondWithSuccess(ctx2, data, "Operation successful")
	assert.NoError(t, err)
}

func TestRespondWithError(t *testing.T) {
	// Test without code
	ctx1 := createTestContext()
	err := RespondWithError(ctx1, 400, "Bad request")
	assert.NoError(t, err)

	// Test with code
	ctx2 := createTestContext()
	err = RespondWithError(ctx2, 400, "Bad request", "INVALID_INPUT")
	assert.NoError(t, err)
}

func TestGetRequestID(t *testing.T) {
	ctx := createTestContext()

	// Test that GetRequestID doesn't panic (actual request ID would be set by Lift runtime)
	assert.NotPanics(t, func() {
		GetRequestID(ctx)
	})
}

func TestGetUserAgent(t *testing.T) {
	userAgent := "Mozilla/5.0 (Test Browser)"
	ctx := createTestContextWithHeaders(map[string]string{
		"User-Agent": userAgent,
	})

	result := GetUserAgent(ctx)
	assert.Equal(t, userAgent, result)
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		expectedIP string
	}{
		{
			name: "X-Forwarded-For header",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.1",
			},
			expectedIP: "192.168.1.1",
		},
		{
			name: "X-Real-IP header",
			headers: map[string]string{
				"X-Real-IP": "10.0.0.1",
			},
			expectedIP: "10.0.0.1",
		},
		{
			name:       "no IP headers",
			headers:    map[string]string{},
			expectedIP: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := createTestContextWithHeaders(tt.headers)

			ip := GetClientIP(ctx)
			assert.Equal(t, tt.expectedIP, ip)
		})
	}
}

func TestGetQueryParam(t *testing.T) {
	ctx := createTestContextWithQuery(map[string]string{
		"param1": "value1",
	})

	// Test existing parameter
	value := GetQueryParam(ctx, "param1")
	assert.Equal(t, "value1", value)

	// Test missing parameter without default
	value = GetQueryParam(ctx, "missing")
	assert.Equal(t, "", value)

	// Test missing parameter with default
	value = GetQueryParam(ctx, "missing", "default")
	assert.Equal(t, "default", value)
}

func TestGetQueryParamInt(t *testing.T) {
	ctx := createTestContextWithQuery(map[string]string{
		"number":  "42",
		"invalid": "not-a-number",
	})

	// Test valid integer
	value := GetQueryParamInt(ctx, "number")
	assert.Equal(t, 42, value)

	// Test invalid integer without default
	value = GetQueryParamInt(ctx, "invalid")
	assert.Equal(t, 0, value)

	// Test invalid integer with default
	value = GetQueryParamInt(ctx, "invalid", 100)
	assert.Equal(t, 100, value)

	// Test missing parameter with default
	value = GetQueryParamInt(ctx, "missing", 200)
	assert.Equal(t, 200, value)
}

func TestGetQueryParamBool(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true", "true", true},
		{"1", "1", true},
		{"yes", "yes", true},
		{"false", "false", false},
		{"0", "0", false},
		{"no", "no", false},
		{"empty", "", false},
		{"invalid", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queryParams := map[string]string{}
			if tt.value != "" {
				queryParams["param"] = tt.value
			}

			ctx := createTestContextWithQuery(queryParams)

			result := GetQueryParamBool(ctx, "param")
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPathParam(t *testing.T) {
	ctx := createTestContext()

	// Test that GetPathParam doesn't panic (actual path param testing would require more complex setup)
	assert.NotPanics(t, func() {
		GetPathParam(ctx, "id")
	})
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"user@example.com", true},
		{"test.email@domain.co.uk", true},
		{"invalid-email", false},
		{"@example.com", false},
		{"user@", false},
		{"", false},
		{"a@b.c", true},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			result := ValidateEmail(tt.email)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestAuthenticationHelpers(t *testing.T) {
	ctx := createTestContext()

	// Test without authentication
	assert.False(t, IsUserAuthenticated(ctx))
	assert.Equal(t, "", GetOptionalAuthenticatedUsername(ctx))
	assert.False(t, CheckUserScope(ctx, "read"))

	// Test with authentication
	claims := &auth.EnhancedClaims{
		Claims: auth.Claims{
			Username: "testuser",
			Scopes:   []string{"read", "write"},
		},
		SessionID: "session123",
		DeviceID:  "device456",
	}
	ctx.Set("claims", claims)
	ctx.Set("username", "testuser")
	ctx.Set("session_id", "session123")

	assert.True(t, IsUserAuthenticated(ctx))
	assert.Equal(t, "testuser", GetOptionalAuthenticatedUsername(ctx))
	assert.True(t, CheckUserScope(ctx, "read"))
	assert.False(t, CheckUserScope(ctx, "admin"))

	// Test authenticated user retrieval
	retrievedClaims, err := GetAuthenticatedUser(ctx)
	assert.NoError(t, err)
	assert.Equal(t, claims, retrievedClaims)

	username, err := GetAuthenticatedUsername(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "testuser", username)

	sessionID, err := GetCurrentSession(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "session123", sessionID)
}

func TestHeaderHelpers(t *testing.T) {
	ctx := createTestContext()

	// Test setting cache headers
	SetCacheHeaders(ctx, 3600)

	// Test setting no-cache headers
	SetNoCacheHeaders(ctx)

	// Test setting security headers
	SetSecurityHeaders(ctx)

	// These functions should not panic
	assert.NotPanics(t, func() {
		SetCacheHeaders(ctx, 3600)
		SetNoCacheHeaders(ctx)
		SetSecurityHeaders(ctx)
	})
}

func TestValidateRequired(t *testing.T) {
	ctx := createTestContext()

	// Test with all required fields present
	fields := map[string]string{
		"name":  "John",
		"email": "john@example.com",
	}
	err := ValidateRequired(ctx, fields)
	assert.NoError(t, err)

	// Test with missing required field
	fieldsWithMissing := map[string]string{
		"name":  "John",
		"email": "",
	}
	err = ValidateRequired(ctx, fieldsWithMissing)
	assert.Error(t, err)
}

func TestGetContentType(t *testing.T) {
	contentType := "application/json"
	ctx := createTestContextWithHeaders(map[string]string{
		"Content-Type": contentType,
	})

	result := GetContentType(ctx)
	assert.Equal(t, contentType, result)
}

func TestGetAcceptHeader(t *testing.T) {
	accept := "application/json, text/plain"
	ctx := createTestContextWithHeaders(map[string]string{
		"Accept": accept,
	})

	result := GetAcceptHeader(ctx)
	assert.Equal(t, accept, result)
}
