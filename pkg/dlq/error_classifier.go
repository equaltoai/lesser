package dlq

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ErrorClassifier categorizes and analyzes errors from failed messages
type ErrorClassifier struct {
	patterns map[string]*ErrorPattern
}

// ErrorPattern represents a pattern for classifying errors
type ErrorPattern struct {
	ErrorType     string   `json:"error_type"`
	Patterns      []string `json:"patterns"`
	IsPermanent   bool     `json:"is_permanent"`
	Priority      string   `json:"priority"`
	FailureReason string   `json:"failure_reason"`
}

// ErrorInfo contains classified error information
type ErrorInfo struct {
	ErrorType     string `json:"error_type"`
	ErrorMessage  string `json:"error_message"`
	StackTrace    string `json:"stack_trace,omitempty"`
	FailureReason string `json:"failure_reason"`
	IsPermanent   bool   `json:"is_permanent"`
	Priority      string `json:"priority"`
	Category      string `json:"category"`
}

// NewErrorClassifier creates a new error classifier with predefined patterns
func NewErrorClassifier() *ErrorClassifier {
	classifier := &ErrorClassifier{
		patterns: make(map[string]*ErrorPattern),
	}

	// Initialize with common error patterns
	classifier.initializePatterns()
	
	return classifier
}

// initializePatterns sets up common error classification patterns
func (ec *ErrorClassifier) initializePatterns() {
	patterns := []*ErrorPattern{
		// Validation errors (permanent)
		{
			ErrorType:     "validation_error",
			Patterns:      []string{"validation failed", "invalid", "required field", "missing", "malformed"},
			IsPermanent:   true,
			Priority:      "medium",
			FailureReason: "Message failed validation and cannot be processed",
		},
		
		// Authentication errors (permanent)
		{
			ErrorType:     "auth_error",
			Patterns:      []string{"unauthorized", "forbidden", "authentication failed", "invalid token", "expired token"},
			IsPermanent:   true,
			Priority:      "high",
			FailureReason: "Authentication or authorization failed",
		},
		
		// Resource not found (permanent)
		{
			ErrorType:     "not_found_error",
			Patterns:      []string{"not found", "does not exist", "404", "no such", "unknown"},
			IsPermanent:   true,
			Priority:      "medium",
			FailureReason: "Referenced resource does not exist",
		},
		
		// Network/connectivity errors (transient)
		{
			ErrorType:     "network_error",
			Patterns:      []string{"connection", "timeout", "network", "unreachable", "dns", "socket"},
			IsPermanent:   false,
			Priority:      "high",
			FailureReason: "Network connectivity issues",
		},
		
		// Rate limiting (transient)
		{
			ErrorType:     "rate_limit_error", 
			Patterns:      []string{"rate limit", "throttle", "too many requests", "429", "quota exceeded"},
			IsPermanent:   false,
			Priority:      "medium",
			FailureReason: "Rate limiting or quota exceeded",
		},
		
		// Service unavailable (transient)
		{
			ErrorType:     "service_unavailable",
			Patterns:      []string{"service unavailable", "503", "502", "500", "internal server error", "bad gateway"},
			IsPermanent:   false,
			Priority:      "high",
			FailureReason: "External service temporarily unavailable",
		},
		
		// Database errors (transient)
		{
			ErrorType:     "database_error",
			Patterns:      []string{"database", "connection pool", "deadlock", "lock timeout", "query timeout"},
			IsPermanent:   false,
			Priority:      "high",
			FailureReason: "Database connectivity or performance issues",
		},
		
		// Memory/resource errors (transient)
		{
			ErrorType:     "resource_error",
			Patterns:      []string{"out of memory", "memory", "disk full", "no space", "resource exhausted"},
			IsPermanent:   false,
			Priority:      "critical",
			FailureReason: "System resource exhaustion",
		},
		
		// Serialization errors (permanent)
		{
			ErrorType:     "serialization_error",
			Patterns:      []string{"json", "unmarshal", "parse", "decode", "serialize", "invalid format"},
			IsPermanent:   true,
			Priority:      "medium",
			FailureReason: "Message format or serialization issues",
		},
		
		// Business logic errors (permanent)
		{
			ErrorType:     "business_logic_error",
			Patterns:      []string{"business rule", "constraint", "invariant", "precondition", "postcondition"},
			IsPermanent:   true,
			Priority:      "low",
			FailureReason: "Business logic validation failed",
		},
		
		// Federation errors (mixed)
		{
			ErrorType:     "federation_error",
			Patterns:      []string{"federation", "activitypub", "webfinger", "signature", "actor"},
			IsPermanent:   false, // Default to transient for federation
			Priority:      "medium",
			FailureReason: "ActivityPub federation issues",
		},
		
		// Processing timeout (transient)
		{
			ErrorType:     "timeout_error",
			Patterns:      []string{"timeout", "timed out", "deadline exceeded", "context deadline"},
			IsPermanent:   false,
			Priority:      "medium",
			FailureReason: "Processing timeout exceeded",
		},
	}

	// Register all patterns
	for _, pattern := range patterns {
		ec.patterns[pattern.ErrorType] = pattern
	}
}

// ClassifyError analyzes an error and returns classification information
func (ec *ErrorClassifier) ClassifyError(messageBody, service string) *ErrorInfo {
	// Try to parse the message body to extract error information
	errorInfo := ec.extractErrorFromMessage(messageBody)
	
	// If we couldn't extract error info, create basic info
	if errorInfo == nil {
		errorInfo = &ErrorInfo{
			ErrorMessage:  "Failed to process message",
			FailureReason: "Unknown processing error",
			IsPermanent:   false,
			Priority:      "medium",
			Category:      "unknown",
		}
	}

	// Classify the error based on patterns
	errorInfo.ErrorType = ec.classifyByPatterns(errorInfo.ErrorMessage)
	
	// Apply service-specific classification
	ec.applyServiceSpecificClassification(errorInfo, service)
	
	// Set additional properties based on classification
	if pattern, exists := ec.patterns[errorInfo.ErrorType]; exists {
		errorInfo.IsPermanent = pattern.IsPermanent
		errorInfo.Priority = pattern.Priority
		if errorInfo.FailureReason == "" {
			errorInfo.FailureReason = pattern.FailureReason
		}
	}

	return errorInfo
}

// extractErrorFromMessage attempts to extract error information from the message body
func (ec *ErrorClassifier) extractErrorFromMessage(messageBody string) *ErrorInfo {
	// Try to parse as JSON first (common for Lambda errors)
	var jsonMsg map[string]interface{}
	if err := json.Unmarshal([]byte(messageBody), &jsonMsg); err == nil {
		return ec.extractFromJSON(jsonMsg)
	}

	// Try to parse as plain text error
	return ec.extractFromText(messageBody)
}

// extractFromJSON extracts error info from JSON message
func (ec *ErrorClassifier) extractFromJSON(jsonMsg map[string]interface{}) *ErrorInfo {
	errorInfo := &ErrorInfo{}

	// Look for common error fields
	if errorMessage, exists := jsonMsg["errorMessage"]; exists {
		if msg, ok := errorMessage.(string); ok {
			errorInfo.ErrorMessage = msg
		}
	} else if err, exists := jsonMsg["error"]; exists {
		if msg, ok := err.(string); ok {
			errorInfo.ErrorMessage = msg
		}
	} else if message, exists := jsonMsg["message"]; exists {
		if msg, ok := message.(string); ok {
			errorInfo.ErrorMessage = msg
		}
	}

	// Look for stack trace
	if stackTrace, exists := jsonMsg["stackTrace"]; exists {
		if stack, ok := stackTrace.(string); ok {
			errorInfo.StackTrace = stack
		} else if stackArray, ok := stackTrace.([]interface{}); ok {
			// Convert array to string
			var stackLines []string
			for _, line := range stackArray {
				if lineStr, ok := line.(string); ok {
					stackLines = append(stackLines, lineStr)
				}
			}
			errorInfo.StackTrace = strings.Join(stackLines, "\n")
		}
	}

	// Look for error type
	if errorType, exists := jsonMsg["errorType"]; exists {
		if eType, ok := errorType.(string); ok {
			errorInfo.Category = eType
		}
	}

	return errorInfo
}

// extractFromText extracts error info from plain text
func (ec *ErrorClassifier) extractFromText(messageBody string) *ErrorInfo {
	errorInfo := &ErrorInfo{
		ErrorMessage: messageBody,
	}

	// Look for stack traces
	if strings.Contains(messageBody, "at ") && strings.Contains(messageBody, "(") {
		lines := strings.Split(messageBody, "\n")
		var errorLines []string
		var stackLines []string
		
		inStack := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			
			if strings.HasPrefix(line, "at ") || strings.Contains(line, ".go:") {
				inStack = true
				stackLines = append(stackLines, line)
			} else if !inStack {
				errorLines = append(errorLines, line)
			}
		}
		
		if len(errorLines) > 0 {
			errorInfo.ErrorMessage = strings.Join(errorLines, " ")
		}
		if len(stackLines) > 0 {
			errorInfo.StackTrace = strings.Join(stackLines, "\n")
		}
	}

	return errorInfo
}

// classifyByPatterns matches error message against known patterns
func (ec *ErrorClassifier) classifyByPatterns(errorMessage string) string {
	errorMessageLower := strings.ToLower(errorMessage)

	// Score each pattern
	bestMatch := ""
	bestScore := 0

	for errorType, pattern := range ec.patterns {
		score := 0
		for _, patternStr := range pattern.Patterns {
			if strings.Contains(errorMessageLower, strings.ToLower(patternStr)) {
				score += len(patternStr) // Longer matches get higher scores
			}
		}
		
		if score > bestScore {
			bestScore = score
			bestMatch = errorType
		}
	}

	if bestMatch != "" {
		return bestMatch
	}

	return "processing_error" // Default classification
}

// applyServiceSpecificClassification applies service-specific error classification logic
func (ec *ErrorClassifier) applyServiceSpecificClassification(errorInfo *ErrorInfo, service string) {
	switch service {
	case "notification-processor":
		ec.classifyNotificationErrors(errorInfo)
	case "activity-processor":
		ec.classifyActivityErrors(errorInfo)
	case "media-processor":
		ec.classifyMediaErrors(errorInfo)
	case "federation-delivery":
		ec.classifyFederationErrors(errorInfo)
	case "search-indexer":
		ec.classifySearchErrors(errorInfo)
	}
}

// classifyNotificationErrors applies notification-specific classification
func (ec *ErrorClassifier) classifyNotificationErrors(errorInfo *ErrorInfo) {
	message := strings.ToLower(errorInfo.ErrorMessage)

	if strings.Contains(message, "user not found") || strings.Contains(message, "invalid user") {
		errorInfo.ErrorType = "user_not_found"
		errorInfo.IsPermanent = true
		errorInfo.Priority = "low"
		errorInfo.FailureReason = "Target user no longer exists"
	} else if strings.Contains(message, "email") && strings.Contains(message, "invalid") {
		errorInfo.ErrorType = "invalid_email"
		errorInfo.IsPermanent = true
		errorInfo.Priority = "medium"
		errorInfo.FailureReason = "Invalid email address for notification delivery"
	} else if strings.Contains(message, "push") && (strings.Contains(message, "endpoint") || strings.Contains(message, "subscription")) {
		errorInfo.ErrorType = "push_subscription_error"
		errorInfo.IsPermanent = true
		errorInfo.Priority = "low"
		errorInfo.FailureReason = "Push subscription is invalid or expired"
	}
}

// classifyActivityErrors applies activity processing specific classification
func (ec *ErrorClassifier) classifyActivityErrors(errorInfo *ErrorInfo) {
	message := strings.ToLower(errorInfo.ErrorMessage)

	if strings.Contains(message, "signature") && strings.Contains(message, "verification") {
		errorInfo.ErrorType = "signature_verification_error"
		errorInfo.IsPermanent = true
		errorInfo.Priority = "high"
		errorInfo.FailureReason = "ActivityPub signature verification failed"
	} else if strings.Contains(message, "actor") && strings.Contains(message, "not found") {
		errorInfo.ErrorType = "actor_not_found"
		errorInfo.IsPermanent = false // Actor might come back online
		errorInfo.Priority = "medium"
		errorInfo.FailureReason = "ActivityPub actor not accessible"
	}
}

// classifyMediaErrors applies media processing specific classification
func (ec *ErrorClassifier) classifyMediaErrors(errorInfo *ErrorInfo) {
	message := strings.ToLower(errorInfo.ErrorMessage)

	if strings.Contains(message, "format") && (strings.Contains(message, "unsupported") || strings.Contains(message, "invalid")) {
		errorInfo.ErrorType = "unsupported_media_format"
		errorInfo.IsPermanent = true
		errorInfo.Priority = "low"
		errorInfo.FailureReason = "Media format not supported for processing"
	} else if strings.Contains(message, "size") && strings.Contains(message, "too large") {
		errorInfo.ErrorType = "media_too_large"
		errorInfo.IsPermanent = true
		errorInfo.Priority = "low"
		errorInfo.FailureReason = "Media file exceeds size limits"
	} else if strings.Contains(message, "download") || strings.Contains(message, "fetch") {
		errorInfo.ErrorType = "media_fetch_error"
		errorInfo.IsPermanent = false
		errorInfo.Priority = "medium"
		errorInfo.FailureReason = "Failed to download media file"
	}
}

// classifyFederationErrors applies federation specific classification
func (ec *ErrorClassifier) classifyFederationErrors(errorInfo *ErrorInfo) {
	message := strings.ToLower(errorInfo.ErrorMessage)

	if strings.Contains(message, "webfinger") {
		errorInfo.ErrorType = "webfinger_error"
		errorInfo.IsPermanent = false
		errorInfo.Priority = "medium"
		errorInfo.FailureReason = "WebFinger discovery failed"
	} else if strings.Contains(message, "inbox") && strings.Contains(message, "unreachable") {
		errorInfo.ErrorType = "inbox_unreachable"
		errorInfo.IsPermanent = false
		errorInfo.Priority = "high"
		errorInfo.FailureReason = "Remote inbox is unreachable"
	}
}

// classifySearchErrors applies search indexing specific classification
func (ec *ErrorClassifier) classifySearchErrors(errorInfo *ErrorInfo) {
	message := strings.ToLower(errorInfo.ErrorMessage)

	if strings.Contains(message, "index") && strings.Contains(message, "full") {
		errorInfo.ErrorType = "index_full_error"
		errorInfo.IsPermanent = false
		errorInfo.Priority = "critical"
		errorInfo.FailureReason = "Search index capacity exceeded"
	} else if strings.Contains(message, "embedding") {
		errorInfo.ErrorType = "embedding_error"
		errorInfo.IsPermanent = false
		errorInfo.Priority = "medium"
		errorInfo.FailureReason = "Failed to generate text embeddings"
	}
}

// AddCustomPattern adds a custom error pattern
func (ec *ErrorClassifier) AddCustomPattern(errorType string, patterns []string, isPermanent bool, priority, failureReason string) {
	ec.patterns[errorType] = &ErrorPattern{
		ErrorType:     errorType,
		Patterns:      patterns,
		IsPermanent:   isPermanent,
		Priority:      priority,
		FailureReason: failureReason,
	}
}

// GetPatterns returns all registered patterns
func (ec *ErrorClassifier) GetPatterns() map[string]*ErrorPattern {
	return ec.patterns
}

// AnalyzeErrorTrends analyzes error trends from a collection of messages
func (ec *ErrorClassifier) AnalyzeErrorTrends(messages []string) *ErrorTrendAnalysis {
	analysis := &ErrorTrendAnalysis{
		TotalMessages:    len(messages),
		ErrorTypeCounts:  make(map[string]int),
		PermanentErrors:  0,
		TransientErrors:  0,
		PriorityBreakdown: make(map[string]int),
	}

	for _, message := range messages {
		errorInfo := ec.ClassifyError(message, "")
		
		analysis.ErrorTypeCounts[errorInfo.ErrorType]++
		analysis.PriorityBreakdown[errorInfo.Priority]++
		
		if errorInfo.IsPermanent {
			analysis.PermanentErrors++
		} else {
			analysis.TransientErrors++
		}
	}

	// Calculate percentages
	if analysis.TotalMessages > 0 {
		analysis.PermanentErrorRate = float64(analysis.PermanentErrors) / float64(analysis.TotalMessages) * 100
		analysis.TransientErrorRate = float64(analysis.TransientErrors) / float64(analysis.TotalMessages) * 100
	}

	return analysis
}

// ErrorTrendAnalysis represents analysis of error patterns
type ErrorTrendAnalysis struct {
	TotalMessages       int                `json:"total_messages"`
	ErrorTypeCounts     map[string]int     `json:"error_type_counts"`
	PermanentErrors     int                `json:"permanent_errors"`
	TransientErrors     int                `json:"transient_errors"`
	PermanentErrorRate  float64            `json:"permanent_error_rate"`
	TransientErrorRate  float64            `json:"transient_error_rate"`
	PriorityBreakdown   map[string]int     `json:"priority_breakdown"`
}

// Helper functions for pattern matching

// matchesRegex checks if a string matches a regex pattern
func matchesRegex(text, pattern string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(text)
}

// containsAny checks if text contains any of the given substrings
func containsAny(text string, substrings []string) bool {
	textLower := strings.ToLower(text)
	for _, substr := range substrings {
		if strings.Contains(textLower, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}