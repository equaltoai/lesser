package main

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
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// StreamConnection represents a WebSocket connection with user context
type StreamConnection struct {
	PK           string    `json:"PK" dynamodbav:"PK"`
	SK           string    `json:"SK" dynamodbav:"SK"`
	ConnectionID string    `json:"ConnectionId" dynamodbav:"ConnectionId"`
	UserID       string    `json:"UserId" dynamodbav:"UserId"`
	Username     string    `json:"Username" dynamodbav:"Username"`
	Streams      []string  `json:"Streams" dynamodbav:"Streams"` // subscribed streams
	Established  time.Time `json:"Established" dynamodbav:"Established"`
	LastActivity time.Time `json:"LastActivity" dynamodbav:"LastActivity"`
	TTL          int64     `json:"TTL" dynamodbav:"TTL"`
}

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

// StreamingHandler handles WebSocket streaming connections using DynamORM
type StreamingHandler struct {
	userRepo           *repositories.UserRepository
	logger             *zap.Logger
	connectionsTable   string
	subscriptionsTable string
	domain             string
	dynamoClient       *dynamodb.Client
	apiClient          *apigatewaymanagementapi.Client
}

var (
	globalCfg aws.Config
	log       *zap.Logger
)

func init() {
	// Initialize logger
	log = common.Logger()

	// Initialize AWS config
	var err error
	globalCfg, err = config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal("failed to load AWS config", zap.Error(err))
	}
}

// NewStreamingHandler creates a new streaming handler with DynamORM
func NewStreamingHandler() (*StreamingHandler, error) {
	// Initialize DynamORM database connection
	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM: %w", err)
	}

	// Initialize repositories
	logger := common.Logger()
	tableName := "lesser-main"
	userRepo := repositories.NewUserRepository(db, tableName, logger)

	// Get environment variables
	connectionsTable := os.Getenv("CONNECTIONS_TABLE")
	if connectionsTable == "" {
		connectionsTable = "lesser-streaming-connections"
	}

	subscriptionsTable := os.Getenv("SUBSCRIPTIONS_TABLE")
	if subscriptionsTable == "" {
		subscriptionsTable = "lesser-streaming-subscriptions"
	}

	domain := os.Getenv("DOMAIN")
	if domain == "" {
		return nil, fmt.Errorf("DOMAIN environment variable not set")
	}

	// Create DynamoDB client for connection tracking
	dynamoClient := dynamodb.NewFromConfig(globalCfg)

	return &StreamingHandler{
		userRepo:           userRepo,
		logger:             log,
		connectionsTable:   connectionsTable,
		subscriptionsTable: subscriptionsTable,
		domain:             domain,
		dynamoClient:       dynamoClient,
	}, nil
}

// HandleWebSocketEvent handles WebSocket events (connect, disconnect, message)
func (sh *StreamingHandler) HandleWebSocketEvent(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	logger := sh.logger.With(
		zap.String("connectionID", event.RequestContext.ConnectionID),
		zap.String("routeKey", event.RequestContext.RouteKey),
	)

	// Construct the API Gateway Management Endpoint
	managementApiEndpoint := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com/%s",
		event.RequestContext.APIID,
		globalCfg.Region,
		event.RequestContext.Stage,
	)

	// Initialize API Gateway Management API client for this connection
	currentApiClient := apigatewaymanagementapi.NewFromConfig(globalCfg, func(o *apigatewaymanagementapi.Options) {
		o.BaseEndpoint = aws.String(managementApiEndpoint)
	})

	// Store the API client for this request
	sh.apiClient = currentApiClient

	switch event.RequestContext.RouteKey {
	case "$connect":
		return sh.handleConnect(ctx, event)
	case "$disconnect":
		return sh.handleDisconnect(ctx, event)
	case "$default":
		return sh.handleMessage(ctx, event)
	default:
		logger.Warn("unknown route key", zap.String("routeKey", event.RequestContext.RouteKey))
		return events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}
}

// handleConnect handles WebSocket connection events
func (sh *StreamingHandler) handleConnect(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
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
			log.Warn("invalid token", zap.Error(err))
			// Don't reject connection, allow anonymous access for public streams
		} else {
			userID = claims.Subject
			username = claims.Username
			authSuccess = true
			log.Info("user authenticated",
				zap.String("userID", userID),
				zap.String("username", username),
				zap.Strings("scopes", claims.Scopes))
		}
	} else {
		log.Info("anonymous connection allowed")
	}

	// Create connection record
	connection := &StreamConnection{
		PK:           "CONN#" + event.RequestContext.ConnectionID,
		SK:           "CONN#" + event.RequestContext.ConnectionID,
		ConnectionID: event.RequestContext.ConnectionID,
		UserID:       userID,
		Username:     username,
		Streams:      []string{}, // No streams subscribed initially
		Established:  time.Now(),
		LastActivity: time.Now(),
		TTL:          time.Now().Add(24 * time.Hour).Unix(),
	}

	if err := sh.writeConnection(ctx, connection); err != nil {
		log.Error("failed to write connection", zap.Error(err))
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	// IMPORTANT: Do not send any messages during $connect
	// API Gateway doesn't allow this and it causes errors
	log.Info("connection established",
		zap.String("userID", userID),
		zap.Bool("authSuccess", authSuccess),
	)

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

// handleDisconnect handles WebSocket disconnection events
func (sh *StreamingHandler) handleDisconnect(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	logger := sh.logger.With(
		zap.String("connectionID", event.RequestContext.ConnectionID),
		zap.String("operation", "disconnect"),
	)

	// Delete connection from DynamoDB
	if err := sh.deleteConnection(ctx, event.RequestContext.ConnectionID); err != nil {
		logger.Error("failed to delete connection", zap.Error(err))
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	// Clean up any subscriptions
	if err := sh.deleteSubscriptions(ctx, event.RequestContext.ConnectionID); err != nil {
		logger.Error("failed to delete subscriptions", zap.Error(err))
		// Don't fail the disconnect
	}

	logger.Info("connection closed")
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

// handleMessage handles WebSocket messages
func (sh *StreamingHandler) handleMessage(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
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

	// Get connection details
	connection, err := sh.getConnection(ctx, event.RequestContext.ConnectionID)
	if err != nil {
		logger.Error("failed to get connection", zap.Error(err))
		return sh.sendError(event.RequestContext.ConnectionID, "Connection not found")
	}

	// Update last activity
	connection.LastActivity = time.Now()
	if err := sh.writeConnection(ctx, connection); err != nil {
		logger.Error("failed to update connection", zap.Error(err))
	}

	// Handle different message types
	switch message.Type {
	case "subscribe":
		return sh.handleSubscribe(ctx, connection, message)
	case "unsubscribe":
		return sh.handleUnsubscribe(ctx, connection, message)
	case "ping":
		return sh.handlePing(event.RequestContext.ConnectionID)
	default:
		logger.Warn("unknown message type", zap.String("type", message.Type))
		return sh.sendError(event.RequestContext.ConnectionID, "Unknown message type")
	}
}

func (sh *StreamingHandler) handleSubscribe(ctx context.Context, connection *StreamConnection, message StreamMessage) (events.APIGatewayProxyResponse, error) {
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
		if err := sh.writeConnection(ctx, connection); err != nil {
			logger.Error("failed to update connection streams", zap.Error(err))
			return sh.sendError(connection.ConnectionID, "Failed to subscribe")
		}
	}

	// Store subscription in subscriptions table for efficient lookup
	if err := sh.writeSubscription(ctx, connection.ConnectionID, connection.UserID, message.Stream); err != nil {
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
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	logger.Info("subscribed to stream")
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func (sh *StreamingHandler) handleUnsubscribe(ctx context.Context, connection *StreamConnection, message StreamMessage) (events.APIGatewayProxyResponse, error) {
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

	if err := sh.writeConnection(ctx, connection); err != nil {
		logger.Error("failed to update connection streams", zap.Error(err))
		return sh.sendError(connection.ConnectionID, "Failed to unsubscribe")
	}

	// Remove subscription from subscriptions table
	if err := sh.deleteSubscription(ctx, connection.ConnectionID, message.Stream); err != nil {
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
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	logger.Info("unsubscribed from stream")
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func (sh *StreamingHandler) handlePing(connectionID string) (events.APIGatewayProxyResponse, error) {
	pongMessage := StreamMessage{
		Type: "pong",
		Payload: map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := sh.sendMessageToConnection(connectionID, pongMessage); err != nil {
		log.Error("failed to send pong", zap.Error(err))
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

// Database operations

func (sh *StreamingHandler) writeConnection(ctx context.Context, connection *StreamConnection) error {
	item, err := attributevalue.MarshalMap(connection)
	if err != nil {
		return err
	}

	_, err = sh.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &sh.connectionsTable,
		Item:      item,
	})
	return err
}

func (sh *StreamingHandler) getConnection(ctx context.Context, connectionID string) (*StreamConnection, error) {
	result, err := sh.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &sh.connectionsTable,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
			"SK": &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
		},
	})
	if err != nil {
		return nil, err
	}

	var connection StreamConnection
	if err := attributevalue.UnmarshalMap(result.Item, &connection); err != nil {
		return nil, err
	}

	return &connection, nil
}

func (sh *StreamingHandler) deleteConnection(ctx context.Context, connectionID string) error {
	_, err := sh.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: &sh.connectionsTable,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
			"SK": &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
		},
	})
	return err
}

func (sh *StreamingHandler) writeSubscription(ctx context.Context, connectionID, userID, stream string) error {
	item := map[string]types.AttributeValue{
		"PK":           &types.AttributeValueMemberS{Value: "SUB#" + stream},
		"SK":           &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
		"ConnectionID": &types.AttributeValueMemberS{Value: connectionID},
		"UserID":       &types.AttributeValueMemberS{Value: userID},
		"Stream":       &types.AttributeValueMemberS{Value: stream},
		"TTL":          &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(24*time.Hour).Unix())},
	}

	_, err := sh.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &sh.subscriptionsTable,
		Item:      item,
	})
	return err
}

func (sh *StreamingHandler) deleteSubscription(ctx context.Context, connectionID, stream string) error {
	_, err := sh.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: &sh.subscriptionsTable,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "SUB#" + stream},
			"SK": &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
		},
	})
	return err
}

func (sh *StreamingHandler) deleteSubscriptions(ctx context.Context, connectionID string) error {
	// This would need to query all subscriptions for a connection
	// For now, we'll rely on TTL to clean these up
	// In production, you'd want to implement a GSI to query by connectionID
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

func (sh *StreamingHandler) sendError(connectionID string, errorMessage string) (events.APIGatewayProxyResponse, error) {
	errMsg := StreamMessage{
		Type: "error",
		Payload: map[string]any{
			"error":     errorMessage,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := sh.sendMessageToConnection(connectionID, errMsg); err != nil {
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
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

func main() {
	// Create the streaming handler
	handler, err := NewStreamingHandler()
	if err != nil {
		log.Fatal("failed to create streaming handler", zap.Error(err))
	}

	// Start Lambda with the handler
	lambda.Start(handler.HandleWebSocketEvent)
}