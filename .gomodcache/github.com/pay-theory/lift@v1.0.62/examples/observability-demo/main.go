package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pay-theory/lift/pkg/observability"
	"github.com/pay-theory/lift/pkg/observability/cloudwatch"
	"github.com/pay-theory/lift/pkg/observability/zap"
)

func main() {
	fmt.Println("🚀 Lift Observability Demo")
	fmt.Println("==========================")

	// Demo 1: Zap Logger (for local development)
	fmt.Println("\n📝 Demo 1: Zap Logger")
	demoZapLogger()

	// Demo 2: CloudWatch Logger with Mock (for testing)
	fmt.Println("\n☁️  Demo 2: CloudWatch Logger (Mock)")
	demoCloudWatchLoggerMock()

	// Demo 3: CloudWatch Logger with Real AWS (commented out - requires AWS credentials)
	fmt.Println("\n🔒 Demo 3: CloudWatch Logger (Real AWS) - Skipped (requires AWS credentials)")
	// demoCloudWatchLoggerReal()

	// Demo 4: Multi-tenant Logging
	fmt.Println("\n🏢 Demo 4: Multi-tenant Logging")
	demoMultiTenantLogging()

	// Demo 5: Performance and Stats
	fmt.Println("\n📊 Demo 5: Performance and Stats")
	demoPerformanceAndStats()

	fmt.Println("\n✅ All demos completed successfully!")
}

func demoZapLogger() {
	// Create Zap logger factory
	factory := zap.NewZapLoggerFactory()

	// Create console logger for development
	config := observability.LoggerConfig{
		Level:  "debug", // Debug level available but with enhanced sanitization
		Format: "console",
	}

	logger, err := factory.CreateConsoleLogger(config)
	if err != nil {
		log.Fatalf("Failed to create Zap logger: %v", err)
	}
	defer func() {
		if err := logger.Close(); err != nil {
			log.Printf("Error closing logger: %v", err)
		}
	}()

	// Basic logging with enhanced sanitization for security
	logger.Debug("Debug message from Zap (sanitized)")
	logger.Info("Info message from Zap", map[string]any{
		"component": "demo",
		"version":   "1.0.0",
	})
	logger.Warn("Warning message from Zap")
	logger.Error("Error message from Zap", map[string]any{
		"error_code": "DEMO_ERROR",
		"details":    "[REDACTED_ERROR_DETAIL]",
	})

	// Context logging
	contextLogger := logger.
		WithRequestID("req-12345").
		WithTenantID("tenant-abc").
		WithUserID("user-xyz")

	contextLogger.Info("Message with context")

	// Check stats
	stats := logger.GetStats()
	fmt.Printf("   Zap Logger Stats: %d entries logged, %d errors\n",
		stats.EntriesLogged, stats.ErrorCount)
}

func demoCloudWatchLoggerMock() {
	// Create mock CloudWatch client for testing
	mockClient := cloudwatch.NewMockCloudWatchLogsClient()

	// Configure CloudWatch logger
	config := observability.LoggerConfig{
		LogGroup:      "/aws/lambda/lift-demo",
		LogStream:     fmt.Sprintf("demo-stream-%d", time.Now().Unix()),
		BatchSize:     5,
		FlushInterval: 2 * time.Second,
		BufferSize:    20,
		Level:         "info",
		Format:        "json",
	}

	// Create CloudWatch logger
	logger, err := cloudwatch.NewCloudWatchLogger(config, mockClient)
	if err != nil {
		log.Fatalf("Failed to create CloudWatch logger: %v", err)
	}
	defer func() {
		if err := logger.Close(); err != nil {
			log.Printf("Error closing logger: %v", err)
		}
	}()

	// Log some messages
	logger.Info("CloudWatch demo started", map[string]any{
		"demo_type": "mock",
		"timestamp": time.Now().Unix(),
	})

	// Multi-tenant logging
	tenantLogger := logger.
		WithTenantID("tenant-123").
		WithUserID("user-456")

	tenantLogger.Info("Tenant-specific operation", map[string]any{
		"operation": "create_payment",
		"amount":    1000,
		"currency":  "USD",
	})

	tenantLogger.Error("Payment processing failed", map[string]any{
		"error":      "insufficient_funds",
		"account_id": "acc-789",
	})

	// Force flush to see immediate results
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := logger.Flush(ctx); err != nil {
		log.Printf("Failed to flush logger: %v", err)
	}

	// Check mock client stats
	fmt.Printf("   Mock CloudWatch Stats:\n")
	fmt.Printf("   - CreateLogGroup calls: %d\n", mockClient.GetCallCount("CreateLogGroup"))
	fmt.Printf("   - CreateLogStream calls: %d\n", mockClient.GetCallCount("CreateLogStream"))
	fmt.Printf("   - PutLogEvents calls: %d\n", mockClient.GetCallCount("PutLogEvents"))
	fmt.Printf("   - Log events stored: %d\n", len(mockClient.GetLogEvents()))

	// Check logger stats
	stats := logger.GetStats()
	fmt.Printf("   Logger Stats: %d entries logged, %d dropped, %d flushes\n",
		stats.EntriesLogged, stats.EntriesDropped, stats.FlushCount)
}

func demoMultiTenantLogging() {
	// Create mock client
	mockClient := cloudwatch.NewMockCloudWatchLogsClient()

	config := observability.LoggerConfig{
		LogGroup:      "/aws/lambda/lift-multitenant",
		LogStream:     "multitenant-demo",
		BatchSize:     3,
		FlushInterval: 1 * time.Second,
		BufferSize:    15,
	}

	logger, err := cloudwatch.NewCloudWatchLogger(config, mockClient)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		if err := logger.Close(); err != nil {
			log.Printf("Error closing logger: %v", err)
		}
	}()

	// Simulate multiple tenants
	tenants := []struct {
		tenantID string
		userID   string
		action   string
	}{
		{"tenant-paytheory", "user-admin", "create_merchant"},
		{"tenant-acme", "user-john", "process_payment"},
		{"tenant-globex", "user-jane", "refund_transaction"},
	}

	for _, tenant := range tenants {
		tenantLogger := logger.
			WithTenantID(tenant.tenantID).
			WithUserID(tenant.userID).
			WithTraceID(fmt.Sprintf("trace-%s-%d", tenant.tenantID, time.Now().UnixNano()))

		tenantLogger.Info("Tenant operation", map[string]any{
			"action":    tenant.action,
			"timestamp": time.Now().Unix(),
			"source":    "api",
		})

		// Simulate some tenant-specific error
		if tenant.tenantID == "tenant-acme" {
			tenantLogger.Error("Tenant-specific error", map[string]any{
				"error_type": "rate_limit_exceeded",
				"limit":      1000,
				"current":    1001,
			})
		}
	}

	// Wait for flush
	time.Sleep(2 * time.Second)

	fmt.Printf("   Multi-tenant logging completed\n")
	fmt.Printf("   - Total log events: %d\n", len(mockClient.GetLogEvents()))

	stats := logger.GetStats()
	fmt.Printf("   - Entries logged: %d\n", stats.EntriesLogged)
}

func demoPerformanceAndStats() {
	// Create high-performance logger configuration
	mockClient := cloudwatch.NewMockCloudWatchLogsClient()

	config := observability.LoggerConfig{
		LogGroup:      "/aws/lambda/lift-performance",
		LogStream:     "performance-test",
		BatchSize:     10,
		FlushInterval: 500 * time.Millisecond,
		BufferSize:    100,
	}

	logger, err := cloudwatch.NewCloudWatchLogger(config, mockClient)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		if err := logger.Close(); err != nil {
			log.Printf("Error closing logger: %v", err)
		}
	}()

	// Performance test: log many messages quickly
	start := time.Now()
	messageCount := 50

	for i := 0; i < messageCount; i++ {
		logger.Info("Performance test message", map[string]any{
			"message_id": i,
			"batch":      i / 10,
			"timestamp":  time.Now().UnixNano(),
		})
	}

	loggingDuration := time.Since(start)

	// Wait for all flushes to complete
	time.Sleep(1 * time.Second)

	// Get final stats
	stats := logger.GetStats()

	fmt.Printf("   Performance Test Results:\n")
	fmt.Printf("   - Messages logged: %d in %v\n", messageCount, loggingDuration)
	fmt.Printf("   - Average per message: %v\n", loggingDuration/time.Duration(messageCount))
	fmt.Printf("   - Entries logged: %d\n", stats.EntriesLogged)
	fmt.Printf("   - Entries dropped: %d\n", stats.EntriesDropped)
	fmt.Printf("   - Flush count: %d\n", stats.FlushCount)
	fmt.Printf("   - Average flush time: %v\n", stats.AverageFlushTime)
	fmt.Printf("   - Buffer utilization: %d/%d\n", stats.BufferSize, stats.BufferCapacity)
	fmt.Printf("   - Error count: %d\n", stats.ErrorCount)
	fmt.Printf("   - Healthy: %t\n", logger.IsHealthy())

	// Verify performance target (<1ms per log entry)
	avgPerMessage := loggingDuration / time.Duration(messageCount)
	if avgPerMessage < 1*time.Millisecond {
		fmt.Printf("   ✅ Performance target met: %v < 1ms\n", avgPerMessage)
	} else {
		fmt.Printf("   ⚠️  Performance target missed: %v >= 1ms\n", avgPerMessage)
	}
}
