package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthCommands_DeviceFlow_EndToEnd(t *testing.T) {
	var appCalls atomic.Int32
	var revokeCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/apps":
			appCalls.Add(1)
			require.NoError(t, r.ParseForm())
			require.NotEmpty(t, r.FormValue("client_name"))
			require.NotEmpty(t, r.FormValue("redirect_uris"))
			require.NotEmpty(t, r.FormValue("scopes"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"client_id":"client-1"}`))
		case "/oauth/device/code":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "client-1", r.FormValue("client_id"))
			scope := r.FormValue("scope")
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(scope, "no_complete") {
				_, _ = w.Write([]byte(`{"device_code":"dev-1","user_code":"USER-1","verification_uri":"https://example.com/device","expires_in":600,"interval":0}`))
				return
			}
			_, _ = w.Write([]byte(`{"device_code":"dev-1","user_code":"USER-1","verification_uri":"https://example.com/device","verification_uri_complete":"https://example.com/device?user_code=USER-1","expires_in":600,"interval":1}`))
		case "/oauth/token":
			require.NoError(t, r.ParseForm())
			grant := r.FormValue("grant_type")
			w.Header().Set("Content-Type", "application/json")
			switch grant {
			case "urn:ietf:params:oauth:grant-type:device_code":
				require.Equal(t, "dev-1", r.FormValue("device_code"))
				require.Equal(t, "client-1", r.FormValue("client_id"))
				_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read write","created_at":1}`))
			case "refresh_token":
				require.Equal(t, "client-1", r.FormValue("client_id"))
				require.NotEmpty(t, r.FormValue("refresh_token"))
				_, _ = w.Write([]byte(`{"access_token":"access-2","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-2","scope":"read write","created_at":2}`))
			default:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"unsupported_grant_type","error_description":"bad grant"}`))
			}
		case "/api/v1/accounts/verify_credentials":
			require.Regexp(t, `^Bearer\s+\S+`, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"username":"alice"}`))
		case "/oauth/revoke":
			revokeCalls.Add(1)
			require.NoError(t, r.ParseForm())
			require.Equal(t, "refresh_token", r.FormValue("token_type_hint"))
			require.Equal(t, "client-1", r.FormValue("client_id"))
			require.NotEmpty(t, r.FormValue("token"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := srv.URL
	key := deriveAuthKey(baseURL, "test-secret")

	require.NoError(t, runAuth(nil))
	require.NoError(t, runAuth([]string{helpFlagLong}))
	require.NoError(t, runAuth([]string{"-x"}))
	require.Error(t, runAuth([]string{"nope"}))

	// First login registers an OAuth app.
	require.NoError(t, runAuth([]string{"login", "--base-url", baseURL, "--scopes", "read write"}))
	require.Equal(t, int32(1), appCalls.Load())

	// Second login reuses stored client_id and should avoid re-registering an app.
	require.NoError(t, runAuth([]string{"login", "--base-url", baseURL, "--scopes", "read write", "--json"}))
	require.Equal(t, int32(1), appCalls.Load())

	require.NoError(t, runAuth([]string{"login", "--base-url", baseURL, "--scopes", "read write", "--debug", "--json"}))

	require.NoError(t, runAuth([]string{"login", "--base-url", baseURL, "--scopes", "read write"}))
	require.Equal(t, int32(1), appCalls.Load())

	require.NoError(t, runAuth([]string{"login", "--base-url", baseURL, "--scopes", "read write no_complete"}))

	session, err := readAuthSession(baseURL, key)
	require.NoError(t, err)
	require.Equal(t, "client-1", session.ClientID)
	require.Equal(t, "refresh-1", session.RefreshToken)
	require.Equal(t, "alice", session.Username)

	require.NoError(t, runAuth([]string{"status", "--base-url", baseURL}))
	require.NoError(t, runAuth([]string{"whoami", "--base-url", baseURL}))

	updated, err := readAuthSession(baseURL, key)
	require.NoError(t, err)
	require.Equal(t, "refresh-2", updated.RefreshToken)
	require.Equal(t, "alice", updated.Username)

	require.NoError(t, runAuth([]string{"device"}))
	require.NoError(t, runAuth([]string{"device", helpFlagLong}))
	require.NoError(t, runAuth([]string{"device", "start", "--base-url", baseURL, "--client-id", "client-1", "--scopes", "read write"}))
	require.NoError(t, runAuth([]string{"device", "start", "--base-url", baseURL, "--scopes", "read write", "--json"}))
	require.Error(t, runAuth([]string{"device", "poll", "--base-url", baseURL}))
	require.Error(t, runAuth([]string{"device", "wat"}))

	require.NoError(t, runAuth([]string{"device", "poll", "--base-url", baseURL, "--client-id", "client-1", "--device-code", "dev-1", "--expires-in", "600", "--interval", "1", "--json"}))
	require.NoError(t, runAuth([]string{"device", "poll", "--base-url", baseURL, "--client-id", "client-1", "--device-code", "dev-1", "--expires-in", "600", "--interval", "0"}))

	require.NoError(t, runAuth([]string{"logout", "--base-url", baseURL}))
	require.Equal(t, int32(1), revokeCalls.Load())
	require.NoError(t, runAuth([]string{"logout", "--base-url", baseURL}))
	require.NoError(t, runAuth([]string{"status", "--base-url", baseURL}))
	require.Error(t, runAuth([]string{"whoami", "--base-url", baseURL}))

	// Allow goroutines started by token buckets to exit promptly in other tests.
	time.Sleep(1 * time.Millisecond)
}
