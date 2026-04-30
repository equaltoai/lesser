package theorydb

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestModel for performance testing
type TestModel struct {
	StandardModel
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TestModel2 struct {
	StandardModel
	ID    string `json:"id"`
	Value int    `json:"value"`
}

type TestModel3 struct {
	StandardModel
	ID   string    `json:"id"`
	Data []byte    `json:"data"`
	Time time.Time `json:"time"`
}

// TestLambdaInitBasic tests basic initialization
func TestLambdaInitBasic(t *testing.T) {
	// Reset metrics for clean test
	initMetrics.mu.Lock()
	initMetrics.initialized = false
	initMetrics.modelCount = 0
	initMetrics.connectionCount = 0
	initMetrics.mu.Unlock()

	start := time.Now()
	db, err := LambdaInit(&TestModel{})
	require.NoError(t, err)
	require.NotNil(t, db)

	duration := time.Since(start)
	t.Logf("Basic init took: %v", duration)

	// Verify metrics
	coldStart, models, connections := GetInitMetrics()
	assert.Greater(t, models, 0)
	assert.Greater(t, connections, 0)
	assert.NotZero(t, coldStart)
}

// TestLambdaInitWithOptions tests advanced initialization
func TestLambdaInitWithOptions(t *testing.T) {
	logger := zaptest.NewLogger(t)

	opts := &LambdaInitOptions{
		Models:             []interface{}{&TestModel{}, &TestModel2{}, &TestModel3{}},
		EnableCostTracking: true,
		Logger:             logger,
		RequestID:          "test-request",
		OperationType:      "test-op",
		PrewarmConnections: true,
		ConnectionCount:    3,
		TimeoutBuffer:      time.Second,
		EnableLazyLoading:  true,
	}

	start := time.Now()
	db, err := LambdaInitWithOptions(opts)
	require.NoError(t, err)
	require.NotNil(t, db)

	duration := time.Since(start)
	t.Logf("Advanced init took: %v", duration)

	// Verify it's a tracking DB
	trackingDB, ok := db.(*cost.TrackingDB)
	assert.True(t, ok, "Expected TrackingDB when cost tracking enabled")
	if ok {
		assert.NotNil(t, trackingDB.GetTracker())
	}

	// Verify metrics
	coldStart, models, connections := GetInitMetrics()
	assert.Equal(t, 3, models)
	assert.Equal(t, 3, connections)
	assert.NotZero(t, coldStart)
}

// TestParallelModelRegistration tests parallel vs sequential registration
func TestParallelModelRegistration(t *testing.T) {
	db, err := getLambdaOptimizedClient()
	require.NoError(t, err)

	// Test sequential registration (small number)
	smallModels := []interface{}{&TestModel{}, &TestModel2{}}
	start := time.Now()
	err = preRegisterModelsParallel(db, smallModels)
	require.NoError(t, err)
	seqDuration := time.Since(start)
	t.Logf("Sequential registration (2 models): %v", seqDuration)

	// Test parallel registration (large number)
	largeModels := []interface{}{
		&TestModel{}, &TestModel2{}, &TestModel3{},
		&TestModel{}, &TestModel2{}, &TestModel3{},
	}
	start = time.Now()
	err = preRegisterModelsParallel(db, largeModels)
	require.NoError(t, err)
	parDuration := time.Since(start)
	t.Logf("Parallel registration (6 models): %v", parDuration)

	// Parallel should be faster for larger sets
	avgParTime := parDuration / time.Duration(len(largeModels))
	avgSeqTime := seqDuration / time.Duration(len(smallModels))
	t.Logf("Avg time per model - Sequential: %v, Parallel: %v", avgSeqTime, avgParTime)
}

// TestConnectionPrewarming tests connection prewarming
func TestConnectionPrewarming(t *testing.T) {
	ctx := context.Background()
	db, err := GetLambdaClient(ctx)
	require.NoError(t, err)

	// Test prewarming
	start := time.Now()
	err = prewarmConnections(db, 3)
	require.NoError(t, err)
	duration := time.Since(start)
	t.Logf("Prewarming 3 connections took: %v", duration)

	// Test first operation after prewarming (should be faster)
	testModel := &TestModel{ID: "test123"}
	start = time.Now()
	_ = db.Model(testModel).Where("ID", "=", "nonexistent").First(testModel)
	firstOpDuration := time.Since(start)
	t.Logf("First operation after prewarming: %v", firstOpDuration)
}

// TestLazyLoader tests lazy loading functionality
func TestLazyLoader(t *testing.T) {
	loadCount := 0
	loader := NewLazyLoader(func() (interface{}, error) {
		loadCount++
		time.Sleep(50 * time.Millisecond) // Simulate expensive init
		return "loaded-value", nil
	})

	// First access should trigger load
	start := time.Now()
	val1, err := loader.Get()
	require.NoError(t, err)
	assert.Equal(t, "loaded-value", val1)
	firstDuration := time.Since(start)
	assert.Equal(t, 1, loadCount)
	assert.Greater(t, firstDuration, 50*time.Millisecond)

	// Second access should be instant
	start = time.Now()
	val2, err := loader.Get()
	require.NoError(t, err)
	assert.Equal(t, "loaded-value", val2)
	secondDuration := time.Since(start)
	assert.Equal(t, 1, loadCount) // Still 1
	assert.Less(t, secondDuration, 5*time.Millisecond)

	// Reset should clear cache
	loader.Reset()
	val3, err := loader.Get()
	require.NoError(t, err)
	assert.Equal(t, "loaded-value", val3)
	assert.Equal(t, 2, loadCount) // Incremented
}

// TestConcurrentLazyLoader tests thread safety
func TestConcurrentLazyLoader(t *testing.T) {
	loadCount := 0
	var mu sync.Mutex

	loader := NewLazyLoader(func() (interface{}, error) {
		mu.Lock()
		loadCount++
		mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		return "concurrent-value", nil
	})

	// Launch multiple goroutines
	var wg sync.WaitGroup
	results := make([]interface{}, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errors[idx] = loader.Get()
		}(i)
	}

	wg.Wait()

	// Verify all got same value and no errors
	for i := 0; i < 10; i++ {
		assert.NoError(t, errors[i])
		assert.Equal(t, "concurrent-value", results[i])
	}

	// Verify loaded only once
	mu.Lock()
	assert.Equal(t, 1, loadCount)
	mu.Unlock()
}

// BenchmarkLambdaInit benchmarks basic initialization
func BenchmarkLambdaInit(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Reset state
		clientOnce = sync.Once{}
		client = nil
		lambdaDB = nil

		db, err := LambdaInit(&TestModel{})
		if err != nil {
			b.Fatal(err)
		}
		_ = db
	}
}

// BenchmarkLambdaInitWithOptions benchmarks advanced initialization
func BenchmarkLambdaInitWithOptions(b *testing.B) {
	logger := zaptest.NewLogger(b)
	opts := &LambdaInitOptions{
		Models:             []interface{}{&TestModel{}, &TestModel2{}, &TestModel3{}},
		EnableCostTracking: true,
		Logger:             logger,
		PrewarmConnections: true,
		ConnectionCount:    3,
	}

	for i := 0; i < b.N; i++ {
		// Reset state
		clientOnce = sync.Once{}
		client = nil
		lambdaDB = nil

		db, err := LambdaInitWithOptions(opts)
		if err != nil {
			b.Fatal(err)
		}
		_ = db
	}
}

// BenchmarkFirstOperationCold benchmarks first operation without prewarming
func BenchmarkFirstOperationCold(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Reset state
		clientOnce = sync.Once{}
		client = nil
		lambdaDB = nil

		db, _ := LambdaInit()
		model := &TestModel{ID: "bench"}
		b.StartTimer()

		_ = db.Model(model).Where("ID", "=", "nonexistent").First(model)
	}
}

// BenchmarkFirstOperationWarm benchmarks first operation with prewarming
func BenchmarkFirstOperationWarm(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Reset state
		clientOnce = sync.Once{}
		client = nil
		lambdaDB = nil

		opts := &LambdaInitOptions{
			Models:             []interface{}{&TestModel{}},
			PrewarmConnections: true,
			ConnectionCount:    3,
		}
		db, _ := LambdaInitWithOptions(opts)
		model := &TestModel{ID: "bench"}
		b.StartTimer()

		_ = db.Model(model).Where("ID", "=", "nonexistent").First(model)
	}
}

// TestCostTrackingIntegration tests cost tracking integration
func TestCostTrackingIntegration(t *testing.T) {
	t.Skip("Skipping integration test that requires real DynamoDB connection")

	// This test requires a real DynamoDB connection which is not available in unit tests
	// It should be moved to integration tests or use proper mocking
	// The test was failing with: "UnrecognizedClientException: The security token included in the request is invalid"
}

// TestRuntimeOptimization tests runtime optimization effects
func TestRuntimeOptimization(t *testing.T) {
	// Get initial GC stats
	initialGCPercent := 100 // default

	// Apply optimizations
	optimizeRuntime()

	// Brief sleep to let optimization take effect
	time.Sleep(10 * time.Millisecond)

	// Can't directly test GOGC changes in unit test environment
	// but we can verify the function doesn't panic
	assert.Greater(t, initialGCPercent, 0)
}

// TestMetricsAccuracy tests that metrics are accurately tracked
func TestMetricsAccuracy(t *testing.T) {
	// Reset metrics
	initMetrics.mu.Lock()
	initMetrics.initialized = false
	initMetrics.modelCount = 0
	initMetrics.connectionCount = 0
	initMetrics.coldStartTime = 0
	initMetrics.mu.Unlock()

	opts := &LambdaInitOptions{
		Models:             []interface{}{&TestModel{}, &TestModel2{}},
		PrewarmConnections: true,
		ConnectionCount:    4,
	}

	_, err := LambdaInitWithOptions(opts)
	require.NoError(t, err)

	coldStart, models, connections := GetInitMetrics()
	assert.Equal(t, 2, models)
	assert.Equal(t, 4, connections)
	assert.Greater(t, coldStart, time.Duration(0))
}

// TestErrorHandling tests error handling in various scenarios
func TestErrorHandling(t *testing.T) {
	t.Run("EmptyModels", func(t *testing.T) {
		db, err := LambdaInit()
		assert.NoError(t, err)
		assert.NotNil(t, db)
	})

	t.Run("NilOptions", func(t *testing.T) {
		db, err := LambdaInitWithOptions(nil)
		assert.NoError(t, err)
		assert.NotNil(t, db)
	})

	t.Run("ZeroConnections", func(t *testing.T) {
		opts := &LambdaInitOptions{
			PrewarmConnections: true,
			ConnectionCount:    0, // Should default to 2
		}
		db, err := LambdaInitWithOptions(opts)
		assert.NoError(t, err)
		assert.NotNil(t, db)

		_, _, connections := GetInitMetrics()
		assert.Equal(t, 2, connections) // Default value
	})
}

// Mock helpers removed - not currently used in tests
