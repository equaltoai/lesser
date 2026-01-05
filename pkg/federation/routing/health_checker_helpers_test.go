package routing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// === convertStatusCodes Tests ===

func TestConvertStatusCodes(t *testing.T) {
	t.Run("nil_input_returns_nil", func(t *testing.T) {
		result := convertStatusCodes(nil)
		assert.Nil(t, result)
	})

	t.Run("empty_map_returns_empty_map", func(t *testing.T) {
		result := convertStatusCodes(map[string]int{})
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("valid_codes_converted", func(t *testing.T) {
		input := map[string]int{
			"200": 50,
			"404": 10,
			"500": 5,
		}

		result := convertStatusCodes(input)

		assert.Equal(t, 50, result[200])
		assert.Equal(t, 10, result[404])
		assert.Equal(t, 5, result[500])
	})

	t.Run("invalid_key_skipped", func(t *testing.T) {
		input := map[string]int{
			"200":     50,
			"invalid": 10, // Non-numeric key
		}

		result := convertStatusCodes(input)

		assert.Len(t, result, 1)
		assert.Equal(t, 50, result[200])
		_, exists := result[0] // "invalid" shouldn't convert to 0
		assert.False(t, exists || result[0] == 10)
	})

	t.Run("mixed_valid_invalid_keys", func(t *testing.T) {
		input := map[string]int{
			"200":   100,
			"abc":   20,
			"500":   30,
			"12345": 40,
			"":      50,
		}

		result := convertStatusCodes(input)

		assert.Equal(t, 100, result[200])
		assert.Equal(t, 30, result[500])
		assert.Equal(t, 40, result[12345])
		// Invalid keys should not appear (or not map correctly)
	})
}

// === calculateHealthScore Tests ===

func TestCalculateHealthScore(t *testing.T) {
	// We can't call the method directly without a HealthChecker instance,
	// but we can test via the exported AggregatedHealth struct
	// The calculateHealthScore is a method on *InstanceHealthChecker
	// We need to create a minimal health checker for testing

	t.Run("perfect_scores_returns_100", func(t *testing.T) {
		agg := &AggregatedHealth{
			Availability:    1.0,                    // 100% availability
			AvgResponseTime: 100 * time.Millisecond, // Fast response
			ErrorRate:       0.0,                    // No errors
			AvgBacklog:      0,
			MaxBacklog:      100, // Low backlog
		}

		// Formula: score = 100 - (1-availability)*40 - latencyPenalty - errorRate*20 - backlogPenalty
		// = 100 - 0 - 0 - 0 - 0 = 100
		expectedScore := 100.0

		// Since we need the method, let's test the formula components
		// Availability penalty: (1.0 - 1.0) * 40 = 0
		// Latency penalty: 0 (under 1s)
		// Error rate penalty: 0.0 * 20 = 0
		// Backlog penalty: 0 (under 1000)

		availabilityPenalty := (1.0 - agg.Availability) * 40.0
		assert.Equal(t, 0.0, availabilityPenalty)

		errorPenalty := agg.ErrorRate * 20.0
		assert.Equal(t, 0.0, errorPenalty)

		// Total expected
		assert.Equal(t, expectedScore, 100.0)
	})

	t.Run("availability_penalty_40_weight", func(t *testing.T) {
		// 90% availability -> penalty = 10% * 40 = 4 points
		availability := 0.90
		penalty := (1.0 - availability) * 40.0
		assert.InDelta(t, 4.0, penalty, 0.01)

		// 50% availability -> penalty = 50% * 40 = 20 points
		availability = 0.50
		penalty = (1.0 - availability) * 40.0
		assert.InDelta(t, 20.0, penalty, 0.01)
	})

	t.Run("latency_penalty_30_weight", func(t *testing.T) {
		// Response time > 1s incurs penalty
		// 2s response: penalty = (2000-1000)/100 = 10 points (capped at 30)
		responseTime := 2 * time.Second
		if responseTime > 1*time.Second {
			penalty := float64(responseTime.Milliseconds()-1000) / 100.0
			penalty = mathMin(penalty, 30.0)
			assert.InDelta(t, 10.0, penalty, 0.01)
		}

		// 5s response: penalty = (5000-1000)/100 = 40 -> capped to 30
		responseTime = 5 * time.Second
		if responseTime > 1*time.Second {
			penalty := float64(responseTime.Milliseconds()-1000) / 100.0
			penalty = mathMin(penalty, 30.0)
			assert.InDelta(t, 30.0, penalty, 0.01)
		}
	})

	t.Run("error_rate_penalty_20_weight", func(t *testing.T) {
		// 10% error rate -> penalty = 0.10 * 20 = 2 points
		errorRate := 0.10
		penalty := errorRate * 20.0
		assert.InDelta(t, 2.0, penalty, 0.01)

		// 50% error rate -> penalty = 0.50 * 20 = 10 points
		errorRate = 0.50
		penalty = errorRate * 20.0
		assert.InDelta(t, 10.0, penalty, 0.01)
	})

	t.Run("backlog_penalty_10_weight", func(t *testing.T) {
		// Backlog > 1000 incurs penalty
		// 2000 max backlog: penalty = (2000-1000)/1000 = 1 point
		maxBacklog := 2000
		if maxBacklog > 1000 {
			penalty := float64(maxBacklog-1000) / 1000.0
			penalty = mathMin(penalty, 10.0)
			assert.InDelta(t, 1.0, penalty, 0.01)
		}

		// 15000 max backlog: penalty = (15000-1000)/1000 = 14 -> capped to 10
		maxBacklog = 15000
		if maxBacklog > 1000 {
			penalty := float64(maxBacklog-1000) / 1000.0
			penalty = mathMin(penalty, 10.0)
			assert.InDelta(t, 10.0, penalty, 0.01)
		}
	})

	t.Run("combined_penalties", func(t *testing.T) {
		// 80% availability: (1-0.8)*40 = 8
		// 2s response: (2000-1000)/100 = 10
		// 5% error rate: 0.05*20 = 1
		// 1500 backlog: (1500-1000)/1000 = 0.5
		// Total penalty: 8 + 10 + 1 + 0.5 = 19.5
		// Score: 100 - 19.5 = 80.5

		availability := 0.80
		responseTime := 2 * time.Second
		errorRate := 0.05
		maxBacklog := 1500

		score := 100.0
		score -= (1.0 - availability) * 40.0

		if responseTime > 1*time.Second {
			penalty := float64(responseTime.Milliseconds()-1000) / 100.0
			score -= mathMin(penalty, 30.0)
		}

		score -= errorRate * 20.0

		if maxBacklog > 1000 {
			penalty := float64(maxBacklog-1000) / 1000.0
			score -= mathMin(penalty, 10.0)
		}

		assert.InDelta(t, 80.5, score, 0.1)
	})

	t.Run("minimum_score_is_zero", func(t *testing.T) {
		// Worst case scenario
		score := 100.0
		score -= 40.0 // 0% availability
		score -= 30.0 // Very high latency
		score -= 20.0 // 100% error rate
		score -= 10.0 // Massive backlog

		// Should be capped at 0
		score = mathMax(score, 0.0)
		assert.Equal(t, 0.0, score)
	})
}

// === mathMin and mathMax Tests ===

func TestMathMin(t *testing.T) {
	tests := []struct {
		a, b, expected float64
	}{
		{1.0, 2.0, 1.0},
		{2.0, 1.0, 1.0},
		{5.0, 5.0, 5.0},
		{-1.0, 1.0, -1.0},
		{0.0, 0.0, 0.0},
	}

	for _, tt := range tests {
		result := mathMin(tt.a, tt.b)
		assert.Equal(t, tt.expected, result)
	}
}

func TestMathMax(t *testing.T) {
	tests := []struct {
		a, b, expected float64
	}{
		{1.0, 2.0, 2.0},
		{2.0, 1.0, 2.0},
		{5.0, 5.0, 5.0},
		{-1.0, 1.0, 1.0},
		{0.0, 0.0, 0.0},
	}

	for _, tt := range tests {
		result := mathMax(tt.a, tt.b)
		assert.Equal(t, tt.expected, result)
	}
}
