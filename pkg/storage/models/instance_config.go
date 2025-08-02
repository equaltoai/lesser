package models

import (
	"time"
)

// InstanceConfig represents instance configuration data stored in DynamoDB
// Matches legacy InstanceData structure from dynamodb/instance.go
type InstanceConfig struct {
	// Primary key fields - EXACT pattern from legacy
	PK string `dynamorm:"pk" json:"-"` // INSTANCE#CONFIG
	SK string `dynamorm:"sk" json:"-"` // RULES or EXTENDED_DESC

	// Configuration data - use storage.InstanceRule to avoid dependency cycle
	RulesJSON           string    `json:"rules_json,omitempty"`           // JSON serialized rules
	ExtendedDescription string    `json:"extended_description,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// UpdateKeys updates the DynamoDB keys (no GSI needed for this simple structure)
func (c *InstanceConfig) UpdateKeys() {
	// Keys are set explicitly when creating/updating records
	// PK will be "INSTANCE#CONFIG"
	// SK will be "RULES" or "EXTENDED_DESC"
}

// NewInstanceRulesConfig creates a new config for storing rules
func NewInstanceRulesConfig(rulesJSON string) *InstanceConfig {
	return &InstanceConfig{
		PK:        "INSTANCE#CONFIG",
		SK:        "RULES",
		RulesJSON: rulesJSON,
		UpdatedAt: time.Now(),
	}
}

// NewExtendedDescriptionConfig creates a new config for storing extended description
func NewExtendedDescriptionConfig(description string) *InstanceConfig {
	return &InstanceConfig{
		PK:                  "INSTANCE#CONFIG",
		SK:                  "EXTENDED_DESC",
		ExtendedDescription: description,
		UpdatedAt:           time.Now(),
	}
}