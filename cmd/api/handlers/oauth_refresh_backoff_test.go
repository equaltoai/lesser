package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRefreshCASBackoffScheduleIsExponentialAndCapped pins the retry schedule the
// compare-and-set refresh path walks: the delay doubles per attempt and stops at
// the configured ceiling, so a contended refresh cannot back off without bound.
func TestRefreshCASBackoffScheduleIsExponentialAndCapped(t *testing.T) {
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})

	var caps []time.Duration
	handler.oauthRefreshJitter = func(delayCap time.Duration) time.Duration {
		caps = append(caps, delayCap)
		return delayCap
	}
	var slept []time.Duration
	handler.oauthRefreshSleep = func(_ context.Context, delay time.Duration) error {
		slept = append(slept, delay)
		return nil
	}

	for attempt := 0; attempt < 5; attempt++ {
		require.NoError(t, handler.refreshCASBackoff(context.Background(), attempt))
	}

	assert.Equal(t, []time.Duration{
		oauthRefreshCASBaseDelay,
		oauthRefreshCASBaseDelay * 2,
		oauthRefreshCASBaseDelay * 4,
		oauthRefreshCASDelayCap,
		oauthRefreshCASDelayCap,
	}, caps, "each attempt doubles the delay until the cap is reached")
	assert.Equal(t, caps, slept, "the sleep receives exactly the jittered delay")

	// The production wiring is the unset case: a nil jitter and sleep fall back to
	// the real jitter and timer, and the attempt still completes.
	handler.oauthRefreshJitter = nil
	handler.oauthRefreshSleep = nil
	require.NoError(t, handler.refreshCASBackoff(context.Background(), 1))
}

// TestFullJitterDurationStaysWithinTheDelayCap checks the property the refresh
// backoff relies on: jitter spreads retries over [0, cap] and never exceeds it,
// and a non-positive cap yields no delay at all.
func TestFullJitterDurationStaysWithinTheDelayCap(t *testing.T) {
	assert.Zero(t, fullJitterDuration(0))
	assert.Zero(t, fullJitterDuration(-time.Second))

	const cap = 50 * time.Millisecond
	for i := 0; i < 256; i++ {
		got := fullJitterDuration(cap)
		require.GreaterOrEqual(t, got, time.Duration(0))
		require.LessOrEqual(t, got, cap)
	}
}

// TestSleepWithContextReturnsEarlyWhenTheContextIsDone pins that a cancelled
// refresh stops waiting instead of holding the request open for the full delay.
func TestSleepWithContextReturnsEarlyWhenTheContextIsDone(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	require.ErrorIs(t, sleepWithContext(cancelled, time.Hour), context.Canceled)
	assert.Less(t, time.Since(start), time.Minute, "a cancelled context must not wait out the delay")

	expired, expire := context.WithTimeout(context.Background(), time.Millisecond)
	defer expire()
	require.ErrorIs(t, sleepWithContext(expired, time.Hour), context.DeadlineExceeded)

	require.NoError(t, sleepWithContext(context.Background(), time.Millisecond))
}
