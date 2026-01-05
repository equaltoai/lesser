package federation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// === CalculateFederationCosts Tests ===

func TestCalculateFederationCosts(t *testing.T) {
	calc := NewCostCalculator()

	t.Run("calculates_lambda_gb_seconds_cost", func(t *testing.T) {
		params := &CostCalculationParams{
			LambdaDurationMs: 1000, // 1 second
			LambdaMemoryMB:   1024, // 1 GB
		}

		result := calc.CalculateFederationCosts(params)

		// 1GB * 1s * 17 microdollars/GB-second = 17 microdollars
		assert.Equal(t, int64(17), result.LambdaExecutionCost)
	})

	t.Run("calculates_signature_verification_cost", func(t *testing.T) {
		params := &CostCalculationParams{
			SignatureVerificationMs: 100, // 100ms
		}

		result := calc.CalculateFederationCosts(params)

		// Base cost (5) + time-based (100 * 0.1 = 10) = 15
		assert.Equal(t, int64(15), result.SignatureVerificationCost)
	})

	t.Run("calculates_http_request_cost", func(t *testing.T) {
		params := &CostCalculationParams{
			HTTPRequestCount: 5,
		}

		result := calc.CalculateFederationCosts(params)

		// 5 requests * 100 microdollars = 500
		assert.Equal(t, int64(500), result.HTTPRequestCost)
	})

	t.Run("calculates_data_transfer_cost_outbound_only", func(t *testing.T) {
		paramsOutbound := &CostCalculationParams{
			Direction:         "outbound",
			DataTransferBytes: 1024 * 1024 * 1024, // 1 GB
		}

		paramsInbound := &CostCalculationParams{
			Direction:         "inbound",
			DataTransferBytes: 1024 * 1024 * 1024, // 1 GB
		}

		resultOutbound := calc.CalculateFederationCosts(paramsOutbound)
		resultInbound := calc.CalculateFederationCosts(paramsInbound)

		// Outbound: 1 GB * 90,000 microdollars = 90,000
		assert.Equal(t, int64(90000), resultOutbound.DataTransferCost)
		// Inbound: Free
		assert.Equal(t, int64(0), resultInbound.DataTransferCost)
	})

	t.Run("calculates_dynamodb_costs", func(t *testing.T) {
		params := &CostCalculationParams{
			DynamoDBWriteCount: 10,
			DynamoDBReadCount:  5,
		}

		result := calc.CalculateFederationCosts(params)

		// Writes: 10 * 1 = 10, Reads: 5 * 0 = 0 (rounded)
		assert.Equal(t, int64(10), result.DynamoDBWriteCost)
		assert.Equal(t, int64(0), result.DynamoDBReadCost)
	})

	t.Run("calculates_retry_cost_exponentially", func(t *testing.T) {
		tests := []struct {
			retryCount   int
			expectedCost int64
		}{
			{0, 0},
			{1, 50},  // 1*50
			{2, 150}, // 1*50 + 2*50
			{3, 300}, // 1*50 + 2*50 + 3*50
			{5, 750}, // 1*50 + 2*50 + 3*50 + 4*50 + 5*50 = 50*(1+2+3+4+5) = 50*15
		}

		for _, tt := range tests {
			t.Run("", func(t *testing.T) {
				params := &CostCalculationParams{
					RetryCount: tt.retryCount,
				}

				result := calc.CalculateFederationCosts(params)
				assert.Equal(t, tt.expectedCost, result.RetryCost)
			})
		}
	})

	t.Run("calculates_compression_ratio", func(t *testing.T) {
		params := &CostCalculationParams{
			PayloadSize:    1000,
			CompressedSize: 250,
		}

		result := calc.CalculateFederationCosts(params)

		assert.InDelta(t, 0.25, result.CompressionRatio, 0.001)
	})

	t.Run("total_cost_equals_sum_of_parts", func(t *testing.T) {
		params := &CostCalculationParams{
			LambdaDurationMs:        500,
			LambdaMemoryMB:          512,
			SignatureVerificationMs: 50,
			HTTPRequestCount:        3,
			Direction:               "outbound",
			DataTransferBytes:       1024 * 1024, // 1 MB
			DynamoDBWriteCount:      2,
			DynamoDBReadCount:       1,
			DNSLookupCount:          1,
			WebFingerCount:          1,
			SQSMessageCount:         2,
			RetryCount:              1,
		}

		result := calc.CalculateFederationCosts(params)

		expectedTotal := result.LambdaExecutionCost +
			result.SignatureVerificationCost +
			result.HTTPRequestCost +
			result.DataTransferCost +
			result.DynamoDBWriteCost +
			result.DynamoDBReadCost +
			result.DNSLookupCost +
			result.WebFingerCost +
			result.SQSMessageCost +
			result.RetryCost

		assert.Equal(t, expectedTotal, result.TotalCostMicroCents)
	})
}

// === EstimateInboundActivityCost Tests ===

func TestEstimateInboundActivityCost(t *testing.T) {
	calc := NewCostCalculator()

	t.Run("monotonic_with_payload_size", func(t *testing.T) {
		// Use significantly different sizes to ensure cost difference
		smallCost := calc.EstimateInboundActivityCost("Create", 1024, false)      // 1KB
		largeCost := calc.EstimateInboundActivityCost("Create", 1024*1024, false) // 1MB - adds ~1000ms duration

		assert.Greater(t, largeCost, smallCost)
	})

	t.Run("signature_verification_increases_cost", func(t *testing.T) {
		withoutSig := calc.EstimateInboundActivityCost("Create", 1024, false)
		withSig := calc.EstimateInboundActivityCost("Create", 1024, true)

		assert.Greater(t, withSig, withoutSig)
	})
}

// === EstimateOutboundActivityCost Tests ===

func TestEstimateOutboundActivityCost(t *testing.T) {
	calc := NewCostCalculator()

	t.Run("monotonic_with_payload_size", func(t *testing.T) {
		// Use significantly different sizes to ensure cost difference
		smallCost := calc.EstimateOutboundActivityCost("Create", 1024, 1)      // 1KB
		largeCost := calc.EstimateOutboundActivityCost("Create", 1024*1024, 1) // 1MB

		assert.Greater(t, largeCost, smallCost)
	})

	t.Run("monotonic_with_target_count", func(t *testing.T) {
		singleTarget := calc.EstimateOutboundActivityCost("Create", 1024, 1)
		multiTarget := calc.EstimateOutboundActivityCost("Create", 1024, 10)

		assert.Greater(t, multiTarget, singleTarget)
	})

	t.Run("includes_transfer_and_request_costs", func(t *testing.T) {
		// With 10 targets, we should have HTTP requests and data transfer
		cost := calc.EstimateOutboundActivityCost("Create", 1024, 10)
		assert.Greater(t, cost, int64(0))
	})
}

// === estimateLambdaDuration Tests ===

func TestEstimateLambdaDuration(t *testing.T) {
	calc := NewCostCalculator()

	tests := []struct {
		activityType     string
		payloadSize      int64
		expectedBaseDur  int64
		additionalFromKB int64
	}{
		{"Create", 0, 200, 0},
		{"Update", 0, 150, 0},
		{"Delete", 0, 100, 0},
		{"Follow", 0, 100, 0},
		{"Accept", 0, 80, 0},
		{"Like", 0, 50, 0},
		{"Unknown", 0, 100, 0},       // Default
		{"Create", 5 * 1024, 200, 5}, // 5KB adds 5ms
	}

	for _, tt := range tests {
		t.Run(tt.activityType, func(t *testing.T) {
			result := calc.estimateLambdaDuration(tt.activityType, tt.payloadSize)
			expected := tt.expectedBaseDur + tt.additionalFromKB
			assert.Equal(t, expected, result)
		})
	}
}

// === estimateDynamoDBWrites Tests ===

func TestEstimateDynamoDBWrites(t *testing.T) {
	calc := NewCostCalculator()

	tests := []struct {
		activityType   string
		expectedWrites int64
	}{
		{"Create", 3},
		{"Update", 2},
		{"Delete", 2},
		{"Follow", 2},
		{"Like", 2},
		{"Unknown", 2}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.activityType, func(t *testing.T) {
			result := calc.estimateDynamoDBWrites(tt.activityType)
			assert.Equal(t, tt.expectedWrites, result)
		})
	}
}

// === estimateDynamoDBReads Tests ===

func TestEstimateDynamoDBReads(t *testing.T) {
	calc := NewCostCalculator()

	tests := []struct {
		activityType  string
		expectedReads int64
	}{
		{"Create", 2},
		{"Follow", 3},
		{"Undo", 3},
		{"Flag", 3},
		{"Like", 2},
		{"Unknown", 2}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.activityType, func(t *testing.T) {
			result := calc.estimateDynamoDBReads(tt.activityType)
			assert.Equal(t, tt.expectedReads, result)
		})
	}
}

// === GetCostEstimate Tests ===

func TestGetCostEstimate(t *testing.T) {
	calc := NewCostCalculator()

	t.Run("breakdown_maps_cost_components", func(t *testing.T) {
		params := &CostCalculationParams{
			LambdaDurationMs:        1000,
			LambdaMemoryMB:          512,
			SignatureVerificationMs: 50,
			HTTPRequestCount:        2,
			Direction:               "outbound",
			DataTransferBytes:       1024,
			DynamoDBWriteCount:      3,
			DynamoDBReadCount:       2,
			RetryCount:              1,
		}

		estimate := calc.GetCostEstimate(params)

		require.NotNil(t, estimate.Breakdown)
		assert.Greater(t, estimate.Breakdown.LambdaCost, int64(0))
		assert.Greater(t, estimate.Breakdown.SignatureVerificationCost, int64(0))
		assert.Greater(t, estimate.Breakdown.HTTPRequestCost, int64(0))
		assert.Greater(t, estimate.Breakdown.RetryCost, int64(0))
	})

	t.Run("confidence_high_with_duration_and_bytes", func(t *testing.T) {
		params := &CostCalculationParams{
			LambdaDurationMs:  500,
			DataTransferBytes: 1024,
		}

		estimate := calc.GetCostEstimate(params)
		assert.Equal(t, "high", estimate.Confidence)
	})

	t.Run("confidence_medium_with_type_and_payload", func(t *testing.T) {
		params := &CostCalculationParams{
			ActivityType: "Create",
			PayloadSize:  1024,
		}

		estimate := calc.GetCostEstimate(params)
		assert.Equal(t, "medium", estimate.Confidence)
		assert.Contains(t, estimate.Notes, "Estimate based on activity type and payload size")
	})

	t.Run("confidence_low_with_limited_data", func(t *testing.T) {
		params := &CostCalculationParams{}

		estimate := calc.GetCostEstimate(params)
		assert.Equal(t, "low", estimate.Confidence)
		assert.Contains(t, estimate.Notes, "Limited data available for estimation")
	})

	t.Run("estimated_cost_dollars_correct", func(t *testing.T) {
		params := &CostCalculationParams{
			HTTPRequestCount: 10, // 10 * 100 = 1000 microdollars
		}

		estimate := calc.GetCostEstimate(params)
		assert.InDelta(t, 0.001, estimate.EstimatedCostDollars, 0.0001)
	})
}

// === NewCostCalculator Tests ===

func TestNewCostCalculator(t *testing.T) {
	calc := NewCostCalculator()

	assert.Equal(t, int64(17), calc.LambdaMemoryGBSecondRate)
	assert.Equal(t, int64(100), calc.HTTPRequestRate)
	assert.Equal(t, int64(90000), calc.DataTransferOutboundGBRate)
	assert.Equal(t, int64(0), calc.DataTransferInboundGBRate)
	assert.Equal(t, int64(5), calc.SignatureVerificationCPURate)
}

// === Edge Cases ===

func TestCalculateFederationCosts_EdgeCases(t *testing.T) {
	calc := NewCostCalculator()

	t.Run("zero_values_produce_zero_costs", func(t *testing.T) {
		params := &CostCalculationParams{}
		result := calc.CalculateFederationCosts(params)

		assert.Equal(t, int64(0), result.TotalCostMicroCents)
	})

	t.Run("compression_ratio_zero_when_no_payload", func(t *testing.T) {
		params := &CostCalculationParams{
			PayloadSize:    0,
			CompressedSize: 0,
		}
		result := calc.CalculateFederationCosts(params)

		assert.Equal(t, float64(0), result.CompressionRatio)
	})

	t.Run("copies_metadata_fields", func(t *testing.T) {
		now := time.Now()
		params := &CostCalculationParams{
			ActivityID:    "test-123",
			Domain:        "example.com",
			ActivityType:  "Create",
			Direction:     "outbound",
			OperationType: "delivery",
			Success:       true,
			Timestamp:     now,
		}

		result := calc.CalculateFederationCosts(params)

		assert.Equal(t, "test-123", result.ActivityID)
		assert.Equal(t, "example.com", result.Domain)
		assert.Equal(t, "Create", result.ActivityType)
		assert.Equal(t, "outbound", result.Direction)
		assert.Equal(t, "delivery", result.OperationType)
		assert.True(t, result.Success)
		assert.Equal(t, now, result.Timestamp)
	})
}
