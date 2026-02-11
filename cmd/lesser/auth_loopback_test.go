package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthCommands_LoopbackFlow_EndToEnd(t *testing.T) {
	var appCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/apps":
			appCalls.Add(1)
			require.NoError(t, r.ParseForm())
			require.Equal(t, defaultAuthClientName, r.FormValue("client_name"))
			require.NotEmpty(t, r.FormValue("redirect_uris"))
			require.NotEmpty(t, r.FormValue("scopes"))
			require.Equal(t, defaultAuthClientClass, r.FormValue("client_class"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"client_id":"client-1"}`))
		case "/oauth/authorize":
			q := r.URL.Query()
			redirectURI := q.Get("redirect_uri")
			require.NotEmpty(t, redirectURI)
			state := q.Get("state")
			require.NotEmpty(t, state)

			cb, err := url.Parse(redirectURI)
			require.NoError(t, err)
			cbQ := cb.Query()
			cbQ.Set("code", "code-1")
			cbQ.Set("state", state)
			cb.RawQuery = cbQ.Encode()
			http.Redirect(w, r, cb.String(), http.StatusFound)
		case "/oauth/token":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "authorization_code", r.FormValue("grant_type"))
			require.Equal(t, "code-1", r.FormValue("code"))
			require.Equal(t, "client-1", r.FormValue("client_id"))
			require.NotEmpty(t, r.FormValue("redirect_uri"))
			require.NotEmpty(t, r.FormValue("code_verifier"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read write","created_at":1}`))
		case "/api/v1/accounts/verify_credentials":
			require.Regexp(t, `^Bearer\s+\S+`, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"username":"alice"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	origOpenBrowserFn := openBrowserFn
	t.Cleanup(func() { openBrowserFn = origOpenBrowserFn })

	openBrowserFn = func(targetURL string) error {
		client := &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: 5 * time.Second,
		}

		resp, err := client.Get(targetURL)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()

		loc := strings.TrimSpace(resp.Header.Get("Location"))
		if loc == "" {
			return nil
		}

		var lastErr error
		for i := 0; i < 50; i++ {
			cbResp, err := http.Get(loc)
			if err == nil {
				_, _ = io.Copy(io.Discard, cbResp.Body)
				_ = cbResp.Body.Close()
				return nil
			}
			lastErr = err
			time.Sleep(10 * time.Millisecond)
		}
		return lastErr
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := srv.URL
	key := deriveAuthKey(baseURL, "test-secret")

	require.NoError(t, runAuth([]string{"login", "--flow", "loopback", "--base-url", baseURL, "--scopes", "read write"}))
	require.Equal(t, int32(1), appCalls.Load())

	session, err := readAuthSession(baseURL, key)
	require.NoError(t, err)
	require.Equal(t, "client-1", session.ClientID)
	require.Equal(t, "refresh-1", session.RefreshToken)
	require.Equal(t, "alice", session.Username)

	// Allow goroutines started by token buckets to exit promptly in other tests.
	time.Sleep(1 * time.Millisecond)
}

func TestAuthCommands_LoopbackFlow_OpenBrowserErrorStillSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/apps":
			require.NoError(t, r.ParseForm())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"client_id":"client-1"}`))
		case "/oauth/authorize":
			q := r.URL.Query()
			redirectURI := q.Get("redirect_uri")
			state := q.Get("state")
			cb, err := url.Parse(redirectURI)
			require.NoError(t, err)
			cbQ := cb.Query()
			cbQ.Set("code", "code-1")
			cbQ.Set("state", state)
			cb.RawQuery = cbQ.Encode()
			http.Redirect(w, r, cb.String(), http.StatusFound)
		case "/oauth/token":
			require.NoError(t, r.ParseForm())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read write","created_at":1}`))
		case "/api/v1/accounts/verify_credentials":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"username":"alice"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	origOpenBrowserFn := openBrowserFn
	t.Cleanup(func() { openBrowserFn = origOpenBrowserFn })

	openBrowserFn = func(targetURL string) error {
		client := &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: 5 * time.Second,
		}

		resp, err := client.Get(targetURL)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()

		loc := strings.TrimSpace(resp.Header.Get("Location"))
		if loc != "" {
			cbResp, cbErr := http.Get(loc)
			if cbErr == nil {
				_, _ = io.Copy(io.Discard, cbResp.Body)
				_ = cbResp.Body.Close()
			}
		}
		return errTestIntentionalOpenBrowserFailure
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := srv.URL
	key := deriveAuthKey(baseURL, "test-secret")

	require.NoError(t, runAuth([]string{"login", "--flow", "loopback", "--base-url", baseURL, "--scopes", "read write", "--debug"}))

	session, err := readAuthSession(baseURL, key)
	require.NoError(t, err)
	require.Equal(t, "refresh-1", session.RefreshToken)
	require.Equal(t, "alice", session.Username)
}

var errTestIntentionalOpenBrowserFailure = errors.New("test: open browser failed")
