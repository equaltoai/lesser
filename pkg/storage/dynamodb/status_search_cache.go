package dynamodb

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// StatusSearchCache provides caching for status search results
type StatusSearchCache struct {
	dynamo    DynamoDBAPI
	tableName string

	// In-memory cache for hot queries
	memCache map[string]*statusCacheEntry
	mu       sync.RWMutex

	// TTL for cache entries
	ttl time.Duration
}

type statusCacheEntry struct {
	Results   []*StatusSearchResult
	Timestamp time.Time
}

// NewStatusSearchCache creates a new status search cache
func NewStatusSearchCache(dynamo DynamoDBAPI, tableName string) *StatusSearchCache {
	cache := &StatusSearchCache{
		dynamo:    dynamo,
		tableName: tableName,
		memCache:  make(map[string]*statusCacheEntry),
		ttl:       5 * time.Minute, // 5 minute TTL for status searches
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves cached search results
func (c *StatusSearchCache) Get(ctx context.Context, key string) ([]*StatusSearchResult, bool) {
	// Check memory cache first
	c.mu.RLock()
	entry, found := c.memCache[key]
	c.mu.RUnlock()

	if found && time.Since(entry.Timestamp) < c.ttl {
		return entry.Results, true
	}

	// Check DynamoDB cache for less frequent queries
	cacheKey := fmt.Sprintf("CACHE#STATUS_SEARCH#%s", key)

	result, err := c.dynamo.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(c.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: cacheKey},
			"SK": &types.AttributeValueMemberS{Value: "RESULTS"},
		},
	})

	if err != nil || result.Item == nil {
		return nil, false
	}

	// Check TTL
	if ttlAttr, ok := result.Item["TTL"]; ok {
		if ttlNum, ok := ttlAttr.(*types.AttributeValueMemberN); ok {
			ttlTimestamp := parseFloat64(ttlNum.Value)
			ttlTime := time.Unix(int64(ttlTimestamp), 0)
			if time.Now().After(ttlTime) {
				return nil, false
			}
		}
	}

	// Unmarshal results
	if dataAttr, ok := result.Item["Data"]; ok {
		if dataStr, ok := dataAttr.(*types.AttributeValueMemberS); ok {
			var results []*StatusSearchResult
			if err := json.Unmarshal([]byte(dataStr.Value), &results); err == nil {
				// Update memory cache
				c.mu.Lock()
				c.memCache[key] = &statusCacheEntry{
					Results:   results,
					Timestamp: time.Now(),
				}
				c.mu.Unlock()

				return results, true
			}
		}
	}

	return nil, false
}

// Set caches search results
func (c *StatusSearchCache) Set(ctx context.Context, key string, results []*StatusSearchResult) {
	// Update memory cache
	c.mu.Lock()
	c.memCache[key] = &statusCacheEntry{
		Results:   results,
		Timestamp: time.Now(),
	}
	c.mu.Unlock()

	// For popular queries, also cache in DynamoDB
	if len(results) > 0 {
		go c.persistToDynamoDB(context.Background(), key, results)
	}
}

// persistToDynamoDB saves cache entry to DynamoDB for persistence
func (c *StatusSearchCache) persistToDynamoDB(ctx context.Context, key string, results []*StatusSearchResult) {
	// Serialize results
	data, err := json.Marshal(results)
	if err != nil {
		return
	}

	// Calculate TTL (5 minutes from now)
	ttl := time.Now().Add(c.ttl).Unix()

	cacheKey := fmt.Sprintf("CACHE#STATUS_SEARCH#%s", key)

	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: cacheKey},
		"SK":        &types.AttributeValueMemberS{Value: "RESULTS"},
		"Data":      &types.AttributeValueMemberS{Value: string(data)},
		"TTL":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", ttl)},
		"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		"Type":      &types.AttributeValueMemberS{Value: "StatusSearchCache"},
	}

	_, _ = c.dynamo.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(c.tableName),
		Item:      item,
	})
}

// cleanupLoop periodically removes expired entries from memory cache
func (c *StatusSearchCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.memCache {
			if now.Sub(entry.Timestamp) > c.ttl {
				delete(c.memCache, key)
			}
		}
		c.mu.Unlock()
	}
}

// Clear removes all cached entries
func (c *StatusSearchCache) Clear() {
	c.mu.Lock()
	c.memCache = make(map[string]*statusCacheEntry)
	c.mu.Unlock()
}

// parseFloat64 safely parses a string to float64
func parseFloat64(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
