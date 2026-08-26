package handlers

import (
	"sync"
	"time"
)

// instanceStatsCacheTTL bounds how long the public instance usage/stats blocks
// stay cached in-memory. The cache guards the public surface against bursts:
// back-to-back or concurrent requests within the TTL collapse to a single
// compute per process (under a mutex), and a transient repository failure is
// never cached as public zeros. The compute itself is already O(1) counter
// reads, so the cache mainly bounds seed storms and burst amplification.
//
// Honest scope: "single-flight" here is per-process only. Each warm Lambda
// instance has its own in-memory cache, so a burst across instances computes
// once per instance. The cross-instance storm is bounded by the persisted seed
// markers (on success) and the jittered seed backoff (after a failed seed) in
// the repository layer — not by this cache.
const instanceStatsCacheTTL = 60 * time.Second

// instanceCountsCache caches the /api/v1/instance user/status/domain counts.
// Values are cached only when every underlying read succeeded; on any failure
// the previous value is served when one exists (stale fallback) and nothing is
// cached, so a transient repository error is never pinned for the TTL window.
type instanceCountsCache struct {
	mu        sync.Mutex
	users     int
	statuses  int64
	domains   int64
	expiresAt time.Time
	hasValue  bool
}

// get returns the cached triple if it is still fresh.
func (c *instanceCountsCache) get() (int, int64, int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasValue || time.Now().After(c.expiresAt) {
		return 0, 0, 0, false
	}
	return c.users, c.statuses, c.domains, true
}

// getOrCompute returns the fresh cached triple when available; otherwise it
// computes once under a per-process mutex (re-checking the cache after
// acquiring the lock, so concurrent misses collapse) and caches only when the
// compute succeeded. When the compute fails, a previously cached value is
// served stale and the failure is never cached.
func (c *instanceCountsCache) getOrCompute(compute func() (int, int64, int64, bool)) (int, int64, int64) {
	if users, statuses, domains, ok := c.get(); ok {
		return users, statuses, domains
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Re-check freshness under the lock (the mutex is not reentrant, so the
	// check is inlined instead of calling get()).
	if c.hasValue && !time.Now().After(c.expiresAt) {
		return c.users, c.statuses, c.domains
	}
	users, statuses, domains, ok := compute()
	if ok {
		c.users, c.statuses, c.domains = users, statuses, domains
		c.expiresAt = time.Now().Add(instanceStatsCacheTTL)
		c.hasValue = true
		return users, statuses, domains
	}
	if c.hasValue {
		return c.users, c.statuses, c.domains
	}
	return users, statuses, domains
}

// activeMonthUsersCache caches the /api/v2/instance usage.users.active_month
// count with the same success-only, compute-under-lock semantics.
type activeMonthUsersCache struct {
	mu        sync.Mutex
	count     int
	expiresAt time.Time
	hasValue  bool
}

func (c *activeMonthUsersCache) get() (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasValue || time.Now().After(c.expiresAt) {
		return 0, false
	}
	return c.count, true
}

func (c *activeMonthUsersCache) getOrCompute(compute func() (int, bool)) int {
	if count, ok := c.get(); ok {
		return count
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Re-check freshness under the lock (the mutex is not reentrant, so the
	// check is inlined instead of calling get()).
	if c.hasValue && !time.Now().After(c.expiresAt) {
		return c.count
	}
	count, ok := compute()
	if ok {
		c.count = count
		c.expiresAt = time.Now().Add(instanceStatsCacheTTL)
		c.hasValue = true
		return count
	}
	if c.hasValue {
		return c.count
	}
	return count
}
