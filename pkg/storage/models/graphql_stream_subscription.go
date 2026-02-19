package models

import (
	"fmt"
	"time"
)

// GraphQLStreamSubscription represents a GraphQL WebSocket subscription tied to a stream.
type GraphQLStreamSubscription struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - stream-based partitioning
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: GQLSUB#<stream>
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: CONN#<connectionID>#SUB#<subscriptionID>

	// GSI1 - Query by connection to support cleanup
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1pk"` // Format: CONN#<connectionID>
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1sk"` // Format: SUB#<subscriptionID>#STREAM#<stream>

	// Business fields
	ConnectionID   string    `theorydb:"attr:connectionID" json:"connection_id"`
	SubscriptionID string    `theorydb:"attr:subscriptionID" json:"subscription_id"`
	Stream         string    `theorydb:"attr:stream" json:"stream"`
	Field          string    `theorydb:"attr:field" json:"field"`
	UserID         string    `theorydb:"attr:userID" json:"user_id"`
	CreatedAt      time.Time `theorydb:"attr:createdAt" json:"created_at"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table backing GraphQLStreamSubscription.
func (GraphQLStreamSubscription) TableName() string {
	return MainTableName
}

// UpdateKeys sets PK/SK and GSI keys/state for the subscription record.
func (g *GraphQLStreamSubscription) UpdateKeys() error {
	if g.Stream == "" {
		return fmt.Errorf("stream is required")
	}
	if g.ConnectionID == "" {
		return fmt.Errorf("connection_id is required")
	}
	if g.SubscriptionID == "" {
		return fmt.Errorf("subscription_id is required")
	}
	if g.Field == "" {
		return fmt.Errorf("field is required")
	}
	if g.UserID == "" {
		return fmt.Errorf("user_id is required")
	}

	g.PK = fmt.Sprintf("GQLSUB#%s", g.Stream)
	g.SK = fmt.Sprintf("CONN#%s#SUB#%s", g.ConnectionID, g.SubscriptionID)
	g.GSI1PK = fmt.Sprintf("CONN#%s", g.ConnectionID)
	g.GSI1SK = fmt.Sprintf("SUB#%s#STREAM#%s", g.SubscriptionID, g.Stream)
	return nil
}

// GetPK returns the partition key (required for BaseModel).
func (g *GraphQLStreamSubscription) GetPK() string {
	return g.PK
}

// GetSK returns the sort key (required for BaseModel).
func (g *GraphQLStreamSubscription) GetSK() string {
	return g.SK
}
