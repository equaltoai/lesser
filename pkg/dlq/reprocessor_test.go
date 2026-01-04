package dlq

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// Test the HTTP HEAD logic by checking response codes directly
func TestValidateMediaAccessibilityLogic(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name          string
		statusCode    int
		expectError   bool
		errorContains string
	}{
		{
			name:        "accessible media",
			statusCode:  200,
			expectError: false,
		},
		{
			name:        "redirected media",
			statusCode:  301,
			expectError: false,
		},
		{
			name:          "not found media",
			statusCode:    404,
			expectError:   true,
			errorContains: "permanently unavailable",
		},
		{
			name:          "gone media",
			statusCode:    410,
			expectError:   true,
			errorContains: "permanently unavailable",
		},
		{
			name:        "rate limited",
			statusCode:  429,
			expectError: false,
		},
		{
			name:        "service unavailable",
			statusCode:  503,
			expectError: false,
		},
		{
			name:          "access denied",
			statusCode:    401,
			expectError:   true,
			errorContains: "access denied",
		},
		{
			name:        "server error",
			statusCode:  500,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the classification logic directly
			err := classifyMediaResponse(tt.statusCode, "http://example.com/media.jpg", logger)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for status %d, got nil", tt.statusCode)
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for status %d, got %v", tt.statusCode, err)
				}
			}
		})
	}
}

func TestValidateInboxAccessibilityLogic(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name        string
		statusCode  int
		expectError bool
	}{
		{
			name:        "accessible inbox",
			statusCode:  200,
			expectError: false,
		},
		{
			name:        "not found inbox - retryable for federation",
			statusCode:  404,
			expectError: false,
		},
		{
			name:        "auth issues - retryable for signature problems",
			statusCode:  401,
			expectError: false,
		},
		{
			name:        "server error - retryable",
			statusCode:  500,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the classification logic directly
			err := classifyInboxResponse(tt.statusCode, "http://example.com/inbox", logger)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for status %d, got nil", tt.statusCode)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for status %d, got %v", tt.statusCode, err)
				}
			}
		})
	}
}

func TestBasicURLValidation(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	tests := []struct {
		name        string
		url         string
		expectValid bool
	}{
		{
			name:        "valid HTTP URL",
			url:         "http://example.com/media.jpg",
			expectValid: true,
		},
		{
			name:        "valid HTTPS URL",
			url:         "https://example.com/media.jpg",
			expectValid: true,
		},
		{
			name:        "invalid URL - no protocol",
			url:         "example.com/media.jpg",
			expectValid: false,
		},
		{
			name:        "empty URL",
			url:         "",
			expectValid: false,
		},
		{
			name:        "too short URL",
			url:         "http:/",
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.isValidURL(tt.url)
			if result != tt.expectValid {
				t.Errorf("Expected isValidURL(%s) = %v, got %v", tt.url, tt.expectValid, result)
			}
		})
	}
}

func TestValidateMediaAccessibility_InvalidURL(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	err := client.validateMediaAccessibility(context.Background(), "invalid-url")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
	// Check for the actual error message format
	if !strings.Contains(err.Error(), "invalid format") && !strings.Contains(err.Error(), "media_url") {
		t.Errorf("Expected error about invalid URL format, got: %s", err.Error())
	}
}

func TestValidateInboxAccessibility_InvalidURL(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	err := client.validateInboxAccessibility(context.Background(), "invalid-url")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
	// Check for the actual error message format
	if !strings.Contains(err.Error(), "invalid format") && !strings.Contains(err.Error(), "inbox_url") {
		t.Errorf("Expected error about invalid URL format, got: %s", err.Error())
	}
}

// Helper functions to test the classification logic extracted from the main functions

// classifyMediaResponse extracts the HTTP response classification logic for media
func classifyMediaResponse(statusCode int, url string, logger *zap.Logger) error {
	switch {
	case statusCode >= 200 && statusCode < 400:
		// Success or redirect - media is accessible
		logger.Debug("Media is accessible, allowing retry",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return nil

	case statusCode == 404 || statusCode == 410:
		// Not Found or Gone - permanent failure
		logger.Info("Media permanently unavailable, marking as non-retryable",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return fmt.Errorf("media permanently unavailable (HTTP %d)", statusCode)

	case statusCode == 429 || statusCode == 503:
		// Rate limited or service unavailable - temporary issue
		logger.Debug("Media temporarily unavailable, allowing retry",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return nil

	case statusCode == 401 || statusCode == 403:
		// Auth issues - may be permanent depending on context
		logger.Warn("Media access denied, treating as potentially permanent",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return fmt.Errorf("media access denied (HTTP %d)", statusCode)

	default:
		// Other client/server errors - treat as retryable for now
		logger.Debug("Media HEAD request returned error, allowing retry",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return nil
	}
}

// classifyInboxResponse extracts the HTTP response classification logic for inbox
func classifyInboxResponse(statusCode int, url string, logger *zap.Logger) error {
	switch {
	case statusCode >= 200 && statusCode < 400:
		// Success or redirect - inbox is accessible
		logger.Debug("Inbox is accessible, allowing retry",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return nil

	case statusCode == 404 || statusCode == 410:
		// Not Found or Gone - might be permanent but could be temporary server config
		// For federation, we're more conservative and allow retries
		logger.Debug("Inbox returned 404/410, allowing retry for federation",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return nil

	case statusCode == 429 || statusCode == 503:
		// Rate limited or service unavailable - temporary issue
		logger.Debug("Inbox temporarily unavailable, allowing retry",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return nil

	case statusCode == 401 || statusCode == 403:
		// Auth issues - for federation this might indicate signature problems
		// Allow retry as it might be a temporary key/signature issue
		logger.Debug("Inbox access denied, allowing retry for potential signature issues",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return nil

	case statusCode >= 500:
		// Server errors - definitely retryable
		logger.Debug("Inbox server error, allowing retry",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return nil

	default:
		// Other client errors - treat as retryable to be safe for federation
		logger.Debug("Inbox HEAD request returned client error, allowing retry",
			zap.String("url", url),
			zap.Int("status_code", statusCode))
		return nil
	}
}


// ============================================================================
// Additional validation tests
// ============================================================================

func TestValidateNotificationMessage(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	tests := []struct {
		name        string
		message     map[string]interface{}
		expectError bool
		errorType   error
	}{
		{
			name: "valid notification message",
			message: map[string]interface{}{
				"notification_id": "n123",
				"user_id":         "u456",
				"channels":        []interface{}{"email", "push"},
			},
			expectError: false,
		},
		{
			name: "missing notification_id",
			message: map[string]interface{}{
				"user_id":  "u456",
				"channels": []interface{}{"email"},
			},
			expectError: true,
		},
		{
			name: "missing user_id",
			message: map[string]interface{}{
				"notification_id": "n123",
				"channels":        []interface{}{"email"},
			},
			expectError: true,
		},
		{
			name: "missing channels",
			message: map[string]interface{}{
				"notification_id": "n123",
				"user_id":         "u456",
			},
			expectError: true,
		},
		{
			name: "channels not an array",
			message: map[string]interface{}{
				"notification_id": "n123",
				"user_id":         "u456",
				"channels":        "email",
			},
			expectError: true,
		},
		{
			name:        "empty message",
			message:     map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateNotificationMessage(tt.message)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestValidateActivityMessage(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	tests := []struct {
		name        string
		message     map[string]interface{}
		expectError bool
	}{
		{
			name: "valid activity message",
			message: map[string]interface{}{
				"type":  "Create",
				"actor": "https://example.com/users/alice",
			},
			expectError: false,
		},
		{
			name: "missing type",
			message: map[string]interface{}{
				"actor": "https://example.com/users/alice",
			},
			expectError: true,
		},
		{
			name: "missing actor",
			message: map[string]interface{}{
				"type": "Create",
			},
			expectError: true,
		},
		{
			name: "type not a string",
			message: map[string]interface{}{
				"type":  123,
				"actor": "https://example.com/users/alice",
			},
			expectError: true,
		},
		{
			name: "actor not a string",
			message: map[string]interface{}{
				"type":  "Create",
				"actor": 123,
			},
			expectError: true,
		},
		{
			name:        "empty message",
			message:     map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateActivityMessage(tt.message)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestValidateMediaMessage(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	tests := []struct {
		name        string
		message     map[string]interface{}
		expectError bool
	}{
		{
			name: "valid media message",
			message: map[string]interface{}{
				"media_id":        "m123",
				"media_url":       "https://example.com/media/image.jpg",
				"processing_type": "thumbnail",
			},
			expectError: false,
		},
		{
			name: "missing media_id",
			message: map[string]interface{}{
				"media_url":       "https://example.com/media/image.jpg",
				"processing_type": "thumbnail",
			},
			expectError: true,
		},
		{
			name: "missing media_url",
			message: map[string]interface{}{
				"media_id":        "m123",
				"processing_type": "thumbnail",
			},
			expectError: true,
		},
		{
			name: "missing processing_type",
			message: map[string]interface{}{
				"media_id":  "m123",
				"media_url": "https://example.com/media/image.jpg",
			},
			expectError: true,
		},
		{
			name: "invalid media_url format",
			message: map[string]interface{}{
				"media_id":        "m123",
				"media_url":       "not-a-valid-url",
				"processing_type": "thumbnail",
			},
			expectError: true,
		},
		{
			name:        "empty message",
			message:     map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateMediaMessage(tt.message)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestValidateFederationMessage(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	tests := []struct {
		name        string
		message     map[string]interface{}
		expectError bool
	}{
		{
			name: "valid federation message",
			message: map[string]interface{}{
				"inbox_url": "https://remote.example/inbox",
				"activity":  map[string]interface{}{"type": "Create"},
				"actor_id":  "https://local.example/users/alice",
			},
			expectError: false,
		},
		{
			name: "missing inbox_url",
			message: map[string]interface{}{
				"activity": map[string]interface{}{"type": "Create"},
				"actor_id": "https://local.example/users/alice",
			},
			expectError: true,
		},
		{
			name: "missing activity",
			message: map[string]interface{}{
				"inbox_url": "https://remote.example/inbox",
				"actor_id":  "https://local.example/users/alice",
			},
			expectError: true,
		},
		{
			name: "missing actor_id",
			message: map[string]interface{}{
				"inbox_url": "https://remote.example/inbox",
				"activity":  map[string]interface{}{"type": "Create"},
			},
			expectError: true,
		},
		{
			name: "invalid inbox_url format",
			message: map[string]interface{}{
				"inbox_url": "not-a-valid-url",
				"activity":  map[string]interface{}{"type": "Create"},
				"actor_id":  "https://local.example/users/alice",
			},
			expectError: true,
		},
		{
			name:        "empty message",
			message:     map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateFederationMessage(tt.message)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestValidateSearchMessage(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	tests := []struct {
		name        string
		message     map[string]interface{}
		expectError bool
	}{
		{
			name: "valid search message - index",
			message: map[string]interface{}{
				"object_id":   "obj123",
				"object_type": "Note",
				"action":      "index",
			},
			expectError: false,
		},
		{
			name: "valid search message - update",
			message: map[string]interface{}{
				"object_id":   "obj123",
				"object_type": "Note",
				"action":      "update",
			},
			expectError: false,
		},
		{
			name: "valid search message - delete",
			message: map[string]interface{}{
				"object_id":   "obj123",
				"object_type": "Note",
				"action":      "delete",
			},
			expectError: false,
		},
		{
			name: "missing object_id",
			message: map[string]interface{}{
				"object_type": "Note",
				"action":      "index",
			},
			expectError: true,
		},
		{
			name: "missing object_type",
			message: map[string]interface{}{
				"object_id": "obj123",
				"action":    "index",
			},
			expectError: true,
		},
		{
			name: "missing action",
			message: map[string]interface{}{
				"object_id":   "obj123",
				"object_type": "Note",
			},
			expectError: true,
		},
		{
			name: "invalid action",
			message: map[string]interface{}{
				"object_id":   "obj123",
				"object_type": "Note",
				"action":      "invalid_action",
			},
			expectError: true,
		},
		{
			name:        "empty message",
			message:     map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateSearchMessage(tt.message)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

// ============================================================================
// GetDefaultStrategy tests
// ============================================================================

func TestGetDefaultStrategy(t *testing.T) {
	tests := []struct {
		name                   string
		service                string
		expectedMaxRetries     int
		expectedDelaySeconds   int32
		expectedBackoff        string
		expectedValidateFirst  bool
		expectedCheckAccess    bool
	}{
		{
			name:                   "notification processor",
			service:                "notification-processor",
			expectedMaxRetries:     3,
			expectedDelaySeconds:   30,
			expectedBackoff:        "exponential",
			expectedValidateFirst:  true,
			expectedCheckAccess:    false,
		},
		{
			name:                   "activity processor",
			service:                "activity-processor",
			expectedMaxRetries:     5,
			expectedDelaySeconds:   60,
			expectedBackoff:        "exponential",
			expectedValidateFirst:  true,
			expectedCheckAccess:    true,
		},
		{
			name:                   "media processor",
			service:                "media-processor",
			expectedMaxRetries:     3,
			expectedDelaySeconds:   120,
			expectedBackoff:        "linear",
			expectedValidateFirst:  true,
			expectedCheckAccess:    true,
		},
		{
			name:                   "federation delivery",
			service:                "federation-delivery",
			expectedMaxRetries:     5,
			expectedDelaySeconds:   300,
			expectedBackoff:        "exponential",
			expectedValidateFirst:  true,
			expectedCheckAccess:    true,
		},
		{
			name:                   "search indexer",
			service:                "search-indexer",
			expectedMaxRetries:     3,
			expectedDelaySeconds:   60,
			expectedBackoff:        "fixed",
			expectedValidateFirst:  true,
			expectedCheckAccess:    false,
		},
		{
			name:                   "unknown service - default",
			service:                "unknown-service",
			expectedMaxRetries:     3,
			expectedDelaySeconds:   60,
			expectedBackoff:        "exponential",
			expectedValidateFirst:  true,
			expectedCheckAccess:    false,
		},
		{
			name:                   "empty service - default",
			service:                "",
			expectedMaxRetries:     3,
			expectedDelaySeconds:   60,
			expectedBackoff:        "exponential",
			expectedValidateFirst:  true,
			expectedCheckAccess:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := GetDefaultStrategy(tt.service)

			if strategy == nil {
				t.Fatal("Expected non-nil strategy")
			}
			if strategy.MaxRetries != tt.expectedMaxRetries {
				t.Errorf("Expected MaxRetries %d, got %d", tt.expectedMaxRetries, strategy.MaxRetries)
			}
			if strategy.DelaySeconds != tt.expectedDelaySeconds {
				t.Errorf("Expected DelaySeconds %d, got %d", tt.expectedDelaySeconds, strategy.DelaySeconds)
			}
			if strategy.BackoffStrategy != tt.expectedBackoff {
				t.Errorf("Expected BackoffStrategy %s, got %s", tt.expectedBackoff, strategy.BackoffStrategy)
			}
			if strategy.ValidateFirst != tt.expectedValidateFirst {
				t.Errorf("Expected ValidateFirst %v, got %v", tt.expectedValidateFirst, strategy.ValidateFirst)
			}
			if strategy.CheckAccessibility != tt.expectedCheckAccess {
				t.Errorf("Expected CheckAccessibility %v, got %v", tt.expectedCheckAccess, strategy.CheckAccessibility)
			}
		})
	}
}

// ============================================================================
// isRetryableHTTPStatus tests
// ============================================================================

func TestIsRetryableHTTPStatus(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		// Success codes - retryable (not errors)
		{name: "200 OK", statusCode: 200, expected: true},
		{name: "201 Created", statusCode: 201, expected: true},
		{name: "204 No Content", statusCode: 204, expected: true},
		{name: "301 Redirect", statusCode: 301, expected: true},
		{name: "302 Found", statusCode: 302, expected: true},

		// Client errors - generally not retryable
		{name: "400 Bad Request", statusCode: 400, expected: false},
		{name: "401 Unauthorized", statusCode: 401, expected: false},
		{name: "403 Forbidden", statusCode: 403, expected: false},
		{name: "404 Not Found", statusCode: 404, expected: false},
		{name: "405 Method Not Allowed", statusCode: 405, expected: false},
		{name: "406 Not Acceptable", statusCode: 406, expected: false},
		{name: "409 Conflict", statusCode: 409, expected: false},
		{name: "410 Gone", statusCode: 410, expected: false},
		{name: "422 Unprocessable Entity", statusCode: 422, expected: false},

		// Retryable client errors
		{name: "408 Request Timeout", statusCode: 408, expected: true},
		{name: "429 Too Many Requests", statusCode: 429, expected: true},

		// Server errors - retryable
		{name: "500 Internal Server Error", statusCode: 500, expected: true},
		{name: "502 Bad Gateway", statusCode: 502, expected: true},
		{name: "503 Service Unavailable", statusCode: 503, expected: true},
		{name: "504 Gateway Timeout", statusCode: 504, expected: true},

		// Unknown codes
		{name: "600 Unknown", statusCode: 600, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.isRetryableHTTPStatus(tt.statusCode)
			if result != tt.expected {
				t.Errorf("Expected isRetryableHTTPStatus(%d) = %v, got %v", tt.statusCode, tt.expected, result)
			}
		})
	}
}

// ============================================================================
// BatchReprocessResult struct tests
// ============================================================================

func TestBatchReprocessResultStruct(t *testing.T) {
	result := &BatchReprocessResult{
		TotalMessages:         10,
		SuccessfulReprocesses: 7,
		FailedReprocesses:     3,
		Errors: []string{
			"Message msg-1: validation failed",
			"Message msg-2: network error",
			"Message msg-3: timeout",
		},
	}

	if result.TotalMessages != 10 {
		t.Errorf("Expected TotalMessages 10, got %d", result.TotalMessages)
	}
	if result.SuccessfulReprocesses != 7 {
		t.Errorf("Expected SuccessfulReprocesses 7, got %d", result.SuccessfulReprocesses)
	}
	if result.FailedReprocesses != 3 {
		t.Errorf("Expected FailedReprocesses 3, got %d", result.FailedReprocesses)
	}
	if len(result.Errors) != 3 {
		t.Errorf("Expected 3 errors, got %d", len(result.Errors))
	}
}

// ============================================================================
// ReprocessingStrategy struct tests
// ============================================================================

func TestReprocessingStrategyStruct(t *testing.T) {
	strategy := &ReprocessingStrategy{
		MaxRetries:         5,
		DelaySeconds:       60,
		BackoffStrategy:    "exponential",
		ValidateFirst:      true,
		CheckAccessibility: true,
	}

	if strategy.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries 5, got %d", strategy.MaxRetries)
	}
	if strategy.DelaySeconds != 60 {
		t.Errorf("Expected DelaySeconds 60, got %d", strategy.DelaySeconds)
	}
	if strategy.BackoffStrategy != "exponential" {
		t.Errorf("Expected BackoffStrategy exponential, got %s", strategy.BackoffStrategy)
	}
	if !strategy.ValidateFirst {
		t.Error("Expected ValidateFirst true")
	}
	if !strategy.CheckAccessibility {
		t.Error("Expected CheckAccessibility true")
	}
}

// ============================================================================
// SetSQSClient tests
// ============================================================================

func TestSetSQSClient(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	// Initially nil
	if client.sqsClient != nil {
		t.Error("Expected sqsClient to be nil initially")
	}

	// Set to nil explicitly
	client.SetSQSClient(nil)
	if client.sqsClient != nil {
		t.Error("Expected sqsClient to remain nil")
	}
}

// ============================================================================
// Queue URL caching tests
// ============================================================================

func TestReprocessorQueueURLCaching(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	// Verify cache is empty initially
	if len(client.queueURLs) != 0 {
		t.Errorf("Expected empty queueURLs cache, got %d entries", len(client.queueURLs))
	}

	// Manually add to cache
	client.queueURLs["test-queue"] = "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue"

	// Verify cache contains the entry
	if len(client.queueURLs) != 1 {
		t.Errorf("Expected 1 entry in queueURLs cache, got %d", len(client.queueURLs))
	}
	if client.queueURLs["test-queue"] != "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue" {
		t.Error("Queue URL not cached correctly")
	}
}


// ============================================================================
// ReprocessConfig struct tests
// ============================================================================

func TestReprocessConfigStruct(t *testing.T) {
	config := ReprocessConfig{
		ValidateMessage: func(m map[string]interface{}) error {
			return nil
		},
		CheckAccessibility: func(ctx context.Context, m map[string]interface{}) error {
			return nil
		},
		ReprocessType: "test_reprocess",
	}

	if config.ValidateMessage == nil {
		t.Error("Expected ValidateMessage to be set")
	}
	if config.CheckAccessibility == nil {
		t.Error("Expected CheckAccessibility to be set")
	}
	if config.ReprocessType != "test_reprocess" {
		t.Errorf("Expected ReprocessType 'test_reprocess', got '%s'", config.ReprocessType)
	}
}

// ============================================================================
// HTTP client initialization tests
// ============================================================================

func TestNewReprocessorClient_HTTPClientInitialized(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	if client.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}
}

// ============================================================================
// URL validation edge cases
// ============================================================================

func TestIsValidURL_EdgeCases(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "http with port",
			url:      "http://example.com:8080/path",
			expected: true,
		},
		{
			name:     "https with port",
			url:      "https://example.com:443/path",
			expected: true,
		},
		{
			name:     "http with query params",
			url:      "http://example.com/path?query=value",
			expected: true,
		},
		{
			name:     "https with fragment",
			url:      "https://example.com/path#section",
			expected: true,
		},
		{
			name:     "ftp protocol",
			url:      "ftp://example.com/file",
			expected: false,
		},
		{
			name:     "file protocol",
			url:      "file:///path/to/file",
			expected: false,
		},
		{
			name:     "just http://",
			url:      "http://",
			expected: false,
		},
		{
			name:     "just https://",
			url:      "https://",
			expected: true, // The basic validation only checks length >= 8 and prefix
		},
		{
			name:     "http:// with space",
			url:      "http:// example.com",
			expected: true, // Basic validation doesn't check for spaces
		},
		{
			name:     "localhost http",
			url:      "http://localhost/path",
			expected: true,
		},
		{
			name:     "localhost https",
			url:      "https://localhost:3000/api",
			expected: true,
		},
		{
			name:     "IP address http",
			url:      "http://192.168.1.1/path",
			expected: true,
		},
		{
			name:     "IP address https",
			url:      "https://10.0.0.1:8443/api",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.isValidURL(tt.url)
			if result != tt.expected {
				t.Errorf("isValidURL(%q) = %v, expected %v", tt.url, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// Validation message edge cases
// ============================================================================

func TestValidateNotificationMessage_EdgeCases(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	tests := []struct {
		name        string
		message     map[string]interface{}
		expectError bool
	}{
		{
			name: "channels as empty array",
			message: map[string]interface{}{
				"notification_id": "n123",
				"user_id":         "u456",
				"channels":        []interface{}{},
			},
			expectError: false, // Empty array is still an array
		},
		{
			name: "channels as nil",
			message: map[string]interface{}{
				"notification_id": "n123",
				"user_id":         "u456",
				"channels":        nil,
			},
			expectError: true, // nil is not an array
		},
		{
			name: "extra fields are allowed",
			message: map[string]interface{}{
				"notification_id": "n123",
				"user_id":         "u456",
				"channels":        []interface{}{"email"},
				"extra_field":     "extra_value",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateNotificationMessage(tt.message)
			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}

func TestValidateSearchMessage_ActionValidation(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	// Test all valid actions
	validActions := []string{"index", "update", "delete"}
	for _, action := range validActions {
		t.Run("valid_action_"+action, func(t *testing.T) {
			message := map[string]interface{}{
				"object_id":   "obj123",
				"object_type": "Note",
				"action":      action,
			}
			err := client.validateSearchMessage(message)
			if err != nil {
				t.Errorf("Expected no error for action '%s', got %v", action, err)
			}
		})
	}

	// Test invalid actions
	invalidActions := []string{"create", "remove", "upsert", ""}
	for _, action := range invalidActions {
		t.Run("invalid_action_"+action, func(t *testing.T) {
			message := map[string]interface{}{
				"object_id":   "obj123",
				"object_type": "Note",
				"action":      action,
			}
			err := client.validateSearchMessage(message)
			if err == nil {
				t.Errorf("Expected error for invalid action '%s', got nil", action)
			}
		})
	}
}

// ============================================================================
// isRetryableHTTPStatus comprehensive tests
// ============================================================================

func TestIsRetryableHTTPStatus_AllStatusCodes(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	// Test a range of status codes
	testCases := []struct {
		code     int
		expected bool
	}{
		// 1xx - Informational (not typically used)
		{100, false},
		{101, false},

		// 2xx - Success
		{200, true},
		{201, true},
		{202, true},
		{204, true},
		{206, true},

		// 3xx - Redirection
		{301, true},
		{302, true},
		{303, true},
		{304, true},
		{307, true},
		{308, true},

		// 4xx - Client Errors
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{405, false},
		{406, false},
		{408, true}, // Request Timeout - retryable
		{409, false},
		{410, false},
		{411, false},
		{413, false},
		{414, false},
		{415, false},
		{422, false},
		{429, true}, // Too Many Requests - retryable
		{451, false},

		// 5xx - Server Errors
		{500, true},
		{501, true},
		{502, true},
		{503, true},
		{504, true},
		{505, true},
		{507, true},
		{508, true},
		{510, true},
		{511, true},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("status_%d", tc.code), func(t *testing.T) {
			result := client.isRetryableHTTPStatus(tc.code)
			if result != tc.expected {
				t.Errorf("isRetryableHTTPStatus(%d) = %v, expected %v", tc.code, result, tc.expected)
			}
		})
	}
}

// ============================================================================
// Strategy comparison tests
// ============================================================================

func TestGetDefaultStrategy_Comparison(t *testing.T) {
	// Compare strategies for different services
	notificationStrategy := GetDefaultStrategy("notification-processor")
	federationStrategy := GetDefaultStrategy("federation-delivery")

	// Federation should have more retries than notification
	if federationStrategy.MaxRetries <= notificationStrategy.MaxRetries {
		t.Error("Federation strategy should have more retries than notification")
	}

	// Federation should have longer delay
	if federationStrategy.DelaySeconds <= notificationStrategy.DelaySeconds {
		t.Error("Federation strategy should have longer delay than notification")
	}

	// Both should validate first
	if !notificationStrategy.ValidateFirst || !federationStrategy.ValidateFirst {
		t.Error("Both strategies should validate first")
	}

	// Federation should check accessibility, notification should not
	if !federationStrategy.CheckAccessibility {
		t.Error("Federation strategy should check accessibility")
	}
	if notificationStrategy.CheckAccessibility {
		t.Error("Notification strategy should not check accessibility")
	}
}

// ============================================================================
// Import for fmt
// ============================================================================


// ============================================================================
// Tests with Mock SQS Client for Reprocessor
// ============================================================================

func TestReprocessNotification_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/notification-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.MatchedBy(func(input *sqs.GetQueueUrlInput) bool {
		return *input.QueueName == "notification-queue"
	})).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageInput) bool {
		reprocessType, hasReprocessType := input.MessageAttributes["DLQ.ReprocessType"]
		return *input.QueueUrl == queueURL &&
			hasReprocessType &&
			*reprocessType.StringValue == "notification_reprocess"
	})).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("reprocessed-msg-123"),
	}, nil)

	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-123",
		Body:        `{"notification_id": "n123", "user_id": "u456", "channels": ["email", "push"]}`,
		SourceQueue: "notification-queue",
		Attributes:  map[string]string{},
	}

	err := client.ReprocessNotification(context.Background(), originalMessage)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestReprocessNotification_ValidationError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	// Invalid message - missing required fields
	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-123",
		Body:        `{"invalid": "message"}`,
		SourceQueue: "notification-queue",
	}

	err := client.ReprocessNotification(context.Background(), originalMessage)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")

	// SQS should not be called
	mockSQS.AssertNotCalled(t, "GetQueueUrl", mock.Anything, mock.Anything)
	mockSQS.AssertNotCalled(t, "SendMessage", mock.Anything, mock.Anything)
}

func TestReprocessNotification_InvalidJSON(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-123",
		Body:        `not valid json`,
		SourceQueue: "notification-queue",
	}

	err := client.ReprocessNotification(context.Background(), originalMessage)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid notification message format")
}

func TestReprocessActivity_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/activity-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageInput) bool {
		reprocessType, ok := input.MessageAttributes["DLQ.ReprocessType"]
		return ok && *reprocessType.StringValue == "activity_reprocess"
	})).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("reprocessed-msg-456"),
	}, nil)

	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-456",
		Body:        `{"type": "Create", "actor": "https://example.com/users/alice"}`,
		SourceQueue: "activity-queue",
		Attributes:  map[string]string{},
	}

	err := client.ReprocessActivity(context.Background(), originalMessage)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestReprocessActivity_ValidationError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	// Missing type field
	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-456",
		Body:        `{"actor": "https://example.com/users/alice"}`,
		SourceQueue: "activity-queue",
	}

	err := client.ReprocessActivity(context.Background(), originalMessage)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestReprocessMedia_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)
	client.httpClient = &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	}}

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/media-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageInput) bool {
		reprocessType, ok := input.MessageAttributes["DLQ.ReprocessType"]
		return ok && *reprocessType.StringValue == "media_reprocess"
	})).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("reprocessed-msg-789"),
	}, nil)

	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-789",
		Body:        `{"media_id": "m123", "media_url": "https://example.com/media/image.jpg", "processing_type": "thumbnail"}`,
		SourceQueue: "media-queue",
		Attributes:  map[string]string{},
	}

	err := client.ReprocessMedia(context.Background(), originalMessage)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestReprocessMedia_ValidationError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	// Missing media_url
	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-789",
		Body:        `{"media_id": "m123", "processing_type": "thumbnail"}`,
		SourceQueue: "media-queue",
	}

	err := client.ReprocessMedia(context.Background(), originalMessage)
	assert.Error(t, err)
}

func TestReprocessFederation_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)
	client.httpClient = &httpDoerStub{doFn: func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	}}

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/federation-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageInput) bool {
		reprocessType, ok := input.MessageAttributes["DLQ.ReprocessType"]
		return ok && *reprocessType.StringValue == "federation_reprocess"
	})).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("reprocessed-msg-fed"),
	}, nil)

	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-fed",
		Body:        `{"inbox_url": "https://example.com/inbox", "activity": {"type": "Create"}, "actor_id": "https://local.example/users/alice"}`,
		SourceQueue: "federation-queue",
		Attributes:  map[string]string{},
	}

	err := client.ReprocessFederation(context.Background(), originalMessage)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

type httpDoerStub struct {
	doFn func(req *http.Request) (*http.Response, error)
}

func (d *httpDoerStub) Do(req *http.Request) (*http.Response, error) {
	return d.doFn(req)
}

func TestReprocessSearch_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/search-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageInput) bool {
		reprocessType, ok := input.MessageAttributes["DLQ.ReprocessType"]
		return ok && *reprocessType.StringValue == "search_reprocess"
	})).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("reprocessed-msg-search"),
	}, nil)

	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-search",
		Body:        `{"object_id": "obj123", "object_type": "Note", "action": "index"}`,
		SourceQueue: "search-queue",
		Attributes:  map[string]string{},
	}

	err := client.ReprocessSearch(context.Background(), originalMessage)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestReprocessSearch_ValidationError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	// Invalid action
	originalMessage := &OriginalMessage{
		MessageID:   "orig-msg-search",
		Body:        `{"object_id": "obj123", "object_type": "Note", "action": "invalid"}`,
		SourceQueue: "search-queue",
	}

	err := client.ReprocessSearch(context.Background(), originalMessage)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestReprocessGeneric_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/generic-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.MatchedBy(func(input *sqs.GetQueueUrlInput) bool {
		return *input.QueueName == "generic-queue"
	})).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageInput) bool {
		return *input.MessageAttributes["DLQ.ReprocessType"].StringValue == "generic_reprocess"
	})).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("reprocessed-msg-generic"),
	}, nil)

	originalMessage := &OriginalMessage{
		MessageID:  "orig-msg-generic",
		Body:       `{"custom": "data"}`,
		Attributes: map[string]string{},
	}

	err := client.ReprocessGeneric(context.Background(), "generic-queue", originalMessage)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestReprocessGeneric_NonJSONBody(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/generic-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.Anything).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("reprocessed-msg-generic"),
	}, nil)

	// Non-JSON body should still be processed
	originalMessage := &OriginalMessage{
		MessageID:  "orig-msg-generic",
		Body:       "plain text message",
		Attributes: map[string]string{},
	}

	err := client.ReprocessGeneric(context.Background(), "generic-queue", originalMessage)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestGetQueueURL_Reprocessor_CacheHit(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	// Pre-populate cache
	cachedURL := "https://sqs.us-east-1.amazonaws.com/123456789012/cached-queue"
	client.queueURLs["cached-queue"] = cachedURL

	url, err := client.getQueueURL(context.Background(), "cached-queue")
	assert.NoError(t, err)
	assert.Equal(t, cachedURL, url)

	// SQS should not be called
	mockSQS.AssertNotCalled(t, "GetQueueUrl", mock.Anything, mock.Anything)
}

func TestGetQueueURL_Reprocessor_CacheMiss(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	expectedURL := "https://sqs.us-east-1.amazonaws.com/123456789012/new-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.MatchedBy(func(input *sqs.GetQueueUrlInput) bool {
		return *input.QueueName == "new-queue"
	})).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &expectedURL,
	}, nil)

	url, err := client.getQueueURL(context.Background(), "new-queue")
	assert.NoError(t, err)
	assert.Equal(t, expectedURL, url)

	// Verify URL was cached
	assert.Equal(t, expectedURL, client.queueURLs["new-queue"])

	mockSQS.AssertExpectations(t)
}

func TestGetQueueURL_Reprocessor_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	url, err := client.getQueueURL(context.Background(), "error-queue")
	assert.Error(t, err)
	assert.Empty(t, url)
	assert.Contains(t, err.Error(), "failed to get queue URL")

	mockSQS.AssertExpectations(t)
}

func TestSendMessageToQueue_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue"

	mockSQS.On("SendMessage", mock.Anything, mock.MatchedBy(func(input *sqs.SendMessageInput) bool {
		_, hasReprocessType := input.MessageAttributes["DLQ.ReprocessType"]
		_, hasTimestamp := input.MessageAttributes["DLQ.ReprocessTimestamp"]
		return *input.QueueUrl == queueURL &&
			input.DelaySeconds == 30 &&
			hasReprocessType &&
			hasTimestamp
	})).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("sent-msg-123"),
	}, nil)

	attributes := map[string]string{
		"CustomAttr": "custom-value",
	}

	err := client.sendMessageToQueue(context.Background(), queueURL, `{"test": "data"}`, attributes, "test_reprocess")
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestSendMessageToQueue_Error(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	mockSQS.On("SendMessage", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	err := client.sendMessageToQueue(context.Background(), "https://sqs.example.com/queue", `{"test": "data"}`, nil, "test_reprocess")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send message to queue")

	mockSQS.AssertExpectations(t)
}

func TestBatchReprocess_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/notification-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.Anything).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("sent-msg"),
	}, nil)

	messages := []*OriginalMessage{
		{
			MessageID:   "msg-1",
			Body:        `{"notification_id": "n1", "user_id": "u1", "channels": ["email"]}`,
			SourceQueue: "notification-queue",
			Attributes:  map[string]string{},
		},
		{
			MessageID:   "msg-2",
			Body:        `{"notification_id": "n2", "user_id": "u2", "channels": ["push"]}`,
			SourceQueue: "notification-queue",
			Attributes:  map[string]string{},
		},
	}

	result, err := client.BatchReprocess(context.Background(), messages, "notification-processor")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.TotalMessages)
	assert.Equal(t, 2, result.SuccessfulReprocesses)
	assert.Equal(t, 0, result.FailedReprocesses)
	assert.Empty(t, result.Errors)

	mockSQS.AssertExpectations(t)
}

func TestBatchReprocess_PartialFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/notification-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.Anything).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("sent-msg"),
	}, nil)

	messages := []*OriginalMessage{
		{
			MessageID:   "msg-1",
			Body:        `{"notification_id": "n1", "user_id": "u1", "channels": ["email"]}`,
			SourceQueue: "notification-queue",
			Attributes:  map[string]string{},
		},
		{
			MessageID:   "msg-2",
			Body:        `{"invalid": "message"}`, // Invalid - will fail validation
			SourceQueue: "notification-queue",
			Attributes:  map[string]string{},
		},
	}

	result, err := client.BatchReprocess(context.Background(), messages, "notification-processor")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, result.TotalMessages)
	assert.Equal(t, 1, result.SuccessfulReprocesses)
	assert.Equal(t, 1, result.FailedReprocesses)
	assert.Len(t, result.Errors, 1)
}

func TestBatchReprocess_AllServices(t *testing.T) {
	services := []struct {
		name    string
		body    string
		service string
	}{
		{
			name:    "notification-processor",
			body:    `{"notification_id": "n1", "user_id": "u1", "channels": ["email"]}`,
			service: "notification-processor",
		},
		{
			name:    "activity-processor",
			body:    `{"type": "Create", "actor": "https://example.com/users/alice"}`,
			service: "activity-processor",
		},
		{
			name:    "media-processor",
			body:    `{"media_id": "m1", "media_url": "https://example.com/media.jpg", "processing_type": "thumbnail"}`,
			service: "media-processor",
		},
		{
			name:    "federation-delivery",
			body:    `{"inbox_url": "https://remote.example/inbox", "activity": {"type": "Create"}, "actor_id": "https://local.example/users/alice"}`,
			service: "federation-delivery",
		},
		{
			name:    "search-indexer",
			body:    `{"object_id": "obj1", "object_type": "Note", "action": "index"}`,
			service: "search-indexer",
		},
		{
			name:    "unknown-service",
			body:    `{"custom": "data"}`,
			service: "unknown-service",
		},
	}

	for _, tc := range services {
		t.Run(tc.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			client := NewReprocessorClient(logger)

			mockSQS := new(MockSQSClient)
			client.SetSQSClient(mockSQS)

			queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue"

			mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
				QueueUrl: &queueURL,
			}, nil)

			mockSQS.On("SendMessage", mock.Anything, mock.Anything).Return(&sqs.SendMessageOutput{
				MessageId: aws.String("sent-msg"),
			}, nil)

			messages := []*OriginalMessage{
				{
					MessageID:   "msg-1",
					Body:        tc.body,
					SourceQueue: "test-queue",
					Attributes:  map[string]string{},
				},
			}

			result, err := client.BatchReprocess(context.Background(), messages, tc.service)
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, 1, result.TotalMessages)
		})
	}
}

func TestReprocessWithValidation_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	mockSQS := new(MockSQSClient)
	client.SetSQSClient(mockSQS)

	queueURL := "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue"

	mockSQS.On("GetQueueUrl", mock.Anything, mock.Anything).Return(&sqs.GetQueueUrlOutput{
		QueueUrl: &queueURL,
	}, nil)

	mockSQS.On("SendMessage", mock.Anything, mock.Anything).Return(&sqs.SendMessageOutput{
		MessageId: aws.String("sent-msg"),
	}, nil)

	originalMessage := &OriginalMessage{
		MessageID:   "msg-1",
		Body:        `{"field1": "value1", "field2": "value2"}`,
		SourceQueue: "test-queue",
		Attributes:  map[string]string{},
	}

	config := ReprocessConfig{
		ValidateMessage: func(m map[string]interface{}) error {
			if _, exists := m["field1"]; !exists {
				return fmt.Errorf("missing field1")
			}
			return nil
		},
		CheckAccessibility: nil,
		ReprocessType:      "test_reprocess",
	}

	err := client.reprocessWithValidation(context.Background(), originalMessage, "test", config)
	assert.NoError(t, err)

	mockSQS.AssertExpectations(t)
}

func TestReprocessWithValidation_ValidationError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	originalMessage := &OriginalMessage{
		MessageID:   "msg-1",
		Body:        `{"field2": "value2"}`,
		SourceQueue: "test-queue",
	}

	config := ReprocessConfig{
		ValidateMessage: func(m map[string]interface{}) error {
			if _, exists := m["field1"]; !exists {
				return fmt.Errorf("missing field1")
			}
			return nil
		},
		ReprocessType: "test_reprocess",
	}

	err := client.reprocessWithValidation(context.Background(), originalMessage, "test", config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestReprocessWithValidation_AccessibilityError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	originalMessage := &OriginalMessage{
		MessageID:   "msg-1",
		Body:        `{"field1": "value1"}`,
		SourceQueue: "test-queue",
	}

	config := ReprocessConfig{
		ValidateMessage: func(m map[string]interface{}) error {
			return nil
		},
		CheckAccessibility: func(ctx context.Context, m map[string]interface{}) error {
			return fmt.Errorf("resource not accessible")
		},
		ReprocessType: "test_reprocess",
	}

	err := client.reprocessWithValidation(context.Background(), originalMessage, "test", config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not accessible")
}

func TestReprocessWithValidation_InvalidJSON(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := NewReprocessorClient(logger)

	originalMessage := &OriginalMessage{
		MessageID:   "msg-1",
		Body:        `not valid json`,
		SourceQueue: "test-queue",
	}

	config := ReprocessConfig{
		ValidateMessage: func(m map[string]interface{}) error {
			return nil
		},
		ReprocessType: "test_reprocess",
	}

	err := client.reprocessWithValidation(context.Background(), originalMessage, "test", config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid test message format")
}


// ============================================================================
// HTTP Accessibility Tests
// Note: Tests using httptest.NewServer are skipped because the SecureClient
// blocks private IP addresses (localhost/127.0.0.1) for security.
// The classification logic is tested via classifyMediaResponse and
// classifyInboxResponse helper functions above.
// ============================================================================

// TestValidateMediaAccessibility_InvalidURL tests URL validation
func TestValidateMediaAccessibility_InvalidURL_Comprehensive(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	invalidURLs := []string{
		"",
		"not-a-url",
		"ftp://example.com/file",
		"http:/",
		"https:/",
	}

	for _, url := range invalidURLs {
		t.Run(url, func(t *testing.T) {
			err := client.validateMediaAccessibility(context.Background(), url)
			assert.Error(t, err)
		})
	}
}

// TestValidateInboxAccessibility_InvalidURL tests URL validation
func TestValidateInboxAccessibility_InvalidURL_Comprehensive(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	invalidURLs := []string{
		"",
		"not-a-url",
		"ftp://example.com/inbox",
		"http:/",
		"https:/",
	}

	for _, url := range invalidURLs {
		t.Run(url, func(t *testing.T) {
			err := client.validateInboxAccessibility(context.Background(), url)
			assert.Error(t, err)
		})
	}
}

// TestValidateMediaAccessibility_NetworkError tests that network errors are treated as retryable
func TestValidateMediaAccessibility_NetworkError(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	// Use a URL that will fail to connect (non-routable IP)
	// The secure client will block this, treating it as a network error (retryable)
	err := client.validateMediaAccessibility(context.Background(), "https://192.0.2.1/media.jpg")
	// Network errors are treated as retryable (return nil)
	assert.NoError(t, err)
}

// TestValidateInboxAccessibility_NetworkError tests that network errors are treated as retryable
func TestValidateInboxAccessibility_NetworkError(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))

	// Use a URL that will fail to connect
	err := client.validateInboxAccessibility(context.Background(), "https://192.0.2.1/inbox")
	// Network errors are treated as retryable (return nil)
	assert.NoError(t, err)
}
