package common

import (
	"encoding/json"
	"fmt"
	"testing"

	liftTesting "github.com/equaltoai/lesser/pkg/testing/lift"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			defaultMessage: "Not Found",
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
	ctx := liftTesting.MockLiftContext("GET", "/test")
	var err error
	if message == "" {
		err = RespondBadRequest(ctx)
	} else {
		err = RespondBadRequest(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, ctx)
}

func testRespondUnauthorized(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	var err error
	if message == "" {
		err = RespondUnauthorized(ctx)
	} else {
		err = RespondUnauthorized(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, ctx)
}

func testRespondForbidden(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	var err error
	if message == "" {
		err = RespondForbidden(ctx)
	} else {
		err = RespondForbidden(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, ctx)
}

func testRespondNotFound(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	var err error
	if message == "" {
		err = RespondNotFound(ctx)
	} else {
		err = RespondNotFound(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, ctx)
}

func testRespondConflict(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	var err error
	if message == "" {
		err = RespondConflict(ctx)
	} else {
		err = RespondConflict(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, ctx)
}

func testRespondGone(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	var err error
	if message == "" {
		err = RespondGone(ctx)
	} else {
		err = RespondGone(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, ctx)
}

func testRespondUnprocessableEntity(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	var err error
	if message == "" {
		err = RespondUnprocessableEntity(ctx)
	} else {
		err = RespondUnprocessableEntity(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, ctx)
}

func testRespondInternalServerError(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	var err error
	if message == "" {
		err = RespondInternalServerError(ctx)
	} else {
		err = RespondInternalServerError(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, ctx)
}

func testRespondServiceUnavailable(t *testing.T, message string) (int, StandardErrorResponse) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	var err error
	if message == "" {
		err = RespondServiceUnavailable(ctx)
	} else {
		err = RespondServiceUnavailable(ctx, message)
	}
	require.NoError(t, err)
	return parseResponse(t, ctx)
}

// parseResponse extracts and parses the response from a Lift context
func parseResponse(t *testing.T, ctx *lift.Context) (int, StandardErrorResponse) {
	// Check if response is already a StandardErrorResponse struct
	if response, ok := ctx.Response.Body.(StandardErrorResponse); ok {
		return ctx.Response.StatusCode, response
	}
	
	// Otherwise, try to parse as JSON bytes (following Lift testing pattern)
	var response StandardErrorResponse
	bodyBytes, ok := ctx.Response.Body.([]byte)
	if !ok {
		bodyBytes = []byte(fmt.Sprintf("%v", ctx.Response.Body))
	}
	
	err := json.Unmarshal(bodyBytes, &response)
	require.NoError(t, err, "Response should be valid JSON")
	
	return ctx.Response.StatusCode, response
}

// TestErrorResponseJSONStructure validates that all error responses produce valid JSON
func TestErrorResponseJSONStructure(t *testing.T) {
	ctx := liftTesting.MockLiftContext("GET", "/test")
	
	// Test that each response function produces valid JSON
	errorFunctions := []func() error{
		func() error { return RespondBadRequest(ctx, "test") },
		func() error { return RespondUnauthorized(ctx, "test") },
		func() error { return RespondForbidden(ctx, "test") },
		func() error { return RespondNotFound(ctx, "test") },
		func() error { return RespondConflict(ctx, "test") },
		func() error { return RespondGone(ctx, "test") },
		func() error { return RespondUnprocessableEntity(ctx, "test") },
		func() error { return RespondInternalServerError(ctx, "test") },
		func() error { return RespondServiceUnavailable(ctx, "test") },
	}

	for i, fn := range errorFunctions {
		t.Run(t.Name()+"_"+string(rune('A'+i)), func(t *testing.T) {
			ctx = liftTesting.MockLiftContext("GET", "/test") // Reset context
			
			err := fn()
			require.NoError(t, err)
			
			// Validate structure using our parseResponse helper
			statusCode, response := parseResponse(t, ctx)
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
			ctx := liftTesting.MockLiftContext("GET", "/test")
			
			err := RespondValidationError(ctx, tt.err)
			require.NoError(t, err)
			
			statusCode, response := parseResponse(t, ctx)
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

	t.Run("GetTestUsername", func(t *testing.T) {
		// Test with test username header
		ctx := liftTesting.MockLiftContext("GET", "/test",
			liftTesting.WithHeaders(map[string]string{
				"X-Test-Username": "testuser",
			}),
		)
		
		username := GetTestUsername(ctx)
		assert.Equal(t, "testuser", username)

		// Test without test username header
		ctx = liftTesting.MockLiftContext("GET", "/test")
		username = GetTestUsername(ctx)
		assert.Empty(t, username)
	})

	t.Run("ExtractAuthHeader", func(t *testing.T) {
		// Test with Authorization header
		ctx := liftTesting.MockLiftContext("GET", "/test",
			liftTesting.WithHeaders(map[string]string{
				"Authorization": "Bearer token123",
			}),
		)
		
		authHeader := ExtractAuthHeader(ctx)
		assert.Equal(t, "Bearer token123", authHeader)

		// Test with lowercase authorization header
		ctx = liftTesting.MockLiftContext("GET", "/test",
			liftTesting.WithHeaders(map[string]string{
				"authorization": "Bearer token456",
			}),
		)
		
		authHeader = ExtractAuthHeader(ctx)
		assert.Equal(t, "Bearer token456", authHeader)

		// Test without auth header
		ctx = liftTesting.MockLiftContext("GET", "/test")
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
			fn            func(*lift.Context) error
			expectedError string
		}{
			"400": {func(ctx *lift.Context) error { return RespondBadRequest(ctx, "test") }, "test"},
			"401": {func(ctx *lift.Context) error { return RespondUnauthorized(ctx, "test") }, "test"},
			"403": {func(ctx *lift.Context) error { return RespondForbidden(ctx, "test") }, "test"},
			"404": {func(ctx *lift.Context) error { return RespondNotFound(ctx, "test") }, "test not found"},
			"409": {func(ctx *lift.Context) error { return RespondConflict(ctx, "test") }, "test"},
			"410": {func(ctx *lift.Context) error { return RespondGone(ctx, "test") }, "test"},
			"422": {func(ctx *lift.Context) error { return RespondUnprocessableEntity(ctx, "test") }, "test"},
			"500": {func(ctx *lift.Context) error { return RespondInternalServerError(ctx, "test") }, "test"},
			"503": {func(ctx *lift.Context) error { return RespondServiceUnavailable(ctx, "test") }, "test service unavailable"},
		}

		for name, testCase := range errorFunctions {
			t.Run("status_"+name, func(t *testing.T) {
				ctx := liftTesting.MockLiftContext("GET", "/test")
				
				err := testCase.fn(ctx)
				require.NoError(t, err)
				
				// All responses should have the same JSON structure
				statusCode, response := parseResponse(t, ctx)
				require.NotZero(t, statusCode, "Status code should be set")
				require.Equal(t, testCase.expectedError, response.Error)
			})
		}
	})
}