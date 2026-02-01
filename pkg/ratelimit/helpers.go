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

	apptheoryLimited "github.com/theory-cloud/apptheory/pkg/limited"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"github.com/theory-cloud/tabletheory"
	tablecore "github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
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

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

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
		decision, err := limiter.CheckAndIncrement(ctx.Context(), rateLimitKey(ctx))
		if err != nil {
			logger.Error("rate limit check failed - allowing request",
				zap.Error(err),
				zap.String("path", ctx.Request.Path),
				zap.String("method", ctx.Request.Method),
			)
			return handler(ctx)
		}

		headers := rateLimitHeaders(decision)

		if decision != nil && !decision.Allowed {
			retryAfter := 60
			if decision.RetryAfter != nil {
				retryAfter = int(decision.RetryAfter.Seconds())
			}

			body := map[string]any{
				"error":       "Rate limit exceeded",
				"limit":       decision.Limit,
				"remaining":   maxInt(0, decision.Limit-decision.CurrentCount),
				"reset_at":    decision.ResetsAt.Unix(),
				"retry_after": retryAfter,
			}

			raw, marshalErr := json.Marshal(body)
			if marshalErr != nil {
				return &apptheory.Response{
					Status:  http.StatusTooManyRequests,
					Headers: headers,
				}, nil
			}

			if headers == nil {
				headers = map[string][]string{}
			}
			headers["retry-after"] = []string{strconvItoa(retryAfter)}

			return &apptheory.Response{
				Status:  http.StatusTooManyRequests,
				Headers: headers,
				Body:    raw,
			}, nil
		}

		resp, handlerErr := handler(ctx)
		if resp != nil {
			if resp.Headers == nil {
				resp.Headers = map[string][]string{}
			}
			for k, v := range headers {
				resp.Headers[k] = append([]string(nil), v...)
			}
		}
		return resp, handlerErr
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
	if username, ok := ctx.Get("username").(string); ok && strings.TrimSpace(username) != "" {
		key.Identifier = "user:" + strings.TrimSpace(username)
		key.Metadata["user_id"] = strings.TrimSpace(username)
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
