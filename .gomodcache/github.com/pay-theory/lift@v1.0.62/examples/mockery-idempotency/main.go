package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
)

// Example: Mockery payment processing with idempotency

// PaymentRequest represents an incoming payment request
type PaymentRequest struct {
	Currency    string  `json:"currency" validate:"required,len=3"`
	CustomerID  string  `json:"customer_id" validate:"required"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount" validate:"required,min=0.01"`
}

// PaymentResponse represents the payment result
type PaymentResponse struct {
	ProcessedAt    time.Time `json:"processed_at"`
	TransactionID  string    `json:"transaction_id"`
	Status         string    `json:"status"`
	Currency       string    `json:"currency"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	Amount         float64   `json:"amount"`
}

// MockeryPaymentProcessor simulates payment processing
type MockeryPaymentProcessor struct {
	// Track processed payments for demo
	processedPayments map[string]*PaymentResponse
}

func NewMockeryPaymentProcessor() *MockeryPaymentProcessor {
	return &MockeryPaymentProcessor{
		processedPayments: make(map[string]*PaymentResponse),
	}
}

func (p *MockeryPaymentProcessor) ProcessPayment(req PaymentRequest) (*PaymentResponse, error) {
	// Simulate payment processing time
	time.Sleep(100 * time.Millisecond)

	// Simulate occasional failures (10% failure rate)
	if time.Now().UnixNano()%10 == 0 {
		return nil, fmt.Errorf("payment gateway timeout")
	}

	// Create successful response
	txnID := fmt.Sprintf("txn_%d", time.Now().UnixNano())
	response := &PaymentResponse{
		TransactionID: txnID,
		Status:        "completed",
		Amount:        req.Amount,
		Currency:      req.Currency,
		ProcessedAt:   time.Now(),
	}

	p.processedPayments[txnID] = response
	log.Printf("💰 Processed payment: %s for %.2f %s", txnID, req.Amount, req.Currency)

	return response, nil
}

func main() {
	// Initialize Lift app
	app := lift.New()

	// Create payment processor
	processor := NewMockeryPaymentProcessor()

	// Setup idempotency middleware
	// For production, use DynamoDB store instead
	idempotencyStore := middleware.NewMemoryIdempotencyStore()

	idempotencyMiddleware := middleware.Idempotency(middleware.IdempotencyOptions{
		Store:             idempotencyStore,
		HeaderName:        "Idempotency-Key",
		TTL:               24 * time.Hour,
		ProcessingTimeout: 30 * time.Second,
		OnDuplicate: func(_ *lift.Context, record *middleware.IdempotencyRecord) {
			// Log when duplicate requests are detected
			log.Printf("🔄 Duplicate request detected: key=%s, original_time=%v",
				record.Key, record.CreatedAt)
		},
	})

	// Apply middleware
	app.Use(lift.Middleware(idempotencyMiddleware))

	// Add request logging
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			idempKey := ctx.Header("Idempotency-Key")

			log.Printf("📥 Request: %s %s (Idempotency-Key: %s)",
				ctx.Request.Method, ctx.Request.Path, idempKey)

			err := next.Handle(ctx)

			log.Printf("📤 Response: %s %s - Status: %d - Duration: %v",
				ctx.Request.Method, ctx.Request.Path,
				ctx.Response.StatusCode, time.Since(start))

			return err
		})
	})

	// Payment processing endpoint
	if err := app.POST("/api/v1/payments", func(ctx *lift.Context) error {
		// Parse and validate request
		var req PaymentRequest
		if err := ctx.ParseRequest(&req); err != nil {
			return ctx.BadRequest("Invalid payment request", err)
		}

		// Get idempotency key for response
		idempKey := ctx.Header("Idempotency-Key")

		// Process payment (this only runs once per idempotency key!)
		response, err := processor.ProcessPayment(req)
		if err != nil {
			// Even errors are cached with idempotency
			return lift.NewLiftError("PAYMENT_FAILED", err.Error(), 402)
		}

		// Add idempotency key to response for transparency
		response.IdempotencyKey = idempKey

		// Return successful response
		return ctx.Status(201).JSON(response)
	}); err != nil {
		log.Fatalf("Failed to register POST /api/v1/payments: %v", err)
	}

	// Health check endpoint (no idempotency needed)
	if err := app.GET("/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{
			"status":  "healthy",
			"service": "mockery-payment-service",
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /health: %v", err)
	}

	// List processed payments (for demo)
	if err := app.GET("/api/v1/payments", func(ctx *lift.Context) error {
		payments := make([]*PaymentResponse, 0, len(processor.processedPayments))
		for _, p := range processor.processedPayments {
			payments = append(payments, p)
		}
		return ctx.JSON(map[string]interface{}{
			"payments": payments,
			"count":    len(payments),
		})
	}); err != nil {
		log.Fatalf("Failed to register GET /api/v1/payments: %v", err)
	}

	// Start the app
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}

	log.Println("🚀 Mockery Payment Service started")
	log.Println("📝 Example requests:")
	log.Println("")

	// Simulate some payment requests
	simulatePaymentRequests(app)
}

// simulatePaymentRequests demonstrates idempotency in action
func simulatePaymentRequests(app *lift.App) {
	log.Println("=== DEMO: Payment Request Simulation ===")

	// Scenario 1: Normal payment with retry
	log.Println("📌 Scenario 1: Customer payment with network retry")

	paymentReq := &lift.Request{
		Method: "POST",
		Path:   "/api/v1/payments",
		Headers: map[string]string{
			"Content-Type":    "application/json",
			"Idempotency-Key": "customer-123-payment-456",
		},
		Body: []byte(`{
			"amount": 99.99,
			"currency": "USD",
			"customer_id": "cust-123",
			"description": "Monthly subscription"
		}`),
	}

	// First attempt
	log.Println("  Attempt 1: Initial request")
	ctx1 := lift.NewContext(context.Background(), paymentReq)
	if err := app.HandleTestRequest(ctx1); err != nil {
		log.Printf("  ❌ Error: %v", err)
	} else {
		log.Println("  ✅ Success: Payment processed")
	}

	// Simulate network retry (same idempotency key)
	time.Sleep(500 * time.Millisecond)
	log.Println("  Attempt 2: Network retry (same idempotency key)")
	ctx2 := lift.NewContext(context.Background(), paymentReq)
	if err := app.HandleTestRequest(ctx2); err != nil {
		log.Printf("  ❌ Error: %v", err)
	} else if ctx2.Response.Headers["X-Idempotent-Replay"] == "true" {
		log.Println("  ✅ Success: Returned cached response (no double charge!)")
	}

	// Scenario 2: Different payment with new idempotency key
	log.Println("\n📌 Scenario 2: New payment from same customer")

	newPaymentReq := &lift.Request{
		Method: "POST",
		Path:   "/api/v1/payments",
		Headers: map[string]string{
			"Content-Type":    "application/json",
			"Idempotency-Key": "customer-123-payment-789", // Different key!
		},
		Body: []byte(`{
			"amount": 49.99,
			"currency": "USD",
			"customer_id": "cust-123",
			"description": "Add-on purchase"
		}`),
	}

	ctx3 := lift.NewContext(context.Background(), newPaymentReq)
	if err := app.HandleTestRequest(ctx3); err != nil {
		log.Printf("  ❌ Error: %v", err)
	} else {
		log.Println("  ✅ Success: New payment processed")
	}

	// Scenario 3: Concurrent requests (same idempotency key)
	log.Println("\n📌 Scenario 3: Concurrent duplicate requests")

	concurrentReq := &lift.Request{
		Method: "POST",
		Path:   "/api/v1/payments",
		Headers: map[string]string{
			"Content-Type":    "application/json",
			"Idempotency-Key": "customer-456-payment-001",
		},
		Body: []byte(`{
			"amount": 199.99,
			"currency": "USD",
			"customer_id": "cust-456",
			"description": "Premium upgrade"
		}`),
	}

	// Start two requests concurrently
	done1 := make(chan bool)
	done2 := make(chan bool)

	go func() {
		log.Println("  Request A: Started")
		ctx := lift.NewContext(context.Background(), concurrentReq)
		if err := app.HandleTestRequest(ctx); err != nil {
			log.Printf("  Request A: ❌ Error: %v", err)
		} else {
			log.Println("  Request A: ✅ Processed")
		}
		done1 <- true
	}()

	// Small delay to ensure first request starts processing
	time.Sleep(50 * time.Millisecond)

	go func() {
		log.Println("  Request B: Started (will be blocked)")
		ctx := lift.NewContext(context.Background(), concurrentReq)
		if err := app.HandleTestRequest(ctx); err != nil {
			if liftErr, ok := err.(*lift.LiftError); ok && liftErr.Code == "IDEMPOTENCY_CONFLICT" {
				log.Println("  Request B: ⏸️  Blocked - request already processing")
			} else {
				log.Printf("  Request B: ❌ Error: %v", err)
			}
		} else {
			log.Println("  Request B: ✅ Processed")
		}
		done2 <- true
	}()

	<-done1
	<-done2

	// Show final state
	log.Println("\n📊 Final State:")
	listReq := &lift.Request{
		Method: "GET",
		Path:   "/api/v1/payments",
	}
	ctx := lift.NewContext(context.Background(), listReq)
	if err := app.HandleTestRequest(ctx); err == nil {
		if payments, ok := ctx.Response.Body.(map[string]interface{}); ok {
			log.Printf("  Total payments processed: %v", payments["count"])
			log.Println("  (Note: No duplicate charges despite retries!)")
		}
	}

	log.Println("\n✅ Demo complete - Idempotency prevents duplicate charges!")
}
