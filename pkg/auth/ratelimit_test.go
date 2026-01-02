package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rateLimitRepoStub struct {
	limited map[string]bool
	until   map[string]time.Time
	counts  map[string]int

	recordErr error
	clearErr  error
	countErr  error
	limitErr  error
}

func (s *rateLimitRepoStub) IsRateLimited(_ context.Context, identifier string) (bool, time.Time, error) {
	if s.limitErr != nil {
		return false, time.Time{}, s.limitErr
	}
	return s.limited[identifier], s.until[identifier], nil
}

func (s *rateLimitRepoStub) RecordLoginAttempt(_ context.Context, identifier string, success bool) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	if !success {
		s.counts[identifier]++
	}
	return nil
}

func (s *rateLimitRepoStub) ClearLoginAttempts(_ context.Context, identifier string) error {
	if s.clearErr != nil {
		return s.clearErr
	}
	s.counts[identifier] = 0
	return nil
}

func (s *rateLimitRepoStub) GetLoginAttemptCount(_ context.Context, identifier string, _ time.Time) (int, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	return s.counts[identifier], nil
}

func TestRateLimiter_CheckRateLimit(t *testing.T) {
	now := time.Now()
	stub := &rateLimitRepoStub{
		limited: map[string]bool{
			RateLimitTypeIP + ":192.0.2.1": true,
		},
		until: map[string]time.Time{
			RateLimitTypeIP + ":192.0.2.1": now.Add(time.Hour),
		},
		counts: map[string]int{},
	}

	rl := &RateLimiter{accountRepo: stub}
	err := rl.CheckRateLimit(context.Background(), "alice", "192.0.2.1")
	require.ErrorIs(t, err, ErrIPRateLimited)

	stub.limited = map[string]bool{
		RateLimitTypeAccount + ":alice": true,
	}
	stub.until = map[string]time.Time{
		RateLimitTypeAccount + ":alice": now.Add(time.Hour),
	}
	err = rl.CheckRateLimit(context.Background(), "alice", "192.0.2.1")
	require.ErrorIs(t, err, ErrAccountLocked)

	stub.limited = map[string]bool{}
	stub.until = map[string]time.Time{}
	require.NoError(t, rl.CheckRateLimit(context.Background(), "alice", "192.0.2.1"))
}

func TestRateLimiter_RecordAttemptAndEnforcementBranches(t *testing.T) {
	stub := &rateLimitRepoStub{
		limited: map[string]bool{},
		until:   map[string]time.Time{},
		counts:  map[string]int{},
	}
	rl := &RateLimiter{accountRepo: stub}

	// Successful attempt clears account attempts.
	stub.counts[RateLimitTypeAccount+":alice"] = 3
	require.NoError(t, rl.RecordAttempt(context.Background(), "alice", "192.0.2.1", true))
	assert.Equal(t, 0, stub.counts[RateLimitTypeAccount+":alice"])

	// Failed attempt hits IP lockout branch.
	stub.counts[RateLimitTypeIP+":192.0.2.1"] = MaxIPAttempts
	err := rl.RecordAttempt(context.Background(), "alice", "192.0.2.1", false)
	require.ErrorIs(t, err, ErrIPRateLimited)

	// Failed attempt hits account lockout branch.
	stub.counts[RateLimitTypeIP+":192.0.2.1"] = 0
	stub.counts[RateLimitTypeAccount+":alice"] = MaxLoginAttempts
	err = rl.RecordAttempt(context.Background(), "alice", "192.0.2.1", false)
	require.ErrorIs(t, err, ErrAccountLocked)

	// Failed attempt with low remaining attempts triggers ErrInvalidCredentials.
	stub.counts[RateLimitTypeAccount+":alice"] = MaxLoginAttempts - 2
	err = rl.RecordAttempt(context.Background(), "alice", "192.0.2.1", false)
	require.ErrorIs(t, err, ErrInvalidCredentials)

	// Error propagation.
	stub.recordErr = errors.New("record failed")
	err = rl.RecordAttempt(context.Background(), "alice", "192.0.2.1", false)
	require.Error(t, err)
}

func TestRateLimiter_GetAccountStatusAndFailedAttempts(t *testing.T) {
	stub := &rateLimitRepoStub{
		limited: map[string]bool{},
		until:   map[string]time.Time{},
		counts: map[string]int{
			RateLimitTypeAccount + ":alice": MaxLoginAttempts + 2,
		},
	}
	rl := &RateLimiter{accountRepo: stub}

	status, err := rl.GetAccountStatus(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, 0, status.RemainingAttempts)

	attempts, err := rl.GetFailedAttempts(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, MaxLoginAttempts+2, attempts)
}
