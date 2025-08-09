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

// ImportCostSummary represents aggregated import costs
type ImportCostSummary struct {
	Username  string    `json:"username"`
	Period    string    `json:"period"` // daily, weekly, monthly
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`

	TotalImports     int64 `json:"total_imports"`
	CompletedImports int64 `json:"completed_imports"`
	FailedImports    int64 `json:"failed_imports"`

	TotalLambdaCost     int64 `json:"total_lambda_cost"`
	TotalS3Cost         int64 `json:"total_s3_cost"`
	TotalDynamoDBCost   int64 `json:"total_dynamodb_cost"`
	TotalNetworkCost    int64 `json:"total_network_cost"`
	TotalCostMicroCents int64 `json:"total_cost_micro_cents"`

	TotalRecordsProcessed int64 `json:"total_records_processed"`
	TotalRecordsSucceeded int64 `json:"total_records_succeeded"`
	TotalRecordsFailed    int64 `json:"total_records_failed"`

	AverageCostPerImport float64 `json:"average_cost_per_import"`
	AverageCostPerRecord float64 `json:"average_cost_per_record"`
	OverallSuccessRate   float64 `json:"overall_success_rate"`

	TypeBreakdown map[string]*ImportTypeCostStats `json:"type_breakdown"`
}

// ImportTypeCostStats represents cost statistics for a specific import type
type ImportTypeCostStats struct {
	Type                  string  `json:"type"`
	Count                 int64   `json:"count"`
	TotalCostMicroCents   int64   `json:"total_cost_micro_cents"`
	TotalCostDollars      float64 `json:"total_cost_dollars"`
	AverageCostMicroCents int64   `json:"average_cost_micro_cents"`
	TotalRecords          int64   `json:"total_records"`
	SuccessfulRecords     int64   `json:"successful_records"`
	FailedRecords         int64   `json:"failed_records"`
	SuccessRate           float64 `json:"success_rate"`
}

// ImportBudget represents budget limits for import operations
type ImportBudget struct {
	// Primary keys - import budgets use USER_BUDGET#{username}#{period} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// Budget configuration
	Username string `json:"username"`
	Period   string `json:"period"` // daily, weekly, monthly

	// Limits (in microcents)
	ImportLimitMicroCents   int64 `json:"import_limit_micro_cents"`
	ExportLimitMicroCents   int64 `json:"export_limit_micro_cents"`
	CombinedLimitMicroCents int64 `json:"combined_limit_micro_cents"`

	// Current usage (reset per period)
	CurrentImportCost   int64 `json:"current_import_cost"`
	CurrentExportCost   int64 `json:"current_export_cost"`
	CurrentCombinedCost int64 `json:"current_combined_cost"`

	// Usage tracking
	ImportCount  int64      `json:"import_count"`
	ExportCount  int64      `json:"export_count"`
	LastImportAt *time.Time `json:"last_import_at,omitempty"`
	LastExportAt *time.Time `json:"last_export_at,omitempty"`

	// Period tracking
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`

	// Alert settings
	AlertThresholdPercent float64    `json:"alert_threshold_percent"` // Send alert at this % of limit
	AlertSendingEnabled   bool       `json:"alert_sending_enabled"`
	LastAlertSentAt       *time.Time `json:"last_alert_sent_at,omitempty"`

	// Status
	IsActive bool `json:"is_active"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateKeys sets the primary keys for the ImportBudget model
func (b *ImportBudget) UpdateKeys() {
	b.PK = fmt.Sprintf("USER_BUDGET#%s#%s", b.Username, b.Period)
	b.SK = SKConfig
}

// BeforeCreate is called before creating the record
func (b *ImportBudget) BeforeCreate() error {
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	b.UpdatedAt = now
	b.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (b *ImportBudget) BeforeUpdate() error {
	b.UpdatedAt = time.Now()
	b.UpdateKeys()
	return nil
}

// IsOverImportLimit checks if the user is over their import limit
func (b *ImportBudget) IsOverImportLimit() bool {
	return b.CurrentImportCost >= b.ImportLimitMicroCents
}

// IsOverExportLimit checks if the user is over their export limit
func (b *ImportBudget) IsOverExportLimit() bool {
	return b.CurrentExportCost >= b.ExportLimitMicroCents
}

// IsOverCombinedLimit checks if the user is over their combined limit
func (b *ImportBudget) IsOverCombinedLimit() bool {
	return b.CurrentCombinedCost >= b.CombinedLimitMicroCents
}

// GetImportUsagePercent returns import usage as a percentage of limit
func (b *ImportBudget) GetImportUsagePercent() float64 {
	if b.ImportLimitMicroCents == 0 {
		return 0.0
	}
	return (float64(b.CurrentImportCost) / float64(b.ImportLimitMicroCents)) * 100.0
}

// GetExportUsagePercent returns export usage as a percentage of limit
func (b *ImportBudget) GetExportUsagePercent() float64 {
	if b.ExportLimitMicroCents == 0 {
		return 0.0
	}
	return (float64(b.CurrentExportCost) / float64(b.ExportLimitMicroCents)) * 100.0
}

// GetCombinedUsagePercent returns combined usage as a percentage of limit
func (b *ImportBudget) GetCombinedUsagePercent() float64 {
	if b.CombinedLimitMicroCents == 0 {
		return 0.0
	}
	return (float64(b.CurrentCombinedCost) / float64(b.CombinedLimitMicroCents)) * 100.0
}

// ShouldSendAlert checks if an alert should be sent
func (b *ImportBudget) ShouldSendAlert() bool {
	if !b.AlertSendingEnabled {
		return false
	}

	// Check if we've exceeded the alert threshold
	if b.GetCombinedUsagePercent() < b.AlertThresholdPercent {
		return false
	}

	// Check if we've already sent an alert recently (within 1 hour)
	if b.LastAlertSentAt != nil && time.Since(*b.LastAlertSentAt) < time.Hour {
		return false
	}

	return true
}

// TableName returns the DynamoDB table name
func (b *ImportBudget) TableName() string {
	return "" // Will be set by the repository
}
