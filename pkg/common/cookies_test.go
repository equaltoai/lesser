package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCookies(t *testing.T) {
	tests := []struct {
		name         string
		cookieHeader string
		expected     map[string]string
	}{
		{
			name:         "empty header",
			cookieHeader: "",
			expected:     map[string]string{},
		},
		{
			name:         "single cookie",
			cookieHeader: "session=abc123",
			expected:     map[string]string{"session": "abc123"},
		},
		{
			name:         "multiple cookies",
			cookieHeader: "session=abc123; user=john; token=xyz",
			expected: map[string]string{
				"session": "abc123",
				"user":    "john",
				"token":   "xyz",
			},
		},
		{
			name:         "cookie with spaces in value (URL encoded)",
			cookieHeader: "name=hello%20world",
			expected:     map[string]string{"name": "hello%20world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCookies(tt.cookieHeader)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetCookie(t *testing.T) {
	t.Run("cookie exists with Cookie header", func(t *testing.T) {
		headers := map[string]string{
			"Cookie": "session=abc123; user=john",
		}
		result := GetCookie(headers, "session")
		assert.Equal(t, "abc123", result)
	})

	t.Run("cookie exists with lowercase cookie header", func(t *testing.T) {
		headers := map[string]string{
			"cookie": "session=abc123",
		}
		result := GetCookie(headers, "session")
		assert.Equal(t, "abc123", result)
	})

	t.Run("cookie not found", func(t *testing.T) {
		headers := map[string]string{
			"Cookie": "other=value",
		}
		result := GetCookie(headers, "session")
		assert.Empty(t, result)
	})

	t.Run("no cookie header", func(t *testing.T) {
		headers := map[string]string{}
		result := GetCookie(headers, "session")
		assert.Empty(t, result)
	})
}

func TestSetSecureCookieWithConfig(t *testing.T) {
	t.Run("sets cookie with config", func(t *testing.T) {
		w := httptest.NewRecorder()
		config := CookieConfig{
			Path:     "/api",
			Secure:   true,
			HTTPOnly: true,
			SameSite: http.SameSiteStrictMode,
		}

		SetSecureCookieWithConfig(w, "test", "value", 3600, config)

		cookies := w.Result().Cookies()
		require.Len(t, cookies, 1)

		cookie := cookies[0]
		assert.Equal(t, "test", cookie.Name)
		assert.Equal(t, "value", cookie.Value)
		assert.Equal(t, "/api", cookie.Path)
		assert.True(t, cookie.Secure)
		assert.True(t, cookie.HttpOnly)
		assert.Equal(t, 3600, cookie.MaxAge)
	})

	t.Run("negative maxAge sets expires for deletion", func(t *testing.T) {
		w := httptest.NewRecorder()
		config := DefaultCookieConfig

		SetSecureCookieWithConfig(w, "test", "", -1, config)

		cookies := w.Result().Cookies()
		require.Len(t, cookies, 1)

		cookie := cookies[0]
		assert.Equal(t, -1, cookie.MaxAge)
		assert.True(t, cookie.Expires.Before(time.Now()))
	})
}

func TestSetSecureCookie(t *testing.T) {
	w := httptest.NewRecorder()
	SetSecureCookie(w, "session", "xyz", 3600)

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)

	cookie := cookies[0]
	assert.Equal(t, "session", cookie.Name)
	assert.Equal(t, "xyz", cookie.Value)
	assert.Equal(t, "/", cookie.Path)
	assert.True(t, cookie.Secure)
	assert.True(t, cookie.HttpOnly)
}

func TestDeleteSecureCookie(t *testing.T) {
	w := httptest.NewRecorder()
	DeleteSecureCookie(w, "session")

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)

	cookie := cookies[0]
	assert.Equal(t, "session", cookie.Name)
	assert.Empty(t, cookie.Value)
	assert.Equal(t, -1, cookie.MaxAge)
}

func TestDefaultCookieConfig(t *testing.T) {
	assert.Empty(t, DefaultCookieConfig.Domain)
	assert.Equal(t, "/", DefaultCookieConfig.Path)
	assert.True(t, DefaultCookieConfig.Secure)
	assert.True(t, DefaultCookieConfig.HTTPOnly)
	assert.Equal(t, http.SameSiteStrictMode, DefaultCookieConfig.SameSite)
}
