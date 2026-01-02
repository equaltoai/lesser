// Package common provides Lambda initialization helpers and patterns.
package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/config"
	"go.uber.org/zap"
)

var initializeAWSServicesFunc = awsInit.InitializeServices

// LambdaType defines the type of Lambda function for initialization
type LambdaType string

const (
	// LambdaTypeAPI represents the API Lambda function type
	LambdaTypeAPI LambdaType = "api"
	// LambdaTypeProcessor represents the processor Lambda function type
	LambdaTypeProcessor LambdaType = "processor"
	// LambdaTypeMedia represents the media Lambda function type
	LambdaTypeMedia LambdaType = "media"
	// LambdaTypeFederation represents the federation Lambda function type
	LambdaTypeFederation LambdaType = "federation"
	// LambdaTypeAI represents the AI Lambda function type
	LambdaTypeAI LambdaType = "ai"
	// LambdaTypeBasic represents the basic Lambda function type
	LambdaTypeBasic LambdaType = "basic"
)

// LambdaConfig defines Lambda initialization configuration
type LambdaConfig struct {
	// Basic configuration
	ServiceName string
	LambdaType  LambdaType
	Version     string

	// Feature flags
	EnableMetrics      bool
	EnableTracing      bool
	EnableHealthCheck  bool
	EnableCostTracking bool

	// Timeout and retry settings
	RequestTimeout   time.Duration
	RetryMaxAttempts int

	// Custom AWS service requirements (overrides defaults)
	CustomServiceConfig *awsInit.ServiceConfig
}

// LambdaContext contains initialized Lambda dependencies
type LambdaContext struct {
	// Configuration
	Config    *config.Config
	Logger    *zap.Logger
	StartTime time.Time

	// AWS Services
	AWSServices *awsInit.AWSServices

	// Storage (interfaces to avoid import cycles)
	DynamoDB interface{} // *dynamorm.Client
	Repos    interface{} // core.RepositoryStorage

	// Observability (interfaces to avoid import cycles)
	EMFMetrics        interface{} // *observability.EMFMetrics
	HealthChecker     interface{} // *observability.HealthChecker
	TracingManager    interface{} // *observability.TracingManager
	MetricsCollector  interface{} // *observability.MetricsCollector
	LatencyAggregator interface{} // *observability.LatencyAggregator
	LatencyAlerter    interface{} // *observability.LatencyAlerter
	AlertManager      interface{} // *monitoring.AlertManager

	// Service specific
	ServiceName string
	LambdaType  LambdaType

	// Service utilities (interfaces to avoid import cycles)
	AuthService      interface{} // *auth.AuthService
	AuthMiddleware   interface{} // *auth.Middleware
	StreamQueue      interface{} // *streaming.StreamQueue
	SignatureService interface{} // *federation.SignatureService
	DeliveryService  interface{} // *federation.DeliveryService
	CostCalculator   interface{} // *federation.CostCalculator
	RateLimiter      interface{} // *auth.RateLimiter
}

// InitializeLambda initializes a Lambda function with standard dependencies
func InitializeLambda(lambdaConfig LambdaConfig) (*LambdaContext, error) {
	startTime := time.Now()

	// Initialize basic dependencies
	cfg := config.Get()
	logger := Logger()

	// Set defaults with environment variable overrides
	if lambdaConfig.ServiceName == "" {
		lambdaConfig.ServiceName = GetLambdaFunctionName()
		if lambdaConfig.ServiceName == "" {
			lambdaConfig.ServiceName = "unknown"
		}
	}

	if string(lambdaConfig.LambdaType) == "" {
		lambdaConfig.LambdaType = detectLambdaType(lambdaConfig.ServiceName)
	}

	// Apply environment-based defaults
	if lambdaConfig.RequestTimeout == 0 {
		lambdaConfig.RequestTimeout = getTimeoutForLambdaType(lambdaConfig.LambdaType)
	}

	if lambdaConfig.RetryMaxAttempts == 0 {
		lambdaConfig.RetryMaxAttempts = 3
	}

	// Apply feature flag defaults
	if !lambdaConfig.EnableMetrics {
		lambdaConfig.EnableMetrics = !cfg.DisableMetrics
	}

	if !lambdaConfig.EnableTracing {
		lambdaConfig.EnableTracing = cfg.XRayTracingEnabled
	}

	if !lambdaConfig.EnableHealthCheck {
		lambdaConfig.EnableHealthCheck = shouldEnableHealthCheck(lambdaConfig.LambdaType)
	}

	if !lambdaConfig.EnableCostTracking {
		lambdaConfig.EnableCostTracking = !cfg.DisableCostTracking
	}

	logger.Info("initializing Lambda function",
		zap.String("service", lambdaConfig.ServiceName),
		zap.String("type", string(lambdaConfig.LambdaType)),
		zap.String("version", lambdaConfig.Version),
		zap.Bool("metrics", lambdaConfig.EnableMetrics),
		zap.Bool("tracing", lambdaConfig.EnableTracing),
		zap.Bool("health_check", lambdaConfig.EnableHealthCheck),
		zap.Bool("cost_tracking", lambdaConfig.EnableCostTracking))

	ctx := context.Background()

	// Get AWS service configuration with intelligent defaults
	var serviceConfig awsInit.ServiceConfig
	if lambdaConfig.CustomServiceConfig != nil {
		serviceConfig = *lambdaConfig.CustomServiceConfig
	} else {
		serviceConfig = getDefaultServiceConfig(lambdaConfig.LambdaType)
	}

	// Override with Lambda config settings
	serviceConfig.ServiceName = lambdaConfig.ServiceName
	serviceConfig.RequestTimeout = lambdaConfig.RequestTimeout
	serviceConfig.RetryMaxAttempts = lambdaConfig.RetryMaxAttempts

	// Initialize AWS services
	awsServices, err := initializeAWSServicesFunc(ctx, serviceConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS services: %w", err)
	}

	// Create Lambda context
	lambdaCtx := &LambdaContext{
		Config:      cfg,
		Logger:      logger,
		StartTime:   startTime,
		AWSServices: awsServices,
		DynamoDB:    nil, // To be set by helper functions
		Repos:       nil, // To be set by helper functions
		ServiceName: lambdaConfig.ServiceName,
		LambdaType:  lambdaConfig.LambdaType,
	}

	initDuration := time.Since(startTime)
	logger.Info("Lambda initialization completed",
		zap.String("service", lambdaConfig.ServiceName),
		zap.Duration("init_duration", initDuration))

	return lambdaCtx, nil
}

// MustInitializeLambda is like InitializeLambda but panics on error
func MustInitializeLambda(lambdaConfig LambdaConfig) *LambdaContext {
	lambdaCtx, err := InitializeLambda(lambdaConfig)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize Lambda: %v", err))
	}
	return lambdaCtx
}

// initializeObservability would set up observability services
// Removed to avoid import cycles - services should initialize observability independently

// getDefaultServiceConfig returns default AWS service configuration for Lambda type
func getDefaultServiceConfig(lambdaType LambdaType) awsInit.ServiceConfig {
	switch lambdaType {
	case LambdaTypeAPI:
		return awsInit.APIServiceConfig()
	case LambdaTypeProcessor:
		return awsInit.ProcessorServiceConfig()
	case LambdaTypeMedia:
		return awsInit.MediaServiceConfig()
	case LambdaTypeFederation:
		return awsInit.FederationServiceConfig()
	case LambdaTypeAI:
		return awsInit.AIServiceConfig()
	default:
		return awsInit.BasicServiceConfig()
	}
}

// FlushObservability flushes all observability data before Lambda termination
func (lambdaCtx *LambdaContext) FlushObservability() {
	// Observability flushing removed to avoid import cycles
	// Services should handle flushing independently
}

// CreateLambdaHandler creates a Lambda handler wrapper with basic logging
func (lambdaCtx *LambdaContext) CreateLambdaHandler(handler func(ctx context.Context, event interface{}) (interface{}, error)) func(ctx context.Context, event interface{}) (interface{}, error) {
	return func(ctx context.Context, event interface{}) (interface{}, error) {
		requestStart := time.Now()

		// Log cold start if this is a cold start
		if time.Since(lambdaCtx.StartTime) < 30*time.Second {
			coldStartDuration := time.Since(lambdaCtx.StartTime)
			lambdaCtx.Logger.Info("cold start detected",
				zap.String("service", lambdaCtx.ServiceName),
				zap.Duration("cold_start_duration", coldStartDuration))
		}

		// Process the request
		result, err := handler(ctx, event)

		// Log request completion
		requestDuration := time.Since(requestStart)
		lambdaCtx.Logger.Info("request completed",
			zap.String("service", lambdaCtx.ServiceName),
			zap.Duration("duration", requestDuration),
			zap.Bool("success", err == nil))

		// Flush observability data
		lambdaCtx.FlushObservability()

		return result, err
	}
}

// detectLambdaType automatically detects Lambda type from service name
func detectLambdaType(serviceName string) LambdaType {
	switch {
	case serviceName == "api":
		return LambdaTypeAPI
	case strings.Contains(serviceName, "processor"):
		return LambdaTypeProcessor
	case strings.Contains(serviceName, "media"):
		return LambdaTypeMedia
	case serviceName == "inbox" || serviceName == "outbox" || strings.Contains(serviceName, "federation"):
		return LambdaTypeFederation
	case strings.Contains(serviceName, "ai"):
		return LambdaTypeAI
	default:
		return LambdaTypeBasic
	}
}

// getTimeoutForLambdaType returns appropriate timeout for Lambda type
func getTimeoutForLambdaType(lambdaType LambdaType) time.Duration {
	switch lambdaType {
	case LambdaTypeMedia:
		return 5 * time.Minute // Media processing takes longer
	case LambdaTypeAI:
		return 2 * time.Minute // AI processing takes longer
	default:
		return 30 * time.Second // Standard timeout
	}
}

// shouldEnableHealthCheck determines if health checks should be enabled
func shouldEnableHealthCheck(lambdaType LambdaType) bool {
	// API and federation services should have health checks
	return lambdaType == LambdaTypeAPI || lambdaType == LambdaTypeFederation
}

// TitleCase converts a string to title case (first letter uppercase)
func TitleCase(s string) string {
	if len(s) == 0 {
		return s
	}

	// Handle ASCII uppercase conversion
	first := s[0]
	if first >= 'a' && first <= 'z' {
		return string(first-32) + s[1:]
	}
	return s
}
