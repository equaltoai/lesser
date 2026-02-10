// Package main implements the streaming Lambda function for handling WebSocket connections and real-time streaming.
package main

/*
Streaming Service - WebSocket Handler

This Lambda function handles WebSocket connections for real-time streaming.
It manages WebSocket connection lifecycle and stream subscriptions.

Migrated to use standardized Lambda initialization and Lift framework.
*/

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/theory-cloud/apptheory/pkg/streamer"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/streaming/handlers"
)

// StreamMessage represents a message sent over WebSocket
type StreamMessage struct {
	Type    string         `json:"type"`
	Stream  string         `json:"stream,omitempty"`
	Event   string         `json:"event,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

// StreamEvent represents an event to be streamed
type StreamEvent struct {
	Event   string          `json:"event"` // "update", "delete", "notification", "status.update"
	Payload json.RawMessage `json:"payload"`
	Stream  []string        `json:"stream"` // streams this event should go to
}

type streamingConnectionRepository interface {
	WriteConnection(ctx context.Context, connectionID, userID, username string, streams []string) (*models.WebSocketConnection, error)
	DeleteConnection(ctx context.Context, connectionID string) error
	DeleteAllSubscriptions(ctx context.Context, connectionID string) error
	GetConnection(ctx context.Context, connectionID string) (*models.WebSocketConnection, error)
	UpdateConnection(ctx context.Context, connection *models.WebSocketConnection) error
	WriteSubscription(ctx context.Context, connectionID, userID, stream string) error
	DeleteSubscription(ctx context.Context, connectionID, stream string) error
}

type websocketCostTracker interface {
	TrackWebSocketOperation(ctx context.Context, opCtx *repositories.WebSocketOperationContext, result *repositories.WebSocketOperationResult) error
}

type streamingCommandRouter interface {
	RegisterHandler(handler streaming.CommandHandler)
	HandleCommand(ctx context.Context, conn *streaming.ConnectionInfo, cmd *streaming.Command) (*streaming.CommandResponse, error)
}

// StreamingHandler handles WebSocket streaming connections using DynamORM and Lift
type StreamingHandler struct {
	userRepo        *repositories.UserRepository
	connectionRepo  streamingConnectionRepository
	costTracker     websocketCostTracker
	logger          *zap.Logger
	cfg             *config.Config
	awsConfig       aws.Config
	wsClient        streamer.Client
	commandRouter   streamingCommandRouter
	serviceRegistry *services.Registry
	storageFactory  core.RepositoryStorage
}

// Global variables for standardized Lambda initialization
var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
	handler   *StreamingHandler
)

var (
	runningUnitTestsFn = common.RunningUnitTests

	mustInitializeLambdaFn    = common.MustInitializeLambda
	initializeWithDefaultsFn  = func(ctx *common.LambdaContext) error { return ctx.InitializeWithDefaults() }
	ensureRepositoryFactoryFn = ensureRepositoryFactory
	resolveDynamoClientFn     = resolveDynamoClient
	newUserRepositoryFn       = repositories.NewUserRepository
	newMockPublisherFn        = streaming.NewMockPublisher
	newCommandRouterFn        = func(logger *zap.Logger) streamingCommandRouter { return streaming.NewCommandRouter(logger) }
	registerCommandHandlersFn = registerCommandHandlers
	newStatusCommandHandlerFn = func(registry *services.Registry, logger *zap.Logger) streaming.CommandHandler {
		return handlers.NewStatusCommandHandlerV2(registry.Notes(), logger)
	}
	newAccountCommandHandlerFn = func(registry *services.Registry, logger *zap.Logger) streaming.CommandHandler {
		return handlers.NewAccountCommandHandler(registry.Accounts(), logger)
	}
	newRelationshipCommandHandlerFn = func(registry *services.Registry, logger *zap.Logger) streaming.CommandHandler {
		return handlers.NewRelationshipCommandHandler(registry.Relationships(), registry.Accounts(), logger)
	}
	newSystemCommandHandlerFn = func(registry *services.Registry, logger *zap.Logger) streaming.CommandHandler {
		return handlers.NewSystemCommandHandler(
			registry.Notes(),
			registry.Lists(),
			registry.Media(),
			registry.Notifications(),
			logger,
		)
	}
	newStreamerClientFn          = streamer.NewClient
	runAsyncFn                   = func(fn func()) { go fn() }
	lambdaStartFn                = lambda.Start
	newStreamingConnectionRepoFn = func(db dynamormCore.DB, connectionsTable string, subscriptionDB dynamormCore.DB, subscriptionsTable string, logger *zap.Logger) streamingConnectionRepository {
		return repositories.NewStreamingConnectionRepository(db, connectionsTable, subscriptionDB, subscriptionsTable, logger, nil)
	}
	newWebSocketCostTrackerFn = func(db dynamormCore.DB, tableName string, logger *zap.Logger) websocketCostTracker {
		costRepo := repositories.NewWebSocketCostRepository(db, tableName, logger, nil)
		return repositories.NewWebSocketCostTracker(costRepo, logger)
	}
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	newRepositoryFactoryFn     = factory.NewRepositoryFactory
	newServiceRegistryFn       = func(repos core.RepositoryStorage, publisher streaming.Publisher, logger *zap.Logger, serviceConfig *services.ServiceConfig) (*services.Registry, error) {
		return services.NewRegistry(
			services.WithStorage(repos),
			services.WithPublisher(publisher),
			services.WithLogger(logger),
			services.WithConfig(serviceConfig),
		)
	}
	authorizeListStreamSubscriptionFn = authorizeListStreamSubscription
)

func init() {
	initializeStreamingOnStart()
}

func initializeStreamingOnStart() {
	if runningUnitTestsFn() {
		return
	}
	if err := initializeStreaming(); err != nil {
		if logger == nil {
			logger = zap.NewNop()
		}
		logger.Fatal("failed to initialize streaming lambda", zap.Error(err))
	}
}

func initializeStreaming() error {
	// Standardized Lambda initialization for streaming API
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "streaming",
		LambdaType:  common.LambdaTypeAPI, // WebSocket/HTTP streaming endpoints
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Initialize with API-specific defaults
	if err := initializeWithDefaultsFn(lambdaCtx); err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	if err := ensureRepositoryFactoryFn(); err != nil {
		return err
	}

	db := resolveDynamoClientFn()
	if db == nil {
		return fmt.Errorf("streaming lambda missing dynamo client after initialization")
	}

	tableName := strings.TrimSpace(cfg.DynamoTableName)
	if tableName == "" {
		return fmt.Errorf("DYNAMODB_TABLE environment variable is required for streaming lambda")
	}

	connectionsTable := tableName
	if override := strings.TrimSpace(cfg.ConnectionsTable); override != "" {
		connectionsTable = override
	}

	subscriptionsTable := tableName
	if override := strings.TrimSpace(cfg.SubscriptionsTable); override != "" {
		subscriptionsTable = override
	}

	userRepo := newUserRepositoryFn(db, tableName, logger)
	connectionRepo := newStreamingConnectionRepoFn(db, connectionsTable, db, subscriptionsTable, logger)

	costTracker := newWebSocketCostTrackerFn(db, tableName, logger)

	publisher := newMockPublisherFn() // Use mock publisher for Lambda

	serviceConfig := &services.ServiceConfig{
		BaseURL:   cfg.Domain,
		JWTSecret: cfg.JWTSecret,
		Config:    cfg,
	}

	serviceRegistry, err := newServiceRegistryFn(repos, publisher, logger, serviceConfig)
	if err != nil {
		return fmt.Errorf("failed to create service registry: %w", err)
	}

	commandRouter := newCommandRouterFn(logger)
	registerCommandHandlersFn(commandRouter, serviceRegistry, logger)

	handler = &StreamingHandler{
		userRepo:        userRepo,
		connectionRepo:  connectionRepo,
		costTracker:     costTracker,
		logger:          logger,
		cfg:             cfg,
		awsConfig:       lambdaCtx.AWSServices.Config,
		commandRouter:   commandRouter,
		serviceRegistry: serviceRegistry,
		storageFactory:  repos,
	}

	return nil
}

func registerCommandHandlers(router streamingCommandRouter, serviceRegistry *services.Registry, logger *zap.Logger) {
	if router == nil || serviceRegistry == nil {
		return
	}

	statusHandler := newStatusCommandHandlerFn(serviceRegistry, logger)
	accountHandler := newAccountCommandHandlerFn(serviceRegistry, logger)
	relationshipHandler := newRelationshipCommandHandlerFn(serviceRegistry, logger)
	systemHandler := newSystemCommandHandlerFn(serviceRegistry, logger)

	router.RegisterHandler(statusHandler)
	router.RegisterHandler(accountHandler)
	router.RegisterHandler(relationshipHandler)
	router.RegisterHandler(systemHandler)
}

func webSocketEventFromAppTheory(ctx *apptheory.Context) (events.APIGatewayWebsocketProxyRequest, error) {
	if ctx == nil {
		return events.APIGatewayWebsocketProxyRequest{}, fmt.Errorf("nil apptheory context")
	}
	wsCtx := ctx.AsWebSocket()
	if wsCtx == nil {
		return events.APIGatewayWebsocketProxyRequest{}, fmt.Errorf("missing websocket context")
	}

	headers := make(map[string]string, len(ctx.Request.Headers))
	for key, values := range ctx.Request.Headers {
		if len(values) == 0 {
			continue
		}
		headers[key] = values[0]
	}

	query := make(map[string]string, len(ctx.Request.Query))
	for key, values := range ctx.Request.Query {
		if len(values) == 0 {
			continue
		}
		query[key] = values[0]
	}

	return events.APIGatewayWebsocketProxyRequest{
		Headers:               headers,
		QueryStringParameters: query,
		Body:                  string(wsCtx.Body),
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			ConnectionID: wsCtx.ConnectionID,
			RouteKey:     wsCtx.RouteKey,
			DomainName:   wsCtx.DomainName,
			Stage:        wsCtx.Stage,
		},
	}, nil
}

func (sh *StreamingHandler) HandleWebSocketConnect(ctx *apptheory.Context) (*apptheory.Response, error) {
	event, err := webSocketEventFromAppTheory(ctx)
	if err != nil {
		return apptheory.Text(400, "invalid websocket event"), nil
	}

	if err := sh.handleConnect(ctx.Context(), event); err != nil {
		return nil, err
	}
	return &apptheory.Response{Status: 200}, nil
}

func (sh *StreamingHandler) HandleWebSocketDisconnect(ctx *apptheory.Context) (*apptheory.Response, error) {
	event, err := webSocketEventFromAppTheory(ctx)
	if err != nil {
		return apptheory.Text(400, "invalid websocket event"), nil
	}

	if err := sh.handleDisconnect(ctx.Context(), event); err != nil {
		return nil, err
	}
	return &apptheory.Response{Status: 200}, nil
}

func (sh *StreamingHandler) HandleWebSocketDefault(ctx *apptheory.Context) (*apptheory.Response, error) {
	event, err := webSocketEventFromAppTheory(ctx)
	if err != nil {
		return apptheory.Text(400, "invalid websocket event"), nil
	}

	wsCtx := ctx.AsWebSocket()
	endpoint := strings.TrimSpace(wsCtx.ManagementEndpoint)
	if endpoint == "" {
		return apptheory.Text(500, "missing websocket management endpoint"), nil
	}

	wsClient, err := newStreamerClientFn(ctx.Context(), endpoint, streamer.WithAWSConfig(sh.awsConfig))
	if err != nil {
		return nil, err
	}
	sh.wsClient = wsClient

	if err := sh.handleMessage(ctx.Context(), event); err != nil {
		return nil, err
	}

	return &apptheory.Response{Status: 200}, nil
}

// handleConnect handles WebSocket connection events
func (sh *StreamingHandler) handleConnect(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) error {
	startTime := time.Now()
	// Extract token from query parameters or headers
	token := ""
	if event.QueryStringParameters != nil {
		token = decodeQueryToken(event.QueryStringParameters["access_token"]) // Mastodon uses access_token
	}

	if event.Headers != nil {
		authHeader := event.Headers["Authorization"]
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = decodeQueryToken(strings.TrimPrefix(authHeader, "Bearer "))
		} else {
			// Check lowercase
			authHeader = event.Headers["authorization"]
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = decodeQueryToken(strings.TrimPrefix(authHeader, "Bearer "))
			}
		}
	}

	var userID, username string
	authSuccess := false

	if err := common.ValidateRequiredParam("token", token); err == nil {
		// Validate OAuth token with auth middleware
		// Since we can't use RequireAuth (it expects APIGatewayV2HTTPRequest),
		// we'll extract the token and validate directly
		// Create minimal audit logger for OAuth service
		// Use storageFactory from the handler
		auditLogger := auth.NewAuditLogger(sh.storageFactory, sh.logger, auth.DefaultAuditConfig())
		authService := auth.NewOAuthService(sh.cfg.JWTSecret, sh.cfg, sh.storageFactory, auditLogger)
		claims, err := authService.ValidateAccessToken(token)
		if err != nil {
			sh.logger.Warn("invalid token", zap.Error(err))
			// Don't reject connection, allow anonymous access for public streams
		} else {
			userID = claims.Subject
			username = claims.Username
			authSuccess = true
			sh.logger.Info("user authenticated",
				zap.String("userID", userID),
				zap.String("username", username),
				zap.Strings("scopes", claims.Scopes))
		}
	} else {
		sh.logger.Info("anonymous connection allowed")
	}

	// Create connection record using DynamORM repository
	if _, err := sh.connectionRepo.WriteConnection(ctx, event.RequestContext.ConnectionID, userID, username, []string{}); err != nil {
		sh.logger.Error("failed to write connection", zap.Error(err))
		return errors.Join(streaming.ErrConnectionWriteFailed, err)
	}

	// IMPORTANT: Do not send any messages during $connect
	// API Gateway doesn't allow this and it causes errors
	sh.logger.Info("connection established",
		zap.String("userID", userID),
		zap.Bool("authSuccess", authSuccess),
	)

	// Track connection establishment cost
	runAsyncFn(func() {
		trackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		opCtx := &repositories.WebSocketOperationContext{
			ConnectionID:     event.RequestContext.ConnectionID,
			UserID:           userID,
			Username:         username,
			OperationType:    "connect",
			StartTime:        startTime,
			RequestID:        fmt.Sprintf("connect-%s", event.RequestContext.ConnectionID),
			ConnectionSource: "web", // Default, could be enhanced
			AuthMethod:       getAuthMethodFromEvent(event, token),
		}

		result := &repositories.WebSocketOperationResult{
			Success:          true,
			ProcessingTimeMs: time.Since(startTime).Milliseconds(),
		}

		if err := sh.costTracker.TrackWebSocketOperation(trackCtx, opCtx, result); err != nil {
			sh.logger.Error("failed to track connection cost",
				zap.String("connection_id", event.RequestContext.ConnectionID),
				zap.Error(err))
		}
	})

	return nil
}

func decodeQueryToken(raw string) string {
	token := strings.TrimSpace(raw)
	if token == "" {
		return ""
	}

	if decoded, err := url.QueryUnescape(token); err == nil {
		token = decoded
	}

	return strings.ReplaceAll(token, " ", "+")
}

// handleDisconnect handles WebSocket disconnection events
func (sh *StreamingHandler) handleDisconnect(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) error {
	logger := sh.logger.With(
		zap.String("connectionID", event.RequestContext.ConnectionID),
		zap.String("operation", "disconnect"),
	)

	// Delete connection using DynamORM repository
	if err := sh.connectionRepo.DeleteConnection(ctx, event.RequestContext.ConnectionID); err != nil {
		logger.Error("failed to delete connection", zap.Error(err))
		return errors.Join(streaming.ErrConnectionDeleteFailed, err)
	}

	// Clean up any subscriptions
	if err := sh.connectionRepo.DeleteAllSubscriptions(ctx, event.RequestContext.ConnectionID); err != nil {
		logger.Error("failed to delete subscriptions", zap.Error(err))
		// Don't fail the disconnect - this is cleanup
	}

	logger.Info("connection closed")
	return nil
}

// handleMessage handles WebSocket messages
func (sh *StreamingHandler) handleMessage(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) error {
	startTime := time.Now()
	logger := sh.logger.With(
		zap.String("connectionID", event.RequestContext.ConnectionID),
		zap.String("operation", "message"),
	)

	// Parse incoming message
	var message StreamMessage
	if err := common.ParseRequestBody([]byte(event.Body), &message); err != nil {
		logger.Error("failed to parse message", zap.Error(err))
		return sh.sendError(event.RequestContext.ConnectionID, pkgErrors.StreamingInvalidMessageFormat().Error())
	}

	// Get connection details using DynamORM repository
	connection, err := sh.connectionRepo.GetConnection(ctx, event.RequestContext.ConnectionID)
	if err != nil {
		logger.Error("failed to get connection", zap.Error(err))
		return sh.sendError(event.RequestContext.ConnectionID, pkgErrors.StreamingConnectionNotFound().Error())
	}

	// Update last activity
	connection.LastActivity = time.Now()
	if err := sh.connectionRepo.UpdateConnection(ctx, connection); err != nil {
		logger.Error("failed to update connection", zap.Error(err))
	}

	// Handle different message types
	switch message.Type {
	case "subscribe":
		err = sh.handleSubscribe(ctx, connection, message)
	case "unsubscribe":
		err = sh.handleUnsubscribe(ctx, connection, message)
	case "ping":
		err = sh.handlePing(event.RequestContext.ConnectionID)
	case "command":
		err = sh.handleCommand(ctx, connection, message)
	default:
		logger.Warn("unknown message type", zap.String("type", message.Type))
		err = sh.sendError(event.RequestContext.ConnectionID, pkgErrors.StreamingUnknownMessageType().Error())
	}

	// Track message processing cost
	runAsyncFn(func() {
		trackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		opCtx := &repositories.WebSocketOperationContext{
			ConnectionID:  event.RequestContext.ConnectionID,
			UserID:        connection.UserID,
			Username:      connection.Username,
			OperationType: "message_in",
			StartTime:     startTime,
			RequestID:     fmt.Sprintf("message-%s-%d", event.RequestContext.ConnectionID, time.Now().UnixNano()),
			ActiveStreams: connection.Streams,
		}

		result := &repositories.WebSocketOperationResult{
			Success:          err == nil,
			ProcessingTimeMs: time.Since(startTime).Milliseconds(),
			MessageCount:     1,
			MessageSizeBytes: int64(len(event.Body)),
			Error:            err,
		}

		if trackErr := sh.costTracker.TrackWebSocketOperation(trackCtx, opCtx, result); trackErr != nil {
			sh.logger.Error("failed to track message cost",
				zap.String("connection_id", event.RequestContext.ConnectionID),
				zap.Error(trackErr))
		}
	})

	return err
}

func (sh *StreamingHandler) handleSubscribe(ctx context.Context, connection *models.WebSocketConnection, message StreamMessage) error {
	requestedStream := strings.TrimSpace(message.Stream)
	canonicalStream, appErr := sh.resolveCanonicalStreamSubscription(ctx, connection, requestedStream)
	if appErr != nil {
		sh.logger.Info("stream subscription rejected",
			zap.String("connectionID", connection.ConnectionID),
			zap.String("requested_stream", requestedStream),
			zap.String("user_id", connection.UserID),
			zap.String("username", connection.Username),
			zap.String("reason", appErr.Message),
		)
		return sh.sendError(connection.ConnectionID, appErr.Error())
	}

	logger := sh.logger.With(
		zap.String("connectionID", connection.ConnectionID),
		zap.String("operation", "subscribe"),
		zap.String("stream", canonicalStream),
		zap.String("requested_stream", requestedStream),
	)

	// Add stream to connection's subscriptions
	if !contains(connection.Streams, canonicalStream) {
		connection.Streams = append(connection.Streams, canonicalStream)
		if err := sh.connectionRepo.UpdateConnection(ctx, connection); err != nil {
			logger.Error("failed to update connection streams", zap.Error(err))
			return sh.sendError(connection.ConnectionID, pkgErrors.StreamingFailedToSubscribe().Error())
		}
	}

	// Store subscription using DynamORM repository
	if err := sh.connectionRepo.WriteSubscription(ctx, connection.ConnectionID, connection.UserID, canonicalStream); err != nil {
		logger.Error("failed to write subscription", zap.Error(err))
		return sh.sendError(connection.ConnectionID, pkgErrors.StreamingFailedToSubscribe().Error())
	}

	// Send confirmation
	confirmMsg := StreamMessage{
		Type:   "subscribed",
		Stream: requestedStream,
		Payload: map[string]any{
			"timestamp":        time.Now().UTC().Format(time.RFC3339),
			"canonical_stream": canonicalStream,
		},
	}

	if err := sh.sendMessageToConnection(connection.ConnectionID, confirmMsg); err != nil {
		logger.Error("failed to send confirmation", zap.Error(err))
		return errors.Join(streaming.ErrConfirmationSendFailed, err)
	}

	logger.Info("subscribed to stream")
	return nil
}

func (sh *StreamingHandler) handleUnsubscribe(ctx context.Context, connection *models.WebSocketConnection, message StreamMessage) error {
	requestedStream := strings.TrimSpace(message.Stream)
	canonicalStream := canonicalizeStreamAlias(connection, requestedStream)

	logger := sh.logger.With(
		zap.String("connectionID", connection.ConnectionID),
		zap.String("operation", "unsubscribe"),
		zap.String("stream", canonicalStream),
		zap.String("requested_stream", requestedStream),
	)

	streamsToRemove := map[string]struct{}{}
	if requestedStream != "" {
		streamsToRemove[requestedStream] = struct{}{}
	}
	if canonicalStream != "" {
		streamsToRemove[canonicalStream] = struct{}{}
	}

	// Remove stream from connection's subscriptions
	newStreams := []string{}
	for _, s := range connection.Streams {
		if _, remove := streamsToRemove[s]; !remove {
			newStreams = append(newStreams, s)
		}
	}
	connection.Streams = newStreams

	if err := sh.connectionRepo.UpdateConnection(ctx, connection); err != nil {
		logger.Error("failed to update connection streams", zap.Error(err))
		return sh.sendError(connection.ConnectionID, pkgErrors.StreamingFailedToUnsubscribe().Error())
	}

	// Remove subscription using DynamORM repository
	for streamName := range streamsToRemove {
		if err := sh.connectionRepo.DeleteSubscription(ctx, connection.ConnectionID, streamName); err != nil {
			logger.Error("failed to delete subscription", zap.Error(err))
			// Don't fail the unsubscribe
		}
	}

	// Send confirmation
	confirmMsg := StreamMessage{
		Type:   "unsubscribed",
		Stream: requestedStream,
		Payload: map[string]any{
			"timestamp":        time.Now().UTC().Format(time.RFC3339),
			"canonical_stream": canonicalStream,
		},
	}

	if err := sh.sendMessageToConnection(connection.ConnectionID, confirmMsg); err != nil {
		logger.Error("failed to send confirmation", zap.Error(err))
		return errors.Join(streaming.ErrConfirmationSendFailed, err)
	}

	logger.Info("unsubscribed from stream")
	return nil
}

func canonicalizeStreamAlias(connection *models.WebSocketConnection, requestedStream string) string {
	if connection == nil {
		return requestedStream
	}

	stream := strings.TrimSpace(requestedStream)
	if stream == "" {
		return ""
	}

	username := strings.TrimSpace(connection.Username)
	if username == "" {
		return stream
	}

	switch stream {
	case streaming.UserStream:
		return streaming.UserStreamName(username)
	case streaming.UserNotificationStream:
		return streaming.UserNotificationStreamName(username)
	case streaming.DirectStream:
		return streaming.DirectStreamName(username)
	default:
		return stream
	}
}

func authorizeListStreamSubscription(ctx context.Context, repos core.RepositoryStorage, listID, username string) error {
	if repos == nil {
		return pkgErrors.Internal("repositories not initialized")
	}

	listRepo := repos.List()
	if listRepo == nil {
		return pkgErrors.Internal("list repository not initialized")
	}

	list, err := listRepo.GetList(ctx, listID)
	if err != nil {
		return err
	}

	if list == nil || list.Username != username {
		return pkgErrors.NotFound("list")
	}

	return nil
}

//nolint:gocognit,gocyclo // Stream resolution handles many protocol variants and edge cases.
func (sh *StreamingHandler) resolveCanonicalStreamSubscription(ctx context.Context, connection *models.WebSocketConnection, requestedStream string) (string, *pkgErrors.AppError) {
	stream := strings.TrimSpace(requestedStream)
	if stream == "" {
		return "", pkgErrors.StreamingInvalidStream()
	}

	parts := strings.Split(stream, ":")
	root := parts[0]
	switch root {
	case "public":
		if stream == streaming.PublicStream || stream == streaming.PublicLocalStream || stream == streaming.PublicRemoteStream {
			return stream, nil
		}
		return "", pkgErrors.StreamingInvalidStream()
	case streaming.HashtagStreamPrefix:
		if len(parts) < 2 {
			return "", pkgErrors.StreamingInvalidStream()
		}

		tag := strings.TrimSpace(strings.Join(parts[1:], ":"))
		if tag == "" {
			return "", pkgErrors.StreamingInvalidStream()
		}
		return streaming.HashtagStreamName(tag), nil
	case streaming.UserStream:
		if err := common.ValidateRequiredParam("user_id", connection.UserID); err != nil {
			return "", pkgErrors.StreamingAuthenticationRequired()
		}
		if strings.TrimSpace(connection.Username) == "" {
			return "", pkgErrors.StreamingFailedToSubscribe()
		}

		username := strings.TrimSpace(connection.Username)
		switch len(parts) {
		case 1:
			return streaming.UserStreamName(username), nil
		case 2:
			if parts[1] == "notification" {
				return streaming.UserNotificationStreamName(username), nil
			}
			if parts[1] != username {
				return "", pkgErrors.StreamingInvalidStream()
			}
			return streaming.UserStreamName(username), nil
		case 3:
			if parts[1] != "notification" {
				return "", pkgErrors.StreamingInvalidStream()
			}
			if parts[2] != username {
				return "", pkgErrors.StreamingInvalidStream()
			}
			return streaming.UserNotificationStreamName(username), nil
		default:
			return "", pkgErrors.StreamingInvalidStream()
		}
	case streaming.DirectStream:
		if err := common.ValidateRequiredParam("user_id", connection.UserID); err != nil {
			return "", pkgErrors.StreamingAuthenticationRequired()
		}
		if strings.TrimSpace(connection.Username) == "" {
			return "", pkgErrors.StreamingFailedToSubscribe()
		}

		username := strings.TrimSpace(connection.Username)
		switch len(parts) {
		case 1:
			return streaming.DirectStreamName(username), nil
		case 2:
			if parts[1] != username {
				return "", pkgErrors.StreamingInvalidStream()
			}
			return streaming.DirectStreamName(username), nil
		default:
			return "", pkgErrors.StreamingInvalidStream()
		}
	case streaming.ListStreamPrefix:
		if err := common.ValidateRequiredParam("user_id", connection.UserID); err != nil {
			return "", pkgErrors.StreamingAuthenticationRequired()
		}
		if strings.TrimSpace(connection.Username) == "" {
			return "", pkgErrors.StreamingFailedToSubscribe()
		}
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			return "", pkgErrors.StreamingInvalidStream()
		}

		listID := strings.TrimSpace(parts[1])
		if err := authorizeListStreamSubscriptionFn(ctx, sh.storageFactory, listID, strings.TrimSpace(connection.Username)); err != nil {
			if pkgErrors.HasCode(err, pkgErrors.CodeNotFound) || pkgErrors.HasCode(err, pkgErrors.CodeForbidden) {
				return "", pkgErrors.StreamingInvalidStream()
			}
			sh.logger.Error("failed to authorize list stream subscription",
				zap.String("connection_id", connection.ConnectionID),
				zap.String("username", connection.Username),
				zap.String("list_id", listID),
				zap.Error(err))
			return "", pkgErrors.StreamingFailedToSubscribe()
		}

		return streaming.ListStreamName(listID), nil
	default:
		return "", pkgErrors.StreamingInvalidStream()
	}
}

func (sh *StreamingHandler) handlePing(connectionID string) error {
	pongMessage := StreamMessage{
		Type: "pong",
		Payload: map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := sh.sendMessageToConnection(connectionID, pongMessage); err != nil {
		sh.logger.Error("failed to send pong", zap.Error(err))
		return errors.Join(streaming.ErrPongSendFailed, err)
	}

	return nil
}

func (sh *StreamingHandler) handleCommand(ctx context.Context, connection *models.WebSocketConnection, message StreamMessage) error {
	logger := sh.logger.With(
		zap.String("connectionID", connection.ConnectionID),
		zap.String("operation", "command"),
		zap.String("command_type", sh.getCommandType(message)),
	)

	// Parse the command from the message payload
	command, err := sh.parseCommand(message)
	if err != nil {
		logger.Error("failed to parse command", zap.Error(err))
		return sh.sendError(connection.ConnectionID, pkgErrors.StreamingInvalidCommandFormat().Error())
	}

	// Create connection info for the command handler
	connInfo := &streaming.ConnectionInfo{
		ConnectionID:    connection.ConnectionID,
		UserID:          connection.UserID,
		Username:        connection.Username,
		Streams:         connection.Streams,
		IsAuthenticated: connection.UserID != "",
	}

	// Route and execute the command
	response, err := sh.commandRouter.HandleCommand(ctx, connInfo, command)
	if err != nil {
		logger.Error("command execution failed", zap.Error(err))
		return sh.sendError(connection.ConnectionID, pkgErrors.StreamingCommandExecutionFailed().Error())
	}

	// Send the response back to the client
	if response != nil {
		return sh.sendCommandResponse(connection.ConnectionID, response)
	}

	return nil
}

func (sh *StreamingHandler) parseCommand(message StreamMessage) (*streaming.Command, error) {
	// Extract command details from message payload
	var command streaming.Command

	// Get command ID (required for request/response matching)
	if id, exists := message.Payload["id"]; exists {
		if idStr, ok := id.(string); ok {
			command.ID = idStr
		} else {
			return nil, streaming.ErrCommandIDMustBeString
		}
	} else {
		return nil, streaming.ErrCommandIDRequired
	}

	// Get command type (required)
	if cmdType, exists := message.Payload["command"]; exists {
		if cmdTypeStr, ok := cmdType.(string); ok {
			command.Type = cmdTypeStr
		} else {
			return nil, streaming.ErrCommandTypeMustBeString
		}
	} else {
		return nil, streaming.ErrCommandTypeRequired
	}

	// Get command payload (optional)
	if payload, exists := message.Payload["data"]; exists {
		if payloadMap, ok := payload.(map[string]interface{}); ok {
			command.Payload = payloadMap
		} else {
			// Try to convert to map
			command.Payload = map[string]interface{}{"data": payload}
		}
	} else {
		command.Payload = make(map[string]interface{})
	}

	return &command, nil
}

func (sh *StreamingHandler) getCommandType(message StreamMessage) string {
	if cmdType, exists := message.Payload["command"]; exists {
		if cmdTypeStr, ok := cmdType.(string); ok {
			return cmdTypeStr
		}
	}
	return "unknown"
}

func (sh *StreamingHandler) sendCommandResponse(connectionID string, response *streaming.CommandResponse) error {
	// Convert response to StreamMessage format
	responseMsg := StreamMessage{
		Type: "command_response",
		Payload: map[string]any{
			"id":      response.ID,
			"type":    response.Type,
			"success": response.Success,
			"data":    response.Data,
		},
	}

	// Add error information if present
	if response.Error != nil {
		responseMsg.Payload["error"] = map[string]any{
			"code":    response.Error.Code,
			"message": response.Error.Message,
			"details": response.Error.Details,
		}
	}

	return sh.sendMessageToConnection(connectionID, responseMsg)
}

// Messaging functions

func (sh *StreamingHandler) sendMessageToConnection(connectionID string, message StreamMessage) error {
	if sh.wsClient == nil {
		return streaming.ErrAPIGatewayClientNotInit
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return sh.wsClient.PostToConnection(context.Background(), connectionID, messageBytes)
}

func (sh *StreamingHandler) sendError(connectionID string, errorMessage string) error {
	errMsg := StreamMessage{
		Type: "error",
		Payload: map[string]any{
			"error":     errorMessage,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := sh.sendMessageToConnection(connectionID, errMsg); err != nil {
		return errors.Join(streaming.ErrErrorMessageSendFailed, err)
	}

	return nil
}

// Helper functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getAuthMethodFromEvent determines the authentication method used
func getAuthMethodFromEvent(event events.APIGatewayWebsocketProxyRequest, token string) string {
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return "anonymous"
	}

	// Check query parameters
	if event.QueryStringParameters != nil {
		if event.QueryStringParameters["access_token"] != "" {
			return "oauth"
		}
	}

	// Check headers
	if event.Headers != nil {
		if auth := event.Headers["Authorization"]; auth != "" {
			if strings.HasPrefix(auth, "Bearer ") {
				return "bearer"
			}
			if strings.HasPrefix(auth, "Basic ") {
				return "basic"
			}
		}
		if auth := event.Headers["authorization"]; auth != "" {
			if strings.HasPrefix(auth, "Bearer ") {
				return "bearer"
			}
		}
	}

	return "oauth" // Default if token exists but method is unclear
}

func main() {
	app := apptheory.New()
	app.WebSocket("$connect", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		if handler == nil {
			return nil, fmt.Errorf("streaming handler not initialized")
		}
		return handler.HandleWebSocketConnect(ctx)
	})
	app.WebSocket("$disconnect", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		if handler == nil {
			return nil, fmt.Errorf("streaming handler not initialized")
		}
		return handler.HandleWebSocketDisconnect(ctx)
	})
	app.WebSocket("$default", func(ctx *apptheory.Context) (*apptheory.Response, error) {
		if handler == nil {
			return nil, fmt.Errorf("streaming handler not initialized")
		}
		return handler.HandleWebSocketDefault(ctx)
	})

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func ensureRepositoryFactory() error {
	if lambdaCtx == nil {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	if repos == nil && lambdaCtx.Repos != nil {
		if storage, ok := lambdaCtx.Repos.(core.RepositoryStorage); ok && storage != nil {
			repos = storage
		} else {
			logger.Warn("lambda context repository is not core.RepositoryStorage")
		}
	}

	if repos != nil {
		return nil
	}

	return initializeManualRepositories()
}

func resolveDynamoClient() dynamormCore.DB {
	if lambdaCtx != nil {
		if db, ok := lambdaCtx.DynamoDB.(dynamormCore.DB); ok && db != nil {
			return db
		}
	}

	if repos != nil {
		if db := repos.GetDB(); db != nil {
			if lambdaCtx != nil && lambdaCtx.DynamoDB == nil {
				lambdaCtx.DynamoDB = db
			}
			return db
		}
	}

	return nil
}

func initializeManualRepositories() error {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg == nil {
		cfg = config.Get()
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		if envRegion := strings.TrimSpace(os.Getenv("AWS_REGION")); envRegion != "" {
			region = envRegion
		} else if envDefault := strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")); envDefault != "" {
			region = envDefault
		} else {
			region = "us-east-1"
		}
		cfg.Region = region
	}

	if os.Getenv("AWS_REGION") == "" && region != "" {
		_ = os.Setenv("AWS_REGION", region)
	}
	if os.Getenv("AWS_DEFAULT_REGION") == "" && region != "" {
		_ = os.Setenv("AWS_DEFAULT_REGION", region)
	}

	tableName := strings.TrimSpace(cfg.DynamoTableName)
	if tableName == "" {
		return fmt.Errorf("DYNAMODB_TABLE environment variable is required for streaming lambda")
	}

	logger.Info("falling back to manual repository initialization for streaming lambda",
		zap.String("region", region),
		zap.String("table_name", tableName))

	client, err := newLambdaOptimizedClientFn(context.Background(), region)
	if err != nil {
		return fmt.Errorf("failed to initialize dynamo client for streaming lambda: %w", err)
	}

	repoFactory, err := newRepositoryFactoryFn(client, tableName, logger)
	if err != nil {
		return fmt.Errorf("failed to create repository factory for streaming lambda: %w", err)
	}

	repos = repoFactory
	if lambdaCtx != nil {
		lambdaCtx.Repos = repoFactory
		lambdaCtx.DynamoDB = client
	}
	return nil
}
