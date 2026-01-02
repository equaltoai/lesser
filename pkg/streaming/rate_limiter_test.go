package streaming

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubRateLimitRepo struct {
	isUserBlocked     func(ctx context.Context, userID string) (bool, time.Time, error)
	checkAPIRateLimit func(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error
}

func (r *stubRateLimitRepo) RecordLoginAttempt(context.Context, string, bool) error { return nil }
func (r *stubRateLimitRepo) GetLoginAttemptCount(context.Context, string, time.Time) (int, error) {
	return 0, nil
}
func (r *stubRateLimitRepo) IsRateLimited(context.Context, string) (bool, time.Time, error) {
	return false, time.Time{}, nil
}
func (r *stubRateLimitRepo) ClearLoginAttempts(context.Context, string) error { return nil }
func (r *stubRateLimitRepo) CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	if r.checkAPIRateLimit == nil {
		return nil
	}
	return r.checkAPIRateLimit(ctx, userID, endpoint, limit, window)
}
func (r *stubRateLimitRepo) GetAPIRateLimitInfo(context.Context, string, string, int, time.Duration) (int, time.Time, error) {
	return 0, time.Time{}, nil
}
func (r *stubRateLimitRepo) CheckFederationRateLimit(context.Context, string, string, int, time.Duration) error {
	return nil
}
func (r *stubRateLimitRepo) GetFederationRateLimitInfo(context.Context, string, string, int, time.Duration) (int, time.Time, error) {
	return 0, time.Time{}, nil
}
func (r *stubRateLimitRepo) GetViolationCount(context.Context, string, string, time.Duration) (int, error) {
	return 0, nil
}
func (r *stubRateLimitRepo) IsUserBlocked(ctx context.Context, userID string) (bool, time.Time, error) {
	if r.isUserBlocked == nil {
		return false, time.Time{}, nil
	}
	return r.isUserBlocked(ctx, userID)
}
func (r *stubRateLimitRepo) IsDomainBlocked(context.Context, string) (bool, time.Time, error) {
	return false, time.Time{}, nil
}
func (r *stubRateLimitRepo) CheckCommunityNoteRateLimit(context.Context, string, int) (bool, int, error) {
	return true, 0, nil
}

var _ interfaces.RateLimitRepository = (*stubRateLimitRepo)(nil)

func TestSlidingWindow_Cleanup(t *testing.T) {
	sw := NewSlidingWindow(10 * time.Second)

	now := time.Unix(1000, 0).UTC()
	sw.buckets[time.Unix(900, 0).Unix()] = 1  // stale
	sw.buckets[time.Unix(999, 0).Unix()] = 10 // fresh

	sw.cleanup(now)

	_, staleExists := sw.buckets[time.Unix(900, 0).Unix()]
	assert.False(t, staleExists)
	_, freshExists := sw.buckets[time.Unix(999, 0).Unix()]
	assert.True(t, freshExists)
}

func TestWebSocketRateLimiter_CheckConnection_GlobalLimitsAndBlocks(t *testing.T) {
	repo := &stubRateLimitRepo{
		isUserBlocked: func(context.Context, string) (bool, time.Time, error) {
			return true, time.Unix(123, 0).UTC(), nil
		},
	}
	config := DefaultWebSocketRateLimitConfig()
	config.MaxConnectionsPerUser = 1
	config.MaxConnectionsPerIP = 1

	limiter := NewWebSocketRateLimiter(config, repo, zap.NewNop())
	limiter.OnConnect("c1", "u1", "1.1.1.1")

	allowed, msg, err := limiter.CheckConnection(context.Background(), "u1", "1.1.1.1")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Contains(t, msg, "Too many connections for this user")

	allowed, msg, err = limiter.CheckConnection(context.Background(), "u2", "2.2.2.2")
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Contains(t, msg, "User blocked until")
}

func TestWebSocketRateLimiter_OnConnectAndDisconnect_GlobalCleanup(t *testing.T) {
	limiter := NewWebSocketRateLimiter(DefaultWebSocketRateLimitConfig(), &stubRateLimitRepo{}, zap.NewNop())

	limiter.OnConnect("c1", "u1", "1.1.1.1")
	limiter.OnConnect("c2", "u1", "1.1.1.1")

	// Disconnect one connection keeps global tracking.
	limiter.OnDisconnect("c1")
	_, stillExists := limiter.globalLimits["user:u1"]
	assert.True(t, stillExists)

	// Disconnect remaining removes global tracking.
	limiter.OnDisconnect("c2")
	_, stillExists = limiter.globalLimits["user:u1"]
	assert.False(t, stillExists)
}

func TestWebSocketRateLimiter_CheckCommand_BurstAndPenalty(t *testing.T) {
	var checkCalls atomic.Int32
	done := make(chan struct{}, 1)

	repo := &stubRateLimitRepo{
		checkAPIRateLimit: func(context.Context, string, string, int, time.Duration) error {
			if checkCalls.Add(1) >= 1 {
				select {
				case done <- struct{}{}:
				default:
				}
			}
			return nil
		},
	}

	config := DefaultWebSocketRateLimitConfig()
	config.BurstWindowSize = 10 * time.Second
	config.ViolationThreshold = 1
	config.PenaltyDuration = 20 * time.Millisecond
	config.CommandLimits = map[string]CommandRateLimit{
		"cmd": {BurstLimit: 1, MaxPerMinute: 1000, MaxPerHour: 0, CostMultiplier: 1},
	}

	limiter := NewWebSocketRateLimiter(config, repo, zap.NewNop())
	limiter.OnConnect("c1", "u1", "1.1.1.1")

	allowed, _, err := limiter.CheckCommand(context.Background(), "c1", &Command{Type: "cmd"})
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, delay, err := limiter.CheckCommand(context.Background(), "c1", &Command{Type: "cmd"})
	require.Error(t, err)
	assert.False(t, allowed)
	assert.Equal(t, config.PenaltyDuration, delay)

	require.Eventually(t, func() bool { return checkCalls.Load() > 0 }, time.Second, 5*time.Millisecond)
	select {
	case <-done:
	default:
	}
}

func TestWebSocketRateLimiter_CheckCommand_RateLimitHourlyAndResetViolations(t *testing.T) {
	repo := &stubRateLimitRepo{}

	config := DefaultWebSocketRateLimitConfig()
	config.EnableProgressiveDelays = false
	config.MaxBurstCommands = 1000
	config.MaxCommandsPerMinute = 1
	config.CommandLimits = map[string]CommandRateLimit{
		"cmd": {BurstLimit: 1000, MaxPerMinute: 1, MaxPerHour: 1, CostMultiplier: 1},
	}

	limiter := NewWebSocketRateLimiter(config, repo, zap.NewNop())
	limiter.OnConnect("c1", "u1", "1.1.1.1")

	allowed, _, err := limiter.CheckCommand(context.Background(), "c1", &Command{Type: "cmd"})
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, delay, err := limiter.CheckCommand(context.Background(), "c1", &Command{Type: "cmd"})
	require.Error(t, err)
	assert.False(t, allowed)
	assert.True(t, delay > 0)

	// Force success after stale lastViolation to cover reset logic.
	limiter.connectionLimits["c1"].mu.Lock()
	limiter.connectionLimits["c1"].violations = 1
	limiter.connectionLimits["c1"].lastViolation = time.Now().Add(-6 * time.Minute)
	limiter.connectionLimits["c1"].commandWindow = NewSlidingWindow(1 * time.Second)
	limiter.connectionLimits["c1"].mu.Unlock()

	config.MaxCommandsPerMinute = 1000
	allowed, _, err = limiter.CheckCommand(context.Background(), "c1", &Command{Type: "other"})
	require.NoError(t, err)
	assert.True(t, allowed)

	limiter.connectionLimits["c1"].mu.RLock()
	assert.Equal(t, 0, limiter.connectionLimits["c1"].violations)
	limiter.connectionLimits["c1"].mu.RUnlock()
}

func TestWebSocketRateLimiter_GetConnectionStatusAndReset(t *testing.T) {
	limiter := NewWebSocketRateLimiter(DefaultWebSocketRateLimitConfig(), &stubRateLimitRepo{}, zap.NewNop())

	_, err := limiter.GetConnectionStatus("missing")
	require.Error(t, err)

	limiter.OnConnect("c1", "u1", "1.1.1.1")
	limiter.connectionLimits["c1"].mu.Lock()
	limiter.connectionLimits["c1"].penaltyUntil = time.Now().Add(50 * time.Millisecond)
	limiter.connectionLimits["c1"].mu.Unlock()

	status, err := limiter.GetConnectionStatus("c1")
	require.NoError(t, err)
	assert.Equal(t, "c1", status["connection_id"])
	assert.Equal(t, true, status["in_penalty"])

	require.NoError(t, limiter.ResetConnection("c1"))
	err = limiter.ResetConnection("missing")
	require.Error(t, err)
}

func TestWebSocketRateLimiter_CalculateProgressiveDelay(t *testing.T) {
	cfg := DefaultWebSocketRateLimitConfig()
	cfg.EnableProgressiveDelays = true
	cfg.BaseDelayMillis = 100
	cfg.MaxDelayMillis = 150

	limiter := NewWebSocketRateLimiter(cfg, &stubRateLimitRepo{}, zap.NewNop())

	assert.Equal(t, time.Duration(0), limiter.calculateProgressiveDelay(0))
	assert.Equal(t, 100*time.Millisecond, limiter.calculateProgressiveDelay(1))
	assert.Equal(t, 150*time.Millisecond, limiter.calculateProgressiveDelay(2))
}
