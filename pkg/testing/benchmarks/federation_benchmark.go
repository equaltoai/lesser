// Package benchmarks provides federation performance benchmarks
package benchmarks

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/testing/factories"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
)

// FederationBenchmarkSuite provides federation benchmarks
type FederationBenchmarkSuite struct {
	activityProcessor *mocks.MockExternalService
	actorFactory     *factories.ActorFactory
	activityFactory  *factories.ActivityFactory
	testActors       []*activitypub.Actor
	testActivities   []*activitypub.Activity
}

// NewFederationBenchmarkSuite creates a new federation benchmark suite
func NewFederationBenchmarkSuite() *FederationBenchmarkSuite {
	return &FederationBenchmarkSuite{
		activityProcessor: mocks.NewMockExternalService(),
		actorFactory:     factories.NewActorFactory("test.example.com"),
		activityFactory:  factories.NewActivityFactory("test.example.com"),
		testActors:       make([]*activitypub.Actor, 0),
		testActivities:   make([]*activitypub.Activity, 0),
	}
}

// Setup prepares test data for federation benchmarks
// Note: b parameter is used for reporting allocation stats during setup
func (s *FederationBenchmarkSuite) Setup(_ *testing.B) { //nolint:revive // b will be used for ReportAllocs in future
	// Create test actors from different domains
	domains := []string{"remote1.example.com", "remote2.example.com", "remote3.example.com"}
	
	for _, domain := range domains {
		factory := factories.NewActorFactory(domain)
		for i := 0; i < 50; i++ {
			actor := factory.CreateActor(factories.ActorOptions{
				Username: fmt.Sprintf("remoteuser%d", i),
			})
			s.testActors = append(s.testActors, actor)
		}
	}
	
	// Create test activities for federation
	for i := 0; i < 1000; i++ {
		actor := s.testActors[i%len(s.testActors)]
		
		activity := s.activityFactory.CreateActivity(factories.ActivityOptions{
			Type:  "Create",
			Actor: actor.ID,
			Object: s.activityFactory.CreateNote(
				fmt.Sprintf("Federation benchmark note %d", i),
				actor.ID,
			),
		})
		
		s.testActivities = append(s.testActivities, activity)
	}
}

// BenchmarkHTTPSignatureVerification benchmarks HTTP signature verification
func (s *FederationBenchmarkSuite) BenchmarkHTTPSignatureVerification(b *testing.B) {
	// Generate test RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatalf("Failed to generate RSA key: %v", err)
	}
	
	// Create test signature headers
	testHeaders := map[string][]string{
		"Host":           {"test.example.com"},
		"Date":           {time.Now().Format(time.RFC1123)},
		"Content-Type":   {"application/activity+json"},
		"Content-Length": {"1234"},
		"Signature":      {"test-signature"},
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		// This would benchmark actual HTTP signature verification
		// For now, we simulate the computational cost
		_, err := rsa.SignPKCS1v15(rand.Reader, privateKey, 0, []byte("test-message"))
		if err != nil {
			b.Fatalf("Failed to sign message: %v", err)
		}
		
		// Simulate verification process
		_ = testHeaders
	}
}

// BenchmarkActivityValidation benchmarks ActivityPub activity validation
func (s *FederationBenchmarkSuite) BenchmarkActivityValidation(b *testing.B) {
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		activity := s.testActivities[i%len(s.testActivities)]
		
		// Benchmark activity validation
		if activity.Type == "" {
			b.Fatalf("Invalid activity type")
		}
		
		if activity.Actor == "" {
			b.Fatalf("Invalid activity actor")
		}
		
		// Simulate more complex validation logic
		_ = validateActivityStructure(activity)
	}
}

// BenchmarkActorResolution benchmarks remote actor resolution
func (s *FederationBenchmarkSuite) BenchmarkActorResolution(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		actor := s.testActors[i%len(s.testActors)]
		
		// Simulate actor resolution process
		resolved := s.resolveActor(ctx, actor.ID)
		if resolved == nil {
			b.Fatalf("Failed to resolve actor")
		}
	}
}

// BenchmarkActivityDelivery benchmarks activity delivery
func (s *FederationBenchmarkSuite) BenchmarkActivityDelivery(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		activity := s.testActivities[i%len(s.testActivities)]
		
		// Simulate activity delivery
		err := s.deliverActivity(ctx, activity, "https://remote.example.com/inbox")
		if err != nil {
			b.Fatalf("Failed to deliver activity: %v", err)
		}
	}
}

// BenchmarkInboxProcessing benchmarks inbox activity processing
func (s *FederationBenchmarkSuite) BenchmarkInboxProcessing(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		activity := s.testActivities[i%len(s.testActivities)]
		
		// Simulate inbox processing
		err := s.processInboxActivity(ctx, activity)
		if err != nil {
			b.Fatalf("Failed to process inbox activity: %v", err)
		}
	}
}

// BenchmarkWebfingerLookup benchmarks WebFinger lookups
func (s *FederationBenchmarkSuite) BenchmarkWebfingerLookup(b *testing.B) {
	ctx := context.Background()
	
	webfingerTargets := []string{
		"acct:user1@remote1.example.com",
		"acct:user2@remote2.example.com", 
		"acct:user3@remote3.example.com",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		target := webfingerTargets[i%len(webfingerTargets)]
		
		// Simulate WebFinger lookup
		result := s.performWebfingerLookup(ctx, target)
		if result == nil {
			b.Fatalf("WebFinger lookup failed")
		}
	}
}

// BenchmarkFederationCaching benchmarks federation data caching
func (s *FederationBenchmarkSuite) BenchmarkFederationCaching(b *testing.B) {
	cache := make(map[string]*activitypub.Actor, 1000)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		actor := s.testActors[i%len(s.testActors)]
		
		// Simulate cache lookup
		if cached, exists := cache[actor.ID]; exists {
			_ = cached
		} else {
			// Cache miss - store actor
			cache[actor.ID] = actor
		}
	}
}

// BenchmarkBatchActivityProcessing benchmarks processing multiple activities
func (s *FederationBenchmarkSuite) BenchmarkBatchActivityProcessing(b *testing.B) {
	ctx := context.Background()
	batchSizes := []int{10, 50, 100, 500}
	
	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("Batch%d", batchSize), func(b *testing.B) {
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				batch := s.testActivities[i*batchSize : (i*batchSize)+batchSize]
				
				// Process batch of activities
				for _, activity := range batch {
					err := s.processInboxActivity(ctx, activity)
					if err != nil {
						b.Fatalf("Failed to process batch activity: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkConcurrentFederation benchmarks concurrent federation operations
func (s *FederationBenchmarkSuite) BenchmarkConcurrentFederation(b *testing.B) {
	ctx := context.Background()
	
	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			activity := s.testActivities[i%len(s.testActivities)]
			
			// Simulate concurrent federation processing
			err := s.processInboxActivity(ctx, activity)
			if err != nil {
				b.Fatalf("Concurrent federation processing failed: %v", err)
			}
			
			i++
		}
	})
}

// BenchmarkRemoteInstanceDiscovery benchmarks discovering remote instances
func (s *FederationBenchmarkSuite) BenchmarkRemoteInstanceDiscovery(b *testing.B) {
	ctx := context.Background()
	instances := []string{
		"remote1.example.com",
		"remote2.example.com",
		"remote3.example.com",
		"remote4.example.com",
		"remote5.example.com",
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		instance := instances[i%len(instances)]
		
		// Simulate instance discovery
		info := s.discoverInstance(ctx, instance)
		if info == nil {
			b.Fatalf("Instance discovery failed for %s", instance)
		}
	}
}

// Helper methods for simulation
func validateActivityStructure(activity *activitypub.Activity) bool {
	// Simulate activity validation logic
	if activity.Type == "" || activity.Actor == "" {
		return false
	}
	
	// Additional validation checks
	time.Sleep(1 * time.Microsecond) // Simulate validation time
	return true
}

// resolveActor simulates actor resolution
// Note: ctx parameter reserved for future context-aware caching and cancellation
func (s *FederationBenchmarkSuite) resolveActor(_ context.Context, actorID string) *activitypub.Actor { //nolint:revive // context will be used for request cancellation
	// Simulate network delay and processing
	time.Sleep(5 * time.Microsecond)
	
	// Find actor in test data
	for _, actor := range s.testActors {
		if actor.ID == actorID {
			return actor
		}
	}
	
	return nil
}

// deliverActivity simulates activity delivery to remote inbox
// Note: ctx parameter reserved for timeout and retry control
func (s *FederationBenchmarkSuite) deliverActivity(_ context.Context, activity *activitypub.Activity, inbox string) error { //nolint:revive // context will be used for HTTP timeouts
	// Simulate HTTP request processing time
	time.Sleep(10 * time.Microsecond)
	
	s.activityProcessor.LogRequest("POST", inbox, activity, map[string]string{
		"Content-Type": "application/activity+json",
	})
	
	return nil
}

// processInboxActivity simulates inbox processing
// Note: ctx parameter reserved for database transaction context
func (s *FederationBenchmarkSuite) processInboxActivity(_ context.Context, activity *activitypub.Activity) error { //nolint:revive // context will be used for transactions
	// Simulate activity processing
	time.Sleep(2 * time.Microsecond)
	
	switch activity.Type {
	case "Create", "Update", "Delete":
		// Object processing
		time.Sleep(1 * time.Microsecond)
	case "Follow", "Accept", "Reject":
		// Relationship processing
		time.Sleep(500 * time.Nanosecond)
	case "Like", "Announce", "Undo":
		// Interaction processing  
		time.Sleep(300 * time.Nanosecond)
	}
	
	return nil
}

// performWebfingerLookup simulates WebFinger discovery
// Note: ctx parameter reserved for HTTP client timeout control
func (s *FederationBenchmarkSuite) performWebfingerLookup(_ context.Context, resource string) map[string]interface{} { //nolint:revive // context will be used for HTTP client
	// Simulate WebFinger lookup delay
	time.Sleep(15 * time.Microsecond)
	
	return map[string]interface{}{
		"subject": resource,
		"links": []interface{}{
			map[string]interface{}{
				"rel":  "self",
				"type": "application/activity+json",
				"href": fmt.Sprintf("https://example.com/users/%s", resource),
			},
		},
	}
}

// discoverInstance simulates remote instance discovery
// Note: ctx parameter reserved for discovery timeout and caching
func (s *FederationBenchmarkSuite) discoverInstance(_ context.Context, domain string) map[string]interface{} { //nolint:revive // context will be used for discovery
	// Simulate instance discovery
	time.Sleep(20 * time.Microsecond)
	
	return map[string]interface{}{
		"domain":     domain,
		"version":    "4.0.0",
		"title":      fmt.Sprintf("Test Instance %s", domain),
		"short_description": "A test instance for benchmarking",
	}
}

// RunAllFederationBenchmarks runs all federation benchmarks
func (s *FederationBenchmarkSuite) RunAllFederationBenchmarks(b *testing.B) {
	s.Setup(b)
	
	benchmarks := []struct {
		name string
		fn   func(*testing.B)
	}{
		{"HTTPSignatureVerification", s.BenchmarkHTTPSignatureVerification},
		{"ActivityValidation", s.BenchmarkActivityValidation},
		{"ActorResolution", s.BenchmarkActorResolution},
		{"ActivityDelivery", s.BenchmarkActivityDelivery},
		{"InboxProcessing", s.BenchmarkInboxProcessing},
		{"WebfingerLookup", s.BenchmarkWebfingerLookup},
		{"FederationCaching", s.BenchmarkFederationCaching},
		{"BatchActivityProcessing", s.BenchmarkBatchActivityProcessing},
		{"ConcurrentFederation", s.BenchmarkConcurrentFederation},
		{"RemoteInstanceDiscovery", s.BenchmarkRemoteInstanceDiscovery},
	}
	
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, benchmark.fn)
	}
}