// +build benchmark

package benchmarks

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/testing/factories"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
)

// TestStorageBenchmarks demonstrates how to run storage benchmarks
func TestStorageBenchmarks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark tests in short mode")
	}

	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	
	// Setup benchmark data
	t.Log("Setting up benchmark data...")
	suite.Setup(&testing.B{})
	
	t.Log("Storage benchmark suite ready")
}

// BenchmarkStorageOperations runs all storage benchmarks
func BenchmarkStorageOperations(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	
	suite.Setup(b)
	suite.RunAllBenchmarks(b)
}

// BenchmarkFederationOperations runs all federation benchmarks
func BenchmarkFederationOperations(b *testing.B) {
	suite := NewFederationBenchmarkSuite()
	suite.RunAllFederationBenchmarks(b)
}

// Example individual benchmarks

func BenchmarkActorCreation(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	
	suite.BenchmarkCreateActor(b)
}

func BenchmarkActorRetrieval(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	
	suite.Setup(b)
	suite.BenchmarkGetActor(b)
}

func BenchmarkTimelineRetrieval(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	
	suite.Setup(b)
	suite.BenchmarkGetTimeline(b)
}

func BenchmarkActivityCreation(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	
	suite.Setup(b)
	suite.BenchmarkStoreActivity(b)
}

func BenchmarkHTTPSignatureVerification(b *testing.B) {
	suite := NewFederationBenchmarkSuite()
	suite.BenchmarkHTTPSignatureVerification(b)
}

func BenchmarkActivityValidation(b *testing.B) {
	suite := NewFederationBenchmarkSuite()
	suite.Setup(b)
	suite.BenchmarkActivityValidation(b)
}

// Benchmark with different configurations

func BenchmarkStorageWithLatency(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	
	// Simulate network latency
	storage.SetLatencySimulation(5 * 1000) // 5ms
	
	suite := NewStorageBenchmarkSuite(storage)
	suite.Setup(b)
	
	b.Run("WithLatency", func(b *testing.B) {
		suite.BenchmarkGetActor(b)
	})
}

func BenchmarkStorageWithErrors(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	
	// Simulate 5% error rate
	storage.SetErrorRate(0.05)
	
	suite := NewStorageBenchmarkSuite(storage)
	suite.Setup(b)
	
	b.Run("WithErrors", func(b *testing.B) {
		// This benchmark should handle errors gracefully
		for i := 0; i < b.N; i++ {
			_, _ = storage.GetActor(nil, "testuser")
		}
	})
}

// Memory allocation benchmarks

func BenchmarkMemoryAllocations(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	suite.Setup(b)
	
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		// Test memory allocations for common operations
		actor := suite.actorFactory.CreateActor(factories.ActorOptions{
			Username: "memtestuser",
		})
		
		_ = storage.CreateActor(nil, actor, "test-key")
		_, _ = storage.GetActor(nil, actor.PreferredUsername)
	}
}