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

func TestCLIAPIClient_DoRequestWithRetries_DefaultRetryAfter(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/test" {
			http.NotFound(w, r)
			return
		}
		call := calls.Add(1)
		if call == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	var slept time.Duration
	client := &cliAPIClient{
		baseURL:     srv.URL,
		httpClient:  &http.Client{Timeout: 2 * time.Second},
		limiter:     &clientLimiter{sem: newSemaphore(1)},
		retries:     1,
		nowFn:       func() time.Time { return time.Unix(0, 0).UTC() },
		sleepFn:     func(d time.Duration) { slept += d },
		accessToken: "access-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	status, _, body, err := client.doRequestWithRetries(ctx, http.MethodGet, "/api/test", nil, nil)
	require.NoError(t, err)
	require.Equal(t, 200, status)
	require.Equal(t, "ok", string(body))
	require.Equal(t, int32(2), calls.Load())
	require.Equal(t, 5*time.Second, slept)
}

func TestCLIAPIClient_DoRequestWithRetries_BackoffOn5xx(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/test" {
			http.NotFound(w, r)
			return
		}
		call := calls.Add(1)
		if call == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	var slept time.Duration
	client := &cliAPIClient{
		baseURL:     srv.URL,
		httpClient:  &http.Client{Timeout: 2 * time.Second},
		limiter:     &clientLimiter{sem: newSemaphore(1)},
		retries:     1,
		nowFn:       time.Now,
		sleepFn:     func(d time.Duration) { slept += d },
		accessToken: "access-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	status, _, body, err := client.doRequestWithRetries(ctx, http.MethodGet, "/api/test", nil, nil)
	require.NoError(t, err)
	require.Equal(t, 200, status)
	require.Equal(t, "ok", string(body))
	require.Equal(t, int32(2), calls.Load())
	require.Equal(t, 250*time.Millisecond, slept)
}
