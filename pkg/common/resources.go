package common

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// LambdaResourceMonitor tracks resource usage in Lambda environment
type LambdaResourceMonitor struct {
	maxMemoryMB   uint64
	maxDurationMS int
	startTime     time.Time
	mu            sync.Mutex
	checkpoints   []ResourceCheckpoint
}

type ResourceCheckpoint struct {
	Timestamp   time.Time
	MemoryUsed  uint64
	Goroutines  int
	Description string
}

// NewLambdaResourceMonitor creates a monitor based on Lambda limits
func NewLambdaResourceMonitor() *LambdaResourceMonitor {
	// Get Lambda memory limit from environment
	memoryMB := 512 // default
	if envMem := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE"); envMem != "" {
		if parsed, err := strconv.Atoi(envMem); err == nil {
			memoryMB = parsed
		}
	}

	// Lambda timeout is in context, but we'll use 90% as safety margin
	maxDuration := 30000 // 30 seconds default

	return &LambdaResourceMonitor{
		maxMemoryMB:   uint64(float64(memoryMB) * 0.9), // 90% of limit
		maxDurationMS: int(float64(maxDuration) * 0.9),
		startTime:     time.Now(),
	}
}

// CheckResources verifies we're within Lambda limits
func (m *LambdaResourceMonitor) CheckResources(operation string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check duration
	elapsed := time.Since(m.startTime)
	if elapsed.Milliseconds() > int64(m.maxDurationMS) {
		return fmt.Errorf("operation %s approaching Lambda timeout: %v", operation, elapsed)
	}

	// Check memory
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	usedMB := memStats.Alloc / 1024 / 1024
	if usedMB > m.maxMemoryMB {
		// Try garbage collection
		runtime.GC()
		runtime.ReadMemStats(&memStats)
		usedMB = memStats.Alloc / 1024 / 1024

		if usedMB > m.maxMemoryMB {
			return fmt.Errorf("memory limit exceeded for %s: %dMB > %dMB",
				operation, usedMB, m.maxMemoryMB)
		}
	}

	// Record checkpoint
	m.checkpoints = append(m.checkpoints, ResourceCheckpoint{
		Timestamp:   time.Now(),
		MemoryUsed:  memStats.Alloc,
		Goroutines:  runtime.NumGoroutine(),
		Description: operation,
	})

	return nil
}

// WrapWithResourceCheck wraps an operation with resource monitoring
func (m *LambdaResourceMonitor) WrapWithResourceCheck(operation string, fn func() error) error {
	// Check before
	if err := m.CheckResources(fmt.Sprintf("%s-start", operation)); err != nil {
		return err
	}

	// Run operation
	err := fn()

	// Check after
	if checkErr := m.CheckResources(fmt.Sprintf("%s-end", operation)); checkErr != nil {
		if err == nil {
			err = checkErr
		}
	}

	return err
}

// GetCheckpoints returns all recorded checkpoints
func (m *LambdaResourceMonitor) GetCheckpoints() []ResourceCheckpoint {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to prevent external modification
	checkpoints := make([]ResourceCheckpoint, len(m.checkpoints))
	copy(checkpoints, m.checkpoints)
	return checkpoints
}

// GetMemoryUsageMB returns current memory usage in MB
func (m *LambdaResourceMonitor) GetMemoryUsageMB() uint64 {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return memStats.Alloc / 1024 / 1024
}

// GetElapsedTime returns time elapsed since monitor creation
func (m *LambdaResourceMonitor) GetElapsedTime() time.Duration {
	return time.Since(m.startTime)
}

// GetResourceUtilization returns current resource utilization as percentages
func (m *LambdaResourceMonitor) GetResourceUtilization() (memoryPercent float64, timePercent float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	memoryMB := m.GetMemoryUsageMB()
	memoryPercent = float64(memoryMB) / float64(m.maxMemoryMB) * 100

	elapsed := m.GetElapsedTime()
	timePercent = float64(elapsed.Milliseconds()) / float64(m.maxDurationMS) * 100

	return memoryPercent, timePercent
}

// Global monitor for Lambda functions
var lambdaMonitor = NewLambdaResourceMonitor()

// GetLambdaMonitor returns the global Lambda resource monitor
func GetLambdaMonitor() *LambdaResourceMonitor {
	return lambdaMonitor
}

// CheckLambdaResources is a convenience function to check resources using the global monitor
func CheckLambdaResources(operation string) error {
	return lambdaMonitor.CheckResources(operation)
}

// WrapWithLambdaResourceCheck is a convenience function to wrap operations with the global monitor
func WrapWithLambdaResourceCheck(operation string, fn func() error) error {
	return lambdaMonitor.WrapWithResourceCheck(operation, fn)
}
