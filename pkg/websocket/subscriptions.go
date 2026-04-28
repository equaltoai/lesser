// Package websocket provides WebSocket subscription management with API Gateway integration for real-time notifications.
package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/theory-cloud/apptheory/pkg/streamer"
	"go.uber.org/zap"
)

type subscriptionRepository interface {
	HandleConnect(ctx context.Context, connectionID, userID string) error
	HandleDisconnect(ctx context.Context, connectionID string) error
	CreateSubscription(ctx context.Context, connectionID, subscriptionType string, filter map[string]any) error
	DeleteSubscription(ctx context.Context, connectionID, subscriptionType string) error
	GetSubscriptionsForType(ctx context.Context, subscriptionType string) ([]storageModels.WebSocketEventSubscription, error)
}

// SubscriptionManager manages WebSocket subscriptions
type SubscriptionManager interface {
	SubscribeModerationQueue(connectionID string, filter ModerationFilter) error
	SubscribeThreatIntel(connectionID string) error
	SubscribePerformanceAlerts(connectionID string, severity string) error
	SubscribeInfrastructureEvents(connectionID string) error
	SubscribeCommunityNotes(connectionID string) error
	SubscribeTimeline(connectionID string) error
	SubscribeNotifications(connectionID string) error
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
	ConnectionID     string         `json:"connection_id" dynamodbav:"ConnectionID"`
	SubscriptionType string         `json:"subscription_type" dynamodbav:"SubscriptionType"`
	Filter           map[string]any `json:"filter" dynamodbav:"Filter"`
	CreatedAt        time.Time      `json:"created_at" dynamodbav:"CreatedAt"`
	TTL              int64          `json:"ttl" dynamodbav:"TTL"`
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
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Severity    string         `json:"severity"`
	ContentID   string         `json:"content_id"`
	UserID      string         `json:"user_id"`
	Description string         `json:"description"`
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata"`
}

// ThreatAlert represents a security threat alert
type ThreatAlert struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Severity    string         `json:"severity"`
	Source      string         `json:"source"`
	Target      string         `json:"target"`
	Description string         `json:"description"`
	Indicators  []string       `json:"indicators"`
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata"`
}

// PerformanceAlert represents a performance alert
type PerformanceAlert struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Severity    string         `json:"severity"`
	Service     string         `json:"service"`
	Metric      string         `json:"metric"`
	Value       float64        `json:"value"`
	Threshold   float64        `json:"threshold"`
	Description string         `json:"description"`
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata"`
}

// InfrastructureEvent represents an infrastructure event
type InfrastructureEvent struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Severity    string         `json:"severity"`
	Service     string         `json:"service"`
	Region      string         `json:"region"`
	Description string         `json:"description"`
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata"`
}

// WebSocketMessage represents a message sent over WebSocket
//
//nolint:revive // WebSocket prefix clarifies this is WebSocket-specific message
type WebSocketMessage struct {
	Type      string    `json:"type"`
	Data      any       `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

// subscriptionManager implements SubscriptionManager
type subscriptionManager struct {
	repo          subscriptionRepository
	apiGW         streamer.Client
	endpoint      string
	connections   map[string]*Connection
	subscriptions map[string]map[string]*Subscription // connectionID -> subscriptionType -> subscription
	mutex         sync.RWMutex
	logger        *zap.Logger
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager(repo *repositories.WebSocketSubscriptionManagerRepository, apiGWEndpoint string, logger *zap.Logger) (SubscriptionManager, error) {
	apiClient, err := streamer.NewClient(context.Background(), apiGWEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create WebSocket client: %w", err)
	}

	return &subscriptionManager{
		repo:          repo,
		apiGW:         apiClient,
		endpoint:      apiGWEndpoint,
		connections:   make(map[string]*Connection),
		subscriptions: make(map[string]map[string]*Subscription),
		logger:        logger,
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

	// Store in DynamoDB using repository
	// Use background context for async repository operation
	err := sm.repo.HandleConnect(context.Background(), connectionID, userID)
	if err != nil {
		return fmt.Errorf("failed to store connection: %w", err)
	}

	sm.logger.Info("WebSocket connection established",
		zap.String("connection_id", connectionID),
		zap.String("user_id", userID),
	)
	return nil
}

// HandleDisconnect handles WebSocket disconnection
func (sm *subscriptionManager) HandleDisconnect(connectionID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Remove from memory
	delete(sm.connections, connectionID)
	delete(sm.subscriptions, connectionID)

	// Remove from DynamoDB using repository
	// Use background context for async repository operation
	err := sm.repo.HandleDisconnect(context.Background(), connectionID)
	if err != nil {
		sm.logger.Warn("Failed to delete connection from DynamoDB",
			zap.String("connection_id", connectionID),
			zap.Error(err),
		)
	}

	sm.logger.Info("WebSocket connection disconnected",
		zap.String("connection_id", connectionID),
	)
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
	filter := map[string]any{
		"severity": severity,
	}
	return sm.createSubscription(connectionID, "performance", filter)
}

// SubscribeInfrastructureEvents subscribes to infrastructure events
func (sm *subscriptionManager) SubscribeInfrastructureEvents(connectionID string) error {
	return sm.createSubscription(connectionID, "infrastructure", nil)
}

// SubscribeCommunityNotes subscribes to community note events
func (sm *subscriptionManager) SubscribeCommunityNotes(connectionID string) error {
	return sm.createSubscription(connectionID, "community_notes", nil)
}

// SubscribeTimeline subscribes to timeline events
func (sm *subscriptionManager) SubscribeTimeline(connectionID string) error {
	return sm.createSubscription(connectionID, "timeline", nil)
}

// SubscribeNotifications subscribes to notification events
func (sm *subscriptionManager) SubscribeNotifications(connectionID string) error {
	return sm.createSubscription(connectionID, "notifications", nil)
}

// createSubscription creates a new subscription
func (sm *subscriptionManager) createSubscription(connectionID string, subscriptionType string, filter any) error {
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

	var filterMap map[string]any
	if filter != nil {
		var err error
		filterMap, err = convertToMap(filter)
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

	// Store in DynamoDB using repository
	// Use background context for async repository operation
	err := sm.repo.CreateSubscription(context.Background(), connectionID, subscriptionType, filterMap)
	if err != nil {
		return fmt.Errorf("failed to store subscription: %w", err)
	}

	sm.logger.Info("Subscription created",
		zap.String("connection_id", connectionID),
		zap.String("subscription_type", subscriptionType),
	)
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

	// Remove from DynamoDB using repository
	// Use background context for async repository operation
	err := sm.repo.DeleteSubscription(context.Background(), connectionID, subscriptionType)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	sm.logger.Info("Subscription removed",
		zap.String("connection_id", connectionID),
		zap.String("subscription_type", subscriptionType),
	)
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
	messageData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Get subscriptions from repository instead of memory
	// Use background context for async repository operation
	subscriptions, err := sm.repo.GetSubscriptionsForType(context.Background(), subscriptionType)
	if err != nil {
		return fmt.Errorf("failed to get subscriptions for type %s: %w", subscriptionType, err)
	}

	var errors []error
	for _, dbSub := range subscriptions {
		// Convert repository model to legacy Subscription for filter compatibility
		sub := &Subscription{
			ConnectionID:     dbSub.ConnectionID,
			SubscriptionType: dbSub.SubscriptionType,
			Filter:           dbSub.Filter,
			CreatedAt:        dbSub.CreatedAt,
			TTL:              dbSub.TTL,
		}

		// Apply filter if provided
		if filter != nil && !filter(sub) {
			continue
		}

		// Send message to connection
		if err := sm.sendMessage(dbSub.ConnectionID, messageData); err != nil {
			errors = append(errors, fmt.Errorf("failed to send to %s: %w", dbSub.ConnectionID, err))
			// Remove dead connections
			sm.handleDeadConnection(dbSub.ConnectionID)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send to some connections: %v", errors)
	}

	return nil
}

// sendMessage sends a message to a WebSocket connection
func (sm *subscriptionManager) sendMessage(connectionID string, data []byte) error {
	// Use background context for async API Gateway operation
	return sm.apiGW.PostToConnection(context.Background(), connectionID, data)
}

// handleDeadConnection removes a dead connection
func (sm *subscriptionManager) handleDeadConnection(connectionID string) {
	go func() {
		if err := sm.HandleDisconnect(connectionID); err != nil {
			sm.logger.Error("Failed to handle dead connection",
				zap.String("connection_id", connectionID),
				zap.Error(err),
			)
		}
	}()
}

// matchesModerationFilter checks if a moderation event matches the filter
func (sm *subscriptionManager) matchesModerationFilter(event *ModerationEvent, filter map[string]any) bool {
	if filter == nil {
		return true
	}

	// Check severity filter
	if severities, ok := filter["severity"].([]any); ok {
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
	if types, ok := filter["types"].([]any); ok {
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
func (sm *subscriptionManager) matchesPerformanceFilter(alert *PerformanceAlert, filter map[string]any) bool {
	if filter == nil {
		return true
	}

	// Check severity filter
	if severity, ok := filter["severity"].(string); ok && severity != "" && severity != alert.Severity {
		return false
	}

	return true
}

// convertToMap converts an any to map[string]any
func convertToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// WebSocketHandler handles API Gateway WebSocket events
//
//nolint:revive // WebSocket prefix clarifies this is WebSocket-specific handler
type WebSocketHandler struct {
	subscriptionManager SubscriptionManager
	logger              *zap.Logger
	principalMu         sync.RWMutex
	principals          map[string]webSocketPrincipal
}

type webSocketPrincipal struct {
	UserID        string
	Role          string
	Authenticated bool
}

type webSocketStatusError struct {
	statusCode int
	message    string
}

func (e *webSocketStatusError) Error() string {
	return e.message
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(sm SubscriptionManager, logger *zap.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		subscriptionManager: sm,
		logger:              logger,
		principals:          make(map[string]webSocketPrincipal),
	}
}

// HandleAPIGatewayWebSocketEvent handles WebSocket events from API Gateway
func (h *WebSocketHandler) HandleAPIGatewayWebSocketEvent(_ context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	connectionID := event.RequestContext.ConnectionID
	route := event.RequestContext.RouteKey

	switch route {
	case "$connect":
		return h.handleConnect(connectionID, event)
	case "$disconnect":
		return h.handleDisconnect(connectionID)
	case "subscribe":
		return h.handleSubscribe(connectionID, event.Body)
	case "unsubscribe":
		return h.handleUnsubscribe(connectionID, event.Body)
	default:
		return events.APIGatewayProxyResponse{StatusCode: 404}, fmt.Errorf("unknown route: %s", route)
	}
}

// handleConnect handles WebSocket connection events
func (h *WebSocketHandler) handleConnect(connectionID string, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	principal := h.extractPrincipal(event)
	userID := principal.UserID
	if userID == "" {
		userID = h.extractUserID(event)
		principal.UserID = userID
	}

	if err := h.subscriptionManager.HandleConnect(connectionID, userID); err != nil {
		h.logger.Error("Failed to handle connect",
			zap.String("connection_id", connectionID),
			zap.String("user_id", userID),
			zap.Error(err),
		)
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	h.rememberConnectionPrincipal(connectionID, principal)

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

// extractUserID extracts user ID from query parameters or returns anonymous
func (h *WebSocketHandler) extractUserID(event events.APIGatewayWebsocketProxyRequest) string {
	userID := event.QueryStringParameters["user_id"]
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		userID = "anonymous"
	}
	return userID
}

func (h *WebSocketHandler) extractPrincipal(event events.APIGatewayWebsocketProxyRequest) webSocketPrincipal {
	fields := flattenAuthorizerFields(event.RequestContext.Authorizer)
	userID := firstAuthorizerField(fields,
		"username",
		"user_id",
		"userid",
		"user",
		"sub",
		"principal_id",
		"principalid",
		"cognito_username",
		"cognitousername",
	)
	role := firstAuthorizerField(fields,
		"role",
		"roles",
		"user_role",
		"userrole",
		"cognito_groups",
		"cognitogroups",
	)

	return webSocketPrincipal{
		UserID:        userID,
		Role:          role,
		Authenticated: userID != "",
	}
}

func flattenAuthorizerFields(authorizer any) map[string]string {
	if authorizer == nil {
		return nil
	}
	data, err := json.Marshal(authorizer)
	if err != nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil
	}

	fields := map[string]string{}
	collectAuthorizerFields(fields, "", decoded)
	return fields
}

func collectAuthorizerFields(fields map[string]string, prefix string, value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			collectAuthorizerFields(fields, key, child)
			if prefix != "" {
				collectAuthorizerFields(fields, prefix+"_"+key, child)
			}
		}
	case []any:
		parts := make([]string, 0, len(v))
		for _, child := range v {
			if part := strings.TrimSpace(fmt.Sprint(child)); part != "" && part != "<nil>" {
				parts = append(parts, part)
			}
		}
		if prefix != "" && len(parts) > 0 {
			fields[normalizeAuthorizerKey(prefix)] = strings.Join(parts, ",")
		}
	default:
		if prefix == "" {
			return
		}
		value := strings.TrimSpace(fmt.Sprint(v))
		if value == "" || value == "<nil>" {
			return
		}
		fields[normalizeAuthorizerKey(prefix)] = value
	}
}

func firstAuthorizerField(fields map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fields[normalizeAuthorizerKey(key)]); value != "" {
			return value
		}
	}
	return ""
}

func normalizeAuthorizerKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	replacer := strings.NewReplacer("_", "", "-", "", ".", "", ":", "", " ", "")
	return replacer.Replace(key)
}

func (h *WebSocketHandler) rememberConnectionPrincipal(connectionID string, principal webSocketPrincipal) {
	h.principalMu.Lock()
	defer h.principalMu.Unlock()

	if h.principals == nil {
		h.principals = make(map[string]webSocketPrincipal)
	}
	h.principals[connectionID] = principal
}

func (h *WebSocketHandler) forgetConnectionPrincipal(connectionID string) {
	h.principalMu.Lock()
	defer h.principalMu.Unlock()

	delete(h.principals, connectionID)
}

func (h *WebSocketHandler) connectionPrincipal(connectionID string) (webSocketPrincipal, bool) {
	h.principalMu.RLock()
	defer h.principalMu.RUnlock()

	if h.principals == nil {
		return webSocketPrincipal{}, false
	}
	principal, ok := h.principals[connectionID]
	return principal, ok
}

// handleDisconnect handles WebSocket disconnection events
func (h *WebSocketHandler) handleDisconnect(connectionID string) (events.APIGatewayProxyResponse, error) {
	h.forgetConnectionPrincipal(connectionID)
	if err := h.subscriptionManager.HandleDisconnect(connectionID); err != nil {
		h.logger.Error("Failed to handle disconnect",
			zap.String("connection_id", connectionID),
			zap.Error(err),
		)
	}
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

// handleSubscribe handles subscription requests
func (h *WebSocketHandler) handleSubscribe(connectionID string, body string) (events.APIGatewayProxyResponse, error) {
	var request struct {
		Type   string `json:"type"`
		Filter any    `json:"filter,omitempty"`
	}

	if err := json.Unmarshal([]byte(body), &request); err != nil {
		return events.APIGatewayProxyResponse{StatusCode: 400}, fmt.Errorf("invalid request body: %w", err)
	}

	err := h.processSubscriptionType(connectionID, request.Type, request.Filter)
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: webSocketErrorStatus(err)}, err
	}

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}

// processSubscriptionType processes different subscription types
func (h *WebSocketHandler) processSubscriptionType(connectionID string, subType string, filter any) error {
	if isAdminAlertSubscriptionType(subType) {
		if err := h.ensureAdminAlertSubscriptionAuthorized(connectionID); err != nil {
			return err
		}
	}

	switch subType {
	case "moderation":
		return h.subscribeModerationQueue(connectionID, filter)
	case "threat_intel":
		return h.subscriptionManager.SubscribeThreatIntel(connectionID)
	case "performance":
		return h.subscribePerformanceAlerts(connectionID, filter)
	case "infrastructure":
		return h.subscriptionManager.SubscribeInfrastructureEvents(connectionID)
	case "community_notes":
		return h.subscriptionManager.SubscribeCommunityNotes(connectionID)
	case "timeline":
		return h.subscriptionManager.SubscribeTimeline(connectionID)
	case "notifications":
		return h.subscriptionManager.SubscribeNotifications(connectionID)
	default:
		return fmt.Errorf("unknown subscription type: %s", subType)
	}
}

func isAdminAlertSubscriptionType(subType string) bool {
	switch subType {
	case "moderation", "threat_intel", "performance", "infrastructure":
		return true
	default:
		return false
	}
}

func (h *WebSocketHandler) ensureAdminAlertSubscriptionAuthorized(connectionID string) error {
	principal, ok := h.connectionPrincipal(connectionID)
	if !ok || !principal.Authenticated {
		return &webSocketStatusError{
			statusCode: http.StatusUnauthorized,
			message:    "admin alert subscriptions require authentication",
		}
	}
	if !webSocketRoleAllowsAdminAlerts(principal.Role) {
		return &webSocketStatusError{
			statusCode: http.StatusForbidden,
			message:    "admin alert subscriptions require admin or moderator role",
		}
	}
	return nil
}

func webSocketRoleAllowsAdminAlerts(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	for _, part := range strings.FieldsFunc(role, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	}) {
		switch strings.TrimSpace(part) {
		case "admin", "moderator", "mod":
			return true
		}
	}
	return false
}

func webSocketErrorStatus(err error) int {
	var statusErr *webSocketStatusError
	if errors.As(err, &statusErr) {
		return statusErr.statusCode
	}
	return http.StatusInternalServerError
}

// subscribeModerationQueue handles moderation queue subscriptions
func (h *WebSocketHandler) subscribeModerationQueue(connectionID string, filter any) error {
	modFilter := h.parseModerationFilter(filter)
	return h.subscriptionManager.SubscribeModerationQueue(connectionID, modFilter)
}

// parseModerationFilter parses the moderation filter from request
func (h *WebSocketHandler) parseModerationFilter(filter any) ModerationFilter {
	if filterMap, ok := filter.(map[string]any); ok {
		var modFilter ModerationFilter
		if data, err := json.Marshal(filterMap); err == nil {
			if err := json.Unmarshal(data, &modFilter); err != nil {
				h.logger.Warn("failed to unmarshal moderation filter", zap.Error(err))
			}
		}
		return modFilter
	}
	return ModerationFilter{}
}

// subscribePerformanceAlerts handles performance alert subscriptions
func (h *WebSocketHandler) subscribePerformanceAlerts(connectionID string, filter any) error {
	severity := h.extractSeverity(filter)
	return h.subscriptionManager.SubscribePerformanceAlerts(connectionID, severity)
}

// extractSeverity extracts severity from filter or returns default
func (h *WebSocketHandler) extractSeverity(filter any) string {
	if filterMap, ok := filter.(map[string]any); ok {
		if sev, ok := filterMap["severity"].(string); ok {
			return sev
		}
	}
	return "medium" // default
}

// handleUnsubscribe handles unsubscribe requests
func (h *WebSocketHandler) handleUnsubscribe(connectionID string, body string) (events.APIGatewayProxyResponse, error) {
	var request struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal([]byte(body), &request); err != nil {
		return events.APIGatewayProxyResponse{StatusCode: 400}, fmt.Errorf("invalid request body: %w", err)
	}

	if err := h.subscriptionManager.Unsubscribe(connectionID, request.Type); err != nil {
		return events.APIGatewayProxyResponse{StatusCode: 500}, err
	}

	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}
