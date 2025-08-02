package models

import (
	"fmt"
	"time"
)

// ThreadSync represents thread synchronization metadata
type ThreadSync struct {
	// Primary key fields
	PK string `dynamorm:"pk" json:"-"` // THREAD_SYNC#{statusID}
	SK string `dynamorm:"sk" json:"-"` // METADATA

	// Thread sync data
	StatusID         string    `json:"status_id"`
	LastSyncAt       time.Time `json:"last_sync_at"`
	SyncStatus       string    `json:"sync_status"`        // "pending", "syncing", "completed", "failed"
	MissingReplies   []string  `json:"missing_replies"`    // List of missing reply IDs
	RemoteFetched    bool      `json:"remote_fetched"`     // Whether we've attempted remote fetch
	ThreadDepth      int       `json:"thread_depth"`       // Current thread depth known
	LastErrorMessage string    `json:"last_error_message,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
	CreatedAt        time.Time `json:"created_at"`

	// TTL for auto-cleanup (30 days)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// NewThreadSync creates a new thread sync record
func NewThreadSync(statusID string) *ThreadSync {
	now := time.Now()
	return &ThreadSync{
		StatusID:       statusID,
		SyncStatus:     "pending",
		MissingReplies: []string{},
		RemoteFetched:  false,
		ThreadDepth:    0,
		CreatedAt:      now,
		UpdatedAt:      now,
		TTL:            now.Add(30 * 24 * time.Hour).Unix(),
	}
}

// UpdateKeys updates the primary key fields
func (t *ThreadSync) UpdateKeys() {
	t.PK = fmt.Sprintf("THREAD_SYNC#%s", t.StatusID)
	t.SK = "METADATA"
}

// MarkSyncing updates the sync status to "syncing"
func (t *ThreadSync) MarkSyncing() {
	t.SyncStatus = "syncing"
	t.LastSyncAt = time.Now()
	t.UpdatedAt = time.Now()
}

// MarkCompleted updates the sync status to "completed"
func (t *ThreadSync) MarkCompleted() {
	t.SyncStatus = "completed"
	t.LastSyncAt = time.Now()
	t.UpdatedAt = time.Now()
	t.LastErrorMessage = ""
}

// MarkFailed updates the sync status to "failed" with an error message
func (t *ThreadSync) MarkFailed(errorMessage string) {
	t.SyncStatus = "failed"
	t.LastErrorMessage = errorMessage
	t.UpdatedAt = time.Now()
}

// AddMissingReply adds a reply ID to the missing replies list
func (t *ThreadSync) AddMissingReply(replyID string) {
	// Check if already exists
	for _, existing := range t.MissingReplies {
		if existing == replyID {
			return
		}
	}
	t.MissingReplies = append(t.MissingReplies, replyID)
	t.UpdatedAt = time.Now()
}

// RemoveMissingReply removes a reply ID from the missing replies list
func (t *ThreadSync) RemoveMissingReply(replyID string) {
	for i, existing := range t.MissingReplies {
		if existing == replyID {
			t.MissingReplies = append(t.MissingReplies[:i], t.MissingReplies[i+1:]...)
			t.UpdatedAt = time.Now()
			break
		}
	}
}

// IsRecentlyCompleted checks if the sync was completed recently (within 30 minutes)
func (t *ThreadSync) IsRecentlyCompleted() bool {
	return t.SyncStatus == "completed" && time.Since(t.LastSyncAt) < 30*time.Minute
}