package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFormatSortableTimestampPreservesNanosecondOrder(t *testing.T) {
	base := time.Date(2026, 8, 8, 1, 0, 0, 900_000_000, time.UTC)
	later := base.Add(10 * time.Millisecond)

	require.Less(t, formatSortableTimestamp(base), formatSortableTimestamp(later))
	require.Equal(t, "2026-08-08T01:00:00.900000000Z", formatSortableTimestamp(base))
	require.Equal(t, "2026-08-08T01:00:00.910000000Z", formatSortableTimestamp(later))
}
