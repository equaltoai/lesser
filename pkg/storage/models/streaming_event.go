package models

import (
	"fmt"
	"time"
)

// StreamingEvent represents a queued streaming event in DynamoDB
// These events are picked up by DynamoDB Streams and processed by the stream-router Lambda
type StreamingEvent struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - unique event ID
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "STREAM_EVENT#{eventID}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "EVENT"

	// GSI1 - Query by target (user, stream, conversation, followers)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "STREAM_TARGET#{targetType}#{targetID}"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{createdAt}#{eventID}"

	// GSI2 - Query by event type
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"` // Format: "STREAM_TYPE#{eventType}"
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"` // Format: "{createdAt}#{eventID}"

	// Core event data
	EventID    string                 `theorydb:"attr:eventID" json:"event_id"`
	EventType  string                 `theorydb:"attr:eventType" json:"event_type"`   // e.g., "status.created", "notification.created"
	TargetType string                 `theorydb:"attr:targetType" json:"target_type"` // "user", "stream", "conversation", "followers"
	TargetID   string                 `theorydb:"attr:targetID" json:"target_id"`     // The specific user/stream/conversation ID
	Payload    map[string]interface{} `theorydb:"attr:payload" json:"payload"`        // The event data to send

	// Metadata
	CreatedAt   time.Time  `theorydb:"attr:createdAt" json:"created_at"`
	ProcessedAt *time.Time `theorydb:"attr:processedAt" json:"processed_at,omitempty"` // When stream-router processed it
	DeliveredTo []string   `theorydb:"attr:deliveredTo" json:"delivered_to,omitempty"` // Connection IDs it was delivered to

	// TTL for automatic cleanup (Unix timestamp)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the GSI keys based on the event data
func (e *StreamingEvent) UpdateKeys() {
	// Primary key
	e.PK = fmt.Sprintf("STREAM_EVENT#%s", e.EventID)
	e.SK = "EVENT"

	// GSI1 - For querying by target
	e.GSI1PK = fmt.Sprintf("STREAM_TARGET#%s#%s", e.TargetType, e.TargetID)
	e.GSI1SK = fmt.Sprintf("%d#%s", e.CreatedAt.Unix(), e.EventID)

	// GSI2 - For querying by event type
	e.GSI2PK = fmt.Sprintf("STREAM_TYPE#%s", e.EventType)
	e.GSI2SK = fmt.Sprintf("%d#%s", e.CreatedAt.Unix(), e.EventID)
}

// GetTableName returns the table name for this model
func (e *StreamingEvent) GetTableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table backing StreamingEvent.
func (e *StreamingEvent) TableName() string {
	return MainTableName
}

// GetPrimaryKey returns the primary key for this model
func (e *StreamingEvent) GetPrimaryKey() (string, string) {
	return e.PK, e.SK
}
