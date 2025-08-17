package batch

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// CostMetrics represents the cost metrics at a point in time
type CostMetrics struct {
	DynamoDBReads  int64
	DynamoDBWrites int64
}

// CostTracker interface defines the methods needed for cost tracking
type CostTracker interface {
	// CalculateCost returns the current cost metrics
	CalculateCost() CostMetrics
	// TrackDynamoWrite tracks DynamoDB write operations
	TrackDynamoWrite(items int)
	// TrackDynamoRead tracks DynamoDB read operations
	TrackDynamoRead(items int)
}

// DynamoDB batch operation limits
const (
	MaxBatchWriteSize = 25  // DynamoDB limit for batch write operations
	MaxBatchReadSize  = 100 // DynamoDB limit for batch get operations
	DefaultBatchSize  = 25  // Default batch size for write operations
	DefaultWorkers    = 5   // Default number of worker goroutines
)

// BatchWriter provides efficient batch write operations with configurable batch sizes
//
//nolint:revive // Batch prefix clarifies this is batch-specific writer
type BatchWriter struct {
	client    core.DB
	batchSize int
	logger    *zap.Logger
	tracker   CostTracker
}

// BatchWriterConfig holds configuration for BatchWriter
//
//nolint:revive // Batch prefix clarifies this is batch-specific config
type BatchWriterConfig struct {
	BatchSize int
	Logger    *zap.Logger
	Tracker   CostTracker
}

// NewBatchWriter creates a new BatchWriter with the specified configuration
func NewBatchWriter(client core.DB, config BatchWriterConfig) *BatchWriter {
	batchSize := config.BatchSize
	if batchSize <= 0 || batchSize > MaxBatchWriteSize {
		batchSize = DefaultBatchSize
	}

	return &BatchWriter{
		client:    client,
		batchSize: batchSize,
		logger:    config.Logger,
		tracker:   config.Tracker,
	}
}

// NewDefaultBatchWriter creates a BatchWriter with default settings
func NewDefaultBatchWriter(client core.DB) *BatchWriter {
	return NewBatchWriter(client, BatchWriterConfig{
		BatchSize: DefaultBatchSize,
	})
}

// BatchWriteResult contains the results of a batch write operation
//
//nolint:revive // Batch prefix clarifies this is batch-specific result
type BatchWriteResult struct {
	TotalItems     int
	ProcessedItems int
	FailedItems    int
	Errors         []BatchError
	Duration       time.Duration
	ConsumedWCU    int64
}

// BatchError represents an error that occurred during batch processing
//
//nolint:revive // Batch prefix clarifies this is batch-specific error
type BatchError struct {
	Index int
	Item  any
	Error error
}

// WriteItems writes items in batches, processing them sequentially
func (bw *BatchWriter) WriteItems(ctx context.Context, items []any) (*BatchWriteResult, error) {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return &BatchWriteResult{}, nil
	}

	startTime := time.Now()
	result := &BatchWriteResult{
		TotalItems: len(items),
		Errors:     make([]BatchError, 0),
	}

	// Process items in batches
	for i := 0; i < len(items); i += bw.batchSize {
		end := i + bw.batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		if err := bw.writeBatch(ctx, batch, i, result); err != nil {
			// Continue processing other batches even if one fails
			bw.logError("batch write failed", err, zap.Int("batch_start", i), zap.Int("batch_size", len(batch)))
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			return result, ctx.Err()
		default:
		}
	}

	result.Duration = time.Since(startTime)
	result.FailedItems = len(result.Errors)
	result.ProcessedItems = result.TotalItems - result.FailedItems

	if bw.logger != nil {
		bw.logger.Info("batch_write_completed",
			zap.Int("total_items", result.TotalItems),
			zap.Int("processed_items", result.ProcessedItems),
			zap.Int("failed_items", result.FailedItems),
			zap.Duration("duration", result.Duration),
			zap.Int64("consumed_wcu", result.ConsumedWCU),
		)
	}

	return result, nil
}

// WriteItemsParallel writes items in parallel using worker pools
func (bw *BatchWriter) WriteItemsParallel(ctx context.Context, items []any, workers int) (*BatchWriteResult, error) {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return &BatchWriteResult{}, nil
	}

	if workers <= 0 {
		workers = DefaultWorkers
	}

	startTime := time.Now()
	result := &BatchWriteResult{
		TotalItems: len(items),
		Errors:     make([]BatchError, 0),
	}

	// Create channels for work distribution
	workChan := make(chan batchWork, workers)
	resultChan := make(chan batchResult, workers)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go bw.worker(ctx, workChan, resultChan, &wg)
	}

	// Send work to workers
	go func() {
		defer close(workChan)
		for i := 0; i < len(items); i += bw.batchSize {
			end := i + bw.batchSize
			if end > len(items) {
				end = len(items)
			}

			select {
			case workChan <- batchWork{
				items:      items[i:end],
				startIndex: i,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	var mu sync.Mutex
	var totalWCU int64

	for batchRes := range resultChan {
		mu.Lock()
		result.Errors = append(result.Errors, batchRes.errors...)
		atomic.AddInt64(&totalWCU, batchRes.consumedWCU)
		mu.Unlock()
	}

	result.ConsumedWCU = totalWCU
	result.Duration = time.Since(startTime)
	result.FailedItems = len(result.Errors)
	result.ProcessedItems = result.TotalItems - result.FailedItems

	if bw.logger != nil {
		bw.logger.Info("parallel_batch_write_completed",
			zap.Int("total_items", result.TotalItems),
			zap.Int("processed_items", result.ProcessedItems),
			zap.Int("failed_items", result.FailedItems),
			zap.Int("workers", workers),
			zap.Duration("duration", result.Duration),
			zap.Int64("consumed_wcu", result.ConsumedWCU),
		)
	}

	return result, nil
}

// batchWork represents work to be done by a worker
type batchWork struct {
	items      []any
	startIndex int
}

// batchResult represents the result of processing a batch
type batchResult struct {
	errors      []BatchError
	consumedWCU int64
}

// worker processes batches from the work channel
func (bw *BatchWriter) worker(ctx context.Context, workChan <-chan batchWork, resultChan chan<- batchResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for work := range workChan {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result := batchResult{
			errors: make([]BatchError, 0),
		}

		// Create a temporary result for this batch
		tempResult := &BatchWriteResult{
			Errors: make([]BatchError, 0),
		}

		if err := bw.writeBatch(ctx, work.items, work.startIndex, tempResult); err != nil {
			bw.logError("worker batch write failed", err,
				zap.Int("start_index", work.startIndex),
				zap.Int("batch_size", len(work.items)))
		}

		result.errors = tempResult.Errors
		result.consumedWCU = tempResult.ConsumedWCU

		select {
		case resultChan <- result:
		case <-ctx.Done():
			return
		}
	}
}

// writeBatch writes a single batch of items
func (bw *BatchWriter) writeBatch(_ context.Context, items []any, startIndex int, result *BatchWriteResult) error {
	if err := common.ValidateSliceNotEmpty("items", items); err != nil {
		return nil
	}

	// Track cost if tracker is available
	var consumedWCU int64
	if bw.tracker != nil {
		initialCost := bw.tracker.CalculateCost()
		defer func() {
			finalCost := bw.tracker.CalculateCost()
			consumedWCU = finalCost.DynamoDBWrites - initialCost.DynamoDBWrites
			atomic.AddInt64(&result.ConsumedWCU, consumedWCU)
		}()
	}

	// Use DynamORM's batch write functionality
	query := bw.client.Model(items[0]) // Use first item to determine model type
	err := query.BatchCreate(items)
	if err != nil {
		// If batch write fails, add all items as failed
		for i, item := range items {
			result.Errors = append(result.Errors, BatchError{
				Index: startIndex + i,
				Item:  item,
				Error: err,
			})
		}
		return fmt.Errorf("batch write failed: %w", err)
	}

	// Track successful writes
	if bw.tracker != nil {
		bw.tracker.TrackDynamoWrite(len(items))
	}

	return nil
}

// BatchReader provides efficient batch read operations
//
//nolint:revive // Batch prefix clarifies this is batch-specific reader
type BatchReader struct {
	client    core.DB
	batchSize int
	logger    *zap.Logger
	tracker   CostTracker
}

// BatchReaderConfig holds configuration for BatchReader
//
//nolint:revive // Batch prefix clarifies this is batch-specific config
type BatchReaderConfig struct {
	BatchSize int
	Logger    *zap.Logger
	Tracker   CostTracker
}

// NewBatchReader creates a new BatchReader with the specified configuration
func NewBatchReader(client core.DB, config BatchReaderConfig) *BatchReader {
	batchSize := config.BatchSize
	if batchSize <= 0 || batchSize > MaxBatchReadSize {
		batchSize = MaxBatchReadSize
	}

	return &BatchReader{
		client:    client,
		batchSize: batchSize,
		logger:    config.Logger,
		tracker:   config.Tracker,
	}
}

// NewDefaultBatchReader creates a BatchReader with default settings
func NewDefaultBatchReader(client core.DB) *BatchReader {
	return NewBatchReader(client, BatchReaderConfig{
		BatchSize: MaxBatchReadSize,
	})
}

// BatchReadResult contains the results of a batch read operation
//
//nolint:revive // Batch prefix clarifies this is batch-specific result
type BatchReadResult struct {
	TotalKeys      int
	RetrievedItems int
	NotFoundItems  int
	Errors         []BatchError
	Duration       time.Duration
	ConsumedRCU    int64
}

// ReadItems reads items in batches using their keys
func (br *BatchReader) ReadItems(ctx context.Context, keys []any, dest any) (*BatchReadResult, error) {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return &BatchReadResult{}, nil
	}

	startTime := time.Now()
	result := &BatchReadResult{
		TotalKeys: len(keys),
		Errors:    make([]BatchError, 0),
	}

	// Ensure dest is a pointer to a slice
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return nil, fmt.Errorf("dest must be a pointer to a slice")
	}

	destSlice := destValue.Elem()
	elementType := destSlice.Type().Elem()

	// Process keys in batches
	for i := 0; i < len(keys); i += br.batchSize {
		end := i + br.batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]

		// Create a temporary slice for this batch
		tempSlice := reflect.MakeSlice(reflect.SliceOf(elementType), 0, len(batch))
		tempDest := reflect.New(tempSlice.Type()).Interface()

		if err := br.readBatch(ctx, batch, tempDest, i, result); err != nil {
			br.logError("batch read failed", err, zap.Int("batch_start", i), zap.Int("batch_size", len(batch)))
			continue
		}

		// Append results to destination slice
		tempValue := reflect.ValueOf(tempDest).Elem()
		for j := 0; j < tempValue.Len(); j++ {
			destSlice = reflect.Append(destSlice, tempValue.Index(j))
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			return result, ctx.Err()
		default:
		}
	}

	// Update the destination slice
	destValue.Elem().Set(destSlice)

	result.Duration = time.Since(startTime)
	result.RetrievedItems = destSlice.Len()
	result.NotFoundItems = result.TotalKeys - result.RetrievedItems - len(result.Errors)

	if br.logger != nil {
		br.logger.Info("batch_read_completed",
			zap.Int("total_keys", result.TotalKeys),
			zap.Int("retrieved_items", result.RetrievedItems),
			zap.Int("not_found_items", result.NotFoundItems),
			zap.Int("errors", len(result.Errors)),
			zap.Duration("duration", result.Duration),
			zap.Int64("consumed_rcu", result.ConsumedRCU),
		)
	}

	return result, nil
}

// readBatch reads a single batch of items
func (br *BatchReader) readBatch(_ context.Context, keys []any, dest any, startIndex int, result *BatchReadResult) error {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return nil
	}

	// Track cost if tracker is available
	var consumedRCU int64
	if br.tracker != nil {
		initialCost := br.tracker.CalculateCost()
		defer func() {
			finalCost := br.tracker.CalculateCost()
			consumedRCU = finalCost.DynamoDBReads - initialCost.DynamoDBReads
			atomic.AddInt64(&result.ConsumedRCU, consumedRCU)
		}()
	}

	// Use DynamORM's batch get functionality
	query := br.client.Model(keys[0]) // Use first key to determine model type
	err := query.BatchGet(keys, dest)
	if err != nil {
		// If batch read fails, add all keys as failed
		for i, key := range keys {
			result.Errors = append(result.Errors, BatchError{
				Index: startIndex + i,
				Item:  key,
				Error: err,
			})
		}
		return fmt.Errorf("batch read failed: %w", err)
	}

	// Track successful reads
	if br.tracker != nil {
		br.tracker.TrackDynamoRead(len(keys))
	}

	return nil
}

// ProgressTracker tracks the progress of batch operations
type ProgressTracker struct {
	total     int64
	processed int64
	failed    int64
	mu        sync.RWMutex
	callbacks []ProgressCallback
}

// ProgressCallback is called when progress is updated
type ProgressCallback func(total, processed, failed int64)

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(total int64) *ProgressTracker {
	return &ProgressTracker{
		total:     total,
		callbacks: make([]ProgressCallback, 0),
	}
}

// AddCallback adds a progress callback
func (pt *ProgressTracker) AddCallback(callback ProgressCallback) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.callbacks = append(pt.callbacks, callback)
}

// UpdateProcessed updates the number of processed items
func (pt *ProgressTracker) UpdateProcessed(count int64) {
	pt.mu.Lock()
	pt.processed += count
	total, processed, failed := pt.total, pt.processed, pt.failed
	callbacks := make([]ProgressCallback, len(pt.callbacks))
	copy(callbacks, pt.callbacks)
	pt.mu.Unlock()

	// Call callbacks outside of lock
	for _, callback := range callbacks {
		callback(total, processed, failed)
	}
}

// UpdateFailed updates the number of failed items
func (pt *ProgressTracker) UpdateFailed(count int64) {
	pt.mu.Lock()
	pt.failed += count
	total, processed, failed := pt.total, pt.processed, pt.failed
	callbacks := make([]ProgressCallback, len(pt.callbacks))
	copy(callbacks, pt.callbacks)
	pt.mu.Unlock()

	// Call callbacks outside of lock
	for _, callback := range callbacks {
		callback(total, processed, failed)
	}
}

// GetProgress returns the current progress
func (pt *ProgressTracker) GetProgress() (total, processed, failed int64) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.total, pt.processed, pt.failed
}

// IsComplete returns true if all items have been processed
func (pt *ProgressTracker) IsComplete() bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.processed+pt.failed >= pt.total
}

// GetPercentComplete returns the percentage of completion
func (pt *ProgressTracker) GetPercentComplete() float64 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	if pt.total == 0 {
		return 100.0
	}
	return float64(pt.processed+pt.failed) / float64(pt.total) * 100.0
}

// BatchWriterWithProgress wraps BatchWriter with progress tracking
//
//nolint:revive // Batch prefix clarifies this is batch-specific writer with progress
type BatchWriterWithProgress struct {
	*BatchWriter
	tracker *ProgressTracker
}

// NewBatchWriterWithProgress creates a BatchWriter with progress tracking
func NewBatchWriterWithProgress(client core.DB, config BatchWriterConfig, totalItems int64) *BatchWriterWithProgress {
	return &BatchWriterWithProgress{
		BatchWriter: NewBatchWriter(client, config),
		tracker:     NewProgressTracker(totalItems),
	}
}

// GetProgressTracker returns the progress tracker
func (bwp *BatchWriterWithProgress) GetProgressTracker() *ProgressTracker {
	return bwp.tracker
}

// WriteItemsWithProgress writes items with progress tracking
func (bwp *BatchWriterWithProgress) WriteItemsWithProgress(ctx context.Context, items []any) (*BatchWriteResult, error) {
	result, err := bwp.WriteItems(ctx, items)
	if result != nil {
		bwp.tracker.UpdateProcessed(int64(result.ProcessedItems))
		bwp.tracker.UpdateFailed(int64(result.FailedItems))
	}
	return result, err
}

// WriteItemsParallelWithProgress writes items in parallel with progress tracking
func (bwp *BatchWriterWithProgress) WriteItemsParallelWithProgress(ctx context.Context, items []any, workers int) (*BatchWriteResult, error) {
	result, err := bwp.WriteItemsParallel(ctx, items, workers)
	if result != nil {
		bwp.tracker.UpdateProcessed(int64(result.ProcessedItems))
		bwp.tracker.UpdateFailed(int64(result.FailedItems))
	}
	return result, err
}

// Helper functions

// logError logs an error if logger is available
func (bw *BatchWriter) logError(message string, err error, fields ...zap.Field) {
	if bw.logger != nil {
		allFields := append(fields, zap.Error(err))
		bw.logger.Error(message, allFields...)
	}
}

// logError logs an error if logger is available
func (br *BatchReader) logError(message string, err error, fields ...zap.Field) {
	if br.logger != nil {
		allFields := append(fields, zap.Error(err))
		br.logger.Error(message, allFields...)
	}
}

// BatchDeleter provides efficient batch delete operations
//
//nolint:revive // Batch prefix clarifies this is batch-specific deleter
type BatchDeleter struct {
	client    core.DB
	batchSize int
	logger    *zap.Logger
	tracker   CostTracker
}

// BatchDeleterConfig holds configuration for BatchDeleter
//
//nolint:revive // Batch prefix clarifies this is batch-specific config
type BatchDeleterConfig struct {
	BatchSize int
	Logger    *zap.Logger
	Tracker   CostTracker
}

// NewBatchDeleter creates a new BatchDeleter with the specified configuration
func NewBatchDeleter(client core.DB, config BatchDeleterConfig) *BatchDeleter {
	batchSize := config.BatchSize
	if batchSize <= 0 || batchSize > MaxBatchWriteSize {
		batchSize = DefaultBatchSize
	}

	return &BatchDeleter{
		client:    client,
		batchSize: batchSize,
		logger:    config.Logger,
		tracker:   config.Tracker,
	}
}

// NewDefaultBatchDeleter creates a BatchDeleter with default settings
func NewDefaultBatchDeleter(client core.DB) *BatchDeleter {
	return NewBatchDeleter(client, BatchDeleterConfig{
		BatchSize: DefaultBatchSize,
	})
}

// BatchDeleteResult contains the results of a batch delete operation
//
//nolint:revive // Batch prefix clarifies this is batch-specific result
type BatchDeleteResult struct {
	TotalItems     int
	ProcessedItems int
	FailedItems    int
	Errors         []BatchError
	Duration       time.Duration
	ConsumedWCU    int64
}

// DeleteItems deletes items in batches, processing them sequentially
func (bd *BatchDeleter) DeleteItems(ctx context.Context, keys []any) (*BatchDeleteResult, error) {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return &BatchDeleteResult{}, nil
	}

	startTime := time.Now()
	result := &BatchDeleteResult{
		TotalItems: len(keys),
		Errors:     make([]BatchError, 0),
	}

	// Process keys in batches
	for i := 0; i < len(keys); i += bd.batchSize {
		end := i + bd.batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]
		if err := bd.deleteBatch(ctx, batch, i, result); err != nil {
			// Continue processing other batches even if one fails
			bd.logError("batch delete failed", err, zap.Int("batch_start", i), zap.Int("batch_size", len(batch)))
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			return result, ctx.Err()
		default:
		}
	}

	result.Duration = time.Since(startTime)
	result.FailedItems = len(result.Errors)
	result.ProcessedItems = result.TotalItems - result.FailedItems

	if bd.logger != nil {
		bd.logger.Info("batch_delete_completed",
			zap.Int("total_items", result.TotalItems),
			zap.Int("processed_items", result.ProcessedItems),
			zap.Int("failed_items", result.FailedItems),
			zap.Duration("duration", result.Duration),
			zap.Int64("consumed_wcu", result.ConsumedWCU),
		)
	}

	return result, nil
}

// DeleteItemsParallel deletes items in parallel using worker pools
func (bd *BatchDeleter) DeleteItemsParallel(ctx context.Context, keys []any, workers int) (*BatchDeleteResult, error) {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return &BatchDeleteResult{}, nil
	}

	if workers <= 0 {
		workers = DefaultWorkers
	}

	startTime := time.Now()
	result := &BatchDeleteResult{
		TotalItems: len(keys),
		Errors:     make([]BatchError, 0),
	}

	// Create channels for work distribution
	workChan := make(chan batchWork, workers)
	resultChan := make(chan batchResult, workers)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go bd.deleteWorker(ctx, workChan, resultChan, &wg)
	}

	// Send work to workers
	go func() {
		defer close(workChan)
		for i := 0; i < len(keys); i += bd.batchSize {
			end := i + bd.batchSize
			if end > len(keys) {
				end = len(keys)
			}

			select {
			case workChan <- batchWork{
				items:      keys[i:end],
				startIndex: i,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	var mu sync.Mutex
	var totalWCU int64

	for batchRes := range resultChan {
		mu.Lock()
		result.Errors = append(result.Errors, batchRes.errors...)
		atomic.AddInt64(&totalWCU, batchRes.consumedWCU)
		mu.Unlock()
	}

	result.ConsumedWCU = totalWCU
	result.Duration = time.Since(startTime)
	result.FailedItems = len(result.Errors)
	result.ProcessedItems = result.TotalItems - result.FailedItems

	if bd.logger != nil {
		bd.logger.Info("parallel_batch_delete_completed",
			zap.Int("total_items", result.TotalItems),
			zap.Int("processed_items", result.ProcessedItems),
			zap.Int("failed_items", result.FailedItems),
			zap.Int("workers", workers),
			zap.Duration("duration", result.Duration),
			zap.Int64("consumed_wcu", result.ConsumedWCU),
		)
	}

	return result, nil
}

// DeleteItemsWithRetry deletes items with exponential backoff retry logic
func (bd *BatchDeleter) DeleteItemsWithRetry(ctx context.Context, keys []any, maxRetries int) (*BatchDeleteResult, error) {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return &BatchDeleteResult{}, nil
	}

	retryState := bd.initializeRetryState(keys)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if shouldDelay := bd.handleRetryDelay(ctx, attempt, len(retryState.remainingKeys)); shouldDelay != nil {
			return retryState.lastResult, shouldDelay
		}

		result, err := bd.DeleteItems(ctx, retryState.remainingKeys)
		retryState.lastResult = result

		if bd.isOperationSuccessful(result, err) {
			bd.logSuccessAfterRetry(attempt, len(keys))
			return result, nil
		}

		retryableKeys := bd.extractRetryableKeys(result)
		if err := common.ValidateSliceNotEmpty("retryable_keys", retryableKeys); err != nil {
			break
		}

		retryState.remainingKeys = retryableKeys
		retryState.lastErr = err
	}

	bd.logFinalFailure(maxRetries, len(keys), retryState.lastResult, retryState.lastErr)
	return retryState.lastResult, fmt.Errorf("batch delete failed after %d attempts: %w", maxRetries+1, retryState.lastErr)
}

// retryState holds the state during retry operations
type retryState struct {
	remainingKeys []any
	lastResult    *BatchDeleteResult
	lastErr       error
}

// initializeRetryState creates initial retry state
func (bd *BatchDeleter) initializeRetryState(keys []any) *retryState {
	return &retryState{
		remainingKeys: keys,
	}
}

// handleRetryDelay manages the exponential backoff delay logic
func (bd *BatchDeleter) handleRetryDelay(ctx context.Context, attempt int, remainingCount int) error {
	if attempt == 0 {
		return nil
	}

	delay := bd.calculateBackoffDelay(attempt)
	bd.logRetryAttempt(attempt, remainingCount, delay)

	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// calculateBackoffDelay computes exponential backoff delay with bounds checking
func (bd *BatchDeleter) calculateBackoffDelay(attempt int) time.Duration {
	backoff := 100 * time.Millisecond
	shift := bd.safeBoundShift(attempt - 1)
	return backoff * time.Duration(1<<min(shift, 30))
}

// safeBoundShift ensures shift value is within safe bounds
func (bd *BatchDeleter) safeBoundShift(shift int) int {
	if shift > 63 {
		return 63
	}
	if shift < 0 {
		return 0
	}
	return shift
}

// isOperationSuccessful checks if the delete operation completed successfully
func (bd *BatchDeleter) isOperationSuccessful(result *BatchDeleteResult, err error) bool {
	return err == nil && result.FailedItems == 0
}

// extractRetryableKeys identifies which keys can be retried
func (bd *BatchDeleter) extractRetryableKeys(result *BatchDeleteResult) []any {
	retryableErrors := make([]any, 0)
	for _, batchErr := range result.Errors {
		if bd.isRetryableError(batchErr.Error) {
			retryableErrors = append(retryableErrors, batchErr.Item)
		}
	}
	return retryableErrors
}

// logRetryAttempt logs information about retry attempts
func (bd *BatchDeleter) logRetryAttempt(attempt int, remainingCount int, delay time.Duration) {
	if bd.logger != nil {
		bd.logger.Info("retrying_batch_delete",
			zap.Int("attempt", attempt),
			zap.Int("remaining_keys", remainingCount),
			zap.Duration("delay", delay),
		)
	}
}

// logSuccessAfterRetry logs successful completion after retries
func (bd *BatchDeleter) logSuccessAfterRetry(attempt int, totalItems int) {
	if bd.logger != nil && attempt > 0 {
		bd.logger.Info("batch_delete_succeeded_after_retry",
			zap.Int("attempts", attempt+1),
			zap.Int("total_items", totalItems),
		)
	}
}

// logFinalFailure logs the final failure after all retries exhausted
func (bd *BatchDeleter) logFinalFailure(maxRetries int, totalItems int, lastResult *BatchDeleteResult, lastErr error) {
	if bd.logger != nil {
		bd.logger.Error("batch_delete_failed_after_retries",
			zap.Int("attempts", maxRetries+1),
			zap.Int("total_items", totalItems),
			zap.Int("failed_items", lastResult.FailedItems),
			zap.Error(lastErr),
		)
	}
}

// deleteWorker processes delete batches from the work channel
func (bd *BatchDeleter) deleteWorker(ctx context.Context, workChan <-chan batchWork, resultChan chan<- batchResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for work := range workChan {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result := batchResult{
			errors: make([]BatchError, 0),
		}

		// Create a temporary result for this batch
		tempResult := &BatchDeleteResult{
			Errors: make([]BatchError, 0),
		}

		if err := bd.deleteBatch(ctx, work.items, work.startIndex, tempResult); err != nil {
			bd.logError("worker batch delete failed", err,
				zap.Int("start_index", work.startIndex),
				zap.Int("batch_size", len(work.items)))
		}

		result.errors = tempResult.Errors
		result.consumedWCU = tempResult.ConsumedWCU

		select {
		case resultChan <- result:
		case <-ctx.Done():
			return
		}
	}
}

// deleteBatch deletes a single batch of items
func (bd *BatchDeleter) deleteBatch(_ context.Context, keys []any, startIndex int, result *BatchDeleteResult) error {
	if err := common.ValidateSliceNotEmpty("keys", keys); err != nil {
		return nil
	}

	// Track cost if tracker is available
	var consumedWCU int64
	if bd.tracker != nil {
		initialCost := bd.tracker.CalculateCost()
		defer func() {
			finalCost := bd.tracker.CalculateCost()
			consumedWCU = finalCost.DynamoDBWrites - initialCost.DynamoDBWrites
			atomic.AddInt64(&result.ConsumedWCU, consumedWCU)
		}()
	}

	// Use DynamORM's batch delete functionality
	query := bd.client.Model(keys[0]) // Use first key to determine model type
	err := query.BatchDelete(keys)
	if err != nil {
		// If batch delete fails, add all keys as failed
		for i, key := range keys {
			result.Errors = append(result.Errors, BatchError{
				Index: startIndex + i,
				Item:  key,
				Error: err,
			})
		}
		return fmt.Errorf("batch delete failed: %w", err)
	}

	// Track successful deletes
	if bd.tracker != nil {
		bd.tracker.TrackDynamoWrite(len(keys))
	}

	return nil
}

// isRetryableError determines if an error is retryable for delete operations
func (bd *BatchDeleter) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())
	retryablePatterns := []string{
		"provisioned throughput exceeded",
		"throttling",
		"temporary error",
		"timeout",
		"connection",
		"internal server error",
		"service unavailable",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}

	return false
}

// logError logs an error if logger is available
func (bd *BatchDeleter) logError(message string, err error, fields ...zap.Field) {
	if bd.logger != nil {
		allFields := append(fields, zap.Error(err))
		bd.logger.Error(message, allFields...)
	}
}

// Convenience functions for easy usage

// BatchWrite is a convenience function for simple batch writes
//
//nolint:revive // Batch prefix clarifies this is batch-specific operation
func BatchWrite(ctx context.Context, client core.DB, items []any) (*BatchWriteResult, error) {
	writer := NewDefaultBatchWriter(client)
	return writer.WriteItems(ctx, items)
}

// BatchWriteParallel is a convenience function for parallel batch writes
//
//nolint:revive // Batch prefix clarifies this is batch-specific parallel operation
func BatchWriteParallel(ctx context.Context, client core.DB, items []any, workers int) (*BatchWriteResult, error) {
	writer := NewDefaultBatchWriter(client)
	return writer.WriteItemsParallel(ctx, items, workers)
}

// BatchDelete is a convenience function for simple batch deletes
//
//nolint:revive // Batch prefix clarifies this is batch-specific operation
func BatchDelete(ctx context.Context, client core.DB, keys []any) (*BatchDeleteResult, error) {
	deleter := NewDefaultBatchDeleter(client)
	return deleter.DeleteItems(ctx, keys)
}

// BatchDeleteParallel is a convenience function for parallel batch deletes
//
//nolint:revive // Batch prefix clarifies this is batch-specific parallel operation
func BatchDeleteParallel(ctx context.Context, client core.DB, keys []any, workers int) (*BatchDeleteResult, error) {
	deleter := NewDefaultBatchDeleter(client)
	return deleter.DeleteItemsParallel(ctx, keys, workers)
}

// BatchDeleteWithRetry is a convenience function for batch deletes with retry logic
//
//nolint:revive // Batch prefix clarifies this is batch-specific operation with retry
func BatchDeleteWithRetry(ctx context.Context, client core.DB, keys []any, maxRetries int) (*BatchDeleteResult, error) {
	deleter := NewDefaultBatchDeleter(client)
	return deleter.DeleteItemsWithRetry(ctx, keys, maxRetries)
}

// BatchRead is a convenience function for simple batch reads
//
//nolint:revive // Batch prefix clarifies this is batch-specific operation
func BatchRead(ctx context.Context, client core.DB, keys []any, dest any) (*BatchReadResult, error) {
	reader := NewDefaultBatchReader(client)
	return reader.ReadItems(ctx, keys, dest)
}

// BatchWriteWithCostTracking performs batch write with cost tracking
//
//nolint:revive // Batch prefix clarifies this is batch-specific operation with tracking
func BatchWriteWithCostTracking(ctx context.Context, client core.DB, items []any, tracker CostTracker, logger *zap.Logger) (*BatchWriteResult, error) {
	writer := NewBatchWriter(client, BatchWriterConfig{
		BatchSize: DefaultBatchSize,
		Logger:    logger,
		Tracker:   tracker,
	})
	return writer.WriteItems(ctx, items)
}

// BatchDeleteWithCostTracking performs batch delete with cost tracking
//
//nolint:revive // Batch prefix clarifies this is batch-specific operation with tracking
func BatchDeleteWithCostTracking(ctx context.Context, client core.DB, keys []any, tracker CostTracker, logger *zap.Logger) (*BatchDeleteResult, error) {
	deleter := NewBatchDeleter(client, BatchDeleterConfig{
		BatchSize: DefaultBatchSize,
		Logger:    logger,
		Tracker:   tracker,
	})
	return deleter.DeleteItems(ctx, keys)
}

// BatchReadWithCostTracking performs batch read with cost tracking
//
//nolint:revive // Batch prefix clarifies this is batch-specific operation with tracking
func BatchReadWithCostTracking(ctx context.Context, client core.DB, keys []any, dest any, tracker CostTracker, logger *zap.Logger) (*BatchReadResult, error) {
	reader := NewBatchReader(client, BatchReaderConfig{
		BatchSize: MaxBatchReadSize,
		Logger:    logger,
		Tracker:   tracker,
	})
	return reader.ReadItems(ctx, keys, dest)
}
