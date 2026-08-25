package handlers

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInstanceCountsCache_TTLAndValues(t *testing.T) {
	var c instanceCountsCache

	// Empty cache is a miss.
	_, _, _, ok := c.get()
	require.False(t, ok)

	// A successful compute populates the cache.
	users, statuses, domains := c.getOrCompute(func() (int, int64, int64, bool) {
		return 1, 7, 3, true
	})
	require.Equal(t, 1, users)
	require.Equal(t, int64(7), statuses)
	require.Equal(t, int64(3), domains)
	users, statuses, domains, ok = c.get()
	require.True(t, ok)
	require.Equal(t, 1, users)
	require.Equal(t, int64(7), statuses)
	require.Equal(t, int64(3), domains)

	// Expiry produces a miss; the stored values are not clobbered and are
	// served stale by getOrCompute when a recompute fails.
	c.expiresAt = time.Now().Add(-time.Second)
	_, _, _, ok = c.get()
	require.False(t, ok)
	users, statuses, domains = c.getOrCompute(func() (int, int64, int64, bool) {
		return 0, 0, 0, false
	})
	require.Equal(t, 1, users)
	require.Equal(t, int64(7), statuses)
	require.Equal(t, int64(3), domains)

	// A fresh successful compute refreshes the TTL.
	c.expiresAt = time.Now().Add(-time.Second)
	users, statuses, domains = c.getOrCompute(func() (int, int64, int64, bool) {
		return 2, 8, 4, true
	})
	require.Equal(t, 2, users)
	require.Equal(t, int64(8), statuses)
	require.Equal(t, int64(4), domains)
	users, statuses, domains, ok = c.get()
	require.True(t, ok)
	require.Equal(t, 2, users)
	require.Equal(t, int64(8), statuses)
	require.Equal(t, int64(4), domains)
}

// TestInstanceCountsCache_GetOrCompute_SuccessOnly pins F3: a failed compute
// must not be cached — the next getOrCompute within the (unset) TTL recomputes
// instead of serving cached zeros — and a previously cached value is served
// stale when a recompute fails.
func TestInstanceCountsCache_GetOrCompute_SuccessOnly(t *testing.T) {
	var c instanceCountsCache
	computeCalls := 0

	// No prior value: a failing compute returns zeros but is NOT cached, so
	// the next call recomputes.
	users, statuses, domains := c.getOrCompute(func() (int, int64, int64, bool) {
		computeCalls++
		return 0, 0, 0, false
	})
	require.Equal(t, 0, users)
	require.Equal(t, int64(0), statuses)
	require.Equal(t, int64(0), domains)
	require.Equal(t, 1, computeCalls)

	users, statuses, domains = c.getOrCompute(func() (int, int64, int64, bool) {
		computeCalls++
		return 9, 8, 7, true
	})
	require.Equal(t, 9, users)
	require.Equal(t, int64(8), statuses)
	require.Equal(t, int64(7), domains)
	require.Equal(t, 2, computeCalls)

	// Second call within the TTL hits the cache.
	users, statuses, domains = c.getOrCompute(func() (int, int64, int64, bool) {
		computeCalls++
		return -1, -1, -1, true
	})
	require.Equal(t, 9, users)
	require.Equal(t, int64(8), statuses)
	require.Equal(t, int64(7), domains)
	require.Equal(t, 2, computeCalls)

	// Expire the cache: a failing recompute serves the stale value (9/8/7),
	// not zeros, and is not cached.
	c.expiresAt = time.Now().Add(-time.Second)
	users, statuses, domains = c.getOrCompute(func() (int, int64, int64, bool) {
		computeCalls++
		return 0, 0, 0, false
	})
	require.Equal(t, 9, users)
	require.Equal(t, int64(8), statuses)
	require.Equal(t, int64(7), domains)
	require.Equal(t, 3, computeCalls)

	// The failed recompute was not cached: the next call still recomputes.
	users, statuses, domains = c.getOrCompute(func() (int, int64, int64, bool) {
		computeCalls++
		return 4, 5, 6, true
	})
	require.Equal(t, 4, users)
	require.Equal(t, int64(5), statuses)
	require.Equal(t, int64(6), domains)
	require.Equal(t, 4, computeCalls)
}

// TestInstanceCountsCache_GetOrCompute_ComputeUnderLock pins F4: concurrent
// misses collapse to a single compute under the per-process mutex, and the
// losers of the race hit the under-lock fresh re-check instead of recomputing.
func TestInstanceCountsCache_GetOrCompute_ComputeUnderLock(t *testing.T) {
	var c instanceCountsCache

	var mu sync.Mutex
	computeCalls := 0
	computeStarted := make(chan struct{})
	releaseCompute := make(chan struct{})

	compute := func() (int, int64, int64, bool) {
		mu.Lock()
		computeCalls++
		mu.Unlock()
		close(computeStarted)
		<-releaseCompute
		return 11, 12, 13, true
	}

	const goroutines = 8
	results := make(chan [3]any, goroutines)
	ready := make(chan struct{}, goroutines)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			u, s, d := c.getOrCompute(compute)
			results <- [3]any{u, s, d}
		}()
	}

	// Release all goroutines at once so the losers of the lock race queue on
	// the mutex while the winner computes; after the winner caches, each queued
	// goroutine acquires the lock and hits the under-lock fresh re-check.
	for i := 0; i < goroutines; i++ {
		<-ready
	}
	close(start)

	// Wait until the first goroutine is inside compute, then let it finish.
	<-computeStarted
	close(releaseCompute)
	wg.Wait()
	close(results)

	for r := range results {
		require.Equal(t, 11, r[0])
		require.Equal(t, int64(12), r[1])
		require.Equal(t, int64(13), r[2])
	}
	mu.Lock()
	require.Equal(t, 1, computeCalls)
	mu.Unlock()
}

func TestActiveMonthUsersCache_TTLAndValues(t *testing.T) {
	var c activeMonthUsersCache

	_, ok := c.get()
	require.False(t, ok)

	require.Equal(t, 42, c.getOrCompute(func() (int, bool) { return 42, true }))
	count, ok := c.get()
	require.True(t, ok)
	require.Equal(t, 42, count)

	// Expiry produces a miss; the stale value is served when a recompute fails.
	c.expiresAt = time.Now().Add(-time.Second)
	_, ok = c.get()
	require.False(t, ok)
	require.Equal(t, 42, c.getOrCompute(func() (int, bool) { return 1, false }))
}

// TestActiveMonthUsersCache_GetOrCompute_ComputeUnderLock pins F4 for the
// active-month cache: concurrent misses collapse to a single compute and the
// losers hit the under-lock fresh re-check.
func TestActiveMonthUsersCache_GetOrCompute_ComputeUnderLock(t *testing.T) {
	var c activeMonthUsersCache

	var mu sync.Mutex
	computeCalls := 0
	computeStarted := make(chan struct{})
	releaseCompute := make(chan struct{})

	compute := func() (int, bool) {
		mu.Lock()
		computeCalls++
		mu.Unlock()
		close(computeStarted)
		<-releaseCompute
		return 42, true
	}

	const goroutines = 6
	results := make(chan int, goroutines)
	ready := make(chan struct{}, goroutines)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			results <- c.getOrCompute(compute)
		}()
	}
	for i := 0; i < goroutines; i++ {
		<-ready
	}
	close(start)

	<-computeStarted
	close(releaseCompute)
	wg.Wait()
	close(results)

	for r := range results {
		require.Equal(t, 42, r)
	}
	mu.Lock()
	require.Equal(t, 1, computeCalls)
	mu.Unlock()
}

// TestActiveMonthUsersCache_GetOrCompute_SuccessOnly pins the F3 semantics for
// the active-month cache: failures are never cached, stale is served.
func TestActiveMonthUsersCache_GetOrCompute_SuccessOnly(t *testing.T) {
	var c activeMonthUsersCache
	computeCalls := 0

	require.Equal(t, 1, c.getOrCompute(func() (int, bool) {
		computeCalls++
		return 1, false // documented fallback
	}))
	require.Equal(t, 1, computeCalls)

	require.Equal(t, 55, c.getOrCompute(func() (int, bool) {
		computeCalls++
		return 55, true
	}))
	require.Equal(t, 2, computeCalls)

	// Cached within TTL.
	require.Equal(t, 55, c.getOrCompute(func() (int, bool) {
		computeCalls++
		return -1, true
	}))
	require.Equal(t, 2, computeCalls)

	// Expired + failing recompute: stale value served, not cached.
	c.expiresAt = time.Now().Add(-time.Second)
	require.Equal(t, 55, c.getOrCompute(func() (int, bool) {
		computeCalls++
		return 1, false
	}))
	require.Equal(t, 3, computeCalls)

	require.Equal(t, 66, c.getOrCompute(func() (int, bool) {
		computeCalls++
		return 66, true
	}))
	require.Equal(t, 4, computeCalls)
}
