// Package benchmarks provides performance benchmarks for critical paths
package benchmarks

import (
	"context"
	"fmt"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/testing/factories"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
)

// StorageBenchmarkSuite provides storage operation benchmarks
type StorageBenchmarkSuite struct {
	storage         *mocks.EnhancedMockStorage
	actorFactory    *factories.ActorFactory
	activityFactory *factories.ActivityFactory
	testActors      []*activitypub.Actor
	testActivities  []*activitypub.Activity
}

// NewStorageBenchmarkSuite creates a new storage benchmark suite
func NewStorageBenchmarkSuite(storage *mocks.EnhancedMockStorage) *StorageBenchmarkSuite {
	return &StorageBenchmarkSuite{
		storage:         storage,
		actorFactory:    factories.NewActorFactory("test.example.com"),
		activityFactory: factories.NewActivityFactory("test.example.com"),
		testActors:      make([]*activitypub.Actor, 0),
		testActivities:  make([]*activitypub.Activity, 0),
	}
}

// Setup prepares test data for benchmarks
func (s *StorageBenchmarkSuite) Setup(b *testing.B) {
	ctx := context.Background()
	
	// Create test actors
	for i := 0; i < 1000; i++ {
		actor := s.actorFactory.CreateActor(factories.ActorOptions{
			Username: fmt.Sprintf("benchuser%d", i),
		})
		
		err := s.storage.CreateActor(ctx, actor, "test-key")
		if err != nil {
			b.Fatalf("Failed to create test actor: %v", err)
		}
		
		s.testActors = append(s.testActors, actor)
	}
	
	// Create test activities
	for i := 0; i < 10000; i++ {
		actor := s.testActors[i%len(s.testActors)]
		activity := s.activityFactory.CreateActivity(factories.ActivityOptions{
			Type:  "Create",
			Actor: actor.ID,
			Object: s.activityFactory.CreateNote(
				fmt.Sprintf("Benchmark test note %d", i),
				actor.ID,
			),
		})
		
		err := s.storage.StoreActivity(ctx, activity)
		if err != nil {
			b.Fatalf("Failed to store test activity: %v", err)
		}
		
		s.testActivities = append(s.testActivities, activity)
	}
}

// BenchmarkCreateActor benchmarks actor creation
func (s *StorageBenchmarkSuite) BenchmarkCreateActor(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		actor := s.actorFactory.CreateActor(factories.ActorOptions{
			Username: fmt.Sprintf("newbenchuser%d", i),
		})
		
		err := s.storage.CreateActor(ctx, actor, "test-key")
		if err != nil {
			b.Fatalf("Failed to create actor: %v", err)
		}
	}
}

// BenchmarkGetActor benchmarks actor retrieval
func (s *StorageBenchmarkSuite) BenchmarkGetActor(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		actor := s.testActors[i%len(s.testActors)]
		_, err := s.storage.GetActor(ctx, actor.PreferredUsername)
		if err != nil {
			b.Fatalf("Failed to get actor: %v", err)
		}
	}
}

// BenchmarkStoreActivity benchmarks activity storage
func (s *StorageBenchmarkSuite) BenchmarkStoreActivity(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		actor := s.testActors[i%len(s.testActors)]
		activity := s.activityFactory.CreateActivity(factories.ActivityOptions{
			Type:  "Create",
			Actor: actor.ID,
			Object: s.activityFactory.CreateNote(
				fmt.Sprintf("Benchmark note %d", i),
				actor.ID,
			),
		})
		
		err := s.storage.StoreActivity(ctx, activity)
		if err != nil {
			b.Fatalf("Failed to store activity: %v", err)
		}
	}
}

// BenchmarkGetActivity benchmarks activity retrieval  
func (s *StorageBenchmarkSuite) BenchmarkGetActivity(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		activity := s.testActivities[i%len(s.testActivities)]
		_, err := s.storage.GetActivity(ctx, activity.ID)
		if err != nil {
			b.Fatalf("Failed to get activity: %v", err)
		}
	}
}

// BenchmarkGetTimeline benchmarks timeline retrieval
func (s *StorageBenchmarkSuite) BenchmarkGetTimeline(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		actor := s.testActors[i%len(s.testActors)]
		_, err := s.storage.GetTimeline(ctx, actor.PreferredUsername, 20, "")
		if err != nil {
			b.Fatalf("Failed to get timeline: %v", err)
		}
	}
}

// BenchmarkFollowActor benchmarks following relationships
func (s *StorageBenchmarkSuite) BenchmarkFollowActor(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		follower := s.testActors[i%len(s.testActors)]
		target := s.testActors[(i+1)%len(s.testActors)]
		
		err := s.storage.FollowActor(ctx, follower.PreferredUsername, target.PreferredUsername)
		if err != nil {
			b.Fatalf("Failed to follow actor: %v", err)
		}
	}
}

// RunAllBenchmarks runs all storage benchmarks
func (s *StorageBenchmarkSuite) RunAllBenchmarks(b *testing.B) {
	benchmarks := []struct {
		name string
		fn   func(*testing.B)
	}{
		{"CreateActor", s.BenchmarkCreateActor},
		{"GetActor", s.BenchmarkGetActor},
		{"StoreActivity", s.BenchmarkStoreActivity},
		{"GetActivity", s.BenchmarkGetActivity},
		{"GetTimeline", s.BenchmarkGetTimeline},
		{"FollowActor", s.BenchmarkFollowActor},
	}
	
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, benchmark.fn)
	}
}

// BenchmarkStorageMemoryUsage benchmarks memory allocation patterns in storage operations
func BenchmarkStorageMemoryUsage(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	
	b.ReportAllocs()
	suite.Setup(b)
	suite.RunAllBenchmarks(b)
}

// BenchmarkConcurrentActorAccess benchmarks concurrent read/write access to actor data
func BenchmarkConcurrentActorAccess(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	suite.Setup(b)
	
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			actor := suite.testActors[i%len(suite.testActors)]
			_, err := storage.GetActor(ctx, actor.PreferredUsername)
			if err != nil {
				b.Fatalf("Failed to get actor: %v", err)
			}
			i++
		}
	})
}

// BenchmarkBatchActorCreation benchmarks bulk actor creation performance
func BenchmarkBatchActorCreation(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	factory := factories.NewActorFactory("test.example.com")
	ctx := context.Background()
	
	batchSizes := []int{10, 50, 100, 500, 1000}
	
	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("Batch%d", batchSize), func(b *testing.B) {
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				actors := factory.CreateActorBatch(batchSize, fmt.Sprintf("batch%d_", i))
				
				for _, actor := range actors {
					err := storage.CreateActor(ctx, actor, "test-key")
					if err != nil {
						b.Fatalf("Failed to create actor in batch: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkTimelinePerformance benchmarks timeline query performance under various load conditions
func BenchmarkTimelinePerformance(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	timelineFactory := factories.NewTimelineFactory("test.example.com")
	ctx := context.Background()
	
	scenarios := []factories.TimelineScenario{
		factories.SimpleTimeline,
		factories.MixedTimeline,
		factories.HighVolumeTimeline,
	}
	
	for _, scenario := range scenarios {
		b.Run(string(scenario), func(b *testing.B) {
			// Setup timeline data
			timelineData := timelineFactory.CreateTimelineScenario("benchuser", scenario)
			
			// Store actors and activities
			for _, actor := range timelineData.Following {
				_ = storage.CreateActor(ctx, actor, "test-key")
			}
			_ = storage.CreateActor(ctx, timelineData.User, "test-key")
			
			for _, activity := range timelineData.Activities {
				_ = storage.StoreActivity(ctx, activity)
			}
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				_, err := storage.GetTimeline(ctx, timelineData.User.PreferredUsername, 20, "")
				if err != nil {
					b.Fatalf("Failed to get timeline: %v", err)
				}
			}
		})
	}
}

// BenchmarkSearchPerformance benchmarks full-text search and filtering operations
func BenchmarkSearchPerformance(b *testing.B) {
	storage := mocks.NewEnhancedMockStorage()
	suite := NewStorageBenchmarkSuite(storage)
	suite.Setup(b)
	
	ctx := context.Background()
	searchTerms := []string{"test", "benchmark", "user", "activity", "note"}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		term := searchTerms[i%len(searchTerms)]
		
		// This would call a search method if it existed in the storage interface
		// For now, we'll benchmark getting actors with similar names
		for _, actor := range suite.testActors[:10] { // Limit search scope
			if len(actor.PreferredUsername) > 0 && actor.PreferredUsername[0:1] == term[0:1] {
				_, _ = storage.GetActor(ctx, actor.PreferredUsername)
			}
		}
	}
}