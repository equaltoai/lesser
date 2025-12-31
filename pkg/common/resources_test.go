package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLambdaResourceMonitor(t *testing.T) {
	monitor := NewLambdaResourceMonitor()
	require.NotNil(t, monitor)
	assert.True(t, monitor.maxMemoryMB > 0)
	assert.True(t, monitor.maxDurationMS > 0)
	assert.False(t, monitor.startTime.IsZero())
}

func TestLambdaResourceMonitor_GetElapsedTime(t *testing.T) {
	monitor := NewLambdaResourceMonitor()

	// Small sleep to ensure elapsed time is measurable
	time.Sleep(1 * time.Millisecond)

	elapsed := monitor.GetElapsedTime()
	assert.True(t, elapsed > 0)
}

func TestLambdaResourceMonitor_GetMemoryUsageMB(t *testing.T) {
	monitor := NewLambdaResourceMonitor()
	memMB := monitor.GetMemoryUsageMB()
	// Memory usage should be non-negative (could be 0 if very little memory used)
	assert.True(t, memMB >= 0)
}

func TestLambdaResourceMonitor_GetCheckpoints(t *testing.T) {
	monitor := NewLambdaResourceMonitor()

	// Initially should have no checkpoints
	checkpoints := monitor.GetCheckpoints()
	assert.Empty(t, checkpoints)

	// CheckResources adds a checkpoint
	_ = monitor.CheckResources("test-operation")

	checkpoints = monitor.GetCheckpoints()
	assert.Len(t, checkpoints, 1)
	assert.Equal(t, "test-operation", checkpoints[0].Description)
}

func TestLambdaResourceMonitor_CheckResources(t *testing.T) {
	monitor := NewLambdaResourceMonitor()

	// Normal operation should succeed
	err := monitor.CheckResources("test-op")
	assert.NoError(t, err)

	// Verify checkpoint was added
	checkpoints := monitor.GetCheckpoints()
	assert.GreaterOrEqual(t, len(checkpoints), 1)
}

func TestLambdaResourceMonitor_GetResourceUtilization(t *testing.T) {
	monitor := NewLambdaResourceMonitor()

	memPercent, timePercent := monitor.GetResourceUtilization()

	// Memory and time percentages should be non-negative
	assert.True(t, memPercent >= 0)
	assert.True(t, timePercent >= 0)

	// Since we just started, time percent should be very low
	assert.True(t, timePercent < 50)
}

func TestLambdaResourceMonitor_WrapWithResourceCheck(t *testing.T) {
	monitor := NewLambdaResourceMonitor()

	t.Run("successful operation", func(t *testing.T) {
		called := false
		err := monitor.WrapWithResourceCheck("test-wrap", func() error {
			called = true
			return nil
		})
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("operation with error", func(t *testing.T) {
		expectedErr := assert.AnError
		err := monitor.WrapWithResourceCheck("test-error", func() error {
			return expectedErr
		})
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestGetLambdaMonitor(t *testing.T) {
	monitor := GetLambdaMonitor()
	require.NotNil(t, monitor)

	// Same monitor returned on subsequent calls
	monitor2 := GetLambdaMonitor()
	assert.Same(t, monitor, monitor2)
}

func TestCheckLambdaResources(t *testing.T) {
	err := CheckLambdaResources("global-test")
	assert.NoError(t, err)
}

func TestWrapWithLambdaResourceCheck(t *testing.T) {
	called := false
	err := WrapWithLambdaResourceCheck("global-wrap", func() error {
		called = true
		return nil
	})
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestResourceCheckpoint(t *testing.T) {
	checkpoint := ResourceCheckpoint{
		Timestamp:   time.Now(),
		MemoryUsed:  1024 * 1024,
		Goroutines:  10,
		Description: "test checkpoint",
	}

	assert.False(t, checkpoint.Timestamp.IsZero())
	assert.Equal(t, uint64(1024*1024), checkpoint.MemoryUsed)
	assert.Equal(t, 10, checkpoint.Goroutines)
	assert.Equal(t, "test checkpoint", checkpoint.Description)
}
