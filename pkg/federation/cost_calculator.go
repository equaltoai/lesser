package federation

import (
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// CostCalculator provides standardized cost calculations for federation activities
type CostCalculator struct {
	// Cost rates in microdollars (1/1,000,000 of a dollar)
	LambdaMemoryGBSecondRate     int64 // $0.0000166667 per GB-second -> 16.6667 microdollars
	HTTPRequestRate              int64 // $0.0001 per request -> 100 microdollars
	DataTransferOutboundGBRate   int64 // $0.09 per GB -> 90,000 microdollars
	DataTransferInboundGBRate    int64 // Free for inbound
	DynamoDBWriteRequestRate     int64 // $1.25 per million requests -> 1.25 microdollars per request
	DynamoDBReadRequestRate      int64 // $0.25 per million requests -> 0.25 microdollars per request
	DNSQueryRate                 int64 // $0.0004 per query -> 400 microdollars per 1000 queries -> 0.4 microdollars per query
	SQSMessageRate               int64 // $0.0000004 per message -> 0.4 microdollars per 1000 messages -> 0.0004 microdollars per message
	SignatureVerificationCPURate int64 // Estimated CPU cost for signature verification
}

// NewCostCalculator creates a new cost calculator with standard AWS pricing
func NewCostCalculator() *CostCalculator {
	return &CostCalculator{
		LambdaMemoryGBSecondRate:     17,    // $0.0000166667 -> ~17 microdollars per GB-second
		HTTPRequestRate:              100,   // $0.0001 -> 100 microdollars per request
		DataTransferOutboundGBRate:   90000, // $0.09 -> 90,000 microdollars per GB
		DataTransferInboundGBRate:    0,     // Free
		DynamoDBWriteRequestRate:     1,     // $1.25 per million -> ~1 microdollar per request
		DynamoDBReadRequestRate:      0,     // $0.25 per million -> ~0.25 microdollars per request (rounded to 0)
		DNSQueryRate:                 0,     // $0.0004 per query -> 0.4 microdollars (rounded to 0)
		SQSMessageRate:               0,     // $0.0000004 per message -> 0.0004 microdollars (rounded to 0)
		SignatureVerificationCPURate: 5,     // Estimated 5 microdollars per signature verification
	}
}

// CalculateFederationCosts calculates comprehensive costs for a federation activity
func (c *CostCalculator) CalculateFederationCosts(params *CostCalculationParams) *models.FederationCostTracking {
	cost := &models.FederationCostTracking{
		ActivityID:    params.ActivityID,
		Domain:        params.Domain,
		ActivityType:  params.ActivityType,
		Direction:     params.Direction,
		OperationType: params.OperationType,
		Success:       params.Success,
		ErrorMessage:  params.ErrorMessage,
		Timestamp:     params.Timestamp,

		// Raw metrics
		LambdaDurationMs:        params.LambdaDurationMs,
		LambdaMemoryMB:          params.LambdaMemoryMB,
		SignatureVerificationMs: params.SignatureVerificationMs,
		HTTPRequestCount:        params.HTTPRequestCount,
		DataTransferBytes:       params.DataTransferBytes,
		DynamoDBWriteCount:      params.DynamoDBWriteCount,
		DynamoDBReadCount:       params.DynamoDBReadCount,
		DNSLookupCount:          params.DNSLookupCount,
		WebFingerCount:          params.WebFingerCount,
		SQSMessageCount:         params.SQSMessageCount,
		RetryCount:              params.RetryCount,
		ResponseTimeMs:          params.ResponseTimeMs,
		ProcessingTimeMs:        params.ProcessingTimeMs,
		QueueWaitTimeMs:         params.QueueWaitTimeMs,
		PayloadSize:             params.PayloadSize,
		CompressedSize:          params.CompressedSize,
	}

	// Calculate compression ratio
	if params.PayloadSize > 0 && params.CompressedSize > 0 {
		cost.CompressionRatio = float64(params.CompressedSize) / float64(params.PayloadSize)
	}

	// Calculate Lambda execution cost
	if params.LambdaDurationMs > 0 && params.LambdaMemoryMB > 0 {
		gbSeconds := (float64(params.LambdaMemoryMB) / 1024.0) * (float64(params.LambdaDurationMs) / 1000.0)
		cost.LambdaExecutionCost = int64(gbSeconds * float64(c.LambdaMemoryGBSecondRate))
	}

	// Calculate signature verification cost (CPU-intensive operation)
	if params.SignatureVerificationMs > 0 {
		// Base cost plus time-based cost
		cost.SignatureVerificationCost = c.SignatureVerificationCPURate +
			int64(float64(params.SignatureVerificationMs)*0.1) // 0.1 microdollars per ms
	}

	// Calculate HTTP request cost
	cost.HTTPRequestCost = params.HTTPRequestCount * c.HTTPRequestRate

	// Calculate data transfer cost (only outbound is charged)
	if params.DataTransferBytes > 0 && params.Direction == "outbound" {
		gb := float64(params.DataTransferBytes) / (1024 * 1024 * 1024)
		cost.DataTransferCost = int64(gb * float64(c.DataTransferOutboundGBRate))
	}

	// Calculate DynamoDB costs
	cost.DynamoDBWriteCost = params.DynamoDBWriteCount * c.DynamoDBWriteRequestRate
	cost.DynamoDBReadCost = params.DynamoDBReadCount * c.DynamoDBReadRequestRate

	// Calculate DNS lookup cost
	cost.DNSLookupCost = params.DNSLookupCount * c.DNSQueryRate

	// Calculate WebFinger cost (treat as HTTP request)
	cost.WebFingerCost = params.WebFingerCount * c.HTTPRequestRate

	// Calculate SQS message cost
	cost.SQSMessageCost = params.SQSMessageCount * c.SQSMessageRate

	// Calculate retry penalty cost (exponential penalty)
	if params.RetryCount > 0 {
		// Each retry adds exponentially increasing cost
		retryCost := int64(0)
		for i := 1; i <= params.RetryCount; i++ {
			retryCost += int64(i) * 50 // 50 microdollars per retry, increasing per attempt
		}
		cost.RetryCost = retryCost
	}

	// Calculate total cost
	cost.CalculateTotalCost()

	return cost
}

// EstimateInboundActivityCost estimates the cost of processing an inbound activity
func (c *CostCalculator) EstimateInboundActivityCost(activityType string, payloadSize int64, requiresSignatureVerification bool) int64 {
	params := &CostCalculationParams{
		ActivityType:       activityType,
		Direction:          "inbound",
		PayloadSize:        payloadSize,
		LambdaDurationMs:   c.estimateLambdaDuration(activityType, payloadSize),
		LambdaMemoryMB:     512,         // Standard memory allocation
		HTTPRequestCount:   0,           // No outbound requests for inbound processing
		DataTransferBytes:  payloadSize, // Inbound data (free)
		DynamoDBWriteCount: c.estimateDynamoDBWrites(activityType),
		DynamoDBReadCount:  c.estimateDynamoDBReads(activityType),
	}

	if requiresSignatureVerification {
		params.SignatureVerificationMs = 50 // Estimated signature verification time
	}

	cost := c.CalculateFederationCosts(params)
	return cost.TotalCostMicroCents
}

// EstimateOutboundActivityCost estimates the cost of delivering an outbound activity
func (c *CostCalculator) EstimateOutboundActivityCost(activityType string, payloadSize int64, targetCount int) int64 {
	params := &CostCalculationParams{
		ActivityType:       activityType,
		Direction:          "outbound",
		PayloadSize:        payloadSize,
		LambdaDurationMs:   c.estimateLambdaDuration(activityType, payloadSize) * int64(targetCount),
		LambdaMemoryMB:     512,                              // Standard memory allocation
		HTTPRequestCount:   int64(targetCount),               // One request per target
		DataTransferBytes:  payloadSize * int64(targetCount), // Outbound data (charged)
		DynamoDBWriteCount: int64(targetCount) + 1,           // Delivery tracking + activity storage
		DynamoDBReadCount:  2,                                // Actor lookup + followers list
		SQSMessageCount:    int64(targetCount),               // One SQS message per delivery
	}

	cost := c.CalculateFederationCosts(params)
	return cost.TotalCostMicroCents
}

// estimateLambdaDuration estimates Lambda execution time based on activity type and payload size
func (c *CostCalculator) estimateLambdaDuration(activityType string, payloadSize int64) int64 {
	// Base duration by activity type (in milliseconds)
	baseDuration := map[string]int64{
		"Create":   200, // Creating posts requires more processing
		"Update":   150, // Updates require validation
		"Delete":   100, // Deletes are simpler
		"Follow":   100, // Follows are simple
		"Accept":   80,  // Accepts are quick
		"Reject":   80,  // Rejects are quick
		"Like":     50,  // Likes are very simple
		"Announce": 100, // Boosts require validation
		"Undo":     80,  // Undos are simple
		"Block":    100, // Blocks require processing
		"Flag":     150, // Flags require more validation
	}

	duration := baseDuration[activityType]
	if duration == 0 {
		duration = 100 // Default duration
	}

	// Add time based on payload size (1ms per KB)
	sizeKB := payloadSize / 1024
	if sizeKB > 0 {
		duration += sizeKB
	}

	return duration
}

// estimateDynamoDBWrites estimates the number of DynamoDB write operations for an activity type
func (c *CostCalculator) estimateDynamoDBWrites(activityType string) int64 {
	writes := map[string]int64{
		"Create":   3, // Activity + Object + Timeline entry
		"Update":   2, // Activity + Object update
		"Delete":   2, // Activity + Object deletion
		"Follow":   2, // Activity + Relationship
		"Accept":   2, // Activity + Relationship update
		"Reject":   2, // Activity + Relationship deletion
		"Like":     2, // Activity + Like record
		"Announce": 2, // Activity + Boost record
		"Undo":     2, // Activity + Undo operation
		"Block":    2, // Activity + Block record
		"Flag":     2, // Activity + Report record
	}

	count := writes[activityType]
	if count == 0 {
		count = 2 // Default: activity + one other operation
	}

	return count
}

// estimateDynamoDBReads estimates the number of DynamoDB read operations for an activity type
func (c *CostCalculator) estimateDynamoDBReads(activityType string) int64 {
	reads := map[string]int64{
		"Create":   2, // Actor lookup + Timeline query
		"Update":   2, // Actor lookup + Object lookup
		"Delete":   2, // Actor lookup + Object lookup
		"Follow":   3, // Actor lookup + Target lookup + Relationship check
		"Accept":   2, // Actor lookup + Relationship lookup
		"Reject":   2, // Actor lookup + Relationship lookup
		"Like":     2, // Actor lookup + Object lookup
		"Announce": 2, // Actor lookup + Object lookup
		"Undo":     3, // Actor lookup + Original activity + Target lookup
		"Block":    2, // Actor lookup + Target lookup
		"Flag":     3, // Actor lookup + Object lookup + Rules lookup
	}

	count := reads[activityType]
	if count == 0 {
		count = 2 // Default: actor lookup + one other read
	}

	return count
}

// CostCalculationParams holds parameters for cost calculation
type CostCalculationParams struct {
	// Activity identification
	ActivityID    string
	Domain        string
	ActivityType  string
	Direction     string // inbound, outbound
	OperationType string // inbox_processing, outbox_delivery, signature_verification

	// Success/failure tracking
	Success      bool
	ErrorMessage string
	Timestamp    time.Time

	// Resource usage metrics
	LambdaDurationMs        int64 // Lambda execution time in milliseconds
	LambdaMemoryMB          int64 // Lambda memory allocation in MB
	SignatureVerificationMs int64 // Time spent verifying signatures
	HTTPRequestCount        int64 // Number of outbound HTTP requests
	DataTransferBytes       int64 // Bytes transferred
	DynamoDBWriteCount      int64 // Number of DynamoDB write operations
	DynamoDBReadCount       int64 // Number of DynamoDB read operations
	DNSLookupCount          int64 // Number of DNS lookups
	WebFingerCount          int64 // Number of WebFinger lookups
	SQSMessageCount         int64 // Number of SQS messages sent
	RetryCount              int   // Number of retry attempts

	// Performance metrics
	ResponseTimeMs   int64 // Total response time
	ProcessingTimeMs int64 // Time spent processing
	QueueWaitTimeMs  int64 // Time spent waiting in queue

	// Data volume metrics
	PayloadSize    int64 // Size of activity payload
	CompressedSize int64 // Size after compression
}

// CostEstimate represents a cost estimate for a federation operation
type CostEstimate struct {
	EstimatedCostMicroCents int64          `json:"estimated_cost_micro_cents"`
	EstimatedCostDollars    float64        `json:"estimated_cost_dollars"`
	Breakdown               *CostBreakdown `json:"breakdown"`
	Confidence              string         `json:"confidence"` // high, medium, low
	Notes                   []string       `json:"notes,omitempty"`
}

// CostBreakdown shows the breakdown of estimated costs
type CostBreakdown struct {
	LambdaCost                int64 `json:"lambda_cost"`
	SignatureVerificationCost int64 `json:"signature_verification_cost"`
	HTTPRequestCost           int64 `json:"http_request_cost"`
	DataTransferCost          int64 `json:"data_transfer_cost"`
	DynamoDBCost              int64 `json:"dynamodb_cost"`
	NetworkingCost            int64 `json:"networking_cost"`
	RetryCost                 int64 `json:"retry_cost"`
}

// GetCostEstimate returns a cost estimate with breakdown
func (c *CostCalculator) GetCostEstimate(params *CostCalculationParams) *CostEstimate {
	cost := c.CalculateFederationCosts(params)

	estimate := &CostEstimate{
		EstimatedCostMicroCents: cost.TotalCostMicroCents,
		EstimatedCostDollars:    cost.GetTotalCostDollars(),
		Breakdown: &CostBreakdown{
			LambdaCost:                cost.LambdaExecutionCost,
			SignatureVerificationCost: cost.SignatureVerificationCost,
			HTTPRequestCost:           cost.HTTPRequestCost,
			DataTransferCost:          cost.DataTransferCost,
			DynamoDBCost:              cost.DynamoDBWriteCost + cost.DynamoDBReadCost,
			NetworkingCost:            cost.DNSLookupCost + cost.WebFingerCost,
			RetryCost:                 cost.RetryCost,
		},
		Confidence: "medium", // Default confidence level
	}

	// Adjust confidence based on data availability
	if params.LambdaDurationMs > 0 && params.DataTransferBytes > 0 {
		estimate.Confidence = "high"
	} else if params.ActivityType != "" && params.PayloadSize > 0 {
		estimate.Confidence = "medium"
		estimate.Notes = append(estimate.Notes, "Estimate based on activity type and payload size")
	} else {
		estimate.Confidence = "low"
		estimate.Notes = append(estimate.Notes, "Limited data available for estimation")
	}

	return estimate
}
