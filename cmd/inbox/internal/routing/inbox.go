// Package routing implements the inbox Lambda function for receiving ActivityPub federation messages.
package routing

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	inboxvalidation "github.com/equaltoai/lesser/cmd/inbox/internal/validation"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	costpkg "github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/federation/surface"
	"github.com/equaltoai/lesser/pkg/monitoring"
	notifpush "github.com/equaltoai/lesser/pkg/notifications"
	"github.com/equaltoai/lesser/pkg/observability"
	notifsvc "github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/google/uuid"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

var (
	runAsync = func(fn func()) { go fn() }

	getDynamormClient     = theorydb.GetClient
	newRepositoryFactory  = factory.NewRepositoryFactory
	getAuthMiddleware     = auth.GetMiddleware
	startLambda           = lambda.Start
	mustInitializeLambda  = common.MustInitializeLambda
	initializeWithOptions = func(lambdaCtx *common.LambdaContext, options common.LambdaInitOptions) error {
		return lambdaCtx.InitializeWithOptions(options)
	}
	initializeLambdaCtxFn = func(lambdaConfig common.LambdaConfig) *common.LambdaContext {
		lambdaCtx := mustInitializeLambda(lambdaConfig)

		options := common.DefaultLambdaInitOptions(common.LambdaTypeFederation)
		if err := initializeWithOptions(lambdaCtx, options); err != nil {
			// Fallback to manual initialization if needed
			lambdaCtx.Logger.Warn("falling back to manual service initialization", zap.Error(err))
		}

		return lambdaCtx
	}
)

// InboxHandler handles ActivityPub inbox requests using AppTheory.
type InboxHandler struct {
	db                           dynamormCore.DB
	actorRepository              interfaces.ActorRepository
	activityRepository           interfaces.ActivityRepository
	relationshipRepository       interfaces.ConcreteRelationshipRepository
	objectRepository             interfaces.ObjectRepository
	statusRepository             interfaces.StatusRepository
	conversationRepository       interfaces.ConversationRepository
	likeRepository               *repositories.LikeRepository
	socialRepository             *repositories.SocialRepository
	federationActivityRepository *repositories.FederationActivityRepository
	federationCostRepository     *repositories.FederationCostRepository
	domainBlockRepository        *repositories.DomainBlockRepository
	userRepository               interfaces.UserRepository
	instanceRepository           *repositories.InstanceRepository
	publicKeyCacheRepository     *repositories.PublicKeyCacheRepository
	notificationRepository       interfaces.NotificationRepository
	inboxProcessingRepository    inboxProcessingRecorder
	signatureService             *federation.SignatureService
	logger                       *zap.Logger
	authMiddleware               *auth.Middleware
	rateLimiter                  *auth.RateLimiter
	costCalculator               *federation.CostCalculator
	centralizedCostService       *costpkg.TrackingService
	deliveryService              *federation.DeliveryService
	remoteActorResolver          inboxRemoteActorResolver
	activityDeliverer            inboxActivityDeliverer
	publisher                    streaming.Publisher
	tableName                    string
	storageAdapter               storageCore.RepositoryStorage
	baseURL                      string
	emfMetrics                   *observability.EMFMetrics
	alertManager                 *monitoring.AlertManager
	startTime                    time.Time
}

type inboxRemoteActorResolver interface {
	ResolveDeliverableActor(ctx context.Context, input, localDomain string) (*federation.ExactActorResolution, error)
}

type inboxActivityDeliverer interface {
	DeliverActivity(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error
}

type inboxProcessingRecorder interface {
	TryRecordTarget(ctx context.Context, activityID, targetActorID, activityType string) (bool, error)
	ForgetTarget(ctx context.Context, activityID, targetActorID string) error
}

// extractedServices holds services extracted from lambda context
type extractedServices struct {
	repoFactory      storageCore.RepositoryStorage
	db               interface{}
	signatureService *federation.SignatureService
	deliveryService  *federation.DeliveryService
	costCalculator   *federation.CostCalculator
	rateLimiter      *auth.RateLimiter
	authMiddleware   *auth.Middleware
	emfMetrics       *observability.EMFMetrics
	alertManager     *monitoring.AlertManager
	streamQueue      streaming.StreamQueueService
}

// federationServices holds federation-related services
type federationServices struct {
	signatureService *federation.SignatureService
	deliveryService  *federation.DeliveryService
	costCalculator   *federation.CostCalculator
	rateLimiter      *auth.RateLimiter
	authMiddleware   *auth.Middleware
}

// observabilityServices holds observability-related services
type observabilityServices struct {
	emfMetrics             *observability.EMFMetrics
	alertManager           *monitoring.AlertManager
	centralizedCostService *costpkg.TrackingService
}

// repositoryCollection holds all repository instances
type repositoryCollection struct {
	actorRepo              interfaces.ActorRepository
	activityRepo           interfaces.ActivityRepository
	followRepo             interfaces.ConcreteRelationshipRepository
	objectRepo             interfaces.ObjectRepository
	statusRepo             interfaces.StatusRepository
	conversationRepo       interfaces.ConversationRepository
	likeRepo               *repositories.LikeRepository
	socialRepo             *repositories.SocialRepository
	federationActivityRepo *repositories.FederationActivityRepository
	federationCostRepo     *repositories.FederationCostRepository
	domainBlockRepo        *repositories.DomainBlockRepository
	userRepo               interfaces.UserRepository
	instanceRepo           *repositories.InstanceRepository
	publicKeyCacheRepo     *repositories.PublicKeyCacheRepository
	notificationRepo       interfaces.NotificationRepository
	inboxProcessingRepo    inboxProcessingRecorder
}

// extractServicesFromContext extracts services from lambda context
func extractServicesFromContext(lambdaCtx *common.LambdaContext) extractedServices {
	var services extractedServices

	if lambdaCtx.Repos != nil {
		services.repoFactory = lambdaCtx.Repos.(storageCore.RepositoryStorage)
	}
	if lambdaCtx.DynamoDB != nil {
		services.db = lambdaCtx.DynamoDB
	}
	if lambdaCtx.SignatureService != nil {
		services.signatureService = lambdaCtx.SignatureService.(*federation.SignatureService)
	}
	if lambdaCtx.DeliveryService != nil {
		services.deliveryService = lambdaCtx.DeliveryService.(*federation.DeliveryService)
	}
	if lambdaCtx.CostCalculator != nil {
		services.costCalculator = lambdaCtx.CostCalculator.(*federation.CostCalculator)
	}
	if lambdaCtx.RateLimiter != nil {
		services.rateLimiter = lambdaCtx.RateLimiter.(*auth.RateLimiter)
	}
	if lambdaCtx.AuthMiddleware != nil {
		services.authMiddleware = lambdaCtx.AuthMiddleware.(*auth.Middleware)
	}
	if lambdaCtx.EMFMetrics != nil {
		services.emfMetrics = lambdaCtx.EMFMetrics.(*observability.EMFMetrics)
	}
	if lambdaCtx.AlertManager != nil {
		services.alertManager = lambdaCtx.AlertManager.(*monitoring.AlertManager)
	}
	if lambdaCtx.StreamQueue != nil {
		if streamQueue, ok := lambdaCtx.StreamQueue.(streaming.StreamQueueService); ok {
			services.streamQueue = streamQueue
		}
	}

	return services
}

// initializeStorage initializes storage components if needed
func initializeStorage(repoFactory storageCore.RepositoryStorage, db interface{}, cfg *config.Config, _ *common.LambdaContext, logger *zap.Logger) (storageCore.RepositoryStorage, dynamormCore.DB, error) {
	if repoFactory != nil && db != nil {
		coreDB, ok := db.(dynamormCore.DB)
		if !ok {
			return nil, nil, dynamoDBInterfaceError()
		}
		return repoFactory, coreDB, nil
	}

	logger.Info("falling back to manual storage initialization")

	// Initialize storage manually
	manualDB, err := getDynamormClient(context.Background())
	if err != nil {
		return nil, nil, dynamORMInitError()
	}

	// Initialize repository factory
	manualRepoFactory, err := newRepositoryFactory(manualDB, cfg.DynamoTableName, logger)
	if err != nil {
		return nil, nil, repositoryFactoryInitError()
	}

	return manualRepoFactory, manualDB, nil
}

// initializeFederationServices initializes federation-related services
func initializeFederationServices(services extractedServices, repoFactory storageCore.RepositoryStorage, coreDB dynamormCore.DB, cfg *config.Config, logger *zap.Logger) federationServices {
	fed := federationServices{
		signatureService: services.signatureService,
		deliveryService:  services.deliveryService,
		costCalculator:   services.costCalculator,
		rateLimiter:      services.rateLimiter,
		authMiddleware:   services.authMiddleware,
	}

	// Initialize missing federation services
	if fed.signatureService == nil {
		fed.signatureService = createSignatureService(repoFactory, coreDB, cfg, logger)
	}
	if fed.costCalculator == nil {
		fed.costCalculator = federation.NewCostCalculator()
	}
	if fed.authMiddleware == nil {
		middleware, err := getAuthMiddleware()
		if err != nil {
			logger.Error("failed to initialize auth middleware", zap.Error(err))
		} else {
			fed.authMiddleware = middleware
		}
	}
	if fed.rateLimiter == nil {
		fed.rateLimiter = auth.NewRateLimiter(repoFactory)
	}
	if fed.deliveryService == nil {
		fed.deliveryService = federation.NewDeliveryService(
			federation.NewRepositoryStorageAdapter(repoFactory),
			cfg,
		)
	}

	return fed
}

// createSignatureService creates a signature service with public key cache repository
func createSignatureService(repoFactory storageCore.RepositoryStorage, coreDB dynamormCore.DB, cfg *config.Config, logger *zap.Logger) *federation.SignatureService {
	var publicKeyCacheRepo *repositories.PublicKeyCacheRepository
	if factory, ok := repoFactory.(*factory.RepositoryFactory); ok {
		publicKeyCacheRepo = factory.PublicKeyCache()
	} else {
		// Fallback: create repository directly
		publicKeyCacheRepo = repositories.NewPublicKeyCacheRepository(coreDB, cfg.DynamoTableName, logger, nil)
	}
	return federation.NewSignatureService(publicKeyCacheRepo, logger)
}

// initializeObservabilityServices initializes observability-related services
func initializeObservabilityServices(services extractedServices, lambdaCtx *common.LambdaContext, cfg *config.Config, logger *zap.Logger) observabilityServices {
	obs := observabilityServices{
		emfMetrics:   services.emfMetrics,
		alertManager: services.alertManager,
	}

	metricsDisabled := lambdaCtx.Config.DisableAWSModeration // Use config from lambda context

	// Initialize missing observability services
	if obs.emfMetrics == nil && !metricsDisabled {
		obs.emfMetrics = observability.NewEMFMetrics(logger, "Lesser/Federation", "inbox")
		obs.emfMetrics.AddDimension(observability.DimensionService, "inbox")
		obs.emfMetrics.AddDimension(observability.DimensionEnvironment, cfg.Stage)
		obs.emfMetrics.AddDimension(observability.DimensionRegion, cfg.Region)
	}
	if obs.alertManager == nil && !metricsDisabled {
		obs.alertManager = monitoring.NewAlertManagerWithConfig(&monitoring.AlertManagerConfig{
			Logger:  logger,
			Enabled: true,
		})
	}

	// Initialize centralized cost tracking service
	if !metricsDisabled && lambdaCtx.AWSServices != nil && lambdaCtx.AWSServices.CloudWatch != nil {
		obs.centralizedCostService = costpkg.NewCostTrackingServiceForLambda(lambdaCtx.AWSServices.CloudWatch, logger, "inbox")
		logger.Info("initialized centralized cost tracking service for inbox")
	}

	return obs
}

// initializeRepositories initializes all repository instances
func initializeRepositories(repoFactory storageCore.RepositoryStorage, coreDB dynamormCore.DB, cfg *config.Config, logger *zap.Logger) repositoryCollection {
	repos := repositoryCollection{
		actorRepo:        repoFactory.Actor(),
		activityRepo:     repoFactory.Activity(),
		followRepo:       repoFactory.Relationship(),
		objectRepo:       repoFactory.Object(),
		statusRepo:       repoFactory.Status(),
		conversationRepo: repoFactory.Conversation(),
		likeRepo:         repoFactory.Like(),
		domainBlockRepo:  repoFactory.DomainBlock(),
		userRepo:         repoFactory.User(),
		instanceRepo:     repoFactory.Instance(),
		notificationRepo: repoFactory.Notification(),
	}

	// Get PublicKeyCache repository directly from factory
	if factory, ok := repoFactory.(*factory.RepositoryFactory); ok {
		repos.publicKeyCacheRepo = factory.PublicKeyCache()
	} else {
		// Fallback: create repository directly
		repos.publicKeyCacheRepo = repositories.NewPublicKeyCacheRepository(coreDB, cfg.DynamoTableName, logger, nil)
	}

	// Initialize legacy repositories that don't use factory pattern yet
	repos.socialRepo = repositories.NewSocialRepository(coreDB, cfg.DynamoTableName, logger, nil)
	repos.federationActivityRepo = repositories.NewFederationActivityRepository(coreDB, cfg.DynamoTableName, logger, nil)
	repos.inboxProcessingRepo = repositories.NewInboxProcessingRepository(coreDB, cfg.DynamoTableName, logger, nil)
	costTrackingBaseRepo := repositories.NewBaseRepository[*models.FederationCostTracking](coreDB, cfg.DynamoTableName, logger)
	budgetBaseRepo := repositories.NewBaseRepository[*models.FederationBudget](coreDB, cfg.DynamoTableName, logger)
	repos.federationCostRepo = repositories.NewFederationCostRepositoryFromBase(costTrackingBaseRepo, budgetBaseRepo, nil)

	var accountRepo *repositories.AccountRepository
	if accessor, ok := repoFactory.(interface {
		Account() *repositories.AccountRepository
	}); ok {
		accountRepo = accessor.Account()
	}
	if accountRepo == nil {
		accountRepo = repositories.NewAccountRepository(coreDB, cfg.DynamoTableName, cfg.Domain, logger)
	}

	var pushService *notifpush.PushService
	if svc, err := notifpush.NewPushService(cfg); err != nil {
		logger.Warn("inbox handler: failed to initialize push service", zap.Error(err))
	} else {
		pushService = svc
	}

	if repos.notificationRepo != nil {
		notificationService := notifsvc.NewService(
			repos.notificationRepo,
			accountRepo,
			nil,
			logger,
			cfg.Domain,
			pushService,
		)
		repos.notificationRepo.SetDispatcher(notificationService)
	}

	return repos
}

func inboxPublisherFromStreamQueue(streamQueue streaming.StreamQueueService, logger *zap.Logger) streaming.Publisher {
	if streamQueue == nil {
		return streaming.NewNoopPublisher()
	}
	return streaming.NewQueuePublisher(streamQueue, logger)
}

// NewInboxHandler creates a new inbox handler using standardized initialization
func NewInboxHandler(lambdaCtx *common.LambdaContext) (*InboxHandler, error) {
	logger := lambdaCtx.Logger
	cfg := lambdaCtx.Config

	// Extract services from lambda context
	services := extractServicesFromContext(lambdaCtx)

	// Initialize storage if needed
	repoFactory, coreDB, err := initializeStorage(services.repoFactory, services.db, cfg, lambdaCtx, logger)
	if err != nil {
		return nil, err
	}

	// Initialize federation services
	federationServices := initializeFederationServices(services, repoFactory, coreDB, cfg, logger)

	// Initialize observability services
	observabilityServices := initializeObservabilityServices(services, lambdaCtx, cfg, logger)

	// Initialize repositories
	repositories := initializeRepositories(repoFactory, coreDB, cfg, logger)

	logger.Info("initialized inbox handler with standardized services")

	return &InboxHandler{
		db:                           coreDB,
		actorRepository:              repositories.actorRepo,
		activityRepository:           repositories.activityRepo,
		relationshipRepository:       repositories.followRepo,
		objectRepository:             repositories.objectRepo,
		statusRepository:             repositories.statusRepo,
		conversationRepository:       repositories.conversationRepo,
		likeRepository:               repositories.likeRepo,
		socialRepository:             repositories.socialRepo,
		federationActivityRepository: repositories.federationActivityRepo,
		federationCostRepository:     repositories.federationCostRepo,
		domainBlockRepository:        repositories.domainBlockRepo,
		userRepository:               repositories.userRepo,
		instanceRepository:           repositories.instanceRepo,
		publicKeyCacheRepository:     repositories.publicKeyCacheRepo,
		notificationRepository:       repositories.notificationRepo,
		inboxProcessingRepository:    repositories.inboxProcessingRepo,
		signatureService:             federationServices.signatureService,
		logger:                       logger,
		authMiddleware:               federationServices.authMiddleware,
		rateLimiter:                  federationServices.rateLimiter,
		costCalculator:               federationServices.costCalculator,
		centralizedCostService:       observabilityServices.centralizedCostService,
		deliveryService:              federationServices.deliveryService,
		remoteActorResolver:          federation.NewRemoteSearchService(repoFactory),
		activityDeliverer:            federationServices.deliveryService,
		publisher:                    inboxPublisherFromStreamQueue(services.streamQueue, logger),
		tableName:                    cfg.DynamoTableName,
		storageAdapter:               repoFactory,
		baseURL:                      cfg.BaseURL(),
		emfMetrics:                   observabilityServices.emfMetrics,
		alertManager:                 observabilityServices.alertManager,
		startTime:                    time.Now(),
	}, nil
}

// RegisterRoutes registers all inbox routes
func (ih *InboxHandler) RegisterRoutes(app *apptheory.App) {
	// ActivityPub inbox endpoints
	sharedInbox := surface.SharedInbox()
	for _, method := range sharedInbox.ServedMethods() {
		switch method {
		case http.MethodGet:
			app.Get(sharedInbox.Path, ih.handleGetSharedInbox)
		case http.MethodPost:
			app.Post(sharedInbox.Path, ih.handlePostSharedInbox)
		default:
			panic(fmt.Sprintf("unsupported shared inbox method in federation surface manifest: %s", method))
		}
	}
	app.Get("/users/:username/inbox", ih.handleGetInbox)
	app.Post("/users/:username/inbox", ih.handlePostInbox)
}

// handleGetInbox handles GET requests to retrieve inbox activities
func (ih *InboxHandler) handleGetInbox(ctx *apptheory.Context) (*apptheory.Response, error) {
	username := ctx.Param("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, errors.ValidationFailed("username", "missing username parameter")
	}

	ih.logger.Info("received inbox GET request",
		zap.String("username", username),
		zap.String("user_agent", headerValue(ctx, "User-Agent")),
		zap.String("request_id", ctx.RequestID))

	// Authenticate and authorize the request
	actor, err := ih.authenticateInboxRequest(ctx, username)
	if err != nil {
		return nil, err
	}

	// Parse pagination parameters
	limit, cursor, page := ih.parsePaginationParams(ctx)

	// Handle collection metadata request (no page parameter)
	if common.ValidateRequiredParam("page", page) != nil && common.ValidateRequiredParam("cursor", cursor) != nil {
		return ih.returnInboxCollection(ctx, actor, username)
	}

	// Get and process activities for the requested page
	return ih.returnInboxPage(ctx, actor, username, limit, cursor)
}

// authenticateInboxRequest handles authentication and authorization for inbox GET requests
func (ih *InboxHandler) authenticateInboxRequest(ctx *apptheory.Context, username string) (*activitypub.Actor, error) {
	// Validate authentication header
	authHeader := headerValue(ctx, "Authorization")
	if err := common.ValidateRequiredParam("authorization", authHeader); err != nil {
		return nil, errors.Unauthorized("authentication required")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, errors.Unauthorized("invalid authorization header format")
	}

	// Validate JWT token
	claims, err := ih.authMiddleware.ValidateToken(authHeader)
	if err != nil {
		ih.logger.Warn("JWT validation failed",
			zap.Error(err),
			zap.String("username", username),
			zap.String("user_agent", headerValue(ctx, "User-Agent")))

		if err == auth.ErrMissingAuthHeader || err == auth.ErrInvalidToken {
			return nil, errors.Unauthorized("invalid or expired token").WithInternalError(err)
		}
		return nil, errors.Unauthorized("authentication failed").WithInternalError(err)
	}

	// Verify user authorization
	if claims.Username != username {
		ih.logger.Warn("user mismatch in inbox request",
			zap.String("auth_username", claims.Username),
			zap.String("requested_user", username))
		return nil, errors.Forbidden("cannot access another user's inbox")
	}

	// Get and validate actor
	actor, err := ih.actorRepository.GetActorByUsername(ctx.Context(), username)
	if err != nil {
		if err.Error() == actorNotFoundError {
			return nil, errors.NotFound("actor")
		}
		ih.logger.Error("failed to get actor", zap.Error(err))
		return nil, errors.InternalWithCause(err, "internal server error")
	}

	return actor, nil
}

// parsePaginationParams extracts and validates pagination parameters from the request
func (ih *InboxHandler) parsePaginationParams(ctx *apptheory.Context) (int, string, string) {
	limitStr := queryValue(ctx, "limit")
	limit, err := common.ParseAndValidateActivityPubLimit(limitStr)
	if err != nil {
		limit = 20
	}

	cursor := queryValue(ctx, "cursor")
	page := queryValue(ctx, "page")

	return limit, cursor, page
}

// returnInboxCollection returns the inbox collection metadata
func (ih *InboxHandler) returnInboxCollection(ctx *apptheory.Context, actor *activitypub.Actor, username string) (*apptheory.Response, error) {
	// Get first page to calculate total items (simplified approach)
	activities, _, err := ih.activityRepository.GetInboxActivities(ctx.Context(), username, 1, "")
	if err != nil {
		ih.logger.Error("failed to get inbox count", zap.Error(err))
		return nil, errors.InternalWithCause(err, "internal server error")
	}

	collection := &activitypub.OrderedCollection{
		Collection: activitypub.Collection{
			BaseObject: activitypub.BaseObject{
				Context: activitypub.Context,
				ID:      actor.Inbox,
				Type:    activitypub.OrderedCollectionType,
			},
			TotalItems: len(activities),
			First:      fmt.Sprintf("%s?page=true", actor.Inbox),
		},
	}

	resp, err := apptheory.JSON(http.StatusOK, collection)
	if resp != nil {
		if resp.Headers == nil {
			resp.Headers = map[string][]string{}
		}
		resp.Headers["content-type"] = []string{"application/activity+json"}
	}
	return resp, err
}

// returnInboxPage returns a paginated collection of inbox activities
func (ih *InboxHandler) returnInboxPage(ctx *apptheory.Context, actor *activitypub.Actor, username string, limit int, cursor string) (*apptheory.Response, error) {
	// Get activities for the page
	activities, nextCursor, err := ih.activityRepository.GetInboxActivities(ctx.Context(), username, limit, cursor)
	if err != nil {
		ih.logger.Error("failed to get inbox activities", zap.Error(err))
		return nil, errors.InternalWithCause(err, "internal server error")
	}

	// Enrich activities with full object data
	ih.enrichActivitiesWithObjects(ctx, activities)

	// Build collection page
	collectionPage := ih.buildCollectionPage(actor, activities, cursor, nextCursor, limit)

	resp, err := apptheory.JSON(http.StatusOK, collectionPage)
	if resp != nil {
		if resp.Headers == nil {
			resp.Headers = map[string][]string{}
		}
		resp.Headers["content-type"] = []string{"application/activity+json"}
	}
	return resp, err
}

// enrichActivitiesWithObjects enriches Create activities with full object data
func (ih *InboxHandler) enrichActivitiesWithObjects(ctx *apptheory.Context, activities []*activitypub.Activity) {
	for _, activity := range activities {
		if activity.Type != activitypub.CreateType || activity.Object == nil {
			continue
		}

		objID, ok := activity.Object.(string)
		if !ok {
			continue
		}

		obj, err := ih.objectRepository.GetObject(ctx.Context(), objID)
		if err != nil {
			ih.logger.Warn("failed to fetch object for activity",
				zap.String("activity_id", activity.ID),
				zap.String("object_id", objID),
				zap.Error(err))
			continue
		}
		activity.Object = obj
	}
}

// buildCollectionPage constructs an ActivityPub OrderedCollectionPage
func (ih *InboxHandler) buildCollectionPage(actor *activitypub.Actor, activities []*activitypub.Activity, cursor, nextCursor string, limit int) *activitypub.OrderedCollectionPage {
	// Convert activities to ordered items
	orderedItems := make([]any, len(activities))
	for i, activity := range activities {
		orderedItems[i] = activity
	}

	collectionPage := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      fmt.Sprintf("%s?page=true", actor.Inbox),
					Type:    "OrderedCollectionPage",
				},
				OrderedItems: orderedItems,
			},
			PartOf: actor.Inbox,
		},
	}

	// Add pagination links
	if nextCursor != "" {
		collectionPage.Next = fmt.Sprintf("%s?page=true&cursor=%s&limit=%d", actor.Inbox, nextCursor, limit)
	}
	if cursor != "" {
		collectionPage.Prev = fmt.Sprintf("%s?page=true&limit=%d", actor.Inbox, limit)
	}

	return collectionPage
}

// InboxRequest represents an incoming ActivityPub request
type InboxRequest struct {
	Username     string
	Activity     *activitypub.Activity
	Actor        *activitypub.Actor
	TargetActors []*activitypub.Actor
	Body         []byte
	ActorDomain  string
	StartTime    time.Time
	CostParams   *federation.CostCalculationParams
}

// handlePostInbox handles POST requests to receive activities
func (ih *InboxHandler) handlePostInbox(ctx *apptheory.Context) (*apptheory.Response, error) {
	req, err := ih.initializeActorInboxRequest(ctx)
	if err != nil {
		return nil, err
	}

	if err := ih.performSecurityChecks(ctx, req); err != nil {
		return nil, err
	}
	if err := ih.verifyAuthentication(ctx, req); err != nil {
		return nil, err
	}
	if err := ih.verifyCreateAuthorization(req.Activity); err != nil {
		return nil, err
	}
	if err := ih.validateActorInboxAddressingAndPrivacy(req); err != nil {
		return nil, err
	}
	if err := ih.storeAndProcessActivity(ctx, req); err != nil {
		return nil, err
	}

	ih.recordSuccessAndComplete(ctx, req)
	return apptheory.Text(http.StatusAccepted, ""), nil
}

// verifyCreateAuthorization binds an embedded object's attribution to the
// Activity actor whose key authenticated the request. It must run only after
// verifyAuthentication so activity.Actor represents the verified signer.
func (ih *InboxHandler) verifyCreateAuthorization(activity *activitypub.Activity) error {
	if activity == nil || activity.Type != activitypub.CreateType {
		return nil
	}

	object, ok := activity.Object.(map[string]any)
	if !ok {
		return nil
	}
	attributedTo, ok := object["attributedTo"].(string)
	if !ok || strings.TrimSpace(attributedTo) == "" {
		return nil
	}
	if !common.SameCanonicalActorID(activity.Actor, attributedTo) {
		return errors.InsufficientPermissions("create object attribution")
	}

	return nil
}

// validateRequestBody validates the request body size and content
func (ih *InboxHandler) validateRequestBody(body []byte) error {
	return inboxvalidation.ValidateRequestBody(ih.logger, body)
}

// parseActivity parses and sanitizes the ActivityPub activity
func (ih *InboxHandler) parseActivity(body []byte) (*activitypub.Activity, error) {
	return inboxvalidation.ParseActivity(ih.logger, body)
}

func stringSliceToInterfaceSlice(values []string) []interface{} {
	result := make([]interface{}, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

// validateBasicActivity validates the basic activity structure
func (ih *InboxHandler) validateBasicActivity(activity *activitypub.Activity) error {
	return inboxvalidation.ValidateBasicActivity(activity)
}

// validateBasicActor validates the basic actor structure
func (ih *InboxHandler) validateBasicActor(actor *activitypub.Actor) error {
	return inboxvalidation.ValidateBasicActor(actor)
}

// validateActivityAddressing validates all addressing fields of the activity
func (ih *InboxHandler) validateActivityAddressing(activity *activitypub.Activity) error {
	return inboxvalidation.ValidateActivityAddressing(activity)
}

// validateActorUsername validates the actor's username format
func (ih *InboxHandler) validateActorUsername(actorURL string) error {
	return inboxvalidation.ValidateActorUsername(actorURL)
}

// validateActorPublicKey validates the actor's public key if present
func (ih *InboxHandler) validateActorPublicKey(actor *activitypub.Actor) error {
	return inboxvalidation.ValidateActorPublicKey(actor)
}

// validateCreateActivityObject validates objects within Create activities
func (ih *InboxHandler) validateCreateActivityObject(activity *activitypub.Activity) error {
	if activity.Type != "Create" {
		return nil
	}

	objMap, ok := activity.Object.(map[string]interface{})
	if !ok {
		return nil
	}

	if err := ih.validateObjectAttachments(objMap); err != nil {
		return err
	}

	if err := ih.validateObjectTags(objMap); err != nil {
		return err
	}

	if err := ih.validateNoteObject(objMap); err != nil {
		return err
	}

	return nil
}

// validateObjectAttachments validates object attachments
func (ih *InboxHandler) validateObjectAttachments(objMap map[string]interface{}) error {
	return inboxvalidation.ValidateObjectAttachments(objMap)
}

// validateObjectTags validates object tags
func (ih *InboxHandler) validateObjectTags(objMap map[string]interface{}) error {
	return inboxvalidation.ValidateObjectTags(objMap)
}

// validateNoteObject validates Note object structure
func (ih *InboxHandler) validateNoteObject(objMap map[string]interface{}) error {
	return inboxvalidation.ValidateNoteObject(objMap)
}

// validateComprehensiveAddressing validates addressing using comprehensive validator
func (ih *InboxHandler) validateComprehensiveAddressing(activity *activitypub.Activity) error {
	return inboxvalidation.ValidateComprehensiveAddressing(ih.logger, activity)
}

// validateActivityTargeting checks if the activity is addressed to this actor
func (ih *InboxHandler) validateActivityTargeting(activity *activitypub.Activity, actor *activitypub.Actor) error {
	return inboxvalidation.ValidateActivityTargeting(ih.logger, activity, actor)
}

// performSecurityChecks handles rate limiting and domain blocking
func (ih *InboxHandler) performSecurityChecks(ctx *apptheory.Context, req *InboxRequest) error {
	if err := common.ValidateRequiredParam("actorDomain", req.ActorDomain); err != nil {
		return nil
	}

	// Check rate limiting
	if err := ih.checkRateLimit(ctx, req); err != nil {
		return err
	}

	// Check domain blocking
	if err := ih.checkDomainBlock(ctx, req); err != nil {
		return err
	}

	return nil
}

// checkRateLimit performs rate limiting checks
func (ih *InboxHandler) checkRateLimit(ctx *apptheory.Context, req *InboxRequest) error {
	if err := ih.rateLimiter.CheckRateLimit(ctx.Context(), req.ActorDomain, headerValue(ctx, "X-Forwarded-For")); err != nil {
		ih.logger.Warn("rate limit exceeded",
			zap.String("domain", req.ActorDomain),
			zap.Error(err))

		ih.recordFailureCost(req, "Rate limit exceeded", 2)
		return errors.RateLimitExceededGeneric("domain").WithInternalError(err)
	}

	// Record the rate limit attempt
	if err := ih.rateLimiter.RecordAttempt(ctx.Context(), req.ActorDomain, headerValue(ctx, "X-Forwarded-For"), false); err != nil {
		ih.logger.Warn("failed to record rate limit attempt", zap.Error(err))
	}

	return nil
}

// checkDomainBlock checks if the domain is blocked
func (ih *InboxHandler) checkDomainBlock(ctx *apptheory.Context, req *InboxRequest) error {
	isBlocked, block, err := ih.domainBlockRepository.IsDomainBlocked(ctx.Context(), req.ActorDomain)
	if err != nil {
		ih.logger.Error("failed to check domain block status",
			zap.String("domain", req.ActorDomain),
			zap.Error(err))
		return nil // Fail open rather than closed
	}

	if !isBlocked || block == nil {
		return nil
	}

	ih.logger.Info("rejecting activity from blocked domain",
		zap.String("domain", req.ActorDomain),
		zap.String("severity", block.Severity),
		zap.String("actor", req.Activity.Actor))

	// For suspended domains, reject completely
	if block.Severity == "suspend" {
		ih.recordFailureCost(req, "Domain is suspended", 3)
		return errors.Forbidden("domain is suspended")
	}

	// For silenced domains, we accept but may limit visibility
	return nil
}

// verifyAuthentication handles public key fetching and signature verification with enhanced security
func (ih *InboxHandler) verifyAuthentication(ctx *apptheory.Context, req *InboxRequest) error {
	start := time.Now()
	trace := ih.followTraceForRequest(req)

	ih.logFollowTraceRawRequest(ctx, trace, req.Username)

	// Convert the AppTheory request to http.Request for signature verification.
	httpReq, err := ih.convertRequest(ctx, req.Body)
	if err != nil {
		ih.logger.Error("failed to convert request for signature verification",
			zap.String("actor", req.Activity.Actor),
			zap.Error(err))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Request conversion failed: %v", err), 3)
		return errors.ValidationFailed("request", "malformed request").WithInternalError(err)
	}

	if trace != nil {
		httpReq = httpReq.WithContext(federation.WithFollowTrace(httpReq.Context(), trace))
		ih.logFollowTraceReconstructedRequest(trace, httpReq)
	}

	if err := ih.validateInboundSignatureHost(httpReq); err != nil {
		ih.logger.Warn("inbound signature host binding failed",
			zap.String("actor", req.Activity.Actor),
			zap.String("request_host", httpReq.Host),
			zap.String("url_host", httpReq.URL.Host),
			zap.Error(err))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Signature host binding failed: %v", err), 3)
		return errors.Unauthorized("signature host binding failed").WithInternalError(err)
	}

	if err := federation.RequireSignedBodyIntegrity(httpReq); err != nil {
		ih.logger.Warn("inbound signature integrity headers missing or unsigned",
			zap.String("actor", req.Activity.Actor),
			zap.Error(err))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Signature integrity headers failed: %v", err), 3)
		return errors.Unauthorized("signature integrity headers required").WithInternalError(err)
	}

	// Enhanced signature verification with caching and retry logic
	signatureVerifyStart := time.Now()
	if err := ih.signatureService.VerifySignature(httpReq.Context(), httpReq, req.Activity.Actor); err != nil {
		signatureVerifyDuration := time.Since(signatureVerifyStart)
		totalDuration := time.Since(start)

		// Map authentication errors to appropriate HTTP status codes
		switch err.(type) {
		case *errors.AppError:
			ih.logger.Warn("signature verification failed - authentication error",
				zap.String("actor", req.Activity.Actor),
				zap.Error(err),
				zap.Duration("duration", signatureVerifyDuration))

			req.CostParams.ProcessingTimeMs = totalDuration.Milliseconds()
			req.CostParams.SignatureVerificationMs = signatureVerifyDuration.Milliseconds()
			ih.recordFailureCost(req, fmt.Sprintf("Signature verification failed: %v", err), 3)
			return errors.Unauthorized("signature verification failed").WithInternalError(err)
		default:
			ih.logger.Error("signature verification error - service unavailable",
				zap.String("actor", req.Activity.Actor),
				zap.Error(err),
				zap.Duration("duration", signatureVerifyDuration))

			req.CostParams.ProcessingTimeMs = totalDuration.Milliseconds()
			req.CostParams.SignatureVerificationMs = signatureVerifyDuration.Milliseconds()
			// Estimate cost for key fetch attempts
			req.CostParams.HTTPRequestCount = 3 // Max retry attempts
			req.CostParams.DNSLookupCount = 1
			ih.recordFailureCost(req, fmt.Sprintf("Unable to verify signature: %v", err), 3)
			return errors.NewFederationInternalError(errors.CodeExternalServiceUnavailable, "unable to verify sender", err)
		}
	}

	signatureVerifyDuration := time.Since(signatureVerifyStart)
	req.CostParams.SignatureVerificationMs = signatureVerifyDuration.Milliseconds()
	// Estimate costs for successful verification (could be cache hit or fetch)
	req.CostParams.HTTPRequestCount = 1 // Conservative estimate
	req.CostParams.DNSLookupCount = 1

	ih.logger.Debug("signature verification successful",
		zap.String("actor", req.Activity.Actor),
		zap.Duration("duration", signatureVerifyDuration))

	// Enhanced digest verification with compatibility support
	if err := ih.verifyDigestEnhanced(ctx, req); err != nil {
		digestDuration := time.Since(start)
		req.CostParams.ProcessingTimeMs = digestDuration.Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Digest verification failed: %v", err), 1)
		return errors.ValidationFailed("digest", "digest verification failed").WithInternalError(err)
	}

	req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
	return nil
}

// validateDirectMessage performs additional validation for direct messages
func (ih *InboxHandler) validateDirectMessage(activity *activitypub.Activity, _ *activitypub.Actor) error {
	// Validate all addressing fields using ActivityPub validators
	if err := common.ValidateActivityPubAddressing(stringSliceToInterfaceSlice(activity.To), "to"); err != nil {
		return dmToAddressingError()
	}
	if err := common.ValidateActivityPubAddressing(stringSliceToInterfaceSlice(activity.CC), "cc"); err != nil {
		return dmCcAddressingError()
	}
	if err := common.ValidateActivityPubAddressing(stringSliceToInterfaceSlice(activity.BTo), "bto"); err != nil {
		return dmBtoAddressingError()
	}
	if err := common.ValidateActivityPubAddressing(stringSliceToInterfaceSlice(activity.BCC), "bcc"); err != nil {
		return dmBccAddressingError()
	}

	// Ensure direct messages don't leak to public timelines
	publicAddr := activitypub.PublicAddress

	// Check that public address is not in any addressing field
	allAddresses := append(append(append(activity.To, activity.CC...), activity.BTo...), activity.BCC...)
	for _, addr := range allAddresses {
		if addr == publicAddr {
			return dmPublicAddressError()
		}
	}

	// Ensure at least one specific recipient is mentioned
	hasSpecificRecipient := false
	for _, addr := range allAddresses {
		// Validate each recipient URL format
		if err := common.ValidateActivityPubURL(addr, "recipient"); err != nil {
			return dmRecipientURLError()
		}

		// Check if it's a specific actor (not a collection)
		if addr != publicAddr && !strings.Contains(addr, "/followers") && !strings.Contains(addr, "/following") {
			hasSpecificRecipient = true
		}
	}

	if !hasSpecificRecipient {
		return dmNoRecipientsError()
	}

	return nil
}

// verifyDigestEnhanced verifies the digest header with enhanced compatibility support
func (ih *InboxHandler) verifyDigestEnhanced(ctx *apptheory.Context, req *InboxRequest) error {
	digestHeader := headerValue(ctx, "Digest")
	trace := ih.followTraceForRequest(req)
	if err := common.ValidateRequiredParam("digestHeader", digestHeader); err != nil {
		ih.logFollowTrace(trace, "receiver.digest.missing")
		ih.logger.Warn("missing digest header", zap.String("actor", req.Activity.Actor))
		return common.AuthenticationError{Message: "missing digest header"}
	}

	ih.logFollowTrace(trace, "receiver.digest.start",
		zap.String("digest_header", digestHeader),
	)

	httpReq, err := ih.convertRequest(ctx, req.Body)
	if err != nil {
		ih.logger.Warn("failed to convert request for digest verification",
			zap.String("actor", req.Activity.Actor),
			zap.Error(err))
		// Don't fail if we can't convert the request for digest verification
		return nil
	}

	// Use the enhanced digest verification with compatibility support
	if err := ih.signatureService.VerifyDigestWithCompatibility(httpReq, req.Body); err != nil {
		ih.logFollowTrace(trace, "receiver.digest.failure",
			zap.String("digest_header", digestHeader),
			zap.String("digest_error", err.Error()),
		)
		ih.logger.Warn("digest verification failed",
			zap.String("actor", req.Activity.Actor),
			zap.String("digest_header", digestHeader),
			zap.Error(err))
		return err // Return the specific error from the service
	}

	ih.logFollowTrace(trace, "receiver.digest.success",
		zap.String("digest_header", digestHeader),
	)
	ih.logger.Debug("digest verification successful",
		zap.String("actor", req.Activity.Actor))
	return nil
}

// storeAndProcessActivity stores the activity and processes it based on type
func (ih *InboxHandler) storeAndProcessActivity(ctx *apptheory.Context, req *InboxRequest) error {
	targetActors := req.TargetActors
	if len(targetActors) == 0 && req.Actor != nil {
		targetActors = []*activitypub.Actor{req.Actor}
	}
	if len(targetActors) == 0 {
		ih.recordFailureCost(req, "No local target actors resolved", 1)
		return errors.NotFound("resource")
	}

	processingStart := time.Now()
	targetsToProcess := make([]*activitypub.Actor, 0, len(targetActors))
	for _, targetActor := range targetActors {
		if targetActor == nil {
			continue
		}
		shouldProcess, err := ih.shouldProcessInboundActivityTarget(ctx.Context(), req.Activity, targetActor)
		if err != nil {
			for _, claimedTargetActor := range targetsToProcess {
				ih.releaseInboundActivityTargetClaim(ctx.Context(), req.Activity, claimedTargetActor)
			}
			processingDuration := time.Since(processingStart)
			req.CostParams.ProcessingTimeMs += processingDuration.Milliseconds()
			ih.recordFailureCost(req, fmt.Sprintf("Failed to claim %s activity for idempotent processing: %v", req.Activity.Type, err), 0)
			return errors.InternalWithCause(err, fmt.Sprintf("failed to claim %s activity", req.Activity.Type))
		}
		if !shouldProcess {
			continue
		}
		targetsToProcess = append(targetsToProcess, targetActor)
	}

	if len(targetsToProcess) == 0 {
		processingDuration := time.Since(processingStart)
		req.CostParams.ProcessingTimeMs += processingDuration.Milliseconds()
		return nil
	}

	// Store the activity after target idempotency has confirmed there is at
	// least one local actor that still needs this activity processed. Duplicate
	// deliveries that race through shared-inbox and actor-inbox lanes therefore
	// do not create duplicate activity rows.
	if err := ih.activityRepository.CreateActivity(ctx.Context(), req.Activity); err != nil {
		for _, targetActor := range targetsToProcess {
			ih.releaseInboundActivityTargetClaim(ctx.Context(), req.Activity, targetActor)
		}
		ih.logger.Error("failed to store activity", zap.Error(err))
		ih.recordFailureCost(req, fmt.Sprintf("Failed to store activity: %v", err), 3)
		return errors.InternalWithCause(err, "failed to store activity")
	}

	req.CostParams.DynamoDBWriteCount = 1 // Activity storage

	for i, targetActor := range targetsToProcess {
		if err := ih.processActivityByTypeForTarget(ctx.Context(), req.Activity, targetActor, req.CostParams); err != nil {
			for _, unprocessedTargetActor := range targetsToProcess[i:] {
				ih.releaseInboundActivityTargetClaim(ctx.Context(), req.Activity, unprocessedTargetActor)
			}
			processingDuration := time.Since(processingStart)
			req.CostParams.ProcessingTimeMs += processingDuration.Milliseconds()
			ih.recordFailureCost(req, fmt.Sprintf("Failed to process %s activity: %v", req.Activity.Type, err), 0)
			return errors.InternalWithCause(err, fmt.Sprintf("failed to process %s activity", req.Activity.Type))
		}
	}

	processingDuration := time.Since(processingStart)
	req.CostParams.ProcessingTimeMs += processingDuration.Milliseconds()
	return nil
}

func (ih *InboxHandler) shouldProcessInboundActivityTarget(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) (bool, error) {
	if ih.inboxProcessingRepository == nil || activity == nil || targetActor == nil {
		return true, nil
	}

	activityID := strings.TrimSpace(activity.ID)
	targetActorID := strings.TrimSpace(targetActor.ID)
	if activityID == "" || targetActorID == "" {
		return true, nil
	}

	created, err := ih.inboxProcessingRepository.TryRecordTarget(ctx, activityID, targetActorID, activity.Type)
	if err != nil {
		ih.logger.Error("failed to record inbound activity processing receipt",
			zap.String("activity_id", activityID),
			zap.String("activity_type", activity.Type),
			zap.String("target_actor", targetActorID),
			zap.Error(err))
		return false, err
	}
	if !created {
		ih.logger.Info("duplicate inbound activity target processing skipped",
			zap.String("activity_id", activityID),
			zap.String("activity_type", activity.Type),
			zap.String("target_actor", targetActorID))
		return false, nil
	}

	return true, nil
}

func (ih *InboxHandler) releaseInboundActivityTargetClaim(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) {
	if ih.inboxProcessingRepository == nil || activity == nil || targetActor == nil {
		return
	}

	activityID := strings.TrimSpace(activity.ID)
	targetActorID := strings.TrimSpace(targetActor.ID)
	if activityID == "" || targetActorID == "" {
		return
	}

	if err := ih.inboxProcessingRepository.ForgetTarget(ctx, activityID, targetActorID); err != nil {
		ih.logger.Warn("failed to release inbound activity processing receipt after processing error",
			zap.String("activity_id", activityID),
			zap.String("activity_type", activity.Type),
			zap.String("target_actor", targetActorID),
			zap.Error(err))
	}
}

func (ih *InboxHandler) processActivityByTypeForTarget(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor, costParams *federation.CostCalculationParams) error {
	switch activity.Type {
	case activitypub.FollowType:
		if err := ih.processFollowActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process follow activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount++ // Relationship creation
		costParams.DynamoDBReadCount++  // Follow approval check

	case activitypub.AcceptType:
		if err := ih.processAcceptActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process accept activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount++ // Relationship update
		costParams.DynamoDBReadCount++  // Original activity lookup

	case activitypub.RejectType:
		if err := ih.processRejectActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process reject activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount++ // Relationship deletion
		costParams.DynamoDBReadCount++  // Original activity lookup

	case activitypub.CreateType:
		if err := ih.processRemoteCreateActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process create activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount += 2 // Object creation + timeline entry
		costParams.DynamoDBReadCount++     // Content validation

	case activitypub.UpdateType:
		if err := ih.processRemoteUpdateActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process update activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount++ // Object update
		costParams.DynamoDBReadCount++  // Object lookup

	case activitypub.DeleteType:
		if err := ih.processRemoteDeleteActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process delete activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount++ // Object deletion
		costParams.DynamoDBReadCount++  // Object lookup

	case activitypub.LikeType:
		if err := ih.processLikeActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process like activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount += 2 // Like creation + notification
		costParams.DynamoDBReadCount++     // Object verification

	case activitypub.AnnounceType:
		if err := ih.processAnnounceActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process announce activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount += 2 // Announce creation + notification
		costParams.DynamoDBReadCount++     // Object verification

	case activitypub.UndoType:
		if err := ih.processUndoActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process undo activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount++   // Undo operation
		costParams.DynamoDBReadCount += 2 // Original activity + target lookup

	case activitypub.BlockType:
		if err := ih.processBlockActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process block activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount += 2 // Block creation + remove follow relationships
		costParams.DynamoDBReadCount++     // Relationship check

	case activitypub.AddType:
		if err := ih.processAddActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process add activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount++   // Collection item creation
		costParams.DynamoDBReadCount += 2 // Target collection + authorization check

	case activitypub.RemoveType:
		if err := ih.processRemoveActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process remove activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount++   // Collection item removal
		costParams.DynamoDBReadCount += 2 // Target collection + authorization check

	case activitypub.FlagType:
		if err := ih.processFlagActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process flag activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount += 2 // Report creation + moderation queue
		costParams.DynamoDBReadCount++     // Authorization check

	case activitypub.MoveType:
		if err := ih.processMoveActivity(ctx, activity, targetActor); err != nil {
			ih.logger.Error("failed to process move activity", zap.Error(err))
			return err
		}
		costParams.DynamoDBWriteCount++   // Migration record
		costParams.DynamoDBReadCount += 2 // Actor validation + authorization check

	default:
		ih.logger.Info("ignoring unsupported activity type in inbox",
			zap.String("type", activity.Type),
			zap.String("actor", activity.Actor),
			zap.String("id", activity.ID),
		)
	}

	return nil
}

// recordSuccessAndComplete handles final success logging and cost tracking
func (ih *InboxHandler) recordSuccessAndComplete(ctx *apptheory.Context, req *InboxRequest) {
	ih.logger.Info("activity accepted and processed",
		zap.String("id", req.Activity.ID),
		zap.String("type", req.Activity.Type),
		zap.String("from", req.Activity.Actor))

	// Record successful cost tracking using centralized service
	req.CostParams.Success = true
	req.CostParams.ResponseTimeMs = time.Since(req.StartTime).Milliseconds()
	req.CostParams.LambdaDurationMs = time.Since(req.StartTime).Milliseconds()
	req.CostParams.DynamoDBWriteCount++ // Cost tracking record itself

	// Track with both federation-specific and centralized cost tracking
	runAsync(func() {
		// Legacy federation cost tracking
		cost := ih.costCalculator.CalculateFederationCosts(req.CostParams)
		if err := ih.federationCostRepository.RecordFederationCost(context.Background(), cost); err != nil {
			ih.logger.Warn("failed to record federation cost", zap.Error(err))
		}

		if err := ih.federationCostRepository.UpdateBudgetUsage(context.Background(),
			req.ActorDomain, "daily", req.Activity.Type, "inbound", cost.TotalCostMicroCents); err != nil {
			ih.logger.Warn("failed to update budget usage", zap.Error(err))
		}

		// Centralized cost tracking
		ih.trackCentralizedCost(req, "Federation")
	})

	// Mark rate limit success
	if req.ActorDomain != "" {
		if err := ih.rateLimiter.RecordAttempt(ctx.Context(), req.ActorDomain, headerValue(ctx, "X-Forwarded-For"), true); err != nil {
			ih.logger.Warn("failed to record rate limit success", zap.Error(err))
		}
	}
}

// recordFailureCost records cost tracking for failures
func (ih *InboxHandler) recordFailureCost(req *InboxRequest, errorMsg string, readCount int) {
	req.CostParams.Success = false
	req.CostParams.ErrorMessage = errorMsg
	req.CostParams.ResponseTimeMs = time.Since(req.StartTime).Milliseconds()
	req.CostParams.LambdaDurationMs = time.Since(req.StartTime).Milliseconds()
	if readCount > 0 {
		req.CostParams.DynamoDBReadCount = int64(readCount)
	}

	// Track with both federation-specific and centralized cost tracking
	runAsync(func() {
		// Legacy federation cost tracking
		cost := ih.costCalculator.CalculateFederationCosts(req.CostParams)
		if err := ih.federationCostRepository.RecordFederationCost(context.Background(), cost); err != nil {
			ih.logger.Warn("failed to record federation cost", zap.Error(err))
		}

		// Centralized cost tracking for failures
		ih.trackCentralizedCost(req, "Federation.Error")
	})
}

// isAddressedTo checks if the activity is addressed to the given actor
func (ih *InboxHandler) isAddressedTo(activity *activitypub.Activity, actor *activitypub.Actor) bool {
	return inboxvalidation.IsAddressedTo(activity, actor)
}

// convertRequest converts an AppTheory request to an http.Request.
func (ih *InboxHandler) convertRequest(ctx *apptheory.Context, body []byte) (*http.Request, error) {
	u := inboxRequestURL(ctx)

	// Create request with context
	req, err := http.NewRequestWithContext(ctx.Context(), ctx.Request.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Copy headers
	for k, values := range ctx.Request.Headers {
		canonicalKey := http.CanonicalHeaderKey(k)
		for _, v := range values {
			req.Header.Add(canonicalKey, v)
		}
	}

	// Ensure Host is set (both in request and header), as signature verification depends on it.
	if u.Host != "" {
		req.Host = u.Host
		req.Header.Set("Host", u.Host)
	}

	return req, nil
}

func (ih *InboxHandler) validateInboundSignatureHost(req *http.Request) error {
	if req == nil || req.URL == nil {
		return fmt.Errorf("missing request URL")
	}
	requestHost := strings.ToLower(strings.TrimSpace(req.URL.Hostname()))
	if requestHost == "" {
		return fmt.Errorf("missing request host")
	}
	instanceHost := strings.ToLower(strings.TrimSpace(ih.localDomain()))
	if instanceHost == "" {
		return nil
	}
	if requestHost != instanceHost {
		return fmt.Errorf("request host %q does not match instance host %q", requestHost, instanceHost)
	}
	return nil
}

func inboxRequestURL(ctx *apptheory.Context) *url.URL {
	if ctx == nil {
		return &url.URL{Scheme: "https", Path: "/"}
	}
	return common.RequestURLFromHeaders(ctx.Request.Headers, ctx.Request.Path, ctx.Request.Query)
}

// getConfig returns the current configuration
func (ih *InboxHandler) getConfig() *config.Config {
	return config.Get()
}

// generateActivityID generates a unique activity ID
func generateActivityID() string {
	// Use timestamp and random bytes for uniqueness
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-only ID on crypto error
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%x", time.Now().UnixNano(), b)
}

func (ih *InboxHandler) resolveDeliverableActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	resolver := ih.remoteActorResolver
	if resolver == nil && ih.storageAdapter != nil {
		resolver = federation.NewRemoteSearchService(ih.storageAdapter)
	}
	if resolver == nil {
		return nil, stdErrors.New("remote actor resolver unavailable")
	}

	localDomain := ""
	if cfg := ih.getConfig(); cfg != nil {
		localDomain = cfg.Domain
	}

	resolution, err := resolver.ResolveDeliverableActor(ctx, actorID, localDomain)
	if err != nil {
		return nil, err
	}
	if resolution == nil || resolution.Actor == nil {
		return nil, common.ActorNotFoundError{Username: actorID}
	}

	return resolution.Actor, nil
}

func (ih *InboxHandler) deliverResolvedActivity(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error {
	deliverer := ih.activityDeliverer
	if deliverer == nil {
		deliverer = ih.deliveryService
	}
	if deliverer == nil {
		return stdErrors.New("delivery service unavailable")
	}

	return deliverer.DeliverActivity(ctx, activity, targetInbox, signingActor)
}

// processFollowActivity processes an incoming Follow activity
func (ih *InboxHandler) processFollowActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)
	trace := federation.NewFollowTraceMetadata(activity, activity.Actor, targetActor.PreferredUsername)

	// Check for block relationship before processing follow
	if err := ih.checkBlockStatus(ctx, activity.Actor, targetActor.ID); err != nil {
		log.Warn("follow activity blocked due to block relationship",
			zap.String("follower", activity.Actor),
			zap.String("target", targetActor.ID))
		// Return success to avoid revealing block status to blocked actor
		return nil
	}

	// Extract follower username from actor ID
	followerHandle := ih.extractHandleFromActorID(activity.Actor)
	ih.logFollowTrace(trace, "receiver.follow.start",
		zap.String("follower_handle", followerHandle),
		zap.String("target_username", targetActor.PreferredUsername),
	)

	// Create the follow relationship with pending state
	err := ih.relationshipRepository.CreateRelationship(ctx, followerHandle, targetActor.PreferredUsername, activity.ID)
	if err != nil {
		ih.logFollowTrace(trace, "receiver.follow.relationship_create_failed",
			zap.String("follower_handle", followerHandle),
			zap.String("target_username", targetActor.PreferredUsername),
			zap.String("relationship_error", err.Error()),
		)
		log.Error("failed to create follow relationship", zap.Error(err))
		return err
	}
	ih.logFollowTrace(trace, "receiver.follow.relationship_created",
		zap.String("follower_handle", followerHandle),
		zap.String("target_username", targetActor.PreferredUsername),
		zap.String("relationship_result", "created_pending"),
	)

	// Check if the target actor requires manual approval for follows
	if targetActor.ManuallyApprovesFollowers {
		ih.logFollowTrace(trace, "receiver.follow.manual_pending",
			zap.String("follower_handle", followerHandle),
			zap.String("target_username", targetActor.PreferredUsername),
		)
		log.Info("follow request pending manual approval",
			zap.String("follower", followerHandle),
			zap.String("target", targetActor.PreferredUsername))

		// Send notification to the target user about pending follow request
		followRequestNotif := models.NewFollowRequestNotification(targetActor.PreferredUsername, followerHandle)
		if err := ih.notificationRepository.CreateNotification(ctx, followRequestNotif); err != nil {
			log.Warn("failed to create follow request notification", zap.Error(err))
		} else {
			log.Info("created follow request notification",
				zap.String("notification_id", followRequestNotif.ID),
				zap.String("target_user", targetActor.PreferredUsername),
				zap.String("follower", followerHandle))
		}

		// Follow request stays in pending state - no further action needed
		return nil
	}

	// Auto-accept follows for non-locked accounts
	err = ih.relationshipRepository.AcceptFollowRequest(ctx, followerHandle, targetActor.PreferredUsername)
	if err != nil {
		log.Error("failed to accept follow", zap.Error(err))
		return err
	}

	log.Info("follow request auto-accepted",
		zap.String("follower", followerHandle),
		zap.String("target", targetActor.PreferredUsername))

	// Create Accept activity
	acceptActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AcceptType,
			ID:      fmt.Sprintf("https://%s/activities/%s", ih.getConfig().Domain, generateActivityID()),
			To:      []string{activity.Actor},
		},
		Actor:  targetActor.ID,
		Object: activity.ID,
	}

	// Resolve a deliverable remote actor rather than depending on cache luck.
	followerActor, err := ih.resolveDeliverableActor(ctx, activity.Actor)
	if err != nil {
		ih.logFollowTrace(trace, "receiver.follow.accept_delivery_skipped",
			zap.String("follower_handle", followerHandle),
			zap.String("target_username", targetActor.PreferredUsername),
			zap.String("accept_result", "remote_actor_resolution_failed"),
			zap.String("resolution_error", err.Error()),
		)
		log.Warn("follow accepted but accept delivery was skipped because the remote actor could not be resolved",
			zap.String("actor", activity.Actor),
			zap.Error(err))
		return nil // Don't fail the persisted acceptance
	}

	ih.logFollowTrace(trace, "receiver.follow.accept_prepare",
		zap.String("follower_handle", followerHandle),
		zap.String("target_username", targetActor.PreferredUsername),
		zap.String("accept_target_inbox", followerActor.Inbox),
	)

	// Send Accept activity back to the follower
	if err := ih.deliverResolvedActivity(ctx, acceptActivity, followerActor.Inbox, targetActor); err != nil {
		ih.logFollowTrace(trace, "receiver.follow.accept_delivery_failed",
			zap.String("follower_handle", followerHandle),
			zap.String("target_username", targetActor.PreferredUsername),
			zap.String("accept_target_inbox", followerActor.Inbox),
			zap.String("accept_result", err.Error()),
		)
		log.Error("failed to deliver accept activity",
			zap.String("to", activity.Actor),
			zap.String("from", targetActor.ID),
			zap.Error(err))
		// Don't fail the whole operation if delivery fails
	} else {
		ih.logFollowTrace(trace, "receiver.follow.accept_delivered",
			zap.String("follower_handle", followerHandle),
			zap.String("target_username", targetActor.PreferredUsername),
			zap.String("accept_target_inbox", followerActor.Inbox),
			zap.String("accept_result", "delivered"),
		)
		log.Info("delivered accept activity",
			zap.String("to", activity.Actor),
			zap.String("from", targetActor.ID))
	}

	return nil
}

// processAcceptActivity processes an incoming Accept activity
func (ih *InboxHandler) processAcceptActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)
	acceptorHandle := ih.extractHandleFromActorID(activity.Actor)
	if acceptorHandle == "" {
		return nil
	}

	switch object := activity.Object.(type) {
	case string:
		return ih.processAcceptByActivityID(ctx, object, targetActor, acceptorHandle)
	case map[string]any:
		return ih.processAcceptByEmbeddedObject(ctx, object, targetActor, acceptorHandle)
	default:
		log.Warn("accept activity has unsupported object type",
			zap.String("object_type", fmt.Sprintf("%T", object)))
		return nil
	}
}

func (ih *InboxHandler) processAcceptByActivityID(ctx context.Context, objectID string, targetActor *activitypub.Actor, acceptorHandle string) error {
	log := common.WithContext(ctx)

	originalActivity, err := ih.activityRepository.GetActivity(ctx, objectID)
	if err == nil {
		if validationErr := ih.ensureStoredActivityHydrated(ctx, objectID, originalActivity); validationErr != nil {
			return validationErr
		}
		if originalActivity.Type == activitypub.FollowType {
			followerHandle := ih.extractHandleFromActorID(originalActivity.Actor)
			if followerHandle == "" {
				followerHandle = targetActor.PreferredUsername
			}
			return ih.acceptFollowRelationship(ctx, followerHandle, acceptorHandle)
		}
	}

	if err != nil {
		log.Warn("failed to find original activity", zap.String("id", objectID), zap.Error(err))
		reconciled, fallbackErr := ih.reconcileFollowResponseFromRelationship(ctx, activitypub.AcceptType, objectID, targetActor, acceptorHandle)
		if fallbackErr != nil {
			return fallbackErr
		}
		if reconciled {
			return nil
		}
		return nil
	}

	log.Warn("accept activity did not reference a stored follow activity",
		zap.String("id", objectID),
		zap.String("original_type", fmt.Sprintf("%v", originalActivity.Type)))
	return nil
}

func (ih *InboxHandler) processAcceptByEmbeddedObject(ctx context.Context, object map[string]any, targetActor *activitypub.Actor, acceptorHandle string) error {
	log := common.WithContext(ctx)

	if objectType, _ := object["type"].(string); objectType != activitypub.FollowType {
		return nil
	}

	followerHandle := targetActor.PreferredUsername
	if actorID, ok := object["actor"].(string); ok {
		if handle := ih.extractHandleFromActorID(actorID); handle != "" {
			followerHandle = handle
		}
	}

	followTargetHandle := acceptorHandle
	if followObjectID, ok := object["object"].(string); ok {
		if handle := ih.extractHandleFromActorID(followObjectID); handle != "" {
			followTargetHandle = handle
		}
	}
	if followTargetHandle == "" || !strings.EqualFold(strings.TrimSpace(followTargetHandle), strings.TrimSpace(acceptorHandle)) {
		log.Warn("embedded accept follow target mismatch",
			zap.String("follower", followerHandle),
			zap.String("acceptor", acceptorHandle),
			zap.String("follow_target", followTargetHandle))
		return nil
	}

	return ih.acceptFollowRelationship(ctx, followerHandle, acceptorHandle)
}

func (ih *InboxHandler) acceptFollowRelationship(ctx context.Context, followerHandle, acceptorHandle string) error {
	if followerHandle == "" || acceptorHandle == "" {
		return nil
	}

	if err := ih.relationshipRepository.AcceptFollowRequest(ctx, followerHandle, acceptorHandle); err != nil {
		common.WithContext(ctx).Error("failed to update follow status",
			zap.String("follower", followerHandle),
			zap.String("acceptor", acceptorHandle),
			zap.Error(err))
		return err
	}

	return nil
}

func (ih *InboxHandler) rejectFollowRelationship(ctx context.Context, followerHandle, rejectorHandle string) error {
	if followerHandle == "" || rejectorHandle == "" {
		return nil
	}

	if err := ih.relationshipRepository.RejectFollowRequest(ctx, followerHandle, rejectorHandle); err != nil {
		common.WithContext(ctx).Error("failed to update follow rejection",
			zap.String("follower", followerHandle),
			zap.String("rejector", rejectorHandle),
			zap.Error(err))
		return err
	}

	return nil
}

func (ih *InboxHandler) reconcileFollowResponseFromRelationship(ctx context.Context, responseType, objectID string, targetActor *activitypub.Actor, remoteHandle string) (bool, error) {
	log := common.WithContext(ctx)
	if targetActor == nil || strings.TrimSpace(remoteHandle) == "" {
		return false, nil
	}
	if _, ok := ih.localActivityIDSuffix(objectID); !ok {
		return false, nil
	}

	followerHandle := strings.TrimSpace(targetActor.PreferredUsername)
	if followerHandle == "" {
		followerHandle = ih.extractHandleFromActorID(targetActor.ID)
	}
	if followerHandle == "" {
		return false, nil
	}

	relationship, err := ih.relationshipRepository.GetRelationship(ctx, followerHandle, remoteHandle)
	if err != nil {
		log.Warn("failed to load pending relationship for follow response fallback",
			zap.String("follower", followerHandle),
			zap.String("remote", remoteHandle),
			zap.Error(err))
		return false, nil
	}
	if relationship == nil || relationship.State != models.RelationshipPending {
		return false, nil
	}
	if !ih.followResponseActivityIDsMatch(objectID, relationship.ActivityID) {
		log.Warn("follow response fallback activity id mismatch",
			zap.String("object_id", objectID),
			zap.String("stored_activity_id", relationship.ActivityID),
			zap.String("follower", followerHandle),
			zap.String("remote", remoteHandle))
		return false, nil
	}

	switch responseType {
	case activitypub.AcceptType:
		return true, ih.acceptFollowRelationship(ctx, followerHandle, remoteHandle)
	case activitypub.RejectType:
		return true, ih.rejectFollowRelationship(ctx, followerHandle, remoteHandle)
	default:
		return false, nil
	}
}

func (ih *InboxHandler) localActivityIDSuffix(activityID string) (string, bool) {
	trimmed := strings.TrimSpace(activityID)
	baseURL := strings.TrimSuffix(ih.getConfig().BaseURL(), "/")
	if trimmed == "" || baseURL == "" {
		return "", false
	}

	prefix := baseURL + "/activities/"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}

	suffix := strings.TrimPrefix(trimmed, prefix)
	if suffix == "" || strings.Contains(suffix, "/") {
		return "", false
	}

	return suffix, true
}

func (ih *InboxHandler) followResponseActivityIDsMatch(objectID, storedActivityID string) bool {
	objectID = strings.TrimSpace(objectID)
	storedActivityID = strings.TrimSpace(storedActivityID)
	if objectID == "" || storedActivityID == "" {
		return false
	}
	if objectID == storedActivityID {
		return true
	}

	objectSuffix, ok := ih.localActivityIDSuffix(objectID)
	if !ok {
		return false
	}
	if objectSuffix == storedActivityID {
		return true
	}

	storedSuffix, storedIsLocal := ih.localActivityIDSuffix(storedActivityID)
	return storedIsLocal && storedSuffix == objectSuffix
}

func (ih *InboxHandler) ensureStoredActivityHydrated(ctx context.Context, objectID string, activity *activitypub.Activity) error {
	log := common.WithContext(ctx)

	missingFields := make([]string, 0, 3)
	if activity == nil {
		missingFields = append(missingFields, "activity")
	} else {
		if strings.TrimSpace(activity.ID) == "" {
			missingFields = append(missingFields, "id")
		}
		if strings.TrimSpace(activity.Type) == "" {
			missingFields = append(missingFields, "type")
		}
	}

	if len(missingFields) == 0 {
		return nil
	}

	log.Error("stored activity missing required routing fields",
		zap.String("id", objectID),
		zap.Strings("missing_fields", missingFields))

	return fmt.Errorf("%w: stored activity %q missing required routing fields: %s",
		storage.ErrInvalidInput,
		objectID,
		strings.Join(missingFields, ", "))
}

// processRejectActivity processes an incoming Reject activity
func (ih *InboxHandler) processRejectActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	log.Info("processing reject activity",
		zap.String("rejector", activity.Actor),
		zap.String("activity_id", activity.ID))

	// Handle different object types for rejection
	switch obj := activity.Object.(type) {
	case string:
		// Object is an activity ID - fetch the original activity
		return ih.processRejectByActivityID(ctx, activity, targetActor, obj)

	case map[string]any:
		// Object is embedded - handle based on type
		return ih.processRejectByEmbeddedObject(ctx, activity, targetActor, obj)

	default:
		log.Warn("reject activity has unsupported object type",
			zap.String("object_type", fmt.Sprintf("%T", obj)))
		return nil // Don't fail on unknown object types
	}
}

// processRejectByActivityID processes rejection by fetching the original activity
func (ih *InboxHandler) processRejectByActivityID(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor, objectID string) error {
	log := common.WithContext(ctx)

	// Fetch the original activity being rejected
	originalActivity, err := ih.activityRepository.GetActivity(ctx, objectID)
	if err != nil {
		log.Warn("failed to find original activity for rejection",
			zap.String("activity_id", objectID),
			zap.Error(err))
		reconciled, fallbackErr := ih.reconcileFollowResponseFromRelationship(ctx, activitypub.RejectType, objectID, targetActor, ih.extractHandleFromActorID(activity.Actor))
		if fallbackErr != nil {
			return fallbackErr
		}
		if reconciled {
			return nil
		}
		return nil // Don't fail, just ignore unknown activities
	}

	if validationErr := ih.ensureStoredActivityHydrated(ctx, objectID, originalActivity); validationErr != nil {
		return validationErr
	}

	log.Info("processing reject for original activity",
		zap.String("original_type", originalActivity.Type),
		zap.String("original_id", originalActivity.ID),
		zap.String("original_actor", originalActivity.Actor))

	// Handle rejection based on original activity type
	switch originalActivity.Type {
	case activitypub.FollowType:
		return ih.processRejectFollow(ctx, activity, targetActor, originalActivity)
	case activitypub.LikeType:
		return ih.processRejectLike(ctx, activity, targetActor, originalActivity)
	case activitypub.AnnounceType:
		return ih.processRejectAnnounce(ctx, activity, targetActor, originalActivity)
	case activitypub.CreateType:
		return ih.processRejectCreate(ctx, activity, targetActor, originalActivity)
	case activitypub.UpdateType:
		return ih.processRejectUpdate(ctx, activity, targetActor, originalActivity)
	case activitypub.DeleteType:
		return ih.processRejectDelete(ctx, activity, targetActor, originalActivity)
	case activitypub.AcceptType:
		return ih.processRejectAccept(ctx, activity, targetActor, originalActivity)
	case activitypub.AddType:
		return ih.processRejectAdd(ctx, activity, targetActor, originalActivity)
	case activitypub.RemoveType:
		return ih.processRejectRemove(ctx, activity, targetActor, originalActivity)
	case activitypub.FlagType:
		return ih.processRejectFlag(ctx, activity, targetActor, originalActivity)
	case activitypub.MoveType:
		return ih.processRejectMove(ctx, activity, targetActor, originalActivity)
	default:
		// Handle unsupported activity types by logging and recording the rejection
		log.Info("processing reject activity for unsupported type",
			zap.String("rejected_type", originalActivity.Type),
			zap.String("rejecting_actor", activity.Actor),
			zap.String("activity_id", activity.ID))

		// Record the rejection for monitoring purposes using federation activity tracking
		if ih.federationActivityRepository != nil {
			domain := ih.extractDomainFromURL(activity.Actor)
			timestamp := time.Now()
			activityID := fmt.Sprintf("reject_%d", timestamp.UnixNano())

			federationActivity := &models.FederationActivity{
				// Set the composite keys manually
				PK:     fmt.Sprintf("fed_activity#%s", domain),
				SK:     fmt.Sprintf("activity#%d#%s", timestamp.Unix(), activityID),
				GSI1PK: fmt.Sprintf("FED_TYPE#%s", "Reject"),
				GSI1SK: fmt.Sprintf("%d#%s#%s", timestamp.Unix(), domain, activityID),
				GSI2PK: fmt.Sprintf("FED_ACTOR#%s", activity.Actor),
				GSI2SK: fmt.Sprintf("%d#%s", timestamp.Unix(), activityID),

				// Core activity data
				ID:           activityID,
				Domain:       domain,
				ActivityType: "Reject",
				ActorID:      activity.Actor,
				ObjectType:   originalActivity.Type,
				Timestamp:    timestamp,
				Success:      true, // Successfully processed the rejection
				ErrorMessage: fmt.Sprintf("Rejected unsupported activity type: %s", originalActivity.Type),
			}

			err := ih.federationActivityRepository.Create(ctx, federationActivity)
			if err != nil {
				log.Warn("failed to record rejection federation activity",
					zap.Error(err))
			}
		}

		// Return success - rejecting unknown activity types is valid behavior
		return nil
	}
}

// processRejectByEmbeddedObject processes rejection with embedded object
func (ih *InboxHandler) processRejectByEmbeddedObject(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor, obj map[string]any) error {
	log := common.WithContext(ctx)

	objType, ok := obj["type"].(string)
	if !ok {
		log.Warn("embedded object has no type")
		return nil
	}

	log.Info("processing reject for embedded object",
		zap.String("object_type", objType))

	// Handle rejection based on embedded object type
	switch objType {
	case activitypub.FollowType:
		// Convert to Follow activity
		objJSON, err := json.Marshal(obj)
		if err != nil {
			return marshalFollowError()
		}

		var followActivity activitypub.Activity
		if err := common.ParseActivityPubObject(objJSON, &followActivity); err != nil {
			return parseFollowError()
		}

		return ih.processRejectFollow(ctx, activity, targetActor, &followActivity)

	default:
		log.Info("reject activity for unsupported embedded object type",
			zap.String("object_type", objType))
		return nil
	}
}

// processRejectFollow processes rejection of a Follow request
func (ih *InboxHandler) processRejectFollow(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, followActivity *activitypub.Activity) error {
	log := common.WithContext(ctx)

	// Extract requester handle from the original follow activity
	requesterHandle := ih.extractHandleFromActorID(followActivity.Actor)
	rejectorHandle := ih.extractHandleFromActorID(rejectActivity.Actor)

	log.Info("processing follow rejection",
		zap.String("requester", requesterHandle),
		zap.String("rejector", rejectorHandle),
		zap.String("follow_id", followActivity.ID))

	if err := ih.rejectFollowRelationship(ctx, requesterHandle, rejectorHandle); err != nil {
		return err
	}

	log.Info("successfully processed follow rejection",
		zap.String("requester", requesterHandle),
		zap.String("rejector", rejectorHandle))

	return nil
}

// rejectActivityConfig holds configuration for reject processing
type rejectActivityConfig struct {
	activityType   string
	actorFieldName string
	deleteFunc     func(ctx context.Context, actor, objectID string) error
}

type simpleRejectConfig struct {
	activityType    string
	actorFieldName  string
	objectFieldName string
	warningMessage  string
	successMessage  string
	includeTarget   bool // Whether to include target collection in logs
}

// processRejectInteraction processes rejection of a Like or Announce activity
func (ih *InboxHandler) processRejectInteraction(ctx context.Context, rejectActivity *activitypub.Activity, targetActivity *activitypub.Activity, config rejectActivityConfig) error {
	log := common.WithContext(ctx)

	// Extract object ID from activity
	var objectID string
	switch obj := targetActivity.Object.(type) {
	case string:
		objectID = obj
	case map[string]any:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		log.Warn(fmt.Sprintf("%s activity has no object ID to reject", config.activityType))
		return nil
	}

	log.Info(fmt.Sprintf("processing %s rejection", config.activityType),
		zap.String(config.actorFieldName, targetActivity.Actor),
		zap.String("rejector", rejectActivity.Actor),
		zap.String("object", objectID))

	// Remove the activity
	if err := config.deleteFunc(ctx, targetActivity.Actor, objectID); err != nil {
		log.Debug(fmt.Sprintf("no %s to remove during rejection", config.activityType),
			zap.String("actor", targetActivity.Actor),
			zap.String("object", objectID),
			zap.Error(err))
	}

	log.Info(fmt.Sprintf("successfully processed %s rejection", config.activityType),
		zap.String(config.actorFieldName, targetActivity.Actor),
		zap.String("object", objectID))

	return nil
}

// processSimpleReject handles simple rejection activities that only require logging
func (ih *InboxHandler) processSimpleReject(ctx context.Context, rejectActivity *activitypub.Activity, targetActivity *activitypub.Activity, config simpleRejectConfig) error {
	log := common.WithContext(ctx)

	// Extract object ID from activity
	var objectID string
	switch obj := targetActivity.Object.(type) {
	case string:
		objectID = obj
	case map[string]any:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if err := common.ValidateRequiredParam(config.objectFieldName, objectID); err != nil {
		log.Warn(config.warningMessage)
		return nil
	}

	// Base log fields
	logFields := []zap.Field{
		zap.String(config.actorFieldName, targetActivity.Actor),
		zap.String("rejector", rejectActivity.Actor),
		zap.String(config.objectFieldName, objectID),
	}

	// Add target collection if requested
	if config.includeTarget {
		var targetCollection string
		if targetActivity.Target != "" {
			targetCollection = targetActivity.Target
		}
		logFields = append(logFields, zap.String("collection", targetCollection))
	}

	log.Info(fmt.Sprintf("processing %s rejection", config.activityType), logFields...)

	// Success log fields
	successFields := []zap.Field{
		zap.String(config.actorFieldName, targetActivity.Actor),
		zap.String(config.objectFieldName, objectID),
	}

	// Add target collection to success log if requested
	if config.includeTarget {
		var targetCollection string
		if targetActivity.Target != "" {
			targetCollection = targetActivity.Target
		}
		successFields = append(successFields, zap.String("collection", targetCollection))
	}

	log.Info(config.successMessage, successFields...)

	return nil
}

// processRejectLike processes rejection of a Like activity
func (ih *InboxHandler) processRejectLike(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, likeActivity *activitypub.Activity) error {
	return ih.processRejectInteraction(ctx, rejectActivity, likeActivity, rejectActivityConfig{
		activityType:   "like",
		actorFieldName: "liker",
		deleteFunc:     ih.likeRepository.DeleteLike,
	})
}

// processRejectAnnounce processes rejection of an Announce activity
func (ih *InboxHandler) processRejectAnnounce(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, announceActivity *activitypub.Activity) error {
	return ih.processRejectInteraction(ctx, rejectActivity, announceActivity, rejectActivityConfig{
		activityType:   "announce",
		actorFieldName: "announcer",
		deleteFunc:     ih.socialRepository.DeleteAnnounce,
	})
}

// processRejectCreate processes rejection of a Create activity
func (ih *InboxHandler) processRejectCreate(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, createActivity *activitypub.Activity) error {
	return ih.processSimpleReject(ctx, rejectActivity, createActivity, simpleRejectConfig{
		activityType:    "create",
		actorFieldName:  "creator",
		objectFieldName: "object",
		warningMessage:  "create activity has no object ID to reject",
		successMessage:  "successfully processed create rejection - object not accepted",
	})
}

// processRejectUpdate processes rejection of an Update activity
func (ih *InboxHandler) processRejectUpdate(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, updateActivity *activitypub.Activity) error {
	return ih.processSimpleReject(ctx, rejectActivity, updateActivity, simpleRejectConfig{
		activityType:    "update",
		actorFieldName:  "updater",
		objectFieldName: "object",
		warningMessage:  "update activity has no object ID to reject",
		successMessage:  "successfully processed update rejection - update not applied",
	})
}

// processRejectDelete processes rejection of a Delete activity
func (ih *InboxHandler) processRejectDelete(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, deleteActivity *activitypub.Activity) error {
	return ih.processSimpleReject(ctx, rejectActivity, deleteActivity, simpleRejectConfig{
		activityType:    "delete",
		actorFieldName:  "deleter",
		objectFieldName: "object",
		warningMessage:  "delete activity has no object ID to reject",
		successMessage:  "successfully processed delete rejection - object preserved",
	})
}

// processRejectAccept processes rejection of an Accept activity
func (ih *InboxHandler) processRejectAccept(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, acceptActivity *activitypub.Activity) error {
	return ih.processSimpleReject(ctx, rejectActivity, acceptActivity, simpleRejectConfig{
		activityType:    "accept",
		actorFieldName:  "accepter",
		objectFieldName: "original_activity",
		warningMessage:  "accept activity has no original activity ID to reject",
		successMessage:  "successfully processed accept rejection",
	})
}

// processRejectAdd processes rejection of an Add activity
func (ih *InboxHandler) processRejectAdd(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, addActivity *activitypub.Activity) error {
	return ih.processSimpleReject(ctx, rejectActivity, addActivity, simpleRejectConfig{
		activityType:    "add",
		actorFieldName:  "adder",
		objectFieldName: "object",
		warningMessage:  "add activity has no object ID to reject",
		successMessage:  "successfully processed add rejection - object not added",
		includeTarget:   true,
	})
}

// processRejectRemove processes rejection of a Remove activity
func (ih *InboxHandler) processRejectRemove(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, removeActivity *activitypub.Activity) error {
	return ih.processSimpleReject(ctx, rejectActivity, removeActivity, simpleRejectConfig{
		activityType:    "remove",
		actorFieldName:  "remover",
		objectFieldName: "object",
		warningMessage:  "remove activity has no object ID to reject",
		successMessage:  "successfully processed remove rejection - object preserved in collection",
		includeTarget:   true,
	})
}

// processRejectFlag processes rejection of a Flag activity
func (ih *InboxHandler) processRejectFlag(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, flagActivity *activitypub.Activity) error {
	return ih.processSimpleReject(ctx, rejectActivity, flagActivity, simpleRejectConfig{
		activityType:    "flag",
		actorFieldName:  "flagger",
		objectFieldName: "flagged_object",
		warningMessage:  "flag activity has no object ID to reject",
		successMessage:  "successfully processed flag rejection - report not processed",
	})
}

// processRejectMove processes rejection of a Move activity
func (ih *InboxHandler) processRejectMove(ctx context.Context, rejectActivity *activitypub.Activity, _ *activitypub.Actor, moveActivity *activitypub.Activity) error {
	log := common.WithContext(ctx)

	// Extract moved object ID from move
	var movedObjectID string
	switch obj := moveActivity.Object.(type) {
	case string:
		movedObjectID = obj
	case map[string]any:
		if id, ok := obj["id"].(string); ok {
			movedObjectID = id
		}
	}

	if err := common.ValidateRequiredParam("movedObjectID", movedObjectID); err != nil {
		log.Warn("move activity has no object ID to reject")
		return nil
	}

	// Extract target location
	var targetLocation string
	if moveActivity.Target != "" {
		targetLocation = moveActivity.Target
	}

	log.Info("processing move rejection",
		zap.String("mover", moveActivity.Actor),
		zap.String("rejector", rejectActivity.Actor),
		zap.String("moved_object", movedObjectID),
		zap.String("target", targetLocation))

	// Rejecting a Move means the recipient refuses to acknowledge the migration
	log.Info("successfully processed move rejection - migration not acknowledged",
		zap.String("mover", moveActivity.Actor),
		zap.String("moved_object", movedObjectID),
		zap.String("target", targetLocation))

	return nil
}

func (ih *InboxHandler) buildCanonicalRemoteStatus(note *activitypub.Note) *models.Status {
	return federation.BuildCanonicalRemoteStatus(note, ih.baseURL)
}

func (ih *InboxHandler) materializeRemoteNoteStatus(ctx context.Context, note *activitypub.Note) (*models.Status, error) {
	return federation.MaterializeRemoteNote(ctx, ih.objectRepository, ih.statusRepository, note, ih.baseURL)
}

func (ih *InboxHandler) upsertRemoteNoteStatus(ctx context.Context, note *activitypub.Note) error {
	if ih.statusRepository == nil {
		return fmt.Errorf("status repository not configured")
	}

	status := ih.buildCanonicalRemoteStatus(note)
	if status == nil {
		return fmt.Errorf("canonical remote status payload is invalid")
	}

	existing, err := ih.statusRepository.GetStatus(ctx, status.StatusID)
	if err != nil {
		if isRemoteStatusNotFound(err) {
			_, materializeErr := ih.materializeRemoteNoteStatus(ctx, note)
			return materializeErr
		}
		return err
	}

	mergeExistingRemoteStatusProjection(status, existing)
	return ih.statusRepository.UpdateStatus(ctx, status)
}

func (ih *InboxHandler) deleteRemoteNoteStatus(ctx context.Context, objectID string) error {
	if ih.statusRepository == nil {
		return fmt.Errorf("status repository not configured")
	}

	statusID := models.CanonicalStatusID(objectID)
	if statusID == "" {
		return fmt.Errorf("canonical remote status id is invalid")
	}

	if err := ih.statusRepository.DeleteStatus(ctx, statusID); err != nil {
		if isRemoteStatusNotFound(err) {
			return nil
		}
		return err
	}

	return nil
}

func mergeExistingRemoteStatusProjection(projected, existing *models.Status) {
	if projected == nil || existing == nil {
		return
	}

	projected.PK = existing.PK
	projected.SK = existing.SK
	projected.CreatedAt = existing.CreatedAt
	projected.LikeCount = existing.LikeCount
	projected.ReblogCount = existing.ReblogCount
	projected.ReplyCount = existing.ReplyCount
	projected.QuoteCount = existing.QuoteCount
	projected.Deleted = existing.Deleted
	projected.DeletedAt = existing.DeletedAt
	projected.Flagged = existing.Flagged
	projected.Version = existing.Version
	projected.BoostOfStatusID = existing.BoostOfStatusID
	projected.BoostOfAuthorID = existing.BoostOfAuthorID
	projected.BoostAnnounceID = existing.BoostAnnounceID
	projected.ReblogOfID = existing.ReblogOfID
	projected.QuoteTargetStatusID = existing.QuoteTargetStatusID
	projected.QuoteTargetAuthorID = existing.QuoteTargetAuthorID

	if projected.Language == "" {
		projected.Language = existing.Language
	}
	if projected.PublishedAt.IsZero() {
		projected.PublishedAt = existing.PublishedAt
	}
	if len(projected.URLs) == 0 && len(existing.URLs) > 0 {
		projected.URLs = append([]string(nil), existing.URLs...)
	}

	if projected.Note == nil || existing.Note == nil {
		return
	}

	if projected.Note.Published == nil && !existing.PublishedAt.IsZero() {
		publishedAt := existing.PublishedAt
		projected.Note.Published = &publishedAt
	}
}

func isRemoteStatusNotFound(err error) bool {
	if err == nil {
		return false
	}

	return errors.HasCode(err, errors.CodeNotFound) ||
		stdErrors.Is(err, storage.ErrNotFound) ||
		dynamormerrors.IsNotFound(err)
}

// processRemoteCreateActivity processes an incoming Create activity from a remote instance
func (ih *InboxHandler) processRemoteCreateActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Check for block relationship before processing create activity
	if err := ih.checkBlockStatus(ctx, activity.Actor, targetActor.ID); err != nil {
		log.Warn("create activity blocked due to block relationship",
			zap.String("creator", activity.Actor),
			zap.String("target", targetActor.ID))
		// Return success to avoid revealing block status to blocked actor
		return nil
	}

	// Extract the object
	objMap, ok := activity.Object.(map[string]any)
	if !ok {
		log.Warn("create activity has invalid object")
		return nil
	}

	// Sanitize the object content to prevent XSS
	common.SanitizeActivityPubObjectDefault(objMap)

	// Store the object if it's a Note. Remote Article ingestion is explicitly
	// unsupported for the Blog/CMS MVP; do not materialize it as a disguised
	// status or silently route it through the Note path.
	objType, _ := objMap["type"].(string)
	switch strings.TrimSpace(objType) {
	case activitypub.NoteType:
		return ih.processRemoteCreateNote(ctx, activity, targetActor, objMap)
	case activitypub.ArticleType:
		ih.logUnsupportedRemoteArticle(ctx, activity, objMap, "create")
	default:
		log.Info("ignoring unsupported remote create object type",
			zap.String("activity_id", activity.ID),
			zap.String("actor", activity.Actor),
			zap.String("object_type", objType))
	}

	return nil
}

func (ih *InboxHandler) processRemoteCreateNote(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor, objMap map[string]any) error {
	log := common.WithContext(ctx)

	// Validate ActivityPub Note object
	if err := common.ValidateActivityPubNote(objMap); err != nil {
		log.Warn("invalid note object in create activity", zap.Error(err))
		return invalidNoteError()
	}

	// Convert to Note object
	objJSON, err := json.Marshal(objMap)
	if err != nil {
		return err
	}

	var note activitypub.Note
	if err := common.ParseActivityPubObject(objJSON, &note); err != nil {
		return err
	}

	directInfo := ih.classifyInboundDirectCreate(activity, &note, targetActor)
	if directInfo.unsupportedGroup {
		ih.logUnsupportedInboundDirectGroup(activity, &note, targetActor, directInfo)
		return nil
	}

	directConversation, createDirectConversation, skipCreate, err := ih.prepareDirectCreateState(ctx, activity, targetActor, &note, directInfo)
	if err != nil {
		return err
	}
	if skipCreate {
		return nil
	}

	// Store the note (it will be marked as remote)
	if err := ih.objectRepository.CreateObject(ctx, &note); err != nil {
		if !dynamormerrors.IsConditionFailed(err) && !stdErrors.Is(err, storage.ErrAlreadyExists) {
			log.Error("failed to store remote note", zap.Error(err))
			return err
		}
	}

	status, err := ih.materializeRemoteNoteStatus(ctx, &note)
	if err != nil {
		log.Error("failed to materialize remote note status",
			zap.String("note_id", note.ID),
			zap.Error(err))
		return err
	}

	return ih.finishRemoteCreateDeliverySideEffects(ctx, activity, targetActor, &note, status, directConversation, createDirectConversation, directInfo)
}

func (ih *InboxHandler) logUnsupportedRemoteArticle(ctx context.Context, activity *activitypub.Activity, objMap map[string]any, operation string) {
	objectID := ""
	if objMap != nil {
		objectID, _ = objMap["id"].(string)
	}

	common.WithContext(ctx).Info("ignoring unsupported remote Article activity",
		zap.String("operation", operation),
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.String("object_id", objectID))
}

func (ih *InboxHandler) prepareDirectCreateState(
	ctx context.Context,
	activity *activitypub.Activity,
	targetActor *activitypub.Actor,
	note *activitypub.Note,
	directInfo inboundDirectCreateInfo,
) (*models.Conversation, bool, bool, error) {
	if !directInfo.isDirect {
		return nil, false, false, nil
	}

	log := common.WithContext(ctx)
	existingStatus, err := ih.existingInboundDirectStatus(ctx, note)
	if err != nil {
		log.Error("failed to check existing inbound direct status",
			zap.String("activity_id", activity.ID),
			zap.String("note_id", note.ID),
			zap.Error(err))
		return nil, false, false, err
	}
	if existingStatus != nil {
		log.Info("skipping replayed inbound direct status",
			zap.String("activity_id", activity.ID),
			zap.String("note_id", note.ID),
			zap.String("status_id", existingStatus.StatusID),
			zap.String("conversation_id", existingStatus.ConversationID))
		return nil, false, true, nil
	}

	directConversation, createDirectConversation, err := ih.prepareInboundDirectConversation(ctx, activity, note, targetActor, directInfo)
	if err != nil {
		log.Error("failed to prepare inbound direct conversation",
			zap.String("activity_id", activity.ID),
			zap.String("target_actor", targetActor.ID),
			zap.Error(err))
		return nil, false, false, err
	}

	note.ConversationID = directConversation.ID
	note.Visibility = models.VisibilityDirect
	return directConversation, createDirectConversation, false, nil
}

func (ih *InboxHandler) finishRemoteCreateDeliverySideEffects(
	ctx context.Context,
	activity *activitypub.Activity,
	targetActor *activitypub.Actor,
	note *activitypub.Note,
	status *models.Status,
	directConversation *models.Conversation,
	createDirectConversation bool,
	directInfo inboundDirectCreateInfo,
) error {
	if !directInfo.isDirect {
		ih.createRemoteCreateNotifications(ctx, activity, targetActor, note, status)
		return nil
	}
	if directConversation == nil {
		return nil
	}

	log := common.WithContext(ctx)
	if err := ih.persistInboundDirectConversation(ctx, directConversation, createDirectConversation, status, directInfo); err != nil {
		log.Error("failed to persist inbound direct conversation",
			zap.String("activity_id", activity.ID),
			zap.String("conversation_id", directConversation.ID),
			zap.Error(err))
		return err
	}
	ih.createRemoteDirectNotification(ctx, activity, targetActor, note, status, directConversation, directInfo)
	ih.emitInboundDirectConversationEvent(ctx, directConversation, status, directInfo.localParticipantID)
	return nil
}

func (ih *InboxHandler) existingInboundDirectStatus(ctx context.Context, note *activitypub.Note) (*models.Status, error) {
	if ih.statusRepository == nil || note == nil {
		return nil, nil
	}

	statusID := models.CanonicalStatusIDForDomain(note.ID, ih.localDomain())
	if statusID == "" {
		return nil, nil
	}

	existing, err := ih.statusRepository.GetStatus(ctx, statusID)
	if err == nil && existing != nil {
		return existing, nil
	}
	if isRemoteStatusNotFound(err) {
		return nil, nil
	}
	return nil, err
}

type inboundDirectCreateInfo struct {
	isDirect           bool
	unsupportedGroup   bool
	participantRefs    []models.ConversationParticipantRef
	localParticipantID string
	remoteActorID      string
	remoteAcct         string
	remoteDomain       string
	specificRecipients []string
}

type inboundDirectConversationLookupRepository interface {
	GetConversationByParticipantRefs(ctx context.Context, refs []models.ConversationParticipantRef) (*models.Conversation, error)
}

func (ih *InboxHandler) classifyInboundDirectCreate(activity *activitypub.Activity, note *activitypub.Note, targetActor *activitypub.Actor) inboundDirectCreateInfo {
	info := inboundDirectCreateInfo{}
	if activity == nil || note == nil || targetActor == nil {
		return info
	}

	targetIDs := ih.localTargetActorIDs(targetActor)
	actorID := strings.TrimRight(strings.TrimSpace(activity.Actor), "/")
	if actorID == "" {
		actorID = strings.TrimRight(strings.TrimSpace(note.AttributedTo), "/")
	}

	recipients, hasPublic, hasCollection, targetsLocalActor := ih.inboundDirectSpecificRecipients(activity, note, targetIDs, actorID)
	info.specificRecipients = recipients
	if hasPublic || hasCollection || !targetsLocalActor {
		return info
	}
	if len(recipients) > 1 {
		info.unsupportedGroup = true
		return info
	}
	if len(recipients) != 1 {
		return info
	}

	localParticipantID := strings.TrimSpace(targetActor.PreferredUsername)
	if localParticipantID == "" {
		localParticipantID = ih.extractUsernameFromActorID(targetActor.ID)
	}
	if localParticipantID == "" || actorID == "" {
		return info
	}

	identity := federation.DescribeActorIdentity(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: actorID, Type: activitypub.PersonType},
	}, ih.localDomain())
	now := remoteCreateNotificationCreatedAt(activity, note, nil)
	remoteRef := models.NormalizeConversationParticipantRef(models.ConversationParticipantRef{
		ParticipantType: models.ConversationParticipantTypeRemoteActor,
		ParticipantID:   actorID,
		Acct:            identity.Acct,
		Domain:          identity.Domain,
		ResolvedAt:      &now,
	})

	info.isDirect = true
	info.localParticipantID = localParticipantID
	info.remoteActorID = actorID
	info.remoteAcct = remoteRef.Acct
	info.remoteDomain = remoteRef.Domain
	info.participantRefs = models.NormalizeConversationParticipantRefs([]models.ConversationParticipantRef{
		{
			ParticipantType: models.ConversationParticipantTypeLocalUser,
			ParticipantID:   localParticipantID,
		},
		remoteRef,
	})
	return info
}

func (ih *InboxHandler) inboundDirectSpecificRecipients(
	activity *activitypub.Activity,
	note *activitypub.Note,
	targetIDs map[string]struct{},
	actorID string,
) ([]string, bool, bool, bool) {
	seen := map[string]string{}
	hasPublic := false
	hasCollection := false
	targetsLocalActor := false
	actorKey := strings.ToLower(strings.TrimRight(actorID, "/"))

	visit := func(raw string) {
		recipient := strings.TrimRight(strings.TrimSpace(raw), "/")
		if recipient == "" {
			return
		}
		recipientKey := strings.ToLower(recipient)
		if recipientKey == strings.ToLower(strings.TrimRight(activitypub.PublicAddress, "/")) {
			hasPublic = true
			return
		}
		if isInboundDirectCollectionRecipient(recipient) {
			hasCollection = true
			return
		}
		if _, ok := targetIDs[recipientKey]; ok {
			targetsLocalActor = true
		}
		if actorKey != "" && recipientKey == actorKey {
			return
		}
		seen[recipientKey] = recipient
	}

	for _, recipient := range activity.To {
		visit(recipient)
	}
	for _, recipient := range activity.CC {
		visit(recipient)
	}
	for _, recipient := range activity.BTo {
		visit(recipient)
	}
	for _, recipient := range activity.BCC {
		visit(recipient)
	}
	for _, recipient := range note.To {
		visit(recipient)
	}
	for _, recipient := range note.CC {
		visit(recipient)
	}
	for _, recipient := range note.BTo {
		visit(recipient)
	}
	for _, recipient := range note.BCC {
		visit(recipient)
	}

	recipients := make([]string, 0, len(seen))
	for _, recipient := range seen {
		recipients = append(recipients, recipient)
	}
	sort.Strings(recipients)
	return recipients, hasPublic, hasCollection, targetsLocalActor
}

func isInboundDirectCollectionRecipient(recipient string) bool {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	return strings.Contains(recipient, "/followers") ||
		strings.Contains(recipient, "/following") ||
		strings.HasSuffix(recipient, "/collections/featured") ||
		strings.HasSuffix(recipient, "/featured")
}

func (ih *InboxHandler) logUnsupportedInboundDirectGroup(activity *activitypub.Activity, note *activitypub.Note, targetActor *activitypub.Actor, info inboundDirectCreateInfo) {
	if ih.logger == nil {
		return
	}
	ih.logger.Info("unsupported_direct_group",
		zap.String("activity_id", strings.TrimSpace(activity.ID)),
		zap.String("note_id", strings.TrimSpace(note.ID)),
		zap.String("actor_id", strings.TrimSpace(activity.Actor)),
		zap.String("target_actor", strings.TrimSpace(targetActor.ID)),
		zap.Strings("specific_recipients", info.specificRecipients),
		zap.Int("specific_recipient_count", len(info.specificRecipients)))
}

func (ih *InboxHandler) prepareInboundDirectConversation(
	ctx context.Context,
	activity *activitypub.Activity,
	note *activitypub.Note,
	_ *activitypub.Actor,
	info inboundDirectCreateInfo,
) (*models.Conversation, bool, error) {
	if ih.conversationRepository == nil {
		return nil, false, fmt.Errorf("conversation repository not configured")
	}

	participants := models.ConversationParticipantIDsFromRefs(info.participantRefs)
	if len(participants) != 2 {
		return nil, false, fmt.Errorf("inbound direct conversation requires exactly two participants")
	}

	if typedRepo, ok := ih.conversationRepository.(inboundDirectConversationLookupRepository); ok {
		conversation, err := typedRepo.GetConversationByParticipantRefs(ctx, info.participantRefs)
		if err == nil && conversation != nil && strings.TrimSpace(conversation.ID) != "" {
			return conversation, false, nil
		}
		if err != nil && !isInboundDirectNotFound(err) {
			return nil, false, err
		}
	} else {
		conversation, err := ih.conversationRepository.GetConversationByParticipants(ctx, participants)
		if err == nil && conversation != nil && strings.TrimSpace(conversation.ID) != "" {
			return conversation, false, nil
		}
		if err != nil && !isInboundDirectNotFound(err) {
			return nil, false, err
		}
	}

	conversationID := ""
	statusID := models.CanonicalStatusIDForDomain(note.ID, ih.localDomain())
	if statusID != "" && ih.statusRepository != nil {
		if existing, err := ih.statusRepository.GetStatus(ctx, statusID); err == nil && existing != nil {
			conversationID = strings.TrimSpace(existing.ConversationID)
		}
	}
	if conversationID == "" {
		conversationID = uuid.NewString()
	}

	now := remoteCreateNotificationCreatedAt(activity, note, nil)
	return &models.Conversation{
		ID:              conversationID,
		Participants:    participants,
		ParticipantRefs: info.participantRefs,
		CreatedAt:       now,
		UpdatedAt:       now,
		LastMessageTime: now,
	}, true, nil
}

func (ih *InboxHandler) persistInboundDirectConversation(
	ctx context.Context,
	conversation *models.Conversation,
	createConversation bool,
	status *models.Status,
	info inboundDirectCreateInfo,
) error {
	if ih.conversationRepository == nil || conversation == nil || status == nil {
		return nil
	}

	publishedAt := status.PublishedAt.UTC()
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}

	skipConversationMetadata := !createConversation &&
		!conversation.LastMessageTime.IsZero() &&
		publishedAt.Before(conversation.LastMessageTime)

	// CSR-042: Always count unique messages, even when preserving
	// last-message metadata for an older inbound status. The status-level
	// idempotency check in prepareDirectCreateState prevents exact-replay
	// double-counts.
	if conversation.LastStatusID != status.StatusID {
		conversation.TotalMessageCount++
	}

	if !skipConversationMetadata {
		conversation.LastStatusID = status.StatusID
		conversation.LastMessageTime = publishedAt
		conversation.UpdatedAt = publishedAt
	}
	conversation.ParticipantRefs = models.NormalizeConversationParticipantRefs(conversation.ParticipantRefs)
	if len(conversation.ParticipantRefs) == 0 {
		conversation.ParticipantRefs = info.participantRefs
	}
	conversation.Participants = models.ConversationParticipantIDsFromRefs(conversation.ParticipantRefs)

	remoteRef := inboundDirectRemoteParticipantRef(info)
	stateSortAt := publishedAt
	if skipConversationMetadata && !conversation.LastMessageTime.IsZero() {
		// When the incoming message is older, still record the state but
		// anchor sort order to conversation time so the thread stays in place.
		stateSortAt = conversation.LastMessageTime
	}
	state := &models.UserConversationState{
		ViewerID:                 info.localParticipantID,
		ConversationID:           conversation.ID,
		CounterpartID:            info.remoteActorID,
		CounterpartType:          remoteRef.ParticipantType,
		CounterpartAcct:          remoteRef.Acct,
		CounterpartDomain:        remoteRef.Domain,
		CounterpartResolvedAt:    remoteRef.ResolvedAt,
		Folder:                   models.UserConversationFolderInbox,
		RequestState:             models.DmRequestStateAccepted,
		PreviewStatusID:          status.StatusID,
		PreviewStatusPublishedAt: publishedAt,
		SortAt:                   stateSortAt,
		Unread:                   true,
		CreatedAt:                conversation.CreatedAt,
		UpdatedAt:                publishedAt,
	}

	if createConversation {
		if err := ih.conversationRepository.CreateConversationWithParticipantStates(ctx, conversation, conversation.Participants, []*models.UserConversationState{state}); err != nil {
			if !stdErrors.Is(err, storage.ErrAlreadyExists) {
				return err
			}
			createConversation = false
		}
	}
	if !createConversation {
		if err := ih.conversationRepository.UpdateConversation(ctx, conversation); err != nil && !isInboundDirectNotFound(err) {
			return err
		}
		if err := ih.conversationRepository.PutUserConversationState(ctx, state); err != nil {
			return err
		}
	}

	return nil
}

func inboundDirectRemoteParticipantRef(info inboundDirectCreateInfo) models.ConversationParticipantRef {
	for _, ref := range models.NormalizeConversationParticipantRefs(info.participantRefs) {
		if ref.ParticipantType == models.ConversationParticipantTypeRemoteActor {
			return ref
		}
	}
	return models.NormalizeConversationParticipantRef(models.ConversationParticipantRef{
		ParticipantType: models.ConversationParticipantTypeRemoteActor,
		ParticipantID:   info.remoteActorID,
		Acct:            info.remoteAcct,
		Domain:          info.remoteDomain,
	})
}

func isInboundDirectNotFound(err error) bool {
	return isRemoteStatusNotFound(err) || stdErrors.Is(err, storage.ErrNotFound)
}

func (ih *InboxHandler) createRemoteDirectNotification(
	ctx context.Context,
	activity *activitypub.Activity,
	targetActor *activitypub.Actor,
	note *activitypub.Note,
	status *models.Status,
	conversation *models.Conversation,
	info inboundDirectCreateInfo,
) {
	if conversation == nil || status == nil {
		return
	}
	recipient := info.localParticipantID
	if recipient == "" && targetActor != nil {
		recipient = strings.TrimSpace(targetActor.PreferredUsername)
	}
	if recipient == "" || info.remoteActorID == "" {
		return
	}

	ih.createRemoteCreateNotification(ctx, remoteCreateNotificationInput{
		kind:      common.NotificationTypeMention,
		recipient: recipient,
		actorID:   info.remoteActorID,
		activity:  activity,
		note:      note,
		status:    status,
		title:     fmt.Sprintf("%s sent you a direct message", ih.remoteNotificationActorLabel(info.remoteActorID)),
		body:      fmt.Sprintf("%s sent you a direct message", ih.remoteNotificationActorLabel(info.remoteActorID)),
		groupKey:  fmt.Sprintf("remote-direct:%s:%s", recipient, conversation.ID),
		extraData: map[string]interface{}{
			"conversationID":  conversation.ID,
			"conversation_id": conversation.ID,
			"visibility":      models.VisibilityDirect,
			"actorID":         info.remoteActorID,
			"actor_id":        info.remoteActorID,
			"targetID":        status.StatusID,
			"target_id":       status.StatusID,
		},
		stableParts: []string{strings.TrimSpace(activity.ID), recipient},
	})
}

func (ih *InboxHandler) emitInboundDirectConversationEvent(ctx context.Context, conversation *models.Conversation, status *models.Status, localRecipient string) {
	if ih.publisher == nil || conversation == nil || status == nil || localRecipient == "" {
		return
	}

	event := &streaming.Event{
		Type:      "conversation.message",
		Timestamp: time.Now().UTC(),
		Payload: map[string]interface{}{
			"message":      status,
			"conversation": conversation,
		},
	}

	conversationEvent := *event
	conversationEvent.Stream = fmt.Sprintf("conversation:%s", conversation.ID)
	if err := ih.publisher.PublishToConversation(ctx, conversation.ID, &conversationEvent); err != nil && ih.logger != nil {
		ih.logger.Warn("failed to publish inbound direct conversation event",
			zap.String("conversation_id", conversation.ID),
			zap.Error(err))
	}

	userEvent := *event
	userEvent.Stream = fmt.Sprintf("user:%s:direct", localRecipient)
	if err := ih.publisher.PublishToUser(ctx, localRecipient, &userEvent); err != nil && ih.logger != nil {
		ih.logger.Warn("failed to publish inbound direct user event",
			zap.String("conversation_id", conversation.ID),
			zap.String("recipient", localRecipient),
			zap.Error(err))
	}
}

func (ih *InboxHandler) createRemoteCreateNotifications(
	ctx context.Context,
	activity *activitypub.Activity,
	targetActor *activitypub.Actor,
	note *activitypub.Note,
	status *models.Status,
) {
	if ih.notificationRepository == nil || activity == nil || targetActor == nil || note == nil || status == nil {
		return
	}

	recipient := strings.TrimSpace(targetActor.PreferredUsername)
	if recipient == "" {
		recipient = ih.extractUsernameFromActorID(targetActor.ID)
	}
	if recipient == "" {
		return
	}

	actorID := strings.TrimSpace(activity.Actor)
	if actorID == "" {
		actorID = strings.TrimSpace(note.AttributedTo)
	}
	if actorID == "" || strings.EqualFold(actorID, strings.TrimSpace(targetActor.ID)) {
		return
	}

	if ih.remoteNoteMentionsTargetActor(note, targetActor) {
		ih.createRemoteCreateNotification(ctx, remoteCreateNotificationInput{
			kind:        common.NotificationTypeMention,
			recipient:   recipient,
			actorID:     actorID,
			activity:    activity,
			note:        note,
			status:      status,
			title:       fmt.Sprintf("%s mentioned you", ih.remoteNotificationActorLabel(actorID)),
			body:        fmt.Sprintf("%s mentioned you", ih.remoteNotificationActorLabel(actorID)),
			groupKey:    fmt.Sprintf("remote-mention:%s:%s", recipient, status.StatusID),
			extraData:   map[string]interface{}{"mentioner": actorID},
			stableParts: []string{recipient, actorID, strings.TrimSpace(activity.ID), strings.TrimSpace(note.ID), status.StatusID},
		})
	}

	parentStatus, parentID := ih.remoteCreateReplyParent(ctx, note, targetActor)
	if parentStatus == nil || parentID == "" {
		return
	}

	ih.createRemoteCreateNotification(ctx, remoteCreateNotificationInput{
		kind:      common.NotificationTypeReply,
		recipient: recipient,
		actorID:   actorID,
		activity:  activity,
		note:      note,
		status:    status,
		title:     fmt.Sprintf("%s replied to your post", ih.remoteNotificationActorLabel(actorID)),
		body:      fmt.Sprintf("%s replied to your post", ih.remoteNotificationActorLabel(actorID)),
		groupKey:  fmt.Sprintf("remote-reply:%s:%s:%s", recipient, parentID, status.StatusID),
		extraData: map[string]interface{}{
			"parent_status_id": parentID,
			"replier":          actorID,
		},
		stableParts: []string{recipient, actorID, strings.TrimSpace(activity.ID), strings.TrimSpace(note.ID), status.StatusID, parentID},
	})
}

type remoteCreateNotificationInput struct {
	kind        string
	recipient   string
	actorID     string
	activity    *activitypub.Activity
	note        *activitypub.Note
	status      *models.Status
	title       string
	body        string
	groupKey    string
	extraData   map[string]interface{}
	stableParts []string
}

func (ih *InboxHandler) createRemoteCreateNotification(ctx context.Context, input remoteCreateNotificationInput) {
	if input.status == nil || input.note == nil || input.activity == nil {
		return
	}

	createdAt := remoteCreateNotificationCreatedAt(input.activity, input.note, input.status)
	notification := models.NewNotificationBuilder().
		ForUser(input.recipient).
		OfType(input.kind).
		FromActor(input.actorID, "remote_actor").
		AboutTarget(input.status.StatusID, "status").
		WithContent(input.title, input.body).
		WithGroupKey(input.groupKey).
		Build()
	notification.ID = deterministicRemoteCreateNotificationID(input.kind, input.stableParts...)
	notification.CreatedAt = createdAt
	notification.UpdatedAt = createdAt

	if notification.Data == nil {
		notification.Data = make(map[string]interface{})
	}
	for key, value := range input.extraData {
		notification.Data[key] = value
	}
	notification.Data["activity_id"] = strings.TrimSpace(input.activity.ID)
	notification.Data["remote_actor_id"] = input.actorID
	notification.Data["remote_note_id"] = strings.TrimSpace(input.note.ID)
	notification.Data["status_id"] = input.status.StatusID
	notification.Data["status_url"] = strings.TrimSpace(input.note.ID)
	notification.Data["postSnapshot"] = remoteCreatePostSnapshot(input.note, input.status)

	if err := ih.notificationRepository.CreateNotification(ctx, notification); err != nil {
		if isRemoteCreateNotificationDuplicate(err) {
			ih.logger.Debug("remote create notification already exists",
				zap.String("notification_id", notification.ID),
				zap.String("notification_type", input.kind),
				zap.String("recipient", input.recipient))
			return
		}
		ih.logger.Warn("failed to create remote create notification",
			zap.String("notification_id", notification.ID),
			zap.String("notification_type", input.kind),
			zap.String("recipient", input.recipient),
			zap.String("activity_id", input.activity.ID),
			zap.Error(err))
		return
	}

	ih.logger.Info("created remote create notification",
		zap.String("notification_id", notification.ID),
		zap.String("notification_type", input.kind),
		zap.String("recipient", input.recipient),
		zap.String("activity_id", input.activity.ID))
}

func remoteCreateNotificationCreatedAt(activity *activitypub.Activity, note *activitypub.Note, status *models.Status) time.Time {
	switch {
	case note != nil && note.Published != nil && !note.Published.IsZero():
		return note.Published.UTC()
	case activity != nil && activity.Published != nil && !activity.Published.IsZero():
		return activity.Published.UTC()
	case status != nil && !status.PublishedAt.IsZero():
		return status.PublishedAt.UTC()
	case status != nil && !status.CreatedAt.IsZero():
		return status.CreatedAt.UTC()
	default:
		return time.Now().UTC()
	}
}

func deterministicRemoteCreateNotificationID(kind string, parts ...string) string {
	normalizedKind := strings.ToLower(strings.TrimSpace(kind))
	var normalized strings.Builder
	normalized.WriteString(normalizedKind)
	for _, part := range parts {
		normalized.WriteByte('\x00')
		normalized.WriteString(strings.TrimSpace(part))
	}
	sum := sha256.Sum256([]byte(normalized.String()))
	return fmt.Sprintf("remote-create-%s-%s", normalizedKind, hex.EncodeToString(sum[:]))
}

func remoteCreatePostSnapshot(note *activitypub.Note, status *models.Status) map[string]interface{} {
	snapshot := map[string]interface{}{}
	if note != nil {
		snapshot["id"] = strings.TrimSpace(note.ID)
		snapshot["content"] = note.Content
		snapshot["attributedTo"] = strings.TrimSpace(note.AttributedTo)
		snapshot["url"] = strings.TrimSpace(note.ID)
		if note.InReplyTo != "" {
			snapshot["inReplyToId"] = strings.TrimSpace(note.InReplyTo)
		}
		if note.Visibility != "" {
			snapshot["visibility"] = strings.TrimSpace(note.Visibility)
		}
		if note.Published != nil && !note.Published.IsZero() {
			snapshot["createdAt"] = note.Published.UTC().Format(time.RFC3339)
		}
	}
	if status != nil {
		snapshot["statusId"] = status.StatusID
		if snapshot["visibility"] == nil && status.Visibility != "" {
			snapshot["visibility"] = status.Visibility
		}
		if snapshot["createdAt"] == nil && !status.PublishedAt.IsZero() {
			snapshot["createdAt"] = status.PublishedAt.UTC().Format(time.RFC3339)
		}
	}
	return snapshot
}

func (ih *InboxHandler) remoteNoteMentionsTargetActor(note *activitypub.Note, targetActor *activitypub.Actor) bool {
	if note == nil || targetActor == nil {
		return false
	}

	targetIDs := ih.localTargetActorIDs(targetActor)
	targetUsername := strings.ToLower(strings.TrimSpace(targetActor.PreferredUsername))
	targetDomain := ih.localDomain()

	for _, tag := range note.Tag {
		if !strings.EqualFold(strings.TrimSpace(tag.Type), "Mention") {
			continue
		}

		href := strings.TrimSpace(tag.Href)
		if href != "" {
			if _, ok := targetIDs[strings.ToLower(strings.TrimRight(href, "/"))]; ok {
				return true
			}
			if username, domain := actorURLUsernameDomain(href); username != "" &&
				strings.EqualFold(username, targetUsername) &&
				(targetDomain == "" || strings.EqualFold(domain, targetDomain)) {
				return true
			}
		}

		name := strings.TrimSpace(tag.Name)
		if name == "" || targetUsername == "" {
			continue
		}
		name = strings.TrimPrefix(name, "@")
		parts := strings.Split(name, "@")
		if len(parts) == 1 && strings.EqualFold(parts[0], targetUsername) {
			return true
		}
		if len(parts) >= 2 && strings.EqualFold(parts[0], targetUsername) &&
			(targetDomain == "" || strings.EqualFold(parts[len(parts)-1], targetDomain)) {
			return true
		}
	}

	return false
}

func (ih *InboxHandler) localTargetActorIDs(targetActor *activitypub.Actor) map[string]struct{} {
	targets := make(map[string]struct{}, 2)
	add := func(value string) {
		value = strings.ToLower(strings.TrimRight(strings.TrimSpace(value), "/"))
		if value != "" {
			targets[value] = struct{}{}
		}
	}
	if targetActor != nil {
		add(targetActor.ID)
		if username := strings.TrimSpace(targetActor.PreferredUsername); username != "" {
			add(strings.TrimRight(ih.baseURL, "/") + "/users/" + username)
		}
	}
	return targets
}

func (ih *InboxHandler) remoteCreateReplyParent(ctx context.Context, note *activitypub.Note, targetActor *activitypub.Actor) (*models.Status, string) {
	if ih.statusRepository == nil || note == nil || strings.TrimSpace(note.InReplyTo) == "" || targetActor == nil {
		return nil, ""
	}

	for _, candidate := range models.StatusLookupCandidatesForDomain(note.InReplyTo, ih.localDomain()) {
		parent, err := ih.statusRepository.GetStatus(ctx, candidate)
		if err != nil {
			if !isRemoteStatusNotFound(err) {
				ih.logger.Debug("failed to resolve remote create reply parent for notification",
					zap.String("in_reply_to", note.InReplyTo),
					zap.String("candidate", candidate),
					zap.Error(err))
			}
			continue
		}
		if ih.replyParentBelongsToTarget(parent, targetActor) {
			return parent, parent.StatusID
		}
	}

	return nil, ""
}

func (ih *InboxHandler) replyParentBelongsToTarget(parent *models.Status, targetActor *activitypub.Actor) bool {
	if parent == nil || targetActor == nil {
		return false
	}

	targetID := strings.TrimRight(strings.TrimSpace(targetActor.ID), "/")
	targetUsername := strings.TrimSpace(targetActor.PreferredUsername)
	if targetID != "" && strings.EqualFold(strings.TrimRight(strings.TrimSpace(parent.AuthorID), "/"), targetID) {
		return true
	}
	if parent.Note != nil && targetID != "" && strings.EqualFold(strings.TrimRight(strings.TrimSpace(parent.Note.AttributedTo), "/"), targetID) {
		return true
	}
	if targetUsername != "" && strings.EqualFold(strings.TrimSpace(parent.AuthorUsername), targetUsername) {
		return true
	}
	return false
}

func (ih *InboxHandler) remoteNotificationActorLabel(actorID string) string {
	if handle := ih.extractHandleFromActorID(actorID); handle != "" {
		return handle
	}
	if username := ih.extractUsernameFromActorID(actorID); username != "" {
		return username
	}
	return strings.TrimSpace(actorID)
}

func (ih *InboxHandler) localDomain() string {
	if cfg := ih.getConfig(); cfg != nil && strings.TrimSpace(cfg.Domain) != "" {
		return strings.TrimSpace(cfg.Domain)
	}
	if parsed, err := url.Parse(strings.TrimSpace(ih.baseURL)); err == nil && parsed != nil {
		return parsed.Hostname()
	}
	return ""
}

func actorURLUsernameDomain(raw string) (string, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return "", ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 {
		return "", parsed.Hostname()
	}
	username := strings.TrimPrefix(parts[len(parts)-1], "@")
	return username, parsed.Hostname()
}

func isRemoteCreateNotificationDuplicate(err error) bool {
	if err == nil {
		return false
	}
	return errors.HasCode(err, errors.CodeAlreadyExists) ||
		errors.HasCode(err, errors.CodeConflict) ||
		stdErrors.Is(err, storage.ErrAlreadyExists) ||
		dynamormerrors.IsConditionFailed(err) ||
		strings.Contains(strings.ToLower(err.Error()), "already exists")
}

// processRemoteUpdateActivity processes an incoming Update activity from a remote instance
func (ih *InboxHandler) processRemoteUpdateActivity(ctx context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	log := common.WithContext(ctx)

	log.Info("processing remote update activity",
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor))

	// Extract the object
	objMap, ok := activity.Object.(map[string]any)
	if !ok {
		log.Warn("update activity has invalid object")
		return nil
	}

	// Get object ID for authorization and history tracking
	objectID, ok := objMap["id"].(string)
	if !ok || common.ValidateRequiredParam("objectID", objectID) != nil {
		log.Warn("update activity object has no ID")
		return nil
	}

	// Check if we have the original object
	existingObject, err := ih.objectRepository.GetObject(ctx, objectID)
	if err != nil {
		log.Debug("object not found for update, ignoring",
			zap.String("object_id", objectID),
			zap.Error(err))
		return nil // We can't update what we don't have
	}

	// Verify authorization - only object owner can update
	if err := ih.verifyUpdateAuthorization(ctx, activity, existingObject); err != nil {
		log.Warn("update authorization failed",
			zap.String("object_id", objectID),
			zap.String("actor", activity.Actor),
			zap.Error(err))
		return err
	}

	// Sanitize the object content to prevent XSS
	common.SanitizeActivityPubObjectDefault(objMap)

	// Update the object if it's a Note
	objType, _ := objMap["type"].(string)
	switch strings.TrimSpace(objType) {
	case activitypub.NoteType:
		// Updates carry a complete Note just like Creates. Validate before edit
		// history or object writes so missing attribution cannot blank the author.
		if err := common.ValidateActivityPubNote(objMap); err != nil {
			log.Warn("invalid note object in update activity", zap.Error(err))
			return invalidNoteError()
		}

		// Convert to Note object
		objJSON, err := json.Marshal(objMap)
		if err != nil {
			return err
		}

		var note activitypub.Note
		if err := common.ParseActivityPubObject(objJSON, &note); err != nil {
			return err
		}
		// Remote projection owns the local-author invariant for every ingestion
		// route. Check it before mutating the stored object or edit history.
		if ih.buildCanonicalRemoteStatus(&note) == nil {
			return invalidNoteError()
		}

		// Store edit history before updating supported Note objects.
		if err := ih.storeEditHistory(ctx, objectID, existingObject, activity.Actor); err != nil {
			log.Error("failed to store edit history",
				zap.String("object_id", objectID),
				zap.Error(err))
			// Continue even if history storage fails
		}

		// Set updated timestamp
		now := time.Now()
		note.Updated = &now

		// Update the note with edit tracking
		if err := ih.objectRepository.UpdateObjectWithHistory(ctx, &note, activity.Actor); err != nil {
			log.Error("failed to update remote note",
				zap.String("object_id", objectID),
				zap.Error(err))
			return err
		}

		if err := ih.upsertRemoteNoteStatus(ctx, &note); err != nil {
			log.Error("failed to refresh canonical remote note status",
				zap.String("object_id", objectID),
				zap.Error(err))
			return err
		}

		log.Info("successfully updated remote note",
			zap.String("object_id", objectID),
			zap.String("updated_by", activity.Actor))
	case activitypub.ArticleType:
		ih.logUnsupportedRemoteArticle(ctx, activity, objMap, "update")
	default:
		log.Info("ignoring unsupported remote update object type",
			zap.String("activity_id", activity.ID),
			zap.String("actor", activity.Actor),
			zap.String("object_id", objectID),
			zap.String("object_type", objType))
	}

	return nil
}

// processRemoteDeleteActivity processes an incoming Delete activity from a remote instance
// Implements proper tombstone creation and cascade deletion per ActivityPub specification
func (ih *InboxHandler) processRemoteDeleteActivity(ctx context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	log := common.WithContext(ctx)

	log.Info("processing remote delete activity",
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.String("activity_type", activity.Type))

	// Extract object ID to delete
	objectID, originalObject, err := ih.extractDeleteTarget(activity)
	if err != nil {
		log.Warn("failed to extract delete target", zap.Error(err))
		return nil // Don't fail on malformed activities
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		log.Warn("delete activity has no object ID")
		return nil
	}

	// Verify the object exists and get its details
	existingObject, err := ih.objectRepository.GetObject(ctx, objectID)
	if err != nil {
		log.Debug("object not found for deletion - may already be deleted",
			zap.String("object_id", objectID),
			zap.Error(err))
		// This is idempotent - deleting an already deleted object succeeds
		return nil
	}

	// Verify authorization - only object owner can delete
	if err := ih.verifyDeleteAuthorization(ctx, activity, existingObject); err != nil {
		log.Warn("delete authorization failed",
			zap.String("object_id", objectID),
			zap.String("actor", activity.Actor),
			zap.Error(err))
		return err
	}

	// Perform cascade deletion operations
	if err := ih.cascadeDeleteOperations(ctx, objectID, activity.Actor); err != nil {
		log.Error("failed to perform cascade deletions",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Continue - partial failure is acceptable for remote deletes
	}

	// Create tombstone (soft delete) instead of hard delete
	if err := ih.createDeleteTombstone(ctx, objectID, activity, originalObject, existingObject); err != nil {
		log.Error("failed to create tombstone",
			zap.String("object_id", objectID),
			zap.Error(err))
		return err
	}

	if err := ih.deleteRemoteNoteStatus(ctx, objectID); err != nil {
		log.Error("failed to tombstone canonical remote note status",
			zap.String("object_id", objectID),
			zap.Error(err))
		return err
	}

	log.Info("successfully processed remote delete activity",
		zap.String("object_id", objectID),
		zap.String("actor", activity.Actor))

	return nil
}

// processLikeActivity processes an incoming Like activity
func (ih *InboxHandler) processLikeActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Check for block relationship before processing like
	if err := ih.checkBlockStatus(ctx, activity.Actor, targetActor.ID); err != nil {
		log.Warn("like activity blocked due to block relationship",
			zap.String("liker", activity.Actor),
			zap.String("target", targetActor.ID))
		// Return success to avoid revealing block status to blocked actor
		return nil
	}

	// Extract and canonicalize the object being liked when it maps to a local
	// known status. Unknown remote objects remain permissive and are stored as
	// received.
	objectID := extractObjectID(activity.Object)

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		log.Warn("like activity has no object ID")
		return nil // Don't fail, just ignore malformed likes
	}
	objectID, obj := ih.canonicalizeKnownInteractionObject(ctx, objectID)

	// Extract actor handle from actor ID
	actorHandle := ih.extractHandleFromActorID(activity.Actor)

	log.Info("processing like activity",
		zap.String("actor", activity.Actor),
		zap.String("actor_handle", actorHandle),
		zap.String("object", objectID),
		zap.String("activity_id", activity.ID))

	// Store the like
	_, err := ih.likeRepository.CreateLike(ctx, activity.Actor, objectID, targetActor.ID)
	if err != nil {
		log.Error("failed to create like",
			zap.String("actor", activity.Actor),
			zap.String("object", objectID),
			zap.Error(err))
		return createLikeError()
	}

	// Send notification if this is a local object
	if obj != nil {
		// Extract liker username from actor ID
		likerUsername := ih.extractUsernameFromActorID(activity.Actor)
		if likerUsername != "" {
			// Create favourite notification for the object owner
			favouriteNotif := models.NewFavouriteNotification(targetActor.PreferredUsername, likerUsername, objectID)
			if err := ih.notificationRepository.CreateNotification(ctx, favouriteNotif); err != nil {
				log.Warn("failed to create like notification", zap.Error(err))
				// Don't fail the whole operation if notification fails
			} else {
				log.Info("created like notification",
					zap.String("notification_id", favouriteNotif.ID),
					zap.String("object_owner", targetActor.PreferredUsername),
					zap.String("liker", likerUsername))
			}
		}
	}

	log.Info("successfully processed like activity",
		zap.String("actor", activity.Actor),
		zap.String("object", objectID))

	return nil
}

// processAnnounceActivity processes an incoming Announce activity (boost/reblog)
func (ih *InboxHandler) processAnnounceActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Check for block relationship before processing announce
	if err := ih.checkBlockStatus(ctx, activity.Actor, targetActor.ID); err != nil {
		log.Warn("announce activity blocked due to block relationship",
			zap.String("announcer", activity.Actor),
			zap.String("target", targetActor.ID))
		// Return success to avoid revealing block status to blocked actor
		return nil
	}

	// Extract and canonicalize the object being announced when it maps to a
	// local known status. Unknown remote objects remain permissive and are
	// stored as received.
	objectID := extractObjectID(activity.Object)

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		log.Warn("announce activity has no object ID")
		return nil // Don't fail, just ignore malformed announces
	}
	objectID, obj := ih.canonicalizeKnownInteractionObject(ctx, objectID)

	// Extract actor handle from actor ID
	actorHandle := ih.extractHandleFromActorID(activity.Actor)

	log.Info("processing announce activity",
		zap.String("actor", activity.Actor),
		zap.String("actor_handle", actorHandle),
		zap.String("object", objectID),
		zap.String("activity_id", activity.ID))

	// Create storage.Announce struct for SocialRepository
	announce := &storage.Announce{
		Actor:     activity.Actor,
		Object:    objectID,
		ID:        activity.ID,
		Published: time.Now(),
		CreatedAt: time.Now(),
		To:        activity.To,
		CC:        activity.CC,
	}

	// Store the announce using social repository
	err := ih.socialRepository.CreateAnnounce(ctx, announce)
	if err != nil {
		// Handle duplicate announces gracefully
		if strings.Contains(err.Error(), "already announced") {
			log.Debug("announce already exists",
				zap.String("actor", activity.Actor),
				zap.String("object", objectID))
			return nil // Idempotent - don't fail
		}
		log.Error("failed to create announce",
			zap.String("actor", activity.Actor),
			zap.String("object", objectID),
			zap.Error(err))
		return createAnnounceError()
	}

	// Send notification if this is a local object
	if obj != nil {
		// Extract reblogger username from actor ID
		rebloggerUsername := ih.extractUsernameFromActorID(activity.Actor)
		if rebloggerUsername != "" {
			// Create reblog notification for the object owner
			reblogNotif := models.NewReblogNotification(targetActor.PreferredUsername, rebloggerUsername, objectID)
			if err := ih.notificationRepository.CreateNotification(ctx, reblogNotif); err != nil {
				log.Warn("failed to create announce notification", zap.Error(err))
				// Don't fail the whole operation if notification fails
			} else {
				log.Info("created announce notification",
					zap.String("notification_id", reblogNotif.ID),
					zap.String("object_owner", targetActor.PreferredUsername),
					zap.String("reblogger", rebloggerUsername))
			}
		}
	}

	log.Info("successfully processed announce activity",
		zap.String("actor", activity.Actor),
		zap.String("object", objectID))

	return nil
}

func (ih *InboxHandler) canonicalizeKnownInteractionObject(ctx context.Context, objectID string) (string, any) {
	objectID = strings.TrimSpace(objectID)
	if objectID == "" {
		return "", nil
	}

	if status := ih.resolveKnownInteractionStatus(ctx, objectID); status != nil {
		if canonical := models.CanonicalActivityPubObjectIDForStatus(status, ih.baseURL); canonical != "" {
			if status.Note != nil {
				return canonical, status.Note
			}
			return canonical, status
		}
	}

	if ih.objectRepository == nil {
		return objectID, nil
	}
	obj, err := ih.objectRepository.GetObject(ctx, objectID)
	if err != nil {
		common.WithContext(ctx).Debug("object not found for interaction, assuming unknown remote object",
			zap.String("object_id", objectID),
			zap.Error(err))
		return objectID, nil
	}

	return objectID, obj
}

func (ih *InboxHandler) resolveKnownInteractionStatus(ctx context.Context, objectID string) *models.Status {
	if ih.statusRepository == nil {
		return nil
	}

	if status, err := ih.statusRepository.GetStatusByURL(ctx, objectID); err == nil &&
		models.CanonicalActivityPubObjectIDForStatus(status, ih.baseURL) != "" {
		return status
	}

	for _, candidate := range models.StatusLookupCandidatesForDomain(objectID, ih.baseURL) {
		status, err := ih.statusRepository.GetStatus(ctx, candidate)
		if err == nil && models.CanonicalActivityPubObjectIDForStatus(status, ih.baseURL) != "" {
			return status
		}
	}

	return nil
}

// processUndoActivity processes an incoming Undo activity
func (ih *InboxHandler) processUndoActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Get the activity being undone
	var originalActivity *activitypub.Activity

	switch obj := activity.Object.(type) {
	case string:
		// Fetch the activity by ID
		var err error
		originalActivity, err = ih.activityRepository.GetActivity(ctx, obj)
		if err != nil {
			log.Warn("failed to find activity to undo", zap.String("id", obj))
			return nil
		}
		if validationErr := ih.ensureStoredActivityHydrated(ctx, obj, originalActivity); validationErr != nil {
			return validationErr
		}
	case map[string]any:
		// Convert to activity
		objJSON, err := json.Marshal(obj)
		if err != nil {
			return err
		}

		originalActivity = &activitypub.Activity{}
		if err := common.ParseActivityPubObject(objJSON, originalActivity); err != nil {
			return err
		}
	default:
		log.Warn("undo activity has invalid object")
		return nil
	}

	// Process based on the original activity type
	switch originalActivity.Type {
	case activitypub.FollowType:
		// Undo follow
		unfollowerHandle := ih.extractHandleFromActorID(activity.Actor)
		err := ih.relationshipRepository.DeleteRelationship(ctx, unfollowerHandle, targetActor.PreferredUsername)
		if err != nil {
			log.Error("failed to remove follow", zap.Error(err))
			return err
		}
	case activitypub.LikeType:
		// Undo like
		if objectID := extractObjectID(originalActivity.Object); objectID != "" {
			objectID, _ = ih.canonicalizeKnownInteractionObject(ctx, objectID)
			err := ih.likeRepository.DeleteLike(ctx, activity.Actor, objectID)
			if err != nil {
				log.Warn("failed to remove like", zap.Error(err))
				// Don't fail - idempotent operation
			}
		}
	case activitypub.AnnounceType:
		// Undo announce (boost/reblog)
		if objectID := extractObjectID(originalActivity.Object); objectID != "" {
			objectID, _ = ih.canonicalizeKnownInteractionObject(ctx, objectID)
			err := ih.socialRepository.DeleteAnnounce(ctx, activity.Actor, objectID)
			if err != nil {
				log.Warn("failed to remove announce", zap.Error(err))
				// Don't fail - idempotent operation
			}
			log.Info("successfully processed undo announce",
				zap.String("actor", activity.Actor),
				zap.String("object", objectID))
		}
	case activitypub.BlockType:
		// Undo block
		if err := ih.processUndoBlock(ctx, activity, originalActivity); err != nil {
			log.Error("failed to process undo block", zap.Error(err))
			return err
		}
	}

	return nil
}

// processUndoBlock processes undoing a Block activity
func (ih *InboxHandler) processUndoBlock(ctx context.Context, undoActivity *activitypub.Activity, blockActivity *activitypub.Activity) error {
	log := common.WithContext(ctx)

	// Extract the blocked actor ID from the original block activity
	var blockedActorID string
	switch obj := blockActivity.Object.(type) {
	case string:
		blockedActorID = obj
	case map[string]any:
		if id, ok := obj["id"].(string); ok {
			blockedActorID = id
		}
	}

	if err := common.ValidateRequiredParam("blockedActorID", blockedActorID); err != nil {
		log.Warn("undo block activity has no object ID")
		return nil
	}

	// The blocker is the activity actor (same as the undo actor)
	blockerActorID := undoActivity.Actor

	log.Info("processing undo block activity",
		zap.String("blocker", blockerActorID),
		zap.String("blocked", blockedActorID),
		zap.String("block_id", blockActivity.ID),
		zap.String("undo_id", undoActivity.ID))

	// Verify authorization - only the original blocker can undo their block
	if blockActivity.Actor != undoActivity.Actor {
		log.Warn("unauthorized undo block attempt",
			zap.String("original_blocker", blockActivity.Actor),
			zap.String("undo_actor", undoActivity.Actor))
		return unauthorizedBlockUndoError()
	}

	// Remove the block relationship
	if err := ih.relationshipRepository.DeleteBlock(ctx, blockerActorID, blockedActorID); err != nil {
		log.Error("failed to delete block relationship during undo",
			zap.String("blocker", blockerActorID),
			zap.String("blocked", blockedActorID),
			zap.Error(err))
		return deleteBlockError()
	}

	log.Info("successfully processed undo block activity",
		zap.String("blocker", blockerActorID),
		zap.String("blocked", blockedActorID))

	return nil
}

// processBlockActivity processes an incoming Block activity
func (ih *InboxHandler) processBlockActivity(ctx context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Extract the object being blocked (should be an actor ID)
	var blockedActorID string
	switch obj := activity.Object.(type) {
	case string:
		blockedActorID = obj
	case map[string]any:
		if id, ok := obj["id"].(string); ok {
			blockedActorID = id
		}
	}

	if err := common.ValidateRequiredParam("blockedActorID", blockedActorID); err != nil {
		log.Warn("block activity has no object ID")
		return nil // Don't fail on malformed blocks
	}

	// The blocker is the activity actor
	blockerActorID := activity.Actor

	log.Info("processing block activity",
		zap.String("blocker", blockerActorID),
		zap.String("blocked", blockedActorID),
		zap.String("activity_id", activity.ID))

	// Create the block relationship
	if err := ih.relationshipRepository.CreateBlock(ctx, blockerActorID, blockedActorID, activity.ID); err != nil {
		log.Error("failed to create block relationship",
			zap.String("blocker", blockerActorID),
			zap.String("blocked", blockedActorID),
			zap.Error(err))
		return createBlockError()
	}

	// Remove any existing follow relationships in both directions
	if err := ih.removeFollowRelationshipsForBlock(ctx, blockerActorID, blockedActorID); err != nil {
		log.Warn("failed to remove follow relationships during block",
			zap.String("blocker", blockerActorID),
			zap.String("blocked", blockedActorID),
			zap.Error(err))
		// Don't fail the block operation if cleanup fails
	}

	// Remove likes and announces from blocked actor on blocker's content
	if err := ih.removeInteractionsFromBlockedActor(ctx, blockerActorID, blockedActorID); err != nil {
		log.Warn("failed to remove interactions during block",
			zap.String("blocker", blockerActorID),
			zap.String("blocked", blockedActorID),
			zap.Error(err))
		// Don't fail the block operation if cleanup fails
	}

	log.Info("successfully processed block activity",
		zap.String("blocker", blockerActorID),
		zap.String("blocked", blockedActorID))

	return nil
}

// removeFollowRelationshipsForBlock removes follow relationships between two actors during a block
func (ih *InboxHandler) removeFollowRelationshipsForBlock(ctx context.Context, actor1, actor2 string) error {
	log := common.WithContext(ctx)

	// Extract usernames from actor IDs for relationship operations
	username1 := ih.extractHandleFromActorID(actor1)
	username2 := ih.extractHandleFromActorID(actor2)

	// Remove follow in both directions
	if err := ih.relationshipRepository.DeleteRelationship(ctx, username1, username2); err != nil {
		log.Debug("no follow relationship to remove",
			zap.String("follower", username1),
			zap.String("following", username2),
			zap.Error(err))
	}

	if err := ih.relationshipRepository.DeleteRelationship(ctx, username2, username1); err != nil {
		log.Debug("no follow relationship to remove",
			zap.String("follower", username2),
			zap.String("following", username1),
			zap.Error(err))
	}

	log.Debug("removed follow relationships for block",
		zap.String("actor1", actor1),
		zap.String("actor2", actor2))

	return nil
}

// removeInteractionsFromBlockedActor removes likes, announces, etc. from blocked actor on blocker's content
func (ih *InboxHandler) removeInteractionsFromBlockedActor(ctx context.Context, blockerActorID, blockedActorID string) error {
	log := common.WithContext(ctx)

	// This is a placeholder for comprehensive interaction cleanup
	// In a full implementation, you would:
	// 1. Remove all likes by blockedActor on blockerActor's content
	// 2. Remove all announces by blockedActor on blockerActor's content
	// 3. Remove any other interactions (replies, mentions, etc.)

	log.Debug("cleaned up interactions from blocked actor",
		zap.String("blocker", blockerActorID),
		zap.String("blocked_actor", blockedActorID))

	return nil
}

// processAddActivity processes an incoming Add activity for collection management
func (ih *InboxHandler) processAddActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	log.Info("processing add activity",
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.String("target", activity.Target))

	// Validate required fields
	if err := common.ValidateRequiredParam("activityTarget", activity.Target); err != nil {
		log.Warn("add activity missing target collection")
		return addNoTargetError()
	}

	if activity.Object == nil {
		log.Warn("add activity missing object")
		return addNoObjectError()
	}

	// Extract object ID to add
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]any:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		log.Warn("add activity object has no ID")
		return addObjectNoIDError()
	}

	// Extract collection type from target URL
	collectionType, err := ih.extractCollectionType(activity.Target)
	if err != nil {
		log.Warn("failed to extract collection type from target",
			zap.String("target", activity.Target),
			zap.Error(err))
		return invalidCollectionTargetError()
	}

	// Verify authorization - only the collection owner can add items
	if err := ih.verifyCollectionAuthorization(ctx, activity, collectionType, targetActor); err != nil {
		log.Warn("add activity authorization failed",
			zap.String("actor", activity.Actor),
			zap.String("collection", collectionType),
			zap.Error(err))
		return unauthorizedAddError()
	}

	// Determine object type (for metadata)
	objectType := "Object" // Default
	if objMap, ok := activity.Object.(map[string]any); ok {
		if objTypeStr, ok := objMap["type"].(string); ok {
			objectType = objTypeStr
		}
	}

	log.Info("adding item to collection",
		zap.String("collection", collectionType),
		zap.String("object_id", objectID),
		zap.String("object_type", objectType),
		zap.String("added_by", activity.Actor))

	// Add item to the collection using object repository
	if err := ih.objectRepository.AddToCollection(ctx, collectionType, &storage.CollectionItem{
		ItemID:   objectID,
		ItemType: objectType,
		AddedBy:  activity.Actor,
		AddedAt:  time.Now(),
	}); err != nil {
		log.Error("failed to add item to collection",
			zap.String("collection", collectionType),
			zap.String("object_id", objectID),
			zap.Error(err))
		return addItemFailedError()
	}

	// Update collection counters if needed
	if err := ih.updateCollectionCounters(ctx, collectionType, targetActor.PreferredUsername, 1); err != nil {
		log.Warn("failed to update collection counters", zap.Error(err))
		// Don't fail the operation for counter updates
	}

	log.Info("successfully processed add activity",
		zap.String("collection", collectionType),
		zap.String("object_id", objectID),
		zap.String("actor", activity.Actor))

	return nil
}

// processRemoveActivity processes an incoming Remove activity for collection management
func (ih *InboxHandler) processRemoveActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	log.Info("processing remove activity",
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.String("target", activity.Target))

	// Validate required fields
	if err := common.ValidateRequiredParam("activityTarget", activity.Target); err != nil {
		log.Warn("remove activity missing target collection")
		return removeNoTargetError()
	}

	if activity.Object == nil {
		log.Warn("remove activity missing object")
		return removeNoObjectError()
	}

	// Extract object ID to remove
	var objectID string
	switch obj := activity.Object.(type) {
	case string:
		objectID = obj
	case map[string]any:
		if id, ok := obj["id"].(string); ok {
			objectID = id
		}
	}

	if err := common.ValidateRequiredParam("objectID", objectID); err != nil {
		log.Warn("remove activity object has no ID")
		return removeObjectNoIDError()
	}

	// Extract collection type from target URL
	collectionType, err := ih.extractCollectionType(activity.Target)
	if err != nil {
		log.Warn("failed to extract collection type from target",
			zap.String("target", activity.Target),
			zap.Error(err))
		return invalidCollectionTargetError()
	}

	// Verify authorization - only the collection owner can remove items
	if err := ih.verifyCollectionAuthorization(ctx, activity, collectionType, targetActor); err != nil {
		log.Warn("remove activity authorization failed",
			zap.String("actor", activity.Actor),
			zap.String("collection", collectionType),
			zap.Error(err))
		return unauthorizedRemoveError()
	}

	log.Info("removing item from collection",
		zap.String("collection", collectionType),
		zap.String("object_id", objectID),
		zap.String("removed_by", activity.Actor))

	// Remove item from the collection using object repository
	if err := ih.objectRepository.RemoveFromCollection(ctx, collectionType, objectID); err != nil {
		// Handle the case where item doesn't exist (idempotent operation)
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not in collection") {
			log.Debug("item not in collection during remove (idempotent)",
				zap.String("collection", collectionType),
				zap.String("object_id", objectID))
			return nil // Success - removing non-existent item is idempotent
		}

		log.Error("failed to remove item from collection",
			zap.String("collection", collectionType),
			zap.String("object_id", objectID),
			zap.Error(err))
		return removeItemFailedError()
	}

	// Update collection counters if needed
	if err := ih.updateCollectionCounters(ctx, collectionType, targetActor.PreferredUsername, -1); err != nil {
		log.Warn("failed to update collection counters", zap.Error(err))
		// Don't fail the operation for counter updates
	}

	log.Info("successfully processed remove activity",
		zap.String("collection", collectionType),
		zap.String("object_id", objectID),
		zap.String("actor", activity.Actor))

	return nil
}

// extractCollectionType extracts the collection type from a target collection URL
func (ih *InboxHandler) extractCollectionType(targetURL string) (string, error) {
	// Parse common ActivityPub collection patterns
	// Examples:
	// - https://example.com/users/alice/featured -> "featured"
	// - https://example.com/users/alice/collections/pinned -> "pinned"
	// - https://example.com/users/alice/likes -> "likes"

	if err := common.ValidateRequiredParam("targetURL", targetURL); err != nil {
		return "", targetURLEmptyError()
	}

	parts := strings.Split(targetURL, "/")
	if len(parts) < 2 {
		return "", targetURLFormatError()
	}

	// Get the last part of the URL as collection type
	collectionType := parts[len(parts)-1]

	// Validate against known collection types
	validCollections := map[string]bool{
		"featured":  true, // Featured posts (Mastodon pinned posts)
		"pinned":    true, // Pinned items
		"likes":     true, // Liked items
		"shares":    true, // Shared/boosted items
		"bookmarks": true, // Bookmarked items
		"follows":   true, // Following collection (for certain Add/Remove operations)
		"followers": true, // Followers collection
	}

	if !validCollections[collectionType] {
		// Allow unknown collection types but log a warning
		ih.logger.Warn("unknown collection type",
			zap.String("collection_type", collectionType),
			zap.String("target_url", targetURL))
	}

	return collectionType, nil
}

// processFlagActivity processes a Flag activity for content moderation
func (ih *InboxHandler) processFlagActivity(ctx context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	log := common.WithContext(ctx)

	log.Info("processing flag activity",
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.Any("object", activity.Object))

	// Extract the flagged object(s)
	flaggedObjects, err := ih.extractFlaggedObjects(activity)
	if err != nil {
		log.Warn("failed to extract flagged objects", zap.Error(err))
		return invalidFlagError()
	}

	if err := common.ValidateSliceNotEmpty("flaggedObjects", flaggedObjects); err != nil {
		log.Warn("flag activity contains no flagged objects")
		return flagNoObjectsError()
	}

	// Create moderation flag record using legacy storage type
	flag := &storage.Flag{
		ID:        activity.ID,
		Actor:     activity.Actor,
		Object:    flaggedObjects,
		Content:   ih.extractFlagReason(activity),
		Status:    "pending",
		Published: time.Now(),
	}

	// Store the flag
	if err := ih.storageAdapter.Moderation().CreateFlag(ctx, flag); err != nil {
		log.Error("failed to store moderation flag", zap.Error(err))
		return storeModerationFlagError()
	}

	// Optionally trigger automated moderation analysis
	if ih.shouldRunAutomatedModeration(flag) {
		if err := ih.triggerAutomatedModeration(ctx, flag); err != nil {
			log.Warn("failed to trigger automated moderation", zap.Error(err))
			// Continue processing even if automated moderation fails
		}
	}

	log.Info("successfully processed flag activity",
		zap.String("flag_id", flag.ID),
		zap.String("reporter", activity.Actor),
		zap.Int("flagged_objects_count", len(flaggedObjects)))

	return nil
}

// processMoveActivity processes a Move activity for account migration
func (ih *InboxHandler) processMoveActivity(ctx context.Context, activity *activitypub.Activity, _ *activitypub.Actor) error {
	log := common.WithContext(ctx)

	log.Info("processing move activity",
		zap.String("activity_id", activity.ID),
		zap.String("actor", activity.Actor),
		zap.String("target", activity.Target))

	// Validate required fields for Move activity
	if err := common.ValidateRequiredParam("activityTarget", activity.Target); err != nil {
		log.Warn("move activity missing target")
		return moveNoTargetError()
	}

	// The actor field is the old account, target is the new account
	oldAccountID := activity.Actor
	newAccountID := activity.Target

	// Validate that the move is properly authorized
	if err := ih.validateMoveAuthorization(ctx, oldAccountID, newAccountID, activity); err != nil {
		log.Warn("move activity authorization failed", zap.Error(err))
		return moveAuthorizationError()
	}

	// Create migration record
	migration := &models.Move{
		ID:        activity.ID,
		Actor:     oldAccountID,
		Target:    newAccountID,
		Published: time.Now(),
		CreatedAt: time.Now(),
	}

	// Set TTL to expire after 30 days
	migration.SetTTL(time.Now().Add(30 * 24 * time.Hour))

	// Store the migration
	if err := ih.storageAdapter.GetDB().WithContext(ctx).Model(migration).Create(); err != nil {
		log.Error("failed to store account migration", zap.Error(err))
		return storeMigrationError()
	}

	// Update local followers to follow the new account
	if err := ih.processMoveFollowerMigration(ctx, oldAccountID, newAccountID); err != nil {
		log.Error("failed to process follower migration", zap.Error(err))
		// Don't fail the entire operation if follower migration has issues
	}

	// Create tombstone for the old account (if it's a local account being moved)
	if strings.HasPrefix(oldAccountID, ih.baseURL) {
		if err := ih.createAccountTombstone(ctx, oldAccountID, newAccountID); err != nil {
			log.Error("failed to create account tombstone", zap.Error(err))
			// Continue even if tombstone creation fails
		}
	}

	// Optionally send notifications to followers about the migration
	if err := ih.notifyFollowersOfMove(ctx, oldAccountID, newAccountID); err != nil {
		log.Warn("failed to notify followers of account move", zap.Error(err))
		// Continue even if notification fails
	}

	log.Info("successfully processed move activity",
		zap.String("old_account", oldAccountID),
		zap.String("new_account", newAccountID),
		zap.String("migration_id", migration.ID))

	return nil
}

// Helper functions for Flag activity processing

func (ih *InboxHandler) extractFlaggedObjects(activity *activitypub.Activity) ([]string, error) {
	var flaggedObjects []string

	switch obj := activity.Object.(type) {
	case string:
		// Single object URL
		flaggedObjects = append(flaggedObjects, obj)
	case []interface{}:
		// Array of objects
		for _, item := range obj {
			if objURL, ok := item.(string); ok {
				flaggedObjects = append(flaggedObjects, objURL)
			} else if objMap, ok := item.(map[string]interface{}); ok {
				if id, exists := objMap["id"].(string); exists {
					flaggedObjects = append(flaggedObjects, id)
				}
			}
		}
	case map[string]interface{}:
		// Single object with embedded data
		if id, exists := obj["id"].(string); exists {
			flaggedObjects = append(flaggedObjects, id)
		}
	default:
		return nil, unsupportedFlagObjectError()
	}

	return flaggedObjects, nil
}

func (ih *InboxHandler) extractFlagReason(activity *activitypub.Activity) string {
	// Check for reason in summary field (common in ActivityPub)
	if activity.Summary != "" {
		return activity.Summary
	}

	// Check for content field
	if content, ok := activity.Object.(map[string]interface{}); ok {
		if reason, exists := content["content"].(string); exists {
			return reason
		}
	}

	return "No reason provided"
}

func (ih *InboxHandler) shouldRunAutomatedModeration(flag *storage.Flag) bool {
	// Run automated moderation for certain types of reports
	reason := strings.ToLower(flag.Content)
	return strings.Contains(reason, "spam") ||
		strings.Contains(reason, "bot") ||
		len(flag.Object) > 3
}

func (ih *InboxHandler) triggerAutomatedModeration(ctx context.Context, flag *storage.Flag) error {
	log := common.WithContext(ctx)

	// This would integrate with the AI moderation service
	// For now, just log that automated moderation would be triggered
	log.Info("triggering automated moderation analysis",
		zap.String("flag_id", flag.ID),
		zap.String("reason", flag.Content),
		zap.Int("object_count", len(flag.Object)))

	return nil
}

// Helper functions for Move activity processing

func (ih *InboxHandler) validateMoveAuthorization(ctx context.Context, oldAccountID, newAccountID string, _ *activitypub.Activity) error {
	log := common.WithContext(ctx)

	// Extract username from the new account ID to check alsoKnownAs
	newUsername := ih.extractHandleFromActorID(newAccountID)
	if err := common.ValidateRequiredParam("newUsername", newUsername); err != nil {
		log.Error("failed to extract username from new account ID", zap.String("new_account_id", newAccountID))
		return extractUsernameError()
	}

	// Check if the new account has the old account in its alsoKnownAs field
	// This is the proper ActivityPub way to validate account migration authorization
	hasAlsoKnownAs, err := ih.actorRepository.CheckAlsoKnownAs(ctx, newUsername, oldAccountID)
	if err != nil {
		log.Error("failed to check alsoKnownAs for move authorization",
			zap.String("new_username", newUsername),
			zap.String("old_account_id", oldAccountID),
			zap.Error(err))
		return verifyMoveAuthError()
	}

	if !hasAlsoKnownAs {
		log.Warn("move authorization failed - new account does not have old account in alsoKnownAs",
			zap.String("old_account", oldAccountID),
			zap.String("new_account", newAccountID),
			zap.String("new_username", newUsername))
		return moveNotAuthorizedError()
	}

	log.Info("move authorization validated - alsoKnownAs confirmation found",
		zap.String("old_account", oldAccountID),
		zap.String("new_account", newAccountID),
		zap.String("new_username", newUsername))

	return nil
}

func (ih *InboxHandler) processMoveFollowerMigration(ctx context.Context, oldAccountID, newAccountID string) error {
	log := common.WithContext(ctx)

	// Get followers of the old account
	oldActorHandle := ih.extractHandleFromActorID(oldAccountID)
	followers, _, err := ih.relationshipRepository.GetFollowers(ctx, oldActorHandle, 1000, "")
	if err != nil {
		log.Error("failed to get followers for migration", zap.Error(err))
		return err
	}

	log.Info("processing follower migration",
		zap.String("old_account", oldAccountID),
		zap.String("new_account", newAccountID),
		zap.Int("follower_count", len(followers)))

	// For each local follower, update their following to point to the new account
	successCount := 0
	for _, followerHandle := range followers {
		// Check if this is a local follower
		if strings.Contains(followerHandle, "@") {
			baseDomain := ih.extractDomainFromURL(ih.baseURL)
			// Validate the base domain
			if err := common.ValidateDomainName(baseDomain); err != nil {
				ih.logger.Warn("invalid base domain",
					zap.String("domain", baseDomain),
					zap.Error(err))
				continue
			}

			if !strings.HasSuffix(followerHandle, "@"+baseDomain) {
				// Skip remote followers - they should handle the migration themselves
				continue
			}
		}

		// Remove old following relationship
		if err := ih.relationshipRepository.DeleteRelationship(ctx, followerHandle, oldActorHandle); err != nil {
			log.Warn("failed to remove old following relationship during migration",
				zap.String("follower", followerHandle),
				zap.String("old_account", oldActorHandle),
				zap.Error(err))
			continue
		}

		// Add new following relationship
		newActorHandle := ih.extractHandleFromActorID(newAccountID)
		if err := ih.relationshipRepository.CreateRelationship(ctx, followerHandle, newActorHandle, "following"); err != nil {
			log.Warn("failed to create new following relationship during migration",
				zap.String("follower", followerHandle),
				zap.String("new_account", newActorHandle),
				zap.Error(err))
			continue
		}

		successCount++
	}

	log.Info("completed follower migration",
		zap.String("old_account", oldAccountID),
		zap.String("new_account", newAccountID),
		zap.Int("migrated_followers", successCount),
		zap.Int("total_followers", len(followers)))

	return nil
}

func (ih *InboxHandler) createAccountTombstone(ctx context.Context, oldAccountID, newAccountID string) error {
	// Create a simple tombstone record using the existing Tombstone model
	tombstone := &models.Tombstone{
		ID:         oldAccountID,
		FormerType: "Person",
		DeletedBy:  oldAccountID,
		Summary:    fmt.Sprintf("Account moved to %s", newAccountID),
		Deleted:    time.Now(),
		CreatedAt:  time.Now(),
	}

	// Store the tombstone
	return ih.storageAdapter.GetDB().WithContext(ctx).Model(tombstone).Create()
}

func (ih *InboxHandler) notifyFollowersOfMove(ctx context.Context, oldAccountID, newAccountID string) error {
	log := common.WithContext(ctx)

	// This would create notifications for followers about the account migration
	// For now, just log that notifications would be sent
	log.Info("would notify followers of account move",
		zap.String("old_account", oldAccountID),
		zap.String("new_account", newAccountID))

	return nil
}

func actorMatchesCollectionOwner(actorID, targetCollectionURL, collectionOwnerUsername string) bool {
	actorURL, err := url.Parse(actorID)
	if err != nil || actorURL.Host == "" {
		return false
	}

	targetURL, err := url.Parse(targetCollectionURL)
	if err != nil || targetURL.Host == "" {
		return false
	}

	actorDomain := strings.ToLower(actorURL.Host)
	targetDomain := strings.ToLower(targetURL.Host)
	if actorDomain != targetDomain {
		return false
	}

	actorLocalUsername := extractLocalUsernameFromURLPath(actorURL.Path)
	if actorLocalUsername == "" {
		return false
	}

	return actorLocalUsername == strings.TrimPrefix(collectionOwnerUsername, "@")
}

func extractLocalUsernameFromURLPath(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}

	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "users" && i+1 < len(parts) {
			return strings.TrimPrefix(parts[i+1], "@")
		}
	}

	return strings.TrimPrefix(parts[len(parts)-1], "@")
}

// verifyCollectionAuthorization verifies that the actor is authorized to modify the collection
func (ih *InboxHandler) verifyCollectionAuthorization(_ context.Context, activity *activitypub.Activity, collectionType string, targetActor *activitypub.Actor) error {
	// Basic rule: Only the collection owner can add/remove items
	// For user collections (featured, likes, etc.), only that user can modify

	// Extract the collection owner from the target URL
	// Target format: https://domain.com/users/username/collection
	targetParts := strings.Split(activity.Target, "/")
	if len(targetParts) < 4 {
		return determineCollectionOwnerError()
	}

	var collectionOwnerUsername string
	for i, part := range targetParts {
		if part == "users" && i+1 < len(targetParts) {
			collectionOwnerUsername = targetParts[i+1]
			break
		}
	}

	if err := common.ValidateRequiredParam("collectionOwnerUsername", collectionOwnerUsername); err != nil {
		return extractCollectionOwnerError()
	}

	// For local collections, check if the actor matches the target actor
	if activity.Actor == targetActor.ID {
		return nil // Actor is modifying their own collection
	}

	// For non-canonical actor IDs, allow same-username only on the same domain.
	// Never use substring matching here: it allows cross-domain impersonation (e.g., "@alice@attacker.com" matching "alice").
	if actorMatchesCollectionOwner(activity.Actor, activity.Target, collectionOwnerUsername) {
		return nil
	}

	// Special case: allow certain actors to manage specific collection types
	// This could be extended for admin actions, moderation, etc.
	if collectionType == "featured" {
		// Only the actor themselves can manage their featured posts
		return unauthorizedCollectionError()
	}

	return unauthorizedCollectionModifyError()
}

// updateCollectionCounters updates metadata counters for collections
func (ih *InboxHandler) updateCollectionCounters(_ context.Context, collectionType, username string, delta int) error {
	// This is a framework for updating collection counters
	// In a full implementation, you would update actor statistics:
	// - featured_count
	// - pinned_count
	// - likes_count
	// etc.

	ih.logger.Debug("collection counter update",
		zap.String("collection", collectionType),
		zap.String("username", username),
		zap.Int("delta", delta))

	// For now, we'll just log the counter update
	// In a production system, you would:
	// 1. Get the actor record
	// 2. Update the appropriate counter field
	// 3. Save the updated actor record

	return nil
}

// Helper functions

// checkBlockStatus verifies that two actors haven't blocked each other
func (ih *InboxHandler) checkBlockStatus(ctx context.Context, actor1, actor2 string) error {
	log := common.WithContext(ctx)

	// Check if either actor has blocked the other
	isBlocked, err := ih.relationshipRepository.IsBlockedBidirectional(ctx, actor1, actor2)
	if err != nil {
		log.Error("failed to check block status",
			zap.String("actor1", actor1),
			zap.String("actor2", actor2),
			zap.Error(err))
		// Fail open - don't block legitimate activities due to DB errors
		return nil
	}

	if isBlocked {
		log.Info("activity blocked due to block relationship",
			zap.String("actor1", actor1),
			zap.String("actor2", actor2))
		return activityBlockedError()
	}

	return nil
}

// verifyUpdateAuthorization verifies that the actor is authorized to update the object
func (ih *InboxHandler) verifyUpdateAuthorization(_ context.Context, activity *activitypub.Activity, existingObject any) error {
	// Extract the object's attributed actor
	var objectOwner string

	if objMap, ok := existingObject.(map[string]any); ok {
		if attr, ok := objMap["attributedTo"].(string); ok {
			objectOwner = attr
		}
	} else if article, ok := existingObject.(*activitypub.Article); ok {
		objectOwner = article.AttributedTo
	} else if note, ok := existingObject.(*activitypub.Note); ok {
		objectOwner = note.AttributedTo
	}

	if err := common.ValidateRequiredParam("objectOwner", objectOwner); err != nil {
		return determineObjectOwnerError()
	}

	// Only the object owner can update it
	if !common.SameCanonicalActorID(activity.Actor, objectOwner) {
		return unauthorizedUpdateError()
	}

	return nil
}

// storeEditHistory stores the current object state as edit history before updating
func (ih *InboxHandler) storeEditHistory(ctx context.Context, objectID string, existingObject any, updatedBy string) error {
	log := common.WithContext(ctx)

	// Convert existing object to map for storage
	var previousState map[string]any

	// Serialize the existing object to JSON then deserialize to map
	objectJSON, err := json.Marshal(existingObject)
	if err != nil {
		return serializeObjectError()
	}

	if err := json.Unmarshal(objectJSON, &previousState); err != nil {
		return deserializeObjectError()
	}

	// Get the current version number (start from version 1 for first edit)
	historyEntries, err := ih.objectRepository.GetUpdateHistory(ctx, objectID, 1)
	if err != nil {
		log.Debug("failed to get update history, assuming first edit",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	version := len(historyEntries) + 1 // Version 1 is the first edit (original is version 0)

	// Create update history record
	updateHistory := &storage.UpdateHistory{
		ObjectID:      objectID,
		Version:       version,
		UpdatedAt:     time.Now(),
		UpdatedBy:     updatedBy,
		PreviousState: previousState,
		Summary:       "Remote update via ActivityPub",
	}

	// Store the history
	if err := ih.objectRepository.CreateUpdateHistory(ctx, updateHistory); err != nil {
		return createUpdateHistoryError()
	}

	log.Debug("stored edit history",
		zap.String("object_id", objectID),
		zap.Int("version", version),
		zap.String("updated_by", updatedBy))

	return nil
}

// extractDeleteTarget extracts the object ID and original object from a Delete activity
func (ih *InboxHandler) extractDeleteTarget(activity *activitypub.Activity) (string, map[string]any, error) {
	var objectID string
	var originalObject map[string]any

	switch obj := activity.Object.(type) {
	case string:
		// Object is just an ID
		objectID = obj
	case map[string]any:
		// Object is embedded
		if id, ok := obj["id"].(string); ok {
			objectID = id
			originalObject = obj
		}
	case *activitypub.Article:
		objectID = obj.ID
		originalObject = map[string]any{"id": obj.ID, "type": activitypub.ArticleType}
	case *activitypub.Note:
		objectID = obj.ID
		objectType := obj.Type
		if strings.TrimSpace(objectType) == "" {
			objectType = activitypub.NoteType
		}
		originalObject = map[string]any{"id": obj.ID, "type": objectType}
	case *activitypub.BaseObject:
		// Object is typed
		objectID = obj.ID
		if strings.TrimSpace(obj.Type) != "" {
			originalObject = map[string]any{"id": obj.ID, "type": obj.Type}
		}
	default:
		return "", nil, unsupportedDeleteObjectError()
	}

	return objectID, originalObject, nil
}

// verifyDeleteAuthorization verifies that the actor is authorized to delete the object
func (ih *InboxHandler) verifyDeleteAuthorization(_ context.Context, activity *activitypub.Activity, existingObject any) error {
	// Extract the object's attributed actor
	var objectOwner string

	if objMap, ok := existingObject.(map[string]any); ok {
		if attr, ok := objMap["attributedTo"].(string); ok {
			objectOwner = attr
		}
	} else if article, ok := existingObject.(*activitypub.Article); ok {
		objectOwner = article.AttributedTo
	} else if note, ok := existingObject.(*activitypub.Note); ok {
		objectOwner = note.AttributedTo
	}

	if err := common.ValidateRequiredParam("objectOwner", objectOwner); err != nil {
		return determineObjectOwnerError()
	}

	// Only the object owner can delete it
	if !common.SameCanonicalActorID(activity.Actor, objectOwner) {
		return unauthorizedDeleteError()
	}

	return nil
}

// cascadeDeleteOperations performs cascade deletion of related data
func (ih *InboxHandler) cascadeDeleteOperations(ctx context.Context, objectID, deletedBy string) error {
	log := common.WithContext(ctx)

	log.Info("performing cascade delete operations",
		zap.String("object_id", objectID),
		zap.String("deleted_by", deletedBy))

	// Remove likes for this object
	if err := ih.cascadeDeleteLikes(ctx, objectID); err != nil {
		log.Warn("failed to cascade delete likes", zap.Error(err))
	}

	// Remove announces/boosts for this object
	if err := ih.cascadeDeleteAnnounces(ctx, objectID); err != nil {
		log.Warn("failed to cascade delete announces", zap.Error(err))
	}

	// Remove from collections (featured, pinned, etc.)
	if err := ih.cascadeDeleteFromCollections(ctx, objectID); err != nil {
		log.Warn("failed to cascade delete from collections", zap.Error(err))
	}

	// Remove replies if this is a parent object
	if err := ih.cascadeDeleteReplies(ctx, objectID); err != nil {
		log.Warn("failed to cascade delete replies", zap.Error(err))
	}

	// Clean up notifications related to this object
	if err := ih.cascadeDeleteNotifications(ctx, objectID); err != nil {
		log.Warn("failed to cascade delete notifications", zap.Error(err))
	}

	log.Info("completed cascade delete operations", zap.String("object_id", objectID))
	return nil
}

// cascadeDeleteLikes removes likes for the deleted object
func (ih *InboxHandler) cascadeDeleteLikes(ctx context.Context, objectID string) error {
	// Get all likes for this object
	likes, _, err := ih.likeRepository.GetObjectLikes(ctx, objectID, 1000, "")
	if err != nil {
		return getObjectLikesError()
	}

	// Delete each like
	for _, like := range likes {
		if err := ih.likeRepository.DeleteLike(ctx, like.Actor, objectID); err != nil {
			ih.logger.Warn("failed to delete like during cascade",
				zap.String("object_id", objectID),
				zap.String("like_actor", like.Actor),
				zap.Error(err))
		}
	}

	ih.logger.Debug("cascade deleted likes",
		zap.String("object_id", objectID),
		zap.Int("count", len(likes)))

	return nil
}

// cascadeDeleteAnnounces removes announces/boosts for the deleted object
func (ih *InboxHandler) cascadeDeleteAnnounces(_ context.Context, objectID string) error {
	// For now, we'll log that announce cleanup should happen
	// This would require implementing announce repository methods
	ih.logger.Debug("cascade delete announces - implementation framework ready",
		zap.String("object_id", objectID))

	return nil
}

// cascadeDeleteFromCollections removes the object from all collections
func (ih *InboxHandler) cascadeDeleteFromCollections(ctx context.Context, objectID string) error {
	// This would remove from featured posts, pinned items, etc.
	// Implementation depends on your collection structure

	// Remove from any collections that might contain this object
	collections := []string{"featured", "pinned", "bookmarks"}

	for _, collection := range collections {
		if err := ih.objectRepository.RemoveFromCollection(ctx, collection, objectID); err != nil {
			ih.logger.Debug("failed to remove from collection during cascade",
				zap.String("object_id", objectID),
				zap.String("collection", collection),
				zap.Error(err))
		}
	}

	return nil
}

// cascadeDeleteReplies marks replies as orphaned or deletes them (depending on policy)
func (ih *InboxHandler) cascadeDeleteReplies(ctx context.Context, objectID string) error {
	// Get replies to this object
	replies, _, err := ih.objectRepository.GetReplies(ctx, objectID, 1000, "")
	if err != nil {
		return getRepliesError()
	}

	// For ActivityPub compliance, we typically don't cascade delete replies
	// Instead, we just mark them as orphaned by updating their inReplyTo field
	for _, reply := range replies {
		if replyMap, ok := reply.(map[string]any); ok {
			if replyID, ok := replyMap["id"].(string); ok {
				ih.logger.Debug("reply will be orphaned due to parent deletion",
					zap.String("reply_id", replyID),
					zap.String("deleted_parent", objectID))
			}
		}
	}

	ih.logger.Debug("processed replies for cascade deletion",
		zap.String("object_id", objectID),
		zap.Int("reply_count", len(replies)))

	return nil
}

// cascadeDeleteNotifications removes notifications related to the deleted object
func (ih *InboxHandler) cascadeDeleteNotifications(ctx context.Context, objectID string) error {
	// Remove notifications about likes, announces, replies to this object
	if err := ih.notificationRepository.DeleteNotificationsByObject(ctx, objectID); err != nil {
		ih.logger.Error("failed to delete notifications for object",
			zap.String("object_id", objectID),
			zap.Error(err))
		return err
	}

	ih.logger.Debug("cascade deleted notifications", zap.String("object_id", objectID))
	return nil
}

func activityPubObjectFormerType(existingObject any) string {
	switch obj := existingObject.(type) {
	case map[string]any:
		objectType, _ := obj["type"].(string)
		return strings.TrimSpace(objectType)
	case *activitypub.Article:
		if obj == nil {
			return ""
		}
		if strings.TrimSpace(obj.Type) != "" {
			return strings.TrimSpace(obj.Type)
		}
		return activitypub.ArticleType
	case *activitypub.Note:
		if obj == nil {
			return ""
		}
		if strings.TrimSpace(obj.Type) != "" {
			return strings.TrimSpace(obj.Type)
		}
		return activitypub.NoteType
	case *activitypub.BaseObject:
		if obj == nil {
			return ""
		}
		return strings.TrimSpace(obj.Type)
	case *models.Object:
		if obj == nil {
			return ""
		}
		return strings.TrimSpace(obj.Type)
	default:
		return ""
	}
}

// createDeleteTombstone creates a tombstone for the deleted object
func (ih *InboxHandler) createDeleteTombstone(ctx context.Context, objectID string, deleteActivity *activitypub.Activity, originalObject map[string]any, existingObject any) error {
	log := common.WithContext(ctx)

	// Determine the original object type
	var formerType string
	if originalObject != nil {
		if objType, ok := originalObject["type"].(string); ok {
			formerType = objType
		}
	}
	if common.ValidateRequiredParam("formerType", formerType) != nil {
		formerType = activityPubObjectFormerType(existingObject)
	}
	if common.ValidateRequiredParam("formerType", formerType) != nil {
		formerType = activitypub.NoteType // Default assumption
	}

	// Create enhanced tombstone using models.Tombstone
	tombstone := &models.Tombstone{
		ID:         objectID,
		FormerType: formerType,
		DeletedBy:  deleteActivity.Actor,
		Summary:    fmt.Sprintf("Object deleted by %s", deleteActivity.Actor),
		Deleted:    time.Now(),
	}

	// Use the enhanced tombstone creation method which handles GSI keys and TTL
	if err := ih.objectRepository.CreateTombstone(ctx, tombstone); err != nil {
		log.Error("failed to create enhanced tombstone",
			zap.String("object_id", objectID),
			zap.String("deleted_by", deleteActivity.Actor),
			zap.Error(err))
		return createTombstoneError()
	}

	// Also create ActivityPub-compliant tombstone object for federation compatibility
	now := time.Now()
	apTombstone := &activitypub.Tombstone{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			ID:        objectID,
			Type:      activitypub.TombstoneType,
			Published: &now,
			Summary:   tombstone.Summary,
		},
		FormerType: formerType,
		Deleted:    now.Format(time.RFC3339),
	}

	// Store the ActivityPub tombstone for federation compatibility
	if err := ih.objectRepository.CreateObject(ctx, apTombstone); err != nil {
		log.Warn("failed to create ActivityPub tombstone (enhanced tombstone created successfully)",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Don't fail - the enhanced tombstone is the primary record
	}

	log.Info("created enhanced tombstone for deleted object",
		zap.String("object_id", objectID),
		zap.String("former_type", formerType),
		zap.String("deleted_by", deleteActivity.Actor),
		zap.Int64("ttl", tombstone.TTL))

	return nil
}

func (ih *InboxHandler) extractHandleFromActorID(actorID string) string {
	if normalized := models.NormalizeRelationshipIdentity(actorID, ih.getConfig().Domain); normalized != "" {
		return normalized
	}

	return strings.TrimSpace(actorID)
}

// extractUsernameFromActorID extracts just the username from an actor ID
func (ih *InboxHandler) extractUsernameFromActorID(actorID string) string {
	// Extract username from actor ID
	// Format: https://domain.com/users/username -> username
	parts := strings.Split(actorID, "/")
	if len(parts) < 3 {
		return "" // Return empty if not in expected format
	}

	// Return the last part (username)
	return parts[len(parts)-1]
}

// extractDomainFromURL extracts the domain from an ActivityPub actor URL
func (ih *InboxHandler) extractDomainFromURL(actorURL string) string {
	u, err := url.Parse(actorURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// trackCentralizedCost tracks Lambda and DynamoDB costs for inbox operations
// The operationType parameter distinguishes between "Federation" (success) and "Federation.Error" (failure)
func (ih *InboxHandler) trackCentralizedCost(req *InboxRequest, operationType string) {
	if ih.centralizedCostService == nil {
		return
	}

	duration := time.Since(req.StartTime)
	memoryMB := int64(128) // Default Lambda memory
	// Lambda memory size is not configurable in config package, keep environment lookup for AWS-specific values

	// Track Lambda execution
	lambdaOp := costpkg.LambdaOperation{
		FunctionName: "inbox",
		Duration:     duration,
		MemoryMB:     memoryMB,
		Timestamp:    req.StartTime,
	}
	logMessage := "failed to track Lambda cost"
	if operationType == "Federation.Error" {
		logMessage = "failed to track Lambda cost for failure"
	}
	if err := ih.centralizedCostService.TrackLambdaInvocation(context.Background(), lambdaOp); err != nil {
		ih.logger.Warn(logMessage, zap.Error(err))
	}

	// Track DynamoDB operations
	if req.CostParams.DynamoDBReadCount > 0 || req.CostParams.DynamoDBWriteCount > 0 {
		dynamoOp := costpkg.DynamoOperation{
			Type:               operationType,
			TableName:          ih.tableName,
			ConsumedReadUnits:  req.CostParams.DynamoDBReadCount,
			ConsumedWriteUnits: req.CostParams.DynamoDBWriteCount,
			Timestamp:          req.StartTime,
		}
		logMessage := "failed to track DynamoDB cost"
		if operationType == "Federation.Error" {
			logMessage = "failed to track DynamoDB cost for failure"
		}
		if err := ih.centralizedCostService.TrackDynamoOperation(context.Background(), dynamoOp); err != nil {
			ih.logger.Warn(logMessage, zap.Error(err))
		}
	}
}

func buildInboxApp(_ *common.LambdaContext, handler *InboxHandler) *apptheory.App {
	app := apptheory.New()

	// Panic recovery middleware (MUST be first to catch all panics)
	app.Use(panicRecovery(handler.logger))

	// Ensure a request id is always present.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx.RequestID == "" {
				ctx.RequestID = fmt.Sprintf("inbox-%d", time.Now().UnixNano())
			}
			ctx.Set("requestID", ctx.RequestID)
			return next(ctx)
		}
	})

	// Logging middleware
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			start := time.Now()
			resp, err := next(ctx)

			status := http.StatusOK
			if err != nil {
				status = http.StatusInternalServerError
			} else if resp != nil && resp.Status != 0 {
				status = resp.Status
			}

			handler.logger.Info("inbox request completed",
				zap.String("request_id", ctx.RequestID),
				zap.String("method", ctx.Request.Method),
				zap.String("path", ctx.Request.Path),
				zap.Int("status", status),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil || status >= 400),
			)

			return resp, err
		}
	})

	// Federation security (CORS + headers + body limit)
	app.Use(federationSecurityMiddleware())

	// Add unified error handling middleware
	app.Use(common.CreateFederationErrorMiddleware(handler.logger))

	// Add federation metrics middleware (inside error middleware, so it can observe handler errors)
	if handler.emfMetrics != nil {
		app.Use(handler.createFederationMetricsMiddleware())
	}

	// Register all inbox routes
	handler.RegisterRoutes(app)

	return app
}

func buildInboxLambdaHandler(app *apptheory.App, handler *InboxHandler) func(ctx context.Context, event json.RawMessage) (any, error) {
	// Wrap Lambda handler with federation observability
	return func(ctx context.Context, event json.RawMessage) (any, error) {
		requestStart := time.Now()

		// Record cold start metric if this is a cold start
		if time.Since(handler.startTime) < 30*time.Second && handler.emfMetrics != nil {
			handler.emfMetrics.RecordBusinessMetric(observability.MetricColdStarts, 1.0, observability.UnitCount, nil)
			coldStartDuration := time.Since(handler.startTime)
			handler.emfMetrics.RecordBusinessMetric(observability.MetricColdStartDuration, float64(coldStartDuration.Milliseconds()), observability.UnitMilliseconds, nil)
		}

		// Process the request
		result, err := app.HandleLambda(ctx, event)

		// Record request-level metrics
		requestDuration := time.Since(requestStart)
		if handler.emfMetrics != nil {
			handler.emfMetrics.RecordLatency("federation_request", requestDuration)
			handler.emfMetrics.RecordThroughput("federation_request", 1)

			if err != nil {
				handler.emfMetrics.RecordError("federation_request", "lambda_error")
			} else {
				handler.emfMetrics.RecordSuccess("federation_request")
			}
		}

		// CRITICAL: Flush all metrics before Lambda terminates
		if handler.emfMetrics != nil {
			handler.emfMetrics.Flush()
		}

		return result, err
	}
}

func headerValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.ToLower(strings.TrimSpace(key))
	values := ctx.Request.Headers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func queryValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	values := ctx.Request.Query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func appendHeaderValue(existing []string, add string) []string {
	add = strings.TrimSpace(add)
	if add == "" {
		return existing
	}
	for _, v := range existing {
		if strings.EqualFold(strings.TrimSpace(v), add) {
			return existing
		}
	}
	return append(existing, add)
}

func panicRecovery(logger *zap.Logger) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (resp *apptheory.Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					requestID := ctx.RequestID
					if requestID == "" {
						requestID = common.GenerateRequestIDULID()
						ctx.RequestID = requestID
					}

					if logger != nil {
						logger.Error("panic recovered",
							zap.Any("panic", r),
							zap.String("request_id", requestID),
							zap.String("path", ctx.Request.Path),
							zap.String("method", ctx.Request.Method),
							zap.ByteString("stack", debug.Stack()),
						)
					}

					resp = apptheory.MustJSON(http.StatusInternalServerError, map[string]any{
						"error":             "internal_server_error",
						"error_description": "An unexpected error occurred",
						"request_id":        requestID,
					})
					err = nil
				}
			}()

			return next(ctx)
		}
	}
}

func federationSecurityMiddleware() apptheory.Middleware {
	const maxFederationBodyBytes = 1024 * 1024 // 1MB
	const allowMethods = "GET, POST, OPTIONS"
	const allowHeaders = "Accept, Content-Type, Date, Digest, Host, Signature, User-Agent, Accept-Encoding, Authorization"
	const exposeHeaders = "Content-Type, Date"
	const maxAgeSeconds = 86400

	applyResponseHeaders := func(resp *apptheory.Response) {
		if resp == nil {
			return
		}
		if resp.Headers == nil {
			resp.Headers = map[string][]string{}
		}

		// CORS (federation requires wildcard origins)
		resp.Headers["access-control-allow-origin"] = []string{"*"}
		resp.Headers["access-control-expose-headers"] = []string{exposeHeaders}
		resp.Headers["vary"] = appendHeaderValue(resp.Headers["vary"], "Origin")

		// Federation-friendly security headers (no CSP).
		resp.Headers["x-content-type-options"] = []string{"nosniff"}
		resp.Headers["x-frame-options"] = []string{"SAMEORIGIN"}
		resp.Headers["referrer-policy"] = []string{"strict-origin-when-cross-origin"}
		resp.Headers["cross-origin-resource-policy"] = []string{"cross-origin"}
		resp.Headers["x-robots-tag"] = []string{"noindex, nofollow"}
	}

	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			// Enforce federation body size limits (ActivityPub inbox).
			if contentLength := headerValue(ctx, "Content-Length"); contentLength != "" {
				if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil && size > maxFederationBodyBytes {
					resp, sendErr := common.SendError(ctx, http.StatusRequestEntityTooLarge, "Request body too large")
					applyResponseHeaders(resp)
					return resp, sendErr
				}
			}
			if int64(len(ctx.Request.Body)) > maxFederationBodyBytes {
				resp, sendErr := common.SendError(ctx, http.StatusRequestEntityTooLarge, "Request body too large")
				applyResponseHeaders(resp)
				return resp, sendErr
			}

			// Handle preflight requests.
			if ctx.Request.Method == http.MethodOptions {
				resp := apptheory.Text(http.StatusNoContent, "")
				if resp.Headers == nil {
					resp.Headers = map[string][]string{}
				}
				resp.Headers["access-control-allow-origin"] = []string{"*"}
				resp.Headers["access-control-allow-methods"] = []string{allowMethods}
				resp.Headers["access-control-allow-headers"] = []string{allowHeaders}
				resp.Headers["access-control-max-age"] = []string{strconv.Itoa(maxAgeSeconds)}
				resp.Headers["access-control-expose-headers"] = []string{exposeHeaders}
				resp.Headers["vary"] = appendHeaderValue(resp.Headers["vary"], "Origin")
				applyResponseHeaders(resp)
				return resp, nil
			}

			resp, err := next(ctx)
			applyResponseHeaders(resp)
			return resp, err
		}
	}
}

// Run initializes dependencies and starts the inbox Lambda handler.
func Run() {
	// Initialize Lambda with standardized federation configuration
	config := common.LambdaConfig{
		ServiceName: "inbox",
		LambdaType:  common.LambdaTypeFederation, // Changed from API to Federation
	}

	lambdaCtx := initializeLambdaCtxFn(config)

	handler, err := NewInboxHandler(lambdaCtx)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize inbox handler: %v", err))
	}

	app := buildInboxApp(lambdaCtx, handler)
	lambdaHandler := buildInboxLambdaHandler(app, handler)

	// Use AppTheory's Lambda event handler (not a local server)
	startLambda(lambdaHandler)
}

// createFederationMetricsMiddleware creates middleware for federation-specific metrics collection
func (ih *InboxHandler) createFederationMetricsMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ih.emfMetrics == nil {
				return next(ctx)
			}

			// Extract federation context information
			federationCtx := ih.extractFederationContext(ctx)

			// Start latency timer and record incoming message
			timer := ih.emfMetrics.StartLatencyTimer(ctx.Context(), "federation_activity")
			ih.recordIncomingFederationMessage(federationCtx)

			// Execute request
			resp, err := next(ctx)

			// Record post-request metrics
			ih.recordFederationMetrics(federationCtx, timer, err)

			return resp, err
		}
	}
}

// federationContext holds extracted federation request information
type federationContext struct {
	username       string
	userAgent      string
	remoteInstance string
	path           string
	method         string
	dimensions     map[string]string
	hasSignature   bool
}

// extractFederationContext extracts federation-related information from the request context
func (ih *InboxHandler) extractFederationContext(ctx *apptheory.Context) *federationContext {
	federationCtx := &federationContext{
		username:     ctx.Param("username"),
		userAgent:    headerValue(ctx, "User-Agent"),
		hasSignature: headerValue(ctx, "Signature") != "",
		path:         ctx.Request.Path,
		method:       ctx.Request.Method,
	}

	// Determine remote instance from User-Agent
	federationCtx.remoteInstance = ih.parseRemoteInstance(federationCtx.userAgent)

	// Build dimensions map
	federationCtx.dimensions = ih.buildMetricsDimensions(federationCtx)

	return federationCtx
}

// parseRemoteInstance extracts the remote instance identifier from User-Agent header
func (ih *InboxHandler) parseRemoteInstance(userAgent string) string {
	if userAgent == "" {
		return "unknown"
	}

	// Parse instance from User-Agent (many ActivityPub servers include this)
	if strings.Contains(userAgent, "http") {
		if u, err := url.Parse(userAgent); err == nil {
			return u.Host
		}
	}

	// Simple parsing for common formats like "Mastodon/4.0.0"
	parts := strings.Fields(userAgent)
	if len(parts) > 0 {
		return strings.ToLower(parts[0])
	}

	return "unknown"
}

// buildMetricsDimensions creates the dimensions map for metrics recording
func (ih *InboxHandler) buildMetricsDimensions(federationCtx *federationContext) map[string]string {
	dimensions := map[string]string{
		observability.DimensionEndpoint: federationCtx.path,
		observability.DimensionMethod:   federationCtx.method,
		observability.DimensionInstance: federationCtx.remoteInstance,
	}

	if federationCtx.username != "" {
		dimensions["username"] = federationCtx.username
	}

	return dimensions
}

// recordIncomingFederationMessage records metrics for incoming federation messages
func (ih *InboxHandler) recordIncomingFederationMessage(federationCtx *federationContext) {
	ih.emfMetrics.RecordBusinessMetric(
		observability.MetricInboxMessages,
		1.0,
		observability.UnitCount,
		federationCtx.dimensions,
	)
}

// recordFederationMetrics records post-request federation metrics
func (ih *InboxHandler) recordFederationMetrics(federationCtx *federationContext, timer interface{}, err error) {
	statusCode := ih.determineStatusCode(err)
	success := ih.isRequestSuccessful(err, statusCode)

	// Record federation health metrics
	if success {
		ih.recordSuccessfulDelivery(timer, federationCtx.remoteInstance)
	} else {
		ih.recordFailedDelivery(timer, federationCtx.remoteInstance, statusCode)
	}

	// Record signature verification metrics
	if federationCtx.hasSignature {
		ih.recordSignatureVerificationMetrics(federationCtx.remoteInstance, success)
	}

	// Trigger federation health alerts for failures
	if !success {
		ih.triggerFederationHealthAlert(federationCtx.remoteInstance)
	}
}

// determineStatusCode determines the HTTP status code from the error
func (ih *InboxHandler) determineStatusCode(err error) int {
	if err != nil {
		return 500
	}
	return 200
}

// isRequestSuccessful determines if the request was successful based on error and status code
func (ih *InboxHandler) isRequestSuccessful(err error, statusCode int) bool {
	return err == nil && statusCode >= 200 && statusCode < 400
}

// recordSuccessfulDelivery records metrics for successful federation delivery
func (ih *InboxHandler) recordSuccessfulDelivery(timer interface{}, remoteInstance string) {
	if t, ok := timer.(interface {
		Finish(metrics interface{}, success bool)
	}); ok {
		t.Finish(ih.emfMetrics, true)
	}
	ih.emfMetrics.RecordFederationMetric("inbox_delivery", remoteInstance, true, 0)
}

// recordFailedDelivery records metrics for failed federation delivery
func (ih *InboxHandler) recordFailedDelivery(timer interface{}, remoteInstance string, statusCode int) {
	errorType := ih.categorizeErrorType(statusCode)

	if t, ok := timer.(interface {
		FinishWithError(metrics interface{}, errorType string)
	}); ok {
		t.FinishWithError(ih.emfMetrics, errorType)
	}
	ih.emfMetrics.RecordFederationMetric("inbox_delivery", remoteInstance, false, 0)
}

// categorizeErrorType categorizes the error type based on status code
func (ih *InboxHandler) categorizeErrorType(statusCode int) string {
	switch {
	case statusCode == 401 || statusCode == 403:
		return observability.ErrorTypeAuthentication
	case statusCode == 429:
		return observability.ErrorTypeRateLimit
	case statusCode >= 400 && statusCode < 500:
		return observability.ErrorTypeValidation
	default:
		return observability.ErrorTypeFederation
	}
}

// recordSignatureVerificationMetrics records metrics for signature verification attempts
func (ih *InboxHandler) recordSignatureVerificationMetrics(remoteInstance string, success bool) {
	status := "failure"
	if success {
		status = "success"
	}

	dimensions := map[string]string{
		observability.DimensionInstance: remoteInstance,
		"status":                        status,
	}

	ih.emfMetrics.RecordBusinessMetric(
		observability.MetricSignatureVerification,
		1.0,
		observability.UnitCount,
		dimensions,
	)
}

// triggerFederationHealthAlert triggers health alerts for federation failures
func (ih *InboxHandler) triggerFederationHealthAlert(remoteInstance string) {
	if ih.alertManager == nil {
		return
	}

	runAsync(func() {
		// Use a separate context for alerts to avoid blocking the request
		alertCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// This would need to be enhanced with actual federation failure rate calculation
		// For now, we'll trigger an alert for repeated failures from the same instance
		ih.alertManager.CheckFederationHealth(alertCtx, remoteInstance, 100.0, 1) // 100% failure rate for this request
	})
}
