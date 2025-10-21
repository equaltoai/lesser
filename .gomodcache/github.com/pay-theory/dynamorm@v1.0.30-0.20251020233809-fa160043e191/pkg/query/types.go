package query

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// CompiledBatchGet represents a compiled batch get operation
type CompiledBatchGet struct {
	TableName                string
	Keys                     []map[string]types.AttributeValue
	ProjectionExpression     string
	ExpressionAttributeNames map[string]string
	ConsistentRead           bool
}

// CompiledBatchWrite represents a compiled batch write operation
type CompiledBatchWrite struct {
	TableName string
	Items     []map[string]types.AttributeValue
}

// BatchExecutor extends QueryExecutor with batch operations
type BatchExecutor interface {
	QueryExecutor
	ExecuteBatchGet(input *CompiledBatchGet, dest any) error
	ExecuteBatchWrite(input *CompiledBatchWrite) error
}

// QueryResult represents the result of a query operation
type QueryResult struct {
	Items            []map[string]types.AttributeValue
	Count            int64
	ScannedCount     int64
	LastEvaluatedKey map[string]types.AttributeValue
}

// ScanResult represents the result of a scan operation
type ScanResult struct {
	Items            []map[string]types.AttributeValue
	Count            int64
	ScannedCount     int64
	LastEvaluatedKey map[string]types.AttributeValue
}

// BatchGetResult represents the result of a batch get operation
type BatchGetResult struct {
	Responses       []map[string]types.AttributeValue
	UnprocessedKeys []map[string]types.AttributeValue
}

// PaginatedResult represents a paginated query result
type PaginatedResult struct {
	Items        any    `json:"items"`
	NextCursor   string `json:"nextCursor,omitempty"`
	Count        int    `json:"count"`
	HasMore      bool   `json:"hasMore"`
	ScannedCount int    `json:"scannedCount,omitempty"`
}

// CompiledScan represents a compiled scan operation
type CompiledScan struct {
	TableName                 string
	FilterExpression          string
	ProjectionExpression      string
	ExpressionAttributeNames  map[string]string
	ExpressionAttributeValues map[string]types.AttributeValue
	Limit                     *int32
	ExclusiveStartKey         map[string]types.AttributeValue
	ConsistentRead            bool
	Segment                   *int32
	TotalSegments             *int32
}
