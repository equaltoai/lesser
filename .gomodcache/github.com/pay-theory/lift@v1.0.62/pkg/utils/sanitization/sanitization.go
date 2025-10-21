// Package sanitization provides centralized data sanitization utilities
// to prevent sensitive data exposure across the Lift framework.
package sanitization

import (
	"fmt"
	"strings"

	"github.com/pay-theory/lift/pkg/security"
)

const (
	redactedValue = "[REDACTED]"
)

// AllowedFields are field names that should not be sanitized
var AllowedFields = map[string]bool{
	"card_bin":   true,
	"card_brand": true,
	"card_type":  true,
}

// Sanitizer provides methods for sanitizing various types of data
type Sanitizer struct {
	dataProtectionManager *security.DataProtectionManager
}

// New creates a new Sanitizer with a data protection manager
func New(dpm *security.DataProtectionManager) *Sanitizer {
	return &Sanitizer{dataProtectionManager: dpm}
}

// Default returns a sanitizer with default data protection configuration
func Default() *Sanitizer {
	// Create a default data protection config for sanitization
	config := security.DataProtectionConfig{
		DefaultClassification: security.DataPublic,
		FieldClassifications:  make(map[string]security.DataClassification),
		EncryptionKey:         "default-sanitization-key", // This is only for classification, not actual encryption
	}

	dpm, err := security.NewDataProtectionManager(config)
	if err != nil {
		// If we can't create data protection manager, return a minimal sanitizer
		// that will redact everything for safety
		return &Sanitizer{dataProtectionManager: nil}
	}

	return &Sanitizer{dataProtectionManager: dpm}
}

// SanitizeFieldValue sanitizes a field value based on its key name and data classification
func (s *Sanitizer) SanitizeFieldValue(key string, value any) any {
	processor := newFieldSanitizationProcessor(s, key, value)
	return processor.sanitize()
}

// fieldSanitizationProcessor handles field value sanitization
type fieldSanitizationProcessor struct {
	sanitizer *Sanitizer
	key       string
	value     any
	keyLower  string
}

// newFieldSanitizationProcessor creates a new field sanitization processor
func newFieldSanitizationProcessor(sanitizer *Sanitizer, key string, value any) *fieldSanitizationProcessor {
	return &fieldSanitizationProcessor{
		sanitizer: sanitizer,
		key:       key,
		keyLower:  strings.ToLower(key),
		value:     value,
	}
}

// sanitize performs the sanitization process
func (p *fieldSanitizationProcessor) sanitize() any {
	// Check if field is explicitly allowed
	if AllowedFields[p.keyLower] {
		return p.value
	}

	// If no data protection manager, redact everything for safety
	if p.sanitizer.dataProtectionManager == nil {
		return redactedValue
	}

	// Get data classification
	classification := p.getDataClassification()

	// Apply sanitization based on classification
	return p.applySanitization(classification)
}

// getDataClassification determines the data classification for the field
func (p *fieldSanitizationProcessor) getDataClassification() security.DataClassification {
	// Use the data protection manager to classify the field
	dataCtx := p.sanitizer.dataProtectionManager.ClassifyData(
		map[string]any{p.key: p.value},
		map[string]any{"source": "sanitizer"},
	)

	// Get the classification for this specific field
	if fieldClass, exists := dataCtx.Fields[p.key]; exists {
		return fieldClass
	}

	// Use overall classification if field-specific not found
	return dataCtx.Classification
}

// applySanitization applies the appropriate sanitization based on classification
func (p *fieldSanitizationProcessor) applySanitization(classification security.DataClassification) any {
	handler := newClassificationHandler(p.keyLower, p.value)

	switch classification {
	case security.DataRestricted:
		return handler.handleRestricted()
	case security.DataConfidential:
		return handler.handleConfidential()
	case security.DataInternal:
		return handler.handleInternal()
	case security.DataPublic:
		return handler.handlePublic()
	default:
		return handler.handleUnknown()
	}
}

// classificationHandler handles sanitization for different data classifications
type classificationHandler struct {
	value    any
	keyLower string
}

// newClassificationHandler creates a new classification handler
func newClassificationHandler(keyLower string, value any) *classificationHandler {
	return &classificationHandler{
		keyLower: keyLower,
		value:    value,
	}
}

// handleRestricted handles restricted data sanitization
func (h *classificationHandler) handleRestricted() any {
	str, ok := h.value.(string)
	if !ok {
		return redactedValue
	}

	// Clean the string to check if it's a number
	cleaned := strings.ReplaceAll(strings.ReplaceAll(str, " ", ""), "-", "")
	if len(cleaned) >= 4 && isNumeric(cleaned) {
		// Show last 4 digits, mask the rest
		masked := strings.Repeat("*", len(cleaned)-4) + cleaned[len(cleaned)-4:]
		return masked
	}

	return redactedValue
}

// handleConfidential handles confidential data sanitization
func (h *classificationHandler) handleConfidential() any {
	return redactedValue
}

// handleInternal handles internal data sanitization
func (h *classificationHandler) handleInternal() any {
	str, ok := h.value.(string)
	if !ok {
		return h.value
	}

	// Handle user-generated content
	if isUserContentField(h.keyLower) {
		return h.sanitizeUserContent(str)
	}

	// Handle error messages
	if h.isErrorField() {
		return h.sanitizeErrorMessage(str)
	}

	// Handle large strings
	if len(str) > 200 {
		return fmt.Sprintf("[LARGE_STRING_%d_CHARS]", len(str))
	}

	return h.value
}

// handlePublic handles public data (no sanitization needed)
func (h *classificationHandler) handlePublic() any {
	return h.value
}

// handleUnknown handles unknown classification (default to redact)
func (h *classificationHandler) handleUnknown() any {
	return redactedValue
}

// sanitizeUserContent sanitizes user-generated content
func (h *classificationHandler) sanitizeUserContent(str string) string {
	if len(str) > 0 {
		return fmt.Sprintf("[USER_CONTENT_%d_CHARS]", len(str))
	}
	return "[USER_CONTENT]"
}

// isErrorField checks if the field is an error field
func (h *classificationHandler) isErrorField() bool {
	return h.keyLower == "error" || strings.Contains(h.keyLower, "error")
}

// sanitizeErrorMessage sanitizes error messages
func (h *classificationHandler) sanitizeErrorMessage(str string) any {
	if len(str) > 50 ||
		strings.Contains(strings.ToLower(str), "input") ||
		strings.Contains(strings.ToLower(str), "invalid") {
		return "[SANITIZED_ERROR]"
	}
	return str
}

// isUserContentField checks if a field contains user-generated content
func isUserContentField(fieldLower string) bool {
	userContentFields := []string{
		"body", "request_body", "response_body", "user_input",
		"query", "search", "message", "comment", "description",
	}
	for _, field := range userContentFields {
		if strings.Contains(fieldLower, field) {
			return true
		}
	}
	return false
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, char := range s {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

// SanitizeHeaders removes sensitive headers from a map
func (s *Sanitizer) SanitizeHeaders(headers map[string][]string) map[string]string {
	sensitiveHeaders := map[string]bool{
		"authorization": true,
		"cookie":        true,
		"x-api-key":     true,
		"x-auth-token":  true,
		"x-csrf-token":  true,
		"x-session-id":  true,
	}

	result := make(map[string]string)
	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		if sensitiveHeaders[lowerKey] {
			result[key] = redactedValue
		} else if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

// SanitizeQueryParams sanitizes query parameters
func (s *Sanitizer) SanitizeQueryParams(params map[string][]string) map[string]string {
	sensitiveParams := map[string]bool{
		"token":    true,
		"api_key":  true,
		"apikey":   true,
		"password": true,
		"secret":   true,
		"auth":     true,
		"session":  true,
	}

	result := make(map[string]string)
	for key, values := range params {
		lowerKey := strings.ToLower(key)
		if sensitiveParams[lowerKey] {
			result[key] = "[SANITIZED_QUERY_PARAMS]"
		} else if len(values) > 0 {
			// Also apply field value sanitization to query param values
			sanitizedValue := s.SanitizeFieldValue(key, values[0])
			result[key] = fmt.Sprintf("%v", sanitizedValue)
		}
	}
	return result
}

// SanitizeMap applies sanitization to all values in a map
func (s *Sanitizer) SanitizeMap(data map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range data {
		result[key] = s.SanitizeFieldValue(key, value)
	}
	return result
}

// Global default sanitizer for convenience
var defaultSanitizer = Default()

// SanitizeFieldValue uses the default sanitizer
func SanitizeFieldValue(key string, value any) any {
	return defaultSanitizer.SanitizeFieldValue(key, value)
}

// SanitizeHeaders uses the default sanitizer
func SanitizeHeaders(headers map[string][]string) map[string]string {
	return defaultSanitizer.SanitizeHeaders(headers)
}

// SanitizeQueryParams uses the default sanitizer
func SanitizeQueryParams(params map[string][]string) map[string]string {
	return defaultSanitizer.SanitizeQueryParams(params)
}

// SanitizeMap uses the default sanitizer
func SanitizeMap(data map[string]any) map[string]any {
	return defaultSanitizer.SanitizeMap(data)
}
