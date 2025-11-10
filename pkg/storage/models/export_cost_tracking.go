package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// ExportCostTracking represents cost tracking for export operations
type ExportCostTracking struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - export cost tracking uses EXPORT_COST#{export_id}#{timestamp} pattern
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI1 for user queries - USER#{username}, COST#{timestamp}
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsI1PK" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsI1SK" json:"gsi1_sk"`

	// GSI2 for date range queries - EXPORT_COSTS#{date}, TS#{timestamp}
	GSI2PK string `dynamorm:"index:GSI2,pk,attr:gsI2PK" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:GSI2,sk,attr:gsI2SK" json:"gsi2_sk"`

	// Export metadata
	ExportID     string `dynamorm:"attr:exportID" json:"export_id"`
	Username     string `dynamorm:"attr:username" json:"username"`
	Type         string `dynamorm:"attr:type" json:"type"`     // archive, followers, following, etc.
	Format       string `dynamorm:"attr:format" json:"format"` // activitypub, mastodon, csv
	IncludeMedia bool   `dynamorm:"attr:includeMedia" json:"include_media"`

	// Cost breakdown (all in microcents)
	LambdaExecutionCost int64 `dynamorm:"attr:lambdaExecutionCost" json:"lambda_execution_cost"` // Lambda compute cost
	LambdaDurationMs    int64 `dynamorm:"attr:lambdaDurationMs" json:"lambda_duration_ms"`       // Lambda execution time

	S3StorageCost      int64 `dynamorm:"attr:s3StorageCost" json:"s3_storage_cost"`            // S3 storage for export file
	S3PutRequestCost   int64 `dynamorm:"attr:s3PutRequestCost" json:"s3_put_request_cost"`     // S3 PUT operations
	S3GetRequestCost   int64 `dynamorm:"attr:s3GetRequestCost" json:"s3_get_request_cost"`     // S3 GET operations for media
	S3DataTransferCost int64 `dynamorm:"attr:s3DataTransferCost" json:"s3_data_transfer_cost"` // Data transfer costs

	DynamoDBReadCost   int64   `dynamorm:"attr:dynamoDBReadCost" json:"dynamodb_read_cost"`    // DynamoDB read operations
	DynamoDBReadUnits  float64 `dynamorm:"attr:dynamoDBReadUnits" json:"dynamodb_read_units"`  // Read capacity consumed
	DynamoDBOperations int64   `dynamorm:"attr:dynamoDBOperations" json:"dynamodb_operations"` // Number of DB operations

	TotalCostMicroCents int64 `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"` // Total cost in microcents

	// Operation metrics
	FileSize           int64 `dynamorm:"attr:fileSize" json:"file_size"`                      // Size of exported file in bytes
	RecordCount        int64 `dynamorm:"attr:recordCount" json:"record_count"`                // Number of records exported
	MediaFilesIncluded int64 `dynamorm:"attr:mediaFilesIncluded" json:"media_files_included"` // Number of media files included
	MediaSizeBytes     int64 `dynamorm:"attr:mediaSizeBytes" json:"media_size_bytes"`         // Total size of media files

	S3PutRequests     int64 `dynamorm:"attr:s3PutRequests" json:"s3_put_requests"`         // Number of S3 PUT requests
	S3GetRequests     int64 `dynamorm:"attr:s3GetRequests" json:"s3_get_requests"`         // Number of S3 GET requests
	DataTransferBytes int64 `dynamorm:"attr:dataTransferBytes" json:"data_transfer_bytes"` // Bytes transferred

	// Status tracking
	Status      string     `dynamorm:"attr:status" json:"status"`                      // pending, processing, completed, failed
	StartedAt   time.Time  `dynamorm:"attr:startedAt" json:"started_at"`               // When export processing started
	CompletedAt *time.Time `dynamorm:"attr:completedAt" json:"completed_at,omitempty"` // When export completed

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Timestamps
	Timestamp time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// UpdateKeys sets the primary keys for the ExportCostTracking model
func (e *ExportCostTracking) UpdateKeys() {
	timestampStr := e.Timestamp.Format(common.CompactTimeFormat)
	e.PK = fmt.Sprintf("EXPORT_COST#%s#%s", e.ExportID, timestampStr)
	e.SK = fmt.Sprintf("COST#%s", timestampStr)
	e.GSI1PK = fmt.Sprintf(KeyPatternUser, e.Username)
	e.GSI1SK = fmt.Sprintf("COST#%s", e.Timestamp.Format(time.RFC3339))
	e.GSI2PK = fmt.Sprintf("EXPORT_COSTS#%s", e.Timestamp.Format(common.CompactDateFormat))
	e.GSI2SK = fmt.Sprintf("TS#%s", timestampStr)
}

// BeforeCreate is called before creating the record
func (e *ExportCostTracking) BeforeCreate() error {
	now := time.Now()
	if e.Timestamp.IsZero() {
		e.Timestamp = now
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now

	// Set TTL to 90 days from creation
	e.TTL = now.AddDate(0, 0, 90).Unix()

	e.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (e *ExportCostTracking) BeforeUpdate() error {
	e.UpdatedAt = time.Now()
	e.UpdateKeys()
	return nil
}

// CalculateTotalCost calculates the total cost from all components
func (e *ExportCostTracking) CalculateTotalCost() {
	e.TotalCostMicroCents = e.LambdaExecutionCost +
		e.S3StorageCost +
		e.S3PutRequestCost +
		e.S3GetRequestCost +
		e.S3DataTransferCost +
		e.DynamoDBReadCost
}

// GetTotalCostDollars returns the total cost in dollars
func (e *ExportCostTracking) GetTotalCostDollars() float64 {
	return float64(e.TotalCostMicroCents) / 1_000_000.0
}

// GetTimestamp returns the timestamp for cost tracking
func (e *ExportCostTracking) GetTimestamp() time.Time {
	return e.Timestamp
}

// GetTotalCostMicroCents returns the total cost in microcents
func (e *ExportCostTracking) GetTotalCostMicroCents() int64 {
	return e.TotalCostMicroCents
}

// TableName returns the DynamoDB table backing ExportCostTracking.
func (e *ExportCostTracking) TableName() string {
	return MainTableName
}

// ExportCostSummary represents aggregated export costs
type ExportCostSummary struct {
	Username  string    `dynamorm:"attr:username" json:"username"`
	Period    string    `dynamorm:"attr:period" json:"period"` // daily, weekly, monthly
	StartDate time.Time `dynamorm:"attr:startDate" json:"start_date"`
	EndDate   time.Time `dynamorm:"attr:endDate" json:"end_date"`

	TotalExports     int64 `dynamorm:"attr:totalExports" json:"total_exports"`
	CompletedExports int64 `dynamorm:"attr:completedExports" json:"completed_exports"`
	FailedExports    int64 `dynamorm:"attr:failedExports" json:"failed_exports"`

	TotalLambdaCost     int64 `dynamorm:"attr:totalLambdaCost" json:"total_lambda_cost"`
	TotalS3Cost         int64 `dynamorm:"attr:totalS3Cost" json:"total_s3_cost"`
	TotalDynamoDBCost   int64 `dynamorm:"attr:totalDynamoDBCost" json:"total_dynamodb_cost"`
	TotalCostMicroCents int64 `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`

	TotalFileSize    int64 `dynamorm:"attr:totalFileSize" json:"total_file_size"`
	TotalRecordCount int64 `dynamorm:"attr:totalRecordCount" json:"total_record_count"`
	TotalMediaFiles  int64 `dynamorm:"attr:totalMediaFiles" json:"total_media_files"`

	AverageCostPerExport float64 `dynamorm:"attr:averageCostPerExport" json:"average_cost_per_export"`
	AverageExportSize    int64   `dynamorm:"attr:averageExportSize" json:"average_export_size"`

	TypeBreakdown map[string]*ExportTypeCostStats `dynamorm:"attr:typeBreakdown" json:"type_breakdown"`
}

// TableName returns the DynamoDB table backing ExportCostSummary.
func (ExportCostSummary) TableName() string {
	return MainTableName
}

// ExportTypeCostStats represents cost statistics for a specific export type
type ExportTypeCostStats struct {
	Type                  string  `dynamorm:"attr:type" json:"type"`
	Count                 int64   `dynamorm:"attr:count" json:"count"`
	TotalCostMicroCents   int64   `dynamorm:"attr:totalCostMicroCents" json:"total_cost_micro_cents"`
	TotalCostDollars      float64 `dynamorm:"attr:totalCostDollars" json:"total_cost_dollars"`
	AverageCostMicroCents int64   `dynamorm:"attr:averageCostMicroCents" json:"average_cost_micro_cents"`
	AverageFileSize       int64   `dynamorm:"attr:averageFileSize" json:"average_file_size"`
	AverageRecordCount    int64   `dynamorm:"attr:averageRecordCount" json:"average_record_count"`
}

// TableName returns the DynamoDB table backing ExportTypeCostStats.
func (ExportTypeCostStats) TableName() string {
	return MainTableName
}
