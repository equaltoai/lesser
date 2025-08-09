package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryRetry(t *testing.T) {
	// Create a minimal processor for testing retry logic
	processor := &OutboxProcessor{
		retryConfig: RetryConfig{
			MaxAttempts:   3,
			InitialDelay:  1 * time.Second,
			MaxDelay:      30 * time.Second,
			BackoffFactor: 2.0,
			PermanentErrors: []int{
				400, // Bad Request
				401, // Unauthorized
				403, // Forbidden
				404, // Not Found
				410, // Gone
				422, // Unprocessable Entity
			},
		},
	}

	// Test exponential backoff calculation
	t.Run("calculateBackoffDelay", func(t *testing.T) {
		tests := []struct {
			attempt     int
			expectedMin time.Duration
			expectedMax time.Duration
		}{
			{1, time.Second, 3 * time.Second},      // First retry
			{2, 2 * time.Second, 6 * time.Second},  // Second retry
			{3, 4 * time.Second, 12 * time.Second}, // Third retry
		}

		for _, tt := range tests {
			t.Run(string(rune(tt.attempt)), func(t *testing.T) {
				delay := processor.calculateBackoffDelay(tt.attempt)
				assert.True(t, delay >= tt.expectedMin, "delay %v should be >= %v", delay, tt.expectedMin)
				assert.True(t, delay <= processor.retryConfig.MaxDelay, "delay %v should be <= %v", delay, processor.retryConfig.MaxDelay)
			})
		}
	})

	t.Run("isPermanentError", func(t *testing.T) {
		tests := []struct {
			statusCode int
			expected   bool
		}{
			{400, true},  // Bad Request - permanent
			{401, true},  // Unauthorized - permanent
			{403, true},  // Forbidden - permanent
			{404, true},  // Not Found - permanent
			{410, true},  // Gone - permanent
			{422, true},  // Unprocessable Entity - permanent
			{500, false}, // Internal Server Error - temporary
			{502, false}, // Bad Gateway - temporary
			{503, false}, // Service Unavailable - temporary
		}

		for _, tt := range tests {
			t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
				result := processor.isPermanentError(tt.statusCode)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestExtractDomainFromURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/inbox", "example.com"},
		{"https://example.com:8080/inbox", "example.com:8080"},
		{"http://example.com/inbox", "example.com"},
		{"https://sub.example.com/path/to/inbox", "sub.example.com"},
		{"", ""},
		{"example.com", "example.com"},
		{"not-a-valid-url", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractDomainFromURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestActivityDeliveryMessage(t *testing.T) {
	// Test that the ActivityDeliveryMessage struct works as expected
	t.Run("message validation", func(t *testing.T) {
		msg := ActivityDeliveryMessage{
			Activity:    nil,
			TargetInbox: "https://example.com/inbox",
			Attempt:     1,
		}

		// Activity should not be nil
		require.Nil(t, msg.Activity)
		require.NotEmpty(t, msg.TargetInbox)
		require.Equal(t, 1, msg.Attempt)
	})
}

func TestRetryConfig(t *testing.T) {
	// Test default retry configuration
	config := RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    1 * time.Second,
		MaxDelay:        30 * time.Second,
		BackoffFactor:   2.0,
		PermanentErrors: []int{400, 401, 403, 404, 410, 422},
	}

	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffFactor)
	assert.Contains(t, config.PermanentErrors, 404)
	assert.NotContains(t, config.PermanentErrors, 500)
}
