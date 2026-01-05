// Package streaming provides WebSocket streaming and rate limiting
package streaming

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"go.uber.org/zap"
)

// WebSocketRateLimiter provides rate limiting for WebSocket connections and commands
type WebSocketRateLimiter struct {
	logger *zap.Logger
	repo   interfaces.RateLimitRepository

	// Connection-level rate limiting
	connectionLimits map[string]*ConnectionRateLimit
	connMu           sync.RWMutex

	// Global rate limiting (across all connections)
	globalLimits map[string]*GlobalRateLimit
	globalMu     sync.RWMutex

	// Configuration
	config *WebSocketRateLimitConfig
}

// WebSocketRateLimitConfig defines WebSocket rate limiting configuration
type WebSocketRateLimitConfig struct {
	// Connection limits
	MaxConnectionsPerUser int
	MaxConnectionsPerIP   int
	ConnectionWindowSize  time.Duration

	// Command rate limits (per connection)
	MaxCommandsPerMinute int
	MaxCommandsPerSecond int
	CommandWindowSize    time.Duration

	// Command-specific limits
	CommandLimits map[string]CommandRateLimit

	// Progressive delays
	EnableProgressiveDelays bool
	BaseDelayMillis         int
	MaxDelayMillis          int

	// Burst protection
	BurstWindowSize  time.Duration
	MaxBurstCommands int

	// Penalty configuration
	ViolationThreshold int
	PenaltyDuration    time.Duration
	PenaltyMultiplier  float64
}

// CommandRateLimit defines rate limits for specific WebSocket commands
type CommandRateLimit struct {
	MaxPerMinute   int
	MaxPerHour     int
	BurstLimit     int
	RequiresAuth   bool
	CostMultiplier float64 // Some commands cost more (e.g., searches)
}

// ConnectionRateLimit tracks rate limiting for a single connection
type ConnectionRateLimit struct {
	ConnectionID string
	UserID       string
	IPAddress    string

	// Sliding window for commands
	commandWindow *SlidingWindow

	// Command-specific windows
	commandWindows map[string]*SlidingWindow

	// Violation tracking
	violations    int
	lastViolation time.Time
	penaltyUntil  time.Time

	mu sync.RWMutex
}

// GlobalRateLimit tracks rate limiting across all connections for a user/IP
type GlobalRateLimit struct {
	Identifier string // user:id or ip:address

	// Connection tracking
	activeConnections map[string]time.Time // connection_id -> last_activity

	// Global command window
	commandWindow *SlidingWindow

	// Violation tracking
	violations    int
	lastViolation time.Time
	blockedUntil  time.Time

	mu sync.RWMutex
}

// SlidingWindow implements a time-based sliding window counter
type SlidingWindow struct {
	windowSize  time.Duration
	buckets     map[int64]int // timestamp -> count
	lastCleanup time.Time
	mu          sync.RWMutex
}

// NewSlidingWindow creates a new sliding window counter
func NewSlidingWindow(windowSize time.Duration) *SlidingWindow {
	return &SlidingWindow{
		windowSize:  windowSize,
		buckets:     make(map[int64]int),
		lastCleanup: time.Now(),
	}
}

// Add increments the counter for the current time
func (sw *SlidingWindow) Add(count int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	bucket := now.Unix()
	sw.buckets[bucket] += count

	// Cleanup old buckets periodically
	if now.Sub(sw.lastCleanup) > sw.windowSize {
		sw.cleanup(now)
		sw.lastCleanup = now
	}
}

// Count returns the total count in the window
func (sw *SlidingWindow) Count() int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-sw.windowSize).Unix()

	total := 0
	for timestamp, count := range sw.buckets {
		if timestamp > cutoff {
			total += count
		}
	}

	return total
}

// CountRecent returns the count in a recent time period
func (sw *SlidingWindow) CountRecent(duration time.Duration) int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-duration).Unix()

	total := 0
	for timestamp, count := range sw.buckets {
		if timestamp > cutoff {
			total += count
		}
	}

	return total
}

// cleanup removes old buckets
func (sw *SlidingWindow) cleanup(now time.Time) {
	cutoff := now.Add(-sw.windowSize * 2).Unix()
	for timestamp := range sw.buckets {
		if timestamp < cutoff {
			delete(sw.buckets, timestamp)
		}
	}
}

// NewWebSocketRateLimiter creates a new WebSocket rate limiter
func NewWebSocketRateLimiter(config *WebSocketRateLimitConfig, repo interfaces.RateLimitRepository, logger *zap.Logger) *WebSocketRateLimiter {
	if config == nil {
		config = DefaultWebSocketRateLimitConfig()
	}

	return &WebSocketRateLimiter{
		logger:           logger,
		repo:             repo,
		connectionLimits: make(map[string]*ConnectionRateLimit),
		globalLimits:     make(map[string]*GlobalRateLimit),
		config:           config,
	}
}

// DefaultWebSocketRateLimitConfig returns default WebSocket rate limit configuration
func DefaultWebSocketRateLimitConfig() *WebSocketRateLimitConfig {
	return &WebSocketRateLimitConfig{
		MaxConnectionsPerUser:   10,
		MaxConnectionsPerIP:     20,
		ConnectionWindowSize:    1 * time.Hour,
		MaxCommandsPerMinute:    60,
		MaxCommandsPerSecond:    5,
		CommandWindowSize:       1 * time.Minute,
		EnableProgressiveDelays: true,
		BaseDelayMillis:         100,
		MaxDelayMillis:          5000,
		BurstWindowSize:         5 * time.Second,
		MaxBurstCommands:        10,
		ViolationThreshold:      3,
		PenaltyDuration:         5 * time.Minute,
		PenaltyMultiplier:       0.5,
		CommandLimits: map[string]CommandRateLimit{
			CmdCreateStatus: {
				MaxPerMinute:   10,
				MaxPerHour:     100,
				BurstLimit:     2,
				RequiresAuth:   true,
				CostMultiplier: 2.0,
			},
			CmdSearchStatuses: {
				MaxPerMinute:   30,
				MaxPerHour:     500,
				BurstLimit:     5,
				RequiresAuth:   false,
				CostMultiplier: 3.0,
			},
			CmdGetTimeline: {
				MaxPerMinute:   60,
				MaxPerHour:     1000,
				BurstLimit:     10,
				RequiresAuth:   false,
				CostMultiplier: 1.5,
			},
			CmdFollowUser: {
				MaxPerMinute:   20,
				MaxPerHour:     200,
				BurstLimit:     3,
				RequiresAuth:   true,
				CostMultiplier: 1.0,
			},
			CmdUploadMedia: {
				MaxPerMinute:   5,
				MaxPerHour:     50,
				BurstLimit:     1,
				RequiresAuth:   true,
				CostMultiplier: 5.0,
			},
		},
	}
}

// CheckConnection checks if a new connection should be allowed
func (wrl *WebSocketRateLimiter) CheckConnection(ctx context.Context, userID, ipAddress string) (bool, string, error) {
	// Check global limits for user
	if userID != "" {
		userKey := "user:" + userID
		if !wrl.checkGlobalConnectionLimit(userKey, wrl.config.MaxConnectionsPerUser) {
			return false, "Too many connections for this user", nil
		}
	}

	// Check global limits for IP
	ipKey := "ip:" + ipAddress
	if !wrl.checkGlobalConnectionLimit(ipKey, wrl.config.MaxConnectionsPerIP) {
		return false, "Too many connections from this IP address", nil
	}

	// Check if user/IP is blocked
	if userID != "" {
		blocked, until, err := wrl.repo.IsUserBlocked(ctx, userID)
		if err != nil {
			wrl.logger.Error("failed to check user block status", zap.Error(err))
		}
		if blocked {
			return false, fmt.Sprintf("User blocked until %s", until.Format(time.RFC3339)), nil
		}
	}

	return true, "", nil
}

// OnConnect handles a new WebSocket connection
func (wrl *WebSocketRateLimiter) OnConnect(connectionID, userID, ipAddress string) {
	// Create connection rate limit tracker
	connLimit := &ConnectionRateLimit{
		ConnectionID:   connectionID,
		UserID:         userID,
		IPAddress:      ipAddress,
		commandWindow:  NewSlidingWindow(wrl.config.CommandWindowSize),
		commandWindows: make(map[string]*SlidingWindow),
	}

	wrl.connMu.Lock()
	wrl.connectionLimits[connectionID] = connLimit
	wrl.connMu.Unlock()

	// Update global tracking
	if userID != "" {
		wrl.updateGlobalConnection("user:"+userID, connectionID, true)
	}
	wrl.updateGlobalConnection("ip:"+ipAddress, connectionID, true)

	wrl.logger.Debug("WebSocket connection established",
		zap.String("connection_id", connectionID),
		zap.String("user_id", userID),
		zap.String("ip", ipAddress))
}

// OnDisconnect handles a WebSocket disconnection
func (wrl *WebSocketRateLimiter) OnDisconnect(connectionID string) {
	wrl.connMu.Lock()
	connLimit, exists := wrl.connectionLimits[connectionID]
	if exists {
		delete(wrl.connectionLimits, connectionID)
	}
	wrl.connMu.Unlock()

	if !exists {
		return
	}

	// Update global tracking
	if connLimit.UserID != "" {
		wrl.updateGlobalConnection("user:"+connLimit.UserID, connectionID, false)
	}
	wrl.updateGlobalConnection("ip:"+connLimit.IPAddress, connectionID, false)

	wrl.logger.Debug("WebSocket connection closed",
		zap.String("connection_id", connectionID))
}

// CheckCommand checks if a WebSocket command should be allowed
func (wrl *WebSocketRateLimiter) CheckCommand(ctx context.Context, connectionID string, command *Command) (bool, time.Duration, error) {
	wrl.connMu.RLock()
	connLimit, exists := wrl.connectionLimits[connectionID]
	wrl.connMu.RUnlock()

	if !exists {
		return false, 0, fmt.Errorf("connection not found")
	}

	connLimit.mu.Lock()
	defer connLimit.mu.Unlock()

	// Check if connection is in penalty
	now := time.Now()
	if now.Before(connLimit.penaltyUntil) {
		delay := connLimit.penaltyUntil.Sub(now)
		return false, delay, fmt.Errorf("connection in penalty period")
	}

	// Get command-specific limits
	cmdLimit, exists := wrl.config.CommandLimits[command.Type]
	if !exists {
		// Use default limits
		cmdLimit = CommandRateLimit{
			MaxPerMinute:   wrl.config.MaxCommandsPerMinute,
			BurstLimit:     wrl.config.MaxBurstCommands,
			CostMultiplier: 1.0,
		}
	}

	// Calculate command cost
	cost := int(cmdLimit.CostMultiplier)
	if cost < 1 {
		cost = 1
	}

	// Check burst limit
	recentCount := connLimit.commandWindow.CountRecent(wrl.config.BurstWindowSize)
	if recentCount+cost > cmdLimit.BurstLimit {
		// Burst limit exceeded
		connLimit.violations++
		connLimit.lastViolation = now

		if connLimit.violations >= wrl.config.ViolationThreshold {
			// Apply penalty
			connLimit.penaltyUntil = now.Add(wrl.config.PenaltyDuration)

			// Record violation in database
			go wrl.recordViolation(ctx, connLimit.UserID, connLimit.IPAddress, command.Type)

			return false, wrl.config.PenaltyDuration, fmt.Errorf("burst limit exceeded, penalty applied")
		}

		// Progressive delay
		delay := wrl.calculateProgressiveDelay(connLimit.violations)
		return false, delay, fmt.Errorf("burst limit exceeded")
	}

	// Check rate limit
	currentCount := connLimit.commandWindow.Count()
	if currentCount+cost > cmdLimit.MaxPerMinute {
		// Rate limit exceeded
		connLimit.violations++
		connLimit.lastViolation = now

		if connLimit.violations >= wrl.config.ViolationThreshold {
			// Apply penalty
			connLimit.penaltyUntil = now.Add(wrl.config.PenaltyDuration)

			// Record violation
			go wrl.recordViolation(ctx, connLimit.UserID, connLimit.IPAddress, command.Type)

			return false, wrl.config.PenaltyDuration, fmt.Errorf("rate limit exceeded, penalty applied")
		}

		// Calculate time until next allowed request
		delay := time.Minute - time.Since(connLimit.lastViolation)
		if delay < 0 {
			delay = time.Second
		}

		return false, delay, fmt.Errorf("rate limit exceeded")
	}

	// Check command-specific window
	cmdWindow, exists := connLimit.commandWindows[command.Type]
	if !exists {
		cmdWindow = NewSlidingWindow(1 * time.Hour)
		connLimit.commandWindows[command.Type] = cmdWindow
	}

	// Check hourly limit for this specific command
	hourlyCount := cmdWindow.Count()
	if cmdLimit.MaxPerHour > 0 && hourlyCount+cost > cmdLimit.MaxPerHour {
		return false, time.Hour, fmt.Errorf("hourly limit exceeded for command type")
	}

	// Update counters
	connLimit.commandWindow.Add(cost)
	cmdWindow.Add(cost)

	// Update global counters
	wrl.updateGlobalCommand(connLimit.UserID, connLimit.IPAddress, cost)

	// Reset violations on successful command
	if connLimit.violations > 0 && now.Sub(connLimit.lastViolation) > 5*time.Minute {
		connLimit.violations = 0
	}

	return true, 0, nil
}

// checkGlobalConnectionLimit checks if global connection limit is exceeded
func (wrl *WebSocketRateLimiter) checkGlobalConnectionLimit(identifier string, maxConnections int) bool {
	wrl.globalMu.RLock()
	globalLimit, exists := wrl.globalLimits[identifier]
	wrl.globalMu.RUnlock()

	if !exists {
		return true // No limit data, allow connection
	}

	globalLimit.mu.RLock()
	defer globalLimit.mu.RUnlock()

	// Check if blocked
	if time.Now().Before(globalLimit.blockedUntil) {
		return false
	}

	// Count active connections
	activeCount := 0
	cutoff := time.Now().Add(-wrl.config.ConnectionWindowSize)
	for _, lastActivity := range globalLimit.activeConnections {
		if lastActivity.After(cutoff) {
			activeCount++
		}
	}

	return activeCount < maxConnections
}

// updateGlobalConnection updates global connection tracking
func (wrl *WebSocketRateLimiter) updateGlobalConnection(identifier, connectionID string, connect bool) {
	wrl.globalMu.Lock()
	globalLimit, exists := wrl.globalLimits[identifier]
	if !exists {
		globalLimit = &GlobalRateLimit{
			Identifier:        identifier,
			activeConnections: make(map[string]time.Time),
			commandWindow:     NewSlidingWindow(wrl.config.CommandWindowSize),
		}
		wrl.globalLimits[identifier] = globalLimit
	}
	wrl.globalMu.Unlock()

	globalLimit.mu.Lock()
	defer globalLimit.mu.Unlock()

	if connect {
		globalLimit.activeConnections[connectionID] = time.Now()
	} else {
		delete(globalLimit.activeConnections, connectionID)

		// Cleanup if no active connections
		if len(globalLimit.activeConnections) == 0 {
			wrl.globalMu.Lock()
			delete(wrl.globalLimits, identifier)
			wrl.globalMu.Unlock()
		}
	}
}

// updateGlobalCommand updates global command counters
func (wrl *WebSocketRateLimiter) updateGlobalCommand(userID, ipAddress string, cost int) {
	// Update user global counter
	if userID != "" {
		wrl.updateGlobalCommandCounter("user:"+userID, cost)
	}

	// Update IP global counter
	wrl.updateGlobalCommandCounter("ip:"+ipAddress, cost)
}

// updateGlobalCommandCounter updates a specific global command counter
func (wrl *WebSocketRateLimiter) updateGlobalCommandCounter(identifier string, cost int) {
	wrl.globalMu.RLock()
	globalLimit, exists := wrl.globalLimits[identifier]
	wrl.globalMu.RUnlock()

	if !exists {
		return
	}

	globalLimit.mu.Lock()
	defer globalLimit.mu.Unlock()

	globalLimit.commandWindow.Add(cost)

	// Check if global limit exceeded
	if globalLimit.commandWindow.Count() > wrl.config.MaxCommandsPerMinute*len(globalLimit.activeConnections) {
		globalLimit.violations++
		globalLimit.lastViolation = time.Now()

		if globalLimit.violations >= wrl.config.ViolationThreshold {
			// Block all connections from this identifier
			globalLimit.blockedUntil = time.Now().Add(wrl.config.PenaltyDuration)
		}
	}
}

// calculateProgressiveDelay calculates progressive delay based on violations
func (wrl *WebSocketRateLimiter) calculateProgressiveDelay(violations int) time.Duration {
	if !wrl.config.EnableProgressiveDelays || violations == 0 {
		return 0
	}

	// Exponential backoff
	delay := time.Duration(wrl.config.BaseDelayMillis) * time.Millisecond
	for i := 1; i < violations; i++ {
		delay = delay * 2
		if delay > time.Duration(wrl.config.MaxDelayMillis)*time.Millisecond {
			delay = time.Duration(wrl.config.MaxDelayMillis) * time.Millisecond
			break
		}
	}

	return delay
}

// recordViolation records a rate limit violation in the database
func (wrl *WebSocketRateLimiter) recordViolation(ctx context.Context, userID, ipAddress, commandType string) {
	// Record for user if authenticated
	if userID != "" {
		if err := wrl.repo.CheckAPIRateLimit(ctx, userID, "websocket:"+commandType,
			wrl.config.MaxCommandsPerMinute, wrl.config.CommandWindowSize); err != nil {
			wrl.logger.Error("failed to record user violation",
				zap.String("user_id", userID),
				zap.String("command", commandType),
				zap.Error(err))
		}
	}

	// Record for IP
	if err := wrl.repo.CheckAPIRateLimit(ctx, "ip:"+ipAddress, "websocket:"+commandType,
		wrl.config.MaxCommandsPerMinute, wrl.config.CommandWindowSize); err != nil {
		wrl.logger.Error("failed to record IP violation",
			zap.String("ip", ipAddress),
			zap.String("command", commandType),
			zap.Error(err))
	}
}

// GetConnectionStatus returns the rate limit status for a connection
func (wrl *WebSocketRateLimiter) GetConnectionStatus(connectionID string) (map[string]interface{}, error) {
	wrl.connMu.RLock()
	connLimit, exists := wrl.connectionLimits[connectionID]
	wrl.connMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("connection not found")
	}

	connLimit.mu.RLock()
	defer connLimit.mu.RUnlock()

	status := map[string]interface{}{
		"connection_id":  connectionID,
		"user_id":        connLimit.UserID,
		"ip_address":     connLimit.IPAddress,
		"commands_count": connLimit.commandWindow.Count(),
		"violations":     connLimit.violations,
		"in_penalty":     time.Now().Before(connLimit.penaltyUntil),
	}

	if time.Now().Before(connLimit.penaltyUntil) {
		status["penalty_until"] = connLimit.penaltyUntil.Format(time.RFC3339)
		status["penalty_remaining"] = time.Until(connLimit.penaltyUntil).Seconds()
	}

	// Add command-specific counts
	cmdCounts := make(map[string]int)
	for cmdType, window := range connLimit.commandWindows {
		cmdCounts[cmdType] = window.Count()
	}
	status["command_counts"] = cmdCounts

	return status, nil
}

// ResetConnection resets rate limits for a specific connection (admin function)
func (wrl *WebSocketRateLimiter) ResetConnection(connectionID string) error {
	wrl.connMu.Lock()
	defer wrl.connMu.Unlock()

	connLimit, exists := wrl.connectionLimits[connectionID]
	if !exists {
		return fmt.Errorf("connection not found")
	}

	// Reset the connection's rate limit data
	connLimit.mu.Lock()
	connLimit.commandWindow = NewSlidingWindow(wrl.config.CommandWindowSize)
	connLimit.commandWindows = make(map[string]*SlidingWindow)
	connLimit.violations = 0
	connLimit.lastViolation = time.Time{}
	connLimit.penaltyUntil = time.Time{}
	connLimit.mu.Unlock()

	wrl.logger.Info("reset rate limits for connection",
		zap.String("connection_id", connectionID))

	return nil
}
