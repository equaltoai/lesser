package storage

import (
	"time"
)

// RelayState represents the updateable state of a relay
type RelayState struct {
	Active     bool   `json:"active"`
	Status     string `json:"status"`     // pending/active/rejected/error
	ErrorCount int    `json:"error_count"`
}

// DeliveryStatus represents the delivery status of an activity to a remote instance
type DeliveryStatus struct {
	ActivityID   string    `json:"activity_id"`
	TargetDomain string    `json:"target_domain"`
	Status       string    `json:"status"`       // pending/delivered/failed
	Attempts     int       `json:"attempts"`     // Number of delivery attempts
	LastAttempt  time.Time `json:"last_attempt"` // Time of last delivery attempt
	Error        string    `json:"error,omitempty"` // Error message if failed
	CreatedAt    time.Time `json:"created_at"`
	DeliveredAt  time.Time `json:"delivered_at,omitempty"`
	NextRetry    time.Time `json:"next_retry,omitempty"`
}

// InstanceStats represents comprehensive statistics for an instance
type InstanceStats struct {
	Domain          string    `json:"domain"`
	Software        string    `json:"software"`
	Version         string    `json:"version"`
	ActiveUsers     int       `json:"active_users"`
	TotalMessages   int64     `json:"total_messages"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
	TrustScore      float64   `json:"trust_score"`
	ErrorRate       float64   `json:"error_rate"`
	AvgResponseTime float64   `json:"avg_response_time"`
	TotalRequests   int64     `json:"total_requests"`
	LastDayStats    *DayStats `json:"last_day_stats,omitempty"`
}

// DayStats represents daily statistics
type DayStats struct {
	Messages     int64   `json:"messages"`
	Errors       int64   `json:"errors"`
	ResponseTime float64 `json:"response_time"`
}

