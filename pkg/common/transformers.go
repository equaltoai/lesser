package common

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Transformer defines the core interface for data transformations
// This interface provides type-safe transformations with error handling
type Transformer[TSource, TTarget any] interface {
	Transform(ctx context.Context, source TSource) (TTarget, error)
}

// BatchTransformer extends Transformer to handle batch operations efficiently
type BatchTransformer[TSource, TTarget any] interface {
	Transformer[TSource, TTarget]
	TransformBatch(ctx context.Context, sources []TSource) ([]TTarget, error)
}

// ConditionalTransformer allows transformation based on runtime conditions
type ConditionalTransformer[TSource, TTarget any] interface {
	Transformer[TSource, TTarget]
	CanTransform(ctx context.Context, source TSource) bool
}

// TransformationStep represents a single step in a transformation pipeline
type TransformationStep[T any] struct {
	Name        string
	Transformer Transformer[T, T]
	Required    bool // If true, failure stops the pipeline
}

// TransformationPipeline provides a framework for chaining transformations
type TransformationPipeline[TSource, TTarget any] struct {
	steps   []TransformationStep[any] //nolint:unused // Framework pattern - will be used when pipeline is implemented
	logger  *zap.Logger               //nolint:unused // Framework pattern - will be used when pipeline is implemented
	metrics *TransformationMetrics    //nolint:unused // Framework pattern - will be used when pipeline is implemented
}

// TransformationMetrics tracks performance and success rates
type TransformationMetrics struct {
	mu                sync.RWMutex
	transformCount    int64
	errorCount        int64
	totalDuration     time.Duration
	lastTransformTime time.Time
}

// TransformationCache provides caching for expensive transformations
type TransformationCache[TSource, TTarget any] struct {
	cache     sync.Map
	ttl       time.Duration
	maxSize   int
	keyFunc   func(TSource) string
	logger    *zap.Logger
	hitCount  int64
	missCount int64
}

// CacheEntry represents a cached transformation result
type CacheEntry[T any] struct {
	Value     T
	CreatedAt time.Time
}

// TransformationRegistry manages named transformers for dynamic lookup
type TransformationRegistry struct {
	mu           sync.RWMutex
	transformers map[string]interface{}
	logger       *zap.Logger
}

// TransformationContext carries metadata through transformation pipelines
type TransformationContext struct {
	UserID       string
	RequestID    string
	Metadata     map[string]interface{}
	StartTime    time.Time
	Transformers map[string]interface{}
}

// TransformationError provides detailed error information for failed transformations
type TransformationError struct {
	Step       string
	SourceType string
	TargetType string
	Cause      error
}

func (e TransformationError) Error() string {
	return fmt.Sprintf("transformation failed at step '%s' (%s -> %s): %v",
		e.Step, e.SourceType, e.TargetType, e.Cause)
}

func (e TransformationError) Unwrap() error {
	return e.Cause
}

// BaseTransformer provides a concrete implementation of the Transformer interface
type BaseTransformer[TSource, TTarget any] struct {
	name        string
	transformFn func(context.Context, TSource) (TTarget, error)
	logger      *zap.Logger
	cache       *TransformationCache[TSource, TTarget]
}

// NewBaseTransformer creates a new BaseTransformer with the given function
func NewBaseTransformer[TSource, TTarget any](
	name string,
	transformFn func(context.Context, TSource) (TTarget, error),
	logger *zap.Logger,
) *BaseTransformer[TSource, TTarget] {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &BaseTransformer[TSource, TTarget]{
		name:        name,
		transformFn: transformFn,
		logger:      logger,
	}
}

// Transform executes the transformation function
func (bt *BaseTransformer[TSource, TTarget]) Transform(ctx context.Context, source TSource) (TTarget, error) {
	var zero TTarget

	if bt.transformFn == nil {
		return zero, TransformationError{
			Step:       bt.name,
			SourceType: fmt.Sprintf("%T", source),
			TargetType: fmt.Sprintf("%T", zero),
			Cause:      ErrTransformFunctionNotSet,
		}
	}

	// Check cache if available
	if bt.cache != nil {
		if cached, found := bt.cache.Get(source); found {
			return cached, nil
		}
	}

	start := time.Now()
	result, err := bt.transformFn(ctx, source)
	duration := time.Since(start)

	bt.logger.Debug("transformation completed",
		zap.String("transformer", bt.name),
		zap.Duration("duration", duration),
		zap.Error(err))

	if err != nil {
		return zero, TransformationError{
			Step:       bt.name,
			SourceType: fmt.Sprintf("%T", source),
			TargetType: fmt.Sprintf("%T", zero),
			Cause:      err,
		}
	}

	// Cache result if caching is enabled
	if bt.cache != nil {
		bt.cache.Set(source, result)
	}

	return result, nil
}

// WithCache enables caching for this transformer
func (bt *BaseTransformer[TSource, TTarget]) WithCache(
	ttl time.Duration,
	maxSize int,
	keyFunc func(TSource) string,
) *BaseTransformer[TSource, TTarget] {
	bt.cache = NewTransformationCache[TSource, TTarget](ttl, maxSize, keyFunc, bt.logger)
	return bt
}

// BatchTransformerImpl provides efficient batch processing capabilities
type BatchTransformerImpl[TSource, TTarget any] struct {
	*BaseTransformer[TSource, TTarget]
	batchFn    func(context.Context, []TSource) ([]TTarget, error)
	batchSize  int
	concurrent bool
}

// NewBatchTransformer creates a new batch transformer
func NewBatchTransformer[TSource, TTarget any](
	name string,
	transformFn func(context.Context, TSource) (TTarget, error),
	batchFn func(context.Context, []TSource) ([]TTarget, error),
	logger *zap.Logger,
) *BatchTransformerImpl[TSource, TTarget] {
	return &BatchTransformerImpl[TSource, TTarget]{
		BaseTransformer: NewBaseTransformer(name, transformFn, logger),
		batchFn:         batchFn,
		batchSize:       100, // Default batch size
		concurrent:      false,
	}
}

// TransformBatch processes multiple items efficiently
func (bt *BatchTransformerImpl[TSource, TTarget]) TransformBatch(
	ctx context.Context,
	sources []TSource,
) ([]TTarget, error) {
	if len(sources) == 0 {
		return []TTarget{}, nil
	}

	// Use batch function if available and efficient
	if bt.batchFn != nil && len(sources) > 10 {
		return bt.batchFn(ctx, sources)
	}

	// Fall back to individual transformations
	results := make([]TTarget, 0, len(sources))

	if bt.concurrent && len(sources) > 5 {
		return bt.transformConcurrently(ctx, sources)
	}

	for _, source := range sources {
		result, err := bt.Transform(ctx, source)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

// transformConcurrently processes items in parallel for better performance
func (bt *BatchTransformerImpl[TSource, TTarget]) transformConcurrently(
	ctx context.Context,
	sources []TSource,
) ([]TTarget, error) {
	type result struct {
		index int
		value TTarget
		err   error
	}

	results := make([]TTarget, len(sources))
	resultChan := make(chan result, len(sources))

	// Start workers
	for i, source := range sources {
		go func(idx int, src TSource) {
			value, err := bt.Transform(ctx, src)
			resultChan <- result{index: idx, value: value, err: err}
		}(i, source)
	}

	// Collect results
	for i := 0; i < len(sources); i++ {
		res := <-resultChan
		if res.err != nil {
			return nil, res.err
		}
		results[res.index] = res.value
	}

	return results, nil
}

// NewTransformationCache creates a new cache for transformations
func NewTransformationCache[TSource, TTarget any](
	ttl time.Duration,
	maxSize int,
	keyFunc func(TSource) string,
	logger *zap.Logger,
) *TransformationCache[TSource, TTarget] {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &TransformationCache[TSource, TTarget]{
		ttl:     ttl,
		maxSize: maxSize,
		keyFunc: keyFunc,
		logger:  logger,
	}
}

// Get retrieves a cached transformation result
func (tc *TransformationCache[TSource, TTarget]) Get(source TSource) (TTarget, bool) {
	var zero TTarget

	if tc.keyFunc == nil {
		return zero, false
	}

	key := tc.keyFunc(source)
	if value, ok := tc.cache.Load(key); ok {
		if entry, ok := value.(CacheEntry[TTarget]); ok {
			// Check TTL
			if tc.ttl > 0 && time.Since(entry.CreatedAt) > tc.ttl {
				tc.cache.Delete(key)
				tc.missCount++
				return zero, false
			}

			tc.hitCount++
			return entry.Value, true
		}
	}

	tc.missCount++
	return zero, false
}

// Set stores a transformation result in the cache
func (tc *TransformationCache[TSource, TTarget]) Set(source TSource, target TTarget) {
	if tc.keyFunc == nil {
		return
	}

	key := tc.keyFunc(source)
	entry := CacheEntry[TTarget]{
		Value:     target,
		CreatedAt: time.Now(),
	}

	tc.cache.Store(key, entry)
}

// Clear removes all cached entries
func (tc *TransformationCache[TSource, TTarget]) Clear() {
	tc.cache.Range(func(key, _ interface{}) bool {
		tc.cache.Delete(key)
		return true
	})
}

// Stats returns cache performance statistics
func (tc *TransformationCache[TSource, TTarget]) Stats() (hits, misses int64, hitRate float64) {
	hits = tc.hitCount
	misses = tc.missCount
	total := hits + misses

	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return hits, misses, hitRate
}

// NewTransformationRegistry creates a new registry for managing transformers
func NewTransformationRegistry(logger *zap.Logger) *TransformationRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &TransformationRegistry{
		transformers: make(map[string]interface{}),
		logger:       logger,
	}
}

// Register adds a transformer to the registry
func (tr *TransformationRegistry) Register(name string, transformer interface{}) error {
	if name == "" {
		return ValidationError{Field: "name", Message: "transformer name cannot be empty"}
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()

	if _, exists := tr.transformers[name]; exists {
		return ValidationError{Field: "name", Message: fmt.Sprintf("transformer '%s' already registered", name)}
	}

	tr.transformers[name] = transformer
	tr.logger.Debug("transformer registered", zap.String("name", name))

	return nil
}

// Get retrieves a transformer from the registry
func (tr *TransformationRegistry) Get(name string) (interface{}, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	transformer, exists := tr.transformers[name]
	return transformer, exists
}

// List returns all registered transformer names
func (tr *TransformationRegistry) List() []string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	names := make([]string, 0, len(tr.transformers))
	for name := range tr.transformers {
		names = append(names, name)
	}

	return names
}

// Unregister removes a transformer from the registry
func (tr *TransformationRegistry) Unregister(name string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if _, exists := tr.transformers[name]; exists {
		delete(tr.transformers, name)
		tr.logger.Debug("transformer unregistered", zap.String("name", name))
		return true
	}

	return false
}

// NewTransformationContext creates a new transformation context
func NewTransformationContext(userID, requestID string) *TransformationContext {
	return &TransformationContext{
		UserID:       userID,
		RequestID:    requestID,
		Metadata:     make(map[string]interface{}),
		StartTime:    time.Now(),
		Transformers: make(map[string]interface{}),
	}
}

// WithMetadata adds metadata to the transformation context
func (tc *TransformationContext) WithMetadata(key string, value interface{}) *TransformationContext {
	tc.Metadata[key] = value
	return tc
}

// GetMetadata retrieves metadata from the transformation context
func (tc *TransformationContext) GetMetadata(key string) (interface{}, bool) {
	value, exists := tc.Metadata[key]
	return value, exists
}

// Duration returns the elapsed time since context creation
func (tc *TransformationContext) Duration() time.Duration {
	return time.Since(tc.StartTime)
}

// Utility transformation functions for common patterns

// IdentityTransformer returns the input unchanged (useful for pipeline testing)
func IdentityTransformer[T any](_ context.Context, input T) (T, error) {
	return input, nil
}

// ValidatingTransformer wraps a transformer with input validation
func ValidatingTransformer[TSource, TTarget any](
	transformer Transformer[TSource, TTarget],
	validator func(TSource) error,
) Transformer[TSource, TTarget] {
	return NewBaseTransformer(
		"validating_transformer",
		func(ctx context.Context, source TSource) (TTarget, error) {
			var zero TTarget

			if err := validator(source); err != nil {
				return zero, fmt.Errorf("validation failed: %w", err)
			}

			return transformer.Transform(ctx, source)
		},
		nil,
	)
}

// ConditionalTransformerImpl implements conditional transformation logic
type ConditionalTransformerImpl[TSource, TTarget any] struct {
	*BaseTransformer[TSource, TTarget]
	conditionFn func(context.Context, TSource) bool
}

// NewConditionalTransformer creates a transformer that only runs when condition is met
func NewConditionalTransformer[TSource, TTarget any](
	name string,
	transformFn func(context.Context, TSource) (TTarget, error),
	conditionFn func(context.Context, TSource) bool,
	logger *zap.Logger,
) *ConditionalTransformerImpl[TSource, TTarget] {
	return &ConditionalTransformerImpl[TSource, TTarget]{
		BaseTransformer: NewBaseTransformer(name, transformFn, logger),
		conditionFn:     conditionFn,
	}
}

// CanTransform checks if the transformation should be applied
func (ct *ConditionalTransformerImpl[TSource, TTarget]) CanTransform(
	ctx context.Context,
	source TSource,
) bool {
	if ct.conditionFn == nil {
		return true
	}
	return ct.conditionFn(ctx, source)
}

// Transform only executes if the condition is met
func (ct *ConditionalTransformerImpl[TSource, TTarget]) Transform(
	ctx context.Context,
	source TSource,
) (TTarget, error) {
	var zero TTarget

	if !ct.CanTransform(ctx, source) {
		return zero, TransformationError{
			Step:       ct.name,
			SourceType: fmt.Sprintf("%T", source),
			TargetType: fmt.Sprintf("%T", zero),
			Cause:      ErrTransformationConditionNotMet,
		}
	}

	return ct.BaseTransformer.Transform(ctx, source)
}

// MemoizedTransformer provides memoization for expensive transformations
func MemoizedTransformer[TSource, TTarget any](
	transformer Transformer[TSource, TTarget],
	keyFunc func(TSource) string,
	ttl time.Duration,
) Transformer[TSource, TTarget] {
	cache := NewTransformationCache[TSource, TTarget](ttl, 1000, keyFunc, nil)

	return NewBaseTransformer(
		"memoized_transformer",
		func(ctx context.Context, source TSource) (TTarget, error) {
			// Check cache first
			if cached, found := cache.Get(source); found {
				return cached, nil
			}

			// Transform and cache result
			result, err := transformer.Transform(ctx, source)
			if err != nil {
				return result, err
			}

			cache.Set(source, result)
			return result, nil
		},
		nil,
	)
}

// NewTransformationMetrics creates a new metrics tracker
func NewTransformationMetrics() *TransformationMetrics {
	return &TransformationMetrics{}
}

// RecordTransformation updates transformation metrics
func (tm *TransformationMetrics) RecordTransformation(duration time.Duration, success bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.transformCount++
	tm.totalDuration += duration
	tm.lastTransformTime = time.Now()

	if !success {
		tm.errorCount++
	}
}

// GetStats returns current transformation statistics
func (tm *TransformationMetrics) GetStats() (count, errors int64, avgDuration time.Duration, errorRate float64) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	count = tm.transformCount
	errors = tm.errorCount

	if count > 0 {
		avgDuration = tm.totalDuration / time.Duration(count)
		errorRate = float64(errors) / float64(count)
	}

	return count, errors, avgDuration, errorRate
}

// Reset clears all metrics
func (tm *TransformationMetrics) Reset() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.transformCount = 0
	tm.errorCount = 0
	tm.totalDuration = 0
	tm.lastTransformTime = time.Time{}
}
