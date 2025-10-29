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
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
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

// StreamingHandler handles WebSocket streaming connections using DynamORM and Lift
type StreamingHandler struct {
	userRepo        *repositories.UserRepository
	connectionRepo  *repositories.StreamingConnectionRepository
	costTracker     *repositories.WebSocketCostTracker
	logger          *zap.Logger
	cfg             *config.Config
	awsConfig       aws.Config
	apiClient       *apigatewaymanagementapi.Client
	commandRouter   *streaming.CommandRouter
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

func init() {
	if common.RunningUnitTests() {
		return
	}
	// Standardized Lambda initialization for streaming API
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
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
	if err := lambdaCtx.InitializeWithDefaults(); err != nil {
		logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	ensureRepositoryFactory()

	db := resolveDynamoClient()
	if db == nil {
		logger.Fatal("streaming lambda missing dynamo client after initialization")
	}

	tableName := strings.TrimSpace(cfg.DynamoTableName)
	if tableName == "" {
		logger.Fatal("DYNAMODB_TABLE environment variable is required for streaming lambda")
	}

	connectionsTable := tableName
	if override := strings.TrimSpace(cfg.ConnectionsTable); override != "" {
		connectionsTable = override
	}

	subscriptionsTable := tableName
	if override := strings.TrimSpace(cfg.SubscriptionsTable); override != "" {
		subscriptionsTable = override
	}

	userRepo := repositories.NewUserRepository(db, tableName, logger)
	connectionRepo := repositories.NewStreamingConnectionRepository(db, connectionsTable, db, subscriptionsTable, logger, nil)

	// Initialize WebSocket cost tracking
	costRepo := repositories.NewWebSocketCostRepository(db, tableName, logger, nil)
	costTracker := repositories.NewWebSocketCostTracker(costRepo, logger)

	// Initialize service registry
	publisher := streaming.NewMockPublisher() // Use mock publisher for Lambda

	// Convert config to ServiceConfig
	serviceConfig := &services.ServiceConfig{
		BaseURL:   cfg.Domain,
		JWTSecret: cfg.JWTSecret,
		Config:    cfg,
	}

	serviceRegistry, err := services.NewRegistry(
		services.WithStorage(repos),
		services.WithPublisher(publisher),
		services.WithLogger(logger),
		services.WithConfig(serviceConfig),
	)
	if err != nil {
		logger.Fatal("failed to create service registry", zap.Error(err))
	}

	// Initialize command router and register handlers
	commandRouter := streaming.NewCommandRouter(logger)

	// Register command handlers
	statusHandler := handlers.NewStatusCommandHandlerV2(serviceRegistry.Notes(), logger)
	accountHandler := handlers.NewAccountCommandHandler(serviceRegistry.Accounts(), logger)
	relationshipHandler := handlers.NewRelationshipCommandHandler(serviceRegistry.Relationships(), serviceRegistry.Accounts(), logger)
	systemHandler := handlers.NewSystemCommandHandler(
		serviceRegistry.Notes(),
		serviceRegistry.Lists(),
		serviceRegistry.Media(),
		serviceRegistry.Notifications(),
		logger,
	)

	commandRouter.RegisterHandler(statusHandler)
	commandRouter.RegisterHandler(accountHandler)
	commandRouter.RegisterHandler(relationshipHandler)
	commandRouter.RegisterHandler(systemHandler)

	// Create handler instance
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
}

// HandleWebSocketEvent handles WebSocket events using Lift patterns (connect, disconnect, message)
func (sh *StreamingHandler) HandleWebSocketEvent(ctx *lift.Context) error {
	// Extract WebSocket event from Lift context
	if ctx.Request.RawEvent == nil {
		return lift.NewLiftError("MISSING_EVENT", "no WebSocket event in request", 400)
	}

	// Parse the raw event as WebSocket event
	var event events.APIGatewayWebsocketProxyRequest
	if wsEvent, ok := ctx.Request.RawEvent.(events.APIGatewayWebsocketProxyRequest); ok {
		event = wsEvent
	} else {
		// Try to parse from interface if it's a map
		eventBytes, err := json.Marshal(ctx.Request.RawEvent)
		if err != nil {
			return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to marshal raw event", 500)
		}

		if err := json.Unmarshal(eventBytes, &event); err != nil {
			return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse WebSocket event", 500)
		}
	}

	logger := sh.logger.With(
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("connectionID", event.RequestContext.ConnectionID),
		zap.String("routeKey", event.RequestContext.RouteKey),
	)

	// Construct the API Gateway Management Endpoint
	managementAPIEndpoint := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com/%s",
		event.RequestContext.APIID,
		sh.awsConfig.Region,
		event.RequestContext.Stage,
	)

	// Initialize API Gateway Management API client for this connection
	currentAPIClient := apigatewaymanagementapi.NewFromConfig(sh.awsConfig, func(o *apigatewaymanagementapi.Options) {
		o.BaseEndpoint = aws.String(managementAPIEndpoint)
	})

	// Store the API client for this request
	sh.apiClient = currentAPIClient

	switch event.RequestContext.RouteKey {
	case "$connect":
		return sh.handleConnect(ctx.Request.Context(), event)
	case "$disconnect":
		return sh.handleDisconnect(ctx.Request.Context(), event)
	case "$default":
		return sh.handleMessage(ctx.Request.Context(), event)
	default:
		logger.Warn("unknown route key", zap.String("routeKey", event.RequestContext.RouteKey))
		return lift.NewLiftError("UNKNOWN_ROUTE", pkgErrors.StreamingUnknownRoute().Error(), 400)
	}
}

// handleConnect handles WebSocket connection events
func (sh *StreamingHandler) handleConnect(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) error {
	startTime := time.Now()
	// Extract token from query parameters or headers
	token := ""
	if event.QueryStringParameters != nil {
		token = event.QueryStringParameters["access_token"] // Mastodon uses access_token
	}

	if event.Headers != nil {
		authHeader := event.Headers["Authorization"]
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			// Check lowercase
			authHeader = event.Headers["authorization"]
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
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
	if err := sh.connectionRepo.WriteConnection(ctx, event.RequestContext.ConnectionID, userID, username, []string{}); err != nil {
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
	go func() {
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
	}()

	return nil
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
	go func() {
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
	}()

	return err
}

func (sh *StreamingHandler) handleSubscribe(ctx context.Context, connection *models.WebSocketConnection, message StreamMessage) error {
	logger := sh.logger.With(
		zap.String("connectionID", connection.ConnectionID),
		zap.String("operation", "subscribe"),
		zap.String("stream", message.Stream),
	)

	// Validate stream name
	validStreams := []string{"public", "public:local", "public:remote", "user", "user:notification", "list", "direct", "hashtag"}
	isValid := false
	for _, valid := range validStreams {
		if message.Stream == valid || strings.HasPrefix(message.Stream, valid+":") {
			isValid = true
			break
		}
	}

	if !isValid {
		logger.Error("invalid stream name", zap.String("stream", message.Stream))
		return sh.sendError(connection.ConnectionID, pkgErrors.StreamingInvalidStream().Error())
	}

	// Check authorization for stream
	if message.Stream == "user" || strings.HasPrefix(message.Stream, "user:") ||
		message.Stream == "direct" || strings.HasPrefix(message.Stream, "list:") {
		// These streams require authentication
		if err := common.ValidateRequiredParam("user_id", connection.UserID); err != nil {
			logger.Error("authentication required for stream", zap.String("stream", message.Stream))
			return sh.sendError(connection.ConnectionID, pkgErrors.StreamingAuthenticationRequired().Error())
		}
	}

	// Add stream to connection's subscriptions
	if !contains(connection.Streams, message.Stream) {
		connection.Streams = append(connection.Streams, message.Stream)
		if err := sh.connectionRepo.UpdateConnection(ctx, connection); err != nil {
			logger.Error("failed to update connection streams", zap.Error(err))
			return sh.sendError(connection.ConnectionID, pkgErrors.StreamingFailedToSubscribe().Error())
		}
	}

	// Store subscription using DynamORM repository
	if err := sh.connectionRepo.WriteSubscription(ctx, connection.ConnectionID, connection.UserID, message.Stream); err != nil {
		logger.Error("failed to write subscription", zap.Error(err))
		return sh.sendError(connection.ConnectionID, pkgErrors.StreamingFailedToSubscribe().Error())
	}

	// Send confirmation
	confirmMsg := StreamMessage{
		Type:   "subscribed",
		Stream: message.Stream,
		Payload: map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
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
	logger := sh.logger.With(
		zap.String("connectionID", connection.ConnectionID),
		zap.String("operation", "unsubscribe"),
		zap.String("stream", message.Stream),
	)

	// Remove stream from connection's subscriptions
	newStreams := []string{}
	for _, s := range connection.Streams {
		if s != message.Stream {
			newStreams = append(newStreams, s)
		}
	}
	connection.Streams = newStreams

	if err := sh.connectionRepo.UpdateConnection(ctx, connection); err != nil {
		logger.Error("failed to update connection streams", zap.Error(err))
		return sh.sendError(connection.ConnectionID, pkgErrors.StreamingFailedToUnsubscribe().Error())
	}

	// Remove subscription using DynamORM repository
	if err := sh.connectionRepo.DeleteSubscription(ctx, connection.ConnectionID, message.Stream); err != nil {
		logger.Error("failed to delete subscription", zap.Error(err))
		// Don't fail the unsubscribe
	}

	// Send confirmation
	confirmMsg := StreamMessage{
		Type:   "unsubscribed",
		Stream: message.Stream,
		Payload: map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := sh.sendMessageToConnection(connection.ConnectionID, confirmMsg); err != nil {
		logger.Error("failed to send confirmation", zap.Error(err))
		return errors.Join(streaming.ErrConfirmationSendFailed, err)
	}

	logger.Info("unsubscribed from stream")
	return nil
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
	if sh.apiClient == nil {
		return streaming.ErrAPIGatewayClientNotInit
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	_, err = sh.apiClient.PostToConnection(context.Background(), &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: &connectionID,
		Data:         messageBytes,
	})

	return err
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

// handleWebSocketRequest is the main Lambda handler for WebSocket events
// This follows the same pattern as graphql-ws for reliable WebSocket handling
func handleWebSocketRequest(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	start := time.Now()
	routeKey := event.RequestContext.RouteKey
	connectionID := event.RequestContext.ConnectionID

	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			handler.logger.Error("panic recovered in WebSocket handler",
				zap.String("connection_id", connectionID),
				zap.String("route_key", routeKey),
				zap.Any("panic", r),
			)
		}
	}()

	// Log the incoming request
	handler.logger.Info("processing WebSocket event",
		zap.String("route_key", routeKey),
		zap.String("connection_id", connectionID),
		zap.String("stage", event.RequestContext.Stage),
		zap.String("api_id", event.RequestContext.APIID),
	)

	// Construct the API Gateway Management Endpoint
	managementAPIEndpoint := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com/%s",
		event.RequestContext.APIID,
		handler.awsConfig.Region,
		event.RequestContext.Stage,
	)

	// Initialize API Gateway Management API client for this connection
	handler.apiClient = apigatewaymanagementapi.NewFromConfig(handler.awsConfig, func(o *apigatewaymanagementapi.Options) {
		o.BaseEndpoint = aws.String(managementAPIEndpoint)
	})

	// Route based on event type
	var err error
	switch routeKey {
	case "$connect":
		err = handler.handleConnect(ctx, event)
	case "$disconnect":
		err = handler.handleDisconnect(ctx, event)
	case "$default":
		err = handler.handleMessage(ctx, event)
	default:
		handler.logger.Warn("unknown route key", zap.String("route_key", routeKey))
		err = fmt.Errorf("unknown route key: %s", routeKey)
	}

	duration := time.Since(start)

	if err != nil {
		handler.logger.Error("failed to process WebSocket event",
			zap.String("route_key", routeKey),
			zap.String("connection_id", connectionID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
		// For WebSocket, we still return 200 but log the error
		return events.APIGatewayProxyResponse{StatusCode: 200}, nil
	}

	handler.logger.Info("successfully processed WebSocket event",
		zap.String("route_key", routeKey),
		zap.String("connection_id", connectionID),
		zap.Duration("duration", duration),
	)

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func main() {
	// Start the Lambda handler directly for WebSocket events
	// WebSocket protocol events don't use Lift's HTTP routing - use direct handler like graphql-ws
	lambda.Start(handleWebSocketRequest)
}

func ensureRepositoryFactory() {
	if lambdaCtx == nil {
		return
	}

	if repos == nil && lambdaCtx.Repos != nil {
		if storage, ok := lambdaCtx.Repos.(core.RepositoryStorage); ok && storage != nil {
			repos = storage
		} else {
			logger.Warn("lambda context repository is not core.RepositoryStorage")
		}
	}

	if repos != nil {
		return
	}

	initializeManualRepositories()
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

func initializeManualRepositories() {
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
		logger.Fatal("DYNAMODB_TABLE environment variable is required for streaming lambda")
	}

	logger.Info("falling back to manual repository initialization for streaming lambda",
		zap.String("region", region),
		zap.String("table_name", tableName))

	client, err := dynamorm.NewLambdaOptimizedClient(context.Background(), region)
	if err != nil {
		logger.Fatal("failed to initialize dynamo client for streaming lambda", zap.Error(err))
	}

	repoFactory, err := factory.NewRepositoryFactory(client, tableName, logger)
	if err != nil {
		logger.Fatal("failed to create repository factory for streaming lambda", zap.Error(err))
	}

	repos = repoFactory
	lambdaCtx.Repos = repoFactory
	lambdaCtx.DynamoDB = client
}
