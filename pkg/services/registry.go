// Package services provides a centralized service registry for the Lesser ActivityPub server.
//
// The Registry replaces the factory pattern with a more flexible, centralized approach
// for managing service dependencies and lifecycle. It follows the functional options
// pattern for configuration and provides thread-safe access to all services.
//
// # Usage Example
//
//	// Create a registry with all dependencies
//	registry, err := services.NewRegistry(
//		services.WithStorage(storageImpl),
//		services.WithPublisher(publisherImpl),
//		services.WithLogger(logger),
//		services.WithConfig(&services.ServiceConfig{
//			BaseURL:   "https://example.com",
//			JWTSecret: "your-jwt-secret",
//		}),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer registry.Close()
//
//	// Access services as needed (lazy initialization)
//	businessLogic := registry.BusinessLogic()
//	validation := registry.Validation()
//
//	// Use services...
//	result, err := businessLogic.CreatePost(ctx, user, input)
//
// # Key Features
//
//   - Lazy initialization: Services are created only when first accessed
//   - Thread-safe: All service access methods are safe for concurrent use
//   - Functional options: Flexible configuration with sensible defaults
//   - Dependency injection: Clean separation of concerns
//   - Health monitoring: Built-in health checks and metrics
//   - Graceful shutdown: Proper resource cleanup via Close()
//
// # Required Dependencies
//
//   - Storage: MUST be provided via WithStorage() - no default
//
// # Optional Dependencies
//
//   - Publisher: For real-time streaming events (can be nil)
//   - Logger: Defaults to no-op logger if not provided
//   - Config: Defaults to localhost configuration if not provided
//
// # Thread Safety
//
// The Registry uses a read-write mutex to ensure thread-safe access to services.
// Service initialization is performed using the double-checked locking pattern
// to prevent deadlocks while maintaining thread safety.
package services

import (
	"fmt"
	"sync"

	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Registry provides centralized access to all application services
// It follows the functional options pattern for flexible configuration
// and is thread-safe for concurrent access.
type Registry struct {
	// Dependencies
	storage   core.RepositoryStorage
	publisher streaming.Publisher
	logger    *zap.Logger
	config    *ServiceConfig

	// Service instances (lazily initialized)
	businessLogic    BusinessLogicService
	validation       ValidationService
	authentication   AuthenticationService
	federation       FederationService
	timeline         TimelineService
	analytics        AnalyticsService
	notification     NotificationService

	// Domain services (new service-first architecture)
	notesService          *notes.Service
	accountsService       *accounts.Service
	relationshipsService  *relationships.Service
	conversationsService  *conversations.Service
	mediaService          *media.Service
	listsService          *lists.Service
	notificationsService  *notifications.Service

	// Service management
	mu          sync.RWMutex
	initialized map[string]bool
}

// RegistryOption defines functional options for Registry configuration
type RegistryOption func(*Registry) error

// NewRegistry creates a new service registry with the provided options
// At minimum, WithStorage must be provided. Other dependencies are optional
// but recommended for full functionality.
func NewRegistry(opts ...RegistryOption) (*Registry, error) {
	r := &Registry{
		initialized: make(map[string]bool),
	}

	// Apply all options
	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, fmt.Errorf("failed to apply registry option: %w", err)
		}
	}

	// Validate required dependencies
	if err := r.validate(); err != nil {
		return nil, fmt.Errorf("registry validation failed: %w", err)
	}

	return r, nil
}

// WithStorage configures the storage dependency (required)
func WithStorage(storage core.RepositoryStorage) RegistryOption {
	return func(r *Registry) error {
		if storage == nil {
			return fmt.Errorf("storage cannot be nil")
		}
		r.storage = storage
		return nil
	}
}

// WithPublisher configures the streaming publisher (optional)
func WithPublisher(publisher streaming.Publisher) RegistryOption {
	return func(r *Registry) error {
		if publisher == nil {
			return fmt.Errorf("publisher cannot be nil")
		}
		r.publisher = publisher
		return nil
	}
}

// WithLogger configures the logger (optional, defaults to no-op logger)
func WithLogger(logger *zap.Logger) RegistryOption {
	return func(r *Registry) error {
		if logger == nil {
			return fmt.Errorf("logger cannot be nil")
		}
		r.logger = logger
		return nil
	}
}

// WithConfig configures the service configuration (optional, uses defaults if not provided)
func WithConfig(config *ServiceConfig) RegistryOption {
	return func(r *Registry) error {
		if config == nil {
			return fmt.Errorf("config cannot be nil")
		}
		r.config = config
		return nil
	}
}

// validate ensures all required dependencies are provided and sets defaults
func (r *Registry) validate() error {
	if r.storage == nil {
		return fmt.Errorf("storage is required - use WithStorage()")
	}

	// Set defaults for optional dependencies
	if r.logger == nil {
		r.logger = zap.NewNop()
	}

	if r.config == nil {
		r.config = &ServiceConfig{
			BaseURL:   "https://localhost",
			JWTSecret: "default-secret-change-in-production",
		}
	}

	return nil
}

// BusinessLogic returns the business logic service, initializing it if necessary
func (r *Registry) BusinessLogic() BusinessLogicService {
	r.mu.Lock()
	if r.businessLogic != nil {
		r.mu.Unlock()
		return r.businessLogic
	}
	
	// Initialize dependencies without holding the lock
	r.mu.Unlock()
	
	deps := &ServiceDependencies{
		Repos:  r.storage,
		Config: r.config,
		Logger: r.logger,
	}

	// Get dependency services (these may acquire their own locks)
	validation := r.Validation()
	authentication := r.Authentication()
	federation := r.Federation()
	timeline := r.Timeline()
	analytics := r.Analytics()

	// Now acquire lock to set the service
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Double-check pattern in case another goroutine initialized it
	if r.businessLogic == nil {
		r.businessLogic = NewBusinessLogicService(
			deps,
			validation,
			authentication,
			federation,
			timeline,
			analytics,
		)
		r.initialized["BusinessLogic"] = true
	}

	return r.businessLogic
}

// Validation returns the validation service, initializing it if necessary
func (r *Registry) Validation() ValidationService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.validation == nil {
		r.validation = NewValidationService(r.config)
		r.initialized["Validation"] = true
	}

	return r.validation
}

// Authentication returns the authentication service, initializing it if necessary
func (r *Registry) Authentication() AuthenticationService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.authentication == nil {
		r.authentication = NewAuthenticationService(r.config.JWTSecret, r.storage)
		r.initialized["Authentication"] = true
	}

	return r.authentication
}

// Federation returns the federation service, initializing it if necessary
func (r *Registry) Federation() FederationService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.federation == nil {
		deps := &ServiceDependencies{
			Repos:  r.storage,
			Config: r.config,
			Logger: r.logger,
		}
		r.federation = NewFederationService(deps)
		r.initialized["Federation"] = true
	}

	return r.federation
}

// Timeline returns the timeline service, initializing it if necessary
func (r *Registry) Timeline() TimelineService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.timeline == nil {
		deps := &ServiceDependencies{
			Repos:  r.storage,
			Config: r.config,
			Logger: r.logger,
		}
		r.timeline = NewTimelineService(deps)
		r.initialized["Timeline"] = true
	}

	return r.timeline
}

// Analytics returns the analytics service, initializing it if necessary
func (r *Registry) Analytics() AnalyticsService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.analytics == nil {
		deps := &ServiceDependencies{
			Repos:  r.storage,
			Config: r.config,
			Logger: r.logger,
		}
		r.analytics = NewAnalyticsService(deps)
		r.initialized["Analytics"] = true
	}

	return r.analytics
}

// Notification returns the notification service, initializing it if necessary
func (r *Registry) Notification() NotificationService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.notification == nil {
		deps := &ServiceDependencies{
			Repos:  r.storage,
			Config: r.config,
			Logger: r.logger,
		}
		r.notification = NewNotificationService(deps)
		r.initialized["Notification"] = true
	}

	return r.notification
}

// GetStorage returns the configured storage interface
func (r *Registry) GetStorage() core.RepositoryStorage {
	return r.storage
}

// GetPublisher returns the configured publisher interface (may be nil)
func (r *Registry) GetPublisher() streaming.Publisher {
	return r.publisher
}

// GetLogger returns the configured logger
func (r *Registry) GetLogger() *zap.Logger {
	return r.logger
}

// GetConfig returns the service configuration
func (r *Registry) GetConfig() *ServiceConfig {
	return r.config
}

// GetInitializedServices returns a list of service names that have been initialized
func (r *Registry) GetInitializedServices() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	services := make([]string, 0, len(r.initialized))
	for service := range r.initialized {
		services = append(services, service)
	}
	return services
}

// Close gracefully shuts down the registry and its resources
func (r *Registry) Close() error {
	r.mu.Lock()
	var lastError error

	// Close publisher if it exists
	if r.publisher != nil {
		if err := r.publisher.Close(); err != nil {
			r.logger.Error("failed to close publisher", zap.Error(err))
			lastError = err
		}
	}

	// Get initialized services without holding lock (to avoid deadlock)
	r.mu.Unlock()
	initializedServices := r.GetInitializedServices()
	
	// Log closure
	r.logger.Info("service registry closed", zap.Strings("initialized_services", initializedServices))

	return lastError
}

// Health returns the health status of the registry and its dependencies
func (r *Registry) Health() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	health := map[string]interface{}{
		"status":               "healthy",
		"initialized_services": r.GetInitializedServices(),
		"dependencies": map[string]interface{}{
			"storage":   r.storage != nil,
			"publisher": r.publisher != nil,
			"logger":    r.logger != nil,
			"config":    r.config != nil,
		},
	}

	return health
}

// Notes returns the notes service, initializing it if necessary
func (r *Registry) Notes() *notes.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.notesService == nil {
		// TODO: Initialize with real repositories
		// For now, return nil - will be implemented when we integrate repositories
		r.initialized["Notes"] = true
	}

	return r.notesService
}

// Accounts returns the accounts service, initializing it if necessary
func (r *Registry) Accounts() *accounts.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.accountsService == nil {
		// TODO: Initialize with real repositories
		r.initialized["Accounts"] = true
	}

	return r.accountsService
}

// Relationships returns the relationships service, initializing it if necessary
func (r *Registry) Relationships() *relationships.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.relationshipsService == nil {
		// TODO: Initialize with real repositories
		r.initialized["Relationships"] = true
	}

	return r.relationshipsService
}

// Conversations returns the conversations service, initializing it if necessary
func (r *Registry) Conversations() *conversations.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conversationsService == nil {
		// TODO: Initialize with real repositories
		r.initialized["Conversations"] = true
	}

	return r.conversationsService
}

// Media returns the media service, initializing it if necessary
func (r *Registry) Media() *media.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.mediaService == nil {
		// TODO: Initialize with real repositories
		r.initialized["Media"] = true
	}

	return r.mediaService
}

// Lists returns the lists service, initializing it if necessary
func (r *Registry) Lists() *lists.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.listsService == nil {
		// TODO: Initialize with real repositories
		r.initialized["Lists"] = true
	}

	return r.listsService
}

// Notifications returns the notifications service, initializing it if necessary
func (r *Registry) Notifications() *notifications.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.notificationsService == nil {
		// TODO: Initialize with real repositories
		r.initialized["Notifications"] = true
	}

	return r.notificationsService
}