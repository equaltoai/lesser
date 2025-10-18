package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCalculateTrend tests the trend classification logic
func TestCalculateTrend(t *testing.T) {
	resolver := &queryResolver{}

	tests := []struct {
		name         string
		currentCost  float64
		previousCost float64
		expected     string
	}{
		{
			name:         "increasing trend >10%",
			currentCost:  120.0,
			previousCost: 100.0,
			expected:     "INCREASING",
		},
		{
			name:         "increasing trend exactly 10%",
			currentCost:  110.0,
			previousCost: 100.0,
			expected:     "STABLE",
		},
		{
			name:         "increasing trend >10% boundary",
			currentCost:  110.01,
			previousCost: 100.0,
			expected:     "INCREASING",
		},
		{
			name:         "decreasing trend <-10%",
			currentCost:  80.0,
			previousCost: 100.0,
			expected:     "DECREASING",
		},
		{
			name:         "decreasing trend exactly -10%",
			currentCost:  90.0,
			previousCost: 100.0,
			expected:     "STABLE",
		},
		{
			name:         "decreasing trend <-10% boundary",
			currentCost:  89.99,
			previousCost: 100.0,
			expected:     "DECREASING",
		},
		{
			name:         "stable trend 5%",
			currentCost:  105.0,
			previousCost: 100.0,
			expected:     "STABLE",
		},
		{
			name:         "stable trend -5%",
			currentCost:  95.0,
			previousCost: 100.0,
			expected:     "STABLE",
		},
		{
			name:         "stable trend no change",
			currentCost:  100.0,
			previousCost: 100.0,
			expected:     "STABLE",
		},
		{
			name:         "no previous data",
			currentCost:  100.0,
			previousCost: 0.0,
			expected:     "STABLE",
		},
		{
			name:         "large increase 100%",
			currentCost:  200.0,
			previousCost: 100.0,
			expected:     "INCREASING",
		},
		{
			name:         "large decrease 75%",
			currentCost:  25.0,
			previousCost: 100.0,
			expected:     "DECREASING",
		},
		{
			name:         "small costs stable",
			currentCost:  0.5,
			previousCost: 0.5,
			expected:     "STABLE",
		},
		{
			name:         "small costs increasing 100%",
			currentCost:  1.0,
			previousCost: 0.5,
			expected:     "INCREASING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.calculateTrend(tt.currentCost, tt.previousCost)
			assert.Equal(t, tt.expected, result, "Trend classification mismatch")
		})
	}
}

// TestCalculateTrend_EdgeCases tests edge cases for trend calculation
func TestCalculateTrend_EdgeCases(t *testing.T) {
	resolver := &queryResolver{}

	t.Run("zero current cost with previous", func(t *testing.T) {
		// 0 vs 100 is a 100% decrease
		result := resolver.calculateTrend(0.0, 100.0)
		assert.Equal(t, "DECREASING", result)
	})

	t.Run("both zero", func(t *testing.T) {
		result := resolver.calculateTrend(0.0, 0.0)
		assert.Equal(t, "STABLE", result)
	})

	t.Run("very small previous cost", func(t *testing.T) {
		// 1.0 vs 0.001 is a massive increase
		result := resolver.calculateTrend(1.0, 0.001)
		assert.Equal(t, "INCREASING", result)
	})

	t.Run("10.001% increase boundary", func(t *testing.T) {
		result := resolver.calculateTrend(110.001, 100.0)
		assert.Equal(t, "INCREASING", result)
	})

	t.Run("9.999% increase boundary", func(t *testing.T) {
		result := resolver.calculateTrend(109.999, 100.0)
		assert.Equal(t, "STABLE", result)
	})

	t.Run("-10.001% decrease boundary", func(t *testing.T) {
		result := resolver.calculateTrend(89.999, 100.0)
		assert.Equal(t, "DECREASING", result)
	})

	t.Run("-9.999% decrease boundary", func(t *testing.T) {
		result := resolver.calculateTrend(90.001, 100.0)
		assert.Equal(t, "STABLE", result)
	})

	t.Run("negative previous cost edge case", func(t *testing.T) {
		// Should handle gracefully even with unusual inputs
		result := resolver.calculateTrend(100.0, -50.0)
		// Formula: (100 - (-50)) / (-50) * 100 = -300%
		assert.Equal(t, "DECREASING", result)
	})
}

// TestCalculateTrend_PercentageCalculation validates percentage calculation
func TestCalculateTrend_PercentageCalculation(t *testing.T) {
	resolver := &queryResolver{}

	tests := []struct {
		name            string
		currentCost     float64
		previousCost    float64
		expectedPercent float64
		expectedTrend   string
	}{
		{
			name:            "50% increase",
			currentCost:     150.0,
			previousCost:    100.0,
			expectedPercent: 50.0,
			expectedTrend:   "INCREASING",
		},
		{
			name:            "50% decrease",
			currentCost:     50.0,
			previousCost:    100.0,
			expectedPercent: -50.0,
			expectedTrend:   "DECREASING",
		},
		{
			name:            "100% increase (double)",
			currentCost:     200.0,
			previousCost:    100.0,
			expectedPercent: 100.0,
			expectedTrend:   "INCREASING",
		},
		{
			name:            "25% increase",
			currentCost:     125.0,
			previousCost:    100.0,
			expectedPercent: 25.0,
			expectedTrend:   "INCREASING",
		},
		{
			name:            "25% decrease",
			currentCost:     75.0,
			previousCost:    100.0,
			expectedPercent: -25.0,
			expectedTrend:   "DECREASING",
		},
		{
			name:            "exact 10% threshold",
			currentCost:     110.0,
			previousCost:    100.0,
			expectedPercent: 10.0,
			expectedTrend:   "STABLE", // Exactly at threshold, not greater
		},
		{
			name:            "exact -10% threshold",
			currentCost:     90.0,
			previousCost:    100.0,
			expectedPercent: -10.0,
			expectedTrend:   "STABLE", // Exactly at threshold, not less
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify percentage calculation
			actualPercent := ((tt.currentCost - tt.previousCost) / tt.previousCost) * 100
			assert.InDelta(t, tt.expectedPercent, actualPercent, 0.001, "Percentage calculation mismatch")

			// Test trend classification
			result := resolver.calculateTrend(tt.currentCost, tt.previousCost)
			assert.Equal(t, tt.expectedTrend, result, "Trend classification mismatch")
		})
	}
}

// TestCalculateTrend_ConsistentResults tests consistency of trend calculation
func TestCalculateTrend_ConsistentResults(t *testing.T) {
	resolver := &queryResolver{}

	// Same inputs should always produce same outputs
	for i := 0; i < 100; i++ {
		result1 := resolver.calculateTrend(120.0, 100.0)
		result2 := resolver.calculateTrend(120.0, 100.0)
		assert.Equal(t, result1, result2, "Trend calculation should be deterministic")
		assert.Equal(t, "INCREASING", result1)
	}
}
