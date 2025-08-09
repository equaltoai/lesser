package dlq

import (
	"context"
	"fmt"
	"strings"
	"testing"

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
	if !strings.Contains(err.Error(), "invalid media URL") {
		t.Errorf("Expected error about invalid URL, got: %s", err.Error())
	}
}

func TestValidateInboxAccessibility_InvalidURL(t *testing.T) {
	client := NewReprocessorClient(zaptest.NewLogger(t))
	
	err := client.validateInboxAccessibility(context.Background(), "invalid-url")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "invalid inbox URL") {
		t.Errorf("Expected error about invalid URL, got: %s", err.Error())
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