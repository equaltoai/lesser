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
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK"`
	SK string `dynamorm:"sk,attr:SK"`

	// GSI fields for failed delivery queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK"`
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK"`

	// Business fields
	ActivityID   string    `dynamorm:"attr:activityID" json:"activity_id"`
	TargetDomain string    `dynamorm:"attr:targetDomain" json:"target_domain"`
	Status       string    `dynamorm:"attr:status" json:"status"`            // pending/delivered/failed
	Attempts     int       `dynamorm:"attr:attempts" json:"attempts"`        // Number of delivery attempts
	LastAttempt  time.Time `dynamorm:"attr:lastAttempt" json:"last_attempt"` // Time of last delivery attempt
	Error        string    `dynamorm:"attr:error" json:"error,omitempty"`    // Error message if failed
	CreatedAt    time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	DeliveredAt  time.Time `dynamorm:"attr:deliveredAt" json:"delivered_at,omitempty"`
	NextRetry    time.Time `dynamorm:"attr:nextRetry" json:"next_retry,omitempty"`
	TTL          int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"` // For automatic cleanup
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
