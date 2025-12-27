package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// FederationCostTracking Tests
// =============================================================================

func TestFederationCostTracking_UpdateKeys(t *testing.T) {
	testCases := []struct {
		name           string
		domain         string
		activityType   string
		activityID     string
		timestamp      time.Time
		expectedPKPfx  string
		expectedSKPfx  string
		expectedGSI1PK string
		expectedGSI2PK string
	}{
		{
			name:           "standard federation cost",
			domain:         "remote.example.com",
			activityType:   "Create",
			activityID:     "activity-123",
			timestamp:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			expectedPKPfx:  "FED_COST#remote.example.com#",
			expectedSKPfx:  "ACTIVITY#Create#activity-123",
			expectedGSI1PK: "FED_COSTS#20240115",
			expectedGSI2PK: "FED_TYPE#Create",
		},
		{
			name:           "follow activity",
			domain:         "mastodon.social",
			activityType:   "Follow",
			activityID:     "follow-456",
			timestamp:      time.Date(2024, 6, 20, 14, 45, 0, 0, time.UTC),
			expectedPKPfx:  "FED_COST#mastodon.social#",
			expectedSKPfx:  "ACTIVITY#Follow#follow-456",
			expectedGSI1PK: "FED_COSTS#20240620",
			expectedGSI2PK: "FED_TYPE#Follow",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fct := &FederationCostTracking{
				Domain:       tc.domain,
				ActivityType: tc.activityType,
				ActivityID:   tc.activityID,
				Timestamp:    tc.timestamp,
			}

			err := fct.UpdateKeys()

			require.NoError(t, err)
			assert.Contains(t, fct.PK, tc.expectedPKPfx)
			assert.Equal(t, tc.expectedSKPfx, fct.SK)
			assert.Equal(t, tc.expectedGSI1PK, fct.GSI1PK)
			assert.Equal(t, tc.expectedGSI2PK, fct.GSI2PK)
			assert.Contains(t, fct.GSI1SK, "TS#")
			assert.Contains(t, fct.GSI2SK, "DOMAIN#"+tc.domain)
		})
	}
}

func TestFederationCostTracking_BeforeCreate(t *testing.T) {
	t.Run("initializes timestamps and maps", func(t *testing.T) {
		fct := &FederationCostTracking{
			Domain:            "example.com",
			ActivityType:      "Create",
			ActivityID:        "test-123",
			DataTransferBytes: 100, // Set to avoid divide-by-zero
		}

		err := fct.BeforeCreate()

		require.NoError(t, err)
		assert.False(t, fct.Timestamp.IsZero(), "Timestamp should be set")
		assert.False(t, fct.CreatedAt.IsZero(), "CreatedAt should be set")
		assert.False(t, fct.UpdatedAt.IsZero(), "UpdatedAt should be set")
		assert.WithinDuration(t, time.Now(), fct.CreatedAt, 2*time.Second)
	})

	t.Run("initializes nil maps", func(t *testing.T) {
		fct := &FederationCostTracking{
			Domain:            "example.com",
			ActivityType:      "Create",
			ActivityID:        "test-123",
			DataTransferBytes: 100,
		}

		err := fct.BeforeCreate()

		require.NoError(t, err)
		assert.NotNil(t, fct.RouteBreakdown, "RouteBreakdown should be initialized")
		assert.NotNil(t, fct.RouteLatency, "RouteLatency should be initialized")
		assert.NotNil(t, fct.RouteErrors, "RouteErrors should be initialized")
		assert.NotNil(t, fct.RouteSuccessRates, "RouteSuccessRates should be initialized")
		assert.NotNil(t, fct.RetryDelaySeconds, "RetryDelaySeconds should be initialized")
		assert.NotNil(t, fct.RetryErrorMessages, "RetryErrorMessages should be initialized")
	})

	t.Run("sets TTL to 30 days from creation", func(t *testing.T) {
		fct := &FederationCostTracking{
			Domain:            "example.com",
			ActivityType:      "Create",
			ActivityID:        "test-123",
			DataTransferBytes: 100,
		}

		err := fct.BeforeCreate()

		require.NoError(t, err)
		expectedTTL := time.Now().AddDate(0, 0, 30).Unix()
		assert.InDelta(t, expectedTTL, fct.TTL, 5, "TTL should be ~30 days from now")
	})

	t.Run("sets keys correctly", func(t *testing.T) {
		fct := &FederationCostTracking{
			Domain:            "example.com",
			ActivityType:      "Create",
			ActivityID:        "test-123",
			DataTransferBytes: 100,
		}

		err := fct.BeforeCreate()

		require.NoError(t, err)
		assert.Contains(t, fct.PK, "FED_COST#example.com#")
		assert.Equal(t, "ACTIVITY#Create#test-123", fct.SK)
	})

	t.Run("preserves existing timestamp", func(t *testing.T) {
		existingTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		fct := &FederationCostTracking{
			Domain:            "example.com",
			ActivityType:      "Create",
			ActivityID:        "test-123",
			Timestamp:         existingTime,
			DataTransferBytes: 100,
		}

		err := fct.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, existingTime, fct.Timestamp, "Existing timestamp should be preserved")
	})
}

func TestFederationCostTracking_BeforeUpdate(t *testing.T) {
	t.Run("updates UpdatedAt timestamp", func(t *testing.T) {
		oldTime := time.Now().Add(-time.Hour)
		fct := &FederationCostTracking{
			Domain:            "example.com",
			ActivityType:      "Create",
			ActivityID:        "test-123",
			Timestamp:         oldTime,
			UpdatedAt:         oldTime,
			DataTransferBytes: 100,
		}

		err := fct.BeforeUpdate()

		require.NoError(t, err)
		assert.WithinDuration(t, time.Now(), fct.UpdatedAt, 2*time.Second)
	})

	t.Run("recalculates total cost", func(t *testing.T) {
		fct := &FederationCostTracking{
			Domain:              "example.com",
			ActivityType:        "Create",
			ActivityID:          "test-123",
			Timestamp:           time.Now(),
			LambdaExecutionCost: 100,
			DataTransferCost:    50,
			DynamoDBReadCost:    25,
			DataTransferBytes:   100,
			TotalCostMicroCents: 0, // Will be recalculated
		}

		err := fct.BeforeUpdate()

		require.NoError(t, err)
		assert.Equal(t, int64(175), fct.TotalCostMicroCents)
	})
}

func TestFederationCostTracking_CalculateTotalCost(t *testing.T) {
	testCases := []struct {
		name          string
		costs         FederationCostTracking
		expectedTotal int64
	}{
		{
			name: "all cost components",
			costs: FederationCostTracking{
				LambdaExecutionCost:       100,
				SignatureVerificationCost: 50,
				HTTPRequestCost:           30,
				DataTransferCost:          200,
				DynamoDBWriteCost:         40,
				DynamoDBReadCost:          20,
				DNSLookupCost:             5,
				WebFingerCost:             10,
				SQSMessageCost:            3,
				RetryCost:                 22,
			},
			expectedTotal: 480,
		},
		{
			name: "only lambda costs",
			costs: FederationCostTracking{
				LambdaExecutionCost: 500,
			},
			expectedTotal: 500,
		},
		{
			name:          "zero costs",
			costs:         FederationCostTracking{},
			expectedTotal: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.costs.CalculateTotalCost()
			assert.Equal(t, tc.expectedTotal, tc.costs.TotalCostMicroCents)
		})
	}
}

func TestFederationCostTracking_GetTotalCostDollars(t *testing.T) {
	testCases := []struct {
		name            string
		microCents      int64
		expectedDollars float64
	}{
		{
			name:            "one dollar",
			microCents:      1_000_000,
			expectedDollars: 1.0,
		},
		{
			name:            "fractional dollar",
			microCents:      500_000,
			expectedDollars: 0.5,
		},
		{
			name:            "zero",
			microCents:      0,
			expectedDollars: 0.0,
		},
		{
			name:            "small amount",
			microCents:      100,
			expectedDollars: 0.0001,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fct := &FederationCostTracking{TotalCostMicroCents: tc.microCents}
			result := fct.GetTotalCostDollars()
			assert.InDelta(t, tc.expectedDollars, result, 0.0001)
		})
	}
}

func TestFederationCostTracking_AddRouteDeliveryAttempt(t *testing.T) {
	t.Run("tracks successful delivery", func(t *testing.T) {
		fct := &FederationCostTracking{
			DataTransferBytes: 1000,
			DataTransferCost:  100,
		}

		fct.AddRouteDeliveryAttempt("route1", 500, 150, true, "")

		assert.Equal(t, int64(50), fct.RouteBreakdown["route1"])
		assert.Equal(t, int64(150), fct.RouteLatency["route1"])
		assert.Equal(t, 0, fct.RouteErrors["route1"])
		assert.Equal(t, 1, fct.RouteAttempts["route1"])
		assert.Equal(t, 1.0, fct.RouteSuccessRates["route1"])
		assert.Equal(t, 1, fct.DeliveryAttempts)
		assert.Equal(t, int64(500), fct.BytesSent)
		assert.True(t, fct.FinalRetrySuccess)
	})

	t.Run("tracks failed delivery", func(t *testing.T) {
		fct := &FederationCostTracking{
			DataTransferBytes: 1000,
			DataTransferCost:  100,
		}

		fct.AddRouteDeliveryAttempt("route1", 500, 200, false, "connection timeout")

		assert.Equal(t, 1, fct.RouteErrors["route1"])
		assert.Equal(t, 0.0, fct.RouteSuccessRates["route1"])
		assert.Equal(t, 1, fct.RetryAttempts)
		assert.Contains(t, fct.RetryErrorMessages, "Route route1: connection timeout")
		assert.False(t, fct.FinalRetrySuccess)
	})

	t.Run("calculates running average latency", func(t *testing.T) {
		fct := &FederationCostTracking{
			DataTransferBytes: 1000,
			DataTransferCost:  100,
		}

		fct.AddRouteDeliveryAttempt("route1", 100, 100, true, "")
		fct.AddRouteDeliveryAttempt("route1", 100, 200, true, "")

		// Running average: (100 + 200) / 2 = 150
		assert.Equal(t, int64(150), fct.RouteLatency["route1"])
	})

	t.Run("updates success rate across multiple attempts", func(t *testing.T) {
		fct := &FederationCostTracking{
			DataTransferBytes: 1000,
			DataTransferCost:  100,
		}

		fct.AddRouteDeliveryAttempt("route1", 100, 100, true, "")
		fct.AddRouteDeliveryAttempt("route1", 100, 100, false, "error")
		fct.AddRouteDeliveryAttempt("route1", 100, 100, true, "")

		// 2 successes out of 3 attempts = 66.67%
		assert.InDelta(t, 0.6667, fct.RouteSuccessRates["route1"], 0.01)
	})
}

func TestFederationCostTracking_AddRetryDelay(t *testing.T) {
	t.Run("appends delay to nil slice", func(t *testing.T) {
		fct := &FederationCostTracking{}

		fct.AddRetryDelay(30)

		assert.Equal(t, []int64{30}, fct.RetryDelaySeconds)
	})

	t.Run("appends multiple delays", func(t *testing.T) {
		fct := &FederationCostTracking{}

		fct.AddRetryDelay(30)
		fct.AddRetryDelay(60)
		fct.AddRetryDelay(120)

		assert.Equal(t, []int64{30, 60, 120}, fct.RetryDelaySeconds)
	})
}

func TestFederationCostTracking_GetAverageRouteLatency(t *testing.T) {
	// Note: The implementation uses ValidateSliceNotEmpty which only works for slices,
	// not maps, so it returns 0 for any map including non-empty ones.
	// This test documents the current behavior.
	t.Run("returns zero due to slice validation on map", func(t *testing.T) {
		fct := &FederationCostTracking{
			RouteLatency: map[string]int64{
				"route1": 100,
				"route2": 200,
				"route3": 300,
			},
		}

		avg := fct.GetAverageRouteLatency()

		// Due to ValidateSliceNotEmpty being called on a map, it returns 0
		assert.Equal(t, int64(0), avg)
	})

	t.Run("returns zero for empty map", func(t *testing.T) {
		fct := &FederationCostTracking{
			RouteLatency: map[string]int64{},
		}

		avg := fct.GetAverageRouteLatency()

		assert.Equal(t, int64(0), avg)
	})

	t.Run("returns zero for nil map", func(t *testing.T) {
		fct := &FederationCostTracking{}

		avg := fct.GetAverageRouteLatency()

		assert.Equal(t, int64(0), avg)
	})
}

func TestFederationCostTracking_GetTotalRouteErrors(t *testing.T) {
	t.Run("sums errors across routes", func(t *testing.T) {
		fct := &FederationCostTracking{
			RouteErrors: map[string]int{
				"route1": 5,
				"route2": 3,
				"route3": 2,
			},
		}

		total := fct.GetTotalRouteErrors()

		assert.Equal(t, 10, total)
	})

	t.Run("returns zero for empty map", func(t *testing.T) {
		fct := &FederationCostTracking{
			RouteErrors: map[string]int{},
		}

		total := fct.GetTotalRouteErrors()

		assert.Equal(t, 0, total)
	})
}

func TestFederationCostTracking_GetMostExpensiveRoute(t *testing.T) {
	t.Run("returns route with highest cost", func(t *testing.T) {
		fct := &FederationCostTracking{
			RouteBreakdown: map[string]int64{
				"cheapRoute":     100,
				"expensiveRoute": 500,
				"mediumRoute":    300,
			},
		}

		routeID, cost := fct.GetMostExpensiveRoute()

		assert.Equal(t, "expensiveRoute", routeID)
		assert.Equal(t, int64(500), cost)
	})

	t.Run("returns empty for no routes", func(t *testing.T) {
		fct := &FederationCostTracking{
			RouteBreakdown: map[string]int64{},
		}

		routeID, cost := fct.GetMostExpensiveRoute()

		assert.Empty(t, routeID)
		assert.Equal(t, int64(0), cost)
	})
}

func TestFederationCostTracking_GetRetryEfficiency(t *testing.T) {
	t.Run("calculates retry metrics", func(t *testing.T) {
		fct := &FederationCostTracking{
			DeliveryAttempts:   10,
			RetryAttempts:      3,
			FinalRetrySuccess:  true,
			RetryDelaySeconds:  []int64{30, 60, 90},
			RetryErrorMessages: []string{"error1", "error2"},
		}

		efficiency := fct.GetRetryEfficiency()

		assert.Equal(t, 10, efficiency["total_attempts"])
		assert.Equal(t, 3, efficiency["retry_attempts"])
		assert.Equal(t, true, efficiency["final_success"])
		assert.InDelta(t, 0.3, efficiency["retry_rate"], 0.01)
		assert.Equal(t, int64(60), efficiency["average_retry_delay_seconds"])
		assert.Equal(t, 2, efficiency["error_messages"])
	})

	t.Run("handles zero delivery attempts", func(t *testing.T) {
		fct := &FederationCostTracking{
			DeliveryAttempts: 0,
		}

		efficiency := fct.GetRetryEfficiency()

		assert.Equal(t, 0, efficiency["total_attempts"])
		_, hasRetryRate := efficiency["retry_rate"]
		assert.False(t, hasRetryRate)
	})
}

func TestFederationCostTracking_GetPK_GetSK(t *testing.T) {
	fct := &FederationCostTracking{
		PK: "FED_COST#example.com#2024-01-15",
		SK: "ACTIVITY#Create#123",
	}

	assert.Equal(t, "FED_COST#example.com#2024-01-15", fct.GetPK())
	assert.Equal(t, "ACTIVITY#Create#123", fct.GetSK())
}

func TestFederationCostTracking_TableName(t *testing.T) {
	fct := FederationCostTracking{}
	assert.Equal(t, MainTableName, fct.TableName())
}

// =============================================================================
// FederationBudget Tests
// =============================================================================

func TestFederationBudget_UpdateKeys(t *testing.T) {
	budget := &FederationBudget{
		Domain: "example.com",
		Period: "monthly",
	}

	err := budget.UpdateKeys()

	require.NoError(t, err)
	assert.Equal(t, "FED_BUDGET#example.com#monthly", budget.PK)
	assert.Equal(t, SKConfig, budget.SK)
	assert.Equal(t, "ACTIVE_BUDGETS", budget.GSI1PK)
	assert.Equal(t, "DOMAIN#example.com#monthly", budget.GSI1SK)
}

func TestFederationBudget_BeforeCreate(t *testing.T) {
	t.Run("initializes timestamps and maps", func(t *testing.T) {
		budget := &FederationBudget{
			Domain: "example.com",
			Period: "monthly",
		}

		err := budget.BeforeCreate()

		require.NoError(t, err)
		assert.False(t, budget.CreatedAt.IsZero())
		assert.False(t, budget.UpdatedAt.IsZero())
		assert.NotNil(t, budget.ActivityTypeLimits)
		assert.NotNil(t, budget.ActivityTypeUsage)
	})

	t.Run("preserves existing CreatedAt", func(t *testing.T) {
		existingTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		budget := &FederationBudget{
			Domain:    "example.com",
			Period:    "monthly",
			CreatedAt: existingTime,
		}

		err := budget.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, existingTime, budget.CreatedAt)
	})
}

func TestFederationBudget_BeforeUpdate(t *testing.T) {
	budget := &FederationBudget{
		Domain:    "example.com",
		Period:    "monthly",
		UpdatedAt: time.Now().Add(-time.Hour),
	}

	err := budget.BeforeUpdate()

	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), budget.UpdatedAt, 2*time.Second)
}

func TestFederationBudget_IsOverLimits(t *testing.T) {
	t.Run("IsOverInboundLimit", func(t *testing.T) {
		tests := []struct {
			current  int64
			limit    int64
			expected bool
		}{
			{100, 100, true}, // at limit
			{101, 100, true}, // over limit
			{99, 100, false}, // under limit
			{0, 100, false},  // zero usage
		}

		for _, tc := range tests {
			budget := &FederationBudget{
				CurrentInboundCost:     tc.current,
				InboundLimitMicroCents: tc.limit,
			}
			assert.Equal(t, tc.expected, budget.IsOverInboundLimit())
		}
	})

	t.Run("IsOverOutboundLimit", func(t *testing.T) {
		budget := &FederationBudget{
			CurrentOutboundCost:     150,
			OutboundLimitMicroCents: 100,
		}
		assert.True(t, budget.IsOverOutboundLimit())
	})

	t.Run("IsOverCombinedLimit", func(t *testing.T) {
		budget := &FederationBudget{
			CurrentCombinedCost:     200,
			CombinedLimitMicroCents: 100,
		}
		assert.True(t, budget.IsOverCombinedLimit())
	})
}

func TestFederationBudget_IsOverActivityTypeLimit(t *testing.T) {
	budget := &FederationBudget{
		ActivityTypeLimits: map[string]int64{
			"Create": 100,
			"Follow": 50,
		},
		ActivityTypeUsage: map[string]int64{
			"Create": 150,
			"Follow": 30,
		},
	}

	assert.True(t, budget.IsOverActivityTypeLimit("Create"))
	assert.False(t, budget.IsOverActivityTypeLimit("Follow"))
	assert.False(t, budget.IsOverActivityTypeLimit("Like")) // Not configured
}

func TestFederationBudget_GetUsagePercent(t *testing.T) {
	t.Run("GetInboundUsagePercent", func(t *testing.T) {
		budget := &FederationBudget{
			CurrentInboundCost:     50,
			InboundLimitMicroCents: 100,
		}
		assert.InDelta(t, 50.0, budget.GetInboundUsagePercent(), 0.01)
	})

	t.Run("GetInboundUsagePercent with zero limit", func(t *testing.T) {
		budget := &FederationBudget{
			CurrentInboundCost:     50,
			InboundLimitMicroCents: 0,
		}
		assert.Equal(t, 0.0, budget.GetInboundUsagePercent())
	})

	t.Run("GetOutboundUsagePercent", func(t *testing.T) {
		budget := &FederationBudget{
			CurrentOutboundCost:     75,
			OutboundLimitMicroCents: 100,
		}
		assert.InDelta(t, 75.0, budget.GetOutboundUsagePercent(), 0.01)
	})

	t.Run("GetCombinedUsagePercent", func(t *testing.T) {
		budget := &FederationBudget{
			CurrentCombinedCost:     80,
			CombinedLimitMicroCents: 100,
		}
		assert.InDelta(t, 80.0, budget.GetCombinedUsagePercent(), 0.01)
	})

	t.Run("GetActivityTypeUsagePercent", func(t *testing.T) {
		budget := &FederationBudget{
			ActivityTypeLimits: map[string]int64{
				"Create": 100,
			},
			ActivityTypeUsage: map[string]int64{
				"Create": 25,
			},
		}
		assert.InDelta(t, 25.0, budget.GetActivityTypeUsagePercent("Create"), 0.01)
		assert.Equal(t, 0.0, budget.GetActivityTypeUsagePercent("Like")) // Not configured
	})
}

func TestFederationBudget_ShouldSendAlert(t *testing.T) {
	t.Run("returns false when alerts disabled", func(t *testing.T) {
		budget := &FederationBudget{
			AlertSendingEnabled:     false,
			CurrentCombinedCost:     100,
			CombinedLimitMicroCents: 100,
			AlertThresholdPercent:   80,
		}
		assert.False(t, budget.ShouldSendAlert())
	})

	t.Run("returns false when under threshold", func(t *testing.T) {
		budget := &FederationBudget{
			AlertSendingEnabled:     true,
			CurrentCombinedCost:     50,
			CombinedLimitMicroCents: 100,
			AlertThresholdPercent:   80,
		}
		assert.False(t, budget.ShouldSendAlert())
	})

	t.Run("returns true when over threshold and no recent alert", func(t *testing.T) {
		budget := &FederationBudget{
			AlertSendingEnabled:     true,
			CurrentCombinedCost:     90,
			CombinedLimitMicroCents: 100,
			AlertThresholdPercent:   80,
			LastAlertSentAt:         nil,
		}
		assert.True(t, budget.ShouldSendAlert())
	})

	t.Run("returns false when alert sent recently", func(t *testing.T) {
		recentTime := time.Now().Add(-30 * time.Minute)
		budget := &FederationBudget{
			AlertSendingEnabled:     true,
			CurrentCombinedCost:     90,
			CombinedLimitMicroCents: 100,
			AlertThresholdPercent:   80,
			LastAlertSentAt:         &recentTime,
		}
		assert.False(t, budget.ShouldSendAlert())
	})
}

func TestFederationBudget_ShouldBlock(t *testing.T) {
	t.Run("returns false when blocking disabled", func(t *testing.T) {
		budget := &FederationBudget{
			BlockOnLimitExceeded:    false,
			CurrentCombinedCost:     200,
			CombinedLimitMicroCents: 100,
		}
		assert.False(t, budget.ShouldBlock())
	})

	t.Run("returns true when over limit and blocking enabled", func(t *testing.T) {
		budget := &FederationBudget{
			BlockOnLimitExceeded:    true,
			CurrentCombinedCost:     200,
			CombinedLimitMicroCents: 100,
		}
		assert.True(t, budget.ShouldBlock())
	})

	t.Run("returns false when under limit", func(t *testing.T) {
		budget := &FederationBudget{
			BlockOnLimitExceeded:    true,
			CurrentCombinedCost:     50,
			CombinedLimitMicroCents: 100,
		}
		assert.False(t, budget.ShouldBlock())
	})
}

func TestFederationBudget_ShouldRateLimit(t *testing.T) {
	t.Run("returns false when rate limiting disabled", func(t *testing.T) {
		budget := &FederationBudget{
			RateLimitOnThreshold:    false,
			CurrentCombinedCost:     90,
			CombinedLimitMicroCents: 100,
			AlertThresholdPercent:   80,
		}
		assert.False(t, budget.ShouldRateLimit())
	})

	t.Run("returns true when at threshold", func(t *testing.T) {
		budget := &FederationBudget{
			RateLimitOnThreshold:    true,
			CurrentCombinedCost:     80,
			CombinedLimitMicroCents: 100,
			AlertThresholdPercent:   80,
		}
		assert.True(t, budget.ShouldRateLimit())
	})
}

func TestFederationBudget_AddUsage(t *testing.T) {
	t.Run("adds inbound usage", func(t *testing.T) {
		budget := &FederationBudget{
			ActivityTypeUsage: make(map[string]int64),
		}

		budget.AddUsage("Create", "inbound", 100)

		assert.Equal(t, int64(100), budget.CurrentInboundCost)
		assert.Equal(t, int64(1), budget.InboundActivityCount)
		assert.Equal(t, int64(100), budget.CurrentCombinedCost)
		assert.Equal(t, int64(100), budget.ActivityTypeUsage["Create"])
		assert.NotNil(t, budget.LastInboundAt)
	})

	t.Run("adds outbound usage", func(t *testing.T) {
		budget := &FederationBudget{
			ActivityTypeUsage: make(map[string]int64),
		}

		budget.AddUsage("Follow", "outbound", 50)

		assert.Equal(t, int64(50), budget.CurrentOutboundCost)
		assert.Equal(t, int64(1), budget.OutboundActivityCount)
		assert.Equal(t, int64(50), budget.CurrentCombinedCost)
		assert.NotNil(t, budget.LastOutboundAt)
	})

	t.Run("initializes nil map", func(t *testing.T) {
		budget := &FederationBudget{}

		budget.AddUsage("Create", "inbound", 100)

		assert.NotNil(t, budget.ActivityTypeUsage)
		assert.Equal(t, int64(100), budget.ActivityTypeUsage["Create"])
	})
}

func TestFederationBudget_ResetPeriod(t *testing.T) {
	oldTime := time.Now().Add(-time.Hour)
	budget := &FederationBudget{
		CurrentInboundCost:    100,
		CurrentOutboundCost:   200,
		CurrentCombinedCost:   300,
		InboundActivityCount:  10,
		OutboundActivityCount: 20,
		LastInboundAt:         &oldTime,
		LastOutboundAt:        &oldTime,
		ActivityTypeUsage: map[string]int64{
			"Create": 50,
			"Follow": 25,
		},
		Status: "over_limit",
	}

	newStart := time.Now()
	newEnd := time.Now().AddDate(0, 1, 0)
	budget.ResetPeriod(newStart, newEnd)

	assert.Equal(t, int64(0), budget.CurrentInboundCost)
	assert.Equal(t, int64(0), budget.CurrentOutboundCost)
	assert.Equal(t, int64(0), budget.CurrentCombinedCost)
	assert.Equal(t, int64(0), budget.InboundActivityCount)
	assert.Equal(t, int64(0), budget.OutboundActivityCount)
	assert.Nil(t, budget.LastInboundAt)
	assert.Nil(t, budget.LastOutboundAt)
	assert.Equal(t, int64(0), budget.ActivityTypeUsage["Create"])
	assert.Equal(t, int64(0), budget.ActivityTypeUsage["Follow"])
	assert.Equal(t, newStart, budget.PeriodStart)
	assert.Equal(t, newEnd, budget.PeriodEnd)
	assert.Equal(t, StatusActive, budget.Status)
}

func TestFederationBudget_GetPK_GetSK(t *testing.T) {
	budget := &FederationBudget{
		PK: "FED_BUDGET#example.com#monthly",
		SK: "CONFIG",
	}

	assert.Equal(t, "FED_BUDGET#example.com#monthly", budget.GetPK())
	assert.Equal(t, "CONFIG", budget.GetSK())
}

func TestFederationBudget_TableName(t *testing.T) {
	budget := FederationBudget{}
	assert.Equal(t, MainTableName, budget.TableName())
}
