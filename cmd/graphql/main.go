// Package main implements the graphql Lambda function for serving GraphQL API endpoints.
package main

/*
Lesser GraphQL Server - GraphQL API for ActivityPub implementation

This Lambda function serves the Lesser GraphQL API using the Lift framework
with full DynamORM integration for type-safe database operations.

Features:
- GraphQL API with 60+ operations
- WebSocket subscriptions for real-time updates
- DataLoader for N+1 query prevention
- Cost tracking and monitoring
- GraphQL playground for development
*/

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/apollotracing"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/middleware"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/streaming"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"
)

// Context key types for type-safe context values
type contextKey string

const (
	contextKeyUser        contextKey = "user"
	contextKeyCostTracker contextKey = "cost_tracker"
	contextKeyLoaders     contextKey = "loaders"
)

type lambdaInvocationTracker interface {
	TrackLambdaInvocation(ctx context.Context, operation cost.LambdaOperation) error
}

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	repos     core.RepositoryStorage
	//nolint:gochecknoglobals // These are initialized once at startup
	logger              *zap.Logger
	graphQLHandler      http.Handler
	emfMetricsService   interface{}   // *observability.EMFMetricsService interface
	costTracker         *cost.Tracker // Legacy tracker for resolver compatibility
	costTrackingService lambdaInvocationTracker
	initTime            time.Time
)

var (
	runningUnitTestsFn       = common.RunningUnitTests
	mustInitializeLambdaFn   = common.MustInitializeLambda
	initializeWithDefaultsFn = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }

	extractStandardizedServicesFn       = extractStandardizedServices
	initializeManualServicesFn          = initializeManualServices
	initializeGraphQLSpecificServicesFn = initializeGraphQLSpecificServices
	lambdaStartFn                       = lambda.Start
	newLambdaOptimizedClientFn          = dynamorm.NewLambdaOptimizedClient
	newRepositoryFactoryFn              = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (core.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
)

type oauthMiddlewareAdapter struct {
	service *auth.OAuthService
}

func (a *oauthMiddlewareAdapter) ValidateAccessToken(token string) (common.Claims, error) {
	claims, err := a.service.ValidateAccessToken(token)
	if err != nil {
		logger.Warn("OAuthService.ValidateAccessToken failed",
			zap.Error(err),
			zap.String("error_type", fmt.Sprintf("%T", err)),
		)
		return nil, err
	}
	return claims, nil
}

func init() {
	initializeGraphQLOnStart()
}

func initializeGraphQLOnStart() {
	if runningUnitTestsFn() {
		return
	}

	initializeGraphQL()
}

func initializeGraphQL() {
	initTime = time.Now()

	// Standardized Lambda initialization with automatic service detection
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName:    "graphql",
		LambdaType:     common.LambdaTypeAPI,
		RequestTimeout: 30 * time.Second,
	})

	// Initialize with default options for API Lambda type
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		// Fallback to manual initialization for services requiring it
		initializeManualServicesFn()
	} else {
		// Extract standardized services
		extractStandardizedServicesFn()
	}

	// Initialize GraphQL-specific services
	initializeGraphQLSpecificServicesFn()
}

// extractStandardizedServices extracts services from standardized initialization
func extractStandardizedServices() {
	// Automatic dependency injection from Lambda context
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	repos = lambdaCtx.Repos.(core.RepositoryStorage)
	emfMetricsService = lambdaCtx.EMFMetrics

	// Initialize centralized cost tracking service if CloudWatch client is available
	if lambdaCtx.AWSServices != nil && lambdaCtx.AWSServices.CloudWatch != nil {
		costTrackingService = cost.NewCostTrackingServiceForLambda(lambdaCtx.AWSServices.CloudWatch, logger, "graphql")
		logger.Info("initialized centralized cost tracking service from standardized initialization")
	}

	// Initialize unified cost tracker for resolver compatibility
	// Use standard cost tracker for resolver compatibility
	costTracker = cost.New()

	logger.Info("extracted services from standardized initialization")
}

// initializeManualServices provides fallback manual initialization
func initializeManualServices() {
	logger = lambdaCtx.Logger
	cfg = lambdaCtx.Config

	logger.Info("falling back to manual service initialization")

	// Determine DynamoDB table name
	tableName := cfg.DynamoTableName
	logger.Info("manual initialization: configured DynamoDB table",
		zap.String("table_name", tableName))

	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main"
		logger.Warn("manual initialization: missing table name, falling back to default",
			zap.String("table_name", tableName),
			zap.Error(err))
	}

	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		logger.Fatal("DYNAMODB_TABLE environment variable is required")
	}

	// Initialize DynamORM client optimized for Lambda cold starts
	db, err := newLambdaOptimizedClientFn(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize repository factory to provide core storage interfaces
	repoFactory, err := newRepositoryFactoryFn(db, tableName, logger)
	if err != nil {
		logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	repos = repoFactory
	lambdaCtx.Repos = repoFactory

	// Initialize cost tracker for resolver compatibility
	costTracker = cost.New()

	logger.Info("manual service initialization completed",
		zap.String("table_name", tableName),
		zap.String("region", cfg.Region))
}

func resolveStreamQueue() streaming.StreamQueueService {
	if lambdaCtx != nil && lambdaCtx.StreamQueue != nil {
		if sq, ok := lambdaCtx.StreamQueue.(streaming.StreamQueueService); ok {
			return sq
		}
	}

	var coreDB dynamormCore.DB
	if lambdaCtx != nil && lambdaCtx.DynamoDB != nil {
		if db, ok := lambdaCtx.DynamoDB.(dynamormCore.DB); ok {
			coreDB = db
		}
	}

	if coreDB == nil && repos != nil {
		coreDB = repos.GetDB()
	}

	if coreDB == nil {
		client, err := newLambdaOptimizedClientFn(context.Background(), cfg.Region)
		if err != nil {
			logger.Error("failed to initialize dynamo client for stream queue",
				zap.String("region", cfg.Region),
				zap.Error(err))
			return nil
		}
		coreDB = client
	}

	return streaming.NewDynamoStreamQueue(coreDB, cfg.DynamoTableName, logger)
}

// initializeGraphQLSpecificServices initializes GraphQL-specific components
func initializeGraphQLSpecificServices() {
	// Initialize AI service (optional)
	var aiService *ai.AIService
	if !cfg.DisableAI {
		aiConfig := &ai.AIConfig{
			ToxicityThreshold:   0.7,
			NSFWThreshold:       0.8,
			SpamThreshold:       0.75,
			AIContentThreshold:  0.85,
			EnablePIIDetection:  true,
			EnableAIDetection:   false,
			EnableImageAnalysis: false,
			BedrockModelID:      "anthropic.claude-v2",
			S3Bucket:            cfg.S3BucketName,
		}
		aiService = ai.NewAIService(lambdaCtx.AWSServices.Config, aiConfig)
		logger.Info("AI service initialized")
	} else {
		logger.Info("AI service disabled")
	}

	// Initialize event publisher for real-time updates via DynamoDB streams
	streamQueue := resolveStreamQueue()
	if streamQueue == nil {
		logger.Fatal("stream queue not available for graphql publisher")
	}
	publisher := streaming.NewQueuePublisher(streamQueue, logger)

	// Create service registry with all dependencies
	serviceConfig := &services.ServiceConfig{
		BaseURL:   cfg.BaseURL(),
		JWTSecret: cfg.JWTSecret,
		Config:    cfg,
	}

	registry, err := services.NewRegistry(
		services.WithStorage(repos),
		services.WithPublisher(publisher),
		services.WithLogger(logger),
		services.WithConfig(serviceConfig),
	)
	if err != nil {
		logger.Fatal("Failed to create service registry", zap.Error(err))
	}

	// Create unified tracker for centralized cost tracking
	unifiedTracker := cost.NewRepositoryTracker(nil, logger, "GraphQLResolver", "", "")

	// Create subscription manager with DynamoDB-backed persistence
	subscriptionManager := graph.NewSubscriptionManager(
		registry.StreamingConnectionRepository(),
		registry.Publisher(),
		logger,
	)

	// Start the subscription manager
	if err := subscriptionManager.Start(context.Background()); err != nil {
		logger.Warn("failed to start subscription manager", zap.Error(err))
	}

	// Initialize GraphQL resolver with service registry
	resolver := &graph.Resolver{
		Registry:            registry,
		Storage:             repos, // Keep for legacy resolvers
		Config:              cfg,
		CostTracker:         costTracker,
		UnifiedTracker:      unifiedTracker,
		TableName:           cfg.DynamoTableName,
		S3BucketName:        cfg.S3BucketName,
		MastodonConv:        mastodon.NewConverter(cfg.BaseURL()),
		Logger:              logger,
		SubscriptionManager: subscriptionManager,
		AIService:           aiService,
	}

	// Create GraphQL schema
	schema := graph.NewExecutableSchema(graph.Config{
		Resolvers: resolver,
	})

	// Create GraphQL handler
	server := handler.NewDefaultServer(schema)
	server.SetErrorPresenter(graphQLErrorPresenter)

	// Configure GraphQL handler
	server.AddTransport(transport.Websocket{})
	server.AddTransport(transport.Options{})
	server.AddTransport(transport.GET{})
	server.AddTransport(transport.POST{})
	server.AddTransport(transport.MultipartForm{
		MaxUploadSize: cfg.MaxUploadSize,
		MaxMemory:     cfg.MaxUploadSize,
	})

	// Add extensions
	server.Use(extension.Introspection{})

	// Add Apollo tracing in development
	if cfg.DebugMode {
		server.Use(apollotracing.Tracer{})
	}

	graphQLHandler = server

	logger.Info("GraphQL service initialized successfully",
		zap.String("version", "lift-dynamorm"),
		zap.Bool("enabled", true),
		zap.String("status", "ready"))
}

func graphQLErrorPresenter(ctx context.Context, err error) *gqlerror.Error {
	gqlErr := graphql.DefaultErrorPresenter(ctx, err)
	if appErr, ok := apperrors.AsAppError(err); ok {
		if gqlErr.Extensions == nil {
			gqlErr.Extensions = map[string]any{}
		}
		gqlErr.Extensions["code"] = string(appErr.Code)
		gqlErr.Extensions["http_status"] = appErr.HTTPStatusCode
		gqlErr.Message = appErr.Message
	}
	return gqlErr
}

// handleGraphQL processes GraphQL requests with proper context and DataLoader
func handleGraphQL(ctx *lift.Context) error {
	// Create request context with user information
	requestCtx := context.WithValue(ctx.Request.Context(), contextKeyUser, ctx.Get("user"))

	authHeader := common.ExtractAuthHeader(ctx)
	logger.Info("GraphQL request auth header check",
		zap.String("path", ctx.Request.Path),
		zap.Bool("has_header", authHeader != ""),
	)

	if claimsVal := ctx.Get("claims"); claimsVal != nil {
		logger.Info("GraphQL authentication context detected",
			zap.String("path", ctx.Request.Path),
			zap.String("claims_type", fmt.Sprintf("%T", claimsVal)),
		)
	} else {
		logger.Info("GraphQL request without authentication context",
			zap.String("path", ctx.Request.Path))
	}

	// Propagate authenticated username if available
	if username, ok := ctx.Get("username").(string); ok && username != "" {
		requestCtx = context.WithValue(requestCtx, contextKeyUser, username)
	}

	// Propagate claims for resolvers that require authentication
	claimsVal := ctx.Get("claims")
	if claimsVal != nil {
		logger.Info("GraphQL claims found in context",
			zap.String("claims_type", fmt.Sprintf("%T", claimsVal)),
			zap.Bool("is_common_claims", claimsVal != nil),
		)
		if claims, ok := claimsVal.(common.Claims); ok && claims != nil {
			logger.Info("GraphQL claims type assertion successful",
				zap.String("username", claims.GetUsername()),
			)
			requestCtx = context.WithValue(requestCtx, common.ContextKeyClaims, claims)
			// Ensure contextKeyUser is set even if username wasn't populated separately
			if username := claims.GetUsername(); username != "" {
				requestCtx = context.WithValue(requestCtx, contextKeyUser, username)
			} else {
				logger.Warn("GraphQL claims have empty username",
					zap.String("claims_type", fmt.Sprintf("%T", claims)),
				)
			}
		} else {
			logger.Warn("GraphQL claims type assertion failed",
				zap.String("actual_type", fmt.Sprintf("%T", claimsVal)),
			)
		}
	}

	requestCtx = context.WithValue(requestCtx, contextKeyCostTracker, ctx.Get("cost_tracker"))

	// Attach DataLoaders to request context so resolvers can batch fetches safely
	if loadersVal := ctx.Get("loaders"); loadersVal != nil {
		if loaders, ok := loadersVal.(*graph.Loaders); ok {
			requestCtx = graph.WithLoaders(requestCtx, loaders)
			requestCtx = context.WithValue(requestCtx, contextKeyLoaders, loaders)
		} else {
			logger.Warn("GraphQL loaders type assertion failed",
				zap.String("actual_type", fmt.Sprintf("%T", loadersVal)))
			requestCtx = context.WithValue(requestCtx, contextKeyLoaders, loadersVal)
		}
	}

	// Create HTTP request wrapper for GraphQL handler
	liftURL := ctx.Request.URL()
	parsedURL, _ := url.Parse(liftURL.Path)
	httpReq := &http.Request{
		Method: ctx.Request.Method,
		URL:    parsedURL,
		Header: make(http.Header),
		Body:   &bytesReader{data: ctx.Request.Body},
	}
	httpReq = httpReq.WithContext(requestCtx)

	// Copy headers
	for k, v := range ctx.Request.Headers {
		httpReq.Header.Set(k, v)
	}

	// Create response writer
	responseWriter := &graphQLResponseWriter{
		liftCtx: ctx,
		header:  make(http.Header),
	}

	// Process GraphQL request
	handlerStart := time.Now()
	logger.Info("GraphQL handler invocation started",
		zap.String("path", ctx.Request.Path),
		zap.String("method", ctx.Request.Method))
	graphQLHandler.ServeHTTP(responseWriter, httpReq)
	if hits, misses := graph.QuoteLoaderMetrics(requestCtx); hits > 0 || misses > 0 {
		logger.Info("quote target loader usage",
			zap.Int64("cache_hits", hits),
			zap.Int64("misses", misses))
	}
	logger.Info("GraphQL handler invocation completed",
		zap.String("path", ctx.Request.Path),
		zap.String("method", ctx.Request.Method),
		zap.Duration("duration", time.Since(handlerStart)))

	return nil
}

// handlePlayground serves the GraphQL playground for development
func handlePlayground(ctx *lift.Context) error {
	if !cfg.EnablePlayground {
		return lift.NotFound("Playground not enabled")
	}

	logger.Info("GraphQL playground request received",
		zap.String("method", ctx.Request.Method),
		zap.String("path", ctx.Request.Path))

	// Create HTTP request wrapper
	liftURL := ctx.Request.URL()
	parsedURL, _ := url.Parse(liftURL.Path)
	httpReq := &http.Request{
		Method: ctx.Request.Method,
		URL:    parsedURL,
		Header: make(http.Header),
	}
	httpReq = httpReq.WithContext(ctx.Request.Context())

	// Copy headers
	for k, v := range ctx.Request.Headers {
		httpReq.Header.Set(k, v)
	}

	// Create response writer
	responseWriter := &graphQLResponseWriter{
		liftCtx: ctx,
		header:  make(http.Header),
	}

	// Serve GraphQL playground
	playgroundHandler := playground.Handler("GraphQL Playground", "/graphql")
	playgroundHandler.ServeHTTP(responseWriter, httpReq)

	return nil
}

// graphQLResponseWriter implements http.ResponseWriter for GraphQL handlers
type graphQLResponseWriter struct {
	liftCtx    *lift.Context
	header     http.Header
	statusCode int
}

func (w *graphQLResponseWriter) Header() http.Header {
	return w.header
}

func (w *graphQLResponseWriter) Write(data []byte) (int, error) {
	// Copy headers to lift context
	for k, v := range w.header {
		if len(v) > 0 {
			w.liftCtx.Response.Headers[k] = v[0]
		}
	}

	// Set status code if not already set
	if w.statusCode != 0 {
		w.liftCtx.Response.StatusCode = w.statusCode
	}

	// Write response body
	w.liftCtx.Response.Body = string(data)

	return len(data), nil
}

func (w *graphQLResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// bytesReader implements io.ReadCloser for request body
type bytesReader struct {
	data   []byte
	offset int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *bytesReader) Close() error {
	return nil
}

// extractBearerToken is now handled by unified auth middleware

// createDataLoaderMiddleware creates DataLoader middleware for N+1 query prevention
func createDataLoaderMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Create DataLoaders for this request
			loaders := graph.NewLoaders(repos, logger)
			ctx.Set("loaders", loaders)
			return next.Handle(ctx)
		})
	}
}

// createCostTrackingMiddleware creates cost tracking middleware with centralized service integration
func createCostTrackingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Add legacy cost tracker to context for resolver compatibility
			ctx.Set("cost_tracker", costTracker)

			// Track request start
			start := time.Now()

			err := next.Handle(ctx)

			// Track costs with centralized service
			duration := time.Since(start)

			// Track with centralized cost tracking service
			if costTrackingService != nil {
				go func() {
					memoryMB := int64(128) // Default Lambda memory
					if envMem := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE"); envMem != "" {
						if mem, parseErr := strconv.ParseInt(envMem, 10, 64); parseErr == nil {
							memoryMB = mem
						}
					}

					// Track Lambda execution
					lambdaOp := cost.LambdaOperation{
						FunctionName: "graphql",
						Duration:     duration,
						MemoryMB:     memoryMB,
						Timestamp:    start,
					}
					if trackErr := costTrackingService.TrackLambdaInvocation(context.Background(), lambdaOp); trackErr != nil {
						logger.Warn("failed to track GraphQL Lambda cost", zap.Error(trackErr))
					}

					// Note: DynamoDB costs are tracked at the resolver level via the unified tracker
					// This middleware focuses on Lambda execution costs
				}()
			}

			// Log cost information
			logger.Info("GraphQL request completed",
				zap.Duration("duration", duration),
				zap.String("path", ctx.Request.Path),
				zap.String("method", ctx.Request.Method))

			return err
		})
	}
}

// createAuthMiddleware creates authentication middleware using unified patterns
func createAuthMiddleware() lift.Middleware {
	// Create OAuth service matching REST authentication semantics
	if cfg.JWTSecret == "" {
		logger.Fatal("JWT secret is not configured; cannot initialize GraphQL auth middleware")
	}

	auditLogger := auth.NewAuditLogger(repos, logger, auth.DefaultAuditConfig())
	oauthService := auth.NewOAuthService(cfg.JWTSecret, cfg, repos, auditLogger)
	return auth.CreateGraphQLAuthMiddleware(&oauthMiddlewareAdapter{service: oauthService}, logger)
}

func main() {
	// Create a new Lift application
	app := lift.New()
	if cfg.DebugMode {
		app = lift.New(lift.WithDebug())
	}

	// Add global middleware in correct order
	// 1. Panic recovery middleware (MUST be first to catch all panics)
	app.Use(middleware.PanicRecovery(lambdaCtx.Logger))

	// Apply strict security middleware for web clients
	middleware.ApplySecurityMiddleware(app, middleware.SecurityTypeAPI, logger)

	// Block GraphQL POST requests until instance activation completes to prevent mutations.
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if strings.EqualFold(ctx.Request.Method, http.MethodPost) {
				state, err := repos.Instance().GetInstanceState(ctx.Context)
				if err != nil || state.Locked {
					ctx.Response.StatusCode = http.StatusForbidden
					return ctx.JSON(map[string]any{
						"errors": []map[string]any{
							{"message": "instance is locked"},
						},
					})
				}
			}
			return next.Handle(ctx)
		})
	})

	// Timeout middleware
	app.Use(liftMiddleware.TimeoutMiddleware(liftMiddleware.TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
	}))

	// Request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("graphql-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			path := ctx.Request.Path
			method := ctx.Request.Method
			err := next.Handle(ctx)
			logger.Info("GraphQL request completed",
				zap.String("request_id", fmt.Sprintf("%v", ctx.Get("requestID"))),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Int("status", ctx.Response.StatusCode))
			return err
		})
	})

	// Recovery middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Panic recovered", zap.Any("panic", r))
					ctx.Response.StatusCode = 500
					_ = ctx.JSON(map[string]interface{}{
						"errors": []map[string]interface{}{
							{"message": "Internal server error"},
						},
					})
				}
			}()
			return next.Handle(ctx)
		})
	})

	// Unified error handling middleware
	app.Use(common.CreateGraphQLErrorMiddleware(logger))

	// Authentication middleware
	app.Use(createAuthMiddleware())

	// Cost tracking middleware
	app.Use(createCostTrackingMiddleware())

	// DataLoader middleware
	app.Use(createDataLoaderMiddleware())

	// EMF performance monitoring middleware
	if emfMetricsService != nil {
		if emfService, ok := emfMetricsService.(*observability.EMFMetricsService); ok {
			app.Use(observability.CreateEMFPerformanceMonitoringMiddleware(emfService))
		}
	}

	// Configure GraphQL routes
	_ = app.POST("/graphql", handleGraphQL)
	_ = app.GET("/graphql", handleGraphQL)
	_ = app.POST("/api/graphql", handleGraphQL)
	_ = app.GET("/api/graphql", handleGraphQL)

	// OPTIONS handlers for CORS preflight (CORS headers set by middleware)
	optionsHandler := func(ctx *lift.Context) error {
		// CORS headers are already set by middleware, just return 204 No Content
		return ctx.Status(204).Text("")
	}
	_ = app.Handle("OPTIONS", "/graphql", optionsHandler)
	_ = app.Handle("OPTIONS", "/api/graphql", optionsHandler)

	// GraphQL playground (development only)
	_ = app.GET("/playground", handlePlayground)

	// WebSocket endpoint for subscriptions
	_ = app.GET("/subscriptions", handleGraphQL) // GraphQL handler supports WebSocket

	// Health check endpoint
	_ = app.GET("/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]interface{}{
			"status":      "healthy",
			"service":     "graphql",
			"version":     "lift-dynamorm",
			"uptime":      time.Since(initTime).String(),
			"environment": cfg.Stage,
			"features": map[string]bool{
				"graphql":       true,
				"subscriptions": true,
				"playground":    cfg.EnablePlayground,
				"dataloaders":   true,
				"cost_tracking": true,
			},
		})
	})

	// Ready endpoint for load balancers
	_ = app.GET("/ready", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]interface{}{
			"ready":     true,
			"timestamp": time.Now().Unix(),
		})
	})

	logger.Info("GraphQL service starting",
		zap.String("version", "lift-dynamorm"),
		zap.Bool("enabled", true),
		zap.String("status", "ready"),
		zap.Bool("playground", cfg.EnablePlayground),
		zap.Bool("debug", cfg.DebugMode))

	// Use standardized Lambda handler with observability
	standardHandler := lambdaCtx.CreateStandardizedLambdaHandler(func(ctx context.Context, event interface{}) (interface{}, error) {
		return app.HandleRequest(ctx, event)
	})

	lambdaStartFn(standardHandler)
}
