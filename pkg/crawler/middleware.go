package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cloudwatchTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/observability"
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
	routeClassOther           = "other"
	trustedSourceIPHeader     = "x-lesser-trusted-source-ip"
)

type atomicLimiter interface {
	CheckAndIncrement(ctx context.Context, key apptheoryLimited.RateLimitKey) (*apptheoryLimited.LimitDecision, error)
}

type limiterConfig struct {
	limit  int
	window time.Duration
}

const (
	defaultSearchEngineLimitPerHour = 100
	defaultGenericBotLimitPerHour   = 30
	defaultSuspiciousLimitPerHour   = 10

	crawlerBypassCIDRsEnv = "CRAWLER_PROTECTION_BYPASS_CIDRS"
	// CRAWLER_TRUSTED_PROXY_CIDRS is intentionally empty by default; forwarded IP
	// headers are only used when the AWS request source IP is in a configured
	// trusted proxy CIDR.
	crawlerTrustedProxyCIDRsEnv = "CRAWLER_TRUSTED_PROXY_CIDRS"
)

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

func crawlerLimitFromEnv(key string, fallback int, logger *zap.Logger) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		if logger == nil {
			logger = zap.NewNop()
		}
		logger.Error("invalid crawler rate limit; using default",
			zap.String("key", key),
			zap.String("value", raw),
			zap.Int("default", fallback),
			zap.Error(err),
		)
		return fallback
	}

	return value
}

func limiterConfigsForMode(mode protectionMode, logger *zap.Logger) map[Category]limiterConfig {
	searchEngineLimit := crawlerLimitFromEnv("CRAWLER_LIMIT_SEARCH_ENGINE_PER_HOUR", defaultSearchEngineLimitPerHour, logger)
	genericBotLimit := crawlerLimitFromEnv("CRAWLER_LIMIT_GENERIC_BOT_PER_HOUR", defaultGenericBotLimitPerHour, logger)

	configs := map[Category]limiterConfig{
		CategorySearchEngine: {limit: searchEngineLimit, window: time.Hour},
		CategoryGenericBot:   {limit: genericBotLimit, window: time.Hour},
	}

	if mode == protectionModeBlock {
		suspiciousLimit := crawlerLimitFromEnv("CRAWLER_LIMIT_SUSPICIOUS_PER_HOUR", defaultSuspiciousLimitPerHour, logger)
		configs[CategorySuspicious] = limiterConfig{limit: suspiciousLimit, window: time.Hour}
	}

	return configs
}

func isCrawlerMetricsDisabled() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DISABLE_METRICS")), "true") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("EMF_METRICS_ENABLED")), "false") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CRAWLER_METRICS_ENABLED")), "false") {
		return true
	}
	return false
}

type crawlerMetrics struct {
	collector *observability.EMFMetricsCollector
}

func newCrawlerMetrics(logger *zap.Logger) *crawlerMetrics {
	if isCrawlerMetricsDisabled() {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	collector := observability.NewEMFMetricsCollector("Lesser/Crawler", logger)
	collector.RemoveDimension("FunctionName")
	collector.RemoveDimension("Version")
	if stage := strings.TrimSpace(os.Getenv("STAGE")); stage != "" {
		collector.SetDimension(observability.DimensionEnvironment, stage)
	}
	return &crawlerMetrics{collector: collector}
}

func (m *crawlerMetrics) recordEvent(totalMetricName, byRouteMetricName string, category Category, routeClass string) {
	if m == nil || m.collector == nil {
		return
	}

	categoryValue := category.String()
	routeClass = strings.TrimSpace(routeClass)
	if routeClass == "" {
		routeClass = routeClassOther
	}

	m.collector.RecordMetric(totalMetricName, 1.0, cloudwatchTypes.StandardUnitCount,
		cloudwatchTypes.Dimension{Name: aws.String(observability.DimensionCrawlerCategory), Value: aws.String(categoryValue)},
	)
	m.collector.RecordMetric(byRouteMetricName, 1.0, cloudwatchTypes.StandardUnitCount,
		cloudwatchTypes.Dimension{Name: aws.String(observability.DimensionCrawlerCategory), Value: aws.String(categoryValue)},
		cloudwatchTypes.Dimension{Name: aws.String(observability.DimensionRouteClass), Value: aws.String(routeClass)},
	)
	_ = m.collector.Flush()
}

func (m *crawlerMetrics) recordBlocked(category Category, routeClass string) {
	m.recordEvent(observability.MetricCrawlerBlocked, observability.MetricCrawlerBlockedByRoute, category, routeClass)
}

func (m *crawlerMetrics) recordRateLimited(category Category, routeClass string) {
	m.recordEvent(observability.MetricCrawlerRateLimited, observability.MetricCrawlerRateLimitedByRoute, category, routeClass)
}

func (m *crawlerMetrics) recordBypassed(category Category) {
	if m == nil || m.collector == nil {
		return
	}

	categoryValue := category.String()

	m.collector.RecordMetric(observability.MetricCrawlerBypassed, 1.0, cloudwatchTypes.StandardUnitCount,
		cloudwatchTypes.Dimension{Name: aws.String(observability.DimensionCrawlerCategory), Value: aws.String(categoryValue)},
	)
	_ = m.collector.Flush()
}

func isAICrawlerBlockingDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("CRAWLER_BLOCK_AI_CRAWLERS")), "false")
}

func parseCIDRAllowlistFromEnv(logger *zap.Logger) []*net.IPNet {
	raw := strings.TrimSpace(os.Getenv(crawlerBypassCIDRsEnv))
	return parseCIDRList(raw, logger)
}

func parseTrustedProxyCIDRsFromEnv(logger *zap.Logger) []*net.IPNet {
	raw := strings.TrimSpace(os.Getenv(crawlerTrustedProxyCIDRsEnv))
	return parseCIDRList(raw, logger)
}

func parseCIDRList(raw string, logger *zap.Logger) []*net.IPNet {
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
				logger.Error("invalid crawler CIDR ignored", zap.String("entry", entry), zap.Error(err))
				continue
			}
			nets = append(nets, ipNet)
			continue
		}

		ip := net.ParseIP(entry)
		if ip == nil {
			logger.Error("invalid crawler IP ignored", zap.String("entry", entry))
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

// InjectTrustedSourceIPHeader copies the AWS event's trusted source IP into an
// internal header before AppTheory normalizes the event into its portable
// Request model. AppTheory intentionally exposes only HTTP method/path/query/
// headers/body, so crawler protection needs this narrow bridge to distinguish
// a trusted forwarding header chain from client-supplied spoofed headers.
//
// The internal header is always removed from the incoming event first. This
// prevents a client from supplying the bridge header directly; only the value
// derived from requestContext.http.sourceIp (API Gateway v2 / Lambda URL) or
// requestContext.identity.sourceIp (API Gateway REST v1) is trusted.
func InjectTrustedSourceIPHeader(event json.RawMessage, logger *zap.Logger) json.RawMessage {
	if len(bytes.TrimSpace(event)) == 0 {
		return event
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	var envelope map[string]any
	decoder := json.NewDecoder(bytes.NewReader(event))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil || envelope == nil {
		logger.Debug("crawler source ip injection skipped: invalid event envelope", zap.Error(err))
		return event
	}

	sourceIP := sourceIPFromAWSEvent(envelope)
	changed := sanitizeInternalSourceIPHeader(envelope, sourceIP)
	if !changed {
		return event
	}

	updated, err := json.Marshal(envelope)
	if err != nil {
		logger.Debug("crawler source ip injection skipped: failed to marshal event envelope", zap.Error(err))
		return event
	}
	return json.RawMessage(updated)
}

func sourceIPFromAWSEvent(event map[string]any) string {
	requestContext, ok := event["requestContext"].(map[string]any)
	if !ok || requestContext == nil {
		return ""
	}

	if httpContext, ok := requestContext["http"].(map[string]any); ok {
		if sourceIP := normalizeIPString(stringField(httpContext, "sourceIp")); sourceIP != "" {
			return sourceIP
		}
	}

	if identity, ok := requestContext["identity"].(map[string]any); ok {
		if sourceIP := normalizeIPString(stringField(identity, "sourceIp")); sourceIP != "" {
			return sourceIP
		}
	}

	return ""
}

func stringField(values map[string]any, field string) string {
	for key, value := range values {
		if strings.EqualFold(strings.TrimSpace(key), field) {
			if text, ok := value.(string); ok {
				return text
			}
		}
	}
	return ""
}

func sanitizeInternalSourceIPHeader(event map[string]any, sourceIP string) bool {
	changed := false

	headers, ok := event["headers"].(map[string]any)
	if ok && headers != nil {
		if removeHeaderValue(headers, trustedSourceIPHeader) {
			changed = true
		}
	} else if sourceIP != "" {
		headers = map[string]any{}
		event["headers"] = headers
		changed = true
	}

	if sourceIP != "" {
		if headers == nil {
			headers = map[string]any{}
			event["headers"] = headers
		}
		headers[trustedSourceIPHeader] = sourceIP
		changed = true
	}

	if multiHeaders, ok := event["multiValueHeaders"].(map[string]any); ok && multiHeaders != nil {
		if removeHeaderValue(multiHeaders, trustedSourceIPHeader) {
			changed = true
		}
	}

	return changed
}

func removeHeaderValue(headers map[string]any, name string) bool {
	removed := false
	for key := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			delete(headers, key)
			removed = true
		}
	}
	return removed
}

func buildLimiters(mode protectionMode, logger *zap.Logger) map[Category]atomicLimiter {
	if !isEnforcementMode(mode) || isRateLimitingDisabled() {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	region := resolveAWSRegion()
	configs := limiterConfigsForMode(mode, logger)

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

	metrics := newCrawlerMetrics(logger)
	bypassCIDRs := parseCIDRAllowlistFromEnv(logger)
	trustedProxyCIDRs := parseTrustedProxyCIDRsFromEnv(logger)
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

			routeClass := routeClassForPath(ctx.Request.Path)
			clientIP := extractClientIP(ctx, trustedProxyCIDRs)
			enforcementBypassed := isClientIPBypassed(clientIP, bypassCIDRs)
			logger.Debug("crawler classification",
				zap.String("request_id", strings.TrimSpace(ctx.RequestID)),
				zap.String("method", strings.TrimSpace(ctx.Request.Method)),
				zap.String("path", strings.TrimSpace(ctx.Request.Path)),
				zap.String("route_class", routeClass),
				zap.String("category", category.String()),
				zap.String("reason", reason),
				zap.String("user_agent", userAgent),
				zap.String("client_ip", clientIP),
				zap.Bool("enforcement_bypassed", enforcementBypassed),
			)

			if enforcementBypassed {
				if metrics != nil {
					metrics.recordBypassed(category)
				}
				return next(ctx)
			}

			if mode == protectionModeBlock && category == CategoryAICrawler && !isAICrawlerBlockingDisabled() {
				if metrics != nil {
					metrics.recordBlocked(category, routeClass)
				}
				return blockedResponse(category, reason), nil
			}

			resp, handled, rateLimited, handlerErr := maybeApplyRateLimit(ctx, next, logger, limiters, category, userAgent, clientIP, routeClass)
			if handled {
				if rateLimited && metrics != nil {
					metrics.recordRateLimited(category, routeClass)
				}
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
	routeClass string,
) (*apptheory.Response, bool, bool, error) {
	if ctx == nil || next == nil || logger == nil || len(limiters) == 0 {
		return nil, false, false, nil
	}

	limiter, ok := limiters[category]
	if !ok || limiter == nil {
		return nil, false, false, nil
	}

	if strings.TrimSpace(routeClass) == "" {
		routeClass = routeClassForPath(ctx.Request.Path)
	}
	key := buildRateLimitKey(category, routeClass, clientIP, userAgent)

	decision, err := limiter.CheckAndIncrement(ctx.Context(), key)
	if err != nil {
		logger.Error("crawler rate limit check failed (fail-open)",
			zap.String("category", category.String()),
			zap.String("route_class", routeClass),
			zap.String("client_ip", clientIP),
			zap.Error(err),
		)
		return nil, false, false, nil
	}

	headers := rateLimitHeaders(decision)
	if decision != nil && !decision.Allowed {
		return rateLimitedResponse(decision, headers), true, true, nil
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
	return resp, true, false, handlerErr
}

func extractClientIP(ctx *apptheory.Context, trustedProxyCIDRs []*net.IPNet) string {
	sourceIP := normalizeIPString(headerValue(ctx, trustedSourceIPHeader))
	if sourceIP == "" {
		return unknownString
	}

	source := normalizeIP(sourceIP)
	if source == nil || !ipInCIDRs(source, trustedProxyCIDRs) {
		return sourceIP
	}

	if clientIP, trusted := extractClientIPFromForwardedFor(headerValue(ctx, "X-Forwarded-For"), trustedProxyCIDRs); trusted {
		if clientIP != "" {
			return clientIP
		}
	}

	return sourceIP
}

func extractClientIPFromForwardedFor(raw string, trustedProxyCIDRs []*net.IPNet) (string, bool) {
	if strings.TrimSpace(raw) == "" || len(trustedProxyCIDRs) == 0 {
		return "", false
	}

	parts := strings.Split(raw, ",")
	ips := make([]net.IP, 0, len(parts))
	for _, part := range parts {
		ip := normalizeIP(part)
		if ip != nil {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		return "", false
	}

	for i := len(ips) - 1; i >= 0; i-- {
		if ipInCIDRs(ips[i], trustedProxyCIDRs) {
			continue
		}
		return ips[i].String(), true
	}

	return "", true
}

func normalizeIP(raw string) net.IP {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return nil
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip
}

func normalizeIPString(raw string) string {
	ip := normalizeIP(raw)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func ipInCIDRs(ip net.IP, cidrs []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, ipNet := range cidrs {
		if ipNet != nil && ipNet.Contains(ip) {
			return true
		}
	}
	return false
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
		return routeClassOther
	}
}

func buildRateLimitKey(category Category, routeClass, clientIP, userAgent string) apptheoryLimited.RateLimitKey {
	routeClass = strings.TrimSpace(routeClass)
	if routeClass == "" {
		routeClass = routeClassOther
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
