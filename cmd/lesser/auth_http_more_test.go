package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/stretchr/testify/require"
)

func TestSleepWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, sleepWithContext(ctx, time.Second), context.Canceled)
	require.NoError(t, sleepWithContext(context.Background(), 0))
}

func TestOAuthHTTPError_ErrorFormatting(t *testing.T) {
	require.Equal(t, "bad (invalid_grant)", (&oauthHTTPError{
		Status: 400,
		OAuth: apimodels.OAuthErrorResponse{
			Error:            "invalid_grant",
			ErrorDescription: "bad",
		},
	}).Error())

	require.Equal(t, "bad", (&oauthHTTPError{
		Status: 400,
		OAuth: apimodels.OAuthErrorResponse{
			Error:            "",
			ErrorDescription: "bad",
		},
	}).Error())

	require.Equal(t, oauthErrorDescriptionDefault, (&oauthHTTPError{
		Status: 400,
		OAuth: apimodels.OAuthErrorResponse{
			Error:            "",
			ErrorDescription: "",
		},
	}).Error())

	require.Equal(t, oauthErrorDescriptionDefault+" (weird)", (&oauthHTTPError{
		Status: 400,
		OAuth: apimodels.OAuthErrorResponse{
			Error:            "weird",
			ErrorDescription: "",
		},
	}).Error())
}

func TestResolveViewerAndScopes_EmptyScopeAndMissingUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/accounts/verify_credentials" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"username":"alice"}`))
	}))
	t.Cleanup(srv.Close)

	username, scopes, err := resolveViewerAndScopes(context.Background(), srv.URL, "token", "")
	require.NoError(t, err)
	require.Equal(t, "alice", username)
	require.Nil(t, scopes)

	srvMissing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srvMissing.Close)

	_, _, err = resolveViewerAndScopes(context.Background(), srvMissing.URL, "token", "read")
	require.Error(t, err)
}

func TestDoFormPOST_And_DoGETJSON_ErrorBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok-nil-out":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
		case "/ok-invalid-json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
		case "/bad-oauth":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"bad"}`))
		case "/bad-plain":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("plain error"))
		case "/bad-empty":
			w.WriteHeader(http.StatusBadRequest)
		case "/get-ok":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"username":"alice"}`))
		case "/get-invalid":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
		case "/get-bad-empty":
			w.WriteHeader(http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	form := url.Values{}
	form.Set("a", "b")

	require.NoError(t, doFormPOST(context.Background(), srv.URL, "/ok-nil-out", form, nil))

	var out apimodels.OAuthTokenResponse
	require.Error(t, doFormPOST(context.Background(), srv.URL, "/ok-invalid-json", form, &out))

	err := doFormPOST(context.Background(), srv.URL, "/bad-oauth", form, &out)
	require.Error(t, err)
	var oauthErr *oauthHTTPError
	require.ErrorAs(t, err, &oauthErr)

	err = doFormPOST(context.Background(), srv.URL, "/bad-plain", form, &out)
	require.Error(t, err)

	err = doFormPOST(context.Background(), srv.URL, "/bad-empty", form, &out)
	require.Error(t, err)

	var viewer verifyCredentialsResponse
	require.NoError(t, doGETJSON(context.Background(), srv.URL, "/get-ok", "token", &viewer))
	require.Equal(t, "alice", viewer.Username)

	require.NoError(t, doGETJSON(context.Background(), srv.URL, "/get-ok", "token", nil))

	require.Error(t, doGETJSON(context.Background(), srv.URL, "/get-invalid", "token", &viewer))
	require.Error(t, doGETJSON(context.Background(), srv.URL, "/get-bad-empty", "token", &viewer))
}

func TestExchangeDeviceCodeForToken_ErrorBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending","error_description":"wait"}`))
	}))
	t.Cleanup(srv.Close)

	token, oauthErr, err := exchangeDeviceCodeForToken(context.Background(), srv.URL, "client-1", "dev-1")
	require.NoError(t, err)
	require.Nil(t, token)
	require.NotNil(t, oauthErr)
	require.Equal(t, "authorization_pending", oauthErr.Error)

	token, oauthErr, err = exchangeDeviceCodeForToken(context.Background(), "http://%", "client-1", "dev-1")
	require.Error(t, err)
	require.Nil(t, token)
	require.Nil(t, oauthErr)
}

func TestPollDeviceToken_UsesDeviceStreamWhenAvailable_Round23(t *testing.T) {
	var tokenCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/streaming/oauth/device":
			require.Equal(t, "dev-1", r.Header.Get("X-Lesser-Device-Code"))
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: oauth.device\n"))
			_, _ = w.Write([]byte("data: {\"status\":\"pending\"}\n\n"))
			_, _ = w.Write([]byte("event: oauth.device\n"))
			_, _ = w.Write([]byte("data: {\"status\":\"approved\"}\n\n"))
		case "/oauth/token":
			tokenCalls.Add(1)
			require.NoError(t, r.ParseForm())
			require.Equal(t, "urn:ietf:params:oauth:grant-type:device_code", r.FormValue("grant_type"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read write","created_at":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	token, err := pollDeviceToken(context.Background(), srv.URL, "client-1", "dev-1", time.Second, time.Minute, nil)
	require.NoError(t, err)
	require.NotNil(t, token)
	require.Equal(t, "access-1", token.AccessToken)
	require.Equal(t, int32(1), tokenCalls.Load())
}

func TestWaitForDeviceAuthorizationStream_NotSupported_Round23(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/streaming/oauth/device" {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	status, supported, err := waitForDeviceAuthorizationStream(context.Background(), srv.URL, "dev-1", time.Second, &authFlags{Debug: true})
	require.NoError(t, err)
	require.False(t, supported)
	require.Empty(t, status)
}

func TestWaitForDeviceAuthorizationStream_BadStatus_Round23(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/streaming/oauth/device" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(srv.Close)

	_, supported, err := waitForDeviceAuthorizationStream(context.Background(), srv.URL, "dev-1", time.Second, &authFlags{Debug: true})
	require.Error(t, err)
	require.True(t, supported)
}

func TestDeviceAuthorizationStreamStatusFromData_Round23(t *testing.T) {
	status, ok := deviceAuthorizationStreamStatusFromData("")
	require.False(t, ok)
	require.Empty(t, status)

	status, ok = deviceAuthorizationStreamStatusFromData("not-json")
	require.False(t, ok)
	require.Empty(t, status)

	status, ok = deviceAuthorizationStreamStatusFromData(`{"status":"pending"}`)
	require.False(t, ok)
	require.Empty(t, status)

	status, ok = deviceAuthorizationStreamStatusFromData(`{"status":"approved"}`)
	require.True(t, ok)
	require.Equal(t, "approved", status)
}

func TestPollDeviceToken_StreamApproved_RetriesTokenOnce_Round23(t *testing.T) {
	var tokenCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/streaming/oauth/device":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: oauth.device\n"))
			_, _ = w.Write([]byte("data: {\"status\":\"approved\"}\n\n"))
		case "/oauth/token":
			call := tokenCalls.Add(1)
			require.NoError(t, r.ParseForm())
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"authorization_pending","error_description":"wait"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read write","created_at":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	token, err := pollDeviceToken(context.Background(), srv.URL, "client-1", "dev-1", time.Second, time.Minute, &authFlags{Debug: true})
	require.NoError(t, err)
	require.NotNil(t, token)
	require.Equal(t, int32(2), tokenCalls.Load())
}

func TestPollDeviceToken_StreamEOF_FallsBackToPolling_Round23(t *testing.T) {
	var streamCalls atomic.Int32
	var tokenCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/streaming/oauth/device":
			streamCalls.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: oauth.device\n"))
			_, _ = w.Write([]byte("data: {\"status\":\"pending\"}\n\n"))
		case "/oauth/token":
			tokenCalls.Add(1)
			require.NoError(t, r.ParseForm())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-1","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh-1","scope":"read write","created_at":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	token, err := pollDeviceToken(context.Background(), srv.URL, "client-1", "dev-1", time.Second, time.Minute, &authFlags{Debug: true})
	require.NoError(t, err)
	require.NotNil(t, token)
	require.Equal(t, int32(1), streamCalls.Load())
	require.Equal(t, int32(1), tokenCalls.Load())
}

func TestWaitForDeviceAuthorizationStream_InvalidBaseURL_Round23(t *testing.T) {
	_, supported, err := waitForDeviceAuthorizationStream(context.Background(), "http://%", "dev-1", time.Second, &authFlags{Debug: true})
	require.Error(t, err)
	require.False(t, supported)
}

func TestWaitForDeviceAuthorizationStream_RequestFailure_Round23(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()

	_, supported, err := waitForDeviceAuthorizationStream(context.Background(), srv.URL, "dev-1", time.Second, &authFlags{Debug: true})
	require.Error(t, err)
	require.False(t, supported)
}

func TestPollDeviceToken_StreamApproved_UnknownTokenError_Round23(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/streaming/oauth/device":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: oauth.device\n"))
			_, _ = w.Write([]byte("data: {\"status\":\"approved\"}\n\n"))
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"weird","error_description":"nope"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	_, err := pollDeviceToken(context.Background(), srv.URL, "client-1", "dev-1", time.Second, time.Minute, &authFlags{Debug: true})
	require.Error(t, err)
	require.Equal(t, "nope (weird)", err.Error())
}

func TestWaitForDeviceAuthorizationStream_MultilineData_DefaultTTL_Round23(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/streaming/oauth/device" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: oauth.device\n"))
		_, _ = w.Write([]byte("data: {\"status\":\n"))
		_, _ = w.Write([]byte("data: \"approved\"}\n\n"))
	}))
	t.Cleanup(srv.Close)

	status, supported, err := waitForDeviceAuthorizationStream(context.Background(), srv.URL, "dev-1", 0, &authFlags{Debug: true})
	require.NoError(t, err)
	require.True(t, supported)
	require.Equal(t, "approved", status)
}

func TestExchangeDeviceCodeWithRetries_ErrorMappings_Round23(t *testing.T) {
	cases := []struct {
		name string
		resp string
		want string
	}{
		{name: "access_denied", resp: `{"error":"access_denied","error_description":"no"}`, want: oauthErrorDeviceAuthDenied},
		{name: "expired_token", resp: `{"error":"expired_token","error_description":"no"}`, want: oauthErrorDeviceCodeExpired},
		{name: "invalid_grant", resp: `{"error":"invalid_grant","error_description":"no"}`, want: oauthErrorDeviceAuthInvalid},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/oauth/token" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.resp))
			}))
			t.Cleanup(srv.Close)

			_, err := exchangeDeviceCodeWithRetries(context.Background(), srv.URL, "client-1", "dev-1")
			require.Error(t, err)
			require.Equal(t, tc.want, err.Error())
		})
	}
}

func TestPollDeviceToken_StreamDenied_Round23(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/streaming/oauth/device":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: oauth.device\n"))
			_, _ = w.Write([]byte("data: {\"status\":\"denied\"}\n\n"))
		case "/oauth/token":
			t.Fatal("unexpected token exchange call")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	_, err := pollDeviceToken(context.Background(), srv.URL, "client-1", "dev-1", time.Second, time.Minute, &authFlags{Debug: true})
	require.Error(t, err)
	require.Equal(t, oauthErrorDeviceAuthDenied, err.Error())
}
