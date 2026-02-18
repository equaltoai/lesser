package models

import "time"

// InstanceTrustConfig stores instance-owned trust configuration.
//
// This record lives under PK="INSTANCE#CONFIG" and SK="TRUST_CONFIG".
type InstanceTrustConfig struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	Managed  *InstanceTrustConfigManaged  `theorydb:"attr:managed" json:"managed"`
	Override *InstanceTrustConfigOverride `theorydb:"attr:override,omitempty" json:"override,omitempty"`

	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// InstanceTrustConfigManaged stores managed/default trust configuration values.
type InstanceTrustConfigManaged struct {
	BaseURL              string `theorydb:"attr:baseURL,omitempty" json:"base_url,omitempty"`
	AttestationsURL      string `theorydb:"attr:attestationsURL,omitempty" json:"attestations_url,omitempty"`
	InstanceKeySecretARN string `theorydb:"attr:instanceKeySecretARN,omitempty" json:"instance_key_secret_arn,omitempty"`
}

// InstanceTrustConfigOverride stores operator overrides for trust configuration values.
// Nil fields mean "no override".
type InstanceTrustConfigOverride struct {
	BaseURL              *string `theorydb:"attr:baseURL,omitempty" json:"base_url,omitempty"`
	AttestationsURL      *string `theorydb:"attr:attestationsURL,omitempty" json:"attestations_url,omitempty"`
	InstanceKeySecretARN *string `theorydb:"attr:instanceKeySecretARN,omitempty" json:"instance_key_secret_arn,omitempty"`
}

// TableName returns the DynamoDB table backing InstanceTrustConfig.
func (InstanceTrustConfig) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys.
func (c *InstanceTrustConfig) UpdateKeys() error {
	c.PK = instanceConfigPK
	c.SK = SKTrustConfig
	if c.Managed == nil {
		c.Managed = &InstanceTrustConfigManaged{}
	}
	return nil
}

// GetPK returns the partition key.
func (c *InstanceTrustConfig) GetPK() string {
	return c.PK
}

// GetSK returns the sort key.
func (c *InstanceTrustConfig) GetSK() string {
	return c.SK
}

// NewInstanceTrustConfig creates a new trust config with built-in defaults.
func NewInstanceTrustConfig() *InstanceTrustConfig {
	return &InstanceTrustConfig{
		PK:        instanceConfigPK,
		SK:        SKTrustConfig,
		Managed:   &InstanceTrustConfigManaged{},
		Override:  nil,
		UpdatedAt: time.Now(),
	}
}
