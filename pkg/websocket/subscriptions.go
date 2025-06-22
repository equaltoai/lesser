package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
)

// SubscriptionManager manages WebSocket subscriptions
type SubscriptionManager interface {
	SubscribeModerationQueue(connectionID string, filter ModerationFilter) error
	SubscribeThreatIntel(connectionID string) error
	SubscribePerformanceAlerts(connectionID string, severity string) error
	SubscribeInfrastructureEvents(connectionID string) error
	Unsubscribe(connectionID string, subscriptionType string) error
	PublishModerationEvent(event *ModerationEvent) error
	PublishThreatAlert(alert *ThreatAlert) error
	PublishPerformanceAlert(alert *PerformanceAlert) error
	PublishInfrastructureEvent(event *InfrastructureEvent) error
	HandleConnect(connectionID, userID string) error
	HandleDisconnect(connectionID string) error
}

// Connection represents a WebSocket connection
type Connection struct {
	ConnectionID string    `json:"connection_id" dynamodbav:"ConnectionID"`
	UserID       string    `json:"user_id" dynamodbav:"UserID"`
	ConnectedAt  time.Time `json:"connected_at" dynamodbav:"ConnectedAt"`
	LastSeen     time.Time `json:"last_seen" dynamodbav:"LastSeen"`
	TTL          int64     `json:"ttl" dynamodbav:"TTL"`
}

// Subscription represents a subscription to events
type Subscription struct {
	ConnectionID     string                 `json:"connection_id" dynamodbav:"ConnectionID"`
	SubscriptionType string                 `json:"subscription_type" dynamodbav:"SubscriptionType"`
	Filter           map[string]interface{} `json:"filter" dynamodbav:"Filter"`
	CreatedAt        time.Time              `json:"created_at" dynamodbav:"CreatedAt"`
	TTL              int64                  `json:"ttl" dynamodbav:"TTL"`
}

// ModerationFilter filters moderation events
type ModerationFilter struct {
	Severity  []string `json:"severity"`
	Types     []string `json:"types"`
	UserID    string   `json:"user_id,omitempty"`
	ContentID string   `json:"content_id,omitempty"`
}

// ModerationEvent represents a moderation event
type ModerationEvent struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	ContentID   string    `json:"content_id"`
	UserID      string    `json:"user_id"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ThreatAlert represents a security threat alert
type ThreatAlert struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Source      string    `json:"source"`
	Target      string    `json:"target"`
	Description string    `json:"description"`
	Indicators  []string  `json:"indicators"`
	Timestamp   time.Time `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// PerformanceAlert represents a performance alert
type PerformanceAlert struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Service     string    `json:"service"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// InfrastructureEvent represents an infrastructure event
type InfrastructureEvent struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Service     string    `json:"service"`
	Region      string    `json:"region"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// subscriptionManager implements SubscriptionManager
type subscriptionManager struct {
	dynamoDB      *dynamodb.Client
	apiGW         *apigatewaymanagementapi.Client
	tableName     string
	endpoint      string
	connections   map[string]*Connection
	subscriptions map[string]map[string]*Subscription // connectionID -> subscriptionType -> subscription
	mutex         sync.RWMutex
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(tableName, apiGWEndpoint string) (SubscriptionManager, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Configure API Gateway Management API endpoint
	apiGWCfg := cfg.Copy()
	apiGWCfg.BaseEndpoint = aws.String(apiGWEndpoint)

	return &subscriptionManager{
		dynamoDB:      dynamodb.NewFromConfig(cfg),
		apiGW:         apigatewaymanagementapi.NewFromConfig(apiGWCfg),
		tableName:     tableName,
		endpoint:      apiGWEndpoint,
		connections:   make(map[string]*Connection),
		subscriptions: make(map[string]map[string]*Subscription),
	}, nil
}

// HandleConnect handles a new WebSocket connection
func (sm *subscriptionManager) HandleConnect(connectionID, userID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	connection := &Connection{
		ConnectionID: connectionID,
		UserID:       userID,
		ConnectedAt:  time.Now(),
		LastSeen:     time.Now(),
		TTL:          time.Now().Add(24 * time.Hour).Unix(), // Expire after 24 hours
	}

	// Store in memory
	sm.connections[connectionID] = connection

	// Store in DynamoDB
	item, err := attributevalue.MarshalMap(connection)
	if err != nil {
		return fmt.Errorf("failed to marshal connection: %w", err)
	}

	item["PK"] = &types.AttributeValueMemberS{Value: "CONNECTION#" + connectionID}
	item["SK"] = &types.AttributeValueMemberS{Value: "METADATA"}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(sm.tableName),
		Item:      item,
	}

	_, err = sm.dynamoDB.PutItem(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to store connection: %w", err)
	}

	log.Printf("WebSocket connection established: %s (user: %s)", connectionID, userID)
	return nil
}

// HandleDisconnect handles WebSocket disconnection
func (sm *subscriptionManager) HandleDisconnect(connectionID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Remove from memory
	delete(sm.connections, connectionID)
	delete(sm.subscriptions, connectionID)

	// Remove from DynamoDB
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(sm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CONNECTION#" + connectionID},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	}

	_, err := sm.dynamoDB.DeleteItem(context.TODO(), input)
	if err != nil {
		log.Printf("Failed to delete connection from DynamoDB: %v", err)
	}

	// Clean up subscriptions
	sm.cleanupSubscriptions(connectionID)

	log.Printf("WebSocket connection disconnected: %s", connectionID)
	return nil
}

// SubscribeModerationQueue subscribes to moderation events
func (sm *subscriptionManager) SubscribeModerationQueue(connectionID string, filter ModerationFilter) error {
	return sm.createSubscription(connectionID, "moderation", filter)
}

// SubscribeThreatIntel subscribes to threat intelligence alerts
func (sm *subscriptionManager) SubscribeThreatIntel(connectionID string) error {
	return sm.createSubscription(connectionID, "threat_intel", nil)
}

// SubscribePerformanceAlerts subscribes to performance alerts
func (sm *subscriptionManager) SubscribePerformanceAlerts(connectionID string, severity string) error {
	filter := map[string]interface{}{
		"severity": severity,
	}
	return sm.createSubscription(connectionID, "performance", filter)
}

// SubscribeInfrastructureEvents subscribes to infrastructure events
func (sm *subscriptionManager) SubscribeInfrastructureEvents(connectionID string) error {
	return sm.createSubscription(connectionID, "infrastructure", nil)
}

// createSubscription creates a new subscription
func (sm *subscriptionManager) createSubscription(connectionID string, subscriptionType string, filter interface{}) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Check if connection exists
	if _, exists := sm.connections[connectionID]; !exists {
		return fmt.Errorf("connection not found: %s", connectionID)
	}

	subscription := &Subscription{
		ConnectionID:     connectionID,
		SubscriptionType: subscriptionType,
		CreatedAt:        time.Now(),
		TTL:              time.Now().Add(24 * time.Hour).Unix(),
	}

	if filter != nil {
		filterMap, err := convertToMap(filter)
		if err != nil {
			return fmt.Errorf("failed to convert filter: %w", err)
		}
		subscription.Filter = filterMap
	}

	// Store in memory
	if sm.subscriptions[connectionID] == nil {
		sm.subscriptions[connectionID] = make(map[string]*Subscription)
	}
	sm.subscriptions[connectionID][subscriptionType] = subscription

	// Store in DynamoDB
	item, err := attributevalue.MarshalMap(subscription)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription: %w", err)
	}

	item["PK"] = &types.AttributeValueMemberS{Value: "CONNECTION#" + connectionID}
	item["SK"] = &types.AttributeValueMemberS{Value: "SUBSCRIPTION#" + subscriptionType}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(sm.tableName),
		Item:      item,
	}

	_, err = sm.dynamoDB.PutItem(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to store subscription: %w", err)
	}

	log.Printf("Subscription created: %s -> %s", connectionID, subscriptionType)
	return nil
}

// Unsubscribe removes a subscription
func (sm *subscriptionManager) Unsubscribe(connectionID string, subscriptionType string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Remove from memory
	if subs, exists := sm.subscriptions[connectionID]; exists {
		delete(subs, subscriptionType)
	}

	// Remove from DynamoDB
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(sm.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "CONNECTION#" + connectionID},
			"SK": &types.AttributeValueMemberS{Value: "SUBSCRIPTION#" + subscriptionType},
		},
	}

	_, err := sm.dynamoDB.DeleteItem(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	log.Printf("Subscription removed: %s -> %s", connectionID, subscriptionType)
	return nil
}

// PublishModerationEvent publishes a moderation event to subscribers
func (sm *subscriptionManager) PublishModerationEvent(event *ModerationEvent) error {
	message := WebSocketMessage{
		Type:      "moderation_event",
		Data:      event,
		Timestamp: time.Now(),
	}

	return sm.publishToSubscribers("moderation", message, func(sub *Subscription) bool {
		return sm.matchesModerationFilter(event, sub.Filter)
	})
}

// PublishThreatAlert publishes a threat alert to subscribers
func (sm *subscriptionManager) PublishThreatAlert(alert *ThreatAlert) error {
	message := WebSocketMessage{
		Type:      "threat_alert",
		Data:      alert,
		Timestamp: time.Now(),
	}

	return sm.publishToSubscribers("threat_intel", message, nil)
}

// PublishPerformanceAlert publishes a performance alert to subscribers
func (sm *subscriptionManager) PublishPerformanceAlert(alert *PerformanceAlert) error {
	message := WebSocketMessage{
		Type:      "performance_alert",
		Data:      alert,
		Timestamp: time.Now(),
	}

	return sm.publishToSubscribers("performance", message, func(sub *Subscription) bool {
		return sm.matchesPerformanceFilter(alert, sub.Filter)
	})
}

// PublishInfrastructureEvent publishes an infrastructure event to subscribers
func (sm *subscriptionManager) PublishInfrastructureEvent(event *InfrastructureEvent) error {
	message := WebSocketMessage{
		Type:      "infrastructure_event",
		Data:      event,
		Timestamp: time.Now(),
	}

	return sm.publishToSubscribers("infrastructure", message, nil)
}

// publishToSubscribers publishes a message to all matching subscribers
func (sm *subscriptionManager) publishToSubscribers(subscriptionType string, message WebSocketMessage, filter func(*Subscription) bool) error {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	messageData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	var errors []error
	for connectionID, subscriptions := range sm.subscriptions {
		if sub, exists := subscriptions[subscriptionType]; exists {
			// Apply filter if provided
			if filter != nil && !filter(sub) {
				continue
			}

			// Send message to connection
			if err := sm.sendMessage(connectionID, messageData); err != nil {
				errors = append(errors, fmt.Errorf("failed to send to %s: %w", connectionID, err))
				// Remove dead connections
				sm.handleDeadConnection(connectionID)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send to some connections: %v", errors)
	}

	return nil
}

// sendMessage sends a message to a WebSocket connection
func (sm *subscriptionManager) sendMessage(connectionID string, data []byte) error {
	input := &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: aws.String(connectionID),
		Data:         data,
	}

	_, err := sm.apiGW.PostToConnection(context.TODO(), input)
	return err
}

// handleDeadConnection removes a dead connection
func (sm *subscriptionManager) handleDeadConnection(connectionID string) {
	go func() {
		if err := sm.HandleDisconnect(connectionID); err != nil {
			log.Printf("Failed to handle dead connection %s: %v", connectionID, err)
		}
	}()
}

// matchesModerationFilter checks if a moderation event matches the filter
func (sm *subscriptionManager) matchesModerationFilter(event *ModerationEvent, filter map[string]interface{}) bool {
	if filter == nil {
		return true
	}

	// Check severity filter
	if severities, ok := filter["severity"].([]interface{}); ok {
		matched := false
		for _, sev := range severities {
			if sevStr, ok := sev.(string); ok && sevStr == event.Severity {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check types filter
	if types, ok := filter["types"].([]interface{}); ok {
		matched := false
		for _, typ := range types {
			if typStr, ok := typ.(string); ok && typStr == event.Type {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check user ID filter
	if userID, ok := filter["user_id"].(string); ok && userID != "" && userID != event.UserID {
		return false
	}

	// Check content ID filter
	if contentID, ok := filter["content_id"].(string); ok && contentID != "" && contentID != event.ContentID {
		return false
	}

	return true
}

// matchesPerformanceFilter checks if a performance alert matches the filter
func (sm *subscriptionManager) matchesPerformanceFilter(alert *PerformanceAlert, filter map[string]interface{}) bool {
	if filter == nil {
		return true
	}

	// Check severity filter
	if severity, ok := filter["severity"].(string); ok && severity != "" && severity != alert.Severity {
		return false
	}

	return true
}

// cleanupSubscriptions removes all subscriptions for a connection
func (sm *subscriptionManager) cleanupSubscriptions(connectionID string) {
	// Query DynamoDB for all subscriptions for this connection
	input := &dynamodb.QueryInput{
		TableName:              aws.String(sm.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk_prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":        &types.AttributeValueMemberS{Value: "CONNECTION#" + connectionID},
			":sk_prefix": &types.AttributeValueMemberS{Value: "SUBSCRIPTION#"},
		},
	}

	result, err := sm.dynamoDB.Query(context.TODO(), input)
	if err != nil {
		log.Printf("Failed to query subscriptions for cleanup: %v", err)
		return
	}

	// Delete each subscription
	for _, item := range result.Items {
		deleteInput := &dynamodb.DeleteItemInput{
			TableName: aws.String(sm.tableName),
			Key: map[string]types.AttributeValue{
				"PK": item["PK"],
				"SK": item["SK"],
			},
		}

		if _, err := sm.dynamoDB.DeleteItem(context.TODO(), deleteInput); err != nil {
			log.Printf("Failed to delete subscription: %v", err)
		}
	}
}

// convertToMap converts an interface{} to map[string]interface{}
func convertToMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// WebSocketHandler handles API Gateway WebSocket events
type WebSocketHandler struct {
	subscriptionManager SubscriptionManager
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(sm SubscriptionManager) *WebSocketHandler {
	return &WebSocketHandler{
		subscriptionManager: sm,
	}
}

// HandleAPIGatewayWebSocketEvent handles WebSocket events from API Gateway
func (h *WebSocketHandler) HandleAPIGatewayWebSocketEvent(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	connectionID := event.RequestContext.ConnectionID
	route := event.RequestContext.RouteKey

	switch route {
	case "$connect":
		// Extract user ID from query parameters or headers
		userID := event.QueryStringParameters["user_id"]
		if userID == "" {
			userID = "anonymous"
		}

		if err := h.subscriptionManager.HandleConnect(connectionID, userID); err != nil {
			log.Printf("Failed to handle connect: %v", err)
			return events.APIGatewayProxyResponse{StatusCode: 500}, err
		}

		return events.APIGatewayProxyResponse{StatusCode: 200}, nil

	case "$disconnect":
		if err := h.subscriptionManager.HandleDisconnect(connectionID); err != nil {
			log.Printf("Failed to handle disconnect: %v", err)
		}

		return events.APIGatewayProxyResponse{StatusCode: 200}, nil

	case "subscribe":
		var request struct {
			Type   string      `json:"type"`
			Filter interface{} `json:"filter,omitempty"`
		}

		if err := json.Unmarshal([]byte(event.Body), &request); err != nil {
			return events.APIGatewayProxyResponse{StatusCode: 400}, fmt.Errorf("invalid request body: %w", err)
		}

		var err error
		switch request.Type {
		case "moderation":
			if filter, ok := request.Filter.(map[string]interface{}); ok {
				var modFilter ModerationFilter
				if data, marshalErr := json.Marshal(filter); marshalErr == nil {
					json.Unmarshal(data, &modFilter)
				}
				err = h.subscriptionManager.SubscribeModerationQueue(connectionID, modFilter)
			} else {
				err = h.subscriptionManager.SubscribeModerationQueue(connectionID, ModerationFilter{})
			}
		case "threat_intel":
			err = h.subscriptionManager.SubscribeThreatIntel(connectionID)
		case "performance":
			severity := "medium" // default
			if filter, ok := request.Filter.(map[string]interface{}); ok {
				if sev, ok := filter["severity"].(string); ok {
					severity = sev
				}
			}
			err = h.subscriptionManager.SubscribePerformanceAlerts(connectionID, severity)
		case "infrastructure":
			err = h.subscriptionManager.SubscribeInfrastructureEvents(connectionID)
		default:
			return events.APIGatewayProxyResponse{StatusCode: 400}, fmt.Errorf("unknown subscription type: %s", request.Type)
		}

		if err != nil {
			return events.APIGatewayProxyResponse{StatusCode: 500}, err
		}

		return events.APIGatewayProxyResponse{StatusCode: 200}, nil

	case "unsubscribe":
		var request struct {
			Type string `json:"type"`
		}

		if err := json.Unmarshal([]byte(event.Body), &request); err != nil {
			return events.APIGatewayProxyResponse{StatusCode: 400}, fmt.Errorf("invalid request body: %w", err)
		}

		if err := h.subscriptionManager.Unsubscribe(connectionID, request.Type); err != nil {
			return events.APIGatewayProxyResponse{StatusCode: 500}, err
		}

		return events.APIGatewayProxyResponse{StatusCode: 200}, nil

	default:
		return events.APIGatewayProxyResponse{StatusCode: 404}, fmt.Errorf("unknown route: %s", route)
	}
}