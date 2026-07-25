package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
	apptheoryLimited "github.com/theory-cloud/apptheory/v2/pkg/limited"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	tablecore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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

func providerSourceIP(sourceIP string) apptheory.SourceProvenance {
	return apptheory.SourceProvenance{
		SourceIP: sourceIP,
		Provider: "apigw-v2",
		Source:   "provider_request_context",
		Valid:    true,
	}
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

func TestMiddleware_Observe_SanitizesPrivateMintConversationPathInClassificationLogs(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	rawConversationID := "conv-crawler-private"
	rawCursor := "raw-crawler-cursor"
	ctx := &apptheory.Context{Request: apptheory.Request{
		Method: "GET",
		Path:   "/api/v1/souls/bound/me/mint-conversations/" + rawConversationID + "?cursor=" + rawCursor,
		Headers: map[string][]string{
			"user-agent": {"GPTBot/1.0"},
			"accept":     {"application/activity+json"},
		},
	}}

	resp, err := Middleware(protectionModeObserve, logger)(func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(200, "ok"), nil
	})(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)

	entries := observed.FilterMessage("crawler classification").All()
	require.Len(t, entries, 1)
	path, ok := entries[0].ContextMap()["path"].(string)
	require.True(t, ok)
	require.Contains(t, path, "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:")
	require.NotContains(t, path, rawConversationID)
	require.NotContains(t, path, rawCursor)
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
	t.Setenv("CRAWLER_TRUSTED_PROXY_CIDRS", "70.0.0.0/8")

	called := 0
	next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
		called++
		require.Equal(t, "ai_crawler", ctx.Get(contextCrawlerCategoryKey))
		require.Equal(t, "ua:gptbot", ctx.Get(contextCrawlerReasonKey))
		return apptheory.Text(200, "ok"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method:           "GET",
		Path:             "/users/alice",
		SourceProvenance: providerSourceIP("70.0.0.2"),
		Headers: map[string][]string{
			"user-agent":      {"GPTBot/1.0"},
			"accept":          {"application/activity+json"},
			"x-forwarded-for": {"203.0.113.10, 70.0.0.1"},
		},
	}}

	mw := Middleware(protectionModeBlock, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)
}

func TestMiddleware_Block_SpoofedTrustedProxyChainDoesNotBypass(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	t.Setenv("CRAWLER_PROTECTION_BYPASS_CIDRS", "203.0.113.0/24")
	t.Setenv("CRAWLER_TRUSTED_PROXY_CIDRS", "70.0.0.0/8")

	called := 0
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "should not run"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method: "GET",
		Path:   "/users/alice",
		Headers: map[string][]string{
			"user-agent":      {"GPTBot/1.0"},
			"accept":          {"application/activity+json"},
			"x-forwarded-for": {"203.0.113.10, 70.0.0.1"},
		},
	}}

	resp, err := Middleware(protectionModeBlock, zap.NewNop())(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 403, resp.Status)
	require.Equal(t, 0, called)
}

func TestMiddleware_Block_SpoofedBypassCIDRDoesNotBypass(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	t.Setenv("CRAWLER_PROTECTION_BYPASS_CIDRS", "203.0.113.0/24")

	called := 0
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "should not run"), nil
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
	require.Equal(t, 403, resp.Status)
	require.Equal(t, 0, called)
}

func TestMiddleware_Block_SpoofedTrustedProxyWithoutClientDoesNotBypass(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	t.Setenv("CRAWLER_PROTECTION_BYPASS_CIDRS", "203.0.113.0/24")
	t.Setenv("CRAWLER_TRUSTED_PROXY_CIDRS", "70.0.0.0/8")

	called := 0
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "should not run"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method: "GET",
		Path:   "/users/alice",
		Headers: map[string][]string{
			"user-agent":      {"GPTBot/1.0"},
			"accept":          {"application/activity+json"},
			"x-forwarded-for": {"70.0.0.1"},
			"x-real-ip":       {"203.0.113.10"},
		},
	}}

	mw := Middleware(protectionModeBlock, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 403, resp.Status)
	require.Equal(t, 0, called)
}

func TestParseCIDRAllowlistFromEnv(t *testing.T) {
	t.Setenv("CRAWLER_PROTECTION_BYPASS_CIDRS", " 203.0.113.0/24, ,not-a-cidr,203.0.113.9,not-an-ip ")

	nets := parseCIDRAllowlistFromEnv(nil)
	require.NotEmpty(t, nets)
	require.True(t, isClientIPBypassed("203.0.113.10", nets))
	require.False(t, isClientIPBypassed(unknownString, nets))
}

func TestMiddleware_Block_AppTheoryProviderSourceAllowsTrustedForwarding(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	t.Setenv("CRAWLER_PROTECTION_BYPASS_CIDRS", "203.0.113.0/24")
	t.Setenv("CRAWLER_TRUSTED_PROXY_CIDRS", "70.0.0.0/8")

	called := 0
	app := apptheory.New()
	app.Use(Middleware(protectionModeBlock, zap.NewNop()))
	app.Get("/users/{username}", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		called++
		require.Equal(t, "70.0.0.2", ctx.SourceIP())
		return apptheory.Text(200, "ok"), nil
	})

	event := json.RawMessage(`{
		"version":"2.0",
		"routeKey":"GET /users/{username}",
		"rawPath":"/users/alice",
		"headers":{
			"user-agent":"GPTBot/1.0",
			"accept":"application/activity+json",
			"x-forwarded-for":"203.0.113.10, 70.0.0.1"
		},
		"requestContext":{"http":{"method":"GET","path":"/users/alice","sourceIp":"70.0.0.2"}},
		"isBase64Encoded":false
	}`)

	respAny, err := app.HandleLambda(context.Background(), event)
	require.NoError(t, err)
	resp, ok := respAny.(events.APIGatewayV2HTTPResponse)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, 1, called)
}

func TestMiddleware_Block_AppTheoryRESTProviderSourceAllowsTrustedForwarding(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	t.Setenv("CRAWLER_PROTECTION_BYPASS_CIDRS", "203.0.113.0/24")
	t.Setenv("CRAWLER_TRUSTED_PROXY_CIDRS", "70.0.0.0/8")

	called := 0
	app := apptheory.New()
	app.Use(Middleware(protectionModeBlock, zap.NewNop()))
	app.Get("/users/{username}", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		called++
		require.Equal(t, "70.0.0.2", ctx.SourceIP())
		return apptheory.Text(200, "ok"), nil
	})

	event := json.RawMessage(`{
		"httpMethod":"GET",
		"path":"/users/alice",
		"headers":{
			"user-agent":"GPTBot/1.0",
			"accept":"application/activity+json",
			"x-forwarded-for":"203.0.113.10, 70.0.0.1"
		},
		"requestContext":{"identity":{"sourceIp":"70.0.0.2"}},
		"isBase64Encoded":false
	}`)

	respAny, err := app.HandleLambda(context.Background(), event)
	require.NoError(t, err)
	resp, ok := respAny.(events.APIGatewayProxyResponse)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, 1, called)
}

func TestMiddleware_Block_ClientSuppliedSourceHeaderCannotBypass(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	t.Setenv("CRAWLER_PROTECTION_BYPASS_CIDRS", "203.0.113.0/24")
	t.Setenv("CRAWLER_TRUSTED_PROXY_CIDRS", "70.0.0.0/8")

	called := 0
	app := apptheory.New()
	app.Use(Middleware(protectionModeBlock, zap.NewNop()))
	app.Get("/users/{username}", func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "ok"), nil
	})

	event := json.RawMessage(`{
		"version":"2.0",
		"routeKey":"GET /users/{username}",
		"rawPath":"/users/alice",
		"headers":{
			"user-agent":"GPTBot/1.0",
			"accept":"application/activity+json",
			"x-forwarded-for":"203.0.113.10, 70.0.0.1",
			"X-Lesser-Trusted-Source-IP":"70.0.0.2"
		},
		"requestContext":{"http":{"method":"GET","path":"/users/alice","sourceIp":"198.51.100.9"}},
		"isBase64Encoded":false
	}`)

	respAny, err := app.HandleLambda(context.Background(), event)
	require.NoError(t, err)
	resp, ok := respAny.(events.APIGatewayV2HTTPResponse)
	require.True(t, ok)
	require.Equal(t, 403, resp.StatusCode)
	require.Equal(t, 0, called)
}

func TestNewLimiterFunc(t *testing.T) {
	originalLimiterDB := limiterDB
	originalLimiterDBErr := limiterDBErr
	originalNewLimiterDBFunc := newLimiterDBFunc

	t.Cleanup(func() {
		limiterDBOnce = sync.Once{}
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
	t.Setenv("CRAWLER_TRUSTED_PROXY_CIDRS", "70.0.0.0/8")

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
		Method:           "GET",
		Path:             "/users/alice",
		SourceProvenance: providerSourceIP("70.0.0.2"),
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

func TestMiddleware_Limit_SpoofedForwardedHeaderUsesUnknownKey(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "false")

	orig := newLimiterFunc
	t.Cleanup(func() { newLimiterFunc = orig })

	limiter := &stubLimiter{decision: &apptheoryLimited.LimitDecision{
		Allowed:      true,
		CurrentCount: 1,
		Limit:        30,
		ResetsAt:     time.Unix(1700000000, 0),
	}}
	newLimiterFunc = func(_ string, limit int, _ time.Duration, _ *zap.Logger) (atomicLimiter, error) {
		if limit == 30 {
			return limiter, nil
		}
		return nil, context.Canceled
	}

	next := func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(200, "ok"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method: "GET",
		Path:   "/users/alice",
		Headers: map[string][]string{
			"user-agent":      {"ExampleScraperBot/1.0"},
			"accept":          {"text/html"},
			"x-forwarded-for": {"203.0.113.1"},
		},
	}}

	resp, err := Middleware(protectionModeLimit, zap.NewNop())(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, "ip:"+unknownString, limiter.lastKey.Identifier)
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
	t.Setenv("CRAWLER_TRUSTED_PROXY_CIDRS", "70.0.0.0/8")

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
		Method:           "GET",
		Path:             "/api/v1/instance",
		SourceProvenance: providerSourceIP("70.0.0.3"),
		Headers: map[string][]string{
			"user-agent":      {"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
			"accept":          {"text/html"},
			"x-forwarded-for": {"203.0.113.2, 70.0.0.2"},
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
	_, trustedProxy, err := net.ParseCIDR("70.0.0.0/8")
	require.NoError(t, err)
	trusted := []*net.IPNet{trustedProxy}

	require.Equal(t, unknownString, extractClientIP(nil, trusted))

	xff := &apptheory.Context{Request: apptheory.Request{Headers: map[string][]string{
		"x-forwarded-for": {"203.0.113.1, 70.0.0.1"},
	}, SourceProvenance: providerSourceIP("70.0.0.2")}}
	require.Equal(t, "70.0.0.2", extractClientIP(xff, nil))
	require.Equal(t, "203.0.113.1", extractClientIP(xff, trusted))

	xri := &apptheory.Context{Request: apptheory.Request{Headers: map[string][]string{
		"x-forwarded-for": {"70.0.0.1"},
		"x-real-ip":       {"203.0.113.9"},
	}, SourceProvenance: providerSourceIP("70.0.0.2")}}
	require.Equal(t, "70.0.0.2", extractClientIP(xri, trusted))

	direct := &apptheory.Context{Request: apptheory.Request{Headers: map[string][]string{
		"x-forwarded-for": {"203.0.113.9"},
	}, SourceProvenance: providerSourceIP("198.51.100.9")}}
	require.Equal(t, "198.51.100.9", extractClientIP(direct, trusted))

	unknown := &apptheory.Context{Request: apptheory.Request{Headers: map[string][]string{}}}
	require.Equal(t, unknownString, extractClientIP(unknown, trusted))
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

func TestCrawlerLimitFromEnv(t *testing.T) {
	require.Equal(t, 5, crawlerLimitFromEnv("CRAWLER_LIMIT_TEST", 5, nil))

	t.Setenv("CRAWLER_LIMIT_TEST", "nope")
	require.Equal(t, 5, crawlerLimitFromEnv("CRAWLER_LIMIT_TEST", 5, nil))

	t.Setenv("CRAWLER_LIMIT_TEST", "0")
	require.Equal(t, 5, crawlerLimitFromEnv("CRAWLER_LIMIT_TEST", 5, zap.NewNop()))

	t.Setenv("CRAWLER_LIMIT_TEST", "7")
	require.Equal(t, 7, crawlerLimitFromEnv("CRAWLER_LIMIT_TEST", 5, zap.NewNop()))
}

func TestLimiterConfigsForMode(t *testing.T) {
	configs := limiterConfigsForMode(protectionModeLimit, zap.NewNop())
	require.Len(t, configs, 2)
	require.Equal(t, defaultSearchEngineLimitPerHour, configs[CategorySearchEngine].limit)
	require.Equal(t, defaultGenericBotLimitPerHour, configs[CategoryGenericBot].limit)

	configs = limiterConfigsForMode(protectionModeBlock, zap.NewNop())
	require.Len(t, configs, 3)
	require.Equal(t, defaultSuspiciousLimitPerHour, configs[CategorySuspicious].limit)
}

func TestIsCrawlerMetricsDisabled(t *testing.T) {
	t.Setenv("DISABLE_METRICS", "true")
	require.True(t, isCrawlerMetricsDisabled())

	t.Setenv("DISABLE_METRICS", "false")
	t.Setenv("EMF_METRICS_ENABLED", "false")
	require.True(t, isCrawlerMetricsDisabled())

	t.Setenv("EMF_METRICS_ENABLED", "true")
	t.Setenv("CRAWLER_METRICS_ENABLED", "false")
	require.True(t, isCrawlerMetricsDisabled())
}

func TestNewCrawlerMetrics(t *testing.T) {
	t.Setenv("DISABLE_METRICS", "true")
	require.Nil(t, newCrawlerMetrics(zap.NewNop()))

	t.Setenv("DISABLE_METRICS", "false")
	t.Setenv("EMF_METRICS_ENABLED", "true")
	t.Setenv("CRAWLER_METRICS_ENABLED", "true")
	t.Setenv("STAGE", "dev")

	metrics := newCrawlerMetrics(zap.NewNop())
	require.NotNil(t, metrics)
	metrics.recordEvent("TestMetric", "TestMetricByRoute", CategoryGenericBot, "")
	metrics.recordBypassed(CategoryGenericBot)
}
