package validation

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// SecurityError represents a security validation error
type SecurityError struct {
	Type   string
	Field  string
	Detail string
}

func (e *SecurityError) Error() string {
	// SECURITY: Don't expose user-generated field names or content in error messages
	// Only return the error type for secure logging
	return fmt.Sprintf("security validation failed: %s", e.Type)
}

// Field validation constants
const (
	MaxFieldNameLength   = 255
	MaxOperatorLength    = 20
	MaxValueStringLength = 400000 // DynamoDB item size limit
	MaxNestedDepth       = 32
	MaxExpressionLength  = 4096
)

// SQL injection and dangerous patterns - exact matches or patterns that are clearly malicious
var dangerousPatterns = []string{
	"'", "\"", ";", "--", "/*", "*/",
	"<script", "</script", "eval(", "expression(", "import(", "require(",
}

// SQL keywords that should only be rejected if they appear as whole words or in suspicious contexts
var sqlKeywords = []string{
	"union", "select", "insert", "update", "delete", "drop", "alter", "exec", "execute",
	"script", "javascript", "vbscript",
}

// Common legitimate field name patterns that contain SQL keywords but are safe
var legitimateFieldPatterns = []regexp.Regexp{
	*regexp.MustCompile(`(?i)^(created|updated)at$`),              // CreatedAt, UpdatedAt
	*regexp.MustCompile(`(?i)^create(d|r)_?(at|time|date)$`),      // created_at, creator_time, etc.
	*regexp.MustCompile(`(?i)^update(d|r)_?(at|time|date)$`),      // updated_at, updater_time, etc.
	*regexp.MustCompile(`(?i)^delete(d|r)_?(at|time|date|flag)$`), // deleted_at, deleter_time, delete_flag, etc.
	*regexp.MustCompile(`(?i)^insert(ed|er)_?(at|time|date)$`),    // inserted_at, inserter_time, etc.
	*regexp.MustCompile(`(?i)^select(ed|or)_?(at|time|date)$`),    // selected_at, selector_time, etc.
}

// Valid operator whitelist
var allowedOperators = map[string]bool{
	"=":                    true,
	"!=":                   true,
	"<>":                   true,
	"<":                    true,
	"<=":                   true,
	">":                    true,
	">=":                   true,
	"BETWEEN":              true,
	"IN":                   true,
	"BEGINS_WITH":          true,
	"CONTAINS":             true,
	"EXISTS":               true,
	"NOT_EXISTS":           true,
	"ATTRIBUTE_EXISTS":     true,
	"ATTRIBUTE_NOT_EXISTS": true,
	"EQ":                   true,
	"NE":                   true,
	"LT":                   true,
	"LE":                   true,
	"GT":                   true,
	"GE":                   true,
}

// ValidateFieldName validates a DynamoDB attribute name according to AWS rules and security best practices
func ValidateFieldName(field string) error {
	if field == "" {
		return &SecurityError{
			Type:   "InvalidField",
			Field:  "",
			Detail: "field name cannot be empty",
		}
	}

	if len(field) > MaxFieldNameLength {
		return &SecurityError{
			Type:   "InvalidField",
			Field:  "",
			Detail: "field name exceeds maximum length",
		}
	}

	// Check for dangerous patterns (exact matches and clearly malicious patterns)
	fieldLower := strings.ToLower(field)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(fieldLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "field name contains dangerous pattern",
			}
		}
	}

	// Check for SQL keywords, but allow legitimate field name patterns
	for _, keyword := range sqlKeywords {
		if strings.Contains(fieldLower, keyword) {
			// Check if this matches a legitimate field pattern
			isLegitimate := false
			for _, pattern := range legitimateFieldPatterns {
				if pattern.MatchString(field) {
					isLegitimate = true
					break
				}
			}

			// If not a legitimate pattern, check if it's a suspicious usage
			if !isLegitimate {
				// Allow if it's part of a compound word (like "CreateTime" or "user_created")
				// but reject standalone keywords or suspicious combinations
				if isStandaloneOrSuspiciousKeyword(fieldLower, keyword) {
					return &SecurityError{
						Type:   "InjectionAttempt",
						Field:  "",
						Detail: "field name contains suspicious content",
					}
				}
			}
		}
	}

	// Check for control characters
	for _, r := range field {
		if unicode.IsControl(r) {
			return &SecurityError{
				Type:   "InvalidField",
				Field:  "",
				Detail: "field name contains control characters",
			}
		}
	}

	// Validate nested field paths
	if strings.Contains(field, ".") {
		parts := strings.Split(field, ".")
		if len(parts) > MaxNestedDepth {
			return &SecurityError{
				Type:   "InvalidField",
				Field:  "",
				Detail: "nested field depth exceeds maximum",
			}
		}

		for _, part := range parts {
			if err := validateFieldPart(part); err != nil {
				return &SecurityError{
					Type:   "InvalidField",
					Field:  "",
					Detail: "invalid field part",
				}
			}
		}
	} else {
		return validateFieldPart(field)
	}

	return nil
}

// isStandaloneOrSuspiciousKeyword checks if a SQL keyword appears in a suspicious way
func isStandaloneOrSuspiciousKeyword(fieldLower, keyword string) bool {
	// Exact match (standalone keyword)
	if fieldLower == keyword {
		return true
	}

	// Check for suspicious patterns like "field; DROP" or "field UNION"
	suspiciousPatterns := []string{
		keyword + ";", ";" + keyword, keyword + " ", " " + keyword,
		keyword + ".", "." + keyword, keyword + "-", "-" + keyword,
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(fieldLower, pattern) {
			return true
		}
	}

	// Allow compound words where the keyword is naturally part of a field name
	// For example, "UserCreated", "PostUpdated", "OrderDeleted" are likely legitimate
	return false
}

// validateFieldPart validates a single part of a field name
func validateFieldPart(part string) error {
	if part == "" {
		return fmt.Errorf("field part cannot be empty")
	}

	// Handle DynamoDB list element access syntax: fieldName[index]
	if strings.Contains(part, "[") && strings.Contains(part, "]") {
		// Split into field name and index parts
		openBracket := strings.Index(part, "[")
		closeBracket := strings.LastIndex(part, "]")

		if closeBracket <= openBracket {
			return fmt.Errorf("invalid bracket syntax in field part")
		}

		fieldName := part[:openBracket]
		indexPart := part[openBracket+1 : closeBracket]
		remainingPart := part[closeBracket+1:]

		// Validate the field name part
		fieldPattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
		if !fieldPattern.MatchString(fieldName) {
			return fmt.Errorf("field name part must start with letter or underscore and contain only alphanumeric characters and underscores")
		}

		// Validate the index part (must be a number)
		indexPattern := regexp.MustCompile(`^[0-9]+$`)
		if !indexPattern.MatchString(indexPart) {
			return fmt.Errorf("list index must be a number")
		}

		// Validate any remaining part after the bracket (should be empty for simple cases)
		if remainingPart != "" {
			return fmt.Errorf("unexpected characters after list index")
		}

		return nil
	}

	// AWS DynamoDB attribute name rules for regular field names
	validPattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if !validPattern.MatchString(part) {
		return fmt.Errorf("field part must start with letter or underscore and contain only alphanumeric characters and underscores")
	}

	return nil
}

// ValidateOperator validates a DynamoDB condition operator
func ValidateOperator(op string) error {
	if op == "" {
		return &SecurityError{
			Type:   "InvalidOperator",
			Field:  "",
			Detail: "operator cannot be empty",
		}
	}

	if len(op) > MaxOperatorLength {
		return &SecurityError{
			Type:   "InvalidOperator",
			Field:  "",
			Detail: "operator exceeds maximum length",
		}
	}

	// Check against whitelist
	opUpper := strings.ToUpper(strings.TrimSpace(op))
	if !allowedOperators[opUpper] {
		return &SecurityError{
			Type:   "InvalidOperator",
			Field:  "",
			Detail: "operator not allowed",
		}
	}

	// Check for dangerous patterns
	opLower := strings.ToLower(op)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(opLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "operator contains dangerous pattern",
			}
		}
	}

	return nil
}

// ValidateValue validates a value used in DynamoDB expressions
func ValidateValue(value any) error {
	if value == nil {
		return nil // NULL values are allowed
	}

	switch v := value.(type) {
	case string:
		return validateStringValue(v)
	case []any:
		return validateSliceValue(v)
	case []string:
		return validateSliceLength(len(v))
	case []int:
		return validateSliceLength(len(v))
	case []int8:
		return validateSliceLength(len(v))
	case []int16:
		return validateSliceLength(len(v))
	case []int32:
		return validateSliceLength(len(v))
	case []int64:
		return validateSliceLength(len(v))
	case []uint:
		return validateSliceLength(len(v))
	case []uint8:
		return validateSliceLength(len(v))
	case []uint16:
		return validateSliceLength(len(v))
	case []uint32:
		return validateSliceLength(len(v))
	case []uint64:
		return validateSliceLength(len(v))
	case []float32:
		return validateSliceLength(len(v))
	case []float64:
		return validateSliceLength(len(v))
	case []bool:
		return validateSliceLength(len(v))
	case map[string]any:
		return validateMapValue(v)
	case map[string]string:
		return validateTypedMapValue(v)
	case map[string]int:
		return validateTypedMapIntValue(v)
	default:
		// For other types (int, float, bool, etc.), basic validation
		return validateBasicValue(v)
	}
}

// validateStringValue validates string values
func validateStringValue(s string) error {
	if len(s) > MaxValueStringLength {
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "string value exceeds maximum length",
		}
	}

	// For string VALUES (not field names), we should be more permissive
	// DynamoDB stores data, including JSON, HTML, etc. which naturally contain quotes
	// We only need to check for actual injection attempts, not legitimate data

	stringLower := strings.ToLower(s)

	// Check for script injection patterns (but allow quotes and semicolons in data)
	scriptPatterns := []string{
		"<script", "</script", "eval(", "expression(", "import(", "require(",
		"javascript:", "vbscript:", "onload=", "onerror=", "onclick=",
	}

	for _, pattern := range scriptPatterns {
		if strings.Contains(stringLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "string value contains dangerous pattern",
			}
		}
	}

	// Check for comment patterns that are clearly malicious
	if strings.Contains(s, "/*") && strings.Contains(s, "*/") {
		return &SecurityError{
			Type:   "InjectionAttempt",
			Field:  "",
			Detail: "string value contains dangerous pattern",
		}
	}

	// Check for SQL injection patterns in string values
	// Be more nuanced - allow legitimate use of SQL keywords in data
	// but reject clear injection attempts
	sqlInjectionPatterns := []string{
		"'; drop table", "'; delete from", "'; update ", "'; insert into",
		"\"; drop table", "\"; delete from", "\"; update ", "\"; insert into",
		"' or 1=1", "\" or 1=1", "' or '1'='1", "\" or \"1\"=\"1",
		"/**/union/**/select", "concat(0x", "char(", "load_file(",
		"--", // SQL comment at end of value is suspicious
	}

	for _, pattern := range sqlInjectionPatterns {
		if strings.Contains(stringLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "string value contains dangerous pattern",
			}
		}
	}

	// Check for specific dangerous command patterns
	// Allow "union select" in general text but block with dangerous context
	if strings.Contains(stringLower, "union") && strings.Contains(stringLower, "select") {
		// Check if it looks like a SQL injection attempt
		if strings.Contains(stringLower, "union select") ||
			strings.Contains(stringLower, "union all select") ||
			strings.Contains(stringLower, "union/**/select") {
			// Check for additional SQL patterns that indicate injection
			if strings.Contains(stringLower, "from") ||
				strings.Contains(stringLower, "*") ||
				strings.HasSuffix(s, "--") ||
				strings.HasSuffix(s, ";") {
				return &SecurityError{
					Type:   "InjectionAttempt",
					Field:  "",
					Detail: "string value contains dangerous pattern",
				}
			}
		}
	}

	return nil
}

// validateSliceValue validates slice values (for IN operator, etc.)
func validateSliceValue(slice []any) error {
	if len(slice) > 100 { // DynamoDB IN operator limit
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "slice value exceeds maximum length of 100 items",
		}
	}

	for _, item := range slice {
		if err := ValidateValue(item); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid item in collection",
			}
		}
	}

	return nil
}

// validateMapValue validates map values
func validateMapValue(m map[string]any) error {
	if len(m) > 100 { // Reasonable limit for map size
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "map value exceeds maximum keys",
		}
	}

	for key, value := range m {
		if err := ValidateFieldName(key); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map key",
			}
		}

		if err := ValidateValue(value); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map value",
			}
		}
	}

	return nil
}

// validateTypedMapValue validates typed map values
func validateTypedMapValue(m map[string]string) error {
	if len(m) > 100 { // Reasonable limit for map size
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "map value exceeds maximum keys",
		}
	}

	for key, value := range m {
		if err := ValidateFieldName(key); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map key",
			}
		}

		if err := ValidateValue(value); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map value",
			}
		}
	}

	return nil
}

// validateTypedMapIntValue validates typed map values
func validateTypedMapIntValue(m map[string]int) error {
	if len(m) > 100 { // Reasonable limit for map size
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "map value exceeds maximum keys",
		}
	}

	for key, value := range m {
		if err := ValidateFieldName(key); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map key",
			}
		}

		if err := ValidateValue(value); err != nil {
			return &SecurityError{
				Type:   "InvalidValue",
				Field:  "",
				Detail: "invalid map value",
			}
		}
	}

	return nil
}

// validateBasicValue validates basic types (int, float, bool)
func validateBasicValue(value any) error {
	// Basic type validation - these are generally safe
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return nil
	case uint, uint8, uint16, uint32, uint64:
		return nil
	case float32, float64:
		return nil
	case bool:
		return nil
	case time.Time, *time.Time:
		return nil
	default:
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "unsupported value type",
		}
	}
}

// ValidateExpression validates a complete expression for security
func ValidateExpression(expression string) error {
	if len(expression) > MaxExpressionLength {
		return &SecurityError{
			Type:   "InvalidExpression",
			Field:  "",
			Detail: "expression exceeds maximum length",
		}
	}

	// Check for dangerous patterns
	exprLower := strings.ToLower(expression)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(exprLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "expression contains dangerous pattern",
			}
		}
	}

	// Check for SQL injection patterns in expressions
	sqlInjectionPatterns := []string{
		"union select", "insert into", "update set", "delete from",
		"drop table", "alter table", "exec ", "execute ",
	}

	for _, pattern := range sqlInjectionPatterns {
		if strings.Contains(exprLower, pattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "expression contains dangerous pattern",
			}
		}
	}

	return nil
}

// ValidateTableName validates a DynamoDB table name
func ValidateTableName(name string) error {
	if len(name) < 3 || len(name) > 255 {
		return &SecurityError{
			Type:   "InvalidTableName",
			Field:  "",
			Detail: "table name length invalid",
		}
	}

	// AWS table name pattern
	pattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !pattern.MatchString(name) {
		return &SecurityError{
			Type:   "InvalidTableName",
			Field:  "",
			Detail: "table name contains invalid characters",
		}
	}

	// Check for dangerous patterns
	nameLower := strings.ToLower(name)
	for _, dangerousPattern := range dangerousPatterns {
		if strings.Contains(nameLower, dangerousPattern) {
			return &SecurityError{
				Type:   "InjectionAttempt",
				Field:  "",
				Detail: "table name contains dangerous pattern",
			}
		}
	}

	return nil
}

// ValidateIndexName validates a DynamoDB index name
func ValidateIndexName(name string) error {
	if name == "" {
		return nil // Empty index name is allowed (means no index)
	}

	if len(name) < 3 || len(name) > 255 {
		return &SecurityError{
			Type:   "InvalidIndexName",
			Field:  "",
			Detail: "index name length invalid",
		}
	}

	// AWS index name pattern (similar to table name)
	pattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !pattern.MatchString(name) {
		return &SecurityError{
			Type:   "InvalidIndexName",
			Field:  "",
			Detail: "index name contains invalid characters",
		}
	}

	return nil
}

// validateSliceLength validates slice lengths for typed slices
func validateSliceLength(length int) error {
	if length > 100 { // DynamoDB IN operator limit
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "slice value exceeds maximum length",
		}
	}

	return nil
}

// validateMapLength validates map lengths
func validateMapLength(length int) error {
	if length > 100 { // Reasonable limit for map size
		return &SecurityError{
			Type:   "InvalidValue",
			Field:  "",
			Detail: "map value exceeds maximum keys",
		}
	}

	return nil
}
