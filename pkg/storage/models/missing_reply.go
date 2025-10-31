package models

import (
	"fmt"
	"time"
)

// MissingReply represents a reply that we know should exist but haven't fetched yet
// Used for tracking gaps in thread synchronization
type MissingReply struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"-"` // THREAD#{rootStatusID}
	SK string `dynamorm:"sk" json:"-"` // MISSING#{replyID}

	// GSI for querying by parent status
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"-"` // STATUS#{parentStatusID}
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"-"` // MISSING_REPLY

	// Missing reply data
	RootStatusID   string     `json:"root_status_id"`            // Root of the thread
	ParentStatusID string     `json:"parent_status_id"`          // Parent that references this reply
	ReplyID        string     `json:"reply_id"`                  // The missing reply ID (could be URL)
	ReplyURL       string     `json:"reply_url,omitempty"`       // URL if known
	DetectedAt     time.Time  `json:"detected_at"`               // When we first detected this gap
	LastAttemptAt  *time.Time `json:"last_attempt_at,omitempty"` // Last fetch attempt
	AttemptCount   int        `json:"attempt_count"`             // Number of fetch attempts
	LastError      string     `json:"last_error,omitempty"`      // Last error message
	Status         string     `json:"status"`                    // pending, fetching, failed, resolved
	FailureReason  string     `json:"failure_reason,omitempty"`  // deleted, 404, 403, timeout, etc.
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`     // When the reply was successfully fetched
	UpdatedAt      time.Time  `json:"updated_at"`                // Last update

	// Metadata for retry logic
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"` // When to retry (exponential backoff)
	Priority    int        `json:"priority"`                // Priority for fetching (1=high, 5=low)

	// TTL for auto-cleanup (7 days for resolved, 30 days for failed)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

const (
	// MissingReplyStatusPending indicates the reply hasn't been fetched yet
	MissingReplyStatusPending = "pending"
	// MissingReplyStatusFetching indicates the reply is currently being fetched
	MissingReplyStatusFetching = "fetching"
	// MissingReplyStatusFailed indicates the reply fetch failed
	MissingReplyStatusFailed = "failed"
	// MissingReplyStatusResolved indicates the reply was successfully fetched
	MissingReplyStatusResolved = "resolved"
)

// Failure reason constants
const (
	// FailureReasonDeleted indicates the reply was deleted (410 Gone)
	FailureReasonDeleted = "deleted"
	// FailureReasonNotFound indicates the reply was not found (404)
	FailureReasonNotFound = "not_found"
	// FailureReasonForbidden indicates access was forbidden (403)
	FailureReasonForbidden = "forbidden"
	// FailureReasonTimeout indicates the request timed out
	FailureReasonTimeout = "timeout"
	// FailureReasonUnreachable indicates the instance was unreachable
	FailureReasonUnreachable = "unreachable"
	// FailureReasonInvalid indicates the response was invalid
	FailureReasonInvalid = "invalid"
)

// NewMissingReply creates a new missing reply record
func NewMissingReply(rootStatusID, parentStatusID, replyID string) *MissingReply {
	now := time.Now()
	return &MissingReply{
		RootStatusID:   rootStatusID,
		ParentStatusID: parentStatusID,
		ReplyID:        replyID,
		DetectedAt:     now,
		AttemptCount:   0,
		Status:         MissingReplyStatusPending,
		UpdatedAt:      now,
		Priority:       3, // Default medium priority
		TTL:            now.Add(30 * 24 * time.Hour).Unix(),
	}
}

// UpdateKeys updates the primary and GSI keys
func (m *MissingReply) UpdateKeys() error {
	m.PK = fmt.Sprintf("THREAD#%s", m.RootStatusID)
	m.SK = fmt.Sprintf("MISSING#%s", m.ReplyID)
	m.GSI1PK = fmt.Sprintf(KeyPatternStatus, m.ParentStatusID)
	m.GSI1SK = "MISSING_REPLY"
	return nil
}

// GetPK returns the primary key
func (m *MissingReply) GetPK() string {
	return m.PK
}

// GetSK returns the sort key
func (m *MissingReply) GetSK() string {
	return m.SK
}

// MarkFetching marks the reply as being fetched
func (m *MissingReply) MarkFetching() {
	m.Status = MissingReplyStatusFetching
	now := time.Now()
	m.LastAttemptAt = &now
	m.AttemptCount++
	m.UpdatedAt = now
}

// MarkResolved marks the reply as successfully fetched
func (m *MissingReply) MarkResolved() {
	m.Status = MissingReplyStatusResolved
	now := time.Now()
	m.ResolvedAt = &now
	m.UpdatedAt = now
	m.LastError = ""
	m.FailureReason = ""
	// Set TTL to 7 days for resolved replies
	m.TTL = now.Add(7 * 24 * time.Hour).Unix()
}

// MarkFailed marks the reply fetch as failed with a reason
func (m *MissingReply) MarkFailed(errorMsg, failureReason string) {
	m.Status = MissingReplyStatusFailed
	m.LastError = errorMsg
	m.FailureReason = failureReason
	m.UpdatedAt = time.Now()

	// Calculate next retry time with exponential backoff
	m.calculateNextRetry()
}

// calculateNextRetry calculates the next retry time based on attempt count
func (m *MissingReply) calculateNextRetry() {
	// Exponential backoff: 5min, 15min, 1hr, 6hr, 24hr
	var delay time.Duration
	switch m.AttemptCount {
	case 1:
		delay = 5 * time.Minute
	case 2:
		delay = 15 * time.Minute
	case 3:
		delay = 1 * time.Hour
	case 4:
		delay = 6 * time.Hour
	default:
		delay = 24 * time.Hour
	}

	// Don't retry for permanent failures
	if m.IsPermanentFailure() {
		m.NextRetryAt = nil
		return
	}

	nextRetry := time.Now().Add(delay)
	m.NextRetryAt = &nextRetry
}

// IsPermanentFailure checks if the failure is permanent (no need to retry)
func (m *MissingReply) IsPermanentFailure() bool {
	return m.FailureReason == FailureReasonDeleted ||
		m.FailureReason == FailureReasonForbidden ||
		m.FailureReason == FailureReasonInvalid ||
		m.AttemptCount >= 5 // Give up after 5 attempts
}

// ShouldRetry checks if we should retry fetching this reply
func (m *MissingReply) ShouldRetry() bool {
	if m.Status != MissingReplyStatusFailed {
		return false
	}
	if m.IsPermanentFailure() {
		return false
	}
	if m.NextRetryAt == nil {
		return false
	}
	return time.Now().After(*m.NextRetryAt)
}

// SetPriority sets the fetch priority
func (m *MissingReply) SetPriority(priority int) {
	if priority < 1 {
		priority = 1
	}
	if priority > 5 {
		priority = 5
	}
	m.Priority = priority
	m.UpdatedAt = time.Now()
}

// TableName returns the DynamoDB table backing MissingReply.
func (MissingReply) TableName() string {
	return MainTableName
}
