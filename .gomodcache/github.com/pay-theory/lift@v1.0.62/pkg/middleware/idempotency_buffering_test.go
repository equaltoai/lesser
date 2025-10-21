package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

func TestIdempotencyMiddlewareWithBuffering(t *testing.T) {
	// Create app and idempotency middleware
	app := lift.New()
	store := NewMemoryIdempotencyStore()

	// Apply idempotency middleware
	idempotencyMiddleware := Idempotency(IdempotencyOptions{
		Store:      store,
		HeaderName: "Idempotency-Key",
		TTL:        1 * time.Hour,
	})
	app.Use(lift.Middleware(idempotencyMiddleware))

	// Track handler calls
	handlerCalls := 0

	// Register a test handler
	if err := app.POST("/api/test", func(ctx *lift.Context) error {
		handlerCalls++
		return ctx.JSON(map[string]interface{}{
			"status": "success",
			"calls":  handlerCalls,
		})
	}); err != nil {
		t.Fatalf("Failed to register route: %v", err)
	}

	// Create first request with idempotency key
	req1 := &lift.Request{
		Method: "POST",
		Path:   "/api/test",
		Headers: map[string]string{
			"Idempotency-Key": "test-key-123",
			"Content-Type":    "application/json",
		},
		Body: []byte(`{"data": "test"}`),
	}

	// First request should execute handler
	ctx1 := lift.NewContext(context.Background(), req1)

	// Route the request through the app
	if err := app.HandleTestRequest(ctx1); err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	// Verify handler was called
	if handlerCalls != 1 {
		t.Errorf("Expected handler to be called once, got %d", handlerCalls)
	}

	// Check response
	if ctx1.Response.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", ctx1.Response.StatusCode)
	}

	// Create second request with same idempotency key
	req2 := &lift.Request{
		Method: "POST",
		Path:   "/api/test",
		Headers: map[string]string{
			"Idempotency-Key": "test-key-123",
			"Content-Type":    "application/json",
		},
		Body: []byte(`{"data": "test"}`),
	}

	ctx2 := lift.NewContext(context.Background(), req2)

	// Second request should return cached response
	if err := app.HandleTestRequest(ctx2); err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	// Handler should NOT be called again
	if handlerCalls != 1 {
		t.Errorf("Expected handler to still have 1 call, got %d", handlerCalls)
	}

	// Should have idempotent replay header
	if ctx2.Response.Headers["X-Idempotent-Replay"] != "true" {
		t.Error("Expected X-Idempotent-Replay header")
	}

	// Response should be the same (cached)
	if ctx2.Response.StatusCode != 200 {
		t.Errorf("Expected cached status 200, got %d", ctx2.Response.StatusCode)
	}

	// Create third request with different idempotency key
	req3 := &lift.Request{
		Method: "POST",
		Path:   "/api/test",
		Headers: map[string]string{
			"Idempotency-Key": "different-key-456",
			"Content-Type":    "application/json",
		},
		Body: []byte(`{"data": "test"}`),
	}

	ctx3 := lift.NewContext(context.Background(), req3)

	if err := app.HandleTestRequest(ctx3); err != nil {
		t.Fatalf("Third request failed: %v", err)
	}

	// Handler should be called again for different key
	if handlerCalls != 2 {
		t.Errorf("Expected handler to be called twice total, got %d", handlerCalls)
	}
}

func TestIdempotencyBufferingCapture(t *testing.T) {
	// Test that response buffering properly captures response data
	store := NewMemoryIdempotencyStore()

	// Create middleware
	idempotencyMiddleware := Idempotency(IdempotencyOptions{
		Store:      store,
		HeaderName: "Idempotency-Key",
		TTL:        1 * time.Hour,
	})

	// Create a handler that returns specific data
	testData := map[string]interface{}{
		"id":     "12345",
		"name":   "Test Item",
		"amount": 100.50,
	}

	handler := lift.HandlerFunc(func(ctx *lift.Context) error {
		ctx.Status(201)
		return ctx.JSON(testData)
	})

	// Wrap with middleware
	wrappedHandler := idempotencyMiddleware(handler)

	// Create request with idempotency key
	req := &lift.Request{
		Method: "POST",
		Path:   "/api/create",
		Headers: map[string]string{
			"Idempotency-Key": "create-key-789",
		},
		Body: []byte(`{}`),
	}

	ctx := lift.NewContext(context.Background(), req)

	// Execute handler
	if err := wrappedHandler.Handle(ctx); err != nil {
		t.Fatalf("Handler execution failed: %v", err)
	}

	// Check that response was properly set
	if ctx.Response.StatusCode != 201 {
		t.Errorf("Expected status 201, got %d", ctx.Response.StatusCode)
	}

	// Check stored record
	storedRecord, err := store.Get(context.Background(), "create-key-789")
	if err != nil {
		t.Fatalf("Failed to get stored record: %v", err)
	}

	if storedRecord == nil {
		t.Fatal("Expected stored record, got nil")
	}

	if storedRecord.StatusCode != 201 {
		t.Errorf("Expected stored status 201, got %d", storedRecord.StatusCode)
	}

	if storedRecord.Response == nil {
		t.Error("Expected stored response data, got nil")
	}

	// Verify the stored response matches what we sent
	if respMap, ok := storedRecord.Response.(map[string]interface{}); ok {
		if respMap["id"] != testData["id"] {
			t.Errorf("Expected stored id=%v, got %v", testData["id"], respMap["id"])
		}
	} else {
		t.Errorf("Expected response to be a map, got %T", storedRecord.Response)
	}
}
