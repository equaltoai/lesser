package cost

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"go.uber.org/zap"
)

// UnifiedTracker implements the UnifiedCostTracker interface using the centralized tracking service
type UnifiedTracker struct {
	service      *TrackingService
	logger       *zap.Logger
	userID       string
	requestID    string
	
	// Accumulated costs
	mu               sync.RWMutex
	totalCostMicroCents int64
	serviceCosts     map[string]int64
	operationCounts  map[string]int64
}

// NewUnifiedTracker creates a new unified cost tracker
func NewUnifiedTracker(cloudWatch *cloudwatch.Client, logger *zap.Logger, userID, requestID string) *UnifiedTracker {
	return &UnifiedTracker{
		service:         NewCostTrackingService(cloudWatch, logger),
		logger:          logger,
		userID:          userID,
		requestID:       requestID,
		serviceCosts:    make(map[string]int64),
		operationCounts: make(map[string]int64),
	}
}

// NewUnifiedTrackerWithService creates a unified tracker using an existing tracking service
func NewUnifiedTrackerWithService(service *TrackingService, logger *zap.Logger, userID, requestID string) *UnifiedTracker {
	return &UnifiedTracker{
		service:         service,
		logger:          logger,
		userID:          userID,
		requestID:       requestID,
		serviceCosts:    make(map[string]int64),
		operationCounts: make(map[string]int64),
	}
}

// DynamoDB Operations

// TrackDynamoRead tracks a DynamoDB read operation
func (ut *UnifiedTracker) TrackDynamoRead(ctx context.Context, tableName string, units int64) error {
	operation := DynamoOperation{
		Type:               "Read",
		TableName:          tableName,
		ConsumedReadUnits:  units,
		ConsumedWriteUnits: 0,
		ItemCount:          1,
		OperationID:        ut.requestID,
		UserID:             ut.userID,
		Timestamp:          time.Now(),
	}
	
	cost := CalculateDynamoDBCost(float64(units), 0)
	ut.updateCosts("DynamoDB", "Read", cost.TotalMicroCents)
	
	return ut.service.TrackDynamoOperation(ctx, operation)
}

// TrackDynamoWrite tracks a DynamoDB write operation
func (ut *UnifiedTracker) TrackDynamoWrite(ctx context.Context, tableName string, units int64) error {
	operation := DynamoOperation{
		Type:               "Write",
		TableName:          tableName,
		ConsumedReadUnits:  0,
		ConsumedWriteUnits: units,
		ItemCount:          1,
		OperationID:        ut.requestID,
		UserID:             ut.userID,
		Timestamp:          time.Now(),
	}
	
	cost := CalculateDynamoDBCost(0, float64(units))
	ut.updateCosts("DynamoDB", "Write", cost.TotalMicroCents)
	
	return ut.service.TrackDynamoOperation(ctx, operation)
}

// TrackDynamoQuery tracks a DynamoDB query operation
func (ut *UnifiedTracker) TrackDynamoQuery(ctx context.Context, tableName string, units int64) error {
	operation := DynamoOperation{
		Type:               "Query",
		TableName:          tableName,
		ConsumedReadUnits:  units,
		ConsumedWriteUnits: 0,
		ItemCount:          1,
		OperationID:        ut.requestID,
		UserID:             ut.userID,
		Timestamp:          time.Now(),
	}
	
	cost := CalculateDynamoDBCost(float64(units), 0)
	ut.updateCosts("DynamoDB", "Query", cost.TotalMicroCents)
	
	return ut.service.TrackDynamoOperation(ctx, operation)
}

// TrackDynamoScan tracks a DynamoDB scan operation
func (ut *UnifiedTracker) TrackDynamoScan(ctx context.Context, tableName string, units int64) error {
	operation := DynamoOperation{
		Type:               "Scan",
		TableName:          tableName,
		ConsumedReadUnits:  units,
		ConsumedWriteUnits: 0,
		ItemCount:          1,
		OperationID:        ut.requestID,
		UserID:             ut.userID,
		Timestamp:          time.Now(),
	}
	
	cost := CalculateDynamoDBCost(float64(units), 0)
	ut.updateCosts("DynamoDB", "Scan", cost.TotalMicroCents)
	
	return ut.service.TrackDynamoOperation(ctx, operation)
}

// S3 Operations

// TrackS3Get tracks an S3 get operation
func (ut *UnifiedTracker) TrackS3Get(ctx context.Context, bucketName string, bytes int64) error {
	operation := S3Operation{
		Type:             "GetObject",
		BucketName:       bucketName,
		RequestCount:     1,
		BytesTransferred: bytes,
		StorageClass:     "Standard",
		OperationID:      ut.requestID,
		UserID:           ut.userID,
		Timestamp:        time.Now(),
	}
	
	cost := CalculateS3Cost(1, float64(bytes))
	ut.updateCosts("S3", "Get", cost.TotalMicroCents)
	
	return ut.service.TrackS3Operation(ctx, operation)
}

// TrackS3Put tracks an S3 put operation
func (ut *UnifiedTracker) TrackS3Put(ctx context.Context, bucketName string, bytes int64) error {
	operation := S3Operation{
		Type:             "PutObject",
		BucketName:       bucketName,
		RequestCount:     1,
		BytesTransferred: bytes,
		StorageClass:     "Standard",
		OperationID:      ut.requestID,
		UserID:           ut.userID,
		Timestamp:        time.Now(),
	}
	
	cost := CalculateS3Cost(1, float64(bytes))
	ut.updateCosts("S3", "Put", cost.TotalMicroCents)
	
	return ut.service.TrackS3Operation(ctx, operation)
}

// TrackS3Delete tracks an S3 delete operation
func (ut *UnifiedTracker) TrackS3Delete(ctx context.Context, bucketName string) error {
	operation := S3Operation{
		Type:             "DeleteObject",
		BucketName:       bucketName,
		RequestCount:     1,
		BytesTransferred: 0,
		StorageClass:     "Standard",
		OperationID:      ut.requestID,
		UserID:           ut.userID,
		Timestamp:        time.Now(),
	}
	
	cost := CalculateS3Cost(1, 0)
	ut.updateCosts("S3", "Delete", cost.TotalMicroCents)
	
	return ut.service.TrackS3Operation(ctx, operation)
}

// Lambda Operations

// TrackLambdaInvocation tracks a Lambda function invocation
func (ut *UnifiedTracker) TrackLambdaInvocation(ctx context.Context, functionName string, duration time.Duration, memoryMB int64) error {
	operation := LambdaOperation{
		FunctionName: functionName,
		Duration:     duration,
		MemoryMB:     memoryMB,
		ColdStart:    false, // Assume warm start unless specified
		RequestID:    ut.requestID,
		UserID:       ut.userID,
		Timestamp:    time.Now(),
	}
	
	cost := CalculateLambdaCost(duration, memoryMB)
	ut.updateCosts("Lambda", functionName, cost.TotalMicroCents)
	
	return ut.service.TrackLambdaInvocation(ctx, operation)
}

// Metrics and Reporting

// GetCurrentCostMicroCents returns the total accumulated cost in microcents
func (ut *UnifiedTracker) GetCurrentCostMicroCents() int64 {
	ut.mu.RLock()
	defer ut.mu.RUnlock()
	return ut.totalCostMicroCents
}

// GetCostBreakdown returns cost breakdown by service
func (ut *UnifiedTracker) GetCostBreakdown() map[string]int64 {
	ut.mu.RLock()
	defer ut.mu.RUnlock()
	
	// Return a copy to prevent external modification
	breakdown := make(map[string]int64)
	for service, cost := range ut.serviceCosts {
		breakdown[service] = cost
	}
	return breakdown
}

// Reset clears all accumulated cost data
func (ut *UnifiedTracker) Reset() {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	
	ut.totalCostMicroCents = 0
	ut.serviceCosts = make(map[string]int64)
	ut.operationCounts = make(map[string]int64)
}

// GetOperationCounts returns the count of operations by type
func (ut *UnifiedTracker) GetOperationCounts() map[string]int64 {
	ut.mu.RLock()
	defer ut.mu.RUnlock()
	
	// Return a copy to prevent external modification
	counts := make(map[string]int64)
	for operation, count := range ut.operationCounts {
		counts[operation] = count
	}
	return counts
}

// GetCostDollars returns the total cost in dollars
func (ut *UnifiedTracker) GetCostDollars() float64 {
	return float64(ut.GetCurrentCostMicroCents()) / float64(MicroCentsToCents)
}

// Close gracefully shuts down the unified tracker
func (ut *UnifiedTracker) Close(ctx context.Context) error {
	if ut.service != nil {
		return ut.service.Close(ctx)
	}
	return nil
}

// Private helper methods

func (ut *UnifiedTracker) updateCosts(service, operation string, costMicroCents int64) {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	
	ut.totalCostMicroCents += costMicroCents
	ut.serviceCosts[service] += costMicroCents
	ut.operationCounts[operation]++
}

// Factory functions for common use cases

// NewRepositoryTracker creates a cost tracker optimized for repository operations
func NewRepositoryTracker(cloudWatch *cloudwatch.Client, logger *zap.Logger, repositoryName, userID, requestID string) *UnifiedTracker {
	service := NewCostTrackingServiceForRepository(cloudWatch, logger, repositoryName)
	return NewUnifiedTrackerWithService(service, logger, userID, requestID)
}

// NewLambdaUnifiedTracker creates a cost tracker optimized for Lambda function operations
func NewLambdaUnifiedTracker(cloudWatch *cloudwatch.Client, logger *zap.Logger, functionName, userID, requestID string) *UnifiedTracker {
	service := NewCostTrackingServiceForLambda(cloudWatch, logger, functionName)
	return NewUnifiedTrackerWithService(service, logger, userID, requestID)
}

// NewRequestTracker creates a cost tracker for a specific HTTP request
func NewRequestTracker(cloudWatch *cloudwatch.Client, logger *zap.Logger, endpoint, userID, requestID string) *UnifiedTracker {
	config := DefaultTrackingServiceConfig()
	config.CloudWatchNamespace = fmt.Sprintf("Lesser/API/%s", endpoint)
	config.MetricsFlushInterval = 5 * time.Second // Quick flushing for API requests
	
	service := NewTrackingService(cloudWatch, logger, config)
	return NewUnifiedTrackerWithService(service, logger, userID, requestID)
}

// NewBatchTracker creates a cost tracker optimized for batch operations
func NewBatchTracker(cloudWatch *cloudwatch.Client, logger *zap.Logger, batchJobName, userID, requestID string) *UnifiedTracker {
	config := DefaultTrackingServiceConfig()
	config.CloudWatchNamespace = fmt.Sprintf("Lesser/Batch/%s", batchJobName)
	config.MetricsBatchSize = 50 // Larger batch size for batch operations
	config.MetricsFlushInterval = 60 * time.Second // Less frequent flushing for batch jobs
	
	service := NewTrackingService(cloudWatch, logger, config)
	return NewUnifiedTrackerWithService(service, logger, userID, requestID)
}

// Convenience methods for common patterns

// TrackDynamoOperationWithConsumedCapacity tracks a DynamoDB operation using consumed capacity information
func (ut *UnifiedTracker) TrackDynamoOperationWithConsumedCapacity(ctx context.Context, tableName, operationType string, consumedCapacity *ConsumedCapacity) error {
	operation := DynamoOperation{
		Type:               operationType,
		TableName:          tableName,
		ConsumedReadUnits:  int64(consumedCapacity.ReadCapacityUnits),
		ConsumedWriteUnits: int64(consumedCapacity.WriteCapacityUnits),
		ItemCount:          1,
		OperationID:        ut.requestID,
		UserID:             ut.userID,
		Timestamp:          time.Now(),
	}
	
	cost := CalculateDynamoDBCost(consumedCapacity.ReadCapacityUnits, consumedCapacity.WriteCapacityUnits)
	ut.updateCosts("DynamoDB", operationType, cost.TotalMicroCents)
	
	return ut.service.TrackDynamoOperation(ctx, operation)
}

// ConsumedCapacity represents DynamoDB consumed capacity information
type ConsumedCapacity struct {
	TableName           string  // Table that consumed the capacity
	ReadCapacityUnits   float64 // Read capacity units consumed
	WriteCapacityUnits  float64 // Write capacity units consumed
	GlobalSecondaryIndexes map[string]ConsumedCapacity // GSI capacity consumption
	LocalSecondaryIndexes  map[string]ConsumedCapacity // LSI capacity consumption
}

// TrackMultipleOperations tracks multiple operations in a single call for efficiency
func (ut *UnifiedTracker) TrackMultipleOperations(ctx context.Context, operations []OperationInfo) error {
	for _, op := range operations {
		switch op.Type {
		case "DynamoDB.Read":
			if err := ut.TrackDynamoRead(ctx, op.ResourceName, op.Units); err != nil {
				return err
			}
		case "DynamoDB.Write":
			if err := ut.TrackDynamoWrite(ctx, op.ResourceName, op.Units); err != nil {
				return err
			}
		case "S3.Get":
			if err := ut.TrackS3Get(ctx, op.ResourceName, op.Bytes); err != nil {
				return err
			}
		case "S3.Put":
			if err := ut.TrackS3Put(ctx, op.ResourceName, op.Bytes); err != nil {
				return err
			}
		case "Lambda.Invoke":
			if err := ut.TrackLambdaInvocation(ctx, op.ResourceName, op.Duration, op.MemoryMB); err != nil {
				return err
			}
		}
	}
	return nil
}

// OperationInfo represents information about an operation to be tracked
type OperationInfo struct {
	Type         string        // Operation type (e.g., "DynamoDB.Read", "S3.Put")
	ResourceName string        // Resource name (table name, bucket name, function name)
	Units        int64         // Capacity units (for DynamoDB)
	Bytes        int64         // Bytes transferred (for S3)
	Duration     time.Duration // Duration (for Lambda)
	MemoryMB     int64         // Memory allocation (for Lambda)
}