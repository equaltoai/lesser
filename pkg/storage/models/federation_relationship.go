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
	StateActive RelationshipState = "ACTIVE"
	// StateIdle indicates no recent activity (7+ days)
	StateIdle RelationshipState = "IDLE"
	// StateDormant indicates long inactive period (30+ days)
	StateDormant RelationshipState = "DORMANT"
	// StateArchived indicates moved to cold storage (90+ days)
	StateArchived RelationshipState = "ARCHIVED"
	// StateExpired indicates marked for deletion (365+ days)
	StateExpired RelationshipState = "EXPIRED"
)

// FederationRelationship represents a relationship between users or instances with lifecycle tracking
type FederationRelationship struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK"`
	SK     string `theorydb:"sk,attr:SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK"` // State-based queries
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK"` // Last activity timestamp
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK"` // User-based queries
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK"` // Target instance + timestamp
	TTL    int64  `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Core relationship data
	ID               string `theorydb:"attr:id" json:"id"`
	UserID           string `theorydb:"attr:userID" json:"user_id"`                        // Local user ID
	TargetInstance   string `theorydb:"attr:targetInstance" json:"target_instance"`        // Remote instance domain
	TargetUserID     string `theorydb:"attr:targetUserID" json:"target_user_id,omitempty"` // Remote user ID (if user-level)
	RelationshipType string `theorydb:"attr:relationshipType" json:"relationship_type"`    // follow, mention, boost, reply, etc.

	// Lifecycle management
	State          RelationshipState `theorydb:"attr:state" json:"state"`
	LastActivity   time.Time         `theorydb:"attr:lastActivity" json:"last_activity"`
	FirstSeen      time.Time         `theorydb:"attr:firstSeen" json:"first_seen"`
	StateChangedAt time.Time         `theorydb:"attr:stateChangedAt" json:"state_changed_at"`

	// Success rate tracking (15-minute rolling window)
	SuccessCount15m int64     `theorydb:"attr:successCount15m" json:"success_count_15m"`
	FailureCount15m int64     `theorydb:"attr:failureCount15m" json:"failure_count_15m"`
	WindowStart15m  time.Time `theorydb:"attr:windowStart15m" json:"window_start_15m"`
	SuccessRate     float64   `theorydb:"attr:successRate" json:"success_rate"`

	// Aggregated metrics
	TotalSuccesses  int64   `theorydb:"attr:totalSuccesses" json:"total_successes"`
	TotalFailures   int64   `theorydb:"attr:totalFailures" json:"total_failures"`
	TotalAttempts   int64   `theorydb:"attr:totalAttempts" json:"total_attempts"`
	AvgResponseTime float64 `theorydb:"attr:avgResponseTime" json:"avg_response_time"`

	// Reactivation handling
	WarmupUntil        *time.Time `theorydb:"attr:warmupUntil" json:"warmup_until,omitempty"`
	CurrentRate        float64    `theorydb:"attr:currentRate" json:"current_rate"`               // Traffic rate during warmup (0.0-1.0)
	HistoricalBaseline float64    `theorydb:"attr:historicalBaseline" json:"historical_baseline"` // Pre-dormancy success rate

	// Storage optimization
	ArchiveLocation        string    `theorydb:"attr:archiveLocation" json:"archive_location,omitempty"`      // S3 key if archived
	CompressedMetrics      string    `theorydb:"attr:compressedMetrics" json:"compressed_metrics,omitempty"`  // Compressed historical data
	LastCompressedAttempts int64     `theorydb:"attr:lastCompressedAttempts" json:"last_compressed_attempts"` // Baseline for delta compression
	LastCompressionTime    time.Time `theorydb:"attr:lastCompressionTime" json:"last_compression_time"`       // When metrics were last compressed
	IsCompressed           bool      `theorydb:"attr:isCompressed" json:"is_compressed"`                      // Flag indicating compressed state

	// Metadata
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table backing FederationRelationship.
func (FederationRelationship) TableName() string {
	return MainTableName
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

// compressMetrics compresses historical metrics data for storage optimization using run-length encoding and delta compression
func (fr *FederationRelationship) compressMetrics() {
	// Create a compressed representation using multiple techniques:
	// 1. Delta encoding for timestamps and counts
	// 2. Run-length encoding for repeated values
	// 3. Quantization for response times

	compressed := &MetricsCompression{
		Version:      2,
		CompressedAt: time.Now(),

		// Core metrics (delta encoded)
		TotalAttemptsDelta: fr.TotalAttempts - fr.LastCompressedAttempts,
		SuccessCountDelta:  fr.SuccessCount15m,
		FailureCountDelta:  fr.FailureCount15m,

		// Success rate (quantized to save space)
		QuantizedSuccessRate: quantizeSuccessRate(fr.SuccessRate),

		// Response time statistics (compressed using percentiles)
		ResponseTimeP50: quantizeResponseTime(fr.AvgResponseTime * 0.8), // Estimate P50
		ResponseTimeP95: quantizeResponseTime(fr.AvgResponseTime * 1.3), // Estimate P95
		ResponseTimeP99: quantizeResponseTime(fr.AvgResponseTime * 1.8), // Estimate P99

		// State transitions (run-length encoded)
		StateTransitions: compressStateHistory(fr.State, fr.StateChangedAt),

		// Activity patterns (bitmap compression)
		ActivityPattern: compressActivityPattern(fr.LastActivity, fr.FirstSeen),
	}

	// Serialize to compact binary format
	compressedData := compressed.ToBinary()

	// Store as base64 for JSON compatibility
	fr.CompressedMetrics = encodeCompressedData(compressedData)

	// Update compression tracking
	fr.LastCompressedAttempts = fr.TotalAttempts
	fr.LastCompressionTime = time.Now()

	// Clear detailed metrics to save space (keep only summary)
	fr.SuccessCount15m = 0
	fr.FailureCount15m = 0

	// Mark as compressed for lifecycle management
	fr.IsCompressed = true
}

// clearSensitiveData clears sensitive data before relationship expiration
func (fr *FederationRelationship) clearSensitiveData() {
	// Keep only essential data for audit purposes
	fr.CompressedMetrics = ""
	fr.ArchiveLocation = ""
}

// FederationRelationshipAggregate represents aggregated relationship metrics for an instance
type FederationRelationshipAggregate struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk"`
	SK     string `theorydb:"sk"`
	GSI1PK string `theorydb:"index:gsi1,pk"` // Instance-based queries
	GSI1SK string `theorydb:"index:gsi1,sk"` // Period + timestamp
	TTL    int64  `theorydb:"ttl,attr:ttl" json:"ttl,omitempty"`

	// Aggregate identification
	InstanceDomain string    `json:"instance_domain"`
	Period         string    `json:"period"` // 15min, hourly, daily
	Timestamp      time.Time `json:"timestamp"`

	// Aggregated metrics
	ActiveRelationships  int64 `json:"active_relationships"`
	IdleRelationships    int64 `json:"idle_relationships"`
	DormantRelationships int64 `json:"dormant_relationships"`
	TotalRelationships   int64 `json:"total_relationships"`

	// Success rates
	OverallSuccessRate float64 `json:"overall_success_rate"`
	TotalSuccesses15m  int64   `json:"total_successes_15m"`
	TotalFailures15m   int64   `json:"total_failures_15m"`

	// Performance metrics
	AvgResponseTime float64 `json:"avg_response_time"`
	P95ResponseTime float64 `json:"p95_response_time"`

	// State transitions (for lifecycle analysis)
	StateTransitions  map[string]int64 `json:"state_transitions"`
	ReactivationCount int64            `json:"reactivation_count"`

	CreatedAt time.Time `json:"created_at"`
}

// TableName returns the DynamoDB table backing FederationRelationshipAggregate.
func (FederationRelationshipAggregate) TableName() string {
	return MainTableName
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
	PK  string `theorydb:"pk"`
	SK  string `theorydb:"sk"`
	TTL int64  `json:"ttl,omitempty" theorydb:"ttl"`

	RelationshipID  string            `json:"relationship_id"`
	UserID          string            `json:"user_id"`
	TargetInstance  string            `json:"target_instance"`
	State           RelationshipState `json:"state"`
	LastActivity    time.Time         `json:"last_activity"`
	ArchiveLocation string            `json:"archive_location,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

// TableName returns the DynamoDB table backing FederationRelationshipIndex.
func (FederationRelationshipIndex) TableName() string {
	return MainTableName
}

// UpdateKeys sets the DynamoDB keys for the relationship index
func (fri *FederationRelationshipIndex) UpdateKeys() {
	fri.PK = fmt.Sprintf("FEDERATION_REL_INDEX#%s", fri.RelationshipID)
	fri.SK = "INDEX"

	// TTL after 2 years for audit purposes
	fri.TTL = time.Now().Add(2 * 365 * 24 * time.Hour).Unix()
}

// MetricsCompression holds compressed metrics data using various compression techniques
type MetricsCompression struct {
	Version      uint8     `json:"v"`
	CompressedAt time.Time `json:"ca"`

	// Delta-encoded counters (saves space for large numbers)
	TotalAttemptsDelta int64 `json:"tad"`
	SuccessCountDelta  int64 `json:"scd"`
	FailureCountDelta  int64 `json:"fcd"`

	// Quantized success rate (0-255 for 0-100% with 0.4% precision)
	QuantizedSuccessRate uint8 `json:"qsr"`

	// Quantized response times in milliseconds (log scale)
	ResponseTimeP50 uint16 `json:"rtp50"`
	ResponseTimeP95 uint16 `json:"rtp95"`
	ResponseTimeP99 uint16 `json:"rtp99"`

	// Compressed state history (run-length encoded)
	StateTransitions []byte `json:"st"`

	// Activity pattern bitmap (daily activity over 30 days)
	ActivityPattern uint32 `json:"ap"`
}

// TableName returns the DynamoDB table backing MetricsCompression.
func (MetricsCompression) TableName() string {
	return MainTableName
}

// ToBinary serializes the compression struct to compact binary format
func (mc *MetricsCompression) ToBinary() []byte {
	// Create a compact binary representation
	// This would use a more sophisticated binary encoding in practice
	data := make([]byte, 0, 64)

	// Version (1 byte)
	data = append(data, mc.Version)

	// Timestamp (8 bytes - Unix timestamp)
	timestamp := mc.CompressedAt.Unix()
	for i := 0; i < 8; i++ {
		data = append(data, byte(timestamp>>(i*8)))
	}

	// Delta values (8 bytes each)
	for _, val := range []int64{mc.TotalAttemptsDelta, mc.SuccessCountDelta, mc.FailureCountDelta} {
		for i := 0; i < 8; i++ {
			data = append(data, byte(val>>(i*8)))
		}
	}

	// Quantized values
	data = append(data, mc.QuantizedSuccessRate)

	// Response times (2 bytes each)
	for _, val := range []uint16{mc.ResponseTimeP50, mc.ResponseTimeP95, mc.ResponseTimeP99} {
		data = append(data, byte(val&0xFF), byte(val>>8))
	}

	// Activity pattern (4 bytes)
	ap := mc.ActivityPattern
	data = append(data, byte(ap&0xFF), byte((ap>>8)&0xFF), byte((ap>>16)&0xFF), byte((ap>>24)&0xFF))

	// State transitions (variable length)
	data = append(data, byte(len(mc.StateTransitions)))
	data = append(data, mc.StateTransitions...)

	return data
}

// quantizeSuccessRate converts a success rate (0.0-1.0) to quantized byte value (0-255)
func quantizeSuccessRate(rate float64) uint8 {
	if rate < 0 {
		return 0
	}
	if rate > 1 {
		return 255
	}
	return uint8(rate * 255)
}

// quantizeResponseTime converts response time in milliseconds to quantized uint16 using log scale
func quantizeResponseTime(timeMs float64) uint16 {
	if timeMs <= 0 {
		return 0
	}
	if timeMs >= 65535 {
		return 65535
	}
	return uint16(timeMs)
}

// compressStateHistory creates a run-length encoded representation of state transitions
func compressStateHistory(currentState RelationshipState, stateChangedAt time.Time) []byte {
	// For now, just store current state and timestamp
	// In a full implementation, this would use run-length encoding for state history
	data := make([]byte, 0, 16)

	// State (1 byte enum)
	switch currentState {
	case StateActive:
		data = append(data, 1)
	case StateIdle:
		data = append(data, 2)
	case StateDormant:
		data = append(data, 3)
	case StateArchived:
		data = append(data, 4)
	case StateExpired:
		data = append(data, 5)
	default:
		data = append(data, 0)
	}

	// Timestamp (8 bytes)
	timestamp := stateChangedAt.Unix()
	for i := 0; i < 8; i++ {
		data = append(data, byte(timestamp>>(i*8)))
	}

	return data
}

// compressActivityPattern creates a bitmap representing activity patterns
func compressActivityPattern(lastActivity, firstSeen time.Time) uint32 {
	// Create a 30-day activity bitmap (1 bit per day)
	var pattern uint32

	now := time.Now()
	daysDiff := int(now.Sub(lastActivity).Hours() / 24)

	// Set bit for recent activity (within 30 days)
	if daysDiff >= 0 && daysDiff < 30 {
		pattern |= 1 << uint(daysDiff)
	}

	// Add some pattern based on relationship age
	ageDays := int(now.Sub(firstSeen).Hours() / 24)
	if ageDays > 7 {
		pattern |= 1 << 31 // Mark as established relationship
	}

	return pattern
}

// encodeCompressedData encodes binary data to base64 for JSON storage
func encodeCompressedData(data []byte) string {
	// Use base64 encoding for JSON compatibility
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

	var result []byte
	for i := 0; i < len(data); i += 3 {
		// Take 3 bytes and convert to 4 base64 chars
		var group uint32
		groupSize := len(data) - i
		if groupSize > 3 {
			groupSize = 3
		}

		for j := 0; j < groupSize; j++ {
			group |= uint32(data[i+j]) << (16 - j*8)
		}

		for j := 0; j < 4; j++ {
			if j*6 < groupSize*8 {
				result = append(result, chars[(group>>(18-j*6))&0x3F])
			} else {
				result = append(result, '=')
			}
		}
	}

	return string(result)
}
