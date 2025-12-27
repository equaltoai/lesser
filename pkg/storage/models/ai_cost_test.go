package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixed timestamp for deterministic key generation
var aiCostFixedTime = time.Date(2024, 6, 15, 10, 30, 45, 0, time.UTC)

// TestAICost_CalculateTotalCost tests total cost calculation
func TestAICost_CalculateTotalCost(t *testing.T) {
	t.Run("sums all cost components", func(t *testing.T) {
		a := &AICost{
			InputTokenCost:        100,
			OutputTokenCost:       200,
			ModelInferenceCost:    50,
			ComplexityPenaltyCost: 25,
			RetryCost:             10,
			InputTokens:           1000,
			OutputTokens:          500,
			InputCharacters:       5000,
		}
		a.CalculateTotalCost()

		expectedTotal := int64(100 + 200 + 50 + 25 + 10)
		assert.Equal(t, expectedTotal, a.TotalCostMicroCents)
	})

	t.Run("calculates CostPerInputToken", func(t *testing.T) {
		a := &AICost{
			InputTokenCost: 1000000, // 1 million microcents
			InputTokens:    1000,
		}
		a.CalculateTotalCost()

		// Expected: 1000000 / 1000 / 1_000_000 = 0.001 dollars per token
		assert.InDelta(t, 0.001, a.CostPerInputToken, 0.0001)
	})

	t.Run("calculates CostPerOutputToken", func(t *testing.T) {
		a := &AICost{
			OutputTokenCost: 500000, // 500k microcents
			OutputTokens:    500,
		}
		a.CalculateTotalCost()

		// Expected: 500000 / 500 / 1_000_000 = 0.001 dollars per token
		assert.InDelta(t, 0.001, a.CostPerOutputToken, 0.0001)
	})

	t.Run("calculates CostPerCharacter", func(t *testing.T) {
		a := &AICost{
			InputTokenCost:  500000,
			InputCharacters: 10000,
		}
		a.CalculateTotalCost()

		// Expected: 500000 / 10000 / 1_000_000 = 0.00005
		assert.InDelta(t, 0.00005, a.CostPerCharacter, 0.00001)
	})

	t.Run("handles zero tokens gracefully", func(t *testing.T) {
		a := &AICost{
			InputTokenCost:  100,
			OutputTokenCost: 200,
		}
		a.CalculateTotalCost()

		assert.Equal(t, float64(0), a.CostPerInputToken)
		assert.Equal(t, float64(0), a.CostPerOutputToken)
	})
}

// TestAICost_determineCostTier tests tier boundary logic
func TestAICost_determineCostTier(t *testing.T) {
	testCases := []struct {
		name                string
		totalCostMicroCents int64
		expectedTier        string
	}{
		// >= $1.00 = premium (1,000,000 microcents)
		{"$1.00 exactly - premium", 1000000, CostTierPremium},
		{"$5.00 - premium", 5000000, CostTierPremium},

		// >= $0.10 = high (100,000 microcents)
		{"$0.10 exactly - high", 100000, CostTierHigh},
		{"$0.50 - high", 500000, CostTierHigh},
		{"$0.99 - high", 990000, CostTierHigh},

		// >= $0.01 = medium (10,000 microcents)
		{"$0.01 exactly - medium", 10000, "medium"},
		{"$0.05 - medium", 50000, "medium"},
		{"$0.099 - medium", 99000, "medium"},

		// < $0.01 = low
		{"$0.009 - low", 9000, CostTierLow},
		{"$0.001 - low", 1000, CostTierLow},
		{"$0.00 - low", 0, CostTierLow},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := &AICost{
				TotalCostMicroCents: tc.totalCostMicroCents,
			}
			a.determineCostTier()
			assert.Equal(t, tc.expectedTier, a.CostTier)
		})
	}
}

// TestAICost_GetTotalCostDollars tests dollar conversion
func TestAICost_GetTotalCostDollars(t *testing.T) {
	testCases := []struct {
		name            string
		microCents      int64
		expectedDollars float64
	}{
		{"1 million microcents = $1.00", 1000000, 1.0},
		{"500k microcents = $0.50", 500000, 0.5},
		{"100k microcents = $0.10", 100000, 0.1},
		{"1k microcents = $0.001", 1000, 0.001},
		{"zero", 0, 0.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := &AICost{TotalCostMicroCents: tc.microCents}
			assert.InDelta(t, tc.expectedDollars, a.GetTotalCostDollars(), 0.0001)
		})
	}
}

// TestAICost_calculateEfficiencyMetrics tests efficiency calculation
func TestAICost_calculateEfficiencyMetrics(t *testing.T) {
	t.Run("calculates TokensPerSecond", func(t *testing.T) {
		start := aiCostFixedTime
		end := aiCostFixedTime.Add(10 * time.Second)

		a := &AICost{
			OutputTokens:    100,
			ProcessingStart: start,
			ProcessingEnd:   end,
		}
		a.calculateEfficiencyMetrics()

		// 100 tokens / 10 seconds = 10 tokens/sec
		assert.InDelta(t, 10.0, a.TokensPerSecond, 0.1)
	})

	t.Run("calculates EfficiencyScore", func(t *testing.T) {
		a := &AICost{
			CostPerOutputToken: 0.001,
			QualityScore:       0.8,
			RelevanceScore:     0.9,
			ComplexityScore:    0.5,
		}
		a.calculateEfficiencyMetrics()

		// (Quality + Relevance) / (CostPerToken * (1 + Complexity))
		// = (0.8 + 0.9) / (0.001 * 1.5)
		// = 1.7 / 0.0015 = 1133.33
		assert.InDelta(t, 1133.33, a.EfficiencyScore, 1.0)
	})

	t.Run("handles zero cost gracefully", func(t *testing.T) {
		a := &AICost{
			CostPerOutputToken: 0,
			QualityScore:       0.8,
			RelevanceScore:     0.9,
		}
		a.calculateEfficiencyMetrics()
		assert.Equal(t, float64(0), a.EfficiencyScore)
	})
}

// TestAICost_SetModelPricing tests model-specific pricing
func TestAICost_SetModelPricing(t *testing.T) {
	testCases := []struct {
		name               string
		modelName          string
		inputTokens        int64
		outputTokens       int64
		expectedInputCost  int64
		expectedOutputCost int64
	}{
		{
			name:               "claude-3-haiku",
			modelName:          "claude-3-haiku",
			inputTokens:        1000,
			outputTokens:       500,
			expectedInputCost:  1000 * 25, // 25,000 microcents
			expectedOutputCost: 500 * 125, // 62,500 microcents
		},
		{
			name:               "claude-3-sonnet",
			modelName:          "claude-3-sonnet",
			inputTokens:        1000,
			outputTokens:       500,
			expectedInputCost:  1000 * 300, // 300,000 microcents
			expectedOutputCost: 500 * 1500, // 750,000 microcents
		},
		{
			name:               "claude-3-opus",
			modelName:          "claude-3-opus",
			inputTokens:        1000,
			outputTokens:       500,
			expectedInputCost:  1000 * 1500, // 1,500,000 microcents
			expectedOutputCost: 500 * 7500,  // 3,750,000 microcents
		},
		{
			name:               "titan-text-express",
			modelName:          "titan-text-express",
			inputTokens:        1000,
			outputTokens:       500,
			expectedInputCost:  1000 * 13, // 13,000 microcents
			expectedOutputCost: 500 * 17,  // 8,500 microcents
		},
		{
			name:               "unknown model defaults to haiku",
			modelName:          "unknown-model",
			inputTokens:        1000,
			outputTokens:       500,
			expectedInputCost:  1000 * 25,
			expectedOutputCost: 500 * 125,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := &AICost{
				ModelName:    tc.modelName,
				InputTokens:  tc.inputTokens,
				OutputTokens: tc.outputTokens,
			}
			a.SetModelPricing()

			assert.Equal(t, tc.expectedInputCost, a.InputTokenCost)
			assert.Equal(t, tc.expectedOutputCost, a.OutputTokenCost)
			assert.Equal(t, int64(100), a.ModelInferenceCost, "base inference cost should be 100")
		})
	}

	t.Run("applies complexity penalty", func(t *testing.T) {
		a := &AICost{
			ModelName:       "claude-3-haiku",
			InputTokens:     1000,
			OutputTokens:    500,
			ComplexityScore: 0.5, // 50% complexity
		}
		a.SetModelPricing()

		// complexityMultiplier = 1 + 0.5 = 1.5
		// penalty = (25000 + 62500) * 0.5 = 43750
		baseCost := int64(1000*25 + 500*125)
		expectedPenalty := int64(float64(baseCost) * 0.5)
		assert.Equal(t, expectedPenalty, a.ComplexityPenaltyCost)
	})
}

// TestAICost_BeforeCreate tests lifecycle hook
func TestAICost_BeforeCreate(t *testing.T) {
	t.Run("sets timestamps", func(t *testing.T) {
		a := &AICost{
			OperationID:   "op-123",
			OperationType: "sentiment_analysis",
		}
		before := time.Now()
		err := a.BeforeCreate()
		require.NoError(t, err)

		assert.WithinDuration(t, before, a.CreatedAt, time.Second)
		assert.WithinDuration(t, before, a.UpdatedAt, time.Second)
	})

	t.Run("sets billing period", func(t *testing.T) {
		a := &AICost{
			OperationID:   "op-123",
			OperationType: "sentiment_analysis",
			Timestamp:     aiCostFixedTime,
		}
		err := a.BeforeCreate()
		require.NoError(t, err)

		assert.Equal(t, "2024-06", a.BillingPeriod)
	})

	t.Run("TTL 90 days", func(t *testing.T) {
		a := &AICost{
			OperationID:   "op-123",
			OperationType: "sentiment_analysis",
		}
		before := time.Now()
		err := a.BeforeCreate()
		require.NoError(t, err)

		expectedExpiry := before.AddDate(0, 0, 90)
		actualExpiry := time.Unix(a.TTL, 0)
		assert.WithinDuration(t, expectedExpiry, actualExpiry, 2*time.Second)
	})

	t.Run("calculates TotalTokens", func(t *testing.T) {
		a := &AICost{
			OperationID:   "op-123",
			OperationType: "sentiment_analysis",
			InputTokens:   100,
			OutputTokens:  50,
		}
		err := a.BeforeCreate()
		require.NoError(t, err)

		assert.Equal(t, int64(150), a.TotalTokens)
	})

	t.Run("calculates CharactersPerToken", func(t *testing.T) {
		a := &AICost{
			OperationID:     "op-123",
			OperationType:   "sentiment_analysis",
			InputTokens:     100,
			InputCharacters: 400,
		}
		err := a.BeforeCreate()
		require.NoError(t, err)

		assert.InDelta(t, 4.0, a.CharactersPerToken, 0.1)
	})

	t.Run("initializes slices and maps", func(t *testing.T) {
		a := &AICost{
			OperationID:   "op-123",
			OperationType: "sentiment_analysis",
		}
		err := a.BeforeCreate()
		require.NoError(t, err)

		assert.NotNil(t, a.ComplexityFactors)
		assert.NotNil(t, a.ModelConfig)
		assert.NotNil(t, a.OperationContext)
	})
}

// TestAICost_AddComplexityFactor tests deduplication
func TestAICost_AddComplexityFactor(t *testing.T) {
	t.Run("adds factors", func(t *testing.T) {
		a := &AICost{}
		a.AddComplexityFactor("long_input")
		a.AddComplexityFactor("multiple_languages")

		assert.Len(t, a.ComplexityFactors, 2)
		assert.Contains(t, a.ComplexityFactors, "long_input")
		assert.Contains(t, a.ComplexityFactors, "multiple_languages")
	})

	t.Run("deduplicates factors", func(t *testing.T) {
		a := &AICost{}
		a.AddComplexityFactor("long_input")
		a.AddComplexityFactor("long_input")
		a.AddComplexityFactor("long_input")

		assert.Len(t, a.ComplexityFactors, 1)
	})
}

// TestAICost_UpdateKeys tests key generation
func TestAICost_UpdateKeys(t *testing.T) {
	t.Run("PK format", func(t *testing.T) {
		a := &AICost{
			OperationID:   "op-123",
			OperationType: "sentiment_analysis",
			Timestamp:     aiCostFixedTime,
		}
		err := a.UpdateKeys()
		require.NoError(t, err)

		assert.Equal(t, "AI_COST#op-123", a.PK)
	})

	t.Run("GSI1 time-based queries", func(t *testing.T) {
		a := &AICost{
			OperationID:   "op-123",
			OperationType: "sentiment_analysis",
			Timestamp:     aiCostFixedTime,
		}
		err := a.UpdateKeys()
		require.NoError(t, err)

		assert.Equal(t, "AI_COSTS#20240615", a.GSI1PK)
		assert.Contains(t, a.GSI1SK, "TS#")
		assert.Contains(t, a.GSI1SK, "op-123")
	})

	t.Run("GSI2 operation type queries", func(t *testing.T) {
		a := &AICost{
			OperationID:   "op-123",
			OperationType: "content_moderation",
			ModelName:     "claude-3-haiku",
			Timestamp:     aiCostFixedTime,
		}
		err := a.UpdateKeys()
		require.NoError(t, err)

		assert.Equal(t, "AI_TYPE#content_moderation", a.GSI2PK)
		assert.Contains(t, a.GSI2SK, "MODEL#claude-3-haiku")
	})

	t.Run("GSI3 cost tier queries", func(t *testing.T) {
		a := &AICost{
			OperationID:         "op-123",
			OperationType:       "sentiment_analysis",
			Timestamp:           aiCostFixedTime,
			TotalCostMicroCents: 500000, // $0.50 = high tier
			CostTier:            CostTierHigh,
		}
		err := a.UpdateKeys()
		require.NoError(t, err)

		assert.Equal(t, "AI_COST_RANGE#high", a.GSI3PK)
		assert.Contains(t, a.GSI3SK, "COST#")
	})
}

// TestAICost_GetOperationSummary tests summary generation
func TestAICost_GetOperationSummary(t *testing.T) {
	a := &AICost{
		OperationID:         "op-123",
		OperationType:       "sentiment_analysis",
		ModelName:           "claude-3-haiku",
		InputTokens:         100,
		OutputTokens:        50,
		TotalCostMicroCents: 10000,
		CostTier:            CostTierLow,
		ComplexityScore:     0.3,
		EfficiencyScore:     500.0,
		RequestLatencyMs:    150,
		Success:             true,
		Timestamp:           aiCostFixedTime,
	}

	summary := a.GetOperationSummary()

	assert.Equal(t, "op-123", summary["operation_id"])
	assert.Equal(t, "sentiment_analysis", summary["operation_type"])
	assert.Equal(t, "claude-3-haiku", summary["model_name"])
	assert.Equal(t, int64(100), summary["input_tokens"])
	assert.Equal(t, int64(50), summary["output_tokens"])
	assert.Equal(t, true, summary["success"])
}

// TestAIAggregatedCost_BeforeCreate tests aggregated cost lifecycle
func TestAIAggregatedCost_BeforeCreate(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)

	t.Run("calculates derived metrics", func(t *testing.T) {
		a := &AIAggregatedCost{
			Period:               "hour",
			PeriodStart:          earlier,
			PeriodEnd:            now,
			OperationType:        "sentiment_analysis",
			ModelName:            "claude-3-haiku",
			TotalOperations:      100,
			SuccessfulOperations: 95,
			TotalInputTokens:     10000,
			TotalOutputTokens:    5000,
			TotalCostMicroCents:  500000,
		}
		err := a.BeforeCreate()
		require.NoError(t, err)

		assert.InDelta(t, 0.95, a.SuccessRate, 0.01)
		assert.InDelta(t, 100.0, a.AvgInputTokens, 0.1)
		assert.InDelta(t, 50.0, a.AvgOutputTokens, 0.1)
		assert.Equal(t, int64(5000), a.AvgCostMicroCents)
		assert.InDelta(t, 0.5, a.TotalCostDollars, 0.01)
	})

	t.Run("TTL 1 year", func(t *testing.T) {
		a := &AIAggregatedCost{
			Period:        "hour",
			PeriodStart:   earlier,
			PeriodEnd:     now,
			OperationType: "sentiment_analysis",
			ModelName:     "claude-3-haiku",
		}
		before := time.Now()
		err := a.BeforeCreate()
		require.NoError(t, err)

		expectedExpiry := before.AddDate(1, 0, 0)
		actualExpiry := time.Unix(a.TTL, 0)
		assert.WithinDuration(t, expectedExpiry, actualExpiry, 2*time.Second)
	})
}
