package cost

// DailyCostAggregate represents aggregated costs for a single day.
type DailyCostAggregate struct {
	Date                string
	TotalCostMicrocents int64
	RequestCount        int64
	UniqueUsers         int64
	DynamoDBReads       int64
	DynamoDBWrites      int64
	LambdaInvocations   int64
	LambdaDurationMs    int64
	DataTransferBytes   int64
}

// MonthlyCostAggregate represents aggregated costs for a month.
type MonthlyCostAggregate struct {
	Year                    int
	Month                   int
	TotalCostMicrocents     int64
	ProjectedCostMicrocents int64
	RequestCount            int64
	UniqueUsers             int64
	DynamoDBReads           int64
	DynamoDBWrites          int64
	LambdaInvocations       int64
	LambdaDurationMs        int64
	DataTransferGB          float64
}

