// Package middleware provides HTTP middleware for the Lesser application
package middleware

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// RateLimiterConfig defines rate limiting configuration
type RateLimiterConfig struct {
	// Default limits
	DefaultRequestsPerMinute int
	DefaultBurstSize         int

	// Endpoint-specific limits
	EndpointLimits map[string]EndpointLimit

	// Progressive delay configuration
	EnableProgressiveDelays bool
	BaseDelayMillis         int
	MaxDelayMillis          int
	DelayIncrementFactor    float64

	// Sliding window configuration
	WindowSize        time.Duration
	WindowGranularity time.Duration // How often to update the window

	// Penalty configuration
	ViolationThreshold int           // Number of violations before penalty
	PenaltyDuration    time.Duration // How long to penalize
	PenaltyMultiplier  float64       // Reduce rate limit by this factor during penalty
}

// EndpointLimit defines rate limits for a specific endpoint
type EndpointLimit struct {
	RequestsPerMinute int
	BurstSize         int
	WindowSize        time.Duration
	// Specific limits for authenticated vs anonymous users
	AuthenticatedRPM int
	AnonymousRPM     int
}

// SlidingWindowCounter tracks requests in a sliding window
type SlidingWindowCounter struct {
	mu          sync.RWMutex
	buckets     map[int64]int // timestamp -> count
	windowSize  time.Duration
	granularity time.Duration
	lastCleanup time.Time
}

// NewSlidingWindowCounter creates a new sliding window counter
func NewSlidingWindowCounter(windowSize, granularity time.Duration) *SlidingWindowCounter {
	return &SlidingWindowCounter{
		buckets:     make(map[int64]int),
		windowSize:  windowSize,
		granularity: granularity,
		lastCleanup: time.Now(),
	}
}

// Increment adds a request to the current window
func (s *SlidingWindowCounter) Increment() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	bucket := now.Truncate(s.granularity).Unix()
	s.buckets[bucket]++

	// Cleanup old buckets periodically
	if now.Sub(s.lastCleanup) > s.windowSize {
		s.cleanupOldBuckets(now)
		s.lastCleanup = now
	}
}

// Count returns the total count in the sliding window
func (s *SlidingWindowCounter) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-s.windowSize).Unix()

	total := 0
	for timestamp, count := range s.buckets {
		if timestamp >= cutoff {
			total += count
		}
	}

	return total
}

// cleanupOldBuckets removes buckets outside the window
func (s *SlidingWindowCounter) cleanupOldBuckets(now time.Time) {
	cutoff := now.Add(-s.windowSize * 2).Unix() // Keep some buffer

	for timestamp := range s.buckets {
		if timestamp < cutoff {
			delete(s.buckets, timestamp)
		}
	}
}

// RateLimiter provides enhanced rate limiting with sliding windows
type RateLimiter struct {
	config *RateLimiterConfig
	repo   *repositories.RateLimitRepository
	logger *zap.Logger

	// In-memory sliding window counters for performance
	counters  map[string]*SlidingWindowCounter
	counterMu sync.RWMutex

	// Progressive delay tracking
	delays  map[string]time.Time // track last request time for delays
	delayMu sync.RWMutex
}

// NewRateLimiter creates a new enhanced rate limiter
func NewRateLimiter(config *RateLimiterConfig, repo *repositories.RateLimitRepository, logger *zap.Logger) *RateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}

	return &RateLimiter{
		config:   config,
		repo:     repo,
		logger:   logger,
		counters: make(map[string]*SlidingWindowCounter),
		delays:   make(map[string]time.Time),
	}
}

// DefaultRateLimiterConfig returns default rate limiter configuration
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		DefaultRequestsPerMinute: 60,
		DefaultBurstSize:         10,
		EnableProgressiveDelays:  true,
		BaseDelayMillis:          100,
		MaxDelayMillis:           5000,
		DelayIncrementFactor:     1.5,
		WindowSize:               1 * time.Minute,
		WindowGranularity:        1 * time.Second,
		ViolationThreshold:       3,
		PenaltyDuration:          5 * time.Minute,
		PenaltyMultiplier:        0.5,
		EndpointLimits: map[string]EndpointLimit{
			"/api/v1/statuses": {
				RequestsPerMinute: 30,
				BurstSize:         5,
				AuthenticatedRPM:  60,
				AnonymousRPM:      10,
			},
			"/api/v1/timelines/public": {
				RequestsPerMinute: 120,
				BurstSize:         20,
				AuthenticatedRPM:  180,
				AnonymousRPM:      60,
			},
			"/api/v1/accounts/verify_credentials": {
				RequestsPerMinute: 60,
				BurstSize:         10,
			},
			"/api/v1/media": {
				RequestsPerMinute: 10,
				BurstSize:         2,
				AuthenticatedRPM:  20,
				AnonymousRPM:      5,
			},
			"/api/v1/search": {
				RequestsPerMinute: 30,
				BurstSize:         5,
				AuthenticatedRPM:  60,
				AnonymousRPM:      15,
			},
		},
	}
}

// Middleware returns the rate limiting middleware for Lift
func (rl *RateLimiter) Middleware() func(lift.HandlerFunc) lift.HandlerFunc {
	return func(next lift.HandlerFunc) lift.HandlerFunc {
		return func(ctx *lift.Context) error {
			// Extract client identifier
			clientID := rl.getClientID(ctx)
			endpoint := rl.normalizeEndpoint(ctx.Request.Path)

			// Check if client is allowed to proceed
			allowed, delay, headers := rl.checkRateLimit(ctx.Context, clientID, endpoint, rl.isAuthenticated(ctx))

			// Set rate limit headers
			for key, value := range headers {
				ctx.Set(key, value)
			}

			if !allowed {
				// Apply progressive delay if configured
				if rl.config.EnableProgressiveDelays && delay > 0 {
					time.Sleep(delay)
				}

				// Return rate limit error
				return ctx.Status(429).JSON(map[string]interface{}{
					"error":       "Too Many Requests",
					"message":     "Rate limit exceeded. Please slow down.",
					"retry_after": headers["Retry-After"],
				})
			}

			// Apply small progressive delay if approaching limit
			if rl.config.EnableProgressiveDelays {
				rl.applyProgressiveDelay(clientID)
			}

			// Continue to next handler
			return next(ctx)
		}
	}
}

// checkRateLimit checks if a request should be allowed
func (rl *RateLimiter) checkRateLimit(ctx context.Context, clientID, endpoint string, authenticated bool) (bool, time.Duration, map[string]string) {
	// Get endpoint-specific configuration
	limit := rl.getEndpointLimit(endpoint, authenticated)

	// Get or create sliding window counter
	counter := rl.getOrCreateCounter(clientID, endpoint)

	// Check sliding window count
	currentCount := counter.Count()

	// Calculate remaining requests
	remaining := limit.RequestsPerMinute - currentCount
	if remaining < 0 {
		remaining = 0
	}

	// Prepare headers
	headers := map[string]string{
		"X-RateLimit-Limit":     strconv.Itoa(limit.RequestsPerMinute),
		"X-RateLimit-Remaining": strconv.Itoa(remaining),
		"X-RateLimit-Reset":     strconv.FormatInt(time.Now().Add(rl.config.WindowSize).Unix(), 10),
	}

	// Check if rate limit exceeded
	if currentCount >= limit.RequestsPerMinute {
		// Check for violations and apply penalties
		violationCount, err := rl.repo.GetViolationCount(ctx, clientID, "", rl.config.WindowSize)
		if err != nil {
			rl.logger.Warn("failed to get violation count",
				zap.String("client_id", clientID),
				zap.Error(err))
			violationCount = 0
		}

		// Calculate delay based on violations
		delay := rl.calculateProgressiveDelay(violationCount)
		headers["Retry-After"] = strconv.Itoa(int(delay.Seconds()))

		// Record violation if threshold exceeded
		if violationCount >= rl.config.ViolationThreshold {
			if err := rl.repo.CheckAPIRateLimit(ctx, clientID, endpoint, limit.RequestsPerMinute, rl.config.WindowSize); err != nil {
				rl.logger.Error("failed to record rate limit violation",
					zap.String("client_id", clientID),
					zap.String("endpoint", endpoint),
					zap.Error(err))
			}
		}

		return false, delay, headers
	}

	// Check burst limit
	recentCount := rl.getRecentBurstCount(counter, 5*time.Second)
	if recentCount > limit.BurstSize {
		delay := 1 * time.Second
		headers["Retry-After"] = "1"
		return false, delay, headers
	}

	// Increment counter
	counter.Increment()

	// Update database asynchronously for persistence
	go func() {
		if err := rl.repo.CheckAPIRateLimit(context.Background(), clientID, endpoint, limit.RequestsPerMinute, rl.config.WindowSize); err != nil {
			rl.logger.Warn("failed to update rate limit in database",
				zap.String("client_id", clientID),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}
	}()

	return true, 0, headers
}

// getRecentBurstCount gets the count of very recent requests for burst detection
func (rl *RateLimiter) getRecentBurstCount(counter *SlidingWindowCounter, duration time.Duration) int {
	// This is a simplified version - in production you'd want more granular tracking
	// For now, we'll use a percentage of the total count as an approximation
	totalCount := counter.Count()
	windowRatio := float64(duration) / float64(rl.config.WindowSize)
	return int(float64(totalCount) * windowRatio * 2) // Multiply by 2 to be more sensitive to bursts
}

// applyProgressiveDelay applies a small delay that increases with request rate
func (rl *RateLimiter) applyProgressiveDelay(clientID string) {
	rl.delayMu.Lock()
	defer rl.delayMu.Unlock()

	now := time.Now()
	lastRequest, exists := rl.delays[clientID]

	if !exists {
		rl.delays[clientID] = now
		return
	}

	// Calculate time since last request
	timeSinceLastRequest := now.Sub(lastRequest)

	// If requests are coming too fast, apply progressive delay
	if timeSinceLastRequest < 100*time.Millisecond {
		// Start with base delay
		delay := time.Duration(rl.config.BaseDelayMillis) * time.Millisecond

		// Increase delay based on how fast requests are coming
		if timeSinceLastRequest < 50*time.Millisecond {
			delay = time.Duration(float64(delay) * rl.config.DelayIncrementFactor)
		}
		if timeSinceLastRequest < 25*time.Millisecond {
			delay = time.Duration(float64(delay) * rl.config.DelayIncrementFactor)
		}

		// Cap at max delay
		if delay > time.Duration(rl.config.MaxDelayMillis)*time.Millisecond {
			delay = time.Duration(rl.config.MaxDelayMillis) * time.Millisecond
		}

		time.Sleep(delay)
	}

	rl.delays[clientID] = now

	// Cleanup old entries periodically
	if len(rl.delays) > 10000 {
		cutoff := now.Add(-5 * time.Minute)
		for id, timestamp := range rl.delays {
			if timestamp.Before(cutoff) {
				delete(rl.delays, id)
			}
		}
	}
}

// calculateProgressiveDelay calculates delay based on violation count
func (rl *RateLimiter) calculateProgressiveDelay(violationCount int) time.Duration {
	if violationCount == 0 {
		return 0
	}

	// Exponential backoff with jitter
	baseDelay := time.Duration(rl.config.BaseDelayMillis) * time.Millisecond
	// Safely convert to uint with bounds checking
	exponent := violationCount - 1
	if exponent < 0 {
		exponent = 0
	}
	if exponent > 30 { // Cap to prevent overflow (2^30 is large enough)
		exponent = 30
	}
	// Safe conversion to uint after bounds checking
	exponentUint := uint(exponent)                      //nolint:gosec // G115: bounded by check above
	delay := baseDelay * time.Duration(1<<exponentUint) // 2^(violations-1)

	// Cap at max delay
	maxDelay := time.Duration(rl.config.MaxDelayMillis) * time.Millisecond
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// getOrCreateCounter gets or creates a sliding window counter for a client/endpoint
func (rl *RateLimiter) getOrCreateCounter(clientID, endpoint string) *SlidingWindowCounter {
	key := fmt.Sprintf("%s:%s", clientID, endpoint)

	rl.counterMu.RLock()
	counter, exists := rl.counters[key]
	rl.counterMu.RUnlock()

	if exists {
		return counter
	}

	rl.counterMu.Lock()
	defer rl.counterMu.Unlock()

	// Double-check after acquiring write lock
	if counter, exists = rl.counters[key]; exists {
		return counter
	}

	// Create new counter
	counter = NewSlidingWindowCounter(rl.config.WindowSize, rl.config.WindowGranularity)
	rl.counters[key] = counter

	// Cleanup old counters if too many
	if len(rl.counters) > 10000 {
		rl.cleanupOldCounters()
	}

	return counter
}

// cleanupOldCounters removes inactive counters
func (rl *RateLimiter) cleanupOldCounters() {
	// Remove counters with zero recent activity
	for key, counter := range rl.counters {
		if counter.Count() == 0 {
			delete(rl.counters, key)
		}
	}
}

// getEndpointLimit gets the rate limit configuration for an endpoint
func (rl *RateLimiter) getEndpointLimit(endpoint string, authenticated bool) EndpointLimit {
	// Check for exact match
	if limit, exists := rl.config.EndpointLimits[endpoint]; exists {
		// Adjust for authentication status
		if authenticated && limit.AuthenticatedRPM > 0 {
			limit.RequestsPerMinute = limit.AuthenticatedRPM
		} else if !authenticated && limit.AnonymousRPM > 0 {
			limit.RequestsPerMinute = limit.AnonymousRPM
		}
		return limit
	}

	// Check for pattern match (e.g., /api/v1/accounts/*)
	for pattern, limit := range rl.config.EndpointLimits {
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(endpoint, prefix) {
				if authenticated && limit.AuthenticatedRPM > 0 {
					limit.RequestsPerMinute = limit.AuthenticatedRPM
				} else if !authenticated && limit.AnonymousRPM > 0 {
					limit.RequestsPerMinute = limit.AnonymousRPM
				}
				return limit
			}
		}
	}

	// Return default limit
	rpm := rl.config.DefaultRequestsPerMinute
	if authenticated {
		rpm = rpm * 2 // Authenticated users get double the rate limit
	}

	return EndpointLimit{
		RequestsPerMinute: rpm,
		BurstSize:         rl.config.DefaultBurstSize,
		WindowSize:        rl.config.WindowSize,
	}
}

// normalizeEndpoint normalizes the endpoint path for rate limiting
func (rl *RateLimiter) normalizeEndpoint(path string) string {
	// Remove query parameters
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	// Remove trailing slash
	path = strings.TrimSuffix(path, "/")

	// Normalize common patterns
	// e.g., /api/v1/accounts/123 -> /api/v1/accounts/*
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[1] == "api" && parts[2] == "v1" {
		// Check if last part looks like an ID
		lastPart := parts[len(parts)-1]
		if isID(lastPart) {
			parts[len(parts)-1] = "*"
			path = strings.Join(parts, "/")
		}
	}

	return path
}

// isID checks if a string looks like an ID (numeric or UUID-like)
func isID(s string) bool {
	// Check if numeric
	if _, err := strconv.Atoi(s); err == nil {
		return true
	}

	// Check if it looks like a UUID or base64 ID
	if len(s) >= 16 && !strings.Contains(s, " ") {
		return true
	}

	return false
}

// getClientID extracts a client identifier from the request
func (rl *RateLimiter) getClientID(ctx *lift.Context) string {
	// Try to get authenticated user ID first
	if userID := ctx.Get("user_id"); userID != nil {
		if id, ok := userID.(string); ok && id != "" {
			return "user:" + id
		}
	}

	// Try to get from JWT claims
	if claims := ctx.Get("claims"); claims != nil {
		if claimsMap, ok := claims.(map[string]interface{}); ok {
			if userID, ok := claimsMap["sub"].(string); ok && userID != "" {
				return "user:" + userID
			}
		}
	}

	// Fall back to IP address
	ip := rl.getClientIP(ctx)
	return "ip:" + ip
}

// getClientIP extracts the client IP address
func (rl *RateLimiter) getClientIP(ctx *lift.Context) string {
	// Check X-Forwarded-For header (for proxies/load balancers)
	if xff := ctx.Header("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return common.SanitizeInput(xff[:idx])
		}
		return common.SanitizeInput(xff)
	}

	// Check X-Real-IP header
	if xri := ctx.Header("X-Real-IP"); xri != "" {
		return common.SanitizeInput(xri)
	}

	// Fall back to remote address from Lambda context if available
	if ctx.Request != nil && ctx.Request.Request != nil {
		// Try to get from headers map
		if headers := ctx.Request.Request.Headers; headers != nil {
			if sourceIP, ok := headers["X-Forwarded-For"]; ok && sourceIP != "" {
				return sourceIP
			}
		}
	}

	return "unknown"
}

// isAuthenticated checks if the request is authenticated
func (rl *RateLimiter) isAuthenticated(ctx *lift.Context) bool {
	// Check for user_id in context
	if userID := ctx.Get("user_id"); userID != nil {
		if id, ok := userID.(string); ok && id != "" {
			return true
		}
	}

	// Check for Authorization header
	if auth := ctx.Header("Authorization"); auth != "" && strings.HasPrefix(auth, "Bearer ") {
		return true
	}

	return false
}

// GetRateLimitStatus returns the current rate limit status for a client
func (rl *RateLimiter) GetRateLimitStatus(ctx context.Context, clientID, endpoint string) (map[string]interface{}, error) {
	// Get endpoint configuration
	authenticated := strings.HasPrefix(clientID, "user:")
	limit := rl.getEndpointLimit(endpoint, authenticated)

	// Get current count
	counter := rl.getOrCreateCounter(clientID, endpoint)
	currentCount := counter.Count()

	// Get violation count
	violationCount, err := rl.repo.GetViolationCount(ctx, clientID, "", 24*time.Hour)
	if err != nil {
		rl.logger.Warn("failed to get violation count",
			zap.String("client_id", clientID),
			zap.Error(err))
		violationCount = 0
	}

	// Calculate remaining
	remaining := limit.RequestsPerMinute - currentCount
	if remaining < 0 {
		remaining = 0
	}

	status := map[string]interface{}{
		"limit":         limit.RequestsPerMinute,
		"remaining":     remaining,
		"reset":         time.Now().Add(rl.config.WindowSize).Unix(),
		"current_count": currentCount,
		"violations":    violationCount,
		"window_size":   rl.config.WindowSize.String(),
		"burst_limit":   limit.BurstSize,
	}

	// Add penalty info if applicable
	if violationCount >= rl.config.ViolationThreshold {
		status["penalty_active"] = true
		status["penalty_multiplier"] = rl.config.PenaltyMultiplier
		status["effective_limit"] = int(float64(limit.RequestsPerMinute) * rl.config.PenaltyMultiplier)
	}

	return status, nil
}

// ResetClientLimits resets rate limits for a specific client (admin function)
func (rl *RateLimiter) ResetClientLimits(ctx context.Context, clientID string) error {
	// Clear in-memory counters
	rl.counterMu.Lock()
	for key := range rl.counters {
		if strings.HasPrefix(key, clientID+":") {
			delete(rl.counters, key)
		}
	}
	rl.counterMu.Unlock()

	// Clear delays
	rl.delayMu.Lock()
	delete(rl.delays, clientID)
	rl.delayMu.Unlock()

	// Clear database records
	if err := rl.repo.ClearLoginAttempts(ctx, clientID); err != nil {
		return fmt.Errorf("failed to clear database rate limits: %w", err)
	}

	rl.logger.Info("reset rate limits for client",
		zap.String("client_id", clientID))

	return nil
}
