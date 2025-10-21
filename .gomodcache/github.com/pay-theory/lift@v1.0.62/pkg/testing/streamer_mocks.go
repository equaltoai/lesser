package testing

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Streamer-Compatible API Gateway Client Mock
// =============================================================================

// These interfaces match the streamer team's exact interface definitions
// without importing streamer (which would create a circular dependency)

// StreamerAPIGatewayClient defines the interface that matches streamer's APIGatewayClient
type StreamerAPIGatewayClient interface {
	PostToConnection(ctx context.Context, connectionID string, data []byte) error
	DeleteConnection(ctx context.Context, connectionID string) error
	GetConnection(ctx context.Context, connectionID string) (*StreamerConnectionInfo, error)
}

// StreamerConnectionInfo matches streamer's ConnectionInfo struct
type StreamerConnectionInfo struct {
	ConnectionID string
	ConnectedAt  string
	LastActiveAt string
	SourceIP     string
	UserAgent    string
}

// StreamerAPIError matches streamer's APIError interface
type StreamerAPIError interface {
	error
	HTTPStatusCode() int
	ErrorCode() string
	IsRetryable() bool
}

// Streamer-compatible error types that match their exact implementation
type (
	StreamerGoneError struct {
		ConnectionID string
		Message      string
	}

	StreamerForbiddenError struct {
		ConnectionID string
		Message      string
	}

	StreamerPayloadTooLargeError struct {
		ConnectionID string
		Message      string
		PayloadSize  int
		MaxSize      int
	}

	StreamerThrottlingError struct {
		ConnectionID string
		Message      string
		RetryAfter   int
	}

	StreamerInternalServerError struct {
		Message string
	}
)

// Error implementations for streamer-compatible errors
func (e StreamerGoneError) Error() string       { return e.Message }
func (e StreamerGoneError) HTTPStatusCode() int { return 410 }
func (e StreamerGoneError) ErrorCode() string   { return "GoneException" }
func (e StreamerGoneError) IsRetryable() bool   { return false }

func (e StreamerForbiddenError) Error() string       { return e.Message }
func (e StreamerForbiddenError) HTTPStatusCode() int { return 403 }
func (e StreamerForbiddenError) ErrorCode() string   { return "ForbiddenException" }
func (e StreamerForbiddenError) IsRetryable() bool   { return false }

func (e StreamerPayloadTooLargeError) Error() string       { return e.Message }
func (e StreamerPayloadTooLargeError) HTTPStatusCode() int { return 413 }
func (e StreamerPayloadTooLargeError) ErrorCode() string   { return "PayloadTooLargeException" }
func (e StreamerPayloadTooLargeError) IsRetryable() bool   { return false }

func (e StreamerThrottlingError) Error() string       { return e.Message }
func (e StreamerThrottlingError) HTTPStatusCode() int { return 429 }
func (e StreamerThrottlingError) ErrorCode() string   { return "ThrottlingException" }
func (e StreamerThrottlingError) IsRetryable() bool   { return true }

func (e StreamerInternalServerError) Error() string       { return e.Message }
func (e StreamerInternalServerError) HTTPStatusCode() int { return 500 }
func (e StreamerInternalServerError) ErrorCode() string   { return "InternalServerError" }
func (e StreamerInternalServerError) IsRetryable() bool   { return true }

// StreamerAPIGatewayClientMock implements the StreamerAPIGatewayClient interface
type StreamerAPIGatewayClientMock struct {
	connections map[string]*StreamerMockConnection
	messages    map[string][][]byte
	errors      map[string]error
	callCount   map[string]int
	config      *StreamerMockConfig
	mu          sync.RWMutex
}

// StreamerMockConnection represents a WebSocket connection in the streamer-compatible mock
type StreamerMockConnection struct {
	CreatedTime  time.Time
	Metadata     map[string]any
	ConnectionID string
	ConnectedAt  string
	LastActiveAt string
	SourceIP     string
	UserAgent    string
	State        StreamerConnectionState
}

// StreamerConnectionState represents the state of a connection
type StreamerConnectionState string

const (
	StreamerConnectionStateActive       StreamerConnectionState = "ACTIVE"
	StreamerConnectionStateDisconnected StreamerConnectionState = "DISCONNECTED"
	StreamerConnectionStateStale        StreamerConnectionState = "STALE"
)

// StreamerMockConfig configures the behavior of the streamer-compatible mock
type StreamerMockConfig struct {
	DefaultSourceIP  string
	DefaultUserAgent string
	ConnectionTTL    int64
	MaxMessageSize   int64
	NetworkDelay     time.Duration
}

// DefaultStreamerMockConfig returns default configuration for streamer mocks
func DefaultStreamerMockConfig() *StreamerMockConfig {
	return &StreamerMockConfig{
		ConnectionTTL:    7200,   // 2 hours
		MaxMessageSize:   131072, // 128KB
		NetworkDelay:     0,
		DefaultSourceIP:  "127.0.0.1",
		DefaultUserAgent: "MockClient/1.0",
	}
}

// NewStreamerAPIGatewayClientMock creates a new streamer-compatible API Gateway client mock
func NewStreamerAPIGatewayClientMock() *StreamerAPIGatewayClientMock {
	return &StreamerAPIGatewayClientMock{
		connections: make(map[string]*StreamerMockConnection),
		messages:    make(map[string][][]byte),
		errors:      make(map[string]error),
		callCount:   make(map[string]int),
		config:      DefaultStreamerMockConfig(),
	}
}

// Ensure StreamerAPIGatewayClientMock implements StreamerAPIGatewayClient interface
var _ StreamerAPIGatewayClient = (*StreamerAPIGatewayClientMock)(nil)

// WithConfig sets the mock configuration
func (m *StreamerAPIGatewayClientMock) WithConfig(config *StreamerMockConfig) *StreamerAPIGatewayClientMock {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return m
}

// WithConnection adds a connection to the mock
func (m *StreamerAPIGatewayClientMock) WithConnection(connectionID string, conn *StreamerMockConnection) *StreamerAPIGatewayClientMock {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn == nil {
		now := time.Now()
		conn = &StreamerMockConnection{
			ConnectionID: connectionID,
			ConnectedAt:  now.Format(time.RFC3339),
			LastActiveAt: now.Format(time.RFC3339),
			SourceIP:     m.config.DefaultSourceIP,
			UserAgent:    m.config.DefaultUserAgent,
			State:        StreamerConnectionStateActive,
			CreatedTime:  now,
			Metadata:     make(map[string]any),
		}
	}

	m.connections[connectionID] = conn
	return m
}

// WithError configures an error for a specific connection
func (m *StreamerAPIGatewayClientMock) WithError(connectionID string, err error) *StreamerAPIGatewayClientMock {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[connectionID] = err
	return m
}

// PostToConnection sends data to a WebSocket connection
// Implements StreamerAPIGatewayClient interface
func (m *StreamerAPIGatewayClientMock) PostToConnection(_ context.Context, connectionID string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track call count
	m.callCount["PostToConnection"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors[connectionID]; exists {
		return err
	}

	// Check if connection exists
	conn, exists := m.connections[connectionID]
	if !exists {
		return StreamerGoneError{
			ConnectionID: connectionID,
			Message:      fmt.Sprintf("Connection %s not found", connectionID),
		}
	}

	// Check connection state
	if conn.State != StreamerConnectionStateActive {
		return StreamerGoneError{
			ConnectionID: connectionID,
			Message:      fmt.Sprintf("Connection %s is not active", connectionID),
		}
	}

	// Check if connection is stale (TTL expired)
	if time.Since(conn.CreatedTime).Seconds() > float64(m.config.ConnectionTTL) {
		conn.State = StreamerConnectionStateStale
		return StreamerGoneError{
			ConnectionID: connectionID,
			Message:      fmt.Sprintf("Connection %s has expired", connectionID),
		}
	}

	// Check message size
	if int64(len(data)) > m.config.MaxMessageSize {
		return StreamerPayloadTooLargeError{
			ConnectionID: connectionID,
			PayloadSize:  len(data),
			MaxSize:      int(m.config.MaxMessageSize),
			Message:      fmt.Sprintf("Message size %d exceeds maximum %d", len(data), m.config.MaxMessageSize),
		}
	}

	// Store the message
	m.messages[connectionID] = append(m.messages[connectionID], data)

	// Update last active time
	now := time.Now()
	conn.LastActiveAt = now.Format(time.RFC3339)

	return nil
}

// DeleteConnection terminates a WebSocket connection
// Implements StreamerAPIGatewayClient interface
func (m *StreamerAPIGatewayClientMock) DeleteConnection(_ context.Context, connectionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Track call count
	m.callCount["DeleteConnection"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors[connectionID]; exists {
		return err
	}

	// Check if connection exists
	conn, exists := m.connections[connectionID]
	if !exists {
		return StreamerGoneError{
			ConnectionID: connectionID,
			Message:      fmt.Sprintf("Connection %s not found", connectionID),
		}
	}

	// Mark connection as disconnected
	conn.State = StreamerConnectionStateDisconnected
	now := time.Now()
	conn.LastActiveAt = now.Format(time.RFC3339)

	return nil
}

// GetConnection retrieves connection information
// Implements StreamerAPIGatewayClient interface
func (m *StreamerAPIGatewayClientMock) GetConnection(_ context.Context, connectionID string) (*StreamerConnectionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Track call count
	m.callCount["GetConnection"]++

	// Simulate network delay
	if m.config.NetworkDelay > 0 {
		time.Sleep(m.config.NetworkDelay)
	}

	// Check for configured error
	if err, exists := m.errors[connectionID]; exists {
		return nil, err
	}

	// Check if connection exists
	conn, exists := m.connections[connectionID]
	if !exists {
		return nil, StreamerGoneError{
			ConnectionID: connectionID,
			Message:      fmt.Sprintf("Connection %s not found", connectionID),
		}
	}

	// Return connection info using the exact streamer ConnectionInfo type
	return &StreamerConnectionInfo{
		ConnectionID: conn.ConnectionID,
		ConnectedAt:  conn.ConnectedAt,
		LastActiveAt: conn.LastActiveAt,
		SourceIP:     conn.SourceIP,
		UserAgent:    conn.UserAgent,
	}, nil
}

// =============================================================================
// Helper Methods for Testing
// =============================================================================

// GetMessages returns all messages sent to a connection
func (m *StreamerAPIGatewayClientMock) GetMessages(connectionID string) [][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	messages, exists := m.messages[connectionID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([][]byte, len(messages))
	for i, msg := range messages {
		result[i] = make([]byte, len(msg))
		copy(result[i], msg)
	}

	return result
}

// GetMessageCount returns the number of messages sent to a connection
func (m *StreamerAPIGatewayClientMock) GetMessageCount(connectionID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.messages[connectionID])
}

// GetCallCount returns the number of times an operation was called
func (m *StreamerAPIGatewayClientMock) GetCallCount(operation string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount[operation]
}

// GetConnectionState returns a copy of the connection state
func (m *StreamerAPIGatewayClientMock) GetConnectionState(connectionID string) *StreamerMockConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[connectionID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	connCopy := *conn
	if conn.Metadata != nil {
		connCopy.Metadata = make(map[string]any)
		for k, v := range conn.Metadata {
			connCopy.Metadata[k] = v
		}
	}

	return &connCopy
}

// GetActiveConnections returns all active connections
func (m *StreamerAPIGatewayClientMock) GetActiveConnections() map[string]*StreamerMockConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make(map[string]*StreamerMockConnection)
	for id, conn := range m.connections {
		if conn.State == StreamerConnectionStateActive {
			// Return a copy
			connCopy := *conn
			if conn.Metadata != nil {
				connCopy.Metadata = make(map[string]any)
				for k, v := range conn.Metadata {
					connCopy.Metadata[k] = v
				}
			}
			active[id] = &connCopy
		}
	}

	return active
}

// Reset clears all mock state
func (m *StreamerAPIGatewayClientMock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connections = make(map[string]*StreamerMockConnection)
	m.messages = make(map[string][][]byte)
	m.errors = make(map[string]error)
	m.callCount = make(map[string]int)
	m.config = DefaultStreamerMockConfig()
}

// SimulateConnectionExpiry marks connections as stale based on TTL
func (m *StreamerAPIGatewayClientMock) SimulateConnectionExpiry() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, conn := range m.connections {
		if conn.State == StreamerConnectionStateActive &&
			now.Sub(conn.CreatedTime).Seconds() > float64(m.config.ConnectionTTL) {
			conn.State = StreamerConnectionStateStale
		}
	}
}

// =============================================================================
// Error Helper Functions
// =============================================================================

// WithGoneError configures a GoneError for a specific connection
func (m *StreamerAPIGatewayClientMock) WithGoneError(connectionID, message string) *StreamerAPIGatewayClientMock {
	return m.WithError(connectionID, StreamerGoneError{
		ConnectionID: connectionID,
		Message:      message,
	})
}

// WithForbiddenError configures a ForbiddenError for a specific connection
func (m *StreamerAPIGatewayClientMock) WithForbiddenError(connectionID, message string) *StreamerAPIGatewayClientMock {
	return m.WithError(connectionID, StreamerForbiddenError{
		ConnectionID: connectionID,
		Message:      message,
	})
}

// WithPayloadTooLargeError configures a PayloadTooLargeError for a specific connection
func (m *StreamerAPIGatewayClientMock) WithPayloadTooLargeError(connectionID string, payloadSize, maxSize int, message string) *StreamerAPIGatewayClientMock {
	return m.WithError(connectionID, StreamerPayloadTooLargeError{
		ConnectionID: connectionID,
		PayloadSize:  payloadSize,
		MaxSize:      maxSize,
		Message:      message,
	})
}

// WithThrottlingError configures a ThrottlingError for a specific connection
func (m *StreamerAPIGatewayClientMock) WithThrottlingError(connectionID string, retryAfter int, message string) *StreamerAPIGatewayClientMock {
	return m.WithError(connectionID, StreamerThrottlingError{
		ConnectionID: connectionID,
		RetryAfter:   retryAfter,
		Message:      message,
	})
}

// WithInternalServerError configures an InternalServerError for a specific connection
func (m *StreamerAPIGatewayClientMock) WithInternalServerError(connectionID, message string) *StreamerAPIGatewayClientMock {
	return m.WithError(connectionID, StreamerInternalServerError{
		Message: message,
	})
}
