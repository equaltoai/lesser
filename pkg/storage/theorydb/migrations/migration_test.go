package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// MockMigration for testing
type MockMigration struct {
	mock.Mock
	id           string
	version      int64
	description  string
	dependencies []string
}

func (m *MockMigration) ID() string {
	return m.id
}

func (m *MockMigration) Version() int64 {
	return m.version
}

func (m *MockMigration) Description() string {
	return m.description
}

func (m *MockMigration) Dependencies() []string {
	return m.dependencies
}

func (m *MockMigration) Up(ctx context.Context, db core.DB) error {
	args := m.Called(ctx, db)
	return args.Error(0)
}

func (m *MockMigration) Down(ctx context.Context, db core.DB) error {
	args := m.Called(ctx, db)
	return args.Error(0)
}

func TestBaseMigration(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		version      int64
		description  string
		dependencies []string
	}{
		{
			name:         "basic migration",
			id:           "test_migration",
			version:      20240101120000,
			description:  "Test migration",
			dependencies: nil,
		},
		{
			name:         "migration with dependencies",
			id:           "dependent_migration",
			version:      20240102120000,
			description:  "Dependent migration",
			dependencies: []string{"test_migration"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewBaseMigration(tt.id, tt.version, tt.description, tt.dependencies...)

			assert.Equal(t, tt.id, m.ID())
			assert.Equal(t, tt.version, m.Version())
			assert.Equal(t, tt.description, m.Description())
			assert.Equal(t, tt.dependencies, m.Dependencies())
		})
	}
}

func TestMigrationHistory_GetTableKeys(t *testing.T) {
	h := &MigrationHistory{
		ID: "test_migration",
	}

	pk, sk := h.GetTableKeys()
	assert.Equal(t, "MIGRATION#HISTORY", pk)
	assert.Equal(t, "test_migration", sk)
}

func TestMigrationStatus_GetTableKeys(t *testing.T) {
	s := &MigrationStatus{}

	pk, sk := s.GetTableKeys()
	assert.Equal(t, "MIGRATION#STATUS", pk)
	assert.Equal(t, "CURRENT", sk)
}
