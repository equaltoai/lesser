package routing

import (
	"context"
	"fmt"
	"sync"
	"time"

	fedTypes "github.com/equaltoai/lesser/pkg/federation/types"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// QueryOptimizer optimizes DynamoDB query patterns for federation routing
type QueryOptimizer struct {
	cacheRepo *repositories.QueryCacheRepository
	logger    *zap.Logger

	// Query result cache (in-memory for batching)
	cache *queryCache

	// Batch query coordinator
	batcher *queryBatcher
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

// queryBatcher batches multiple queries for efficiency (serverless-compatible)
type queryBatcher struct {
	mu      sync.Mutex
	batches map[string]*queryBatch
}

type queryBatch struct {
	queries []batchedQuery
	created time.Time
}

type batchedQuery struct {
	key        string
	resultChan chan any
}

// NewQueryOptimizer creates a new query optimizer
func NewQueryOptimizer(cacheRepo *repositories.QueryCacheRepository, logger *zap.Logger) *QueryOptimizer {
	qo := &QueryOptimizer{
		cacheRepo: cacheRepo,
		logger:    logger,
		cache: &queryCache{
			entries: make(map[string]*cacheEntry),
			lru:     &lruList{},
			maxSize: 10000, // Cache up to 10k entries
			ttl:     5 * time.Minute,
		},
		batcher: &queryBatcher{
			batches: make(map[string]*queryBatch),
		},
	}

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

	// Direct fetch on cache miss (no background batching)
	instances, err := qo.cacheRepo.BatchGetInstances(ctx, []string{instanceID})
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	fetchedInstance := instances[0]

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

	if len(uncachedIDs) == 0 {
		return instances, nil // All cached in memory
	}

	// Use repository for batch get (handles persistent cache and database)
	freshInstances, err := qo.cacheRepo.BatchGetInstances(ctx, uncachedIDs)
	if err != nil {
		return nil, fmt.Errorf("batch get instances: %w", err)
	}

	// Cache in memory and add to results
	for _, instance := range freshInstances {
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
			return instances, nil
		}
	}

	// Use repository (handles persistent cache and database query)
	instances, err := qo.cacheRepo.GetInstancesByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("query by status: %w", err)
	}

	// Cache in memory
	qo.cache.set(cacheKey, instances, len(instances))

	return instances, nil
}

// OptimizedQueryRecentMetrics queries recent metrics with intelligent pagination
func (qo *QueryOptimizer) OptimizedQueryRecentMetrics(ctx context.Context, routeID string, limit int) ([]*fedTypes.DeliveryResult, error) {
	// Use parallel queries for different time ranges
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
		return fmt.Errorf("prewarm active instances: %w", err)
	}

	// Also load into in-memory cache
	activeInstances, err := qo.OptimizedQueryByStatus(ctx, fedTypes.InstanceStatusActive)
	if err != nil {
		return fmt.Errorf("prewarm active instances in memory: %w", err)
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

func (c *queryCache) evictExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiry) {
			delete(c.entries, key)
			c.lru.remove(entry.listNode)
		}
	}
}

// LRU list operations

func (l *lruList) pushFront(node *lruNode) {
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
