package models

import (
	"fmt"
	"time"
	"github.com/equaltoai/lesser/pkg/common"
)

// ExportCostTracking represents cost tracking for export operations
type ExportCostTracking struct {
	// Primary keys - export cost tracking uses EXPORT_COST#{export_id}#{timestamp} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1 for user queries - USER#{username}, COST#{timestamp}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`

	// GSI2 for date range queries - EXPORT_COSTS#{date}, TS#{timestamp}
	GSI2PK string `dynamorm:"index:GSI2,pk" json:"gsi2_pk"`
	GSI2SK string `dynamorm:"index:GSI2,sk" json:"gsi2_sk"`

	// Export metadata
	ExportID     string `json:"export_id"`
	Username     string `json:"username"`
	Type         string `json:"type"`   // archive, followers, following, etc.
	Format       string `json:"format"` // activitypub, mastodon, csv
	IncludeMedia bool   `json:"include_media"`

	// Cost breakdown (all in microcents)
	LambdaExecutionCost int64 `json:"lambda_execution_cost"` // Lambda compute cost
	LambdaDurationMs    int64 `json:"lambda_duration_ms"`    // Lambda execution time

	S3StorageCost      int64 `json:"s3_storage_cost"`       // S3 storage for export file
	S3PutRequestCost   int64 `json:"s3_put_request_cost"`   // S3 PUT operations
	S3GetRequestCost   int64 `json:"s3_get_request_cost"`   // S3 GET operations for media
	S3DataTransferCost int64 `json:"s3_data_transfer_cost"` // Data transfer costs

	DynamoDBReadCost   int64   `json:"dynamodb_read_cost"`  // DynamoDB read operations
	DynamoDBReadUnits  float64 `json:"dynamodb_read_units"` // Read capacity consumed
	DynamoDBOperations int64   `json:"dynamodb_operations"` // Number of DB operations

	TotalCostMicroCents int64 `json:"total_cost_micro_cents"` // Total cost in microcents

	// Operation metrics
	FileSize           int64 `json:"file_size"`            // Size of exported file in bytes
	RecordCount        int64 `json:"record_count"`         // Number of records exported
	MediaFilesIncluded int64 `json:"media_files_included"` // Number of media files included
	MediaSizeBytes     int64 `json:"media_size_bytes"`     // Total size of media files

	S3PutRequests     int64 `json:"s3_put_requests"`     // Number of S3 PUT requests
	S3GetRequests     int64 `json:"s3_get_requests"`     // Number of S3 GET requests
	DataTransferBytes int64 `json:"data_transfer_bytes"` // Bytes transferred

	// Status tracking
	Status      string     `json:"status"`                 // pending, processing, completed, failed
	StartedAt   time.Time  `json:"started_at"`             // When export processing started
	CompletedAt *time.Time `json:"completed_at,omitempty"` // When export completed

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty"`

	// Timestamps
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

// TableName returns the DynamoDB table name
func (e *ExportCostTracking) TableName() string {
	return "" // Will be set by the repository
}

// ExportCostSummary represents aggregated export costs
type ExportCostSummary struct {
	Username  string    `json:"username"`
	Period    string    `json:"period"` // daily, weekly, monthly
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`

	TotalExports     int64 `json:"total_exports"`
	CompletedExports int64 `json:"completed_exports"`
	FailedExports    int64 `json:"failed_exports"`

	TotalLambdaCost     int64 `json:"total_lambda_cost"`
	TotalS3Cost         int64 `json:"total_s3_cost"`
	TotalDynamoDBCost   int64 `json:"total_dynamodb_cost"`
	TotalCostMicroCents int64 `json:"total_cost_micro_cents"`

	TotalFileSize    int64 `json:"total_file_size"`
	TotalRecordCount int64 `json:"total_record_count"`
	TotalMediaFiles  int64 `json:"total_media_files"`

	AverageCostPerExport float64 `json:"average_cost_per_export"`
	AverageExportSize    int64   `json:"average_export_size"`

	TypeBreakdown map[string]*ExportTypeCostStats `json:"type_breakdown"`
}

// ExportTypeCostStats represents cost statistics for a specific export type
type ExportTypeCostStats struct {
	Type                  string  `json:"type"`
	Count                 int64   `json:"count"`
	TotalCostMicroCents   int64   `json:"total_cost_micro_cents"`
	TotalCostDollars      float64 `json:"total_cost_dollars"`
	AverageCostMicroCents int64   `json:"average_cost_micro_cents"`
	AverageFileSize       int64   `json:"average_file_size"`
	AverageRecordCount    int64   `json:"average_record_count"`
}
