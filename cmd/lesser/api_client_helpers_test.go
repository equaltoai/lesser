package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAPIClient_HelpersAndLimiters(t *testing.T) {
	require.Equal(t, 1, requestCost(http.MethodGet, "/api/v1/accounts/verify_credentials"))
	require.Equal(t, 3, requestCost(http.MethodPost, "/api/v1/search"))
	require.Equal(t, 2, requestCost(http.MethodGet, "/graphql"))

	require.Equal(t, time.Duration(0), backoffDuration(-1))
	require.Equal(t, 250*time.Millisecond, backoffDuration(0))
	require.Equal(t, 500*time.Millisecond, backoffDuration(1))
	require.Equal(t, 5*time.Second, backoffDuration(10))

	now := time.Unix(0, 0).UTC()
	require.Equal(t, time.Duration(0), retryAfterDuration("", now))
	require.Equal(t, time.Duration(0), retryAfterDuration("0", now))
	require.Equal(t, 2*time.Second, retryAfterDuration("2", now))
	future := now.Add(3 * time.Second).Format(http.TimeFormat)
	require.Equal(t, 3*time.Second, retryAfterDuration(future, now))
	past := now.Add(-3 * time.Second).Format(http.TimeFormat)
	require.Equal(t, time.Duration(0), retryAfterDuration(past, now))
	require.Equal(t, time.Duration(0), retryAfterDuration("not-a-date", now))

	tb, err := newTokenBucket(1000, 2)
	require.NoError(t, err)
	require.NoError(t, tb.Wait(context.Background(), 0))
	tb.Stop()
	tb.Stop()

	tb2, err := newTokenBucket(0.1, 1)
	require.NoError(t, err)
	require.NoError(t, tb2.Wait(context.Background(), 1))
	tb2.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, tb2.Wait(ctx, 1))

	_, err = newTokenBucket(0, 1)
	require.Error(t, err)

	sem := newSemaphore(0)
	require.NoError(t, sem.Acquire(context.Background()))
	sem.Release()
	sem.Release()

	l, err := newClientLimiter(1, 0, 1)
	require.NoError(t, err)
	require.NoError(t, l.Acquire(context.Background(), 0))
	l.Release()
	l.Close()

	var nilLimiter *clientLimiter
	require.NoError(t, nilLimiter.Acquire(context.Background(), 1))
	nilLimiter.Release()
	nilLimiter.Close()

	var nilTB *tokenBucket
	nilTB.Stop()
	require.NoError(t, nilTB.Wait(context.Background(), 1))

	tb3, err := newTokenBucket(1, 0)
	require.NoError(t, err)
	tb3.Stop()

	tb4, err := newTokenBucket(1e18, 1)
	require.NoError(t, err)
	tb4.Stop()

	limiter2, err := newClientLimiter(1, 1, 1)
	require.NoError(t, err)
	require.NotNil(t, limiter2.tb)
	limiter2.Close()
}
