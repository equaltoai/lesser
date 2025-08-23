package models

import (
	"fmt"
	"time"
)

// WebSocketEventConnection represents a WebSocket connection for event subscriptions
type WebSocketEventConnection struct {
	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk" json:"pk"` // CONNECTION#{connectionID}
	SK string `dynamorm:"sk" json:"sk"` // METADATA

	// GSI keys for querying by user
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // USER#{userID}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // CONNECTION#{connectionID}

	// Business fields
	ConnectionID string    `json:"connection_id"`
	UserID       string    `json:"user_id"`
	ConnectedAt  time.Time `json:"connected_at"`
	LastSeen     time.Time `json:"last_seen"`

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
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
	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk" json:"pk"` // CONNECTION#{connectionID}
	SK string `dynamorm:"sk" json:"sk"` // SUBSCRIPTION#{subscriptionType}

	// GSI keys for querying by subscription type
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // SUBSCRIPTION#{subscriptionType}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // CONNECTION#{connectionID}

	// Business fields
	ConnectionID     string         `json:"connection_id"`
	SubscriptionType string         `json:"subscription_type"`
	Filter           map[string]any `json:"filter"`
	CreatedAt        time.Time      `json:"created_at"`

	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
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
