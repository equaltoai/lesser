package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"go.uber.org/zap"
)

// RouteThresholdManager manages route health thresholds and decisions based on guidance document
type RouteThresholdManager struct {
	logger *zap.Logger
	config *ThresholdConfig
}

// ThresholdConfig defines the thresholds from the route optimization guidance
type ThresholdConfig struct {
	// Latency Thresholds (from guidance document)
	P95LatencyThreshold  time.Duration // 5s - Trigger route change
	P99LatencyThreshold  time.Duration // 10s - Hard limit before timeout
	AvgLatencyThreshold  time.Duration // 2s - Sustained poor performance
	DegradationWindow    time.Duration // 5min - Window for degradation detection
	DegradationIncrease  float64       // 0.5 - 50% increase triggers degradation

	// Success Rate Thresholds
	CriticalSuccessRate  float64 // 0.5 - < 50% = Open circuit immediately
	DegradedSuccessRate  float64 // 0.7 - < 70% = Mark as degraded, reduce traffic
	MonitorSuccessRate   float64 // 0.9 - < 90% = Monitor closely
	PreferredSuccessRate float64 // 0.95 - > 95% = Preferred route

	// Decision Windows
	SampleWindow       time.Duration // 5min or last 100 requests
	MinSamplesRequired int           // 10 - Minimum samples before decisions

	// Cache TTL Strategy
	HealthyRouteTTL   time.Duration // 5min - Stable routes
	DegradedRouteTTL  time.Duration // 30s - Re-evaluate frequently
	UnknownRouteTTL   time.Duration // 1min - New routes
	HighPriorityTTL   time.Duration // 10s - Direct messages, mentions
	NormalPriorityTTL time.Duration // 2min - Regular posts
	LowPriorityTTL    time.Duration // 10min - Bulk updates, deletes

	// Emergency Mode Configuration
	EmergencyThreshold float64       // 0.3 - When to enter emergency mode
	RecoveryProbeInterval time.Duration // 30s - Probe interval during recovery
	RecoverySuccessThreshold int      // 3 - Consecutive successes to mark healthy
}

// RouteHealthStatus represents the current health status of a route
type RouteHealthStatus int

// Route health status levels
const (
	// RouteHealthUnknown indicates unknown health status
	RouteHealthUnknown RouteHealthStatus = iota
	// RouteHealthPreferred indicates preferred route status
	RouteHealthPreferred
	// RouteHealthHealthy indicates healthy route status
	RouteHealthHealthy
	// RouteHealthMonitored indicates route is being monitored
	RouteHealthMonitored
	// RouteHealthDegraded indicates degraded performance
	RouteHealthDegraded
	// RouteHealthCritical indicates critical issues
	RouteHealthCritical
	// RouteHealthEmergency indicates emergency status
	RouteHealthEmergency
)

// String returns the string representation of RouteHealthStatus
func (rhs RouteHealthStatus) String() string {
	switch rhs {
	case RouteHealthPreferred:
		return "preferred"
	case RouteHealthHealthy:
		return "healthy"
	case RouteHealthMonitored:
		return "monitored"
	case RouteHealthDegraded:
		return "degraded"
	case RouteHealthCritical:
		return "critical"
	case RouteHealthEmergency:
		return "emergency"
	default:
		return "unknown"
	}
}

// RouteHealthAssessment contains the complete health assessment of a route
type RouteHealthAssessment struct {
	RouteID           string
	Status            RouteHealthStatus
	SuccessRate       float64
	AvgLatency        time.Duration
	P95Latency        time.Duration
	P99Latency        time.Duration
	SampleCount       int
	LastUpdated       time.Time
	CacheTTL          time.Duration
	RecommendedAction string
	DegradationReason string
}

// MessagePriority represents priority levels for message prioritization during degraded conditions
type MessagePriority int

// Message priority levels
const (
	// PriorityCritical for direct replies and mentions
	PriorityCritical MessagePriority = iota
	// PriorityHigh for follows and likes from verified accounts
	PriorityHigh
	// PriorityNormal for regular posts and boosts
	PriorityNormal
	// PriorityLow for deletes and updates to old content
	PriorityLow
)

// NewRouteThresholdManager creates a new threshold manager with guidance document defaults
func NewRouteThresholdManager(logger *zap.Logger, config *ThresholdConfig) *RouteThresholdManager {
	if config == nil {
		config = DefaultThresholdConfig()
	}

	return &RouteThresholdManager{
		logger: logger,
		config: config,
	}
}

// DefaultThresholdConfig returns the default configuration from the guidance document
func DefaultThresholdConfig() *ThresholdConfig {
	return &ThresholdConfig{
		// Latency Thresholds
		P95LatencyThreshold:  5 * time.Second,
		P99LatencyThreshold:  10 * time.Second,
		AvgLatencyThreshold:  2 * time.Second,
		DegradationWindow:    5 * time.Minute,
		DegradationIncrease:  0.5, // 50%

		// Success Rate Thresholds
		CriticalSuccessRate:  0.5,  // 50%
		DegradedSuccessRate:  0.7,  // 70%
		MonitorSuccessRate:   0.9,  // 90%
		PreferredSuccessRate: 0.95, // 95%

		// Decision Windows
		SampleWindow:       5 * time.Minute,
		MinSamplesRequired: 10,

		// Cache TTL Strategy
		HealthyRouteTTL:   5 * time.Minute,
		DegradedRouteTTL:  30 * time.Second,
		UnknownRouteTTL:   1 * time.Minute,
		HighPriorityTTL:   10 * time.Second,
		NormalPriorityTTL: 2 * time.Minute,
		LowPriorityTTL:    10 * time.Minute,

		// Emergency Mode
		EmergencyThreshold:        0.3,
		RecoveryProbeInterval:     30 * time.Second,
		RecoverySuccessThreshold: 3,
	}
}

// AssessRouteHealth assesses the health of a route based on metrics
func (rtm *RouteThresholdManager) AssessRouteHealth(_ context.Context, routeID string, metrics *types.RouteMetrics) *RouteHealthAssessment {
	assessment := &RouteHealthAssessment{
		RouteID:     routeID,
		Status:      RouteHealthUnknown,
		SampleCount: int(metrics.TotalMessages),
		LastUpdated: metrics.LastUpdated,
		CacheTTL:    rtm.config.UnknownRouteTTL,
	}

	// Not enough data for assessment
	if metrics.TotalMessages < int64(rtm.config.MinSamplesRequired) {
		assessment.RecommendedAction = "collect more samples"
		return assessment
	}

	// Calculate metrics
	if metrics.TotalMessages > 0 {
		assessment.SuccessRate = float64(metrics.SuccessfulCount) / float64(metrics.TotalMessages)
	}
	assessment.AvgLatency = metrics.AvgLatency
	assessment.P95Latency = metrics.P95Latency
	assessment.P99Latency = metrics.P99Latency

	// Assess based on success rate thresholds
	if assessment.SuccessRate < rtm.config.CriticalSuccessRate {
		assessment.Status = RouteHealthCritical
		assessment.CacheTTL = rtm.config.DegradedRouteTTL
		assessment.RecommendedAction = "open circuit immediately"
		assessment.DegradationReason = fmt.Sprintf("success rate %.1f%% < %.1f%%", assessment.SuccessRate*100, rtm.config.CriticalSuccessRate*100)
	} else if assessment.SuccessRate < rtm.config.DegradedSuccessRate {
		assessment.Status = RouteHealthDegraded
		assessment.CacheTTL = rtm.config.DegradedRouteTTL
		assessment.RecommendedAction = "reduce traffic, implement backpressure"
		assessment.DegradationReason = fmt.Sprintf("success rate %.1f%% < %.1f%%", assessment.SuccessRate*100, rtm.config.DegradedSuccessRate*100)
	} else if assessment.SuccessRate < rtm.config.MonitorSuccessRate {
		assessment.Status = RouteHealthMonitored
		assessment.CacheTTL = rtm.config.HealthyRouteTTL
		assessment.RecommendedAction = "monitor closely, consider alternatives"
		assessment.DegradationReason = fmt.Sprintf("success rate %.1f%% < %.1f%%", assessment.SuccessRate*100, rtm.config.MonitorSuccessRate*100)
	} else if assessment.SuccessRate >= rtm.config.PreferredSuccessRate {
		assessment.Status = RouteHealthPreferred
		assessment.CacheTTL = rtm.config.HealthyRouteTTL
		assessment.RecommendedAction = "preferred route"
	} else {
		assessment.Status = RouteHealthHealthy
		assessment.CacheTTL = rtm.config.HealthyRouteTTL
		assessment.RecommendedAction = "healthy route"
	}

	// Check latency thresholds - can override success rate assessment
	latencyDegradation := rtm.assessLatencyDegradation(assessment)
	if latencyDegradation != "" {
		if assessment.Status == RouteHealthPreferred || assessment.Status == RouteHealthHealthy {
			assessment.Status = RouteHealthDegraded
			assessment.CacheTTL = rtm.config.DegradedRouteTTL
			assessment.RecommendedAction = "reduce traffic due to latency"
		}
		if assessment.DegradationReason == "" {
			assessment.DegradationReason = latencyDegradation
		} else {
			assessment.DegradationReason += "; " + latencyDegradation
		}
	}

	rtm.logger.Debug("Route health assessed",
		zap.String("routeID", routeID),
		zap.String("status", assessment.Status.String()),
		zap.Float64("successRate", assessment.SuccessRate),
		zap.Duration("avgLatency", assessment.AvgLatency),
		zap.Duration("p95Latency", assessment.P95Latency),
		zap.Duration("cacheTTL", assessment.CacheTTL),
		zap.String("action", assessment.RecommendedAction))

	return assessment
}

// assessLatencyDegradation checks if latency thresholds are exceeded
func (rtm *RouteThresholdManager) assessLatencyDegradation(assessment *RouteHealthAssessment) string {
	var reasons []string

	if assessment.P99Latency > rtm.config.P99LatencyThreshold {
		reasons = append(reasons, fmt.Sprintf("P99 latency %v > %v", assessment.P99Latency, rtm.config.P99LatencyThreshold))
	}

	if assessment.P95Latency > rtm.config.P95LatencyThreshold {
		reasons = append(reasons, fmt.Sprintf("P95 latency %v > %v", assessment.P95Latency, rtm.config.P95LatencyThreshold))
	}

	if assessment.AvgLatency > rtm.config.AvgLatencyThreshold {
		reasons = append(reasons, fmt.Sprintf("Avg latency %v > %v", assessment.AvgLatency, rtm.config.AvgLatencyThreshold))
	}

	if len(reasons) > 0 {
		return fmt.Sprintf("latency degradation: %v", reasons)
	}

	return ""
}

// GetCacheTTLForMessageType returns appropriate cache TTL based on message priority
func (rtm *RouteThresholdManager) GetCacheTTLForMessageType(messageType types.MessageType, routeHealth RouteHealthStatus) time.Duration {
	// Priority-based TTL
	priority := rtm.getMessagePriority(messageType)

	switch priority {
	case PriorityCritical:
		return rtm.config.HighPriorityTTL
	case PriorityHigh:
		if routeHealth == RouteHealthDegraded || routeHealth == RouteHealthCritical {
			return rtm.config.DegradedRouteTTL
		}
		return rtm.config.NormalPriorityTTL
	case PriorityNormal:
		if routeHealth == RouteHealthDegraded || routeHealth == RouteHealthCritical {
			return rtm.config.DegradedRouteTTL
		}
		return rtm.config.NormalPriorityTTL
	case PriorityLow:
		return rtm.config.LowPriorityTTL
	default:
		return rtm.config.NormalPriorityTTL
	}
}

// getMessagePriority determines the priority of a message type
func (rtm *RouteThresholdManager) getMessagePriority(messageType types.MessageType) MessagePriority {
	switch messageType {
	case types.MessageTypeCreate:
		return PriorityNormal
	case types.MessageTypeUpdate:
		return PriorityNormal
	case types.MessageTypeDelete:
		return PriorityLow
	case types.MessageTypeAnnounce:
		return PriorityNormal
	case types.MessageTypeUndo:
		return PriorityLow
	case types.MessageTypeFollow:
		return PriorityHigh
	case types.MessageTypeLike:
		return PriorityHigh
	default:
		return PriorityNormal
	}
}

// ShouldEnterEmergencyMode determines if the system should enter emergency mode
func (rtm *RouteThresholdManager) ShouldEnterEmergencyMode(healthyRoutes int, totalRoutes int) bool {
	if totalRoutes == 0 {
		return false
	}

	healthyRatio := float64(healthyRoutes) / float64(totalRoutes)
	return healthyRatio < rtm.config.EmergencyThreshold
}

// GetEmergencyBackpressureRules returns backpressure rules for emergency mode
func (rtm *RouteThresholdManager) GetEmergencyBackpressureRules() map[MessagePriority]BackpressureRule {
	return map[MessagePriority]BackpressureRule{
		PriorityCritical: {
			Threshold:  0.0, // Always allow critical messages
			Action:     "allow",
			RateLimit:  1 * time.Second,
			QueueDepth: 1000,
		},
		PriorityHigh: {
			Threshold:  0.3, // Queue high priority when < 30% healthy
			Action:     "queue_if_below_threshold",
			RateLimit:  5 * time.Second,
			QueueDepth: 500,
		},
		PriorityNormal: {
			Threshold:  0.5, // Queue normal priority when < 50% healthy
			Action:     "queue_if_below_threshold",
			RateLimit:  30 * time.Second,
			QueueDepth: 100,
		},
		PriorityLow: {
			Threshold:  0.7, // Queue low priority when < 70% healthy
			Action:     "queue_if_below_threshold",
			RateLimit:  5 * time.Minute,
			QueueDepth: 50,
		},
	}
}

// BackpressureRule defines how to handle messages during degraded conditions
type BackpressureRule struct {
	Threshold  float64       // Health ratio threshold
	Action     string        // "allow", "queue", "drop", "queue_if_below_threshold"
	RateLimit  time.Duration // Rate limiting interval
	QueueDepth int           // Maximum queue depth
}

// GetRecoverySteps returns the gradual recovery steps from guidance document
func (rtm *RouteThresholdManager) GetRecoverySteps() []RecoveryStep {
	return []RecoveryStep{
		{Load: 0.1, Duration: 1 * time.Minute, Description: "10% traffic"},
		{Load: 0.3, Duration: 2 * time.Minute, Description: "30% traffic"},
		{Load: 0.5, Duration: 5 * time.Minute, Description: "50% traffic"},
		{Load: 1.0, Duration: 0, Description: "Full traffic"},
	}
}

// RecoveryStep represents a step in the gradual recovery process
type RecoveryStep struct {
	Load        float64       // Traffic load percentage (0.0-1.0)
	Duration    time.Duration // How long to maintain this load
	Description string        // Human-readable description
}

// CalculateRouteCacheKey generates cache key considering all factors from guidance
func (rtm *RouteThresholdManager) CalculateRouteCacheKey(sourceInstance, targetInstance string, activityType types.MessageType, messageSizeClass int) string {
	return fmt.Sprintf("%s:%s:%s:%d",
		sourceInstance,
		targetInstance,
		string(activityType),
		messageSizeClass,
	)
}

// GetMessageSizeClass classifies message size for cache key generation
func (rtm *RouteThresholdManager) GetMessageSizeClass(messageSize int64) int {
	switch {
	case messageSize <= 1024: // 0-1KB
		return 0
	case messageSize <= 10*1024: // 1-10KB
		return 1
	default: // 10KB+
		return 2
	}
}