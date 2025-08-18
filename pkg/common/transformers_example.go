package common

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ExampleUsage demonstrates how to use the transformation framework
func ExampleUsage() {
	logger := zap.NewNop()
	ctx := context.Background()

	// Example 1: Basic transformation
	stringToIntTransformer := NewBaseTransformer(
		"string_to_int",
		func(ctx context.Context, source string) (int, error) {
			// Simplified conversion for example
			switch source {
			case "one":
				return 1, nil
			case "two":
				return 2, nil
			case "three":
				return 3, nil
			default:
				return 0, fmt.Errorf("unknown number: %s", source)
			}
		},
		logger,
	)

	result, _ := stringToIntTransformer.Transform(ctx, "two")
	fmt.Printf("Basic transformation: 'two' -> %d\n", result)

	// Example 2: Cached transformation
	expensiveTransformer := NewBaseTransformer(
		"expensive_operation",
		func(ctx context.Context, source string) (string, error) {
			// Simulate expensive operation
			time.Sleep(time.Millisecond * 100)
			return fmt.Sprintf("processed_%s", source), nil
		},
		logger,
	).WithCache(
		time.Minute,     // Cache for 1 minute
		100,             // Max 100 entries
		func(s string) string { return s }, // Use input as cache key
	)

	// First call takes 100ms
	start := time.Now()
	result1, _ := expensiveTransformer.Transform(ctx, "test")
	duration1 := time.Since(start)

	// Second call uses cache (much faster)
	start = time.Now()
	result2, _ := expensiveTransformer.Transform(ctx, "test")
	duration2 := time.Since(start)

	fmt.Printf("Cached transformation - First: %v, Second: %v (cached)\n", duration1, duration2)
	fmt.Printf("Results identical: %t\n", result1 == result2)

	// Example 3: Batch transformation
	batchTransformer := NewBatchTransformer(
		"batch_processor",
		func(ctx context.Context, source int) (string, error) {
			return fmt.Sprintf("item_%d", source), nil
		},
		nil, // No custom batch function
		logger,
	)

	inputs := []int{1, 2, 3, 4, 5}
	results, _ := batchTransformer.TransformBatch(ctx, inputs)
	fmt.Printf("Batch transformation: %v -> %v\n", inputs, results)

	// Example 4: Conditional transformation
	conditionalTransformer := NewConditionalTransformer(
		"positive_only",
		func(ctx context.Context, source int) (string, error) {
			return fmt.Sprintf("positive_%d", source), nil
		},
		func(ctx context.Context, source int) bool {
			return source > 0 // Only transform positive numbers
		},
		logger,
	)

	// This will succeed
	positiveResult, _ := conditionalTransformer.Transform(ctx, 5)
	fmt.Printf("Conditional (positive): %s\n", positiveResult)

	// This will fail
	_, err := conditionalTransformer.Transform(ctx, -1)
	fmt.Printf("Conditional (negative): Error - %v\n", err)

	// Example 5: Validation wrapper
	validator := func(input string) error {
		if len(input) < 3 {
			return ValidationError{Field: "input", Message: "must be at least 3 characters"}
		}
		return nil
	}

	validatingTransformer := ValidatingTransformer(
		stringToIntTransformer,
		validator,
	)

	// This will fail validation
	_, err = validatingTransformer.Transform(ctx, "hi")
	fmt.Printf("Validation error: %v\n", err)

	// Example 6: Registry usage
	registry := NewTransformationRegistry(logger)
	registry.Register("string_to_int", stringToIntTransformer)
	registry.Register("expensive", expensiveTransformer)

	// Retrieve and use from registry
	if retrieved, exists := registry.Get("string_to_int"); exists {
		if transformer, ok := retrieved.(*BaseTransformer[string, int]); ok {
			result, _ := transformer.Transform(ctx, "three")
			fmt.Printf("Registry usage: 'three' -> %d\n", result)
		}
	}

	// Example 7: Metrics tracking
	metrics := NewTransformationMetrics()
	
	// Simulate some transformations with metrics
	for i := 0; i < 10; i++ {
		start := time.Now()
		_, err := stringToIntTransformer.Transform(ctx, "one")
		duration := time.Since(start)
		metrics.RecordTransformation(duration, err == nil)
	}

	count, errors, avgDuration, errorRate := metrics.GetStats()
	fmt.Printf("Metrics - Count: %d, Errors: %d, Avg Duration: %v, Error Rate: %.2f%%\n", 
		count, errors, avgDuration, errorRate*100)

	// Example 8: Transformation context
	transformCtx := NewTransformationContext("user123", "req456")
	transformCtx.WithMetadata("source", "api_endpoint")
	transformCtx.WithMetadata("version", "v1.0")

	fmt.Printf("Context - User: %s, Request: %s, Duration: %v\n", 
		transformCtx.UserID, transformCtx.RequestID, transformCtx.Duration())
}

// DomainSpecificExample shows how to create domain-specific transformers
func DomainSpecificExample() {
	logger := zap.NewNop()

	// ActivityPub Actor to Mastodon Account transformation example
	type ActivityPubActor struct {
		ID                string
		PreferredUsername string
		DisplayName       string
		FollowersCount    int
	}

	type MastodonAccount struct {
		ID             string
		Username       string
		DisplayName    string
		FollowersCount int
		URL            string
	}

	actorToAccountTransformer := NewBaseTransformer(
		"actor_to_account",
		func(ctx context.Context, actor ActivityPubActor) (MastodonAccount, error) {
			return MastodonAccount{
				ID:             actor.ID,
				Username:       actor.PreferredUsername,
				DisplayName:    actor.DisplayName,
				FollowersCount: actor.FollowersCount,
				URL:            fmt.Sprintf("https://example.com/@%s", actor.PreferredUsername),
			}, nil
		},
		logger,
	)

	// Example transformation
	actor := ActivityPubActor{
		ID:                "123",
		PreferredUsername: "alice",
		DisplayName:       "Alice Smith",
		FollowersCount:    150,
	}

	account, _ := actorToAccountTransformer.Transform(context.Background(), actor)
	fmt.Printf("Domain transformation: Actor -> Account\n")
	fmt.Printf("  ID: %s -> %s\n", actor.ID, account.ID)
	fmt.Printf("  Username: %s -> %s\n", actor.PreferredUsername, account.Username)
	fmt.Printf("  URL: -> %s\n", account.URL)
}

// PerformanceOptimizedExample demonstrates performance optimization patterns
func PerformanceOptimizedExample() {
	logger := zap.NewNop()

	// Create a high-performance batch transformer with caching
	transformer := NewBatchTransformer(
		"optimized_processor",
		func(ctx context.Context, source string) (string, error) {
			// Simulate processing
			return fmt.Sprintf("processed_%s", source), nil
		},
		func(ctx context.Context, sources []string) ([]string, error) {
			// Custom batch function for better performance
			results := make([]string, len(sources))
			for i, source := range sources {
				results[i] = fmt.Sprintf("batch_processed_%s", source)
			}
			return results, nil
		},
		logger,
	)

	// Add memoization for frequently accessed items
	memoizedTransformer := MemoizedTransformer(
		transformer,
		func(source string) string { return source },
		time.Minute*5,
	)

	ctx := context.Background()
	inputs := []string{"item1", "item2", "item3", "item1", "item2"} // Some duplicates

	start := time.Now()
	results, _ := transformer.TransformBatch(ctx, inputs)
	batchDuration := time.Since(start)

	start = time.Now()
	for _, input := range inputs {
		_, _ = memoizedTransformer.Transform(ctx, input)
	}
	memoizedDuration := time.Since(start)

	fmt.Printf("Performance comparison:\n")
	fmt.Printf("  Batch processing: %v\n", batchDuration)
	fmt.Printf("  Memoized processing: %v\n", memoizedDuration)
	fmt.Printf("  Results: %v\n", results)
}