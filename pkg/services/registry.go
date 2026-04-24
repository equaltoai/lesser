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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	pkgconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/federation/remotenotes"
	notifpush "github.com/equaltoai/lesser/pkg/notifications"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/ai"
	"github.com/equaltoai/lesser/pkg/services/bulk"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	"github.com/equaltoai/lesser/pkg/services/federationgraph"
	"github.com/equaltoai/lesser/pkg/services/hashtags"
	"github.com/equaltoai/lesser/pkg/services/importexport"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/services/media/transcoding"
	"github.com/equaltoai/lesser/pkg/services/moderationml"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/services/performance"
	"github.com/equaltoai/lesser/pkg/services/quotes"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/equaltoai/lesser/pkg/services/severance"
	"github.com/equaltoai/lesser/pkg/services/streaminganalytics"
	"github.com/equaltoai/lesser/pkg/services/threads"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

const (
	// DefaultLocalhost is the default domain name when no config is provided
	DefaultLocalhost = "localhost"
)

// Priority constants for federation activities
const (
	federationPriorityHigh   = "high"
	federationPriorityNormal = "normal"
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
	businessLogic  BusinessLogicService
	validation     ValidationService
	authentication AuthenticationService
	federation     FederationService
	timeline       TimelineService
	analytics      AnalyticsService
	notification   NotificationService

	// Domain services (new service-first architecture)
	notesService              *notes.Service
	accountsService           *accounts.Service
	relationshipsService      *relationships.Service
	conversationsService      *conversations.Service
	mediaService              *media.Service
	listsService              *lists.Service
	notificationsService      *notifications.Service
	aiService                 *ai.Service
	emojiService              *emoji.Service
	hashtagsService           *hashtags.Service
	scheduledService          *scheduled.Service
	searchService             *search.Service
	importExportService       *importexport.Service
	bulkService               *bulk.Service
	threadsService            *threads.Service
	severanceService          *severance.Service
	moderationMLService       *moderationml.Service
	quotesService             *quotes.QuoteService
	federationGraphService    *federationgraph.Service
	streamingAnalyticsService *streaminganalytics.Service
	performanceService        *performance.Service
	queryTracker              *performance.QueryTracker

	// CMS Services
	articleService     *cms.ArticleService
	draftService       *cms.DraftService
	revisionService    *cms.RevisionService
	seriesService      *cms.SeriesService
	categoryService    *cms.CategoryService
	publicationService *cms.PublicationService

	// Service management
	mu          sync.RWMutex
	initialized map[string]bool

	// Cached secrets
	secretsCache     map[string]string
	secretsCacheMu   sync.RWMutex
	awsConfigCached  *aws.Config
	awsConfigCacheMu sync.Mutex
}

// RegistryOption defines functional options for Registry configuration
type RegistryOption func(*Registry) error

// NewRegistry creates a new service registry with the provided options
// At minimum, WithStorage must be provided. Other dependencies are optional
// but recommended for full functionality.
func NewRegistry(opts ...RegistryOption) (*Registry, error) {
	r := &Registry{
		initialized:  make(map[string]bool),
		secretsCache: make(map[string]string),
	}

	// Apply all options
	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, errors.Join(ErrApplyRegistryOption, err)
		}
	}

	// Validate required dependencies
	if err := r.validate(); err != nil {
		return nil, errors.Join(ErrRegistryValidation, err)
	}

	return r, nil
}

// WithStorage configures the storage dependency (required)
func WithStorage(storage core.RepositoryStorage) RegistryOption {
	return func(r *Registry) error {
		if storage == nil {
			return ErrStorageCannotBeNil
		}
		r.storage = storage
		return nil
	}
}

// WithPublisher configures the streaming publisher (optional)
func WithPublisher(publisher streaming.Publisher) RegistryOption {
	return func(r *Registry) error {
		if publisher == nil {
			return ErrPublisherCannotBeNil
		}
		r.publisher = publisher
		return nil
	}
}

// WithLogger configures the logger (optional, defaults to no-op logger)
func WithLogger(logger *zap.Logger) RegistryOption {
	return func(r *Registry) error {
		if logger == nil {
			return ErrLoggerCannotBeNil
		}
		r.logger = logger
		return nil
	}
}

// WithConfig configures the service configuration (optional, uses defaults if not provided)
func WithConfig(config *ServiceConfig) RegistryOption {
	return func(r *Registry) error {
		if config == nil {
			return ErrConfigCannotBeNil
		}
		r.config = config
		return nil
	}
}

// validate ensures all required dependencies are provided and sets defaults
func (r *Registry) validate() error {
	if r.storage == nil {
		return ErrStorageRequired
	}

	// Set defaults for optional dependencies
	if r.logger == nil {
		r.logger = zap.NewNop()
	}

	if r.config == nil {
		cfg := pkgconfig.Get()
		jwtSecret := strings.TrimSpace(cfg.JWTSecret)
		if jwtSecret == "" && common.RunningUnitTests() {
			jwtSecret = strings.Repeat("x", 32)
		}

		if jwtSecret == "" {
			r.logger.Fatal("JWT secret configuration is required")
			panic("JWT secret configuration is required")
		}

		if err := validateJWTSecret(jwtSecret); err != nil {
			if common.RunningUnitTests() {
				jwtSecret = strings.Repeat("x", 32)
			} else {
				r.logger.Fatal("invalid JWT secret", zap.Error(err))
				panic(fmt.Sprintf("invalid JWT secret: %v", err))
			}
		}

		baseURL := cfg.BaseURL()
		if strings.TrimSpace(baseURL) == "" {
			baseURL = "https://" + DefaultLocalhost
		}

		r.config = &ServiceConfig{
			BaseURL:   baseURL,
			JWTSecret: jwtSecret,
			Config:    cfg,
		}
	}

	return nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Func, reflect.Chan:
		return rv.IsNil()
	default:
		return false
	}
}

func (r *Registry) publisherOrNoop() streaming.Publisher {
	if isNilInterface(r.publisher) {
		return streaming.NewNoopPublisher()
	}
	return r.publisher
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
		// Get or create job queue service
		jobQueue := r.getJobQueue()

		r.businessLogic = NewBusinessLogicService(
			deps,
			validation,
			authentication,
			federation,
			timeline,
			analytics,
			r.publisherOrNoop(),
			jobQueue,
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
		r.authentication = NewAuthenticationService(r.config.JWTSecret, r.config.Config, r.storage)
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

	return r.ensureAnalyticsLocked()
}

// ensureAnalyticsLocked initializes the analytics service when the registry mutex is already held.
func (r *Registry) ensureAnalyticsLocked() AnalyticsService {
	if r.analytics == nil {
		deps := &ServiceDependencies{
			Repos:  r.storage,
			Config: r.config,
			Logger: r.logger,
		}
		r.analytics = NewAnalyticsService(deps)
		if r.initialized != nil {
			r.initialized["Analytics"] = true
		}
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

// Threads returns the threads service, initializing it if necessary
func (r *Registry) Threads() *threads.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.threadsService == nil {
		threadRepo := r.storage.Thread()
		statusRepo := r.storage.Status()
		objectRepo := r.storage.Object()
		actorRepo := r.storage.Actor()

		if threadRepo != nil && statusRepo != nil {
			domain := DefaultLocalhost
			if r.config != nil && r.config.BaseURL != "" {
				domain = r.config.BaseURL
			}

			r.threadsService = threads.NewService(
				threadRepo,
				statusRepo,
				objectRepo,
				actorRepo,
				r.createThreadsFederationAdapterUnlocked(),
				r.publisherOrNoop(),
				r.logger,
				domain,
			)
			if r.initialized != nil {
				r.initialized["Threads"] = true
			}
		} else if r.logger != nil {
			r.logger.Warn("failed to initialize Threads service: required repositories not available")
		}
	}

	return r.threadsService
}

// Severance returns the severance service, initializing it if necessary
func (r *Registry) Severance() *severance.Service {
	r.mu.Lock()
	if r.severanceService != nil {
		service := r.severanceService
		r.mu.Unlock()
		return service
	}

	severanceRepo := r.storage.Severance()
	if severanceRepo == nil {
		if r.logger != nil {
			r.logger.Warn("failed to initialize Severance service: required repository not available")
		}
		r.mu.Unlock()
		return nil
	}

	domain := r.getDomainName()
	logger := r.logger
	publisherAdapter := r.createSeveranceEventPublisherAdapter()

	// Release registry lock before calling methods that acquire it (Federation / Notification).
	r.mu.Unlock()

	federationAdapter := r.createSeveranceFederationAdapter()
	notificationAdapter := r.createSeveranceNotificationAdapter()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check in case another goroutine initialized it while we were unlocked.
	if r.severanceService == nil {
		r.severanceService = severance.NewService(
			severanceRepo,
			federationAdapter,
			notificationAdapter,
			publisherAdapter,
			logger,
			domain,
		)
		if r.initialized != nil {
			r.initialized["Severance"] = true
		}
	}

	return r.severanceService
}

// FederationGraph returns the FederationGraph service instance (lazy initialization)
func (r *Registry) FederationGraph() *federationgraph.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.federationGraphService == nil {
		federationRepo := r.storage.Federation()

		if federationRepo != nil {
			domain := r.getDomainName()

			r.federationGraphService = federationgraph.NewService(
				federationRepo,
				r.logger,
				domain,
			)
			if r.initialized != nil {
				r.initialized["FederationGraph"] = true
			}
		} else if r.logger != nil {
			r.logger.Warn("failed to initialize FederationGraph service: required repository not available")
		}
	}

	return r.federationGraphService
}

// StreamingAnalytics returns the streaming analytics service instance
func (r *Registry) StreamingAnalytics() *streaminganalytics.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.streamingAnalyticsService == nil {
		analyticsRepo := r.storage.MediaAnalytics()
		popularityRepo := r.storage.MediaPopularity()
		sessionRepo := r.storage.MediaSession()

		if analyticsRepo != nil && popularityRepo != nil && sessionRepo != nil {
			r.streamingAnalyticsService = streaminganalytics.NewService(
				analyticsRepo,
				popularityRepo,
				sessionRepo,
				r.logger,
			)
			if r.initialized != nil {
				r.initialized["StreamingAnalytics"] = true
			}
		} else if r.logger != nil {
			r.logger.Warn("failed to initialize StreamingAnalytics service: required repositories not available")
		}
	}

	return r.streamingAnalyticsService
}

// ModerationML returns the moderation ML service, initializing it if necessary
func (r *Registry) ModerationML() *moderationml.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.moderationMLService == nil {
		mlRepo := r.storage.ModerationML()

		if mlRepo != nil {
			// Get AWS config
			awsCfg, err := r.getAWSConfig()
			if err != nil {
				if r.logger != nil {
					r.logger.Warn("failed to load AWS config for moderation ML", zap.Error(err))
				}
				return nil
			}

			// Build service config
			config := moderationml.Config{
				TrainingBucket:       r.getConfigString(r.config.Config, "ModerationTrainingBucketName"),
				TrainingRegion:       r.getConfigString(r.config.Config, "BedrockTrainingRegion"),
				InferenceModelID:     r.getConfigString(r.config.Config, "BedrockInferenceModelID"),
				GuardrailID:          r.getConfigString(r.config.Config, "BedrockGuardrailID"),
				GuardrailVersion:     r.getConfigString(r.config.Config, "BedrockGuardrailVersion"),
				CustomizationRoleARN: r.getConfigString(r.config.Config, "BedrockCustomizationRoleARN"),
			}

			// Set default guardrail version if not specified
			if config.GuardrailVersion == "" {
				config.GuardrailVersion = "DRAFT"
			}

			service := moderationml.NewService(
				mlRepo,
				*awsCfg,
				config,
				r.logger,
			)

			// Inject DynamoDB for event emission
			if r.storage != nil {
				if db, ok := r.storage.(interface{ GetDB() dynamormcore.DB }); ok {
					service.SetDB(db.GetDB())
				}
			}

			// Inject status repository for content fetching
			statusRepo := r.storage.Status()
			if statusRepo != nil {
				service.SetStatusRepository(statusRepo)
			}

			r.moderationMLService = service

			if r.initialized != nil {
				r.initialized["ModerationML"] = true
			}

			if config.CustomizationRoleARN == "" {
				r.logger.Warn("BEDROCK_CUSTOMIZATION_ROLE_ARN not configured - ML training will fail")
			}

			r.logger.Info("initialized moderation ML service",
				zap.String("training_bucket", config.TrainingBucket),
				zap.String("guardrail_id", config.GuardrailID),
				zap.String("customization_role", config.CustomizationRoleARN))
		} else if r.logger != nil {
			r.logger.Warn("failed to initialize ModerationML service: repository not available")
		}
	}

	return r.moderationMLService
}

// Performance returns the performance monitoring service, initializing it if necessary
func (r *Registry) Performance() *performance.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.performanceService == nil {
		// Get AWS config for CloudWatch access
		awsCfg, err := r.getAWSConfig()
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("failed to load AWS config for performance monitoring", zap.Error(err))
			}
			return nil
		}

		// Get environment name
		environment := "production"
		if r.config != nil && r.config.Config != nil {
			if env := r.getConfigString(r.config.Config, "Environment"); env != "" {
				environment = env
			}
		}

		// Create CloudWatch client
		cloudWatch := cloudwatch.NewFromConfig(*awsCfg)

		r.performanceService = performance.NewService(
			cloudWatch,
			environment,
			r.logger,
		)

		if r.initialized != nil {
			r.initialized["Performance"] = true
		}

		r.logger.Info("initialized performance monitoring service",
			zap.String("environment", environment))
	}

	return r.performanceService
}

// QueryTracker returns the query performance tracker, initializing it if necessary
func (r *Registry) QueryTracker() *performance.QueryTracker {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.queryTracker == nil {
		r.queryTracker = performance.NewQueryTracker(r.logger)

		if r.initialized != nil {
			r.initialized["QueryTracker"] = true
		}

		r.logger.Info("initialized query performance tracker")
	}

	return r.queryTracker
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

// getDomainName extracts the domain name from the registry configuration
func (r *Registry) getDomainName() string {
	domainName := DefaultLocalhost
	if r.config != nil && r.config.BaseURL != "" {
		// Extract domain from base URL
		if strings.HasPrefix(r.config.BaseURL, "https://") {
			domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
		} else if strings.HasPrefix(r.config.BaseURL, "http://") {
			domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
		}
	}
	return domainName
}

func (r *Registry) getCMSDomainName() string {
	if r.config != nil && r.config.Config != nil {
		if domain := strings.TrimSpace(r.config.Config.Domain); domain != "" {
			return domain
		}
	}
	return strings.TrimSpace(r.getDomainName())
}

func (r *Registry) getCMSMaxRevisionsPerObject() int {
	if r.config != nil && r.config.Config != nil {
		if maxRevisions := r.config.Config.CMSMaxRevisionsPerObject; maxRevisions > 0 {
			return maxRevisions
		}
	}
	return 0
}

func (r *Registry) cmsLongFormEnabled() bool {
	if r.config == nil || r.config.Config == nil {
		return true
	}
	return r.config.Config.CMSLongFormEnabled()
}

func (r *Registry) cmsDraftsEnabled() bool {
	if r.config == nil || r.config.Config == nil {
		return true
	}
	return r.config.Config.CMSDraftsEnabled()
}

func (r *Registry) cmsRevisionsEnabled() bool {
	if r.config == nil || r.config.Config == nil {
		return true
	}
	return r.config.Config.CMSRevisionsEnabled()
}

func (r *Registry) cmsSchedulingEnabled() bool {
	if r.config == nil || r.config.Config == nil {
		return true
	}
	return r.config.Config.CMSSchedulingEnabled()
}

func (r *Registry) cmsSeriesEnabled() bool {
	if r.config == nil || r.config.Config == nil {
		return true
	}
	return r.config.Config.CMSSeriesAllowed()
}

func (r *Registry) cmsCategoriesEnabled() bool {
	if r.config == nil || r.config.Config == nil {
		return true
	}
	return r.config.Config.CMSCategoriesAllowed()
}

func (r *Registry) ensureCMSArticleServiceLocked() {
	if !r.cmsLongFormEnabled() {
		return
	}
	if r.articleService != nil || r.storage == nil {
		return
	}

	if r.revisionService == nil && r.cmsRevisionsEnabled() {
		revisionRepo := r.storage.Revision()
		articleRepo := r.storage.Article()
		if revisionRepo != nil && articleRepo != nil {
			r.revisionService = cms.NewRevisionService(revisionRepo, articleRepo, r.storage.Series(), r.storage.Category(), r.getCMSMaxRevisionsPerObject(), r.logger)
			r.initialized["Revisions"] = true
		}
	}

	articleRepo := r.storage.Article()
	if articleRepo == nil {
		return
	}

	if r.federation == nil {
		deps := &ServiceDependencies{
			Repos:  r.storage,
			Config: r.config,
			Logger: r.logger,
		}
		r.federation = NewFederationService(deps)
		r.initialized["Federation"] = true
	}

	r.articleService = cms.NewArticleService(
		articleRepo,
		r.storage.Actor(),
		r.storage.Series(),
		r.storage.Category(),
		r.revisionService,
		r.federation,
		r.logger,
	)
	r.initialized["Articles"] = true
}

// Revisions returns the revision service, initializing it if necessary
func (r *Registry) Revisions() *cms.RevisionService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cmsLongFormEnabled() || !r.cmsRevisionsEnabled() {
		return nil
	}

	if r.revisionService == nil && r.storage != nil {
		revisionRepo := r.storage.Revision()
		articleRepo := r.storage.Article()

		if revisionRepo != nil && articleRepo != nil {
			r.revisionService = cms.NewRevisionService(
				revisionRepo,
				articleRepo,
				r.storage.Series(),
				r.storage.Category(),
				r.getCMSMaxRevisionsPerObject(),
				r.logger,
			)
			r.initialized["Revisions"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Revision service: required repositories not available")
			}
		}
	}

	return r.revisionService
}

// Articles returns the article service, initializing it if necessary
func (r *Registry) Articles() *cms.ArticleService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cmsLongFormEnabled() {
		return nil
	}

	if r.articleService == nil && r.storage != nil {
		articleRepo := r.storage.Article()

		// Ensure RevisionService is initialized (we can't call r.Revisions() here because we hold the lock)
		// So we manually initialize it if needed, or better, we create it here if it doesn't exist,
		// duplicating logic or extracting a helper.
		// To avoid deadlock or complexity, we'll just instantiate it if missing,
		// but we need to be careful about the 'initialized' map and consistency.
		// A better pattern used in this file is to use helper methods or just instantiate dependencies.

		// Let's use the pattern seen in other methods: check dependencies.
		// But ArticleService needs RevisionService instance.

		// We can call r.ensureRevisionsLocked() if we extract it.
		// Or just instantiate it here if nil.
		if r.revisionService == nil && r.cmsRevisionsEnabled() {
			revisionRepo := r.storage.Revision()
			articleRepo := r.storage.Article()
			if revisionRepo != nil && articleRepo != nil {
				r.revisionService = cms.NewRevisionService(revisionRepo, articleRepo, r.storage.Series(), r.storage.Category(), r.getCMSMaxRevisionsPerObject(), r.logger)
				r.initialized["Revisions"] = true
			}
		}

		if articleRepo != nil {
			// Ensure FederationService is initialized
			if r.federation == nil {
				deps := &ServiceDependencies{
					Repos:  r.storage,
					Config: r.config,
					Logger: r.logger,
				}
				r.federation = NewFederationService(deps)
				r.initialized["Federation"] = true
			}

			r.articleService = cms.NewArticleService(
				articleRepo,
				r.storage.Actor(), // Inject ActorRepository
				r.storage.Series(),
				r.storage.Category(),
				r.revisionService,
				r.federation, // Inject FederationService
				r.logger,
			)
			r.initialized["Articles"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Article service: required repositories or dependencies not available")
			}
		}
	}

	return r.articleService
}

// Drafts returns the draft service, initializing it if necessary
func (r *Registry) Drafts() *cms.DraftService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cmsLongFormEnabled() || !r.cmsDraftsEnabled() {
		return nil
	}

	if r.draftService != nil || r.storage == nil {
		return r.draftService
	}

	draftRepo := r.storage.Draft()
	if draftRepo == nil {
		if r.logger != nil {
			r.logger.Warn("failed to initialize Draft service: required repositories or dependencies not available")
		}
		return nil
	}

	r.ensureCMSArticleServiceLocked()
	if r.articleService == nil {
		if r.logger != nil {
			r.logger.Warn("failed to initialize Draft service: article service is not available")
		}
		return nil
	}

	r.draftService = cms.NewDraftService(
		draftRepo,
		r.articleService,
		r.getCMSDomainName(),
		r.cmsSchedulingEnabled(),
		r.logger,
	)
	r.initialized["Drafts"] = true

	return r.draftService
}

// Series returns the series service, initializing it if necessary
func (r *Registry) Series() *cms.SeriesService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cmsLongFormEnabled() || !r.cmsSeriesEnabled() {
		return nil
	}

	if r.seriesService == nil && r.storage != nil {
		// Assuming SeriesRepository is available in storage interface
		// If not, we would need to instantiate it here using r.storage.GetDB() if available
		seriesRepo := r.storage.Series()
		articleRepo := r.storage.Article()

		if seriesRepo != nil && articleRepo != nil {
			r.seriesService = cms.NewSeriesService(
				seriesRepo,
				articleRepo,
				r.logger,
			)
			r.initialized["Series"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Series service: required repositories not available")
			}
		}
	}

	return r.seriesService
}

// Categories returns the category service, initializing it if necessary
func (r *Registry) Categories() *cms.CategoryService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cmsLongFormEnabled() || !r.cmsCategoriesEnabled() {
		return nil
	}

	if r.categoryService == nil && r.storage != nil {
		categoryRepo := r.storage.Category()

		if categoryRepo != nil {
			r.categoryService = cms.NewCategoryService(
				categoryRepo,
				r.logger,
			)
			r.initialized["Categories"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Category service: required repository not available")
			}
		}
	}

	return r.categoryService
}

// Publications returns the publication service, initializing it if necessary
func (r *Registry) Publications() *cms.PublicationService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cmsLongFormEnabled() {
		return nil
	}

	if r.publicationService == nil && r.storage != nil {
		pubRepo := r.storage.Publication()
		// We assume PublicationMember repository is also available via storage
		// If not, we might need to add it to the interface
		pubMemberRepo := r.storage.PublicationMember()

		if pubRepo != nil && pubMemberRepo != nil {
			r.publicationService = cms.NewPublicationService(
				pubRepo,
				pubMemberRepo,
				r.logger,
			)
			r.initialized["Publications"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Publication service: required repositories not available")
			}
		}
	}

	return r.publicationService
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

	if r.notesService == nil && r.storage != nil {
		// Initialize the Notes service with repository interfaces
		statusRepo := r.storage.Status()
		accountRepo := r.storage.Account()
		bookmarkRepo := r.storage.Bookmark()
		likeRepo := r.storage.Like()
		socialRepo := r.storage.Social()
		conversationRepo := r.storage.Conversation()
		objectRepo := r.storage.Object()
		searchRepo := r.storage.Search()
		communityNoteRepo := r.storage.CommunityNote()
		userRepo := r.storage.User()
		pollRepo := r.storage.Poll()

		// Check if repositories are available
		if statusRepo != nil && accountRepo != nil {
			domainName := DefaultLocalhost
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}

			analyticsService := r.ensureAnalyticsLocked()
			notificationsService := r.ensureNotificationsServiceLocked()

			r.notesService = notes.NewService(
				statusRepo,
				accountRepo,
				bookmarkRepo,
				r.storage.Relationship(), // Add relationship repository
				r.storage.Media(),
				likeRepo,
				socialRepo,
				conversationRepo,
				objectRepo,
				searchRepo,
				communityNoteRepo,
				userRepo,
				pollRepo, // Add poll repository
				r.publisherOrNoop(),
				analyticsService,                         // Analytics service
				r.createNotesFederationAdapterUnlocked(), // Federation service adapter
				r.createNotesReplyParentResolverUnlocked(domainName),
				notificationsService,
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
		if r.logger != nil {
			r.logger.Info("registry: initializing Accounts service")
		}
		// Create adapter services for crypto and auth
		cryptoAdapter := NewCryptoAdapter()
		authAdapter := NewAuthAdapter(r.config.JWTSecret, r.storage)

		// Create federation adapter using unlocked helper to avoid deadlock
		federationAdapter := r.createAccountsFederationAdapterUnlocked()

		// Extract domain from base URL (same pattern as Notes and Relationships services)
		domainName := DefaultLocalhost
		if r.config != nil && r.config.BaseURL != "" {
			// Extract domain from base URL
			if strings.HasPrefix(r.config.BaseURL, "https://") {
				domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
			} else if strings.HasPrefix(r.config.BaseURL, "http://") {
				domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
			} else {
				domainName = r.config.BaseURL
			}
		}

		r.accountsService = accounts.NewService(
			r.storage,
			r.publisherOrNoop(),
			federationAdapter,
			cryptoAdapter,
			authAdapter,
			r.logger,
			domainName,
		)
		r.initialized["Accounts"] = true
		if r.logger != nil {
			r.logger.Info("registry: Accounts service initialized")
		}
	}

	return r.accountsService
}

// Relationships returns the relationships service, initializing it if necessary
func (r *Registry) Relationships() *relationships.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.relationshipsService == nil && r.storage != nil {
		domainName := DefaultLocalhost
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
			r.publisherOrNoop(),
			r.createRelationshipsFederationAdapterUnlocked(), // federation service adapter
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
		conversationRepo := r.storage.Conversation()
		noteRepo := r.storage.Status()
		accountRepo := r.storage.Account()

		// Check if required repositories are available
		if conversationRepo != nil && noteRepo != nil && accountRepo != nil {
			relationshipRepo := r.storage.Relationship()
			userRepo := r.storage.User()
			rateLimitRepo := r.storage.RateLimit()
			auditRepo := r.storage.Audit()

			domainName := DefaultLocalhost
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}

			// Create a simple federation service adapter for conversations
			federationService := &simpleFederationService{logger: r.logger}

			r.conversationsService = conversations.NewService(
				conversationRepo,
				noteRepo,
				repositories.NewDirectMessageTombstoneRepository(r.storage.GetDB(), r.storage.GetTableName(), r.logger),
				accountRepo,
				relationshipRepo,
				userRepo,
				rateLimitRepo,
				auditRepo,
				r.publisherOrNoop(),
				federationService,
				r.logger,
				domainName,
			)
			r.initialized["Conversations"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Conversations service: required repositories not available")
			}
		}
	}

	return r.conversationsService
}

// Media returns the media service, initializing it if necessary
func (r *Registry) Media() *media.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.mediaService == nil && r.storage != nil {
		// Initialize the Media service with repository interfaces
		mediaRepo := r.storage.Media()
		accountRepo := r.storage.Account()

		// Check if repositories are available
		if mediaRepo != nil && accountRepo != nil {
			// Create a simple job queue service if not available
			// In production, this would be a proper SQS-based implementation
			jobQueue := &simpleJobQueue{logger: r.logger}

			// Create an adapter for the media service's job queue interface
			mediaJobQueue := &mediaJobQueueAdapter{jobQueue: jobQueue}

			// Get media configuration from config
			sourceBucket := r.getMediaSourceBucket()
			cdnDomain := r.getCloudFrontDomain()

			r.mediaService = media.NewService(
				mediaRepo,
				accountRepo,
				r.publisherOrNoop(),
				mediaJobQueue,
				r.logger,
				sourceBucket,
				cdnDomain,
			)

			// Wire up optional streaming services if config is available
			r.wireMediaStreamingServices(r.mediaService)

			if r.config != nil && r.config.Config != nil && r.config.Config.MaxUploadSize > 0 {
				r.mediaService.SetMaxFileSize(r.config.Config.MaxUploadSize)
			}

			r.initialized["Media"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Media service: required repositories not available")
			}
		}
	}

	return r.mediaService
}

// wireMediaStreamingServices wires up the optional transcoding, manifest, and CloudFront services
func (r *Registry) wireMediaStreamingServices(mediaService *media.Service) {
	if mediaService == nil || r.config == nil || r.config.Config == nil {
		return
	}

	cfg := r.config.Config

	// Initialize MediaConvert transcoding service if configured
	transcodingService := r.initializeTranscodingService(cfg)
	if transcodingService != nil {
		mediaService.SetTranscodingService(transcodingService)
		r.logger.Info("MediaConvert transcoding service initialized")
	}

	// Initialize manifest generation service if configured
	manifestService := r.initializeManifestService(cfg)
	if manifestService != nil {
		mediaService.SetManifestService(manifestService)
		r.logger.Info("Manifest generation service initialized")
	}

	// Initialize CloudFront signing service if configured
	cloudfrontService := r.initializeCloudFrontService(cfg)
	if cloudfrontService != nil {
		mediaService.SetCloudFrontService(cloudfrontService)
		r.logger.Info("CloudFront signing service initialized")
	}
}

// initializeTranscodingService creates the AWS MediaConvert transcoding service
func (r *Registry) initializeTranscodingService(cfg interface{}) *transcoding.Service {
	// Extract config fields
	endpoint := r.getConfigString(cfg, "MediaConvertEndpoint")
	if endpoint == "" {
		r.logger.Debug("MediaConvert endpoint not configured, skipping transcoding service initialization")
		return nil
	}

	// Get cached AWS config
	awsCfg, err := r.getAWSConfig()
	if err != nil {
		r.logger.Warn("failed to load AWS config for MediaConvert", zap.Error(err))
		return nil
	}
	awsConfig := *awsCfg

	// Create transcoding service config
	transcodingConfig := transcoding.Config{
		Endpoint:          endpoint,
		DestinationBucket: r.getMediaStreamingBucket(),
		DestinationPrefix: "transcoded/",
		Role:              r.getConfigString(cfg, "MediaConvertRoleArn"),
	}

	// Create the service
	service, err := transcoding.NewService(awsConfig, transcodingConfig, r.logger)
	if err != nil {
		r.logger.Warn("failed to initialize MediaConvert transcoding service", zap.Error(err))
		return nil
	}

	return service
}

// initializeManifestService creates the HLS/DASH manifest generation service
func (r *Registry) initializeManifestService(_ interface{}) *transcoding.ManifestService {
	streamingBucket := r.getMediaStreamingBucket()
	if streamingBucket == "" {
		r.logger.Debug("streaming bucket not configured, skipping manifest service initialization")
		return nil
	}

	// Get cached AWS config and create S3 client
	awsCfg, err := r.getAWSConfig()
	if err != nil {
		r.logger.Warn("failed to load AWS config for manifest service", zap.Error(err))
		return nil
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(*awsCfg)

	// Create manifest service config
	manifestConfig := transcoding.ManifestConfig{
		Bucket:    streamingBucket,
		CDNDomain: r.getCloudFrontDomain(),
	}

	// Create the service
	service := transcoding.NewManifestService(s3Client, manifestConfig, r.logger)
	return service
}

// initializeCloudFrontService creates the CloudFront URL signing service
func (r *Registry) initializeCloudFrontService(cfg interface{}) *transcoding.CloudFrontService {
	domain := r.getCloudFrontDomain()
	keyPairID := r.getConfigString(cfg, "CloudFrontKeyPairID")
	privateKeyPath := r.getConfigString(cfg, "CloudFrontPrivateKeyPath")

	if domain == "" || keyPairID == "" || privateKeyPath == "" {
		r.logger.Debug("CloudFront configuration incomplete, skipping signing service initialization")
		return nil
	}

	// Read private key
	privateKey, err := r.readCloudFrontPrivateKey(privateKeyPath)
	if err != nil {
		r.logger.Warn("failed to read CloudFront private key", zap.Error(err))
		return nil
	}

	// Get TTL from config (default 24 hours)
	ttlHours := r.getConfigInt(cfg, "ManifestTTLHours")
	if ttlHours == 0 {
		ttlHours = 24
	}

	// Create CloudFront service config
	cloudfrontConfig := transcoding.CloudFrontConfig{
		Domain:        domain,
		KeyPairID:     keyPairID,
		PrivateKeyPEM: privateKey,
		DefaultTTL:    time.Duration(ttlHours) * time.Hour,
	}

	// Create the service
	service, err := transcoding.NewCloudFrontService(cloudfrontConfig, r.logger)
	if err != nil {
		r.logger.Warn("failed to initialize CloudFront signing service", zap.Error(err))
		return nil
	}

	return service
}

// Helper functions to extract configuration values

func (r *Registry) getMediaSourceBucket() string {
	if r.config == nil || r.config.Config == nil {
		return "lesser-media-bucket" // Default fallback
	}
	val := r.getConfigString(r.config.Config, "MediaSourceBucketName")
	if val == "" {
		val = r.getConfigString(r.config.Config, "S3BucketName") // Fallback to main bucket
	}
	if val == "" {
		return "lesser-media-bucket"
	}
	return val
}

func (r *Registry) getMediaStreamingBucket() string {
	if r.config == nil || r.config.Config == nil {
		return ""
	}
	return r.getConfigString(r.config.Config, "MediaStreamingBucketName")
}

func (r *Registry) getCloudFrontDomain() string {
	if r.config == nil || r.config.Config == nil {
		return "cdn.example.com" // Default fallback
	}
	val := r.getConfigString(r.config.Config, "CloudFrontDomain")
	if val == "" {
		return "cdn.example.com"
	}
	return val
}

func (r *Registry) getConfigString(cfg interface{}, key string) string {
	// Try reflection to get field value
	if cfgStruct, ok := cfg.(*pkgconfig.Config); ok {
		switch key {
		case "MediaConvertEndpoint":
			return cfgStruct.MediaConvertEndpoint
		case "MediaConvertRoleArn":
			return cfgStruct.MediaConvertRoleArn
		case "CloudFrontKeyPairID":
			return cfgStruct.CloudFrontKeyPairID
		case "CloudFrontPrivateKeyPath":
			return cfgStruct.CloudFrontPrivateKeyPath
		case "MediaSourceBucketName":
			return cfgStruct.MediaSourceBucketName
		case "MediaStreamingBucketName":
			return cfgStruct.MediaStreamingBucketName
		case "CloudFrontDomain":
			return cfgStruct.CloudFrontDomain
		case "S3BucketName":
			return cfgStruct.S3BucketName
		case "ModerationTrainingBucketName":
			return cfgStruct.ModerationTrainingBucketName
		case "BedrockTrainingRegion":
			return cfgStruct.BedrockTrainingRegion
		case "BedrockInferenceModelID":
			return cfgStruct.BedrockInferenceModelID
		case "BedrockGuardrailID":
			return cfgStruct.BedrockGuardrailID
		case "BedrockGuardrailVersion":
			return cfgStruct.BedrockGuardrailVersion
		case "BedrockCustomizationRoleARN":
			return cfgStruct.BedrockCustomizationRoleARN
		}
	}
	return ""
}

func (r *Registry) getConfigInt(cfg interface{}, key string) int {
	if cfgStruct, ok := cfg.(*pkgconfig.Config); ok {
		switch key {
		case "ManifestTTLHours":
			return cfgStruct.ManifestTTLHours
		}
	}
	return 0
}

func (r *Registry) readCloudFrontPrivateKey(keyPath string) (string, error) {
	// If it looks like an ARN or secret path, try Secrets Manager
	if strings.HasPrefix(keyPath, "arn:aws:secretsmanager:") || strings.HasPrefix(keyPath, "lesser/") {
		return r.getSecretFromSecretsManager(keyPath)
	}

	// Otherwise, try reading from file
	// #nosec G304 -- keyPath is from configuration, not user input
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read private key from %s: %w", keyPath, err)
	}

	return string(keyBytes), nil
}

// getSecretFromSecretsManager retrieves a secret from AWS Secrets Manager with caching
func (r *Registry) getSecretFromSecretsManager(secretID string) (string, error) {
	// Check cache first
	r.secretsCacheMu.RLock()
	if cached, ok := r.secretsCache[secretID]; ok {
		r.secretsCacheMu.RUnlock()
		return cached, nil
	}
	r.secretsCacheMu.RUnlock()

	// Load AWS config
	awsCfg, err := r.getAWSConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config for Secrets Manager: %w", err)
	}

	// Create Secrets Manager client
	client := secretsmanager.NewFromConfig(*awsCfg)

	// Get secret value
	ctx := context.Background()
	result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to retrieve secret %s from Secrets Manager: %w", secretID, err)
	}

	if result.SecretString == nil {
		return "", fmt.Errorf("secret %s has no string value", secretID)
	}

	secretValue := *result.SecretString

	// If the secret is JSON (from CloudFront key generation), extract the privateKey field
	if strings.HasPrefix(strings.TrimSpace(secretValue), "{") {
		var secretData map[string]interface{}
		if err := json.Unmarshal([]byte(secretValue), &secretData); err == nil {
			if privateKey, ok := secretData["privateKey"].(string); ok {
				secretValue = privateKey
			}
		}
	}

	// Cache the secret
	r.secretsCacheMu.Lock()
	r.secretsCache[secretID] = secretValue
	r.secretsCacheMu.Unlock()

	r.logger.Info("successfully retrieved secret from Secrets Manager",
		zap.String("secret_id", secretID))

	return secretValue, nil
}

// getAWSConfig returns a cached AWS config or loads a new one
func (r *Registry) getAWSConfig() (*aws.Config, error) {
	r.awsConfigCacheMu.Lock()
	defer r.awsConfigCacheMu.Unlock()

	if r.awsConfigCached != nil {
		return r.awsConfigCached, nil
	}

	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	r.awsConfigCached = &cfg
	return r.awsConfigCached, nil
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

	return r.ensureNotificationsServiceLocked()
}

func (r *Registry) ensureNotificationsServiceLocked() *notifications.Service {
	if r.notificationsService == nil && r.storage != nil {
		// Initialize the Notifications service with repository interfaces
		notificationRepo := r.storage.Notification()
		accountRepo := r.storage.Account()

		// Check if repositories are available
		if notificationRepo != nil && accountRepo != nil {
			domainName := DefaultLocalhost
			if r.config != nil && r.config.BaseURL != "" {
				// Extract domain from base URL
				if strings.HasPrefix(r.config.BaseURL, "https://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
				} else if strings.HasPrefix(r.config.BaseURL, "http://") {
					domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
				}
			}

			var (
				pushService *notifpush.PushService
				err         error
			)
			if r.config != nil {
				pushService, err = notifpush.NewPushService(r.config.Config)
				if err != nil && r.logger != nil {
					r.logger.Warn("failed to initialize push service",
						zap.Error(err))
				}
			}

			r.notificationsService = notifications.NewService(
				notificationRepo,
				accountRepo,
				r.publisher,
				r.logger,
				domainName,
				pushService,
			)
			notificationRepo.SetDispatcher(r.notificationsService)
			if r.initialized != nil {
				r.initialized["Notifications"] = true
			}
		} else if r.logger != nil {
			r.logger.Warn("failed to initialize Notifications service: required repositories not available")
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
			domainName := DefaultLocalhost
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

// Hashtags returns the hashtags service, initializing it if necessary
func (r *Registry) Hashtags() *hashtags.Service {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hashtagsService == nil && r.storage != nil {
		// Initialize the Hashtags service with repository interfaces
		hashtagRepo := r.storage.Hashtag()
		accountRepo := r.storage.Account()
		objectRepo := r.storage.Object()

		// Check if repositories are available
		if hashtagRepo != nil && accountRepo != nil && objectRepo != nil {
			r.hashtagsService = hashtags.NewService(
				hashtagRepo,
				accountRepo,
				objectRepo,
				r.publisher,
				r.logger,
			)
			r.initialized["Hashtags"] = true
		} else {
			if r.logger != nil {
				r.logger.Warn("failed to initialize Hashtags service: required repositories not available")
			}
		}
	}

	return r.hashtagsService
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
			r.scheduledService = scheduled.NewService(
				scheduledRepo,
				statusRepo,
				mediaRepo,
				r.publisher,
				r.logger,
				r.getDomainName(),
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
		hashtagRepo := r.storage.Hashtag()

		// Check if repositories are available
		if searchRepo != nil && actorRepo != nil && relationshipRepo != nil && statusRepo != nil && hashtagRepo != nil {
			domainName := DefaultLocalhost
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
				hashtagRepo,
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
		if r.initializeImportExportService() {
			r.initialized["ImportExport"] = true
		}
	}

	return r.importExportService
}

// initializeImportExportService initializes the ImportExport service with all dependencies
func (r *Registry) initializeImportExportService() bool {
	// Get and validate all required repositories
	repos := r.getImportExportRepositories()
	if !r.validateImportExportRepositories(repos) {
		return false
	}

	// Extract domain name from configuration
	domainName := r.extractDomainName()

	var (
		queueService  importexport.QueueService
		storageClient importexport.StorageClient
	)

	// Initialize AWS services for import/export.
	//
	// Unit tests and local harnesses don't need (and often can't support) live AWS clients.
	// When IntegrationTestMode is enabled, keep initialization local and fast by skipping
	// AWS wiring entirely.
	if r.config != nil && r.config.Config != nil && !r.config.Config.IntegrationTestMode {
		ctx := context.Background()
		queue, queueErr := NewImportExportQueueService(ctx, r.config.Config, repos.exportRepo, repos.importRepo, r.logger)
		storage, storageErr := NewAWSS3StorageClient(ctx, r.logger)

		queueService = queue
		storageClient = storage

		// Log errors but don't fail initialization - services can work without AWS integration
		if queueErr != nil {
			r.logger.Warn("failed to initialize AWS queue service, import/export will work without async processing",
				zap.Error(queueErr))
		}
		if storageErr != nil {
			r.logger.Warn("failed to initialize AWS storage client, import/export will work with limited file support",
				zap.Error(storageErr))
		}
	}

	// Create the ImportExport service
	r.importExportService = importexport.NewService(
		repos.exportRepo,
		repos.importRepo,
		repos.statusRepo,
		repos.accountRepo,
		repos.mediaRepo,
		repos.socialRepo,
		r.publisher,
		queueService,  // Will be nil if initialization failed
		storageClient, // Will be nil if initialization failed
		r.logger,
		domainName,
	)

	return true
}

// importExportRepositories holds all required repositories for ImportExport service
type importExportRepositories struct {
	exportRepo  *repositories.ExportRepository
	importRepo  *repositories.ImportRepository
	statusRepo  interfaces.StatusRepository
	accountRepo interfaces.AccountRepository
	mediaRepo   *repositories.MediaRepository
	socialRepo  interfaces.SocialRepository
}

// getImportExportRepositories retrieves all required repositories
func (r *Registry) getImportExportRepositories() importExportRepositories {
	return importExportRepositories{
		exportRepo:  r.storage.Export(),
		importRepo:  r.storage.Import(),
		statusRepo:  r.storage.Status(),
		accountRepo: r.storage.Account(),
		mediaRepo:   r.storage.Media(),
		socialRepo:  r.storage.Social(),
	}
}

// validateImportExportRepositories checks if all required repositories are available
func (r *Registry) validateImportExportRepositories(repos importExportRepositories) bool {
	if repos.exportRepo == nil || repos.importRepo == nil || repos.statusRepo == nil ||
		repos.accountRepo == nil || repos.mediaRepo == nil || repos.socialRepo == nil {
		if r.logger != nil {
			r.logger.Warn("failed to initialize ImportExport service: required repositories not available")
		}
		return false
	}
	return true
}

// extractDomainName extracts domain name from configuration
func (r *Registry) extractDomainName() string {
	if r.config == nil || r.config.BaseURL == "" {
		return DefaultLocalhost
	}

	baseURL := r.config.BaseURL
	if strings.HasPrefix(baseURL, "https://") {
		return strings.TrimPrefix(baseURL, "https://")
	}
	if strings.HasPrefix(baseURL, "http://") {
		return strings.TrimPrefix(baseURL, "http://")
	}

	return DefaultLocalhost
}

// Bulk returns the Bulk service, initializing it if necessary
func (r *Registry) Bulk() *bulk.Service {
	r.mu.Lock()
	if r.bulkService != nil || r.storage == nil {
		service := r.bulkService
		r.mu.Unlock()
		return service
	}

	// Initialize the Bulk service with repository interfaces
	statusRepo := r.storage.Status()
	accountRepo := r.storage.Account()
	socialRepo := r.storage.Social()
	listRepo := r.storage.List()
	relationshipRepo := r.storage.Relationship()

	// Check if repositories are available
	if statusRepo == nil || accountRepo == nil || socialRepo == nil || listRepo == nil || relationshipRepo == nil {
		if r.logger != nil {
			r.logger.Warn("failed to initialize Bulk service: required repositories not available")
		}
		r.mu.Unlock()
		return nil
	}

	domainName := DefaultLocalhost
	if r.config != nil && r.config.BaseURL != "" {
		// Extract domain from base URL
		if strings.HasPrefix(r.config.BaseURL, "https://") {
			domainName = strings.TrimPrefix(r.config.BaseURL, "https://")
		} else if strings.HasPrefix(r.config.BaseURL, "http://") {
			domainName = strings.TrimPrefix(r.config.BaseURL, "http://")
		}
	}

	publisher := r.publisher
	logger := r.logger
	r.mu.Unlock()

	// Initialize federation service (may be nil during testing)
	federationService := r.Federation()

	// Create adapter for federation service interface
	var bulkFederation bulk.FederationService
	if federationService != nil {
		jobQueue := r.getJobQueue()
		bulkFederation = &federationServiceAdapter{
			federation: federationService,
			jobQueue:   jobQueue,
		}
	}

	service := bulk.NewService(
		statusRepo,
		accountRepo,
		socialRepo,
		listRepo,
		relationshipRepo,
		publisher,
		bulkFederation,
		logger,
		domainName,
	)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check in case another goroutine initialized it while we were unlocked.
	if r.bulkService == nil {
		r.bulkService = service
		if r.initialized != nil {
			r.initialized["Bulk"] = true
		}
	}

	return r.bulkService
}

// Quotes returns the quotes service, initializing it if necessary
func (r *Registry) Quotes() *quotes.QuoteService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.quotesService == nil && r.storage != nil {
		r.quotesService = quotes.NewQuoteService(r.storage, r.logger)
		r.initialized["Quotes"] = true
	}

	return r.quotesService
}

// createFederationAdapterUnlocked creates federation adapter without locking mutex
// This is used internally when already holding the lock to avoid deadlock
func (r *Registry) createFederationAdapterUnlocked() *queueFederationAdapter {
	// Initialize federation service inline without locking
	if r.federation == nil {
		deps := &ServiceDependencies{
			Repos:  r.storage,
			Config: r.config,
			Logger: r.logger,
		}
		r.federation = NewFederationService(deps)
		r.initialized["Federation"] = true
	}

	return &queueFederationAdapter{
		federation: r.federation,
		storage:    r.storage,
		logger:     r.logger,
	}
}

// Convenience methods that use the unlocked version
func (r *Registry) createAccountsFederationAdapterUnlocked() *queueFederationAdapter {
	return r.createFederationAdapterUnlocked()
}

func (r *Registry) createNotesFederationAdapterUnlocked() *queueFederationAdapter {
	return r.createFederationAdapterUnlocked()
}

func (r *Registry) createNotesReplyParentResolverUnlocked(domainName string) notes.ReplyParentResolver {
	if r == nil || r.storage == nil {
		return nil
	}

	return remotenotes.NewReplyParentResolver(
		r.storage.Status(),
		r.storage.Object(),
		r.storage.DomainBlock(),
		federation.NewAuthorizedFetchService(r.storage, domainName, r.logger),
		domainName,
		r.logger,
	)
}

func (r *Registry) createRelationshipsFederationAdapterUnlocked() *queueFederationAdapter {
	return r.createFederationAdapterUnlocked()
}

func (r *Registry) createThreadsFederationAdapterUnlocked() *queueFederationAdapter {
	return r.createFederationAdapterUnlocked()
}

// createSeveranceFederationAdapter creates the federation adapter for the Severance service
func (r *Registry) createSeveranceFederationAdapter() severance.FederationService {
	return &severanceFederationAdapter{
		federation: r.Federation(),
	}
}

// createSeveranceNotificationAdapter creates the notification adapter for the Severance service
func (r *Registry) createSeveranceNotificationAdapter() severance.NotificationService {
	return &severanceNotificationAdapter{
		notification: r.Notification(),
	}
}

// severanceEventPublisherAdapter adapts streaming.Publisher to severance.EventPublisher
type severanceEventPublisherAdapter struct {
	storage core.RepositoryStorage
}

// PublishEvent implements severance.EventPublisher by saving the event to DynamoDB
func (a *severanceEventPublisherAdapter) PublishEvent(ctx context.Context, event *models.StreamingEvent) error {
	if a.storage == nil {
		return nil // Skip if no storage
	}

	// Get the DB directly and save the event
	db := a.storage.GetDB()
	if db == nil {
		return nil
	}

	// Save the streaming event to DynamoDB - it will be picked up by the stream-router
	return db.WithContext(ctx).Model(event).Create()
}

// createSeveranceEventPublisherAdapter creates the event publisher adapter for the Severance service
func (r *Registry) createSeveranceEventPublisherAdapter() severance.EventPublisher {
	return &severanceEventPublisherAdapter{
		storage: r.storage,
	}
}

// StreamingConnectionRepository returns the streaming connection repository for WebSocket subscriptions
func (r *Registry) StreamingConnectionRepository() interfaces.StreamingConnectionRepository {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.storage == nil {
		r.logger.Warn("storage is nil, cannot return StreamingConnectionRepository")
		return nil
	}

	return r.storage.StreamingConnection()
}

// Publisher returns the configured streaming publisher for real-time events
func (r *Registry) Publisher() streaming.Publisher {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.publisher
}

// federationServiceAdapter adapts the registry's FederationService to the bulk service's interface
type federationServiceAdapter struct {
	federation FederationService
	jobQueue   JobQueueServiceInterface
}

// QueueActivity implements bulk.FederationService
func (a *federationServiceAdapter) QueueActivity(ctx context.Context, activity *activitypub.Activity) error {
	if a.jobQueue == nil {
		// Fallback to direct delivery if queue is not available
		return a.federation.DeliverToFollowers(ctx, activity, nil)
	}

	// Serialize activity to map for queuing
	activityData := make(map[string]interface{})
	if data, err := json.Marshal(activity); err == nil {
		_ = json.Unmarshal(data, &activityData)
	}

	// Determine recipients based on activity
	recipients, err := a.determineRecipients(ctx, activity)
	if err != nil {
		// If we can't determine recipients, fall back to delivery
		return a.federation.DeliverToFollowers(ctx, activity, nil)
	}

	// Determine priority based on activity type
	priority := a.determinePriority(activity)

	// Create activity job message
	activityJob := ActivityJobMessage{
		ActivityID:   activity.ID,
		ActivityData: activityData,
		ActorID:      activity.Actor,
		Recipients:   recipients,
		Priority:     priority,
		Timestamp:    time.Now().Unix(),
	}

	// Queue the activity for delivery
	return a.jobQueue.QueueActivityJob(ctx, activityJob)
}

// determineRecipients extracts recipients from the activity
func (a *federationServiceAdapter) determineRecipients(_ context.Context, activity *activitypub.Activity) ([]string, error) {
	var recipients []string

	// Add 'to' recipients
	recipients = append(recipients, activity.To...)

	// Add 'cc' recipients
	recipients = append(recipients, activity.CC...)

	// Filter out public addresses and collections, keep only specific actors
	var filteredRecipients []string
	for _, recipient := range recipients {
		if recipient != activitypub.PublicAddress &&
			!strings.Contains(recipient, "/followers") &&
			!strings.Contains(recipient, "/following") {
			filteredRecipients = append(filteredRecipients, recipient)
		}
	}

	// If no specific recipients but has followers collection, use federation service
	if len(filteredRecipients) == 0 && a.federation != nil {
		// This would need to get followers, but for now return empty to use fallback
		return []string{}, nil
	}

	return filteredRecipients, nil
}

// determinePriority assigns priority based on activity type
func (a *federationServiceAdapter) determinePriority(activity *activitypub.Activity) string {
	switch activity.Type {
	case "Delete", "Undo":
		return federationPriorityHigh // Deletions and undos should be processed quickly
	case "Follow", "Accept", "Reject":
		return federationPriorityHigh // Social actions should be timely
	case "Like", "Announce":
		return federationPriorityNormal // Engagement can have normal priority
	case "Create", "Update":
		return federationPriorityNormal // Content creation is important but not urgent
	case "Flag", "Block":
		return federationPriorityHigh // Moderation actions need quick processing
	default:
		return federationPriorityNormal
	}
}

// mediaJobQueueAdapter adapts our JobQueueServiceInterface to the media service's JobQueueService interface
type mediaJobQueueAdapter struct {
	jobQueue JobQueueServiceInterface
}

// QueueMediaJob implements the media service's JobQueueService interface
func (a *mediaJobQueueAdapter) QueueMediaJob(ctx context.Context, msg media.JobMessage) error {
	// Convert media.JobMessage to our MediaJobMessage
	mediaJobMsg := MediaJobMessage{
		JobID:     msg.JobID,
		MediaID:   msg.MediaID,
		Username:  msg.Username,
		Timestamp: msg.Timestamp,
	}
	return a.jobQueue.QueueMediaJob(ctx, mediaJobMsg)
}

// simpleJobQueue provides a basic implementation of JobQueueService for development
// In production, this would be replaced with a proper SQS-based implementation
type simpleJobQueue struct {
	logger *zap.Logger
}

// QueueImportJob queues an import processing job (simple logging implementation)
func (q *simpleJobQueue) QueueImportJob(_ context.Context, msg ImportJobMessage) error {
	if q.logger != nil {
		q.logger.Info("queued import job (simple implementation)",
			zap.String("import_id", msg.ImportID),
			zap.String("username", msg.Username),
			zap.String("type", msg.Type))
	}
	return nil
}

// QueueExportJob queues an export generation job (simple logging implementation)
func (q *simpleJobQueue) QueueExportJob(_ context.Context, msg ExportJobMessage) error {
	if q.logger != nil {
		q.logger.Info("queued export job (simple implementation)",
			zap.String("export_id", msg.ExportID),
			zap.String("username", msg.Username),
			zap.String("type", msg.Type))
	}
	return nil
}

// QueueMediaJob queues a media processing job (simple logging implementation)
func (q *simpleJobQueue) QueueMediaJob(_ context.Context, msg MediaJobMessage) error {
	if q.logger != nil {
		q.logger.Info("queued media job (simple implementation)",
			zap.String("job_id", msg.JobID),
			zap.String("media_id", msg.MediaID),
			zap.String("username", msg.Username))
	}
	return nil
}

// QueueScheduledJob queues a scheduled status publishing job (simple logging implementation)
func (q *simpleJobQueue) QueueScheduledJob(_ context.Context, msg ScheduledJobMessage) error {
	if q.logger != nil {
		q.logger.Info("queued scheduled job (simple implementation)",
			zap.String("scheduled_status_id", msg.ScheduledStatusID),
			zap.String("username", msg.Username),
			zap.Time("scheduled_at", msg.ScheduledAt))
	}
	return nil
}

// QueueActivityJob queues a federation activity delivery job (simple logging implementation)
func (q *simpleJobQueue) QueueActivityJob(_ context.Context, msg ActivityJobMessage) error {
	if q.logger != nil {
		q.logger.Info("queued activity job (simple implementation)",
			zap.String("activity_id", msg.ActivityID),
			zap.String("actor_id", msg.ActorID),
			zap.String("priority", msg.Priority),
			zap.Int("recipients_count", len(msg.Recipients)))
	}
	return nil
}

// QueueDelayedJob queues a delayed job (simple logging implementation)
func (q *simpleJobQueue) QueueDelayedJob(_ context.Context, queueName string, _ interface{}, delaySeconds int32) error {
	if q.logger != nil {
		q.logger.Info("queued delayed job (simple implementation)",
			zap.String("queue", queueName),
			zap.Int32("delay_seconds", delaySeconds))
	}
	return nil
}

// queueFederationAdapter adapts the main FederationService to service-specific FederationService interfaces
// that only need QueueActivity functionality (used by notes, accounts, and relationships services)
type queueFederationAdapter struct {
	federation FederationService
	storage    core.RepositoryStorage
	logger     *zap.Logger
}

type followerRecipientFederationService interface {
	DeliverToFollowersAndRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error
}

// severanceFederationAdapter adapts FederationService to severance.FederationService
type severanceFederationAdapter struct {
	federation FederationService
}

// CheckInstanceReachability checks if a remote instance is reachable
func (a *severanceFederationAdapter) CheckInstanceReachability(_ context.Context, instance string) (bool, error) {
	if instance == "" {
		return false, fmt.Errorf("instance cannot be empty")
	}

	// Use federation service to check instance health if available
	if a.federation == nil {
		// If no federation service, assume reachable
		return true, nil
	}

	// Check if the instance is blocked or has health issues
	// This integrates with the existing federation infrastructure
	// Note: GetInstanceHealth is part of the FederationService interface
	return true, nil // Simplified for now - full implementation would use federation.GetInstanceHealth
}

// severanceNotificationAdapter adapts NotificationService to severance.NotificationService
type severanceNotificationAdapter struct {
	notification NotificationService
}

// SendSeveranceNotification sends a notification about a severed relationship
func (a *severanceNotificationAdapter) SendSeveranceNotification(_ context.Context, userID, severanceID string, reason models.SeveranceReason) error {
	if userID == "" || severanceID == "" {
		return fmt.Errorf("userID and severanceID cannot be empty")
	}

	// Use the notification service to send actual notifications
	if a.notification == nil {
		// If no notification service, skip notification
		return nil
	}

	// Create a severance notification
	// This integrates with the existing notification infrastructure
	notificationType := "severance_detected"
	targetID := severanceID

	// Simplified notification - in production would call notification.CreateNotification
	// with proper severance metadata
	_ = notificationType
	_ = targetID
	_ = reason

	return nil
}

// NotifySeverance sends a notification about a severed relationship
func (a *severanceNotificationAdapter) NotifySeverance(ctx context.Context, userID, severanceID string) error {
	return a.SendSeveranceNotification(ctx, userID, severanceID, models.SeveranceReasonOther)
}

// FetchObject implements threads.FederationClient.FetchObject for fetching remote ActivityPub objects
func (a *queueFederationAdapter) FetchObject(_ context.Context, objectURL string, _ *activitypub.Actor) (any, error) {
	// This is a stub implementation - in a real system, this would:
	// 1. Make an HTTP GET request to the objectURL with proper ActivityPub headers
	// 2. Sign the request using the signingActor's keys (HTTP Signatures)
	// 3. Parse and validate the response JSON-LD
	// 4. Return the parsed ActivityPub object

	a.logger.Warn("FetchObject not fully implemented yet - federation client needed",
		zap.String("object_url", objectURL))

	// For now, return an error indicating this needs to be implemented
	return nil, errors.New("federation client not fully configured")
}

// QueueActivity implements the FederationService interface by using the main federation service
func (a *queueFederationAdapter) QueueActivity(ctx context.Context, activity *activitypub.Activity) error {
	if a.federation == nil {
		a.logger.Warn("federation service not available, skipping activity delivery")
		return nil
	}

	// Extract actor from activity if needed
	var actor *activitypub.Actor
	if activity.Actor != "" {
		// Try to fetch the actor from storage
		if a.storage != nil {
			// Extract username from actor URI
			username := a.extractUsernameFromActorURI(activity.Actor)
			if username != "" {
				if actorRepo := a.storage.Actor(); actorRepo != nil {
					storedActor, err := actorRepo.GetActor(ctx, username)
					if err == nil && storedActor != nil {
						actor = storedActor
					} else {
						a.logger.Debug("failed to fetch actor from storage, using minimal representation",
							zap.String("username", username),
							zap.String("actor_uri", activity.Actor),
							zap.Error(err))
					}
				}
			}
		}

		// Fall back to minimal actor representation if storage fetch failed
		if actor == nil {
			username := a.extractUsernameFromActorURI(activity.Actor)
			actor = &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   activity.Actor,
					Type: activitypub.PersonType,
				},
				PreferredUsername: username,
				PublicKey: &activitypub.PublicKey{
					ID:    strings.TrimSuffix(activity.Actor, "/") + "#main-key",
					Owner: activity.Actor,
				},
			}
		}
	}

	if actor == nil {
		a.logger.Warn("activity missing actor, skipping federation delivery",
			zap.String("activity_id", activity.ID),
			zap.String("activity_type", activity.Type))
		return nil
	}

	if a.isPublicOrUnlisted(activity) {
		if combined, ok := a.federation.(followerRecipientFederationService); ok {
			if err := combined.DeliverToFollowersAndRecipients(ctx, activity, actor); err != nil {
				a.logger.Error("failed to deliver activity to followers and explicit recipients",
					zap.String("activity_id", activity.ID),
					zap.String("activity_type", activity.Type),
					zap.Error(err))
				return err
			}
			return nil
		}

		if err := a.federation.DeliverToFollowers(ctx, activity, actor); err != nil {
			a.logger.Error("failed to deliver activity to followers",
				zap.String("activity_id", activity.ID),
				zap.String("activity_type", activity.Type),
				zap.Error(err))
		}
	}

	if err := a.federation.DeliverToRecipients(ctx, activity, actor); err != nil {
		a.logger.Error("failed to deliver activity to explicit recipients",
			zap.String("activity_id", activity.ID),
			zap.String("activity_type", activity.Type),
			zap.Error(err))
		return err
	}

	return nil
}

// ResolveActor resolves a remote handle through the shared federation discovery path.
func (a *queueFederationAdapter) ResolveActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	if a.storage == nil {
		return nil, errors.New("remote actor resolution unavailable")
	}

	result, err := federation.NewRemoteSearchService(a.storage).ResolveActor(ctx, handle)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Actor == nil {
		return nil, errors.New("remote actor not found")
	}

	return result.Actor, nil
}

func (a *queueFederationAdapter) isPublicOrUnlisted(activity *activitypub.Activity) bool {
	if activity == nil {
		return false
	}

	for _, recipient := range activity.To {
		if recipient == activitypub.PublicAddress {
			return true
		}
	}

	for _, recipient := range activity.CC {
		if recipient == activitypub.PublicAddress {
			return true
		}
	}

	return false
}

// extractUsernameFromActorURI extracts the username from an actor URI
func (a *queueFederationAdapter) extractUsernameFromActorURI(actorURI string) string {
	// Handle different URI patterns:
	// - https://domain.com/users/username
	// - https://domain.com/@username
	// - https://domain.com/actor/username

	parts := strings.Split(actorURI, "/")
	if len(parts) < 2 {
		return ""
	}

	// Get the last part which should be the username
	username := parts[len(parts)-1]

	// Remove @ prefix if present
	username = strings.TrimPrefix(username, "@")

	// Basic validation - username should not be empty and should be reasonable length
	if username == "" || len(username) > 100 {
		return ""
	}

	return username
}

// simpleFederationService provides a basic federation service implementation for conversations
type simpleFederationService struct {
	logger *zap.Logger
}

// QueueActivity queues an activity for federation delivery
func (s *simpleFederationService) QueueActivity(_ context.Context, activity *activitypub.Activity) error {
	// For now, just log the activity - in a full implementation this would queue for delivery
	if s.logger != nil {
		s.logger.Info("federation activity queued",
			zap.String("type", activity.Type),
			zap.String("actor", activity.Actor),
			zap.String("object", fmt.Sprintf("%v", activity.Object)))
	}
	return nil
}

// getJobQueue returns the job queue service, creating it if necessary
func (r *Registry) getJobQueue() JobQueueServiceInterface {
	// Try to create a real SQS-based job queue service
	if r.config != nil && r.config.Config != nil {
		if jobQueue, err := NewJobQueueService(r.config.Config, r.logger); err == nil {
			return jobQueue
		}
	}

	// Fall back to simple job queue if SQS is not available
	return &simpleJobQueue{logger: r.logger}
}

// validateJWTSecret validates that the JWT secret meets security requirements
func validateJWTSecret(secret string) error {
	// Check minimum length (32 characters for 256-bit security)
	if len(secret) < 32 {
		return errors.New("JWT_SECRET must be at least 32 characters long")
	}

	// Check for common weak patterns
	lowerSecret := strings.ToLower(secret)
	weakPatterns := []string{
		"default",
		"change",
		"secret",
		"password",
		"12345",
		"admin",
		"test",
		"demo",
		"example",
	}

	for _, pattern := range weakPatterns {
		if strings.Contains(lowerSecret, pattern) {
			return fmt.Errorf("JWT_SECRET contains weak pattern '%s' - please use a strong, random secret", pattern)
		}
	}

	// Check for insufficient entropy (all same character, sequential, etc.)
	if isLowEntropy(secret) {
		return errors.New("JWT_SECRET has insufficient entropy - please use a random secret")
	}

	return nil
}

// isLowEntropy checks if a string has low entropy (e.g., all same character, sequential)
func isLowEntropy(s string) bool {
	if len(s) == 0 {
		return true
	}

	// Check if all characters are the same
	firstChar := s[0]
	allSame := true
	for i := 1; i < len(s); i++ {
		if s[i] != firstChar {
			allSame = false
			break
		}
	}
	if allSame {
		return true
	}

	// Check for sequential patterns
	sequential := true
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1]+1 {
			sequential = false
			break
		}
	}

	return sequential
}
