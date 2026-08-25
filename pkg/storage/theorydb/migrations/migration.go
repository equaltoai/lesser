package migrations

import (
	"context"
	"time"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// Migration represents a database schema change
type Migration interface {
	// ID returns a unique identifier for this migration (e.g., "20240101_add_user_index")
	ID() string

	// Version returns the version number for ordering (e.g., 20240101120000)
	Version() int64

	// Description returns a human-readable description of what this migration does
	Description() string

	// Up applies the migration
	Up(ctx context.Context, db core.DB) error

	// Down reverses the migration
	Down(ctx context.Context, db core.DB) error

	// Dependencies returns a list of migration IDs that must be applied before this one
	Dependencies() []string
}

// BaseMigration provides common functionality for migrations
type BaseMigration struct {
	id           string
	version      int64
	description  string
	dependencies []string
}

// NewBaseMigration creates a new base migration
func NewBaseMigration(id string, version int64, description string, dependencies ...string) BaseMigration {
	return BaseMigration{
		id:           id,
		version:      version,
		description:  description,
		dependencies: dependencies,
	}
}

// ID returns the migration ID
func (m BaseMigration) ID() string {
	return m.id
}

// Version returns the migration version
func (m BaseMigration) Version() int64 {
	return m.version
}

// Description returns the migration description
func (m BaseMigration) Description() string {
	return m.description
}

// Dependencies returns the migration dependencies
func (m BaseMigration) Dependencies() []string {
	return m.dependencies
}

// MigrationHistory represents a record of an applied migration
type MigrationHistory struct {
	PK          string    `theorydb:"pk"`
	SK          string    `theorydb:"sk"`
	ID          string    `theorydb:"attr:id"`
	Version     int64     `theorydb:"version"`
	Description string    `theorydb:"attr:description"`
	AppliedAt   time.Time `theorydb:"attr:appliedAt"`
	AppliedBy   string    `theorydb:"attr:appliedBy"`
	Checksum    string    `theorydb:"attr:checksum"`
	Status      string    `theorydb:"attr:status"` // "applied", "failed", "rolled_back"
	Error       string    `theorydb:"attr:error,omitempty"`
}

// GetTableKeys returns the primary key values for DynamoDB
func (m *MigrationHistory) GetTableKeys() (string, string) {
	return "MIGRATION#HISTORY", m.ID
}

// MigrationStatus represents the current state of migrations
type MigrationStatus struct {
	PK              string    `theorydb:"pk"`
	SK              string    `theorydb:"sk"`
	LastMigrationID string    `theorydb:"attr:lastMigrationID"`
	LastVersion     int64     `theorydb:"attr:lastVersion"`
	UpdatedAt       time.Time `theorydb:"updated_at,attr:updatedAt"`
	IsLocked        bool      `theorydb:"attr:isLocked"`
	LockedBy        string    `theorydb:"attr:lockedBy,omitempty"`
	LockedAt        time.Time `theorydb:"attr:lockedAt,omitempty"`
}

// GetTableKeys returns the primary key values for DynamoDB
func (m *MigrationStatus) GetTableKeys() (string, string) {
	return MigrationStatusKey, StatusCurrent
}
