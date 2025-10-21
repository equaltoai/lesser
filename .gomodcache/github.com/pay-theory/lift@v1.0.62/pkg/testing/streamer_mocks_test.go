package testing

import (
	"context"
	"testing"
	"time"
)

const (
	goneExceptionCode = "GoneException"
)

// TestStreamerAPIGatewayClientMock demonstrates the interface compatibility
func TestStreamerAPIGatewayClientMock(t *testing.T) {
	ctx := context.Background()
	mock := NewStreamerAPIGatewayClientMock()

	// Test PostToConnection with non-existent connection
	err := mock.PostToConnection(ctx, "non-existent", []byte("test message"))
	if err == nil {
		t.Error("Expected error for non-existent connection")
	}

	// Verify it returns the correct error type
	if goneErr, ok := err.(StreamerGoneError); ok {
		if goneErr.ConnectionID != "non-existent" {
			t.Errorf("Expected connection ID 'non-existent', got %s", goneErr.ConnectionID)
		}
		if goneErr.HTTPStatusCode() != 410 {
			t.Errorf("Expected status code 410, got %d", goneErr.HTTPStatusCode())
		}
		if goneErr.ErrorCode() != goneExceptionCode {
			t.Errorf("Expected error code 'GoneException', got %s", goneErr.ErrorCode())
		}
		if goneErr.IsRetryable() {
			t.Error("Expected GoneError to not be retryable")
		}
	} else {
		t.Errorf("Expected StreamerGoneError, got %T", err)
	}

	// Add a connection
	mock.WithConnection("user-123", nil)

	// Test successful PostToConnection
	message := []byte("Hello WebSocket!")
	err = mock.PostToConnection(ctx, "user-123", message)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify message was stored
	messages := mock.GetMessages("user-123")
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
	if string(messages[0]) != string(message) {
		t.Errorf("Expected message %s, got %s", string(message), string(messages[0]))
	}

	// Test GetConnection - returns correct type
	connInfo, err := mock.GetConnection(ctx, "user-123")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if connInfo.ConnectionID != "user-123" {
		t.Errorf("Expected connection ID user-123, got %s", connInfo.ConnectionID)
	}

	// Verify the returned type is exactly StreamerConnectionInfo
	if connInfo == nil {
		t.Error("Expected non-nil connection info")
	} else {
		// Test all required fields are present
		if connInfo.ConnectionID == "" {
			t.Error("ConnectionID should not be empty")
		}
		if connInfo.ConnectedAt == "" {
			t.Error("ConnectedAt should not be empty")
		}
		if connInfo.LastActiveAt == "" {
			t.Error("LastActiveAt should not be empty")
		}
		if connInfo.SourceIP == "" {
			t.Error("SourceIP should not be empty")
		}
		if connInfo.UserAgent == "" {
			t.Error("UserAgent should not be empty")
		}
	}

	// Test DeleteConnection
	err = mock.DeleteConnection(ctx, "user-123")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify connection state changed
	conn := mock.GetConnectionState("user-123")
	if conn.State != StreamerConnectionStateDisconnected {
		t.Errorf("Expected connection state %s, got %s", StreamerConnectionStateDisconnected, conn.State)
	}

	// Test call counting
	if mock.GetCallCount("PostToConnection") != 2 {
		t.Errorf("Expected 2 PostToConnection calls, got %d", mock.GetCallCount("PostToConnection"))
	}
	if mock.GetCallCount("DeleteConnection") != 1 {
		t.Errorf("Expected 1 DeleteConnection call, got %d", mock.GetCallCount("DeleteConnection"))
	}
	if mock.GetCallCount("GetConnection") != 1 {
		t.Errorf("Expected 1 GetConnection call, got %d", mock.GetCallCount("GetConnection"))
	}
}

// TestStreamerErrorTypes tests all the streamer-compatible error types
func TestStreamerErrorTypes(t *testing.T) {
	ctx := context.Background()
	mock := NewStreamerAPIGatewayClientMock()

	// Test GoneError
	mock.WithGoneError("conn-gone", "Connection is gone")
	err := mock.PostToConnection(ctx, "conn-gone", []byte("test"))
	if goneErr, ok := err.(StreamerGoneError); ok {
		if goneErr.HTTPStatusCode() != 410 {
			t.Errorf("Expected 410, got %d", goneErr.HTTPStatusCode())
		}
		if goneErr.ErrorCode() != goneExceptionCode {
			t.Errorf("Expected GoneException, got %s", goneErr.ErrorCode())
		}
		if goneErr.IsRetryable() {
			t.Error("GoneError should not be retryable")
		}
	} else {
		t.Errorf("Expected StreamerGoneError, got %T", err)
	}

	// Test ForbiddenError
	mock.WithForbiddenError("conn-forbidden", "Access denied")
	err = mock.PostToConnection(ctx, "conn-forbidden", []byte("test"))
	if forbiddenErr, ok := err.(StreamerForbiddenError); ok {
		if forbiddenErr.HTTPStatusCode() != 403 {
			t.Errorf("Expected 403, got %d", forbiddenErr.HTTPStatusCode())
		}
		if forbiddenErr.ErrorCode() != "ForbiddenException" {
			t.Errorf("Expected ForbiddenException, got %s", forbiddenErr.ErrorCode())
		}
		if forbiddenErr.IsRetryable() {
			t.Error("ForbiddenError should not be retryable")
		}
	} else {
		t.Errorf("Expected StreamerForbiddenError, got %T", err)
	}

	// Test PayloadTooLargeError
	mock.WithPayloadTooLargeError("conn-large", 1000, 500, "Payload too large")
	err = mock.PostToConnection(ctx, "conn-large", []byte("test"))
	if payloadErr, ok := err.(StreamerPayloadTooLargeError); ok {
		if payloadErr.HTTPStatusCode() != 413 {
			t.Errorf("Expected 413, got %d", payloadErr.HTTPStatusCode())
		}
		if payloadErr.ErrorCode() != "PayloadTooLargeException" {
			t.Errorf("Expected PayloadTooLargeException, got %s", payloadErr.ErrorCode())
		}
		if payloadErr.IsRetryable() {
			t.Error("PayloadTooLargeError should not be retryable")
		}
		if payloadErr.PayloadSize != 1000 {
			t.Errorf("Expected payload size 1000, got %d", payloadErr.PayloadSize)
		}
		if payloadErr.MaxSize != 500 {
			t.Errorf("Expected max size 500, got %d", payloadErr.MaxSize)
		}
	} else {
		t.Errorf("Expected StreamerPayloadTooLargeError, got %T", err)
	}

	// Test ThrottlingError
	mock.WithThrottlingError("conn-throttle", 30, "Rate limit exceeded")
	err = mock.PostToConnection(ctx, "conn-throttle", []byte("test"))
	if throttleErr, ok := err.(StreamerThrottlingError); ok {
		if throttleErr.HTTPStatusCode() != 429 {
			t.Errorf("Expected 429, got %d", throttleErr.HTTPStatusCode())
		}
		if throttleErr.ErrorCode() != "ThrottlingException" {
			t.Errorf("Expected ThrottlingException, got %s", throttleErr.ErrorCode())
		}
		if !throttleErr.IsRetryable() {
			t.Error("ThrottlingError should be retryable")
		}
		if throttleErr.RetryAfter != 30 {
			t.Errorf("Expected retry after 30, got %d", throttleErr.RetryAfter)
		}
	} else {
		t.Errorf("Expected StreamerThrottlingError, got %T", err)
	}

	// Test InternalServerError
	mock.WithInternalServerError("conn-server", "Internal server error")
	err = mock.PostToConnection(ctx, "conn-server", []byte("test"))
	if serverErr, ok := err.(StreamerInternalServerError); ok {
		if serverErr.HTTPStatusCode() != 500 {
			t.Errorf("Expected 500, got %d", serverErr.HTTPStatusCode())
		}
		if serverErr.ErrorCode() != "InternalServerError" {
			t.Errorf("Expected InternalServerError, got %s", serverErr.ErrorCode())
		}
		if !serverErr.IsRetryable() {
			t.Error("InternalServerError should be retryable")
		}
	} else {
		t.Errorf("Expected StreamerInternalServerError, got %T", err)
	}
}

// TestStreamerMockConfiguration tests the configuration options
func TestStreamerMockConfiguration(t *testing.T) {
	ctx := context.Background()
	mock := NewStreamerAPIGatewayClientMock()

	// Test message size limit
	config := DefaultStreamerMockConfig()
	config.MaxMessageSize = 10
	mock.WithConfig(config)
	mock.WithConnection("conn-123", nil)

	largeMessage := make([]byte, 20)
	err := mock.PostToConnection(ctx, "conn-123", largeMessage)
	if err == nil {
		t.Error("Expected error for oversized message")
	}

	// Should be PayloadTooLargeError
	if payloadErr, ok := err.(StreamerPayloadTooLargeError); ok {
		if payloadErr.PayloadSize != 20 {
			t.Errorf("Expected payload size 20, got %d", payloadErr.PayloadSize)
		}
		if payloadErr.MaxSize != 10 {
			t.Errorf("Expected max size 10, got %d", payloadErr.MaxSize)
		}
	} else {
		t.Errorf("Expected StreamerPayloadTooLargeError, got %T", err)
	}
}

// TestStreamerMockTTL tests connection TTL functionality
func TestStreamerMockTTL(t *testing.T) {
	ctx := context.Background()
	mock := NewStreamerAPIGatewayClientMock()

	// Set very short TTL for testing
	config := DefaultStreamerMockConfig()
	config.ConnectionTTL = 1 // 1 second
	mock.WithConfig(config)

	// Add connection with past creation time
	pastTime := time.Now().Add(-2 * time.Second)
	conn := &StreamerMockConnection{
		ConnectionID: "conn-expired",
		ConnectedAt:  pastTime.Format(time.RFC3339),
		LastActiveAt: pastTime.Format(time.RFC3339),
		SourceIP:     "127.0.0.1",
		UserAgent:    "TestClient/1.0",
		State:        StreamerConnectionStateActive,
		CreatedTime:  pastTime,
		Metadata:     make(map[string]any),
	}
	mock.WithConnection("conn-expired", conn)

	// Try to send message to expired connection
	err := mock.PostToConnection(ctx, "conn-expired", []byte("test"))
	if err == nil {
		t.Error("Expected error for expired connection")
	}

	// Should be GoneError
	if goneErr, ok := err.(StreamerGoneError); ok {
		if goneErr.ConnectionID != "conn-expired" {
			t.Errorf("Expected connection ID 'conn-expired', got %s", goneErr.ConnectionID)
		}
	} else {
		t.Errorf("Expected StreamerGoneError, got %T", err)
	}

	// Verify connection state changed to stale
	updatedConn := mock.GetConnectionState("conn-expired")
	if updatedConn.State != StreamerConnectionStateStale {
		t.Errorf("Expected connection state %s, got %s", StreamerConnectionStateStale, updatedConn.State)
	}
}

// TestStreamerMockReset tests the reset functionality
func TestStreamerMockReset(t *testing.T) {
	ctx := context.Background()
	mock := NewStreamerAPIGatewayClientMock()

	// Add connection and send message
	mock.WithConnection("conn-123", nil)
	mock.PostToConnection(ctx, "conn-123", []byte("test"))

	if len(mock.GetActiveConnections()) == 0 {
		t.Error("Expected active connections before reset")
	}
	if mock.GetCallCount("PostToConnection") == 0 {
		t.Error("Expected call count before reset")
	}

	// Reset
	mock.Reset()

	if len(mock.GetActiveConnections()) != 0 {
		t.Error("Expected no active connections after reset")
	}
	if mock.GetCallCount("PostToConnection") != 0 {
		t.Error("Expected call count to be reset")
	}
}

// TestStreamerInterfaceCompatibility demonstrates that the mock can be used
// anywhere the streamer's APIGatewayClient interface is expected
func TestStreamerInterfaceCompatibility(t *testing.T) {
	// This function demonstrates that our mock implements the exact interface
	// that the streamer team expects
	var client StreamerAPIGatewayClient = NewStreamerAPIGatewayClientMock()

	ctx := context.Background()

	// These calls should compile and work exactly like the real implementation
	err := client.PostToConnection(ctx, "test-conn", []byte("test message"))
	if err == nil {
		t.Error("Expected error for non-existent connection")
	}

	err = client.DeleteConnection(ctx, "test-conn")
	if err == nil {
		t.Error("Expected error for non-existent connection")
	}

	_, err = client.GetConnection(ctx, "test-conn")
	if err == nil {
		t.Error("Expected error for non-existent connection")
	}

	// All errors should implement StreamerAPIError interface
	if apiErr, ok := err.(StreamerAPIError); ok {
		if apiErr.HTTPStatusCode() == 0 {
			t.Error("Expected non-zero HTTP status code")
		}
		if apiErr.ErrorCode() == "" {
			t.Error("Expected non-empty error code")
		}
		// IsRetryable() can be true or false, both are valid
	} else {
		t.Errorf("Expected error to implement StreamerAPIError interface, got %T", err)
	}
}
