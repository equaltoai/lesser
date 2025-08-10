package federation

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
)

func TestFederationRelationshipModel_UpdateKeys(t *testing.T) {
	now := time.Now()
	rel := &models.FederationRelationship{
		ID:               "test-rel-123",
		UserID:           "user123",
		TargetInstance:   "example.com",
		RelationshipType: "follow",
		State:            models.StateActive,
		LastActivity:     now,
	}

	rel.UpdateKeys()

	// Verify primary keys are set correctly
	assert.Equal(t, "USER#user123#FEDERATION", rel.PK)
	assert.Equal(t, "REL#example.com#follow#test-rel-123", rel.SK)

	// Verify GSI keys are set correctly
	assert.Equal(t, "FEDERATION_STATE#ACTIVE", rel.GSI1PK)
	assert.Contains(t, rel.GSI1SK, "example.com")
	assert.Contains(t, rel.GSI1SK, "test-rel-123")

	assert.Equal(t, "USER#user123#TARGET#example.com", rel.GSI2PK)
	assert.Contains(t, rel.GSI2SK, "follow")
}

func TestFederationRelationship_StateTransitions(t *testing.T) {
	now := time.Now()
	rel := &models.FederationRelationship{
		ID:           "test-rel-123",
		State:        models.StateActive,
		LastActivity: now.Add(-8 * 24 * time.Hour), // 8 days ago
	}

	// Should transition from ACTIVE to IDLE after 7 days
	newState, shouldTransition := rel.ShouldTransitionState()
	assert.True(t, shouldTransition)
	assert.Equal(t, models.StateIdle, newState)

	// Test transition from IDLE to DORMANT after 30 days
	rel.State = models.StateIdle
	rel.LastActivity = now.Add(-31 * 24 * time.Hour) // 31 days ago
	newState, shouldTransition = rel.ShouldTransitionState()
	assert.True(t, shouldTransition)
	assert.Equal(t, models.StateDormant, newState)

	// Test transition from DORMANT to ARCHIVED after 90 days
	rel.State = models.StateDormant
	rel.LastActivity = now.Add(-91 * 24 * time.Hour) // 91 days ago
	newState, shouldTransition = rel.ShouldTransitionState()
	assert.True(t, shouldTransition)
	assert.Equal(t, models.StateArchived, newState)
}

func TestFederationRelationship_SuccessRateCalculation(t *testing.T) {
	now := time.Now()
	rel := &models.FederationRelationship{
		WindowStart15m:  now.Truncate(15 * time.Minute),
		SuccessCount15m: 0,
		FailureCount15m: 0,
	}

	// Test successful request
	rel.UpdateSuccessRate(true, 500.0)
	assert.Equal(t, int64(1), rel.SuccessCount15m)
	assert.Equal(t, int64(0), rel.FailureCount15m)
	assert.Equal(t, 1.0, rel.SuccessRate)
	assert.Equal(t, 500.0, rel.AvgResponseTime)

	// Test failed request
	rel.UpdateSuccessRate(false, 1000.0)
	assert.Equal(t, int64(1), rel.SuccessCount15m)
	assert.Equal(t, int64(1), rel.FailureCount15m)
	assert.Equal(t, 0.5, rel.SuccessRate) // 50% success rate

	// Test response time averaging
	expectedAvgResponseTime := 500.0*0.9 + 1000.0*0.1 // Weighted average
	assert.InDelta(t, expectedAvgResponseTime, rel.AvgResponseTime, 0.1)
}

func TestFederationRelationship_WarmupPeriod(t *testing.T) {
	now := time.Now()
	rel := &models.FederationRelationship{
		State:          models.StateActive,
		StateChangedAt: now,
	}

	// Set warmup period
	warmupEnd := now.Add(1 * time.Hour)
	rel.WarmupUntil = &warmupEnd

	// Should be in warmup
	assert.True(t, rel.IsInWarmup())

	// Test traffic rate calculation during warmup
	trafficRate := rel.GetTrafficRate()
	assert.Greater(t, trafficRate, 0.0)
	assert.LessOrEqual(t, trafficRate, 1.0)

	// After warmup period, should return full traffic
	rel.WarmupUntil = &now // Warmup ended
	trafficRate = rel.GetTrafficRate()
	assert.Equal(t, 1.0, trafficRate)
}

func TestFederationRelationshipAggregate_UpdateKeys(t *testing.T) {
	now := time.Now()
	agg := &models.FederationRelationshipAggregate{
		InstanceDomain: "example.com",
		Period:         "15min",
		Timestamp:      now,
	}

	agg.UpdateKeys()

	// Verify primary keys
	assert.Equal(t, "INSTANCE#example.com#FEDERATION_AGG", agg.PK)
	assert.Contains(t, agg.SK, "PERIOD#15min")

	// Verify GSI keys
	assert.Equal(t, "FEDERATION_AGG#15min", agg.GSI1PK)
	assert.Contains(t, agg.GSI1SK, "example.com")

	// Verify TTL is set (15-minute data should have 7 days TTL)
	assert.Greater(t, agg.TTL, time.Now().Unix())
}

func TestGenerateRelationshipID(t *testing.T) {
	rt := &RelationshipTracker{}

	id1 := rt.generateRelationshipID("user1", "example.com", "follow")
	id2 := rt.generateRelationshipID("user1", "example.com", "follow")
	id3 := rt.generateRelationshipID("user2", "example.com", "follow")

	// Same inputs should generate same ID (deterministic)
	assert.Equal(t, id1, id2)

	// Different inputs should generate different IDs
	assert.NotEqual(t, id1, id3)

	// ID should be reasonable length (we truncate to 8 chars)
	assert.Len(t, id1, 8)
}
