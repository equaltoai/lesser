package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestApplyRateLimit_FailOpenOnLimiterError(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("RATE_LIMIT_TABLE_NAME", "")
	t.Setenv("LIMITED_TABLE_NAME", "")

	orig := limitedRateLimitFunc
	t.Cleanup(func() { limitedRateLimitFunc = orig })

	limitedRateLimitFunc = func(cfg liftMiddleware.LimitedConfig) (lift.Middleware, error) {
		require.Equal(t, "us-east-1", cfg.Region)
		require.Equal(t, "rate-limits", cfg.TableName)
		return nil, errors.New("boom")
	}

	called := 0
	handler := lift.HandlerFunc(func(_ *lift.Context) error {
		called++
		return nil
	})

	out := ApplyRateLimit(handler, 10, time.Minute, zap.NewNop())

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{TriggerType: adapters.TriggerAPIGatewayV2}))
	require.NoError(t, out.Handle(ctx))
	require.Equal(t, 1, called)
}

func TestApplyRateLimit_WrapsHandlerWhenLimiterSucceeds(t *testing.T) {
	orig := limitedRateLimitFunc
	t.Cleanup(func() { limitedRateLimitFunc = orig })

	wrappedCalls := 0
	limitedRateLimitFunc = func(_ liftMiddleware.LimitedConfig) (lift.Middleware, error) {
		return lift.Middleware(func(next lift.Handler) lift.Handler {
			return lift.HandlerFunc(func(ctx *lift.Context) error {
				wrappedCalls++
				return next.Handle(ctx)
			})
		}), nil
	}

	called := 0
	handler := lift.HandlerFunc(func(_ *lift.Context) error {
		called++
		return nil
	})

	out := ApplyRateLimit(handler, 10, time.Minute, zap.NewNop())
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{TriggerType: adapters.TriggerAPIGatewayV2}))
	require.NoError(t, out.Handle(ctx))
	require.Equal(t, 1, called)
	require.Equal(t, 1, wrappedCalls)
}

func TestLiftRateLimitMiddleware_CachesLimitersAndFailsOpen(t *testing.T) {
	orig := limitedRateLimitFunc
	t.Cleanup(func() { limitedRateLimitFunc = orig })

	cacheMutex.Lock()
	rateLimiterCache = make(map[string]lift.Middleware)
	cacheMutex.Unlock()

	created := 0
	limitedRateLimitFunc = func(_ liftMiddleware.LimitedConfig) (lift.Middleware, error) {
		created++
		return lift.Middleware(func(next lift.Handler) lift.Handler {
			return lift.HandlerFunc(func(ctx *lift.Context) error {
				return next.Handle(ctx)
			})
		}), nil
	}

	logger := zap.NewNop()
	mw := LiftRateLimitMiddleware(logger)

	nextCalls := 0
	next := lift.HandlerFunc(func(_ *lift.Context) error {
		nextCalls++
		return nil
	})

	handler := mw(next)
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      "POST",
		Path:        "/oauth/token",
		TriggerType: adapters.TriggerAPIGatewayV2,
	}))

	require.NoError(t, handler.Handle(ctx))
	require.NoError(t, handler.Handle(ctx))
	require.Equal(t, 2, nextCalls)
	require.Equal(t, 1, created)

	// Unlisted endpoints should skip limiter entirely.
	created = 0
	nextCalls = 0
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      "GET",
		Path:        "/health",
		TriggerType: adapters.TriggerAPIGatewayV2,
	}))
	require.NoError(t, handler.Handle(ctx))
	require.Equal(t, 1, nextCalls)
	require.Equal(t, 0, created)

	// Limiter creation failure should fail open.
	limitedRateLimitFunc = func(_ liftMiddleware.LimitedConfig) (lift.Middleware, error) {
		return nil, errors.New("boom")
	}
	cacheMutex.Lock()
	rateLimiterCache = make(map[string]lift.Middleware)
	cacheMutex.Unlock()

	nextCalls = 0
	ctx = lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{
		Method:      "POST",
		Path:        "/oauth/token",
		TriggerType: adapters.TriggerAPIGatewayV2,
	}))
	require.NoError(t, handler.Handle(ctx))
	require.Equal(t, 1, nextCalls)
}
