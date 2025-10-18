package models

import (
	"fmt"
	"time"
)

// RouteDeliveryResult represents a delivery result record for route optimization
type RouteDeliveryResult struct {
	// Primary keys
	PK string `dynamorm:"pk" json:"pk"` // ROUTE#{routeID}
	SK string `dynamorm:"sk" json:"sk"` // RESULT#{timestampNano}

	// GSI keys for time-based queries
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"gsi1pk"` // RESULTS
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"gsi1sk"` // {timestamp}#{routeID}

	// Delivery data
	MessageID    string    `json:"message_id"`
	RouteID      string    `json:"route_id"`
	Success      bool      `json:"success"`
	StatusCode   int       `json:"status_code"`
	Duration     int64     `json:"duration_ms"` // Duration in milliseconds
	BytesSent    int64     `json:"bytes_sent"`
	Cost         float64   `json:"cost"`
	ErrorMessage string    `json:"error_message,omitempty"`
	Timestamp    time.Time `json:"timestamp"`

	// TTL for cleanup
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the GSI keys based on the current data
func (r *RouteDeliveryResult) UpdateKeys() error {
	r.PK = fmt.Sprintf("ROUTE#%s", r.RouteID)
	r.SK = fmt.Sprintf("RESULT#%d", r.Timestamp.UnixNano())
	r.GSI1PK = "RESULTS"
	r.GSI1SK = fmt.Sprintf("%d#%s", r.Timestamp.Unix(), r.RouteID)

	// Set TTL for 30 days from now
	r.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key
func (r *RouteDeliveryResult) GetPK() string {
	return r.PK
}

// GetSK returns the sort key
func (r *RouteDeliveryResult) GetSK() string {
	return r.SK
}

// OptimizationDecision represents a route optimization decision record
type OptimizationDecision struct {
	// Primary keys
	PK string `dynamorm:"pk" json:"pk"` // OPTIMIZATION
	SK string `dynamorm:"sk" json:"sk"` // DECISION#{timestampNano}

	// Decision data
	Timestamp   time.Time `json:"timestamp"`
	MessageSize int64     `json:"message_size"`
	RouteIDs    []string  `json:"route_ids"`
	Decision    string    `json:"decision"` // JSON-encoded decision data

	// TTL for cleanup (7 days)
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the keys based on the current data
func (o *OptimizationDecision) UpdateKeys() error {
	o.PK = "OPTIMIZATION"
	o.SK = fmt.Sprintf("DECISION#%d", o.Timestamp.UnixNano())

	// Set TTL for 7 days from now
	o.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key
func (o *OptimizationDecision) GetPK() string {
	return o.PK
}

// GetSK returns the sort key
func (o *OptimizationDecision) GetSK() string {
	return o.SK
}
