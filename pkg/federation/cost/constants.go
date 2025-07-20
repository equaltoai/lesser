package cost

// AWS pricing constants (US East 1 region, as of 2024)
const (
	// Data transfer costs
	DataTransferCostPerGB = 0.09 // $0.09 per GB for outbound data transfer

	// Lambda costs
	LambdaCostPerMillionRequests = 0.20         // $0.20 per million requests
	LambdaCostPerGBSecond        = 0.0000166667 // $0.0000166667 per GB-second

	// DynamoDB costs (on-demand capacity)
	DynamoDBReadCostPerMillion  = 0.25 // $0.25 per million read request units
	DynamoDBWriteCostPerMillion = 1.25 // $1.25 per million write request units

	// S3 costs
	S3StorageCostPerGB       = 0.023  // $0.023 per GB per month for standard storage
	S3RequestCostPerThousand = 0.0004 // $0.0004 per 1,000 GET requests
)

// CostCalculator defines the interface for estimating AWS costs
type CostCalculator interface {
	EstimateDataTransferCost(bytes int64, region string) float64
	EstimateLambdaCost(invocations int, durationMs int64) float64
	EstimateDynamoDBCost(readUnits, writeUnits int) float64
	EstimateS3Cost(storageGB, requestCount int64) float64
}
