package cost

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// DynamoDBAPI defines the subset of DynamoDB operations (matching storage package interface)
type DynamoDBAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
}

// DynamoDBCostWrapper wraps a DynamoDB client to track costs
type DynamoDBCostWrapper struct {
	client DynamoDBAPI
}

// NewDynamoDBWrapper creates a new cost-tracking DynamoDB wrapper
func NewDynamoDBWrapper(client DynamoDBAPI) *DynamoDBCostWrapper {
	return &DynamoDBCostWrapper{client: client}
}

// PutItem tracks the cost of a DynamoDB PutItem operation
func (w *DynamoDBCostWrapper) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	// Track write operation (1 write unit per item)
	TrackDynamoWriteContext(ctx, 1)

	output, err := w.client.PutItem(ctx, params, optFns...)

	// Track consumed capacity if available
	if output != nil && output.ConsumedCapacity != nil {
		if output.ConsumedCapacity.WriteCapacityUnits != nil {
			TrackDynamoWriteContext(ctx, int(*output.ConsumedCapacity.WriteCapacityUnits))
		}
	}

	return output, err
}

// GetItem tracks the cost of a DynamoDB GetItem operation
func (w *DynamoDBCostWrapper) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	// Track read operation (1 read unit for consistent read, 0.5 for eventual)
	reads := 1
	if params.ConsistentRead != nil && !*params.ConsistentRead {
		// For simplicity, still count as 1 read but could optimize later
		reads = 1
	}
	TrackDynamoReadContext(ctx, reads)

	output, err := w.client.GetItem(ctx, params, optFns...)

	// Track consumed capacity if available
	if output != nil && output.ConsumedCapacity != nil {
		if output.ConsumedCapacity.ReadCapacityUnits != nil {
			TrackDynamoReadContext(ctx, int(*output.ConsumedCapacity.ReadCapacityUnits))
		}
	}

	return output, err
}

// UpdateItem tracks the cost of a DynamoDB UpdateItem operation
func (w *DynamoDBCostWrapper) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	// Track write operation
	TrackDynamoWriteContext(ctx, 1)

	output, err := w.client.UpdateItem(ctx, params, optFns...)

	// Track consumed capacity if available
	if output != nil && output.ConsumedCapacity != nil {
		if output.ConsumedCapacity.WriteCapacityUnits != nil {
			TrackDynamoWriteContext(ctx, int(*output.ConsumedCapacity.WriteCapacityUnits))
		}
		if output.ConsumedCapacity.ReadCapacityUnits != nil {
			TrackDynamoReadContext(ctx, int(*output.ConsumedCapacity.ReadCapacityUnits))
		}
	}

	return output, err
}

// DeleteItem tracks the cost of a DynamoDB DeleteItem operation
func (w *DynamoDBCostWrapper) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	// Track write operation
	TrackDynamoWriteContext(ctx, 1)

	output, err := w.client.DeleteItem(ctx, params, optFns...)

	// Track consumed capacity if available
	if output != nil && output.ConsumedCapacity != nil {
		if output.ConsumedCapacity.WriteCapacityUnits != nil {
			TrackDynamoWriteContext(ctx, int(*output.ConsumedCapacity.WriteCapacityUnits))
		}
	}

	return output, err
}

// Query tracks the cost of a DynamoDB Query operation
func (w *DynamoDBCostWrapper) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	output, err := w.client.Query(ctx, params, optFns...)

	// Track consumed capacity if available
	if output != nil && output.ConsumedCapacity != nil {
		if output.ConsumedCapacity.ReadCapacityUnits != nil {
			TrackDynamoReadContext(ctx, int(*output.ConsumedCapacity.ReadCapacityUnits))
		}
	} else if output != nil {
		// Estimate based on returned items if capacity not available
		TrackDynamoReadContext(ctx, int(output.Count))
	}

	return output, err
}

// Scan tracks the cost of a DynamoDB Scan operation
func (w *DynamoDBCostWrapper) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	output, err := w.client.Scan(ctx, params, optFns...)

	// Track consumed capacity if available
	if output != nil && output.ConsumedCapacity != nil {
		if output.ConsumedCapacity.ReadCapacityUnits != nil {
			TrackDynamoReadContext(ctx, int(*output.ConsumedCapacity.ReadCapacityUnits))
		}
	} else if output != nil {
		// Scan consumes capacity for all scanned items, not just returned
		TrackDynamoReadContext(ctx, int(output.ScannedCount))
	}

	return output, err
}

// BatchWriteItem tracks the cost of a DynamoDB BatchWriteItem operation
func (w *DynamoDBCostWrapper) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	// Count items to be written
	itemCount := 0
	for _, items := range params.RequestItems {
		itemCount += len(items)
	}

	// Track estimated writes
	if itemCount > 0 {
		TrackDynamoWriteContext(ctx, itemCount)
	}

	output, err := w.client.BatchWriteItem(ctx, params, optFns...)

	// Track consumed capacity if available
	if output != nil && output.ConsumedCapacity != nil {
		totalWrites := 0
		for _, cap := range output.ConsumedCapacity {
			if cap.WriteCapacityUnits != nil {
				totalWrites += int(*cap.WriteCapacityUnits)
			}
		}
		if totalWrites > 0 {
			// Adjust tracking based on actual consumption
			TrackDynamoWriteContext(ctx, totalWrites-itemCount)
		}
	}

	return output, err
}

// BatchGetItem tracks the cost of a DynamoDB BatchGetItem operation
func (w *DynamoDBCostWrapper) BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	// Count items to be read
	itemCount := 0
	for _, keys := range params.RequestItems {
		itemCount += len(keys.Keys)
	}

	// Track estimated reads
	if itemCount > 0 {
		TrackDynamoReadContext(ctx, itemCount)
	}

	output, err := w.client.BatchGetItem(ctx, params, optFns...)

	// Track consumed capacity if available
	if output != nil && output.ConsumedCapacity != nil {
		totalReads := 0
		for _, cap := range output.ConsumedCapacity {
			if cap.ReadCapacityUnits != nil {
				totalReads += int(*cap.ReadCapacityUnits)
			}
		}
		if totalReads > 0 {
			// Adjust tracking based on actual consumption
			TrackDynamoReadContext(ctx, totalReads-itemCount)
		}
	}

	return output, err
}
