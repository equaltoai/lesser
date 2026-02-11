package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/stretchr/testify/require"
)

type testClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *testClock) Now() time.Time {
	return c.now
}

func (c *testClock) Sleep(_ context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	return nil
}

func TestPollDeviceTokenInternal_AuthorizationPendingThenToken(t *testing.T) {
	clock := &testClock{now: time.Date(2026, time.February, 11, 0, 0, 0, 0, time.UTC)}

	calls := 0
	exchange := func(_ context.Context, _, _, _ string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
		calls++
		if calls == 1 {
			return nil, &apimodels.OAuthErrorResponse{Error: "authorization_pending"}, nil
		}
		return &apimodels.OAuthTokenResponse{AccessToken: "at-1", RefreshToken: "rt-1", Scope: "read"}, nil, nil
	}

	token, err := pollDeviceTokenInternal(context.Background(), "https://example.com", "client-1", "device-1", time.Second, time.Minute, nil, clock.Now, clock.Sleep, exchange)
	require.NoError(t, err)
	require.NotNil(t, token)
	require.Equal(t, "at-1", token.AccessToken)
	require.Equal(t, []time.Duration{time.Second}, clock.sleeps)
}

func TestPollDeviceTokenInternal_SlowDownIncreasesInterval(t *testing.T) {
	clock := &testClock{now: time.Date(2026, time.February, 11, 0, 0, 0, 0, time.UTC)}

	calls := 0
	exchange := func(_ context.Context, _, _, _ string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
		calls++
		if calls == 1 {
			return nil, &apimodels.OAuthErrorResponse{Error: "slow_down"}, nil
		}
		return &apimodels.OAuthTokenResponse{AccessToken: "at-1", RefreshToken: "rt-1", Scope: "read"}, nil, nil
	}

	_, err := pollDeviceTokenInternal(context.Background(), "https://example.com", "client-1", "device-1", time.Second, time.Minute, nil, clock.Now, clock.Sleep, exchange)
	require.NoError(t, err)
	require.Equal(t, []time.Duration{time.Second + defaultDevicePollBackoff}, clock.sleeps)
}

func TestPollDeviceTokenInternal_AccessDenied(t *testing.T) {
	exchange := func(_ context.Context, _, _, _ string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
		return nil, &apimodels.OAuthErrorResponse{Error: "access_denied"}, nil
	}

	_, err := pollDeviceTokenInternal(context.Background(), "https://example.com", "client-1", "device-1", time.Second, time.Minute, nil, time.Now, func(context.Context, time.Duration) error { return nil }, exchange)
	require.Error(t, err)
	require.Equal(t, oauthErrorDeviceAuthDenied, err.Error())
}

func TestPollDeviceTokenInternal_ExpiredToken(t *testing.T) {
	exchange := func(_ context.Context, _, _, _ string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
		return nil, &apimodels.OAuthErrorResponse{Error: "expired_token"}, nil
	}

	_, err := pollDeviceTokenInternal(context.Background(), "https://example.com", "client-1", "device-1", time.Second, time.Minute, nil, time.Now, func(context.Context, time.Duration) error { return nil }, exchange)
	require.Error(t, err)
	require.Equal(t, oauthErrorDeviceCodeExpired, err.Error())
}

func TestPollDeviceTokenInternal_InvalidGrant(t *testing.T) {
	exchange := func(_ context.Context, _, _, _ string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
		return nil, &apimodels.OAuthErrorResponse{Error: oauthErrorInvalidGrant}, nil
	}

	_, err := pollDeviceTokenInternal(context.Background(), "https://example.com", "client-1", "device-1", time.Second, time.Minute, nil, time.Now, func(context.Context, time.Duration) error { return nil }, exchange)
	require.Error(t, err)
	require.Equal(t, oauthErrorDeviceAuthInvalid, err.Error())
}

func TestPollDeviceTokenInternal_UnknownOAuthErrorUsesDescription(t *testing.T) {
	exchange := func(_ context.Context, _, _, _ string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
		return nil, &apimodels.OAuthErrorResponse{Error: "weird", ErrorDescription: "nope"}, nil
	}

	_, err := pollDeviceTokenInternal(context.Background(), "https://example.com", "client-1", "device-1", time.Second, time.Minute, nil, time.Now, func(context.Context, time.Duration) error { return nil }, exchange)
	require.Error(t, err)
	require.Equal(t, "nope (weird)", err.Error())
}

func TestPollDeviceTokenInternal_TTL(t *testing.T) {
	clock := &testClock{now: time.Date(2026, time.February, 11, 0, 0, 0, 0, time.UTC)}

	exchange := func(_ context.Context, _, _, _ string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
		return nil, &apimodels.OAuthErrorResponse{Error: "authorization_pending"}, nil
	}

	_, err := pollDeviceTokenInternal(context.Background(), "https://example.com", "client-1", "device-1", time.Second, 1*time.Second, nil, clock.Now, clock.Sleep, exchange)
	require.Error(t, err)
	require.Equal(t, oauthErrorDeviceAuthorizationTTL, err.Error())
}

func TestRefreshAccessToken_InvalidGrantRequiresReauth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"bad refresh token"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := refreshAccessToken(context.Background(), srv.URL, "client-1", "refresh-1")
	require.Error(t, err)
	require.Equal(t, oauthErrorRefreshReauthRequired, err.Error())
}

func TestPollDeviceTokenInternal_ErrorsWhenOAuthResponseMissing(t *testing.T) {
	exchange := func(_ context.Context, _, _, _ string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
		return nil, nil, nil
	}

	_, err := pollDeviceTokenInternal(context.Background(), "https://example.com", "client-1", "device-1", time.Second, time.Minute, nil, time.Now, func(context.Context, time.Duration) error { return nil }, exchange)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no oauth error")
}

func TestPollDeviceTokenInternal_UnknownOAuthErrorUsesDefaultDescription(t *testing.T) {
	exchange := func(_ context.Context, _, _, _ string) (*apimodels.OAuthTokenResponse, *apimodels.OAuthErrorResponse, error) {
		return nil, &apimodels.OAuthErrorResponse{Error: "WeIrD"}, nil
	}

	_, err := pollDeviceTokenInternal(context.Background(), "https://example.com", "client-1", "device-1", time.Second, time.Minute, nil, time.Now, func(context.Context, time.Duration) error { return nil }, exchange)
	require.Error(t, err)
	require.Equal(t, oauthErrorDescriptionDefault+" (weird)", err.Error())
}
