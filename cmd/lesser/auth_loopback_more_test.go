package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoopbackHelpers_Round22(t *testing.T) {
	t.Run("randomURLSafeString validates length", func(t *testing.T) {
		_, err := randomURLSafeString(0)
		require.Error(t, err)
	})

	t.Run("generatePKCE returns verifier + challenge", func(t *testing.T) {
		verifier, challenge, err := generatePKCE()
		require.NoError(t, err)
		require.NotEmpty(t, verifier)
		require.NotEmpty(t, challenge)
		require.NotEqual(t, verifier, challenge)
	})

	t.Run("buildAuthorizationCodeAuthorizeURL validates baseURL", func(t *testing.T) {
		_, err := buildAuthorizationCodeAuthorizeURL("://bad", "client-1", "http://127.0.0.1/cb", "read", "state", "challenge")
		require.Error(t, err)
	})

	t.Run("openBrowser validates url", func(t *testing.T) {
		require.Error(t, openBrowser(""))
	})

	t.Run("openBrowser starts xdg-open when present", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("this test only exercises the linux xdg-open path")
		}

		binDir := t.TempDir()
		xdgOpen := filepath.Join(binDir, "xdg-open")
		require.NoError(t, os.WriteFile(xdgOpen, []byte("#!/bin/sh\nexit 0\n"), 0o755))

		t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
		require.NoError(t, openBrowser("http://example.com"))
	})

	t.Run("htmlEscape escapes specials", func(t *testing.T) {
		require.Equal(t, "a&amp;b&lt;c&gt;&quot;&#39;", htmlEscape("a&b<c>\"'"))
	})

	t.Run("writeLoopbackHTML writes a page", func(t *testing.T) {
		rec := httptest.NewRecorder()
		writeLoopbackHTML(rec, "t", "h", "b")
		require.Contains(t, rec.Body.String(), "<title>t</title>")
		require.Contains(t, rec.Body.String(), "<h2>h</h2>")
		require.Contains(t, rec.Body.String(), "<p>b</p>")
		require.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html"))
	})

	t.Run("exchangeAuthorizationCodeForToken returns error on non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"nope"}`))
		}))
		t.Cleanup(srv.Close)

		_, err := exchangeAuthorizationCodeForToken(context.Background(), srv.URL, "client-1", "code-1", "http://127.0.0.1/cb", "verifier")
		require.Error(t, err)
	})
}

func TestLoopbackCallbackServer_Round22(t *testing.T) {
	t.Run("validates expected state", func(t *testing.T) {
		_, err := startLoopbackCallbackServer("")
		require.Error(t, err)
	})

	t.Run("rejects invalid state", func(t *testing.T) {
		srv, err := startLoopbackCallbackServer("state-1")
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

		u, err := url.Parse(srv.RedirectURI)
		require.NoError(t, err)
		q := u.Query()
		q.Set("state", "wrong")
		q.Set("code", "code-1")
		u.RawQuery = q.Encode()

		resp, err := http.Get(u.String())
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("rejects missing code", func(t *testing.T) {
		srv, err := startLoopbackCallbackServer("state-1")
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

		u, err := url.Parse(srv.RedirectURI)
		require.NoError(t, err)
		q := u.Query()
		q.Set("state", "state-1")
		u.RawQuery = q.Encode()

		resp, err := http.Get(u.String())
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("propagates oauth error", func(t *testing.T) {
		srv, err := startLoopbackCallbackServer("state-1")
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

		u, err := url.Parse(srv.RedirectURI)
		require.NoError(t, err)
		q := u.Query()
		q.Set("state", "state-1")
		q.Set("error", "access_denied")
		q.Set("error_description", "denied")
		u.RawQuery = q.Encode()

		resp, err := http.Get(u.String())
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		t.Cleanup(cancel)

		_, err = srv.WaitForCode(ctx)
		require.ErrorContains(t, err, "denied (access_denied)")
	})

	t.Run("propagates oauth error with default message", func(t *testing.T) {
		srv, err := startLoopbackCallbackServer("state-1")
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

		u, err := url.Parse(srv.RedirectURI)
		require.NoError(t, err)
		q := u.Query()
		q.Set("state", "state-1")
		q.Set("error", "access_denied")
		u.RawQuery = q.Encode()

		resp, err := http.Get(u.String())
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		t.Cleanup(cancel)

		_, err = srv.WaitForCode(ctx)
		require.ErrorContains(t, err, "authorization failed (access_denied)")
	})

	t.Run("WaitForCode returns context error", func(t *testing.T) {
		srv, err := startLoopbackCallbackServer("state-1")
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		t.Cleanup(cancel)

		_, err = srv.WaitForCode(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("waitForCode validates receiver", func(t *testing.T) {
		var srv *loopbackCallbackServer
		_, err := srv.WaitForCode(context.Background())
		require.Error(t, err)
	})

	t.Run("Shutdown is noop when server is missing", func(t *testing.T) {
		var srv loopbackCallbackServer
		require.NoError(t, srv.Shutdown(context.Background()))

		var nilSrv *loopbackCallbackServer
		require.NoError(t, nilSrv.Shutdown(context.Background()))
	})
}
