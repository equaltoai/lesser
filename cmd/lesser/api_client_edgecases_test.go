package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCLIAPIClient_DoRequestWithRetries_AttemptsZero(t *testing.T) {
	client := &cliAPIClient{
		baseURL:     "http://example.com",
		httpClient:  &http.Client{Timeout: time.Second},
		limiter:     &clientLimiter{sem: newSemaphore(1)},
		retries:     -1,
		nowFn:       time.Now,
		sleepFn:     func(time.Duration) {},
		accessToken: "access-1",
	}

	_, _, _, err := client.doRequestWithRetries(context.Background(), http.MethodGet, "/api/test", nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "request failed")
}

func TestCLIAPIClient_DoRequestWithRetries_LastAttemptReturnsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	client := &cliAPIClient{
		baseURL:     srv.URL,
		httpClient:  &http.Client{Timeout: time.Second},
		limiter:     &clientLimiter{sem: newSemaphore(1)},
		retries:     0,
		nowFn:       time.Now,
		sleepFn:     func(time.Duration) {},
		accessToken: "access-1",
	}

	status, _, _, err := client.doRequestWithRetries(context.Background(), http.MethodGet, "/api/test", nil, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, status)
}

func TestCLIAPIClient_DoRequestWithRetries_LimiterAcquireError(t *testing.T) {
	limiter := &clientLimiter{sem: newSemaphore(1)}
	require.NoError(t, limiter.sem.Acquire(context.Background()))
	defer limiter.sem.Release()

	client := &cliAPIClient{
		baseURL:     "http://example.com",
		httpClient:  &http.Client{Timeout: time.Second},
		limiter:     limiter,
		retries:     0,
		nowFn:       time.Now,
		sleepFn:     func(time.Duration) {},
		accessToken: "access-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, _, _, err := client.doRequestWithRetries(ctx, http.MethodGet, "/api/test", nil, nil)
	require.Error(t, err)
}

func TestCLIAPIClient_RefreshTokens_EmptyBodyUsesStatusText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	limiter, err := newClientLimiter(1, 0, 1)
	require.NoError(t, err)

	client := &cliAPIClient{
		baseURL:    srv.URL,
		httpClient: &http.Client{Timeout: time.Second},
		limiter:    limiter,
		retries:    0,
		nowFn:      time.Now,
		sleepFn:    func(time.Duration) {},
	}
	defer client.Close()

	_, err = client.refreshTokens(context.Background(), "client-1", "refresh-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Internal Server Error")
}
