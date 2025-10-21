package middleware

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestPaymentIntent struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Amount int    `json:"amount"`
}

// createIdempotencyTestContext creates a test context for idempotency tests
func createIdempotencyTestContext(method, path string, body []byte) *lift.Context {
	adapterReq := &adapters.Request{
		Method:      method,
		Path:        path,
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		PathParams:  make(map[string]string),
		Body:        body,
	}
	req := lift.NewRequest(adapterReq)
	ctx := lift.NewContext(context.Background(), req)

	// Initialize response headers if not already done
	if ctx.Response.Headers == nil {
		ctx.Response.Headers = make(map[string]string)
	}

	return ctx
}

func TestIdempotencyMiddleware(t *testing.T) {
	t.Run("processes request without idempotency key normally", func(t *testing.T) {
		// Setup
		store := NewMemoryIdempotencyStore()
		middleware := Idempotency(IdempotencyOptions{
			Store: store,
		})

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			return ctx.JSON(TestPaymentIntent{
				ID:     "pi_123",
				Amount: 1000,
				Status: "succeeded",
			})
		}))

		// Execute request without idempotency key
		ctx := createIdempotencyTestContext("POST", "/payment", nil)

		err := handler.Handle(ctx)
		assert.NoError(t, err)

		// Verify response
		assert.NotNil(t, ctx.Response.Body)
		payment, ok := ctx.Response.Body.(TestPaymentIntent)
		assert.True(t, ok)
		assert.Equal(t, "pi_123", payment.ID)
	})

	t.Run("returns cached response for duplicate request", func(t *testing.T) {
		// Setup
		store := NewMemoryIdempotencyStore()
		callCount := 0

		middleware := Idempotency(IdempotencyOptions{
			Store: store,
		})

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			callCount++
			return ctx.JSON(TestPaymentIntent{
				ID:     "pi_123",
				Amount: 1000,
				Status: "succeeded",
			})
		}))

		// First request with idempotency key
		ctx1 := createIdempotencyTestContext("POST", "/payment", nil)
		ctx1.Request.Headers["Idempotency-Key"] = "test-key-123"

		err := handler.Handle(ctx1)
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)

		// Second request with same idempotency key
		ctx2 := createIdempotencyTestContext("POST", "/payment", nil)
		ctx2.Request.Headers["Idempotency-Key"] = "test-key-123"

		err = handler.Handle(ctx2)
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount) // Handler should not be called again

		// Verify both responses are identical
		assert.Equal(t, ctx1.Response.Body, ctx2.Response.Body)
		assert.Equal(t, "true", ctx2.Response.Headers["X-Idempotent-Replay"])
	})

	t.Run("handles concurrent duplicate requests", func(t *testing.T) {
		// Setup
		store := NewMemoryIdempotencyStore()

		middleware := Idempotency(IdempotencyOptions{
			Store:             store,
			ProcessingTimeout: 1 * time.Second,
		})

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			// Simulate slow processing
			time.Sleep(100 * time.Millisecond)
			return ctx.JSON(TestPaymentIntent{
				ID:     "pi_123",
				Amount: 1000,
				Status: "succeeded",
			})
		}))

		// Start first request
		done1 := make(chan error)
		go func() {
			ctx := createIdempotencyTestContext("POST", "/payment", nil)
			ctx.Request.Headers["Idempotency-Key"] = "concurrent-key"
			done1 <- handler.Handle(ctx)
		}()

		// Give first request time to start
		time.Sleep(10 * time.Millisecond)

		// Try concurrent request with same key
		ctx2 := createIdempotencyTestContext("POST", "/payment", nil)
		ctx2.Request.Headers["Idempotency-Key"] = "concurrent-key"

		err := handler.Handle(ctx2)
		assert.Error(t, err)
		liftErr, ok := err.(*lift.LiftError)
		require.True(t, ok)
		assert.Equal(t, 409, liftErr.StatusCode)
		assert.Contains(t, liftErr.Message, "already being processed")

		// Wait for first request to complete
		assert.NoError(t, <-done1)
	})

	t.Run("caches error responses", func(t *testing.T) {
		// Setup
		store := NewMemoryIdempotencyStore()
		callCount := 0

		middleware := Idempotency(IdempotencyOptions{
			Store: store,
		})

		handler := middleware(lift.HandlerFunc(func(_ *lift.Context) error {
			callCount++
			return lift.NewLiftError("PAYMENT_FAILED", "Insufficient funds", 400)
		}))

		// First request
		ctx1 := createIdempotencyTestContext("POST", "/payment", nil)
		ctx1.Request.Headers["Idempotency-Key"] = "error-key"

		err1 := handler.Handle(ctx1)
		assert.Error(t, err1)
		assert.Equal(t, 1, callCount)

		// Second request should return cached error
		ctx2 := createIdempotencyTestContext("POST", "/payment", nil)
		ctx2.Request.Headers["Idempotency-Key"] = "error-key"

		err2 := handler.Handle(ctx2)
		assert.Error(t, err2)
		assert.Equal(t, 1, callCount) // Handler not called again

		// Verify errors match
		var liftErr1, liftErr2 *lift.LiftError
		liftErr1, ok1 := err1.(*lift.LiftError)
		liftErr2, ok2 := err2.(*lift.LiftError)
		assert.True(t, ok1, "err1 should be a LiftError")
		assert.True(t, ok2, "err2 should be a LiftError")
		if ok1 && ok2 {
			assert.Equal(t, liftErr1.StatusCode, liftErr2.StatusCode)
		}
	})

	t.Run("isolates keys by account", func(t *testing.T) {
		// Setup
		store := NewMemoryIdempotencyStore()
		responses := make(map[string]string)

		middleware := Idempotency(IdempotencyOptions{
			Store: store,
		})

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			accountIDVal := ctx.Get("account_id")
			accountID, ok := accountIDVal.(string)
			if !ok {
				return fmt.Errorf("account_id not found")
			}
			response := TestPaymentIntent{
				ID:     "pi_" + accountID,
				Amount: 1000,
				Status: "succeeded",
			}
			responses[accountID] = response.ID
			return ctx.JSON(response)
		}))

		// Request from account1
		ctx1 := createIdempotencyTestContext("POST", "/payment", nil)
		ctx1.Request.Headers["Idempotency-Key"] = "shared-key"
		ctx1.Set("account_id", "account1")

		err := handler.Handle(ctx1)
		assert.NoError(t, err)

		// Request from account2 with same idempotency key
		ctx2 := createIdempotencyTestContext("POST", "/payment", nil)
		ctx2.Request.Headers["Idempotency-Key"] = "shared-key"
		ctx2.Set("account_id", "account2")

		err = handler.Handle(ctx2)
		assert.NoError(t, err)

		// Verify different responses (not cached across accounts)
		assert.Equal(t, "pi_account1", responses["account1"])
		assert.Equal(t, "pi_account2", responses["account2"])
	})

	t.Run("respects TTL for cached responses", func(t *testing.T) {
		// Setup with short TTL
		store := NewMemoryIdempotencyStore()
		callCount := 0

		middleware := Idempotency(IdempotencyOptions{
			Store: store,
			TTL:   100 * time.Millisecond,
		})

		handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
			callCount++
			return ctx.JSON(TestPaymentIntent{
				ID:     "pi_" + time.Now().Format("150405"),
				Amount: 1000,
				Status: "succeeded",
			})
		}))

		// First request
		ctx1 := createIdempotencyTestContext("POST", "/payment", nil)
		ctx1.Request.Headers["Idempotency-Key"] = "ttl-key"

		err := handler.Handle(ctx1)
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)

		// Wait for TTL to expire
		time.Sleep(150 * time.Millisecond)

		// Second request should process normally
		ctx2 := createIdempotencyTestContext("POST", "/payment", nil)
		ctx2.Request.Headers["Idempotency-Key"] = "ttl-key"

		err = handler.Handle(ctx2)
		assert.NoError(t, err)
		assert.Equal(t, 2, callCount) // Handler called again
	})
}

func TestMemoryIdempotencyStore(t *testing.T) {
	t.Run("stores and retrieves records", func(t *testing.T) {
		store := NewMemoryIdempotencyStore()
		ctx := context.Background()

		record := &IdempotencyRecord{
			Key:        "test-key",
			Status:     "completed",
			Response:   map[string]string{"id": "123"},
			StatusCode: 200,
			CreatedAt:  time.Now(),
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		}

		// Store record
		err := store.Set(ctx, "test-key", record)
		assert.NoError(t, err)

		// Retrieve record
		retrieved, err := store.Get(ctx, "test-key")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, record.Key, retrieved.Key)
		assert.Equal(t, record.Status, retrieved.Status)
		assert.Equal(t, record.StatusCode, retrieved.StatusCode)
	})

	t.Run("returns nil for non-existent keys", func(t *testing.T) {
		store := NewMemoryIdempotencyStore()
		ctx := context.Background()

		retrieved, err := store.Get(ctx, "non-existent")
		assert.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("respects expiration", func(t *testing.T) {
		store := NewMemoryIdempotencyStore()
		ctx := context.Background()

		record := &IdempotencyRecord{
			Key:        "expired-key",
			Status:     "completed",
			Response:   map[string]string{"id": "123"},
			StatusCode: 200,
			CreatedAt:  time.Now().Add(-2 * time.Hour),
			ExpiresAt:  time.Now().Add(-1 * time.Hour), // Already expired
		}

		// Store expired record
		err := store.Set(ctx, "expired-key", record)
		assert.NoError(t, err)

		// Try to retrieve - should return nil
		retrieved, err := store.Get(ctx, "expired-key")
		assert.NoError(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("handles concurrent access", func(t *testing.T) {
		store := NewMemoryIdempotencyStore()
		ctx := context.Background()

		// Concurrent writes
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func(idx int) {
				record := &IdempotencyRecord{
					Key:        fmt.Sprintf("key-%d", idx),
					Status:     "completed",
					Response:   map[string]int{"index": idx},
					StatusCode: 200,
					CreatedAt:  time.Now(),
					ExpiresAt:  time.Now().Add(1 * time.Hour),
				}
				if err := store.Set(ctx, record.Key, record); err != nil {
					t.Errorf("Failed to set record: %v", err)
				}
				done <- true
			}(i)
		}

		// Wait for all writes
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify all records exist
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("key-%d", i)
			retrieved, err := store.Get(ctx, key)
			assert.NoError(t, err)
			assert.NotNil(t, retrieved)
			assert.Equal(t, key, retrieved.Key)
		}
	})
}
