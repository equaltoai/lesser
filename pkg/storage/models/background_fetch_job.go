package models

import (
	"fmt"
	"time"
)

// BackgroundFetchJob represents a job for fetching remote content
type BackgroundFetchJob struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "FETCH_JOB#{job_id}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "JOB#{timestamp}"

	// GSI for querying by status
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"-"` // STATUS#{status_id}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"-"` // FETCH#{timestamp}

	// Job identification
	JobID    string `theorydb:"attr:jobID" json:"job_id"`       // Unique job identifier
	StatusID string `theorydb:"attr:statusID" json:"status_id"` // Status/object to fetch

	// Job configuration
	FetchType  string `theorydb:"attr:fetchType" json:"fetch_type"`   // "thread_sync", "object_fetch", "media_fetch"
	Priority   string `theorydb:"attr:priority" json:"priority"`      // "low", "normal", "high", "urgent"
	MaxRetries int    `theorydb:"attr:maxRetries" json:"max_retries"` // Maximum retry attempts

	// Job state
	Status       string     `theorydb:"attr:status" json:"status"`              // "pending", "running", "completed", "failed"
	Attempts     int        `theorydb:"attr:attempts" json:"attempts"`          // Current attempt count
	LastAttempt  *time.Time `theorydb:"attr:lastAttempt" json:"last_attempt"`   // Last attempt timestamp
	NextAttempt  *time.Time `theorydb:"attr:nextAttempt" json:"next_attempt"`   // Next scheduled attempt
	CompletedAt  *time.Time `theorydb:"attr:completedAt" json:"completed_at"`   // Completion timestamp
	LastError    string     `theorydb:"attr:lastError" json:"last_error"`       // Last error message
	ErrorDetails string     `theorydb:"attr:errorDetails" json:"error_details"` // Detailed error information

	// Fetch metadata
	RemoteURL     string            `theorydb:"attr:remoteURL" json:"remote_url"`         // URL to fetch from
	FetchMetadata map[string]string `theorydb:"attr:fetchMetadata" json:"fetch_metadata"` // Additional fetch parameters
	UserAgent     string            `theorydb:"attr:userAgent" json:"user_agent"`         // User agent for fetching

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// TTL for auto-cleanup (7 days)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// NewBackgroundFetchJob creates a new background fetch job
func NewBackgroundFetchJob(statusID, fetchType string) *BackgroundFetchJob {
	now := time.Now()
	jobID := fmt.Sprintf("fetch_%d", now.UnixNano())

	job := &BackgroundFetchJob{
		JobID:         jobID,
		StatusID:      statusID,
		FetchType:     fetchType,
		Priority:      "normal",
		MaxRetries:    3,
		Status:        StatusPending,
		Attempts:      0,
		FetchMetadata: make(map[string]string),
		CreatedAt:     now,
		UpdatedAt:     now,
		TTL:           now.Add(7 * 24 * time.Hour).Unix(),
	}
	job.UpdateKeys()
	return job
}

// UpdateKeys updates the DynamoDB keys
func (j *BackgroundFetchJob) UpdateKeys() {
	j.PK = fmt.Sprintf("FETCH_JOB#%s", j.JobID)
	j.SK = fmt.Sprintf("JOB#%d", j.CreatedAt.Unix())
	j.GSI1PK = fmt.Sprintf("STATUS#%s", j.StatusID)
	j.GSI1SK = fmt.Sprintf("FETCH#%d", j.CreatedAt.Unix())
}

// BeforeCreate is called before creating the record
func (j *BackgroundFetchJob) BeforeCreate() error {
	now := time.Now()
	j.CreatedAt = now
	j.UpdatedAt = now
	j.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (j *BackgroundFetchJob) BeforeUpdate() error {
	j.UpdatedAt = time.Now()
	j.UpdateKeys()
	return nil
}

// TableName returns the DynamoDB table name
func (BackgroundFetchJob) TableName() string {
	return MainTableName
}

// MarkRunning marks the job as running
func (j *BackgroundFetchJob) MarkRunning() {
	now := time.Now()
	j.Status = StatusProcessing
	j.Attempts++
	j.LastAttempt = &now
}

// MarkCompleted marks the job as completed
func (j *BackgroundFetchJob) MarkCompleted() {
	now := time.Now()
	j.Status = StatusCompleted
	j.CompletedAt = &now
	j.LastError = ""
	j.ErrorDetails = ""
}

// MarkFailed marks the job as failed and schedules retry if applicable
func (j *BackgroundFetchJob) MarkFailed(errorMsg, errorDetails string) {
	j.Status = StatusFailed
	j.LastError = errorMsg
	j.ErrorDetails = errorDetails

	// Schedule retry if attempts < max retries
	if j.Attempts < j.MaxRetries {
		// Exponential backoff: 5min, 15min, 45min
		var delayMinutes int64
		switch j.Attempts {
		case 1:
			delayMinutes = 5
		case 2:
			delayMinutes = 15
		case 3:
			delayMinutes = 45
		default:
			delayMinutes = 60
		}

		nextAttempt := time.Now().Add(time.Duration(delayMinutes) * time.Minute)
		j.NextAttempt = &nextAttempt
		j.Status = StatusPending // Reset to pending for retry
	}
}

// IsRetryable returns whether the job can be retried
func (j *BackgroundFetchJob) IsRetryable() bool {
	return j.Attempts < j.MaxRetries && j.Status != StatusCompleted
}

// IsReady returns whether the job is ready to be processed
func (j *BackgroundFetchJob) IsReady() bool {
	if j.Status != StatusPending {
		return false
	}
	if j.NextAttempt == nil {
		return true
	}
	return time.Now().After(*j.NextAttempt)
}

// SetRemoteURL sets the remote URL to fetch from
func (j *BackgroundFetchJob) SetRemoteURL(url string) {
	j.RemoteURL = url
}

// AddMetadata adds metadata for the fetch job
func (j *BackgroundFetchJob) AddMetadata(key, value string) {
	if j.FetchMetadata == nil {
		j.FetchMetadata = make(map[string]string)
	}
	j.FetchMetadata[key] = value
}

// GetMetadata gets metadata value by key
func (j *BackgroundFetchJob) GetMetadata(key string) string {
	if j.FetchMetadata == nil {
		return ""
	}
	return j.FetchMetadata[key]
}
