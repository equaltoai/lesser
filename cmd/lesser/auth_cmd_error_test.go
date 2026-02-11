package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthCommands_ErrorBranches(t *testing.T) {
	t.Run("status returns error for corrupt session file", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("LESSER_AUTH_SECRET", "test-secret")

		baseURL := "https://example.com"

		path, err := authSessionFile(baseURL)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("not-a-session"), 0o600))

		err = runAuth([]string{"status", "--base-url", baseURL})
		require.Error(t, err)
	})

	t.Run("whoami fails when refresh token is invalid", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/oauth/token" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"bad refresh token"}`))
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

		err := runAuth([]string{"whoami", "--base-url", baseURL})
		require.Error(t, err)
		require.Equal(t, oauthErrorRefreshReauthRequired, err.Error())
	})

	t.Run("login fails when app registration is missing client_id", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/apps" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"missing"}`))
		}))
		t.Cleanup(srv.Close)

		t.Setenv("HOME", t.TempDir())
		t.Setenv("LESSER_AUTH_SECRET", "test-secret")

		err := runAuth([]string{"login", "--base-url", srv.URL, "--scopes", "read"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing client_id")
	})

	t.Run("login fails when device code response is missing required fields", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v1/apps":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"client_id":"client-1"}`))
			case "/oauth/device/code":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"user_code":"USER-1"}`))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(srv.Close)

		t.Setenv("HOME", t.TempDir())
		t.Setenv("LESSER_AUTH_SECRET", "test-secret")

		err := runAuth([]string{"login", "--base-url", srv.URL, "--scopes", "read"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing required fields")
	})

	t.Run("normalizeBaseURL rejects missing scheme", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("LESSER_AUTH_SECRET", "test-secret")

		err := runAuth([]string{"login", "--base-url", "example.com", "--scopes", "read"})
		require.Error(t, err)
	})
}
