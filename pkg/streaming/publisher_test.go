package streaming

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
)

// Mock API Gateway Management API client
type mockAPIGatewayClient struct {
	mock.Mock
}

func (m *mockAPIGatewayClient) PostToConnection(ctx context.Context, input *apigatewaymanagementapi.PostToConnectionInput, optFns ...func(*apigatewaymanagementapi.Options)) (*apigatewaymanagementapi.PostToConnectionOutput, error) {
	args := m.Called(ctx, input)
	if output := args.Get(0); output != nil {
		return output.(*apigatewaymanagementapi.PostToConnectionOutput), args.Error(1)
	}
	return nil, args.Error(1)
}

// Mock connection repository
type mockConnectionRepository struct {
	mock.Mock
}

func (m *mockConnectionRepository) GetUserConnections(ctx context.Context, userID string) ([]*StreamConnection, error) {
	args := m.Called(ctx, userID)
	if connections := args.Get(0); connections != nil {
		return connections.([]*StreamConnection), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockConnectionRepository) GetStreamConnections(ctx context.Context, streamName string) ([]*StreamConnection, error) {
	args := m.Called(ctx, streamName)
	if connections := args.Get(0); connections != nil {
		return connections.([]*StreamConnection), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockConnectionRepository) GetConversationConnections(ctx context.Context, conversationID string) ([]*StreamConnection, error) {
	args := m.Called(ctx, conversationID)
	if connections := args.Get(0); connections != nil {
		return connections.([]*StreamConnection), args.Error(1)
	}
	return nil, args.Error(1)
}

func TestEvent_Marshal(t *testing.T) {
	event := &Event{
		Type:      StatusCreated,
		Stream:    "user:alice",
		Payload:   map[string]interface{}{"status_id": "123", "content": "Hello world"},
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(event)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"type":"status.created"`)
	assert.Contains(t, string(data), `"stream":"user:alice"`)
	assert.Contains(t, string(data), `"status_id":"123"`)
}

func TestNewAPIGatewayPublisher(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)
	assert.NotNil(t, publisher)
	
	// Test with nil logger
	publisherNoLog := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", nil)
	assert.NotNil(t, publisherNoLog)
}

func TestAPIGatewayPublisher_PublishToUser_Success(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	// Setup mock connections
	connections := []*StreamConnection{
		{ConnectionID: "conn1", UserID: "user1", Streams: []string{"user"}},
		{ConnectionID: "conn2", UserID: "user1", Streams: []string{"user", "public"}},
	}
	repo.On("GetUserConnections", mock.Anything, "user1").Return(connections, nil)

	// Setup mock API Gateway calls
	client.On("PostToConnection", mock.Anything, mock.MatchedBy(func(input *apigatewaymanagementapi.PostToConnectionInput) bool {
		return *input.ConnectionId == "conn1" || *input.ConnectionId == "conn2"
	})).Return(&apigatewaymanagementapi.PostToConnectionOutput{}, nil)

	event := &Event{
		Type:    StatusCreated,
		Stream:  "user:user1",
		Payload: map[string]interface{}{"status_id": "123"},
	}

	err := publisher.PublishToUser(context.Background(), "user1", event)
	assert.NoError(t, err)
	assert.False(t, event.Timestamp.IsZero()) // Should set timestamp

	repo.AssertExpectations(t)
	client.AssertExpectations(t)
}

func TestAPIGatewayPublisher_PublishToUser_NoConnections(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	// Setup mock to return no connections
	repo.On("GetUserConnections", mock.Anything, "user1").Return([]*StreamConnection{}, nil)

	event := &Event{
		Type:    StatusCreated,
		Stream:  "user:user1",
		Payload: map[string]interface{}{"status_id": "123"},
	}

	err := publisher.PublishToUser(context.Background(), "user1", event)
	assert.NoError(t, err) // Should not error when no connections

	repo.AssertExpectations(t)
}

func TestAPIGatewayPublisher_PublishToUser_ValidationErrors(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	// Test empty userID
	err := publisher.PublishToUser(context.Background(), "", event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "userID cannot be empty")

	// Test nil event
	err = publisher.PublishToUser(context.Background(), "user1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event cannot be nil")
}

func TestAPIGatewayPublisher_PublishToUser_RepositoryError(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	// Setup mock to return error
	repo.On("GetUserConnections", mock.Anything, "user1").Return(nil, errors.New("repository error"))

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	err := publisher.PublishToUser(context.Background(), "user1", event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get user connections")

	repo.AssertExpectations(t)
}

func TestAPIGatewayPublisher_PublishToUser_PartialFailure(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	connections := []*StreamConnection{
		{ConnectionID: "conn1", UserID: "user1"},
		{ConnectionID: "conn2", UserID: "user1"},
	}
	repo.On("GetUserConnections", mock.Anything, "user1").Return(connections, nil)

	// First connection succeeds, second fails
	client.On("PostToConnection", mock.Anything, mock.MatchedBy(func(input *apigatewaymanagementapi.PostToConnectionInput) bool {
		return *input.ConnectionId == "conn1"
	})).Return(&apigatewaymanagementapi.PostToConnectionOutput{}, nil)
	
	client.On("PostToConnection", mock.Anything, mock.MatchedBy(func(input *apigatewaymanagementapi.PostToConnectionInput) bool {
		return *input.ConnectionId == "conn2"
	})).Return(nil, errors.New("connection failed"))

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	err := publisher.PublishToUser(context.Background(), "user1", event)
	assert.NoError(t, err) // Should succeed if at least one delivery succeeds

	repo.AssertExpectations(t)
	client.AssertExpectations(t)
}

func TestAPIGatewayPublisher_PublishToUser_AllFailures(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	connections := []*StreamConnection{
		{ConnectionID: "conn1", UserID: "user1"},
	}
	repo.On("GetUserConnections", mock.Anything, "user1").Return(connections, nil)

	// Connection fails
	client.On("PostToConnection", mock.Anything, mock.Anything).Return(nil, errors.New("all connections failed"))

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	err := publisher.PublishToUser(context.Background(), "user1", event)
	assert.Error(t, err) // Should fail if all deliveries fail
	assert.Contains(t, err.Error(), "failed to publish to any connection")

	repo.AssertExpectations(t)
	client.AssertExpectations(t)
}

func TestAPIGatewayPublisher_PublishToStream_Success(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	connections := []*StreamConnection{
		{ConnectionID: "conn1", UserID: "user1", Streams: []string{"public"}},
		{ConnectionID: "conn2", UserID: "user2", Streams: []string{"public"}},
	}
	repo.On("GetStreamConnections", mock.Anything, "public").Return(connections, nil)

	client.On("PostToConnection", mock.Anything, mock.Anything).Return(&apigatewaymanagementapi.PostToConnectionOutput{}, nil)

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	err := publisher.PublishToStream(context.Background(), "public", event)
	assert.NoError(t, err)
	assert.Equal(t, "public", event.Stream) // Should set stream name

	repo.AssertExpectations(t)
	client.AssertExpectations(t)
}

func TestAPIGatewayPublisher_PublishToConversation_Success(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	connections := []*StreamConnection{
		{ConnectionID: "conn1", UserID: "user1"},
		{ConnectionID: "conn2", UserID: "user2"},
	}
	repo.On("GetConversationConnections", mock.Anything, "conv123").Return(connections, nil)

	client.On("PostToConnection", mock.Anything, mock.Anything).Return(&apigatewaymanagementapi.PostToConnectionOutput{}, nil)

	event := &Event{
		Type:    ConversationUpdated,
		Payload: map[string]interface{}{"conversation_id": "conv123"},
	}

	err := publisher.PublishToConversation(context.Background(), "conv123", event)
	assert.NoError(t, err)

	repo.AssertExpectations(t)
	client.AssertExpectations(t)
}

func TestAPIGatewayPublisher_Close(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	err := publisher.Close()
	assert.NoError(t, err)

	// Should not be able to publish after closing
	event := &Event{Type: StatusCreated, Payload: map[string]interface{}{}}
	err = publisher.PublishToUser(context.Background(), "user1", event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publisher is closed")
}

func TestAPIGatewayPublisher_GetMetrics(t *testing.T) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(t)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger).(*apiGatewayPublisher)

	// Test empty metrics
	metrics := publisher.GetMetrics()
	assert.Equal(t, 0, metrics["total_deliveries"])

	// Add some delivery times manually for testing
	publisher.deliveryTime = []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
	}

	metrics = publisher.GetMetrics()
	assert.Equal(t, 3, metrics["total_deliveries"])
	assert.Equal(t, "200ms", metrics["average_delivery_time"])
	assert.Equal(t, "100ms", metrics["min_delivery_time"])
	assert.Equal(t, "300ms", metrics["max_delivery_time"])
}

func TestMockPublisher_PublishToUser(t *testing.T) {
	publisher := NewMockPublisher()
	mock := publisher.(*mockPublisher)

	event := &Event{
		Type:    StatusCreated,
		Stream:  "user:alice",
		Payload: map[string]interface{}{"status_id": "123"},
	}

	err := publisher.PublishToUser(context.Background(), "alice", event)
	assert.NoError(t, err)

	events := mock.GetPublishedEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, "PublishToUser", events[0].Method)
	assert.Equal(t, "alice", events[0].TargetID)
	assert.Equal(t, StatusCreated, events[0].Event.Type)
	assert.False(t, events[0].Event.Timestamp.IsZero())
}

func TestMockPublisher_PublishToStream(t *testing.T) {
	publisher := NewMockPublisher()
	mock := publisher.(*mockPublisher)

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	err := publisher.PublishToStream(context.Background(), "public", event)
	assert.NoError(t, err)

	events := mock.GetPublishedEventsForStream("public")
	assert.Len(t, events, 1)
	assert.Equal(t, "public", events[0].Event.Stream) // Should set stream name
}

func TestMockPublisher_PublishToConversation(t *testing.T) {
	publisher := NewMockPublisher()
	mock := publisher.(*mockPublisher)

	event := &Event{
		Type:    ConversationUpdated,
		Payload: map[string]interface{}{"conversation_id": "conv123"},
	}

	err := publisher.PublishToConversation(context.Background(), "conv123", event)
	assert.NoError(t, err)

	events := mock.GetPublishedEventsForConversation("conv123")
	assert.Len(t, events, 1)
	assert.Equal(t, "PublishToConversation", events[0].Method)
}

func TestMockPublisher_SetError(t *testing.T) {
	publisher := NewMockPublisher()
	mock := publisher.(*mockPublisher)
	mock.SetError(true, "test error")

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	err := publisher.PublishToUser(context.Background(), "alice", event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test error")

	// Should not record failed events
	assert.Equal(t, 0, mock.GetPublishedEventCount())
}

func TestMockPublisher_SetDelay(t *testing.T) {
	publisher := NewMockPublisher()
	mock := publisher.(*mockPublisher)
	mock.SetDelay(100 * time.Millisecond)

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	start := time.Now()
	err := publisher.PublishToUser(context.Background(), "alice", event)
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.True(t, duration >= 100*time.Millisecond)
}

func TestMockPublisher_SetFailAfterN(t *testing.T) {
	publisher := NewMockPublisher()
	mock := publisher.(*mockPublisher)
	mock.SetFailAfterN(2)

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	// First two should succeed
	err := publisher.PublishToUser(context.Background(), "alice", event)
	assert.NoError(t, err)
	err = publisher.PublishToUser(context.Background(), "alice", event)
	assert.NoError(t, err)

	// Third should fail
	err = publisher.PublishToUser(context.Background(), "alice", event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fail after 2 publishes")

	assert.Equal(t, 2, mock.GetPublishedEventCount())
}

func TestMockPublisher_Close(t *testing.T) {
	publisher := NewMockPublisher()
	mock := publisher.(*mockPublisher)

	err := publisher.Close()
	assert.NoError(t, err)
	assert.True(t, mock.IsClosed())

	// Should not be able to publish after closing
	event := &Event{Type: StatusCreated, Payload: map[string]interface{}{}}
	err = publisher.PublishToUser(context.Background(), "alice", event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publisher is closed")
}

func TestMockPublisher_Reset(t *testing.T) {
	publisher := NewMockPublisher()
	mock := publisher.(*mockPublisher)
	mock.SetError(true, "test")
	mock.SetDelay(100 * time.Millisecond)

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	// This should fail due to error setting
	err := publisher.PublishToUser(context.Background(), "alice", event)
	assert.Error(t, err)

	// Reset and try again
	mock.Reset()
	err = publisher.PublishToUser(context.Background(), "alice", event)
	assert.NoError(t, err)

	assert.Equal(t, 1, mock.GetPublishedEventCount())
	assert.False(t, mock.IsClosed())
}

func TestMockPublisher_GetPublishedEventsFor(t *testing.T) {
	publisher := NewMockPublisher()
	mock := publisher.(*mockPublisher)

	event1 := &Event{Type: StatusCreated, Payload: map[string]interface{}{"status_id": "123"}}
	event2 := &Event{Type: StatusUpdated, Payload: map[string]interface{}{"status_id": "456"}}
	event3 := &Event{Type: ConversationUpdated, Payload: map[string]interface{}{"conversation_id": "conv1"}}

	err := publisher.PublishToUser(context.Background(), "alice", event1)
	assert.NoError(t, err)
	err = publisher.PublishToUser(context.Background(), "bob", event2)
	assert.NoError(t, err)
	err = publisher.PublishToStream(context.Background(), "public", event1)
	assert.NoError(t, err)
	err = publisher.PublishToConversation(context.Background(), "conv1", event3)
	assert.NoError(t, err)

	// Test filtering by user
	aliceEvents := mock.GetPublishedEventsForUser("alice")
	assert.Len(t, aliceEvents, 1)
	assert.Equal(t, "alice", aliceEvents[0].TargetID)

	bobEvents := mock.GetPublishedEventsForUser("bob")
	assert.Len(t, bobEvents, 1)
	assert.Equal(t, "bob", bobEvents[0].TargetID)

	// Test filtering by stream
	publicEvents := mock.GetPublishedEventsForStream("public")
	assert.Len(t, publicEvents, 1)
	assert.Equal(t, "public", publicEvents[0].TargetID)

	// Test filtering by conversation
	convEvents := mock.GetPublishedEventsForConversation("conv1")
	assert.Len(t, convEvents, 1)
	assert.Equal(t, "conv1", convEvents[0].TargetID)

	// Test total count
	assert.Equal(t, 4, mock.GetPublishedEventCount())
}

// Benchmarks

func BenchmarkAPIGatewayPublisher_PublishToUser(b *testing.B) {
	client := &mockAPIGatewayClient{}
	repo := &mockConnectionRepository{}
	logger := zaptest.NewLogger(b)

	publisher := NewAPIGatewayPublisher(client, repo, "wss://api.example.com", logger)

	connections := []*StreamConnection{
		{ConnectionID: "conn1", UserID: "user1"},
	}
	repo.On("GetUserConnections", mock.Anything, "user1").Return(connections, nil)
	client.On("PostToConnection", mock.Anything, mock.Anything).Return(&apigatewaymanagementapi.PostToConnectionOutput{}, nil)

	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = publisher.PublishToUser(ctx, "user1", event)
	}
}

func BenchmarkMockPublisher_PublishToUser(b *testing.B) {
	publisher := NewMockPublisher()
	event := &Event{
		Type:    StatusCreated,
		Payload: map[string]interface{}{"status_id": "123"},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = publisher.PublishToUser(ctx, "user1", event)
	}
}