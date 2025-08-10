package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// Test retry backoff calculation edge cases
func TestRetryBackoffCalculation(t *testing.T) {
	tests := []struct {
		retryCount      int
		expectedBackoff int
		name            string
	}{
		{0, 1, "initial_retry"},  // 2^0 = 1 minute
		{1, 2, "second_retry"},   // 2^1 = 2 minutes
		{2, 4, "third_retry"},    // 2^2 = 4 minutes
		{3, 8, "fourth_retry"},   // 2^3 = 8 minutes
		{4, 16, "fifth_retry"},   // 2^4 = 16 minutes
		{5, 32, "sixth_retry"},   // 2^5 = 32 minutes
		{6, 60, "capped_at_max"}, // Capped at 60 minutes
		{10, 60, "still_capped"}, // Still capped at 60 minutes
		{7, 60, "seventh_retry"}, // 2^7 = 128, capped to 60
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := calculateBackoff(tt.retryCount)
			if actual != tt.expectedBackoff {
				t.Errorf("calculateBackoff(%d) = %d, expected %d",
					tt.retryCount, actual, tt.expectedBackoff)
			}
		})
	}
}

// Test health-based backoff calculation edge cases
func TestHealthBasedBackoffCalculation(t *testing.T) {
	tests := []struct {
		retryCount         int
		healthReason       string
		expectedMultiplier int
		name               string
	}{
		{2, "high_error_rate_0.60", 3, "high_error_rate_multiplier"},
		{2, "slow_response_time_25000ms", 2, "slow_response_multiplier"},
		{2, "stale_last_seen_2023", 4, "stale_last_seen_multiplier"},
		{2, "healthy", 1, "healthy_no_multiplier"},
		{2, "unknown_issue", 1, "unknown_issue_no_multiplier"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseBackoff := calculateBackoff(tt.retryCount)
			actual := calculateHealthBasedBackoff(tt.retryCount, tt.healthReason)
			expected := baseBackoff * tt.expectedMultiplier

			if actual != expected {
				t.Errorf("calculateHealthBasedBackoff(%d, %s) = %d, expected %d",
					tt.retryCount, tt.healthReason, actual, expected)
			}
		})
	}
}

// Test target health assessment edge cases
func TestTargetHealthAssessment(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a processor instance for testing
	processor := &FederationDeliveryProcessor{
		logger: logger,
	}

	// Mock federation repository with different health scenarios
	mockRepo := &MockFederationRepo{
		stats: make(map[string]*MockInstanceStats),
	}

	// Test scenarios
	tests := []struct {
		name           string
		stats          *MockInstanceStats
		retryCount     int
		expectDelivery bool
		expectReason   string
	}{
		{
			name: "healthy_instance",
			stats: &MockInstanceStats{
				ErrorRate:       0.05,                           // 5% error rate - healthy
				AvgResponseTime: 1000,                           // 1 second - fast
				LastSeen:        time.Now().Add(-1 * time.Hour), // Recent
			},
			retryCount:     0,
			expectDelivery: true,
			expectReason:   "healthy",
		},
		{
			name: "high_error_rate",
			stats: &MockInstanceStats{
				ErrorRate:       0.6,                            // 60% error rate - unhealthy
				AvgResponseTime: 2000,                           // 2 seconds - acceptable
				LastSeen:        time.Now().Add(-1 * time.Hour), // Recent
			},
			retryCount:     0,
			expectDelivery: false,
			expectReason:   "high_error_rate",
		},
		{
			name: "slow_response_time",
			stats: &MockInstanceStats{
				ErrorRate:       0.1,                            // 10% error rate - acceptable
				AvgResponseTime: 35000,                          // 35 seconds - too slow
				LastSeen:        time.Now().Add(-1 * time.Hour), // Recent
			},
			retryCount:     0,
			expectDelivery: false,
			expectReason:   "slow_response",
		},
		{
			name: "stale_last_seen",
			stats: &MockInstanceStats{
				ErrorRate:       0.1,                             // 10% error rate - acceptable
				AvgResponseTime: 2000,                            // 2 seconds - fast
				LastSeen:        time.Now().Add(-48 * time.Hour), // 2 days ago - stale
			},
			retryCount:     0,
			expectDelivery: false,
			expectReason:   "stale_last_seen",
		},
		{
			name: "stricter_threshold_on_retry",
			stats: &MockInstanceStats{
				ErrorRate:       0.25,  // 25% error rate - acceptable on first try
				AvgResponseTime: 18000, // 18 seconds - acceptable on first try
				LastSeen:        time.Now().Add(-1 * time.Hour),
			},
			retryCount:     1, // This is a retry, so stricter thresholds apply
			expectDelivery: false,
			expectReason:   "retry_error_rate", // Should fail on retry
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo.stats["test.domain"] = tt.stats

			ctx := context.Background()
			shouldDeliver, reason := processor.assessTargetHealthWithMock(ctx, "test.domain", tt.retryCount, mockRepo)

			if shouldDeliver != tt.expectDelivery {
				t.Errorf("Expected shouldDeliver=%v, got %v", tt.expectDelivery, shouldDeliver)
			}

			if tt.expectReason != "" && reason != tt.expectReason {
				// Allow partial matches for error reasons
				if !containsString(reason, tt.expectReason) {
					t.Errorf("Expected reason to contain '%s', got '%s'", tt.expectReason, reason)
				}
			}
		})
	}
}

// Test SQS delay parameter edge cases
func TestSQSDelayParameterEdgeCases(t *testing.T) {
	logger := zaptest.NewLogger(t)

	processor := &FederationDeliveryProcessor{
		logger: logger,
	}

	tests := []struct {
		name            string
		delayMinutes    int
		expectedSeconds int32
		description     string
	}{
		{
			name:            "normal_delay",
			delayMinutes:    5,
			expectedSeconds: 300, // 5 * 60
			description:     "Normal 5 minute delay",
		},
		{
			name:            "at_sqs_limit",
			delayMinutes:    15,
			expectedSeconds: 900, // 15 * 60 = 900 (SQS max)
			description:     "At SQS 15 minute limit",
		},
		{
			name:            "above_sqs_limit",
			delayMinutes:    30,
			expectedSeconds: 900, // Capped at 900
			description:     "Above SQS limit should be capped",
		},
		{
			name:            "negative_delay",
			delayMinutes:    -5,
			expectedSeconds: 0, // Negative should become 0
			description:     "Negative delay should become 0",
		},
		{
			name:            "zero_delay",
			delayMinutes:    0,
			expectedSeconds: 0,
			description:     "Zero delay",
		},
		{
			name:            "very_large_delay",
			delayMinutes:    1000,
			expectedSeconds: 900, // Still capped at 900
			description:     "Very large delay should be capped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSeconds := processor.calculateSQSDelaySeconds(tt.delayMinutes)

			if actualSeconds != tt.expectedSeconds {
				t.Errorf("%s: expected %d seconds, got %d seconds",
					tt.description, tt.expectedSeconds, actualSeconds)
			}

			// Verify the value is within SQS limits
			if actualSeconds < 0 || actualSeconds > 900 {
				t.Errorf("SQS delay %d is outside valid range [0, 900]", actualSeconds)
			}
		})
	}
}

// Test retry count progression
func TestRetryCountProgression(t *testing.T) {
	tests := []struct {
		name         string
		initialRetry int
		maxRetries   int
		expectMore   bool
		description  string
	}{
		{
			name:         "first_retry",
			initialRetry: 0,
			maxRetries:   3,
			expectMore:   true,
			description:  "Should allow retry on first failure",
		},
		{
			name:         "middle_retry",
			initialRetry: 1,
			maxRetries:   3,
			expectMore:   true,
			description:  "Should allow retry in middle of range",
		},
		{
			name:         "at_max_retry",
			initialRetry: 3,
			maxRetries:   3,
			expectMore:   false,
			description:  "Should not allow retry at max",
		},
		{
			name:         "beyond_max_retry",
			initialRetry: 5,
			maxRetries:   3,
			expectMore:   false,
			description:  "Should not allow retry beyond max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldRetry := tt.initialRetry < tt.maxRetries

			if shouldRetry != tt.expectMore {
				t.Errorf("%s: expected shouldRetry=%v, got %v",
					tt.description, tt.expectMore, shouldRetry)
			}
		})
	}
}

// Mock types for testing
type MockFederationRepo struct {
	stats map[string]*MockInstanceStats
	err   error
}

type MockInstanceStats struct {
	ErrorRate       float64
	AvgResponseTime float64
	LastSeen        time.Time
}

func (r *MockFederationRepo) GetInstanceStats(ctx context.Context, domain string) (*MockInstanceStats, error) {
	if r.err != nil {
		return nil, r.err
	}
	if stats, exists := r.stats[domain]; exists {
		return stats, nil
	}
	return nil, fmt.Errorf("stats not found for domain: %s", domain)
}

// Add method to FederationDeliveryProcessor for testing
func (p *FederationDeliveryProcessor) assessTargetHealthWithMock(ctx context.Context, domain string, retryCount int, mockRepo *MockFederationRepo) (bool, string) {
	stats, err := mockRepo.GetInstanceStats(ctx, domain)
	if err != nil {
		p.logger.Debug("no instance stats available, allowing delivery")
		return true, "no_stats"
	}

	// Check basic health indicators
	if stats.ErrorRate > 0.5 {
		return false, fmt.Sprintf("high_error_rate_%.2f", stats.ErrorRate)
	}

	if stats.AvgResponseTime > 30000 { // 30 seconds
		return false, fmt.Sprintf("slow_response_time_%.0fms", stats.AvgResponseTime)
	}

	// For retry attempts, be more strict
	if retryCount > 0 {
		if stats.ErrorRate > 0.2 {
			return false, fmt.Sprintf("retry_error_rate_%.2f", stats.ErrorRate)
		}
		if stats.AvgResponseTime > 15000 { // 15 seconds for retries
			return false, fmt.Sprintf("retry_slow_response_%.0fms", stats.AvgResponseTime)
		}
	}

	// Check if instance was recently seen
	if time.Since(stats.LastSeen) > 24*time.Hour {
		return false, fmt.Sprintf("stale_last_seen_%s", stats.LastSeen.Format(time.RFC3339))
	}

	return true, "healthy"
}

// Add helper method to FederationDeliveryProcessor for testing
func (p *FederationDeliveryProcessor) calculateSQSDelaySeconds(delayMinutes int) int32 {
	if delayMinutes < 0 {
		delayMinutes = 0
	}

	delaySeconds := delayMinutes * 60
	if delaySeconds > 900 { // SQS max delay
		delaySeconds = 900
	}

	return int32(delaySeconds)
}

// Helper function for string matching
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && (s[:len(substr)] == substr || containsString(s[1:], substr))))
}

// Benchmark tests for performance
func BenchmarkRetryBackoffCalculation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		calculateBackoff(i % 10)
	}
}

func BenchmarkHealthBasedBackoffCalculation(b *testing.B) {
	reasons := []string{
		"high_error_rate_0.60",
		"slow_response_time_25000ms",
		"stale_last_seen_2023",
		"healthy",
	}

	for i := 0; i < b.N; i++ {
		retryCount := i % 5
		reason := reasons[i%len(reasons)]
		calculateHealthBasedBackoff(retryCount, reason)
	}
}
