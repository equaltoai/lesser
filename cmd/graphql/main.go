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
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/streaming"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
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
	costTracker         *cost.Tracker // Legacy tracker for resolver compatibility
	costTrackingService lambdaInvocationTracker
	initTime            time.Time
)

var (
	runningUnitTestsFn     = common.RunningUnitTests
	mustInitializeLambdaFn = common.MustInitializeLambda

	extractStandardizedServicesFn       = extractStandardizedServices
	initializeManualServicesFn          = initializeManualServices
	initializeGraphQLSpecificServicesFn = initializeGraphQLSpecificServices
	lambdaStartFn                       = lambda.Start
	newLambdaOptimizedClientFn          = theorydb.NewLambdaOptimizedClient
	newRepositoryFactoryFn              = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (core.RepositoryStorage, error) {
		return factory.NewRepositoryFactory(db, tableName, logger)
	}
)

var anonymousGraphQLPublicQueryFields = map[string]struct{}{
	"__typename":       {},
	"actor":            {},
	"agent":            {},
	"agents":           {},
	"announcements":    {},
	"article":          {},
	"articleBySlug":    {},
	"articles":         {},
	"categories":       {},
	"customEmojis":     {},
	"instance":         {},
	"instanceActivity": {},
	"instancePeers":    {},
	"object":           {},
	"search":           {},
	"series":           {},
	"seriesBySlug":     {},
	"threadContext":    {},
	"timeline":         {},
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

	if _, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName:          "graphql",
		RequireRepositories:  true,
		NewDB:                newLambdaOptimizedClientFn,
		NewRepositoryStorage: newRepositoryFactoryFn,
	}); err != nil {
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
		aiConfig := &ai.Config{
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
		JWTSecret: resolveJWTSecretForGraphQL(logger),
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
	schema := graph.NewExecutableSchema(graph.NewConfig(resolver))

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
	// Depth enforcement: agents and CLI automation tokens are restricted to shallow queries (max depth 3),
	// humans use configured depth.
	if cfg.GraphQLMaxDepth > 0 {
		server.Use(&gqllimits.DepthLimit{
			Func: func(ctx context.Context, _ *graphql.OperationContext) int {
				if claimsVal := ctx.Value(common.ContextKeyClaims); claimsVal != nil {
					if claims, ok := claimsVal.(*auth.Claims); ok && (claims.IsAgent || strings.EqualFold(claims.ClientClass, auth.ClientClassCLI)) {
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
					if claims, ok := claimsVal.(*auth.Claims); ok && (claims.IsAgent || strings.EqualFold(claims.ClientClass, auth.ClientClassCLI)) {
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
	if root := graphQLErrorRootField(ctx, gqlErr); isCMSGraphQLRootField(root) {
		err = classifyCMSGraphQLError(err)
	}
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

func graphQLErrorRootField(ctx context.Context, gqlErr *gqlerror.Error) string {
	path := graphql.GetPath(ctx)
	if len(path) == 0 && gqlErr != nil {
		path = gqlErr.Path
	}
	if len(path) == 0 {
		return ""
	}
	name, ok := path[0].(ast.PathName)
	if !ok {
		return ""
	}
	return string(name)
}

func isCMSGraphQLRootField(field string) bool {
	switch field {
	case "draft", "draftPreview", "myDrafts", "myDraftReviews", "sharedDraftReviews", "draftReview",
		"revisions", "revision", "article", "articleBySlug", "articles",
		"series", "seriesBySlug", "allSeries", "category", "categoryBySlug",
		"categories", "rootCategories", "publication", "publicationBySlug", "myPublications",
		"createDraft", "updateDraft", "autosaveDraft", "deleteDraft", "publishDraft",
		"scheduleDraft", "cancelScheduledDraft", "shareDraftForReview", "revokeDraftReview",
		"submitDraftReview", "createArticle", "updateArticle", "deleteArticle", "restoreRevision",
		"createSeries", "updateSeries", "deleteSeries", "addArticleToSeries", "removeArticleFromSeries",
		"reorderSeriesArticles", "createCategory", "updateCategory", "deleteCategory",
		"addArticleToCategory", "removeArticleFromCategory", "createPublication", "updatePublication",
		"invitePublicationMember", "removePublicationMember", "updatePublicationMemberRole":
		return true
	default:
		return false
	}
}

func classifyCMSGraphQLError(err error) error {
	if err == nil {
		return nil
	}
	if appErr, ok := apperrors.AsAppError(err); ok {
		if appErr.Category != apperrors.CategoryValidation {
			return err
		}
		classified := appErr.Clone()
		classified.Code = apperrors.CodeValidation
		classified.HTTPStatusCode = apperrors.CodeValidation.GetHTTPStatusCode()
		return classified
	}

	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not found"):
		return apperrors.WrapError(err, apperrors.CodeNotFound, apperrors.CategoryBusiness, "CMS resource not found")
	case strings.Contains(message, "insufficient privileges"),
		strings.Contains(message, "access denied"),
		strings.Contains(message, "does not belong"),
		strings.Contains(message, "cannot review"),
		strings.Contains(message, "only publication owners"):
		return apperrors.WrapError(err, apperrors.CodeForbidden, apperrors.CategoryAuth, "CMS operation is forbidden")
	case strings.Contains(message, "required"),
		strings.Contains(message, "invalid"),
		strings.Contains(message, "must "),
		strings.Contains(message, "already in use"):
		return apperrors.WrapError(err, apperrors.CodeValidation, apperrors.CategoryValidation, err.Error())
	default:
		return apperrors.WrapError(err, apperrors.CodeInternal, apperrors.CategoryBusiness, "CMS operation failed")
	}
}

type graphQLHTTPRequestParts struct {
	method  string
	path    string
	body    []byte
	headers map[string][]string
	query   map[string][]string
}

func graphqlExtractRequestParts(ctx *apptheory.Context) graphQLHTTPRequestParts {
	parts := graphQLHTTPRequestParts{}
	if ctx == nil {
		return parts
	}
	parts.method = ctx.Request.Method
	parts.path = ctx.Request.Path
	parts.body = ctx.Request.Body
	parts.headers = ctx.Request.Headers
	parts.query = ctx.Request.Query
	return parts
}

func graphqlBuildURL(path string, query map[string][]string) *url.URL {
	u := &url.URL{Path: path}
	if len(query) == 0 {
		return u
	}

	q := u.Query()
	for key, values := range query {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	u.RawQuery = q.Encode()
	return u
}

func graphqlCopyHeaders(dst http.Header, headers map[string][]string) {
	if len(headers) == 0 || dst == nil {
		return
	}
	for k, values := range headers {
		for _, value := range values {
			dst.Add(k, value)
		}
	}
}

func graphqlWithTimeout(ctx *apptheory.Context) (context.Context, context.CancelFunc) {
	requestCtx := context.Background()
	if ctx != nil {
		requestCtx = ctx.Context()
	}

	timeout := 30 * time.Second
	if cfg != nil && cfg.GraphQLRequestTimeout > 0 {
		timeout = cfg.GraphQLRequestTimeout
	}

	return context.WithTimeout(requestCtx, timeout)
}

func graphqlWithUser(requestCtx context.Context, ctx *apptheory.Context) context.Context {
	if ctx == nil {
		return requestCtx
	}

	requestCtx = context.WithValue(requestCtx, contextKeyUser, ctx.Get("user"))

	if username := auth.GetAuthenticatedUsername(ctx); username != "" {
		requestCtx = context.WithValue(requestCtx, contextKeyUser, username)
	}

	return requestCtx
}

type graphqlOperationRequest struct {
	Query         string `json:"query"`
	OperationName string `json:"operationName"`
}

func graphqlWithClaims(requestCtx context.Context, ctx *apptheory.Context) context.Context {
	if ctx == nil {
		return requestCtx
	}

	claimsVal := auth.GetJWTClaims(ctx)
	if claimsVal == nil {
		logger.Info("GraphQL request without authentication context",
			zap.String("path", ctx.Request.Path))
		return requestCtx
	}

	logger.Info("GraphQL authentication context detected",
		zap.String("path", ctx.Request.Path),
		zap.String("claims_type", fmt.Sprintf("%T", claimsVal)),
	)

	logger.Info("GraphQL claims type assertion successful",
		zap.String("username", claimsVal.GetUsername()),
	)
	requestCtx = context.WithValue(requestCtx, common.ContextKeyClaims, claimsVal)

	if username := claimsVal.GetUsername(); username != "" {
		requestCtx = context.WithValue(requestCtx, contextKeyUser, username)
	} else {
		logger.Warn("GraphQL claims have empty username",
			zap.String("claims_type", fmt.Sprintf("%T", claimsVal)),
		)
	}
	return requestCtx
}

func graphqlWithCostTracker(requestCtx context.Context, ctx *apptheory.Context) context.Context {
	if ctx == nil {
		return requestCtx
	}
	return context.WithValue(requestCtx, contextKeyCostTracker, ctx.Get("cost_tracker"))
}

func graphqlWithLoaders(requestCtx context.Context, ctx *apptheory.Context) context.Context {
	if ctx == nil {
		return requestCtx
	}

	loadersVal := ctx.Get("loaders")
	if loadersVal == nil {
		return requestCtx
	}

	if loaders, ok := loadersVal.(*graph.Loaders); ok {
		requestCtx = graph.WithLoaders(requestCtx, loaders)
		requestCtx = context.WithValue(requestCtx, contextKeyLoaders, loaders)
		return requestCtx
	}

	logger.Warn("GraphQL loaders type assertion failed",
		zap.String("actual_type", fmt.Sprintf("%T", loadersVal)))
	return context.WithValue(requestCtx, contextKeyLoaders, loadersVal)
}

func graphqlLogQuoteLoaderMetrics(requestCtx context.Context) {
	hits, misses := graph.QuoteLoaderMetrics(requestCtx)
	if hits == 0 && misses == 0 {
		return
	}
	logger.Info("quote target loader usage",
		zap.Int64("cache_hits", hits),
		zap.Int64("misses", misses))
}

func graphqlRequestAuthenticated(ctx *apptheory.Context) bool {
	return auth.IsAuthenticated(ctx)
}

func graphqlAnonymousRequestAllowed(ctx *apptheory.Context) bool {
	if graphqlRequestAuthenticated(ctx) {
		return true
	}

	query, operationName := graphqlExtractOperation(ctx)
	if query == "" {
		return false
	}

	document, err := parser.ParseQuery(&ast.Source{Input: query})
	if err != nil {
		logger.Warn("failed to parse anonymous graphql request",
			zap.String("path", graphqlPath(ctx)),
			zap.Error(err))
		return false
	}

	operation := graphqlSelectOperation(document, operationName)
	if operation == nil || operation.Operation != ast.Query {
		return false
	}

	fields, ok := graphqlCollectTopLevelFields(document, operation.SelectionSet, map[string]struct{}{})
	if !ok || len(fields) == 0 {
		return false
	}

	for _, fieldName := range fields {
		if _, allowed := anonymousGraphQLPublicQueryFields[fieldName]; !allowed {
			return false
		}
	}

	return true
}

func graphqlExtractOperation(ctx *apptheory.Context) (query string, operationName string) {
	parts := graphqlExtractRequestParts(ctx)

	switch strings.ToUpper(strings.TrimSpace(parts.method)) {
	case http.MethodGet:
		query = graphqlFirstRequestValue(parts.query, "query")
		return strings.TrimSpace(query), strings.TrimSpace(graphqlFirstRequestValue(parts.query, "operationName"))
	case http.MethodPost:
		var request graphqlOperationRequest
		if err := json.Unmarshal(parts.body, &request); err != nil {
			return "", ""
		}
		return strings.TrimSpace(request.Query), strings.TrimSpace(request.OperationName)
	default:
		return "", ""
	}
}

func graphqlFirstRequestValue(values map[string][]string, key string) string {
	if len(values) == 0 {
		return ""
	}

	items := values[key]
	if len(items) == 0 {
		return ""
	}

	return items[0]
}

func graphqlSelectOperation(document *ast.QueryDocument, operationName string) *ast.OperationDefinition {
	if document == nil {
		return nil
	}

	if operationName != "" {
		for _, operation := range document.Operations {
			if operation != nil && strings.TrimSpace(operation.Name) == operationName {
				return operation
			}
		}
		return nil
	}

	if len(document.Operations) != 1 {
		return nil
	}

	return document.Operations[0]
}

func graphqlCollectTopLevelFields(document *ast.QueryDocument, selectionSet ast.SelectionSet, visitedFragments map[string]struct{}) ([]string, bool) {
	if len(selectionSet) == 0 {
		return nil, false
	}

	fields := make([]string, 0, len(selectionSet))
	for _, selection := range selectionSet {
		switch current := selection.(type) {
		case *ast.Field:
			name := strings.TrimSpace(current.Name)
			if name == "" {
				return nil, false
			}
			fields = append(fields, name)
		case *ast.FragmentSpread:
			name := strings.TrimSpace(current.Name)
			if name == "" {
				return nil, false
			}
			if _, seen := visitedFragments[name]; seen {
				continue
			}
			visitedFragments[name] = struct{}{}

			fragment := graphqlFindFragment(document, name)
			if fragment == nil {
				return nil, false
			}

			fragmentFields, ok := graphqlCollectTopLevelFields(document, fragment.SelectionSet, visitedFragments)
			if !ok {
				return nil, false
			}
			fields = append(fields, fragmentFields...)
		case *ast.InlineFragment:
			inlineFields, ok := graphqlCollectTopLevelFields(document, current.SelectionSet, visitedFragments)
			if !ok {
				return nil, false
			}
			fields = append(fields, inlineFields...)
		default:
			return nil, false
		}
	}

	return fields, true
}

func graphqlFindFragment(document *ast.QueryDocument, name string) *ast.FragmentDefinition {
	if document == nil {
		return nil
	}

	for _, fragment := range document.Fragments {
		if fragment != nil && strings.TrimSpace(fragment.Name) == name {
			return fragment
		}
	}

	return nil
}

// handleGraphQL processes GraphQL requests with proper context and DataLoader
func handleGraphQL(ctx *apptheory.Context) (*apptheory.Response, error) {
	if !graphqlAnonymousRequestAllowed(ctx) {
		return graphqlErrorResponse(http.StatusUnauthorized, "authentication required"), nil
	}

	requestCtx, cancel := graphqlWithTimeout(ctx)
	defer cancel()

	requestCtx = graphqlWithUser(requestCtx, ctx)

	authHeader := graphqlExtractAuthHeader(ctx)
	logger.Info("GraphQL request auth header check",
		zap.String("path", graphqlPath(ctx)),
		zap.Bool("has_header", authHeader != ""),
	)

	requestCtx = graphqlWithClaims(requestCtx, ctx)
	requestCtx = graphqlWithCostTracker(requestCtx, ctx)
	requestCtx = graphqlWithLoaders(requestCtx, ctx)

	// Create HTTP request wrapper for GraphQL handler.
	parts := graphqlExtractRequestParts(ctx)
	u := graphqlBuildURL(parts.path, parts.query)

	httpReq := &http.Request{
		Method: parts.method,
		URL:    u,
		Header: make(http.Header),
		Body:   &bytesReader{data: parts.body},
	}
	httpReq = httpReq.WithContext(requestCtx)
	graphqlCopyHeaders(httpReq.Header, parts.headers)

	// Process GraphQL request.
	responseWriter := newGraphQLResponseWriter()
	handlerStart := time.Now()
	path := parts.path
	method := parts.method
	logger.Info("GraphQL handler invocation started",
		zap.String("path", path),
		zap.String("method", method))
	graphQLHandler.ServeHTTP(responseWriter, httpReq)
	graphqlLogQuoteLoaderMetrics(requestCtx)
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

// createAuthMiddleware creates authentication middleware using unified patterns.
//
//nolint:unused // Exercised directly in tests; buildApp uses createAuthMiddlewareWithServices for paired auth services.
func createAuthMiddleware() apptheory.Middleware {
	return createAuthMiddlewareWithServices(newGraphQLAuthService(logger), newGraphQLOAuthService(logger), logger)
}

//nolint:unused // Exercised directly in tests; buildApp uses createAuthMiddlewareWithServices for paired auth services.
func createAuthMiddlewareWithService(oauthService *auth.OAuthService, serviceLogger *zap.Logger) apptheory.Middleware {
	return createAuthMiddlewareWithServices(nil, oauthService, serviceLogger)
}

func createAuthMiddlewareWithServices(authService *auth.AuthService, oauthService *auth.OAuthService, serviceLogger *zap.Logger) apptheory.Middleware {
	if authService != nil && oauthService != nil {
		return auth.CreatePrincipalContextBridgeFromAuthAndOAuthServices(authService, oauthService, serviceLogger, "graphql")
	}
	if oauthService == nil {
		return func(next apptheory.Handler) apptheory.Handler { return next }
	}

	return auth.CreatePrincipalContextBridgeFromOAuthService(oauthService, serviceLogger, "graphql")
}

func newGraphQLAuthService(serviceLogger *zap.Logger) *auth.AuthService {
	if cfg == nil || repos == nil {
		return nil
	}

	authService, err := auth.NewAuthService(cfg, repos)
	if err != nil {
		if serviceLogger != nil {
			serviceLogger.Warn("failed to initialize GraphQL native auth service", zap.Error(err))
		}
		return nil
	}
	return authService
}

func newGraphQLOAuthService(serviceLogger *zap.Logger) *auth.OAuthService {
	if cfg == nil {
		if serviceLogger != nil {
			serviceLogger.Fatal("JWT secret is not configured; cannot initialize GraphQL auth middleware")
		}
		return nil
	}
	if common.RunningUnitTests() && strings.TrimSpace(cfg.JWTSecret) == "" && strings.TrimSpace(cfg.JWTSecretARN) == "" {
		return nil
	}
	jwtSecret, err := cfg.ResolveJWTSecret()
	if err != nil || jwtSecret == "" {
		if serviceLogger != nil {
			serviceLogger.Fatal("JWT secret is not configured; cannot initialize GraphQL auth middleware", zap.Error(err))
		}
		return nil
	}

	auditLogger := auth.NewAuditLogger(repos, serviceLogger, auth.DefaultAuditConfig())
	return auth.NewOAuthService(jwtSecret, cfg, repos, auditLogger)
}

func resolveJWTSecretForGraphQL(serviceLogger *zap.Logger) string {
	if cfg == nil {
		return ""
	}
	jwtSecret, err := cfg.ResolveJWTSecret()
	if err != nil {
		if serviceLogger != nil {
			serviceLogger.Warn("failed to resolve GraphQL JWT secret", zap.Error(err))
		}
		return ""
	}
	return jwtSecret
}

func main() {
	app := buildApp(logger)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func buildApp(lambdaLogger *zap.Logger) *apptheory.App {
	authService := newGraphQLAuthService(lambdaLogger)
	oauthService := newGraphQLOAuthService(lambdaLogger)

	options := []apptheory.Option{
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
	}
	if authService != nil && oauthService != nil {
		options = append(options, apptheory.WithAuthPrincipalHook(
			auth.NewAppTheoryPrincipalHookFromAuthAndOAuthServices(authService, oauthService, lambdaLogger, "graphql"),
		))
	} else if oauthService != nil {
		options = append(options, apptheory.WithAuthPrincipalHook(
			auth.NewAppTheoryPrincipalHookFromOAuthService(oauthService, lambdaLogger, "graphql"),
		))
	}
	app := apptheory.New(options...)

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
	app.Use(createAuthMiddlewareWithServices(authService, oauthService, lambdaLogger))

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
