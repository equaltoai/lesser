package models

import (
	"fmt"
	"time"
)

// QueryCacheEntry represents a cached query result in DynamoDB
type QueryCacheEntry struct {
	// Primary keys - using pattern: PK=CACHE#{cacheKey}, SK=ENTRY
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// Cache data
	CacheKey  string    `json:"cache_key"`
	Value     string    `json:"value"`      // JSON-encoded cached value
	Size      int       `json:"size"`       // Size for LRU calculations
	ExpiresAt time.Time `json:"expires_at"` // Manual expiry tracking
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl" json:"ttl"`
}

// UpdateKeys ensures keys are properly set before saving
func (q *QueryCacheEntry) UpdateKeys() error {
	q.PK = fmt.Sprintf("CACHE#%s", q.CacheKey)
	q.SK = SKEntry

	// Set TTL based on ExpiresAt
	q.TTL = q.ExpiresAt.Unix()

	q.UpdatedAt = time.Now()
	if q.CreatedAt.IsZero() {
		q.CreatedAt = time.Now()
	}
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (q *QueryCacheEntry) GetPK() string {
	return q.PK
}

// GetSK returns the sort key for BaseModel interface
func (q *QueryCacheEntry) GetSK() string {
	return q.SK
}

// IsExpired checks if the cache entry has expired
func (q *QueryCacheEntry) IsExpired() bool {
	return time.Now().After(q.ExpiresAt)
}

// TableName returns the DynamoDB table backing QueryCacheEntry.
func (QueryCacheEntry) TableName() string {
	return MainTableName
}

// BatchGetKeys represents batch get keys for instances
type BatchGetKeys struct {
	// Primary keys - using pattern: PK=BATCH#{batchType}, SK=KEY#{key}
	PK string `dynamorm:"pk" json:"pk"`
	SK string `dynamorm:"sk" json:"sk"`

	// Batch data
	BatchType string    `json:"batch_type"` // "instance", "metrics", etc.
	Key       string    `json:"key"`        // The actual key being batched
	CreatedAt time.Time `json:"created_at"`

	// TTL for cleanup (short-lived for batching)
	TTL int64 `dynamorm:"ttl" json:"ttl"`
}

// UpdateKeys ensures keys are properly set for batch keys
func (b *BatchGetKeys) UpdateKeys() error {
	b.PK = fmt.Sprintf("BATCH#%s", b.BatchType)
	b.SK = fmt.Sprintf("KEY#%s", b.Key)

	// Set short TTL (1 minute for batching)
	b.TTL = time.Now().Add(1 * time.Minute).Unix()

	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (b *BatchGetKeys) GetPK() string {
	return b.PK
}

// GetSK returns the sort key for BaseModel interface
func (b *BatchGetKeys) GetSK() string {
	return b.SK
}

// TableName returns the DynamoDB table backing BatchGetKeys.
func (BatchGetKeys) TableName() string {
	return MainTableName
}
