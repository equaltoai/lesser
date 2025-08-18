// Package streaming provides event publishing infrastructure for real-time streaming
package streaming

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// APIGatewayManagementClient defines the interface for API Gateway Management API client
type APIGatewayManagementClient interface {
	PostToConnection(ctx context.Context, params *apigatewaymanagementapi.PostToConnectionInput, optFns ...func(*apigatewaymanagementapi.Options)) (*apigatewaymanagementapi.PostToConnectionOutput, error)
}

// Event represents a streaming event to be published
type Event struct {
	Type      string                 `json:"type"`      // Event type (e.g., "status.created")
	Stream    string                 `json:"stream"`    // Stream name (e.g., "user:alice")
	Payload   map[string]interface{} `json:"payload"`   // Event payload data
	Timestamp time.Time              `json:"timestamp"` // Event timestamp
}

// Publisher defines the interface for publishing streaming events
type Publisher interface {
	// PublishToUser publishes an event to a specific user's streams
	PublishToUser(ctx context.Context, userID string, event *Event) error

	// PublishToStream publishes an event to all subscribers of a stream
	PublishToStream(ctx context.Context, streamName string, event *Event) error

	// PublishToConversation publishes an event to all participants in a conversation
	PublishToConversation(ctx context.Context, conversationID string, event *Event) error

	// Close closes the publisher and cleans up resources
	Close() error
}

// StreamConnection represents a WebSocket connection for streaming
type StreamConnection struct {
	ConnectionID string    `json:"connection_id"`
	UserID       string    `json:"user_id,omitempty"`
	Username     string    `json:"username,omitempty"`
	Streams      []string  `json:"streams"`
	LastActivity time.Time `json:"last_activity"`
}

// ConnectionRepository defines methods for accessing WebSocket connection data
type ConnectionRepository interface {
	// GetUserConnections returns all active connections for a user
	GetUserConnections(ctx context.Context, userID string) ([]*StreamConnection, error)

	// GetStreamConnections returns all connections subscribed to a stream
	GetStreamConnections(ctx context.Context, streamName string) ([]*StreamConnection, error)

	// GetConversationConnections returns all connections for participants in a conversation
	GetConversationConnections(ctx context.Context, conversationID string) ([]*StreamConnection, error)
}

// apiGatewayPublisher implements Publisher using AWS API Gateway Management API
type apiGatewayPublisher struct {
	client       APIGatewayManagementClient
	connRepo     ConnectionRepository
	logger       *zap.Logger
	endpoint     string
	mu           sync.RWMutex
	closed       bool
	deliveryTime []time.Duration // Track delivery times for metrics
}

// NewAPIGatewayPublisher creates a new publisher using API Gateway Management API
func NewAPIGatewayPublisher(
	client APIGatewayManagementClient,
	connRepo ConnectionRepository,
	endpoint string,
	logger *zap.Logger,
) Publisher {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &apiGatewayPublisher{
		client:       client,
		connRepo:     connRepo,
		logger:       logger,
		endpoint:     endpoint,
		deliveryTime: make([]time.Duration, 0),
	}
}

// PublishToUser publishes an event to all of a user's active connections
func (p *apiGatewayPublisher) PublishToUser(ctx context.Context, userID string, event *Event) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("publisher is closed")
	}
	p.mu.RUnlock()

	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		return fmt.Errorf("userID cannot be empty")
	}

	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Get all connections for the user
	connections, err := p.connRepo.GetUserConnections(ctx, userID)
	if err != nil {
		p.logger.Error("failed to get user connections",
			zap.String("user_id", userID),
			zap.Error(err))
		return fmt.Errorf("failed to get user connections: %w", err)
	}

	// Use shared helper for connection publishing
	helper := NewPublishConnectionHelper(p.logger)
	logContext := map[string]interface{}{
		"user_id": userID,
		"stream":  event.Stream,
	}
	return helper.PublishToConnections(ctx, connections, event, p.publishToConnection, logContext)

	return nil
}

// PublishToStream publishes an event to all connections subscribed to a stream
func (p *apiGatewayPublisher) PublishToStream(ctx context.Context, streamName string, event *Event) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("publisher is closed")
	}
	p.mu.RUnlock()

	if err := common.ValidateRequiredParam("streamName", streamName); err != nil {
		return fmt.Errorf("streamName cannot be empty")
	}

	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Get all connections subscribed to the stream
	connections, err := p.connRepo.GetStreamConnections(ctx, streamName)
	if err != nil {
		p.logger.Error("failed to get stream connections",
			zap.String("stream", streamName),
			zap.Error(err))
		return fmt.Errorf("failed to get stream connections: %w", err)
	}

	// Set stream in event if not provided
	if err := common.ValidateRequiredParam("event.Stream", event.Stream); err != nil {
		event.Stream = streamName
	}

	// Use shared helper for connection publishing
	helper := NewPublishConnectionHelper(p.logger)
	logContext := map[string]interface{}{
		"stream": streamName,
	}
	return helper.PublishToConnections(ctx, connections, event, p.publishToConnection, logContext)

	return nil
}

// PublishToConversation publishes an event to all participants in a conversation
func (p *apiGatewayPublisher) PublishToConversation(ctx context.Context, conversationID string, event *Event) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("publisher is closed")
	}
	p.mu.RUnlock()

	if err := common.ValidateRequiredParam("conversationID", conversationID); err != nil {
		return fmt.Errorf("conversationID cannot be empty")
	}

	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Get all connections for conversation participants
	connections, err := p.connRepo.GetConversationConnections(ctx, conversationID)
	if err != nil {
		p.logger.Error("failed to get conversation connections",
			zap.String("conversation_id", conversationID),
			zap.Error(err))
		return fmt.Errorf("failed to get conversation connections: %w", err)
	}

	// Use shared helper for connection publishing
	helper := NewPublishConnectionHelper(p.logger)
	logContext := map[string]interface{}{
		"conversation_id": conversationID,
	}
	return helper.PublishToConnections(ctx, connections, event, p.publishToConnection, logContext)

	return nil
}

// publishToConnection sends an event to a specific WebSocket connection
func (p *apiGatewayPublisher) publishToConnection(ctx context.Context, connectionID string, event *Event) error {
	start := time.Now()
	defer func() {
		p.mu.Lock()
		p.deliveryTime = append(p.deliveryTime, time.Since(start))
		// Keep only last 1000 delivery times for metrics
		if len(p.deliveryTime) > 1000 {
			p.deliveryTime = p.deliveryTime[len(p.deliveryTime)-1000:]
		}
		p.mu.Unlock()
	}()

	// Serialize event to JSON
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Send to connection via API Gateway Management API
	_, err = p.client.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: aws.String(connectionID),
		Data:         eventData,
	})

	if err != nil {
		return fmt.Errorf("failed to post to connection: %w", err)
	}

	return nil
}

// Close closes the publisher and cleans up resources
func (p *apiGatewayPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	p.logger.Info("API Gateway publisher closed")
	return nil
}

// GetMetrics returns delivery metrics for the publisher
func (p *apiGatewayPublisher) GetMetrics() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if err := common.ValidateSliceNotEmpty("p.deliveryTime", p.deliveryTime); err != nil {
		return map[string]interface{}{
			"total_deliveries":      0,
			"average_delivery_time": "0s",
			"min_delivery_time":     "0s",
			"max_delivery_time":     "0s",
		}
	}

	var total time.Duration
	minTime := p.deliveryTime[0]
	maxTime := p.deliveryTime[0]

	for _, dt := range p.deliveryTime {
		total += dt
		if dt < minTime {
			minTime = dt
		}
		if dt > maxTime {
			maxTime = dt
		}
	}

	avg := total / time.Duration(len(p.deliveryTime))

	return map[string]interface{}{
		"total_deliveries":      len(p.deliveryTime),
		"average_delivery_time": avg.String(),
		"min_delivery_time":     minTime.String(),
		"max_delivery_time":     maxTime.String(),
	}
}

// mockPublisher implements Publisher for testing
type mockPublisher struct {
	events       []MockPublishedEvent
	mu           sync.RWMutex
	closed       bool
	shouldError  bool
	errorMessage string
	delay        time.Duration
	failAfterN   int // Fail after N successful publishes
	publishCount int
}

// MockPublishedEvent represents an event that was published via the mock publisher
type MockPublishedEvent struct {
	Method      string    `json:"method"`    // "PublishToUser", "PublishToStream", "PublishToConversation"
	TargetID    string    `json:"target_id"` // userID, streamName, or conversationID
	Event       *Event    `json:"event"`
	PublishedAt time.Time `json:"published_at"`
}

// NewMockPublisher creates a new mock publisher for testing
func NewMockPublisher() Publisher {
	return &mockPublisher{
		events: make([]MockPublishedEvent, 0),
	}
}

// SetError configures the mock to return an error
func (m *mockPublisher) SetError(shouldError bool, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldError = shouldError
	m.errorMessage = message
}

// SetDelay configures the mock to add a delay to publishes
func (m *mockPublisher) SetDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = delay
}

// SetFailAfterN configures the mock to fail after N successful publishes
func (m *mockPublisher) SetFailAfterN(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failAfterN = n
}

// publishEvent is a consolidated helper for all publish methods
func (m *mockPublisher) publishEvent(method, targetID string, event *Event, streamName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("publisher is closed")
	}

	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	m.publishCount++
	if m.failAfterN > 0 && m.publishCount > m.failAfterN {
		return fmt.Errorf("mock configured to fail after %d publishes", m.failAfterN)
	}

	if m.shouldError {
		return fmt.Errorf("mock error: %s", m.errorMessage)
	}

	// Set timestamp if not provided
	eventCopy := *event
	if eventCopy.Timestamp.IsZero() {
		eventCopy.Timestamp = time.Now()
	}

	// Set stream if provided and not already set (for PublishToStream)
	if streamName != "" {
		if err := common.ValidateRequiredParam("eventCopy.Stream", eventCopy.Stream); err != nil {
			eventCopy.Stream = streamName
		}
	}

	m.events = append(m.events, MockPublishedEvent{
		Method:      method,
		TargetID:    targetID,
		Event:       &eventCopy,
		PublishedAt: time.Now(),
	})

	return nil
}

// PublishToUser records the publish call for testing
func (m *mockPublisher) PublishToUser(_ context.Context, userID string, event *Event) error {
	return m.publishEvent("PublishToUser", userID, event, "")
}

// PublishToStream records the publish call for testing
func (m *mockPublisher) PublishToStream(_ context.Context, streamName string, event *Event) error {
	return m.publishEvent("PublishToStream", streamName, event, streamName)
}

// PublishToConversation records the publish call for testing
func (m *mockPublisher) PublishToConversation(_ context.Context, conversationID string, event *Event) error {
	return m.publishEvent("PublishToConversation", conversationID, event, "")
}

// Close closes the mock publisher
func (m *mockPublisher) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// GetPublishedEvents returns all events published via the mock
func (m *mockPublisher) GetPublishedEvents() []MockPublishedEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	events := make([]MockPublishedEvent, len(m.events))
	copy(events, m.events)
	return events
}

// GetPublishedEventCount returns the number of published events
func (m *mockPublisher) GetPublishedEventCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.events)
}

// GetPublishedEventsForUser returns all events published to a specific user
func (m *mockPublisher) GetPublishedEventsForUser(userID string) []MockPublishedEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var userEvents []MockPublishedEvent
	for _, event := range m.events {
		if event.Method == "PublishToUser" && event.TargetID == userID {
			userEvents = append(userEvents, event)
		}
	}
	return userEvents
}

// GetPublishedEventsForStream returns all events published to a specific stream
func (m *mockPublisher) GetPublishedEventsForStream(streamName string) []MockPublishedEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var streamEvents []MockPublishedEvent
	for _, event := range m.events {
		if event.Method == "PublishToStream" && event.TargetID == streamName {
			streamEvents = append(streamEvents, event)
		}
	}
	return streamEvents
}

// GetPublishedEventsForConversation returns all events published to a specific conversation
func (m *mockPublisher) GetPublishedEventsForConversation(conversationID string) []MockPublishedEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var convEvents []MockPublishedEvent
	for _, event := range m.events {
		if event.Method == "PublishToConversation" && event.TargetID == conversationID {
			convEvents = append(convEvents, event)
		}
	}
	return convEvents
}

// Reset clears all recorded events (useful for testing)
func (m *mockPublisher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = make([]MockPublishedEvent, 0)
	m.publishCount = 0
	m.closed = false
	m.shouldError = false
	m.errorMessage = ""
	m.delay = 0
	m.failAfterN = 0
}

// IsClosed returns whether the publisher is closed
func (m *mockPublisher) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}
