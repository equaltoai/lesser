package main

/*
Lesser API Server - Mastodon-compatible ActivityPub implementation

This Lambda function serves the Lesser API using AWS API Gateway v2.
All routing is handled by the Lift framework.

The API Gateway configuration forwards the full request path including /api/v1
and /api/v2 prefixes to this Lambda. All Lift routes must include the complete
path with prefix to match Mastodon API specifications.
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	liftHandlers "github.com/equaltoai/lesser/cmd/api/lift"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/errors"
	liftAuth "github.com/equaltoai/lesser/pkg/lift"
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/streaming"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// Context key types for type-safe context values
type contextKey string

const (
	latencyStartKey contextKey = "latency_start"
	endpointKey     contextKey = "endpoint"
	methodKey       contextKey = "method"
	pathKey         contextKey = "path"
)

var (
	lambdaCtx           *common.LambdaContext
	cfg                 *config.Config
	repos               core.RepositoryStorage
	logger              *zap.Logger
	liftHandler         *liftHandlers.Handler
	authService         *auth.AuthService
	emfMetrics          *observability.EMFMetrics
	healthChecker       *observability.HealthChecker
	metricsCollector    *observability.MetricsCollector //nolint:unused // Extracted from standardized initialization, used in different configurations
	tracingManager      *observability.TracingManager
	latencyAggregator   *observability.LatencyAggregator
	latencyAlerter      *observability.LatencyAlerter
	costTrackingService *cost.TrackingService
	startTime           time.Time
)

var (
	lambdaStart = lambda.Start

	newLambdaOptimizedClient = dynamorm.NewLambdaOptimizedClient
	newRepositoryFactory     = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (core.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
	newAuthService = auth.NewAuthService
	newStreamQueue = streaming.NewDynamoStreamQueue

	createAPIAuthMiddlewareFromAuthService = auth.CreateAPIAuthMiddlewareFromAuthService
	newLiftHandler                         = liftHandlers.NewHandler

	createInstanceLockMiddlewareFn = createInstanceLockMiddleware
	configureLiftRoutesFn          = configureLiftRoutes
	configureAPIRoutesAppTheoryFn  = configureAPIRoutesAppTheory

	trackLambdaInvocation = func(ctx context.Context, service *cost.TrackingService, operation cost.LambdaOperation) error {
		return service.TrackLambdaInvocation(ctx, operation)
	}
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	startTime = time.Now()

	// Initialize Lambda with standardized configuration
	lambdaConfig := common.LambdaConfig{
		ServiceName: "api",
		LambdaType:  common.LambdaTypeAPI,
		// Feature flags will be auto-detected from environment
		RequestTimeout: 30 * time.Second,
	}

	// Use standardized Lambda initialization
	lambdaCtx = common.MustInitializeLambda(lambdaConfig)

	// Initialize with default options for API Lambda type
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		// Fallback to manual initialization for services requiring it
		initializeManualServices()
	} else {
		// Extract standardized services
		extractStandardizedServices()
	}

	// Initialize API-specific services
	initializeAPISpecificServices()
}

// extractStandardizedServices extracts services from standardized initialization
func extractStandardizedServices() {
	// Extract basic services
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger

	// Extract storage services
	if lambdaCtx.Repos != nil {
		repos = lambdaCtx.Repos.(core.RepositoryStorage)
	}

	// Extract auth services
	if lambdaCtx.AuthService != nil {
		authService = lambdaCtx.AuthService.(*auth.AuthService)
	}

	// Extract observability services
	if lambdaCtx.EMFMetrics != nil {
		emfMetrics = lambdaCtx.EMFMetrics.(*observability.EMFMetrics)
	}
	if lambdaCtx.HealthChecker != nil {
		healthChecker = lambdaCtx.HealthChecker.(*observability.HealthChecker)
	}
	if lambdaCtx.TracingManager != nil {
		tracingManager = lambdaCtx.TracingManager.(*observability.TracingManager)
	}
	if lambdaCtx.MetricsCollector != nil {
		metricsCollector = lambdaCtx.MetricsCollector.(*observability.MetricsCollector)
	}
	if lambdaCtx.LatencyAggregator != nil {
		latencyAggregator = lambdaCtx.LatencyAggregator.(*observability.LatencyAggregator)
	}
	if lambdaCtx.LatencyAlerter != nil {
		latencyAlerter = lambdaCtx.LatencyAlerter.(*observability.LatencyAlerter)
	}

	// Initialize centralized cost tracking service if CloudWatch client is available
	if lambdaCtx.AWSServices != nil && lambdaCtx.AWSServices.CloudWatch != nil {
		costTrackingService = cost.NewCostTrackingServiceForLambda(lambdaCtx.AWSServices.CloudWatch, logger, "api")
		logger.Info("initialized centralized cost tracking service from standardized initialization")
	}

	logger.Info("extracted services from standardized initialization")
}

// initializeManualServices provides fallback manual initialization
func initializeManualServices() {
	logger = lambdaCtx.Logger
	cfg = lambdaCtx.Config

	logger.Info("falling back to manual service initialization")

	// Manual DynamORM initialization
	tableName := cfg.DynamoTableName
	logger.Info("manual initialization: configured DynamoDB table",
		zap.String("table_name", tableName))
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main" // fallback
		logger.Warn("manual initialization: missing table name, falling back to default",
			zap.String("table_name", tableName),
			zap.Error(err))
	}
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		logger.Fatal("DYNAMODB_TABLE environment variable is required")
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := newLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Create repository storage using factory pattern
	repos, err = newRepositoryFactory(db, tableName, logger)
	if err != nil {
		logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	// Initialize auth service
	authService, err = newAuthService(cfg, repos)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	// Initialize observability services manually
	if !cfg.DisableMetrics {
		// EMF Metrics for CloudWatch integration
		emfMetrics = observability.NewEMFMetrics(logger, "Lesser/API", "api")
		emfMetrics.AddDimension(observability.DimensionService, "api")
		emfMetrics.AddDimension(observability.DimensionEnvironment, cfg.Stage)
		emfMetrics.AddDimension(observability.DimensionRegion, cfg.Region)

		// Legacy metrics collector
		cloudWatchClient := cloudwatch.NewFromConfig(lambdaCtx.AWSServices.Config)
		metricsCollector = observability.NewMetricsCollector(cloudWatchClient, "Lesser/API", logger)

		// Health checker
		healthConfig := &observability.HealthConfig{
			TableName:        tableName,
			CheckTimeout:     5 * time.Second,
			CacheTimeout:     30 * time.Second,
			DependencyChecks: true,
		}
		healthChecker = observability.NewHealthChecker(logger, lambdaCtx.AWSServices.Config, "api", cfg.Version, healthConfig)

		// Distributed tracing
		tracingConfig := &observability.TracingConfig{
			ServiceName:    "lesser-api",
			ServiceVersion: cfg.Version,
			SamplingRate:   observability.TracingSampleRatePercent / 100.0,
			Enabled:        cfg.XRayTracingEnabled,
		}
		tracingManager = observability.NewTracingManager(logger, tracingConfig)

		// Latency aggregation and alerting
		metricsRecorder := observability.NewDefaultMetricsRecorder(
			func(ctx context.Context, metric *models.MetricRecord) error {
				return repos.MetricRecord().CreateMetricRecord(ctx, metric)
			},
			"api",
		)

		// Initialize latency aggregator
		latencyAggregator = observability.NewLatencyAggregator(
			logger,
			metricsRecorder,
			observability.WithAggregateInterval(5*time.Minute),
			observability.WithRetentionPeriod(24*time.Hour),
			observability.WithMaxBuckets(1000),
		)
		latencyAggregator.Start()

		// Initialize latency alerter
		latencyAlerter = observability.NewLatencyAlerter(logger, metricsRecorder, nil)

		// Initialize centralized cost tracking service
		if cloudWatchClient != nil {
			costTrackingService = cost.NewCostTrackingServiceForLambda(cloudWatchClient, logger, "api")
			logger.Info("initialized centralized cost tracking service")
		}

		logger.Info("initialized observability services manually")
	}
}

// initializeAPISpecificServices initializes API-specific services
func initializeAPISpecificServices() {
	// Initialize Lift-native auth service (future use)
	_ = liftAuth.NewLiftAuthService(authService)

	// Validate VAPID keys in production environment
	if err := liftHandlers.ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger); err != nil {
		logger.Fatal("VAPID keys validation failed in production", zap.Error(err))
	}

	// Create unified auth middleware
	unifiedAuthMiddleware := createAPIAuthMiddlewareFromAuthService(authService, logger)

	// Create stream queue service with proper fallback
	var streamQueue streaming.StreamQueueService
	if lambdaCtx.StreamQueue != nil {
		// Try to cast from standardized initialization
		if sq, ok := lambdaCtx.StreamQueue.(streaming.StreamQueueService); ok {
			streamQueue = sq
		}
	}

	if streamQueue == nil {
		// Fallback to manual stream queue creation
		// Get or create DynamORM client with proper type
		var coreDB dynamormCore.DB
		if lambdaCtx.DynamoDB != nil {
			if db, ok := lambdaCtx.DynamoDB.(dynamormCore.DB); ok {
				coreDB = db
			}
		}

		if coreDB == nil {
			// Last resort: create DynamORM client
			manualDB, dbErr := newLambdaOptimizedClient(context.Background(), cfg.Region)
			if dbErr != nil {
				logger.Fatal("Failed to initialize DynamORM for stream queue", zap.Error(dbErr))
			}
			coreDB = manualDB
		}
		streamQueue = newStreamQueue(coreDB, cfg.DynamoTableName, logger)
	}

	// Create Lift handler for all endpoints
	liftHandler = newLiftHandler(cfg, repos, logger, unifiedAuthMiddleware, streamQueue)

	logger.Info("initialized API-specific services")
}

func main() {
	// Create a new Lift application
	app := lift.New()
	if cfg.DebugMode {
		app = lift.New(lift.WithDebug())
	}

	// Add global middleware
	// Panic recovery middleware (MUST be first to catch all panics)
	app.Use(middleware.PanicRecovery(lambdaCtx.Logger))

	// Add timeout middleware
	app.Use(liftMiddleware.TimeoutMiddleware(liftMiddleware.TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
	}))

	// Add custom logging middleware
	app.Use(createLoggingMiddleware(logger))

	// Apply strict security middleware for web clients
	middleware.ApplySecurityMiddleware(app, middleware.SecurityTypeAPI, logger)

	// Block publishing/signups until instance activation completes.
	app.Use(createInstanceLockMiddlewareFn(repos, logger))

	// Add unified error handling middleware
	app.Use(common.CreateAPIErrorMiddleware(logger))

	// Add centralized cost tracking middleware
	if costTrackingService != nil {
		app.Use(createCentralizedCostTrackingMiddleware())
	}

	// Add distributed tracing middleware
	if tracingManager != nil {
		app.Use(createTracingMiddleware())
	}

	// Add EMF-based performance monitoring middleware
	if emfMetrics != nil {
		app.Use(createEMFMetricsMiddleware())
	}

	// Add comprehensive latency tracking middleware
	app.Use(createLatencyTrackingMiddleware())

	// Note: Rate limiting is now applied per-endpoint in routes_lift.go
	// Not using global middleware to avoid incorrectly rate limiting read endpoints

	// Configure native Lift routes
	configureLiftRoutesFn(app)

		// Configure API routes (AppTheory slices 2+)
		apiApp := apptheory.New(apptheory.WithTier(apptheory.TierP0))
		applyWebClientCORSAppTheory(apiApp)
		configureAPIRoutesAppTheoryFn(apiApp)

	// Configure health check routes (AppTheory slice 1) if available
	var healthApp *apptheory.App
	if healthChecker != nil {
		healthApp = apptheory.New(apptheory.WithTier(apptheory.TierP0))
		configureHealthRoutesAppTheory(healthApp)
	}

	// Start the Lambda handler with observability
	lambdaHandler := func(ctx context.Context, event interface{}) (interface{}, error) {
		requestStart := time.Now()

		// Record cold start metric if this is a cold start
		if time.Since(startTime) < 30*time.Second && emfMetrics != nil {
			emfMetrics.RecordBusinessMetric(observability.MetricColdStarts, 1.0, observability.UnitCount, nil)
			coldStartDuration := time.Since(startTime)
			emfMetrics.RecordBusinessMetric(observability.MetricColdStartDuration, float64(coldStartDuration.Milliseconds()), observability.UnitMilliseconds, nil)
		}

		// Process the request
			result, err := handleAPIRequest(ctx, app, healthApp, apiApp, event)

		// Record request metrics
		requestDuration := time.Since(requestStart)
		if emfMetrics != nil {
			emfMetrics.RecordLatency("api_request", requestDuration)
			emfMetrics.RecordThroughput("api_request", 1)

			if err != nil {
				emfMetrics.RecordError("api_request", "lambda_error")
			} else {
				emfMetrics.RecordSuccess("api_request")
			}
		}

		// Use standardized observability flushing
		lambdaCtx.FlushObservabilityServices()

		return result, err
	}

	lambdaStart(lambdaHandler)
}

func handleAPIRequest(ctx context.Context, liftApp *lift.App, healthApp *apptheory.App, apiApp *apptheory.App, event interface{}) (interface{}, error) {
	method, path, ok := extractHTTPMethodAndPath(event)
	if !ok {
		return liftApp.HandleRequest(ctx, event)
	}

	raw, err := marshalLambdaEvent(event)
	if err != nil {
		return liftApp.HandleRequest(ctx, event)
	}
	raw = normalizeLambdaEventForAppTheory(raw)

	if healthApp != nil && shouldRouteToHealthAppTheory(method, path) {
		out, handleErr := healthApp.HandleLambda(ctx, raw)
		if handleErr == nil {
			return out, nil
		}
	}

	if apiApp != nil && shouldRouteToAPIAppTheory(method, path) {
		out, handleErr := apiApp.HandleLambda(ctx, raw)
		if handleErr == nil {
			return out, nil
		}
	}

	return liftApp.HandleRequest(ctx, event)
}

func shouldRouteToHealthAppTheory(method, path string) bool {
	if strings.TrimSpace(method) != "GET" {
		return false
	}

	switch strings.TrimSpace(path) {
	case "/health", "/health/live", "/health/ready", "/health/detailed":
		return true
	default:
		return false
	}
}

func shouldRouteToAPIAppTheory(method, path string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "OPTIONS" || method == "" {
		return false
	}
	if strings.TrimSpace(path) == "" {
		return false
	}

	for _, route := range apiAppTheoryRoutes {
		if route.method != method {
			continue
		}
		if matchBracedPathPattern(route.pattern, path) {
			return true
		}
	}

	return false
}

type apptheoryRoute struct {
	method  string
	pattern string
}

var apiAppTheoryRoutes = []apptheoryRoute{
	{method: "POST", pattern: "/api/v1/apps"},
	{method: "POST", pattern: "/auth/wallet/challenge"},
	{method: "POST", pattern: "/auth/wallet/verify"},
	{method: "POST", pattern: "/auth/wallet/login"},
	{method: "POST", pattern: "/auth/wallet/link"},
	{method: "DELETE", pattern: "/auth/wallet/unlink/{address}"},
	{method: "GET", pattern: "/auth/wallet/list"},
	{method: "POST", pattern: "/api/v1/auth/webauthn/register/begin"},
	{method: "POST", pattern: "/api/v1/auth/webauthn/register/finish"},
	{method: "POST", pattern: "/api/v1/auth/webauthn/login/begin"},
	{method: "POST", pattern: "/api/v1/auth/webauthn/login/finish"},
	{method: "GET", pattern: "/api/v1/auth/webauthn/credentials"},
	{method: "DELETE", pattern: "/api/v1/auth/webauthn/credentials/{credentialId}"},
	{method: "PUT", pattern: "/api/v1/auth/webauthn/credentials/{credentialId}"},
	{method: "GET", pattern: "/oauth/authorize"},
	{method: "POST", pattern: "/oauth/consent"},
	{method: "POST", pattern: "/oauth/token"},
	{method: "GET", pattern: "/setup/status"},
	{method: "POST", pattern: "/setup/bootstrap/challenge"},
	{method: "POST", pattern: "/setup/bootstrap/verify"},
	{method: "POST", pattern: "/setup/admin"},
	{method: "POST", pattern: "/setup/finalize"},

	// Accounts + user-level endpoints.
	{method: "GET", pattern: "/api/v1/accounts/verify_credentials"},
	{method: "PATCH", pattern: "/api/v1/accounts/update_credentials"},
	{method: "POST", pattern: "/api/v1/accounts"},
	{method: "GET", pattern: "/api/v1/accounts/relationships"},
	{method: "GET", pattern: "/api/v1/accounts/{id}/notes"},
	{method: "GET", pattern: "/api/v1/accounts/{id}/statuses"},
	{method: "GET", pattern: "/api/v1/accounts/search"},
	{method: "GET", pattern: "/api/v1/accounts/search/suggestions"},
	{method: "POST", pattern: "/api/v1/accounts/{id}/follow"},
	{method: "POST", pattern: "/api/v1/accounts/{id}/unfollow"},
	{method: "POST", pattern: "/api/v1/accounts/{id}/block"},
	{method: "POST", pattern: "/api/v1/accounts/{id}/unblock"},
	{method: "POST", pattern: "/api/v1/accounts/{id}/mute"},
	{method: "POST", pattern: "/api/v1/accounts/{id}/unmute"},
	{method: "GET", pattern: "/api/v1/blocks"},
	{method: "GET", pattern: "/api/v1/mutes"},
	{method: "GET", pattern: "/api/v1/follow_requests"},
	{method: "POST", pattern: "/api/v1/follow_requests/{account_id}/authorize"},
	{method: "POST", pattern: "/api/v1/follow_requests/{account_id}/reject"},
	{method: "GET", pattern: "/api/v1/domain_blocks"},
	{method: "POST", pattern: "/api/v1/domain_blocks"},
	{method: "DELETE", pattern: "/api/v1/domain_blocks"},
	{method: "POST", pattern: "/api/v1/exports"},
	{method: "GET", pattern: "/api/v1/exports/{id}"},
	{method: "GET", pattern: "/api/v1/exports/{id}/download"},
	{method: "GET", pattern: "/api/v1/exports"},
	{method: "POST", pattern: "/api/v1/imports"},
	{method: "GET", pattern: "/api/v1/imports/{id}"},
	{method: "DELETE", pattern: "/api/v1/imports/{id}"},
	{method: "GET", pattern: "/api/v1/imports"},
	{method: "GET", pattern: "/api/v1/accounts/{id}/quote_permissions"},
	{method: "PUT", pattern: "/api/v1/accounts/quote_permissions"},
}

func matchBracedPathPattern(pattern, path string) bool {
	pattern = strings.TrimSpace(pattern)
	path = strings.TrimSpace(path)
	if pattern == "" || path == "" {
		return false
	}
	if pattern == path {
		return true
	}

	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	if len(patternParts) != len(pathParts) {
		return false
	}

	for i := range patternParts {
		want := patternParts[i]
		got := pathParts[i]

		if want == got {
			continue
		}
		if strings.HasPrefix(want, "{") && strings.HasSuffix(want, "}") && len(want) > 2 {
			continue
		}
		return false
	}

	return true
}

func marshalLambdaEvent(event interface{}) (json.RawMessage, error) {
	switch v := event.(type) {
	case json.RawMessage:
		return v, nil
	case []byte:
		return json.RawMessage(v), nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(encoded), nil
	}
}

func normalizeLambdaEventForAppTheory(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}

	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		return raw
	}

	requestContext, ok := extractMapField(event, "requestContext")
	if !ok {
		return raw
	}

	stage := extractStringField(requestContext, "stage")
	if stage == "" || stage == "$default" {
		return raw
	}

	if rawPath := extractStringField(event, "rawPath"); rawPath != "" {
		event["rawPath"] = stripStagePrefix(rawPath, stage)
	}
	if path := extractStringField(event, "path"); path != "" {
		event["path"] = stripStagePrefix(path, stage)
	}

	httpContext, ok := extractMapField(requestContext, "http")
	if ok {
		if httpPath := extractStringField(httpContext, "path"); httpPath != "" {
			httpContext["path"] = stripStagePrefix(httpPath, stage)
			requestContext["http"] = httpContext
			event["requestContext"] = requestContext
		}
	}

	normalized, err := json.Marshal(event)
	if err != nil {
		return raw
	}
	return normalized
}

func extractHTTPMethodAndPath(event interface{}) (method string, path string, ok bool) {
	switch v := event.(type) {
	case map[string]any:
		return extractHTTPMethodAndPathFromMap(v)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return "", "", false
		}
		var probe map[string]any
		if err := json.Unmarshal(encoded, &probe); err != nil {
			return "", "", false
		}
		return extractHTTPMethodAndPathFromMap(probe)
	}
}

func extractHTTPMethodAndPathFromMap(event map[string]any) (method string, path string, ok bool) {
	rawPath := extractStringField(event, "rawPath")

	requestContext, _ := extractMapField(event, "requestContext")
	httpContext, _ := extractMapField(requestContext, "http")
	stage := extractStringField(requestContext, "stage")

	method = extractStringField(httpContext, "method")
	if method == "" {
		method = extractStringField(event, "httpMethod")
	}

	path = rawPath
	if path == "" {
		path = extractStringField(httpContext, "path")
	}
	if path == "" {
		path = extractStringField(event, "path")
	}
	path = stripStagePrefix(path, stage)

	return method, path, method != "" && path != ""
}

func stripStagePrefix(path, stage string) string {
	if stage == "" || stage == "$default" || path == "" {
		return path
	}

	stagePrefix := "/" + stage
	if path == stagePrefix {
		return "/"
	}
	if strings.HasPrefix(path, stagePrefix+"/") {
		return strings.TrimPrefix(path, stagePrefix)
	}
	return path
}

func extractStringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func extractMapField(m map[string]any, key string) (map[string]any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil, false
	}
	if mv, ok := v.(map[string]any); ok {
		return mv, true
	}
	m2, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	out := map[string]any{}
	for k, v := range m2 {
		out[k] = v
	}
	return out, true
}

// requestInfo holds extracted request information
type requestInfo struct {
	method    string
	path      string
	endpoint  string
	userAgent string
	clientIP  string
}

// extractRequestInfo extracts request information from Lift context
func extractRequestInfo(ctx *lift.Context) requestInfo {
	info := requestInfo{
		method: "GET",
		path:   "/",
	}

	if ctx.Request == nil || ctx.Request.Request == nil {
		info.endpoint = info.method + " " + info.path
		return info
	}

	req := ctx.Request.Request
	info.method = req.Method
	info.path = req.Path
	info.userAgent = req.Headers["User-Agent"]

	if xForwarded := req.Headers["X-Forwarded-For"]; xForwarded != "" {
		info.clientIP = xForwarded
	}

	info.endpoint = info.method + " " + info.path
	return info
}

// addLatencyContextValues adds latency tracking values to context
func addLatencyContextValues(ctx *lift.Context, info requestInfo, startTime time.Time) {
	ctx.Context = context.WithValue(ctx.Context, latencyStartKey, startTime)
	ctx.Context = context.WithValue(ctx.Context, endpointKey, info.endpoint)
	ctx.Context = context.WithValue(ctx.Context, methodKey, info.method)
	ctx.Context = context.WithValue(ctx.Context, pathKey, info.path)
}

// recordDynamoDBLatencyMetric records latency metric to DynamoDB asynchronously
func recordDynamoDBLatencyMetric(ctx context.Context, info requestInfo, statusCode int, totalLatency time.Duration) {
	go func() {
		if err := recordLatencyMetric(ctx, "api_endpoint", info.endpoint, totalLatency, map[string]string{
			"endpoint":    info.path,
			"method":      info.method,
			"status_code": fmt.Sprintf("%d", statusCode),
			"user_agent":  info.userAgent,
			"client_ip":   info.clientIP,
		}); err != nil {
			logger.Warn("failed to record latency metric", zap.Error(err))
		}
	}()
}

// recordAggregatorLatency records latency to the aggregator if available
func recordAggregatorLatency(info requestInfo, totalLatency time.Duration) {
	if latencyAggregator == nil {
		return
	}
	latencyAggregator.RecordLatency(info.endpoint, "api", totalLatency)
}

// calculatePercentiles extracts percentile values from stats
func calculatePercentiles(stats interface{}, totalLatency time.Duration) (float64, float64) {
	defaultMs := float64(totalLatency.Milliseconds())
	p95Ms, p99Ms := defaultMs, defaultMs

	if stats == nil {
		return p95Ms, p99Ms
	}

	// Try type assertion for stats with Percentiles field
	if statsType := stats; statsType != nil {
		// Use reflection-like approach to safely access percentiles
		if statsMap, ok := stats.(map[string]interface{}); ok {
			if percentiles, exists := statsMap["Percentiles"]; exists {
				if percMap, ok := percentiles.(map[string]float64); ok {
					if p95Val, exists := percMap["p95"]; exists {
						p95Ms = p95Val
					}
					if p99Val, exists := percMap["p99"]; exists {
						p99Ms = p99Val
					}
				}
			}
		}
	}

	return p95Ms, p99Ms
}

// checkLatencyAlerting performs latency alerting if alerter is available
func checkLatencyAlerting(ctx context.Context, info requestInfo, totalLatency time.Duration) {
	if latencyAlerter == nil {
		return
	}

	stats, _ := latencyAggregator.GetCurrentStats(info.endpoint, "api")
	p95Ms, p99Ms := calculatePercentiles(stats, totalLatency)

	latencyAlerter.CheckLatency(ctx, "api_endpoint", "api",
		float64(totalLatency.Milliseconds()), p95Ms, p99Ms)
}

// determineErrorType maps status code to error type
func determineErrorType(statusCode int) string {
	if statusCode < 400 || statusCode >= 500 {
		return observability.ErrorTypeInternal
	}

	switch statusCode {
	case 401:
		return observability.ErrorTypeAuthentication
	case 403:
		return observability.ErrorTypeAuthorization
	case 404:
		return observability.ErrorTypeNotFound
	case 409:
		return observability.ErrorTypeConflict
	case 429:
		return observability.ErrorTypeRateLimit
	default:
		return observability.ErrorTypeValidation
	}
}

// recordEMFMetrics records metrics to EMF if available
func recordEMFMetrics(info requestInfo, statusCode int, totalLatency time.Duration, err error) {
	if emfMetrics == nil {
		return
	}

	dimensions := map[string]string{
		observability.DimensionEndpoint:   info.path,
		observability.DimensionMethod:     info.method,
		observability.DimensionStatusCode: fmt.Sprintf("%d", statusCode),
	}

	emfMetrics.RecordLatency(info.endpoint, totalLatency)

	if err != nil {
		errorType := determineErrorType(statusCode)
		dimensions[observability.DimensionErrorType] = errorType
		emfMetrics.RecordError(info.endpoint, errorType)
	} else {
		emfMetrics.RecordSuccess(info.endpoint)
	}

	emfMetrics.RecordThroughput(info.endpoint, 1)
}

// createLatencyTrackingMiddleware creates comprehensive latency tracking middleware
func createLatencyTrackingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			startTime := time.Now()
			info := extractRequestInfo(ctx)

			addLatencyContextValues(ctx, info, startTime)

			// Execute request
			err := next.Handle(ctx)

			// Calculate latency and determine status
			totalLatency := time.Since(startTime)
			statusCode := 200
			if err != nil {
				statusCode = 500 // Default error status
			}

			// Record metrics to all available systems
			recordDynamoDBLatencyMetric(ctx.Context, info, statusCode, totalLatency)
			recordAggregatorLatency(info, totalLatency)
			checkLatencyAlerting(ctx.Context, info, totalLatency)
			recordEMFMetrics(info, statusCode, totalLatency, err)

			return err
		})
	}
}

// createEMFMetricsMiddleware creates middleware for EMF metrics collection
func createEMFMetricsMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if emfMetrics == nil {
				return next.Handle(ctx)
			}

			// Start latency timer
			timer := emfMetrics.StartLatencyTimer(ctx.Context, "endpoint_request")

			// Extract endpoint information
			method := "GET"
			path := "/"
			if ctx.Request != nil && ctx.Request.Request != nil {
				method = ctx.Request.Request.Method
				path = ctx.Request.Request.Path
			}
			endpoint := method + " " + path
			dimensions := map[string]string{
				observability.DimensionEndpoint: path,
				observability.DimensionMethod:   method,
			}

			// Execute request
			err := next.Handle(ctx)

			// Record metrics based on result
			// Default to 200 if no error
			statusCode := 200
			if err != nil {
				// Infer status from error (this is approximate)
				statusCode = 500
			}
			dimensions[observability.DimensionStatusCode] = fmt.Sprintf("%d", statusCode)

			if err != nil {
				errorType := observability.ErrorTypeInternal
				if statusCode >= 400 && statusCode < 500 {
					switch statusCode {
					case 401:
						errorType = observability.ErrorTypeAuthentication
					case 403:
						errorType = observability.ErrorTypeAuthorization
					case 404:
						errorType = observability.ErrorTypeNotFound
					case 409:
						errorType = observability.ErrorTypeConflict
					case 429:
						errorType = observability.ErrorTypeRateLimit
					default:
						errorType = observability.ErrorTypeValidation
					}
				}

				dimensions[observability.DimensionErrorType] = errorType
				timer.FinishWithError(emfMetrics, errorType)
				emfMetrics.RecordError(endpoint, errorType)
			} else {
				timer.Finish(emfMetrics, true)
				emfMetrics.RecordSuccess(endpoint)
			}

			// Record status code specific metrics
			if statusCode >= 200 && statusCode < 300 {
				emfMetrics.RecordBusinessMetric("SuccessfulRequests", 1.0, observability.UnitCount, dimensions)
			} else if statusCode >= 400 {
				emfMetrics.RecordBusinessMetric("ErrorRequests", 1.0, observability.UnitCount, dimensions)
			}

			// Record throughput
			emfMetrics.RecordThroughput(endpoint, 1)

			return err
		})
	}
}

// configureHealthRoutes adds health check endpoints to the Lift app
func configureHealthRoutes(app *lift.App) {
	// Liveness endpoint
	_ = app.GET("/health/live", func(ctx *lift.Context) error {
		response := map[string]interface{}{
			"status":    observability.HealthStatusHealthy,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
		}
		return ctx.Status(200).JSON(response)
	})

	// Legacy health endpoint (infra + backwards compatibility)
	_ = app.GET("/health", func(ctx *lift.Context) error {
		response := map[string]interface{}{
			"status":    observability.HealthStatusHealthy,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
		}
		return ctx.Status(200).JSON(response)
	})

	// Readiness endpoint
	_ = app.GET("/health/ready", func(ctx *lift.Context) error {
		// Basic readiness check - can we access our dependencies?
		status := observability.HealthStatusHealthy
		checks := []map[string]interface{}{}

		// Check DynamoDB connectivity
		checkCtx, cancel := context.WithTimeout(ctx.Context, 5*time.Second)
		defer cancel()

		start := time.Now()
		_, err := repos.Account().GetUser(checkCtx, "health-check-user")
		duration := time.Since(start)

		dbCheck := map[string]interface{}{
			"name":     "dynamodb",
			"status":   observability.HealthStatusHealthy,
			"duration": duration.Milliseconds(),
		}

		if err != nil && err.Error() != "user not found" { // "not found" is expected
			dbCheck["status"] = observability.HealthStatusCritical
			dbCheck["message"] = "Database connectivity issue"
			status = observability.HealthStatusCritical
		}

		checks = append(checks, dbCheck)

		response := map[string]interface{}{
			"status":    status,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
			"checks":    checks,
		}

		statusCode := 200
		if status == observability.HealthStatusCritical {
			statusCode = 503
		}

		return ctx.Status(statusCode).JSON(response)
	})

	// Detailed health endpoint
	_ = app.GET("/health/detailed", func(ctx *lift.Context) error {
		// Comprehensive health check with all components
		status := observability.HealthStatusHealthy
		checks := []map[string]interface{}{}
		summary := map[string]interface{}{
			"total_checks":    0,
			"healthy_checks":  0,
			"warning_checks":  0,
			"critical_checks": 0,
		}

		// Runtime check
		runtimeCheck := map[string]interface{}{
			"name":      "runtime",
			"status":    observability.HealthStatusHealthy,
			"message":   "Service runtime is healthy",
			"uptime_ms": time.Since(startTime).Milliseconds(),
			"service":   "api",
			"version":   cfg.Version,
		}
		checks = append(checks, runtimeCheck)
		summary["healthy_checks"] = summary["healthy_checks"].(int) + 1

		// Database detailed check
		checkCtx, cancel := context.WithTimeout(ctx.Context, 10*time.Second)
		defer cancel()

		start := time.Now()
		_, err := repos.Account().GetUser(checkCtx, "health-check-user")
		duration := time.Since(start)

		dbCheck := map[string]interface{}{
			"name":       "dynamodb_detailed",
			"status":     observability.HealthStatusHealthy,
			"duration":   duration.Milliseconds(),
			"table_name": cfg.DynamoTableName,
		}

		if err != nil && err.Error() != "user not found" {
			dbCheck["status"] = observability.HealthStatusCritical
			dbCheck["message"] = fmt.Sprintf("Database error: %v", err)
			status = observability.HealthStatusCritical
			summary["critical_checks"] = summary["critical_checks"].(int) + 1
		} else {
			summary["healthy_checks"] = summary["healthy_checks"].(int) + 1
		}

		checks = append(checks, dbCheck)
		summary["total_checks"] = len(checks)

		response := map[string]interface{}{
			"status":    status,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
			"region":    cfg.Region,
			"checks":    checks,
			"summary":   summary,
		}

		statusCode := 200
		if status == observability.HealthStatusCritical {
			statusCode = 503
		}

		return ctx.Status(statusCode).JSON(response)
	})
}

func configureHealthRoutesAppTheory(app *apptheory.App) {
	// Liveness endpoint
	app.Get("/health/live", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		response := map[string]interface{}{
			"status":    observability.HealthStatusHealthy,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
		}
		return apptheoryJSON(200, response)
	})

	// Legacy health endpoint (infra + backwards compatibility)
	app.Get("/health", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		response := map[string]interface{}{
			"status":    observability.HealthStatusHealthy,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
		}
		return apptheoryJSON(200, response)
	})

	// Readiness endpoint
	app.Get("/health/ready", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		// Basic readiness check - can we access our dependencies?
		status := observability.HealthStatusHealthy
		checks := []map[string]interface{}{}

		// Check DynamoDB connectivity
		checkCtx, cancel := context.WithTimeout(ctx.Context(), 5*time.Second)
		defer cancel()

		start := time.Now()
		_, err := repos.Account().GetUser(checkCtx, "health-check-user")
		duration := time.Since(start)

		dbCheck := map[string]interface{}{
			"name":     "dynamodb",
			"status":   observability.HealthStatusHealthy,
			"duration": duration.Milliseconds(),
		}

		if err != nil && err.Error() != "user not found" { // "not found" is expected
			dbCheck["status"] = observability.HealthStatusCritical
			dbCheck["message"] = "Database connectivity issue"
			status = observability.HealthStatusCritical
		}

		checks = append(checks, dbCheck)

		response := map[string]interface{}{
			"status":    status,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
			"checks":    checks,
		}

		statusCode := 200
		if status == observability.HealthStatusCritical {
			statusCode = 503
		}

		return apptheoryJSON(statusCode, response)
	})

	// Detailed health endpoint
	app.Get("/health/detailed", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		// Comprehensive health check with all components
		status := observability.HealthStatusHealthy
		checks := []map[string]interface{}{}
		summary := map[string]interface{}{
			"total_checks":    0,
			"healthy_checks":  0,
			"warning_checks":  0,
			"critical_checks": 0,
		}

		// Runtime check
		runtimeCheck := map[string]interface{}{
			"name":      "runtime",
			"status":    observability.HealthStatusHealthy,
			"message":   "Service runtime is healthy",
			"uptime_ms": time.Since(startTime).Milliseconds(),
			"service":   "api",
			"version":   cfg.Version,
		}
		checks = append(checks, runtimeCheck)
		summary["healthy_checks"] = summary["healthy_checks"].(int) + 1

		// Database detailed check
		checkCtx, cancel := context.WithTimeout(ctx.Context(), 10*time.Second)
		defer cancel()

		start := time.Now()
		_, err := repos.Account().GetUser(checkCtx, "health-check-user")
		duration := time.Since(start)

		dbCheck := map[string]interface{}{
			"name":       "dynamodb_detailed",
			"status":     observability.HealthStatusHealthy,
			"duration":   duration.Milliseconds(),
			"table_name": cfg.DynamoTableName,
		}

		if err != nil && err.Error() != "user not found" {
			dbCheck["status"] = observability.HealthStatusCritical
			dbCheck["message"] = fmt.Sprintf("Database error: %v", err)
			status = observability.HealthStatusCritical
			summary["critical_checks"] = summary["critical_checks"].(int) + 1
		} else {
			summary["healthy_checks"] = summary["healthy_checks"].(int) + 1
		}

		checks = append(checks, dbCheck)
		summary["total_checks"] = len(checks)

		response := map[string]interface{}{
			"status":    status,
			"timestamp": time.Now(),
			"service":   "api",
			"version":   cfg.Version,
			"region":    cfg.Region,
			"checks":    checks,
			"summary":   summary,
		}

		statusCode := 200
		if status == observability.HealthStatusCritical {
			statusCode = 503
		}

		return apptheoryJSON(statusCode, response)
	})
}

func apptheoryJSON(status int, value any) (*apptheory.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	return &apptheory.Response{
		Status: status,
		Headers: map[string][]string{
			"content-type": {"application/json"},
		},
		Body:     body,
		IsBase64: false,
	}, nil
}

// createTracingMiddleware creates middleware for distributed tracing
func createTracingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if tracingManager == nil || !tracingManager.IsEnabled() {
				return next.Handle(ctx)
			}

			// Start tracing segment
			traceCtx, segment := tracingManager.StartSegment(ctx.Context, "api-request")
			defer func() {
				if segment != nil {
					segment.Close(nil)
				}
			}()

			// Extract request info
			method := "GET"
			url := "/"
			userAgent := ""
			clientIP := ""
			if ctx.Request != nil && ctx.Request.Request != nil {
				method = ctx.Request.Request.Method
				url = ctx.Request.Request.Path
				userAgent = ctx.Request.Request.Headers["User-Agent"]
				// Try to get client IP from headers
				if xForwarded := ctx.Request.Request.Headers["X-Forwarded-For"]; xForwarded != "" {
					clientIP = xForwarded
				}
			}

			// Add HTTP request information
			tracingManager.SetHTTPRequest(traceCtx,
				method,
				url,
				userAgent,
				clientIP)

			// Add request metadata
			tracingManager.AddAnnotation(traceCtx, "endpoint", url)
			tracingManager.AddAnnotation(traceCtx, "method", method)

			// Add trace ID to response headers for debugging
			// Note: Lift doesn't provide direct header access for responses in middleware
			// This would need to be set in individual handlers if needed
			_ = tracingManager.GetTraceContext(traceCtx)

			// Execute request
			err := next.Handle(ctx)

			// Record response information
			// Default to 200 if no error, 500 if error
			statusCode := 200
			if err != nil {
				statusCode = 500
			}
			tracingManager.SetHTTPResponse(traceCtx, statusCode, 0) // Content length not easily available in Lift
			tracingManager.AddAnnotation(traceCtx, "status_code", statusCode)

			// Record error if any
			if err != nil {
				tracingManager.AddError(traceCtx, err, false)
			}

			return err
		})
	}
}

// recordLatencyMetric records latency metrics to DynamoDB using DynamORM
func recordLatencyMetric(ctx context.Context, metricType, operation string, duration time.Duration, dimensions map[string]string) error {
	if repos == nil {
		return errors.ServiceInitializationFailed("repositories", nil)
	}

	// Create metric record
	metric := &models.MetricRecord{
		MetricType:       metricType,
		ServiceName:      "api",
		Timestamp:        time.Now(),
		AggregationLevel: "raw",
		Unit:             "ms",
		Dimensions:       dimensions,
		// Statistical values for single measurement
		Count: 1,
		Sum:   float64(duration.Milliseconds()),
		Min:   float64(duration.Milliseconds()),
		Max:   float64(duration.Milliseconds()),
		P50:   float64(duration.Milliseconds()),
		P95:   float64(duration.Milliseconds()),
		P99:   float64(duration.Milliseconds()),
	}

	// Add operation to dimensions
	metric.AddDimension("operation", operation)

	// Store to DynamoDB via metrics repository
	return repos.MetricRecord().CreateMetricRecord(ctx, metric)
}

// createCentralizedCostTrackingMiddleware creates centralized cost tracking middleware using the TrackingService
func createCentralizedCostTrackingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if costTrackingService == nil {
				return next.Handle(ctx)
			}

			startTime := time.Now()

			// Execute request
			err := next.Handle(ctx)

			// Track Lambda execution cost
			duration := time.Since(startTime)
			memoryMB := int64(128) // Default Lambda memory, could be extracted from env
			if mem, parseErr := common.GetLambdaMemorySizeInt(); parseErr == nil && mem > 0 {
				memoryMB = int64(mem)
			}

			// Create Lambda operation for cost tracking
			lambdaOp := cost.LambdaOperation{
				FunctionName: "api",
				Duration:     duration,
				MemoryMB:     memoryMB,
				Timestamp:    startTime,
			}

			// Track the operation asynchronously to avoid blocking the response
			go func() {
				if trackErr := trackLambdaInvocation(context.Background(), costTrackingService, lambdaOp); trackErr != nil {
					logger.Warn("failed to track Lambda cost", zap.Error(trackErr))
				}
			}()

			return err
		})
	}
}
