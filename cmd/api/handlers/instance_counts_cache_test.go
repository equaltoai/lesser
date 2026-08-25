package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInstanceCountsCache_TTLAndValues(t *testing.T) {
	var c instanceCountsCache

	// Empty cache is a miss.
	_, _, _, ok := c.get()
	require.False(t, ok)

	c.set(1, 7, 3)
	users, statuses, domains, ok := c.get()
	require.True(t, ok)
	require.Equal(t, 1, users)
	require.Equal(t, int64(7), statuses)
	require.Equal(t, int64(3), domains)

	// Expiry produces a miss without clobbering the stored values.
	c.expiresAt = time.Now().Add(-time.Second)
	_, _, _, ok = c.get()
	require.False(t, ok)

	// Re-setting refreshes the TTL.
	c.set(2, 8, 4)
	users, statuses, domains, ok = c.get()
	require.True(t, ok)
	require.Equal(t, 2, users)
	require.Equal(t, int64(8), statuses)
	require.Equal(t, int64(4), domains)
}

func TestActiveMonthUsersCache_TTLAndValues(t *testing.T) {
	var c activeMonthUsersCache

	_, ok := c.get()
	require.False(t, ok)

	c.set(42)
	count, ok := c.get()
	require.True(t, ok)
	require.Equal(t, 42, count)

	c.expiresAt = time.Now().Add(-time.Second)
	_, ok = c.get()
	require.False(t, ok)
}
