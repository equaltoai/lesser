package routing

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"go.uber.org/zap/zaptest"
)

func TestRouteThresholdManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, nil)

	// Test with default config
	if manager.config == nil {
		t.Fatal("Expected default config to be set")
	}

	// Verify default thresholds match guidance document
	config := manager.config
	if config.P95LatencyThreshold != 5*time.Second {
		t.Errorf("Expected P95 threshold 5s, got %v", config.P95LatencyThreshold)
	}
	if config.P99LatencyThreshold != 10*time.Second {
		t.Errorf("Expected P99 threshold 10s, got %v", config.P99LatencyThreshold)
	}
	if config.AvgLatencyThreshold != 2*time.Second {
		t.Errorf("Expected avg threshold 2s, got %v", config.AvgLatencyThreshold)
	}
	if config.CriticalSuccessRate != 0.5 {
		t.Errorf("Expected critical success rate 0.5, got %f", config.CriticalSuccessRate)
	}
	if config.DegradedSuccessRate != 0.7 {
		t.Errorf("Expected degraded success rate 0.7, got %f", config.DegradedSuccessRate)
	}
	if config.PreferredSuccessRate != 0.95 {
		t.Errorf("Expected preferred success rate 0.95, got %f", config.PreferredSuccessRate)
	}
}

func TestRouteHealthAssessment(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())
	ctx := context.Background()

	tests := []struct {
		name           string
		metrics        *types.RouteMetrics
		expectedStatus RouteHealthStatus
		expectedAction string
	}{
		{
			name: "insufficient_samples",
			metrics: &types.RouteMetrics{
				TotalMessages:    5, // Less than minimum required (10)
				SuccessfulCount:  5,
				FailedCount:      0,
				AvgLatency:       100 * time.Millisecond,
				P95Latency:       200 * time.Millisecond,
				P99Latency:       300 * time.Millisecond,
				LastUpdated:      time.Now(),
			},
			expectedStatus: RouteHealthUnknown,
			expectedAction: "collect more samples",
		},
		{
			name: "preferred_route",
			metrics: &types.RouteMetrics{
				TotalMessages:    100,
				SuccessfulCount:  97, // 97% success rate > 95%
				FailedCount:      3,
				AvgLatency:       500 * time.Millisecond, // Good latency
				P95Latency:       1 * time.Second,        // Good latency
				P99Latency:       2 * time.Second,        // Good latency
				LastUpdated:      time.Now(),
			},
			expectedStatus: RouteHealthPreferred,
			expectedAction: "preferred route",
		},
		{
			name: "healthy_route",
			metrics: &types.RouteMetrics{
				TotalMessages:    100,
				SuccessfulCount:  92, // 92% success rate (90-95%)
				FailedCount:      8,
				AvgLatency:       800 * time.Millisecond,
				P95Latency:       1500 * time.Millisecond,
				P99Latency:       3 * time.Second,
				LastUpdated:      time.Now(),
			},
			expectedStatus: RouteHealthHealthy,
			expectedAction: "healthy route",
		},
		{
			name: "monitored_route",
			metrics: &types.RouteMetrics{
				TotalMessages:    100,
				SuccessfulCount:  85, // 85% success rate (< 90%)
				FailedCount:      15,
				AvgLatency:       1 * time.Second,
				P95Latency:       2 * time.Second,
				P99Latency:       4 * time.Second,
				LastUpdated:      time.Now(),
			},
			expectedStatus: RouteHealthMonitored,
			expectedAction: "monitor closely, consider alternatives",
		},
		{
			name: "degraded_route_by_success_rate",
			metrics: &types.RouteMetrics{
				TotalMessages:    100,
				SuccessfulCount:  65, // 65% success rate (< 70%)
				FailedCount:      35,
				AvgLatency:       1 * time.Second,
				P95Latency:       3 * time.Second,
				P99Latency:       6 * time.Second,
				LastUpdated:      time.Now(),
			},
			expectedStatus: RouteHealthDegraded,
			expectedAction: "reduce traffic, implement backpressure",
		},
		{
			name: "degraded_route_by_latency",
			metrics: &types.RouteMetrics{
				TotalMessages:    100,
				SuccessfulCount:  92, // Good success rate
				FailedCount:      8,
				AvgLatency:       3 * time.Second,  // > 2s threshold
				P95Latency:       8 * time.Second,  // > 5s threshold
				P99Latency:       15 * time.Second, // > 10s threshold
				LastUpdated:      time.Now(),
			},
			expectedStatus: RouteHealthDegraded,
			expectedAction: "reduce traffic due to latency",
		},
		{
			name: "critical_route",
			metrics: &types.RouteMetrics{
				TotalMessages:    100,
				SuccessfulCount:  45, // 45% success rate (< 50%)
				FailedCount:      55,
				AvgLatency:       2 * time.Second,
				P95Latency:       5 * time.Second,
				P99Latency:       8 * time.Second,
				LastUpdated:      time.Now(),
			},
			expectedStatus: RouteHealthCritical,
			expectedAction: "open circuit immediately",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := manager.AssessRouteHealth(ctx, "test-route", tt.metrics)

			if assessment.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus.String(), assessment.Status.String())
			}

			if assessment.RecommendedAction != tt.expectedAction {
				t.Errorf("Expected action '%s', got '%s'", tt.expectedAction, assessment.RecommendedAction)
			}

			// Verify cache TTL is appropriate for status
			switch tt.expectedStatus {
			case RouteHealthPreferred, RouteHealthHealthy:
				if assessment.CacheTTL != manager.config.HealthyRouteTTL {
					t.Errorf("Expected healthy TTL %v, got %v", manager.config.HealthyRouteTTL, assessment.CacheTTL)
				}
			case RouteHealthDegraded, RouteHealthCritical:
				if assessment.CacheTTL != manager.config.DegradedRouteTTL {
					t.Errorf("Expected degraded TTL %v, got %v", manager.config.DegradedRouteTTL, assessment.CacheTTL)
				}
			case RouteHealthUnknown:
				if assessment.CacheTTL != manager.config.UnknownRouteTTL {
					t.Errorf("Expected unknown TTL %v, got %v", manager.config.UnknownRouteTTL, assessment.CacheTTL)
				}
			}
		})
	}
}

func TestEmergencyModeThresholds(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	tests := []struct {
		name           string
		healthyRoutes  int
		totalRoutes    int
		shouldEmergency bool
	}{
		{
			name:           "healthy_system",
			healthyRoutes:  8,
			totalRoutes:    10,
			shouldEmergency: false, // 80% healthy > 30% threshold
		},
		{
			name:           "degraded_but_not_emergency",
			healthyRoutes:  4,
			totalRoutes:    10,
			shouldEmergency: false, // 40% healthy > 30% threshold
		},
		{
			name:           "emergency_threshold",
			healthyRoutes:  3,
			totalRoutes:    10,
			shouldEmergency: false, // 30% healthy = 30% threshold (not less)
		},
		{
			name:           "emergency_mode",
			healthyRoutes:  2,
			totalRoutes:    10,
			shouldEmergency: true, // 20% healthy < 30% threshold
		},
		{
			name:           "total_failure",
			healthyRoutes:  0,
			totalRoutes:    10,
			shouldEmergency: true, // 0% healthy < 30% threshold
		},
		{
			name:           "no_routes",
			healthyRoutes:  0,
			totalRoutes:    0,
			shouldEmergency: false, // No routes means no emergency mode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.ShouldEnterEmergencyMode(tt.healthyRoutes, tt.totalRoutes)
			if result != tt.shouldEmergency {
				t.Errorf("Expected emergency mode %v, got %v (healthy: %d/%d)",
					tt.shouldEmergency, result, tt.healthyRoutes, tt.totalRoutes)
			}
		})
	}
}

func TestMessagePriority(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	tests := []struct {
		messageType      types.MessageType
		expectedPriority MessagePriority
	}{
		{types.MessageTypeCreate, PriorityNormal},
		{types.MessageTypeUpdate, PriorityNormal},
		{types.MessageTypeDelete, PriorityLow},
		{types.MessageTypeAnnounce, PriorityNormal},
		{types.MessageTypeUndo, PriorityLow},
		{types.MessageTypeFollow, PriorityHigh},
		{types.MessageTypeLike, PriorityHigh},
	}

	for _, tt := range tests {
		t.Run(string(tt.messageType), func(t *testing.T) {
			priority := manager.getMessagePriority(tt.messageType)
			if priority != tt.expectedPriority {
				t.Errorf("Expected priority %v for %s, got %v",
					tt.expectedPriority, tt.messageType, priority)
			}
		})
	}
}

func TestCacheTTLByMessageType(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	tests := []struct {
		name        string
		messageType types.MessageType
		routeHealth RouteHealthStatus
		expectedTTL time.Duration
	}{
		{
			name:        "critical_message_healthy_route",
			messageType: types.MessageTypeFollow, // High priority
			routeHealth: RouteHealthHealthy,
			expectedTTL: manager.config.NormalPriorityTTL,
		},
		{
			name:        "critical_message_degraded_route",
			messageType: types.MessageTypeFollow, // High priority
			routeHealth: RouteHealthDegraded,
			expectedTTL: manager.config.DegradedRouteTTL,
		},
		{
			name:        "low_priority_message",
			messageType: types.MessageTypeDelete, // Low priority
			routeHealth: RouteHealthHealthy,
			expectedTTL: manager.config.LowPriorityTTL,
		},
		{
			name:        "normal_message_healthy_route",
			messageType: types.MessageTypeCreate, // Normal priority
			routeHealth: RouteHealthHealthy,
			expectedTTL: manager.config.NormalPriorityTTL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ttl := manager.GetCacheTTLForMessageType(tt.messageType, tt.routeHealth)
			if ttl != tt.expectedTTL {
				t.Errorf("Expected TTL %v, got %v", tt.expectedTTL, ttl)
			}
		})
	}
}

func TestBackpressureRules(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	rules := manager.GetEmergencyBackpressureRules()

	// Verify all priority levels have rules
	expectedPriorities := []MessagePriority{
		PriorityCritical,
		PriorityHigh,
		PriorityNormal,
		PriorityLow,
	}

	for _, priority := range expectedPriorities {
		if _, exists := rules[priority]; !exists {
			t.Errorf("Missing backpressure rule for priority %v", priority)
		}
	}

	// Verify critical messages are always allowed
	if rules[PriorityCritical].Threshold != 0.0 {
		t.Errorf("Expected critical messages to have 0.0 threshold, got %f",
			rules[PriorityCritical].Threshold)
	}

	// Verify thresholds are increasingly restrictive
	if rules[PriorityLow].Threshold <= rules[PriorityNormal].Threshold {
		t.Errorf("Expected low priority threshold (%f) > normal priority threshold (%f)",
			rules[PriorityLow].Threshold, rules[PriorityNormal].Threshold)
	}
}