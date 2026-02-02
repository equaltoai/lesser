package theorydb

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCopyStruct(t *testing.T) {
	type example struct {
		A int
		B string
	}

	src := &example{A: 1, B: "x"}
	dest := &example{}

	copyStruct(src, dest)
	require.Equal(t, *src, *dest)
}

func TestAssertEventualConsistency(t *testing.T) {
	var attempts int32

	AssertEventualConsistency(t, func() (bool, error) {
		if atomic.AddInt32(&attempts, 1) >= 3 {
			return true, nil
		}
		return false, nil
	}, 2*time.Second)

	require.GreaterOrEqual(t, attempts, int32(3))
}

func TestMockCostTracker(t *testing.T) {
	tracker := NewMockCostTracker()
	tracker.TrackOperation("GetItem", 1, 0)
	tracker.TrackOperation("PutItem", 0, 2)

	rcu, wcu := tracker.GetTotalCost()
	require.Equal(t, float64(1), rcu)
	require.Equal(t, float64(2), wcu)
}
