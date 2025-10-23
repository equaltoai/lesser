package models

import (
	"fmt"
	"time"
)

// SearchCache represents cached search results
type SearchCache struct {
	// Keys
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// Fields
	Query     string                 `json:"query"`   // original query
	Results   map[string]interface{} `json:"results"` // cached search results
	CreatedAt time.Time              `json:"created_at"`
	TTL       int64                  `json:"ttl,omitempty" dynamorm:"ttl"` // Unix timestamp
}

// TableName returns the DynamoDB table backing SearchCache.
func (SearchCache) TableName() string {
	return MainTableName
}

// NewSearchCache creates a new search cache entry
func NewSearchCache(query string) *SearchCache {
	now := time.Now()
	cache := &SearchCache{
		Query:     query,
		Results:   make(map[string]interface{}),
		CreatedAt: now,
		TTL:       now.Add(24 * time.Hour).Unix(), // Cache for 24 hours
	}
	cache.UpdateKeys()
	return cache
}

// UpdateKeys updates the partition and sort keys based on the model's attributes
func (s *SearchCache) UpdateKeys() {
	s.PK = fmt.Sprintf("SEARCH_CACHE#%s", s.Query)
	s.SK = "RESULTS"
}

// InvalidateCache marks the cache as invalid by adding invalidation reason
func (s *SearchCache) InvalidateCache(reason string) {
	s.Results["invalidated"] = true
	s.Results["invalidation_reason"] = reason
	s.Results["invalidated_at"] = time.Now().Unix()
}

// IsValid checks if the cache entry is still valid
func (s *SearchCache) IsValid() bool {
	if invalidated, ok := s.Results["invalidated"].(bool); ok && invalidated {
		return false
	}
	return time.Now().Unix() < s.TTL
}
