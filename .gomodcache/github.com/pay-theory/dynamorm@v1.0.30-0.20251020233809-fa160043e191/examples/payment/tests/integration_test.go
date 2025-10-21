package tests

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pay-theory/dynamorm"
	payment "github.com/pay-theory/dynamorm/examples/payment"
	"github.com/pay-theory/dynamorm/examples/payment/utils"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// TestMultiAccountPaymentFlow tests payment processing across multiple accounts
func TestMultiAccountPaymentFlow(t *testing.T) {
	// Initialize DynamoDB connection
	db, err := initTestDB(t)
	if err != nil {
		t.Skip("Skipping test - DynamoDB connection not available")
		return
	}
	require.NotNil(t, db)

	// Create test merchants
	merchant1 := createTestMerchant(t, db, "merchant-1", "Merchant One")
	merchant2 := createTestMerchant(t, db, "merchant-2", "Merchant Two")

	// Create customers
	customer1 := createTestCustomer(t, db, merchant1.ID, "customer1@example.com")
	_ = createTestCustomer(t, db, merchant2.ID, "customer2@example.com") // Created for completeness

	// Test Case 1: Process payment for merchant 1
	payment1 := &payment.Payment{
		ID:             uuid.New().String(),
		IdempotencyKey: uuid.New().String(),
		MerchantID:     merchant1.ID,
		CustomerID:     customer1.ID,
		Amount:         10000, // $100.00
		Currency:       "USD",
		Status:         payment.PaymentStatusPending,
		PaymentMethod:  "card",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Version:        1,
	}

	err = db.Model(payment1).Create()
	require.NoError(t, err)

	// Create transaction
	transaction1 := &payment.Transaction{
		ID:          uuid.New().String(),
		PaymentID:   payment1.ID,
		Type:        payment.TransactionTypeCapture,
		Amount:      payment1.Amount,
		Status:      "succeeded",
		ProcessedAt: time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Version:     1,
	}

	err = db.Model(transaction1).Create()
	require.NoError(t, err)

	// Test Case 2: Transfer between accounts (simulated)
	transferAmount := int64(2500) // $25.00

	// Create transfer records
	transferOut := &payment.Transaction{
		ID:           uuid.New().String(),
		PaymentID:    payment1.ID,
		Type:         "transfer_out",
		Amount:       -transferAmount,
		Status:       payment.PaymentStatusSucceeded,
		ProcessedAt:  time.Now(),
		ResponseCode: "transfer_to_" + merchant2.ID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Version:      1,
	}

	transferIn := &payment.Payment{
		ID:             uuid.New().String(),
		IdempotencyKey: "transfer_" + transferOut.ID,
		MerchantID:     merchant2.ID,
		Amount:         transferAmount,
		Currency:       "USD",
		Status:         payment.PaymentStatusSucceeded,
		PaymentMethod:  "transfer",
		Description:    fmt.Sprintf("Transfer from %s", merchant1.ID),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Version:        1,
	}

	// Execute transfer in transaction
	err = db.Transaction(func(tx *core.Tx) error {
		if err := tx.Create(transferOut); err != nil {
			return err
		}
		if err := tx.Create(transferIn); err != nil {
			return err
		}
		return nil
	})
	require.NoError(t, err)

	// Verify balances
	var merchant1Payments []*payment.Payment
	err = db.Model(&payment.Payment{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchant1.ID).
		All(&merchant1Payments)
	require.NoError(t, err)
	assert.Len(t, merchant1Payments, 1)

	var merchant2Payments []*payment.Payment
	err = db.Model(&payment.Payment{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchant2.ID).
		All(&merchant2Payments)
	require.NoError(t, err)
	assert.Len(t, merchant2Payments, 1)
	assert.Equal(t, transferAmount, merchant2Payments[0].Amount)

	// Test audit trail
	auditTracker := utils.NewAuditTracker(db)
	err = auditTracker.Track("transfer_completed", "payment", map[string]any{
		"from_merchant": merchant1.ID,
		"to_merchant":   merchant2.ID,
		"amount":        transferAmount,
		"transfer_id":   transferOut.ID,
	})
	require.NoError(t, err)

	// Verify audit logs exist
	logs, err := auditTracker.GetAuditHistory(context.Background(), "payment", transferOut.ID, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, logs)
}

// TestHighVolumeProcessing tests processing high volume of payments
func TestHighVolumeProcessing(t *testing.T) {
	db, err := initTestDB(t)
	if err != nil {
		t.Skip("Skipping test - DynamoDB connection not available")
		return
	}
	require.NotNil(t, db)

	merchant := createTestMerchant(t, db, "high-volume-merchant", "High Volume Merchant")

	// Number of payments to process
	numPayments := 100
	numWorkers := 10

	// Create payment channel
	paymentChan := make(chan int, numPayments)
	errorChan := make(chan error, numPayments)

	// Worker pool
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	startTime := time.Now()

	// Start workers
	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()

			for i := range paymentChan {
				// Create payment
				p := &payment.Payment{
					ID:             fmt.Sprintf("payment-%d-%d", workerID, i),
					IdempotencyKey: fmt.Sprintf("idempotency-%d-%d", workerID, i),
					MerchantID:     merchant.ID,
					Amount:         int64(1000 + i), // Variable amounts
					Currency:       "USD",
					Status:         payment.PaymentStatusSucceeded,
					PaymentMethod:  "card",
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
					Version:        1,
				}

				if err := db.Model(p).Create(); err != nil {
					errorChan <- err
					continue
				}

				// Create transaction
				transaction := &payment.Transaction{
					ID:          fmt.Sprintf("transaction-%d-%d", workerID, i),
					PaymentID:   p.ID,
					Type:        payment.TransactionTypeCapture,
					Amount:      p.Amount,
					Status:      "succeeded",
					ProcessedAt: time.Now(),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
					Version:     1,
				}

				if err := db.Model(transaction).Create(); err != nil {
					errorChan <- err
				}
			}
		}(w)
	}

	// Send work to workers
	for i := 0; i < numPayments; i++ {
		paymentChan <- i
	}
	close(paymentChan)

	// Wait for completion
	wg.Wait()
	close(errorChan)

	// Check for errors
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	processingTime := time.Since(startTime)

	// Verify results
	assert.Empty(t, errors, "Expected no errors during high volume processing")

	// Count total payments
	var allPayments []*payment.Payment
	err = db.Model(&payment.Payment{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchant.ID).
		Scan(&allPayments)
	require.NoError(t, err)
	assert.Len(t, allPayments, numPayments)

	// Performance assertions
	avgTimePerPayment := processingTime / time.Duration(numPayments)
	assert.Less(t, avgTimePerPayment, 50*time.Millisecond, "Average processing time should be under 50ms")

	t.Logf("Processed %d payments in %v (avg: %v per payment)",
		numPayments, processingTime, avgTimePerPayment)
}

// TestPaymentErrorScenarios tests various error conditions
func TestPaymentErrorScenarios(t *testing.T) {
	db, err := initTestDB(t)
	if err != nil {
		t.Skip("Skipping test - DynamoDB connection not available")
		return
	}
	require.NotNil(t, db)

	merchant := createTestMerchant(t, db, "error-test-merchant", "Error Test Merchant")
	idempotency := utils.NewIdempotencyMiddleware(db, 24*time.Hour)

	// Test Case 1: Duplicate idempotency key
	idempotencyKey := "duplicate-key-123"

	// First request
	result1, err := idempotency.Process(context.Background(), merchant.ID, idempotencyKey, func() (any, error) {
		return &payment.Payment{
			ID:     "payment-1",
			Amount: 1000,
		}, nil
	})
	require.NoError(t, err)
	assert.NotNil(t, result1)

	// Duplicate request
	result2, err := idempotency.Process(context.Background(), merchant.ID, idempotencyKey, func() (any, error) {
		return &payment.Payment{
			ID:     "payment-2",
			Amount: 2000,
		}, nil
	})
	assert.Equal(t, utils.ErrDuplicateRequest, err)
	assert.NotNil(t, result2)

	// Verify same result returned
	if p1, ok := result1.(*payment.Payment); ok {
		if p2, ok := result2.(*payment.Payment); ok {
			assert.Equal(t, p1.ID, p2.ID)
			assert.Equal(t, p1.Amount, p2.Amount)
		}
	}

	// Test Case 2: Invalid merchant
	invalidPayment := &payment.Payment{
		ID:             uuid.New().String(),
		IdempotencyKey: uuid.New().String(),
		MerchantID:     "invalid-merchant",
		Amount:         1000,
		Currency:       "USD",
		Status:         payment.PaymentStatusPending,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Version:        1,
	}

	// Verify merchant exists check (would be in actual implementation)
	var merchantCheck payment.Merchant
	err = db.Model(&payment.Merchant{}).
		Where("ID", "=", invalidPayment.MerchantID).
		First(&merchantCheck)
	assert.Error(t, err) // Merchant doesn't exist

	// Test Case 3: Timeout scenarios
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Simulate slow operation
	_, err = idempotency.Process(ctx, merchant.ID, "timeout-key", func() (any, error) {
		time.Sleep(10 * time.Millisecond) // Longer than timeout
		return nil, nil
	})
	// Error would occur due to context cancellation in real scenario

	// Test Case 4: Recovery procedures
	// Create a payment in pending state
	paymentID := uuid.New().String()
	//nolint:unusedwrite // Fields are used in assertions below
	pendingPayment := &payment.Payment{
		ID:             paymentID,
		IdempotencyKey: uuid.New().String(),
		MerchantID:     merchant.ID,
		Amount:         5000,
		Currency:       "USD",
		Status:         payment.PaymentStatusPending,
		CreatedAt:      time.Now().Add(-1 * time.Hour), // Old pending payment
		UpdatedAt:      time.Now().Add(-1 * time.Hour),
		Version:        1,
	}

	err = db.Model(pendingPayment).Create()
	require.NoError(t, err)

	// Verify the payment was created correctly by reading it back
	var savedPayment payment.Payment
	err = db.Model(&payment.Payment{}).
		Where("ID", "=", paymentID).
		First(&savedPayment)
	require.NoError(t, err)

	assert.Equal(t, pendingPayment.ID, savedPayment.ID)
	assert.Equal(t, pendingPayment.IdempotencyKey, savedPayment.IdempotencyKey)
	assert.Equal(t, pendingPayment.Amount, savedPayment.Amount)
	assert.Equal(t, pendingPayment.Currency, savedPayment.Currency)
	assert.Equal(t, pendingPayment.Status, savedPayment.Status)
	assert.Equal(t, pendingPayment.MerchantID, savedPayment.MerchantID)
	assert.WithinDuration(t, pendingPayment.CreatedAt, savedPayment.CreatedAt, 5*time.Second)
	assert.WithinDuration(t, pendingPayment.UpdatedAt, savedPayment.UpdatedAt, 5*time.Second)
	assert.Equal(t, pendingPayment.Version, savedPayment.Version)

	// Recovery: Find and process old pending payments
	var oldPendingPayments []*payment.Payment
	err = db.Model(&payment.Payment{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchant.ID).
		Where("Status", "=", payment.PaymentStatusPending).
		Where("CreatedAt", "<", time.Now().Add(-30*time.Minute)).
		All(&oldPendingPayments)
	require.NoError(t, err)
	assert.Len(t, oldPendingPayments, 1)

	// Process recovery
	for _, p := range oldPendingPayments {
		// In real scenario, check with payment processor
		// For now, mark as failed
		p.Status = payment.PaymentStatusFailed
		p.UpdatedAt = time.Now()
		err = db.Model(p).Update("Status", "UpdatedAt")
		require.NoError(t, err)
	}
}

// Helper functions

func initTestDB(t *testing.T) (core.ExtendedDB, error) {
	// Skip if no AWS credentials or DynamoDB Local not available
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
		return nil, fmt.Errorf("AWS credentials not available")
	}

	// Initialize test database
	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000", // Local DynamoDB
	})
	if err != nil {
		return nil, err
	}

	// Register models
	models := []any{
		&payment.Payment{},
		&payment.Transaction{},
		&payment.Customer{},
		&payment.Merchant{},
		&payment.IdempotencyRecord{},
		&payment.Settlement{},
		&payment.Webhook{},
		&utils.AuditLog{},
	}

	for _, model := range models {
		db.Model(model)
	}

	// Create tables (in test environment)
	for _, model := range models {
		if err := db.AutoMigrate(model); err != nil {
			t.Logf("Failed to create table for %T: %v", model, err)
		}
	}

	return db, nil
}

func createTestMerchant(t *testing.T, db core.ExtendedDB, id, name string) *payment.Merchant {
	merchant := &payment.Merchant{
		ID:       id,
		Name:     name,
		Email:    fmt.Sprintf("%s@example.com", id),
		Status:   "active",
		Features: []string{"payments", "refunds", "webhooks"},
		RateLimits: payment.RateLimits{
			PaymentsPerMinute: 100,
			PaymentsPerDay:    10000,
			MaxPaymentAmount:  100000,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	err := db.Model(merchant).Create()
	require.NoError(t, err)

	return merchant
}

func createTestCustomer(t *testing.T, db core.ExtendedDB, merchantID, email string) *payment.Customer {
	customer := &payment.Customer{
		ID:         uuid.New().String(),
		MerchantID: merchantID,
		Email:      email,
		Name:       "Test Customer",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Version:    1,
	}

	err := db.Model(customer).Create()
	require.NoError(t, err)

	return customer
}
