package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// ImportCostTracking represents cost tracking for import operations
type ImportCostTracking struct {
	// Primary keys - import cost tracking uses IMPORT_COST#{import_id}#{timestamp} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1 for user queries - USER#{username}, COST#{timestamp}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`

	// GSI2 for date range queries - IMPORT_COSTS#{date}, TS#{timestamp}
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2_sk"`

	// Import metadata
	ImportID string `json:"import_id"`
	Username string `json:"username"`
	Type     string `json:"type"` // followers, following, blocks, mutes, lists, bookmarks, archive
	Mode     string `json:"mode"` // merge, overwrite

	// Cost breakdown (all in microcents)
	LambdaExecutionCost int64 `json:"lambda_execution_cost"` // Lambda compute cost
	LambdaDurationMs    int64 `json:"lambda_duration_ms"`    // Lambda execution time

	S3StorageCost      int64 `json:"s3_storage_cost"`       // S3 storage for import file
	S3GetRequestCost   int64 `json:"s3_get_request_cost"`   // S3 GET operations to download file
	S3DataTransferCost int64 `json:"s3_data_transfer_cost"` // Data transfer costs

	DynamoDBWriteCost  int64   `json:"dynamodb_write_cost"`  // DynamoDB write operations
	DynamoDBReadCost   int64   `json:"dynamodb_read_cost"`   // DynamoDB read operations (for lookups)
	DynamoDBWriteUnits float64 `json:"dynamodb_write_units"` // Write capacity consumed
	DynamoDBReadUnits  float64 `json:"dynamodb_read_units"`  // Read capacity consumed
	DynamoDBOperations int64   `json:"dynamodb_operations"`  // Number of DB operations

	ExternalAPICallCost int64 `json:"external_api_call_cost"` // Cost of WebFinger/ActivityPub lookups
	ExternalAPICalls    int64 `json:"external_api_calls"`     // Number of external API calls

	TotalCostMicroCents int64 `json:"total_cost_micro_cents"` // Total cost in microcents

	// Operation metrics
	FileSize       int64 `json:"file_size"`       // Size of imported file in bytes
	RecordCount    int64 `json:"record_count"`    // Number of records in import file
	ProcessedCount int64 `json:"processed_count"` // Number of records processed
	SuccessCount   int64 `json:"success_count"`   // Number of successful operations
	SkipCount      int64 `json:"skip_count"`      // Number of skipped operations
	ErrorCount     int64 `json:"error_count"`     // Number of failed operations

	S3GetRequests     int64 `json:"s3_get_requests"`     // Number of S3 GET requests
	DataTransferBytes int64 `json:"data_transfer_bytes"` // Bytes transferred

	// Network operations (for federated follows/blocks)
	DNSLookups   int64 `json:"dns_lookups"`   // DNS lookups performed
	HTTPRequests int64 `json:"http_requests"` // HTTP requests made
	NetworkBytes int64 `json:"network_bytes"` // Network bytes transferred

	// Status tracking
	Status      string     `json:"status"`                 // pending, processing, completed, failed
	StartedAt   time.Time  `json:"started_at"`             // When import processing started
	CompletedAt *time.Time `json:"completed_at,omitempty"` // When import completed

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty"`

	// Timestamps
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

// TableName returns the DynamoDB table name
func (i *ImportCostTracking) TableName() string {
	return "" // Will be set by the repository
}
