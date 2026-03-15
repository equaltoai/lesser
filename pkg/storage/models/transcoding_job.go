package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// TranscodingJob tracks detailed metrics and costs for individual transcoding operations
type TranscodingJob struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - using job ID as partition key
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "TRANSCODING_JOB#{jobID}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "JOB_METRICS"

	// GSI1 - User-based queries for transcoding jobs
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk"` // Format: "USER_TRANSCODING#{userID}"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk"` // Format: "{timestamp}#{jobID}"

	// GSI2 - Media-based queries
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2_pk"` // Format: "MEDIA_TRANSCODING#{mediaID}"
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2_sk"` // Format: "{timestamp}#{jobID}"

	// Core job data
	JobID    string `theorydb:"attr:jobID" json:"job_id"`
	MediaID  string `theorydb:"attr:mediaID" json:"media_id"`
	UserID   string `theorydb:"attr:userID" json:"user_id"`
	Username string `theorydb:"attr:username" json:"username"`
	JobType  string `theorydb:"attr:jobType" json:"job_type"` // "video", "audio", "image"
	Status   string `theorydb:"attr:status" json:"status"`    // "processing", "completed", "failed"

	// Input details
	InputFormat     string `theorydb:"attr:inputFormat" json:"input_format"`         // "video/mp4", "audio/mpeg", etc.
	InputSize       int64  `theorydb:"attr:inputSize" json:"input_size"`             // bytes
	InputDuration   int64  `theorydb:"attr:inputDuration" json:"input_duration"`     // milliseconds (for video/audio)
	InputResolution string `theorydb:"attr:inputResolution" json:"input_resolution"` // "1920x1080" (for video)

	// Output details
	OutputVariants  map[string]string `theorydb:"attr:outputVariants" json:"output_variants"`    // quality -> format mapping
	OutputSizes     map[string]int64  `theorydb:"attr:outputSizes" json:"output_sizes"`          // quality -> size in bytes
	TotalOutputSize int64             `theorydb:"attr:totalOutputSize" json:"total_output_size"` // sum of all output sizes

	// Processing metrics
	ProcessingTimeMs int64      `theorydb:"attr:processingTimeMs" json:"processing_time_ms"`
	StartedAt        time.Time  `theorydb:"attr:startedAt" json:"started_at"`
	CompletedAt      *time.Time `theorydb:"attr:completedAt" json:"completed_at,omitempty"`
	ErrorMessage     string     `theorydb:"attr:errorMessage" json:"error_message,omitempty"`

	// Cost breakdown (in microdollars)
	TotalCostMicros        int64            `theorydb:"attr:totalCostMicros" json:"total_cost_micros"`
	CostBreakdown          map[string]int64 `theorydb:"attr:costBreakdown" json:"cost_breakdown"` // service -> cost
	MediaConvertCostMicros int64            `theorydb:"attr:mediaConvertCostMicros" json:"mediaconvert_cost_micros"`
	S3StorageCostMicros    int64            `theorydb:"attr:s3StorageCostMicros" json:"s3_storage_cost_micros"`
	S3RequestCostMicros    int64            `theorydb:"attr:s3RequestCostMicros" json:"s3_request_cost_micros"`
	LambdaCostMicros       int64            `theorydb:"attr:lambdaCostMicros" json:"lambda_cost_micros"`
	RekognitionCostMicros  int64            `theorydb:"attr:rekognitionCostMicros" json:"rekognition_cost_micros"`

	// Quality and transcoding settings
	QualityLevels   []string `theorydb:"attr:qualityLevels" json:"quality_levels"`     // ["480p", "720p", "1080p"]
	ThumbnailCount  int      `theorydb:"attr:thumbnailCount" json:"thumbnail_count"`   // number of thumbnails generated
	AnalysisEnabled bool     `theorydb:"attr:analysisEnabled" json:"analysis_enabled"` // whether content analysis was performed

	// AWS service job IDs for tracking
	MediaConvertJobID string   `theorydb:"attr:mediaConvertJobID" json:"mediaconvert_job_id,omitempty"`
	S3Keys            []string `theorydb:"attr:s3Keys" json:"s3_keys"` // all S3 keys created by this job

	// Efficiency metrics
	CompressionRatio    float64 `theorydb:"attr:compressionRatio" json:"compression_ratio"`        // output_size / input_size
	CostPerMB           float64 `theorydb:"attr:costPerMB" json:"cost_per_mb"`                     // cost per MB processed
	ProcessingSpeedMBps float64 `theorydb:"attr:processingSpeedMBps" json:"processing_speed_mbps"` // MB/second processing speed

	// Budget tracking
	EstimatedCostMicros int64 `theorydb:"attr:estimatedCostMicros" json:"estimated_cost_micros"` // initial cost estimate
	CostVariance        int64 `theorydb:"attr:costVariance" json:"cost_variance"`                // actual - estimated cost

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// TTL for old transcoding jobs (keep for 1 year)
	ExpiresAt *int64 `theorydb:"ttl,attr:ttl" json:"expires_at,omitempty"` // Unix timestamp

	// Version for optimistic locking
	ModelVersion int `theorydb:"version,attr:modelVersion" json:"model_version"`
}

// TableName returns the DynamoDB table name for TranscodingJob
func (TranscodingJob) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the TranscodingJob model before creation
func (tj *TranscodingJob) BeforeCreate() error {
	now := time.Now()
	tj.CreatedAt = now
	tj.UpdatedAt = now

	// Set up primary key
	tj.PK = "TRANSCODING_JOB#" + tj.JobID
	tj.SK = "JOB_METRICS"

	// Set up GSI keys
	tj.setupGSIKeys()

	// Initialize maps if nil
	if tj.OutputVariants == nil {
		tj.OutputVariants = make(map[string]string)
	}
	if tj.OutputSizes == nil {
		tj.OutputSizes = make(map[string]int64)
	}
	if tj.CostBreakdown == nil {
		tj.CostBreakdown = make(map[string]int64)
	}
	if tj.S3Keys == nil {
		tj.S3Keys = []string{}
	}
	if tj.QualityLevels == nil {
		tj.QualityLevels = []string{}
	}

	// Set TTL (1 year)
	expires := now.Add(365 * 24 * time.Hour).Unix()
	tj.ExpiresAt = &expires

	// Set default status if not provided
	if err := common.ValidateRequiredParam("status", tj.Status); err != nil {
		tj.Status = "processing"
	}

	return tj.Validate()
}

// BeforeUpdate sets up the model before update
func (tj *TranscodingJob) BeforeUpdate() error {
	tj.UpdatedAt = time.Now()

	// Update GSI keys
	tj.setupGSIKeys()

	// Calculate efficiency metrics
	tj.calculateEfficiencyMetrics()

	// Calculate cost variance
	if tj.EstimatedCostMicros > 0 {
		tj.CostVariance = tj.TotalCostMicros - tj.EstimatedCostMicros
	}

	return tj.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys for TranscodingJob
func (tj *TranscodingJob) setupGSIKeys() {
	// GSI1 - User-based transcoding job queries
	tj.GSI1PK = "USER_TRANSCODING#" + tj.UserID
	tj.GSI1SK = fmt.Sprintf("%s#%s", tj.StartedAt.Format(time.RFC3339), tj.JobID)

	// GSI2 - Media-based transcoding job queries
	tj.GSI2PK = "MEDIA_TRANSCODING#" + tj.MediaID
	tj.GSI2SK = fmt.Sprintf("%s#%s", tj.StartedAt.Format(time.RFC3339), tj.JobID)
}

// calculateEfficiencyMetrics calculates compression ratio, cost per MB, and processing speed
func (tj *TranscodingJob) calculateEfficiencyMetrics() {
	// Compression ratio
	if tj.InputSize > 0 {
		tj.CompressionRatio = float64(tj.TotalOutputSize) / float64(tj.InputSize)
	}

	// Cost per MB
	if tj.InputSize > 0 {
		inputMB := float64(tj.InputSize) / (1024 * 1024)
		tj.CostPerMB = float64(tj.TotalCostMicros) / inputMB
	}

	// Processing speed (MB/second)
	if tj.ProcessingTimeMs > 0 && tj.InputSize > 0 {
		inputMB := float64(tj.InputSize) / (1024 * 1024)
		processingSeconds := float64(tj.ProcessingTimeMs) / 1000
		tj.ProcessingSpeedMBps = inputMB / processingSeconds
	}
}

// Validate performs validation on the TranscodingJob
func (tj *TranscodingJob) Validate() error {
	if err := common.ValidateRequiredParam("JobID", strings.TrimSpace(tj.JobID)); err != nil {
		return ErrTranscodingJobIDRequired
	}
	if err := common.ValidateRequiredParam("MediaID", strings.TrimSpace(tj.MediaID)); err != nil {
		return ErrTranscodingMediaIDRequired
	}
	if err := common.ValidateRequiredParam("UserID", strings.TrimSpace(tj.UserID)); err != nil {
		return ErrTranscodingUserIDRequired
	}

	// Validate job type
	validJobTypes := map[string]bool{
		"video": true, "audio": true, "image": true,
	}
	if !validJobTypes[tj.JobType] {
		return fmt.Errorf("%w: %s", ErrInvalidJobType, tj.JobType)
	}

	// Validate status
	validStatuses := map[string]bool{
		"processing": true, "completed": true, "failed": true,
	}
	if !validStatuses[tj.Status] {
		return fmt.Errorf("%w: %s", ErrInvalidJobStatus, tj.Status)
	}

	// Validate sizes are non-negative
	if tj.InputSize < 0 || tj.TotalOutputSize < 0 {
		return ErrNegativeSize
	}

	// Validate costs are non-negative
	if tj.TotalCostMicros < 0 || tj.EstimatedCostMicros < 0 {
		return ErrNegativeCost
	}

	return nil
}

// SetCompleted marks the job as completed and sets completion time
func (tj *TranscodingJob) SetCompleted() {
	tj.Status = StatusCompleted
	now := time.Now()
	tj.CompletedAt = &now
	if !tj.StartedAt.IsZero() {
		tj.ProcessingTimeMs = now.Sub(tj.StartedAt).Milliseconds()
	}
}

// SetFailed marks the job as failed with an error message
func (tj *TranscodingJob) SetFailed(errorMessage string) {
	tj.Status = StatusFailed
	tj.ErrorMessage = errorMessage
	now := time.Now()
	tj.CompletedAt = &now
	if !tj.StartedAt.IsZero() {
		tj.ProcessingTimeMs = now.Sub(tj.StartedAt).Milliseconds()
	}
}

// AddOutputVariant adds a new output variant to the job
func (tj *TranscodingJob) AddOutputVariant(quality, format string, size int64, s3Key string) {
	if tj.OutputVariants == nil {
		tj.OutputVariants = make(map[string]string)
	}
	if tj.OutputSizes == nil {
		tj.OutputSizes = make(map[string]int64)
	}

	tj.OutputVariants[quality] = format
	tj.OutputSizes[quality] = size
	tj.TotalOutputSize += size

	// Add S3 key to the list if not already present
	if tj.S3Keys == nil {
		tj.S3Keys = []string{}
	}
	for _, key := range tj.S3Keys {
		if key == s3Key {
			return // Already exists
		}
	}
	tj.S3Keys = append(tj.S3Keys, s3Key)
}

// AddCost adds a cost component to the job's cost breakdown
func (tj *TranscodingJob) AddCost(service string, costMicros int64) {
	if tj.CostBreakdown == nil {
		tj.CostBreakdown = make(map[string]int64)
	}

	tj.CostBreakdown[service] = costMicros
	tj.TotalCostMicros += costMicros

	// Also update specific service cost fields
	switch service {
	case "mediaconvert":
		tj.MediaConvertCostMicros += costMicros
	case "s3_storage":
		tj.S3StorageCostMicros += costMicros
	case "s3_upload", "s3_request":
		tj.S3RequestCostMicros += costMicros
	case "lambda", "lambda_processing":
		tj.LambdaCostMicros += costMicros
	case "rekognition":
		tj.RekognitionCostMicros += costMicros
	}
}

// GetCostEfficiency returns cost efficiency metrics
func (tj *TranscodingJob) GetCostEfficiency() map[string]float64 {
	metrics := make(map[string]float64)

	if tj.ProcessingTimeMs > 0 {
		metrics["cost_per_second"] = float64(tj.TotalCostMicros) / (float64(tj.ProcessingTimeMs) / 1000)
	}
	if tj.InputSize > 0 {
		metrics["cost_per_mb"] = tj.CostPerMB
		metrics["compression_ratio"] = tj.CompressionRatio
	}
	if tj.EstimatedCostMicros > 0 {
		metrics["cost_accuracy"] = float64(tj.TotalCostMicros) / float64(tj.EstimatedCostMicros)
	}

	return metrics
}

// GetQualityBreakdown returns a breakdown of costs by quality level
func (tj *TranscodingJob) GetQualityBreakdown() map[string]map[string]interface{} {
	breakdown := make(map[string]map[string]interface{})

	for quality, format := range tj.OutputVariants {
		size, hasSize := tj.OutputSizes[quality]
		breakdown[quality] = map[string]interface{}{
			"format": format,
			"size":   size,
			"valid":  hasSize,
		}
	}

	return breakdown
}

// IsCompleted checks if the job has completed (successfully or with failure)
func (tj *TranscodingJob) IsCompleted() bool {
	return tj.Status == "completed" || tj.Status == "failed"
}

// Duration returns the total processing duration
func (tj *TranscodingJob) Duration() time.Duration {
	if tj.CompletedAt != nil && !tj.StartedAt.IsZero() {
		return tj.CompletedAt.Sub(tj.StartedAt)
	}
	return 0
}

// GetServiceCostBreakdown returns costs broken down by AWS service
func (tj *TranscodingJob) GetServiceCostBreakdown() map[string]int64 {
	return map[string]int64{
		"mediaconvert": tj.MediaConvertCostMicros,
		"s3_storage":   tj.S3StorageCostMicros,
		"s3_requests":  tj.S3RequestCostMicros,
		"lambda":       tj.LambdaCostMicros,
		"rekognition":  tj.RekognitionCostMicros,
	}
}

// === BaseModel Interface Implementation ===

// GetPK returns the partition key for this transcoding job
func (tj *TranscodingJob) GetPK() string {
	return tj.PK
}

// GetSK returns the sort key for this transcoding job
func (tj *TranscodingJob) GetSK() string {
	return tj.SK
}

// UpdateKeys ensures all key fields are properly set
func (tj *TranscodingJob) UpdateKeys() error {
	// Validate required fields
	if err := common.ValidateRequiredParam("JobID", tj.JobID); err != nil {
		return fmt.Errorf("%w: %w", ErrTranscodingJobIDRequired, err)
	}

	// Set primary keys
	tj.PK = "TRANSCODING_JOB#" + tj.JobID
	tj.SK = "JOB_METRICS"

	// Update GSI keys
	tj.setupGSIKeys()

	return nil
}
