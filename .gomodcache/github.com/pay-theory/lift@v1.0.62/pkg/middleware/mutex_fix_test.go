package middleware

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMutexCopyingFix(t *testing.T) {
	t.Run("ServiceMesh WithTags creates new mutex instances", func(t *testing.T) {
		// Create original metrics instance
		original := newMockServiceMeshMetrics()

		// Call WithTags which was previously copying the mutex
		derived := original.WithTags(map[string]string{"test": "value"})

		// Verify that derived is different instance (not same pointer)
		assert.NotEqual(t, original, derived)

		// Test concurrent access - this should not deadlock or race
		var wg sync.WaitGroup
		numOperations := 50

		// Test concurrent access to original
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numOperations; i++ {
				counter := original.Counter("concurrent.test")
				counter.Inc()
				time.Sleep(1 * time.Millisecond)
			}
		}()

		// Test concurrent access to derived - this would deadlock if mutex was copied
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < numOperations; i++ {
				counter := derived.Counter("derived.test")
				counter.Inc()
				time.Sleep(1 * time.Millisecond)
			}
		}()

		// This should complete without deadlocks - if it hangs, the fix didn't work
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			t.Log("SUCCESS: No deadlock occurred - mutex copying fix is working")
		case <-time.After(5 * time.Second):
			t.Fatal("DEADLOCK DETECTED: Test timed out, mutex copying fix may not be working")
		}
	})

	t.Run("Multiple WithTags calls are safe", func(t *testing.T) {
		original := newMockServiceMeshMetrics()

		// Create multiple derived instances concurrently
		const numGoroutines = 10
		var wg sync.WaitGroup

		// Each goroutine creates a derived instance and uses it
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				// Create derived instance
				derived := original.WithTags(map[string]string{
					"goroutine_id": string(rune(id)),
					"test":         "concurrent",
				})

				// Use the derived instance
				counter := derived.Counter("race.test")
				counter.Inc()

				// Also use the original concurrently
				originalCounter := original.Counter("original.counter")
				originalCounter.Inc()
			}(i)
		}

		// Wait for completion with timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			t.Log("SUCCESS: All concurrent WithTags calls completed safely")
		case <-time.After(10 * time.Second):
			t.Fatal("TIMEOUT: Concurrent test failed - possible race condition or deadlock")
		}
	})
}
