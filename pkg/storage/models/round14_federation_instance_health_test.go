package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationInstanceHealthTracking_KeysAndScoring(t *testing.T) {
	t.Run("UpdateKeys sets unhealthy GSI2 keys with inverted score", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		h := &FederationInstanceHealthTracking{
			Domain:          "example.com",
			LastHealthCheck: ts,
			IsHealthy:       false,
			HealthScore:     0.25,
		}
		h.UpdateKeys()
		assert.Equal(t, "INSTANCE#example.com", h.PK)
		assert.Equal(t, "HEALTH", h.SK)
		assert.Equal(t, "HEALTH_CHECK#"+ts.Format("20060102"), h.GSI1PK)
		assert.Contains(t, h.GSI1SK, "TS#")
		assert.Equal(t, "UNHEALTHY", h.GSI2PK)
		assert.Contains(t, h.GSI2SK, "SCORE#0.7500#example.com")
		assert.Equal(t, MainTableName, h.TableName())
	})

	t.Run("UpdateKeys clears GSI2 when healthy", func(t *testing.T) {
		h := &FederationInstanceHealthTracking{
			Domain:          "example.com",
			LastHealthCheck: time.Unix(1700000000, 0).UTC(),
			IsHealthy:       true,
		}
		h.UpdateKeys()
		assert.Empty(t, h.GSI2PK)
		assert.Empty(t, h.GSI2SK)
	})

	t.Run("BeforeCreate sets timestamps, TTL, and computes health score", func(t *testing.T) {
		before := time.Now()
		h := &FederationInstanceHealthTracking{
			Domain:           "example.com",
			SuccessRate:      1.0,
			ResponseTimeP95:  100,
			ConsecutiveFails: 0,
		}
		require.NoError(t, h.BeforeCreate())
		assert.False(t, h.CreatedAt.IsZero())
		assert.False(t, h.UpdatedAt.IsZero())
		assert.False(t, h.LastHealthCheck.IsZero())
		assert.True(t, time.Unix(h.TTL, 0).After(before.Add(6*24*time.Hour)))
		assert.InDelta(t, 1.0, h.HealthScore, 0.000001)
		assert.True(t, h.IsHealthy)
	})

	t.Run("CalculateHealthScore covers response buckets and fail clamp", func(t *testing.T) {
		cases := []struct {
			success float64
			p95     int64
			fails   int
			want    bool
		}{
			{1.0, 400, 0, true},
			{1.0, 800, 0, true},
			{1.0, 1500, 0, true},
			{1.0, 3000, 0, true},
			{1.0, 6000, 0, true},  // still healthy with strong success and no failures
			{0.0, 6000, 0, false}, // low overall score from failures/latency weighting
			{1.0, 400, 3, false},  // consecutive fails threshold
			{1.0, 400, 10, false}, // clamp failScore to 0
		}

		for _, tc := range cases {
			h := &FederationInstanceHealthTracking{
				SuccessRate:      tc.success,
				ResponseTimeP95:  tc.p95,
				ConsecutiveFails: tc.fails,
			}
			h.CalculateHealthScore()
			assert.Equal(t, tc.want, h.IsHealthy)
			assert.True(t, h.HealthScore >= 0)
			assert.True(t, h.HealthScore <= 1.0)
		}
	})
}
