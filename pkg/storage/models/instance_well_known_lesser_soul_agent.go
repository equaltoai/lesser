package models

import "time"

// InstanceWellKnownLesserSoulAgent stores the current HTTPS proof value served at
// GET /.well-known/lesser-soul-agent.
//
// This record lives under PK="INSTANCE#CONFIG" and SK="WELL_KNOWN_LESSER_SOUL_AGENT".
type InstanceWellKnownLesserSoulAgent struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	// ProofValue is served at `GET /.well-known/lesser-soul-agent` as:
	// `{"lesser-soul-agent":"<token>"}`.
	ProofValue string `theorydb:"attr:proofValue,omitempty" json:"proof_value,omitempty"`

	// ExpiresAt gates responses so old proof tokens don't linger indefinitely.
	ExpiresAt time.Time `theorydb:"attr:expiresAt,omitempty" json:"expires_at,omitempty"`

	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing InstanceWellKnownLesserSoulAgent.
func (InstanceWellKnownLesserSoulAgent) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys.
func (c *InstanceWellKnownLesserSoulAgent) UpdateKeys() error {
	c.PK = instanceConfigPK
	c.SK = SKWellKnownLesserSoulAgent
	return nil
}

// GetPK returns the partition key.
func (c *InstanceWellKnownLesserSoulAgent) GetPK() string {
	return c.PK
}

// GetSK returns the sort key.
func (c *InstanceWellKnownLesserSoulAgent) GetSK() string {
	return c.SK
}

// NewInstanceWellKnownLesserSoulAgent creates a new record with built-in defaults.
func NewInstanceWellKnownLesserSoulAgent() *InstanceWellKnownLesserSoulAgent {
	return &InstanceWellKnownLesserSoulAgent{
		PK:         instanceConfigPK,
		SK:         SKWellKnownLesserSoulAgent,
		ProofValue: "",
		ExpiresAt:  time.Time{},
		UpdatedAt:  time.Now().UTC(),
	}
}
