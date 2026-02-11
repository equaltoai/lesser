package main

import (
	"context"
	stdErrors "errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCLIAPIClient_New_NotLoggedIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LESSER_AUTH_SECRET", "test-secret")

	baseURL := "https://example.com"
	key := deriveAuthKey(baseURL, "test-secret")

	_, err := newCLIAPIClient(baseURL, key, cliAPIClientOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not logged in")
}

func TestCLIAPIClient_RefreshTokens_ErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErr    string
		wantErrSub string
	}{
		{
			name:    "invalid_grant requires reauth",
			status:  http.StatusBadRequest,
			body:    `{"error":"invalid_grant","error_description":"bad refresh token"}`,
			wantErr: oauthErrorRefreshReauthRequired,
		},
		{
			name:       "oauth error formats description",
			status:     http.StatusBadRequest,
			body:       `{"error":"invalid_request","error_description":"nope"}`,
			wantErrSub: "nope (invalid_request)",
		},
		{
			name:       "plain error is surfaced",
			status:     http.StatusBadRequest,
			body:       "plain",
			wantErrSub: "token refresh failed (400)",
		},
		{
			name:       "decode error on 200",
			status:     http.StatusOK,
			body:       "not-json",
			wantErrSub: "decode token refresh response",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/oauth/token" {
					http.NotFound(w, r)
					return
				}
				require.NoError(t, r.ParseForm())
				require.Equal(t, "refresh_token", r.FormValue("grant_type"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			limiter, err := newClientLimiter(1, 0, 1)
			require.NoError(t, err)

			client := &cliAPIClient{
				baseURL:     srv.URL,
				httpClient:  &http.Client{Timeout: 2 * time.Second},
				limiter:     limiter,
				retries:     0,
				nowFn:       time.Now,
				sleepFn:     func(time.Duration) {},
				accessToken: "",
			}
			defer client.Close()

			_, err = client.refreshTokens(context.Background(), "client-1", "refresh-1")
			require.Error(t, err)
			if tc.wantErr != "" {
				require.Equal(t, tc.wantErr, err.Error())
			}
			if tc.wantErrSub != "" {
				require.Contains(t, err.Error(), tc.wantErrSub)
			}
		})
	}
}

func TestCLIAPIClient_DoRequestWithRetries_ReturnsLastErr(t *testing.T) {
	type errorTransport struct {
		calls atomic.Int32
	}

	var rt errorTransport
	client := &cliAPIClient{
		baseURL: "http://example.com",
		httpClient: &http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				rt.calls.Add(1)
				return nil, stdErrors.New("boom")
			}),
		},
		retries: 1,
		limiter: &clientLimiter{sem: newSemaphore(1)},
		nowFn:   time.Now,
		sleepFn: func(time.Duration) {},
	}

	_, _, _, err := client.doRequestWithRetries(context.Background(), http.MethodGet, "/api/test", nil, nil)
	require.Error(t, err)
	require.Equal(t, int32(2), rt.calls.Load())

	_, _, _, err = client.doRequestWithRetries(context.Background(), "", "/api/test", nil, nil)
	require.Error(t, err)
	_, _, _, err = client.doRequestWithRetries(context.Background(), http.MethodGet, "", nil, nil)
	require.Error(t, err)
	_, _, _, err = client.doRequestWithRetries(context.Background(), http.MethodGet, "api/test", nil, nil)
	require.Error(t, err)
}

func TestClientLimiter_Acquire_ReleasesSemaphoreOnTokenError(t *testing.T) {
	l, err := newClientLimiter(1, 0.1, 1)
	require.NoError(t, err)

	require.NotNil(t, l.tb)
	require.NoError(t, l.tb.Wait(context.Background(), 1))
	l.tb.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- l.Acquire(ctx, 1)
	}()
	time.Sleep(5 * time.Millisecond)
	cancel()
	require.Error(t, <-done)

	// If semaphore wasn't released, this would block until timeout.
	semCtx, semCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer semCancel()
	require.NoError(t, l.sem.Acquire(semCtx))
	l.sem.Release()
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
