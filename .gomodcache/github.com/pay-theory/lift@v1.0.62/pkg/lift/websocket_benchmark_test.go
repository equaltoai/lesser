package lift

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// BenchmarkWebSocketRouting tests the performance of WebSocket routing
func BenchmarkWebSocketRouting(b *testing.B) {
	app := New(WithWebSocketSupport())

	// Register multiple routes
	routes := []string{"$connect", "$disconnect", "message", "ping", "pong", "broadcast", "subscribe", "unsubscribe"}
	for _, route := range routes {
		r := route // capture loop variable
		app.WebSocket(r, func(ctx *Context) error {
			return ctx.Status(200).JSON(map[string]string{"route": r})
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler := app.RouteWebSocket("message")
		if handler == nil {
			b.Fatal("Handler not found")
		}
	}
}

// BenchmarkWebSocketHandlerExecution tests full handler execution performance
func BenchmarkWebSocketHandlerExecution(b *testing.B) {
	app := New(WithWebSocketSupport())

	// Add middleware
	app.Use(func(next Handler) Handler {
		return HandlerFunc(func(ctx *Context) error {
			ctx.Set("middleware", "executed")
			return next.Handle(ctx)
		})
	})

	// Register handler
	app.WebSocket("$connect", func(ctx *Context) error {
		return ctx.Status(200).JSON(map[string]string{"status": "connected"})
	})

	handler := app.WebSocketHandler()
	event := createTestWebSocketEvent("$connect", "conn123", "")

	wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
	if !ok {
		b.Fatal("Handler is not a WebSocket handler function")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := wsHandler(context.Background(), event)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWebSocketWithAutoConnectionManagement tests performance with connection management
func BenchmarkWebSocketWithAutoConnectionManagement(b *testing.B) {
	mockStore := &mockConnectionStore{
		connections: make(map[string]*Connection),
	}

	app := New(WithWebSocketSupport(WebSocketOptions{
		EnableAutoConnectionManagement: true,
		ConnectionStore:                mockStore,
	}))

	app.WebSocket("$connect", func(ctx *Context) error {
		ctx.SetUserID("user123")
		ctx.Set("tenant_id", "tenant456")
		return ctx.Status(200).JSON(map[string]string{"status": "connected"})
	})

	handler := app.WebSocketHandler()
	event := createTestWebSocketEvent("$connect", "conn123", "")

	wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
	if !ok {
		b.Fatal("Handler is not a WebSocket handler function")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := wsHandler(context.Background(), event)
		if err != nil {
			b.Fatal(err)
		}
		// Clean up for next iteration
		delete(mockStore.connections, "conn123")
	}
}

// BenchmarkLegacyWebSocketPattern benchmarks the old pattern for comparison
func BenchmarkLegacyWebSocketPattern(b *testing.B) {
	app := New()

	// Old pattern with HTTP-style routing
	if err := app.Handle("CONNECT", "/connect", func(ctx *Context) error {
		wsCtx, err := ctx.AsWebSocket()
		if err != nil {
			return ctx.Status(500).JSON(map[string]string{
				"error": "Invalid WebSocket context",
			})
		}

		// Simulate connection storage
		connectionID := wsCtx.ConnectionID()
		if connectionID == "" {
			return ctx.Status(500).JSON(map[string]string{
				"error": "No connection ID",
			})
		}

		return ctx.Status(200).JSON(map[string]string{
			"status": "connected",
		})
	}); err != nil {
		b.Fatalf("Failed to register websocket route: %v", err)
	}

	// Start the app to apply middleware
	if err := app.Start(); err != nil {
		b.Fatal(err)
	}

	// Create adapter request
	event := createTestWebSocketEvent("$connect", "conn123", "")
	genericEvent := convertWebSocketEventToGeneric(event)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := app.parseEvent(genericEvent)
		if err != nil {
			b.Fatal(err)
		}

		ctx := NewContext(context.Background(), req)

		// Simulate routing - the adapter maps $connect to CONNECT /connect
		ctx.Request.Method = "CONNECT"
		ctx.Request.Path = "/connect"

		if err := app.router.Handle(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWebSocketContextConversion tests the performance of context conversion
func BenchmarkWebSocketContextConversion(b *testing.B) {
	event := createTestWebSocketEvent("$connect", "conn123", "")
	genericEvent := convertWebSocketEventToGeneric(event)

	app := New()
	req, err := app.parseEvent(genericEvent)
	if err != nil {
		b.Fatalf("Failed to parse event: %v", err)
	}
	ctx := NewContext(context.Background(), req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ctx.AsWebSocket()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWebSocketMiddlewareStack tests middleware performance
func BenchmarkWebSocketMiddlewareStack(b *testing.B) {
	app := New(WithWebSocketSupport())

	// Add multiple middleware layers
	for i := 0; i < 5; i++ {
		app.Use(func(next Handler) Handler {
			return HandlerFunc(func(ctx *Context) error {
				ctx.Set("middleware", time.Now().UnixNano())
				return next.Handle(ctx)
			})
		})
	}

	app.WebSocket("message", func(ctx *Context) error {
		return ctx.Status(200).JSON(map[string]string{"status": "ok"})
	})

	handler := app.WebSocketHandler()
	event := createTestWebSocketEvent("message", "conn123", `{"data":"test"}`)

	wsHandler, ok := handler.(func(context.Context, events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error))
	if !ok {
		b.Fatal("Handler is not a WebSocket handler function")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := wsHandler(context.Background(), event)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark results comparison structure
type BenchmarkComparison struct {
	TestName    string
	OldNsPerOp  int64
	NewNsPerOp  int64
	Improvement float64
	MemoryDelta int64
}
