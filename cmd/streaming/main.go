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

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
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
	Type    string                 `json:"type"`
	Stream  string                 `json:"stream,omitempty"`
	Event   string                 `json:"event,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// StreamEvent represents an event to be streamed
type StreamEvent struct {
	Event   string          `json:"event"` // "update", "delete", "notification", "status.update"
	Payload json.RawMessage `json:"payload"`
	Stream  []string        `json:"stream"` // streams this event should go to
}

var (
	dynamoClient       *dynamodb.Client
	apiClient          *apigatewaymanagementapi.Client
	globalCfg          aws.Config
	connectionsTable   string
	subscriptionsTable string
	log                *zap.Logger
	domain             string
)

func init() {
	// Initialize logger
	log = common.Logger()

	// Initialize AWS clients
	var err error
	globalCfg, err = config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal("failed to load AWS config", zap.Error(err))
	}

	dynamoClient = dynamodb.NewFromConfig(globalCfg)


	// Get environment variables
	connectionsTable = os.Getenv("CONNECTIONS_TABLE")
	if connectionsTable == "" {
		connectionsTable = "lesser-streaming-connections"
	}

	subscriptionsTable = os.Getenv("SUBSCRIPTIONS_TABLE")
	if subscriptionsTable == "" {
		subscriptionsTable = "lesser-streaming-subscriptions"
	}

	domain = os.Getenv("DOMAIN")
	if domain == "" {
		log.Fatal("DOMAIN environment variable not set")
	}
}

func handler(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	log := log.With(
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
	apiClient = currentApiClient

	switch event.RequestContext.RouteKey {
	case "$connect":
		return handleConnect(ctx, event)
	case "$disconnect":
		return handleDisconnect(ctx, event)
	case "$default":
		return handleMessage(ctx, event)
	default:
		log.Warn("unknown route key", zap.String("routeKey", event.RequestContext.RouteKey))
		return events.APIGatewayProxyResponse{StatusCode: 400}, nil
	}
}

func handleConnect(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	log := log.With(
		zap.String("connectionID", event.RequestContext.ConnectionID),
		zap.String("operation", "connect"),
	)

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

	if err := writeConnection(ctx, connection); err != nil {
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

func handleDisconnect(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	log := log.With(
		zap.String("connectionID", event.RequestContext.ConnectionID),
		zap.String("operation", "disconnect"),
	)

	// Delete connection from DynamoDB
	if err := deleteConnection(ctx, event.RequestContext.ConnectionID); err != nil {
		log.Error("failed to delete connection", zap.Error(err))
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	// Clean up any subscriptions
	if err := deleteSubscriptions(ctx, event.RequestContext.ConnectionID); err != nil {
		log.Error("failed to delete subscriptions", zap.Error(err))
		// Don't fail the disconnect
	}

	log.Info("connection closed")
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func handleMessage(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	log := log.With(
		zap.String("connectionID", event.RequestContext.ConnectionID),
		zap.String("operation", "message"),
	)

	// Parse incoming message
	var message StreamMessage
	if err := common.ParseRequestBody([]byte(event.Body), &message); err != nil {
		log.Error("failed to parse message", zap.Error(err))
		return sendError(event.RequestContext.ConnectionID, "Invalid message format")
	}

	// Get connection details
	connection, err := getConnection(ctx, event.RequestContext.ConnectionID)
	if err != nil {
		log.Error("failed to get connection", zap.Error(err))
		return sendError(event.RequestContext.ConnectionID, "Connection not found")
	}

	// Update last activity
	connection.LastActivity = time.Now()
	if err := writeConnection(ctx, connection); err != nil {
		log.Error("failed to update connection", zap.Error(err))
	}

	// Handle different message types
	switch message.Type {
	case "subscribe":
		return handleSubscribe(ctx, connection, message)
	case "unsubscribe":
		return handleUnsubscribe(ctx, connection, message)
	case "ping":
		return handlePing(event.RequestContext.ConnectionID)
	default:
		log.Warn("unknown message type", zap.String("type", message.Type))
		return sendError(event.RequestContext.ConnectionID, "Unknown message type")
	}
}

func handleSubscribe(ctx context.Context, connection *StreamConnection, message StreamMessage) (events.APIGatewayProxyResponse, error) {
	log := log.With(
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
		return sendError(connection.ConnectionID, "Invalid stream: "+message.Stream)
	}

	// Check authorization for stream
	if message.Stream == "user" || strings.HasPrefix(message.Stream, "user:") ||
		message.Stream == "direct" || strings.HasPrefix(message.Stream, "list:") {
		// These streams require authentication
		if connection.UserID == "" {
			return sendError(connection.ConnectionID, "Authentication required for stream: "+message.Stream)
		}
	}

	// Add stream to connection's subscriptions
	if !contains(connection.Streams, message.Stream) {
		connection.Streams = append(connection.Streams, message.Stream)
		if err := writeConnection(ctx, connection); err != nil {
			log.Error("failed to update connection streams", zap.Error(err))
			return sendError(connection.ConnectionID, "Failed to subscribe")
		}
	}

	// Store subscription in subscriptions table for efficient lookup
	if err := writeSubscription(ctx, connection.ConnectionID, connection.UserID, message.Stream); err != nil {
		log.Error("failed to write subscription", zap.Error(err))
		return sendError(connection.ConnectionID, "Failed to subscribe")
	}

	// Send confirmation
	confirmMsg := StreamMessage{
		Type:   "subscribed",
		Stream: message.Stream,
		Payload: map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := sendMessageToConnection(connection.ConnectionID, confirmMsg); err != nil {
		log.Error("failed to send confirmation", zap.Error(err))
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	log.Info("subscribed to stream")
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func handleUnsubscribe(ctx context.Context, connection *StreamConnection, message StreamMessage) (events.APIGatewayProxyResponse, error) {
	log := log.With(
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

	if err := writeConnection(ctx, connection); err != nil {
		log.Error("failed to update connection streams", zap.Error(err))
		return sendError(connection.ConnectionID, "Failed to unsubscribe")
	}

	// Remove subscription from subscriptions table
	if err := deleteSubscription(ctx, connection.ConnectionID, message.Stream); err != nil {
		log.Error("failed to delete subscription", zap.Error(err))
		// Don't fail the unsubscribe
	}

	// Send confirmation
	confirmMsg := StreamMessage{
		Type:   "unsubscribed",
		Stream: message.Stream,
		Payload: map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := sendMessageToConnection(connection.ConnectionID, confirmMsg); err != nil {
		log.Error("failed to send confirmation", zap.Error(err))
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	log.Info("unsubscribed from stream")
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

func handlePing(connectionID string) (events.APIGatewayProxyResponse, error) {
	pongMessage := StreamMessage{
		Type: "pong",
		Payload: map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := sendMessageToConnection(connectionID, pongMessage); err != nil {
		log.Error("failed to send pong", zap.Error(err))
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

// Database operations

func writeConnection(ctx context.Context, connection *StreamConnection) error {
	item, err := attributevalue.MarshalMap(connection)
	if err != nil {
		return err
	}

	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &connectionsTable,
		Item:      item,
	})
	return err
}

func getConnection(ctx context.Context, connectionID string) (*StreamConnection, error) {
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &connectionsTable,
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

func deleteConnection(ctx context.Context, connectionID string) error {
	_, err := dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: &connectionsTable,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
			"SK": &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
		},
	})
	return err
}

func writeSubscription(ctx context.Context, connectionID, userID, stream string) error {
	item := map[string]types.AttributeValue{
		"PK":           &types.AttributeValueMemberS{Value: "SUB#" + stream},
		"SK":           &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
		"ConnectionID": &types.AttributeValueMemberS{Value: connectionID},
		"UserID":       &types.AttributeValueMemberS{Value: userID},
		"Stream":       &types.AttributeValueMemberS{Value: stream},
		"TTL":          &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(24*time.Hour).Unix())},
	}

	_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &subscriptionsTable,
		Item:      item,
	})
	return err
}

func deleteSubscription(ctx context.Context, connectionID, stream string) error {
	_, err := dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: &subscriptionsTable,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "SUB#" + stream},
			"SK": &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
		},
	})
	return err
}

func deleteSubscriptions(ctx context.Context, connectionID string) error {
	// This would need to query all subscriptions for a connection
	// For now, we'll rely on TTL to clean these up
	// In production, you'd want to implement a GSI to query by connectionID
	return nil
}

// Messaging functions

func sendMessageToConnection(connectionID string, message StreamMessage) error {
	if apiClient == nil {
		return fmt.Errorf("API Gateway client not initialized")
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	_, err = apiClient.PostToConnection(context.Background(), &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: &connectionID,
		Data:         messageBytes,
	})

	return err
}

func sendError(connectionID string, errorMessage string) (events.APIGatewayProxyResponse, error) {
	errMsg := StreamMessage{
		Type: "error",
		Payload: map[string]interface{}{
			"error":     errorMessage,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	if err := sendMessageToConnection(connectionID, errMsg); err != nil {
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
	lambda.Start(handler)
}
