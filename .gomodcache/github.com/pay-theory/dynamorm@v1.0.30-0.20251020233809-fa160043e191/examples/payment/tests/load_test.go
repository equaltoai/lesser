package tests

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/examples/payment"
	"github.com/pay-theory/dynamorm/examples/payment/utils"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormtests "github.com/pay-theory/dynamorm/tests"
)

// LoadTestConfig defines load test parameters
type LoadTestConfig struct {
	Duration            time.Duration
	PaymentsPerSecond   int
	MerchantCount       int
	CustomerPerMerchant int
	ReadWriteRatio      float64 // 0.8 means 80% reads, 20% writes
	BurstMultiplier     float64 // Multiplier for burst traffic
}

// LoadTestMetrics tracks test metrics
type LoadTestMetrics struct {
	TotalPayments      int64
	SuccessfulPayments int64
	FailedPayments     int64
	TotalQueries       int64
	SuccessfulQueries  int64
	FailedQueries      int64
	AvgPaymentLatency  time.Duration
	AvgQueryLatency    time.Duration
	P95PaymentLatency  time.Duration
	P95QueryLatency    time.Duration
	P99PaymentLatency  time.Duration
	P99QueryLatency    time.Duration
}

// TestRealisticLoad simulates realistic payment platform load
func TestRealisticLoad(t *testing.T) {
	dynamormtests.RequireDynamoDBLocal(t)

	config := LoadTestConfig{
		Duration:            5 * time.Minute,
		PaymentsPerSecond:   100,
		MerchantCount:       10,
		CustomerPerMerchant: 100,
		ReadWriteRatio:      0.8,
		BurstMultiplier:     3.0,
	}

	db, err := initLoadTestDB(t)
	if err != nil {
		t.Fatal(err)
	}

	// Create test data
	merchants := createLoadTestMerchants(t, db, config.MerchantCount)
	customers := createLoadTestCustomers(t, db, merchants, config.CustomerPerMerchant)

	// Initialize components
	idempotency := utils.NewIdempotencyMiddleware(db, 24*time.Hour)
	auditTracker := utils.NewAuditTracker(db)

	// Run load test
	metrics := runLoadTest(t, db, config, merchants, customers, idempotency, auditTracker)

	// Report results
	reportLoadTestResults(t, config, metrics)
}

// TestBurstTraffic simulates burst traffic scenarios
func TestBurstTraffic(t *testing.T) {
	dynamormtests.RequireDynamoDBLocal(t)

	db, err := initLoadTestDB(t)
	if err != nil {
		t.Fatal(err)
	}

	merchant := createLoadTestMerchants(t, db, 1)[0]

	// Normal load
	normalRate := 50
	// Burst load (5x normal)
	burstRate := 250

	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	// Metrics
	var normalCount, burstCount int64
	var normalErrors, burstErrors int64

	// Normal traffic goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Second / time.Duration(normalRate))
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := createPayment(db, merchant.ID, "normal"); err != nil {
					atomic.AddInt64(&normalErrors, 1)
				} else {
					atomic.AddInt64(&normalCount, 1)
				}
			case <-stopCh:
				return
			}
		}
	}()

	// Simulate 3 bursts
	for i := 0; i < 3; i++ {
		time.Sleep(10 * time.Second) // Normal traffic

		// Burst traffic for 5 seconds
		t.Logf("Starting burst %d", i+1)
		burstStop := make(chan struct{})

		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Second / time.Duration(burstRate))
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if err := createPayment(db, merchant.ID, "burst"); err != nil {
						atomic.AddInt64(&burstErrors, 1)
					} else {
						atomic.AddInt64(&burstCount, 1)
					}
				case <-burstStop:
					return
				}
			}
		}()

		time.Sleep(5 * time.Second)
		close(burstStop)
	}

	// Stop normal traffic
	close(stopCh)
	wg.Wait()

	// Report results
	t.Logf("Normal traffic: %d successful, %d errors", normalCount, normalErrors)
	t.Logf("Burst traffic: %d successful, %d errors", burstCount, burstErrors)

	// Verify system handled bursts
	errorRate := float64(burstErrors) / float64(burstCount+burstErrors)
	if errorRate > 0.01 { // 1% error rate threshold
		t.Errorf("High error rate during burst: %.2f%%", errorRate*100)
	}
}

// TestMultiRegionSimulation simulates multi-region deployment
func TestMultiRegionSimulation(t *testing.T) {
	dynamormtests.RequireDynamoDBLocal(t)

	regions := []string{"us-east-1", "us-west-2", "eu-west-1"}
	var wg sync.WaitGroup

	type regionMetrics struct {
		region   string
		payments int64
		errors   int64
		latency  time.Duration
	}

	results := make(chan regionMetrics, len(regions))

	for _, region := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()

			// Simulate region-specific DB connection
			db, err := dynamorm.New(dynamorm.Config{
				Region:   r,
				Endpoint: "http://localhost:8000", // Would be real endpoint
			})
			if err != nil {
				t.Errorf("Failed to connect to region %s: %v", r, err)
				return
			}

			// Register models
			if err := db.AutoMigrate(&payment.Payment{}, &payment.Merchant{}); err != nil {
				t.Errorf("Failed to auto-migrate in region %s: %v", r, err)
				return
			}

			// Create region-specific merchant
			merchant := &payment.Merchant{
				ID:     fmt.Sprintf("merchant-%s", r),
				Name:   fmt.Sprintf("Merchant %s", r),
				Status: "active",
			}
			_ = db.Model(merchant).Create()

			// Simulate load
			var payments, errors int64
			start := time.Now()

			for i := 0; i < 1000; i++ {
				if err := createPayment(db, merchant.ID, r); err != nil {
					atomic.AddInt64(&errors, 1)
				} else {
					atomic.AddInt64(&payments, 1)
				}
			}

			results <- regionMetrics{
				region:   r,
				payments: payments,
				errors:   errors,
				latency:  time.Since(start) / 1000,
			}
		}(region)
	}

	wg.Wait()
	close(results)

	// Collect and report results
	t.Log("Multi-region test results:")
	for result := range results {
		t.Logf("Region %s: %d payments, %d errors, avg latency: %v",
			result.region, result.payments, result.errors, result.latency)
	}
}

// Helper functions

func runLoadTest(_ *testing.T, db core.ExtendedDB, config LoadTestConfig,
	merchants []*payment.Merchant, customers []*payment.Customer,
	_ *utils.IdempotencyMiddleware, _ *utils.AuditTracker) *LoadTestMetrics {

	metrics := &LoadTestMetrics{}
	paymentLatencies := make([]time.Duration, 0, 10000)
	queryLatencies := make([]time.Duration, 0, 10000)

	var mu sync.Mutex
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), config.Duration)
	defer cancel()

	// Payment creation workers
	numWorkers := 20
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			ticker := time.NewTicker(time.Second / time.Duration(config.PaymentsPerSecond/numWorkers))
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// Randomly decide read or write based on ratio
					if rand.Float64() < config.ReadWriteRatio {
						// Read operation
						start := time.Now()
						merchant := merchants[rand.Intn(len(merchants))]

						var payments []*payment.Payment
						err := db.Model(&payment.Payment{}).
							Index("gsi-merchant").
							Where("MerchantID", "=", merchant.ID).
							Limit(20).
							All(&payments)

						latency := time.Since(start)
						mu.Lock()
						if err != nil {
							atomic.AddInt64(&metrics.FailedQueries, 1)
						} else {
							atomic.AddInt64(&metrics.SuccessfulQueries, 1)
							queryLatencies = append(queryLatencies, latency)
						}
						mu.Unlock()
					} else {
						// Write operation
						start := time.Now()
						merchant := merchants[rand.Intn(len(merchants))]
						customer := customers[rand.Intn(len(customers))]

						payment := &payment.Payment{
							ID:             uuid.New().String(),
							IdempotencyKey: uuid.New().String(),
							MerchantID:     merchant.ID,
							CustomerID:     customer.ID,
							Amount:         int64(rand.Intn(10000) + 100),
							Currency:       "USD",
							Status:         payment.PaymentStatusPending,
							PaymentMethod:  "card",
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
							Version:        1,
						}

						err := db.Model(payment).Create()
						latency := time.Since(start)

						mu.Lock()
						if err != nil {
							atomic.AddInt64(&metrics.FailedPayments, 1)
						} else {
							atomic.AddInt64(&metrics.SuccessfulPayments, 1)
							paymentLatencies = append(paymentLatencies, latency)
						}
						mu.Unlock()
					}

				case <-ctx.Done():
					return
				}
			}
		}(w)
	}

	// Wait for completion
	wg.Wait()

	// Calculate metrics
	atomic.StoreInt64(&metrics.TotalPayments, metrics.SuccessfulPayments+metrics.FailedPayments)
	atomic.StoreInt64(&metrics.TotalQueries, metrics.SuccessfulQueries+metrics.FailedQueries)

	// Calculate latency percentiles
	if len(paymentLatencies) > 0 {
		metrics.AvgPaymentLatency = calculateAverage(paymentLatencies)
		metrics.P95PaymentLatency = calculatePercentile(paymentLatencies, 95)
		metrics.P99PaymentLatency = calculatePercentile(paymentLatencies, 99)
	}

	if len(queryLatencies) > 0 {
		metrics.AvgQueryLatency = calculateAverage(queryLatencies)
		metrics.P95QueryLatency = calculatePercentile(queryLatencies, 95)
		metrics.P99QueryLatency = calculatePercentile(queryLatencies, 99)
	}

	return metrics
}

func createPayment(db core.ExtendedDB, merchantID, tag string) error {
	payment := &payment.Payment{
		ID:             fmt.Sprintf("%s-%s-%d", tag, uuid.New().String()[:8], time.Now().UnixNano()),
		IdempotencyKey: uuid.New().String(),
		MerchantID:     merchantID,
		Amount:         int64(rand.Intn(10000) + 100),
		Currency:       "USD",
		Status:         payment.PaymentStatusSucceeded,
		PaymentMethod:  "card",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Version:        1,
	}

	return db.Model(payment).Create()
}

func initLoadTestDB(_ *testing.T) (core.ExtendedDB, error) {
	config := dynamorm.Config{
		Region:     "us-east-1",
		Endpoint:   "http://localhost:8000",
		MaxRetries: 3,
	}

	db, err := dynamorm.New(config)
	if err != nil {
		return nil, err
	}

	// Register models
	if err := db.AutoMigrate(&payment.Payment{}, &payment.Merchant{}, &payment.Customer{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate: %w", err)
	}

	return db, nil
}

func createLoadTestMerchants(t *testing.T, db core.ExtendedDB, count int) []*payment.Merchant {
	merchants := make([]*payment.Merchant, count)

	for i := 0; i < count; i++ {
		merchant := &payment.Merchant{
			ID:       fmt.Sprintf("load-merchant-%d", i),
			Name:     fmt.Sprintf("Load Test Merchant %d", i),
			Email:    fmt.Sprintf("merchant%d@loadtest.com", i),
			Status:   "active",
			Features: []string{"payments", "refunds", "webhooks"},
			RateLimits: payment.RateLimits{
				PaymentsPerMinute: 1000,
				PaymentsPerDay:    100000,
				MaxPaymentAmount:  1000000,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Version:   1,
		}

		if err := db.Model(merchant).Create(); err != nil {
			t.Fatalf("Failed to create merchant: %v", err)
		}

		merchants[i] = merchant
	}

	return merchants
}

func createLoadTestCustomers(t *testing.T, db core.ExtendedDB, merchants []*payment.Merchant, perMerchant int) []*payment.Customer {
	var customers []*payment.Customer

	for _, merchant := range merchants {
		for i := 0; i < perMerchant; i++ {
			customer := &payment.Customer{
				ID:         fmt.Sprintf("load-customer-%s-%d", merchant.ID, i),
				MerchantID: merchant.ID,
				Email:      fmt.Sprintf("customer%d@merchant%s.com", i, merchant.ID),
				Name:       fmt.Sprintf("Load Customer %d", i),
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
				Version:    1,
			}

			if err := db.Model(customer).Create(); err != nil {
				t.Fatalf("Failed to create customer: %v", err)
			}

			customers = append(customers, customer)
		}
	}

	return customers
}

func calculateAverage(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}

	return sum / time.Duration(len(latencies))
}

func calculatePercentile(latencies []time.Duration, percentile int) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	// Sort latencies
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)

	// Simple bubble sort for demonstration
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := (len(sorted) * percentile) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

func reportLoadTestResults(t *testing.T, config LoadTestConfig, metrics *LoadTestMetrics) {
	t.Log("=== Load Test Results ===")
	t.Logf("Duration: %v", config.Duration)
	t.Logf("Target Rate: %d payments/sec", config.PaymentsPerSecond)
	t.Log("")

	t.Log("Payment Operations:")
	t.Logf("  Total: %d", metrics.TotalPayments)
	t.Logf("  Successful: %d (%.2f%%)", metrics.SuccessfulPayments,
		float64(metrics.SuccessfulPayments)/float64(metrics.TotalPayments)*100)
	t.Logf("  Failed: %d", metrics.FailedPayments)
	t.Logf("  Average Latency: %v", metrics.AvgPaymentLatency)
	t.Logf("  P95 Latency: %v", metrics.P95PaymentLatency)
	t.Logf("  P99 Latency: %v", metrics.P99PaymentLatency)
	t.Log("")

	t.Log("Query Operations:")
	t.Logf("  Total: %d", metrics.TotalQueries)
	t.Logf("  Successful: %d (%.2f%%)", metrics.SuccessfulQueries,
		float64(metrics.SuccessfulQueries)/float64(metrics.TotalQueries)*100)
	t.Logf("  Failed: %d", metrics.FailedQueries)
	t.Logf("  Average Latency: %v", metrics.AvgQueryLatency)
	t.Logf("  P95 Latency: %v", metrics.P95QueryLatency)
	t.Logf("  P99 Latency: %v", metrics.P99QueryLatency)
	t.Log("")

	t.Log("Overall Performance:")
	actualRate := float64(metrics.TotalPayments) / config.Duration.Seconds()
	t.Logf("  Actual Rate: %.2f payments/sec", actualRate)
	t.Logf("  Success Rate: %.2f%%",
		float64(metrics.SuccessfulPayments+metrics.SuccessfulQueries)/
			float64(metrics.TotalPayments+metrics.TotalQueries)*100)
}
