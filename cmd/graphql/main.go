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
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/apollotracing"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/aws/aws-lambda-go/lambda"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/observability"
	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

const (
	// Environment variable constants
	envTrue = "true"
)

// Context key types for type-safe context values
type contextKey string

const (
	contextKeyUser        contextKey = "user"
	contextKeyCostTracker contextKey = "cost_tracker"
	contextKeyLoaders     contextKey = "loaders"
)

var (
	cfg               *config.Config
	repos             core.RepositoryStorage
	logger            *zap.Logger
	graphQLHandler    *handler.Server
	emfMetricsService *observability.EMFMetricsService
	costTracker       *cost.Tracker
	initTime          time.Time
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

	// Load AWS configuration
	awsCfg, err := awsConfig.LoadDefaultConfig(context.Background(),
		awsConfig.WithRegion(cfg.Region),
	)
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Create repository storage using factory pattern
	repos, err = factory.NewRepositoryFactory(db, tableName, awsCfg, logger)
	if err != nil {
		logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	// Initialize auth service (locally scoped as it's not used globally)
	_, err = auth.NewAuthService(repos)
	if err != nil {
		logger.Fatal("failed to initialize auth service", zap.Error(err))
	}

	// Initialize cost tracker
	costTracker = cost.New()

	// Initialize AI service (optional)
	var aiService *ai.AIService
	if os.Getenv("DISABLE_AI") != envTrue {
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
		aiService = ai.NewAIService(awsCfg, aiConfig, tableName)
		logger.Info("AI service initialized")
	} else {
		logger.Info("AI service disabled")
	}

	// Initialize EMF metrics service
	if os.Getenv("DISABLE_METRICS") != envTrue {
		emfMetricsService = observability.NewEMFMetricsService(logger)
		logger.Info("initialized EMF metrics service for GraphQL")
	}

	// Initialize GraphQL resolver with all dependencies
	resolver := &graph.Resolver{
		Storage:      repos,
		CostTracker:  costTracker,
		MastodonConv: mastodon.NewConverter(cfg.BaseURL()),
		Logger:       logger,
		AIService:    aiService,
	}

	// Create GraphQL schema
	schema := graph.NewExecutableSchema(graph.Config{
		Resolvers: resolver,
	})

	// Create GraphQL handler
	graphQLHandler = handler.NewDefaultServer(schema)

	// Configure GraphQL handler
	graphQLHandler.AddTransport(transport.Websocket{})
	graphQLHandler.AddTransport(transport.Options{})
	graphQLHandler.AddTransport(transport.GET{})
	graphQLHandler.AddTransport(transport.POST{})
	graphQLHandler.AddTransport(transport.MultipartForm{})

	// Add extensions
	graphQLHandler.Use(extension.Introspection{})

	// Add Apollo tracing in development
	if os.Getenv("DEBUG") == envTrue {
		graphQLHandler.Use(apollotracing.Tracer{})
	}

	logger.Info("GraphQL service initialized successfully",
		zap.String("version", "lift-dynamorm"),
		zap.Bool("enabled", true),
		zap.String("status", "ready"))
}

// handleGraphQL processes GraphQL requests with proper context and DataLoader
func handleGraphQL(ctx *lift.Context) error {
	// Create request context with user information
	requestCtx := context.WithValue(ctx.Request.Context(), contextKeyUser, ctx.Get("user"))
	requestCtx = context.WithValue(requestCtx, contextKeyCostTracker, ctx.Get("cost_tracker"))
	requestCtx = context.WithValue(requestCtx, contextKeyLoaders, ctx.Get("loaders"))

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
	graphQLHandler.ServeHTTP(responseWriter, httpReq)

	return nil
}

// handlePlayground serves the GraphQL playground for development
func handlePlayground(ctx *lift.Context) error {
	if os.Getenv("ENABLE_PLAYGROUND") != envTrue {
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

// extractBearerToken extracts Bearer token from Authorization header
func extractBearerToken(ctx *lift.Context) string {
	authHeader := ctx.Request.Headers["Authorization"]
	if authHeader == "" {
		return ""
	}

	// Check for Bearer token format
	const bearerPrefix = "Bearer "
	if strings.HasPrefix(authHeader, bearerPrefix) {
		return strings.TrimPrefix(authHeader, bearerPrefix)
	}

	return ""
}

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

// createCostTrackingMiddleware creates cost tracking middleware
func createCostTrackingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Add cost tracker to context
			ctx.Set("cost_tracker", costTracker)

			// Track request start
			start := time.Now()

			err := next.Handle(ctx)

			// Log cost information
			duration := time.Since(start)
			logger.Info("GraphQL request completed",
				zap.Duration("duration", duration),
				zap.String("path", ctx.Request.Path),
				zap.String("method", ctx.Request.Method))

			return err
		})
	}
}

// createAuthMiddleware creates authentication middleware
func createAuthMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Extract authorization header
			authHeader := ctx.Request.Headers["Authorization"]
			if authHeader == "" {
				// No authentication - continue with anonymous access
				return next.Handle(ctx)
			}

			// Extract Bearer token
			token := extractBearerToken(ctx)
			if token == "" {
				logger.Debug("Invalid authorization format - expected Bearer token")
				// Continue without authentication for GraphQL introspection queries
				return next.Handle(ctx)
			}

			// Create auth service for this request
			authService, err := auth.NewAuthService(repos)
			if err != nil {
				logger.Error("Failed to create auth service", zap.Error(err))
				return next.Handle(ctx)
			}

			// Validate JWT access token
			claims, err := authService.ValidateAccessToken(token)
			if err != nil {
				logger.Debug("Invalid access token", zap.Error(err))
				// Continue without authentication - GraphQL can handle unauthorized requests
				return next.Handle(ctx)
			}

			// Store enhanced claims in context for resolvers
			ctx.Set("claims", claims)
			ctx.Set("user", claims) // For GraphQL context compatibility
			ctx.Set("username", claims.Username)
			ctx.Set("session_id", claims.SessionID)
			ctx.Set("device_id", claims.DeviceID)

			logger.Debug("User authenticated",
				zap.String("username", claims.Username),
				zap.String("session_id", claims.SessionID))

			return next.Handle(ctx)
		})
	}
}

// createCORSMiddleware creates CORS middleware for GraphQL
func createCORSMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Set CORS headers for GraphQL
			ctx.Response.Headers["Access-Control-Allow-Origin"] = "*"
			ctx.Response.Headers["Access-Control-Allow-Methods"] = "GET, POST, OPTIONS"
			ctx.Response.Headers["Access-Control-Allow-Headers"] = "Content-Type, Authorization, X-Requested-With"
			ctx.Response.Headers["Access-Control-Allow-Credentials"] = envTrue
			ctx.Response.Headers["Access-Control-Max-Age"] = "86400"

			// Handle preflight requests
			if ctx.Request.Method == "OPTIONS" {
				ctx.Response.StatusCode = 200
				return nil
			}

			return next.Handle(ctx)
		})
	}
}

func main() {
	// Create a new Lift application
	app := lift.New()
	if os.Getenv("DEBUG") == envTrue {
		app = lift.New(lift.WithDebug())
	}

	// Add global middleware in correct order
	// 1. Timeout middleware
	app.Use(middleware.TimeoutMiddleware(middleware.TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
	}))

	// 2. Request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("graphql-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// 3. Logging middleware
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

	// 4. Recovery middleware
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

	// 5. CORS middleware
	app.Use(createCORSMiddleware())

	// 6. Rate limiting middleware (before auth to catch anonymous users)
	if os.Getenv("DISABLE_RATE_LIMITING") != "true" {
		// Create GraphQL-specific rate limiting config
		graphqlConfig := ratelimit.DefaultRateLimitConfig()
		// Add GraphQL-specific limits
		graphqlConfig.EndpointLimits["POST:/graphql"] = ratelimit.EndpointLimit{Limit: 100, Window: 5 * time.Minute} // 100 queries per 5 minutes
		graphqlConfig.EndpointLimits["GET:/graphql"] = ratelimit.EndpointLimit{Limit: 100, Window: 5 * time.Minute}  // 100 queries per 5 minutes (GET for introspection)
		
		app.Use(ratelimit.Middleware(repos, graphqlConfig))
		logger.Info("enabled rate limiting middleware for GraphQL service")
	}

	// 7. Authentication middleware
	app.Use(createAuthMiddleware())

	// 8. Cost tracking middleware
	app.Use(createCostTrackingMiddleware())

	// 9. DataLoader middleware
	app.Use(createDataLoaderMiddleware())

	// 10. EMF performance monitoring middleware
	if emfMetricsService != nil {
		app.Use(observability.CreateEMFPerformanceMonitoringMiddleware(emfMetricsService))
	}

	// Configure GraphQL routes
	_ = app.POST("/graphql", handleGraphQL)
	_ = app.GET("/graphql", handleGraphQL)
	// OPTIONS requests are handled by CORS middleware

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
			"environment": os.Getenv("ENV"),
			"features": map[string]bool{
				"graphql":       true,
				"subscriptions": true,
				"playground":    os.Getenv("ENABLE_PLAYGROUND") == envTrue,
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
		zap.Bool("playground", os.Getenv("ENABLE_PLAYGROUND") == envTrue),
		zap.Bool("debug", os.Getenv("DEBUG") == envTrue))

	// Start the Lambda handler with EMF metrics flushing
	lambdaHandler := func(ctx context.Context, event interface{}) (interface{}, error) {
		// Process the request
		result, err := app.HandleRequest(ctx, event)

		// CRITICAL: Flush EMF metrics before Lambda terminates
		if emfMetricsService != nil {
			if flushErr := emfMetricsService.FlushMetrics(); flushErr != nil {
				logger.Error("failed to flush EMF metrics", zap.Error(flushErr))
			}
		}

		return result, err
	}

	lambda.Start(lambdaHandler)
}
