package migrations

import (
	"context"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
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
	PK          string    `dynamorm:"pk"`
	SK          string    `dynamorm:"sk"`
	ID          string    `dynamorm:"id"`
	Version     int64     `dynamorm:"version"`
	Description string    `dynamorm:"description"`
	AppliedAt   time.Time `dynamorm:"applied_at"`
	AppliedBy   string    `dynamorm:"applied_by"`
	Checksum    string    `dynamorm:"checksum"`
	Status      string    `dynamorm:"status"` // "applied", "failed", "rolled_back"
	Error       string    `dynamorm:"error,omitempty"`
}

// GetTableKeys returns the primary key values for DynamoDB
func (m *MigrationHistory) GetTableKeys() (string, string) {
	return "MIGRATION#HISTORY", m.ID
}

// MigrationStatus represents the current state of migrations
type MigrationStatus struct {
	PK              string    `dynamorm:"pk"`
	SK              string    `dynamorm:"sk"`
	LastMigrationID string    `dynamorm:"last_migration_id"`
	LastVersion     int64     `dynamorm:"last_version"`
	UpdatedAt       time.Time `dynamorm:"updated_at"`
	IsLocked        bool      `dynamorm:"is_locked"`
	LockedBy        string    `dynamorm:"locked_by,omitempty"`
	LockedAt        time.Time `dynamorm:"locked_at,omitempty"`
}

// GetTableKeys returns the primary key values for DynamoDB
func (m *MigrationStatus) GetTableKeys() (string, string) {
	return MigrationStatusKey, StatusCurrent
}
