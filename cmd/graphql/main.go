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
	"encoding/json"
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
	gqllimits "github.com/equaltoai/lesser/pkg/graphql/limits"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/streaming"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
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
	newLambdaOptimizedClientFn          = theorydb.NewLambdaOptimizedClient
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
	server := handler.New(schema)
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

	// Harden request parsing and execution limits (abuse-resilience).
	if cfg.GraphQLParserTokenLimit > 0 {
		server.SetParserTokenLimit(cfg.GraphQLParserTokenLimit)
	}
	// Depth enforcement: agents are restricted to shallow queries (max depth 3), humans use configured depth.
	if cfg.GraphQLMaxDepth > 0 {
		server.Use(&gqllimits.DepthLimit{
			Func: func(ctx context.Context, _ *graphql.OperationContext) int {
				if claimsVal := ctx.Value(common.ContextKeyClaims); claimsVal != nil {
					if claims, ok := claimsVal.(*auth.Claims); ok && claims.IsAgent {
						return 3
					}
				}
				return cfg.GraphQLMaxDepth
			},
		})
	} else {
		// Even if depth is disabled for humans, enforce a strict limit for agents.
		server.Use(&gqllimits.DepthLimit{
			Func: func(ctx context.Context, _ *graphql.OperationContext) int {
				if claimsVal := ctx.Value(common.ContextKeyClaims); claimsVal != nil {
					if claims, ok := claimsVal.(*auth.Claims); ok && claims.IsAgent {
						return 3
					}
				}
				return 0
			},
		})
	}
	if cfg.GraphQLMaxComplexity > 0 {
		server.Use(extension.FixedComplexityLimit(cfg.GraphQLMaxComplexity))
	}

	// Introspection is disabled by default; enable it explicitly for debug/playground workflows.
	if cfg.DebugMode || cfg.EnablePlayground || cfg.GraphQLAllowIntrospection {
		server.Use(extension.Introspection{})
	}

	// Add Apollo tracing in development
	if cfg.DebugMode {
		server.Use(apollotracing.Tracer{})
	}

	graphQLHandler = server

	logger.Info("GraphQL service initialized successfully",
		zap.String("version", "apptheory-dynamorm"),
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
func handleGraphQL(ctx *apptheory.Context) (*apptheory.Response, error) {
	requestCtx := context.Background()
	if ctx != nil {
		requestCtx = ctx.Context()
	}

	timeout := 30 * time.Second
	if cfg != nil && cfg.GraphQLRequestTimeout > 0 {
		timeout = cfg.GraphQLRequestTimeout
	}
	var cancel context.CancelFunc
	requestCtx, cancel = context.WithTimeout(requestCtx, timeout)
	defer cancel()

	// Create request context with user information.
	if ctx != nil {
		requestCtx = context.WithValue(requestCtx, contextKeyUser, ctx.Get("user"))
	}

	authHeader := graphqlExtractAuthHeader(ctx)
	logger.Info("GraphQL request auth header check",
		zap.String("path", graphqlPath(ctx)),
		zap.Bool("has_header", authHeader != ""),
	)

	if ctx != nil {
		if claimsVal := ctx.Get("claims"); claimsVal != nil {
			logger.Info("GraphQL authentication context detected",
				zap.String("path", ctx.Request.Path),
				zap.String("claims_type", fmt.Sprintf("%T", claimsVal)),
			)
		} else {
			logger.Info("GraphQL request without authentication context",
				zap.String("path", ctx.Request.Path))
		}

		// Propagate authenticated username if available.
		if username, ok := ctx.Get("username").(string); ok && username != "" {
			requestCtx = context.WithValue(requestCtx, contextKeyUser, username)
		}

		// Propagate claims for resolvers that require authentication.
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
				// Ensure contextKeyUser is set even if username wasn't populated separately.
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

		// Attach DataLoaders to request context so resolvers can batch fetches safely.
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
	}

	// Create HTTP request wrapper for GraphQL handler.
	method := ""
	path := ""
	body := []byte(nil)
	headers := map[string][]string(nil)
	query := map[string][]string(nil)
	if ctx != nil {
		method = ctx.Request.Method
		path = ctx.Request.Path
		body = ctx.Request.Body
		headers = ctx.Request.Headers
		query = ctx.Request.Query
	}

	u := &url.URL{Path: path}
	q := u.Query()
	for key, values := range query {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	u.RawQuery = q.Encode()

	httpReq := &http.Request{
		Method: method,
		URL:    u,
		Header: make(http.Header),
		Body:   &bytesReader{data: body},
	}
	httpReq = httpReq.WithContext(requestCtx)

	// Copy headers.
	for k, values := range headers {
		for _, value := range values {
			httpReq.Header.Add(k, value)
		}
	}

	// Process GraphQL request.
	responseWriter := newGraphQLResponseWriter()
	handlerStart := time.Now()
	logger.Info("GraphQL handler invocation started",
		zap.String("path", path),
		zap.String("method", method))
	graphQLHandler.ServeHTTP(responseWriter, httpReq)
	if hits, misses := graph.QuoteLoaderMetrics(requestCtx); hits > 0 || misses > 0 {
		logger.Info("quote target loader usage",
			zap.Int64("cache_hits", hits),
			zap.Int64("misses", misses))
	}
	logger.Info("GraphQL handler invocation completed",
		zap.String("path", path),
		zap.String("method", method),
		zap.Duration("duration", time.Since(handlerStart)))

	return responseWriter.Response(), nil
}

// handlePlayground serves the GraphQL playground for development
func handlePlayground(ctx *apptheory.Context) (*apptheory.Response, error) {
	if cfg == nil || !cfg.EnablePlayground {
		return apptheory.Text(http.StatusNotFound, "Playground not enabled"), nil
	}

	method := ""
	path := ""
	headers := map[string][]string(nil)
	query := map[string][]string(nil)
	reqCtx := context.Background()
	if ctx != nil {
		method = ctx.Request.Method
		path = ctx.Request.Path
		headers = ctx.Request.Headers
		query = ctx.Request.Query
		reqCtx = ctx.Context()
	}

	logger.Info("GraphQL playground request received",
		zap.String("method", method),
		zap.String("path", path))

	u := &url.URL{Path: path}
	q := u.Query()
	for key, values := range query {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	u.RawQuery = q.Encode()

	httpReq := &http.Request{
		Method: method,
		URL:    u,
		Header: make(http.Header),
	}
	httpReq = httpReq.WithContext(reqCtx)

	for k, values := range headers {
		for _, value := range values {
			httpReq.Header.Add(k, value)
		}
	}

	responseWriter := newGraphQLResponseWriter()

	playgroundHandler := playground.Handler("GraphQL Playground", "/graphql")
	playgroundHandler.ServeHTTP(responseWriter, httpReq)

	return responseWriter.Response(), nil
}

// graphQLResponseWriter implements http.ResponseWriter for GraphQL handlers
type graphQLResponseWriter struct {
	header     http.Header
	statusCode int
	body       []byte
}

func (w *graphQLResponseWriter) Header() http.Header {
	return w.header
}

func (w *graphQLResponseWriter) Write(data []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	w.body = append(w.body, data...)
	return len(data), nil
}

func (w *graphQLResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func newGraphQLResponseWriter() *graphQLResponseWriter {
	return &graphQLResponseWriter{
		header: make(http.Header),
	}
}

func (w *graphQLResponseWriter) Response() *apptheory.Response {
	status := w.statusCode
	if status == 0 {
		status = http.StatusOK
	}

	headers := map[string][]string{}
	for key, values := range w.header {
		if len(values) == 0 {
			continue
		}
		headers[strings.ToLower(key)] = append([]string(nil), values...)
	}

	return &apptheory.Response{
		Status:  status,
		Headers: headers,
		Body:    append([]byte(nil), w.body...),
	}
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
func createDataLoaderMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx != nil {
				// Create DataLoaders for this request.
				loaders := graph.NewLoaders(repos, logger)
				ctx.Set("loaders", loaders)
			}
			return next(ctx)
		}
	}
}

// createCostTrackingMiddleware creates cost tracking middleware with centralized service integration
func createCostTrackingMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx != nil {
				// Add legacy cost tracker to context for resolver compatibility.
				ctx.Set("cost_tracker", costTracker)
			}

			start := time.Now()
			resp, err := next(ctx)
			duration := time.Since(start)

			// Track with centralized cost tracking service.
			if costTrackingService != nil {
				go func() {
					memoryMB := int64(128) // Default Lambda memory
					if envMem := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE"); envMem != "" {
						if mem, parseErr := strconv.ParseInt(envMem, 10, 64); parseErr == nil {
							memoryMB = mem
						}
					}

					lambdaOp := cost.LambdaOperation{
						FunctionName: "graphql",
						Duration:     duration,
						MemoryMB:     memoryMB,
						Timestamp:    start,
					}
					if trackErr := costTrackingService.TrackLambdaInvocation(context.Background(), lambdaOp); trackErr != nil {
						logger.Warn("failed to track GraphQL Lambda cost", zap.Error(trackErr))
					}
				}()
			}

			method := ""
			path := ""
			if ctx != nil {
				method = ctx.Request.Method
				path = ctx.Request.Path
			}

			logger.Info("GraphQL request completed",
				zap.Duration("duration", duration),
				zap.String("path", path),
				zap.String("method", method))

			return resp, err
		}
	}
}

// createAuthMiddleware creates authentication middleware using unified patterns
func createAuthMiddleware() apptheory.Middleware {
	// Create OAuth service matching REST authentication semantics
	if cfg.JWTSecret == "" {
		logger.Fatal("JWT secret is not configured; cannot initialize GraphQL auth middleware")
	}

	auditLogger := auth.NewAuditLogger(repos, logger, auth.DefaultAuditConfig())
	oauthService := auth.NewOAuthService(cfg.JWTSecret, cfg, repos, auditLogger)
	adapter := &oauthMiddlewareAdapter{service: oauthService}

	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx != nil {
				authHeader := graphqlHeaderValue(ctx, "authorization")
				token := strings.TrimSpace(authHeader)
				if token != "" {
					token = strings.TrimPrefix(token, "Bearer ")
					token = strings.TrimPrefix(token, "bearer ")
					token = strings.TrimSpace(token)
				}

				if token == "" {
					ctx.Set("is_authenticated", false)
				} else {
					claims, err := adapter.ValidateAccessToken(token)
					if err != nil || claims == nil {
						ctx.Set("is_authenticated", false)
						logger.Warn("optional authentication failed - header present but validation failed",
							zap.String("service", "graphql"),
							zap.String("path", ctx.Request.Path),
							zap.Bool("has_auth_header", true),
						)
					} else {
						ctx.Set("claims", claims)
						ctx.Set("username", claims.GetUsername())
						ctx.Set("is_authenticated", true)
					}
				}
			}

			return next(ctx)
		}
	}
}

func main() {
	app := buildApp(logger)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func buildApp(lambdaLogger *zap.Logger) *apptheory.App {
	app := apptheory.New(
		apptheory.WithCORS(apptheory.CORSConfig{
			AllowedOrigins:   []string{"*"},
			AllowCredentials: false,
			AllowHeaders: []string{
				"Accept",
				"Authorization",
				"Content-Type",
				"User-Agent",
				"X-Forwarded-For",
				"X-Forwarded-Proto",
			},
		}),
		apptheory.WithLimits(apptheory.Limits{
			MaxRequestBytes:  1024 * 1024,
			MaxResponseBytes: 0,
		}),
	)

	// Panic recovery middleware (MUST be first to catch all panics).
	app.Use(graphqlPanicRecovery(lambdaLogger))

	// Block GraphQL POST requests until instance activation completes to prevent mutations.
	app.Use(graphqlInstanceLockMiddleware())

	// Request ID middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx != nil {
				ctx.Set("requestID", graphqlRequestID(ctx, "graphql"))
			}
			return next(ctx)
		}
	})

	// Security headers middleware (web-friendly).
	app.Use(graphqlSecurityHeaders())

	// Authentication middleware.
	app.Use(createAuthMiddleware())

	// Cost tracking middleware.
	app.Use(createCostTrackingMiddleware())

	// DataLoader middleware.
	app.Use(createDataLoaderMiddleware())

	// Logging middleware.
	app.Use(func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			start := time.Now()
			resp, err := next(ctx)

			requestID := graphqlContextRequestID(ctx)
			hasError := err != nil
			if !hasError && resp != nil && resp.Status >= 400 {
				hasError = true
			}

			method := ""
			path := ""
			status := 0
			if ctx != nil {
				method = ctx.Request.Method
				path = ctx.Request.Path
			}
			if resp != nil {
				status = resp.Status
			}

			logger.Info("GraphQL request completed",
				zap.String("request_id", requestID),
				zap.String("method", method),
				zap.String("path", path),
				zap.Duration("duration", time.Since(start)),
				zap.Int("status", status),
				zap.Bool("has_error", hasError),
			)

			return resp, err
		}
	})

	app.Post("/graphql", handleGraphQL)
	app.Get("/graphql", handleGraphQL)
	app.Post("/api/graphql", handleGraphQL)
	app.Get("/api/graphql", handleGraphQL)

	optionsHandler := func(*apptheory.Context) (*apptheory.Response, error) {
		return &apptheory.Response{Status: http.StatusNoContent}, nil
	}
	app.Options("/graphql", optionsHandler)
	app.Options("/api/graphql", optionsHandler)

	// GraphQL playground (development only).
	app.Get("/playground", handlePlayground)

	// WebSocket endpoint for subscriptions (HTTP route; actual WebSockets handled by the dedicated `graphql-ws` Lambda).
	app.Get("/subscriptions", handleGraphQL)

	app.Get("/health", func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.JSON(http.StatusOK, map[string]interface{}{
			"status":      "healthy",
			"service":     "graphql",
			"version":     "apptheory-dynamorm",
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

	app.Get("/ready", func(*apptheory.Context) (*apptheory.Response, error) {
		return apptheory.JSON(http.StatusOK, map[string]interface{}{
			"ready":     true,
			"timestamp": time.Now().Unix(),
		})
	})

	logger.Info("GraphQL service starting",
		zap.String("version", "apptheory-dynamorm"),
		zap.Bool("enabled", true),
		zap.String("status", "ready"),
		zap.Bool("playground", cfg.EnablePlayground),
		zap.Bool("debug", cfg.DebugMode))

	return app
}

func graphqlPanicRecovery(logger *zap.Logger) apptheory.Middleware {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (resp *apptheory.Response, err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered",
						zap.String("request_id", graphqlContextRequestID(ctx)),
						zap.Any("panic", r),
					)
					resp = graphqlErrorResponse(http.StatusInternalServerError, "Internal server error")
					err = nil
				}
			}()
			return next(ctx)
		}
	}
}

func graphqlInstanceLockMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx == nil {
				return next(ctx)
			}
			if !strings.EqualFold(ctx.Request.Method, http.MethodPost) {
				return next(ctx)
			}
			if ctx.Request.Path != "/graphql" && ctx.Request.Path != "/api/graphql" {
				return next(ctx)
			}

			if repos == nil || repos.Instance() == nil {
				return graphqlErrorResponse(http.StatusForbidden, "instance is locked"), nil
			}

			state, err := repos.Instance().GetInstanceState(ctx.Context())
			if err != nil || state == nil || state.Locked {
				return graphqlErrorResponse(http.StatusForbidden, "instance is locked"), nil
			}
			return next(ctx)
		}
	}
}

func graphqlSecurityHeaders() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			resp, err := next(ctx)
			if resp == nil {
				return resp, err
			}
			if resp.Headers == nil {
				resp.Headers = map[string][]string{}
			}
			resp.Headers["x-content-type-options"] = []string{"nosniff"}
			resp.Headers["x-frame-options"] = []string{"DENY"}
			resp.Headers["referrer-policy"] = []string{"strict-origin-when-cross-origin"}
			resp.Headers["cross-origin-resource-policy"] = []string{"same-origin"}
			resp.Headers["x-robots-tag"] = []string{"noindex, nofollow"}
			return resp, err
		}
	}
}

func graphqlHeaderValue(ctx *apptheory.Context, key string) string {
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

func graphqlExtractAuthHeader(ctx *apptheory.Context) string {
	return graphqlHeaderValue(ctx, "authorization")
}

func graphqlPath(ctx *apptheory.Context) string {
	if ctx == nil {
		return ""
	}
	return ctx.Request.Path
}

func graphqlRequestID(ctx *apptheory.Context, prefix string) string {
	if ctx != nil && strings.TrimSpace(ctx.RequestID) != "" {
		return ctx.RequestID
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "graphql"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func graphqlContextRequestID(ctx *apptheory.Context) string {
	if ctx == nil {
		return ""
	}
	if rid, ok := ctx.Get("requestID").(string); ok && strings.TrimSpace(rid) != "" {
		return strings.TrimSpace(rid)
	}
	if strings.TrimSpace(ctx.RequestID) != "" {
		return ctx.RequestID
	}
	return ""
}

func graphqlErrorResponse(status int, message string) *apptheory.Response {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Internal server error"
	}
	return apptheory.MustJSON(status, map[string]any{
		"errors": []map[string]any{
			{"message": message},
		},
	})
}
