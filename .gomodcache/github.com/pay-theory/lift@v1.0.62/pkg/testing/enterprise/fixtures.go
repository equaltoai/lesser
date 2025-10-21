package enterprise

import (
	"fmt"
	"sync"
)

// DataFixtureManager manages test data fixtures across environments
type DataFixtureManager struct {
	fixtures map[string][]DataFixture
	mutex    sync.RWMutex
}

// DataFixture represents a test data fixture
type DataFixture struct {
	Name         string
	Type         string
	Data         any
	Environment  string
	Dependencies []string
}

// NewDataFixtureManager creates a new data fixture manager
func NewDataFixtureManager() *DataFixtureManager {
	return &DataFixtureManager{
		fixtures: make(map[string][]DataFixture),
	}
}

// AddFixture adds a fixture for a specific environment
func (d *DataFixtureManager) AddFixture(envName string, fixture DataFixture) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	fixture.Environment = envName
	d.fixtures[envName] = append(d.fixtures[envName], fixture)
}

// SetupForEnvironment sets up fixtures for an environment
func (d *DataFixtureManager) SetupForEnvironment(env *TestEnvironment) error {
	d.mutex.RLock()
	fixtures, exists := d.fixtures[env.Name]
	d.mutex.RUnlock()

	if !exists {
		return nil // No fixtures for this environment
	}

	// Setup fixtures in dependency order
	for _, fixture := range fixtures {
		if err := d.setupFixture(fixture, env); err != nil {
			return fmt.Errorf("failed to setup fixture %s: %w", fixture.Name, err)
		}
	}

	return nil
}

// CleanupForEnvironment cleans up fixtures for an environment
func (d *DataFixtureManager) CleanupForEnvironment(env *TestEnvironment) error {
	d.mutex.RLock()
	fixtures, exists := d.fixtures[env.Name]
	d.mutex.RUnlock()

	if !exists {
		return nil // No fixtures for this environment
	}

	// Cleanup fixtures in reverse order
	for i := len(fixtures) - 1; i >= 0; i-- {
		if err := d.cleanupFixture(fixtures[i], env); err != nil {
			return fmt.Errorf("failed to cleanup fixture %s: %w", fixtures[i].Name, err)
		}
	}

	return nil
}

// setupFixture sets up a single fixture
func (d *DataFixtureManager) setupFixture(fixture DataFixture, env *TestEnvironment) error {
	// Store fixture data in environment resources
	env.mutex.Lock()
	env.Resources[fmt.Sprintf("fixture_%s", fixture.Name)] = fixture.Data
	env.mutex.Unlock()

	return nil
}

// cleanupFixture cleans up a single fixture
func (d *DataFixtureManager) cleanupFixture(fixture DataFixture, env *TestEnvironment) error {
	// Remove fixture data from environment resources
	env.mutex.Lock()
	delete(env.Resources, fmt.Sprintf("fixture_%s", fixture.Name))
	env.mutex.Unlock()

	return nil
}
