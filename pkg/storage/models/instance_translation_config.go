package models

import "time"

// InstanceTranslationConfig stores instance-owned translation configuration.
//
// This record lives under PK="INSTANCE#CONFIG" and SK="TRANSLATION_CONFIG".
type InstanceTranslationConfig struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	Managed  *InstanceTranslationConfigManaged  `theorydb:"attr:managed" json:"managed"`
	Override *InstanceTranslationConfigOverride `theorydb:"attr:override,omitempty" json:"override,omitempty"`

	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

type InstanceTranslationConfigManaged struct {
	Enabled bool `theorydb:"attr:enabled" json:"enabled"`
}

type InstanceTranslationConfigOverride struct {
	Enabled *bool `theorydb:"attr:enabled,omitempty" json:"enabled,omitempty"`
}

// TableName returns the DynamoDB table backing InstanceTranslationConfig.
func (InstanceTranslationConfig) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys.
func (c *InstanceTranslationConfig) UpdateKeys() error {
	c.PK = instanceConfigPK
	c.SK = SKTranslationConfig
	if c.Managed == nil {
		c.Managed = &InstanceTranslationConfigManaged{}
	}
	return nil
}

// GetPK returns the partition key.
func (c *InstanceTranslationConfig) GetPK() string {
	return c.PK
}

// GetSK returns the sort key.
func (c *InstanceTranslationConfig) GetSK() string {
	return c.SK
}

// NewInstanceTranslationConfig creates a new translation config with built-in defaults.
func NewInstanceTranslationConfig() *InstanceTranslationConfig {
	return &InstanceTranslationConfig{
		PK:        instanceConfigPK,
		SK:        SKTranslationConfig,
		Managed:   &InstanceTranslationConfigManaged{Enabled: false},
		Override:  nil,
		UpdatedAt: time.Now(),
	}
}

