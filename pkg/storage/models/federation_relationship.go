package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// RelationshipState represents the lifecycle state of a federation relationship
type RelationshipState string

// Relationship states for federation lifecycle tracking
const (
	// StateActive indicates recent activity
	StateActive   RelationshipState = "ACTIVE"
	// StateIdle indicates no recent activity (7+ days)
	StateIdle     RelationshipState = "IDLE"
	// StateDormant indicates long inactive period (30+ days)
	StateDormant  RelationshipState = "DORMANT"
	// StateArchived indicates moved to cold storage (90+ days)
	StateArchived RelationshipState = "ARCHIVED"
	// StateExpired indicates marked for deletion (365+ days)
	StateExpired  RelationshipState = "EXPIRED"
)

// FederationRelationship represents a relationship between users or instances with lifecycle tracking
type FederationRelationship struct {
	PK                string            `dynamorm:"pk"`
	SK                string            `dynamorm:"sk"`
	GSI1PK            string            `dynamorm:"index:gsi1,pk"`   // State-based queries
	GSI1SK            string            `dynamorm:"index:gsi1,sk"`   // Last activity timestamp
	GSI2PK            string            `dynamorm:"index:gsi2,pk"`   // User-based queries
	GSI2SK            string            `dynamorm:"index:gsi2,sk"`   // Target instance + timestamp
	TTL               int64             `json:"ttl,omitempty" dynamorm:"ttl"`
	
	// Core relationship data
	ID                string            `json:"id"`
	UserID            string            `json:"user_id"`              // Local user ID
	TargetInstance    string            `json:"target_instance"`      // Remote instance domain
	TargetUserID      string            `json:"target_user_id,omitempty"` // Remote user ID (if user-level)
	RelationshipType  string            `json:"relationship_type"`    // follow, mention, boost, reply, etc.
	
	// Lifecycle management
	State             RelationshipState `json:"state"`
	LastActivity      time.Time         `json:"last_activity"`
	FirstSeen         time.Time         `json:"first_seen"`
	StateChangedAt    time.Time         `json:"state_changed_at"`
	
	// Success rate tracking (15-minute rolling window)
	SuccessCount15m   int64             `json:"success_count_15m"`
	FailureCount15m   int64             `json:"failure_count_15m"`
	WindowStart15m    time.Time         `json:"window_start_15m"`
	SuccessRate       float64           `json:"success_rate"`
	
	// Aggregated metrics
	TotalSuccesses    int64             `json:"total_successes"`
	TotalFailures     int64             `json:"total_failures"`
	TotalAttempts     int64             `json:"total_attempts"`
	AvgResponseTime   float64           `json:"avg_response_time"`
	
	// Reactivation handling
	WarmupUntil       *time.Time        `json:"warmup_until,omitempty"`
	CurrentRate       float64           `json:"current_rate"`           // Traffic rate during warmup (0.0-1.0)
	HistoricalBaseline float64          `json:"historical_baseline"`    // Pre-dormancy success rate
	
	// Storage optimization
	ArchiveLocation   string            `json:"archive_location,omitempty"` // S3 key if archived
	CompressedMetrics string            `json:"compressed_metrics,omitempty"` // Gzip compressed historical data
	
	// Metadata
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// UpdateKeys sets the DynamoDB keys for the federation relationship
func (fr *FederationRelationship) UpdateKeys() {
	// Primary keys: User-specific relationships
	fr.PK = fmt.Sprintf("USER#%s#FEDERATION", fr.UserID)
	fr.SK = fmt.Sprintf("REL#%s#%s#%s", fr.TargetInstance, fr.RelationshipType, fr.ID)
	
	// GSI1: State-based queries for lifecycle management
	fr.GSI1PK = fmt.Sprintf("FEDERATION_STATE#%s", fr.State)
	fr.GSI1SK = fmt.Sprintf("%d#%s#%s", fr.LastActivity.Unix(), fr.TargetInstance, fr.ID)
	
	// GSI2: User + target instance queries
	fr.GSI2PK = fmt.Sprintf("USER#%s#TARGET#%s", fr.UserID, fr.TargetInstance)
	fr.GSI2SK = fmt.Sprintf("%s#%d", fr.RelationshipType, fr.LastActivity.Unix())
	
	// Set TTL based on state
	switch fr.State {
	case StateActive:
		// No TTL for active relationships
		fr.TTL = 0
	case StateIdle:
		// TTL after 90 days of idle
		fr.TTL = time.Now().Add(90 * 24 * time.Hour).Unix()
	case StateDormant:
		// TTL after 365 days of dormant
		fr.TTL = time.Now().Add(365 * 24 * time.Hour).Unix()
	case StateArchived:
		// TTL after 2 years in archive
		fr.TTL = time.Now().Add(2 * 365 * 24 * time.Hour).Unix()
	case StateExpired:
		// TTL immediately (will be cleaned up)
		fr.TTL = time.Now().Add(24 * time.Hour).Unix()
	}
}

// UpdateSuccessRate recalculates the 15-minute rolling window success rate
func (fr *FederationRelationship) UpdateSuccessRate(success bool, responseTime float64) {
	now := time.Now()
	
	// Check if we need to reset the 15-minute window
	if now.Sub(fr.WindowStart15m) >= 15*time.Minute {
		fr.WindowStart15m = now.Truncate(15 * time.Minute)
		fr.SuccessCount15m = 0
		fr.FailureCount15m = 0
	}
	
	// Update 15-minute window counts
	if success {
		fr.SuccessCount15m++
		fr.TotalSuccesses++
	} else {
		fr.FailureCount15m++
		fr.TotalFailures++
	}
	
	fr.TotalAttempts++
	fr.LastActivity = now
	
	// Calculate success rate
	totalWindow := fr.SuccessCount15m + fr.FailureCount15m
	if totalWindow > 0 {
		fr.SuccessRate = float64(fr.SuccessCount15m) / float64(totalWindow)
	} else {
		fr.SuccessRate = 0.0
	}
	
	// Update average response time (simple moving average)
	if responseTime > 0 {
		if fr.AvgResponseTime == 0 {
			fr.AvgResponseTime = responseTime
		} else {
			// Weighted average: 90% historical, 10% new sample
			fr.AvgResponseTime = fr.AvgResponseTime*0.9 + responseTime*0.1
		}
	}
	
	fr.UpdatedAt = now
}

// ShouldTransitionState checks if the relationship should transition to a new state
func (fr *FederationRelationship) ShouldTransitionState() (RelationshipState, bool) {
	now := time.Now()
	timeSinceActivity := now.Sub(fr.LastActivity)
	
	switch fr.State {
	case StateActive:
		if timeSinceActivity > 7*24*time.Hour {
			return StateIdle, true
		}
	case StateIdle:
		if timeSinceActivity <= 7*24*time.Hour {
			return StateActive, true
		}
		if timeSinceActivity > 30*24*time.Hour {
			return StateDormant, true
		}
	case StateDormant:
		if timeSinceActivity <= 7*24*time.Hour {
			return StateActive, true
		}
		if timeSinceActivity <= 30*24*time.Hour {
			return StateIdle, true
		}
		if timeSinceActivity > 90*24*time.Hour {
			return StateArchived, true
		}
	case StateArchived:
		if timeSinceActivity <= 7*24*time.Hour {
			return StateActive, true // Reactivation
		}
		if timeSinceActivity > 365*24*time.Hour {
			return StateExpired, true
		}
	case StateExpired:
		// Can only be reactivated manually or through new activity
		if timeSinceActivity <= 7*24*time.Hour {
			return StateActive, true
		}
	}
	
	return fr.State, false
}

// TransitionToState changes the relationship state and handles associated logic
func (fr *FederationRelationship) TransitionToState(newState RelationshipState) {
	oldState := fr.State
	fr.State = newState
	fr.StateChangedAt = time.Now()
	
	// Handle state-specific transitions
	switch newState {
	case StateActive:
		if oldState == StateArchived || oldState == StateExpired {
			// Reactivation: set warmup period
			warmupEnd := time.Now().Add(1 * time.Hour)
			fr.WarmupUntil = &warmupEnd
			fr.CurrentRate = 0.1 // Start with 10% traffic
			
			// Restore historical baseline if available
			if fr.HistoricalBaseline > 0 {
				fr.SuccessRate = fr.HistoricalBaseline * 0.3 // 30% weight to historical data
			}
		}
		
	case StateDormant:
		if oldState == StateActive || oldState == StateIdle {
			// Store current success rate as historical baseline
			fr.HistoricalBaseline = fr.SuccessRate
		}
		
	case StateArchived:
		// Compress metrics for storage optimization
		fr.compressMetrics()
		
	case StateExpired:
		// Clear sensitive data before deletion
		fr.clearSensitiveData()
	}
	
	// Update keys to reflect new state
	fr.UpdateKeys()
}

// IsInWarmup checks if the relationship is in warmup period after reactivation
func (fr *FederationRelationship) IsInWarmup() bool {
	return fr.WarmupUntil != nil && time.Now().Before(*fr.WarmupUntil)
}

// GetTrafficRate returns the current traffic rate (1.0 = 100%, 0.1 = 10%)
func (fr *FederationRelationship) GetTrafficRate() float64 {
	if !fr.IsInWarmup() {
		return 1.0 // Full traffic
	}
	
	// Ramp up during warmup period
	warmupDuration := fr.WarmupUntil.Sub(fr.StateChangedAt)
	elapsed := time.Since(fr.StateChangedAt)
	progress := elapsed.Seconds() / warmupDuration.Seconds()
	
	// Exponential ramp-up: 10% → 100%
	rate := 0.1 * (1.0 + 9.0*progress*progress)
	if rate > 1.0 {
		rate = 1.0
	}
	
	fr.CurrentRate = rate
	return rate
}

// compressMetrics compresses historical metrics data for storage optimization
func (fr *FederationRelationship) compressMetrics() {
	// In a real implementation, this would compress the historical data
	// For now, just create a summary
	summary := fmt.Sprintf("compressed_metrics_v1:total_attempts=%d,success_rate=%.2f,avg_response_time=%.2f",
		fr.TotalAttempts, fr.SuccessRate, fr.AvgResponseTime)
	fr.CompressedMetrics = summary
	
	// Clear detailed metrics to save space
	fr.SuccessCount15m = 0
	fr.FailureCount15m = 0
}

// clearSensitiveData clears sensitive data before relationship expiration
func (fr *FederationRelationship) clearSensitiveData() {
	// Keep only essential data for audit purposes
	fr.CompressedMetrics = ""
	fr.ArchiveLocation = ""
}

// FederationRelationshipAggregate represents aggregated relationship metrics for an instance
type FederationRelationshipAggregate struct {
	PK                  string            `dynamorm:"pk"`
	SK                  string            `dynamorm:"sk"`
	GSI1PK              string            `dynamorm:"index:gsi1,pk"`   // Instance-based queries
	GSI1SK              string            `dynamorm:"index:gsi1,sk"`   // Period + timestamp
	TTL                 int64             `json:"ttl,omitempty" dynamorm:"ttl"`
	
	// Aggregate identification
	InstanceDomain      string            `json:"instance_domain"`
	Period              string            `json:"period"`             // 15min, hourly, daily
	Timestamp           time.Time         `json:"timestamp"`
	
	// Aggregated metrics
	ActiveRelationships int64             `json:"active_relationships"`
	IdleRelationships   int64             `json:"idle_relationships"`
	DormantRelationships int64            `json:"dormant_relationships"`
	TotalRelationships  int64             `json:"total_relationships"`
	
	// Success rates
	OverallSuccessRate  float64           `json:"overall_success_rate"`
	TotalSuccesses15m   int64             `json:"total_successes_15m"`
	TotalFailures15m    int64             `json:"total_failures_15m"`
	
	// Performance metrics
	AvgResponseTime     float64           `json:"avg_response_time"`
	P95ResponseTime     float64           `json:"p95_response_time"`
	
	// State transitions (for lifecycle analysis)
	StateTransitions    map[string]int64  `json:"state_transitions"`
	ReactivationCount   int64             `json:"reactivation_count"`
	
	CreatedAt           time.Time         `json:"created_at"`
}

// UpdateKeys sets the DynamoDB keys for the relationship aggregate
func (fra *FederationRelationshipAggregate) UpdateKeys() {
	// Primary keys: Instance + period
	fra.PK = fmt.Sprintf("INSTANCE#%s#FEDERATION_AGG", fra.InstanceDomain)
	fra.SK = fmt.Sprintf("PERIOD#%s#%s", fra.Period, fra.Timestamp.Format(common.CompactTimeFormat))
	
	// GSI1: Cross-instance analysis
	fra.GSI1PK = fmt.Sprintf("FEDERATION_AGG#%s", fra.Period)
	fra.GSI1SK = fmt.Sprintf("%s#%s", fra.Timestamp.Format(common.CompactTimeFormat), fra.InstanceDomain)
	
	// Set TTL based on period
	switch fra.Period {
	case "15min":
		// Keep 15-minute data for 7 days
		fra.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
	case "hourly":
		// Keep hourly data for 30 days
		fra.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	case PeriodDaily:
		// Keep daily data for 365 days
		fra.TTL = time.Now().Add(365 * 24 * time.Hour).Unix()
	default:
		// Default to 30 days
		fra.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}
}

// FederationRelationshipIndex represents a minimal index entry for archived relationships
type FederationRelationshipIndex struct {
	PK              string            `dynamorm:"pk"`
	SK              string            `dynamorm:"sk"`
	TTL             int64             `json:"ttl,omitempty" dynamorm:"ttl"`
	
	RelationshipID  string            `json:"relationship_id"`
	UserID          string            `json:"user_id"`
	TargetInstance  string            `json:"target_instance"`
	State           RelationshipState `json:"state"`
	LastActivity    time.Time         `json:"last_activity"`
	ArchiveLocation string            `json:"archive_location,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

// UpdateKeys sets the DynamoDB keys for the relationship index
func (fri *FederationRelationshipIndex) UpdateKeys() {
	fri.PK = fmt.Sprintf("FEDERATION_REL_INDEX#%s", fri.RelationshipID)
	fri.SK = "INDEX"
	
	// TTL after 2 years for audit purposes
	fri.TTL = time.Now().Add(2 * 365 * 24 * time.Hour).Unix()
}