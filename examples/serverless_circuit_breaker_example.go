package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/circuit"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// Example of how to use the new serverless circuit breaker
func main() {
	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// In a real application, you would initialize DynamORM with your actual DynamoDB connection
	// This is just to show the pattern
	var db core.DB // This would be initialized with actual DynamORM connection
	tableName := "your-dynamodb-table"

	// Create the circuit breaker repository
	circuitRepo := repositories.NewCircuitBreakerRepository(db, tableName, logger)

	// Create the serverless circuit breaker directly with the repository
	config := models.DefaultCircuitBreakerConfig()
	serverlessBreaker := circuit.NewServerlessCircuitBreaker(circuitRepo, config, logger)

	ctx := context.Background()
	instanceID := "mastodon.social"

	// Example usage pattern
	fmt.Println("=== Serverless Circuit Breaker Example ===")
	
	// Check if we can make a request
	if !serverlessBreaker.CanAttempt(ctx, instanceID) {
		fmt.Printf("Circuit is open for %s, cannot attempt request\n", instanceID)
		return
	}

	// Simulate making a request
	fmt.Printf("Making request to %s...\n", instanceID)
	
	// Simulate request success or failure
	requestSucceeded := true // This would be determined by your actual request
	
	if requestSucceeded {
		// Record success
		if err := serverlessBreaker.RecordSuccess(ctx, instanceID); err != nil {
			log.Printf("Failed to record success: %v", err)
		}
		fmt.Printf("Request to %s succeeded\n", instanceID)
	} else {
		// Record failure
		requestError := fmt.Errorf("connection timeout")
		if err := serverlessBreaker.RecordFailure(ctx, instanceID, requestError); err != nil {
			log.Printf("Failed to record failure: %v", err)
		}
		fmt.Printf("Request to %s failed: %v\n", instanceID, requestError)
	}

	// Get current metrics
	metrics := serverlessBreaker.GetMetrics(ctx, instanceID)
	fmt.Printf("Circuit metrics for %s:\n", instanceID)
	fmt.Printf("  Status: %v\n", metrics["status"])
	fmt.Printf("  Total Requests: %v\n", metrics["totalRequests"])
	fmt.Printf("  Success Rate: %.2f%%\n", metrics["successRate"].(float64)*100)
	fmt.Printf("  Consecutive Failures: %v\n", metrics["consecutiveFails"])

	// The key advantages of this serverless approach:
	fmt.Println("\n=== Key Advantages ===")
	fmt.Println("✅ No in-memory state - survives Lambda freezing/thawing")
	fmt.Println("✅ No background goroutines - compatible with serverless")
	fmt.Println("✅ Event-driven state evaluation - only when needed")
	fmt.Println("✅ DynamORM integration - type-safe, optimized queries")
	fmt.Println("✅ Automatic TTL cleanup - no manual maintenance")
	fmt.Println("✅ Detailed event logging - for debugging and monitoring")
	fmt.Println("✅ Configurable thresholds and backoff - flexible behavior")
}

// Example of integration in a federation delivery function
func ExampleFederationDelivery(ctx context.Context, breaker *circuit.ServerlessCircuitBreaker, instanceID string, payload []byte) error {
	// Check circuit state before attempting delivery
	if breaker.IsOpen(ctx, instanceID) {
		return fmt.Errorf("circuit breaker is open for instance %s", instanceID)
	}

	// Check if we can attempt (handles half-open transitions)
	if !breaker.CanAttempt(ctx, instanceID) {
		return fmt.Errorf("circuit breaker prevents attempts to instance %s", instanceID)
	}

	// Attempt delivery
	err := deliverPayload(instanceID, payload) // Your actual delivery logic
	
	// Record the result
	if err != nil {
		if recordErr := breaker.RecordFailure(ctx, instanceID, err); recordErr != nil {
			log.Printf("Failed to record circuit breaker failure: %v", recordErr)
		}
		return fmt.Errorf("delivery failed: %w", err)
	} else {
		if recordErr := breaker.RecordSuccess(ctx, instanceID); recordErr != nil {
			log.Printf("Failed to record circuit breaker success: %v", recordErr)
		}
		return nil
	}
}

// Mock delivery function
func deliverPayload(instanceID string, payload []byte) error {
	// Your actual federation delivery logic would go here
	// This might involve HTTP requests, signature verification, etc.
	
	// Simulate occasional failures
	if time.Now().UnixNano()%7 == 0 {
		return fmt.Errorf("simulated network timeout")
	}
	
	return nil
}