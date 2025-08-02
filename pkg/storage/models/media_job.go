package models

import (
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
	JobID           string            `json:"job_id"`
	MediaID         string            `json:"media_id"`
	Username        string            `json:"username"`
	Status          string            `json:"status"` // pending, processing, completed, failed
	ProcessingTasks []string          `json:"processing_tasks"`
	S3Key           string            `json:"s3_key"`
	MimeType        string            `json:"mime_type"`
	Results         map[string]any    `json:"results,omitempty"`
	Error           string            `json:"error,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Version for optimistic locking
	ModelVersion int `dynamorm:"version" json:"model_version"`
}

// TableName returns the DynamoDB table name for the MediaJob model
func (MediaJob) TableName() string {
	return "lesser-main" // Use the main table
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
	mj.GSI2PK = fmt.Sprintf("STATUS#%s", mj.Status)
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

	// Set default status
	if mj.Status == "" {
		mj.Status = "pending"
	}

	// Initialize empty results if nil
	if mj.Results == nil {
		mj.Results = make(map[string]any)
	}

	// Initialize empty processing tasks if nil
	if mj.ProcessingTasks == nil {
		mj.ProcessingTasks = []string{}
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
		return fmt.Errorf("Username is required")
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
	mj.Status = "processing"
	mj.Error = ""
	mj.UpdatedAt = time.Now()
	mj.UpdateKeys()
}

// SetCompleted marks the job as completed with results
func (mj *MediaJob) SetCompleted(results map[string]any) {
	mj.Status = "completed"
	mj.Results = results
	mj.Error = ""
	mj.UpdatedAt = time.Now()
	mj.UpdateKeys()
}

// SetFailed marks the job as failed with an error message
func (mj *MediaJob) SetFailed(errorMsg string) {
	mj.Status = "failed"
	mj.Error = errorMsg
	mj.UpdatedAt = time.Now()
	mj.UpdateKeys()
}

// IsCompleted returns true if the job is completed
func (mj *MediaJob) IsCompleted() bool {
	return mj.Status == "completed"
}

// IsFailed returns true if the job failed
func (mj *MediaJob) IsFailed() bool {
	return mj.Status == "failed"
}

// IsProcessing returns true if the job is processing
func (mj *MediaJob) IsProcessing() bool {
	return mj.Status == "processing"
}

// IsPending returns true if the job is pending
func (mj *MediaJob) IsPending() bool {
	return mj.Status == "pending"
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

// isValidJobStatus checks if the status is valid
func isValidJobStatus(status string) bool {
	validStatuses := map[string]bool{
		"pending":    true,
		"processing": true,
		"completed":  true,
		"failed":     true,
	}

	return validStatuses[strings.ToLower(status)]
}