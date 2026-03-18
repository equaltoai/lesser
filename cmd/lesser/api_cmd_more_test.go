package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAPICmd_Request_ErrorResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			require.NoError(t, r.ParseForm())
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read","created_at":1}`))
		case "/api/retry":
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("slow down"))
		case "/api/fail":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := srv.URL
	key := deriveAuthKey(baseURL, "test-secret")
	require.NoError(t, writeAuthSession(baseURL, key, &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     "client-1",
		RefreshToken: "refresh-1",
		Username:     "alice",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))

	err := runAPIRequest([]string{"--base-url", baseURL, "--method", "GET", "--path", "/api/retry", "--rps", "0", "--max-concurrency", "1", "--retries", "0", "--timeout", "2"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "retry-after=120")

	err = runAPIRequest([]string{"--base-url", baseURL, "--method", "GET", "--path", "/api/fail", "--rps", "0", "--max-concurrency", "1", "--retries", "0", "--timeout", "2"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "api request failed (500)")
}

func TestAPICmd_Request_MethodValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read","created_at":1}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := srv.URL
	key := deriveAuthKey(baseURL, "test-secret")
	require.NoError(t, writeAuthSession(baseURL, key, &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     "client-1",
		RefreshToken: "refresh-1",
		Username:     "alice",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))

	err := runAPIRequest([]string{"--base-url", baseURL, "--method", "   ", "--path", "/api/test", "--rps", "0", "--max-concurrency", "1", "--retries", "0", "--timeout", "2"})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "method is required") || strings.Contains(err.Error(), "invalid method"))
}

func TestAPICmd_Request_PreflightErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read","created_at":1}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := srv.URL
	key := deriveAuthKey(baseURL, "test-secret")
	require.NoError(t, writeAuthSession(baseURL, key, &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     "client-1",
		RefreshToken: "refresh-1",
		Username:     "alice",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))

	require.NoError(t, runAPI([]string{"request", "--base-url", baseURL, "--method", "GET", "--path", "/", "--rps", "0", "--max-concurrency", "1", "--retries", "0", "--timeout", "2"}))
	require.Error(t, runAPI([]string{"request", "--unknown-flag"}))

	err := runAPIRequest([]string{"--base-url", "://bad", "--method", "GET", "--path", "/"})
	require.Error(t, err)

	err = runAPIRequest([]string{"--base-url", baseURL, "--method", "", "--path", "/"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "method is required")

	t.Setenv("LESSER_AUTH_SECRET", "")
	err = runAPIRequest([]string{"--base-url", baseURL, "--method", "GET", "--path", "/", "--secret-file", filepath.Join(t.TempDir(), "missing-secret")})
	require.Error(t, err)

	t.Setenv("LESSER_AUTH_SECRET", "other-secret")
	err = runAPIRequest([]string{"--base-url", baseURL + "/other", "--method", "GET", "--path", "/"})
	require.Error(t, err)
}

func TestAPICmd_Request_HeaderAndRequestErrorBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read","created_at":1}`))
		case "/api/header":
			require.Equal(t, "value", r.Header.Get("X-Test"))
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := srv.URL
	key := deriveAuthKey(baseURL, "test-secret")
	require.NoError(t, writeAuthSession(baseURL, key, &cliAuthSession{
		Version:      cliAuthSessionVersion,
		BaseURL:      baseURL,
		ClientID:     "client-1",
		RefreshToken: "refresh-1",
		Username:     "alice",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}))

	require.NoError(t, runAPIRequest([]string{"--base-url", baseURL, "--method", "GET", "--path", "/api/header", "--header", "X-Test: value", "--rps", "0", "--max-concurrency", "1", "--retries", "0", "--timeout", "2"}))

	err := runAPIRequest([]string{"--base-url", baseURL, "--method", "BAD METHOD", "--path", "/api/header", "--rps", "0", "--max-concurrency", "1", "--retries", "0", "--timeout", "2"})
	require.Error(t, err)

	body, err := readBodyArg("hello", "")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), body)
}
