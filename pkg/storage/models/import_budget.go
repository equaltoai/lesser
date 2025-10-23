package models

import (
	"fmt"
	"time"
)

// ImportBudget represents budget limits for import/export operations
type ImportBudget struct {
	// Primary keys - user budget records use USER_BUDGET#{username}#{period} pattern
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// GSI1 for period queries - BUDGET#{period}, USER#{username}
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1_pk"`
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1_sk"`

	// Budget configuration
	Username string `json:"username"`
	Period   string `json:"period"` // daily, weekly, monthly

	// Cost limits (all in microcents)
	ImportLimitMicroCents   int64 `json:"import_limit_micro_cents"`   // Maximum import cost per period
	ExportLimitMicroCents   int64 `json:"export_limit_micro_cents"`   // Maximum export cost per period
	CombinedLimitMicroCents int64 `json:"combined_limit_micro_cents"` // Maximum combined cost per period

	// Current usage (resets each period)
	CurrentImportCost   int64 `json:"current_import_cost"`   // Current import spending
	CurrentExportCost   int64 `json:"current_export_cost"`   // Current export spending
	CurrentCombinedCost int64 `json:"current_combined_cost"` // Current combined spending

	// Operation counts
	ImportCount int64 `json:"import_count"` // Number of imports this period
	ExportCount int64 `json:"export_count"` // Number of exports this period

	// Alert configuration
	AlertThresholdPercent float64    `json:"alert_threshold_percent"`   // Alert when usage exceeds this percentage
	AlertSendingEnabled   bool       `json:"alert_sending_enabled"`     // Whether to send alerts
	LastAlertSent         *time.Time `json:"last_alert_sent,omitempty"` // When last alert was sent

	// Status tracking
	IsActive     bool       `json:"is_active"`                // Whether budget enforcement is active
	LastImportAt *time.Time `json:"last_import_at,omitempty"` // When last import occurred
	LastExportAt *time.Time `json:"last_export_at,omitempty"` // When last export occurred
	PeriodStart  time.Time  `json:"period_start"`             // Start of current budget period
	PeriodEnd    time.Time  `json:"period_end"`               // End of current budget period
	NextResetAt  time.Time  `json:"next_reset_at"`            // When budget will reset
	LastResetAt  *time.Time `json:"last_reset_at,omitempty"`  // When budget was last reset

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateKeys sets the primary keys for the ImportBudget model
func (b *ImportBudget) UpdateKeys() {
	b.PK = fmt.Sprintf("USER_BUDGET#%s#%s", b.Username, b.Period)
	b.SK = "CONFIG"
	b.GSI1PK = fmt.Sprintf("BUDGET#%s", b.Period)
	b.GSI1SK = fmt.Sprintf(KeyPatternUser, b.Username)
}

// BeforeCreate is called before creating the record
func (b *ImportBudget) BeforeCreate() error {
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	b.UpdatedAt = now

	// Set period boundaries if not set
	if b.PeriodStart.IsZero() {
		b.PeriodStart = now
	}
	if b.PeriodEnd.IsZero() {
		switch b.Period {
		case PeriodDaily:
			b.PeriodEnd = b.PeriodStart.AddDate(0, 0, 1)
			b.NextResetAt = b.PeriodEnd
			// Set TTL to 7 days for daily budgets
			b.TTL = now.AddDate(0, 0, 7).Unix()
		case PeriodWeekly:
			b.PeriodEnd = b.PeriodStart.AddDate(0, 0, 7)
			b.NextResetAt = b.PeriodEnd
			// Set TTL to 30 days for weekly budgets
			b.TTL = now.AddDate(0, 0, 30).Unix()
		case PeriodMonthly:
			b.PeriodEnd = b.PeriodStart.AddDate(0, 1, 0)
			b.NextResetAt = b.PeriodEnd
			// Set TTL to 90 days for monthly budgets
			b.TTL = now.AddDate(0, 0, 90).Unix()
		}
	}

	// Set defaults
	if b.AlertThresholdPercent == 0 {
		b.AlertThresholdPercent = 80.0
	}

	b.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (b *ImportBudget) BeforeUpdate() error {
	b.UpdatedAt = time.Now()
	b.UpdateKeys()
	return nil
}

// GetImportUsagePercent returns the current import usage as a percentage of limit
func (b *ImportBudget) GetImportUsagePercent() float64 {
	if b.ImportLimitMicroCents == 0 {
		return 0.0
	}
	return (float64(b.CurrentImportCost) / float64(b.ImportLimitMicroCents)) * 100.0
}

// GetExportUsagePercent returns the current export usage as a percentage of limit
func (b *ImportBudget) GetExportUsagePercent() float64 {
	if b.ExportLimitMicroCents == 0 {
		return 0.0
	}
	return (float64(b.CurrentExportCost) / float64(b.ExportLimitMicroCents)) * 100.0
}

// GetCombinedUsagePercent returns the current combined usage as a percentage of limit
func (b *ImportBudget) GetCombinedUsagePercent() float64 {
	if b.CombinedLimitMicroCents == 0 {
		return 0.0
	}
	return (float64(b.CurrentCombinedCost) / float64(b.CombinedLimitMicroCents)) * 100.0
}

// IsImportOverLimit checks if import spending would exceed limit
func (b *ImportBudget) IsImportOverLimit(additionalCost int64) bool {
	if !b.IsActive || b.ImportLimitMicroCents == 0 {
		return false
	}
	return (b.CurrentImportCost + additionalCost) > b.ImportLimitMicroCents
}

// IsExportOverLimit checks if export spending would exceed limit
func (b *ImportBudget) IsExportOverLimit(additionalCost int64) bool {
	if !b.IsActive || b.ExportLimitMicroCents == 0 {
		return false
	}
	return (b.CurrentExportCost + additionalCost) > b.ExportLimitMicroCents
}

// IsCombinedOverLimit checks if combined spending would exceed limit
func (b *ImportBudget) IsCombinedOverLimit(additionalImportCost, additionalExportCost int64) bool {
	if !b.IsActive || b.CombinedLimitMicroCents == 0 {
		return false
	}
	return (b.CurrentCombinedCost + additionalImportCost + additionalExportCost) > b.CombinedLimitMicroCents
}

// ShouldSendAlert checks if an alert should be sent based on usage
func (b *ImportBudget) ShouldSendAlert() bool {
	if !b.AlertSendingEnabled || b.AlertThresholdPercent == 0 {
		return false
	}

	// Don't send alert if we sent one in the last hour
	if b.LastAlertSent != nil && time.Since(*b.LastAlertSent) < time.Hour {
		return false
	}

	// Check if any usage exceeds alert threshold
	importUsage := b.GetImportUsagePercent()
	exportUsage := b.GetExportUsagePercent()
	combinedUsage := b.GetCombinedUsagePercent()

	return importUsage >= b.AlertThresholdPercent ||
		exportUsage >= b.AlertThresholdPercent ||
		combinedUsage >= b.AlertThresholdPercent
}

// NeedsReset checks if the budget period has ended and needs reset
func (b *ImportBudget) NeedsReset() bool {
	return time.Now().After(b.NextResetAt)
}

// Reset resets the budget for a new period
func (b *ImportBudget) Reset() {
	now := time.Now()

	// Reset usage counters
	b.CurrentImportCost = 0
	b.CurrentExportCost = 0
	b.CurrentCombinedCost = 0
	b.ImportCount = 0
	b.ExportCount = 0

	// Update period boundaries
	b.LastResetAt = &now
	b.PeriodStart = now

	switch b.Period {
	case PeriodDaily:
		b.PeriodEnd = now.AddDate(0, 0, 1)
	case PeriodWeekly:
		b.PeriodEnd = now.AddDate(0, 0, 7)
	case PeriodMonthly:
		b.PeriodEnd = now.AddDate(0, 1, 0)
	}

	b.NextResetAt = b.PeriodEnd
	b.UpdatedAt = now
}

// GetRemainingImportBudget returns remaining import budget in microcents
func (b *ImportBudget) GetRemainingImportBudget() int64 {
	if b.ImportLimitMicroCents == 0 {
		return -1 // Unlimited
	}
	remaining := b.ImportLimitMicroCents - b.CurrentImportCost
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetRemainingExportBudget returns remaining export budget in microcents
func (b *ImportBudget) GetRemainingExportBudget() int64 {
	if b.ExportLimitMicroCents == 0 {
		return -1 // Unlimited
	}
	remaining := b.ExportLimitMicroCents - b.CurrentExportCost
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetRemainingCombinedBudget returns remaining combined budget in microcents
func (b *ImportBudget) GetRemainingCombinedBudget() int64 {
	if b.CombinedLimitMicroCents == 0 {
		return -1 // Unlimited
	}
	remaining := b.CombinedLimitMicroCents - b.CurrentCombinedCost
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TableName returns the DynamoDB table backing ImportBudget.
func (ImportBudget) TableName() string {
	return MainTableName
}

// ImportCostSummary represents aggregated import costs (same structure as ExportCostSummary)
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

// TableName returns the DynamoDB table backing ImportCostSummary.
func (ImportCostSummary) TableName() string {
	return MainTableName
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

// TableName returns the DynamoDB table backing ImportTypeCostStats.
func (ImportTypeCostStats) TableName() string {
	return MainTableName
}
