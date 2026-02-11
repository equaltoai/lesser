package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
