package models

import (
	"fmt"
	"time"
)

// Delivery status constants
const (
	DeliveryStatusPending   = "pending"
	DeliveryStatusDelivered = "delivered"
	DeliveryStatusFailed    = "failed"
)

// DeliveryStatus tracks the delivery status of activities to remote instances
type DeliveryStatus struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`

	// GSI fields for failed delivery queries
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK"`

	// Business fields
	ActivityID   string    `theorydb:"attr:activityID" json:"activity_id"`
	TargetDomain string    `theorydb:"attr:targetDomain" json:"target_domain"`
	Status       string    `theorydb:"attr:status" json:"status"`            // pending/delivered/failed
	Attempts     int       `theorydb:"attr:attempts" json:"attempts"`        // Number of delivery attempts
	LastAttempt  time.Time `theorydb:"attr:lastAttempt" json:"last_attempt"` // Time of last delivery attempt
	Error        string    `theorydb:"attr:error" json:"error,omitempty"`    // Error message if failed
	CreatedAt    time.Time `theorydb:"attr:createdAt" json:"created_at"`
	DeliveredAt  time.Time `theorydb:"attr:deliveredAt" json:"delivered_at,omitempty"`
	NextRetry    time.Time `theorydb:"attr:nextRetry" json:"next_retry,omitempty"`
	TTL          int64     `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"` // For automatic cleanup
}

// UpdateKeys updates the composite keys based on the delivery status data
func (d *DeliveryStatus) UpdateKeys() {
	// Primary key pattern: DELIVERY#activityID, SK: TARGET#domain
	d.PK = fmt.Sprintf("DELIVERY#%s", d.ActivityID)
	d.SK = fmt.Sprintf("TARGET#%s", d.TargetDomain)

	// GSI1 for failed delivery queries
	if d.Status == DeliveryStatusFailed && !d.NextRetry.IsZero() {
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

// TableName returns the DynamoDB table backing DeliveryStatus.
func (DeliveryStatus) TableName() string {
	return MainTableName
}
