package crawler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apptheoryLimited "github.com/theory-cloud/apptheory/pkg/limited"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	tablecore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

type stubLimiter struct {
	decision *apptheoryLimited.LimitDecision
	err      error
	calls    int
	lastKey  apptheoryLimited.RateLimitKey
}

func (s *stubLimiter) CheckAndIncrement(_ context.Context, key apptheoryLimited.RateLimitKey) (*apptheoryLimited.LimitDecision, error) {
	s.calls++
	s.lastKey = key
	return s.decision, s.err
}

type stubDB struct{}

func (stubDB) Model(any) tablecore.Query {
	return nil
}

func (stubDB) Transaction(fn func(tx *tablecore.Tx) error) error {
	if fn == nil {
		return nil
	}
	return fn(&tablecore.Tx{})
}

func (stubDB) Migrate() error {
	return nil
}

func (stubDB) AutoMigrate(...any) error {
	return nil
}

func (stubDB) Close() error {
	return nil
}

func (stubDB) WithContext(context.Context) tablecore.DB {
	return stubDB{}
}

func TestParseProtectionMode(t *testing.T) {
	require.Equal(t, protectionModeOff, parseProtectionMode(""))
	require.Equal(t, protectionModeOff, parseProtectionMode("nope"))
	require.Equal(t, protectionModeObserve, parseProtectionMode("observe"))
	require.Equal(t, protectionModeObserve, parseProtectionMode(" OBSERVE "))
	require.Equal(t, protectionModeLimit, parseProtectionMode("limit"))
	require.Equal(t, protectionModeBlock, parseProtectionMode("block"))
}

func TestMiddleware_Off_IsNoop(t *testing.T) {
	called := 0
	next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
		called++
		require.NotNil(t, ctx)
		require.Nil(t, ctx.Get(contextCrawlerCategoryKey))
		require.Nil(t, ctx.Get(contextCrawlerReasonKey))
		return apptheory.Text(200, "ok"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method:  "GET",
		Path:    "/users/alice",
		Headers: map[string][]string{"user-agent": {"GPTBot/1.0"}, "accept": {"application/activity+json"}},
	}}

	mw := Middleware(protectionModeOff, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)
}

func TestNewMiddleware_UsesEnv(t *testing.T) {
	t.Setenv("CRAWLER_PROTECTION_MODE", "observe")

	called := 0
	next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
		called++
		require.Equal(t, "ai_crawler", ctx.Get(contextCrawlerCategoryKey))
		require.Equal(t, "ua:gptbot", ctx.Get(contextCrawlerReasonKey))
		return apptheory.Text(200, "ok"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method:  "GET",
		Path:    "/users/alice",
		Headers: map[string][]string{"user-agent": {"GPTBot/1.0"}, "accept": {"application/activity+json"}},
	}}

	mw := NewMiddleware(nil)
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)
}

func TestMiddleware_Observe_SetsContext(t *testing.T) {
	called := 0
	next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
		called++
		require.Equal(t, "ai_crawler", ctx.Get(contextCrawlerCategoryKey))
		require.Equal(t, "ua:gptbot", ctx.Get(contextCrawlerReasonKey))
		return apptheory.Text(200, "ok"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method:  "GET",
		Path:    "/users/alice",
		Headers: map[string][]string{"user-agent": {"GPTBot/1.0"}, "accept": {"application/activity+json"}},
	}}

	mw := Middleware(protectionModeObserve, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)
}

func TestMiddleware_NilContext(t *testing.T) {
	called := 0
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "ok"), nil
	}

	mw := Middleware(protectionModeObserve, zap.NewNop())
	resp, err := mw(next)(nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)
}

func TestMiddleware_Block_BlocksAICrawler(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")

	called := 0
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "should not run"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method:  "GET",
		Path:    "/users/alice",
		Headers: map[string][]string{"user-agent": {"GPTBot/1.0"}, "accept": {"application/activity+json"}},
	}}

	mw := Middleware(protectionModeBlock, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 403, resp.Status)
	require.Contains(t, string(resp.Body), "/robots.txt")
	require.Equal(t, []string{"no-store"}, resp.Headers["cache-control"])
	require.Equal(t, 0, called)
}

func TestMiddleware_Block_BypassCIDR_AllowsAICrawler(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	t.Setenv("CRAWLER_PROTECTION_BYPASS_CIDRS", "203.0.113.0/24")

	called := 0
	next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
		called++
		require.Equal(t, "ai_crawler", ctx.Get(contextCrawlerCategoryKey))
		require.Equal(t, "ua:gptbot", ctx.Get(contextCrawlerReasonKey))
		return apptheory.Text(200, "ok"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method: "GET",
		Path:   "/users/alice",
		Headers: map[string][]string{
			"user-agent":      {"GPTBot/1.0"},
			"accept":          {"application/activity+json"},
			"x-forwarded-for": {"203.0.113.10"},
		},
	}}

	mw := Middleware(protectionModeBlock, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)
}

func TestParseCIDRAllowlistFromEnv(t *testing.T) {
	t.Setenv("CRAWLER_PROTECTION_BYPASS_CIDRS", " 203.0.113.0/24, ,not-a-cidr,203.0.113.9,not-an-ip ")

	nets := parseCIDRAllowlistFromEnv(nil)
	require.NotEmpty(t, nets)
	require.True(t, isClientIPBypassed("203.0.113.10", nets))
	require.False(t, isClientIPBypassed(unknownString, nets))
}

func TestNewLimiterFunc(t *testing.T) {
	originalLimiterDBOnce := limiterDBOnce
	originalLimiterDB := limiterDB
	originalLimiterDBErr := limiterDBErr
	originalNewLimiterDBFunc := newLimiterDBFunc

	t.Cleanup(func() {
		limiterDBOnce = originalLimiterDBOnce
		limiterDB = originalLimiterDB
		limiterDBErr = originalLimiterDBErr
		newLimiterDBFunc = originalNewLimiterDBFunc
	})

	limiter, err := newLimiterFunc("", 1, time.Hour, zap.NewNop())
	require.Error(t, err)
	require.Nil(t, limiter)

	limiterDBOnce = sync.Once{}
	limiterDB = nil
	limiterDBErr = nil
	newLimiterDBFunc = func(string) (tablecore.DB, error) {
		return nil, errors.New("db error")
	}
	limiter, err = newLimiterFunc("us-east-1", 1, time.Hour, zap.NewNop())
	require.Error(t, err)
	require.Nil(t, limiter)

	limiterDBOnce = sync.Once{}
	limiterDB = nil
	limiterDBErr = nil
	newLimiterDBFunc = func(string) (tablecore.DB, error) {
		return nil, nil
	}
	limiter, err = newLimiterFunc("us-east-1", 1, time.Hour, zap.NewNop())
	require.Error(t, err)
	require.Nil(t, limiter)

	limiterDBOnce = sync.Once{}
	limiterDB = nil
	limiterDBErr = nil
	newLimiterDBFunc = func(string) (tablecore.DB, error) {
		return stubDB{}, nil
	}
	limiter, err = newLimiterFunc("us-east-1", 0, 0, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, limiter)

	limiterDBOnce = sync.Once{}
	limiterDB = nil
	limiterDBErr = nil
	newLimiterDBFunc = func(string) (tablecore.DB, error) {
		return stubDB{}, nil
	}
	limiter, err = newLimiterFunc("us-east-1", 1, time.Nanosecond, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, limiter)
}

func TestMiddleware_Limit_Returns429AndUsesStableKey(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "false")

	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	retryAfter := 30 * time.Second
	searchLimiter := &stubLimiter{decision: &apptheoryLimited.LimitDecision{
		Allowed:      true,
		CurrentCount: 1,
		Limit:        100,
		ResetsAt:     time.Unix(1700000000, 0),
	}}
	botLimiter := &stubLimiter{decision: &apptheoryLimited.LimitDecision{
		Allowed:      false,
		CurrentCount: 30,
		Limit:        30,
		ResetsAt:     time.Unix(1700000000, 0),
		RetryAfter:   &retryAfter,
	}}

	newLimiterFunc = func(_ string, limit int, _ time.Duration, _ *zap.Logger) (atomicLimiter, error) {
		switch limit {
		case 100:
			return searchLimiter, nil
		case 30:
			return botLimiter, nil
		default:
			return nil, context.Canceled
		}
	}

	called := 0
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "should not run"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method: "GET",
		Path:   "/users/alice",
		Headers: map[string][]string{
			"user-agent":        {"ExampleScraperBot/1.0"},
			"accept":            {"text/html"},
			"x-forwarded-for":   {"203.0.113.1, 70.0.0.1"},
			"x-forwarded-proto": {"https"},
		},
	}}

	mw := Middleware(protectionModeLimit, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 429, resp.Status)
	require.Equal(t, 0, called)

	require.Equal(t, 1, botLimiter.calls)
	require.Equal(t, "crawler:federation", botLimiter.lastKey.Resource)
	require.Equal(t, "generic_bot", botLimiter.lastKey.Operation)
	require.Equal(t, "ip:203.0.113.1", botLimiter.lastKey.Identifier)

	require.Equal(t, []string{"30"}, resp.Headers["x-ratelimit-limit"])
	require.Equal(t, []string{"0"}, resp.Headers["x-ratelimit-remaining"])
	require.Equal(t, []string{"1700000000"}, resp.Headers["x-ratelimit-reset"])
	require.Equal(t, []string{"30"}, resp.Headers["retry-after"])
}

func TestBuildLimiters_FailOpenOnNewLimiterError(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "false")

	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	newLimiterFunc = func(_ string, _ int, _ time.Duration, _ *zap.Logger) (atomicLimiter, error) {
		return nil, errors.New("boom")
	}

	limiters := buildLimiters(protectionModeLimit, zap.NewNop())
	require.Empty(t, limiters)
}

func TestMiddleware_Limit_AllowsAndAddsHeaders(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "false")

	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	limiter := &stubLimiter{decision: &apptheoryLimited.LimitDecision{
		Allowed:      true,
		CurrentCount: 3,
		Limit:        100,
		ResetsAt:     time.Unix(1700000000, 0),
	}}

	newLimiterFunc = func(_ string, _ int, _ time.Duration, _ *zap.Logger) (atomicLimiter, error) {
		return limiter, nil
	}

	called := 0
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "ok"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method: "GET",
		Path:   "/api/v1/instance",
		Headers: map[string][]string{
			"user-agent":      {"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
			"accept":          {"text/html"},
			"x-forwarded-for": {"203.0.113.2"},
		},
	}}

	mw := Middleware(protectionModeLimit, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)

	require.Equal(t, 1, limiter.calls)
	require.Equal(t, "crawler:api", limiter.lastKey.Resource)
	require.Equal(t, "search_engine", limiter.lastKey.Operation)
	require.Equal(t, "google:ip:203.0.113.2", limiter.lastKey.Identifier)

	require.Equal(t, []string{"100"}, resp.Headers["x-ratelimit-limit"])
	require.Equal(t, []string{"97"}, resp.Headers["x-ratelimit-remaining"])
	require.Equal(t, []string{"1700000000"}, resp.Headers["x-ratelimit-reset"])
}

func TestRouteClassForPath(t *testing.T) {
	require.Equal(t, "api", routeClassForPath("/api/v1/instance"))
	require.Equal(t, "api", routeClassForPath("/api/v2/instance"))
	require.Equal(t, "auth", routeClassForPath("/oauth/token"))
	require.Equal(t, "auth", routeClassForPath("/auth/wallet/login"))
	require.Equal(t, "auth", routeClassForPath("/setup/status"))
	require.Equal(t, "federation", routeClassForPath("/users/alice"))
	require.Equal(t, "federation", routeClassForPath("/objects/123"))
	require.Equal(t, "protocol", routeClassForPath("/.well-known/webfinger"))
	require.Equal(t, "protocol", routeClassForPath("/nodeinfo/2.0"))
	require.Equal(t, "robots", routeClassForPath("/robots.txt"))
	require.Equal(t, "other", routeClassForPath("/"))
}

func TestExtractClientIP(t *testing.T) {
	require.Equal(t, unknownString, extractClientIP(nil))

	xff := &apptheory.Context{Request: apptheory.Request{Headers: map[string][]string{
		"x-forwarded-for": {"203.0.113.1, 70.0.0.1"},
	}}}
	require.Equal(t, "203.0.113.1", extractClientIP(xff))

	xri := &apptheory.Context{Request: apptheory.Request{Headers: map[string][]string{
		"x-real-ip": {"203.0.113.9"},
	}}}
	require.Equal(t, "203.0.113.9", extractClientIP(xri))

	unknown := &apptheory.Context{Request: apptheory.Request{Headers: map[string][]string{}}}
	require.Equal(t, unknownString, extractClientIP(unknown))
}

func TestRateLimitHeaders(t *testing.T) {
	require.Nil(t, rateLimitHeaders(nil))

	headers := rateLimitHeaders(&apptheoryLimited.LimitDecision{
		Allowed:      true,
		CurrentCount: 3,
		Limit:        10,
		ResetsAt:     time.Unix(1700000000, 0),
	})
	require.Equal(t, []string{"10"}, headers["x-ratelimit-limit"])
	require.Equal(t, []string{"7"}, headers["x-ratelimit-remaining"])
	require.Equal(t, []string{"1700000000"}, headers["x-ratelimit-reset"])

	headers = rateLimitHeaders(&apptheoryLimited.LimitDecision{
		Allowed:      false,
		CurrentCount: 15,
		Limit:        10,
		ResetsAt:     time.Unix(1700000000, 0),
	})
	require.Equal(t, []string{"0"}, headers["x-ratelimit-remaining"])
}

func TestBuildRateLimitKey(t *testing.T) {
	key := buildRateLimitKey(CategoryGenericBot, "", "", "")
	require.Equal(t, "crawler:other", key.Resource)
	require.Equal(t, "generic_bot", key.Operation)
	require.Equal(t, "ip:"+unknownString, key.Identifier)

	key = buildRateLimitKey(
		CategorySearchEngine,
		"api",
		"203.0.113.2",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
	)
	require.Equal(t, "crawler:api", key.Resource)
	require.Equal(t, "search_engine", key.Operation)
	require.Equal(t, "google:ip:203.0.113.2", key.Identifier)
}

func TestIdentifySearchEngine(t *testing.T) {
	require.Equal(t, "google", identifySearchEngine("Googlebot/2.1"))
	require.Equal(t, unknownString, identifySearchEngine("Mozilla/5.0"))
}

func TestBuildLimiters(t *testing.T) {
	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	t.Setenv("DISABLE_RATE_LIMITING", "true")
	require.Nil(t, buildLimiters(protectionModeLimit, nil))

	t.Setenv("DISABLE_RATE_LIMITING", "false")
	require.Nil(t, buildLimiters(protectionModeObserve, nil))

	calls := 0
	newLimiterFunc = func(_ string, _ int, _ time.Duration, _ *zap.Logger) (atomicLimiter, error) {
		calls++
		return &stubLimiter{decision: &apptheoryLimited.LimitDecision{Allowed: true, Limit: 1}}, nil
	}

	limiters := buildLimiters(protectionModeLimit, nil)
	require.Len(t, limiters, 2)
	require.Equal(t, 2, calls)
}

func TestBuildLimiters_BlockMode_IncludesSuspicious(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "false")

	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	calls := 0
	newLimiterFunc = func(_ string, _ int, _ time.Duration, _ *zap.Logger) (atomicLimiter, error) {
		calls++
		return &stubLimiter{decision: &apptheoryLimited.LimitDecision{Allowed: true, Limit: 1}}, nil
	}

	limiters := buildLimiters(protectionModeBlock, nil)
	require.Len(t, limiters, 3)
	require.Equal(t, 3, calls)
}
