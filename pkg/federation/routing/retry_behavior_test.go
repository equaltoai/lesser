package routing

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
	"go.uber.org/zap/zaptest"
)

// Test Retry Behavior Without Complex Dependencies
// These tests focus on the retry logic and threshold management

func TestRetryThresholdBehavior(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	t.Run("emergency_mode_activation", func(t *testing.T) {
		tests := []struct {
			name            string
			healthyRoutes   int
			totalRoutes     int
			expectEmergency bool
			description     string
		}{
			{
				name:            "healthy_system",
				healthyRoutes:   8,
				totalRoutes:     10,
				expectEmergency: false,
				description:     "80% healthy routes should not trigger emergency mode",
			},
			{
				name:            "borderline_healthy",
				healthyRoutes:   4,
				totalRoutes:     10,
				expectEmergency: false,
				description:     "40% healthy routes should not trigger emergency mode",
			},
			{
				name:            "emergency_threshold_exact",
				healthyRoutes:   3,
				totalRoutes:     10,
				expectEmergency: false,
				description:     "Exactly 30% healthy should not trigger emergency mode",
			},
			{
				name:            "below_emergency_threshold",
				healthyRoutes:   2,
				totalRoutes:     10,
				expectEmergency: true,
				description:     "20% healthy should trigger emergency mode",
			},
			{
				name:            "critical_failure",
				healthyRoutes:   0,
				totalRoutes:     5,
				expectEmergency: true,
				description:     "0% healthy should trigger emergency mode",
			},
			{
				name:            "no_routes_edge_case",
				healthyRoutes:   0,
				totalRoutes:     0,
				expectEmergency: false,
				description:     "No routes available should not trigger emergency mode",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := manager.ShouldEnterEmergencyMode(tt.healthyRoutes, tt.totalRoutes)
				if result != tt.expectEmergency {
					t.Errorf("%s: expected emergency=%v, got %v (healthy: %d/%d)",
						tt.description, tt.expectEmergency, result, tt.healthyRoutes, tt.totalRoutes)
				}
			})
		}
	})
}

func TestRetryBackpressureRules(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	t.Run("backpressure_rule_consistency", func(t *testing.T) {
		rules := manager.GetEmergencyBackpressureRules()

		// Verify all priority levels have rules
		expectedPriorities := []MessagePriority{
			PriorityCritical,
			PriorityHigh,
			PriorityNormal,
			PriorityLow,
		}

		for _, priority := range expectedPriorities {
			rule, exists := rules[priority]
			if !exists {
				t.Errorf("Missing backpressure rule for priority %v", priority)
				continue
			}

			// Verify rule structure
			if rule.Threshold < 0 || rule.Threshold > 1 {
				t.Errorf("Priority %v threshold %f should be between 0 and 1", priority, rule.Threshold)
			}

			if rule.RateLimit <= 0 {
				t.Errorf("Priority %v rate limit should be positive, got %v", priority, rule.RateLimit)
			}

			if rule.QueueDepth <= 0 {
				t.Errorf("Priority %v queue depth should be positive, got %d", priority, rule.QueueDepth)
			}
		}

		// Critical messages should always be allowed
		criticalRule := rules[PriorityCritical]
		if criticalRule.Threshold != 0.0 {
			t.Errorf("Critical messages should have threshold 0.0 (always allowed), got %f",
				criticalRule.Threshold)
		}

		// Verify threshold progression (lower priority = higher threshold)
		if rules[PriorityLow].Threshold <= rules[PriorityNormal].Threshold {
			t.Error("Low priority should have higher threshold than normal priority")
		}

		if rules[PriorityNormal].Threshold <= rules[PriorityHigh].Threshold {
			t.Error("Normal priority should have higher threshold than high priority")
		}
	})
}

func TestRetryRouteHealthAssessment(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())
	ctx := context.Background()

	t.Run("health_assessment_thresholds", func(t *testing.T) {
		tests := []struct {
			name           string
			metrics        *types.RouteMetrics
			expectedStatus RouteHealthStatus
			expectedAction string
		}{
			{
				name: "insufficient_data",
				metrics: &types.RouteMetrics{
					TotalMessages:   5, // Below minimum threshold of 10
					SuccessfulCount: 5,
					FailedCount:     0,
					LastUpdated:     time.Now(),
				},
				expectedStatus: RouteHealthUnknown,
				expectedAction: "collect more samples",
			},
			{
				name: "preferred_route_high_success",
				metrics: &types.RouteMetrics{
					TotalMessages:   100,
					SuccessfulCount: 97, // 97% success rate
					FailedCount:     3,
					AvgLatency:      500 * time.Millisecond,
					P95Latency:      1 * time.Second,
					P99Latency:      2 * time.Second,
					LastUpdated:     time.Now(),
				},
				expectedStatus: RouteHealthPreferred,
				expectedAction: "preferred route",
			},
			{
				name: "healthy_route_good_success",
				metrics: &types.RouteMetrics{
					TotalMessages:   100,
					SuccessfulCount: 92, // 92% success rate
					FailedCount:     8,
					AvgLatency:      800 * time.Millisecond,
					P95Latency:      2 * time.Second,
					P99Latency:      4 * time.Second,
					LastUpdated:     time.Now(),
				},
				expectedStatus: RouteHealthHealthy,
				expectedAction: "healthy route",
			},
			{
				name: "monitored_route_moderate_success",
				metrics: &types.RouteMetrics{
					TotalMessages:   100,
					SuccessfulCount: 85, // 85% success rate
					FailedCount:     15,
					AvgLatency:      1200 * time.Millisecond,
					P95Latency:      3 * time.Second,
					P99Latency:      6 * time.Second,
					LastUpdated:     time.Now(),
				},
				expectedStatus: RouteHealthMonitored,
				expectedAction: "monitor closely, consider alternatives",
			},
			{
				name: "degraded_route_low_success",
				metrics: &types.RouteMetrics{
					TotalMessages:   100,
					SuccessfulCount: 65, // 65% success rate
					FailedCount:     35,
					AvgLatency:      1500 * time.Millisecond,
					P95Latency:      4 * time.Second,
					P99Latency:      8 * time.Second,
					LastUpdated:     time.Now(),
				},
				expectedStatus: RouteHealthDegraded,
				expectedAction: "reduce traffic, implement backpressure",
			},
			{
				name: "critical_route_very_low_success",
				metrics: &types.RouteMetrics{
					TotalMessages:   100,
					SuccessfulCount: 45, // 45% success rate
					FailedCount:     55,
					AvgLatency:      2000 * time.Millisecond,
					P95Latency:      6 * time.Second,
					P99Latency:      12 * time.Second,
					LastUpdated:     time.Now(),
				},
				expectedStatus: RouteHealthCritical,
				expectedAction: "open circuit immediately",
			},
			{
				name: "latency_override_degradation",
				metrics: &types.RouteMetrics{
					TotalMessages:   100,
					SuccessfulCount: 96, // High success rate
					FailedCount:     4,
					AvgLatency:      3 * time.Second,  // > 2s threshold
					P95Latency:      8 * time.Second,  // > 5s threshold
					P99Latency:      15 * time.Second, // > 10s threshold
					LastUpdated:     time.Now(),
				},
				expectedStatus: RouteHealthDegraded, // Should be degraded due to latency
				expectedAction: "reduce traffic due to latency",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assessment := manager.AssessRouteHealth(ctx, "test-route-"+tt.name, tt.metrics)

				if assessment.Status != tt.expectedStatus {
					t.Errorf("Expected status %s, got %s",
						tt.expectedStatus.String(), assessment.Status.String())
				}

				if assessment.RecommendedAction != tt.expectedAction {
					t.Errorf("Expected action '%s', got '%s'",
						tt.expectedAction, assessment.RecommendedAction)
				}

				// Verify cache TTL is appropriate
				switch tt.expectedStatus {
				case RouteHealthPreferred, RouteHealthHealthy:
					if assessment.CacheTTL != manager.config.HealthyRouteTTL {
						t.Errorf("Expected healthy TTL %v, got %v",
							manager.config.HealthyRouteTTL, assessment.CacheTTL)
					}
				case RouteHealthDegraded, RouteHealthCritical:
					if assessment.CacheTTL != manager.config.DegradedRouteTTL {
						t.Errorf("Expected degraded TTL %v, got %v",
							manager.config.DegradedRouteTTL, assessment.CacheTTL)
					}
				case RouteHealthUnknown:
					if assessment.CacheTTL != manager.config.UnknownRouteTTL {
						t.Errorf("Expected unknown TTL %v, got %v",
							manager.config.UnknownRouteTTL, assessment.CacheTTL)
					}
				}
			})
		}
	})
}

func TestRetryMessagePriorityClassification(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	t.Run("message_type_priority_mapping", func(t *testing.T) {
		tests := []struct {
			messageType      types.MessageType
			expectedPriority MessagePriority
			description      string
		}{
			{types.MessageTypeCreate, PriorityNormal, "Create messages should have normal priority"},
			{types.MessageTypeUpdate, PriorityNormal, "Update messages should have normal priority"},
			{types.MessageTypeDelete, PriorityLow, "Delete messages should have low priority"},
			{types.MessageTypeAnnounce, PriorityNormal, "Announce/boost messages should have normal priority"},
			{types.MessageTypeUndo, PriorityLow, "Undo messages should have low priority"},
			{types.MessageTypeFollow, PriorityHigh, "Follow messages should have high priority"},
			{types.MessageTypeLike, PriorityHigh, "Like messages should have high priority"},
		}

		for _, tt := range tests {
			t.Run(string(tt.messageType), func(t *testing.T) {
				priority := manager.getMessagePriority(tt.messageType)
				if priority != tt.expectedPriority {
					t.Errorf("%s: expected priority %v, got %v",
						tt.description, tt.expectedPriority, priority)
				}
			})
		}
	})
}

func TestRetryCacheTTLBehavior(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	t.Run("cache_ttl_by_message_type_and_health", func(t *testing.T) {
		tests := []struct {
			messageType    types.MessageType
			routeHealth    RouteHealthStatus
			expectShortTTL bool
			description    string
		}{
			{
				messageType:    types.MessageTypeFollow, // High priority
				routeHealth:    RouteHealthHealthy,
				expectShortTTL: false,
				description:    "High priority message with healthy route should use normal TTL",
			},
			{
				messageType:    types.MessageTypeFollow, // High priority
				routeHealth:    RouteHealthDegraded,
				expectShortTTL: true,
				description:    "High priority message with degraded route should use short TTL",
			},
			{
				messageType:    types.MessageTypeDelete, // Low priority
				routeHealth:    RouteHealthHealthy,
				expectShortTTL: false,
				description:    "Low priority message should use long TTL regardless of health",
			},
			{
				messageType:    types.MessageTypeCreate, // Normal priority
				routeHealth:    RouteHealthCritical,
				expectShortTTL: true,
				description:    "Normal priority with critical health should use short TTL",
			},
		}

		for _, tt := range tests {
			t.Run(tt.description, func(t *testing.T) {
				ttl := manager.GetCacheTTLForMessageType(tt.messageType, tt.routeHealth)

				if tt.expectShortTTL {
					if ttl > manager.config.DegradedRouteTTL {
						t.Errorf("Expected short TTL (<= %v), got %v",
							manager.config.DegradedRouteTTL, ttl)
					}
				} else {
					if ttl <= manager.config.DegradedRouteTTL {
						t.Errorf("Expected longer TTL (> %v), got %v",
							manager.config.DegradedRouteTTL, ttl)
					}
				}
			})
		}
	})
}

func TestRetryRecoveryGradualSteps(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	t.Run("recovery_step_progression", func(t *testing.T) {
		steps := manager.GetRecoverySteps()

		if len(steps) == 0 {
			t.Fatal("Recovery steps should not be empty")
		}

		// Verify gradual progression
		for i := 1; i < len(steps); i++ {
			current := steps[i]
			previous := steps[i-1]

			if current.Load <= previous.Load {
				t.Errorf("Step %d load (%f) should be greater than step %d load (%f)",
					i, current.Load, i-1, previous.Load)
			}

			// Each step should have a reasonable description
			if current.Description == "" {
				t.Errorf("Step %d should have a description", i)
			}
		}

		// First step should be reasonable starting load
		if steps[0].Load <= 0 || steps[0].Load > 0.2 {
			t.Errorf("First step load should be reasonable (0-20%%), got %f", steps[0].Load)
		}

		// Last step should be full traffic
		lastStep := steps[len(steps)-1]
		if lastStep.Load != 1.0 {
			t.Errorf("Final recovery step should be 100%% load, got %f", lastStep.Load)
		}

		// Duration should make sense (0 for final step is OK)
		for i, step := range steps[:len(steps)-1] {
			if step.Duration <= 0 {
				t.Errorf("Step %d should have positive duration, got %v", i, step.Duration)
			}
		}
	})
}

// Benchmarks for retry-related operations
func BenchmarkRetryHealthAssessment(b *testing.B) {
	logger := zaptest.NewLogger(b)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())
	ctx := context.Background()

	metrics := &types.RouteMetrics{
		TotalMessages:   100,
		SuccessfulCount: 85,
		FailedCount:     15,
		AvgLatency:      1200 * time.Millisecond,
		P95Latency:      3 * time.Second,
		P99Latency:      6 * time.Second,
		LastUpdated:     time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.AssessRouteHealth(ctx, "benchmark-route", metrics)
	}
}

func BenchmarkRetryEmergencyModeCheck(b *testing.B) {
	logger := zaptest.NewLogger(b)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		healthyRoutes := i % 10
		totalRoutes := 10
		manager.ShouldEnterEmergencyMode(healthyRoutes, totalRoutes)
	}
}

func BenchmarkRetryMessagePriorityClassification(b *testing.B) {
	logger := zaptest.NewLogger(b)
	manager := NewRouteThresholdManager(logger, DefaultThresholdConfig())

	messageTypes := []types.MessageType{
		types.MessageTypeCreate,
		types.MessageTypeUpdate,
		types.MessageTypeDelete,
		types.MessageTypeAnnounce,
		types.MessageTypeUndo,
		types.MessageTypeFollow,
		types.MessageTypeLike,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		messageType := messageTypes[i%len(messageTypes)]
		manager.getMessagePriority(messageType)
	}
}
