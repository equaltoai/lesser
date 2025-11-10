package models

import (
	"fmt"
	"time"
)

// RouteDeliveryResult represents a delivery result record for route optimization
type RouteDeliveryResult struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // ROUTE#{routeID}
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // RESULT#{timestampNano}

	// GSI keys for time-based queries
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsI1PK" json:"gsi1pk"` // RESULTS
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsI1SK" json:"gsi1sk"` // {timestamp}#{routeID}

	// Delivery data
	MessageID    string    `dynamorm:"attr:messageID" json:"message_id"`
	RouteID      string    `dynamorm:"attr:routeID" json:"route_id"`
	Success      bool      `dynamorm:"attr:success" json:"success"`
	StatusCode   int       `dynamorm:"attr:statusCode" json:"status_code"`
	Duration     int64     `dynamorm:"attr:duration" json:"duration_ms"` // Duration in milliseconds
	BytesSent    int64     `dynamorm:"attr:bytesSent" json:"bytes_sent"`
	Cost         float64   `dynamorm:"attr:cost" json:"cost"`
	ErrorMessage string    `dynamorm:"attr:errorMessage" json:"error_message,omitempty"`
	Timestamp    time.Time `dynamorm:"attr:timestamp" json:"timestamp"`

	// TTL for cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
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

// TableName returns the DynamoDB table backing RouteDeliveryResult.
func (RouteDeliveryResult) TableName() string {
	return MainTableName
}

// OptimizationDecision represents a route optimization decision record
type OptimizationDecision struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // OPTIMIZATION
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // DECISION#{timestampNano}

	// Decision data
	Timestamp   time.Time `dynamorm:"attr:timestamp" json:"timestamp"`
	MessageSize int64     `dynamorm:"attr:messageSize" json:"message_size"`
	RouteIDs    []string  `dynamorm:"attr:routeIDs" json:"route_ids"`
	Decision    string    `dynamorm:"attr:decision" json:"decision"` // JSON-encoded decision data

	// TTL for cleanup (7 days)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
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

// TableName returns the DynamoDB table backing OptimizationDecision.
func (OptimizationDecision) TableName() string {
	return MainTableName
}
