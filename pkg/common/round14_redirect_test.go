package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRedirectURL(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		_, err := ValidateRedirectURL("", "example.com")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRedirectURLEmpty)
	})

	t.Run("invalid url", func(t *testing.T) {
		_, err := ValidateRedirectURL("://bad", "example.com")
		require.Error(t, err)
	})

	t.Run("protocol-relative not allowed", func(t *testing.T) {
		_, err := ValidateRedirectURL("//evil.com/path", "example.com")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrProtocolRelativeURLsNotAllowed)
	})

	t.Run("javascript/data urls not allowed", func(t *testing.T) {
		_, err := ValidateRedirectURL("javascript:alert(1)", "example.com")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrJavascriptDataURLsNotAllowed)
	})

	t.Run("relative url allowed", func(t *testing.T) {
		got, err := ValidateRedirectURL("/welcome", "example.com")
		require.NoError(t, err)
		assert.Equal(t, "/welcome", got)
	})

	t.Run("whitelisted host allowed", func(t *testing.T) {
		got, err := ValidateRedirectURL("https://lesser.example.com/auth", "example.com")
		require.NoError(t, err)
		assert.Equal(t, "https://lesser.example.com/auth", got)
	})

	t.Run("same host allowed", func(t *testing.T) {
		got, err := ValidateRedirectURL("https://example.com/ok", "example.com")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/ok", got)
	})

	t.Run("external host blocked", func(t *testing.T) {
		_, err := ValidateRedirectURL("https://evil.com/ok", "example.com")
		require.Error(t, err)
	})

	t.Run("ConfigureAllowedRedirectHosts updates whitelist", func(t *testing.T) {
		orig := allowedRedirectHosts
		t.Cleanup(func() { allowedRedirectHosts = orig })

		ConfigureAllowedRedirectHosts([]string{"example.net"})
		got, err := ValidateRedirectURL("https://example.net/ok", "example.com")
		require.NoError(t, err)
		assert.Equal(t, "https://example.net/ok", got)
	})
}

func TestSafeRedirectAndHelpers(t *testing.T) {
	t.Run("SafeRedirect uses redirect_uri when valid", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "https://example.com/login?redirect_uri=%2Fhome", nil)
		r.Host = "example.com"
		w := httptest.NewRecorder()

		SafeRedirect(w, r, "/default")

		resp := w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode)
		assert.Equal(t, "/home", resp.Header.Get("Location"))
	})

	t.Run("SafeRedirect falls back to default when invalid", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "https://example.com/login?redirect_uri=https://evil.com", nil)
		r.Host = "example.com"
		w := httptest.NewRecorder()

		SafeRedirect(w, r, "/default")

		resp := w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode)
		assert.Equal(t, "/default", resp.Header.Get("Location"))
	})

	t.Run("SafeRedirectOrDefault chooses sanitized when valid", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "https://example.com/login", nil)
		r.Host = "example.com"
		w := httptest.NewRecorder()

		SafeRedirectOrDefault(w, r, "/return", "/default")

		resp := w.Result()
		assert.Equal(t, http.StatusFound, resp.StatusCode)
		assert.Equal(t, "/return", resp.Header.Get("Location"))
	})

	t.Run("GetSafeRedirectURL returns default when invalid", func(t *testing.T) {
		got := GetSafeRedirectURL("https://evil.com", "example.com", "/default")
		assert.Equal(t, "/default", got)
	})
}
