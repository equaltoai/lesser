package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// MediaSpending tracks spending and costs for media processing operations per user
type MediaSpending struct {
	// Primary key - using user ID as partition key with time period as sort key
	PK string `dynamorm:"pk" json:"pk"` // Format: "MEDIA_SPENDING#{userID}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "PERIOD#{year}-{month}" or "DAILY#{year}-{month}-{day}"

	// GSI1 - Global spending queries across all users
	GSI1PK string `dynamorm:"index:spending-time-index,pk" json:"gsi1_pk"` // Format: "SPENDING#{period_type}"
	GSI1SK string `dynamorm:"index:spending-time-index,sk" json:"gsi1_sk"` // Format: "{year}-{month}-{day}#{userID}"

	// GSI2 - Cost category queries
	GSI2PK string `dynamorm:"index:cost-category-index,pk" json:"gsi2_pk"` // Format: "COST_CATEGORY#{category}"
	GSI2SK string `dynamorm:"index:cost-category-index,sk" json:"gsi2_sk"` // Format: "{timestamp}#{userID}"

	// Core spending data
	UserID     string `json:"user_id"`
	Username   string `json:"username"`
	Period     string `json:"period"`      // "2024-01", "2024-01-15" (monthly or daily)
	PeriodType string `json:"period_type"` // "monthly", "daily"

	// Spending totals (in microdollars - $1 = 1,000,000 microdollars)
	TotalSpendMicros      int64 `json:"total_spend_micros"`
	ProcessingSpendMicros int64 `json:"processing_spend_micros"` // MediaConvert, Rekognition, etc.
	StorageSpendMicros    int64 `json:"storage_spend_micros"`    // S3 storage costs
	BandwidthSpendMicros  int64 `json:"bandwidth_spend_micros"`  // CloudFront/CDN costs
	ComputeSpendMicros    int64 `json:"compute_spend_micros"`    // Lambda compute costs

	// Operation counts
	TotalOperations    int64 `json:"total_operations"`
	ImageProcessingOps int64 `json:"image_processing_ops"`
	VideoProcessingOps int64 `json:"video_processing_ops"`
	AudioProcessingOps int64 `json:"audio_processing_ops"`
	StorageOperations  int64 `json:"storage_operations"` // S3 PUT/GET operations
	BandwidthBytes     int64 `json:"bandwidth_bytes"`    // Bytes transferred
	ComputeTimeMs      int64 `json:"compute_time_ms"`    // Lambda execution time

	// Detailed cost breakdown by service
	S3StorageCostMicros    int64 `json:"s3_storage_cost_micros"`
	S3RequestCostMicros    int64 `json:"s3_request_cost_micros"`
	CloudFrontCostMicros   int64 `json:"cloudfront_cost_micros"`
	MediaConvertCostMicros int64 `json:"mediaconvert_cost_micros"`
	RekognitionCostMicros  int64 `json:"rekognition_cost_micros"`
	LambdaCostMicros       int64 `json:"lambda_cost_micros"`

	// Usage statistics
	FilesProcessed      int64 `json:"files_processed"`
	BytesProcessed      int64 `json:"bytes_processed"`
	StorageBytesUsed    int64 `json:"storage_bytes_used"`
	MediaConvertMinutes int64 `json:"mediaconvert_minutes"` // Total minutes processed
	RekognitionImages   int64 `json:"rekognition_images"`   // Images analyzed
	ThumbnailsGenerated int64 `json:"thumbnails_generated"`

	// Error tracking
	FailedOperations int64 `json:"failed_operations"`
	ErrorCostMicros  int64 `json:"error_cost_micros"` // Costs from failed operations

	// Budget tracking
	BudgetLimitMicros  int64      `json:"budget_limit_micros"`          // User's budget limit
	BudgetUsagePercent float64    `json:"budget_usage_percent"`         // Percentage of budget used
	BudgetExceeded     bool       `json:"budget_exceeded"`              // True if over budget
	BudgetExceededAt   *time.Time `json:"budget_exceeded_at,omitempty"` // When budget was exceeded

	// Timestamps
	PeriodStartAt time.Time `json:"period_start_at"`
	PeriodEndAt   time.Time `json:"period_end_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// TTL for old spending records (keep for 2 years)
	ExpiresAt *int64 `dynamorm:"ttl" json:"expires_at,omitempty"` // Unix timestamp

	// Version for optimistic locking
	ModelVersion int `dynamorm:"version" json:"model_version"`
}

// MediaSpendingTransaction represents a single spending transaction
type MediaSpendingTransaction struct {
	// Primary key - using user ID as partition key with transaction ID as sort key
	PK string `dynamorm:"pk" json:"pk"` // Format: "SPENDING_TXN#{userID}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "TXN#{timestamp}#{transactionID}"

	// GSI1 - Time-based transaction queries
	GSI1PK string `dynamorm:"index:transaction-time-index,pk" json:"gsi1_pk"` // Format: "TXN_TIME#{date}"
	GSI1SK string `dynamorm:"index:transaction-time-index,sk" json:"gsi1_sk"` // Format: "{timestamp}#{userID}#{transactionID}"

	// Core transaction data
	TransactionID string `json:"transaction_id"`
	UserID        string `json:"user_id"`
	Username      string `json:"username"`

	// Cost details
	CostMicros  int64  `json:"cost_micros"` // Cost in microdollars
	Category    string `json:"category"`    // "processing", "storage", "bandwidth", "compute"
	Service     string `json:"service"`     // "s3", "mediaconvert", "cloudfront", "lambda", "rekognition"
	Operation   string `json:"operation"`   // "image_resize", "video_transcode", "storage_put", etc.
	Description string `json:"description"` // Human-readable description

	// Associated media
	MediaID     string `json:"media_id,omitempty"`
	JobID       string `json:"job_id,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
	ContentType string `json:"content_type,omitempty"`

	// Processing details
	ProcessingTimeMs int64 `json:"processing_time_ms,omitempty"` // How long the operation took
	BytesProcessed   int64 `json:"bytes_processed,omitempty"`    // Bytes processed
	UnitsConsumed    int64 `json:"units_consumed,omitempty"`     // Service-specific units

	// Error information
	IsError      bool   `json:"is_error"`
	ErrorMessage string `json:"error_message,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`

	// TTL for old transactions (keep for 1 year)
	ExpiresAt *int64 `dynamorm:"ttl" json:"expires_at,omitempty"` // Unix timestamp
}

// TableName returns the DynamoDB table name for MediaSpending
func (MediaSpending) TableName() string {
	return MainTableName // Use the main table
}

// TableName returns the DynamoDB table name for MediaSpendingTransaction
func (MediaSpendingTransaction) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the MediaSpending model before creation
func (ms *MediaSpending) BeforeCreate() error {
	now := time.Now()
	ms.CreatedAt = now
	ms.UpdatedAt = now

	// Set up primary key
	ms.PK = "MEDIA_SPENDING#" + ms.UserID
	ms.SK = "PERIOD#" + ms.Period

	// Set up GSI keys
	ms.setupGSIKeys()

	// Set TTL (2 years)
	expires := now.Add(2 * 365 * 24 * time.Hour).Unix()
	ms.ExpiresAt = &expires

	// Set period start/end times
	if err := ms.setPeriodTimes(); err != nil {
		return err
	}

	return ms.Validate()
}

// BeforeUpdate sets up the model before update
func (ms *MediaSpending) BeforeUpdate() error {
	ms.UpdatedAt = time.Now()

	// Update budget usage percentage
	if ms.BudgetLimitMicros > 0 {
		ms.BudgetUsagePercent = float64(ms.TotalSpendMicros) / float64(ms.BudgetLimitMicros) * 100.0
		ms.BudgetExceeded = ms.TotalSpendMicros > ms.BudgetLimitMicros

		// Set budget exceeded timestamp if just exceeded
		if ms.BudgetExceeded && ms.BudgetExceededAt == nil {
			now := time.Now()
			ms.BudgetExceededAt = &now
		}
	}

	// Update GSI keys
	ms.setupGSIKeys()

	return ms.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys for MediaSpending
func (ms *MediaSpending) setupGSIKeys() {
	// GSI1 - Global spending queries across time
	ms.GSI1PK = "SPENDING#" + ms.PeriodType
	ms.GSI1SK = fmt.Sprintf("%s#%s", ms.Period, ms.UserID)

	// GSI2 - Cost category queries (use largest cost category)
	largestCategory := ms.getLargestCostCategory()
	if largestCategory != "" {
		ms.GSI2PK = "COST_CATEGORY#" + largestCategory
		ms.GSI2SK = fmt.Sprintf("%s#%s", ms.CreatedAt.Format(time.RFC3339), ms.UserID)
	}
}

// getLargestCostCategory returns the category with the highest spending
func (ms *MediaSpending) getLargestCostCategory() string {
	maxCost := int64(0)
	category := ""

	if ms.ProcessingSpendMicros > maxCost {
		maxCost = ms.ProcessingSpendMicros
		category = ResourceProcessing
	}
	if ms.StorageSpendMicros > maxCost {
		maxCost = ms.StorageSpendMicros
		category = ResourceStorage
	}
	if ms.BandwidthSpendMicros > maxCost {
		maxCost = ms.BandwidthSpendMicros
		category = ResourceBandwidth
	}
	if ms.ComputeSpendMicros > maxCost {
		category = ResourceCompute
	}

	return category
}

// setPeriodTimes sets the period start and end times based on the period string
func (ms *MediaSpending) setPeriodTimes() error {
	switch ms.PeriodType {
	case PeriodMonthly:
		// Parse YYYY-MM format
		t, err := time.Parse(common.MonthFormat, ms.Period)
		if err != nil {
			return fmt.Errorf("invalid monthly period format: %s", ms.Period)
		}
		ms.PeriodStartAt = t
		ms.PeriodEndAt = t.AddDate(0, 1, 0).Add(-time.Nanosecond) // End of month
	case PeriodDaily:
		// Parse YYYY-MM-DD format
		t, err := time.Parse(common.DateFormat, ms.Period)
		if err != nil {
			return fmt.Errorf("invalid daily period format: %s", ms.Period)
		}
		ms.PeriodStartAt = t
		ms.PeriodEndAt = t.Add(24*time.Hour - time.Nanosecond) // End of day
	default:
		return fmt.Errorf("invalid period type: %s", ms.PeriodType)
	}

	return nil
}

// Validate performs validation on the MediaSpending
func (ms *MediaSpending) Validate() error {
	if strings.TrimSpace(ms.UserID) == "" {
		return fmt.Errorf("UserID is required")
	}
	if strings.TrimSpace(ms.Period) == "" {
		return fmt.Errorf("period is required")
	}
	if ms.PeriodType != PeriodMonthly && ms.PeriodType != PeriodDaily {
		return fmt.Errorf("PeriodType must be 'monthly' or 'daily'")
	}

	// Validate that individual spending amounts sum to total
	calculatedTotal := ms.ProcessingSpendMicros + ms.StorageSpendMicros +
		ms.BandwidthSpendMicros + ms.ComputeSpendMicros
	if ms.TotalSpendMicros != calculatedTotal {
		ms.TotalSpendMicros = calculatedTotal // Auto-correct
	}

	// Validate spending amounts are non-negative
	if ms.TotalSpendMicros < 0 || ms.ProcessingSpendMicros < 0 ||
		ms.StorageSpendMicros < 0 || ms.BandwidthSpendMicros < 0 ||
		ms.ComputeSpendMicros < 0 {
		return fmt.Errorf("spending amounts cannot be negative")
	}

	return nil
}

// BeforeCreate sets up the MediaSpendingTransaction model before creation
func (mst *MediaSpendingTransaction) BeforeCreate() error {
	now := time.Now()
	mst.CreatedAt = now

	// Generate transaction ID if not provided
	if mst.TransactionID == "" {
		mst.TransactionID = fmt.Sprintf("txn_%d_%s", now.UnixNano(), mst.UserID[:8])
	}

	// Set up primary key
	mst.PK = "SPENDING_TXN#" + mst.UserID
	mst.SK = fmt.Sprintf("TXN#%s#%s", now.Format("20060102150405"), mst.TransactionID)

	// Set up GSI keys
	dateStr := now.Format(common.DateFormat)
	mst.GSI1PK = "TXN_TIME#" + dateStr
	mst.GSI1SK = fmt.Sprintf("%s#%s#%s", now.Format(time.RFC3339), mst.UserID, mst.TransactionID)

	// Set TTL (1 year)
	expires := now.Add(365 * 24 * time.Hour).Unix()
	mst.ExpiresAt = &expires

	return mst.Validate()
}

// Validate performs validation on the MediaSpendingTransaction
func (mst *MediaSpendingTransaction) Validate() error {
	if strings.TrimSpace(mst.UserID) == "" {
		return fmt.Errorf("UserID is required")
	}
	if strings.TrimSpace(mst.TransactionID) == "" {
		return fmt.Errorf("TransactionID is required")
	}
	if mst.CostMicros < 0 {
		return fmt.Errorf("CostMicros cannot be negative")
	}
	if strings.TrimSpace(mst.Category) == "" {
		return fmt.Errorf("category is required")
	}

	// Validate category
	validCategories := map[string]bool{
		ResourceProcessing: true, ResourceStorage: true, ResourceBandwidth: true, ResourceCompute: true,
	}
	if !validCategories[mst.Category] {
		return fmt.Errorf("invalid category: %s", mst.Category)
	}

	return nil
}

// AddSpending adds a transaction amount to the spending record
func (ms *MediaSpending) AddSpending(transaction *MediaSpendingTransaction) {
	ms.TotalSpendMicros += transaction.CostMicros
	ms.TotalOperations++

	// Add to category-specific spending
	switch transaction.Category {
	case ResourceProcessing:
		ms.ProcessingSpendMicros += transaction.CostMicros
		if strings.Contains(transaction.Operation, "image") {
			ms.ImageProcessingOps++
		} else if strings.Contains(transaction.Operation, "video") {
			ms.VideoProcessingOps++
		} else if strings.Contains(transaction.Operation, "audio") {
			ms.AudioProcessingOps++
		}
	case "storage":
		ms.StorageSpendMicros += transaction.CostMicros
		ms.StorageOperations++
	case "bandwidth":
		ms.BandwidthSpendMicros += transaction.CostMicros
		ms.BandwidthBytes += transaction.BytesProcessed
	case "compute":
		ms.ComputeSpendMicros += transaction.CostMicros
		ms.ComputeTimeMs += transaction.ProcessingTimeMs
	}

	// Add to service-specific costs
	switch transaction.Service {
	case "s3":
		if strings.Contains(transaction.Operation, "storage") {
			ms.S3StorageCostMicros += transaction.CostMicros
		} else {
			ms.S3RequestCostMicros += transaction.CostMicros
		}
	case "cloudfront":
		ms.CloudFrontCostMicros += transaction.CostMicros
	case "mediaconvert":
		ms.MediaConvertCostMicros += transaction.CostMicros
		ms.MediaConvertMinutes += transaction.UnitsConsumed
	case "rekognition":
		ms.RekognitionCostMicros += transaction.CostMicros
		ms.RekognitionImages += transaction.UnitsConsumed
	case ResourceLambda:
		ms.LambdaCostMicros += transaction.CostMicros
	}

	// Track file processing
	if transaction.MediaID != "" {
		ms.FilesProcessed++
		ms.BytesProcessed += transaction.FileSize
	}

	// Track errors
	if transaction.IsError {
		ms.FailedOperations++
		ms.ErrorCostMicros += transaction.CostMicros
	}

	ms.UpdatedAt = time.Now()
}

// GetCostBreakdown returns a breakdown of costs by category
func (ms *MediaSpending) GetCostBreakdown() map[string]int64 {
	return map[string]int64{
		ResourceProcessing: ms.ProcessingSpendMicros,
		"storage":          ms.StorageSpendMicros,
		"bandwidth":        ms.BandwidthSpendMicros,
		"compute":          ms.ComputeSpendMicros,
	}
}

// GetServiceBreakdown returns a breakdown of costs by AWS service
func (ms *MediaSpending) GetServiceBreakdown() map[string]int64 {
	return map[string]int64{
		"s3_storage":   ms.S3StorageCostMicros,
		"s3_requests":  ms.S3RequestCostMicros,
		"cloudfront":   ms.CloudFrontCostMicros,
		"mediaconvert": ms.MediaConvertCostMicros,
		"rekognition":  ms.RekognitionCostMicros,
		"lambda":       ms.LambdaCostMicros,
	}
}

// GetEfficiencyMetrics returns efficiency metrics
func (ms *MediaSpending) GetEfficiencyMetrics() map[string]float64 {
	metrics := make(map[string]float64)

	if ms.FilesProcessed > 0 {
		metrics["cost_per_file"] = float64(ms.TotalSpendMicros) / float64(ms.FilesProcessed)
	}
	if ms.BytesProcessed > 0 {
		metrics["cost_per_mb"] = float64(ms.TotalSpendMicros) / float64(ms.BytesProcessed/(1024*1024))
	}
	if ms.TotalOperations > 0 {
		metrics["cost_per_operation"] = float64(ms.TotalSpendMicros) / float64(ms.TotalOperations)
		metrics["failure_rate"] = float64(ms.FailedOperations) / float64(ms.TotalOperations) * 100.0
	}

	return metrics
}

// IsOverBudget checks if spending has exceeded the budget limit
func (ms *MediaSpending) IsOverBudget() bool {
	return ms.BudgetLimitMicros > 0 && ms.TotalSpendMicros > ms.BudgetLimitMicros
}

// GetRemainingBudget returns the remaining budget in microdollars
func (ms *MediaSpending) GetRemainingBudget() int64 {
	if ms.BudgetLimitMicros <= 0 {
		return 0 // No budget set
	}
	remaining := ms.BudgetLimitMicros - ms.TotalSpendMicros
	if remaining < 0 {
		return 0
	}
	return remaining
}
