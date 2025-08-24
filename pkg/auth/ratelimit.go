package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Rate limiting errors
var (
	ErrTooManyAttempts = errors.New("too many login attempts")
	ErrAccountLocked   = errors.New("account temporarily locked")
	ErrIPRateLimited   = errors.New("IP address rate limited")
)

// Rate limiting constants
const (
	// Login attempts before lockout
	MaxLoginAttempts = 5
	MaxIPAttempts    = 20

	// Lockout durations
	AccountLockoutDuration = 30 * time.Minute
	IPLockoutDuration      = 1 * time.Hour

	// Time windows for counting attempts
	AttemptWindow = 15 * time.Minute

	// Rate limit keys
	RateLimitTypeAccount = "account"
	RateLimitTypeIP      = "ip"
)

// RateLimiter handles authentication rate limiting
type RateLimiter struct {
	repos StorageProvider
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(repos StorageProvider) *RateLimiter {
	return &RateLimiter{
		repos: repos,
	}
}

// CheckRateLimit checks if an authentication attempt should be allowed
func (rl *RateLimiter) CheckRateLimit(ctx context.Context, username, ipAddress string) error {
	// Check IP rate limit first (broader protection)
	ipLimited, ipUnlockTime, err := rl.repos.Account().IsRateLimited(ctx, rl.ipKey(ipAddress))
	if err != nil {
		return errors.Join(ErrIPRateLimitCheck, err)
	}
	if ipLimited {
		zap.L().Error("IP rate limited",
			zap.String("ip_address", ipAddress),
			zap.String("unlock_time", ipUnlockTime.Format(time.RFC3339)))
		return ErrIPRateLimited
	}

	// Check account rate limit if username provided
	if username != "" {
		accountLimited, accountUnlockTime, err := rl.repos.Account().IsRateLimited(ctx, rl.accountKey(username))
		if err != nil {
			return errors.Join(ErrAccountRateLimitCheck, err)
		}
		if accountLimited {
			zap.L().Error("Account rate limited",
				zap.String("username", username),
				zap.String("unlock_time", accountUnlockTime.Format(time.RFC3339)))
			return ErrAccountLocked
		}
	}

	return nil
}

// RecordAttempt records a login attempt and enforces rate limits
func (rl *RateLimiter) RecordAttempt(ctx context.Context, username, ipAddress string, success bool) error {
	// Always record IP attempt
	if err := rl.repos.Account().RecordLoginAttempt(ctx, rl.ipKey(ipAddress), success); err != nil {
		return errors.Join(ErrRecordIPAttempt, err)
	}

	// Record account attempt if username provided
	if username != "" {
		if err := rl.repos.Account().RecordLoginAttempt(ctx, rl.accountKey(username), success); err != nil {
			return errors.Join(ErrRecordAccountAttempt, err)
		}
	}

	// If successful, clear attempts
	if success {
		if username != "" {
			_ = rl.repos.Account().ClearLoginAttempts(ctx, rl.accountKey(username))
		}
		// Don't clear IP attempts on success - prevents distributed attacks
		return nil
	}

	// Check if we need to impose rate limits
	return rl.enforceRateLimits(ctx, username, ipAddress)
}

// enforceRateLimits checks attempt counts and imposes rate limits if needed
func (rl *RateLimiter) enforceRateLimits(ctx context.Context, username, ipAddress string) error {
	// Check IP attempts
	ipAttempts, err := rl.repos.Account().GetLoginAttemptCount(ctx, rl.ipKey(ipAddress), time.Now().Add(-AttemptWindow))
	if err != nil {
		return errors.Join(ErrGetIPAttemptCount, err)
	}

	if ipAttempts >= MaxIPAttempts {
		// Impose IP rate limit
		if err := rl.imposeLockout(ctx, rl.ipKey(ipAddress), IPLockoutDuration); err != nil {
			return errors.Join(ErrImposeIPLockout, err)
		}
		return ErrIPRateLimited
	}

	// Check account attempts
	if username != "" {
		accountAttempts, err := rl.repos.Account().GetLoginAttemptCount(ctx, rl.accountKey(username), time.Now().Add(-AttemptWindow))
		if err != nil {
			return errors.Join(ErrGetAccountAttemptCount, err)
		}

		if accountAttempts >= MaxLoginAttempts {
			// Impose account lockout
			if err := rl.imposeLockout(ctx, rl.accountKey(username), AccountLockoutDuration); err != nil {
				return errors.Join(ErrImposeAccountLockout, err)
			}
			return ErrAccountLocked
		}

		// Return remaining attempts info
		remainingAttempts := MaxLoginAttempts - accountAttempts
		if remainingAttempts <= 2 {
			zap.L().Warn("Low remaining login attempts",
				zap.String("username", username),
				zap.Int("remaining_attempts", remainingAttempts))
			return ErrInvalidCredentials
		}
	}

	return ErrInvalidCredentials
}

// imposeLockout creates a rate limit entry
func (rl *RateLimiter) imposeLockout(_ context.Context, _ string, _ time.Duration) error {
	// This would typically create a rate limit entry in storage
	// The storage layer would handle the expiration
	// For now, we'll rely on the attempt count mechanism
	return nil
}

// ClearAccountLockout clears rate limiting for a specific account (admin action)
func (rl *RateLimiter) ClearAccountLockout(ctx context.Context, username string) error {
	return rl.repos.Account().ClearLoginAttempts(ctx, rl.accountKey(username))
}

// GetAccountStatus returns the current rate limit status for an account
func (rl *RateLimiter) GetAccountStatus(ctx context.Context, username string) (*RateLimitStatus, error) {
	// Check if currently rate limited
	limited, unlockTime, err := rl.repos.Account().IsRateLimited(ctx, rl.accountKey(username))
	if err != nil {
		return nil, err
	}

	// Get recent attempt count
	attempts, err := rl.repos.Account().GetLoginAttemptCount(ctx, rl.accountKey(username), time.Now().Add(-AttemptWindow))
	if err != nil {
		return nil, err
	}

	status := &RateLimitStatus{
		IsLocked:          limited,
		UnlockTime:        unlockTime,
		RecentAttempts:    attempts,
		MaxAttempts:       MaxLoginAttempts,
		TimeWindow:        AttemptWindow,
		RemainingAttempts: MaxLoginAttempts - attempts,
	}

	if status.RemainingAttempts < 0 {
		status.RemainingAttempts = 0
	}

	return status, nil
}

// GetFailedAttempts returns the number of failed login attempts for a user
func (rl *RateLimiter) GetFailedAttempts(ctx context.Context, username string) (int, error) {
	// Get recent attempt count within the time window
	attempts, err := rl.repos.Account().GetLoginAttemptCount(ctx, rl.accountKey(username), time.Now().Add(-AttemptWindow))
	if err != nil {
		return 0, err
	}
	return attempts, nil
}

// Helper methods

func (rl *RateLimiter) accountKey(username string) string {
	return fmt.Sprintf("%s:%s", RateLimitTypeAccount, username)
}

func (rl *RateLimiter) ipKey(ipAddress string) string {
	return fmt.Sprintf("%s:%s", RateLimitTypeIP, ipAddress)
}

// RateLimitStatus represents the current rate limit status
type RateLimitStatus struct {
	IsLocked          bool          `json:"is_locked"`
	UnlockTime        time.Time     `json:"unlock_time,omitempty"`
	RecentAttempts    int           `json:"recent_attempts"`
	MaxAttempts       int           `json:"max_attempts"`
	TimeWindow        time.Duration `json:"time_window"`
	RemainingAttempts int           `json:"remaining_attempts"`
}
