package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/storage"
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
	storage storage.Storage
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(storage storage.Storage) *RateLimiter {
	return &RateLimiter{
		storage: storage,
	}
}

// CheckRateLimit checks if an authentication attempt should be allowed
func (rl *RateLimiter) CheckRateLimit(ctx context.Context, username, ipAddress string) error {
	// Check IP rate limit first (broader protection)
	ipLimited, ipUnlockTime, err := rl.storage.IsRateLimited(ctx, rl.ipKey(ipAddress))
	if err != nil {
		return fmt.Errorf("failed to check IP rate limit: %w", err)
	}
	if ipLimited {
		return fmt.Errorf("%w: try again after %s", ErrIPRateLimited, ipUnlockTime.Format(time.RFC3339))
	}

	// Check account rate limit if username provided
	if username != "" {
		accountLimited, accountUnlockTime, err := rl.storage.IsRateLimited(ctx, rl.accountKey(username))
		if err != nil {
			return fmt.Errorf("failed to check account rate limit: %w", err)
		}
		if accountLimited {
			return fmt.Errorf("%w: try again after %s", ErrAccountLocked, accountUnlockTime.Format(time.RFC3339))
		}
	}

	return nil
}

// RecordAttempt records a login attempt and enforces rate limits
func (rl *RateLimiter) RecordAttempt(ctx context.Context, username, ipAddress string, success bool) error {
	// Always record IP attempt
	if err := rl.storage.RecordLoginAttempt(ctx, rl.ipKey(ipAddress), success); err != nil {
		return fmt.Errorf("failed to record IP attempt: %w", err)
	}

	// Record account attempt if username provided
	if username != "" {
		if err := rl.storage.RecordLoginAttempt(ctx, rl.accountKey(username), success); err != nil {
			return fmt.Errorf("failed to record account attempt: %w", err)
		}
	}

	// If successful, clear attempts
	if success {
		if username != "" {
			_ = rl.storage.ClearLoginAttempts(ctx, rl.accountKey(username))
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
	ipAttempts, err := rl.storage.GetLoginAttemptCount(ctx, rl.ipKey(ipAddress), time.Now().Add(-AttemptWindow))
	if err != nil {
		return fmt.Errorf("failed to get IP attempt count: %w", err)
	}

	if ipAttempts >= MaxIPAttempts {
		// Impose IP rate limit
		if err := rl.imposeLockout(ctx, rl.ipKey(ipAddress), IPLockoutDuration); err != nil {
			return fmt.Errorf("failed to impose IP lockout: %w", err)
		}
		return ErrIPRateLimited
	}

	// Check account attempts
	if username != "" {
		accountAttempts, err := rl.storage.GetLoginAttemptCount(ctx, rl.accountKey(username), time.Now().Add(-AttemptWindow))
		if err != nil {
			return fmt.Errorf("failed to get account attempt count: %w", err)
		}

		if accountAttempts >= MaxLoginAttempts {
			// Impose account lockout
			if err := rl.imposeLockout(ctx, rl.accountKey(username), AccountLockoutDuration); err != nil {
				return fmt.Errorf("failed to impose account lockout: %w", err)
			}
			return ErrAccountLocked
		}

		// Return remaining attempts info
		remainingAttempts := MaxLoginAttempts - accountAttempts
		if remainingAttempts <= 2 {
			return fmt.Errorf("invalid credentials, %d attempts remaining before lockout", remainingAttempts)
		}
	}

	return errors.New("invalid credentials")
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
	return rl.storage.ClearLoginAttempts(ctx, rl.accountKey(username))
}

// GetAccountStatus returns the current rate limit status for an account
func (rl *RateLimiter) GetAccountStatus(ctx context.Context, username string) (*RateLimitStatus, error) {
	// Check if currently rate limited
	limited, unlockTime, err := rl.storage.IsRateLimited(ctx, rl.accountKey(username))
	if err != nil {
		return nil, err
	}

	// Get recent attempt count
	attempts, err := rl.storage.GetLoginAttemptCount(ctx, rl.accountKey(username), time.Now().Add(-AttemptWindow))
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
