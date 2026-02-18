package models

import (
	"time"
)

// InstanceConfig represents instance configuration data stored in DynamoDB
// Matches legacy InstanceData structure from dynamodb/instance.go
type InstanceConfig struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields - EXACT pattern from legacy
	PK string `theorydb:"pk,attr:PK" json:"-"` // INSTANCE#CONFIG
	SK string `theorydb:"sk,attr:SK" json:"-"` // RULES or EXTENDED_DESC

	// Configuration data - use storage.InstanceRule to avoid dependency cycle
	RulesJSON           string    `theorydb:"attr:rulesJSON" json:"rules_json,omitempty"` // JSON serialized rules
	ExtendedDescription string    `theorydb:"attr:extendedDescription" json:"extended_description,omitempty"`
	UpdatedAt           time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"-"` // INSTANCE#CONFIG
	SK string `theorydb:"sk,attr:SK" json:"-"` // AI_CONFIG

	Managed  *AIInstanceConfigManaged  `theorydb:"attr:managed" json:"managed"`
	Override *AIInstanceConfigOverride `theorydb:"attr:override,omitempty" json:"override,omitempty"`

	// Legacy flat attributes (read-only).
	// These are preserved for backwards compatibility when reading older records.
	LegacyAIEnabled            bool `theorydb:"attr:aiEnabled" json:"-"`
	LegacyModerationEnabled    bool `theorydb:"attr:moderationEnabled" json:"-"`
	LegacyNSFWDetectionEnabled bool `theorydb:"attr:nsfwDetectionEnabled" json:"-"`
	LegacySpamDetectionEnabled bool `theorydb:"attr:spamDetectionEnabled" json:"-"`
	LegacyPIIDetectionEnabled  bool `theorydb:"attr:piiDetectionEnabled" json:"-"`
	LegacyAIContentDetection   bool `theorydb:"attr:aiContentDetection" json:"-"`

	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// AIInstanceConfigManaged stores managed/default AI configuration values for an instance.
type AIInstanceConfigManaged struct {
	AIEnabled            bool `theorydb:"attr:aiEnabled" json:"ai_enabled"`
	ModerationEnabled    bool `theorydb:"attr:moderationEnabled" json:"moderation_enabled"`
	NSFWDetectionEnabled bool `theorydb:"attr:nsfwDetectionEnabled" json:"nsfw_detection_enabled"`
	SpamDetectionEnabled bool `theorydb:"attr:spamDetectionEnabled" json:"spam_detection_enabled"`
	PIIDetectionEnabled  bool `theorydb:"attr:piiDetectionEnabled" json:"pii_detection_enabled"`
	AIContentDetection   bool `theorydb:"attr:aiContentDetection" json:"ai_content_detection_enabled"`
}

// AIInstanceConfigOverride stores operator overrides for AI configuration values.
// Nil fields mean "no override".
type AIInstanceConfigOverride struct {
	AIEnabled            *bool `theorydb:"attr:aiEnabled,omitempty" json:"ai_enabled,omitempty"`
	ModerationEnabled    *bool `theorydb:"attr:moderationEnabled,omitempty" json:"moderation_enabled,omitempty"`
	NSFWDetectionEnabled *bool `theorydb:"attr:nsfwDetectionEnabled,omitempty" json:"nsfw_detection_enabled,omitempty"`
	SpamDetectionEnabled *bool `theorydb:"attr:spamDetectionEnabled,omitempty" json:"spam_detection_enabled,omitempty"`
	PIIDetectionEnabled  *bool `theorydb:"attr:piiDetectionEnabled,omitempty" json:"pii_detection_enabled,omitempty"`
	AIContentDetection   *bool `theorydb:"attr:aiContentDetection,omitempty" json:"ai_content_detection_enabled,omitempty"`
}

// TableName returns the DynamoDB table backing AIInstanceConfig.
func (AIInstanceConfig) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys
func (c *AIInstanceConfig) UpdateKeys() error {
	c.PK = instanceConfigPK
	c.SK = SKAIConfig
	if c.Managed == nil {
		c.Managed = &AIInstanceConfigManaged{}
	}
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
		PK: instanceConfigPK,
		SK: SKAIConfig,
		Managed: &AIInstanceConfigManaged{
			AIEnabled:            true,  // Default to enabled
			ModerationEnabled:    true,  // Default to enabled
			NSFWDetectionEnabled: true,  // Default to enabled
			SpamDetectionEnabled: true,  // Default to enabled
			PIIDetectionEnabled:  false, // Default to disabled (privacy)
			AIContentDetection:   false, // Default to disabled
		},
		Override:  nil,
		UpdatedAt: time.Now(),
	}
}
