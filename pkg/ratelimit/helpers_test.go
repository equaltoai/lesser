package ratelimit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apptheoryLimited "github.com/theory-cloud/apptheory/pkg/limited"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

type stubLimiter struct {
	decision *apptheoryLimited.LimitDecision
	err      error
}

func (s stubLimiter) CheckAndIncrement(_ context.Context, _ apptheoryLimited.RateLimitKey) (*apptheoryLimited.LimitDecision, error) {
	return s.decision, s.err
}

func TestApplyRateLimit_FailOpenOnLimiterCreationError(t *testing.T) {
	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	newLimiterFunc = func(region string, limit int, window time.Duration, _ *zap.Logger) (atomicLimiter, error) {
		require.Equal(t, "us-east-1", region)
		require.Equal(t, 10, limit)
		require.Equal(t, time.Minute, window)
		return nil, context.Canceled
	}

	called := 0
	handler := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "ok"), nil
	}

	out := ApplyRateLimit(handler, 10, time.Minute, zap.NewNop())
	resp, err := out(&apptheory.Context{Request: apptheory.Request{Method: "GET", Path: "/test"}})
	require.NoError(t, err)
	require.Equal(t, 1, called)
	require.Equal(t, 200, resp.Status)
}

func TestApplyRateLimit_SetsHeadersOnAllowedRequests(t *testing.T) {
	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	newLimiterFunc = func(string, int, time.Duration, *zap.Logger) (atomicLimiter, error) {
		return stubLimiter{decision: &apptheoryLimited.LimitDecision{
			Allowed:      true,
			CurrentCount: 3,
			Limit:        10,
			ResetsAt:     time.Unix(1700000000, 0),
		}}, nil
	}

	out := ApplyRateLimit(func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(200, "ok"), nil
	}, 10, time.Minute, zap.NewNop())

	resp, err := out(&apptheory.Context{Request: apptheory.Request{
		Method:  "GET",
		Path:    "/test",
		Headers: map[string][]string{"x-forwarded-for": {"203.0.113.1"}},
	}})
	require.NoError(t, err)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, []string{"10"}, resp.Headers["x-ratelimit-limit"])
	require.Equal(t, []string{"7"}, resp.Headers["x-ratelimit-remaining"])
	require.Equal(t, []string{"1700000000"}, resp.Headers["x-ratelimit-reset"])
}

func TestApplyRateLimit_Returns429WhenLimited(t *testing.T) {
	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	retryAfter := 30 * time.Second
	newLimiterFunc = func(string, int, time.Duration, *zap.Logger) (atomicLimiter, error) {
		return stubLimiter{decision: &apptheoryLimited.LimitDecision{
			Allowed:      false,
			CurrentCount: 10,
			Limit:        10,
			ResetsAt:     time.Unix(1700000000, 0),
			RetryAfter:   &retryAfter,
		}}, nil
	}

	out := ApplyRateLimit(func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(200, "should not run"), nil
	}, 10, time.Minute, zap.NewNop())

	resp, err := out(&apptheory.Context{Request: apptheory.Request{Method: "POST", Path: "/oauth/token"}})
	require.NoError(t, err)
	require.Equal(t, 429, resp.Status)
	require.Equal(t, []string{"10"}, resp.Headers["x-ratelimit-limit"])
	require.Equal(t, []string{"0"}, resp.Headers["x-ratelimit-remaining"])
	require.Equal(t, []string{"1700000000"}, resp.Headers["x-ratelimit-reset"])
	require.Equal(t, []string{"30"}, resp.Headers["retry-after"])
	require.Contains(t, strings.ToLower(string(resp.Body)), "rate limit exceeded")
}
