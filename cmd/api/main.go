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

	"github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	liftAuth "github.com/equaltoai/lesser/pkg/lift"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage"
	storageDB "github.com/equaltoai/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

var (
	cfg              *config.Config
	store            storage.Storage
	logger           *zap.Logger
	handler          *handlers.Handler
	authService      *auth.AuthService
	liftAuthSvc      *liftAuth.LiftAuthService
	metricsCollector *observability.MetricsCollector
	initTime         time.Time
)

func init() {
	initTime = time.Now()
	cfg = config.Get()
	logger = common.Logger()

	var err error
	store, err = storageDB.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Initialize auth service directly
	authService, err = auth.NewAuthService(store)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	// Initialize Lift-native auth service
	liftAuthSvc = liftAuth.NewLiftAuthService(authService)

	// Initialize metrics collector
	if os.Getenv("DISABLE_METRICS") != "true" {
		ctx := context.Background()
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
		if err != nil {
			logger.Warn("failed to load AWS config for metrics", zap.Error(err))
		} else {
			cloudwatchClient := cloudwatch.NewFromConfig(awsCfg)
			metricsCollector = observability.NewMetricsCollector(
				cloudwatchClient,
				"Lesser/API",
				logger,
			)
		}
	}

	// Create handler with all dependencies (still needs old middleware for legacy handlers)
	legacyAuthMiddleware, err := auth.GetMiddleware()
	if err != nil {
		logger.Fatal("failed to initialize legacy auth middleware", zap.Error(err))
	}
	handler = handlers.NewHandler(cfg, store, logger, legacyAuthMiddleware)
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
	
	// Add performance monitoring middleware
	if metricsCollector != nil {
		app.Use(createPerformanceMonitoringMiddleware(metricsCollector))
	}

	// Configure routes
	configurePublicRoutes(app)
	configureAuthenticatedReadRoutes(app)
	configureAuthenticatedWriteRoutes(app)
	configureAdminRoutes(app)

	// Start the Lambda handler
	// Note: Cost tracking is handled within the handlers via context
	lambda.Start(app.HandleRequest)
}
