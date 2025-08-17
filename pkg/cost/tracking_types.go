package cost

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// Operation types for different AWS services

// DynamoOperation represents a DynamoDB operation for cost tracking
type DynamoOperation struct {
	Type                string    // "Query", "Scan", "PutItem", "UpdateItem", "DeleteItem", "BatchWrite"
	TableName           string    // Name of the DynamoDB table
	ConsumedReadUnits   int64     // Read capacity units consumed
	ConsumedWriteUnits  int64     // Write capacity units consumed
	ItemCount           int64     // Number of items affected
	IndexName           string    // GSI name if applicable
	OperationID         string    // Unique operation identifier
	UserID              string    // User associated with the operation
	Timestamp           time.Time // When the operation occurred
}

// S3Operation represents an S3 operation for cost tracking
type S3Operation struct {
	Type              string    // "PutObject", "GetObject", "DeleteObject", "ListObjects"
	BucketName        string    // Name of the S3 bucket
	ObjectKey         string    // S3 object key
	RequestCount      int64     // Number of requests
	BytesTransferred  int64     // Bytes uploaded/downloaded
	StorageClass      string    // Standard, IA, Glacier, etc.
	OperationID       string    // Unique operation identifier
	UserID            string    // User associated with the operation
	Timestamp         time.Time // When the operation occurred
}

// LambdaOperation represents a Lambda invocation for cost tracking
type LambdaOperation struct {
	FunctionName      string        // Name of the Lambda function
	Duration          time.Duration // Actual execution duration
	MemoryMB          int64         // Memory allocated in MB
	MemoryUsedMB      int64         // Memory actually used (if available)
	ColdStart         bool          // Whether this was a cold start
	RequestID         string        // Lambda request ID
	UserID            string        // User associated with the invocation
	Timestamp         time.Time     // When the invocation occurred
}

// Cost represents the calculated cost of an operation
type Cost struct {
	Service                   string    // "DynamoDB", "S3", "Lambda", etc.
	ReadCostMicroCents        int64     // Cost for read operations
	WriteCostMicroCents       int64     // Cost for write operations
	RequestCostMicroCents     int64     // Cost for requests
	StorageCostMicroCents     int64     // Cost for storage
	InvocationCostMicroCents  int64     // Cost for Lambda invocations
	DurationCostMicroCents    int64     // Cost for Lambda duration
	DataTransferCostMicroCents int64    // Cost for data transfer
	TotalMicroCents           int64     // Total cost in microcents
	Timestamp                 time.Time // When the cost was calculated
}

// TotalDollars returns the total cost in dollars
func (c *Cost) TotalDollars() float64 {
	return float64(c.TotalMicroCents) / float64(MicroCentsToCents)
}

// MetricData represents a custom metric to be sent to CloudWatch
type MetricData struct {
	Name       string              // Metric name
	Value      float64             // Metric value
	Unit       types.StandardUnit  // CloudWatch unit
	Dimensions []types.Dimension   // CloudWatch dimensions
	Timestamp  time.Time           // Metric timestamp
}

// Specialized trackers for each service

// DynamoDBTracker provides DynamoDB-specific cost calculations
type DynamoDBTracker struct {
	readCostCache  map[string]float64 // Cache for read cost calculations
	writeCostCache map[string]float64 // Cache for write cost calculations
}

// NewDynamoDBTracker creates a new DynamoDB cost tracker
func NewDynamoDBTracker() *DynamoDBTracker {
	return &DynamoDBTracker{
		readCostCache:  make(map[string]float64),
		writeCostCache: make(map[string]float64),
	}
}

// CalculateCost calculates the cost of a DynamoDB operation
func (dt *DynamoDBTracker) CalculateCost(operation DynamoOperation) Cost {
	readCostMicroCents := int64(float64(operation.ConsumedReadUnits) * float64(DynamoDBReadRequestUnit))
	writeCostMicroCents := int64(float64(operation.ConsumedWriteUnits) * float64(DynamoDBWriteRequestUnit))
	
	return Cost{
		Service:             "DynamoDB",
		ReadCostMicroCents:  readCostMicroCents,
		WriteCostMicroCents: writeCostMicroCents,
		TotalMicroCents:     readCostMicroCents + writeCostMicroCents,
		Timestamp:           operation.Timestamp,
	}
}

// S3Tracker provides S3-specific cost calculations
type S3Tracker struct {
	requestCostCache map[string]float64 // Cache for request cost calculations
	storageCostCache map[string]float64 // Cache for storage cost calculations
}

// NewS3Tracker creates a new S3 cost tracker
func NewS3Tracker() *S3Tracker {
	return &S3Tracker{
		requestCostCache: make(map[string]float64),
		storageCostCache: make(map[string]float64),
	}
}

// CalculateCost calculates the cost of an S3 operation
func (st *S3Tracker) CalculateCost(operation S3Operation) Cost {
	var requestCostMicroCents int64
	
	// Calculate request costs based on operation type
	switch operation.Type {
	case "PutObject", "PostObject", "CopyObject":
		requestCostMicroCents = int64(float64(operation.RequestCount) * float64(S3PutRequestCost) / 1000)
	case "GetObject", "ListObjects", "HeadObject":
		requestCostMicroCents = int64(float64(operation.RequestCount) * float64(S3GetRequestCost) / 1000)
	}
	
	// Calculate data transfer costs
	dataTransferCostMicroCents := int64(float64(operation.BytesTransferred) * float64(S3DataTransferPerGB) / (1024 * 1024 * 1024))
	
	return Cost{
		Service:                    "S3",
		RequestCostMicroCents:      requestCostMicroCents,
		DataTransferCostMicroCents: dataTransferCostMicroCents,
		TotalMicroCents:            requestCostMicroCents + dataTransferCostMicroCents,
		Timestamp:                  operation.Timestamp,
	}
}

// LambdaTracker provides Lambda-specific cost calculations
type LambdaTracker struct {
	invocationCostCache map[string]float64 // Cache for invocation cost calculations
	durationCostCache   map[string]float64 // Cache for duration cost calculations
}

// NewLambdaTracker creates a new Lambda cost tracker
func NewLambdaTracker() *LambdaTracker {
	return &LambdaTracker{
		invocationCostCache: make(map[string]float64),
		durationCostCache:   make(map[string]float64),
	}
}

// CalculateCost calculates the cost of a Lambda invocation
func (lt *LambdaTracker) CalculateCost(operation LambdaOperation) Cost {
	// Lambda invocation cost (per request)
	invocationCostMicroCents := int64(LambdaRequestCost)
	
	// Lambda duration cost (GB-seconds)
	durationMs := operation.Duration.Milliseconds()
	if durationMs < LambdaDurationMinMS {
		durationMs = LambdaDurationMinMS
	}
	
	gbSeconds := float64(operation.MemoryMB) / 1024.0 * float64(durationMs) / 1000.0
	durationCostMicroCents := int64(gbSeconds * float64(LambdaGBSecondCost))
	
	// Cold start penalty (estimated additional cost)
	var coldStartPenalty int64
	if operation.ColdStart {
		coldStartPenalty = int64(LambdaRequestCost) / 10 // 10% penalty for cold starts
	}
	
	return Cost{
		Service:                  "Lambda",
		InvocationCostMicroCents: invocationCostMicroCents + coldStartPenalty,
		DurationCostMicroCents:   durationCostMicroCents,
		TotalMicroCents:          invocationCostMicroCents + durationCostMicroCents + coldStartPenalty,
		Timestamp:                operation.Timestamp,
	}
}

// CostSummary provides aggregated cost information
type CostSummary struct {
	TotalCostMicroCents       int64                   `json:"total_cost_microcents"`
	ServiceBreakdown          map[string]int64        `json:"service_breakdown"`
	OperationBreakdown        map[string]int64        `json:"operation_breakdown"`
	HourlyBreakdown           map[string]int64        `json:"hourly_breakdown"`
	TopCostDrivers            []CostDriver            `json:"top_cost_drivers"`
	CostTrends                []CostTrendPoint        `json:"cost_trends"`
	BudgetUtilization         float64                 `json:"budget_utilization"`
	ProjectedMonthlyCost      int64                   `json:"projected_monthly_cost"`
	RecommendedOptimizations  []OptimizationSuggestion `json:"optimizations"`
}

// CostDriver represents a significant contributor to costs
type CostDriver struct {
	Service           string  `json:"service"`
	Operation         string  `json:"operation"`
	CostMicroCents    int64   `json:"cost_microcents"`
	PercentageOfTotal float64 `json:"percentage_of_total"`
	OperationCount    int64   `json:"operation_count"`
	AverageCost       int64   `json:"average_cost"`
}

// CostTrendPoint represents a point in cost trends over time
type CostTrendPoint struct {
	Timestamp       time.Time `json:"timestamp"`
	CostMicroCents  int64     `json:"cost_microcents"`
	OperationCount  int64     `json:"operation_count"`
}

// OptimizationSuggestion provides cost optimization recommendations
type OptimizationSuggestion struct {
	Category        string  `json:"category"`        // "DynamoDB", "S3", "Lambda", etc.
	Suggestion      string  `json:"suggestion"`      // Human-readable suggestion
	EstimatedSavings int64  `json:"estimated_savings"` // Estimated savings in microcents
	Priority        string  `json:"priority"`        // "High", "Medium", "Low"
	Effort          string  `json:"effort"`          // "Low", "Medium", "High"
}

// Repository interface for cost tracking storage
type CostTrackingRepository interface {
	// Store cost records
	StoreCost(ctx context.Context, cost *Cost) error
	StoreCostBatch(ctx context.Context, costs []*Cost) error
	
	// Retrieve cost information
	GetCostSummary(ctx context.Context, startTime, endTime time.Time) (*CostSummary, error)
	GetServiceCosts(ctx context.Context, service string, startTime, endTime time.Time) ([]*Cost, error)
	GetUserCosts(ctx context.Context, userID string, startTime, endTime time.Time) ([]*Cost, error)
	
	// Cost analysis
	GetTopCostDrivers(ctx context.Context, startTime, endTime time.Time, limit int) ([]CostDriver, error)
	GetCostTrends(ctx context.Context, startTime, endTime time.Time, granularity string) ([]CostTrendPoint, error)
	GetOptimizationSuggestions(ctx context.Context, lookbackDays int) ([]OptimizationSuggestion, error)
}

// UnifiedCostTracker provides a simplified interface for common cost tracking operations
type UnifiedCostTracker interface {
	// DynamoDB operations
	TrackDynamoRead(ctx context.Context, tableName string, units int64) error
	TrackDynamoWrite(ctx context.Context, tableName string, units int64) error
	TrackDynamoQuery(ctx context.Context, tableName string, units int64) error
	TrackDynamoScan(ctx context.Context, tableName string, units int64) error
	
	// S3 operations
	TrackS3Get(ctx context.Context, bucketName string, bytes int64) error
	TrackS3Put(ctx context.Context, bucketName string, bytes int64) error
	TrackS3Delete(ctx context.Context, bucketName string) error
	
	// Lambda operations
	TrackLambdaInvocation(ctx context.Context, functionName string, duration time.Duration, memoryMB int64) error
	
	// Metrics and reporting
	GetCurrentCostMicroCents() int64
	GetCostBreakdown() map[string]int64
	Reset()
}