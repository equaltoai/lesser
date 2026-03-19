package graph

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGraphLeaseRemainingAbsoluteTTLHours(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 19, 12, 0, 0, 0, time.UTC)

	t.Run("zero absolute expiry uses default lease ttl", func(t *testing.T) {
		require.Equal(t, graphAgentAccessLeaseDefaultAbsHrs, graphLeaseRemainingAbsoluteTTLHours(now, time.Time{}))
	})

	t.Run("future absolute expiry uses remaining whole hours", func(t *testing.T) {
		absoluteExpiry := now.Add(72*time.Hour + 30*time.Minute)
		require.Equal(t, 72, graphLeaseRemainingAbsoluteTTLHours(now, absoluteExpiry))
	})

	t.Run("expired absolute expiry clamps to one hour", func(t *testing.T) {
		absoluteExpiry := now.Add(-15 * time.Minute)
		require.Equal(t, 1, graphLeaseRemainingAbsoluteTTLHours(now, absoluteExpiry))
	})
}
