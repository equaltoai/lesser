package routing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// Query type constants
const (
	QueryTypeInstance = "instance"
	QueryTypeStatus   = "status"
	QueryTypeMetrics  = "metrics"
)

// QueryOptimizer optimizes DynamoDB query patterns for federation routing
type QueryOptimizer struct {
	cacheRepo queryCacheRepository
	logger    *zap.Logger

	// Query result cache (in-memory for batching)
	cache *queryCache

	// Batch query coordinator for optimizing federation requests
	batchCoordinator *BatchQueryCoordinator
}

type queryCacheRepository interface {
	GetInstance(ctx context.Context, instanceID string) (*fedTypes.Instance, error)
	SetInstance(ctx context.Context, instance *fedTypes.Instance, ttl time.Duration) error
	GetInstancesByStatus(ctx context.Context, status fedTypes.InstanceStatus) ([]*fedTypes.Instance, error)
	BatchGetInstances(ctx context.Context, instanceIDs []string) ([]*fedTypes.Instance, error)
	GetMetricsInRange(ctx context.Context, routeID string, start, end time.Time, limit int) ([]*fedTypes.DeliveryResult, error)
	PrewarmActiveInstances(ctx context.Context) error
	InvalidateCachePattern(ctx context.Context, pattern string) error
}

// queryCache implements an LRU cache for query results
type queryCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	lru     *lruList
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	key      string
	value    any
	size     int
	expiry   time.Time
	listNode *lruNode
}

type lruNode struct {
	key  string
	prev *lruNode
	next *lruNode
}

type lruList struct {
	head *lruNode
	tail *lruNode
}

// BatchQueryCoordinator aggregates queries into efficient batches
type BatchQueryCoordinator struct {
	mu        sync.RWMutex
	logger    *zap.Logger
	cacheRepo queryCacheRepository

	// Query queues by type
	instanceQueries map[string]*batchQuery
	statusQueries   map[string]*batchQuery
	metricsQueries  map[string]*batchQuery

	// Configuration
	batchSize     int
	batchWindow   time.Duration
	maxWaitTime   time.Duration
	deduplication bool

	// Background processing
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// batchQuery represents a batched query request
type batchQuery struct {
	mu               sync.RWMutex
	queryType        string
	keys             []string
	deduplicatedKeys map[string]bool
	resultChan       chan batchResult
	responders       []chan batchResult
	createdAt        time.Time
	lastUpdated      time.Time
	priority         int
}

// batchResult holds the result of a batch operation
type batchResult struct {
	results map[string]interface{}
	err     error
}

// QueryRequest represents a query that can be batched
type QueryRequest struct {
	Type     string
	Key      string
	Priority int
	Deadline time.Time
	Context  context.Context
	Callback chan interface{}
}

// NewQueryOptimizer creates a new query optimizer
func NewQueryOptimizer(cacheRepo *repositories.QueryCacheRepository, logger *zap.Logger) *QueryOptimizer {
	var repo queryCacheRepository
	if cacheRepo != nil {
		repo = cacheRepo
	}
	return newQueryOptimizer(repo, logger)
}

func newQueryOptimizer(cacheRepo queryCacheRepository, logger *zap.Logger) *QueryOptimizer {
	qo := &QueryOptimizer{
		cacheRepo: cacheRepo,
		logger:    logger,
		cache: &queryCache{
			entries: make(map[string]*cacheEntry),
			lru:     &lruList{},
			maxSize: 10000, // Cache up to 10k entries
			ttl:     5 * time.Minute,
		},
		batchCoordinator: newBatchQueryCoordinator(cacheRepo, logger),
	}

	// Start the batch coordinator
	qo.batchCoordinator.Start()

	return qo
}

// OptimizedGetInstance retrieves an instance with caching and batching
func (qo *QueryOptimizer) OptimizedGetInstance(ctx context.Context, instanceID string) (*fedTypes.Instance, error) {
	cacheKey := fmt.Sprintf("instance:%s", instanceID)

	// Check in-memory cache first
	if cached := qo.cache.get(cacheKey); cached != nil {
		if instance, ok := cached.(*fedTypes.Instance); ok {
			qo.logger.Debug("in-memory cache hit", zap.String("instanceID", instanceID))
			return instance, nil
		}
	}

	// Try persistent cache (DynamoDB)
	instance, err := qo.cacheRepo.GetInstance(ctx, instanceID)
	if err != nil {
		qo.logger.Warn("Error getting instance from persistent cache",
			zap.String("instanceID", instanceID),
			zap.Error(err))
	}
	if instance != nil {
		// Cache in memory for faster access
		qo.cache.set(cacheKey, instance, 1)
		qo.logger.Debug("persistent cache hit", zap.String("instanceID", instanceID))
		return instance, nil
	}

	// Use batch coordinator for efficient fetching
	result, err := qo.batchCoordinator.AddInstanceQuery(ctx, instanceID, 3) // Normal priority
	if err != nil {
		return nil, errors.Join(ErrBatchQueryFailed, err)
	}

	if result == nil {
		qo.logger.Error("instance not found in batch query", zap.String("instanceID", instanceID))
		return nil, ErrInstanceNotFound
	}

	fetchedInstance, ok := result.(*fedTypes.Instance)
	if !ok {
		qo.logger.Error("invalid result type from batch query",
			zap.String("instanceID", instanceID),
			zap.String("resultType", fmt.Sprintf("%T", result)))
		return nil, ErrInvalidResultType
	}

	// Cache the result in memory and persistent store
	qo.cache.set(cacheKey, fetchedInstance, 1)
	if err := qo.cacheRepo.SetInstance(ctx, fetchedInstance, 5*time.Minute); err != nil {
		qo.logger.Warn("Failed to cache instance persistently",
			zap.String("instanceID", instanceID),
			zap.Error(err))
	}

	return fetchedInstance, nil
}

// OptimizedBatchGetInstances retrieves multiple instances efficiently
func (qo *QueryOptimizer) OptimizedBatchGetInstances(ctx context.Context, instanceIDs []string) ([]*fedTypes.Instance, error) {
	instances := make([]*fedTypes.Instance, 0, len(instanceIDs))
	uncachedIDs := make([]string, 0)

	// Check in-memory cache for each instance
	for _, id := range instanceIDs {
		cacheKey := fmt.Sprintf("instance:%s", id)
		if cached := qo.cache.get(cacheKey); cached != nil {
			if instance, ok := cached.(*fedTypes.Instance); ok {
				instances = append(instances, instance)
				continue
			}
		}
		uncachedIDs = append(uncachedIDs, id)
	}

	if err := common.ValidateSliceNotEmpty("uncachedIDs", uncachedIDs); err != nil {
		return instances, nil // All cached in memory
	}

	// Use batch coordinator for efficient batch retrieval
	var fetchedInstances []*fedTypes.Instance
	for _, id := range uncachedIDs {
		result, err := qo.batchCoordinator.AddInstanceQuery(ctx, id, 2) // High priority for batch requests
		if err != nil {
			qo.logger.Warn("batch query failed for instance",
				zap.String("instanceID", id),
				zap.Error(err))
			continue
		}

		if result != nil {
			if instance, ok := result.(*fedTypes.Instance); ok {
				fetchedInstances = append(fetchedInstances, instance)
			}
		}
	}

	// Cache in memory and add to results
	for _, instance := range fetchedInstances {
		cacheKey := fmt.Sprintf("instance:%s", instance.ID)
		qo.cache.set(cacheKey, instance, 1)
		instances = append(instances, instance)
	}

	return instances, nil
}

// OptimizedQueryByStatus queries instances by status with result caching
func (qo *QueryOptimizer) OptimizedQueryByStatus(ctx context.Context, status fedTypes.InstanceStatus) ([]*fedTypes.Instance, error) {
	cacheKey := fmt.Sprintf("status:%s", status)

	// Check in-memory cache
	if cached := qo.cache.get(cacheKey); cached != nil {
		if instances, ok := cached.([]*fedTypes.Instance); ok {
			qo.logger.Debug("status query cache hit", zap.String("status", string(status)))
			return instances, nil
		}
	}

	// Use batch coordinator for efficient status queries
	result, err := qo.batchCoordinator.AddStatusQuery(ctx, string(status), 2) // High priority
	if err != nil {
		return nil, errors.Join(ErrBatchStatusQueryFailed, err)
	}

	if result == nil {
		return []*fedTypes.Instance{}, nil
	}

	instances, ok := result.([]*fedTypes.Instance)
	if !ok {
		// Fallback to direct repository call
		instances, err = qo.cacheRepo.GetInstancesByStatus(ctx, status)
		if err != nil {
			return nil, errors.Join(ErrFallbackStatusQueryFailed, err)
		}
	}

	// Cache in memory
	qo.cache.set(cacheKey, instances, len(instances))

	return instances, nil
}

// OptimizedQueryRecentMetrics queries recent metrics with intelligent pagination
func (qo *QueryOptimizer) OptimizedQueryRecentMetrics(ctx context.Context, routeID string, limit int) ([]*fedTypes.DeliveryResult, error) {
	// Try batch coordinator first for better performance
	result, err := qo.batchCoordinator.AddMetricsQuery(ctx, routeID, 3) // Normal priority
	if err == nil && result != nil {
		if metrics, ok := result.([]*fedTypes.DeliveryResult); ok {
			if len(metrics) > limit {
				return metrics[:limit], nil
			}
			return metrics, nil
		}
	}

	// Fallback to parallel queries for different time ranges
	now := time.Now()
	timeRanges := []struct {
		start time.Time
		end   time.Time
	}{
		{now.Add(-1 * time.Hour), now},
		{now.Add(-6 * time.Hour), now.Add(-1 * time.Hour)},
		{now.Add(-24 * time.Hour), now.Add(-6 * time.Hour)},
	}

	var wg sync.WaitGroup
	resultsChan := make(chan []*fedTypes.DeliveryResult, len(timeRanges))

	for _, tr := range timeRanges {
		wg.Add(1)
		go func(start, end time.Time) {
			defer wg.Done()

			results, err := qo.cacheRepo.GetMetricsInRange(ctx, routeID, start, end, limit/len(timeRanges))
			if err != nil {
				qo.logger.Warn("failed to query range",
					zap.String("routeID", routeID),
					zap.Time("start", start),
					zap.Time("end", end),
					zap.Error(err))
				return
			}

			resultsChan <- results
		}(tr.start, tr.end)
	}

	// Wait and collect results
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	allResults := make([]*fedTypes.DeliveryResult, 0, limit)
	for results := range resultsChan {
		allResults = append(allResults, results...)
	}

	// Sort by timestamp and limit
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}

	return allResults, nil
}

// PrewarmCache preloads frequently accessed data
func (qo *QueryOptimizer) PrewarmCache(ctx context.Context) error {
	qo.logger.Info("prewarming cache")

	// Prewarm active instances using repository
	err := qo.cacheRepo.PrewarmActiveInstances(ctx)
	if err != nil {
		return errors.Join(ErrPrewarmActiveInstancesFailed, err)
	}

	// Also load into in-memory cache
	activeInstances, err := qo.OptimizedQueryByStatus(ctx, fedTypes.InstanceStatusActive)
	if err != nil {
		return errors.Join(ErrPrewarmActiveInstancesInMemoryFailed, err)
	}

	qo.logger.Info("prewarmed cache",
		zap.Int("activeInstances", len(activeInstances)))

	return nil
}

// InvalidateCache invalidates cache entries matching a pattern
func (qo *QueryOptimizer) InvalidateCache(pattern string) {
	// Invalidate in-memory cache
	qo.cache.invalidatePattern(pattern)

	// Invalidate persistent cache
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := qo.cacheRepo.InvalidateCachePattern(ctx, pattern); err != nil {
		qo.logger.Warn("Failed to invalidate persistent cache",
			zap.String("pattern", pattern),
			zap.Error(err))
	}
}

// Shutdown gracefully shuts down the query optimizer
func (qo *QueryOptimizer) Shutdown() {
	if qo.batchCoordinator != nil {
		qo.batchCoordinator.Stop()
	}
}

// Helper methods

// parseInstance method removed - using repository pattern instead

// queryMetricsInRange method removed - using repository pattern instead

// Cache implementation

func (c *queryCache) get(key string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil
	}

	// Check expiry
	if time.Now().After(entry.expiry) {
		return nil
	}

	// Move to front of LRU
	c.lru.moveToFront(entry.listNode)

	return entry.value
}

func (c *queryCache) set(key string, value any, size int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if exists
	if existing, exists := c.entries[key]; exists {
		existing.value = value
		existing.expiry = time.Now().Add(c.ttl)
		c.lru.moveToFront(existing.listNode)
		return
	}

	// Create new entry
	node := &lruNode{key: key}
	entry := &cacheEntry{
		key:      key,
		value:    value,
		size:     size,
		expiry:   time.Now().Add(c.ttl),
		listNode: node,
	}

	c.entries[key] = entry
	c.lru.pushFront(node)

	// Evict if needed
	for len(c.entries) > c.maxSize {
		oldest := c.lru.tail
		if oldest != nil {
			delete(c.entries, oldest.key)
			c.lru.remove(oldest)
		}
	}
}

func (c *queryCache) invalidatePattern(pattern string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if matched, _ := matchPattern(key, pattern); matched {
			delete(c.entries, key)
			c.lru.remove(entry.listNode)
		}
	}
}

// evictionLoop removed - using passive eviction instead

// evictExpired removed - using passive eviction instead

// LRU list operations

func (l *lruList) pushFront(node *lruNode) {
	node.prev = nil
	node.next = nil
	if l.head == nil {
		l.head = node
		l.tail = node
		return
	}

	node.next = l.head
	l.head.prev = node
	l.head = node
}

func (l *lruList) moveToFront(node *lruNode) {
	if node == l.head {
		return
	}

	l.remove(node)
	l.pushFront(node)
}

func (l *lruList) remove(node *lruNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		l.head = node.next
	}

	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.tail = node.prev
	}

	node.prev = nil
	node.next = nil
}

// addQuery removed - using direct queries instead of background batching

// processBatches removed - using direct queries instead of background batching

// executeBatch removed - using direct queries instead of background batching

// executeBatchGetInstances removed - using direct queries instead of background batching

func matchPattern(str, pattern string) (bool, error) {
	// Simple pattern matching (can be enhanced)
	return str == pattern || (len(pattern) > 0 && pattern[len(pattern)-1] == '*' &&
		len(str) >= len(pattern)-1 && str[:len(pattern)-1] == pattern[:len(pattern)-1]), nil
}

// NewBatchQueryCoordinator creates a new batch query coordinator
func NewBatchQueryCoordinator(cacheRepo *repositories.QueryCacheRepository, logger *zap.Logger) *BatchQueryCoordinator {
	var repo queryCacheRepository
	if cacheRepo != nil {
		repo = cacheRepo
	}
	return newBatchQueryCoordinator(repo, logger)
}

func newBatchQueryCoordinator(cacheRepo queryCacheRepository, logger *zap.Logger) *BatchQueryCoordinator {
	return &BatchQueryCoordinator{
		logger:          logger,
		cacheRepo:       cacheRepo,
		instanceQueries: make(map[string]*batchQuery),
		statusQueries:   make(map[string]*batchQuery),
		metricsQueries:  make(map[string]*batchQuery),
		batchSize:       50,                     // Max items per batch
		batchWindow:     100 * time.Millisecond, // Time window for collecting queries
		maxWaitTime:     500 * time.Millisecond, // Maximum wait before forcing execution
		deduplication:   true,
		stopChan:        make(chan struct{}),
	}
}

// Start begins background batch processing
func (bc *BatchQueryCoordinator) Start() {
	bc.wg.Add(1)
	go bc.processingLoop()
}

// Stop gracefully shuts down the batch coordinator
func (bc *BatchQueryCoordinator) Stop() {
	close(bc.stopChan)
	bc.wg.Wait()

	// Cancel any pending queries
	bc.mu.Lock()
	defer bc.mu.Unlock()

	for _, query := range bc.instanceQueries {
		bc.cancelBatchQuery(query, ErrCoordinatorStopped)
	}
	for _, query := range bc.statusQueries {
		bc.cancelBatchQuery(query, ErrCoordinatorStopped)
	}
	for _, query := range bc.metricsQueries {
		bc.cancelBatchQuery(query, ErrCoordinatorStopped)
	}
}

// AddInstanceQuery adds an instance query to the batch
func (bc *BatchQueryCoordinator) AddInstanceQuery(ctx context.Context, instanceID string, priority int) (interface{}, error) {
	return bc.addQuery(ctx, "instance", instanceID, priority)
}

// AddStatusQuery adds a status query to the batch
func (bc *BatchQueryCoordinator) AddStatusQuery(ctx context.Context, status string, priority int) (interface{}, error) {
	return bc.addQuery(ctx, "status", status, priority)
}

// AddMetricsQuery adds a metrics query to the batch
func (bc *BatchQueryCoordinator) AddMetricsQuery(ctx context.Context, routeID string, priority int) (interface{}, error) {
	return bc.addQuery(ctx, "metrics", routeID, priority)
}

// addQuery is the core method for adding queries to batches
func (bc *BatchQueryCoordinator) addQuery(_ context.Context, queryType, key string, priority int) (interface{}, error) {
	bc.mu.Lock()

	// Check for existing batch or create new one
	batchKey := bc.getBatchKey(queryType, key)
	existingBatch := bc.findExistingBatch(queryType, batchKey)

	if existingBatch != nil {
		// Add to existing batch if deduplication is enabled
		if bc.deduplication && existingBatch.hasKey(key) {
			bc.logger.Debug("query deduplicated",
				zap.String("type", queryType),
				zap.String("key", key))
		} else {
			existingBatch.addKey(key)
		}
		bc.mu.Unlock()
		return bc.waitForBatchResult(existingBatch, key)
	}

	// Create new batch
	batch := &batchQuery{
		queryType:        queryType,
		keys:             []string{key},
		deduplicatedKeys: make(map[string]bool),
		resultChan:       make(chan batchResult, 100),
		responders:       make([]chan batchResult, 0),
		createdAt:        time.Now(),
		lastUpdated:      time.Now(),
		priority:         priority,
	}
	batch.deduplicatedKeys[key] = true

	// Store in appropriate map
	switch queryType {
	case QueryTypeInstance:
		bc.instanceQueries[batchKey] = batch
	case QueryTypeStatus:
		bc.statusQueries[batchKey] = batch
	case QueryTypeMetrics:
		bc.metricsQueries[batchKey] = batch
	default:
		bc.mu.Unlock()
		return nil, ErrUnknownQueryType
	}

	bc.logger.Debug("new batch query created",
		zap.String("type", queryType),
		zap.String("key", key),
		zap.Int("priority", priority))

	bc.mu.Unlock()
	return bc.waitForBatchResult(batch, key)
}

// getBatchKey generates a consistent key for batching similar queries
func (bc *BatchQueryCoordinator) getBatchKey(queryType, key string) string {
	// For simple batching, we can use the query type
	// More sophisticated logic could group by similarity
	switch queryType {
	case QueryTypeInstance:
		return "instances"
	case QueryTypeStatus:
		return key // Group by status value
	case QueryTypeMetrics:
		return "metrics"
	default:
		return queryType
	}
}

// findExistingBatch finds an existing batch that can accommodate the query
func (bc *BatchQueryCoordinator) findExistingBatch(queryType, batchKey string) *batchQuery {
	switch queryType {
	case QueryTypeInstance:
		return bc.instanceQueries[batchKey]
	case QueryTypeStatus:
		return bc.statusQueries[batchKey]
	case QueryTypeMetrics:
		return bc.metricsQueries[batchKey]
	default:
		return nil
	}
}

// waitForBatchResult waits for the batch to complete and returns the specific result
func (bc *BatchQueryCoordinator) waitForBatchResult(batch *batchQuery, key string) (interface{}, error) {
	// Create a response channel for this specific request
	respChan := make(chan batchResult, 1)
	batch.mu.Lock()
	batch.responders = append(batch.responders, respChan)
	batch.mu.Unlock()

	// Wait for result
	select {
	case result := <-respChan:
		if result.err != nil {
			return nil, result.err
		}
		return result.results[key], nil
	case <-time.After(bc.maxWaitTime * 2): // Give extra time for processing
		bc.logger.Error("batch query timeout", zap.String("key", key), zap.Duration("timeout", bc.maxWaitTime*2))
		return nil, ErrBatchQueryTimeout
	}
}

// processingLoop handles batch execution in the background
func (bc *BatchQueryCoordinator) processingLoop() {
	defer bc.wg.Done()

	ticker := time.NewTicker(bc.batchWindow)
	defer ticker.Stop()

	for {
		select {
		case <-bc.stopChan:
			return
		case <-ticker.C:
			bc.processPendingBatches()
		}
	}
}

// processPendingBatches executes ready batches
func (bc *BatchQueryCoordinator) processPendingBatches() {
	bc.mu.Lock()
	readyBatches := bc.getReadyBatches()
	bc.mu.Unlock()

	for _, batch := range readyBatches {
		go bc.executeBatch(batch)
	}
}

// getReadyBatches identifies batches that are ready for execution
func (bc *BatchQueryCoordinator) getReadyBatches() []*batchQuery {
	now := time.Now()
	ready := make([]*batchQuery, 0)

	// Check instance queries
	for key, batch := range bc.instanceQueries {
		if bc.isBatchReady(batch, now) {
			ready = append(ready, batch)
			delete(bc.instanceQueries, key)
		}
	}

	// Check status queries
	for key, batch := range bc.statusQueries {
		if bc.isBatchReady(batch, now) {
			ready = append(ready, batch)
			delete(bc.statusQueries, key)
		}
	}

	// Check metrics queries
	for key, batch := range bc.metricsQueries {
		if bc.isBatchReady(batch, now) {
			ready = append(ready, batch)
			delete(bc.metricsQueries, key)
		}
	}

	// Sort by priority (lower numbers = higher priority)
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].priority < ready[j].priority
	})

	return ready
}

// isBatchReady determines if a batch should be executed
func (bc *BatchQueryCoordinator) isBatchReady(batch *batchQuery, now time.Time) bool {
	batch.mu.RLock()
	defer batch.mu.RUnlock()

	// Execute if batch is full
	if len(batch.keys) >= bc.batchSize {
		return true
	}

	// Execute if batch window has elapsed
	if now.Sub(batch.createdAt) >= bc.batchWindow {
		return true
	}

	// Execute if max wait time exceeded
	if now.Sub(batch.createdAt) >= bc.maxWaitTime {
		return true
	}

	// Execute high priority batches more quickly
	if batch.priority <= 1 && now.Sub(batch.createdAt) >= bc.batchWindow/2 {
		return true
	}

	return false
}

// executeBatch performs the actual batch query
func (bc *BatchQueryCoordinator) executeBatch(batch *batchQuery) {
	batch.mu.RLock()
	queryType := batch.queryType
	keys := make([]string, len(batch.keys))
	copy(keys, batch.keys)
	responders := make([]chan batchResult, len(batch.responders))
	copy(responders, batch.responders)
	batch.mu.RUnlock()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), bc.maxWaitTime)
	defer cancel()

	bc.logger.Debug("executing batch",
		zap.String("type", queryType),
		zap.Int("keyCount", len(keys)),
		zap.Int("responderCount", len(responders)))

	// Execute the appropriate batch operation
	var results map[string]interface{}
	var err error

	switch queryType {
	case QueryTypeInstance:
		results, err = bc.executeBatchGetInstances(ctx, keys)
	case QueryTypeStatus:
		results, err = bc.executeBatchGetByStatus(ctx, keys)
	case QueryTypeMetrics:
		results, err = bc.executeBatchGetMetrics(ctx, keys)
	default:
		bc.logger.Error("unknown query type in batch execution", zap.String("queryType", queryType))
		err = ErrUnknownQueryType
	}

	// Distribute results to all responders
	result := batchResult{
		results: results,
		err:     err,
	}

	for _, responder := range responders {
		select {
		case responder <- result:
		case <-time.After(time.Second):
			bc.logger.Warn("timeout sending batch result to responder")
		}
	}

	bc.logger.Debug("batch execution completed",
		zap.String("type", queryType),
		zap.Duration("duration", time.Since(start)),
		zap.Error(err))
}

// executeBatchGetInstances executes a batch get for instances
func (bc *BatchQueryCoordinator) executeBatchGetInstances(ctx context.Context, instanceIDs []string) (map[string]interface{}, error) {
	instances, err := bc.cacheRepo.BatchGetInstances(ctx, instanceIDs)
	if err != nil {
		return nil, errors.Join(ErrBatchGetInstancesFailed, err)
	}

	results := make(map[string]interface{})
	for _, instance := range instances {
		results[instance.ID] = instance
	}

	return results, nil
}

// executeBatchGetByStatus executes a batch get for instances by status
func (bc *BatchQueryCoordinator) executeBatchGetByStatus(ctx context.Context, statuses []string) (map[string]interface{}, error) {
	results := make(map[string]interface{})

	// Get instances for each unique status
	uniqueStatuses := bc.deduplicateStrings(statuses)
	for _, status := range uniqueStatuses {
		instances, err := bc.cacheRepo.GetInstancesByStatus(ctx, fedTypes.InstanceStatus(status))
		if err != nil {
			bc.logger.Warn("failed to get instances by status",
				zap.String("status", status),
				zap.Error(err))
			continue
		}
		results[status] = instances
	}

	return results, nil
}

// executeBatchGetMetrics executes a batch get for metrics
func (bc *BatchQueryCoordinator) executeBatchGetMetrics(ctx context.Context, routeIDs []string) (map[string]interface{}, error) {
	results := make(map[string]interface{})

	// Get metrics for each route
	uniqueRoutes := bc.deduplicateStrings(routeIDs)
	for _, routeID := range uniqueRoutes {
		metrics, err := bc.cacheRepo.GetMetricsInRange(ctx, routeID, time.Now().Add(-24*time.Hour), time.Now(), 100)
		if err != nil {
			bc.logger.Warn("failed to get metrics",
				zap.String("routeID", routeID),
				zap.Error(err))
			continue
		}
		results[routeID] = metrics
	}

	return results, nil
}

// Helper methods for batchQuery

func (bq *batchQuery) hasKey(key string) bool {
	bq.mu.RLock()
	defer bq.mu.RUnlock()
	return bq.deduplicatedKeys[key]
}

func (bq *batchQuery) addKey(key string) {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	if !bq.deduplicatedKeys[key] {
		bq.keys = append(bq.keys, key)
		bq.deduplicatedKeys[key] = true
		bq.lastUpdated = time.Now()
	}
}

// cancelBatchQuery cancels a batch query with an error
func (bc *BatchQueryCoordinator) cancelBatchQuery(batch *batchQuery, err error) {
	batch.mu.RLock()
	responders := make([]chan batchResult, len(batch.responders))
	copy(responders, batch.responders)
	batch.mu.RUnlock()

	result := batchResult{err: err}
	for _, responder := range responders {
		select {
		case responder <- result:
		default:
			// Channel full or closed, skip
		}
	}
}

// deduplicateStrings removes duplicate strings from a slice
func (bc *BatchQueryCoordinator) deduplicateStrings(slice []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(slice))

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// GetBatchCoordinator returns the batch coordinator (for testing)
func (qo *QueryOptimizer) GetBatchCoordinator() *BatchQueryCoordinator {
	return qo.batchCoordinator
}

// GetBatchStats returns statistics about batch operations
func (bc *BatchQueryCoordinator) GetBatchStats() map[string]interface{} {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	return map[string]interface{}{
		"pending_instance_batches": len(bc.instanceQueries),
		"pending_status_batches":   len(bc.statusQueries),
		"pending_metrics_batches":  len(bc.metricsQueries),
		"batch_size":               bc.batchSize,
		"batch_window_ms":          bc.batchWindow.Milliseconds(),
		"max_wait_time_ms":         bc.maxWaitTime.Milliseconds(),
		"deduplication_enabled":    bc.deduplication,
	}
}
