package models

import (
	"fmt"
	"time"
)

// SlidingWindowEntry represents a single request in the sliding window
type SlidingWindowEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	PK           string    `dynamorm:"pk" json:"-"`
	SK           string    `dynamorm:"sk" json:"-"`
	RateLimitKey string    `json:"rate_limit_key"`
	RequestID    string    `json:"request_id"`
	Weight       int       `json:"weight,omitempty"`
	ExpiresAt    int64     `dynamorm:"ttl" json:"-"`
}

// Key structure for efficient queries
func (s *SlidingWindowEntry) Key(rateLimitKey string, timestamp time.Time) {
	s.PK = fmt.Sprintf("RATELIMIT#%s", rateLimitKey)
	s.SK = fmt.Sprintf("TS#%d", timestamp.UnixNano())
}

// GetTableName returns the DynamoDB table name for this model
func (s *SlidingWindowEntry) GetTableName() string {
	return "rate_limit_sliding_window"
}
