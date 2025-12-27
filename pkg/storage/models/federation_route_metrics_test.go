package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// FederationRouteMetrics - determinePerformanceTier Tests
// =============================================================================

func TestFederationRouteMetrics_DeterminePerformanceTier(t *testing.T) {
	testCases := []struct {
		name         string
		avgLatencyMs int64
		expectedTier string
	}{
		{
			name:         "excellent when latency is 0",
			avgLatencyMs: 0,
			expectedTier: "excellent",
		},
		{
			name:         "excellent when latency is exactly 100ms",
			avgLatencyMs: 100,
			expectedTier: "excellent",
		},
		{
			name:         "excellent when latency is below 100ms",
			avgLatencyMs: 50,
			expectedTier: "excellent",
		},
		{
			name:         "good when latency is 101ms",
			avgLatencyMs: 101,
			expectedTier: "good",
		},
		{
			name:         "good when latency is exactly 300ms",
			avgLatencyMs: 300,
			expectedTier: "good",
		},
		{
			name:         "good when latency is 200ms",
			avgLatencyMs: 200,
			expectedTier: "good",
		},
		{
			name:         "fair when latency is 301ms",
			avgLatencyMs: 301,
			expectedTier: "fair",
		},
		{
			name:         "fair when latency is exactly 1000ms",
			avgLatencyMs: 1000,
			expectedTier: "fair",
		},
		{
			name:         "fair when latency is 500ms",
			avgLatencyMs: 500,
			expectedTier: "fair",
		},
		{
			name:         "poor when latency exceeds 1000ms",
			avgLatencyMs: 1001,
			expectedTier: "poor",
		},
		{
			name:         "poor when latency is 5000ms",
			avgLatencyMs: 5000,
			expectedTier: "poor",
		},
		{
			name:         "poor for very high latency",
			avgLatencyMs: 10000,
			expectedTier: "poor",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			frm := &FederationRouteMetrics{
				AvgLatencyMs: tc.avgLatencyMs,
			}

			tier := frm.determinePerformanceTier()

			assert.Equal(t, tc.expectedTier, tier)
		})
	}
}

// =============================================================================
// FederationRouteMetrics - UpdateKeys Tests
// =============================================================================

func TestFederationRouteMetrics_UpdateKeys(t *testing.T) {
	// Fixed time for deterministic key generation
	fixedPeriodStart := time.Date(2024, 6, 15, 10, 30, 45, 0, time.UTC)

	t.Run("sets PK with route id and compact date", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:     "route-abc-123",
			PeriodStart: fixedPeriodStart,
			PeriodType:  "hour",
		}

		frm.UpdateKeys()

		// Expected: FED_ROUTE#route-abc-123#20240615
		assert.Equal(t, "FED_ROUTE#route-abc-123#20240615", frm.PK)
	})

	t.Run("sets SK with METRICS prefix and period type", func(t *testing.T) {
		testCases := []struct {
			periodType string
			expectedSK string
		}{
			{periodType: "hour", expectedSK: "METRICS#hour"},
			{periodType: "day", expectedSK: "METRICS#day"},
			{periodType: "week", expectedSK: "METRICS#week"},
			{periodType: "month", expectedSK: "METRICS#month"},
		}

		for _, tc := range testCases {
			t.Run(tc.periodType, func(t *testing.T) {
				frm := &FederationRouteMetrics{
					RouteID:     "route-123",
					PeriodStart: fixedPeriodStart,
					PeriodType:  tc.periodType,
				}

				frm.UpdateKeys()

				assert.Equal(t, tc.expectedSK, frm.SK)
			})
		}
	})

	t.Run("sets GSI1PK with date and GSI1SK with route and timestamp", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:     "route-xyz",
			PeriodStart: fixedPeriodStart,
			PeriodType:  "hour",
		}

		frm.UpdateKeys()

		// GSI1PK: FED_ROUTES#{date}
		assert.Equal(t, "FED_ROUTES#20240615", frm.GSI1PK)
		// GSI1SK: ROUTE#{route_id}#{timestamp}
		assert.Equal(t, "ROUTE#route-xyz#20240615103045", frm.GSI1SK)
	})

	t.Run("sets GSI2PK with destination domain", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:           "route-123",
			DestinationDomain: "mastodon.social",
			PeriodStart:       fixedPeriodStart,
			PeriodType:        "hour",
		}

		frm.UpdateKeys()

		// GSI2PK: FED_DOMAIN_ROUTES#{domain}
		assert.Equal(t, "FED_DOMAIN_ROUTES#mastodon.social", frm.GSI2PK)
		// GSI2SK: ROUTE#{route_id}#{timestamp}
		assert.Equal(t, "ROUTE#route-123#20240615103045", frm.GSI2SK)
	})

	t.Run("sets GSI3PK with performance tier", func(t *testing.T) {
		testCases := []struct {
			name         string
			avgLatencyMs int64
			expectedTier string
		}{
			{name: "excellent tier", avgLatencyMs: 50, expectedTier: "excellent"},
			{name: "good tier", avgLatencyMs: 200, expectedTier: "good"},
			{name: "fair tier", avgLatencyMs: 500, expectedTier: "fair"},
			{name: "poor tier", avgLatencyMs: 2000, expectedTier: "poor"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				frm := &FederationRouteMetrics{
					RouteID:      "route-123",
					PeriodStart:  fixedPeriodStart,
					PeriodType:   "hour",
					AvgLatencyMs: tc.avgLatencyMs,
				}

				frm.UpdateKeys()

				expectedGSI3PK := "FED_ROUTE_PERF#" + tc.expectedTier
				assert.Equal(t, expectedGSI3PK, frm.GSI3PK)
			})
		}
	})

	t.Run("sets GSI3SK with 6-digit padded latency and route id", func(t *testing.T) {
		testCases := []struct {
			name         string
			avgLatencyMs int64
			routeID      string
			expectedSK   string
		}{
			{
				name:         "small latency",
				avgLatencyMs: 50,
				routeID:      "route-abc",
				expectedSK:   "LATENCY#000050#route-abc",
			},
			{
				name:         "medium latency",
				avgLatencyMs: 500,
				routeID:      "route-xyz",
				expectedSK:   "LATENCY#000500#route-xyz",
			},
			{
				name:         "large latency",
				avgLatencyMs: 12345,
				routeID:      "route-123",
				expectedSK:   "LATENCY#012345#route-123",
			},
			{
				name:         "zero latency",
				avgLatencyMs: 0,
				routeID:      "route-zero",
				expectedSK:   "LATENCY#000000#route-zero",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				frm := &FederationRouteMetrics{
					RouteID:      tc.routeID,
					PeriodStart:  fixedPeriodStart,
					PeriodType:   "hour",
					AvgLatencyMs: tc.avgLatencyMs,
				}

				frm.UpdateKeys()

				assert.Equal(t, tc.expectedSK, frm.GSI3SK)
			})
		}
	})
}

// =============================================================================
// FederationRouteMetrics - BeforeCreate Tests
// =============================================================================

func TestFederationRouteMetrics_BeforeCreate(t *testing.T) {
	t.Run("initializes CreatedAt when zero", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:    "route-123",
			PeriodType: "hour",
		}

		before := time.Now()
		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.False(t, frm.CreatedAt.IsZero())
		assert.WithinDuration(t, before, frm.CreatedAt, 2*time.Second)
	})

	t.Run("preserves existing CreatedAt", func(t *testing.T) {
		existingTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		frm := &FederationRouteMetrics{
			RouteID:    "route-123",
			PeriodType: "hour",
			CreatedAt:  existingTime,
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, existingTime, frm.CreatedAt)
	})

	t.Run("initializes FirstUsed when zero", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:    "route-123",
			PeriodType: "hour",
		}

		before := time.Now()
		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.False(t, frm.FirstUsed.IsZero())
		assert.WithinDuration(t, before, frm.FirstUsed, 2*time.Second)
	})

	t.Run("preserves existing FirstUsed", func(t *testing.T) {
		existingTime := time.Date(2024, 3, 15, 8, 0, 0, 0, time.UTC)
		frm := &FederationRouteMetrics{
			RouteID:    "route-123",
			PeriodType: "hour",
			FirstUsed:  existingTime,
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, existingTime, frm.FirstUsed)
	})

	t.Run("sets UpdatedAt to now", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:    "route-123",
			PeriodType: "hour",
		}

		before := time.Now()
		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.False(t, frm.UpdatedAt.IsZero())
		assert.WithinDuration(t, before, frm.UpdatedAt, 2*time.Second)
	})

	t.Run("initializes nil ErrorBreakdown map", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:    "route-123",
			PeriodType: "hour",
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.NotNil(t, frm.ErrorBreakdown)
		assert.Empty(t, frm.ErrorBreakdown)
	})

	t.Run("preserves existing ErrorBreakdown map", func(t *testing.T) {
		existingBreakdown := map[string]int64{"500": 5, "502": 3}
		frm := &FederationRouteMetrics{
			RouteID:        "route-123",
			PeriodType:     "hour",
			ErrorBreakdown: existingBreakdown,
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, existingBreakdown, frm.ErrorBreakdown)
	})

	t.Run("initializes nil HealthHistory slice", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:    "route-123",
			PeriodType: "hour",
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		// HealthHistory is initialized as empty slice, but calculateDerivedMetrics
		// appends the current health score to it during BeforeCreate
		assert.NotNil(t, frm.HealthHistory)
		assert.Len(t, frm.HealthHistory, 1, "should have one health score entry from calculateDerivedMetrics")
	})

	t.Run("preserves existing HealthHistory slice", func(t *testing.T) {
		existingHistory := []float64{0.8, 0.85, 0.9}
		frm := &FederationRouteMetrics{
			RouteID:       "route-123",
			PeriodType:    "hour",
			HealthHistory: existingHistory,
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		// Note: calculateDerivedMetrics may append to history
		assert.True(t, len(frm.HealthHistory) >= len(existingHistory))
	})

	t.Run("sets TTL to approximately 90 days from creation", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:    "route-123",
			PeriodType: "hour",
		}

		before := time.Now()
		err := frm.BeforeCreate()

		require.NoError(t, err)
		expectedTTL := before.AddDate(0, 0, 90).Unix()
		assert.InDelta(t, expectedTTL, frm.TTL, 5, "TTL should be ~90 days from now")
	})

	t.Run("calls UpdateKeys", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:           "route-456",
			DestinationDomain: "example.com",
			PeriodType:        "day",
			PeriodStart:       time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.Contains(t, frm.PK, "FED_ROUTE#route-456#")
		assert.Equal(t, "METRICS#day", frm.SK)
	})
}

// =============================================================================
// FederationRouteMetrics - BeforeCreate Derived Metrics Tests
// =============================================================================

func TestFederationRouteMetrics_BeforeCreate_CalculatesDerivedMetrics(t *testing.T) {
	t.Run("calculates success rate", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:            "route-123",
			PeriodType:         "hour",
			TotalAttempts:      100,
			SuccessfulAttempts: 85,
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 85.0, frm.SuccessRate, 0.01)
	})

	t.Run("calculates timeout rate", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:       "route-123",
			PeriodType:    "hour",
			TotalAttempts: 200,
			TimeoutCount:  20,
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 10.0, frm.TimeoutRate, 0.01)
	})

	t.Run("calculates average cost per delivery", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:             "route-123",
			PeriodType:          "hour",
			TotalAttempts:       100,
			SuccessfulAttempts:  50,
			TotalCostMicroCents: 500_000, // 50 cents
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		// 500_000 / 50 = 10_000 micro cents per delivery
		assert.Equal(t, int64(10_000), frm.AvgCostPerDelivery)
	})

	t.Run("handles zero total attempts gracefully", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:       "route-123",
			PeriodType:    "hour",
			TotalAttempts: 0,
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, float64(0), frm.SuccessRate)
		assert.Equal(t, float64(0), frm.TimeoutRate)
	})

	t.Run("handles zero successful attempts gracefully", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:             "route-123",
			PeriodType:          "hour",
			TotalAttempts:       100,
			SuccessfulAttempts:  0,
			TotalCostMicroCents: 100_000,
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, int64(0), frm.AvgCostPerDelivery)
	})

	t.Run("calculates average retries per failure", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:        "route-123",
			PeriodType:     "hour",
			TotalAttempts:  100,
			FailedAttempts: 20,
			TotalRetries:   60, // 3 retries per failure on average
		}

		err := frm.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 3.0, frm.AvgRetriesPerFail, 0.01)
	})
}

// =============================================================================
// FederationRouteMetrics - BeforeUpdate Tests
// =============================================================================

func TestFederationRouteMetrics_BeforeUpdate(t *testing.T) {
	t.Run("updates UpdatedAt timestamp", func(t *testing.T) {
		oldTime := time.Now().Add(-time.Hour)
		frm := &FederationRouteMetrics{
			RouteID:    "route-123",
			PeriodType: "hour",
			UpdatedAt:  oldTime,
		}

		before := time.Now()
		err := frm.BeforeUpdate()

		require.NoError(t, err)
		assert.WithinDuration(t, before, frm.UpdatedAt, 2*time.Second)
		assert.True(t, frm.UpdatedAt.After(oldTime))
	})

	t.Run("recalculates derived metrics", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:            "route-123",
			PeriodType:         "hour",
			TotalAttempts:      200,
			SuccessfulAttempts: 180,
			SuccessRate:        0, // Will be recalculated
		}

		err := frm.BeforeUpdate()

		require.NoError(t, err)
		assert.InDelta(t, 90.0, frm.SuccessRate, 0.01)
	})

	t.Run("updates keys", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			RouteID:           "route-update-test",
			DestinationDomain: "updated.example.com",
			PeriodType:        "week",
			PeriodStart:       time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
			AvgLatencyMs:      150, // good tier
		}

		err := frm.BeforeUpdate()

		require.NoError(t, err)
		assert.Contains(t, frm.PK, "FED_ROUTE#route-update-test#")
		assert.Equal(t, "METRICS#week", frm.SK)
		assert.Equal(t, "FED_DOMAIN_ROUTES#updated.example.com", frm.GSI2PK)
		assert.Equal(t, "FED_ROUTE_PERF#good", frm.GSI3PK)
	})
}

// =============================================================================
// FederationRouteMetrics - TableName Test
// =============================================================================

func TestFederationRouteMetrics_TableName(t *testing.T) {
	frm := FederationRouteMetrics{}
	assert.Equal(t, MainTableName, frm.TableName())
}

// =============================================================================
// FederationRouteMetrics - Health Score Calculation Tests
// =============================================================================

func TestFederationRouteMetrics_HealthScoreCalculation(t *testing.T) {
	t.Run("reliability score decreases for long failure streaks", func(t *testing.T) {
		// Route with short failure streak
		frmShort := &FederationRouteMetrics{
			RouteID:             "route-short",
			PeriodType:          "hour",
			TotalAttempts:       100,
			SuccessfulAttempts:  90,
			MaxConsecutiveFails: 3,
		}

		// Route with long failure streak
		frmLong := &FederationRouteMetrics{
			RouteID:             "route-long",
			PeriodType:          "hour",
			TotalAttempts:       100,
			SuccessfulAttempts:  90,
			MaxConsecutiveFails: 10,
		}

		err := frmShort.BeforeCreate()
		require.NoError(t, err)

		err = frmLong.BeforeCreate()
		require.NoError(t, err)

		// Long failure streak should have lower reliability score
		assert.Greater(t, frmShort.ReliabilityScore, frmLong.ReliabilityScore)
	})

	t.Run("performance score inversely proportional to latency", func(t *testing.T) {
		frmFast := &FederationRouteMetrics{
			RouteID:      "route-fast",
			PeriodType:   "hour",
			P95LatencyMs: 100,
		}

		frmSlow := &FederationRouteMetrics{
			RouteID:      "route-slow",
			PeriodType:   "hour",
			P95LatencyMs: 1000,
		}

		err := frmFast.BeforeCreate()
		require.NoError(t, err)

		err = frmSlow.BeforeCreate()
		require.NoError(t, err)

		// Fast route should have higher performance score
		assert.Greater(t, frmFast.PerformanceScore, frmSlow.PerformanceScore)
	})
}

// =============================================================================
// FederationRouteMetrics - IsHealthy Tests
// =============================================================================

func TestFederationRouteMetrics_IsHealthy(t *testing.T) {
	testCases := []struct {
		name                string
		circuitBreakerState string
		healthScore         float64
		successRate         float64
		expectedHealthy     bool
	}{
		{
			name:                "healthy with high scores",
			circuitBreakerState: CircuitBreakerStateClosed,
			healthScore:         0.85,
			successRate:         95.0,
			expectedHealthy:     true,
		},
		{
			name:                "unhealthy when circuit breaker open",
			circuitBreakerState: CircuitBreakerStateOpen,
			healthScore:         0.9,
			successRate:         95.0,
			expectedHealthy:     false,
		},
		{
			name:                "unhealthy with low health score",
			circuitBreakerState: CircuitBreakerStateClosed,
			healthScore:         0.5,
			successRate:         95.0,
			expectedHealthy:     false,
		},
		{
			name:                "unhealthy with low success rate",
			circuitBreakerState: CircuitBreakerStateClosed,
			healthScore:         0.85,
			successRate:         80.0,
			expectedHealthy:     false,
		},
		{
			name:                "healthy at threshold",
			circuitBreakerState: "",
			healthScore:         0.7,
			successRate:         90.0,
			expectedHealthy:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			frm := &FederationRouteMetrics{
				CircuitBreakerState: tc.circuitBreakerState,
				HealthScore:         tc.healthScore,
				SuccessRate:         tc.successRate,
			}

			result := frm.IsHealthy()

			assert.Equal(t, tc.expectedHealthy, result)
		})
	}
}

// =============================================================================
// FederationRouteMetrics - GetRouteSummary Tests
// =============================================================================

func TestFederationRouteMetrics_GetRouteSummary(t *testing.T) {
	frm := &FederationRouteMetrics{
		RouteID:             "route-summary-test",
		DestinationDomain:   "example.com",
		RouteType:           "primary",
		HealthScore:         0.85,
		SuccessRate:         95.5,
		AvgLatencyMs:        150,
		TotalAttempts:       1000,
		CircuitBreakerState: CircuitBreakerStateClosed,
		CostEfficiencyScore: 0.75,
		HealthTrend:         "stable",
	}

	summary := frm.GetRouteSummary()

	assert.Equal(t, "route-summary-test", summary["route_id"])
	assert.Equal(t, "example.com", summary["destination_domain"])
	assert.Equal(t, "primary", summary["route_type"])
	assert.Equal(t, 0.85, summary["health_score"])
	assert.Equal(t, 95.5, summary["success_rate"])
	assert.Equal(t, int64(150), summary["avg_latency_ms"])
	assert.Equal(t, int64(1000), summary["total_attempts"])
	assert.Equal(t, CircuitBreakerStateClosed, summary["circuit_breaker"])
	assert.Equal(t, 0.75, summary["cost_efficiency"])
	assert.Equal(t, "stable", summary["health_trend"])
}

// =============================================================================
// FederationRouteMetrics - GetOptimizationRecommendations Tests
// =============================================================================

func TestFederationRouteMetrics_GetOptimizationRecommendations(t *testing.T) {
	t.Run("recommends optimization for high latency", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			AvgLatencyMs: 1500,
			SuccessRate:  95.0,
		}

		recs := frm.GetOptimizationRecommendations()

		var found bool
		for _, rec := range recs {
			if rec.Type == "performance" && rec.Title == "High Latency Detected" {
				found = true
				assert.Equal(t, "high", rec.Priority)
				assert.Equal(t, "optimize_route", rec.Action)
				break
			}
		}
		assert.True(t, found, "should have high latency recommendation")
	})

	t.Run("recommends investigation for low success rate", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			AvgLatencyMs: 100,
			SuccessRate:  85.0,
		}

		recs := frm.GetOptimizationRecommendations()

		var found bool
		for _, rec := range recs {
			if rec.Type == "reliability" && rec.Title == "Low Success Rate" {
				found = true
				assert.Equal(t, "high", rec.Priority)
				assert.Equal(t, "investigate_errors", rec.Action)
				break
			}
		}
		assert.True(t, found, "should have low success rate recommendation")
	})

	t.Run("critical recommendation for circuit breaker open", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			CircuitBreakerState: CircuitBreakerStateOpen,
			SuccessRate:         95.0,
			AvgLatencyMs:        100,
		}

		recs := frm.GetOptimizationRecommendations()

		var found bool
		for _, rec := range recs {
			if rec.Type == "availability" && rec.Priority == "critical" {
				found = true
				assert.Equal(t, "Route Circuit Breaker Open", rec.Title)
				break
			}
		}
		assert.True(t, found, "should have circuit breaker recommendation")
	})

	t.Run("returns empty for healthy route", func(t *testing.T) {
		frm := &FederationRouteMetrics{
			AvgLatencyMs:        100,
			SuccessRate:         99.0,
			CircuitBreakerState: CircuitBreakerStateClosed,
			CostEfficiencyScore: 0.8,
			HealthTrend:         "improving",
		}

		recs := frm.GetOptimizationRecommendations()

		assert.Empty(t, recs)
	})
}

// =============================================================================
// RouteRecommendation - TableName Test
// =============================================================================

func TestRouteRecommendation_TableName(t *testing.T) {
	rec := RouteRecommendation{}
	assert.Equal(t, MainTableName, rec.TableName())
}

// =============================================================================
// FederationRouteAggregation - UpdateKeys Tests
// =============================================================================

func TestFederationRouteAggregation_UpdateKeys(t *testing.T) {
	t.Run("sets PK with period and route id", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:     "route-agg-123",
			Period:      "day",
			PeriodStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		}

		fra.UpdateKeys()

		assert.Equal(t, "FED_ROUTE_AGG#day#route-agg-123", fra.PK)
	})

	t.Run("sets SK with compact date", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:     "route-123",
			Period:      "week",
			PeriodStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		}

		fra.UpdateKeys()

		assert.Equal(t, "20240615", fra.SK)
	})

	t.Run("sets GSI1 keys for route comparison", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:        "route-compare-test",
			Period:         "month",
			PeriodStart:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			AvgHealthScore: 0.85,
		}

		fra.UpdateKeys()

		assert.Equal(t, "FED_ROUTE_COMPARE#month", fra.GSI1PK)
		// SCORE#{health_score*100 with 2 decimals, padded to 6}#{route_id}
		assert.Equal(t, "SCORE#085.00#route-compare-test", fra.GSI1SK)
	})
}

// =============================================================================
// FederationRouteAggregation - BeforeCreate Tests
// =============================================================================

func TestFederationRouteAggregation_BeforeCreate(t *testing.T) {
	t.Run("initializes CreatedAt when zero", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:     "route-123",
			Period:      "day",
			PeriodStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		}

		before := time.Now()
		err := fra.BeforeCreate()

		require.NoError(t, err)
		assert.False(t, fra.CreatedAt.IsZero())
		assert.WithinDuration(t, before, fra.CreatedAt, 2*time.Second)
	})

	t.Run("preserves existing CreatedAt", func(t *testing.T) {
		existingTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		fra := &FederationRouteAggregation{
			RouteID:     "route-123",
			Period:      "day",
			PeriodStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			CreatedAt:   existingTime,
		}

		err := fra.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, existingTime, fra.CreatedAt)
	})

	t.Run("sets UpdatedAt to now", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:     "route-123",
			Period:      "day",
			PeriodStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		}

		before := time.Now()
		err := fra.BeforeCreate()

		require.NoError(t, err)
		assert.WithinDuration(t, before, fra.UpdatedAt, 2*time.Second)
	})

	t.Run("calculates percentile score from overall rank", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:     "route-123",
			Period:      "day",
			PeriodStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			OverallRank: 10, // 10th out of assumed 100
		}

		err := fra.BeforeCreate()

		require.NoError(t, err)
		// (100 - 10) / 100 * 100 = 90th percentile
		assert.InDelta(t, 90.0, fra.PercentileScore, 0.01)
	})

	t.Run("skips percentile calculation for zero rank", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:     "route-123",
			Period:      "day",
			PeriodStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			OverallRank: 0,
		}

		err := fra.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, 0.0, fra.PercentileScore)
	})

	t.Run("sets TTL to 1 year from creation", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:     "route-123",
			Period:      "month",
			PeriodStart: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		}

		before := time.Now()
		err := fra.BeforeCreate()

		require.NoError(t, err)
		expectedTTL := before.AddDate(1, 0, 0).Unix()
		assert.InDelta(t, expectedTTL, fra.TTL, 5)
	})

	t.Run("calls UpdateKeys", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:     "route-key-test",
			Period:      "week",
			PeriodStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		}

		err := fra.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, "FED_ROUTE_AGG#week#route-key-test", fra.PK)
		assert.Equal(t, "20240615", fra.SK)
	})
}

// =============================================================================
// FederationRouteAggregation - BeforeUpdate Tests
// =============================================================================

func TestFederationRouteAggregation_BeforeUpdate(t *testing.T) {
	t.Run("updates UpdatedAt timestamp", func(t *testing.T) {
		oldTime := time.Now().Add(-time.Hour)
		fra := &FederationRouteAggregation{
			RouteID:     "route-123",
			Period:      "day",
			PeriodStart: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   oldTime,
		}

		before := time.Now()
		err := fra.BeforeUpdate()

		require.NoError(t, err)
		assert.WithinDuration(t, before, fra.UpdatedAt, 2*time.Second)
		assert.True(t, fra.UpdatedAt.After(oldTime))
	})

	t.Run("updates keys", func(t *testing.T) {
		fra := &FederationRouteAggregation{
			RouteID:        "route-updated",
			Period:         "month",
			PeriodStart:    time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
			AvgHealthScore: 0.95,
		}

		err := fra.BeforeUpdate()

		require.NoError(t, err)
		assert.Equal(t, "FED_ROUTE_AGG#month#route-updated", fra.PK)
		assert.Equal(t, "FED_ROUTE_COMPARE#month", fra.GSI1PK)
	})
}

// =============================================================================
// FederationRouteAggregation - TableName Test
// =============================================================================

func TestFederationRouteAggregation_TableName(t *testing.T) {
	fra := FederationRouteAggregation{}
	assert.Equal(t, MainTableName, fra.TableName())
}

// =============================================================================
// FederationRouteAggregation - GetRouteComparisonMetrics Tests
// =============================================================================

func TestFederationRouteAggregation_GetRouteComparisonMetrics(t *testing.T) {
	fra := &FederationRouteAggregation{
		RouteID:             "route-comparison-test",
		DestinationDomain:   "example.com",
		AvgHealthScore:      0.88,
		OverallSuccessRate:  97.5,
		AvgLatencyMs:        120,
		CostEfficiencyScore: 0.82,
		UptimePercentage:    99.9,
		OverallRank:         5,
		PercentileScore:     95.0,
		PerformanceTrend:    "improving",
		MTBF:                168.5,
		MTTR:                5.2,
	}

	metrics := fra.GetRouteComparisonMetrics()

	assert.Equal(t, "route-comparison-test", metrics["route_id"])
	assert.Equal(t, "example.com", metrics["destination_domain"])
	assert.Equal(t, 0.88, metrics["avg_health_score"])
	assert.Equal(t, 97.5, metrics["overall_success_rate"])
	assert.Equal(t, int64(120), metrics["avg_latency_ms"])
	assert.Equal(t, 0.82, metrics["cost_efficiency"])
	assert.Equal(t, 99.9, metrics["uptime_percentage"])
	assert.Equal(t, 5, metrics["overall_rank"])
	assert.Equal(t, 95.0, metrics["percentile_score"])
	assert.Equal(t, "improving", metrics["performance_trend"])
	assert.Equal(t, 168.5, metrics["mtbf_hours"])
	assert.Equal(t, 5.2, metrics["mttr_minutes"])
}

// =============================================================================
// ErrorFrequency - TableName Test
// =============================================================================

func TestErrorFrequency_TableName(t *testing.T) {
	ef := ErrorFrequency{}
	assert.Equal(t, MainTableName, ef.TableName())
}
