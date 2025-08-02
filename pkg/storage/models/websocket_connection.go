package models

import (
	"fmt"
	"time"
)

// WebSocketConnection represents a WebSocket connection with user context
type WebSocketConnection struct {
	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk" json:"pk"` // CONN#{connectionID}
	SK string `dynamorm:"sk" json:"sk"` // CONN#{connectionID}
	
	// GSI keys for querying
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // USER#{userID}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // CONN#{timestamp}
	
	// Business fields
	ConnectionID string    `json:"connection_id"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	Streams      []string  `json:"streams"`        // subscribed streams
	Established  time.Time `json:"established"`
	LastActivity time.Time `json:"last_activity"`
	
	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the GSI keys based on the current values
func (w *WebSocketConnection) UpdateKeys() {
	// Set primary keys
	w.PK = fmt.Sprintf("CONN#%s", w.ConnectionID)
	w.SK = fmt.Sprintf("CONN#%s", w.ConnectionID)
	
	// Set GSI1 keys for user-based queries
	if w.UserID != "" {
		w.GSI1PK = fmt.Sprintf("USER#%s", w.UserID)
		w.GSI1SK = fmt.Sprintf("CONN#%s", w.Established.Format(time.RFC3339))
	}
}

// WebSocketSubscription represents a stream subscription for a WebSocket connection
type WebSocketSubscription struct {
	// DynamoDB Keys - preserving legacy patterns
	PK string `dynamorm:"pk" json:"pk"` // SUB#{stream}
	SK string `dynamorm:"sk" json:"sk"` // CONN#{connectionID}
	
	// GSI keys for querying
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1pk"` // CONN#{connectionID}
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1sk"` // STREAM#{stream}
	
	// Business fields
	ConnectionID string `json:"connection_id"`
	UserID       string `json:"user_id"`
	Stream       string `json:"stream"`
	SubscribedAt time.Time `json:"subscribed_at"`
	
	// TTL for automatic cleanup
	TTL int64 `json:"ttl,omitempty" dynamorm:"ttl"`
}

// UpdateKeys sets the GSI keys based on the current values
func (w *WebSocketSubscription) UpdateKeys() {
	// Set primary keys
	w.PK = fmt.Sprintf("SUB#%s", w.Stream)
	w.SK = fmt.Sprintf("CONN#%s", w.ConnectionID)
	
	// Set GSI1 keys for connection-based queries
	w.GSI1PK = fmt.Sprintf("CONN#%s", w.ConnectionID)
	w.GSI1SK = fmt.Sprintf("STREAM#%s", w.Stream)
}