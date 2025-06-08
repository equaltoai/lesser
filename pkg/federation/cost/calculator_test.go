package cost

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCostCalculator_EstimateDataTransferCost(t *testing.T) {
	calc := NewCostCalculator("us-east-1")

	tests := []struct {
		name     string
		bytes    int64
		expected float64
	}{
		{
			name:     "free tier - under 1GB",
			bytes:    500 * 1024 * 1024, // 500MB
			expected: 0.0,
		},
		{
			name:     "just over free tier",
			bytes:    2 * 1024 * 1024 * 1024, // 2GB
			expected: 0.09,                   // 1GB at $0.09/GB
		},
		{
			name:     "10GB transfer",
			bytes:    10 * 1024 * 1024 * 1024, // 10GB
			expected: 0.81,                    // 9GB at $0.09/GB
		},
		{
			name:     "regional adjustment",
			bytes:    2 * 1024 * 1024 * 1024, // 2GB
			expected: 0.10,                   // With 15% regional markup
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region := "us-east-1"
			if i == len(tests)-1 { // Last test uses different region
				region = "eu-west-1"
			}

			cost := calc.EstimateDataTransferCost(tt.bytes, region)
			assert.Equal(t, tt.expected, cost, "cost calculation mismatch")
		})
	}
}

func TestCostCalculator_EstimateLambdaCost(t *testing.T) {
	calc := NewCostCalculator("us-east-1")

	tests := []struct {
		name        string
		invocations int
		durationMs  int64
		expected    float64
	}{
		{
			name:        "free tier invocations",
			invocations: 100_000,
			durationMs:  100, // 100ms per invocation
			expected:    0.0, // Within free tier
		},
		{
			name:        "beyond free tier",
			invocations: 2_000_000,
			durationMs:  200,
			expected:    0.87, // Calculated cost
		},
		{
			name:        "high volume",
			invocations: 10_000_000,
			durationMs:  50,
			expected:    3.42, // Calculated cost
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calc.EstimateLambdaCost(tt.invocations, tt.durationMs)
			assert.InDelta(t, tt.expected, cost, 0.01, "lambda cost calculation mismatch")
		})
	}
}

func TestCostCalculator_EstimateDynamoDBCost(t *testing.T) {
	calc := NewCostCalculator("us-east-1")

	tests := []struct {
		name       string
		readUnits  int
		writeUnits int
		expected   float64
	}{
		{
			name:       "free tier operations",
			readUnits:  1_000_000,
			writeUnits: 500_000,
			expected:   0.0, // Within free tier
		},
		{
			name:       "beyond free tier",
			readUnits:  5_000_000,
			writeUnits: 2_000_000,
			expected:   1.75, // (2.5M reads * 0.25 + 1M writes * 1.25) / 1M
		},
		{
			name:       "high volume",
			readUnits:  20_000_000,
			writeUnits: 10_000_000,
			expected:   16.88, // Calculated cost
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calc.EstimateDynamoDBCost(tt.readUnits, tt.writeUnits)
			assert.InDelta(t, tt.expected, cost, 0.01, "dynamodb cost calculation mismatch")
		})
	}
}

func TestCostCalculator_EstimateS3Cost(t *testing.T) {
	calc := NewCostCalculator("us-east-1")

	tests := []struct {
		name         string
		storageGB    int64
		requestCount int64
		expected     float64
	}{
		{
			name:         "free tier",
			storageGB:    3,
			requestCount: 10_000,
			expected:     0.0, // Within free tier
		},
		{
			name:         "beyond free tier storage",
			storageGB:    10,
			requestCount: 10_000,
			expected:     0.12, // 5GB * $0.023
		},
		{
			name:         "beyond free tier requests",
			storageGB:    3,
			requestCount: 50_000,
			expected:     0.01, // 30k requests * $0.0004/1000
		},
		{
			name:         "high volume",
			storageGB:    100,
			requestCount: 1_000_000,
			expected:     2.57, // Storage + requests
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calc.EstimateS3Cost(tt.storageGB, tt.requestCount)
			assert.InDelta(t, tt.expected, cost, 0.01, "s3 cost calculation mismatch")
		})
	}
}

func TestEstimateTotalActivityCost(t *testing.T) {
	// Test comprehensive cost calculation
	cost := EstimateTotalActivityCost(
		10*1024*1024*1024, // 10GB data transfer
		1_000_000,         // 1M Lambda invocations
		100,               // 100ms average duration
		5_000_000,         // 5M DynamoDB reads
		1_000_000,         // 1M DynamoDB writes
		50,                // 50GB S3 storage
		100_000,           // 100k S3 requests
	)

	// Expected: data transfer + lambda + dynamodb + s3
	// = 0.81 + 0.17 + 0.63 + 1.04
	expected := 2.65
	assert.InDelta(t, expected, cost, 0.1, "total cost calculation mismatch")
}
