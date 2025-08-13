package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MediaJob represents a media processing job in the system
type MediaJob struct {
	// Primary key - using job ID
	PK string `dynamorm:"pk" json:"pk"` // Format: "JOB#{jobID}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "JOB#{jobID}"

	// GSI1 - User jobs lookup
	GSI1PK string `dynamorm:"index:user-jobs-index,pk" json:"gsi1_pk"` // Format: "USER_JOBS#{userID}"
	GSI1SK string `dynamorm:"index:user-jobs-index,sk" json:"gsi1_sk"` // Format: "{created_at}#{jobID}"

	// GSI2 - Status-based queries
	GSI2PK string `dynamorm:"index:status-index,pk" json:"gsi2_pk"` // Format: "STATUS#{status}"
	GSI2SK string `dynamorm:"index:status-index,sk" json:"gsi2_sk"` // Format: "UPDATED#{updated_at}"

	// Core job data
	JobID           string         `json:"job_id"`
	MediaID         string         `json:"media_id"`
	Username        string         `json:"username"`
	Status          string         `json:"status"` // pending, processing, completed, failed, cancelled
	ProcessingTasks []string       `json:"processing_tasks"`
	S3Key           string         `json:"s3_key"`
	MimeType        string         `json:"mime_type"`
	Results         map[string]any `json:"results,omitempty"`
	Error           string         `json:"error,omitempty"`

	// Idempotency and duplicate prevention
	IdempotencyKey string `json:"idempotency_key"` // Hash of user_id + file_hash + timestamp
	FileHash       string `json:"file_hash"`       // SHA256 hash of file contents
	FileSize       int64  `json:"file_size"`       // File size in bytes

	// Processing state tracking
	StartedAt     *time.Time `json:"started_at,omitempty"`      // When processing actually began
	CompletedAt   *time.Time `json:"completed_at,omitempty"`    // When processing completed
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"` // Last retry attempt
	Progress      int        `json:"progress"`                  // Progress percentage (0-100)

	// Retry logic
	RetryCount       int        `json:"retry_count"`                  // Number of retries attempted
	MaxRetries       int        `json:"max_retries"`                  // Maximum retries allowed
	LastError        string     `json:"last_error,omitempty"`         // Last error encountered
	RetryScheduledAt *time.Time `json:"retry_scheduled_at,omitempty"` // When next retry is scheduled

	// Budget and timeout enforcement
	EstimatedCostMicros int64         `json:"estimated_cost_micros"`           // Estimated processing cost
	ActualCostMicros    int64         `json:"actual_cost_micros"`              // Actual cost incurred
	MaxProcessingTime   time.Duration `json:"max_processing_time"`             // Maximum allowed processing time
	ProcessingStartedAt *time.Time    `json:"processing_started_at,omitempty"` // When current processing attempt started

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for abandoned jobs (24 hours)
	ExpiresAt *int64 `dynamorm:"ttl" json:"expires_at,omitempty"` // Unix timestamp

	// Version for optimistic locking
	ModelVersion int `dynamorm:"version" json:"model_version"`
}

// TableName returns the DynamoDB table name for the MediaJob model
func (MediaJob) TableName() string {
	return MainTableName // Use the main table
}

// UpdateKeys sets up all the composite keys
func (mj *MediaJob) UpdateKeys() {
	// Primary key
	mj.PK = fmt.Sprintf("JOB#%s", mj.JobID)
	mj.SK = fmt.Sprintf("JOB#%s", mj.JobID)

	// GSI1 - User jobs lookup
	if mj.Username != "" {
		mj.GSI1PK = fmt.Sprintf("USER_JOBS#%s", mj.Username)
		mj.GSI1SK = fmt.Sprintf("%s#%s", mj.CreatedAt.Format(time.RFC3339), mj.JobID)
	}

	// GSI2 - Status-based queries
	mj.GSI2PK = fmt.Sprintf(KeyPatternStatus, mj.Status)
	mj.GSI2SK = fmt.Sprintf("UPDATED#%s", mj.UpdatedAt.Format(time.RFC3339))
}

// BeforeCreate sets up the model before creation
func (mj *MediaJob) BeforeCreate() error {
	now := time.Now()
	mj.CreatedAt = now
	mj.UpdatedAt = now

	// Generate job ID if not provided
	if mj.JobID == "" {
		mj.JobID = uuid.New().String()
	}

	// Generate idempotency key if not provided
	if mj.IdempotencyKey == "" {
		mj.IdempotencyKey = mj.GenerateIdempotencyKey()
	}

	// Set default status
	if mj.Status == "" {
		mj.Status = StatusPending
	}

	// Initialize empty results if nil
	if mj.Results == nil {
		mj.Results = make(map[string]any)
	}

	// Initialize empty processing tasks if nil
	if mj.ProcessingTasks == nil {
		mj.ProcessingTasks = []string{}
	}

	// Set default retry configuration
	if mj.MaxRetries == 0 {
		mj.MaxRetries = 3 // Default to 3 retries
	}

	// Set default processing timeout based on media type
	if mj.MaxProcessingTime == 0 {
		mj.MaxProcessingTime = mj.getDefaultProcessingTimeout()
	}

	// Set TTL for idempotency (24 hours)
	if mj.ExpiresAt == nil {
		ttl := now.Add(24 * time.Hour).Unix()
		mj.ExpiresAt = &ttl
	}

	// Set up keys
	mj.UpdateKeys()

	return mj.Validate()
}

// BeforeUpdate sets up the model before update
func (mj *MediaJob) BeforeUpdate() error {
	mj.UpdatedAt = time.Now()
	mj.UpdateKeys()
	return mj.Validate()
}

// Validate performs validation on the MediaJob
func (mj *MediaJob) Validate() error {
	if strings.TrimSpace(mj.JobID) == "" {
		return fmt.Errorf("JobID is required")
	}
	if strings.TrimSpace(mj.MediaID) == "" {
		return fmt.Errorf("MediaID is required")
	}
	if strings.TrimSpace(mj.Username) == "" {
		return fmt.Errorf("username is required")
	}
	if strings.TrimSpace(mj.S3Key) == "" {
		return fmt.Errorf("S3Key is required")
	}
	if strings.TrimSpace(mj.MimeType) == "" {
		return fmt.Errorf("MimeType is required")
	}

	// Validate status
	if !isValidJobStatus(mj.Status) {
		return fmt.Errorf("invalid job status: %s", mj.Status)
	}

	return nil
}

// SetProcessing marks the job as processing
func (mj *MediaJob) SetProcessing() {
	now := time.Now()
	mj.Status = StatusProcessing
	mj.Error = ""
	mj.UpdatedAt = now

	// Set processing timestamps
	if mj.StartedAt == nil {
		mj.StartedAt = &now
	}
	mj.ProcessingStartedAt = &now
	mj.LastAttemptAt = &now

	mj.UpdateKeys()
}

// SetCompleted marks the job as completed with results
func (mj *MediaJob) SetCompleted(results map[string]any) {
	now := time.Now()
	mj.Status = StatusCompleted
	mj.Results = results
	mj.Error = ""
	mj.UpdatedAt = now
	mj.CompletedAt = &now
	mj.Progress = 100

	// Clear TTL for completed jobs to keep them longer
	mj.ExpiresAt = nil

	mj.UpdateKeys()
}

// SetFailed marks the job as failed with an error message
func (mj *MediaJob) SetFailed(errorMsg string) {
	now := time.Now()
	mj.Status = StatusFailed
	mj.Error = errorMsg
	mj.LastError = errorMsg
	mj.UpdatedAt = now
	mj.CompletedAt = &now

	// Set TTL for failed jobs (7 days)
	ttl := now.Add(7 * 24 * time.Hour).Unix()
	mj.ExpiresAt = &ttl

	mj.UpdateKeys()
}

// IsCompleted returns true if the job is completed
func (mj *MediaJob) IsCompleted() bool {
	return mj.Status == StatusCompleted
}

// IsFailed returns true if the job failed
func (mj *MediaJob) IsFailed() bool {
	return mj.Status == StatusFailed
}

// IsProcessing returns true if the job is processing
func (mj *MediaJob) IsProcessing() bool {
	return mj.Status == StatusProcessing
}

// IsPending returns true if the job is pending
func (mj *MediaJob) IsPending() bool {
	return mj.Status == StatusPending
}

// AddProcessingTask adds a task to the processing tasks list
func (mj *MediaJob) AddProcessingTask(task string) {
	if mj.ProcessingTasks == nil {
		mj.ProcessingTasks = []string{}
	}
	mj.ProcessingTasks = append(mj.ProcessingTasks, task)
}

// HasProcessingTask checks if a specific task is in the processing tasks
func (mj *MediaJob) HasProcessingTask(task string) bool {
	for _, t := range mj.ProcessingTasks {
		if t == task {
			return true
		}
	}
	return false
}

// GenerateIdempotencyKey creates an idempotency key from user, file hash, and timestamp
func (mj *MediaJob) GenerateIdempotencyKey() string {
	// Create idempotency key: hash(username + fileHash + createdAt)
	data := fmt.Sprintf("%s:%s:%d", mj.Username, mj.FileHash, mj.CreatedAt.Unix())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// SetCancelled marks the job as cancelled
func (mj *MediaJob) SetCancelled(reason string) {
	now := time.Now()
	mj.Status = MediaStatusCancelled
	mj.Error = reason
	mj.UpdatedAt = now
	mj.CompletedAt = &now

	// Set TTL for cancelled jobs (24 hours)
	ttl := now.Add(24 * time.Hour).Unix()
	mj.ExpiresAt = &ttl

	mj.UpdateKeys()
}

// CanRetry determines if the job can be retried based on error type and retry count
func (mj *MediaJob) CanRetry() bool {
	if mj.RetryCount >= mj.MaxRetries {
		return false
	}

	// Don't retry if job is not in failed state
	if mj.Status != StatusFailed {
		return false
	}

	// Check if error is retryable (transient vs permanent)
	return mj.IsRetryableError(mj.LastError)
}

// IsRetryableError determines if an error is retryable
func (mj *MediaJob) IsRetryableError(errorMsg string) bool {
	errorLower := strings.ToLower(errorMsg)

	// Permanent errors - don't retry
	permanentErrors := []string{
		"invalid format",
		"unsupported",
		"file too large",
		"corrupted",
		"authorization failed",
		"budget exceeded",
		"quota exceeded",
	}

	for _, permanentErr := range permanentErrors {
		if strings.Contains(errorLower, permanentErr) {
			return false
		}
	}

	// Transient errors - retry
	transientErrors := []string{
		"timeout",
		"network",
		"connection",
		"throttling",
		"rate limit",
		"service unavailable",
		"internal error",
	}

	for _, transientErr := range transientErrors {
		if strings.Contains(errorLower, transientErr) {
			return true
		}
	}

	// Default to retryable for unknown errors
	return true
}

// ScheduleRetry schedules the next retry with exponential backoff
func (mj *MediaJob) ScheduleRetry(baseDelay time.Duration) {
	mj.RetryCount++

	// Exponential backoff: 1s, 4s, 16s, 64s, etc.
	delay := baseDelay
	for i := 0; i < mj.RetryCount; i++ {
		delay *= 4
	}

	// Cap maximum delay at 5 minutes
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}

	now := time.Now()
	retryTime := now.Add(delay)
	mj.RetryScheduledAt = &retryTime
	mj.Status = StatusPending // Reset to pending for retry
	mj.UpdatedAt = now

	mj.UpdateKeys()
}

// IsAbandoned checks if the job has been abandoned (no updates for too long)
func (mj *MediaJob) IsAbandoned(abandonThreshold time.Duration) bool {
	// Job is abandoned if it's processing and hasn't been updated for threshold time
	if mj.Status == StatusProcessing {
		return time.Since(mj.UpdatedAt) > abandonThreshold
	}
	return false
}

// IsBudgetExceeded checks if processing has exceeded time budget
func (mj *MediaJob) IsBudgetExceeded() bool {
	if mj.ProcessingStartedAt == nil || mj.MaxProcessingTime == 0 {
		return false
	}

	processingDuration := time.Since(*mj.ProcessingStartedAt)
	return processingDuration > mj.MaxProcessingTime
}

// UpdateProgress updates the job progress percentage
func (mj *MediaJob) UpdateProgress(progress int) {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	mj.Progress = progress
	mj.UpdatedAt = time.Now()
	mj.UpdateKeys()
}

// AddCost adds to the actual cost incurred for processing
func (mj *MediaJob) AddCost(costMicros int64) {
	mj.ActualCostMicros += costMicros
	mj.UpdatedAt = time.Now()
}

// getDefaultProcessingTimeout returns default timeout based on media type
func (mj *MediaJob) getDefaultProcessingTimeout() time.Duration {
	if strings.HasPrefix(mj.MimeType, "image/") {
		return 30 * time.Second // Images: 30 seconds
	} else if strings.HasPrefix(mj.MimeType, "video/") {
		if mj.FileSize >= 10*1024*1024 { // >= 10MB
			return 5 * time.Minute // Large videos: 5 minutes
		}
		return 2 * time.Minute // Small videos: 2 minutes
	} else if mj.MimeType == "image/gif" {
		return 60 * time.Second // GIFs: 60 seconds
	}

	return 2 * time.Minute // Default: 2 minutes
}

// IsIdle checks if job is idle (pending but not scheduled for retry)
func (mj *MediaJob) IsIdle() bool {
	return mj.Status == StatusPending && mj.RetryScheduledAt == nil
}

// IsReadyForRetry checks if job is ready to be retried
func (mj *MediaJob) IsReadyForRetry() bool {
	if mj.RetryScheduledAt == nil {
		return false
	}
	return time.Now().After(*mj.RetryScheduledAt)
}

// IsCancelled returns true if the job is cancelled
func (mj *MediaJob) IsCancelled() bool {
	return mj.Status == "cancelled"
}

// GetProcessingDuration returns how long the job has been processing
func (mj *MediaJob) GetProcessingDuration() time.Duration {
	if mj.ProcessingStartedAt == nil {
		return 0
	}

	if mj.CompletedAt != nil {
		return mj.CompletedAt.Sub(*mj.ProcessingStartedAt)
	}

	return time.Since(*mj.ProcessingStartedAt)
}

// GetRemainingProcessingTime returns remaining processing time before timeout
func (mj *MediaJob) GetRemainingProcessingTime() time.Duration {
	if mj.ProcessingStartedAt == nil || mj.MaxProcessingTime == 0 {
		return mj.MaxProcessingTime
	}

	elapsed := time.Since(*mj.ProcessingStartedAt)
	remaining := mj.MaxProcessingTime - elapsed

	if remaining < 0 {
		return 0
	}

	return remaining
}

// isValidJobStatus checks if the status is valid
func isValidJobStatus(status string) bool {
	validStatuses := map[string]bool{
		StatusPending:    true,
		StatusProcessing: true,
		StatusCompleted:  true,
		StatusFailed:     true,
		"cancelled":      true,
	}

	return validStatuses[strings.ToLower(status)]
}
