package models

import "time"

const (
	// DirectMessageMigrationStatePK is the singleton partition key for DM migration state.
	DirectMessageMigrationStatePK = "DM_MIGRATION#DIRECT_MESSAGE_STATE"
	// DirectMessageMigrationStateSK is the singleton sort key for DM migration state.
	DirectMessageMigrationStateSK = "STATE"
)

// DirectMessageMigrationState stores the live DM migration lock used to freeze DM writes
// while canonical state backfills and legacy-row cleanup are running.
type DirectMessageMigrationState struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	WritesFrozen bool   `theorydb:"attr:writesFrozen" json:"writes_frozen"`
	Phase        string `theorydb:"attr:phase,omitempty" json:"phase,omitempty"`
	Reason       string `theorydb:"attr:reason,omitempty" json:"reason,omitempty"`
	Owner        string `theorydb:"attr:owner,omitempty" json:"owner,omitempty"`

	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table name.
func (DirectMessageMigrationState) TableName() string {
	return MainTableName
}

// UpdateKeys sets the singleton key material.
func (s *DirectMessageMigrationState) UpdateKeys() error {
	s.PK = DirectMessageMigrationStatePK
	s.SK = DirectMessageMigrationStateSK
	return nil
}

// GetPK returns the partition key.
func (s *DirectMessageMigrationState) GetPK() string {
	return s.PK
}

// GetSK returns the sort key.
func (s *DirectMessageMigrationState) GetSK() string {
	return s.SK
}

// BeforeCreate initializes keys and timestamps before create.
func (s *DirectMessageMigrationState) BeforeCreate() error {
	if err := s.UpdateKeys(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate refreshes keys and updatedAt before update.
func (s *DirectMessageMigrationState) BeforeUpdate() error {
	if err := s.UpdateKeys(); err != nil {
		return err
	}
	s.UpdatedAt = time.Now().UTC()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = s.UpdatedAt
	}
	return nil
}
