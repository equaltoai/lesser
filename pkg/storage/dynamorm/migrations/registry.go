package migrations

import (
	"fmt"
	"sort"
	"sync"
)

// Registry manages available migrations
type Registry struct {
	mu         sync.RWMutex
	migrations map[string]Migration
}

// NewRegistry creates a new migration registry
func NewRegistry() *Registry {
	return &Registry{
		migrations: make(map[string]Migration),
	}
}

// Register adds a migration to the registry
func (r *Registry) Register(migration Migration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	id := migration.ID()
	if _, exists := r.migrations[id]; exists {
		return fmt.Errorf("migration %s already registered", id)
	}
	
	// Validate dependencies exist
	for _, dep := range migration.Dependencies() {
		if _, exists := r.migrations[dep]; !exists {
			// Don't fail registration, as dependencies might be registered later
			// This will be validated when migrations are executed
		}
	}
	
	r.migrations[id] = migration
	return nil
}

// MustRegister registers a migration and panics on error
func (r *Registry) MustRegister(migration Migration) {
	if err := r.Register(migration); err != nil {
		panic(err)
	}
}

// Get returns a migration by ID
func (r *Registry) Get(id string) (Migration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	migration, exists := r.migrations[id]
	return migration, exists
}

// All returns all registered migrations sorted by version
func (r *Registry) All() []Migration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	migrations := make([]Migration, 0, len(r.migrations))
	for _, m := range r.migrations {
		migrations = append(migrations, m)
	}
	
	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version() < migrations[j].Version()
	})
	
	return migrations
}

// GetPending returns migrations that haven't been applied yet
func (r *Registry) GetPending(appliedMigrations map[string]bool) []Migration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	var pending []Migration
	for id, migration := range r.migrations {
		if !appliedMigrations[id] {
			pending = append(pending, migration)
		}
	}
	
	// Sort by version
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Version() < pending[j].Version()
	})
	
	return pending
}

// GetInOrder returns migrations in dependency order
func (r *Registry) GetInOrder(migrations []Migration) ([]Migration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	// Create a map for quick lookup
	migrationMap := make(map[string]Migration)
	for _, m := range migrations {
		migrationMap[m.ID()] = m
	}
	
	// Topological sort to respect dependencies
	var sorted []Migration
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	
	var visit func(Migration) error
	visit = func(m Migration) error {
		id := m.ID()
		
		if visiting[id] {
			return fmt.Errorf("circular dependency detected involving migration %s", id)
		}
		
		if visited[id] {
			return nil
		}
		
		visiting[id] = true
		
		// Visit dependencies first
		for _, depID := range m.Dependencies() {
			dep, exists := r.migrations[depID]
			if !exists {
				return fmt.Errorf("migration %s depends on non-existent migration %s", id, depID)
			}
			
			// Only visit if it's in our list to apply
			if _, inList := migrationMap[depID]; inList {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		
		visiting[id] = false
		visited[id] = true
		sorted = append(sorted, m)
		
		return nil
	}
	
	// Visit all migrations
	for _, m := range migrations {
		if err := visit(m); err != nil {
			return nil, err
		}
	}
	
	return sorted, nil
}

// ValidateDependencies checks if all dependencies are satisfied
func (r *Registry) ValidateDependencies(migration Migration, appliedMigrations map[string]bool) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	for _, depID := range migration.Dependencies() {
		// Check if dependency exists
		if _, exists := r.migrations[depID]; !exists {
			return fmt.Errorf("migration %s depends on non-existent migration %s", migration.ID(), depID)
		}
		
		// Check if dependency has been applied
		if !appliedMigrations[depID] {
			return fmt.Errorf("migration %s depends on unapplied migration %s", migration.ID(), depID)
		}
	}
	
	return nil
}

// Global registry instance
var defaultRegistry = NewRegistry()

// Register adds a migration to the global registry
func Register(migration Migration) error {
	return defaultRegistry.Register(migration)
}

// MustRegister registers a migration to the global registry and panics on error
func MustRegister(migration Migration) {
	defaultRegistry.MustRegister(migration)
}

// GetRegistry returns the global registry
func GetRegistry() *Registry {
	return defaultRegistry
}