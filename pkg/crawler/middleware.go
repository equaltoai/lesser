package crawler

import (
	"context"
	"fmt"
	"net"
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

type protectionMode string

const (
	protectionModeOff     protectionMode = "off"
	protectionModeObserve protectionMode = "observe"
	protectionModeLimit   protectionMode = "limit"
	protectionModeBlock   protectionMode = "block"
)

const (
	contextCrawlerCategoryKey = "crawler_category"
	contextCrawlerReasonKey   = "crawler_reason"
)

type atomicLimiter interface {
	CheckAndIncrement(ctx context.Context, key apptheoryLimited.RateLimitKey) (*apptheoryLimited.LimitDecision, error)
}

type limiterConfig struct {
	limit  int
	window time.Duration
}

var defaultLimiterConfigs = map[Category]limiterConfig{
	CategorySearchEngine: {limit: 100, window: time.Hour},
	CategoryGenericBot:   {limit: 30, window: time.Hour},
}

var blockModeLimiterConfigs = map[Category]limiterConfig{
	CategorySearchEngine: {limit: 100, window: time.Hour},
	CategoryGenericBot:   {limit: 30, window: time.Hour},
	CategorySuspicious:   {limit: 10, window: time.Hour},
}

var (
	limiterDBOnce sync.Once
	limiterDB     tablecore.DB
	limiterDBErr  error
)

var newLimiterDBFunc = func(region string) (tablecore.DB, error) {
	return tabletheory.NewBasic(session.Config{Region: region})
}

var newLimiterFunc = func(region string, limit int, window time.Duration, _ *zap.Logger) (atomicLimiter, error) {
	if region == "" {
		return nil, context.DeadlineExceeded // sentinel
	}

	limiterDBOnce.Do(func() {
		limiterDB, limiterDBErr = newLimiterDBFunc(region)
	})
	if limiterDBErr != nil {
		return nil, limiterDBErr
	}
	if limiterDB == nil {
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
	return apptheoryLimited.NewDynamoRateLimiter(limiterDB, nil, strategy), nil
}

func parseProtectionMode(raw string) protectionMode {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case string(protectionModeObserve):
		return protectionModeObserve
	case string(protectionModeLimit):
		return protectionModeLimit
	case string(protectionModeBlock):
		return protectionModeBlock
	default:
		return protectionModeOff
	}
}

func protectionModeFromEnv() protectionMode {
	return parseProtectionMode(os.Getenv("CRAWLER_PROTECTION_MODE"))
}

func isRateLimitingDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("DISABLE_RATE_LIMITING")), "true")
}

func resolveAWSRegion() string {
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	}
	if region == "" {
		region = "us-east-1"
	}
	return region
}

func isEnforcementMode(mode protectionMode) bool {
	return mode == protectionModeLimit || mode == protectionModeBlock
}

func parseCIDRAllowlistFromEnv(logger *zap.Logger) []*net.IPNet {
	raw := strings.TrimSpace(os.Getenv("CRAWLER_PROTECTION_BYPASS_CIDRS"))
	if raw == "" {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	var nets []*net.IPNet
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if strings.Contains(entry, "/") {
			_, ipNet, err := net.ParseCIDR(entry)
			if err != nil || ipNet == nil {
				logger.Error("invalid crawler bypass CIDR ignored", zap.String("entry", entry), zap.Error(err))
				continue
			}
			nets = append(nets, ipNet)
			continue
		}

		ip := net.ParseIP(entry)
		if ip == nil {
			logger.Error("invalid crawler bypass IP ignored", zap.String("entry", entry))
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			ip = ip4
		}
		maskBits := 128
		if ip.To4() != nil {
			maskBits = 32
		}
		nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(maskBits, maskBits)})
	}

	return nets
}

func isClientIPBypassed(clientIP string, allowlist []*net.IPNet) bool {
	if len(allowlist) == 0 {
		return false
	}

	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}

	for _, ipNet := range allowlist {
		if ipNet != nil && ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func buildLimiters(mode protectionMode, logger *zap.Logger) map[Category]atomicLimiter {
	if !isEnforcementMode(mode) || isRateLimitingDisabled() {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	region := resolveAWSRegion()
	configs := defaultLimiterConfigs
	if mode == protectionModeBlock {
		configs = blockModeLimiterConfigs
	}

	limiters := map[Category]atomicLimiter{}
	for category, cfg := range configs {
		limiter, err := newLimiterFunc(region, cfg.limit, cfg.window, logger)
		if err != nil || limiter == nil {
			logger.Error("failed to create crawler rate limiter (fail-open)",
				zap.String("region", region),
				zap.String("category", category.String()),
				zap.Int("limit", cfg.limit),
				zap.Duration("window", cfg.window),
				zap.Error(err),
			)
			continue
		}
		limiters[category] = limiter
	}

	return limiters
}

// NewMiddleware returns the crawler middleware configured by environment.
//
// Modes (CRAWLER_PROTECTION_MODE):
//   - off (default): no classification, no logging
//   - observe: classify requests, log the classification, do not enforce
//   - limit: enforce rate limits for search engines + generic bots
//   - block: enforce limits + block known AI crawlers
func NewMiddleware(logger *zap.Logger) apptheory.Middleware {
	return Middleware(protectionModeFromEnv(), logger)
}

// Middleware classifies requests and (optionally) logs the classification.
func Middleware(mode protectionMode, logger *zap.Logger) apptheory.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}

	mode = parseProtectionMode(string(mode))
	if mode == protectionModeOff {
		return func(next apptheory.Handler) apptheory.Handler { return next }
	}

	bypassCIDRs := parseCIDRAllowlistFromEnv(logger)
	limiters := buildLimiters(mode, logger)

	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx == nil {
				return next(ctx)
			}

			userAgent := headerValue(ctx, "User-Agent")
			accept := headerValue(ctx, "Accept")
			category, reason := ClassifyRequest(userAgent, accept, ctx.Request.Path)

			ctx.Set(contextCrawlerCategoryKey, category.String())
			ctx.Set(contextCrawlerReasonKey, reason)

			clientIP := extractClientIP(ctx)
			enforcementBypassed := isClientIPBypassed(clientIP, bypassCIDRs)
			logger.Debug("crawler classification",
				zap.String("request_id", strings.TrimSpace(ctx.RequestID)),
				zap.String("method", strings.TrimSpace(ctx.Request.Method)),
				zap.String("path", strings.TrimSpace(ctx.Request.Path)),
				zap.String("category", category.String()),
				zap.String("reason", reason),
				zap.String("user_agent", userAgent),
				zap.String("client_ip", clientIP),
				zap.Bool("enforcement_bypassed", enforcementBypassed),
			)

			if enforcementBypassed {
				return next(ctx)
			}

			if mode == protectionModeBlock && category == CategoryAICrawler {
				return blockedResponse(category, reason), nil
			}

			resp, handled, handlerErr := maybeApplyRateLimit(ctx, next, logger, limiters, category, userAgent, clientIP)
			if handled {
				return resp, handlerErr
			}

			return next(ctx)
		}
	}
}

func maybeApplyRateLimit(
	ctx *apptheory.Context,
	next apptheory.Handler,
	logger *zap.Logger,
	limiters map[Category]atomicLimiter,
	category Category,
	userAgent string,
	clientIP string,
) (*apptheory.Response, bool, error) {
	if ctx == nil || next == nil || logger == nil || len(limiters) == 0 {
		return nil, false, nil
	}

	limiter, ok := limiters[category]
	if !ok || limiter == nil {
		return nil, false, nil
	}

	routeClass := routeClassForPath(ctx.Request.Path)
	key := buildRateLimitKey(category, routeClass, clientIP, userAgent)

	decision, err := limiter.CheckAndIncrement(ctx.Context(), key)
	if err != nil {
		logger.Error("crawler rate limit check failed (fail-open)",
			zap.String("category", category.String()),
			zap.String("route_class", routeClass),
			zap.String("client_ip", clientIP),
			zap.Error(err),
		)
		return nil, false, nil
	}

	headers := rateLimitHeaders(decision)
	if decision != nil && !decision.Allowed {
		return rateLimitedResponse(decision, headers), true, nil
	}

	resp, handlerErr := next(ctx)
	if resp != nil {
		if resp.Headers == nil {
			resp.Headers = map[string][]string{}
		}
		for k, v := range headers {
			resp.Headers[k] = append([]string(nil), v...)
		}
	}
	return resp, true, handlerErr
}

func extractClientIP(ctx *apptheory.Context) string {
	xff := headerValue(ctx, "X-Forwarded-For")
	if xff != "" {
		if idx := strings.Index(xff, ","); idx >= 0 {
			xff = xff[:idx]
		}
		xff = strings.TrimSpace(xff)
		if xff != "" {
			return xff
		}
	}

	xri := strings.TrimSpace(headerValue(ctx, "X-Real-IP"))
	if xri != "" {
		return xri
	}

	return unknownString
}

func routeClassForPath(path string) string {
	path = strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasPrefix(path, "/api/v1/"), strings.HasPrefix(path, "/api/v2/"):
		return "api"
	case strings.HasPrefix(path, "/oauth/"), strings.HasPrefix(path, "/auth/"), strings.HasPrefix(path, "/setup/"):
		return "auth"
	case strings.HasPrefix(path, "/users/"), strings.HasPrefix(path, "/objects/"):
		return "federation"
	case strings.HasPrefix(path, "/.well-known/"), strings.HasPrefix(path, "/nodeinfo"):
		return "protocol"
	case path == "/robots.txt":
		return "robots"
	default:
		return "other"
	}
}

func buildRateLimitKey(category Category, routeClass, clientIP, userAgent string) apptheoryLimited.RateLimitKey {
	routeClass = strings.TrimSpace(routeClass)
	if routeClass == "" {
		routeClass = "other"
	}
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		clientIP = unknownString
	}

	identifier := "ip:" + clientIP
	if category == CategorySearchEngine {
		identifier = identifySearchEngine(userAgent) + ":" + identifier
	}

	return apptheoryLimited.RateLimitKey{
		Resource:   "crawler:" + routeClass,
		Operation:  category.String(),
		Identifier: identifier,
		Metadata: map[string]string{
			"category":    category.String(),
			"route_class": routeClass,
		},
	}
}

func identifySearchEngine(userAgent string) string {
	ua := strings.ToLower(userAgent)

	engines := []struct {
		pattern string
		name    string
	}{
		{pattern: "googlebot", name: "google"},
		{pattern: "bingbot", name: "bing"},
		{pattern: "duckduckbot", name: "duckduckgo"},
		{pattern: "slurp", name: "yahoo"},
		{pattern: "yandexbot", name: "yandex"},
		{pattern: "baiduspider", name: "baidu"},
	}

	for _, engine := range engines {
		if strings.Contains(ua, engine.pattern) {
			return engine.name
		}
	}

	return unknownString
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
		"x-ratelimit-limit":     {strconv.Itoa(decision.Limit)},
		"x-ratelimit-remaining": {strconv.Itoa(remaining)},
		"x-ratelimit-reset":     {strconv.FormatInt(decision.ResetsAt.Unix(), 10)},
	}
}

func rateLimitedResponse(decision *apptheoryLimited.LimitDecision, headers map[string][]string) *apptheory.Response {
	retryAfter := 60
	if decision != nil {
		if decision.RetryAfter != nil {
			retryAfter = int(decision.RetryAfter.Seconds())
		} else {
			wait := int(time.Until(decision.ResetsAt).Seconds())
			if wait > 0 {
				retryAfter = wait
			}
		}
	}
	if retryAfter < 1 {
		retryAfter = 1
	}

	resp := apptheory.Text(429, fmt.Sprintf("rate limit exceeded; retry in %ds\n", retryAfter))
	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}
	for k, v := range headers {
		resp.Headers[k] = append([]string(nil), v...)
	}
	resp.Headers["retry-after"] = []string{strconv.Itoa(retryAfter)}
	return resp
}

func blockedResponse(category Category, reason string) *apptheory.Response {
	categoryValue := strings.TrimSpace(category.String())
	if categoryValue == "" {
		categoryValue = unknownString
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = unknownString
	}

	resp := apptheory.Text(403, fmt.Sprintf(
		"request blocked (category=%s reason=%s); see /robots.txt\n",
		categoryValue,
		reason,
	))
	resp.Headers["cache-control"] = []string{"no-store"}
	return resp
}

func headerValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	values := ctx.Request.Headers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
