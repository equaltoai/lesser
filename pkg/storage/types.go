package storage

import (
	"time"
)

// TrustCategory represents the category of trust
type TrustCategory string

const (
	TrustCategoryContent   TrustCategory = "content"   // Trust in content moderation
	TrustCategoryBehavior  TrustCategory = "behavior"  // Trust in behavior assessment
	TrustCategoryTechnical TrustCategory = "technical" // Trust in technical decisions
	TrustCategoryGeneral   TrustCategory = "general"   // General trust
)

// TrustEvidence represents evidence supporting a trust relationship
type TrustEvidence struct {
	Type        string    `json:"type"`  // consensus_agreement, direct_interaction, etc.
	Score       float64   `json:"score"` // Impact on trust score
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}

// TrustRelationship represents a trust relationship between two actors
type TrustRelationship struct {
	ID         string          `json:"id"`
	TrusterID  string          `json:"truster_id"` // Who trusts
	TrusteeID  string          `json:"trustee_id"` // Who is trusted
	Category   TrustCategory   `json:"category"`
	Score      float64         `json:"score"`      // -1.0 to 1.0 (-1 = distrust, 0 = neutral, 1 = full trust)
	Confidence float64         `json:"confidence"` // 0.0 to 1.0 (how confident in this score)
	Evidence   []TrustEvidence `json:"evidence,omitempty"`
	Created    time.Time       `json:"created"`
	Updated    time.Time       `json:"updated"`
	TTL        int64           `json:"ttl,omitempty"` // Unix timestamp for expiration
}

// TrustScore represents a calculated trust score for an actor
type TrustScore struct {
	ActorID         string             `json:"actor_id"`
	Category        TrustCategory      `json:"category"`
	Score           float64            `json:"score"`            // Aggregated score
	DirectScore     float64            `json:"direct_score"`     // Score from direct relationships
	PropagatedScore float64            `json:"propagated_score"` // Score from network propagation
	Confidence      float64            `json:"confidence"`       // Overall confidence
	TrusterCount    int                `json:"truster_count"`    // Number of actors who trust this one
	LastCalculated  time.Time          `json:"last_calculated"`
	CacheTTL        time.Time          `json:"cache_ttl"` // When to recalculate
	CategoryScores  map[string]float64 `json:"category_scores,omitempty"`
}

// TrustUpdate represents an update to trust based on moderation outcomes
type TrustUpdate struct {
	ActorID   string        `json:"actor_id"`
	Category  TrustCategory `json:"category"`
	Delta     float64       `json:"delta"`    // Change in trust score
	Reason    string        `json:"reason"`   // Why the update occurred
	EventID   string        `json:"event_id"` // Related moderation event
	Timestamp time.Time     `json:"timestamp"`
}

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

