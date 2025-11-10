package models

import (
	"fmt"
	"time"
)

// RelayInfo represents information about a federation relay
type RelayInfo struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"pk"`
	SK string `dynamorm:"sk,attr:SK" json:"sk"`

	// GSI fields for querying active relays
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsi1PK" json:"gsi1pk,omitempty"`
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsi1SK" json:"gsi1sk,omitempty"`

	// GSI fields for querying by status
	GSI2PK string `dynamorm:"index:GSI2,pk,attr:gsi2PK" json:"gsi2pk,omitempty"`
	GSI2SK string `dynamorm:"index:GSI2,sk,attr:gsi2SK" json:"gsi2sk,omitempty"`

	// Business fields
	URL        string    `dynamorm:"attr:url" json:"url"`
	InboxURL   string    `dynamorm:"attr:inboxURL" json:"inbox_url"`
	Active     bool      `dynamorm:"attr:active" json:"active"`
	CreatedAt  time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	LastSeenAt time.Time `dynamorm:"attr:lastSeenAt" json:"last_seen_at"`
	Domain     string    `dynamorm:"attr:domain" json:"domain"`
	Status     string    `dynamorm:"attr:status" json:"status,omitempty"` // pending/active/rejected/error
	ErrorCount int       `dynamorm:"attr:errorCount" json:"error_count,omitempty"`
	TTL        int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"` // For automatic cleanup
}

// TableName returns the DynamoDB table backing RelayInfo.
func (RelayInfo) TableName() string {
	return MainTableName
}

// UpdateKeys updates the composite keys based on the relay info
func (r *RelayInfo) UpdateKeys() {
	// Primary key: RELAY#domain
	r.PK = fmt.Sprintf("RELAY#%s", r.Domain)
	r.SK = SKInfo

	// GSI1: For querying active relays
	if r.Active {
		r.GSI1PK = "ACTIVE_RELAYS"
		r.GSI1SK = fmt.Sprintf("%s#%s", r.LastSeenAt.Format(time.RFC3339), r.Domain)
	} else {
		// Clear GSI1 keys when not active
		r.GSI1PK = ""
		r.GSI1SK = ""
	}

	// GSI2: For querying by status
	if r.Status != "" {
		r.GSI2PK = fmt.Sprintf("RELAY_STATUS#%s", r.Status)
		r.GSI2SK = r.Domain
	}
}

// IsExpired checks if the relay info should be cleaned up based on TTL
func (r *RelayInfo) IsExpired() bool {
	if r.TTL == 0 {
		return false
	}
	return time.Now().Unix() > r.TTL
}

// ShouldRetry determines if the relay should be retried based on error count
func (r *RelayInfo) ShouldRetry() bool {
	// Exponential backoff based on error count
	if r.ErrorCount == 0 {
		return true
	}

	// Don't retry if status is rejected
	if r.Status == "rejected" {
		return false
	}

	// Check if enough time has passed since last attempt
	backoffMinutes := 1 << r.ErrorCount // 2^errorCount minutes
	if backoffMinutes > 1440 {          // Cap at 24 hours
		backoffMinutes = 1440
	}

	nextRetry := r.LastSeenAt.Add(time.Duration(backoffMinutes) * time.Minute)
	return time.Now().After(nextRetry)
}

// SetError increments the error count and updates status
func (r *RelayInfo) SetError() {
	r.ErrorCount++
	r.LastSeenAt = time.Now()

	if r.ErrorCount > 10 {
		r.Status = "error"
		r.Active = false
	}
}

// SetSuccess resets error count and updates status
func (r *RelayInfo) SetSuccess() {
	r.ErrorCount = 0
	r.LastSeenAt = time.Now()
	r.Status = StatusActive
	r.Active = true
}
