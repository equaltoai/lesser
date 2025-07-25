package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()
	
	migration1 := &MockMigration{
		id:      "migration1",
		version: 1,
	}
	
	migration2 := &MockMigration{
		id:      "migration2",
		version: 2,
	}
	
	// Test successful registration
	err := registry.Register(migration1)
	assert.NoError(t, err)
	
	err = registry.Register(migration2)
	assert.NoError(t, err)
	
	// Test duplicate registration
	err = registry.Register(migration1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	
	migration := &MockMigration{
		id:      "test_migration",
		version: 1,
	}
	
	registry.Register(migration)
	
	// Test existing migration
	retrieved, exists := registry.Get("test_migration")
	assert.True(t, exists)
	assert.Equal(t, migration, retrieved)
	
	// Test non-existing migration
	_, exists = registry.Get("non_existing")
	assert.False(t, exists)
}

func TestRegistry_All(t *testing.T) {
	registry := NewRegistry()
	
	migrations := []*MockMigration{
		{id: "migration3", version: 3},
		{id: "migration1", version: 1},
		{id: "migration2", version: 2},
	}
	
	for _, m := range migrations {
		registry.Register(m)
	}
	
	all := registry.All()
	assert.Len(t, all, 3)
	
	// Check sorting by version
	assert.Equal(t, int64(1), all[0].Version())
	assert.Equal(t, int64(2), all[1].Version())
	assert.Equal(t, int64(3), all[2].Version())
}

func TestRegistry_GetPending(t *testing.T) {
	registry := NewRegistry()
	
	migrations := []*MockMigration{
		{id: "migration1", version: 1},
		{id: "migration2", version: 2},
		{id: "migration3", version: 3},
	}
	
	for _, m := range migrations {
		registry.Register(m)
	}
	
	applied := map[string]bool{
		"migration1": true,
		"migration3": true,
	}
	
	pending := registry.GetPending(applied)
	assert.Len(t, pending, 1)
	assert.Equal(t, "migration2", pending[0].ID())
}

func TestRegistry_GetInOrder(t *testing.T) {
	registry := NewRegistry()
	
	// Create migrations with dependencies
	migration1 := &MockMigration{
		id:           "migration1",
		version:      1,
		dependencies: []string{},
	}
	
	migration2 := &MockMigration{
		id:           "migration2",
		version:      2,
		dependencies: []string{"migration1"},
	}
	
	migration3 := &MockMigration{
		id:           "migration3",
		version:      3,
		dependencies: []string{"migration2"},
	}
	
	// Register in wrong order
	registry.Register(migration3)
	registry.Register(migration1)
	registry.Register(migration2)
	
	// Get in dependency order
	ordered, err := registry.GetInOrder([]Migration{migration3, migration2, migration1})
	assert.NoError(t, err)
	assert.Len(t, ordered, 3)
	
	// Check correct order
	assert.Equal(t, "migration1", ordered[0].ID())
	assert.Equal(t, "migration2", ordered[1].ID())
	assert.Equal(t, "migration3", ordered[2].ID())
}

func TestRegistry_GetInOrder_CircularDependency(t *testing.T) {
	registry := NewRegistry()
	
	// Create circular dependency
	migration1 := &MockMigration{
		id:           "migration1",
		version:      1,
		dependencies: []string{"migration2"},
	}
	
	migration2 := &MockMigration{
		id:           "migration2",
		version:      2,
		dependencies: []string{"migration1"},
	}
	
	registry.Register(migration1)
	registry.Register(migration2)
	
	_, err := registry.GetInOrder([]Migration{migration1, migration2})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestRegistry_ValidateDependencies(t *testing.T) {
	registry := NewRegistry()
	
	migration1 := &MockMigration{
		id:      "migration1",
		version: 1,
	}
	
	migration2 := &MockMigration{
		id:           "migration2",
		version:      2,
		dependencies: []string{"migration1"},
	}
	
	migration3 := &MockMigration{
		id:           "migration3",
		version:      3,
		dependencies: []string{"non_existing"},
	}
	
	registry.Register(migration1)
	registry.Register(migration2)
	registry.Register(migration3)
	
	tests := []struct {
		name      string
		migration Migration
		applied   map[string]bool
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "all dependencies satisfied",
			migration: migration2,
			applied:   map[string]bool{"migration1": true},
			wantErr:   false,
		},
		{
			name:      "dependency not applied",
			migration: migration2,
			applied:   map[string]bool{},
			wantErr:   true,
			errMsg:    "unapplied migration",
		},
		{
			name:      "non-existent dependency",
			migration: migration3,
			applied:   map[string]bool{},
			wantErr:   true,
			errMsg:    "non-existent migration",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.ValidateDependencies(tt.migration, tt.applied)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGlobalRegistry(t *testing.T) {
	// Reset global registry for test
	defaultRegistry = NewRegistry()
	
	migration := &MockMigration{
		id:      "global_test",
		version: 1,
	}
	
	// Test global functions
	err := Register(migration)
	assert.NoError(t, err)
	
	registry := GetRegistry()
	assert.NotNil(t, registry)
	
	retrieved, exists := registry.Get("global_test")
	assert.True(t, exists)
	assert.Equal(t, migration, retrieved)
}

func TestMustRegister_Panic(t *testing.T) {
	registry := NewRegistry()
	
	migration := &MockMigration{
		id:      "panic_test",
		version: 1,
	}
	
	// First registration should succeed
	registry.MustRegister(migration)
	
	// Second registration should panic
	assert.Panics(t, func() {
		registry.MustRegister(migration)
	})
}