package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// MediaSpending tracks spending and costs for media processing operations per user
type MediaSpending struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - using user ID as partition key with time period as sort key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "MEDIA_SPENDING#{userID}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "PERIOD#{year}-{month}" or "DAILY#{year}-{month}-{day}"

	// GSI1 - Global spending queries across all users
	GSI1PK string `dynamorm:"index:spending-time-index,pk,attr:gsI1PK" json:"gsi1_pk"` // Format: "SPENDING#{period_type}"
	GSI1SK string `dynamorm:"index:spending-time-index,sk,attr:gsI1SK" json:"gsi1_sk"` // Format: "{year}-{month}-{day}#{userID}"

	// GSI2 - Cost category queries
	GSI2PK string `dynamorm:"index:cost-category-index,pk,attr:gsI2PK" json:"gsi2_pk"` // Format: "COST_CATEGORY#{category}"
	GSI2SK string `dynamorm:"index:cost-category-index,sk,attr:gsI2SK" json:"gsi2_sk"` // Format: "{timestamp}#{userID}"

	// Core spending data
	UserID     string `dynamorm:"attr:userID" json:"user_id"`
	Username   string `dynamorm:"attr:username" json:"username"`
	Period     string `dynamorm:"attr:period" json:"period"`          // "2024-01", "2024-01-15" (monthly or daily)
	PeriodType string `dynamorm:"attr:periodType" json:"period_type"` // "monthly", "daily"

	// Spending totals (in microdollars - $1 = 1,000,000 microdollars)
	TotalSpendMicros      int64 `dynamorm:"attr:totalSpendMicros" json:"total_spend_micros"`
	ProcessingSpendMicros int64 `dynamorm:"attr:processingSpendMicros" json:"processing_spend_micros"` // MediaConvert, Rekognition, etc.
	StorageSpendMicros    int64 `dynamorm:"attr:storageSpendMicros" json:"storage_spend_micros"`       // S3 storage costs
	BandwidthSpendMicros  int64 `dynamorm:"attr:bandwidthSpendMicros" json:"bandwidth_spend_micros"`   // CloudFront/CDN costs
	ComputeSpendMicros    int64 `dynamorm:"attr:computeSpendMicros" json:"compute_spend_micros"`       // Lambda compute costs

	// Operation counts
	TotalOperations    int64 `dynamorm:"attr:totalOperations" json:"total_operations"`
	ImageProcessingOps int64 `dynamorm:"attr:imageProcessingOps" json:"image_processing_ops"`
	VideoProcessingOps int64 `dynamorm:"attr:videoProcessingOps" json:"video_processing_ops"`
	AudioProcessingOps int64 `dynamorm:"attr:audioProcessingOps" json:"audio_processing_ops"`
	StorageOperations  int64 `dynamorm:"attr:storageOperations" json:"storage_operations"` // S3 PUT/GET operations
	BandwidthBytes     int64 `dynamorm:"attr:bandwidthBytes" json:"bandwidth_bytes"`       // Bytes transferred
	ComputeTimeMs      int64 `dynamorm:"attr:computeTimeMs" json:"compute_time_ms"`        // Lambda execution time

	// Detailed cost breakdown by service
	S3StorageCostMicros    int64 `dynamorm:"attr:s3StorageCostMicros" json:"s3_storage_cost_micros"`
	S3RequestCostMicros    int64 `dynamorm:"attr:s3RequestCostMicros" json:"s3_request_cost_micros"`
	CloudFrontCostMicros   int64 `dynamorm:"attr:cloudFrontCostMicros" json:"cloudfront_cost_micros"`
	MediaConvertCostMicros int64 `dynamorm:"attr:mediaConvertCostMicros" json:"mediaconvert_cost_micros"`
	RekognitionCostMicros  int64 `dynamorm:"attr:rekognitionCostMicros" json:"rekognition_cost_micros"`
	LambdaCostMicros       int64 `dynamorm:"attr:lambdaCostMicros" json:"lambda_cost_micros"`

	// Usage statistics
	FilesProcessed      int64 `dynamorm:"attr:filesProcessed" json:"files_processed"`
	BytesProcessed      int64 `dynamorm:"attr:bytesProcessed" json:"bytes_processed"`
	StorageBytesUsed    int64 `dynamorm:"attr:storageBytesUsed" json:"storage_bytes_used"`
	MediaConvertMinutes int64 `dynamorm:"attr:mediaConvertMinutes" json:"mediaconvert_minutes"` // Total minutes processed
	RekognitionImages   int64 `dynamorm:"attr:rekognitionImages" json:"rekognition_images"`     // Images analyzed
	ThumbnailsGenerated int64 `dynamorm:"attr:thumbnailsGenerated" json:"thumbnails_generated"`

	// Error tracking
	FailedOperations int64 `dynamorm:"attr:failedOperations" json:"failed_operations"`
	ErrorCostMicros  int64 `dynamorm:"attr:errorCostMicros" json:"error_cost_micros"` // Costs from failed operations

	// Budget tracking
	BudgetLimitMicros  int64      `dynamorm:"attr:budgetLimitMicros" json:"budget_limit_micros"`         // User's budget limit
	BudgetUsagePercent float64    `dynamorm:"attr:budgetUsagePercent" json:"budget_usage_percent"`       // Percentage of budget used
	BudgetExceeded     bool       `dynamorm:"attr:budgetExceeded" json:"budget_exceeded"`                // True if over budget
	BudgetExceededAt   *time.Time `dynamorm:"attr:budgetExceededAt" json:"budget_exceeded_at,omitempty"` // When budget was exceeded

	// Timestamps
	PeriodStartAt time.Time `dynamorm:"attr:periodStartAt" json:"period_start_at"`
	PeriodEndAt   time.Time `dynamorm:"attr:periodEndAt" json:"period_end_at"`
	CreatedAt     time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt     time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// TTL for old spending records (keep for 2 years)
	ExpiresAt *int64 `dynamorm:"ttl,attr:expiresAt" json:"expires_at,omitempty"` // Unix timestamp

	// Version for optimistic locking
	ModelVersion int `dynamorm:"version,attr:modelVersion" json:"model_version"`
}

// MediaSpendingTransaction represents a single spending transaction
type MediaSpendingTransaction struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - using user ID as partition key with transaction ID as sort key
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "SPENDING_TXN#{userID}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "TXN#{timestamp}#{transactionID}"

	// GSI1 - Time-based transaction queries
	GSI1PK string `dynamorm:"index:transaction-time-index,pk,attr:gsI1PK" json:"gsi1_pk"` // Format: "TXN_TIME#{date}"
	GSI1SK string `dynamorm:"index:transaction-time-index,sk,attr:gsI1SK" json:"gsi1_sk"` // Format: "{timestamp}#{userID}#{transactionID}"

	// Core transaction data
	TransactionID string `dynamorm:"attr:transactionID" json:"transaction_id"`
	UserID        string `dynamorm:"attr:userID" json:"user_id"`
	Username      string `dynamorm:"attr:username" json:"username"`

	// Cost details
	CostMicros  int64  `dynamorm:"attr:costMicros" json:"cost_micros"`  // Cost in microdollars
	Category    string `dynamorm:"attr:category" json:"category"`       // "processing", "storage", "bandwidth", "compute"
	Service     string `dynamorm:"attr:service" json:"service"`         // "s3", "mediaconvert", "cloudfront", "lambda", "rekognition"
	Operation   string `dynamorm:"attr:operation" json:"operation"`     // "image_resize", "video_transcode", "storage_put", etc.
	Description string `dynamorm:"attr:description" json:"description"` // Human-readable description

	// Associated media
	MediaID     string `dynamorm:"attr:mediaID" json:"media_id,omitempty"`
	JobID       string `dynamorm:"attr:jobID" json:"job_id,omitempty"`
	FileName    string `dynamorm:"attr:fileName" json:"file_name,omitempty"`
	FileSize    int64  `dynamorm:"attr:fileSize" json:"file_size,omitempty"`
	ContentType string `dynamorm:"attr:contentType" json:"content_type,omitempty"`

	// Processing details
	ProcessingTimeMs int64 `dynamorm:"attr:processingTimeMs" json:"processing_time_ms,omitempty"` // How long the operation took
	BytesProcessed   int64 `dynamorm:"attr:bytesProcessed" json:"bytes_processed,omitempty"`      // Bytes processed
	UnitsConsumed    int64 `dynamorm:"attr:unitsConsumed" json:"units_consumed,omitempty"`        // Service-specific units

	// Error information
	IsError      bool   `dynamorm:"attr:isError" json:"is_error"`
	ErrorMessage string `dynamorm:"attr:errorMessage" json:"error_message,omitempty"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`

	// TTL for old transactions (keep for 1 year)
	ExpiresAt *int64 `dynamorm:"ttl,attr:expiresAt" json:"expires_at,omitempty"` // Unix timestamp
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
			return fmt.Errorf("%w: %s", ErrInvalidMonthlyPeriodFormat, ms.Period)
		}
		ms.PeriodStartAt = t
		ms.PeriodEndAt = t.AddDate(0, 1, 0).Add(-time.Nanosecond) // End of month
	case PeriodDaily:
		// Parse YYYY-MM-DD format
		t, err := time.Parse(common.DateFormat, ms.Period)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidDailyPeriodFormat, ms.Period)
		}
		ms.PeriodStartAt = t
		ms.PeriodEndAt = t.Add(24*time.Hour - time.Nanosecond) // End of day
	default:
		return fmt.Errorf("%w: %s", ErrInvalidPeriodType, ms.PeriodType)
	}

	return nil
}

// Validate performs validation on the MediaSpending
func (ms *MediaSpending) Validate() error {
	if err := common.ValidateRequiredParam("UserID", strings.TrimSpace(ms.UserID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("period", strings.TrimSpace(ms.Period)); err != nil {
		return err
	}
	if ms.PeriodType != PeriodMonthly && ms.PeriodType != PeriodDaily {
		return ErrInvalidPeriodTypeValue
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
		return ErrNegativeSpendingAmounts
	}

	return nil
}

// BeforeCreate sets up the MediaSpendingTransaction model before creation
func (mst *MediaSpendingTransaction) BeforeCreate() error {
	now := time.Now()
	mst.CreatedAt = now

	// Generate transaction ID if not provided
	if err := common.ValidateRequiredParam("TransactionID", mst.TransactionID); err != nil {
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
	if err := common.ValidateRequiredParam("UserID", strings.TrimSpace(mst.UserID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("TransactionID", strings.TrimSpace(mst.TransactionID)); err != nil {
		return err
	}
	if mst.CostMicros < 0 {
		return ErrNegativeCostMicros
	}
	if err := common.ValidateRequiredParam("category", strings.TrimSpace(mst.Category)); err != nil {
		return err
	}

	// Validate category
	validCategories := map[string]bool{
		ResourceProcessing: true, ResourceStorage: true, ResourceBandwidth: true, ResourceCompute: true,
	}
	if !validCategories[mst.Category] {
		return fmt.Errorf("%w: %s", ErrInvalidSpendingCategory, mst.Category)
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

// === BaseModel Interface Implementation for MediaSpending ===

// GetPK returns the partition key for this media spending record
func (ms *MediaSpending) GetPK() string {
	return ms.PK
}

// GetSK returns the sort key for this media spending record
func (ms *MediaSpending) GetSK() string {
	return ms.SK
}

// UpdateKeys ensures all key fields are properly set
func (ms *MediaSpending) UpdateKeys() error {
	// Validate required fields
	if err := common.ValidateRequiredParam("UserID", ms.UserID); err != nil {
		return fmt.Errorf("%w: %w", ErrMediaSpendingUserIDRequired, err)
	}
	if err := common.ValidateRequiredParam("Period", ms.Period); err != nil {
		return fmt.Errorf("%w: %w", ErrMediaSpendingPeriodRequired, err)
	}

	// Set primary keys
	ms.PK = "MEDIA_SPENDING#" + ms.UserID
	ms.SK = "PERIOD#" + ms.Period

	// Update GSI keys
	ms.setupGSIKeys()

	return nil
}

// === BaseModel Interface Implementation for MediaSpendingTransaction ===

// GetPK returns the partition key for this media spending transaction
func (mst *MediaSpendingTransaction) GetPK() string {
	return mst.PK
}

// GetSK returns the sort key for this media spending transaction
func (mst *MediaSpendingTransaction) GetSK() string {
	return mst.SK
}

// UpdateKeys ensures all key fields are properly set
func (mst *MediaSpendingTransaction) UpdateKeys() error {
	// Validate required fields
	if err := common.ValidateRequiredParam("UserID", mst.UserID); err != nil {
		return fmt.Errorf("%w: %w", ErrMediaSpendingUserIDRequired, err)
	}
	if err := common.ValidateRequiredParam("TransactionID", mst.TransactionID); err != nil {
		return fmt.Errorf("%w: %w", ErrMediaSpendingTransactionIDRequired, err)
	}

	// Set primary keys
	mst.PK = "SPENDING_TXN#" + mst.UserID
	now := mst.CreatedAt
	if now.IsZero() {
		now = time.Now()
	}
	mst.SK = fmt.Sprintf("TXN#%s#%s", now.Format("20060102150405"), mst.TransactionID)

	// Set up GSI keys
	dateStr := now.Format(common.DateFormat)
	mst.GSI1PK = "TXN_TIME#" + dateStr
	mst.GSI1SK = fmt.Sprintf("%s#%s#%s", now.Format(time.RFC3339), mst.UserID, mst.TransactionID)

	return nil
}
