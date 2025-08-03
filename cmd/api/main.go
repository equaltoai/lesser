package main

/*
Lesser API Server - Mastodon-compatible ActivityPub implementation

This Lambda function serves the Lesser API using AWS API Gateway v2.
All routing is handled by the Lift framework.

The API Gateway configuration strips the /api/v1 and /api/v2 prefixes
before passing requests to this Lambda, so the router receives clean paths.
*/

import (
	"context"
	"os"
	"time"

	liftHandlers "github.com/equaltoai/lesser/cmd/api/lift"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	liftAuth "github.com/equaltoai/lesser/pkg/lift"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

var (
	cfg              *config.Config
	store            storage.Storage
	logger           *zap.Logger
	liftHandler      *liftHandlers.Handler
	authService      *auth.AuthService
	liftAuthSvc      *liftAuth.LiftAuthService
	emfMetricsService *observability.EMFMetricsService
	initTime         time.Time
)

func init() {
	initTime = time.Now()
	cfg = config.Get()
	logger = common.Logger()

	// Initialize DynamORM
	tableName := os.Getenv("DYNAMODB_TABLE")
	if tableName == "" {
		tableName = cfg.DynamoTableName
	}
	if tableName == "" {
		logger.Fatal("DYNAMODB_TABLE environment variable is required")
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Create storage adapter
	store = dynamorm.NewStorageAdapter(db, tableName, logger)
	
	// Initialize repositories
	adapter := store.(*dynamorm.StorageAdapter)
	adapter.SetActorRepository(repositories.NewActorRepository(db, tableName, logger))
	adapter.SetObjectRepository(repositories.NewObjectRepository(db, tableName, cfg.Domain, logger))
	adapter.SetActivityRepository(repositories.NewActivityRepository(db, tableName, logger))
	adapter.SetUserRepository(repositories.NewUserRepository(db, tableName, logger))
	adapter.SetTimelineRepository(repositories.NewTimelineRepository(db, tableName, logger))
	adapter.SetNotificationRepository(repositories.NewNotificationRepository(db, tableName, logger))
	adapter.SetLikeRepository(repositories.NewLikeRepository(db, tableName, logger))
	adapter.SetModerationRepository(repositories.NewModerationRepository(db, tableName, logger))
	adapter.SetListRepository(repositories.NewListRepository(db, tableName, logger))
	adapter.SetMediaRepository(repositories.NewMediaRepository(db, tableName, logger))
	adapter.SetPollRepository(repositories.NewPollRepository(db, tableName, logger))
	adapter.SetHashtagRepository(repositories.NewHashtagRepository(db, tableName, logger, cfg.Domain))
	adapter.SetScheduledStatusRepository(repositories.NewScheduledStatusRepository(db, tableName, logger))
	adapter.SetAnnouncementRepository(repositories.NewAnnouncementRepository(db, tableName, logger))
	adapter.SetDomainBlockRepository(repositories.NewDomainBlockRepository(db, tableName, logger))
	adapter.SetAuthRepository(repositories.NewAuthRepository(db, tableName, logger))
	adapter.SetRelationshipRepository(repositories.NewRelationshipRepository(db, tableName, logger))
	adapter.SetInstanceRepository(repositories.NewInstanceRepository(db, tableName, logger))
	adapter.SetFederationRepository(repositories.NewFederationRepository(db, logger))

	// Initialize auth service
	authService, err = auth.NewAuthService(store)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	// Initialize Lift-native auth service
	liftAuthSvc = liftAuth.NewLiftAuthService(authService)

	// Initialize EMF metrics service (replaces polling-based metrics)
	if os.Getenv("DISABLE_METRICS") != "true" {
		emfMetricsService = observability.NewEMFMetricsService(logger)
		logger.Info("initialized EMF metrics service for serverless")
	}

	// Create handler with all dependencies
	legacyAuthMiddleware, err := auth.GetMiddleware()
	if err != nil {
		logger.Fatal("failed to initialize legacy auth middleware", zap.Error(err))
	}
	
	// Create Lift handler for all endpoints
	liftHandler = liftHandlers.NewHandler(cfg, store, logger, legacyAuthMiddleware)
}

func main() {
	// Create a new Lift application
	app := lift.New()
	if os.Getenv("DEBUG") == "true" {
		app = lift.New(lift.WithDebug())
	}

	// Add global middleware
	// Add timeout middleware
	app.Use(middleware.TimeoutMiddleware(middleware.TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
	}))

	// Add custom logging middleware
	app.Use(createLoggingMiddleware(logger))

	// Add CORS middleware
	app.Use(createCORSMiddleware())
	
	// Add cost tracking middleware
	app.Use(createCostTrackingMiddleware(logger))
	
	// Add EMF-based performance monitoring middleware (no polling)
	if emfMetricsService != nil {
		app.Use(observability.CreateEMFPerformanceMonitoringMiddleware(emfMetricsService))
	}

	// Configure routes
	// TODO: Restore routes after migration
	// configurePublicRoutes(app)
	// configureAuthenticatedReadRoutes(app)
	// configureAuthenticatedWriteRoutes(app)
	// configureAdminRoutes(app)
	
	// Configure native Lift routes (controlled by environment variable)
	configureLiftRoutes(app)

	// Start the Lambda handler with EMF metrics flushing
	// Note: Cost tracking is handled within the handlers via context
	lambdaHandler := func(ctx context.Context, event interface{}) (interface{}, error) {
		// Process the request
		result, err := app.HandleRequest(ctx, event)
		
		// CRITICAL: Flush EMF metrics before Lambda terminates
		// This ensures all metrics are written to CloudWatch
		if emfMetricsService != nil {
			if flushErr := emfMetricsService.FlushMetrics(); flushErr != nil {
				logger.Error("failed to flush EMF metrics", zap.Error(flushErr))
				// Don't fail the request due to metrics issues
			}
		}
		
		return result, err
	}
	
	lambda.Start(lambdaHandler)
}
