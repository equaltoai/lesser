package lift

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocketRouting(t *testing.T) {
	tests := []struct {
		handler   WebSocketHandler
		name      string
		routeKey  string
		wantError bool
	}{
		{
			name:     "Connect route",
			routeKey: "$connect",
			handler: func(ctx *Context) error {
				return ctx.Status(200).JSON(map[string]string{"status": "connected"})
			},
			wantError: false,
		},
		{
			name:     "Disconnect route",
			routeKey: "$disconnect",
			handler: func(ctx *Context) error {
				return ctx.Status(200).JSON(map[string]string{"status": "disconnected"})
			},
			wantError: false,
		},
		{
			name:     "Custom route",
			routeKey: "sendMessage",
			handler: func(ctx *Context) error {
				return ctx.Status(200).JSON(map[string]string{"status": "message sent"})
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create app with WebSocket support
			app := New(WithWebSocketSupport())

			// Register handler
			app.WebSocket(tt.routeKey, tt.handler)

			// Verify route was registered
			handler := app.RouteWebSocket(tt.routeKey)
			assert.NotNil(t, handler, "Handler should be registered for route %s", tt.routeKey)
		})
	}
}

func TestWebSocketHandler(t *testing.T) {
	// Create app with WebSocket support
	app := New(WithWebSocketSupport())

	// Track handler calls
	connectCalled := false
	disconnectCalled := false
	messageCalled := false

	// Register handlers
	app.WebSocket("$connect", func(ctx *Context) error {
		connectCalled = true
		return ctx.Status(200).JSON(map[string]string{"status": "connected"})
	})

	app.WebSocket("$disconnect", func(ctx *Context) error {
		disconnectCalled = true
		return ctx.Status(200).JSON(map[string]string{"status": "disconnected"})
	})

	app.WebSocket("sendMessage", func(ctx *Context) error {
		messageCalled = true
		// For non-$connect routes, we can't send messages in the response
		// Just acknowledge the message was received
		return ctx.Status(200).JSON(map[string]string{"status": "message received"})
	})

	// Get the Lambda handler
	handler := app.WebSocketHandler()
	require.NotNil(t, handler)

	// Test connect event
	t.Run("Connect Event", func(t *testing.T) {
		event := createTestWebSocketEvent("$connect", "conn123", "")

		wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
		assert.True(t, ok, "Handler should be a WebSocket handler function")
		resp, err := wsHandler(context.Background(), event)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.True(t, connectCalled)
	})

	// Test disconnect event
	t.Run("Disconnect Event", func(t *testing.T) {
		event := createTestWebSocketEvent("$disconnect", "conn123", "")

		wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
		assert.True(t, ok, "Handler should be a WebSocket handler function")
		resp, err := wsHandler(context.Background(), event)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.True(t, disconnectCalled)
	})

	// Test custom message event
	t.Run("Custom Message Event", func(t *testing.T) {
		event := createTestWebSocketEvent("sendMessage", "conn123", `{"message":"hello"}`)

		wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
		assert.True(t, ok, "Handler should be a WebSocket handler function")
		resp, err := wsHandler(context.Background(), event)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.True(t, messageCalled)
	})

	// Test unhandled route
	t.Run("Unhandled Route", func(t *testing.T) {
		event := createTestWebSocketEvent("unknownRoute", "conn123", "")

		wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
		assert.True(t, ok, "Handler should be a WebSocket handler function")
		resp, err := wsHandler(context.Background(), event)

		assert.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
		assert.Contains(t, resp.Body, "No handler for route")
	})
}

func TestWebSocketWithMiddleware(t *testing.T) {
	// Create app with WebSocket support
	app := New(WithWebSocketSupport())

	// Track middleware execution
	middlewareOrder := []string{}

	// Add middleware
	app.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			middlewareOrder = append(middlewareOrder, "middleware1-before")
			err := next.Handle(ctx)
			middlewareOrder = append(middlewareOrder, "middleware1-after")
			return err
		})
	})

	app.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			middlewareOrder = append(middlewareOrder, "middleware2-before")
			err := next.Handle(ctx)
			middlewareOrder = append(middlewareOrder, "middleware2-after")
			return err
		})
	})

	// Register handler
	app.WebSocket("$connect", func(ctx *Context) error {
		middlewareOrder = append(middlewareOrder, "handler")
		return ctx.Status(200).JSON(map[string]string{"status": "connected"})
	})

	// Get handler and test
	handler := app.WebSocketHandler()
	event := createTestWebSocketEvent("$connect", "conn123", "")

	wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
	assert.True(t, ok, "Handler should be a WebSocket handler function")
	_, err := wsHandler(context.Background(), event)

	assert.NoError(t, err)
	assert.Equal(t, []string{
		"middleware1-before",
		"middleware2-before",
		"handler",
		"middleware2-after",
		"middleware1-after",
	}, middlewareOrder)
}

func TestWebSocketAutoConnectionManagement(t *testing.T) {
	// Mock connection store
	mockStore := &mockConnectionStore{
		connections: make(map[string]*Connection),
	}

	// Create app with auto connection management
	app := New(WithWebSocketSupport(WebSocketOptions{
		EnableAutoConnectionManagement: true,
		ConnectionStore:                mockStore,
	}))

	// Register handlers
	app.WebSocket("$connect", func(ctx *Context) error {
		// Set user context for connection storage
		ctx.SetUserID("user123")
		ctx.Set("tenant_id", "tenant456")
		return ctx.Status(200).JSON(map[string]string{"status": "connected"})
	})

	app.WebSocket("$disconnect", func(ctx *Context) error {
		return ctx.Status(200).JSON(map[string]string{"status": "disconnected"})
	})

	handler := app.WebSocketHandler()

	// Test connect - should auto-save connection
	t.Run("Auto Save Connection", func(t *testing.T) {
		event := createTestWebSocketEvent("$connect", "conn123", "")

		wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
		assert.True(t, ok, "Handler should be a WebSocket handler function")
		_, err := wsHandler(context.Background(), event)

		assert.NoError(t, err)
		assert.Len(t, mockStore.connections, 1)
		assert.Equal(t, "conn123", mockStore.connections["conn123"].ID)
		assert.Equal(t, "user123", mockStore.connections["conn123"].UserID)
		assert.Equal(t, "tenant456", mockStore.connections["conn123"].TenantID)
	})

	// Test disconnect - should auto-remove connection
	t.Run("Auto Remove Connection", func(t *testing.T) {
		event := createTestWebSocketEvent("$disconnect", "conn123", "")

		wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
		assert.True(t, ok, "Handler should be a WebSocket handler function")
		_, err := wsHandler(context.Background(), event)

		assert.NoError(t, err)
		assert.Len(t, mockStore.connections, 0)
	})
}

func TestWebSocketDefaultHandler(t *testing.T) {
	// Create app with default handler
	defaultCalled := false
	app := New(WithWebSocketSupport(WebSocketOptions{
		DefaultHandler: func(ctx *Context) error {
			defaultCalled = true
			return ctx.Status(200).JSON(map[string]string{"status": "default"})
		},
	}))

	// Register specific handler
	app.WebSocket("$connect", func(ctx *Context) error {
		return ctx.Status(200).JSON(map[string]string{"status": "connected"})
	})

	handler := app.WebSocketHandler()

	// Test that specific route uses specific handler
	t.Run("Specific Route", func(t *testing.T) {
		event := createTestWebSocketEvent("$connect", "conn123", "")

		wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
		assert.True(t, ok, "Handler should be a WebSocket handler function")
		resp, err := wsHandler(context.Background(), event)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.False(t, defaultCalled)
	})

	// Test that unknown route uses default handler
	t.Run("Unknown Route", func(t *testing.T) {
		defaultCalled = false
		event := createTestWebSocketEvent("unknownRoute", "conn123", "")

		wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
		assert.True(t, ok, "Handler should be a WebSocket handler function")
		resp, err := wsHandler(context.Background(), event)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.True(t, defaultCalled)
	})
}

// Helper function to create test WebSocket events
func createTestWebSocketEvent(routeKey, connectionID, body string) events.APIGatewayWebsocketProxyRequest {
	return events.APIGatewayWebsocketProxyRequest{
		RequestContext: events.APIGatewayWebsocketProxyRequestContext{
			RouteKey:     routeKey,
			ConnectionID: connectionID,
			EventType:    "MESSAGE",
			RequestID:    "test-request-id",
			DomainName:   "test.execute-api.us-east-1.amazonaws.com",
			Stage:        "test",
			APIID:        "test-api-id",
		},
		Body: body,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}
}

// Mock connection store for testing
type mockConnectionStore struct {
	connections map[string]*Connection
}

func (m *mockConnectionStore) Save(_ context.Context, conn *Connection) error {
	m.connections[conn.ID] = conn
	return nil
}

func (m *mockConnectionStore) Get(_ context.Context, connectionID string) (*Connection, error) {
	conn, ok := m.connections[connectionID]
	if !ok {
		return nil, nil
	}
	return conn, nil
}

func (m *mockConnectionStore) Delete(_ context.Context, connectionID string) error {
	delete(m.connections, connectionID)
	return nil
}

func (m *mockConnectionStore) ListByUser(_ context.Context, userID string) ([]*Connection, error) {
	var conns []*Connection
	for _, conn := range m.connections {
		if conn.UserID == userID {
			conns = append(conns, conn)
		}
	}
	return conns, nil
}

func (m *mockConnectionStore) ListByTenant(_ context.Context, tenantID string) ([]*Connection, error) {
	var conns []*Connection
	for _, conn := range m.connections {
		if conn.TenantID == tenantID {
			conns = append(conns, conn)
		}
	}
	return conns, nil
}

func (m *mockConnectionStore) CountActive(_ context.Context) (int64, error) {
	return int64(len(m.connections)), nil
}

// TestWebSocketHandleRequest tests that WebSocket events are properly routed through the main HandleRequest method
func TestWebSocketHandleRequest(t *testing.T) {
	// Create app with WebSocket support
	app := New(WithWebSocketSupport())

	// Track if handlers were called
	connectCalled := false
	disconnectCalled := false
	defaultCalled := false

	// Register WebSocket handlers
	app.WebSocket("$connect", func(ctx *Context) error {
		connectCalled = true
		return ctx.JSON(map[string]string{"status": "connected"})
	})

	app.WebSocket("$disconnect", func(ctx *Context) error {
		disconnectCalled = true
		return ctx.JSON(map[string]string{"status": "disconnected"})
	})

	app.WebSocket("$default", func(ctx *Context) error {
		defaultCalled = true
		return ctx.JSON(map[string]string{"status": "message received"})
	})

	tests := []struct {
		event     map[string]any
		checkFunc func()
		name      string
		wantError bool
	}{
		{
			name: "$connect event through HandleRequest",
			event: map[string]any{
				"requestContext": map[string]any{
					"routeKey":     "$connect",
					"connectionId": "test-connection-1",
					"eventType":    "CONNECT",
					"stage":        "test",
					"requestId":    "req-1",
					"domainName":   "test.execute-api.us-east-1.amazonaws.com",
					"apiId":        "test-api",
				},
				"headers": map[string]any{
					"Host": "test.execute-api.us-east-1.amazonaws.com",
				},
			},
			wantError: false,
			checkFunc: func() {
				assert.True(t, connectCalled, "$connect handler should have been called")
			},
		},
		{
			name: "$disconnect event through HandleRequest",
			event: map[string]any{
				"requestContext": map[string]any{
					"routeKey":     "$disconnect",
					"connectionId": "test-connection-2",
					"eventType":    "DISCONNECT",
					"stage":        "test",
					"requestId":    "req-2",
					"domainName":   "test.execute-api.us-east-1.amazonaws.com",
					"apiId":        "test-api",
				},
				"headers": map[string]any{
					"Host": "test.execute-api.us-east-1.amazonaws.com",
				},
			},
			wantError: false,
			checkFunc: func() {
				assert.True(t, disconnectCalled, "$disconnect handler should have been called")
			},
		},
		{
			name: "$default event through HandleRequest",
			event: map[string]any{
				"requestContext": map[string]any{
					"routeKey":     "$default",
					"connectionId": "test-connection-3",
					"eventType":    "MESSAGE",
					"stage":        "test",
					"requestId":    "req-3",
					"domainName":   "test.execute-api.us-east-1.amazonaws.com",
					"apiId":        "test-api",
				},
				"body": `{"message": "hello"}`,
				"headers": map[string]any{
					"Host": "test.execute-api.us-east-1.amazonaws.com",
				},
			},
			wantError: false,
			checkFunc: func() {
				assert.True(t, defaultCalled, "$default handler should have been called")
			},
		},
		{
			name: "unhandled route through HandleRequest",
			event: map[string]any{
				"requestContext": map[string]any{
					"routeKey":     "unknownRoute",
					"connectionId": "test-connection-4",
					"eventType":    "MESSAGE",
					"stage":        "test",
					"requestId":    "req-4",
					"domainName":   "test.execute-api.us-east-1.amazonaws.com",
					"apiId":        "test-api",
				},
				"headers": map[string]any{
					"Host": "test.execute-api.us-east-1.amazonaws.com",
				},
			},
			wantError: true,
			checkFunc: func() {
				// Should fallback to $default handler
				assert.True(t, defaultCalled, "Should fallback to $default handler for unknown routes")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			connectCalled = false
			disconnectCalled = false
			defaultCalled = false

			// Call HandleRequest (the main entry point used by lambda.Start(app.HandleRequest))
			resp, err := app.HandleRequest(context.Background(), tt.event)

			if tt.wantError {
				// For unhandled routes, we expect it to fallback to $default which should succeed
				if tt.name == "unhandled route through HandleRequest" {
					assert.NoError(t, err, "Should fallback to $default handler")
					assert.NotNil(t, resp, "Response should not be nil")
				} else {
					assert.Error(t, err, "Expected error for test case")
				}
			} else {
				assert.NoError(t, err, "HandleRequest should not return error: %v", err)
				assert.NotNil(t, resp, "Response should not be nil")
			}

			// Run the test-specific check
			if tt.checkFunc != nil {
				tt.checkFunc()
			}
		})
	}
}
