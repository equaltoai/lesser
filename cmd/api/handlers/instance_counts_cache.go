package handlers

import (
	"sync"
	"time"
)

// instanceStatsCacheTTL bounds how long the public instance usage/stats blocks
// stay cached in-memory. Bursts of requests collapse to a single compute; the
// compute itself is already O(1) counter reads, so the cache only guards
// against seed storms and burst amplification on the public surface.
const instanceStatsCacheTTL = 60 * time.Second

// instanceCountsCache caches the /api/v1/instance user/status/domain counts.
// Values are only cached when every underlying read succeeded, so a transient
// repository error is not pinned for the TTL window.
type instanceCountsCache struct {
	mu        sync.RWMutex
	users     int
	statuses  int64
	domains   int64
	expiresAt time.Time
}

func (c *instanceCountsCache) get() (int, int64, int64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.expiresAt.IsZero() || time.Now().After(c.expiresAt) {
		return 0, 0, 0, false
	}
	return c.users, c.statuses, c.domains, true
}

func (c *instanceCountsCache) set(users int, statuses, domains int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.users = users
	c.statuses = statuses
	c.domains = domains
	c.expiresAt = time.Now().Add(instanceStatsCacheTTL)
}

// activeMonthUsersCache caches the /api/v2/instance usage.users.active_month
// count with the same success-only semantics.
type activeMonthUsersCache struct {
	mu        sync.RWMutex
	count     int
	expiresAt time.Time
}

func (c *activeMonthUsersCache) get() (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.expiresAt.IsZero() || time.Now().After(c.expiresAt) {
		return 0, false
	}
	return c.count, true
}

func (c *activeMonthUsersCache) set(count int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count = count
	c.expiresAt = time.Now().Add(instanceStatsCacheTTL)
}
