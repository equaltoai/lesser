package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCLIAPIClient_RefreshesAndUpdatesSession(t *testing.T) {
	var tokenCalls atomic.Int32
	var verifyCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls.Add(1)
			require.NoError(t, r.ParseForm())
			require.Equal(t, "refresh_token", r.FormValue("grant_type"))
			require.Equal(t, "client-1", r.FormValue("client_id"))
			require.Equal(t, "refresh-1", r.FormValue("refresh_token"))

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-2","scope":"read write","created_at":1}`))
		case "/api/v1/accounts/verify_credentials":
			verifyCalls.Add(1)
			require.Equal(t, "Bearer access-1", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"username":"alice"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
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

	client, err := newCLIAPIClient(baseURL, key, cliAPIClientOptions{Retries: 0, Timeout: 5 * time.Second})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	status, _, body, err := client.Request(ctx, http.MethodGet, "/api/v1/accounts/verify_credentials", nil, nil)
	require.NoError(t, err)
	require.Equal(t, 200, status)
	require.Contains(t, string(body), "alice")

	// Refresh token rotation is persisted.
	updated, err := readAuthSession(baseURL, key)
	require.NoError(t, err)
	require.Equal(t, "refresh-2", updated.RefreshToken)

	// Second call reuses access token (no additional refresh).
	status, _, _, err = client.Request(ctx, http.MethodGet, "/api/v1/accounts/verify_credentials", nil, nil)
	require.NoError(t, err)
	require.Equal(t, 200, status)

	require.Equal(t, int32(1), tokenCalls.Load())
	require.Equal(t, int32(2), verifyCalls.Load())
}

func TestCLIAPIClient_RetryAfterRespected(t *testing.T) {
	var apiCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			require.NoError(t, r.ParseForm())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read","created_at":1}`))
		case "/api/test":
			call := apiCalls.Add(1)
			if call == 1 {
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("rate limited"))
				return
			}
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
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

	client, err := newCLIAPIClient(baseURL, key, cliAPIClientOptions{Retries: 1, Timeout: 5 * time.Second})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	var slept time.Duration
	client.sleepFn = func(d time.Duration) { slept += d }
	client.nowFn = func() time.Time { return time.Unix(0, 0).UTC() }

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	status, _, body, err := client.Request(ctx, http.MethodGet, "/api/test", nil, nil)
	require.NoError(t, err)
	require.Equal(t, 200, status)
	require.Equal(t, "ok", string(body))

	require.Equal(t, int32(2), apiCalls.Load())
	require.GreaterOrEqual(t, slept, 2*time.Second)
}
