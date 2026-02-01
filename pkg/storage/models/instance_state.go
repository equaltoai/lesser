package models

import (
	"strings"
	"time"
)

const (
	// DefaultBootstrapUsername is the reserved username used during instance setup.
	DefaultBootstrapUsername = "bootstrap"
)

// InstanceState represents instance activation/bootstrapping state stored in DynamoDB.
// This record is stage-scoped because each stage has its own DynamoDB table.
type InstanceState struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"-"` // INSTANCE#CONFIG
	SK string `theorydb:"sk,attr:SK" json:"-"` // STATE

	// Activation state
	Locked bool `theorydb:"attr:locked" json:"locked"`

	// Bootstrap identity (public-only)
	BootstrapUsername      string `theorydb:"attr:bootstrapUsername" json:"bootstrap_username"`
	BootstrapWalletAddress string `theorydb:"attr:bootstrapWalletAddress" json:"bootstrap_wallet_address,omitempty"`

	// Setup progression
	PrimaryAdminUsername string     `theorydb:"attr:primaryAdminUsername" json:"primary_admin_username,omitempty"`
	ActivatedAt          *time.Time `theorydb:"attr:activatedAt" json:"activated_at,omitempty"`

	// Audit timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing InstanceState.
func (InstanceState) TableName() string {
	return MainTableName
}

// UpdateKeys sets the fixed DynamoDB keys for the singleton state record.
func (s *InstanceState) UpdateKeys() error {
	s.PK = instanceConfigPK
	s.SK = "STATE"
	return nil
}

// GetPK returns the partition key.
func (s *InstanceState) GetPK() string {
	return s.PK
}

// GetSK returns the sort key.
func (s *InstanceState) GetSK() string {
	return s.SK
}

// BeforeCreate sets defaults before creating the record.
func (s *InstanceState) BeforeCreate() error {
	if err := s.UpdateKeys(); err != nil {
		return err
	}
	s.ensureDefaults()
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate updates timestamps and defaults before updating the record.
func (s *InstanceState) BeforeUpdate() error {
	s.ensureDefaults()
	s.UpdatedAt = time.Now()
	return s.UpdateKeys()
}

func (s *InstanceState) ensureDefaults() {
	if strings.TrimSpace(s.BootstrapUsername) == "" {
		s.BootstrapUsername = DefaultBootstrapUsername
	}
}

// NewDefaultInstanceState returns the default locked instance state.
func NewDefaultInstanceState() *InstanceState {
	now := time.Now()
	return &InstanceState{
		Locked:            true,
		BootstrapUsername: DefaultBootstrapUsername,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}
