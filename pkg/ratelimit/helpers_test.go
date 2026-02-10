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

func TestApplyRateLimit_ReturnsNilWhenHandlerNil(t *testing.T) {
	require.Nil(t, ApplyRateLimit(nil, 10, time.Minute, zap.NewNop()))
}

func TestApplyRateLimit_UsesAWSDefaultRegionAndNilLoggerDoesNotPanic(t *testing.T) {
	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "us-west-2")

	newLimiterFunc = func(region string, limit int, window time.Duration, _ *zap.Logger) (atomicLimiter, error) {
		require.Equal(t, "us-west-2", region)
		require.Equal(t, 10, limit)
		require.Equal(t, time.Minute, window)
		return nil, nil
	}

	called := 0
	handler := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "ok"), nil
	}

	out := ApplyRateLimit(handler, 10, time.Minute, nil)
	resp, err := out(&apptheory.Context{Request: apptheory.Request{Method: "GET", Path: "/test"}})
	require.NoError(t, err)
	require.Equal(t, 1, called)
	require.Equal(t, 200, resp.Status)
}

func TestApplyRateLimit_FailOpenOnLimiterCheckError(t *testing.T) {
	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	newLimiterFunc = func(string, int, time.Duration, *zap.Logger) (atomicLimiter, error) {
		return stubLimiter{err: context.Canceled}, nil
	}

	called := 0
	out := ApplyRateLimit(func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "ok"), nil
	}, 10, time.Minute, zap.NewNop())

	resp, err := out(&apptheory.Context{Request: apptheory.Request{Method: "GET", Path: "/test"}})
	require.NoError(t, err)
	require.Equal(t, 1, called)
	require.Equal(t, 200, resp.Status)
}

func TestApplyRateLimit_PropagatesNilResponse(t *testing.T) {
	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	newLimiterFunc = func(string, int, time.Duration, *zap.Logger) (atomicLimiter, error) {
		return stubLimiter{decision: nil}, nil
	}

	out := ApplyRateLimit(func(*apptheory.Context) (*apptheory.Response, error) {
		return nil, nil
	}, 10, time.Minute, zap.NewNop())

	resp, err := out(&apptheory.Context{Request: apptheory.Request{Method: "GET", Path: "/test"}})
	require.NoError(t, err)
	require.Nil(t, resp)
}

func TestRateLimitKey_PriorityAndFallbacks(t *testing.T) {
	t.Run("nil_context", func(t *testing.T) {
		key := rateLimitKey(nil)
		require.Equal(t, "anonymous", key.Identifier)
		require.Equal(t, "/", key.Resource)
	})

	t.Run("user_has_priority_over_tenant_and_ip", func(t *testing.T) {
		ctx := &apptheory.Context{Request: apptheory.Request{
			Method:  "GET",
			Path:    "/test",
			Headers: map[string][]string{"x-forwarded-for": {"203.0.113.9"}},
		}}
		ctx.TenantID = "tenant"
		ctx.Set("username", " alice ")

		key := rateLimitKey(ctx)
		require.Equal(t, "/test", key.Resource)
		require.Equal(t, "GET", key.Operation)
		require.Equal(t, "user:alice", key.Identifier)
		require.Equal(t, "alice", key.Metadata["user_id"])
	})

	t.Run("tenant_fallback_when_no_user", func(t *testing.T) {
		ctx := &apptheory.Context{Request: apptheory.Request{
			Method:  "POST",
			Path:    "/test",
			Headers: map[string][]string{"x-forwarded-for": {"203.0.113.9"}},
		}}
		ctx.TenantID = " tenant "

		key := rateLimitKey(ctx)
		require.Equal(t, "tenant:tenant", key.Identifier)
		require.Equal(t, "tenant", key.Metadata["tenant_id"])
	})

	t.Run("ip_fallback_prefers_forwarded_for_then_real_ip_then_unknown", func(t *testing.T) {
		key := rateLimitKey(&apptheory.Context{Request: apptheory.Request{
			Method:  "GET",
			Path:    "/test",
			Headers: map[string][]string{"x-forwarded-for": {"203.0.113.9"}},
		}})
		require.Equal(t, "ip:203.0.113.9", key.Identifier)
		require.Equal(t, "203.0.113.9", key.Metadata["ip"])

		key = rateLimitKey(&apptheory.Context{Request: apptheory.Request{
			Method:  "GET",
			Path:    "/test",
			Headers: map[string][]string{"x-real-ip": {"198.51.100.7"}},
		}})
		require.Equal(t, "ip:198.51.100.7", key.Identifier)
		require.Equal(t, "198.51.100.7", key.Metadata["ip"])

		key = rateLimitKey(&apptheory.Context{Request: apptheory.Request{Method: "GET", Path: "/test"}})
		require.Equal(t, "ip:unknown", key.Identifier)
		require.Equal(t, "unknown", key.Metadata["ip"])
	})
}

func TestRateLimitHeaders_NilAndRemainingFloor(t *testing.T) {
	require.Nil(t, rateLimitHeaders(nil))

	headers := rateLimitHeaders(&apptheoryLimited.LimitDecision{
		Allowed:      true,
		CurrentCount: 15,
		Limit:        10,
		ResetsAt:     time.Unix(1700000000, 0),
	})
	require.Equal(t, []string{"10"}, headers["x-ratelimit-limit"])
	require.Equal(t, []string{"0"}, headers["x-ratelimit-remaining"])
	require.Equal(t, []string{"1700000000"}, headers["x-ratelimit-reset"])
}

func TestFirstHeaderValue_NormalizesKeyAndHandlesEmpty(t *testing.T) {
	require.Equal(t, "", firstHeaderValue(nil, "x-test"))
	require.Equal(t, "", firstHeaderValue(map[string][]string{}, "x-test"))

	headers := map[string][]string{"x-test": {"a", "b"}}
	require.Equal(t, "a", firstHeaderValue(headers, " X-TEST "))
}

func TestMaxInt(t *testing.T) {
	require.Equal(t, 2, maxInt(2, 1))
	require.Equal(t, 2, maxInt(1, 2))
}

func TestApplyRateLimit_SetsResponseHeadersMapWhenNil(t *testing.T) {
	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	newLimiterFunc = func(string, int, time.Duration, *zap.Logger) (atomicLimiter, error) {
		return stubLimiter{decision: nil}, nil
	}

	out := ApplyRateLimit(func(*apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{Status: 200}, nil
	}, 10, time.Minute, zap.NewNop())

	resp, err := out(&apptheory.Context{Request: apptheory.Request{Method: "GET", Path: "/test"}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Headers)
}

func TestNewLimiterFunc_EmptyRegionReturnsSentinel(t *testing.T) {
	limiter, err := newLimiterFunc("", 10, time.Minute, zap.NewNop())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, limiter)
}
