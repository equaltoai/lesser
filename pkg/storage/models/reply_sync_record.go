package models

import (
	"fmt"
	"time"
)

// ReplySyncRecord tracks remote reply synchronization attempts
type ReplySyncRecord struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"pk"` // Format: "REPLY_SYNC#{status_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "SYNC#{timestamp}"

	// Core fields
	StatusID    string    `json:"status_id"`    // The status we're syncing replies for
	SyncAttempt time.Time `json:"sync_attempt"` // When the sync was attempted
	SyncResult  string    `json:"sync_result"`  // "success", "partial", "failed"
	
	// Sync details
	TotalReplies   int `json:"total_replies"`   // Total replies known to exist
	FetchedReplies int `json:"fetched_replies"` // Successfully fetched replies
	FailedReplies  int `json:"failed_replies"`  // Failed to fetch
	
	// Error tracking
	LastError     string `json:"last_error,omitempty"`     // Last error message
	RetryCount    int    `json:"retry_count"`              // Number of retries
	NextRetryAt   *time.Time `json:"next_retry_at,omitempty"` // When to retry next
	
	// TTL for automatic cleanup (30 days)
	TTL int64 `dynamorm:"ttl" json:"ttl"`
	
	// Timestamps
	CreatedAt time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// NewReplySyncRecord creates a new reply sync record
func NewReplySyncRecord(statusID string) *ReplySyncRecord {
	now := time.Now()
	record := &ReplySyncRecord{
		StatusID:    statusID,
		SyncAttempt: now,
		SyncResult:  "pending",
		RetryCount:  0,
		TTL:         now.Add(30 * 24 * time.Hour).Unix(), // 30 days TTL
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	record.UpdateKeys()
	return record
}

// UpdateKeys updates the DynamoDB keys
func (r *ReplySyncRecord) UpdateKeys() {
	r.PK = fmt.Sprintf("REPLY_SYNC#%s", r.StatusID)
	r.SK = fmt.Sprintf("SYNC#%d", r.SyncAttempt.Unix())
}

// BeforeCreate is called before creating the record
func (r *ReplySyncRecord) BeforeCreate() error {
	now := time.Now()
	r.CreatedAt = now
	r.UpdatedAt = now
	r.UpdateKeys()
	return nil
}

// BeforeUpdate is called before updating the record
func (r *ReplySyncRecord) BeforeUpdate() error {
	r.UpdatedAt = time.Now()
	r.UpdateKeys()
	return nil
}

// TableName returns the DynamoDB table name
func (ReplySyncRecord) TableName() string {
	return "lesser-main" // Use the main table
}

// MarkSuccess marks the sync as successful
func (r *ReplySyncRecord) MarkSuccess(fetched int) {
	r.SyncResult = "success"
	r.FetchedReplies = fetched
	r.LastError = ""
	r.NextRetryAt = nil
}

// MarkPartial marks the sync as partially successful
func (r *ReplySyncRecord) MarkPartial(fetched, failed int) {
	r.SyncResult = "partial"
	r.FetchedReplies = fetched
	r.FailedReplies = failed
}

// MarkFailed marks the sync as failed and schedules retry
func (r *ReplySyncRecord) MarkFailed(errorMsg string) {
	r.SyncResult = "failed"
	r.LastError = errorMsg
	r.RetryCount++
	
	// Exponential backoff: 1h, 4h, 16h, then 24h max
	retryDelay := time.Duration(1<<uint(r.RetryCount)) * time.Hour
	if retryDelay > 24*time.Hour {
		retryDelay = 24 * time.Hour
	}
	
	nextRetry := time.Now().Add(retryDelay)
	r.NextRetryAt = &nextRetry
}

// ShouldRetry returns whether this sync should be retried
func (r *ReplySyncRecord) ShouldRetry() bool {
	if r.SyncResult == "success" {
		return false
	}
	
	if r.RetryCount >= 5 { // Max 5 retries
		return false
	}
	
	if r.NextRetryAt == nil {
		return true // No retry time set, can retry immediately
	}
	
	return time.Now().After(*r.NextRetryAt)
}

// IsExpired returns whether this record should be cleaned up
func (r *ReplySyncRecord) IsExpired() bool {
	return time.Now().Unix() > r.TTL
}