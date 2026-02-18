package models

import "time"

// InstanceTipsConfig stores instance-owned on-chain tipping configuration.
//
// This record lives under PK="INSTANCE#CONFIG" and SK="TIPS_CONFIG".
type InstanceTipsConfig struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	Managed  *InstanceTipsConfigManaged  `theorydb:"attr:managed" json:"managed"`
	Override *InstanceTipsConfigOverride `theorydb:"attr:override,omitempty" json:"override,omitempty"`

	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

type InstanceTipsConfigManaged struct {
	Enabled         bool   `theorydb:"attr:enabled" json:"enabled"`
	ChainID         int    `theorydb:"attr:chainID" json:"chain_id"`
	ContractAddress string `theorydb:"attr:contractAddress,omitempty" json:"contract_address,omitempty"`
}

type InstanceTipsConfigOverride struct {
	Enabled         *bool   `theorydb:"attr:enabled,omitempty" json:"enabled,omitempty"`
	ChainID         *int    `theorydb:"attr:chainID,omitempty" json:"chain_id,omitempty"`
	ContractAddress *string `theorydb:"attr:contractAddress,omitempty" json:"contract_address,omitempty"`
}

// TableName returns the DynamoDB table backing InstanceTipsConfig.
func (InstanceTipsConfig) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamoDB keys.
func (c *InstanceTipsConfig) UpdateKeys() error {
	c.PK = instanceConfigPK
	c.SK = SKTipsConfig
	if c.Managed == nil {
		c.Managed = &InstanceTipsConfigManaged{}
	}
	return nil
}

// GetPK returns the partition key.
func (c *InstanceTipsConfig) GetPK() string {
	return c.PK
}

// GetSK returns the sort key.
func (c *InstanceTipsConfig) GetSK() string {
	return c.SK
}

// NewInstanceTipsConfig creates a new tips config with built-in defaults.
func NewInstanceTipsConfig() *InstanceTipsConfig {
	return &InstanceTipsConfig{
		PK:        instanceConfigPK,
		SK:        SKTipsConfig,
		Managed:   &InstanceTipsConfigManaged{Enabled: false, ChainID: 0, ContractAddress: ""},
		Override:  nil,
		UpdatedAt: time.Now(),
	}
}

