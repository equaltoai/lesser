// Package ratelimit provides helpers for applying distributed request throttling.
package ratelimit

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	apptheoryLimited "github.com/theory-cloud/apptheory/v2/pkg/limited"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"github.com/theory-cloud/tabletheory/v2"
	tablecore "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/theory-cloud/tabletheory/v2/pkg/session"
	"go.uber.org/zap"
)

type atomicLimiter interface {
	CheckAndIncrement(ctx context.Context, key apptheoryLimited.RateLimitKey) (*apptheoryLimited.LimitDecision, error)
}

var (
	dbOnce sync.Once
	db     tablecore.DB
	dbErr  error
)

var newDBFunc = func(region string) (tablecore.DB, error) {
	return tabletheory.NewBasic(session.Config{Region: region})
}

var newLimiterFunc = func(region string, limit int, window time.Duration, _ *zap.Logger) (atomicLimiter, error) {
	if region == "" {
		return nil, context.DeadlineExceeded // sentinel; should never happen with our region fallback
	}

	dbOnce.Do(func() {
		db, dbErr = newDBFunc(region)
	})
	if dbErr != nil {
		return nil, dbErr
	}
	if db == nil {
		return nil, context.Canceled // sentinel
	}

	if window <= 0 {
		window = time.Hour
	}
	if limit <= 0 {
		limit = 1000
	}

	granularity := window / 10
	if granularity <= 0 {
		granularity = time.Second
	}
	strategy := apptheoryLimited.NewSlidingWindowStrategy(window, limit, granularity)
	return apptheoryLimited.NewDynamoRateLimiter(db, nil, strategy), nil
}

// ApplyRateLimit wraps a handler with a DynamoDB-backed rate limiter.
//
// This is a hard cutover: no Lift runtime types or middleware are used.
//
// Fail-open: if limiter creation/check fails, the request proceeds.
func ApplyRateLimit(handler apptheory.Handler, limit int, window time.Duration, logger *zap.Logger) apptheory.Handler {
	if handler == nil {
		return handler
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	region := rateLimitRegion()

	limiter, err := newLimiterFunc(region, limit, window, logger)
	if err != nil || limiter == nil {
		logger.Error("failed to create rate limiter - allowing request",
			zap.Error(err),
			zap.Int("limit", limit),
			zap.Duration("window", window),
			zap.String("region", region),
		)
		return handler
	}

	return func(ctx *apptheory.Context) (*apptheory.Response, error) {
		resp, allowed := enforceRateLimit(ctx, limiter, rateLimitKey(ctx), logger, nil)
		if !allowed {
			return resp, nil
		}
		resp, handlerErr := handler(ctx)
		attachStoredRateLimitHeaders(ctx, resp)
		return resp, handlerErr
	}
}

// ApplyOAuthTokenRateLimit wraps /oauth/token with the standard path/IP limiter.
func ApplyOAuthTokenRateLimit(handler apptheory.Handler, limit int, window time.Duration, logger *zap.Logger) apptheory.Handler {
	return ApplyRateLimit(handler, limit, window, logger)
}

// ApplyOAuthRegistrationRateLimit wraps /oauth/register with a tighter unauthenticated limit and
// an OAuth-friendly 429 payload for discovery clients.
func ApplyOAuthRegistrationRateLimit(handler apptheory.Handler, limit int, window time.Duration, logger *zap.Logger) apptheory.Handler {
	if handler == nil {
		return handler
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	region := rateLimitRegion()
	limiter, err := newLimiterFunc(region, limit, window, logger)
	if err != nil || limiter == nil {
		logger.Error("failed to create oauth registration rate limiter - allowing request",
			zap.Error(err),
			zap.Int("limit", limit),
			zap.Duration("window", window),
			zap.String("region", region),
		)
		return handler
	}

	return func(ctx *apptheory.Context) (*apptheory.Response, error) {
		resp, allowed := enforceRateLimit(ctx, limiter, rateLimitKey(ctx), logger, func(decision *apptheoryLimited.LimitDecision) map[string]any {
			retryAfter := retryAfterSeconds(decision)
			return map[string]any{
				"error":             "slow_down",
				"error_description": "Too many dynamic client registration requests",
				"limit":             decision.Limit,
				"remaining":         maxInt(0, decision.Limit-decision.CurrentCount),
				"reset_at":          decision.ResetsAt.Unix(),
				"retry_after":       retryAfter,
			}
		})
		if !allowed {
			return resp, nil
		}

		resp, handlerErr := handler(ctx)
		attachStoredRateLimitHeaders(ctx, resp)
		return resp, handlerErr
	}
}

func rateLimitRegion() string {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}
	return region
}

func enforceRateLimit(
	ctx *apptheory.Context,
	limiter atomicLimiter,
	key apptheoryLimited.RateLimitKey,
	logger *zap.Logger,
	bodyBuilder func(decision *apptheoryLimited.LimitDecision) map[string]any,
) (*apptheory.Response, bool) {
	decision, err := limiter.CheckAndIncrement(ctx.Context(), key)
	if err != nil {
		logger.Error("rate limit check failed - allowing request",
			zap.Error(err),
			zap.String("path", ctx.Request.Path),
			zap.String("method", ctx.Request.Method),
		)
		return nil, true
	}

	headers := rateLimitHeaders(decision)
	if decision != nil && !decision.Allowed {
		return rateLimitedResponse(headers, decision, bodyBuilder), false
	}

	if ctx != nil && headers != nil {
		existing, _ := ctx.Get("rate_limit_headers").(map[string][]string)
		if existing == nil {
			existing = map[string][]string{}
		}
		for k, v := range headers {
			existing[k] = append([]string(nil), v...)
		}
		ctx.Set("rate_limit_headers", existing)
	}

	return nil, true
}

func rateLimitedResponse(headers map[string][]string, decision *apptheoryLimited.LimitDecision, bodyBuilder func(decision *apptheoryLimited.LimitDecision) map[string]any) *apptheory.Response {
	retryAfter := retryAfterSeconds(decision)
	if bodyBuilder == nil {
		bodyBuilder = func(decision *apptheoryLimited.LimitDecision) map[string]any {
			return map[string]any{
				"error":       "Rate limit exceeded",
				"limit":       decision.Limit,
				"remaining":   maxInt(0, decision.Limit-decision.CurrentCount),
				"reset_at":    decision.ResetsAt.Unix(),
				"retry_after": retryAfter,
			}
		}
	}

	raw, marshalErr := json.Marshal(bodyBuilder(decision))
	if headers == nil {
		headers = map[string][]string{}
	}
	headers["retry-after"] = []string{strconvItoa(retryAfter)}
	if marshalErr != nil {
		return &apptheory.Response{
			Status:  http.StatusTooManyRequests,
			Headers: headers,
		}
	}
	return &apptheory.Response{
		Status:  http.StatusTooManyRequests,
		Headers: headers,
		Body:    raw,
	}
}

func retryAfterSeconds(decision *apptheoryLimited.LimitDecision) int {
	if decision != nil && decision.RetryAfter != nil {
		return int(decision.RetryAfter.Seconds())
	}
	return 60
}

func attachStoredRateLimitHeaders(ctx *apptheory.Context, resp *apptheory.Response) {
	if ctx == nil || resp == nil {
		return
	}
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}
	headers, _ := ctx.Get("rate_limit_headers").(map[string][]string)
	if headers == nil {
		return
	}
	for k, v := range headers {
		resp.Headers[k] = append([]string(nil), v...)
	}
}

func rateLimitKey(ctx *apptheory.Context) apptheoryLimited.RateLimitKey {
	key := apptheoryLimited.RateLimitKey{
		Resource:  "/",
		Operation: "",
		Metadata:  map[string]string{},
	}
	if ctx == nil {
		key.Identifier = "anonymous"
		return key
	}

	key.Resource = ctx.Request.Path
	key.Operation = ctx.Request.Method

	// Priority: user > tenant > ip.
	if username := strings.TrimSpace(auth.GetAuthenticatedUsername(ctx)); username != "" {
		key.Identifier = "user:" + username
		key.Metadata["user_id"] = username
		return key
	}

	if tenant := strings.TrimSpace(ctx.TenantID); tenant != "" {
		key.Identifier = "tenant:" + tenant
		key.Metadata["tenant_id"] = tenant
		return key
	}

	ip := firstHeaderValue(ctx.Request.Headers, "x-forwarded-for")
	if ip == "" {
		ip = firstHeaderValue(ctx.Request.Headers, "x-real-ip")
	}
	if ip == "" {
		ip = "unknown"
	}
	key.Identifier = "ip:" + ip
	key.Metadata["ip"] = ip
	return key
}

func rateLimitHeaders(decision *apptheoryLimited.LimitDecision) map[string][]string {
	if decision == nil {
		return nil
	}

	remaining := decision.Limit - decision.CurrentCount
	if remaining < 0 {
		remaining = 0
	}

	return map[string][]string{
		"x-ratelimit-limit":     {strconvItoa(decision.Limit)},
		"x-ratelimit-remaining": {strconvItoa(remaining)},
		"x-ratelimit-reset":     {strconvFormatInt(decision.ResetsAt.Unix())},
	}
}

func firstHeaderValue(headers map[string][]string, key string) string {
	if headers == nil {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	values := headers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func strconvItoa(v int) string {
	return strconv.FormatInt(int64(v), 10)
}

func strconvFormatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}
