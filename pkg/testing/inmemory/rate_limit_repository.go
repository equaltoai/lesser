// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// ErrRateLimited is returned when a rate limit is exceeded
var ErrRateLimited = errors.New("rate limit exceeded")

// RateLimitRepository is a thread-safe in-memory implementation of interfaces.RateLimitRepository.
type RateLimitRepository struct {
	mu sync.RWMutex
	// loginAttempts stores login attempts keyed by identifier
	loginAttempts map[string][]*loginAttempt
	// apiRateLimits stores API rate limit counters keyed by "userID:endpoint"
	apiRateLimits map[string]*rateLimitCounter
	// federationRateLimits stores federation rate limit counters keyed by "domain:endpoint"
	federationRateLimits map[string]*rateLimitCounter
	// blockedUsers stores blocked users with unblock time
	blockedUsers map[string]time.Time
	// blockedDomains stores blocked domains with unblock time
	blockedDomains map[string]time.Time
	// communityNoteCreations stores daily community note creation counts
	communityNoteCreations map[string]int
}

type loginAttempt struct {
	timestamp time.Time
	success   bool
}

type rateLimitCounter struct {
	count      int
	windowEnd  time.Time
	violations int
}

// NewRateLimitRepository creates a new in-memory rate limit repository
func NewRateLimitRepository() *RateLimitRepository {
	return &RateLimitRepository{
		loginAttempts:          make(map[string][]*loginAttempt),
		apiRateLimits:          make(map[string]*rateLimitCounter),
		federationRateLimits:   make(map[string]*rateLimitCounter),
		blockedUsers:           make(map[string]time.Time),
		blockedDomains:         make(map[string]time.Time),
		communityNoteCreations: make(map[string]int),
	}
}

// RecordLoginAttempt records a login attempt for rate limiting
func (r *RateLimitRepository) RecordLoginAttempt(_ context.Context, identifier string, success bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.loginAttempts[identifier] = append(r.loginAttempts[identifier], &loginAttempt{
		timestamp: time.Now(),
		success:   success,
	})
	return nil
}

// GetLoginAttemptCount returns the number of login attempts since the given time
func (r *RateLimitRepository) GetLoginAttemptCount(_ context.Context, identifier string, since time.Time) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	attempts := r.loginAttempts[identifier]
	count := 0
	for _, attempt := range attempts {
		if attempt.timestamp.After(since) && !attempt.success {
			count++
		}
	}
	return count, nil
}

// IsRateLimited checks if an identifier is currently rate limited
func (r *RateLimitRepository) IsRateLimited(_ context.Context, identifier string) (bool, time.Time, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if blocked
	if unblockTime, blocked := r.blockedUsers[identifier]; blocked {
		if time.Now().Before(unblockTime) {
			return true, unblockTime, nil
		}
	}
	return false, time.Time{}, nil
}

// ClearLoginAttempts clears all login attempts for an identifier
func (r *RateLimitRepository) ClearLoginAttempts(_ context.Context, identifier string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.loginAttempts, identifier)
	return nil
}

// CheckAPIRateLimit checks and updates API rate limiting
func (r *RateLimitRepository) CheckAPIRateLimit(_ context.Context, userID, endpoint string, limit int, window time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := userID + ":" + endpoint
	now := time.Now()

	counter, exists := r.apiRateLimits[key]
	if !exists || now.After(counter.windowEnd) {
		r.apiRateLimits[key] = &rateLimitCounter{
			count:     1,
			windowEnd: now.Add(window),
		}
		return nil
	}

	if counter.count >= limit {
		return ErrRateLimited
	}
	counter.count++
	return nil
}

// GetAPIRateLimitInfo returns current rate limit info
func (r *RateLimitRepository) GetAPIRateLimitInfo(_ context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := userID + ":" + endpoint
	now := time.Now()

	counter, exists := r.apiRateLimits[key]
	if !exists || now.After(counter.windowEnd) {
		return limit, now.Add(window), nil
	}

	return limit - counter.count, counter.windowEnd, nil
}

// CheckFederationRateLimit checks and updates federation rate limiting
func (r *RateLimitRepository) CheckFederationRateLimit(_ context.Context, domain, endpoint string, limit int, window time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := domain + ":" + endpoint
	now := time.Now()

	counter, exists := r.federationRateLimits[key]
	if !exists || now.After(counter.windowEnd) {
		r.federationRateLimits[key] = &rateLimitCounter{
			count:     1,
			windowEnd: now.Add(window),
		}
		return nil
	}

	if counter.count >= limit {
		return ErrRateLimited
	}
	counter.count++
	return nil
}

// GetFederationRateLimitInfo returns current federation rate limit info
func (r *RateLimitRepository) GetFederationRateLimitInfo(_ context.Context, domain, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := domain + ":" + endpoint
	now := time.Now()

	counter, exists := r.federationRateLimits[key]
	if !exists || now.After(counter.windowEnd) {
		return limit, now.Add(window), nil
	}

	return limit - counter.count, counter.windowEnd, nil
}

// GetViolationCount returns the number of violations in a time period
func (r *RateLimitRepository) GetViolationCount(_ context.Context, userID, domain string, _ time.Duration) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := userID + ":" + domain
	counter, exists := r.apiRateLimits[key]
	if !exists {
		return 0, nil
	}
	return counter.violations, nil
}

// IsUserBlocked checks if a user is currently blocked
func (r *RateLimitRepository) IsUserBlocked(_ context.Context, userID string) (bool, time.Time, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	unblockTime, blocked := r.blockedUsers[userID]
	if blocked && time.Now().Before(unblockTime) {
		return true, unblockTime, nil
	}
	return false, time.Time{}, nil
}

// IsDomainBlocked checks if a federation domain is currently blocked
func (r *RateLimitRepository) IsDomainBlocked(_ context.Context, domain string) (bool, time.Time, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	unblockTime, blocked := r.blockedDomains[domain]
	if blocked && time.Now().Before(unblockTime) {
		return true, unblockTime, nil
	}
	return false, time.Time{}, nil
}

// CheckCommunityNoteRateLimit checks if a user can create more community notes today
func (r *RateLimitRepository) CheckCommunityNoteRateLimit(_ context.Context, userID string, limit int) (bool, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := userID + ":" + time.Now().Format("2006-01-02")
	count := r.communityNoteCreations[key]

	if count >= limit {
		return false, 0, nil
	}

	r.communityNoteCreations[key] = count + 1
	return true, limit - count - 1, nil
}

// BlockUser blocks a user until the specified time (test helper)
func (r *RateLimitRepository) BlockUser(userID string, until time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockedUsers[userID] = until
}

// BlockDomain blocks a domain until the specified time (test helper)
func (r *RateLimitRepository) BlockDomain(domain string, until time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blockedDomains[domain] = until
}

// Clear clears all data (test helper)
func (r *RateLimitRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loginAttempts = make(map[string][]*loginAttempt)
	r.apiRateLimits = make(map[string]*rateLimitCounter)
	r.federationRateLimits = make(map[string]*rateLimitCounter)
	r.blockedUsers = make(map[string]time.Time)
	r.blockedDomains = make(map[string]time.Time)
	r.communityNoteCreations = make(map[string]int)
}

// Ensure RateLimitRepository implements interfaces.RateLimitRepository
var _ interfaces.RateLimitRepository = (*RateLimitRepository)(nil)
