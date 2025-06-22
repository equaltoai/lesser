package cost

import (
	"context"
	"sync/atomic"
	"time"
)

// AWS Pricing Constants (in microcents for precision)
// Prices are per region - using us-east-1 as default
const (
	// DynamoDB Pricing (per request)
	DynamoDBReadRequestUnit  = 25    // $0.25 per million read request units (strongly consistent)
	DynamoDBWriteRequestUnit = 125   // $1.25 per million write request units
	DynamoDBStreamReadUnit   = 20    // $0.20 per million stream read request units
	DynamoDBStoragePerGB     = 25000 // $0.25 per GB-month (calculated per hour)

	// Lambda Pricing
	LambdaRequestCost       = 20   // $0.20 per million requests
	LambdaGBSecondCost      = 1667 // $0.0000166667 per GB-second
	LambdaDurationMinMS     = 1    // Minimum billable duration
	LambdaMemoryIncrementMB = 1    // Memory increments

	// S3 Pricing
	S3PutRequestCost    = 500   // $0.005 per 1,000 PUT requests
	S3GetRequestCost    = 40    // $0.0004 per 1,000 GET requests
	S3StorageStandardGB = 23000 // $0.023 per GB-month (calculated per hour)
	S3DataTransferPerGB = 90000 // $0.09 per GB data transfer out

	// CloudFront Pricing
	CloudFrontRequestCost    = 100   // $0.01 per 10,000 requests
	CloudFrontHTTPSCost      = 10    // $0.01 per 10,000 HTTPS requests
	CloudFrontDataTransferGB = 85000 // $0.085 per GB (first 10TB)

	// Micro to cents conversion
	MicroCentsToCents = 1000000
)

// OperationCost represents the cost breakdown of a single operation
type OperationCost struct {
	DynamoDBReads       int64     `json:"dynamodb_reads"`
	DynamoDBWrites      int64     `json:"dynamodb_writes"`
	DynamoDBStorage     int64     `json:"dynamodb_storage_bytes"`
	LambdaInvocations   int64     `json:"lambda_invocations"`
	LambdaDurationMs    int64     `json:"lambda_duration_ms"`
	LambdaMemoryMB      int64     `json:"lambda_memory_mb"`
	S3Gets              int64     `json:"s3_gets"`
	S3Puts              int64     `json:"s3_puts"`
	S3Storage           int64     `json:"s3_storage_bytes"`
	DataTransferBytes   int64     `json:"data_transfer_bytes"`
	TotalCostMicroCents int64     `json:"total_cost_microcents"`
	Timestamp           time.Time `json:"timestamp"`
	OperationType       string    `json:"operation_type"`
	RequestID           string    `json:"request_id"`
}

// Tracker provides thread-safe cost tracking for operations
type Tracker struct {
	dynamoReads       atomic.Int64
	dynamoWrites      atomic.Int64
	dynamoStorage     atomic.Int64
	lambdaInvocations atomic.Int64
	lambdaDurationMs  atomic.Int64
	lambdaMemoryMB    atomic.Int64
	s3Gets            atomic.Int64
	s3Puts            atomic.Int64
	s3Storage         atomic.Int64
	dataTransfer      atomic.Int64

	// For request-scoped tracking
	requestID     string
	operationType string
	startTime     time.Time
}

// New creates a new cost tracker
func New() *Tracker {
	return &Tracker{
		startTime: time.Now(),
	}
}

// NewWithRequest creates a new cost tracker with request context
func NewWithRequest(requestID, operationType string) *Tracker {
	return &Tracker{
		requestID:     requestID,
		operationType: operationType,
		startTime:     time.Now(),
	}
}

// TrackDynamoRead tracks DynamoDB read operations
func (t *Tracker) TrackDynamoRead(items int) {
	t.dynamoReads.Add(int64(items))
}

// TrackDynamoWrite tracks DynamoDB write operations
func (t *Tracker) TrackDynamoWrite(items int) {
	t.dynamoWrites.Add(int64(items))
}

// TrackDynamoStorage tracks DynamoDB storage usage
func (t *Tracker) TrackDynamoStorage(bytes int64) {
	t.dynamoStorage.Add(bytes)
}

// TrackLambdaInvocation tracks Lambda invocation
func (t *Tracker) TrackLambdaInvocation(durationMs int64, memoryMB int64) {
	t.lambdaInvocations.Add(1)
	// Round up to nearest ms
	if durationMs < LambdaDurationMinMS {
		durationMs = LambdaDurationMinMS
	}
	t.lambdaDurationMs.Add(durationMs)
	t.lambdaMemoryMB.Add(memoryMB)
}

// TrackS3Get tracks S3 GET operations
func (t *Tracker) TrackS3Get(count int) {
	t.s3Gets.Add(int64(count))
}

// TrackS3Put tracks S3 PUT operations
func (t *Tracker) TrackS3Put(count int) {
	t.s3Puts.Add(int64(count))
}

// TrackS3Storage tracks S3 storage usage
func (t *Tracker) TrackS3Storage(bytes int64) {
	t.s3Storage.Add(bytes)
}

// TrackDataTransfer tracks data transfer out
func (t *Tracker) TrackDataTransfer(bytes int64) {
	t.dataTransfer.Add(bytes)
}

// CalculateCost calculates the total cost of tracked operations
func (t *Tracker) CalculateCost() *OperationCost {
	cost := &OperationCost{
		DynamoDBReads:     t.dynamoReads.Load(),
		DynamoDBWrites:    t.dynamoWrites.Load(),
		DynamoDBStorage:   t.dynamoStorage.Load(),
		LambdaInvocations: t.lambdaInvocations.Load(),
		LambdaDurationMs:  t.lambdaDurationMs.Load(),
		LambdaMemoryMB:    t.lambdaMemoryMB.Load(),
		S3Gets:            t.s3Gets.Load(),
		S3Puts:            t.s3Puts.Load(),
		S3Storage:         t.s3Storage.Load(),
		DataTransferBytes: t.dataTransfer.Load(),
		Timestamp:         time.Now(),
		OperationType:     t.operationType,
		RequestID:         t.requestID,
	}

	// Calculate costs in micro cents
	var total int64

	// DynamoDB costs
	if cost.DynamoDBReads > 0 {
		total += (cost.DynamoDBReads * DynamoDBReadRequestUnit) / 1000000
	}
	if cost.DynamoDBWrites > 0 {
		total += (cost.DynamoDBWrites * DynamoDBWriteRequestUnit) / 1000000
	}
	if cost.DynamoDBStorage > 0 {
		// Convert bytes to GB and calculate hourly cost
		gbHours := float64(cost.DynamoDBStorage) / (1024 * 1024 * 1024)
		storageCost := int64(gbHours * float64(DynamoDBStoragePerGB) / (30 * 24))
		total += storageCost
	}

	// Lambda costs
	if cost.LambdaInvocations > 0 {
		// Request cost
		total += (cost.LambdaInvocations * LambdaRequestCost) / 1000000

		// Compute cost (GB-seconds)
		gbSeconds := float64(cost.LambdaDurationMs) * float64(cost.LambdaMemoryMB) / (1000 * 1024)
		computeCost := int64(gbSeconds * float64(LambdaGBSecondCost))
		total += computeCost
	}

	// S3 costs
	if cost.S3Gets > 0 {
		total += (cost.S3Gets * S3GetRequestCost) / 1000
	}
	if cost.S3Puts > 0 {
		total += (cost.S3Puts * S3PutRequestCost) / 1000
	}
	if cost.S3Storage > 0 {
		// Convert bytes to GB and calculate hourly cost
		gbHours := float64(cost.S3Storage) / (1024 * 1024 * 1024)
		storageCost := int64(gbHours * float64(S3StorageStandardGB) / (30 * 24))
		total += storageCost
	}

	// Data transfer costs
	if cost.DataTransferBytes > 0 {
		// Convert bytes to GB
		gb := float64(cost.DataTransferBytes) / (1024 * 1024 * 1024)
		transferCost := int64(gb * float64(S3DataTransferPerGB))
		total += transferCost
	}

	cost.TotalCostMicroCents = total
	return cost
}

// Reset resets all counters
func (t *Tracker) Reset() {
	t.dynamoReads.Store(0)
	t.dynamoWrites.Store(0)
	t.dynamoStorage.Store(0)
	t.lambdaInvocations.Store(0)
	t.lambdaDurationMs.Store(0)
	t.lambdaMemoryMB.Store(0)
	t.s3Gets.Store(0)
	t.s3Puts.Store(0)
	t.s3Storage.Store(0)
	t.dataTransfer.Store(0)
	t.startTime = time.Now()
}

// Merge combines costs from another tracker
func (t *Tracker) Merge(other *Tracker) {
	if other == nil {
		return
	}

	t.dynamoReads.Add(other.dynamoReads.Load())
	t.dynamoWrites.Add(other.dynamoWrites.Load())
	t.dynamoStorage.Add(other.dynamoStorage.Load())
	t.lambdaInvocations.Add(other.lambdaInvocations.Load())
	t.lambdaDurationMs.Add(other.lambdaDurationMs.Load())
	t.lambdaMemoryMB.Add(other.lambdaMemoryMB.Load())
	t.s3Gets.Add(other.s3Gets.Load())
	t.s3Puts.Add(other.s3Puts.Load())
	t.s3Storage.Add(other.s3Storage.Load())
	t.dataTransfer.Add(other.dataTransfer.Load())
}

// Clone creates a copy of the tracker with current values
func (t *Tracker) Clone() *Tracker {
	clone := &Tracker{
		requestID:     t.requestID,
		operationType: t.operationType,
		startTime:     t.startTime,
	}

	clone.dynamoReads.Store(t.dynamoReads.Load())
	clone.dynamoWrites.Store(t.dynamoWrites.Load())
	clone.dynamoStorage.Store(t.dynamoStorage.Load())
	clone.lambdaInvocations.Store(t.lambdaInvocations.Load())
	clone.lambdaDurationMs.Store(t.lambdaDurationMs.Load())
	clone.lambdaMemoryMB.Store(t.lambdaMemoryMB.Load())
	clone.s3Gets.Store(t.s3Gets.Load())
	clone.s3Puts.Store(t.s3Puts.Load())
	clone.s3Storage.Store(t.s3Storage.Load())
	clone.dataTransfer.Store(t.dataTransfer.Load())

	return clone
}

// TrackWrite tracks DynamoDB write operations (convenience function for storage layer)
func TrackWrite(ctx context.Context, tracker *Tracker, operation string, items int) {
	if tracker != nil {
		tracker.TrackDynamoWrite(items)
	}
}

// TrackRead tracks DynamoDB read operations (convenience function for storage layer)
func TrackRead(ctx context.Context, tracker *Tracker, operation string, items int64) {
	if tracker != nil {
		tracker.TrackDynamoRead(int(items))
	}
}
