package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// ImportCostTracking represents cost tracking for import operations
type ImportCostTracking struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - import cost tracking uses IMPORT_COST#{import_id}#{timestamp} pattern
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI1 for user queries - USER#{username}, COST#{timestamp}
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsi1PK" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsi1SK" json:"gsi1_sk"`

	// GSI2 for date range queries - IMPORT_COSTS#{date}, TS#{timestamp}
	GSI2PK string `dynamorm:"index:GSI2,pk,attr:gsi2PK" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:GSI2,sk,attr:gsi2SK" json:"gsi2_sk"`

	// Import metadata
	ImportID string `dynamorm:"attr:importID" json:"import_id"`
	Username string `dynamorm:"attr:username" json:"username"`
	Type     string `dynamorm:"attr:type" json:"type"` // followers, following, blocks, mutes, lists, bookmarks, archive
	Mode     string `dynamorm:"attr:mode" json:"mode"` // merge, overwrite

	// Cost breakdown (all in microcents)
	LambdaExecutionCost int64 `dynamorm:"attr:lambdaExecutionCost" json:"lambda_execution_cost"` // Lambda compute cost
	LambdaDurationMs    int64 `dynamorm:"attr:lambdaDurationMs" json:"lambda_duration_ms"`        // Lambda execution time

	S3StorageCost      int64 `dynamorm:"attr:s3StorageCost" json:"s3_storage_cost"`         // S3 storage for import file
	S3GetRequestCost   int64 `dynamorm:"attr:s3GetRequestCost" json:"s3_get_request_cost"`  // S3 GET operations to download file
	S3DataTransferCost int64 `dynamorm:"attr:s3DataTransferCost" json:"s3_data_transfer_cost"` // Data transfer costs

	DynamoDBWriteCost  int64   `dynamorm:"attr:dynamodbWriteCost" json:"dynamodb_write_cost"`    // DynamoDB write operations
	DynamoDBReadCost   int64   `dynamorm:"attr:dynamodbReadCost" json:"dynamodb_read_cost"`      // DynamoDB read operations (for lookups)
	DynamoDBWriteUnits float64 `dynamorm:"attr:dynamodbWriteUnits" json:"dynamodb_write_units"`  // Write capacity consumed
	DynamoDBReadUnits  float64 `dynamorm:"attr:dynamodbReadUnits" json:"dynamodb_read_units"`    // Read capacity consumed
	DynamoDBOperations int64   `dynamorm:"attr:dynamodbOperations" json:"dynamodb_operations"`   // Number of DB operations

	ExternalAPICallCost int64 `dynamorm:"attr:externalAPICallCost" json:"external_api_call_cost"` // Cost of WebFinger/ActivityPub lookups
	ExternalAPICalls    int64 `dynamorm:"attr:externalAPICalls" json:"external_api_calls"`        // Number of external API calls

	TotalCostMicroCents int64 `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"` // Total cost in microcents

	// Operation metrics
	FileSize       int64 `dynamorm:"attr:fileSize" json:"file_size"`             // Size of imported file in bytes
	RecordCount    int64 `dynamorm:"attr:recordCount" json:"record_count"`       // Number of records in import file
	ProcessedCount int64 `dynamorm:"attr:processedCount" json:"processed_count"` // Number of records processed
	SuccessCount   int64 `dynamorm:"attr:successCount" json:"success_count"`     // Number of successful operations
	SkipCount      int64 `dynamorm:"attr:skipCount" json:"skip_count"`           // Number of skipped operations
	ErrorCount     int64 `dynamorm:"attr:errorCount" json:"error_count"`         // Number of failed operations

	S3GetRequests     int64 `dynamorm:"attr:s3GetRequests" json:"s3_get_requests"`         // Number of S3 GET requests
	DataTransferBytes int64 `dynamorm:"attr:dataTransferBytes" json:"data_transfer_bytes"` // Bytes transferred

	// Network operations (for federated follows/blocks)
	DNSLookups   int64 `dynamorm:"attr:dnsLookups" json:"dns_lookups"`     // DNS lookups performed
	HTTPRequests int64 `dynamorm:"attr:httpRequests" json:"http_requests"` // HTTP requests made
	NetworkBytes int64 `dynamorm:"attr:networkBytes" json:"network_bytes"` // Network bytes transferred

	// Status tracking
	Status      string     `dynamorm:"attr:status" json:"status"`                 // pending, processing, completed, failed
	StartedAt   time.Time  `dynamorm:"attr:startedAt" json:"started_at"`          // When import processing started
	CompletedAt *time.Time `dynamorm:"attr:completedAt" json:"completed_at,omitempty"` // When import completed

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Timestamps
	Timestamp time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// UpdateKeys sets the primary keys for the ImportCostTracking model
func (i *ImportCostTracking) UpdateKeys() {
	timestampStr := i.Timestamp.Format(common.CompactTimeFormat)
	i.PK = fmt.Sprintf("IMPORT_COST#%s#%s", i.ImportID, timestampStr)
	i.SK = fmt.Sprintf("COST#%s", timestampStr)
	i.GSI1PK = fmt.Sprintf(KeyPatternUser, i.Username)
	i.GSI1SK = fmt.Sprintf("COST#%s", i.Timestamp.Format(time.RFC3339))
	i.GSI2PK = fmt.Sprintf("IMPORT_COSTS#%s", i.Timestamp.Format(common.CompactDateFormat))
	i.GSI2SK = fmt.Sprintf("TS#%s", timestampStr)
}

// BeforeCreate is called before creating the record
func (i *ImportCostTracking) BeforeCreate() error {
	now := time.Now()
	if i.Timestamp.IsZero() {
		i.Timestamp = now
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = now
	}
	i.UpdatedAt = now

	// Set TTL to 90 days from creation
	i.TTL = now.AddDate(0, 0, 90).Unix()

	i.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (i *ImportCostTracking) BeforeUpdate() error {
	i.UpdatedAt = time.Now()
	i.UpdateKeys()
	return nil
}

// CalculateTotalCost calculates the total cost from all components
func (i *ImportCostTracking) CalculateTotalCost() {
	i.TotalCostMicroCents = i.LambdaExecutionCost +
		i.S3StorageCost +
		i.S3GetRequestCost +
		i.S3DataTransferCost +
		i.DynamoDBWriteCost +
		i.DynamoDBReadCost +
		i.ExternalAPICallCost
}

// GetTotalCostDollars returns the total cost in dollars
func (i *ImportCostTracking) GetTotalCostDollars() float64 {
	return float64(i.TotalCostMicroCents) / 1_000_000.0
}

// GetTimestamp returns the timestamp for cost tracking
func (i *ImportCostTracking) GetTimestamp() time.Time {
	return i.Timestamp
}

// GetTotalCostMicroCents returns the total cost in microcents
func (i *ImportCostTracking) GetTotalCostMicroCents() int64 {
	return i.TotalCostMicroCents
}

// GetSuccessRate calculates the success rate of the import
func (i *ImportCostTracking) GetSuccessRate() float64 {
	if i.ProcessedCount == 0 {
		return 0.0
	}
	return float64(i.SuccessCount) / float64(i.ProcessedCount)
}

// TableName returns the DynamoDB table backing ImportCostTracking.
func (i *ImportCostTracking) TableName() string {
	return MainTableName
}
