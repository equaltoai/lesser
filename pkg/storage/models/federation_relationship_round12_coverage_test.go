package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFederationRelationship_UpdateKeys_SetsKeysAndTTLByState(t *testing.T) {
	base := &FederationRelationship{
		ID:               "rel-123",
		UserID:           "user-1",
		TargetInstance:   "example.com",
		RelationshipType: "follow",
		LastActivity:     time.Unix(1700000000, 0).UTC(),
	}

	tests := []struct {
		name         string
		state        RelationshipState
		expectTTLSet bool
		expectTTLMin time.Duration
		expectTTLMax time.Duration
	}{
		{
			name:         "active_has_no_ttl",
			state:        StateActive,
			expectTTLSet: false,
		},
		{
			name:         "idle_has_90_day_ttl",
			state:        StateIdle,
			expectTTLSet: true,
			expectTTLMin: 90*24*time.Hour - 5*time.Second,
			expectTTLMax: 90*24*time.Hour + 5*time.Second,
		},
		{
			name:         "dormant_has_365_day_ttl",
			state:        StateDormant,
			expectTTLSet: true,
			expectTTLMin: 365*24*time.Hour - 5*time.Second,
			expectTTLMax: 365*24*time.Hour + 5*time.Second,
		},
		{
			name:         "archived_has_2_year_ttl",
			state:        StateArchived,
			expectTTLSet: true,
			expectTTLMin: 2*365*24*time.Hour - 5*time.Second,
			expectTTLMax: 2*365*24*time.Hour + 5*time.Second,
		},
		{
			name:         "expired_has_1_day_ttl",
			state:        StateExpired,
			expectTTLSet: true,
			expectTTLMin: 24*time.Hour - 5*time.Second,
			expectTTLMax: 24*time.Hour + 5*time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := *base
			fr.State = tt.state

			before := time.Now()
			fr.UpdateKeys()
			after := time.Now()

			assert.Equal(t, "USER#user-1#FEDERATION", fr.PK)
			assert.Equal(t, "REL#example.com#follow#rel-123", fr.SK)
			assert.Equal(t, "FEDERATION_STATE#"+string(tt.state), fr.GSI1PK)
			assert.Equal(t, "1700000000#example.com#rel-123", fr.GSI1SK)
			assert.Equal(t, "USER#user-1#TARGET#example.com", fr.GSI2PK)
			assert.Equal(t, "follow#1700000000", fr.GSI2SK)

			if !tt.expectTTLSet {
				assert.Equal(t, int64(0), fr.TTL)
				return
			}

			assert.NotZero(t, fr.TTL)
			min := before.Add(tt.expectTTLMin).Unix()
			max := after.Add(tt.expectTTLMax).Unix()
			assert.GreaterOrEqual(t, fr.TTL, min)
			assert.LessOrEqual(t, fr.TTL, max)
		})
	}
}

func TestFederationRelationship_UpdateSuccessRate_ResetsWindowAndUpdatesAverages(t *testing.T) {
	fr := &FederationRelationship{
		WindowStart15m:  time.Now().Add(-16 * time.Minute),
		SuccessCount15m: 5,
		FailureCount15m: 2,
		TotalSuccesses:  10,
		TotalFailures:   3,
		TotalAttempts:   13,
	}

	fr.UpdateSuccessRate(true, 100)
	assert.Equal(t, int64(1), fr.SuccessCount15m)
	assert.Equal(t, int64(0), fr.FailureCount15m)
	assert.Equal(t, int64(11), fr.TotalSuccesses)
	assert.Equal(t, int64(3), fr.TotalFailures)
	assert.Equal(t, int64(14), fr.TotalAttempts)
	assert.Equal(t, 1.0, fr.SuccessRate)
	assert.Equal(t, 100.0, fr.AvgResponseTime)
	assert.WithinDuration(t, time.Now(), fr.LastActivity, 2*time.Second)
	assert.WithinDuration(t, time.Now(), fr.UpdatedAt, 2*time.Second)

	fr.UpdateSuccessRate(false, 200)
	assert.Equal(t, int64(1), fr.SuccessCount15m)
	assert.Equal(t, int64(1), fr.FailureCount15m)
	assert.Equal(t, int64(11), fr.TotalSuccesses)
	assert.Equal(t, int64(4), fr.TotalFailures)
	assert.Equal(t, int64(15), fr.TotalAttempts)
	assert.Equal(t, 0.5, fr.SuccessRate)
	assert.InDelta(t, 110.0, fr.AvgResponseTime, 0.0001)

	fr.UpdateSuccessRate(true, 0)
	assert.InDelta(t, 110.0, fr.AvgResponseTime, 0.0001)
}

func TestFederationRelationship_StateTransitions(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		state         RelationshipState
		lastActivity  time.Time
		expectState   RelationshipState
		expectChanged bool
	}{
		{
			name:          "active_to_idle_after_7_days",
			state:         StateActive,
			lastActivity:  now.Add(-8 * 24 * time.Hour),
			expectState:   StateIdle,
			expectChanged: true,
		},
		{
			name:          "idle_to_active_when_recent",
			state:         StateIdle,
			lastActivity:  now.Add(-2 * 24 * time.Hour),
			expectState:   StateActive,
			expectChanged: true,
		},
		{
			name:          "idle_to_dormant_after_30_days",
			state:         StateIdle,
			lastActivity:  now.Add(-31 * 24 * time.Hour),
			expectState:   StateDormant,
			expectChanged: true,
		},
		{
			name:          "dormant_to_archived_after_90_days",
			state:         StateDormant,
			lastActivity:  now.Add(-91 * 24 * time.Hour),
			expectState:   StateArchived,
			expectChanged: true,
		},
		{
			name:          "archived_to_expired_after_365_days",
			state:         StateArchived,
			lastActivity:  now.Add(-366 * 24 * time.Hour),
			expectState:   StateExpired,
			expectChanged: true,
		},
		{
			name:          "expired_to_active_on_new_activity",
			state:         StateExpired,
			lastActivity:  now.Add(-2 * 24 * time.Hour),
			expectState:   StateActive,
			expectChanged: true,
		},
		{
			name:          "dormant_no_change_between_30_and_90_days",
			state:         StateDormant,
			lastActivity:  now.Add(-45 * 24 * time.Hour),
			expectState:   StateDormant,
			expectChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &FederationRelationship{
				State:        tt.state,
				LastActivity: tt.lastActivity,
			}
			got, changed := fr.ShouldTransitionState()
			assert.Equal(t, tt.expectState, got)
			assert.Equal(t, tt.expectChanged, changed)
		})
	}
}

func TestFederationRelationship_TransitionToState_SetsWarmupBaselineCompressAndClears(t *testing.T) {
	t.Run("reactivation_sets_warmup_and_applies_historical_baseline", func(t *testing.T) {
		fr := &FederationRelationship{
			ID:                 "rel-1",
			UserID:             "u",
			TargetInstance:     "example.com",
			RelationshipType:   "follow",
			LastActivity:       time.Now(),
			State:              StateArchived,
			HistoricalBaseline: 0.5,
			SuccessRate:        0.2,
		}

		fr.TransitionToState(StateActive)
		assert.Equal(t, StateActive, fr.State)
		if assert.NotNil(t, fr.WarmupUntil) {
			assert.WithinDuration(t, time.Now().Add(1*time.Hour), *fr.WarmupUntil, 5*time.Second)
		}
		assert.InDelta(t, 0.1, fr.CurrentRate, 0.0001)
		assert.InDelta(t, 0.15, fr.SuccessRate, 0.0001)
		assert.NotEmpty(t, fr.PK)
		assert.NotEmpty(t, fr.GSI1PK)
	})

	t.Run("dormant_transition_stores_historical_baseline", func(t *testing.T) {
		fr := &FederationRelationship{
			ID:               "rel-2",
			UserID:           "u",
			TargetInstance:   "example.com",
			RelationshipType: "follow",
			LastActivity:     time.Now(),
			State:            StateActive,
			SuccessRate:      0.8,
		}

		fr.TransitionToState(StateDormant)
		assert.Equal(t, StateDormant, fr.State)
		assert.InDelta(t, 0.8, fr.HistoricalBaseline, 0.0001)
	})

	t.Run("archived_transition_compresses_metrics", func(t *testing.T) {
		fr := &FederationRelationship{
			ID:                     "rel-3",
			UserID:                 "u",
			TargetInstance:         "example.com",
			RelationshipType:       "follow",
			State:                  StateDormant,
			FirstSeen:              time.Now().Add(-10 * 24 * time.Hour),
			LastActivity:           time.Now().Add(-2 * 24 * time.Hour),
			StateChangedAt:         time.Now().Add(-2 * time.Hour),
			SuccessCount15m:        3,
			FailureCount15m:        1,
			TotalAttempts:          10,
			LastCompressedAttempts: 4,
			SuccessRate:            0.75,
			AvgResponseTime:        123,
		}

		fr.TransitionToState(StateArchived)
		assert.Equal(t, StateArchived, fr.State)
		assert.True(t, fr.IsCompressed)
		assert.NotEmpty(t, fr.CompressedMetrics)
		assert.NotZero(t, fr.LastCompressionTime)
		assert.Equal(t, fr.TotalAttempts, fr.LastCompressedAttempts)
		assert.Equal(t, int64(0), fr.SuccessCount15m)
		assert.Equal(t, int64(0), fr.FailureCount15m)
	})

	t.Run("expired_transition_clears_sensitive_data", func(t *testing.T) {
		fr := &FederationRelationship{
			ID:                "rel-4",
			UserID:            "u",
			TargetInstance:    "example.com",
			RelationshipType:  "follow",
			State:             StateArchived,
			CompressedMetrics: "abc",
			ArchiveLocation:   "s3://bucket/key",
		}

		fr.TransitionToState(StateExpired)
		assert.Equal(t, StateExpired, fr.State)
		assert.Empty(t, fr.CompressedMetrics)
		assert.Empty(t, fr.ArchiveLocation)
	})
}

func TestFederationRelationship_GetTrafficRate_WarmupRamp(t *testing.T) {
	now := time.Now()
	warmupUntil := now.Add(1 * time.Hour)
	fr := &FederationRelationship{
		StateChangedAt: now.Add(-30 * time.Minute),
		WarmupUntil:    &warmupUntil,
	}

	rate := fr.GetTrafficRate()
	assert.GreaterOrEqual(t, rate, 0.1)
	assert.LessOrEqual(t, rate, 1.0)
	assert.InDelta(t, rate, fr.CurrentRate, 0.0001)
}

func TestMetricsCompression_ToBinary_HasStableLayout(t *testing.T) {
	mc := &MetricsCompression{
		Version:              2,
		CompressedAt:         time.Unix(1234, 0),
		TotalAttemptsDelta:   3,
		SuccessCountDelta:    4,
		FailureCountDelta:    5,
		QuantizedSuccessRate: 6,
		ResponseTimeP50:      7,
		ResponseTimeP95:      8,
		ResponseTimeP99:      9,
		StateTransitions:     []byte{1, 2, 3},
		ActivityPattern:      0xAABBCCDD,
	}

	b := mc.ToBinary()
	assert.NotEmpty(t, b)
	assert.Equal(t, byte(2), b[0])
	assert.Contains(t, b, byte(6))
	assert.Equal(t, byte(len(mc.StateTransitions)), b[len(b)-len(mc.StateTransitions)-1])
}

func TestCompressionHelpers(t *testing.T) {
	t.Run("compressStateHistory_maps_states", func(t *testing.T) {
		ts := time.Unix(42, 0)
		assert.Equal(t, byte(1), compressStateHistory(StateActive, ts)[0])
		assert.Equal(t, byte(2), compressStateHistory(StateIdle, ts)[0])
		assert.Equal(t, byte(3), compressStateHistory(StateDormant, ts)[0])
		assert.Equal(t, byte(4), compressStateHistory(StateArchived, ts)[0])
		assert.Equal(t, byte(5), compressStateHistory(StateExpired, ts)[0])
		assert.Equal(t, byte(0), compressStateHistory(RelationshipState("???"), ts)[0])
	})

	t.Run("compressActivityPattern_sets_recent_and_established_bits", func(t *testing.T) {
		now := time.Now()
		pattern := compressActivityPattern(now.Add(-2*24*time.Hour), now.Add(-8*24*time.Hour))
		assert.NotZero(t, pattern&(1<<2))
		assert.NotZero(t, pattern&(1<<31))
	})

	t.Run("encodeCompressedData_matches_base64_padding_cases", func(t *testing.T) {
		assert.Equal(t, "AA==", encodeCompressedData([]byte{0x00}))
		assert.Equal(t, "AAA=", encodeCompressedData([]byte{0x00, 0x00}))
		assert.Equal(t, "AAAA", encodeCompressedData([]byte{0x00, 0x00, 0x00}))
	})

	t.Run("quantizeSuccessRate_clamps", func(t *testing.T) {
		assert.Equal(t, uint8(0), quantizeSuccessRate(-0.1))
		assert.Equal(t, uint8(255), quantizeSuccessRate(2.0))
		assert.Equal(t, uint8(127), quantizeSuccessRate(0.5))
	})

	t.Run("quantizeResponseTime_clamps", func(t *testing.T) {
		assert.Equal(t, uint16(0), quantizeResponseTime(0))
		assert.Equal(t, uint16(0), quantizeResponseTime(-1))
		assert.Equal(t, uint16(65535), quantizeResponseTime(70000))
		assert.Equal(t, uint16(123), quantizeResponseTime(123.4))
	})
}
