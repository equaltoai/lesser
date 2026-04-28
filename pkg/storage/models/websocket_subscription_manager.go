package models

import (
	"fmt"
	"time"
)

// WebSocketEventConnection represents a WebSocket connection for event subscriptions
type WebSocketEventConnection struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB Keys - preserving legacy patterns
	PK string `theorydb:"pk,attr:PK" json:"pk"` // CONNECTION#{connectionID}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // METADATA

	// GSI keys for querying by user
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1pk"` // USER#{userID}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1sk"` // CONNECTION#{connectionID}

	// Business fields
	ConnectionID string    `theorydb:"attr:connectionID" json:"connection_id"`
	UserID       string    `theorydb:"attr:userID" json:"user_id"`
	Role         string    `theorydb:"attr:role,omitempty" json:"role,omitempty"`
	ConnectedAt  time.Time `theorydb:"attr:connectedAt" json:"connected_at"`
	LastSeen     time.Time `theorydb:"attr:lastSeen" json:"last_seen"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing WebSocketEventConnection.
func (WebSocketEventConnection) TableName() string {
	return MainTableName
}

// UpdateKeys sets the GSI keys based on the current values
func (w *WebSocketEventConnection) UpdateKeys() error {
	// Set primary keys
	w.PK = fmt.Sprintf(KeyPatternConnection, w.ConnectionID)
	w.SK = SKMetadata

	// Set GSI1 keys for user-based queries
	if w.UserID != "" {
		w.GSI1PK = fmt.Sprintf(KeyPatternUser, w.UserID)
		w.GSI1SK = fmt.Sprintf(KeyPatternConnection, w.ConnectionID)
	}
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (w *WebSocketEventConnection) GetPK() string {
	return w.PK
}

// GetSK returns the sort key for BaseModel interface
func (w *WebSocketEventConnection) GetSK() string {
	return w.SK
}

// WebSocketEventSubscription represents a subscription to events
type WebSocketEventSubscription struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB Keys - preserving legacy patterns
	PK string `theorydb:"pk,attr:PK" json:"pk"` // CONNECTION#{connectionID}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // SUBSCRIPTION#{subscriptionType}

	// GSI keys for querying by subscription type
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1pk"` // SUBSCRIPTION#{subscriptionType}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1sk"` // CONNECTION#{connectionID}

	// Business fields
	ConnectionID     string         `theorydb:"attr:connectionID" json:"connection_id"`
	SubscriptionType string         `theorydb:"attr:subscriptionType" json:"subscription_type"`
	Filter           map[string]any `theorydb:"attr:filter" json:"filter"`
	CreatedAt        time.Time      `theorydb:"attr:createdAt" json:"created_at"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing WebSocketEventSubscription.
func (WebSocketEventSubscription) TableName() string {
	return MainTableName
}

// UpdateKeys sets the GSI keys based on the current values
func (w *WebSocketEventSubscription) UpdateKeys() error {
	// Set primary keys
	w.PK = fmt.Sprintf(KeyPatternConnection, w.ConnectionID)
	w.SK = fmt.Sprintf("SUBSCRIPTION#%s", w.SubscriptionType)

	// Set GSI1 keys for subscription-type-based queries
	w.GSI1PK = fmt.Sprintf("SUBSCRIPTION#%s", w.SubscriptionType)
	w.GSI1SK = fmt.Sprintf(KeyPatternConnection, w.ConnectionID)
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (w *WebSocketEventSubscription) GetPK() string {
	return w.PK
}

// GetSK returns the sort key for BaseModel interface
func (w *WebSocketEventSubscription) GetSK() string {
	return w.SK
}
