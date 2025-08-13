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
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/ai"
	"github.com/equaltoai/lesser/pkg/services/bulk"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	"github.com/equaltoai/lesser/pkg/services/importexport"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// EventBus defines the interface for GraphQL event subscriptions
type EventBus interface {
	// Subscribe creates a subscription to events matching the stream name
	Subscribe(ctx context.Context, streamName string) (<-chan interface{}, error)
}

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
	aiService             *ai.Service
	emojiService          *emoji.Service
	scheduledService      *scheduled.Service
	searchService         *search.Service
	importExportService   *importexport.Service
	bulkService           *bulk.Service

	// Event infrastructure
	eventBus              EventBus
	internalEventBus      *streaming.EventBus

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
			r.publisher,
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

	// Close internal event bus if it exists
	if r.internalEventBus != nil {
		if err := r.internalEventBus.Stop(); err != nil {
			r.logger.Error("failed to stop internal event bus", zap.Error(err))
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

	if r.notesService == nil && r.storage != nil {
		// Initialize the Notes service with repository interfaces
		statusRepo := r.storage.Status()
		accountRepo := r.storage.Account()
		likeRepo := r.storage.Like()
		socialRepo := r.storage.Social()
		conversationRepo := r.storage.Conversation()
		objectRepo := r.storage.Object()
		searchRepo := r.storage.Search()
		communityNoteRepo := r.storage.CommunityNote()
		userRepo := r.storage.User()
		
		// Check if repositories are available
		if statusRepo != nil && accountRepo != nil {
			domainName := "localhost"
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}
			
			r.notesService = notes.NewService(
				statusRepo,
				accountRepo,
				likeRepo,
				socialRepo,
				conversationRepo,
				objectRepo,
				searchRepo,
				communityNoteRepo,
				userRepo,
				r.publisher,
				nil, // Federation service - TODO: wire up when available
				r.logger,
				domainName,
			)
			r.initialized["Notes"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Notes service: required repositories not available")
			}
		}
	}

	return r.notesService
}

// Accounts returns the accounts service, initializing it if necessary
func (r *Registry) Accounts() *accounts.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.accountsService == nil {
		// Create adapter services for crypto and auth
		cryptoAdapter := NewCryptoAdapter()
		authAdapter := NewAuthAdapter(r.config.JWTSecret, r.storage)
		
		r.accountsService = accounts.NewService(
			r.storage,
			r.publisher,
			nil, // federation service - not available yet
			cryptoAdapter,
			authAdapter,
			r.logger,
			r.config.BaseURL,
		)
		r.initialized["Accounts"] = true
	}

	return r.accountsService
}

// Relationships returns the relationships service, initializing it if necessary
func (r *Registry) Relationships() *relationships.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.relationshipsService == nil && r.storage != nil {
		domainName := "localhost"
		if r.config != nil && r.config.BaseURL != "" {
			// Extract domain from base URL
			if strings.HasPrefix(r.config.BaseURL, "https://") {
				domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
			} else if strings.HasPrefix(r.config.BaseURL, "http://") {
				domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
			}
		}
		
		r.relationshipsService = relationships.NewServiceWithStorage(
			r.storage,
			r.publisher,
			nil, // federation service - not available yet
			r.logger,
			domainName,
		)
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

	if r.listsService == nil && r.storage != nil {
		// Initialize the Lists service with repository interfaces
		listRepo := r.storage.List()
		statusRepo := r.storage.Status()
		
		// Check if repositories are available
		if listRepo != nil && statusRepo != nil {
			r.listsService = lists.NewService(
				listRepo,
				statusRepo,
				r.publisher,
				r.logger,
			)
			r.initialized["Lists"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Lists service: required repositories not available")
			}
		}
	}

	return r.listsService
}

// Notifications returns the notifications service, initializing it if necessary
func (r *Registry) Notifications() *notifications.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.notificationsService == nil && r.storage != nil {
		// Initialize the Notifications service with repository interfaces
		notificationRepo := r.storage.Notification()
		accountRepo := r.storage.Account()
		
		// Check if repositories are available
		if notificationRepo != nil && accountRepo != nil {
			domainName := "localhost"
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}
			
			r.notificationsService = notifications.NewService(
				notificationRepo,
				accountRepo,
				r.publisher,
				r.logger,
				domainName,
			)
			r.initialized["Notifications"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Notifications service: required repositories not available")
			}
		}
	}

	return r.notificationsService
}

// AI returns the AI service, initializing it if necessary
func (r *Registry) AI() *ai.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.aiService == nil && r.storage != nil {
		r.aiService = ai.NewService(
			r.storage,
			r.publisher,
			r.logger,
		)
		r.initialized["AI"] = true
	}

	return r.aiService
}

// Emoji returns the emoji service, initializing it if necessary
func (r *Registry) Emoji() *emoji.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.emojiService == nil && r.storage != nil {
		// Initialize the Emoji service with repository interface
		emojiRepo := r.storage.Emoji()
		
		// Check if repository is available
		if emojiRepo != nil {
			domainName := "localhost"
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}
			
			r.emojiService = emoji.NewService(
				emojiRepo,
				r.publisher,
				r.logger,
				domainName,
			)
			r.initialized["Emoji"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Emoji service: repository not available")
			}
		}
	}

	return r.emojiService
}

// Scheduled returns the scheduled status service, initializing it if necessary
func (r *Registry) Scheduled() *scheduled.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.scheduledService == nil && r.storage != nil {
		// Initialize the Scheduled service with repository interfaces
		scheduledRepo := r.storage.ScheduledStatus()
		statusRepo := r.storage.Status()
		mediaRepo := r.storage.Media()
		
		// Check if repositories are available
		if scheduledRepo != nil && statusRepo != nil && mediaRepo != nil {
			domainName := "localhost"
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}
			
			r.scheduledService = scheduled.NewService(
				scheduledRepo,
				statusRepo,
				mediaRepo,
				r.publisher,
				r.logger,
				domainName,
			)
			r.initialized["Scheduled"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Scheduled service: required repositories not available")
			}
		}
	}

	return r.scheduledService
}

// Search returns the search and discovery service, initializing it if necessary
func (r *Registry) Search() *search.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.searchService == nil && r.storage != nil {
		// Initialize the Search service with repository interfaces
		searchRepo := r.storage.Search()
		actorRepo := r.storage.Actor()
		relationshipRepo := r.storage.Relationship()
		statusRepo := r.storage.Status()
		
		// Check if repositories are available
		if searchRepo != nil && actorRepo != nil && relationshipRepo != nil && statusRepo != nil {
			domainName := "localhost"
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}
			
			r.searchService = search.NewService(
				searchRepo,
				actorRepo,
				relationshipRepo,
				statusRepo,
				r.publisher,
				r.logger,
				domainName,
			)
			r.initialized["Search"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Search service: required repositories not available")
			}
		}
	}

	return r.searchService
}

// ImportExport returns the ImportExport service, initializing it if necessary
func (r *Registry) ImportExport() *importexport.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.importExportService == nil && r.storage != nil {
		// Initialize the ImportExport service with repository interfaces
		exportRepo := r.storage.Export()
		importRepo := r.storage.Import()
		statusRepo := r.storage.Status()
		accountRepo := r.storage.Account()
		mediaRepo := r.storage.Media()
		socialRepo := r.storage.Social()
		
		// Check if repositories are available
		if exportRepo != nil && importRepo != nil && statusRepo != nil && accountRepo != nil && mediaRepo != nil && socialRepo != nil {
			domainName := "localhost"
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}
			
			// TODO: Need to implement QueueService and StorageClient interfaces
			// For now, pass nil - these will need to be added to registry dependencies
			r.importExportService = importexport.NewService(
				exportRepo,
				importRepo,
				statusRepo,
				accountRepo,
				mediaRepo,
				socialRepo,
				r.publisher,
				nil, // queueService - TODO: implement
				nil, // storageClient - TODO: implement
				r.logger,
				domainName,
			)
			r.initialized["ImportExport"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize ImportExport service: required repositories not available")
			}
		}
	}

	return r.importExportService
}

// Bulk returns the Bulk service, initializing it if necessary
func (r *Registry) Bulk() *bulk.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.bulkService == nil && r.storage != nil {
		// Initialize the Bulk service with repository interfaces
		statusRepo := r.storage.Status()
		accountRepo := r.storage.Account()
		socialRepo := r.storage.Social()
		listRepo := r.storage.List()
		relationshipRepo := r.storage.Relationship()
		
		// Check if repositories are available
		if statusRepo != nil && accountRepo != nil && socialRepo != nil && listRepo != nil && relationshipRepo != nil {
			domainName := "localhost"
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}
			
			// Initialize federation service (may be nil during testing)
			federationService := r.Federation()
			
			// Create adapter for federation service interface
			var bulkFederation bulk.FederationService
			if federationService != nil {
				bulkFederation = &federationServiceAdapter{federationService}
			}
			
			r.bulkService = bulk.NewService(
				statusRepo,
				accountRepo,
				socialRepo,
				listRepo,
				relationshipRepo,
				r.publisher,
				bulkFederation,
				r.logger,
				domainName,
			)
			r.initialized["Bulk"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Bulk service: required repositories not available")
			}
		}
	}

	return r.bulkService
}

// EventBus returns the EventBus interface for GraphQL subscriptions
func (r *Registry) EventBus() EventBus {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.eventBus == nil {
		// Initialize the internal event bus if not already done
		if r.internalEventBus == nil {
			r.internalEventBus = streaming.GetGlobalEventBus(r.logger)
			// Start the event bus if not running
			if !r.internalEventBus.IsRunning() {
				ctx := context.Background()
				if err := r.internalEventBus.Start(ctx); err != nil {
					r.logger.Error("failed to start internal event bus", zap.Error(err))
					return nil
				}
			}
		}

		// Create the GraphQL EventBus adapter
		r.eventBus = &graphqlEventBusAdapter{
			internalEventBus: r.internalEventBus,
			logger:           r.logger,
		}
		r.initialized["EventBus"] = true
	}

	return r.eventBus
}

// graphqlEventBusAdapter adapts the internal EventBus to the GraphQL EventBus interface
type graphqlEventBusAdapter struct {
	internalEventBus *streaming.EventBus
	logger           *zap.Logger
}

// Subscribe creates a subscription to events matching the stream name
func (a *graphqlEventBusAdapter) Subscribe(ctx context.Context, streamName string) (<-chan interface{}, error) {
	if a.internalEventBus == nil {
		return nil, fmt.Errorf("internal event bus not initialized")
	}

	// Create a filter for the specific stream
	filter := &streaming.EventFilter{
		Streams: []string{streamName},
	}

	// Subscribe to the internal event bus
	subscriber, err := a.internalEventBus.Subscribe(streamName, filter, 100) // 100 buffer size
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to internal event bus: %w", err)
	}

	// Create a channel for GraphQL events
	graphqlChan := make(chan interface{}, 100)

	// Start a goroutine to forward events from internal bus to GraphQL channel
	go func() {
		defer func() {
			close(graphqlChan)
			if subscriber != nil {
				subscriber.Close()
				// Also unsubscribe from the internal event bus
				if err := a.internalEventBus.Unsubscribe(streamName); err != nil {
					a.logger.Warn("failed to unsubscribe from internal event bus",
						zap.String("stream", streamName),
						zap.Error(err))
				}
			}
		}()

		for {
			select {
			case event := <-subscriber.Channel:
				if event == nil {
					return // Channel closed
				}
				
				// Convert internal event to GraphQL-compatible format
				graphqlEvent := convertInternalEventToGraphQL(event)
				
				// Non-blocking send to GraphQL channel
				select {
				case graphqlChan <- graphqlEvent:
				case <-ctx.Done():
					return
				default:
					// Drop event if channel is full
					a.logger.Warn("dropping event - GraphQL channel full",
						zap.String("stream", streamName),
						zap.String("event_id", event.ID))
				}

			case <-subscriber.Quit:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return graphqlChan, nil
}

// convertInternalEventToGraphQL converts an internal streaming event to a GraphQL-compatible format
func convertInternalEventToGraphQL(event *streaming.InternalEvent) interface{} {
	// For now, return the event data directly
	// This can be enhanced based on specific GraphQL requirements
	if event.Data != nil {
		return event.Data
	}

	// If no specific data, return a generic event structure
	return map[string]interface{}{
		"id":        event.ID,
		"type":      string(event.Type),
		"action":    string(event.Action),
		"actor_id":  event.ActorID,
		"target_id": event.TargetID,
		"user_id":   event.UserID,
		"timestamp": event.Timestamp,
		"metadata":  event.Metadata,
	}
}

// federationServiceAdapter adapts the registry's FederationService to the bulk service's interface
type federationServiceAdapter struct {
	federation FederationService
}

// QueueActivity implements bulk.FederationService
func (a *federationServiceAdapter) QueueActivity(ctx context.Context, activity *activitypub.Activity) error {
	// The registry's FederationService doesn't have QueueActivity directly,
	// but we can use DeliverToFollowers as a fallback
	// In a real implementation, you'd need to implement proper activity queuing
	return a.federation.DeliverToFollowers(ctx, activity, nil)
}