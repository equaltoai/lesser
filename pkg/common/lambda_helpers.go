// Package common provides Lambda-specific helper utilities for standardized initialization
package common

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
)

// LambdaInitOptions provides additional initialization options
type LambdaInitOptions struct {
	// Storage options
	InitializeStorage     bool
	InitializeRepositories bool
	TableName            string
	
	// Observability options
	InitializeEMFMetrics     bool
	InitializeHealthChecker  bool
	InitializeTracingManager bool
	InitializeMetricsCollector bool
	InitializeLatencyTracking bool
	InitializeAlerting       bool
	
	// Service-specific options
	InitializeAuthService    bool
	InitializeFederationServices bool
	InitializeStreamingServices bool
	
	// Performance options
	OptimizeForColdStart bool
	EnableServiceCaching bool
}

// DefaultLambdaInitOptions returns sensible defaults for different Lambda types
func DefaultLambdaInitOptions(lambdaType LambdaType) LambdaInitOptions {
	baseOptions := LambdaInitOptions{
		InitializeStorage:     true,
		InitializeRepositories: true,
		OptimizeForColdStart:  true,
		EnableServiceCaching:  true,
	}
	
	switch lambdaType {
	case LambdaTypeAPI:
		return LambdaInitOptions{
			InitializeStorage:            true,
			InitializeRepositories:       true,
			InitializeEMFMetrics:         true,
			InitializeHealthChecker:      true,
			InitializeTracingManager:     true,
			InitializeMetricsCollector:   true,
			InitializeLatencyTracking:    true,
			InitializeAlerting:           false, // API uses different alerting
			InitializeAuthService:        true,
			InitializeFederationServices: false,
			InitializeStreamingServices:  true,
			OptimizeForColdStart:         true,
			EnableServiceCaching:         true,
		}
		
	case LambdaTypeFederation:
		return LambdaInitOptions{
			InitializeStorage:            true,
			InitializeRepositories:       true,
			InitializeEMFMetrics:         true,
			InitializeHealthChecker:      false,
			InitializeTracingManager:     true,
			InitializeMetricsCollector:   false,
			InitializeLatencyTracking:    false,
			InitializeAlerting:           true,
			InitializeAuthService:        false,
			InitializeFederationServices: true,
			InitializeStreamingServices:  false,
			OptimizeForColdStart:         true,
			EnableServiceCaching:         true,
		}
		
	case LambdaTypeProcessor:
		return LambdaInitOptions{
			InitializeStorage:            true,
			InitializeRepositories:       true,
			InitializeEMFMetrics:         true,
			InitializeHealthChecker:      false,
			InitializeTracingManager:     false,
			InitializeMetricsCollector:   false,
			InitializeLatencyTracking:    false,
			InitializeAlerting:           false,
			InitializeAuthService:        false,
			InitializeFederationServices: false,
			InitializeStreamingServices:  false,
			OptimizeForColdStart:         true,
			EnableServiceCaching:         false, // Processors are typically one-shot
		}
		
	case LambdaTypeMedia:
		return LambdaInitOptions{
			InitializeStorage:            true,
			InitializeRepositories:       true,
			InitializeEMFMetrics:         true,
			InitializeHealthChecker:      false,
			InitializeTracingManager:     true,
			InitializeMetricsCollector:   false,
			InitializeLatencyTracking:    false,
			InitializeAlerting:           true,
			InitializeAuthService:        false,
			InitializeFederationServices: false,
			InitializeStreamingServices:  false,
			OptimizeForColdStart:         false, // Media processing is long-running
			EnableServiceCaching:         true,
		}
		
	case LambdaTypeAI:
		return LambdaInitOptions{
			InitializeStorage:            true,
			InitializeRepositories:       true,
			InitializeEMFMetrics:         true,
			InitializeHealthChecker:      false,
			InitializeTracingManager:     true,
			InitializeMetricsCollector:   false,
			InitializeLatencyTracking:    false,
			InitializeAlerting:           true,
			InitializeAuthService:        false,
			InitializeFederationServices: false,
			InitializeStreamingServices:  false,
			OptimizeForColdStart:         false, // AI processing is long-running
			EnableServiceCaching:         true,
		}
		
	default: // LambdaTypeBasic
		return baseOptions
	}
}

// InitializeStorageServices initializes DynamORM and repository storage
func (lambdaCtx *LambdaContext) InitializeStorageServices(options LambdaInitOptions) error {
	if !options.InitializeStorage {
		return nil
	}
	
	logger := lambdaCtx.Logger
	cfg := lambdaCtx.Config
	
	// Get table name from options or environment
	tableName := options.TableName
	if tableName == "" {
		tableName = os.Getenv("DYNAMODB_TABLE")
		if tableName == "" {
			tableName = cfg.DynamoTableName
		}
	}
	if tableName == "" {
		return fmt.Errorf("DYNAMODB_TABLE is required for storage initialization")
	}
	
	logger.Debug("initializing storage services", 
		zap.String("table_name", tableName),
		zap.Bool("optimize_cold_start", options.OptimizeForColdStart))
	
	// Initialize DynamORM with appropriate optimization
	var db interface{}
	
	if options.OptimizeForColdStart {
		// Use Lambda-optimized client for faster cold starts
		dynamormClient, dynamormErr := initializeDynamORM(context.Background(), cfg.Region, true)
		if dynamormErr != nil {
			return fmt.Errorf("failed to initialize Lambda-optimized DynamORM: %w", dynamormErr)
		}
		db = dynamormClient
	} else {
		// Use standard client for long-running operations
		dynamormClient, dynamormErr := initializeDynamORM(context.Background(), cfg.Region, false)
		if dynamormErr != nil {
			return fmt.Errorf("failed to initialize DynamORM: %w", dynamormErr)
		}
		db = dynamormClient
	}
	
	lambdaCtx.DynamoDB = db
	
	// Initialize repository factory if requested
	if options.InitializeRepositories {
		repos, repoErr := initializeRepositoryFactory(db, tableName, lambdaCtx.AWSServices, logger)
		if repoErr != nil {
			return fmt.Errorf("failed to initialize repository factory: %w", repoErr)
		}
		lambdaCtx.Repos = repos
		
		logger.Debug("initialized repository storage",
			zap.String("table_name", tableName))
	}
	
	return nil
}

// InitializeObservabilityServices initializes observability services based on options
func (lambdaCtx *LambdaContext) InitializeObservabilityServices(options LambdaInitOptions) error {
	logger := lambdaCtx.Logger
	cfg := lambdaCtx.Config
	
	// Skip if metrics are disabled globally
	if os.Getenv("DISABLE_METRICS") == "true" {
		logger.Info("metrics disabled globally, skipping observability initialization")
		return nil
	}
	
	logger.Debug("initializing observability services",
		zap.Bool("emf_metrics", options.InitializeEMFMetrics),
		zap.Bool("health_checker", options.InitializeHealthChecker),
		zap.Bool("tracing", options.InitializeTracingManager),
		zap.Bool("latency_tracking", options.InitializeLatencyTracking))
	
	// Initialize EMF Metrics
	if options.InitializeEMFMetrics {
		namespace := fmt.Sprintf("Lesser/%s", TitleCase(lambdaCtx.ServiceName))
		emfMetrics := initializeEMFMetrics(logger, namespace, lambdaCtx.ServiceName, cfg)
		lambdaCtx.EMFMetrics = emfMetrics
		
		logger.Debug("initialized EMF metrics",
			zap.String("namespace", namespace))
	}
	
	// Initialize Health Checker
	if options.InitializeHealthChecker {
		tableName := ""
		if lambdaCtx.Repos != nil {
			// Extract table name from config
			tableName = cfg.DynamoTableName
		}
		
		healthChecker := initializeHealthChecker(logger, lambdaCtx.AWSServices, lambdaCtx.ServiceName, cfg.Version, tableName)
		lambdaCtx.HealthChecker = healthChecker
		
		logger.Debug("initialized health checker",
			zap.String("service", lambdaCtx.ServiceName))
	}
	
	// Initialize Tracing Manager
	if options.InitializeTracingManager && lambdaCtx.Config != nil {
		tracingManager := initializeTracingManager(logger, lambdaCtx.ServiceName, cfg.Version)
		lambdaCtx.TracingManager = tracingManager
		
		logger.Debug("initialized tracing manager",
			zap.String("service", lambdaCtx.ServiceName),
			zap.Bool("enabled", isTracingEnabled(tracingManager)))
	}
	
	// Initialize Metrics Collector (legacy support)
	if options.InitializeMetricsCollector && lambdaCtx.AWSServices != nil {
		metricsCollector := initializeMetricsCollector(lambdaCtx.AWSServices, logger, lambdaCtx.ServiceName)
		lambdaCtx.MetricsCollector = metricsCollector
		
		logger.Debug("initialized metrics collector")
	}
	
	// Initialize Latency Tracking
	if options.InitializeLatencyTracking && lambdaCtx.Repos != nil {
		latencyAggregator, latencyAlerter := initializeLatencyTracking(logger, lambdaCtx.Repos, lambdaCtx.ServiceName)
		lambdaCtx.LatencyAggregator = latencyAggregator
		lambdaCtx.LatencyAlerter = latencyAlerter
		
		logger.Debug("initialized latency tracking")
	}
	
	// Initialize Alert Manager (federation-specific)
	if options.InitializeAlerting {
		alertManager := initializeAlertManager(logger)
		lambdaCtx.AlertManager = alertManager
		
		logger.Debug("initialized alert manager")
	}
	
	return nil
}

// InitializeServiceSpecificDependencies initializes service-specific dependencies
func (lambdaCtx *LambdaContext) InitializeServiceSpecificDependencies(options LambdaInitOptions) error {
	logger := lambdaCtx.Logger
	
	logger.Debug("initializing service-specific dependencies",
		zap.Bool("auth_service", options.InitializeAuthService),
		zap.Bool("federation_services", options.InitializeFederationServices),
		zap.Bool("streaming_services", options.InitializeStreamingServices))
	
	// Initialize Auth Service
	if options.InitializeAuthService && lambdaCtx.Repos != nil {
		authService, authMiddleware, err := initializeAuthServices(lambdaCtx.Repos)
		if err != nil {
			return fmt.Errorf("failed to initialize auth services: %w", err)
		}
		lambdaCtx.AuthService = authService
		lambdaCtx.AuthMiddleware = authMiddleware
		
		logger.Debug("initialized auth services")
	}
	
	// Initialize Federation Services
	if options.InitializeFederationServices && lambdaCtx.Repos != nil {
		signatureService, deliveryService, costCalculator, rateLimiter := initializeFederationServices(lambdaCtx.Repos, logger)
		lambdaCtx.SignatureService = signatureService
		lambdaCtx.DeliveryService = deliveryService
		lambdaCtx.CostCalculator = costCalculator
		lambdaCtx.RateLimiter = rateLimiter
		
		logger.Debug("initialized federation services")
	}
	
	// Initialize Streaming Services
	if options.InitializeStreamingServices && lambdaCtx.DynamoDB != nil {
		streamQueue := initializeStreamingServices(lambdaCtx.DynamoDB, lambdaCtx.Config.DynamoTableName, logger)
		lambdaCtx.StreamQueue = streamQueue
		
		logger.Debug("initialized streaming services")
	}
	
	return nil
}

// InitializeWithDefaults initializes Lambda with default options for the Lambda type
func (lambdaCtx *LambdaContext) InitializeWithDefaults() error {
	options := DefaultLambdaInitOptions(lambdaCtx.LambdaType)
	
	if err := lambdaCtx.InitializeStorageServices(options); err != nil {
		return fmt.Errorf("failed to initialize storage services: %w", err)
	}
	
	if err := lambdaCtx.InitializeObservabilityServices(options); err != nil {
		return fmt.Errorf("failed to initialize observability services: %w", err)
	}
	
	if err := lambdaCtx.InitializeServiceSpecificDependencies(options); err != nil {
		return fmt.Errorf("failed to initialize service-specific dependencies: %w", err)
	}
	
	return nil
}

// InitializeWithOptions initializes Lambda with custom options
func (lambdaCtx *LambdaContext) InitializeWithOptions(options LambdaInitOptions) error {
	if err := lambdaCtx.InitializeStorageServices(options); err != nil {
		return fmt.Errorf("failed to initialize storage services: %w", err)
	}
	
	if err := lambdaCtx.InitializeObservabilityServices(options); err != nil {
		return fmt.Errorf("failed to initialize observability services: %w", err)
	}
	
	if err := lambdaCtx.InitializeServiceSpecificDependencies(options); err != nil {
		return fmt.Errorf("failed to initialize service-specific dependencies: %w", err)
	}
	
	return nil
}

// FlushObservabilityServices flushes all observability data before Lambda termination
func (lambdaCtx *LambdaContext) FlushObservabilityServices() {
	// Flush EMF metrics
	if lambdaCtx.EMFMetrics != nil {
		flushEMFMetrics(lambdaCtx.EMFMetrics)
	}
	
	// Flush legacy metrics collector
	if lambdaCtx.MetricsCollector != nil {
		flushMetricsCollector(lambdaCtx.MetricsCollector)
	}
	
	// Note: Latency aggregator stops automatically due to Lambda termination
	// No explicit flush needed as it's designed for graceful shutdown
}

// CreateStandardizedLambdaHandler creates a Lambda handler with standardized observability and error handling
func (lambdaCtx *LambdaContext) CreateStandardizedLambdaHandler(handler func(ctx context.Context, event interface{}) (interface{}, error)) func(ctx context.Context, event interface{}) (interface{}, error) {
	return func(ctx context.Context, event interface{}) (interface{}, error) {
		requestStart := time.Now()
		
		// Record cold start metrics if this is a cold start
		if time.Since(lambdaCtx.StartTime) < 30*time.Second {
			recordColdStartMetrics(lambdaCtx, time.Since(lambdaCtx.StartTime))
		}
		
		// Process the request
		result, err := handler(ctx, event)
		
		// Record request metrics
		requestDuration := time.Since(requestStart)
		recordRequestMetrics(lambdaCtx, requestDuration, err)
		
		// Flush all observability data before termination
		lambdaCtx.FlushObservabilityServices()
		
		return result, err
	}
}

// Utility functions to avoid import cycles (implementations use interfaces)

// These functions would be implemented using type assertions to call the actual service methods
// They're placeholders to avoid import cycles while maintaining clean interfaces

func initializeDynamORM(ctx context.Context, region string, optimizeForLambda bool) (interface{}, error) {
	// Implementation would import dynamorm and initialize client
	// Returns interface{} to avoid import cycles
	return nil, fmt.Errorf("DynamORM initialization to be implemented in service-specific code")
}

func initializeRepositoryFactory(db interface{}, tableName string, awsServices interface{}, logger *zap.Logger) (interface{}, error) {
	// Implementation would import factory and create repository storage
	return nil, fmt.Errorf("repository factory initialization to be implemented in service-specific code")
}

func initializeEMFMetrics(logger *zap.Logger, namespace, serviceName string, cfg interface{}) interface{} {
	// Implementation would import observability and create EMF metrics
	return nil
}

func initializeHealthChecker(logger *zap.Logger, awsServices interface{}, serviceName, version, tableName string) interface{} {
	// Implementation would import observability and create health checker
	return nil
}

func initializeTracingManager(logger *zap.Logger, serviceName, version string) interface{} {
	// Implementation would import observability and create tracing manager
	return nil
}

func initializeMetricsCollector(awsServices interface{}, logger *zap.Logger, serviceName string) interface{} {
	// Implementation would import observability and create metrics collector
	return nil
}

func initializeLatencyTracking(logger *zap.Logger, repos interface{}, serviceName string) (interface{}, interface{}) {
	// Implementation would import observability and create latency tracking services
	return nil, nil
}

func initializeAlertManager(logger *zap.Logger) interface{} {
	// Implementation would import monitoring and create alert manager
	return nil
}

func initializeAuthServices(repos interface{}) (interface{}, interface{}, error) {
	// Implementation would import auth and create auth service and middleware
	return nil, nil, fmt.Errorf("auth services initialization to be implemented in service-specific code")
}

func initializeFederationServices(repos interface{}, logger *zap.Logger) (interface{}, interface{}, interface{}, interface{}) {
	// Implementation would import federation and create federation services
	return nil, nil, nil, nil
}

func initializeStreamingServices(db interface{}, tableName string, logger *zap.Logger) interface{} {
	// Implementation would import streaming and create stream queue
	return nil
}

func flushEMFMetrics(emfMetrics interface{}) {
	// Implementation would flush EMF metrics
}

func flushMetricsCollector(metricsCollector interface{}) {
	// Implementation would flush metrics collector
}

func isTracingEnabled(tracingManager interface{}) bool {
	// Implementation would check if tracing is enabled
	return false
}

func recordColdStartMetrics(lambdaCtx *LambdaContext, coldStartDuration time.Duration) {
	// Implementation would record cold start metrics to EMF
	if lambdaCtx.EMFMetrics != nil {
		lambdaCtx.Logger.Info("cold start detected",
			zap.String("service", lambdaCtx.ServiceName),
			zap.Duration("cold_start_duration", coldStartDuration))
	}
}

func recordRequestMetrics(lambdaCtx *LambdaContext, requestDuration time.Duration, err error) {
	// Implementation would record request metrics to EMF
	if lambdaCtx.EMFMetrics != nil {
		lambdaCtx.Logger.Info("request completed",
			zap.String("service", lambdaCtx.ServiceName),
			zap.Duration("duration", requestDuration),
			zap.Bool("success", err == nil))
	}
}