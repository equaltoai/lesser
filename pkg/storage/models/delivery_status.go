package models

import (
	"fmt"
	"time"
)

// DeliveryStatus tracks the delivery status of activities to remote instances
type DeliveryStatus struct {
	// Primary key fields
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`
	
	// GSI fields for failed delivery queries
	GSI1PK string `dynamorm:"index:gsi1,pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk"`
	
	// Business fields
	ActivityID    string    `json:"activity_id"`
	TargetDomain  string    `json:"target_domain"`
	Status        string    `json:"status"`         // pending/delivered/failed
	Attempts      int       `json:"attempts"`       // Number of delivery attempts
	LastAttempt   time.Time `json:"last_attempt"`   // Time of last delivery attempt
	Error         string    `json:"error,omitempty"` // Error message if failed
	CreatedAt     time.Time `json:"created_at"`
	DeliveredAt   time.Time `json:"delivered_at,omitempty"`
	NextRetry     time.Time `json:"next_retry,omitempty"`
	TTL           int64     `json:"ttl,omitempty" dynamorm:"ttl"` // For automatic cleanup
}

// UpdateKeys updates the composite keys based on the delivery status data
func (d *DeliveryStatus) UpdateKeys() {
	// Primary key pattern: DELIVERY#activityID, SK: TARGET#domain
	d.PK = fmt.Sprintf("DELIVERY#%s", d.ActivityID)
	d.SK = fmt.Sprintf("TARGET#%s", d.TargetDomain)
	
	// GSI1 for failed delivery queries
	if d.Status == "failed" && !d.NextRetry.IsZero() {
		d.GSI1PK = "FAILED_DELIVERIES"
		d.GSI1SK = fmt.Sprintf("%d#%s#%s", d.NextRetry.Unix(), d.TargetDomain, d.ActivityID)
	} else {
		// Clear GSI1 keys when not failed or no retry scheduled
		d.GSI1PK = ""
		d.GSI1SK = ""
	}
	
	// Set TTL for automatic cleanup (30 days after creation or delivery)
	if !d.DeliveredAt.IsZero() {
		d.TTL = d.DeliveredAt.Add(30 * 24 * time.Hour).Unix()
	} else {
		d.TTL = d.CreatedAt.Add(30 * 24 * time.Hour).Unix()
	}
}