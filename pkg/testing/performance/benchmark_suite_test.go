package performance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSortDurations(t *testing.T) {
	durations := []time.Duration{3 * time.Millisecond, 1 * time.Millisecond, 2 * time.Millisecond}
	sortDurations(durations)
	require.Equal(t, []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond}, durations)
}

func TestPercentile(t *testing.T) {
	sorted := []time.Duration{1, 2, 3, 4, 5}
	require.Equal(t, time.Duration(3), percentile(sorted, 0.5))
	require.Equal(t, time.Duration(4), percentile(sorted, 0.9))
	require.Equal(t, time.Duration(5), percentile(sorted, 1.0))
}

func TestRateLimiter(t *testing.T) {
	require.Nil(t, NewRateLimiter(0))

	rl := NewRateLimiter(2)
	require.NotNil(t, rl)
	defer rl.Stop()

	// Initial bucket is filled with rps tokens, so these shouldn't block.
	rl.Wait()
	rl.Wait()
}
