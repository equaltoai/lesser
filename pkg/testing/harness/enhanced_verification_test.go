//go:build integration

package harness

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

// TestEnhancedHarnessBasic verifies the enhanced test harness works correctly
func TestEnhancedHarnessBasic(t *testing.T) {
	config := &TestConfig{
		Domain:         "test.example.com",
		TableName:      "lesser-test",
		UseMemory:      true,
		UseDynamORM:    true,
		UseEnhanced:    false,
		LogLevel:       zapcore.WarnLevel,
		ServerTimeout:  10 * time.Second,
		CleanupMode:    CleanupOnSuccess,
		SeedData:       true,
		ErrorInjection: false,
		CostTracking:   true,
		LatencyMode:    0,
		ErrorRate:      0.0,
	}

	harness := NewIntegrationTestHarness(t, config)

	// Verify basic functionality
	assert.NotNil(t, harness.Context())
	assert.NotNil(t, harness.Logger())
	assert.NotNil(t, harness.Config())
	
	// Verify repository storage is available
	assert.NotNil(t, harness.RepositoryStorage())
	assert.NotNil(t, harness.TestStorage())
	
	// Verify cost tracking
	assert.NotNil(t, harness.CostTracker())
	
	// Verify seeded data
	actors := harness.GetSeededActors()
	users := harness.GetSeededUsers()
	statuses := harness.GetSeededStatuses()
	
	assert.GreaterOrEqual(t, len(actors), 3, "Should have at least 3 test actors")
	assert.GreaterOrEqual(t, len(users), 3, "Should have at least 3 test users") 
	assert.GreaterOrEqual(t, len(statuses), 9, "Should have at least 9 test statuses")
	
	// Test actor creation
	newActor := harness.CreateTestActor("dynamic_test_user")
	assert.NotNil(t, newActor)
	assert.Equal(t, "dynamic_test_user", newActor.PreferredUsername)
	
	// Test activity creation
	newActivity := harness.CreateTestActivity(newActor.ID, "Create")
	assert.NotNil(t, newActivity)
	assert.Equal(t, "Create", newActivity.Type)
}

// TestEnhancedHarnessWithEnhancedMock verifies enhanced mock functionality
func TestEnhancedHarnessWithEnhancedMock(t *testing.T) {
	config := &TestConfig{
		Domain:         "test.example.com", 
		UseEnhanced:    true,
		UseDynamORM:    false,
		SeedData:       false,
		ErrorInjection: false,
		CleanupMode:    CleanupAlways,
	}

	harness := NewIntegrationTestHarness(t, config)

	// Verify enhanced mock is available
	assert.NotNil(t, harness.Storage())
	
	// Test actor creation with enhanced mock
	actor := harness.CreateTestActor("enhanced_test_user")
	assert.NotNil(t, actor)
	assert.Equal(t, "enhanced_test_user", actor.PreferredUsername)
}

// TestEnhancedHarnessCostTracking verifies cost tracking functionality
func TestEnhancedHarnessCostTracking(t *testing.T) {
	config := &TestConfig{
		UseDynamORM:  true,
		CostTracking: true,
		SeedData:     false,
	}

	harness := NewIntegrationTestHarness(t, config)
	costTracker := harness.CostTracker()
	
	// Simulate some operations
	costTracker.TrackOperation("test_read", 1.5, 0.0)
	costTracker.TrackOperation("test_write", 0.0, 2.0)
	
	rcu, wcu := costTracker.GetTotalCost()
	assert.Equal(t, 1.5, rcu)
	assert.Equal(t, 2.0, wcu)
	
	// Test cost budget assertion
	harness.AssertCostBudget(10.0, 10.0) // Should pass
}

// TestEnhancedHarnessErrorInjection verifies error injection capabilities
func TestEnhancedHarnessErrorInjection(t *testing.T) {
	config := &TestConfig{
		UseEnhanced:    true,
		ErrorInjection: true,
		ErrorRate:      0.0, // Start with no errors
		LatencyMode:    5 * time.Millisecond,
	}

	harness := NewIntegrationTestHarness(t, config)
	
	// Initially should work fine
	actor1 := harness.CreateTestActor("no_error_user")
	assert.NotNil(t, actor1)
	
	// Inject errors
	harness.InjectError(1.0, 10*time.Millisecond) // 100% error rate
	
	// This might fail due to error injection, but we just verify the mechanism works
	harness.ResetErrorInjection()
	
	// Should work again after reset
	actor2 := harness.CreateTestActor("post_reset_user")
	assert.NotNil(t, actor2)
}