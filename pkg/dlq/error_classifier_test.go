package dlq

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 1) Pattern initialization tests
// ============================================================================

func TestNewErrorClassifier(t *testing.T) {
	classifier := NewErrorClassifier()

	require.NotNil(t, classifier, "NewErrorClassifier should return a non-nil classifier")

	patterns := classifier.GetPatterns()
	require.NotNil(t, patterns, "GetPatterns should return a non-nil map")
	require.NotEmpty(t, patterns, "GetPatterns should contain default patterns")

	// Verify key default patterns exist
	expectedPatterns := []string{
		"validation_error",
		"network_error",
		"timeout_error",
		"auth_error",
		"not_found_error",
		"rate_limit_error",
		"database_error",
		"serialization_error",
		"federation_error",
	}

	for _, patternName := range expectedPatterns {
		_, exists := patterns[patternName]
		assert.True(t, exists, "Expected pattern '%s' to exist in default patterns", patternName)
	}
}

func TestGetPatterns_ReturnsExpectedStructure(t *testing.T) {
	classifier := NewErrorClassifier()
	patterns := classifier.GetPatterns()

	// Check validation_error pattern structure
	validationPattern, exists := patterns["validation_error"]
	require.True(t, exists, "validation_error pattern should exist")
	assert.Equal(t, "validation_error", validationPattern.ErrorType)
	assert.True(t, validationPattern.IsPermanent, "validation_error should be permanent")
	assert.Equal(t, "medium", validationPattern.Priority)
	assert.NotEmpty(t, validationPattern.Patterns, "validation_error should have patterns")
	assert.Contains(t, validationPattern.Patterns, "validation failed")

	// Check network_error pattern structure
	networkPattern, exists := patterns["network_error"]
	require.True(t, exists, "network_error pattern should exist")
	assert.Equal(t, "network_error", networkPattern.ErrorType)
	assert.False(t, networkPattern.IsPermanent, "network_error should be transient")
	assert.Equal(t, "high", networkPattern.Priority)

	// Check timeout_error pattern structure
	timeoutPattern, exists := patterns["timeout_error"]
	require.True(t, exists, "timeout_error pattern should exist")
	assert.Equal(t, "timeout_error", timeoutPattern.ErrorType)
	assert.False(t, timeoutPattern.IsPermanent, "timeout_error should be transient")
}

// ============================================================================
// 2) JSON extraction path tests
// ============================================================================

func TestClassifyError_JSONExtraction(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name             string
		jsonInput        map[string]interface{}
		expectedMessage  string
		expectedStack    string
		expectedCategory string
	}{
		{
			name: "errorMessage field extraction",
			jsonInput: map[string]interface{}{
				"errorMessage": "validation failed: missing field",
			},
			expectedMessage: "validation failed: missing field",
		},
		{
			name: "error field extraction",
			jsonInput: map[string]interface{}{
				"error": "network connection timeout",
			},
			expectedMessage: "network connection timeout",
		},
		{
			name: "message field extraction",
			jsonInput: map[string]interface{}{
				"message": "invalid input data",
			},
			expectedMessage: "invalid input data",
		},
		{
			name: "stackTrace as string",
			jsonInput: map[string]interface{}{
				"errorMessage": "something failed",
				"stackTrace":   "at main.go:10\nat handler.go:25",
			},
			expectedMessage: "something failed",
			expectedStack:   "at main.go:10\nat handler.go:25",
		},
		{
			name: "stackTrace as array joined with newline",
			jsonInput: map[string]interface{}{
				"errorMessage": "validation failed: missing field",
				"errorType":    "TypeError",
				"stackTrace":   []interface{}{"line1", "line2", "line3"},
			},
			expectedMessage:  "validation failed: missing field",
			expectedStack:    "line1\nline2\nline3",
			expectedCategory: "TypeError",
		},
		{
			name: "errorType mapped to Category",
			jsonInput: map[string]interface{}{
				"errorMessage": "test error",
				"errorType":    "ValidationError",
			},
			expectedMessage:  "test error",
			expectedCategory: "ValidationError",
		},
		{
			name: "multiple fields with priority to errorMessage",
			jsonInput: map[string]interface{}{
				"errorMessage": "primary error message",
				"error":        "secondary error",
				"message":      "tertiary message",
			},
			expectedMessage: "primary error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.jsonInput)
			require.NoError(t, err)

			result := classifier.ClassifyError(string(jsonBytes), "")

			require.NotNil(t, result)
			if tt.expectedMessage != "" {
				assert.Equal(t, tt.expectedMessage, result.ErrorMessage)
			}
			if tt.expectedStack != "" {
				assert.Equal(t, tt.expectedStack, result.StackTrace)
			}
			if tt.expectedCategory != "" {
				assert.Equal(t, tt.expectedCategory, result.Category)
			}
		})
	}
}

func TestClassifyError_JSONValidationErrorClassification(t *testing.T) {
	classifier := NewErrorClassifier()

	jsonInput := map[string]interface{}{
		"errorMessage": "validation failed: missing field",
		"errorType":    "TypeError",
		"stackTrace":   []interface{}{"line1", "line2"},
	}
	jsonBytes, err := json.Marshal(jsonInput)
	require.NoError(t, err)

	result := classifier.ClassifyError(string(jsonBytes), "")

	assert.Equal(t, "validation_error", result.ErrorType)
	assert.True(t, result.IsPermanent)
	assert.Equal(t, "line1\nline2", result.StackTrace)
}

// ============================================================================
// 3) Plain-text extraction tests (stack parsing)
// ============================================================================

func TestClassifyError_PlainTextExtraction(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name                 string
		textInput            string
		expectedMessage      string
		expectedStackContent string
		stackShouldBeEmpty   bool
	}{
		{
			name: "text with 'at ...' stack trace",
			textInput: `Error occurred while processing
at main.go:10 (func main())
at handler.go:25 (func Handle())`,
			expectedMessage:      "Error occurred while processing",
			expectedStackContent: "at main.go:10",
		},
		{
			name: "text with '.go:' stack trace pattern",
			textInput: `Unexpected error in handler
at runtime/debug.Stack()
at main.go:42 (handleRequest)
at handler.go:88 (serve)`,
			expectedMessage:      "Unexpected error in handler",
			expectedStackContent: "main.go:42",
		},
		{
			name:               "text without stack patterns - empty stack",
			textInput:          "Simple error message without any stack trace",
			expectedMessage:    "Simple error message without any stack trace",
			stackShouldBeEmpty: true,
		},
		{
			name: "combined 'at' and '.go:' patterns",
			textInput: `Connection timeout
at service.go:100 (Connect)
at main.go:50 (main)`,
			expectedMessage:      "Connection timeout",
			expectedStackContent: "at service.go:100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyError(tt.textInput, "")

			require.NotNil(t, result)

			// For messages with stack traces, the error portion is extracted
			if tt.expectedMessage != "" {
				assert.Contains(t, result.ErrorMessage, tt.expectedMessage)
			}

			if tt.stackShouldBeEmpty {
				assert.Empty(t, result.StackTrace, "Expected empty stack trace")
			} else if tt.expectedStackContent != "" {
				assert.Contains(t, result.StackTrace, tt.expectedStackContent)
			}
		})
	}
}

// ============================================================================
// 4) Pattern scoring / best match tests
// ============================================================================

func TestClassifyError_PatternScoring(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name              string
		errorMessage      string
		expectedErrorType string
		description       string
	}{
		{
			name:              "matches multiple patterns - best match wins by cumulative score",
			errorMessage:      "validation failed with malformed data",
			expectedErrorType: "validation_error",
			description:       "validation_error wins with 'validation failed' (17) + 'malformed' (9) = 26 total vs other patterns",
		},
		{
			name:              "unknown error defaults to processing_error",
			errorMessage:      "qwerty asdfgh zxcvbn",
			expectedErrorType: "processing_error",
			description:       "should default to processing_error when no pattern matches",
		},
		{
			name:              "timeout pattern matches",
			errorMessage:      "operation timed out after 30 seconds",
			expectedErrorType: "timeout_error",
			description:       "should match timeout_error pattern",
		},
		{
			name:              "network pattern matches",
			errorMessage:      "connection refused to remote host",
			expectedErrorType: "network_error",
			description:       "should match network_error pattern via 'connection'",
		},
		{
			name:              "rate limit pattern matches",
			errorMessage:      "rate limit exceeded: too many requests",
			expectedErrorType: "rate_limit_error",
			description:       "should match rate_limit_error pattern",
		},
		{
			name:              "longer pattern wins",
			errorMessage:      "context deadline exceeded during database query",
			expectedErrorType: "timeout_error",
			description:       "'context deadline' from timeout_error should score higher",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyError(tt.errorMessage, "")

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedErrorType, result.ErrorType, tt.description)
		})
	}
}

// ============================================================================
// 5) Service-specific overrides tests
// ============================================================================

func TestClassifyError_NotificationProcessor(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name             string
		errorMessage     string
		expectedType     string
		expectedPerm     bool
		expectedPriority string
		expectedReason   string
	}{
		{
			name:             "user not found",
			errorMessage:     "user not found: actor123",
			expectedType:     "user_not_found",
			expectedPerm:     true,
			expectedPriority: "low",
			expectedReason:   "Target user no longer exists",
		},
		{
			name:             "invalid user",
			errorMessage:     "invalid user reference in notification",
			expectedType:     "user_not_found",
			expectedPerm:     true,
			expectedPriority: "low",
		},
		{
			name:             "invalid email",
			errorMessage:     "email address invalid for delivery",
			expectedType:     "invalid_email",
			expectedPerm:     true,
			expectedPriority: "medium",
			expectedReason:   "Invalid email address for notification delivery",
		},
		{
			name:             "push subscription expired endpoint",
			errorMessage:     "push endpoint is no longer valid",
			expectedType:     "push_subscription_error",
			expectedPerm:     true,
			expectedPriority: "low",
			expectedReason:   "Push subscription is invalid or expired",
		},
		{
			name:             "push subscription error",
			errorMessage:     "push subscription expired",
			expectedType:     "push_subscription_error",
			expectedPerm:     true,
			expectedPriority: "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyError(tt.errorMessage, processorNotification)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.ErrorType)
			assert.Equal(t, tt.expectedPerm, result.IsPermanent)
			assert.Equal(t, tt.expectedPriority, result.Priority)
			if tt.expectedReason != "" {
				assert.Equal(t, tt.expectedReason, result.FailureReason)
			}
		})
	}
}

func TestClassifyError_ActivityProcessor(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name             string
		errorMessage     string
		expectedType     string
		expectedPerm     bool
		expectedPriority string
		expectedReason   string
	}{
		{
			name:             "signature verification failed",
			errorMessage:     "HTTP signature verification failed",
			expectedType:     "signature_verification_error",
			expectedPerm:     true,
			expectedPriority: "high",
			expectedReason:   "ActivityPub signature verification failed",
		},
		{
			name:             "actor not found",
			errorMessage:     "remote actor not found at https://example.com/users/someone",
			expectedType:     "actor_not_found",
			expectedPerm:     false, // Actor might come back online
			expectedPriority: "medium",
			expectedReason:   "ActivityPub actor not accessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyError(tt.errorMessage, processorActivity)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.ErrorType)
			assert.Equal(t, tt.expectedPerm, result.IsPermanent)
			assert.Equal(t, tt.expectedPriority, result.Priority)
			if tt.expectedReason != "" {
				assert.Equal(t, tt.expectedReason, result.FailureReason)
			}
		})
	}
}

func TestClassifyError_MediaProcessor(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name             string
		errorMessage     string
		expectedType     string
		expectedPerm     bool
		expectedPriority string
		expectedReason   string
	}{
		{
			name:             "format unsupported",
			errorMessage:     "media format unsupported: audio/midi",
			expectedType:     "unsupported_media_format",
			expectedPerm:     true,
			expectedPriority: "low",
			expectedReason:   "Media format not supported for processing",
		},
		{
			name:             "invalid format",
			errorMessage:     "invalid format for transcoding",
			expectedType:     "unsupported_media_format",
			expectedPerm:     true,
			expectedPriority: "low",
		},
		{
			name:             "size too large",
			errorMessage:     "file size too large for processing",
			expectedType:     "media_too_large",
			expectedPerm:     true,
			expectedPriority: "low",
			expectedReason:   "Media file exceeds size limits",
		},
		{
			name:             "download failed",
			errorMessage:     "failed to download media from origin",
			expectedType:     "media_fetch_error",
			expectedPerm:     false,
			expectedPriority: "medium",
			expectedReason:   "Failed to download media file",
		},
		{
			name:             "fetch failed",
			errorMessage:     "could not fetch resource from URL",
			expectedType:     "media_fetch_error",
			expectedPerm:     false,
			expectedPriority: "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyError(tt.errorMessage, processorMedia)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.ErrorType)
			assert.Equal(t, tt.expectedPerm, result.IsPermanent)
			assert.Equal(t, tt.expectedPriority, result.Priority)
			if tt.expectedReason != "" {
				assert.Equal(t, tt.expectedReason, result.FailureReason)
			}
		})
	}
}

func TestClassifyError_FederationDeliveryProcessor(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name             string
		errorMessage     string
		expectedType     string
		expectedPerm     bool
		expectedPriority string
		expectedReason   string
	}{
		{
			name:             "webfinger error",
			errorMessage:     "webfinger lookup failed for account",
			expectedType:     "webfinger_error",
			expectedPerm:     false,
			expectedPriority: "medium",
			expectedReason:   "WebFinger discovery failed",
		},
		{
			name:             "inbox unreachable",
			errorMessage:     "remote inbox unreachable after multiple attempts",
			expectedType:     "inbox_unreachable",
			expectedPerm:     false,
			expectedPriority: "high",
			expectedReason:   "Remote inbox is unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyError(tt.errorMessage, processorFederationDelivery)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.ErrorType)
			assert.Equal(t, tt.expectedPerm, result.IsPermanent)
			assert.Equal(t, tt.expectedPriority, result.Priority)
			if tt.expectedReason != "" {
				assert.Equal(t, tt.expectedReason, result.FailureReason)
			}
		})
	}
}

func TestClassifyError_SearchIndexerProcessor(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name             string
		errorMessage     string
		expectedType     string
		expectedPerm     bool
		expectedPriority string
		expectedReason   string
	}{
		{
			name:             "index full error",
			errorMessage:     "search index full, cannot add more documents",
			expectedType:     "index_full_error",
			expectedPerm:     false,
			expectedPriority: "critical",
			expectedReason:   "Search index capacity exceeded",
		},
		{
			name:             "embedding error",
			errorMessage:     "failed to generate embedding for text",
			expectedType:     "embedding_error",
			expectedPerm:     false,
			expectedPriority: "medium",
			expectedReason:   "Failed to generate text embeddings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyError(tt.errorMessage, processorSearchIndexer)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.ErrorType)
			assert.Equal(t, tt.expectedPerm, result.IsPermanent)
			assert.Equal(t, tt.expectedPriority, result.Priority)
			if tt.expectedReason != "" {
				assert.Equal(t, tt.expectedReason, result.FailureReason)
			}
		})
	}
}

// ============================================================================
// 6) Trend analysis tests
// ============================================================================

func TestAnalyzeErrorTrends(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name                      string
		messages                  []string
		expectedTotal             int
		expectedPermanent         int
		expectedTransient         int
		expectedPermanentRate     float64
		expectedTransientRate     float64
		checkErrorTypes           map[string]int
		checkPriorityBreakdown    bool
		expectedPriorityBreakdown map[string]int
	}{
		{
			name: "mixed permanent and transient errors",
			messages: []string{
				"validation failed: missing required field",   // permanent - validation_error
				"context deadline exceeded during processing", // transient - timeout_error
				"authentication failed: invalid token",        // permanent - auth_error
				"rate limit exceeded: too many requests",      // transient - rate_limit_error
				"dns lookup failed for remote host",           // transient - network_error
				`{"errorMessage": "json unmarshal error"}`,    // permanent - serialization_error
			},
			expectedTotal:         6,
			expectedPermanent:     3,
			expectedTransient:     3,
			expectedPermanentRate: 50.0,
			expectedTransientRate: 50.0,
			checkErrorTypes: map[string]int{
				"validation_error":    1,
				"network_error":       1,
				"auth_error":          1,
				"rate_limit_error":    1,
				"timeout_error":       1,
				"serialization_error": 1,
			},
		},
		{
			name:                  "empty messages",
			messages:              []string{},
			expectedTotal:         0,
			expectedPermanent:     0,
			expectedTransient:     0,
			expectedPermanentRate: 0,
			expectedTransientRate: 0,
		},
		{
			name: "all permanent errors",
			messages: []string{
				"validation failed",
				"not found: resource does not exist",
				"authentication failed",
			},
			expectedTotal:         3,
			expectedPermanent:     3,
			expectedTransient:     0,
			expectedPermanentRate: 100.0,
			expectedTransientRate: 0,
		},
		{
			name: "all transient errors",
			messages: []string{
				"connection timeout",
				"rate limit exceeded",
				"service unavailable - 503",
			},
			expectedTotal:         3,
			expectedPermanent:     0,
			expectedTransient:     3,
			expectedPermanentRate: 0,
			expectedTransientRate: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := classifier.AnalyzeErrorTrends(tt.messages)

			require.NotNil(t, analysis)
			assert.Equal(t, tt.expectedTotal, analysis.TotalMessages)
			assert.Equal(t, tt.expectedPermanent, analysis.PermanentErrors)
			assert.Equal(t, tt.expectedTransient, analysis.TransientErrors)
			assert.Equal(t, tt.expectedPermanentRate, analysis.PermanentErrorRate)
			assert.Equal(t, tt.expectedTransientRate, analysis.TransientErrorRate)

			if tt.checkErrorTypes != nil {
				for errorType, count := range tt.checkErrorTypes {
					assert.Equal(t, count, analysis.ErrorTypeCounts[errorType],
						"Expected %d occurrences of %s, got %d",
						count, errorType, analysis.ErrorTypeCounts[errorType])
				}
			}
		})
	}
}

func TestAnalyzeErrorTrends_PriorityBreakdown(t *testing.T) {
	classifier := NewErrorClassifier()

	messages := []string{
		"validation failed",   // medium priority
		"connection timeout",  // medium priority (timeout_error)
		"out of memory error", // critical priority
		"unauthorized access", // high priority
	}

	analysis := classifier.AnalyzeErrorTrends(messages)

	require.NotNil(t, analysis)
	require.NotNil(t, analysis.PriorityBreakdown)

	// The priorities should be tracked
	assert.True(t, len(analysis.PriorityBreakdown) > 0, "PriorityBreakdown should have entries")
}

// ============================================================================
// 7) AppError creation and mapping tests
// ============================================================================

func TestCreateAppError_ErrorCodeAndCategoryMapping(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name             string
		errorMessage     string
		service          string
		expectedCode     errors.ErrorCode
		expectedCategory errors.ErrorCategory
	}{
		{
			name:             "validation_error mapping",
			errorMessage:     "validation failed: missing field",
			service:          "",
			expectedCode:     errors.CodeValidationFailed,
			expectedCategory: errors.CategoryValidation,
		},
		{
			name:             "auth_error mapping",
			errorMessage:     "authentication failed: invalid token",
			service:          "",
			expectedCode:     errors.CodeAuthFailed,
			expectedCategory: errors.CategoryAuth,
		},
		{
			name:             "network_error mapping",
			errorMessage:     "connection refused",
			service:          "",
			expectedCode:     errors.CodeExternalServiceTimeout,
			expectedCategory: errors.CategoryExternal,
		},
		{
			name:             "rate_limit_error mapping",
			errorMessage:     "rate limit exceeded",
			service:          "",
			expectedCode:     errors.CodeRateLimited,
			expectedCategory: errors.CategoryAPI,
		},
		{
			name:             "database_error mapping",
			errorMessage:     "database connection pool exhausted",
			service:          "",
			expectedCode:     errors.CodeDatabaseConnection,
			expectedCategory: errors.CategoryStorage,
		},
		{
			name:             "serialization_error mapping",
			errorMessage:     "json unmarshal failed",
			service:          "",
			expectedCode:     errors.CodeEventProcessingFailed,
			expectedCategory: errors.CategoryLambda,
		},
		{
			name:             "federation_error mapping",
			errorMessage:     "activitypub delivery failed",
			service:          "",
			expectedCode:     errors.CodeDeliveryFailed,
			expectedCategory: errors.CategoryFederation,
		},
		{
			name:             "service-specific: signature_verification_error",
			errorMessage:     "signature verification failed",
			service:          processorActivity,
			expectedCode:     errors.CodeSignatureVerifyFailed,
			expectedCategory: errors.CategoryFederation,
		},
		{
			name:             "service-specific: unsupported_media_format",
			errorMessage:     "format unsupported",
			service:          processorMedia,
			expectedCode:     errors.CodeUnsupportedMediaType,
			expectedCategory: errors.CategoryMedia,
		},
		{
			name:             "default to SQS processing failed",
			errorMessage:     "xyzwq random gibberish abcdef",
			service:          "",
			expectedCode:     errors.CodeSQSProcessingFailed,
			expectedCategory: errors.CategoryLambda,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr := classifier.CreateAppError(tt.errorMessage, tt.service)

			require.NotNil(t, appErr)
			assert.Equal(t, tt.expectedCode, appErr.Code)
			assert.Equal(t, tt.expectedCategory, appErr.Category)
		})
	}
}

func TestCreateAppError_RetryableBehavior(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name          string
		errorMessage  string
		service       string
		expectedRetry bool
		description   string
	}{
		{
			name:          "transient error is retryable",
			errorMessage:  "connection timeout",
			service:       "",
			expectedRetry: true,
			description:   "timeout_error is transient, so should be retryable",
		},
		{
			name:          "permanent error is not retryable",
			errorMessage:  "validation failed",
			service:       "",
			expectedRetry: false,
			description:   "validation_error is permanent, so should not be retryable",
		},
		{
			name:          "rate limit is retryable",
			errorMessage:  "rate limit exceeded",
			service:       "",
			expectedRetry: true,
			description:   "rate_limit_error is transient, so should be retryable",
		},
		{
			name:          "network error is retryable",
			errorMessage:  "network unreachable",
			service:       "",
			expectedRetry: true,
			description:   "network_error is transient, so should be retryable",
		},
		{
			name:          "auth error is not retryable",
			errorMessage:  "authentication failed",
			service:       "",
			expectedRetry: false,
			description:   "auth_error is permanent, so should not be retryable",
		},
		{
			name:          "service-specific permanent is not retryable",
			errorMessage:  "user not found",
			service:       processorNotification,
			expectedRetry: false,
			description:   "user_not_found is permanent, so should not be retryable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr := classifier.CreateAppError(tt.errorMessage, tt.service)

			require.NotNil(t, appErr)
			assert.Equal(t, tt.expectedRetry, appErr.Retryable, tt.description)
		})
	}
}

func TestCreateAppError_MetadataKeys(t *testing.T) {
	classifier := NewErrorClassifier()

	appErr := classifier.CreateAppError("validation failed: missing field", "test-service")

	require.NotNil(t, appErr)
	require.NotNil(t, appErr.Metadata)

	// Check required metadata keys
	assert.Contains(t, appErr.Metadata, "service")
	assert.Equal(t, "test-service", appErr.Metadata["service"])

	assert.Contains(t, appErr.Metadata, "error_type")
	assert.Equal(t, "validation_error", appErr.Metadata["error_type"])

	assert.Contains(t, appErr.Metadata, "priority")
	assert.Equal(t, "medium", appErr.Metadata["priority"])

	assert.Contains(t, appErr.Metadata, "category")
}

func TestCreateAppError_InternalMessage(t *testing.T) {
	classifier := NewErrorClassifier()

	testMessage := "validation failed: missing required field name"
	appErr := classifier.CreateAppError(testMessage, "test-service")

	require.NotNil(t, appErr)
	assert.Equal(t, testMessage, appErr.InternalMessage,
		"InternalMessage should be set from the extracted error message")
}

func TestCreateAppError_StackTraceMetadata(t *testing.T) {
	classifier := NewErrorClassifier()

	jsonInput := map[string]interface{}{
		"errorMessage": "validation failed",
		"stackTrace":   "at main.go:10\nat handler.go:25",
	}
	jsonBytes, _ := json.Marshal(jsonInput)

	appErr := classifier.CreateAppError(string(jsonBytes), "test-service")

	require.NotNil(t, appErr)
	require.NotNil(t, appErr.Metadata)
	assert.Contains(t, appErr.Metadata, "stack_trace")
	assert.Contains(t, appErr.Metadata["stack_trace"], "at main.go:10")
}

// ============================================================================
// Additional edge case tests
// ============================================================================

func TestClassifyError_EmptyMessage(t *testing.T) {
	classifier := NewErrorClassifier()

	result := classifier.ClassifyError("", "")

	require.NotNil(t, result)
	// Empty message results in empty ErrorMessage but still returns valid ErrorInfo
	assert.Empty(t, result.ErrorMessage)
	// With no patterns matching, defaults to processing_error
	assert.Equal(t, "processing_error", result.ErrorType)
}

func TestClassifyError_InvalidJSON(t *testing.T) {
	classifier := NewErrorClassifier()

	// Invalid JSON should fall back to plain text extraction
	result := classifier.ClassifyError("{invalid json", "")

	require.NotNil(t, result)
	assert.Contains(t, result.ErrorMessage, "{invalid json")
}

func TestAddCustomPattern(t *testing.T) {
	classifier := NewErrorClassifier()

	// Add a custom pattern
	classifier.AddCustomPattern(
		"custom_error",
		[]string{"custom pattern", "my special error"},
		true,
		"high",
		"Custom error occurred",
	)

	patterns := classifier.GetPatterns()
	customPattern, exists := patterns["custom_error"]

	require.True(t, exists, "custom_error pattern should exist")
	assert.Equal(t, "custom_error", customPattern.ErrorType)
	assert.True(t, customPattern.IsPermanent)
	assert.Equal(t, "high", customPattern.Priority)
	assert.Contains(t, customPattern.Patterns, "custom pattern")
	assert.Contains(t, customPattern.Patterns, "my special error")

	// Verify the pattern is used for classification
	result := classifier.ClassifyError("my special error happened", "")
	assert.Equal(t, "custom_error", result.ErrorType)
}

func TestMapErrorTypeToCodeCategory_AllMappings(t *testing.T) {
	classifier := NewErrorClassifier()

	// Test all documented error type mappings
	mappings := map[string]struct {
		code     errors.ErrorCode
		category errors.ErrorCategory
	}{
		"validation_error":             {errors.CodeValidationFailed, errors.CategoryValidation},
		"auth_error":                   {errors.CodeAuthFailed, errors.CategoryAuth},
		"not_found_error":              {errors.CodeNotFound, errors.CategoryAPI},
		"network_error":                {errors.CodeExternalServiceTimeout, errors.CategoryExternal},
		"rate_limit_error":             {errors.CodeRateLimited, errors.CategoryAPI},
		"service_unavailable":          {errors.CodeExternalServiceUnavailable, errors.CategoryExternal},
		"database_error":               {errors.CodeDatabaseConnection, errors.CategoryStorage},
		"resource_error":               {errors.CodeLambdaMemoryExceeded, errors.CategoryLambda},
		"serialization_error":          {errors.CodeEventProcessingFailed, errors.CategoryLambda},
		"business_logic_error":         {errors.CodeBusinessRuleViolated, errors.CategoryBusiness},
		"federation_error":             {errors.CodeDeliveryFailed, errors.CategoryFederation},
		"timeout_error":                {errors.CodeTimeout, errors.CategoryLambda},
		"user_not_found":               {errors.CodeNotFound, errors.CategoryAuth},
		"invalid_email":                {errors.CodeInvalidFormat, errors.CategoryValidation},
		"push_subscription_error":      {errors.CodeDeliveryFailed, errors.CategoryExternal},
		"signature_verification_error": {errors.CodeSignatureVerifyFailed, errors.CategoryFederation},
		"actor_not_found":              {errors.CodeActorNotFound, errors.CategoryFederation},
		"unsupported_media_format":     {errors.CodeUnsupportedMediaType, errors.CategoryMedia},
		"media_too_large":              {errors.CodeMediaTooLarge, errors.CategoryMedia},
		"media_fetch_error":            {errors.CodeMediaUploadFailed, errors.CategoryMedia},
		"webfinger_error":              {errors.CodeRemoteFetchFailed, errors.CategoryFederation},
		"inbox_unreachable":            {errors.CodeDeliveryFailed, errors.CategoryFederation},
		"index_full_error":             {errors.CodeStorageQuotaExceeded, errors.CategoryStorage},
		"embedding_error":              {errors.CodeEventProcessingFailed, errors.CategoryLambda},
	}

	for errorType, expected := range mappings {
		t.Run(errorType, func(t *testing.T) {
			code, category := classifier.mapErrorTypeToCodeCategory(errorType)
			assert.Equal(t, expected.code, code, "ErrorCode mismatch for %s", errorType)
			assert.Equal(t, expected.category, category, "ErrorCategory mismatch for %s", errorType)
		})
	}
}

func TestClassifyError_CaseInsensitivePatternMatching(t *testing.T) {
	classifier := NewErrorClassifier()

	tests := []struct {
		name         string
		errorMessage string
		expectedType string
	}{
		{
			name:         "lowercase",
			errorMessage: "validation failed",
			expectedType: "validation_error",
		},
		{
			name:         "uppercase",
			errorMessage: "VALIDATION FAILED",
			expectedType: "validation_error",
		},
		{
			name:         "mixed case",
			errorMessage: "Validation Failed",
			expectedType: "validation_error",
		},
		{
			name:         "connection timeout mixed case",
			errorMessage: "CONNECTION timeout",
			expectedType: "network_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyError(tt.errorMessage, "")
			assert.Equal(t, tt.expectedType, result.ErrorType)
		})
	}
}

func TestErrorInfo_FailureReasonPropagation(t *testing.T) {
	classifier := NewErrorClassifier()

	// When the pattern's FailureReason should be used
	result := classifier.ClassifyError("validation failed", "")

	require.NotNil(t, result)
	assert.NotEmpty(t, result.FailureReason)
	assert.Equal(t, "Message failed validation and cannot be processed", result.FailureReason)
}

func TestClassifyError_ServiceUnknown(t *testing.T) {
	classifier := NewErrorClassifier()

	// When service is not one of the known processors, no service-specific
	// classification should be applied
	result := classifier.ClassifyError("validation failed", "unknown-service")

	require.NotNil(t, result)
	// Should still be classified based on patterns
	assert.Equal(t, "validation_error", result.ErrorType)
}

func TestExtractFromJSON_EmptyFields(t *testing.T) {
	classifier := NewErrorClassifier()

	// JSON with empty error fields
	jsonInput := map[string]interface{}{
		"someOtherField": "value",
	}
	jsonBytes, _ := json.Marshal(jsonInput)

	result := classifier.ClassifyError(string(jsonBytes), "")

	require.NotNil(t, result)
	// Should still produce a result even with no error-related fields
	assert.NotNil(t, result.ErrorType)
}

func TestStackTraceArrayWithNonStringElements(t *testing.T) {
	classifier := NewErrorClassifier()

	// JSON with mixed array elements (some non-string)
	jsonInput := map[string]interface{}{
		"errorMessage": "test error",
		"stackTrace":   []interface{}{"line1", 123, "line2", nil, "line3"},
	}
	jsonBytes, _ := json.Marshal(jsonInput)

	result := classifier.ClassifyError(string(jsonBytes), "")

	require.NotNil(t, result)
	// Only string elements should be joined
	assert.Equal(t, "line1\nline2\nline3", result.StackTrace)
}

func TestClassifyError_PlainTextWithMultipleStackPatterns(t *testing.T) {
	classifier := NewErrorClassifier()

	// Text that triggers the stack parsing logic
	text := `Error processing request
at github.com/example/pkg.Process()
service.go:100 (handleRequest)
at github.com/example/pkg.Handle()`

	result := classifier.ClassifyError(text, "")

	require.NotNil(t, result)
	// The error message should be extracted from non-stack lines
	// Stack lines should be in StackTrace
	assert.True(t, strings.Contains(result.StackTrace, "at github.com") ||
		strings.Contains(result.StackTrace, "service.go:100"),
		"StackTrace should contain stack lines")
}
