// +build integration

package harness

import (
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/testing/factories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

// TestIntegrationHarnessExample demonstrates how to use the integration test harness
func TestIntegrationHarnessExample(t *testing.T) {
	// Create test harness with custom config
	config := &TestConfig{
		Domain:        "test.example.com",
		UseMemory:     true,
		LogLevel:      zapcore.WarnLevel,
		ServerTimeout: 30 * time.Second,
		CleanupMode:   CleanupOnSuccess,
	}

	harness := NewIntegrationTestHarness(t, config)
	
	// Create a simple test handler
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	})
	
	mux.HandleFunc("/users/", func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Path[len("/users/"):]
		
		// Use the harness storage to get actor
		_, err := harness.Storage().GetActor(r.Context(), username)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "Actor not found"}`))
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"username": "` + username + `"}`))
	})

	// Start the test server
	harness.StartServer(mux)

	// Create test data using factories
	testActor := harness.CreateTestActor("testuser")
	
	// Make API requests using the harness client
	resp := harness.MakeRequest("GET", "/health", nil, nil)
	assert.Equal(t, 200, resp.StatusCode)
	
	// Test actor retrieval
	resp = harness.MakeRequest("GET", "/users/testuser", nil, nil)
	assert.Equal(t, 200, resp.StatusCode)
	
	// Test with assertions helper
	assertions := NewTestAssertions(t)
	
	var actor map[string]interface{}
	assertions.AssertStatusCode(&APIResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       []byte("{}"),
		Response:   resp,
	}, 200)
	
	// Test timeline scenario
	timelineFactory := factories.NewTimelineFactory(config.Domain)
	timelineData := timelineFactory.CreateTimelineScenario("timelineuser", factories.SimpleTimeline)
	
	// Store timeline data in harness storage
	for _, actor := range timelineData.Following {
		err := harness.Storage().CreateActor(harness.Context(), actor, "test-key")
		require.NoError(t, err)
	}
	
	err := harness.Storage().CreateActor(harness.Context(), timelineData.User, "test-key")
	require.NoError(t, err)
	
	for _, activity := range timelineData.Activities {
		err := harness.Storage().StoreActivity(harness.Context(), activity)
		require.NoError(t, err)
	}
	
	// Wait for data to be available
	harness.WaitForCondition(func() bool {
		timeline, _ := harness.Storage().GetTimeline(harness.Context(), timelineData.User.PreferredUsername, 10, "")
		return len(timeline) > 0
	}, 5*time.Second, "Timeline should have activities")
	
	// Verify timeline has expected content
	timeline, err := harness.Storage().GetTimeline(harness.Context(), timelineData.User.PreferredUsername, 20, "")
	require.NoError(t, err)
	assert.True(t, len(timeline) > 0, "Timeline should have activities")
	
	harness.Logger().Info("Integration test completed successfully")
}

// TestAPIClientExample demonstrates API client usage
func TestAPIClientExample(t *testing.T) {
	// This would typically connect to a running test server
	// For demo purposes, we'll just show the API
	
	client := NewAPIClient(t, "https://test.example.com")
	mastodonClient := NewMastodonAPIClient(t, "https://test.example.com")
	
	// Set authorization token
	mastodonClient.WithToken("test-token")
	
	// Example usage (these would fail without a real server)
	if false { // Disabled for test compilation
		resp := client.GET("/health")
		assert.Equal(t, 200, resp.StatusCode)
		
		resp = mastodonClient.VerifyCredentials()
		assert.Equal(t, 200, resp.StatusCode)
		
		// Create a post
		status := map[string]interface{}{
			"status": "Hello from integration test!",
		}
		resp = mastodonClient.CreateStatus(status)
		assert.Equal(t, 201, resp.StatusCode)
		
		// Get timeline
		resp = mastodonClient.GetHomeTimeline(map[string]string{
			"limit": "20",
		})
		assert.Equal(t, 200, resp.StatusCode)
	}
}

// TestFactoriesExample demonstrates factory usage
func TestFactoriesExample(t *testing.T) {
	domain := "test.example.com"
	
	// Actor factory
	actorFactory := factories.NewActorFactory(domain)
	
	// Create different types of actors
	basicActor := actorFactory.CreateActor(factories.ActorOptions{
		Username: "basicuser",
	})
	assert.Equal(t, "basicuser", basicActor.PreferredUsername)
	assert.Contains(t, basicActor.ID, domain)
	
	botActor := actorFactory.CreateBotActor("testbot")
	assert.Equal(t, "Service", botActor.Type)
	
	lockedActor := actorFactory.CreateLockedActor("privateuser")
	assert.True(t, lockedActor.ManuallyApprovesFollowers)
	
	// Activity factory
	activityFactory := factories.NewActivityFactory(domain)
	
	// Create different activities
	note := activityFactory.CreateNote("Hello world!", basicActor.ID)
	assert.Equal(t, "Note", note.Type)
	assert.Equal(t, "Hello world!", note.Content)
	
	createActivity := activityFactory.CreateActivity(factories.ActivityOptions{
		Type:   "Create",
		Actor:  basicActor.ID,
		Object: note,
	})
	assert.Equal(t, "Create", createActivity.Type)
	assert.Equal(t, basicActor.ID, createActivity.Actor)
	
	followActivity := activityFactory.CreateFollow(basicActor.ID, botActor.ID)
	assert.Equal(t, "Follow", followActivity.Type)
	
	// Timeline factory
	timelineFactory := factories.NewTimelineFactory(domain)
	
	// Create different timeline scenarios
	emptyTimeline := timelineFactory.CreateTimelineScenario("emptyuser", factories.EmptyTimeline)
	assert.Empty(t, emptyTimeline.Activities)
	
	simpleTimeline := timelineFactory.CreateTimelineScenario("simpleuser", factories.SimpleTimeline)
	assert.NotEmpty(t, simpleTimeline.Following)
	assert.NotEmpty(t, simpleTimeline.Activities)
	
	mixedTimeline := timelineFactory.CreateTimelineScenario("mixeduser", factories.MixedTimeline)
	assert.True(t, len(mixedTimeline.Activities) > len(simpleTimeline.Activities))
	
	// Custom timeline
	customTimeline := timelineFactory.CreateCustomTimeline("customuser", 5, 10)
	assert.Len(t, customTimeline.Following, 5)
	assert.Len(t, customTimeline.Activities, 50) // 5 users * 10 posts
}

// BenchmarkIntegrationHarness benchmarks the integration harness setup
func BenchmarkIntegrationHarness(b *testing.B) {
	for i := 0; i < b.N; i++ {
		config := DefaultTestConfig()
		config.CleanupMode = CleanupNone // Don't cleanup during benchmark
		
		harness := NewIntegrationTestHarness(&testing.T{}, config)
		
		// Create some test data
		harness.CreateTestActor("benchuser")
		harness.CreateTestActivity("https://test.example.com/users/benchuser", "Create")
		
		// Simulate some operations
		_, _ = harness.Storage().GetActor(harness.Context(), "benchuser")
	}
}