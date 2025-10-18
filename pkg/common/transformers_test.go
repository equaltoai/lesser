package common

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap"
)

// Test types for transformation examples
type SourceData struct {
	ID    string
	Value int
	Name  string
}

type TargetData struct {
	Identifier string
	Number     int
	Label      string
	Processed  bool
}

func TestBaseTransformer(t *testing.T) {
	logger := zap.NewNop()

	// Create a simple transformer
	transformer := NewBaseTransformer(
		"test_transformer",
		func(ctx context.Context, source SourceData) (TargetData, error) {
			return TargetData{
				Identifier: source.ID,
				Number:     source.Value * 2,
				Label:      fmt.Sprintf("processed_%s", source.Name),
				Processed:  true,
			}, nil
		},
		logger,
	)

	ctx := context.Background()
	source := SourceData{ID: "123", Value: 42, Name: "test"}

	result, err := transformer.Transform(ctx, source)
	if err != nil {
		t.Fatalf("transformation failed: %v", err)
	}

	if result.Identifier != "123" {
		t.Errorf("expected identifier '123', got '%s'", result.Identifier)
	}
	if result.Number != 84 {
		t.Errorf("expected number 84, got %d", result.Number)
	}
	if result.Label != "processed_test" {
		t.Errorf("expected label 'processed_test', got '%s'", result.Label)
	}
	if !result.Processed {
		t.Error("expected processed to be true")
	}
}

func TestBatchTransformer(t *testing.T) {
	logger := zap.NewNop()

	transformer := NewBatchTransformer(
		"batch_transformer",
		func(ctx context.Context, source SourceData) (TargetData, error) {
			return TargetData{
				Identifier: source.ID,
				Number:     source.Value,
				Label:      source.Name,
				Processed:  true,
			}, nil
		},
		nil, // No batch function, will use individual transforms
		logger,
	)

	ctx := context.Background()
	sources := []SourceData{
		{ID: "1", Value: 10, Name: "first"},
		{ID: "2", Value: 20, Name: "second"},
		{ID: "3", Value: 30, Name: "third"},
	}

	results, err := transformer.TransformBatch(ctx, sources)
	if err != nil {
		t.Fatalf("batch transformation failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	for i, result := range results {
		expected := sources[i]
		if result.Identifier != expected.ID {
			t.Errorf("result %d: expected identifier '%s', got '%s'",
				i, expected.ID, result.Identifier)
		}
		if !result.Processed {
			t.Errorf("result %d: expected processed to be true", i)
		}
	}
}

func TestTransformationCache(t *testing.T) {
	logger := zap.NewNop()

	// Create a transformer with caching
	callCount := 0
	transformer := NewBaseTransformer(
		"cached_transformer",
		func(ctx context.Context, source SourceData) (TargetData, error) {
			callCount++
			return TargetData{
				Identifier: source.ID,
				Number:     source.Value,
				Label:      fmt.Sprintf("call_%d", callCount),
				Processed:  true,
			}, nil
		},
		logger,
	).WithCache(
		time.Minute,
		100,
		func(source SourceData) string { return source.ID },
	)

	ctx := context.Background()
	source := SourceData{ID: "cache_test", Value: 100, Name: "test"}

	// First call should execute the function
	result1, err := transformer.Transform(ctx, source)
	if err != nil {
		t.Fatalf("first transformation failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 function call, got %d", callCount)
	}

	// Second call should use cache
	result2, err := transformer.Transform(ctx, source)
	if err != nil {
		t.Fatalf("second transformation failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 function call after caching, got %d", callCount)
	}

	// Results should be identical
	if result1.Label != result2.Label {
		t.Errorf("cached result differs: '%s' vs '%s'", result1.Label, result2.Label)
	}
}

func TestConditionalTransformer(t *testing.T) {
	logger := zap.NewNop()

	transformer := NewConditionalTransformer(
		"conditional_transformer",
		func(ctx context.Context, source SourceData) (TargetData, error) {
			return TargetData{
				Identifier: source.ID,
				Number:     source.Value,
				Label:      source.Name,
				Processed:  true,
			}, nil
		},
		func(ctx context.Context, source SourceData) bool {
			return source.Value > 50 // Only transform if value > 50
		},
		logger,
	)

	ctx := context.Background()

	// Test with value that meets condition
	source1 := SourceData{ID: "1", Value: 100, Name: "high"}
	result1, err := transformer.Transform(ctx, source1)
	if err != nil {
		t.Errorf("transformation should succeed for high value: %v", err)
	}
	if !result1.Processed {
		t.Error("high value should be processed")
	}

	// Test with value that doesn't meet condition
	source2 := SourceData{ID: "2", Value: 25, Name: "low"}
	_, err = transformer.Transform(ctx, source2)
	if err == nil {
		t.Error("transformation should fail for low value")
	}
}

func TestValidatingTransformer(t *testing.T) {
	logger := zap.NewNop()

	baseTransformer := NewBaseTransformer(
		"base_transformer",
		func(ctx context.Context, source SourceData) (TargetData, error) {
			return TargetData{
				Identifier: source.ID,
				Number:     source.Value,
				Label:      source.Name,
				Processed:  true,
			}, nil
		},
		logger,
	)

	validator := func(source SourceData) error {
		if source.ID == "" {
			return ValidationError{Field: "id", Message: "cannot be empty"}
		}
		if source.Value < 0 {
			return ValidationError{Field: "value", Message: "cannot be negative"}
		}
		return nil
	}

	transformer := ValidatingTransformer(baseTransformer, validator)
	ctx := context.Background()

	// Test valid input
	validSource := SourceData{ID: "valid", Value: 42, Name: "test"}
	_, err := transformer.Transform(ctx, validSource)
	if err != nil {
		t.Errorf("valid input should not fail: %v", err)
	}

	// Test invalid input (empty ID)
	invalidSource1 := SourceData{ID: "", Value: 42, Name: "test"}
	_, err = transformer.Transform(ctx, invalidSource1)
	if err == nil {
		t.Error("empty ID should cause validation error")
	}

	// Test invalid input (negative value)
	invalidSource2 := SourceData{ID: "test", Value: -1, Name: "test"}
	_, err = transformer.Transform(ctx, invalidSource2)
	if err == nil {
		t.Error("negative value should cause validation error")
	}
}

func TestTransformationRegistry(t *testing.T) {
	logger := zap.NewNop()
	registry := NewTransformationRegistry(logger)

	// Create a transformer to register
	transformer := NewBaseTransformer(
		"registry_test",
		func(ctx context.Context, source string) (int, error) {
			return strconv.Atoi(source)
		},
		logger,
	)

	// Test registration
	err := registry.Register("string_to_int", transformer)
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Test retrieval
	retrieved, exists := registry.Get("string_to_int")
	if !exists {
		t.Error("transformer should exist in registry")
	}
	if retrieved == nil {
		t.Error("retrieved transformer should not be nil")
	}

	// Test duplicate registration
	err = registry.Register("string_to_int", transformer)
	if err == nil {
		t.Error("duplicate registration should fail")
	}

	// Test listing
	names := registry.List()
	if len(names) != 1 {
		t.Errorf("expected 1 transformer name, got %d", len(names))
	}
	if names[0] != "string_to_int" {
		t.Errorf("expected 'string_to_int', got '%s'", names[0])
	}

	// Test unregistration
	removed := registry.Unregister("string_to_int")
	if !removed {
		t.Error("unregistration should succeed")
	}

	// Verify removal
	_, exists = registry.Get("string_to_int")
	if exists {
		t.Error("transformer should not exist after unregistration")
	}
}

func TestTransformationMetrics(t *testing.T) {
	metrics := NewTransformationMetrics()

	// Record some transformations
	metrics.RecordTransformation(time.Millisecond*100, true)
	metrics.RecordTransformation(time.Millisecond*200, true)
	metrics.RecordTransformation(time.Millisecond*150, false) // Error case

	count, errors, avgDuration, errorRate := metrics.GetStats()

	if count != 3 {
		t.Errorf("expected 3 transformations, got %d", count)
	}
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}

	expectedAvg := time.Millisecond * 150 // (100 + 200 + 150) / 3
	if avgDuration != expectedAvg {
		t.Errorf("expected average duration %v, got %v", expectedAvg, avgDuration)
	}

	expectedErrorRate := 1.0 / 3.0
	if errorRate != expectedErrorRate {
		t.Errorf("expected error rate %f, got %f", expectedErrorRate, errorRate)
	}
}

func TestTransformationContext(t *testing.T) {
	ctx := NewTransformationContext("user123", "req456")

	if ctx.UserID != "user123" {
		t.Errorf("expected user ID 'user123', got '%s'", ctx.UserID)
	}
	if ctx.RequestID != "req456" {
		t.Errorf("expected request ID 'req456', got '%s'", ctx.RequestID)
	}

	// Test metadata
	ctx.WithMetadata("test_key", "test_value")
	value, exists := ctx.GetMetadata("test_key")
	if !exists {
		t.Error("metadata should exist")
	}
	if value != "test_value" {
		t.Errorf("expected 'test_value', got '%v'", value)
	}

	// Test duration
	time.Sleep(time.Millisecond * 10)
	duration := ctx.Duration()
	if duration < time.Millisecond*10 {
		t.Errorf("duration should be at least 10ms, got %v", duration)
	}
}

func TestIdentityTransformer(t *testing.T) {
	ctx := context.Background()
	input := "test_string"

	result, err := IdentityTransformer(ctx, input)
	if err != nil {
		t.Fatalf("identity transformer should not fail: %v", err)
	}

	if result != input {
		t.Errorf("identity transformer should return input unchanged: expected '%s', got '%s'",
			input, result)
	}
}

func TestMemoizedTransformer(t *testing.T) {
	callCount := 0
	baseTransformer := NewBaseTransformer(
		"memoize_test",
		func(ctx context.Context, source string) (string, error) {
			callCount++
			return fmt.Sprintf("processed_%s_%d", source, callCount), nil
		},
		nil,
	)

	memoized := MemoizedTransformer(
		baseTransformer,
		func(source string) string { return source },
		time.Minute,
	)

	ctx := context.Background()

	// First call
	result1, err := memoized.Transform(ctx, "test")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call should be memoized
	result2, err := memoized.Transform(ctx, "test")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call after memoization, got %d", callCount)
	}

	// Results should be identical
	if result1 != result2 {
		t.Errorf("memoized results should be identical: '%s' vs '%s'", result1, result2)
	}
}

// Benchmark tests to ensure performance is reasonable
func BenchmarkBaseTransformer(b *testing.B) {
	transformer := NewBaseTransformer(
		"benchmark_transformer",
		func(ctx context.Context, source int) (string, error) {
			return strconv.Itoa(source * 2), nil
		},
		zap.NewNop(),
	)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := transformer.Transform(ctx, i)
		if err != nil {
			b.Fatalf("transformation failed: %v", err)
		}
	}
}

func BenchmarkCachedTransformer(b *testing.B) {
	transformer := NewBaseTransformer(
		"cached_benchmark",
		func(ctx context.Context, source int) (string, error) {
			// Simulate some work
			time.Sleep(time.Microsecond)
			return strconv.Itoa(source * 2), nil
		},
		zap.NewNop(),
	).WithCache(
		time.Minute,
		1000,
		func(source int) string { return strconv.Itoa(source) },
	)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use modulo to create cache hits
		_, err := transformer.Transform(ctx, i%100)
		if err != nil {
			b.Fatalf("transformation failed: %v", err)
		}
	}
}
