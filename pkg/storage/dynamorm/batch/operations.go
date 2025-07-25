package batch

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aron23/lesser/pkg/cost"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// DynamoDB batch operation limits
const (
	MaxBatchWriteSize = 25  // DynamoDB limit for batch write operations
	MaxBatchReadSize  = 100 // DynamoDB limit for batch get operations
	DefaultBatchSize  = 25  // Default batch size for write operations
	DefaultWorkers    = 5   // Default number of worker goroutines
)

// BatchWriter provides efficient batch write operations with configurable batch sizes
type BatchWriter struct {
	client    core.DB
	batchSize int
	logger    *zap.Logger
	tracker   *cost.Tracker
}

// BatchWriterConfig holds configuration for BatchWriter
type BatchWriterConfig struct {
	BatchSize int
	Logger    *zap.Logger
	Tracker   *cost.Tracker
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
type BatchWriteResult struct {
	TotalItems     int
	ProcessedItems int
	FailedItems    int
	Errors         []BatchError
	Duration       time.Duration
	ConsumedWCU    int64
}

// BatchError represents an error that occurred during batch processing
type BatchError struct {
	Index int
	Item  any
	Error error
}

// WriteItems writes items in batches, processing them sequentially
func (bw *BatchWriter) WriteItems(ctx context.Context, items []any) (*BatchWriteResult, error) {
	if len(items) == 0 {
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
	if len(items) == 0 {
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
func (bw *BatchWriter) writeBatch(ctx context.Context, items []any, startIndex int, result *BatchWriteResult) error {
	if len(items) == 0 {
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
type BatchReader struct {
	client    core.DB
	batchSize int
	logger    *zap.Logger
	tracker   *cost.Tracker
}

// BatchReaderConfig holds configuration for BatchReader
type BatchReaderConfig struct {
	BatchSize int
	Logger    *zap.Logger
	Tracker   *cost.Tracker
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
	if len(keys) == 0 {
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
func (br *BatchReader) readBatch(ctx context.Context, keys []any, dest any, startIndex int, result *BatchReadResult) error {
	if len(keys) == 0 {
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

// Convenience functions for easy usage

// BatchWrite is a convenience function for simple batch writes
func BatchWrite(ctx context.Context, client core.DB, items []any) (*BatchWriteResult, error) {
	writer := NewDefaultBatchWriter(client)
	return writer.WriteItems(ctx, items)
}

// BatchWriteParallel is a convenience function for parallel batch writes
func BatchWriteParallel(ctx context.Context, client core.DB, items []any, workers int) (*BatchWriteResult, error) {
	writer := NewDefaultBatchWriter(client)
	return writer.WriteItemsParallel(ctx, items, workers)
}

// BatchRead is a convenience function for simple batch reads
func BatchRead(ctx context.Context, client core.DB, keys []any, dest any) (*BatchReadResult, error) {
	reader := NewDefaultBatchReader(client)
	return reader.ReadItems(ctx, keys, dest)
}

// BatchWriteWithCostTracking performs batch write with cost tracking
func BatchWriteWithCostTracking(ctx context.Context, client core.DB, items []any, tracker *cost.Tracker, logger *zap.Logger) (*BatchWriteResult, error) {
	writer := NewBatchWriter(client, BatchWriterConfig{
		BatchSize: DefaultBatchSize,
		Logger:    logger,
		Tracker:   tracker,
	})
	return writer.WriteItems(ctx, items)
}

// BatchReadWithCostTracking performs batch read with cost tracking
func BatchReadWithCostTracking(ctx context.Context, client core.DB, keys []any, dest any, tracker *cost.Tracker, logger *zap.Logger) (*BatchReadResult, error) {
	reader := NewBatchReader(client, BatchReaderConfig{
		BatchSize: MaxBatchReadSize,
		Logger:    logger,
		Tracker:   tracker,
	})
	return reader.ReadItems(ctx, keys, dest)
}
