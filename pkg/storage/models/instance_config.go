package models

import (
	"time"
)

// InstanceConfig represents instance configuration data stored in DynamoDB
// Matches legacy InstanceData structure from dynamodb/instance.go
type InstanceConfig struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields - EXACT pattern from legacy
	PK string `dynamorm:"pk,attr:PK" json:"-"` // INSTANCE#CONFIG
	SK string `dynamorm:"sk,attr:SK" json:"-"` // RULES or EXTENDED_DESC

	// Configuration data - use storage.InstanceRule to avoid dependency cycle
	RulesJSON           string    `dynamorm:"attr:rulesJSON" json:"rules_json,omitempty"` // JSON serialized rules
	ExtendedDescription string    `dynamorm:"attr:extendedDescription" json:"extended_description,omitempty"`
	UpdatedAt           time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing InstanceConfig.
func (InstanceConfig) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys (no GSI needed for this simple structure)
func (c *InstanceConfig) UpdateKeys() error {
	// Keys are set explicitly when creating/updating records
	// PK will be "INSTANCE#CONFIG"
	// SK will be "RULES" or "EXTENDED_DESC"
	return nil
}

// GetPK returns the partition key
func (c *InstanceConfig) GetPK() string {
	return c.PK
}

// GetSK returns the sort key
func (c *InstanceConfig) GetSK() string {
	return c.SK
}

// NewInstanceRulesConfig creates a new config for storing rules
func NewInstanceRulesConfig(rulesJSON string) *InstanceConfig {
	return &InstanceConfig{
		PK:        instanceConfigPK,
		SK:        "RULES",
		RulesJSON: rulesJSON,
		UpdatedAt: time.Now(),
	}
}

// NewExtendedDescriptionConfig creates a new config for storing extended description
func NewExtendedDescriptionConfig(description string) *InstanceConfig {
	return &InstanceConfig{
		PK:                  instanceConfigPK,
		SK:                  "EXTENDED_DESC",
		ExtendedDescription: description,
		UpdatedAt:           time.Now(),
	}
}

// AIInstanceConfig represents AI-specific instance configuration
type AIInstanceConfig struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"` // INSTANCE#CONFIG
	SK string `dynamorm:"sk,attr:SK" json:"-"` // AI_CONFIG

	// AI Configuration
	AIEnabled            bool      `dynamorm:"attr:aiEnabled" json:"ai_enabled"`
	ModerationEnabled    bool      `dynamorm:"attr:moderationEnabled" json:"moderation_enabled"`
	NSFWDetectionEnabled bool      `dynamorm:"attr:nsfwDetectionEnabled" json:"nsfw_detection_enabled"`
	SpamDetectionEnabled bool      `dynamorm:"attr:spamDetectionEnabled" json:"spam_detection_enabled"`
	PIIDetectionEnabled  bool      `dynamorm:"attr:piiDetectionEnabled" json:"pii_detection_enabled"`
	AIContentDetection   bool      `dynamorm:"attr:aiContentDetection" json:"ai_content_detection_enabled"`
	UpdatedAt            time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing AIInstanceConfig.
func (AIInstanceConfig) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys
func (c *AIInstanceConfig) UpdateKeys() error {
	c.PK = instanceConfigPK
	c.SK = "AI_CONFIG"
	return nil
}

// GetPK returns the partition key
func (c *AIInstanceConfig) GetPK() string {
	return c.PK
}

// GetSK returns the sort key
func (c *AIInstanceConfig) GetSK() string {
	return c.SK
}

// NewAIInstanceConfig creates a new AI config with defaults
func NewAIInstanceConfig() *AIInstanceConfig {
	return &AIInstanceConfig{
		PK:                   instanceConfigPK,
		SK:                   "AI_CONFIG",
		AIEnabled:            true,  // Default to enabled
		ModerationEnabled:    true,  // Default to enabled
		NSFWDetectionEnabled: true,  // Default to enabled
		SpamDetectionEnabled: true,  // Default to enabled
		PIIDetectionEnabled:  false, // Default to disabled (privacy)
		AIContentDetection:   false, // Default to disabled
		UpdatedAt:            time.Now(),
	}
}
