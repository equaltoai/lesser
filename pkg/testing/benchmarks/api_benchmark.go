// Package benchmarks provides API performance benchmarks
package benchmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/testing/factories"
	"github.com/equaltoai/lesser/pkg/testing/harness"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
)

// APIBenchmarkSuite provides API endpoint benchmarks
type APIBenchmarkSuite struct {
	server          *httptest.Server
	client          *harness.APIClient
	mastodonClient  *harness.MastodonAPIClient
	storage         *mocks.EnhancedMockStorage
	actorFactory    *factories.ActorFactory
	timelineFactory *factories.TimelineFactory
}

// NewAPIBenchmarkSuite creates a new API benchmark suite
func NewAPIBenchmarkSuite(handler http.Handler) *APIBenchmarkSuite {
	storage := mocks.NewEnhancedMockStorage()
	server := httptest.NewServer(handler)

	return &APIBenchmarkSuite{
		server:          server,
		client:          harness.NewAPIClient(nil, server.URL),
		mastodonClient:  harness.NewMastodonAPIClient(nil, server.URL),
		storage:         storage,
		actorFactory:    factories.NewActorFactory("test.example.com"),
		timelineFactory: factories.NewTimelineFactory("test.example.com"),
	}
}

// Setup prepares test data for API benchmarks
func (s *APIBenchmarkSuite) Setup(b *testing.B) {
	ctx := context.Background()

	// Create test users
	for i := 0; i < 100; i++ {
		actor := s.actorFactory.CreateActor(factories.ActorOptions{
			Username: fmt.Sprintf("apiuser%d", i),
		})

		err := s.storage.CreateActor(ctx, actor, "test-key")
		if err != nil {
			b.Fatalf("Failed to create test actor for API benchmarks: %v", err)
		}
	}

	// Create timeline data for users
	timelineData := s.timelineFactory.CreateTimelineScenario("benchuser", factories.MixedTimeline)

	for _, actor := range timelineData.Following {
		_ = s.storage.CreateActor(ctx, actor, "test-key")
	}
	_ = s.storage.CreateActor(ctx, timelineData.User, "test-key")

	for _, activity := range timelineData.Activities {
		_ = s.storage.StoreActivity(ctx, activity)
	}
}

// Cleanup closes the test server
func (s *APIBenchmarkSuite) Cleanup() {
	if s.server != nil {
		s.server.Close()
	}
}

// BenchmarkHealthCheck benchmarks the health check endpoint
func (s *APIBenchmarkSuite) BenchmarkHealthCheck(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp := s.client.GET("/health")
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("Health check failed with status %d", resp.StatusCode)
		}
	}
}

// BenchmarkGetActor benchmarks actor retrieval endpoint
func (s *APIBenchmarkSuite) BenchmarkGetActor(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		username := fmt.Sprintf("apiuser%d", i%100)
		resp := s.client.GET(fmt.Sprintf("/users/%s", username))

		if resp.StatusCode != http.StatusOK {
			b.Fatalf("Get actor failed with status %d", resp.StatusCode)
		}
	}
}

// BenchmarkVerifyCredentials benchmarks credential verification
func (s *APIBenchmarkSuite) BenchmarkVerifyCredentials(b *testing.B) {
	s.mastodonClient.WithToken("test-token")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp := s.mastodonClient.VerifyCredentials()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
			b.Fatalf("Verify credentials failed with status %d", resp.StatusCode)
		}
	}
}

// BenchmarkGetHomeTimeline benchmarks timeline retrieval
func (s *APIBenchmarkSuite) BenchmarkGetHomeTimeline(b *testing.B) {
	s.mastodonClient.WithToken("test-token")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp := s.mastodonClient.GetHomeTimeline(map[string]string{
			"limit": "20",
		})

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
			b.Fatalf("Get timeline failed with status %d", resp.StatusCode)
		}
	}
}

// BenchmarkCreateStatus benchmarks status creation
func (s *APIBenchmarkSuite) BenchmarkCreateStatus(b *testing.B) {
	s.mastodonClient.WithToken("test-token")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		status := map[string]interface{}{
			"status": fmt.Sprintf("Benchmark test status #%d", i),
		}

		resp := s.mastodonClient.CreateStatus(status)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusUnauthorized {
			b.Fatalf("Create status failed with status %d", resp.StatusCode)
		}
	}
}

// BenchmarkWebfinger benchmarks WebFinger lookup
func (s *APIBenchmarkSuite) BenchmarkWebfinger(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		username := fmt.Sprintf("apiuser%d", i%100)
		resource := fmt.Sprintf("acct:%s@test.example.com", username)

		resp := s.client.GET("/.well-known/webfinger", map[string]string{
			"resource": resource,
		})

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			b.Fatalf("Webfinger failed with status %d", resp.StatusCode)
		}
	}
}

// BenchmarkActivityPubInbox benchmarks ActivityPub inbox
func (s *APIBenchmarkSuite) BenchmarkActivityPubInbox(b *testing.B) {
	activity := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Create",
		"actor":    "https://remote.example.com/users/testuser",
		"object": map[string]interface{}{
			"type":    "Note",
			"content": "Test activity for benchmarking",
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		username := fmt.Sprintf("apiuser%d", i%100)
		resp := s.client.POST(fmt.Sprintf("/users/%s/inbox", username), activity)

		// Accept various status codes as ActivityPub processing can vary
		if resp.StatusCode >= 500 {
			b.Fatalf("Inbox POST failed with status %d", resp.StatusCode)
		}
	}
}

// BenchmarkJSONParsing benchmarks JSON parsing performance
func (s *APIBenchmarkSuite) BenchmarkJSONParsing(b *testing.B) {
	largeStatus := map[string]interface{}{
		"status": "This is a very long status message that contains lots of text to benchmark JSON parsing performance. " +
			"It includes multiple sentences and should be representative of longer posts that users might create. " +
			"The purpose is to measure how well the API handles parsing of larger JSON payloads.",
		"visibility": "public",
		"sensitive":  false,
		"media_ids":  []string{"media1", "media2", "media3"},
		"poll": map[string]interface{}{
			"options":     []string{"Option 1", "Option 2", "Option 3", "Option 4"},
			"expires_in":  86400,
			"multiple":    false,
			"hide_totals": false,
		},
	}

	jsonData, _ := json.Marshal(largeStatus)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", s.server.URL+"/api/v1/statuses", bytes.NewReader(jsonData))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		_ = resp.Body.Close()
	}
}

// BenchmarkConcurrentRequests benchmarks concurrent API access
func (s *APIBenchmarkSuite) BenchmarkConcurrentRequests(b *testing.B) {
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			username := fmt.Sprintf("apiuser%d", i%100)
			resp := s.client.GET(fmt.Sprintf("/users/%s", username))

			if resp.StatusCode >= 500 {
				b.Fatalf("Concurrent request failed with status %d", resp.StatusCode)
			}
			i++
		}
	})
}

// BenchmarkRateLimiting benchmarks rate limiting performance
func (s *APIBenchmarkSuite) BenchmarkRateLimiting(b *testing.B) {
	// This would test the rate limiting middleware performance
	s.mastodonClient.WithToken("test-token")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Make rapid requests to test rate limiting
		for j := 0; j < 10; j++ {
			resp := s.mastodonClient.VerifyCredentials()
			// Rate limiting might return 429, which is expected
			if resp.StatusCode >= 500 {
				b.Fatalf("Rate limiting test failed with status %d", resp.StatusCode)
			}
		}
	}
}

// BenchmarkMemoryUsage benchmarks memory usage during API operations
func (s *APIBenchmarkSuite) BenchmarkMemoryUsage(b *testing.B) {
	b.ReportAllocs()
	s.mastodonClient.WithToken("test-token")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Perform various operations to measure memory usage
		_ = s.mastodonClient.VerifyCredentials()
		_ = s.mastodonClient.GetHomeTimeline(map[string]string{"limit": "20"})
		_ = s.client.GET("/users/apiuser1")
	}
}

// BenchmarkResponseTime benchmarks response time consistency
func (s *APIBenchmarkSuite) BenchmarkResponseTime(b *testing.B) {
	times := make([]time.Duration, b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		resp := s.client.GET("/users/apiuser1")
		times[i] = time.Since(start)

		if resp.StatusCode >= 500 {
			b.Fatalf("Response time test failed with status %d", resp.StatusCode)
		}
	}

	// Calculate statistics
	var total time.Duration
	for _, t := range times {
		total += t
	}

	avgTime := total / time.Duration(len(times))
	b.Logf("Average response time: %v", avgTime)
}

// RunAllAPIBenchmarks runs all API benchmarks
func (s *APIBenchmarkSuite) RunAllAPIBenchmarks(b *testing.B) {
	s.Setup(b)
	defer s.Cleanup()

	benchmarks := []struct {
		name string
		fn   func(*testing.B)
	}{
		{"HealthCheck", s.BenchmarkHealthCheck},
		{"GetActor", s.BenchmarkGetActor},
		{"VerifyCredentials", s.BenchmarkVerifyCredentials},
		{"GetHomeTimeline", s.BenchmarkGetHomeTimeline},
		{"CreateStatus", s.BenchmarkCreateStatus},
		{"Webfinger", s.BenchmarkWebfinger},
		{"ActivityPubInbox", s.BenchmarkActivityPubInbox},
		{"JSONParsing", s.BenchmarkJSONParsing},
		{"ConcurrentRequests", s.BenchmarkConcurrentRequests},
		{"RateLimiting", s.BenchmarkRateLimiting},
		{"MemoryUsage", s.BenchmarkMemoryUsage},
		{"ResponseTime", s.BenchmarkResponseTime},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, benchmark.fn)
	}
}

// BenchmarkEndToEndScenario benchmarks a complete user interaction scenario
func (s *APIBenchmarkSuite) BenchmarkEndToEndScenario(b *testing.B) {
	s.Setup(b)
	defer s.Cleanup()

	s.mastodonClient.WithToken("test-token")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate a typical user session

		// 1. Verify credentials
		resp := s.mastodonClient.VerifyCredentials()
		if resp.StatusCode >= 500 {
			b.Fatalf("Verify credentials failed: %d", resp.StatusCode)
		}

		// 2. Get home timeline
		resp = s.mastodonClient.GetHomeTimeline(map[string]string{"limit": "20"})
		if resp.StatusCode >= 500 {
			b.Fatalf("Get timeline failed: %d", resp.StatusCode)
		}

		// 3. Create a status
		status := map[string]interface{}{
			"status": fmt.Sprintf("End-to-end benchmark test %d", i),
		}
		resp = s.mastodonClient.CreateStatus(status)
		if resp.StatusCode >= 500 {
			b.Fatalf("Create status failed: %d", resp.StatusCode)
		}

		// 4. Get notifications
		resp = s.mastodonClient.GetNotifications(map[string]string{"limit": "10"})
		if resp.StatusCode >= 500 {
			b.Fatalf("Get notifications failed: %d", resp.StatusCode)
		}
	}
}
