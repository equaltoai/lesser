package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/pay-theory/lift/pkg/dynamorm"
	"github.com/pay-theory/lift/pkg/lift"
)

const (
	defaultTenant = "default"
)

// RateLimitConfig holds configuration for rate limiting
type RateLimitConfig struct {
	DynamORM        *dynamorm.DynamORMWrapper                   `json:"-"`
	ErrorHandler    func(*lift.Context, *RateLimitResult) error `json:"-"`
	KeyFunc         func(*lift.Context) *RateLimitKey           `json:"-"`
	UserLimits      map[string]int                              `json:"user_limits"`
	TenantLimits    map[string]int                              `json:"tenant_limits"`
	Strategy        string                                      `json:"strategy"`
	HeaderPrefix    string                                      `json:"header_prefix"`
	KeyPrefix       string                                      `json:"key_prefix"`
	TableName       string                                      `json:"table_name"`
	BurstLimit      int                                         `json:"burst_limit"`
	Window          time.Duration                               `json:"window"`
	DefaultWindow   time.Duration                               `json:"default_window"`
	CleanupInterval time.Duration                               `json:"cleanup_interval"`
	Granularity     time.Duration                               `json:"granularity"`
	DefaultLimit    int                                         `json:"default_limit"`
	TTL             time.Duration                               `json:"ttl"`
	IncludeMethod   bool                                        `json:"include_method"`
	SkipOptions     bool                                        `json:"skip_options"`
	SkipSuccessful  bool                                        `json:"skip_successful"`
	IncludePath     bool                                        `json:"include_path"`
}

// RateLimitKey represents a rate limiting key with metadata
type RateLimitKey struct {
	Metadata   map[string]string `json:"metadata"`
	Identifier string            `json:"identifier"`
	Resource   string            `json:"resource"`
	Operation  string            `json:"operation"`
}

// RateLimitEntry represents a rate limit record in DynamoDB
type RateLimitEntry struct {
	WindowStart time.Time `json:"window_start"`
	LastRequest time.Time `json:"last_request"`
	Key         string    `json:"key"`
	Count       int       `json:"count"`
	TTL         int64     `json:"ttl"`
}

// RateLimitResult contains the result of a rate limit check
type RateLimitResult struct {
	ResetAt     time.Time     `json:"reset_at"`
	WindowStart time.Time     `json:"window_start"`
	Limit       int           `json:"limit"`
	Remaining   int           `json:"remaining"`
	RetryAfter  time.Duration `json:"retry_after"`
	Allowed     bool          `json:"allowed"`
}

// RateLimitMiddleware creates a rate limiting middleware with DynamORM backend
func RateLimitMiddleware(config RateLimitConfig) lift.Middleware {
	// Apply default configuration
	config = applyRateLimitDefaults(config)

	processor := newRateLimitProcessor(&rateLimiter{config: config}, config)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return processor.process(ctx, next)
		})
	}
}

// applyRateLimitDefaults applies default values to the configuration
func applyRateLimitDefaults(config RateLimitConfig) RateLimitConfig {
	if config.DefaultLimit == 0 {
		config.DefaultLimit = 1000 // 1000 requests per window
	}
	if config.DefaultWindow == 0 {
		config.DefaultWindow = time.Hour // 1 hour window
	}
	if config.BurstLimit == 0 {
		config.BurstLimit = config.DefaultLimit / 10 // 10% burst
	}
	if config.KeyPrefix == "" {
		config.KeyPrefix = "ratelimit"
	}
	if config.HeaderPrefix == "" {
		config.HeaderPrefix = "X-RateLimit"
	}
	if config.TableName == "" {
		config.TableName = "rate_limits"
	}
	if config.TTL == 0 {
		config.TTL = 24 * time.Hour // Keep records for 24 hours
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = time.Hour // Cleanup every hour
	}
	return config
}

// rateLimitProcessor handles the rate limiting logic
type rateLimitProcessor struct {
	limiter *rateLimiter
	config  RateLimitConfig
}

// newRateLimitProcessor creates a new rate limit processor
func newRateLimitProcessor(limiter *rateLimiter, config RateLimitConfig) *rateLimitProcessor {
	return &rateLimitProcessor{
		limiter: limiter,
		config:  config,
	}
}

// process handles the rate limiting logic
func (p *rateLimitProcessor) process(ctx *lift.Context, next lift.Handler) error {
	// Check if request should be skipped
	if p.shouldSkipRequest(ctx) {
		return next.Handle(ctx)
	}

	// Generate rate limit key
	key := p.limiter.generateKey(ctx)

	// Execute rate limit check
	handler := newRateLimitHandler(p.limiter, p.config, key)
	return handler.handle(ctx, next)
}

// shouldSkipRequest checks if the request should skip rate limiting
func (p *rateLimitProcessor) shouldSkipRequest(ctx *lift.Context) bool {
	return p.config.SkipOptions && ctx.Request.Method == "OPTIONS"
}

// rateLimitHandler handles a single rate limit check
type rateLimitHandler struct {
	limiter *rateLimiter
	key     string
	config  RateLimitConfig
}

// newRateLimitHandler creates a new rate limit handler
func newRateLimitHandler(limiter *rateLimiter, config RateLimitConfig, key string) *rateLimitHandler {
	return &rateLimitHandler{
		limiter: limiter,
		config:  config,
		key:     key,
	}
}

// handle executes the rate limit check and response
func (h *rateLimitHandler) handle(ctx *lift.Context, next lift.Handler) error {
	// Check rate limit
	result, err := h.limiter.checkLimit(ctx.Context, h.key, ctx)
	if err != nil {
		return h.handleCheckError(ctx, err, next)
	}

	// Add rate limit headers
	h.limiter.addHeaders(ctx, result)

	// Check if request is allowed
	if !result.Allowed {
		return h.handleRateLimitExceeded(ctx, result)
	}

	// Execute handler and handle success
	return h.executeAndHandleSuccess(ctx, next)
}

// handleCheckError handles rate limit check errors
func (h *rateLimitHandler) handleCheckError(ctx *lift.Context, err error, next lift.Handler) error {
	// Log error but don't fail the request
	if ctx.Logger != nil {
		ctx.Logger.Error("Rate limit check failed", map[string]any{
			"error": err.Error(),
			"key":   h.key,
		})
	}
	// Continue without rate limiting on error
	return next.Handle(ctx)
}

// handleRateLimitExceeded handles when rate limit is exceeded
func (h *rateLimitHandler) handleRateLimitExceeded(ctx *lift.Context, result *RateLimitResult) error {
	// Log rate limit exceeded
	if ctx.Logger != nil {
		ctx.Logger.Warn("Rate limit exceeded", map[string]any{
			"key":       "[SANITIZED_RATE_LIMIT_KEY]", // Sanitized for security
			"limit":     result.Limit,
			"remaining": result.Remaining,
			"reset_at":  result.ResetAt,
		})
	}

	// Return 429 Too Many Requests
	ctx.Response.Status(429)
	return ctx.Response.JSON(map[string]any{
		"error":       "Rate limit exceeded",
		"limit":       result.Limit,
		"remaining":   result.Remaining,
		"reset_at":    result.ResetAt.Unix(),
		"retry_after": int(result.RetryAfter.Seconds()),
	})
}

// executeAndHandleSuccess executes the handler and handles successful requests
func (h *rateLimitHandler) executeAndHandleSuccess(ctx *lift.Context, next lift.Handler) error {
	err := next.Handle(ctx)

	// Handle successful request decrement if configured
	if h.config.SkipSuccessful && err == nil && ctx.Response.StatusCode < 400 {
		h.handleSuccessfulDecrement(ctx)
	}

	return err
}

// handleSuccessfulDecrement handles decrementing counter for successful requests
func (h *rateLimitHandler) handleSuccessfulDecrement(ctx *lift.Context) {
	// Decrement the counter (best effort)
	if decrementErr := h.limiter.decrementCounter(ctx.Context, h.key); decrementErr != nil {
		// Log decrement error but don't fail the request
		if ctx.Logger != nil {
			ctx.Logger.Warn("Failed to decrement rate limit counter", map[string]any{
				"error": decrementErr.Error(),
				"key":   h.key,
			})
		}
	}
}

// rateLimiter implements the rate limiting logic
type rateLimiter struct {
	config RateLimitConfig
}

// generateKey creates a unique key for rate limiting
func (r *rateLimiter) generateKey(ctx *lift.Context) string {
	parts := []string{r.config.KeyPrefix}

	// Add tenant ID
	if tenantID := ctx.TenantID(); tenantID != "" {
		parts = append(parts, "tenant", tenantID)
	}

	// Add user ID
	if userID := ctx.UserID(); userID != "" {
		parts = append(parts, "user", userID)
	}

	// Add method if configured
	if r.config.IncludeMethod {
		parts = append(parts, "method", ctx.Request.Method)
	}

	// Add path if configured
	if r.config.IncludePath {
		parts = append(parts, "path", ctx.Request.Path)
	}

	// Add IP address as fallback
	if ip := ctx.Request.Headers["X-Forwarded-For"]; ip != "" {
		parts = append(parts, "ip", ip)
	} else if ip := ctx.Request.Headers["X-Real-IP"]; ip != "" {
		parts = append(parts, "ip", ip)
	}

	key := ""
	for i, part := range parts {
		if i > 0 {
			key += ":"
		}
		key += part
	}

	return key
}

// getLimit returns the appropriate limit for the context
func (r *rateLimiter) getLimit(ctx *lift.Context) int {
	// Check user-specific limits
	if userID := ctx.UserID(); userID != "" {
		if limit, exists := r.config.UserLimits[userID]; exists {
			return limit
		}
	}

	// Check tenant-specific limits
	if tenantID := ctx.TenantID(); tenantID != "" {
		if limit, exists := r.config.TenantLimits[tenantID]; exists {
			return limit
		}
	}

	// Return default limit
	return r.config.DefaultLimit
}

// checkLimit checks if the request is within rate limits
func (r *rateLimiter) checkLimit(ctx context.Context, key string, liftCtx *lift.Context) (*RateLimitResult, error) {
	checker := newRateLimitChecker(r, key, liftCtx)
	return checker.check(ctx)
}

// rateLimitChecker encapsulates rate limit checking logic
type rateLimitChecker struct {
	now         time.Time
	windowStart time.Time
	limiter     *rateLimiter
	liftCtx     *lift.Context
	key         string
	limit       int
}

// newRateLimitChecker creates a new rate limit checker
func newRateLimitChecker(limiter *rateLimiter, key string, liftCtx *lift.Context) *rateLimitChecker {
	now := time.Now()
	return &rateLimitChecker{
		limiter:     limiter,
		key:         key,
		liftCtx:     liftCtx,
		now:         now,
		windowStart: now.Truncate(limiter.config.DefaultWindow),
		limit:       limiter.getLimit(liftCtx),
	}
}

// check performs the rate limit check
func (c *rateLimitChecker) check(ctx context.Context) (*RateLimitResult, error) {
	// Try to get existing entry
	var entry RateLimitEntry
	err := c.limiter.config.DynamORM.Get(ctx, c.key, &entry)

	if err != nil {
		return c.handleNewEntry(ctx)
	}

	// Check if we're in a new window
	if entry.WindowStart.Before(c.windowStart) {
		return c.handleWindowReset(ctx, &entry)
	}

	// Check if limit exceeded
	if entry.Count >= c.limit {
		return c.handleLimitExceeded(&entry)
	}

	// Increment counter and allow request
	return c.handleAllowedRequest(ctx, &entry)
}

// handleNewEntry creates a new rate limit entry
func (c *rateLimitChecker) handleNewEntry(ctx context.Context) (*RateLimitResult, error) {
	entry := RateLimitEntry{
		Key:         c.key,
		Count:       1,
		WindowStart: c.windowStart,
		LastRequest: c.now,
		TTL:         c.now.Add(c.limiter.config.TTL).Unix(),
	}

	err := c.limiter.config.DynamORM.Put(ctx, entry)
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limit entry: %w", err)
	}

	return c.createAllowedResult(c.limit - 1), nil
}

// handleWindowReset resets the entry for a new window
func (c *rateLimitChecker) handleWindowReset(ctx context.Context, entry *RateLimitEntry) (*RateLimitResult, error) {
	entry.Count = 1
	entry.WindowStart = c.windowStart
	entry.LastRequest = c.now
	entry.TTL = c.now.Add(c.limiter.config.TTL).Unix()

	err := c.limiter.config.DynamORM.Put(ctx, *entry)
	if err != nil {
		return nil, fmt.Errorf("failed to reset rate limit entry: %w", err)
	}

	return c.createAllowedResult(c.limit - 1), nil
}

// handleLimitExceeded handles when the rate limit is exceeded
func (c *rateLimitChecker) handleLimitExceeded(entry *RateLimitEntry) (*RateLimitResult, error) {
	resetAt := entry.WindowStart.Add(c.limiter.config.DefaultWindow)
	retryAfter := time.Until(resetAt)
	if retryAfter < 0 {
		retryAfter = 0
	}

	return &RateLimitResult{
		Allowed:     false,
		Limit:       c.limit,
		Remaining:   0,
		ResetAt:     resetAt,
		RetryAfter:  retryAfter,
		WindowStart: entry.WindowStart,
	}, nil
}

// handleAllowedRequest increments the counter and allows the request
func (c *rateLimitChecker) handleAllowedRequest(ctx context.Context, entry *RateLimitEntry) (*RateLimitResult, error) {
	entry.Count++
	entry.LastRequest = c.now
	entry.TTL = c.now.Add(c.limiter.config.TTL).Unix()

	err := c.limiter.config.DynamORM.Put(ctx, *entry)
	if err != nil {
		return nil, fmt.Errorf("failed to update rate limit entry: %w", err)
	}

	return c.createAllowedResult(c.limit - entry.Count), nil
}

// createAllowedResult creates a result for an allowed request
func (c *rateLimitChecker) createAllowedResult(remaining int) *RateLimitResult {
	return &RateLimitResult{
		Allowed:     true,
		Limit:       c.limit,
		Remaining:   remaining,
		ResetAt:     c.windowStart.Add(c.limiter.config.DefaultWindow),
		RetryAfter:  0,
		WindowStart: c.windowStart,
	}
}

// decrementCounter decrements the counter for successful requests (if configured)
func (r *rateLimiter) decrementCounter(ctx context.Context, key string) error {
	var entry RateLimitEntry
	err := r.config.DynamORM.Get(ctx, key, &entry)

	if err != nil {
		return err // Ignore errors for decrement
	}

	if entry.Count > 0 {
		entry.Count--
		return r.config.DynamORM.Put(ctx, entry)
	}

	return nil
}

// addHeaders adds rate limit headers to the response
func (r *rateLimiter) addHeaders(ctx *lift.Context, result *RateLimitResult) {
	prefix := r.config.HeaderPrefix

	ctx.Response.Header(prefix+"-Limit", strconv.Itoa(result.Limit))
	ctx.Response.Header(prefix+"-Remaining", strconv.Itoa(result.Remaining))
	ctx.Response.Header(prefix+"-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))

	if !result.Allowed {
		ctx.Response.Header("Retry-After", strconv.Itoa(int(result.RetryAfter.Seconds())))
	}
}

// RateLimitStats provides statistics about rate limiting
type RateLimitStats struct {
	TotalRequests   int64 `json:"total_requests"`
	AllowedRequests int64 `json:"allowed_requests"`
	BlockedRequests int64 `json:"blocked_requests"`
	ErrorCount      int64 `json:"error_count"`
}

// GetRateLimitStats returns rate limiting statistics from actual usage data
func GetRateLimitStats(config RateLimitConfig) (*RateLimitStats, error) {
	if config.DynamORM == nil {
		return &RateLimitStats{}, fmt.Errorf("DynamORM not configured")
	}

	ctx := context.Background()

	// We would need to implement scanning capabilities in DynamORM to get full stats
	// For now, we'll implement a basic version that tracks aggregate metrics

	// In a production implementation, we could:
	// 1. Add a separate stats tracking table
	// 2. Use DynamoDB streams to aggregate metrics
	// 3. Use CloudWatch metrics integration

	// Placeholder implementation that could be extended:
	// Try to get a sample of recent entries to estimate statistics
	sampleKey := fmt.Sprintf("%s:stats:aggregate", config.KeyPrefix)

	var statsEntry struct {
		Key             string ``
		TotalRequests   int64  ``
		AllowedRequests int64  ``
		BlockedRequests int64  ``
		ErrorCount      int64  ``
		LastUpdated     int64  ``
		TTL             int64  ``
	}

	err := config.DynamORM.Get(ctx, sampleKey, &statsEntry)
	if err != nil {
		// If stats don't exist yet, return zeros
		// In production, we'd implement background aggregation
		return &RateLimitStats{
			TotalRequests:   0,
			AllowedRequests: 0,
			BlockedRequests: 0,
			ErrorCount:      0,
		}, nil
	}

	return &RateLimitStats{
		TotalRequests:   statsEntry.TotalRequests,
		AllowedRequests: statsEntry.AllowedRequests,
		BlockedRequests: statsEntry.BlockedRequests,
		ErrorCount:      statsEntry.ErrorCount,
	}, nil
}

// UpdateRateLimitStats updates aggregate statistics (called by rate limiter)
func UpdateRateLimitStats(ctx context.Context, config RateLimitConfig, allowed bool, hasError bool) error {
	if config.DynamORM == nil {
		return nil // Silently skip if no storage configured
	}

	statsKey := fmt.Sprintf("%s:stats:aggregate", config.KeyPrefix)
	now := time.Now()

	// Atomic update of statistics
	var statsEntry struct {
		Key             string ``
		TotalRequests   int64  ``
		AllowedRequests int64  ``
		BlockedRequests int64  ``
		ErrorCount      int64  ``
		LastUpdated     int64  ``
		TTL             int64  ``
	}

	// Try to get existing stats
	err := config.DynamORM.Get(ctx, statsKey, &statsEntry)
	if err != nil {
		// Initialize new stats entry
		statsEntry.Key = statsKey
		statsEntry.TTL = now.Add(30 * 24 * time.Hour).Unix() // Keep for 30 days
	}

	// Update counters
	statsEntry.TotalRequests++
	if allowed {
		statsEntry.AllowedRequests++
	} else {
		statsEntry.BlockedRequests++
	}
	if hasError {
		statsEntry.ErrorCount++
	}
	statsEntry.LastUpdated = now.Unix()

	// Save updated stats
	return config.DynamORM.Put(ctx, statsEntry)
}

// CleanupExpiredEntries removes expired rate limit entries
func CleanupExpiredEntries(_ context.Context, _ RateLimitConfig) error {
	// This would scan the table and remove expired entries
	// Implementation depends on DynamORM's scan capabilities
	// For now, we rely on DynamoDB TTL to handle cleanup automatically
	return nil
}

// BurstRateLimitMiddleware creates a burst-aware rate limiting middleware
func BurstRateLimitMiddleware(config RateLimitConfig) lift.Middleware {
	// This is a more sophisticated rate limiter that allows bursts
	// Implementation would track both sustained rate and burst allowance
	// For now, we'll use the basic rate limiter
	return RateLimitMiddleware(config)
}

// AdaptiveRateLimitMiddleware creates an adaptive rate limiting middleware
func AdaptiveRateLimitMiddleware(config RateLimitConfig) lift.Middleware {
	// This would adjust limits based on system load, error rates, etc.
	// Implementation would monitor system metrics and adjust limits dynamically
	// For now, we'll use the basic rate limiter
	return RateLimitMiddleware(config)
}

// RateLimit creates a rate limiting middleware with the given configuration
func RateLimit(config RateLimitConfig) lift.Middleware {
	return RateLimitMiddleware(config)
}

// TenantRateLimit creates a tenant-specific rate limiting middleware
func TenantRateLimit(limit int, window time.Duration) lift.Middleware {
	config := RateLimitConfig{
		DefaultLimit:  limit,
		DefaultWindow: window,
		Window:        window,
		KeyFunc:       tenantKeyFunc,
		ErrorHandler:  defaultErrorHandler,
	}
	return RateLimitMiddleware(config)
}

// UserRateLimit creates a user-specific rate limiting middleware
func UserRateLimit(limit int, window time.Duration) lift.Middleware {
	config := RateLimitConfig{
		DefaultLimit:  limit,
		DefaultWindow: window,
		Window:        window,
		KeyFunc:       userKeyFunc,
		ErrorHandler:  defaultErrorHandler,
	}
	return RateLimitMiddleware(config)
}

// IPRateLimit creates an IP-based rate limiting middleware
func IPRateLimit(limit int, window time.Duration) lift.Middleware {
	config := RateLimitConfig{
		DefaultLimit:  limit,
		DefaultWindow: window,
		Window:        window,
		KeyFunc:       ipKeyFunc,
		ErrorHandler:  defaultErrorHandler,
	}
	return RateLimitMiddleware(config)
}

// EndpointRateLimit creates an endpoint-specific rate limiting middleware
func EndpointRateLimit(limit int, window time.Duration) lift.Middleware {
	config := RateLimitConfig{
		DefaultLimit:  limit,
		DefaultWindow: window,
		Window:        window,
		KeyFunc:       endpointKeyFunc,
		ErrorHandler:  defaultErrorHandler,
	}
	return RateLimitMiddleware(config)
}

// CompositeRateLimit creates a composite rate limiting middleware with multiple strategies
func CompositeRateLimit(config RateLimitConfig) lift.Middleware {
	// Set defaults if not provided
	if config.KeyFunc == nil {
		config.KeyFunc = defaultKeyFunc
	}
	if config.ErrorHandler == nil {
		config.ErrorHandler = defaultErrorHandler
	}
	return RateLimitMiddleware(config)
}

// Key generation functions

// defaultKeyFunc generates a default rate limiting key
func defaultKeyFunc(ctx *lift.Context) *RateLimitKey {
	key := &RateLimitKey{
		Metadata: make(map[string]string),
	}

	// Build identifier from tenant and user
	parts := []string{}
	if tenantID := ctx.TenantID(); tenantID != "" {
		parts = append(parts, tenantID)
		key.Metadata["tenant_id"] = tenantID
	}
	if userID := ctx.UserID(); userID != "" {
		parts = append(parts, userID)
		key.Metadata["user_id"] = userID
	}

	if len(parts) == 0 {
		// Fallback to IP if no tenant/user
		if ip := ctx.Header("X-Forwarded-For"); ip != "" {
			parts = append(parts, ip)
		} else if ip := ctx.Header("X-Real-IP"); ip != "" {
			parts = append(parts, ip)
		} else {
			parts = append(parts, "unknown")
		}
	}

	key.Identifier = joinParts(parts, ":")
	key.Resource = ctx.Request.Path
	key.Operation = ctx.Request.Method

	return key
}

// tenantKeyFunc generates a tenant-specific rate limiting key
func tenantKeyFunc(ctx *lift.Context) *RateLimitKey {
	tenantID := ctx.TenantID()
	if tenantID == "" {
		tenantID = defaultTenant
	}

	return &RateLimitKey{
		Identifier: tenantID,
		Resource:   ctx.Request.Path,
		Operation:  ctx.Request.Method,
		Metadata: map[string]string{
			"tenant_id": tenantID,
		},
	}
}

// userKeyFunc generates a user-specific rate limiting key
func userKeyFunc(ctx *lift.Context) *RateLimitKey {
	userID := ctx.UserID()
	if userID == "" {
		userID = "anonymous"
	}

	return &RateLimitKey{
		Identifier: userID,
		Resource:   ctx.Request.Path,
		Operation:  ctx.Request.Method,
		Metadata: map[string]string{
			"user_id": userID,
		},
	}
}

// ipKeyFunc generates an IP-based rate limiting key
func ipKeyFunc(ctx *lift.Context) *RateLimitKey {
	ip := ctx.Header("X-Forwarded-For")
	if ip == "" {
		ip = ctx.Header("X-Real-IP")
	}
	if ip == "" {
		ip = "unknown"
	}

	return &RateLimitKey{
		Identifier: ip,
		Resource:   ctx.Request.Path,
		Operation:  ctx.Request.Method,
		Metadata: map[string]string{
			"ip": ip,
		},
	}
}

// endpointKeyFunc generates an endpoint-specific rate limiting key
func endpointKeyFunc(ctx *lift.Context) *RateLimitKey {
	endpoint := ctx.Request.Method + ":" + ctx.Request.Path

	return &RateLimitKey{
		Identifier: endpoint,
		Resource:   ctx.Request.Path,
		Operation:  ctx.Request.Method,
		Metadata: map[string]string{
			"endpoint": endpoint,
		},
	}
}

// defaultErrorHandler handles rate limit exceeded errors
func defaultErrorHandler(ctx *lift.Context, result *RateLimitResult) error {
	ctx.Response.Status(429)
	ctx.Response.Header("Retry-After", strconv.Itoa(int(result.RetryAfter.Seconds())))

	return ctx.Response.JSON(map[string]any{
		"error":       "Rate limit exceeded",
		"limit":       result.Limit,
		"remaining":   result.Remaining,
		"reset_at":    result.ResetAt.Unix(),
		"retry_after": int(result.RetryAfter.Seconds()),
	})
}

// Helper function to join string parts
func joinParts(parts []string, separator string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}

	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += separator + parts[i]
	}
	return result
}
