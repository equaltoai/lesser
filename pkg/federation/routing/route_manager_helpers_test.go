package routing

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// === isRetryableError Tests ===

func TestIsRetryableError(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a minimal Manager for testing the method
	// The method only uses the status code, doesn't need other dependencies
	m := &Manager{
		logger: logger,
	}

	t.Run("retryable_5xx_errors", func(t *testing.T) {
		retryableCodes := []int{
			http.StatusInternalServerError,           // 500
			http.StatusBadGateway,                    // 502
			http.StatusServiceUnavailable,            // 503
			http.StatusGatewayTimeout,                // 504
			http.StatusInsufficientStorage,           // 507
			http.StatusNetworkAuthenticationRequired, // 511
		}

		for _, code := range retryableCodes {
			assert.True(t, m.isRetryableError(code), "status %d should be retryable", code)
		}
	})

	t.Run("retryable_429_too_many_requests", func(t *testing.T) {
		assert.True(t, m.isRetryableError(http.StatusTooManyRequests)) // 429
	})

	t.Run("non_retryable_4xx_errors", func(t *testing.T) {
		nonRetryableCodes := []int{
			http.StatusBadRequest,          // 400
			http.StatusUnauthorized,        // 401
			http.StatusForbidden,           // 403
			http.StatusNotFound,            // 404
			http.StatusMethodNotAllowed,    // 405
			http.StatusConflict,            // 409
			http.StatusGone,                // 410
			http.StatusUnprocessableEntity, // 422
		}

		for _, code := range nonRetryableCodes {
			assert.False(t, m.isRetryableError(code), "status %d should NOT be retryable", code)
		}
	})

	t.Run("success_codes_not_retryable", func(t *testing.T) {
		successCodes := []int{
			http.StatusOK,        // 200
			http.StatusCreated,   // 201
			http.StatusAccepted,  // 202
			http.StatusNoContent, // 204
		}

		for _, code := range successCodes {
			assert.False(t, m.isRetryableError(code), "status %d should NOT be retryable", code)
		}
	})

	t.Run("redirect_codes_not_retryable", func(t *testing.T) {
		redirectCodes := []int{
			http.StatusMovedPermanently,  // 301
			http.StatusFound,             // 302
			http.StatusTemporaryRedirect, // 307
			http.StatusPermanentRedirect, // 308
		}

		for _, code := range redirectCodes {
			assert.False(t, m.isRetryableError(code), "status %d should NOT be retryable", code)
		}
	})

	t.Run("non_standard_5xx_not_included", func(t *testing.T) {
		// 501 Not Implemented is not in the list
		assert.False(t, m.isRetryableError(http.StatusNotImplemented)) // 501
		// 505 HTTP Version Not Supported is not in the list
		assert.False(t, m.isRetryableError(http.StatusHTTPVersionNotSupported)) // 505
	})
}

// === extractUsernameFromActorID Tests ===

func TestExtractUsernameFromActorID(t *testing.T) {
	tests := []struct {
		name     string
		actorID  string
		expected string
	}{
		{
			name:     "standard_users_path",
			actorID:  "https://mastodon.social/users/john",
			expected: "john",
		},
		{
			name:     "actors_path",
			actorID:  "https://example.com/actors/jane",
			expected: "jane",
		},
		{
			name:     "empty_string",
			actorID:  "",
			expected: "",
		},
		{
			name:     "invalid_url_no_scheme",
			actorID:  "not-a-url",
			expected: "", // url.Parse parses without scheme, but path is invalid
		},
		{
			name:     "trailing_slash",
			actorID:  "https://example.com/users/bob/",
			expected: "bob", // Function correctly extracts username even with trailing slash
		},
		{
			name:     "nested_path",
			actorID:  "https://example.com/api/users/alice",
			expected: "alice", // Falls back to last path segment
		},
		{
			name:     "simple_path",
			actorID:  "https://example.com/username",
			expected: "username", // Last path segment
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUsernameFromActorID(tt.actorID)
			assert.Equal(t, tt.expected, result)
		})
	}
}
