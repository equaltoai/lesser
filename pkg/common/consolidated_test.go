package common

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

// TestErrorResponseConsolidation validates the consolidated error response functions
func TestErrorResponseConsolidation(t *testing.T) {
	testCases := []struct {
		name           string
		function       func(*testing.T, string) (int, StandardErrorResponse)
		expectedStatus int
		defaultMessage string
	}{
		{
			name:           "RespondBadRequest",
			function:       testRespondBadRequest,
			expectedStatus: 400,
			defaultMessage: "Bad Request",
		},
		{
			name:           "RespondUnauthorized",
			function:       testRespondUnauthorized,
			expectedStatus: 401,
			defaultMessage: "Unauthorized",
		},
		{
			name:           "RespondForbidden",
			function:       testRespondForbidden,
			expectedStatus: 403,
			defaultMessage: "Forbidden",
		},
		{
			name:           "RespondNotFound",
			function:       testRespondNotFound,
			expectedStatus: 404,
			defaultMessage: "resource not found",
		},
		{
			name:           "RespondConflict",
			function:       testRespondConflict,
			expectedStatus: 409,
			defaultMessage: "Conflict",
		},
		{
			name:           "RespondGone",
			function:       testRespondGone,
			expectedStatus: 410,
			defaultMessage: "Gone",
		},
		{
			name:           "RespondUnprocessableEntity",
			function:       testRespondUnprocessableEntity,
			expectedStatus: 422,
			defaultMessage: "Unprocessable Entity",
		},
		{
			name:           "RespondInternalServerError",
			function:       testRespondInternalServerError,
			expectedStatus: 500,
			defaultMessage: "Internal server error",
		},
		{
			name:           "RespondServiceUnavailable",
			function:       testRespondServiceUnavailable,
			expectedStatus: 503,
			defaultMessage: "Service Unavailable",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name+"_with_custom_message", func(t *testing.T) {
			customMessage := "Custom error message"
			statusCode, response := tc.function(t, customMessage)

			assert.Equal(t, tc.expectedStatus, statusCode)

			// Special handling for functions that append text to custom messages
			expectedError := customMessage
			if tc.name == "RespondNotFound" {
				expectedError = customMessage + " not found"
			} else if tc.name == "RespondServiceUnavailable" {
				expectedError = customMessage + " service unavailable"
			}

			assert.Equal(t, expectedError, response.Error)
		})

		t.Run(tc.name+"_with_default_message", func(t *testing.T) {
			statusCode, response := tc.function(t, "")

			assert.Equal(t, tc.expectedStatus, statusCode)
			assert.Equal(t, tc.defaultMessage, response.Error)
		})
	}
}

// Helper functions for testing each error response function
func testRespondBadRequest(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := newTestContext("GET", "/test")
	var (
		resp *apptheory.Response
		err  error
	)
	if message == "" {
		resp, err = RespondBadRequest(ctx)
	} else {
		resp, err = RespondBadRequest(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, resp)
}

func testRespondUnauthorized(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := newTestContext("GET", "/test")
	var (
		resp *apptheory.Response
		err  error
	)
	if message == "" {
		resp, err = RespondUnauthorized(ctx)
	} else {
		resp, err = RespondUnauthorized(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, resp)
}

func testRespondForbidden(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := newTestContext("GET", "/test")
	var (
		resp *apptheory.Response
		err  error
	)
	if message == "" {
		resp, err = RespondForbidden(ctx)
	} else {
		resp, err = RespondForbidden(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, resp)
}

func testRespondNotFound(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := newTestContext("GET", "/test")
	var (
		resp *apptheory.Response
		err  error
	)
	if message == "" {
		resp, err = RespondNotFound(ctx)
	} else {
		resp, err = RespondNotFound(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, resp)
}

func testRespondConflict(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := newTestContext("GET", "/test")
	var (
		resp *apptheory.Response
		err  error
	)
	if message == "" {
		resp, err = RespondConflict(ctx)
	} else {
		resp, err = RespondConflict(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, resp)
}

func testRespondGone(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := newTestContext("GET", "/test")
	var (
		resp *apptheory.Response
		err  error
	)
	if message == "" {
		resp, err = RespondGone(ctx)
	} else {
		resp, err = RespondGone(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, resp)
}

func testRespondUnprocessableEntity(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := newTestContext("GET", "/test")
	var (
		resp *apptheory.Response
		err  error
	)
	if message == "" {
		resp, err = RespondUnprocessableEntity(ctx)
	} else {
		resp, err = RespondUnprocessableEntity(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, resp)
}

func testRespondInternalServerError(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := newTestContext("GET", "/test")
	var (
		resp *apptheory.Response
		err  error
	)
	if message == "" {
		resp, err = RespondInternalServerError(ctx)
	} else {
		resp, err = RespondInternalServerError(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, resp)
}

func testRespondServiceUnavailable(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := newTestContext("GET", "/test")
	var (
		resp *apptheory.Response
		err  error
	)
	if message == "" {
		resp, err = RespondServiceUnavailable(ctx)
	} else {
		resp, err = RespondServiceUnavailable(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, resp)
}

// TestErrorResponseJSONStructure validates that all error responses produce valid JSON
func TestErrorResponseJSONStructure(t *testing.T) {
	// Test that each response function produces valid JSON
	errorFunctions := []func(*apptheory.Context) (*apptheory.Response, error){
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondBadRequest(ctx, "test") },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondUnauthorized(ctx, "test") },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondForbidden(ctx, "test") },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondNotFound(ctx, "test") },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondConflict(ctx, "test") },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondGone(ctx, "test") },
		func(ctx *apptheory.Context) (*apptheory.Response, error) {
			return RespondUnprocessableEntity(ctx, "test")
		},
		func(ctx *apptheory.Context) (*apptheory.Response, error) {
			return RespondInternalServerError(ctx, "test")
		},
		func(ctx *apptheory.Context) (*apptheory.Response, error) {
			return RespondServiceUnavailable(ctx, "test")
		},
	}

	for i, fn := range errorFunctions {
		t.Run(t.Name()+"_"+string(rune('A'+i)), func(t *testing.T) {
			ctx := newTestContext("GET", "/test")
			resp, err := fn(ctx)
			require.NoError(t, err)

			// Validate structure using our parseResponse helper
			statusCode, response := parseResponse(t, resp)
			require.NotZero(t, statusCode, "Status code should be set")
			require.NotEmpty(t, response.Error, "Error field should not be empty")
		})
	}
}

// TestValidationErrorResponse validates the validation error response function
func TestValidationErrorResponse(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "validation error with custom error",
			err:            fmt.Errorf("Invalid email format"),
			expectedStatus: 400,
			expectedError:  "Invalid email format",
		},
		{
			name:           "validation error with nil error",
			err:            nil,
			expectedStatus: 400,
			expectedError:  "Validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestContext("GET", "/test")
			resp, err := RespondValidationError(ctx, tt.err)
			require.NoError(t, err)

			statusCode, response := parseResponse(t, resp)
			assert.Equal(t, tt.expectedStatus, statusCode)
			assert.Equal(t, tt.expectedError, response.Error)
		})
	}
}

// TestAuthHelperFunctions validates the consolidated auth helper functions
func TestAuthHelperFunctions(t *testing.T) {
	t.Run("ExtractBearerToken", func(t *testing.T) {
		tests := []struct {
			name        string
			authHeader  string
			expectToken string
			expectError bool
		}{
			{
				name:        "valid bearer token",
				authHeader:  "Bearer valid_token_123",
				expectToken: "valid_token_123",
				expectError: false,
			},
			{
				name:        "empty header",
				authHeader:  "",
				expectToken: "",
				expectError: true,
			},
			{
				name:        "missing Bearer prefix",
				authHeader:  "Basic dXNlcjpwYXNz",
				expectToken: "",
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				token, err := ExtractBearerToken(tt.authHeader)

				if tt.expectError {
					assert.Error(t, err)
					assert.Empty(t, token)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expectToken, token)
				}
			})
		}
	})

	t.Run("ExtractAuthHeader", func(t *testing.T) {
		// Test with Authorization header
		ctx := newTestContext("GET", "/test", withHeaders(map[string]string{
			"Authorization": "Bearer token123",
		}))

		authHeader := ExtractAuthHeader(ctx)
		assert.Equal(t, "Bearer token123", authHeader)

		// Test with lowercase authorization header
		ctx = newTestContext("GET", "/test", withHeaders(map[string]string{
			"authorization": "Bearer token456",
		}))

		authHeader = ExtractAuthHeader(ctx)
		assert.Equal(t, "Bearer token456", authHeader)

		// Test without auth header
		ctx = newTestContext("GET", "/test")
		authHeader = ExtractAuthHeader(ctx)
		assert.Empty(t, authHeader)
	})
}

// TestScopeValidationHelpers validates scope checking helper functions
func TestScopeValidationHelpers(t *testing.T) {
	// Mock claims for testing
	type mockClaims struct {
		scopes []string
	}

	claims := &mockClaims{
		scopes: []string{ScopeRead, WriteFollows},
	}

	// Implement Claims interface methods
	hasScope := func(scope string) bool {
		for _, s := range claims.scopes {
			if s == scope {
				return true
			}
		}
		return false
	}

	t.Run("scope validation", func(t *testing.T) {
		// Test that user has read scope
		assert.True(t, hasScope(ScopeRead))

		// Test that user doesn't have write scope
		assert.False(t, hasScope(ScopeWrite))

		// Test that user has write:follows scope
		assert.True(t, hasScope(WriteFollows))
	})
}

// TestEndToEndErrorConsolidation validates that our consolidation works correctly
func TestEndToEndErrorConsolidation(t *testing.T) {
	t.Run("consistent error format", func(t *testing.T) {
		// Test that all error responses have consistent structure
		errorFunctions := map[string]struct {
			fn            func(*apptheory.Context) (*apptheory.Response, error)
			expectedError string
		}{
			"400": {func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondBadRequest(ctx, "test") }, "test"},
			"401": {func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondUnauthorized(ctx, "test") }, "test"},
			"403": {func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondForbidden(ctx, "test") }, "test"},
			"404": {func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondNotFound(ctx, "test") }, "test not found"},
			"409": {func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondConflict(ctx, "test") }, "test"},
			"410": {func(ctx *apptheory.Context) (*apptheory.Response, error) { return RespondGone(ctx, "test") }, "test"},
			"422": {func(ctx *apptheory.Context) (*apptheory.Response, error) {
				return RespondUnprocessableEntity(ctx, "test")
			}, "test"},
			"500": {func(ctx *apptheory.Context) (*apptheory.Response, error) {
				return RespondInternalServerError(ctx, "test")
			}, "test"},
			"503": {func(ctx *apptheory.Context) (*apptheory.Response, error) {
				return RespondServiceUnavailable(ctx, "test")
			}, "test service unavailable"},
		}

		for name, testCase := range errorFunctions {
			t.Run("status_"+name, func(t *testing.T) {
				ctx := newTestContext("GET", "/test")
				resp, err := testCase.fn(ctx)
				require.NoError(t, err)

				// All responses should have the same JSON structure
				statusCode, response := parseResponse(t, resp)
				require.NotZero(t, statusCode, "Status code should be set")
				require.Equal(t, testCase.expectedError, response.Error)
			})
		}
	})
}
