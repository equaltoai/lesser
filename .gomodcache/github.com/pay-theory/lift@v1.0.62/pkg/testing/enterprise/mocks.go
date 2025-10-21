package enterprise

import (
	"fmt"
	"sync"
)

// ServiceMockRegistry manages mock services across environments
type ServiceMockRegistry struct {
	mocks map[string][]ServiceMock
	mutex sync.RWMutex
}

// ServiceMock represents a mock service
type ServiceMock struct {
	Name        string
	Type        string
	Endpoint    string
	Responses   map[string]any
	Environment string
	Active      bool
}

// NewServiceMockRegistry creates a new service mock registry
func NewServiceMockRegistry() *ServiceMockRegistry {
	return &ServiceMockRegistry{
		mocks: make(map[string][]ServiceMock),
	}
}

// AddMock adds a mock service for a specific environment
func (s *ServiceMockRegistry) AddMock(envName string, mock ServiceMock) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	mock.Environment = envName
	s.mocks[envName] = append(s.mocks[envName], mock)
}

// SetupForEnvironment sets up mock services for an environment
func (s *ServiceMockRegistry) SetupForEnvironment(env *TestEnvironment) error {
	s.mutex.RLock()
	mocks, exists := s.mocks[env.Name]
	s.mutex.RUnlock()

	if !exists {
		return nil // No mocks for this environment
	}

	// Setup mock services
	for _, mock := range mocks {
		if err := s.setupMock(mock, env); err != nil {
			return fmt.Errorf("failed to setup mock %s: %w", mock.Name, err)
		}
	}

	return nil
}

// CleanupForEnvironment cleans up mock services for an environment
func (s *ServiceMockRegistry) CleanupForEnvironment(env *TestEnvironment) error {
	s.mutex.RLock()
	mocks, exists := s.mocks[env.Name]
	s.mutex.RUnlock()

	if !exists {
		return nil // No mocks for this environment
	}

	// Cleanup mock services
	for _, mock := range mocks {
		if err := s.cleanupMock(mock, env); err != nil {
			return fmt.Errorf("failed to cleanup mock %s: %w", mock.Name, err)
		}
	}

	return nil
}

// setupMock sets up a single mock service
func (s *ServiceMockRegistry) setupMock(mock ServiceMock, env *TestEnvironment) error {
	// Store mock configuration in environment resources
	env.mutex.Lock()
	env.Resources[fmt.Sprintf("mock_%s", mock.Name)] = mock

	// Update service endpoints to point to mock
	if env.Config.ServiceEndpoints == nil {
		env.Config.ServiceEndpoints = make(map[string]string)
	}
	env.Config.ServiceEndpoints[mock.Name] = mock.Endpoint
	env.mutex.Unlock()

	return nil
}

// cleanupMock cleans up a single mock service
func (s *ServiceMockRegistry) cleanupMock(mock ServiceMock, env *TestEnvironment) error {
	// Remove mock configuration from environment resources
	env.mutex.Lock()
	delete(env.Resources, fmt.Sprintf("mock_%s", mock.Name))
	env.mutex.Unlock()

	return nil
}
