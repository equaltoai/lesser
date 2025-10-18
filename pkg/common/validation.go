package common // nolint:revive // "common" package name is acceptable for shared utilities

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/errors"
)

// Username validation patterns
var (
	// UsernamePattern is the regex pattern for valid usernames
	// Alphanumeric characters, underscores, and hyphens, 1-30 chars
	UsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,30}$`)

	// StatusIDPattern is the regex pattern for valid status IDs
	// Should be numeric or UUID-like format
	StatusIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// ValidateUsername validates a username according to platform rules
func ValidateUsername(username string) error {
	if username == "" {
		return ValidationError{Field: "username", Message: "cannot be blank"}
	}

	if len(username) > 30 {
		return ValidationError{Field: "username", Message: "cannot be longer than 30 characters"}
	}

	if !UsernamePattern.MatchString(username) {
		return ValidationError{Field: "username", Message: "can only contain letters, numbers, underscores, and hyphens"}
	}

	return nil
}

// ValidateStatusID validates a status ID format
func ValidateStatusID(statusID string) error {
	if statusID == "" {
		return ValidationError{Field: "status_id", Message: "cannot be blank"}
	}

	if len(statusID) > 100 {
		return ValidationError{Field: "status_id", Message: "cannot be longer than 100 characters"}
	}

	if !StatusIDPattern.MatchString(statusID) {
		return ValidationError{Field: "status_id", Message: "contains invalid characters"}
	}

	return nil
}

// ValidateAccountID validates an account ID (can be username, numeric ID, or URL)
func ValidateAccountID(accountID string) error {
	if accountID == "" {
		return ValidationError{Field: "account_id", Message: "cannot be blank"}
	}

	if len(accountID) > 500 {
		return ValidationError{Field: "account_id", Message: "cannot be longer than 500 characters"}
	}

	return nil
}

// ValidateLimit validates a limit parameter with bounds checking
func ValidateLimit(limit int, maxValue int) error {
	if limit < 0 {
		return ValidationError{Field: "limit", Message: "cannot be negative"}
	}

	if limit > maxValue {
		return ValidationError{Field: "limit", Message: fmt.Sprintf("cannot be greater than %d", maxValue)}
	}

	return nil
}

// ValidateOffset validates an offset parameter
func ValidateOffset(offset int) error {
	if offset < 0 {
		return ValidationError{Field: "offset", Message: "cannot be negative"}
	}

	return nil
}

// ParseAndValidateBoolean parses a boolean parameter from string
func ParseAndValidateBoolean(value string) (bool, error) {
	if value == "" {
		return false, nil
	}

	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case StringTrue, "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, ValidationError{Field: "boolean", Message: "must be true/false, 1/0, yes/no, or on/off"}
	}
}

// ValidateRequiredParam validates that a required parameter is not empty
func ValidateRequiredParam(paramName, value string) error {
	if value == "" {
		return ValidationError{Field: paramName, Message: "is required"}
	}
	return nil
}

// ValidateMultipleRequiredParams validates multiple required parameters at once
func ValidateMultipleRequiredParams(params map[string]string) error {
	var missing []string

	for name, value := range params {
		if value == "" {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return ValidationError{Field: "parameters", Message: fmt.Sprintf("missing required parameters: %s", strings.Join(missing, ", "))}
	}

	return nil
}

// ValidateStringLength validates string length within bounds
func ValidateStringLength(field, value string, minLen, maxLen int) error {
	length := len(value)

	if length < minLen {
		return ValidationError{Field: field, Message: fmt.Sprintf("must be at least %d characters long", minLen)}
	}

	if length > maxLen {
		return ValidationError{Field: field, Message: fmt.Sprintf("cannot be longer than %d characters", maxLen)}
	}

	return nil
}

// ValidateEnum validates that a value is within a set of allowed values
func ValidateEnum(field, value string, allowedValues []string) error {
	if value == "" {
		return nil // Allow empty values - use ValidateRequiredParam for required validation
	}

	for _, allowed := range allowedValues {
		if value == allowed {
			return nil
		}
	}

	return ValidationError{Field: field, Message: fmt.Sprintf("must be one of: %s", strings.Join(allowedValues, ", "))}
}

// ParseAndValidateIntWithBounds parses a string to int and validates bounds
// This consolidates the common pattern: strconv.Atoi(str); err == nil && val > min && val <= max
func ParseAndValidateIntWithBounds(field, value string, minValue, maxValue, defaultValue int) (int, error) {
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, ValidationError{Field: field, Message: "must be a valid number"}
	}

	if parsed <= minValue {
		return 0, ValidationError{Field: field, Message: fmt.Sprintf("must be greater than %d", minValue)}
	}

	if parsed > maxValue {
		return 0, ValidationError{Field: field, Message: fmt.Sprintf("cannot be greater than %d", maxValue)}
	}

	return parsed, nil
}

// ParseAndValidateAPILimit parses and validates common API limit parameters
// This consolidates the very common pattern in API handlers
func ParseAndValidateAPILimit(limitStr string, maxLimit int) (int, error) {
	defaultLimit := 20
	if maxLimit <= 20 {
		defaultLimit = maxLimit / 2
	}

	return ParseAndValidateIntWithBounds("limit", limitStr, 0, maxLimit, defaultLimit)
}

// Common API limit validators for different endpoint types
var (
	// Standard limits used across the API
	StandardTimelineLimit   = 40
	StandardFollowLimit     = 80
	StandardSearchLimit     = 80
	StandardHashtagLimit    = 200
	StandardAdminLimit      = 100
	StandardFederationLimit = 200
)

// ParseTimelineLimit parses limit for timeline endpoints (max 40)
func ParseTimelineLimit(limitStr string) (int, error) {
	return ParseAndValidateAPILimit(limitStr, StandardTimelineLimit)
}

// ParseFollowLimit parses limit for follow/follower endpoints (max 80)
func ParseFollowLimit(limitStr string) (int, error) {
	return ParseAndValidateAPILimit(limitStr, StandardFollowLimit)
}

// ParseSearchLimit parses limit for search endpoints (max 80)
func ParseSearchLimit(limitStr string) (int, error) {
	return ParseAndValidateAPILimit(limitStr, StandardSearchLimit)
}

// ParseHashtagLimit parses limit for hashtag endpoints (max 200)
func ParseHashtagLimit(limitStr string) (int, error) {
	return ParseAndValidateAPILimit(limitStr, StandardHashtagLimit)
}

// ParseAdminLimit parses limit for admin endpoints (max 100)
func ParseAdminLimit(limitStr string) (int, error) {
	return ParseAndValidateAPILimit(limitStr, StandardAdminLimit)
}

// ParseFederationLimit parses limit for federation endpoints (max 200)
func ParseFederationLimit(limitStr string) (int, error) {
	return ParseAndValidateAPILimit(limitStr, StandardFederationLimit)
}

// SanitizeInput performs basic input sanitization
func SanitizeInput(input string) string {
	// Trim whitespace
	cleaned := strings.TrimSpace(input)

	// Remove null bytes
	cleaned = strings.ReplaceAll(cleaned, "\x00", "")

	// Remove carriage returns (keep newlines)
	cleaned = strings.ReplaceAll(cleaned, "\r", "")

	return cleaned
}

// ValidateAndSanitizeString validates string length and sanitizes content
func ValidateAndSanitizeString(field, value string, minLen, maxLen int) (string, error) {
	// First sanitize
	cleaned := SanitizeInput(value)

	// Then validate length
	if err := ValidateStringLength(field, cleaned, minLen, maxLen); err != nil {
		return "", err
	}

	return cleaned, nil
}

// ValidateUUID validates a UUID format
func ValidateUUID(field, value string) error {
	if value == "" {
		return ValidationError{Field: field, Message: "cannot be blank"}
	}

	// Simple UUID pattern - can be enhanced
	uuidPattern := regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
	if !uuidPattern.MatchString(strings.ToLower(value)) {
		return ValidationError{Field: field, Message: "must be a valid UUID"}
	}

	return nil
}

// ValidateNumericID validates a numeric ID (string representation)
func ValidateNumericID(field, value string) error {
	if value == "" {
		return ValidationError{Field: field, Message: "cannot be blank"}
	}

	_, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return ValidationError{Field: field, Message: "must be a valid numeric ID"}
	}

	return nil
}

// ValidateAlphanumericID validates an alphanumeric ID
func ValidateAlphanumericID(field, value string) error {
	if value == "" {
		return ValidationError{Field: field, Message: "cannot be blank"}
	}

	if len(value) > 100 {
		return ValidationError{Field: field, Message: "cannot be longer than 100 characters"}
	}

	alphanumericPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !alphanumericPattern.MatchString(value) {
		return ValidationError{Field: field, Message: "can only contain letters, numbers, underscores, and hyphens"}
	}

	return nil
}

// ValidateAccountParamID validates account ID from URL parameters (commonly used in API handlers)
func ValidateAccountParamID(accountID string) error {
	if accountID == "" {
		return ValidationError{Field: "account_id", Message: "is required"}
	}

	// Account IDs can be usernames, numeric IDs, or URLs - just check length
	if len(accountID) > 500 {
		return ValidationError{Field: "account_id", Message: "cannot be longer than 500 characters"}
	}

	return nil
}

// ValidateUsernameParamID validates username from URL parameters
func ValidateUsernameParamID(username string) error {
	if username == "" {
		return ValidationError{Field: "username", Message: "is required"}
	}

	// Use existing username validation
	return ValidateUsername(username)
}

// ValidateAcctParameter validates the 'acct' parameter used in account lookups
func ValidateAcctParameter(acct string) error {
	if acct == "" {
		return ValidationError{Field: "acct", Message: "parameter is required"}
	}

	// Basic validation for acct format (username@domain or just username)
	if len(acct) > 500 {
		return ValidationError{Field: "acct", Message: "cannot be longer than 500 characters"}
	}

	return nil
}

// ParseAndValidateActivityPubLimit parses and validates limits for ActivityPub collections
func ParseAndValidateActivityPubLimit(limitStr string) (int, error) {
	return ParseAndValidateIntWithBounds("limit", limitStr, 1, 100, 20)
}

// ValidateAccountIDsParameter validates comma-separated account IDs (used in familiar followers)
func ValidateAccountIDsParameter(accountIDsStr string) ([]string, error) {
	if accountIDsStr == "" {
		return nil, ValidationError{Field: "id[]", Message: "parameter is required"}
	}

	// Split account IDs
	ids := strings.Split(accountIDsStr, ",")
	if len(ids) == 0 {
		return nil, ValidationError{Field: "id[]", Message: "at least one account ID is required"}
	}

	// Validate each ID
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, ValidationError{Field: fmt.Sprintf("id[%d]", i), Message: "cannot be blank"}
		}
		ids[i] = id
	}

	return ids, nil
}

// ValidateStatusParamID validates status ID from URL parameters
func ValidateStatusParamID(statusID string) error {
	if statusID == "" {
		return ValidationError{Field: "status_id", Message: "is required"}
	}

	// Use existing status ID validation
	return ValidateStatusID(statusID)
}

// ParseStatusContextLimit parses and validates limits for status context endpoints
func ParseStatusContextLimit(limitStr string) (int, error) {
	return ParseAndValidateIntWithBounds("limit", limitStr, 0, 80, 20)
}

// ValidateVisibility validates status visibility values
func ValidateVisibility(visibility string) error {
	if visibility == "" {
		return nil // Allow empty - will default to public
	}

	allowedValues := []string{"public", "unlisted", "private", "direct"}
	return ValidateEnum("visibility", visibility, allowedValues)
}

// IsPubliclyVisible checks if content is visible to public (public or unlisted)
func IsPubliclyVisible(visibility string) bool {
	return visibility == "public" || visibility == "unlisted"
}

// ValidateStatusContent validates status content length and format
func ValidateStatusContent(content string) error {
	if content == "" {
		return ValidationError{Field: "status", Message: "cannot be blank"}
	}

	// Validate length (common Mastodon limit is 500 characters)
	if len(content) > 500 {
		return ValidationError{Field: "status", Message: "cannot be longer than 500 characters"}
	}

	return nil
}

// ValidateLanguageCode validates language codes (ISO 639-1 format)
func ValidateLanguageCode(language string) error {
	if language == "" {
		return nil // Allow empty
	}

	// Basic validation for ISO 639-1 language codes (2 characters)
	if len(language) != 2 {
		return ValidationError{Field: "language", Message: "must be a valid ISO 639-1 language code"}
	}

	return nil
}

// ParseStatusTimelineLimit parses and validates limits for status timelines (home, public)
func ParseStatusTimelineLimit(limitStr string) (int, error) {
	// Special behavior: returns defaults/caps on errors rather than failing
	if limitStr == "" {
		return 20, nil
	}

	result, err := ParseAndValidateIntWithBounds("limit", limitStr, 0, 80, 20)
	if err != nil {
		// For timeline endpoints, return default on parse error instead of failing
		return 20, nil
	}

	return result, nil
}

// ParseAccountStatusesLimit parses and validates limits for account status listings
func ParseAccountStatusesLimit(limitStr string) (int, error) {
	// Special behavior: returns defaults/caps on errors rather than failing
	if limitStr == "" {
		return 20, nil
	}

	result, err := ParseAndValidateIntWithBounds("limit", limitStr, 0, 40, 20)
	if err != nil {
		// For account status endpoints, return default on parse error instead of failing
		return 20, nil
	}

	return result, nil
}

// ValidateFilterParamID validates filter ID from URL parameters
func ValidateFilterParamID(filterID string) error {
	if filterID == "" {
		return ValidationError{Field: "filter_id", Message: "is required"}
	}

	// Filter IDs should be alphanumeric or UUID-like
	if len(filterID) > 100 {
		return ValidationError{Field: "filter_id", Message: "cannot be longer than 100 characters"}
	}

	return nil
}

// ValidateKeywordParamID validates keyword ID from URL parameters
func ValidateKeywordParamID(keywordID string) error {
	if keywordID == "" {
		return ValidationError{Field: "keyword_id", Message: "is required"}
	}

	if len(keywordID) > 100 {
		return ValidationError{Field: "keyword_id", Message: "cannot be longer than 100 characters"}
	}

	return nil
}

// ValidateFilterTitle validates filter title
func ValidateFilterTitle(title string) error {
	if title == "" {
		return ValidationError{Field: "title", Message: "cannot be blank"}
	}

	if len(title) > 200 {
		return ValidationError{Field: "title", Message: "cannot be longer than 200 characters"}
	}

	return nil
}

// ValidateFilterKeyword validates filter keyword
func ValidateFilterKeyword(keyword string) error {
	if keyword == "" {
		return ValidationError{Field: "keyword", Message: "cannot be blank"}
	}

	if len(keyword) > 500 {
		return ValidationError{Field: "keyword", Message: "cannot be longer than 500 characters"}
	}

	return nil
}

// ValidateFilterContext validates filter context values
func ValidateFilterContext(contexts []string) error {
	if len(contexts) == 0 {
		return ValidationError{Field: "context", Message: "cannot be blank"}
	}

	validContexts := []string{"home", "notifications", "public", "thread", "account"}

	for _, context := range contexts {
		if err := ValidateEnum("context", context, validContexts); err != nil {
			return err
		}
	}

	return nil
}

// ValidateFilterAction validates filter action
func ValidateFilterAction(action string) error {
	if action == "" {
		return nil // Allow empty - will default to "warn"
	}

	allowedActions := []string{"warn", "hide", "blur"}
	return ValidateEnum("filter_action", action, allowedActions)
}

// ValidateSearchQuery validates search query parameters
func ValidateSearchQuery(query string) error {
	if query == "" {
		return ValidationError{Field: "q", Message: "parameter is required"}
	}

	// Search queries should have reasonable length limits
	if len(query) > 500 {
		return ValidationError{Field: "q", Message: "cannot be longer than 500 characters"}
	}

	return nil
}

// ParseSearchOffset parses and validates search offset parameters
func ParseSearchOffset(offsetStr string) (int, error) {
	return ParseAndValidateIntWithBounds("offset", offsetStr, -1, 10000, 0)
}

// GetEnvInt gets an integer environment variable with a default value
// DEPRECATED: Use centralized config.Get() for application configuration instead
// This consolidates the common pattern: strconv.Atoi(os.Getenv(key)); err == nil
// Only use this for AWS Lambda runtime variables or when config is not available
func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// Repository-specific validation utilities

// ValidateEntityID validates entity IDs used in repository operations
func ValidateEntityID(id string, entityType string) error {
	if id == "" {
		return ValidationError{Field: entityType + "_id", Message: "cannot be blank"}
	}

	if len(id) > 500 {
		return ValidationError{Field: entityType + "_id", Message: "cannot be longer than 500 characters"}
	}

	return nil
}

// ValidateEntityIDsList validates a list of entity IDs
func ValidateEntityIDsList(ids []string, entityType string) error {
	if len(ids) == 0 {
		return ValidationError{Field: entityType + "_ids", Message: "at least one ID is required"}
	}

	for i, id := range ids {
		if err := ValidateEntityID(id, entityType); err != nil {
			return ValidationError{Field: fmt.Sprintf("%s_id[%d]", entityType, i), Message: err.Error()}
		}
	}

	return nil
}

// ValidateTimestamp validates timestamp formats used in repositories
func ValidateTimestamp(timestamp string, fieldName string) error {
	if timestamp == "" {
		return nil // Allow empty timestamps
	}

	// Try parsing as RFC3339 (standard format)
	if _, err := parseTimestampFormat(timestamp); err != nil {
		return ValidationError{Field: fieldName, Message: "must be a valid timestamp in RFC3339 format"}
	}

	return nil
}

// parseTimestampFormat attempts to parse timestamp in common formats
func parseTimestampFormat(timestamp string) (interface{}, error) {
	// Try RFC3339 first
	if _, err := time.Parse(time.RFC3339, timestamp); err == nil {
		return true, nil
	}

	// Try common date format
	if _, err := time.Parse("2006-01-02", timestamp); err == nil {
		return true, nil
	}

	return nil, errors.TimestampInvalidFormat(timestamp)
}

// ValidateURL validates URL formats used in repositories
func ValidateURL(url string, fieldName string) error {
	if url == "" {
		return nil // Allow empty URLs unless specifically required
	}

	// Basic URL validation - check for common patterns
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ValidationError{Field: fieldName, Message: "must be a valid HTTP or HTTPS URL"}
	}

	if len(url) > 2000 {
		return ValidationError{Field: fieldName, Message: "cannot be longer than 2000 characters"}
	}

	return nil
}

// ValidateJSONField validates JSON string fields in repository models
func ValidateJSONField(jsonData string, fieldName string) error {
	if jsonData == "" {
		return nil // Allow empty JSON fields
	}

	// Basic JSON validation - check if it's valid JSON
	var temp interface{}
	if err := json.Unmarshal([]byte(jsonData), &temp); err != nil {
		return ValidationError{Field: fieldName, Message: "must be valid JSON"}
	}

	return nil
}

// ValidateEnumField validates enum values used in repository models
func ValidateEnumField(value string, allowedValues []string, fieldName string) error {
	if value == "" {
		return nil // Allow empty values unless specifically required
	}

	for _, allowed := range allowedValues {
		if value == allowed {
			return nil
		}
	}

	return ValidationError{Field: fieldName, Message: fmt.Sprintf("must be one of: %s", strings.Join(allowedValues, ", "))}
}

// ValidateQueryLimit validates query limits with repository-specific maximums
func ValidateQueryLimit(limit int, maxLimit int, operationType string) error {
	if limit < 0 {
		return ValidationError{Field: "limit", Message: "cannot be negative"}
	}

	if limit > maxLimit {
		return ValidationError{Field: "limit", Message: fmt.Sprintf("cannot be greater than %d for %s operations", maxLimit, operationType)}
	}

	return nil
}

// ValidateQueryOffset validates query offsets for repository operations
func ValidateQueryOffset(offset int, maxOffset int) error {
	if offset < 0 {
		return ValidationError{Field: "offset", Message: "cannot be negative"}
	}

	if offset > maxOffset {
		return ValidationError{Field: "offset", Message: fmt.Sprintf("cannot be greater than %d", maxOffset)}
	}

	return nil
}

// ValidateQueryFilters validates filter parameters for repository queries
func ValidateQueryFilters(filters map[string]interface{}) error {
	for key, value := range filters {
		// Validate filter key names
		if key == "" {
			return ValidationError{Field: "filter", Message: "filter keys cannot be empty"}
		}

		// Basic validation for filter values
		if value == nil {
			return ValidationError{Field: key, Message: "filter value cannot be null"}
		}

		// Convert to string for length checking
		if str, ok := value.(string); ok && len(str) > 500 {
			return ValidationError{Field: key, Message: "filter value cannot be longer than 500 characters"}
		}
	}

	return nil
}

// ValidateSortParameters validates sort field and order parameters
func ValidateSortParameters(sortBy, sortOrder string, allowedFields []string) error {
	if sortBy != "" {
		found := false
		for _, field := range allowedFields {
			if sortBy == field {
				found = true
				break
			}
		}

		if !found {
			return ValidationError{Field: "sort_by", Message: fmt.Sprintf("must be one of: %s", strings.Join(allowedFields, ", "))}
		}
	}

	if sortOrder != "" && sortOrder != "asc" && sortOrder != "desc" {
		return ValidationError{Field: "sort_order", Message: "must be 'asc' or 'desc'"}
	}

	return nil
}

// ValidateRepositoryRelationship validates relationships between entities
func ValidateRepositoryRelationship(fromEntity, toEntity string, relationType string) error {
	if fromEntity == "" {
		return ValidationError{Field: "from_entity", Message: "cannot be blank"}
	}

	if toEntity == "" {
		return ValidationError{Field: "to_entity", Message: "cannot be blank"}
	}

	if fromEntity == toEntity && relationType != "self_reference" {
		return ValidationError{Field: "relationship", Message: "entity cannot have relationship with itself"}
	}

	return nil
}

// ValidateRepositoryAccess validates access permissions for repository operations
func ValidateRepositoryAccess(userID, resourceID string, operation string) error {
	if userID == "" {
		return ValidationError{Field: "user_id", Message: "is required for access validation"}
	}

	if resourceID == "" {
		return ValidationError{Field: "resource_id", Message: "is required for access validation"}
	}

	if operation == "" {
		return ValidationError{Field: "operation", Message: "is required for access validation"}
	}

	validOperations := []string{"read", "write", "delete", "admin"}
	return ValidateEnumField(operation, validOperations, "operation")
}

// Content-specific validation functions

// ValidateDisplayName validates display name length
func ValidateDisplayName(displayName string) error {
	if len(displayName) > 30 {
		return ValidationError{Field: "display_name", Message: "cannot be longer than 30 characters"}
	}
	return nil
}

// ValidateAccountBio validates account bio length
func ValidateAccountBio(bio string) error {
	if len(bio) > 500 {
		return ValidationError{Field: "bio", Message: "cannot be longer than 500 characters"}
	}
	return nil
}

// ValidateBooleanString validates boolean-like strings
func ValidateBooleanString(value string) bool {
	return value == StringTrue || value == "1" || value == "yes"
}

// ValidateHTTPScheme validates HTTP/HTTPS schemes
func ValidateHTTPScheme(scheme string) error {
	if scheme != SchemeHTTP && scheme != SchemeHTTPS {
		return ValidationError{Field: "scheme", Message: "must be http or https"}
	}
	return nil
}

// ValidateMediaType validates media attachment types
func ValidateMediaType(mediaType string) error {
	validTypes := []string{"Image", "Video", "Document", "Audio"}
	return ValidateEnumField(mediaType, validTypes, "media_type")
}

// IsProcessableMediaType checks if media type can be processed (Image, Video, Document)
func IsProcessableMediaType(mediaType string) bool {
	return mediaType == "Image" || mediaType == "Video" || mediaType == "Document"
}

// ValidateStatusState validates status states (pending, completed, failed, etc.)
func ValidateStatusState(status string) error {
	validStates := []string{"pending", "completed", "failed", "processing", "ready"}
	return ValidateEnumField(status, validStates, "status")
}

// ValidateSliceNotEmpty validates that a slice is not empty
func ValidateSliceNotEmpty(fieldName string, slice interface{}) error {
	// Use reflection to handle any slice type
	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice {
		return ValidationError{Field: fieldName, Message: "expected a slice"}
	}

	if v.Len() == 0 {
		return ValidationError{Field: fieldName, Message: "cannot be empty"}
	}

	return nil
}

// ValidateSliceLength validates slice length against bounds
func ValidateSliceLength(fieldName string, slice interface{}, maxLength int) error {
	// Use reflection to handle any slice type
	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice {
		return ValidationError{Field: fieldName, Message: "expected a slice"}
	}

	length := v.Len()
	if length > maxLength {
		return ValidationError{Field: fieldName, Message: fmt.Sprintf("cannot contain more than %d items", maxLength)}
	}
	return nil
}

// Repository-specific entity validation

// ValidateUserEntity validates user entity data before repository operations
func ValidateUserEntity(username, email string) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}

	// Email validation (basic)
	if email != "" && !strings.Contains(email, "@") {
		return ValidationError{Field: "email", Message: "must be a valid email address"}
	}

	return nil
}

// ValidateActorEntity validates actor entity data before repository operations
func ValidateActorEntity(actorID, username string) error {
	if err := ValidateEntityID(actorID, "actor"); err != nil {
		return err
	}

	if err := ValidateUsername(username); err != nil {
		return err
	}

	return nil
}

// ValidateMediaEntity validates media entity data before repository operations
func ValidateMediaEntity(mediaID, filename string, fileSize int64) error {
	if err := ValidateEntityID(mediaID, "media"); err != nil {
		return err
	}

	if filename == "" {
		return ValidationError{Field: "filename", Message: "cannot be blank"}
	}

	if fileSize <= 0 {
		return ValidationError{Field: "file_size", Message: "must be greater than 0"}
	}

	// Reasonable file size limit (100MB)
	if fileSize > 100*1024*1024 {
		return ValidationError{Field: "file_size", Message: "cannot be larger than 100MB"}
	}

	return nil
}

// ValidateStatusEntity validates status entity data before repository operations
func ValidateStatusEntity(statusID, content string, visibility string) error {
	if err := ValidateEntityID(statusID, "status"); err != nil {
		return err
	}

	if err := ValidateStatusContent(content); err != nil {
		return err
	}

	if err := ValidateVisibility(visibility); err != nil {
		return err
	}

	return nil
}

// Repository query parameter validation

// ValidateRepositorySearchQuery validates search queries for repository operations
func ValidateRepositorySearchQuery(query string, minLength int) error {
	if query == "" {
		return ValidationError{Field: "query", Message: "cannot be blank"}
	}

	// Normalize query
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))

	if len(normalizedQuery) < minLength {
		return ValidationError{Field: "query", Message: fmt.Sprintf("must be at least %d characters long", minLength)}
	}

	if len(normalizedQuery) > 500 {
		return ValidationError{Field: "query", Message: "cannot be longer than 500 characters"}
	}

	return nil
}

// ValidateRepositoryCursor validates cursor parameters for pagination
func ValidateRepositoryCursor(cursor string) error {
	if cursor == "" {
		return nil // Empty cursor is valid (start from beginning)
	}

	if len(cursor) > 500 {
		return ValidationError{Field: "cursor", Message: "cannot be longer than 500 characters"}
	}

	// Cursor should be base64-encoded
	if _, err := base64.StdEncoding.DecodeString(cursor); err != nil {
		return ValidationError{Field: "cursor", Message: "must be a valid base64-encoded cursor"}
	}

	return nil
}

// Repository-specific validation patterns

// ValidateNormalizedQuery validates and normalizes search queries
func ValidateNormalizedQuery(query string) (string, error) {
	if err := ValidateRepositorySearchQuery(query, 1); err != nil {
		return "", err
	}

	// Normalize the query
	normalized := strings.ToLower(strings.TrimSpace(query))
	return normalized, nil
}

// ValidatePreferenceValue validates user preference values
func ValidatePreferenceValue(key string, value interface{}) error {
	if key == "" {
		return ValidationError{Field: "preference_key", Message: "cannot be blank"}
	}

	if value == nil {
		return ValidationError{Field: key, Message: "preference value cannot be null"}
	}

	// Type-specific validation based on common preference patterns
	switch v := value.(type) {
	case string:
		if len(v) > 1000 {
			return ValidationError{Field: key, Message: "string preference cannot be longer than 1000 characters"}
		}
	case bool:
		// Bool values are always valid
	case map[string]interface{}:
		// Validate nested map values recursively
		for nestedKey, nestedValue := range v {
			if err := ValidatePreferenceValue(nestedKey, nestedValue); err != nil {
				return err
			}
		}
	default:
		// Allow other types but convert to string for validation
		if str := fmt.Sprintf("%v", value); len(str) > 1000 {
			return ValidationError{Field: key, Message: "preference value too large"}
		}
	}

	return nil
}

// ValidateFloatRange validates that a float value is within the specified range
func ValidateFloatRange(field string, value, minValue, maxValue float64) error {
	if value < minValue || value > maxValue {
		return ValidationError{Field: field, Message: fmt.Sprintf("must be between %.2f and %.2f", minValue, maxValue)}
	}

	return nil
}

// ValidateIntRange validates that an int value is within the specified range
func ValidateIntRange(field string, value, minValue, maxValue int) error {
	if value < minValue || value > maxValue {
		return ValidationError{Field: field, Message: fmt.Sprintf("must be between %d and %d", minValue, maxValue)}
	}

	return nil
}

// ValidateContentOrAttachments validates that either content or attachments are provided
func ValidateContentOrAttachments(content string, attachmentIDs []string) error {
	if len(content) == 0 && len(attachmentIDs) == 0 {
		return ValidationError{Field: "content", Message: "content or media attachments required"}
	}
	return nil
}
