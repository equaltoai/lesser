package middleware

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pay-theory/lift/pkg/lift"
)

// ValidationConfig configures input validation middleware
type ValidationConfig struct {
	CustomValidators         map[string]func(string) error `json:"-"`
	AllowedContentTypes      []string                      `json:"allowed_content_types"`
	BlockedUserAgents        []string                      `json:"blocked_user_agents"`
	MaxBodySize              int64                         `json:"max_body_size"`
	MaxHeaderSize            int                           `json:"max_header_size"`
	MaxQueryParamSize        int                           `json:"max_query_param_size"`
	MaxPathParamSize         int                           `json:"max_path_param_size"`
	EnableSQLInjectionCheck  bool                          `json:"enable_sql_injection_check"`
	EnableXSSCheck           bool                          `json:"enable_xss_check"`
	EnablePathTraversalCheck bool                          `json:"enable_path_traversal_check"`
}

// DefaultValidationConfig returns a secure default configuration
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxBodySize:       1024 * 1024, // 1MB
		MaxHeaderSize:     8192,        // 8KB
		MaxQueryParamSize: 2048,        // 2KB
		MaxPathParamSize:  256,         // 256 bytes
		AllowedContentTypes: []string{
			"application/json",
			"application/x-www-form-urlencoded",
			"multipart/form-data",
			"text/plain",
		},
		BlockedUserAgents: []string{
			"sqlmap",
			"nikto",
			"nmap",
			"masscan",
			"zap",
			"burp",
		},
		CustomValidators:         make(map[string]func(string) error),
		EnableSQLInjectionCheck:  true,
		EnableXSSCheck:           true,
		EnablePathTraversalCheck: true,
	}
}

// InputValidation creates comprehensive input validation middleware
func InputValidation(config ValidationConfig) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if err := validateRequest(ctx, config); err != nil {
				return err
			}
			return next.Handle(ctx)
		})
	}
}

// validateRequest performs comprehensive request validation
func validateRequest(ctx *lift.Context, config ValidationConfig) error {
	if ctx.Request == nil {
		return lift.ParameterError("request", "Request is nil")
	}

	// Validate request size
	if err := validateRequestSize(ctx, config); err != nil {
		return err
	}

	// Validate content type
	if err := validateContentType(ctx, config); err != nil {
		return err
	}

	// Validate headers
	if err := validateHeaders(ctx, config); err != nil {
		return err
	}

	// Validate user agent
	if err := validateUserAgent(ctx, config); err != nil {
		return err
	}

	// Validate query parameters
	if err := validateQueryParams(ctx, config); err != nil {
		return err
	}

	// Validate path parameters
	if err := validatePathParams(ctx, config); err != nil {
		return err
	}

	// Validate request body
	return validateRequestBody(ctx, config)
}

// validateRequestSize validates the request body size
func validateRequestSize(ctx *lift.Context, config ValidationConfig) error {
	if ctx.Request.Body == nil {
		return nil
	}

	bodySize := int64(len(ctx.Request.Body))
	if bodySize > config.MaxBodySize {
		return lift.ParameterError("body", fmt.Sprintf("Request body too large: %d bytes (max: %d)", bodySize, config.MaxBodySize))
	}

	return nil
}

// validateContentType validates the request content type
func validateContentType(ctx *lift.Context, config ValidationConfig) error {
	if len(config.AllowedContentTypes) == 0 {
		return nil // No restrictions
	}

	contentType := ctx.Header(lift.HeaderContentType)
	if contentType == "" {
		return nil // No content type specified
	}

	// Parse content type (remove charset and other parameters)
	mediaType := strings.Split(contentType, ";")[0]
	mediaType = strings.TrimSpace(mediaType)

	for _, allowed := range config.AllowedContentTypes {
		if strings.EqualFold(mediaType, allowed) {
			return nil
		}
	}

	return lift.ParameterError("content-type", fmt.Sprintf("Content type not allowed: %s", mediaType))
}

// validateHeaders validates all request headers
func validateHeaders(ctx *lift.Context, config ValidationConfig) error {
	if ctx.Request.Headers == nil {
		return nil
	}

	for key, value := range ctx.Request.Headers {
		// Check header size
		if len(value) > config.MaxHeaderSize {
			return lift.ParameterError("header."+key, fmt.Sprintf("Header '%s' too large: %d bytes (max: %d)", key, len(value), config.MaxHeaderSize))
		}

		// Check for malicious patterns
		if err := validateStringContent(value, config); err != nil {
			return lift.ParameterError("header."+key, fmt.Sprintf("Invalid header '%s': %v", key, err))
		}

		// Validate UTF-8 encoding
		if !utf8.ValidString(value) {
			return lift.ParameterError("header."+key, fmt.Sprintf("Header '%s' contains invalid UTF-8", key))
		}
	}

	return nil
}

// validateUserAgent checks for blocked user agents
func validateUserAgent(ctx *lift.Context, config ValidationConfig) error {
	userAgent := strings.ToLower(ctx.Header("User-Agent"))
	if userAgent == "" {
		return nil
	}

	for _, blocked := range config.BlockedUserAgents {
		if strings.Contains(userAgent, strings.ToLower(blocked)) {
			return lift.AuthorizationError(fmt.Sprintf("User agent blocked: %s", blocked))
		}
	}

	return nil
}

// validateQueryParams validates all query parameters
func validateQueryParams(ctx *lift.Context, config ValidationConfig) error {
	if ctx.Request.QueryParams == nil {
		return nil
	}

	for key, value := range ctx.Request.QueryParams {
		// Check parameter size
		if len(value) > config.MaxQueryParamSize {
			return lift.ParameterError("query."+key, fmt.Sprintf("Query parameter '%s' too large: %d bytes (max: %d)", key, len(value), config.MaxQueryParamSize))
		}

		// URL decode and validate
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return lift.ParameterError("query."+key, fmt.Sprintf("Invalid URL encoding in query parameter '%s': %v", key, err))
		}

		// Check for malicious patterns
		if err := validateStringContent(decoded, config); err != nil {
			return lift.ParameterError("query."+key, fmt.Sprintf("Invalid query parameter '%s': %v", key, err))
		}

		// Validate UTF-8 encoding
		if !utf8.ValidString(decoded) {
			return lift.ParameterError("query."+key, fmt.Sprintf("Query parameter '%s' contains invalid UTF-8", key))
		}

		// Apply custom validators
		if validator, exists := config.CustomValidators[key]; exists {
			if err := validator(decoded); err != nil {
				return lift.ParameterError("query."+key, fmt.Sprintf("Validation failed for query parameter '%s': %v", key, err))
			}
		}
	}

	return nil
}

// validatePathParams validates path parameters
func validatePathParams(ctx *lift.Context, config ValidationConfig) error {
	if ctx.Request.PathParams == nil {
		return nil
	}

	for key, value := range ctx.Request.PathParams {
		// Check parameter size
		if len(value) > config.MaxPathParamSize {
			return lift.ParameterError("path."+key, fmt.Sprintf("Path parameter '%s' too large: %d bytes (max: %d)", key, len(value), config.MaxPathParamSize))
		}

		// Check for malicious patterns
		if err := validateStringContent(value, config); err != nil {
			return lift.ParameterError("path."+key, fmt.Sprintf("Invalid path parameter '%s': %v", key, err))
		}

		// Validate UTF-8 encoding
		if !utf8.ValidString(value) {
			return lift.ParameterError("path."+key, fmt.Sprintf("Path parameter '%s' contains invalid UTF-8", key))
		}

		// Apply custom validators
		if validator, exists := config.CustomValidators[key]; exists {
			if err := validator(value); err != nil {
				return lift.ParameterError("path."+key, fmt.Sprintf("Validation failed for path parameter '%s': %v", key, err))
			}
		}
	}

	return nil
}

// validateRequestBody validates the request body content
func validateRequestBody(ctx *lift.Context, config ValidationConfig) error {
	if len(ctx.Request.Body) == 0 {
		return nil
	}

	bodyStr := string(ctx.Request.Body)

	// Validate UTF-8 encoding
	if !utf8.ValidString(bodyStr) {
		return lift.ParameterError("body", "Request body contains invalid UTF-8")
	}

	// Check for malicious patterns
	if err := validateStringContent(bodyStr, config); err != nil {
		return lift.ParameterError("body", fmt.Sprintf("Invalid request body: %v", err))
	}

	// If it's JSON, validate JSON structure
	contentType := ctx.Header(lift.HeaderContentType)
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		var js json.RawMessage
		if err := json.Unmarshal(ctx.Request.Body, &js); err != nil {
			return lift.ParameterError("body", fmt.Sprintf("Invalid JSON in request body: %v", err))
		}
	}

	return nil
}

// validateStringContent performs security checks on string content
func validateStringContent(content string, config ValidationConfig) error {
	// SQL Injection detection
	if config.EnableSQLInjectionCheck {
		if err := checkSQLInjection(content); err != nil {
			return err
		}
	}

	// XSS detection
	if config.EnableXSSCheck {
		if err := checkXSS(content); err != nil {
			return err
		}
	}

	// Path traversal detection
	if config.EnablePathTraversalCheck {
		if err := checkPathTraversal(content); err != nil {
			return err
		}
	}

	return nil
}

// checkSQLInjection detects potential SQL injection patterns
func checkSQLInjection(content string) error {
	// Convert to lowercase for case-insensitive matching
	lower := strings.ToLower(content)

	// Common SQL injection patterns
	sqlPatterns := []string{
		// SQL keywords
		"union select",
		"union all select",
		"' or '1'='1",
		"' or 1=1",
		"\" or \"1\"=\"1",
		"' or 'a'='a",
		"') or ('1'='1",
		"'; drop table",
		"'; delete from",
		"'; insert into",
		"'; update ",
		"exec(",
		"execute(",
		"sp_",
		"xp_",
		"@@version",
		"information_schema",
		"sysobjects",
		"syscolumns",
		// Comment injection
		"/*",
		"--",
		"#",
	}

	for _, pattern := range sqlPatterns {
		if strings.Contains(lower, pattern) {
			return fmt.Errorf("potential SQL injection detected: %s", pattern)
		}
	}

	// Regex patterns for more complex SQL injection attempts
	sqlRegexes := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bunion\s+select\b`),
		regexp.MustCompile(`(?i)\bselect\s+.*\bfrom\b`),
		regexp.MustCompile(`(?i)\binsert\s+into\b`),
		regexp.MustCompile(`(?i)\bdelete\s+from\b`),
		regexp.MustCompile(`(?i)\bupdate\s+.*\bset\b`),
		regexp.MustCompile(`(?i)\bdrop\s+table\b`),
		regexp.MustCompile(`(?i)'\s*or\s*'?\d+'?\s*=\s*'?\d+'?`),
		regexp.MustCompile(`(?i)"\s*or\s*"?\d+"?\s*=\s*"?\d+"?`),
	}

	for _, regex := range sqlRegexes {
		if regex.MatchString(content) {
			return fmt.Errorf("potential SQL injection detected: regex match")
		}
	}

	return nil
}

// checkXSS detects potential cross-site scripting patterns
func checkXSS(content string) error {
	// Convert to lowercase for case-insensitive matching
	lower := strings.ToLower(content)

	// Common XSS patterns
	xssPatterns := []string{
		"<script",
		"</script>",
		"javascript:",
		"vbscript:",
		"onload=",
		"onerror=",
		"onclick=",
		"onmouseover=",
		"onfocus=",
		"onblur=",
		"onchange=",
		"onsubmit=",
		"<iframe",
		"<object",
		"<embed",
		"<applet",
		"document.cookie",
		"document.domain",
		"window.location",
		"eval(",
		"expression(",
	}

	for _, pattern := range xssPatterns {
		if strings.Contains(lower, pattern) {
			return fmt.Errorf("potential XSS detected: %s", pattern)
		}
	}

	// Regex patterns for more complex XSS attempts
	xssRegexes := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<\s*script[^>]*>`),
		regexp.MustCompile(`(?i)<\s*iframe[^>]*>`),
		regexp.MustCompile(`(?i)<\s*object[^>]*>`),
		regexp.MustCompile(`(?i)<\s*embed[^>]*>`),
		regexp.MustCompile(`(?i)on\w+\s*=\s*["\']`),
		regexp.MustCompile(`(?i)javascript\s*:`),
		regexp.MustCompile(`(?i)expression\s*\(`),
		regexp.MustCompile(`(?i)@import\s+`),
	}

	for _, regex := range xssRegexes {
		if regex.MatchString(content) {
			return fmt.Errorf("potential XSS detected: regex match")
		}
	}

	return nil
}

// checkPathTraversal detects potential path traversal patterns
func checkPathTraversal(content string) error {
	// Path traversal patterns
	pathPatterns := []string{
		"../",
		"..\\",
		"..%2f",
		"..%5c",
		"%2e%2e%2f",
		"%2e%2e%5c",
		"....//",
		"....\\\\",
	}

	lower := strings.ToLower(content)
	for _, pattern := range pathPatterns {
		if strings.Contains(lower, pattern) {
			return fmt.Errorf("potential path traversal detected: %s", pattern)
		}
	}

	// Check for encoded path traversal attempts
	decoded, err := url.QueryUnescape(content)
	if err == nil && decoded != content {
		// Check decoded content for path traversal
		decodedLower := strings.ToLower(decoded)
		for _, pattern := range pathPatterns {
			if strings.Contains(decodedLower, pattern) {
				return fmt.Errorf("potential encoded path traversal detected: %s", pattern)
			}
		}
	}

	return nil
}

// Common validation helpers

// ValidateEmail validates email format
func ValidateEmail(email string) error {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// ValidateUUID validates UUID format
func ValidateUUID(uuid string) error {
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	if !uuidRegex.MatchString(uuid) {
		return fmt.Errorf("invalid UUID format")
	}
	return nil
}

// ValidateNumeric validates that a string contains only numeric characters
func ValidateNumeric(value string) error {
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return fmt.Errorf("must be numeric")
	}
	return nil
}

// ValidateAlphaNumeric validates that a string contains only alphanumeric characters
func ValidateAlphaNumeric(value string) error {
	alphaNumRegex := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	if !alphaNumRegex.MatchString(value) {
		return fmt.Errorf("must contain only alphanumeric characters")
	}
	return nil
}

// ValidateLength validates string length
func ValidateLength(minLen, maxLen int) func(string) error {
	return func(value string) error {
		length := len(value)
		if length < minLen {
			return fmt.Errorf("must be at least %d characters", minLen)
		}
		if length > maxLen {
			return fmt.Errorf("must be at most %d characters", maxLen)
		}
		return nil
	}
}
