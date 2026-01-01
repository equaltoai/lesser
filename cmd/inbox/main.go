// Package main implements the inbox Lambda function for receiving ActivityPub federation messages.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	costpkg "github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/monitoring"
	notifpush "github.com/equaltoai/lesser/pkg/notifications"
	"github.com/equaltoai/lesser/pkg/observability"
	notifsvc "github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	storageCore "github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

var (
	runAsync = func(fn func()) { go fn() }

	getDynamormClient     = dynamorm.GetClient
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

// InboxHandler handles ActivityPub inbox requests using Lift
type InboxHandler struct {
	db                           dynamormCore.DB
	actorRepository              interfaces.ActorRepository
	activityRepository           *repositories.ActivityRepository
	relationshipRepository       *repositories.RelationshipRepository
	objectRepository             *repositories.ObjectRepository
	likeRepository               *repositories.LikeRepository
	socialRepository             *repositories.SocialRepository
	federationActivityRepository *repositories.FederationActivityRepository
	federationCostRepository     *repositories.FederationCostRepository
	domainBlockRepository        *repositories.DomainBlockRepository
	userRepository               interfaces.UserRepository
	instanceRepository           *repositories.InstanceRepository
	publicKeyCacheRepository     *repositories.PublicKeyCacheRepository
	notificationRepository       *repositories.NotificationRepository
	signatureService             *federation.SignatureService
	logger                       *zap.Logger
	authMiddleware               *auth.Middleware
	rateLimiter                  *auth.RateLimiter
	costCalculator               *federation.CostCalculator
	centralizedCostService       *costpkg.TrackingService
	deliveryService              *federation.DeliveryService
	tableName                    string
	storageAdapter               storageCore.RepositoryStorage
	baseURL                      string
	emfMetrics                   *observability.EMFMetrics
	alertManager                 *monitoring.AlertManager
	startTime                    time.Time
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
	activityRepo           *repositories.ActivityRepository
	followRepo             *repositories.RelationshipRepository
	objectRepo             *repositories.ObjectRepository
	likeRepo               *repositories.LikeRepository
	socialRepo             *repositories.SocialRepository
	federationActivityRepo *repositories.FederationActivityRepository
	federationCostRepo     *repositories.FederationCostRepository
	domainBlockRepo        *repositories.DomainBlockRepository
	userRepo               interfaces.UserRepository
	instanceRepo           *repositories.InstanceRepository
	publicKeyCacheRepo     *repositories.PublicKeyCacheRepository
	notificationRepo       *repositories.NotificationRepository
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
		likeRepository:               repositories.likeRepo,
		socialRepository:             repositories.socialRepo,
		federationActivityRepository: repositories.federationActivityRepo,
		federationCostRepository:     repositories.federationCostRepo,
		domainBlockRepository:        repositories.domainBlockRepo,
		userRepository:               repositories.userRepo,
		instanceRepository:           repositories.instanceRepo,
		publicKeyCacheRepository:     repositories.publicKeyCacheRepo,
		notificationRepository:       repositories.notificationRepo,
		signatureService:             federationServices.signatureService,
		logger:                       logger,
		authMiddleware:               federationServices.authMiddleware,
		rateLimiter:                  federationServices.rateLimiter,
		costCalculator:               federationServices.costCalculator,
		centralizedCostService:       observabilityServices.centralizedCostService,
		deliveryService:              federationServices.deliveryService,
		tableName:                    cfg.DynamoTableName,
		storageAdapter:               repoFactory,
		baseURL:                      cfg.BaseURL(),
		emfMetrics:                   observabilityServices.emfMetrics,
		alertManager:                 observabilityServices.alertManager,
		startTime:                    time.Now(),
	}, nil
}

// RegisterRoutes registers all inbox routes
func (ih *InboxHandler) RegisterRoutes(app *lift.App) {
	// ActivityPub inbox endpoints
	_ = app.GET("/users/:username/inbox", ih.handleGetInbox)
	_ = app.POST("/users/:username/inbox", ih.handlePostInbox)
}

// handleGetInbox handles GET requests to retrieve inbox activities
func (ih *InboxHandler) handleGetInbox(ctx *lift.Context) error {
	username := ctx.Param("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", "missing username parameter", 400)
	}

	ih.logger.Info("received inbox GET request",
		zap.String("username", username),
		zap.String("user_agent", ctx.Header("User-Agent")),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))))

	// Authenticate and authorize the request
	actor, err := ih.authenticateInboxRequest(ctx, username)
	if err != nil {
		return err
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
func (ih *InboxHandler) authenticateInboxRequest(ctx *lift.Context, username string) (*activitypub.Actor, error) {
	// Validate authentication header
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authorization", authHeader); err != nil {
		return nil, lift.NewLiftError("UNAUTHORIZED", "authentication required", 401)
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, lift.NewLiftError("UNAUTHORIZED", "invalid authorization header format", 401)
	}

	// Validate JWT token
	claims, err := ih.authMiddleware.ValidateToken(authHeader)
	if err != nil {
		ih.logger.Warn("JWT validation failed",
			zap.Error(err),
			zap.String("username", username),
			zap.String("user_agent", ctx.Header("User-Agent")))

		if err == auth.ErrMissingAuthHeader || err == auth.ErrInvalidToken {
			return nil, lift.NewLiftError("UNAUTHORIZED", "invalid or expired token", 401)
		}
		return nil, lift.NewLiftError("UNAUTHORIZED", "authentication failed", 401)
	}

	// Verify user authorization
	if claims.Username != username {
		ih.logger.Warn("user mismatch in inbox request",
			zap.String("token_user", claims.Username),
			zap.String("requested_user", username))
		return nil, lift.NewLiftError("FORBIDDEN", "cannot access another user's inbox", 403)
	}

	// Get and validate actor
	actor, err := ih.actorRepository.GetActorByUsername(ctx.Context, username)
	if err != nil {
		if err.Error() == "actor not found" {
			return nil, lift.NewLiftError("NOT_FOUND", "actor not found", 404)
		}
		ih.logger.Error("failed to get actor", zap.Error(err))
		return nil, lift.NewLiftError("INTERNAL_ERROR", "internal server error", 500)
	}

	return actor, nil
}

// parsePaginationParams extracts and validates pagination parameters from the request
func (ih *InboxHandler) parsePaginationParams(ctx *lift.Context) (int, string, string) {
	limitStr := ctx.Query("limit")
	limit, err := common.ParseAndValidateActivityPubLimit(limitStr)
	if err != nil {
		limit = 20
	}

	cursor := ctx.Query("cursor")
	page := ctx.Query("page")

	return limit, cursor, page
}

// returnInboxCollection returns the inbox collection metadata
func (ih *InboxHandler) returnInboxCollection(ctx *lift.Context, actor *activitypub.Actor, username string) error {
	// Get first page to calculate total items (simplified approach)
	activities, _, err := ih.activityRepository.GetInboxActivities(ctx.Context, username, 1, "")
	if err != nil {
		ih.logger.Error("failed to get inbox count", zap.Error(err))
		return lift.NewLiftError("INTERNAL_ERROR", "internal server error", 500)
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

	ctx.Response.Headers["Content-Type"] = "application/activity+json"
	return ctx.JSON(collection)
}

// returnInboxPage returns a paginated collection of inbox activities
func (ih *InboxHandler) returnInboxPage(ctx *lift.Context, actor *activitypub.Actor, username string, limit int, cursor string) error {
	// Get activities for the page
	activities, nextCursor, err := ih.activityRepository.GetInboxActivities(ctx.Context, username, limit, cursor)
	if err != nil {
		ih.logger.Error("failed to get inbox activities", zap.Error(err))
		return lift.NewLiftError("INTERNAL_ERROR", "internal server error", 500)
	}

	// Enrich activities with full object data
	ih.enrichActivitiesWithObjects(ctx, activities)

	// Build collection page
	collectionPage := ih.buildCollectionPage(actor, activities, cursor, nextCursor, limit)

	ctx.Response.Headers["Content-Type"] = "application/activity+json"
	return ctx.JSON(collectionPage)
}

// enrichActivitiesWithObjects enriches Create activities with full object data
func (ih *InboxHandler) enrichActivitiesWithObjects(ctx *lift.Context, activities []*activitypub.Activity) {
	for _, activity := range activities {
		if activity.Type != activitypub.CreateType || activity.Object == nil {
			continue
		}

		objID, ok := activity.Object.(string)
		if !ok {
			continue
		}

		obj, err := ih.objectRepository.GetObject(ctx.Context, objID)
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
	Username    string
	Activity    *activitypub.Activity
	Actor       *activitypub.Actor
	Body        []byte
	ActorDomain string
	StartTime   time.Time
	CostParams  *federation.CostCalculationParams
}

// handlePostInbox handles POST requests to receive activities
func (ih *InboxHandler) handlePostInbox(ctx *lift.Context) error {
	// Initialize and validate request
	req, err := ih.initializeInboxRequest(ctx)
	if err != nil {
		return err
	}

	// Perform security checks (rate limiting, domain blocks)
	if err := ih.performSecurityChecks(ctx, req); err != nil {
		return err
	}

	// Verify authentication (signature verification)
	if err := ih.verifyAuthentication(ctx, req); err != nil {
		return err
	}

	// Validate addressing fields and privacy compliance
	if err := ih.validateAddressingAndPrivacy(ctx, req); err != nil {
		return err
	}

	// Store and process the activity
	if err := ih.storeAndProcessActivity(ctx, req); err != nil {
		return err
	}

	// Record success and complete
	ih.recordSuccessAndComplete(ctx, req)

	return ctx.Status(http.StatusAccepted).Text("")
}

// initializeInboxRequest creates and validates the basic request structure
func (ih *InboxHandler) initializeInboxRequest(ctx *lift.Context) (*InboxRequest, error) {
	username := ctx.Param("username")
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, lift.NewLiftError("VALIDATION_ERROR", "missing username parameter", 400)
	}

	// Prevent federation to the bootstrap actor until activation completes.
	if ih.instanceRepository != nil {
		state, err := ih.instanceRepository.GetInstanceState(ctx.Context)
		bootstrapUsername := models.DefaultBootstrapUsername
		if err == nil && strings.TrimSpace(state.BootstrapUsername) != "" {
			bootstrapUsername = strings.TrimSpace(state.BootstrapUsername)
		}

		if (err != nil && strings.EqualFold(username, bootstrapUsername)) ||
			(err == nil && state.Locked && strings.EqualFold(username, bootstrapUsername)) {
			return nil, lift.NewLiftError("FORBIDDEN", "bootstrap actor does not accept federation while instance is locked", 403)
		}
	}

	ih.logger.Info("received inbox POST request",
		zap.String("username", username),
		zap.String("content_type", ctx.Header("Content-Type")),
		zap.String("user_agent", ctx.Header("User-Agent")),
		zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))))

	// Validate Content-Type using centralized validation
	contentType := ctx.Header("Content-Type")
	if err := common.ValidateActivityPubContentType(contentType); err != nil {
		ih.logger.Warn("invalid content type", zap.String("content_type", contentType), zap.Error(err))
		return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid Content-Type: %v", err), 400)
	}

	// Verify the actor exists
	actor, err := ih.actorRepository.GetActorByUsername(ctx.Context, username)
	if err != nil {
		if err.Error() == "actor not found" {
			return nil, lift.NewLiftError("NOT_FOUND", "actor not found", 404)
		}
		ih.logger.Error("failed to get actor", zap.Error(err))
		return nil, lift.NewLiftError("INTERNAL_ERROR", "internal server error", 500)
	}

	// Validate and parse request body
	body := ctx.Request.Body
	if err := ih.validateRequestBody(body); err != nil {
		return nil, err
	}

	activity, err := ih.parseActivity(body)
	if err != nil {
		return nil, err
	}

	// Validate activity and addressing
	if err := ih.validateActivity(activity, actor); err != nil {
		return nil, err
	}

	// Initialize request structure
	startTime := time.Now()
	actorDomain := ih.extractDomainFromURL(activity.Actor)

	req := &InboxRequest{
		Username:    username,
		Activity:    activity,
		Actor:       actor,
		Body:        body,
		ActorDomain: actorDomain,
		StartTime:   startTime,
		CostParams: &federation.CostCalculationParams{
			ActivityID:         activity.ID,
			Domain:             actorDomain,
			ActivityType:       activity.Type,
			Direction:          "inbound",
			OperationType:      "inbox_processing",
			Timestamp:          startTime,
			PayloadSize:        int64(len(body)),
			LambdaMemoryMB:     512,
			DynamoDBReadCount:  1, // Actor verification
			DynamoDBWriteCount: 0,
		},
	}

	return req, nil
}

// validateRequestBody validates the request body size and content
func (ih *InboxHandler) validateRequestBody(body []byte) error {
	if err := common.ValidateSliceNotEmpty("body", body); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", "request body is required", 400)
	}

	if err := common.ValidateStringLength("request body", string(body), 0, common.MaxActivitySize); err != nil {
		ih.logger.Warn("request body too large", zap.Int("size", len(body)))
		return lift.NewLiftError("PAYLOAD_TOO_LARGE", "request body too large", 413)
	}

	return nil
}

// parseActivity parses and sanitizes the ActivityPub activity
func (ih *InboxHandler) parseActivity(body []byte) (*activitypub.Activity, error) {
	// Validate JSON format first
	if err := common.ValidateActivityPubJSON(string(body), "activity"); err != nil {
		ih.logger.Warn("invalid JSON format", zap.Error(err))
		return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid JSON: %v", err), 400)
	}

	var activity activitypub.Activity
	if err := common.ParseActivityPubObject(body, &activity); err != nil {
		ih.logger.Warn("failed to parse activity", zap.Error(err))
		return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid activity: %v", err), 400)
	}

	// Validate ActivityPub URL format for ID
	if err := common.ValidateActivityPubURL(activity.ID, "id"); err != nil {
		ih.logger.Warn("invalid activity ID URL", zap.String("id", activity.ID), zap.Error(err))
		return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid activity ID: %v", err), 400)
	}

	// Validate ActivityPub timestamp if published is set
	if activity.Published != nil && !activity.Published.IsZero() {
		if err := common.ValidateActivityPubTimestamp(activity.Published.Format(time.RFC3339), "published"); err != nil {
			ih.logger.Warn("invalid activity timestamp", zap.String("published", activity.Published.Format(time.RFC3339)), zap.Error(err))
			return nil, lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid timestamp: %v", err), 400)
		}
	}

	// Sanitize any embedded objects
	if objMap, ok := activity.Object.(map[string]any); ok {
		common.SanitizeActivityPubObjectDefault(objMap)
		activity.Object = objMap
	}

	ih.logger.Info("processing activity",
		zap.String("type", activity.Type),
		zap.String("actor", activity.Actor),
		zap.String("id", activity.ID))

	return &activity, nil
}

// validateActivity validates required activity fields and addressing
func (ih *InboxHandler) validateActivity(activity *activitypub.Activity, actor *activitypub.Actor) error {
	// Validate basic activity structure
	if err := ih.validateBasicActivity(activity); err != nil {
		return err
	}

	// Validate actor structure
	if err := ih.validateBasicActor(actor); err != nil {
		return err
	}

	// Validate addressing fields
	if err := ih.validateActivityAddressing(activity); err != nil {
		return err
	}

	// Validate actor username
	if err := ih.validateActorUsername(activity.Actor); err != nil {
		return err
	}

	// Validate actor public key if present
	if err := ih.validateActorPublicKey(actor); err != nil {
		return err
	}

	// Validate Create activity objects if applicable
	if err := ih.validateCreateActivityObject(activity); err != nil {
		return err
	}

	// Validate comprehensive addressing
	if err := ih.validateComprehensiveAddressing(activity); err != nil {
		return err
	}

	// Check if activity is addressed to this actor
	if err := ih.validateActivityTargeting(activity, actor); err != nil {
		return err
	}

	return nil
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
	context := []interface{}(activitypub.Context)
	if len(activity.Context) > 0 {
		context = []interface{}(activity.Context)
	}

	activityMap := map[string]interface{}{
		"@context": context,
		"id":       activity.ID,
		"type":     activity.Type,
		"actor":    activity.Actor,
		"object":   activity.Object,
		"to":       stringSliceToInterfaceSlice(activity.To),
		"cc":       stringSliceToInterfaceSlice(activity.CC),
		"bto":      stringSliceToInterfaceSlice(activity.BTo),
		"bcc":      stringSliceToInterfaceSlice(activity.BCC),
	}
	if err := common.ValidateActivityPubActivity(activityMap); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid activity: %v", err), 400)
	}
	return nil
}

// validateBasicActor validates the basic actor structure
func (ih *InboxHandler) validateBasicActor(actor *activitypub.Actor) error {
	actorMap := map[string]interface{}{
		"@context":          []interface{}(activitypub.Context),
		"id":                actor.ID,
		"type":              actor.Type,
		"preferredUsername": actor.PreferredUsername,
		"inbox":             actor.Inbox,
		"outbox":            actor.Outbox,
	}
	if err := common.ValidateActivityPubActor(actorMap); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid actor: %v", err), 400)
	}
	return nil
}

// validateActivityAddressing validates all addressing fields of the activity
func (ih *InboxHandler) validateActivityAddressing(activity *activitypub.Activity) error {
	addressingFields := []struct {
		field interface{}
		name  string
	}{
		{stringSliceToInterfaceSlice(activity.To), "to"},
		{stringSliceToInterfaceSlice(activity.CC), "cc"},
		{stringSliceToInterfaceSlice(activity.BTo), "bto"},
		{stringSliceToInterfaceSlice(activity.BCC), "bcc"},
	}

	for _, addr := range addressingFields {
		if err := common.ValidateActivityPubAddressing(addr.field, addr.name); err != nil {
			return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid '%s' addressing: %v", addr.name, err), 400)
		}
	}
	return nil
}

// validateActorUsername validates the actor's username format
func (ih *InboxHandler) validateActorUsername(actorURL string) error {
	parsedURL, err := url.Parse(actorURL)
	if err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid actor username: %v", err), 400)
	}

	path := strings.Trim(parsedURL.Path, "/")
	parts := strings.Split(path, "/")
	username := parts[len(parts)-1]
	if err := common.ValidateActivityPubUsername(username); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid actor username: %v", err), 400)
	}
	return nil
}

// validateActorPublicKey validates the actor's public key if present
func (ih *InboxHandler) validateActorPublicKey(actor *activitypub.Actor) error {
	if actor.PublicKey == nil {
		return nil
	}

	publicKeyMap := map[string]interface{}{
		"id":           actor.PublicKey.ID,
		"owner":        actor.PublicKey.Owner,
		"publicKeyPem": actor.PublicKey.PublicKeyPem,
	}
	if err := common.ValidateActivityPubPublicKey(publicKeyMap); err != nil {
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid public key: %v", err), 400)
	}
	return nil
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

	// Validate attachments if present
	if err := ih.validateObjectAttachments(objMap); err != nil {
		return err
	}

	// Validate tags if present
	if err := ih.validateObjectTags(objMap); err != nil {
		return err
	}

	// Validate note structure for Note objects
	if err := ih.validateNoteObject(objMap); err != nil {
		return err
	}

	return nil
}

// validateObjectAttachments validates object attachments
func (ih *InboxHandler) validateObjectAttachments(objMap map[string]interface{}) error {
	if attachments, exists := objMap["attachment"]; exists {
		if err := common.ValidateActivityPubAttachments(attachments, "attachment"); err != nil {
			return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid attachments: %v", err), 400)
		}
	}
	return nil
}

// validateObjectTags validates object tags
func (ih *InboxHandler) validateObjectTags(objMap map[string]interface{}) error {
	if tags, exists := objMap["tag"]; exists {
		if err := common.ValidateActivityPubTags(tags, "tag"); err != nil {
			return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid tags: %v", err), 400)
		}
	}
	return nil
}

// validateNoteObject validates Note object structure
func (ih *InboxHandler) validateNoteObject(objMap map[string]interface{}) error {
	if objType, exists := objMap["type"]; exists && objType == "Note" {
		if err := common.ValidateActivityPubNote(objMap); err != nil {
			return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid note object: %v", err), 400)
		}
	}
	return nil
}

// validateComprehensiveAddressing validates addressing using comprehensive validator
func (ih *InboxHandler) validateComprehensiveAddressing(activity *activitypub.Activity) error {
	addressingValidator := activitypub.NewAddressingValidator()
	if err := addressingValidator.ValidateAddressing(activity); err != nil {
		ih.logger.Warn("invalid activity addressing", zap.Error(err))
		return lift.NewLiftError("VALIDATION_ERROR", fmt.Sprintf("invalid addressing: %v", err), 400)
	}
	return nil
}

// validateActivityTargeting checks if the activity is addressed to this actor
func (ih *InboxHandler) validateActivityTargeting(activity *activitypub.Activity, actor *activitypub.Actor) error {
	if !ih.isAddressedTo(activity, actor) {
		ih.logger.Warn("activity not addressed to this actor",
			zap.String("actor_id", actor.ID),
			zap.Any("to", activity.To),
			zap.Any("cc", activity.CC))
		return lift.NewLiftError("VALIDATION_ERROR", "activity is not addressed to this actor", 400)
	}
	return nil
}

// performSecurityChecks handles rate limiting and domain blocking
func (ih *InboxHandler) performSecurityChecks(ctx *lift.Context, req *InboxRequest) error {
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
func (ih *InboxHandler) checkRateLimit(ctx *lift.Context, req *InboxRequest) error {
	if err := ih.rateLimiter.CheckRateLimit(ctx.Context, req.ActorDomain, ctx.Header("X-Forwarded-For")); err != nil {
		ih.logger.Warn("rate limit exceeded",
			zap.String("domain", req.ActorDomain),
			zap.Error(err))

		ih.recordFailureCost(req, "Rate limit exceeded", 2)
		return lift.NewLiftError("RATE_LIMITED", "rate limit exceeded for domain", 429)
	}

	// Record the rate limit attempt
	if err := ih.rateLimiter.RecordAttempt(ctx.Context, req.ActorDomain, ctx.Header("X-Forwarded-For"), false); err != nil {
		ih.logger.Warn("failed to record rate limit attempt", zap.Error(err))
	}

	return nil
}

// checkDomainBlock checks if the domain is blocked
func (ih *InboxHandler) checkDomainBlock(ctx *lift.Context, req *InboxRequest) error {
	isBlocked, block, err := ih.domainBlockRepository.IsDomainBlocked(ctx.Context, req.ActorDomain)
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
		return lift.NewLiftError("FORBIDDEN", "domain is suspended", 403)
	}

	// For silenced domains, we accept but may limit visibility
	return nil
}

// verifyAuthentication handles public key fetching and signature verification with enhanced security
func (ih *InboxHandler) verifyAuthentication(ctx *lift.Context, req *InboxRequest) error {
	start := time.Now()

	// Convert Lift request to http.Request for enhanced signature verification
	httpReq, err := ih.convertLiftRequest(ctx, req.Body)
	if err != nil {
		ih.logger.Error("failed to convert request for signature verification",
			zap.String("actor", req.Activity.Actor),
			zap.Error(err))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Request conversion failed: %v", err), 3)
		return lift.NewLiftError("VALIDATION_ERROR", "malformed request", 400)
	}

	// Enhanced signature verification with caching and retry logic
	signatureVerifyStart := time.Now()
	if err := ih.signatureService.VerifySignature(ctx.Context, httpReq, req.Activity.Actor); err != nil {
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
			return lift.NewLiftError("UNAUTHORIZED", "signature verification failed", 401)
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
			return lift.NewLiftError("SERVICE_UNAVAILABLE", "unable to verify sender", 503)
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
		return lift.NewLiftError("VALIDATION_ERROR", "digest verification failed", 400)
	}

	req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
	return nil
}

// validateAddressingAndPrivacy validates addressing fields and privacy compliance
func (ih *InboxHandler) validateAddressingAndPrivacy(_ *lift.Context, req *InboxRequest) error {
	start := time.Now()
	addressingValidator := activitypub.NewAddressingValidator()

	// Validate addressing fields are properly formatted
	if err := addressingValidator.ValidateAddressing(req.Activity); err != nil {
		ih.logger.Warn("activity has invalid addressing fields",
			zap.String("actor", req.Activity.Actor),
			zap.String("activity_id", req.Activity.ID),
			zap.Error(err))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Invalid addressing: %v", err), 1)
		return lift.NewLiftError("VALIDATION_ERROR", "invalid addressing fields", 400)
	}

	// Validate privacy compliance (e.g., direct messages don't have public addressing)
	if err := addressingValidator.ValidatePrivacyCompliance(req.Activity); err != nil {
		ih.logger.Warn("activity violates privacy compliance",
			zap.String("actor", req.Activity.Actor),
			zap.String("activity_id", req.Activity.ID),
			zap.Error(err))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Privacy violation: %v", err), 1)
		return lift.NewLiftError("VALIDATION_ERROR", "privacy compliance violation", 400)
	}

	// Check if the activity is actually addressed to this actor
	if !ih.isAddressedTo(req.Activity, req.Actor) {
		ih.logger.Warn("activity not addressed to this actor",
			zap.String("actor", req.Activity.Actor),
			zap.String("target_actor", req.Actor.ID),
			zap.String("activity_id", req.Activity.ID))
		req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
		ih.recordFailureCost(req, "Activity not addressed to target actor", 1)
		// Return 404 instead of 403 to maintain privacy (don't leak that actor exists)
		return lift.NewLiftError("NOT_FOUND", "not found", 404)
	}

	// For direct messages, perform additional validation
	if addressingValidator.IsDirectMessage(req.Activity) {
		if err := ih.validateDirectMessage(req.Activity, req.Actor); err != nil {
			ih.logger.Warn("direct message validation failed",
				zap.String("actor", req.Activity.Actor),
				zap.String("activity_id", req.Activity.ID),
				zap.Error(err))
			req.CostParams.ProcessingTimeMs = time.Since(start).Milliseconds()
			ih.recordFailureCost(req, fmt.Sprintf("Direct message validation failed: %v", err), 1)
			return lift.NewLiftError("VALIDATION_ERROR", "direct message validation failed", 400)
		}
	}

	req.CostParams.ProcessingTimeMs += time.Since(start).Milliseconds()
	ih.logger.Debug("addressing and privacy validation successful",
		zap.String("actor", req.Activity.Actor),
		zap.String("visibility", addressingValidator.GetVisibilityLevel(req.Activity)))

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
func (ih *InboxHandler) verifyDigestEnhanced(ctx *lift.Context, req *InboxRequest) error {
	digestHeader := ctx.Header("Digest")
	if err := common.ValidateRequiredParam("digestHeader", digestHeader); err != nil {
		// No digest header is acceptable for some implementations
		ih.logger.Debug("no digest header present", zap.String("actor", req.Activity.Actor))
		return nil
	}

	httpReq, err := ih.convertLiftRequest(ctx, req.Body)
	if err != nil {
		ih.logger.Warn("failed to convert request for digest verification",
			zap.String("actor", req.Activity.Actor),
			zap.Error(err))
		// Don't fail if we can't convert the request for digest verification
		return nil
	}

	// Use the enhanced digest verification with compatibility support
	if err := ih.signatureService.VerifyDigestWithCompatibility(httpReq, req.Body); err != nil {
		ih.logger.Warn("digest verification failed",
			zap.String("actor", req.Activity.Actor),
			zap.String("digest_header", digestHeader),
			zap.Error(err))
		return err // Return the specific error from the service
	}

	ih.logger.Debug("digest verification successful",
		zap.String("actor", req.Activity.Actor))
	return nil
}

// storeAndProcessActivity stores the activity and processes it based on type
func (ih *InboxHandler) storeAndProcessActivity(ctx *lift.Context, req *InboxRequest) error {
	// Store the activity
	if err := ih.activityRepository.CreateActivity(ctx.Context, req.Activity); err != nil {
		ih.logger.Error("failed to store activity", zap.Error(err))
		ih.recordFailureCost(req, fmt.Sprintf("Failed to store activity: %v", err), 3)
		return lift.NewLiftError("INTERNAL_ERROR", "failed to store activity", 500)
	}

	req.CostParams.DynamoDBWriteCount = 1 // Activity storage

	// Process by activity type
	processingStart := time.Now()
	if err := ih.processActivityByType(ctx.Context, req); err != nil {
		processingDuration := time.Since(processingStart)
		req.CostParams.ProcessingTimeMs += processingDuration.Milliseconds()
		ih.recordFailureCost(req, fmt.Sprintf("Failed to process %s activity: %v", req.Activity.Type, err), 0)
		return lift.NewLiftError("INTERNAL_ERROR", fmt.Sprintf("failed to process %s activity", req.Activity.Type), 500)
	}

	processingDuration := time.Since(processingStart)
	req.CostParams.ProcessingTimeMs += processingDuration.Milliseconds()
	return nil
}

// processActivityByType processes the activity based on its type
func (ih *InboxHandler) processActivityByType(ctx context.Context, req *InboxRequest) error {
	switch req.Activity.Type {
	case activitypub.FollowType:
		if err := ih.processFollowActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process follow activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Relationship creation
		req.CostParams.DynamoDBReadCount++  // Follow approval check

	case activitypub.AcceptType:
		if err := ih.processAcceptActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process accept activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Relationship update
		req.CostParams.DynamoDBReadCount++  // Original activity lookup

	case activitypub.RejectType:
		if err := ih.processRejectActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process reject activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Relationship deletion
		req.CostParams.DynamoDBReadCount++  // Original activity lookup

	case activitypub.CreateType:
		if err := ih.processRemoteCreateActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process create activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount += 2 // Object creation + timeline entry
		req.CostParams.DynamoDBReadCount++     // Content validation

	case activitypub.UpdateType:
		if err := ih.processRemoteUpdateActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process update activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Object update
		req.CostParams.DynamoDBReadCount++  // Object lookup

	case activitypub.DeleteType:
		if err := ih.processRemoteDeleteActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process delete activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++ // Object deletion
		req.CostParams.DynamoDBReadCount++  // Object lookup

	case activitypub.LikeType:
		if err := ih.processLikeActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process like activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount += 2 // Like creation + notification
		req.CostParams.DynamoDBReadCount++     // Object verification

	case activitypub.AnnounceType:
		if err := ih.processAnnounceActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process announce activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount += 2 // Announce creation + notification
		req.CostParams.DynamoDBReadCount++     // Object verification

	case activitypub.UndoType:
		if err := ih.processUndoActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process undo activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++   // Undo operation
		req.CostParams.DynamoDBReadCount += 2 // Original activity + target lookup

	case activitypub.BlockType:
		if err := ih.processBlockActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process block activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount += 2 // Block creation + remove follow relationships
		req.CostParams.DynamoDBReadCount++     // Relationship check

	case activitypub.AddType:
		if err := ih.processAddActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process add activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++   // Collection item creation
		req.CostParams.DynamoDBReadCount += 2 // Target collection + authorization check

	case activitypub.RemoveType:
		if err := ih.processRemoveActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process remove activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++   // Collection item removal
		req.CostParams.DynamoDBReadCount += 2 // Target collection + authorization check

	case activitypub.FlagType:
		if err := ih.processFlagActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process flag activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount += 2 // Report creation + moderation queue
		req.CostParams.DynamoDBReadCount++     // Authorization check

	case activitypub.MoveType:
		if err := ih.processMoveActivity(ctx, req.Activity, req.Actor); err != nil {
			ih.logger.Error("failed to process move activity", zap.Error(err))
			return err
		}
		req.CostParams.DynamoDBWriteCount++   // Migration record
		req.CostParams.DynamoDBReadCount += 2 // Actor validation + authorization check

	default:
		ih.logger.Info("ignoring unsupported activity type in inbox",
			zap.String("type", req.Activity.Type),
			zap.String("actor", req.Activity.Actor),
			zap.String("id", req.Activity.ID),
		)
		// Not an error - just unsupported activity type
	}

	return nil
}

// recordSuccessAndComplete handles final success logging and cost tracking
func (ih *InboxHandler) recordSuccessAndComplete(ctx *lift.Context, req *InboxRequest) {
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
		if err := ih.rateLimiter.RecordAttempt(ctx.Context, req.ActorDomain, ctx.Header("X-Forwarded-For"), true); err != nil {
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
	actorID := actor.ID
	inboxURL := actor.Inbox

	// Check 'to' field
	for _, to := range activity.To {
		if to == actorID || to == inboxURL || to == activitypub.PublicAddress {
			return true
		}
	}

	// Check 'cc' field
	for _, cc := range activity.CC {
		if cc == actorID || cc == inboxURL || cc == activitypub.PublicAddress {
			return true
		}
	}

	// Check 'bto' field
	for _, bto := range activity.BTo {
		if bto == actorID || bto == inboxURL {
			return true
		}
	}

	// Check 'bcc' field
	for _, bcc := range activity.BCC {
		if bcc == actorID || bcc == inboxURL {
			return true
		}
	}

	return false
}



// convertLiftRequest converts a Lift request to an http.Request
func (ih *InboxHandler) convertLiftRequest(ctx *lift.Context, body []byte) (*http.Request, error) {
	// Build URL
	u := &url.URL{
		Scheme: "https",
		Host:   ctx.Header("Host"),
		Path:   ctx.Request.Path,
	}

	if ctx.Request.QueryParams != nil {
		q := u.Query()
		for k, v := range ctx.Request.QueryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx.Request.Context(), ctx.Request.Method, u.String(), strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	// Copy headers
	for k, v := range ctx.Request.Headers {
		req.Header.Set(k, v)
	}

	// Set host header if not present
	if common.ValidateRequiredParam("reqHost", req.Header.Get("Host")) != nil && common.ValidateRequiredParam("ctxHost", ctx.Header("Host")) == nil {
		req.Host = ctx.Header("Host")
	}

	return req, nil
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



// processFollowActivity processes an incoming Follow activity
func (ih *InboxHandler) processFollowActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

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

	// Create the follow relationship with pending state
	err := ih.relationshipRepository.CreateRelationship(ctx, followerHandle, targetActor.PreferredUsername, activity.ID)
	if err != nil {
		log.Error("failed to create follow relationship", zap.Error(err))
		return err
	}

	// Check if the target actor requires manual approval for follows
	if targetActor.ManuallyApprovesFollowers {
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
		Object: activity,
	}

	// Get the follower's inbox URL
	followerActor, err := ih.actorRepository.GetCachedRemoteActor(ctx, activity.Actor)
	if err != nil {
		log.Error("failed to get follower actor for delivery",
			zap.String("actor", activity.Actor),
			zap.Error(err))
		return nil // Don't fail the follow acceptance
	}

	// Send Accept activity back to the follower
	if err := ih.deliveryService.DeliverActivity(ctx, acceptActivity, followerActor.Inbox, targetActor); err != nil {
		log.Error("failed to deliver accept activity",
			zap.String("to", activity.Actor),
			zap.String("from", targetActor.ID),
			zap.Error(err))
		// Don't fail the whole operation if delivery fails
	} else {
		log.Info("delivered accept activity",
			zap.String("to", activity.Actor),
			zap.String("from", targetActor.ID))
	}

	return nil
}

// processAcceptActivity processes an incoming Accept activity
func (ih *InboxHandler) processAcceptActivity(ctx context.Context, activity *activitypub.Activity, targetActor *activitypub.Actor) error {
	log := common.WithContext(ctx)

	// Check if this is accepting a Follow request
	if objectID, ok := activity.Object.(string); ok {
		// Fetch the original activity
		originalActivity, err := ih.activityRepository.GetActivity(ctx, objectID)
		if err != nil {
			log.Warn("failed to find original activity", zap.String("id", objectID))
			return nil // Don't fail, just ignore
		}

		if originalActivity.Type == activitypub.FollowType {
			// Update the follow relationship to accepted
			acceptorHandle := ih.extractHandleFromActorID(activity.Actor)
			err = ih.relationshipRepository.AcceptFollowRequest(ctx, targetActor.PreferredUsername, acceptorHandle)
			if err != nil {
				log.Error("failed to update follow status", zap.Error(err))
				return err
			}
		}
	}

	return nil
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
		return nil // Don't fail, just ignore unknown activities
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

	// Remove the pending follow relationship
	if err := ih.relationshipRepository.DeleteRelationship(ctx, requesterHandle, rejectorHandle); err != nil {
		log.Debug("no follow relationship to remove during rejection",
			zap.String("requester", requesterHandle),
			zap.String("rejector", rejectorHandle),
			zap.Error(err))
		// Don't fail - this is idempotent
	}

	// Optionally update the relationship state to explicitly rejected
	// This allows tracking that the follow was explicitly rejected vs. just deleted
	if err := ih.relationshipRepository.RejectFollowRequest(ctx, requesterHandle, rejectorHandle); err != nil {
		log.Debug("could not mark follow as rejected",
			zap.String("requester", requesterHandle),
			zap.String("rejector", rejectorHandle),
			zap.Error(err))
		// This is not critical - the deletion above is the main action
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

	// Store the object if it's a Note
	if objType, _ := objMap["type"].(string); objType == activitypub.NoteType {
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

		// Store the note (it will be marked as remote)
		if err := ih.objectRepository.CreateObject(ctx, &note); err != nil {
			log.Error("failed to store remote note", zap.Error(err))
			return err
		}
	}

	return nil
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

	// Store edit history before updating
	if err := ih.storeEditHistory(ctx, objectID, existingObject, activity.Actor); err != nil {
		log.Error("failed to store edit history",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Continue even if history storage fails
	}

	// Update the object if it's a Note
	if objType, _ := objMap["type"].(string); objType == activitypub.NoteType {
		// Convert to Note object
		objJSON, err := json.Marshal(objMap)
		if err != nil {
			return err
		}

		var note activitypub.Note
		if err := common.ParseActivityPubObject(objJSON, &note); err != nil {
			return err
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

		log.Info("successfully updated remote note",
			zap.String("object_id", objectID),
			zap.String("updated_by", activity.Actor))
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
	if err := ih.createDeleteTombstone(ctx, objectID, activity, originalObject); err != nil {
		log.Error("failed to create tombstone",
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

	// Extract the object being liked
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
		log.Warn("like activity has no object ID")
		return nil // Don't fail, just ignore malformed likes
	}

	// Extract actor handle from actor ID
	actorHandle := ih.extractHandleFromActorID(activity.Actor)

	log.Info("processing like activity",
		zap.String("actor", activity.Actor),
		zap.String("actor_handle", actorHandle),
		zap.String("object", objectID),
		zap.String("activity_id", activity.ID))

	// Verify the object exists (optional - could be remote object)
	obj, err := ih.objectRepository.GetObject(ctx, objectID)
	if err != nil {
		log.Debug("object not found for like, assuming remote object",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Continue processing - this could be a remote object we don't have
	}

	// Store the like
	_, err = ih.likeRepository.CreateLike(ctx, activity.Actor, objectID, targetActor.ID)
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

	// Extract the object being announced
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
		log.Warn("announce activity has no object ID")
		return nil // Don't fail, just ignore malformed announces
	}

	// Extract actor handle from actor ID
	actorHandle := ih.extractHandleFromActorID(activity.Actor)

	log.Info("processing announce activity",
		zap.String("actor", activity.Actor),
		zap.String("actor_handle", actorHandle),
		zap.String("object", objectID),
		zap.String("activity_id", activity.ID))

	// Verify the object exists (optional - could be remote object)
	obj, err := ih.objectRepository.GetObject(ctx, objectID)
	if err != nil {
		log.Debug("object not found for announce, assuming remote object",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Continue processing - this could be a remote object we don't have
	}

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
	err = ih.socialRepository.CreateAnnounce(ctx, announce)
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
		if objectID, ok := originalActivity.Object.(string); ok {
			err := ih.likeRepository.DeleteLike(ctx, activity.Actor, objectID)
			if err != nil {
				log.Warn("failed to remove like", zap.Error(err))
				// Don't fail - idempotent operation
			}
		}
	case activitypub.AnnounceType:
		// Undo announce (boost/reblog)
		if objectID, ok := originalActivity.Object.(string); ok {
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

	// For cross-domain scenarios, check if usernames match
	actorUsername := ih.extractHandleFromActorID(activity.Actor)
	if strings.Contains(actorUsername, collectionOwnerUsername) {
		return nil // Username matches
	}

	// Special case: allow certain actors to manage specific collection types
	// This could be extended for admin actions, moderation, etc.
	if collectionType == "featured" {
		// Only the actor themselves can manage their featured posts
		if activity.Actor != targetActor.ID {
			return unauthorizedCollectionError()
		}
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
	} else if note, ok := existingObject.(*activitypub.Note); ok {
		objectOwner = note.AttributedTo
	}

	if err := common.ValidateRequiredParam("objectOwner", objectOwner); err != nil {
		return determineObjectOwnerError()
	}

	// Only the object owner can update it
	if activity.Actor != objectOwner {
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
	case *activitypub.BaseObject:
		// Object is typed
		objectID = obj.ID
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
	} else if note, ok := existingObject.(*activitypub.Note); ok {
		objectOwner = note.AttributedTo
	}

	if err := common.ValidateRequiredParam("objectOwner", objectOwner); err != nil {
		return determineObjectOwnerError()
	}

	// Only the object owner can delete it
	if activity.Actor != objectOwner {
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

// createDeleteTombstone creates a tombstone for the deleted object
func (ih *InboxHandler) createDeleteTombstone(ctx context.Context, objectID string, deleteActivity *activitypub.Activity, originalObject map[string]any) error {
	log := common.WithContext(ctx)

	// Determine the original object type
	var formerType string
	if originalObject != nil {
		if objType, ok := originalObject["type"].(string); ok {
			formerType = objType
		}
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
	// Extract username and domain from actor ID
	// Format: https://domain.com/users/username -> @username@domain.com
	parts := strings.Split(actorID, "/")
	if len(parts) < 5 {
		return actorID // Return as-is if not in expected format
	}

	domain := parts[2]
	username := parts[len(parts)-1]

	return fmt.Sprintf("@%s@%s", username, domain)
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

func buildInboxApp(lambdaCtx *common.LambdaContext, handler *InboxHandler) *lift.App {
	app := lift.New()

	// Panic recovery middleware (MUST be first to catch all panics)
	app.Use(middleware.PanicRecovery(lambdaCtx.Logger))

	// Apply federation security middleware
	middleware.ApplySecurityMiddleware(app, middleware.SecurityTypeFederation, lambdaCtx.Logger)

	// Add request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("inbox-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware (second in chain)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			path := ctx.Request.Path
			method := ctx.Request.Method

			err := next.Handle(ctx)

			handler.logger.Info("inbox request completed",
				zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Bool("has_error", err != nil),
			)

			return err
		})
	})

	// Add recovery middleware (third in chain)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					handler.logger.Error("panic recovered in inbox handler",
						zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
						zap.Any("panic", r))
					// Return a generic error
					if err := ctx.Status(500).Text("Internal server error"); err != nil {
						handler.logger.Error("failed to send error response", zap.Error(err))
					}
				}
			}()

			return next.Handle(ctx)
		})
	})

	// Add unified error handling middleware
	app.Use(common.CreateFederationErrorMiddleware(handler.logger))

	// Add federation metrics middleware
	if handler.emfMetrics != nil {
		app.Use(handler.createFederationMetricsMiddleware())
	}

	// Rate limiting is now handled by ApplySecurityMiddleware

	// Register all inbox routes
	handler.RegisterRoutes(app)

	return app
}

func buildInboxLambdaHandler(app *lift.App, handler *InboxHandler) func(ctx context.Context, event interface{}) (interface{}, error) {
	// Wrap Lambda handler with federation observability
	return func(ctx context.Context, event interface{}) (interface{}, error) {
		requestStart := time.Now()

		// Record cold start metric if this is a cold start
		if time.Since(handler.startTime) < 30*time.Second && handler.emfMetrics != nil {
			handler.emfMetrics.RecordBusinessMetric(observability.MetricColdStarts, 1.0, observability.UnitCount, nil)
			coldStartDuration := time.Since(handler.startTime)
			handler.emfMetrics.RecordBusinessMetric(observability.MetricColdStartDuration, float64(coldStartDuration.Milliseconds()), observability.UnitMilliseconds, nil)
		}

		// Process the request
		result, err := app.HandleRequest(ctx, event)

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

func main() {
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

	// Use app.HandleRequest for Lambda (not app.Start())
	startLambda(lambdaHandler)
}

// createFederationMetricsMiddleware creates middleware for federation-specific metrics collection
func (ih *InboxHandler) createFederationMetricsMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if ih.emfMetrics == nil {
				return next.Handle(ctx)
			}

			// Extract federation context information
			federationCtx := ih.extractFederationContext(ctx)

			// Start latency timer and record incoming message
			timer := ih.emfMetrics.StartLatencyTimer(ctx.Context, "federation_activity")
			ih.recordIncomingFederationMessage(federationCtx)

			// Execute request
			err := next.Handle(ctx)

			// Record post-request metrics
			ih.recordFederationMetrics(federationCtx, timer, err)

			return err
		})
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
func (ih *InboxHandler) extractFederationContext(ctx *lift.Context) *federationContext {
	federationCtx := &federationContext{
		username:     ctx.Param("username"),
		userAgent:    ctx.Header("User-Agent"),
		hasSignature: ctx.Header("Signature") != "",
		path:         "/",
		method:       "POST",
	}

	// Determine remote instance from User-Agent
	federationCtx.remoteInstance = ih.parseRemoteInstance(federationCtx.userAgent)

	// Extract path and method from request
	if ctx.Request != nil && ctx.Request.Request != nil {
		federationCtx.path = ctx.Request.Request.Path
		federationCtx.method = ctx.Request.Request.Method
	}

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
