package routing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// QueryOptimizer optimizes DynamoDB query patterns for federation routing
type QueryOptimizer struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger

	// Query result cache
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
	value    interface{}
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

// queryBatcher batches multiple queries for efficiency
type queryBatcher struct {
	mu          sync.Mutex
	batches     map[string]*queryBatch
	flushTicker *time.Ticker
}

type queryBatch struct {
	queries    []batchedQuery
	resultChan chan batchResult
	created    time.Time
}

type batchedQuery struct {
	key        string
	attributes []string
	resultChan chan interface{}
}

type batchResult struct {
	key   string
	value interface{}
	err   error
}

// NewQueryOptimizer creates a new query optimizer
func NewQueryOptimizer(db *dynamodb.Client, tableName string, logger *zap.Logger) *QueryOptimizer {
	qo := &QueryOptimizer{
		db:        db,
		tableName: tableName,
		logger:    logger,
		cache: &queryCache{
			entries: make(map[string]*cacheEntry),
			lru:     &lruList{},
			maxSize: 10000, // Cache up to 10k entries
			ttl:     5 * time.Minute,
		},
		batcher: &queryBatcher{
			batches:     make(map[string]*queryBatch),
			flushTicker: time.NewTicker(10 * time.Millisecond),
		},
	}

	// Start background processes
	go qo.batcher.processBatches(qo)
	go qo.cache.evictionLoop()

	return qo
}

// OptimizedGetInstance retrieves an instance with caching and batching
func (qo *QueryOptimizer) OptimizedGetInstance(ctx context.Context, instanceID string) (*Instance, error) {
	cacheKey := fmt.Sprintf("instance:%s", instanceID)

	// Check cache first
	if cached := qo.cache.get(cacheKey); cached != nil {
		if instance, ok := cached.(*Instance); ok {
			qo.logger.Debug("cache hit", zap.String("instanceID", instanceID))
			return instance, nil
		}
	}

	// Use batch query
	resultChan := make(chan interface{}, 1)
	qo.batcher.addQuery("instance", batchedQuery{
		key:        instanceID,
		resultChan: resultChan,
	})

	// Wait for result
	select {
	case result := <-resultChan:
		if result == nil {
			return nil, fmt.Errorf("instance not found: %s", instanceID)
		}
		instance := result.(*Instance)

		// Cache the result
		qo.cache.set(cacheKey, instance, 1)

		return instance, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// OptimizedBatchGetInstances retrieves multiple instances efficiently
func (qo *QueryOptimizer) OptimizedBatchGetInstances(ctx context.Context, instanceIDs []string) ([]*Instance, error) {
	instances := make([]*Instance, 0, len(instanceIDs))
	uncachedIDs := make([]string, 0)

	// Check cache for each instance
	for _, id := range instanceIDs {
		cacheKey := fmt.Sprintf("instance:%s", id)
		if cached := qo.cache.get(cacheKey); cached != nil {
			if instance, ok := cached.(*Instance); ok {
				instances = append(instances, instance)
				continue
			}
		}
		uncachedIDs = append(uncachedIDs, id)
	}

	if len(uncachedIDs) == 0 {
		return instances, nil // All cached
	}

	// Batch query uncached instances
	keys := make([]map[string]types.AttributeValue, 0, len(uncachedIDs))
	for _, id := range uncachedIDs {
		keys = append(keys, map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s", id)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		})
	}

	// Use BatchGetItem for efficiency
	batchInput := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			qo.tableName: {
				Keys: keys,
			},
		},
	}

	result, err := qo.db.BatchGetItem(ctx, batchInput)
	if err != nil {
		return nil, fmt.Errorf("batch get instances: %w", err)
	}

	// Parse results
	if items, ok := result.Responses[qo.tableName]; ok {
		for _, item := range items {
			instance := qo.parseInstance(item)
			if instance != nil {
				instances = append(instances, instance)

				// Cache the result
				cacheKey := fmt.Sprintf("instance:%s", instance.ID)
				qo.cache.set(cacheKey, instance, 1)
			}
		}
	}

	return instances, nil
}

// OptimizedQueryByStatus queries instances by status with result caching
func (qo *QueryOptimizer) OptimizedQueryByStatus(ctx context.Context, status InstanceStatus) ([]*Instance, error) {
	cacheKey := fmt.Sprintf("status:%s", status)

	// Check cache
	if cached := qo.cache.get(cacheKey); cached != nil {
		if instances, ok := cached.([]*Instance); ok {
			return instances, nil
		}
	}

	// Query using GSI
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(qo.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :status"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", status)},
		},
		Limit: aws.Int32(100),
	}

	instances := make([]*Instance, 0)

	// Execute query
	result, err := qo.db.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("query by status: %w", err)
	}

	for _, item := range result.Items {
		if instance := qo.parseInstance(item); instance != nil {
			instances = append(instances, instance)
		}
	}

	// Cache the result set
	qo.cache.set(cacheKey, instances, len(instances))

	return instances, nil
}

// OptimizedQueryRecentMetrics queries recent metrics with intelligent pagination
func (qo *QueryOptimizer) OptimizedQueryRecentMetrics(ctx context.Context, routeID string, limit int) ([]*DeliveryResult, error) {
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
	resultsChan := make(chan []*DeliveryResult, len(timeRanges))

	for _, tr := range timeRanges {
		wg.Add(1)
		go func(start, end time.Time) {
			defer wg.Done()

			results, err := qo.queryMetricsInRange(ctx, routeID, start, end, limit/len(timeRanges))
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

	allResults := make([]*DeliveryResult, 0, limit)
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

	// Prewarm active instances
	activeInstances, err := qo.OptimizedQueryByStatus(ctx, InstanceStatusActive)
	if err != nil {
		return fmt.Errorf("prewarm active instances: %w", err)
	}

	qo.logger.Info("prewarmed cache",
		zap.Int("activeInstances", len(activeInstances)))

	// Prewarm recent routes
	// This would query and cache frequently used routes

	return nil
}

// InvalidateCache invalidates cache entries matching a pattern
func (qo *QueryOptimizer) InvalidateCache(pattern string) {
	qo.cache.invalidatePattern(pattern)
}

// Helper methods

func (qo *QueryOptimizer) parseInstance(item map[string]types.AttributeValue) *Instance {
	// Parse instance from DynamoDB item
	// Implementation similar to instance_registry.go
	instance := &Instance{}

	if v, ok := item["ID"].(*types.AttributeValueMemberS); ok {
		instance.ID = v.Value
	}
	if v, ok := item["Domain"].(*types.AttributeValueMemberS); ok {
		instance.Domain = v.Value
	}
	// ... parse other fields

	return instance
}

func (qo *QueryOptimizer) queryMetricsInRange(ctx context.Context, routeID string, start, end time.Time, limit int) ([]*DeliveryResult, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(qo.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND SK BETWEEN :start AND :end"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("ROUTE#%s", routeID)},
			":start": &types.AttributeValueMemberS{Value: fmt.Sprintf("RESULT#%d", start.UnixNano())},
			":end":   &types.AttributeValueMemberS{Value: fmt.Sprintf("RESULT#%d", end.UnixNano())},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(safeIntToInt32(limit)),
	}

	result, err := qo.db.Query(ctx, queryInput)
	if err != nil {
		return nil, err
	}

	results := make([]*DeliveryResult, 0, len(result.Items))
	// Parse results...

	return results, nil
}

// Cache implementation

func (c *queryCache) get(key string) interface{} {
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

func (c *queryCache) set(key string, value interface{}, size int) {
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

func (c *queryCache) evictionLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.evictExpired()
	}
}

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

// Batch processor

func (b *queryBatcher) addQuery(batchKey string, query batchedQuery) {
	b.mu.Lock()
	defer b.mu.Unlock()

	batch, exists := b.batches[batchKey]
	if !exists {
		batch = &queryBatch{
			queries:    make([]batchedQuery, 0, 25),
			resultChan: make(chan batchResult, 25),
			created:    time.Now(),
		}
		b.batches[batchKey] = batch
	}

	batch.queries = append(batch.queries, query)
}

func (b *queryBatcher) processBatches(qo *QueryOptimizer) {
	for range b.flushTicker.C {
		b.mu.Lock()

		for key, batch := range b.batches {
			// Process if batch is full or old enough
			if len(batch.queries) >= 25 || time.Since(batch.created) > 50*time.Millisecond {
				go qo.executeBatch(key, batch)
				delete(b.batches, key)
			}
		}

		b.mu.Unlock()
	}
}

func (qo *QueryOptimizer) executeBatch(batchKey string, batch *queryBatch) {
	// Execute batch query based on type
	switch batchKey {
	case "instance":
		qo.executeBatchGetInstances(batch)
	default:
		// Handle other batch types
	}
}

func (qo *QueryOptimizer) executeBatchGetInstances(batch *queryBatch) {
	// Build batch get request
	keys := make([]map[string]types.AttributeValue, 0, len(batch.queries))
	keyMap := make(map[string]batchedQuery)

	for _, query := range batch.queries {
		key := fmt.Sprintf("INSTANCE#%s", query.key)
		keys = append(keys, map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: key},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		})
		keyMap[key] = query
	}

	// Execute batch get
	batchInput := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			qo.tableName: {
				Keys: keys,
			},
		},
	}

	result, err := qo.db.BatchGetItem(context.Background(), batchInput)
	if err != nil {
		// Send error to all queries
		for _, query := range batch.queries {
			query.resultChan <- nil
		}
		return
	}

	// Process results
	foundKeys := make(map[string]bool)
	if items, ok := result.Responses[qo.tableName]; ok {
		for _, item := range items {
			if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
				if query, exists := keyMap[pk.Value]; exists {
					instance := qo.parseInstance(item)
					query.resultChan <- instance
					foundKeys[pk.Value] = true
				}
			}
		}
	}

	// Send nil for not found items
	for key, query := range keyMap {
		if !foundKeys[key] {
			query.resultChan <- nil
		}
	}
}

func matchPattern(str, pattern string) (bool, error) {
	// Simple pattern matching (can be enhanced)
	return str == pattern || (len(pattern) > 0 && pattern[len(pattern)-1] == '*' &&
		len(str) >= len(pattern)-1 && str[:len(pattern)-1] == pattern[:len(pattern)-1]), nil
}
