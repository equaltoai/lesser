package cost

import (
	"math"
)

// calculator implements the CostCalculator interface
type calculator struct {
	region string
}

// NewCostCalculator creates a new cost calculator for the specified AWS region
func NewCostCalculator(region string) CostCalculator {
	return &calculator{
		region: region,
	}
}

// EstimateDataTransferCost calculates the cost of data transfer in USD
func (c *calculator) EstimateDataTransferCost(bytes int64, region string) float64 {
	// Convert bytes to GB
	gb := float64(bytes) / (1024 * 1024 * 1024)

	// Apply regional pricing adjustments
	costPerGB := DataTransferCostPerGB
	if region != "us-east-1" {
		// Other regions typically have 10-20% higher costs
		costPerGB *= 1.15
	}

	// First 1GB per month is free (AWS free tier)
	if gb <= 1.0 {
		return 0
	}

	// Tiered pricing for data transfer
	var cost float64
	if gb <= 10 {
		// $0.09 per GB for first 10 TB
		cost = (gb - 1) * costPerGB
	} else if gb <= 50 {
		// $0.085 per GB for next 40 TB
		cost = 9*costPerGB + (gb-10)*costPerGB*0.944
	} else if gb <= 150 {
		// $0.07 per GB for next 100 TB
		cost = 9*costPerGB + 40*costPerGB*0.944 + (gb-50)*costPerGB*0.777
	} else {
		// $0.05 per GB for over 150 TB
		cost = 9*costPerGB + 40*costPerGB*0.944 + 100*costPerGB*0.777 + (gb-150)*costPerGB*0.555
	}

	return math.Round(cost*100) / 100 // Round to cents
}

// EstimateLambdaCost calculates the cost of Lambda invocations
func (c *calculator) EstimateLambdaCost(invocations int, durationMs int64) float64 {
	// Request costs
	requestCost := float64(invocations) / 1_000_000 * LambdaCostPerMillionRequests

	// Compute costs (assuming 1GB memory allocation)
	memoryGB := 1.0
	durationSeconds := float64(durationMs) / 1000.0
	gbSeconds := float64(invocations) * memoryGB * durationSeconds
	computeCost := gbSeconds * LambdaCostPerGBSecond

	// First 400,000 GB-seconds free per month
	freeGBSeconds := 400_000.0
	if gbSeconds <= freeGBSeconds {
		computeCost = 0
	} else {
		computeCost = (gbSeconds - freeGBSeconds) * LambdaCostPerGBSecond
	}

	// First 1M requests free per month
	if invocations <= 1_000_000 {
		requestCost = 0
	}

	return math.Round((requestCost+computeCost)*100) / 100
}

// EstimateDynamoDBCost calculates the cost of DynamoDB operations
func (c *calculator) EstimateDynamoDBCost(readUnits, writeUnits int) float64 {
	// On-demand pricing model
	readCost := float64(readUnits) / 1_000_000 * DynamoDBReadCostPerMillion
	writeCost := float64(writeUnits) / 1_000_000 * DynamoDBWriteCostPerMillion

	// Free tier: 25 GB storage, 25 RCU, 25 WCU (provisioned capacity)
	// For on-demand, approximately 2.5M read requests and 1M write requests free
	freeReads := 2_500_000
	freeWrites := 1_000_000

	if readUnits <= freeReads {
		readCost = 0
	} else {
		readCost = float64(readUnits-freeReads) / 1_000_000 * DynamoDBReadCostPerMillion
	}

	if writeUnits <= freeWrites {
		writeCost = 0
	} else {
		writeCost = float64(writeUnits-freeWrites) / 1_000_000 * DynamoDBWriteCostPerMillion
	}

	return math.Round((readCost+writeCost)*100) / 100
}

// EstimateS3Cost calculates the cost of S3 storage and requests
func (c *calculator) EstimateS3Cost(storageGB, requestCount int64) float64 {
	// Standard storage costs
	storageCost := float64(storageGB) * S3StorageCostPerGB

	// Request costs (GET requests)
	requestCost := float64(requestCount) / 1000 * S3RequestCostPerThousand

	// Free tier: 5GB storage, 20,000 GET requests, 2,000 PUT requests
	if storageGB <= 5 {
		storageCost = 0
	} else {
		storageCost = float64(storageGB-5) * S3StorageCostPerGB
	}

	if requestCount <= 20_000 {
		requestCost = 0
	} else {
		requestCost = float64(requestCount-20_000) / 1000 * S3RequestCostPerThousand
	}

	return math.Round((storageCost+requestCost)*100) / 100
}

// EstimateTotalActivityCost estimates the total cost of a federation activity
func EstimateTotalActivityCost(
	dataTransferBytes int64,
	lambdaInvocations int,
	lambdaDurationMs int64,
	dynamoReads int,
	dynamoWrites int,
	s3StorageGB int64,
	s3Requests int64,
) float64 {
	calc := NewCostCalculator("us-east-1")

	transferCost := calc.EstimateDataTransferCost(dataTransferBytes, "us-east-1")
	lambdaCost := calc.EstimateLambdaCost(lambdaInvocations, lambdaDurationMs)
	dynamoCost := calc.EstimateDynamoDBCost(dynamoReads, dynamoWrites)
	s3Cost := calc.EstimateS3Cost(s3StorageGB, s3Requests)

	return transferCost + lambdaCost + dynamoCost + s3Cost
}
