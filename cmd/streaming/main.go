// Package main implements the streaming Lambda function for handling WebSocket connections and real-time streaming.
package main

/*
Streaming Service - WebSocket Handler

This Lambda function handles WebSocket connections for real-time streaming.
It manages WebSocket connection lifecycle and stream subscriptions.

Migrated to use Lift framework and DynamORM patterns.
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
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
	userRepo       *repositories.UserRepository
	connectionRepo *repositories.StreamingConnectionRepository
	costTracker    *repositories.WebSocketCostTracker
	logger         *zap.Logger
	cfg            *config.Config
	apiClient      *apigatewaymanagementapi.Client
}

var (
	globalCfg aws.Config
	logger    *zap.Logger
	cfg       *config.Config
	handler   *StreamingHandler
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = config.Get()

	// Initialize AWS config
	var err error
	globalCfg, err = awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Fatal("failed to load AWS config", zap.Error(err))
	}

	// Initialize DynamORM database connection
	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
	}

	// Initialize repositories
	tableName := cfg.DynamoTableName
	if tableName == "" {
		tableName = "lesser-main"
	}

	connectionsTable := os.Getenv("CONNECTIONS_TABLE")
	if connectionsTable == "" {
		connectionsTable = "lesser-streaming-connections"
	}

	subscriptionsTable := os.Getenv("SUBSCRIPTIONS_TABLE")
	if subscriptionsTable == "" {
		subscriptionsTable = "lesser-streaming-subscriptions"
	}

	userRepo := repositories.NewUserRepository(db, tableName, logger)
	connectionRepo := repositories.NewStreamingConnectionRepository(db, connectionsTable, db, subscriptionsTable, logger)

	// Initialize WebSocket cost tracking
	costRepo := repositories.NewWebSocketCostRepository(db, tableName, logger)
	costTracker := repositories.NewWebSocketCostTracker(costRepo, logger)

	// Create handler instance
	handler = &StreamingHandler{
		userRepo:       userRepo,
		connectionRepo: connectionRepo,
		costTracker:    costTracker,
		logger:         logger,
		cfg:            cfg,
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
		globalCfg.Region,
		event.RequestContext.Stage,
	)

	// Initialize API Gateway Management API client for this connection
	currentAPIClient := apigatewaymanagementapi.NewFromConfig(globalCfg, func(o *apigatewaymanagementapi.Options) {
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
		return lift.NewLiftError("UNKNOWN_ROUTE", "unknown WebSocket route", 400)
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

	if token != "" {
		// Validate OAuth token with auth middleware
		// Since we can't use RequireAuth (it expects APIGatewayV2HTTPRequest),
		// we'll extract the token and validate directly
		authService := auth.NewOAuthService(os.Getenv("JWT_SECRET"), nil)
		claims, err := authService.ValidateAccessToken(token)
		if err != nil {
			logger.Warn("invalid token", zap.Error(err))
			// Don't reject connection, allow anonymous access for public streams
		} else {
			userID = claims.Subject
			username = claims.Username
			authSuccess = true
			logger.Info("user authenticated",
				zap.String("userID", userID),
				zap.String("username", username),
				zap.Strings("scopes", claims.Scopes))
		}
	} else {
		logger.Info("anonymous connection allowed")
	}

	// Create connection record using DynamORM repository
	if err := sh.connectionRepo.WriteConnection(ctx, event.RequestContext.ConnectionID, userID, username, []string{}); err != nil {
		logger.Error("failed to write connection", zap.Error(err))
		return fmt.Errorf("failed to write connection: %w", err)
	}

	// IMPORTANT: Do not send any messages during $connect
	// API Gateway doesn't allow this and it causes errors
	logger.Info("connection established",
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
		return fmt.Errorf("failed to delete connection: %w", err)
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
		return sh.sendError(event.RequestContext.ConnectionID, "Invalid message format")
	}

	// Get connection details using DynamORM repository
	connection, err := sh.connectionRepo.GetConnection(ctx, event.RequestContext.ConnectionID)
	if err != nil {
		logger.Error("failed to get connection", zap.Error(err))
		return sh.sendError(event.RequestContext.ConnectionID, "Connection not found")
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
	default:
		logger.Warn("unknown message type", zap.String("type", message.Type))
		err = sh.sendError(event.RequestContext.ConnectionID, "Unknown message type")
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
		return sh.sendError(connection.ConnectionID, "Invalid stream: "+message.Stream)
	}

	// Check authorization for stream
	if message.Stream == "user" || strings.HasPrefix(message.Stream, "user:") ||
		message.Stream == "direct" || strings.HasPrefix(message.Stream, "list:") {
		// These streams require authentication
		if connection.UserID == "" {
			return sh.sendError(connection.ConnectionID, "Authentication required for stream: "+message.Stream)
		}
	}

	// Add stream to connection's subscriptions
	if !contains(connection.Streams, message.Stream) {
		connection.Streams = append(connection.Streams, message.Stream)
		if err := sh.connectionRepo.UpdateConnection(ctx, connection); err != nil {
			logger.Error("failed to update connection streams", zap.Error(err))
			return sh.sendError(connection.ConnectionID, "Failed to subscribe")
		}
	}

	// Store subscription using DynamORM repository
	if err := sh.connectionRepo.WriteSubscription(ctx, connection.ConnectionID, connection.UserID, message.Stream); err != nil {
		logger.Error("failed to write subscription", zap.Error(err))
		return sh.sendError(connection.ConnectionID, "Failed to subscribe")
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
		return fmt.Errorf("failed to send confirmation: %w", err)
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
		return sh.sendError(connection.ConnectionID, "Failed to unsubscribe")
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
		return fmt.Errorf("failed to send confirmation: %w", err)
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
		logger.Error("failed to send pong", zap.Error(err))
		return fmt.Errorf("failed to send pong: %w", err)
	}

	return nil
}

// Messaging functions

func (sh *StreamingHandler) sendMessageToConnection(connectionID string, message StreamMessage) error {
	if sh.apiClient == nil {
		return fmt.Errorf("API Gateway client not initialized")
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
		return fmt.Errorf("failed to send error message: %w", err)
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
	if token == "" {
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
	// Create a new Lift application
	app := lift.New()

	// Add request ID middleware (first - generates request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("streaming-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware (second - logs with request ID)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.Get("requestID")

			logger.Info("processing WebSocket event",
				zap.Any("request_id", requestID),
			)

			err := next.Handle(ctx)
			duration := time.Since(start)

			if err != nil {
				logger.Error("failed to process WebSocket event",
					zap.Any("request_id", requestID),
					zap.Error(err),
					zap.Duration("duration", duration),
				)
			} else {
				logger.Info("successfully processed WebSocket event",
					zap.Any("request_id", requestID),
					zap.Duration("duration", duration),
				)
			}

			return err
		})
	})

	// Add recovery middleware (third - catches panics)
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					requestID := ctx.Get("requestID")
					logger.Error("panic recovered in WebSocket handler",
						zap.Any("request_id", requestID),
						zap.Any("panic", r),
					)
					// For WebSocket events, we can't return an HTTP response
					// The panic will be logged and the connection will be terminated
				}
			}()
			return next.Handle(ctx)
		})
	})

	// Add WebSocket cost tracking middleware (fourth - tracks costs)
	app.Use(repositories.WebSocketCostMiddleware(handler.costTracker))

	// Set up WebSocket event handler
	app.Use(func(_ lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// This is our main WebSocket handler
			return handler.HandleWebSocketEvent(ctx)
		})
	})

	// Start the Lambda handler with Lift
	lambda.Start(app.HandleRequest)
}
