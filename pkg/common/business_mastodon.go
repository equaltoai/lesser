// Package common provides Mastodon API-specific business logic utilities for the Lesser project.
// This consolidates Mastodon API business patterns found across the API layer to eliminate
// duplication and ensure consistency with Mastodon API specifications.
package common

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// MastodonAPIConfig configures Mastodon API behavior
type MastodonAPIConfig struct {
	InstanceName       string
	Domain             string
	MaxStatusLength    int
	MaxBioLength       int
	MaxDisplayName     int
	MaxPollOptions     int
	MaxPollExpiry      time.Duration
	MediaUploadLimit   int64
	VideoUploadLimit   int64
	DefaultPagination  int
	MaxPagination      int
}

// DefaultMastodonConfig returns sensible defaults for Mastodon API
func DefaultMastodonConfig() *MastodonAPIConfig {
	return &MastodonAPIConfig{
		InstanceName:      "Lesser",
		MaxStatusLength:   500,
		MaxBioLength:      160,
		MaxDisplayName:    30,
		MaxPollOptions:    4,
		MaxPollExpiry:     7 * 24 * time.Hour,
		MediaUploadLimit:  40 * 1024 * 1024,  // 40MB
		VideoUploadLimit:  100 * 1024 * 1024, // 100MB
		DefaultPagination: 20,
		MaxPagination:     80,
	}
}

// MastodonBusinessLogic provides centralized Mastodon API business operations
type MastodonBusinessLogic struct {
	config *MastodonAPIConfig
	logger *zap.Logger
}

// NewMastodonBusinessLogic creates a new Mastodon business logic service
func NewMastodonBusinessLogic(config *MastodonAPIConfig, logger *zap.Logger) *MastodonBusinessLogic {
	if config == nil {
		config = DefaultMastodonConfig()
	}
	return &MastodonBusinessLogic{
		config: config,
		logger: logger,
	}
}

// Common Mastodon API Business Patterns
// Based on analysis of Mastodon API endpoints and models

// 1. Pagination Pattern
// Found in: timelines, accounts, statuses, notifications

// MastodonPaginationParams represents standard Mastodon API pagination
type MastodonPaginationParams struct {
	Limit   int    `json:"limit,omitempty"`
	MaxID   string `json:"max_id,omitempty"`
	MinID   string `json:"min_id,omitempty"`
	SinceID string `json:"since_id,omitempty"`
	Cursor  string `json:"cursor,omitempty"` // For some endpoints
}

// ValidateMastodonPaginationParams validates Mastodon pagination parameters
func (m *MastodonBusinessLogic) ValidateMastodonPaginationParams(params MastodonPaginationParams) MastodonPaginationParams {
	// Apply default and max limits
	if params.Limit <= 0 {
		params.Limit = m.config.DefaultPagination
	}
	if params.Limit > m.config.MaxPagination {
		params.Limit = m.config.MaxPagination
	}

	// Clean ID parameters
	params.MaxID = strings.TrimSpace(params.MaxID)
	params.MinID = strings.TrimSpace(params.MinID)
	params.SinceID = strings.TrimSpace(params.SinceID)
	params.Cursor = strings.TrimSpace(params.Cursor)

	return params
}

// 2. Status Content Validation Pattern
// Found in: status creation, status updates

// StatusValidationRules defines validation rules for status content
type StatusValidationRules struct {
	MaxLength       int
	AllowMarkdown   bool
	AllowHTML       bool
	RequireContent  bool
	MaxMediaItems   int
	MaxPollOptions  int
}

// ValidateStatusContent validates status content according to Mastodon rules
func (m *MastodonBusinessLogic) ValidateStatusContent(content string, mediaCount int, pollOptions int) error {
	// Content length validation
	if len(content) > m.config.MaxStatusLength {
		return fmt.Errorf("%w of %d characters", ErrStatusContentTooLong, m.config.MaxStatusLength)
	}

	// Require content if no media
	if len(strings.TrimSpace(content)) == 0 && mediaCount == 0 {
		return ErrStatusMustHaveContent
	}

	// Media validation
	if mediaCount > 4 { // Mastodon standard
		return ErrStatusTooManyMedia
	}

	// Poll validation
	if pollOptions > m.config.MaxPollOptions {
		return fmt.Errorf("%w %d", ErrStatusTooManyPollOptions, m.config.MaxPollOptions)
	}

	return nil
}

// 3. Account Validation Pattern
// Found in: account creation, account updates

// ValidateDisplayName validates account display name
func (m *MastodonBusinessLogic) ValidateDisplayName(displayName string) error {
	if len(displayName) > m.config.MaxDisplayName {
		return fmt.Errorf("%w of %d characters", ErrDisplayNameTooLong, m.config.MaxDisplayName)
	}
	return nil
}

// ValidateMastodonUsername validates Mastodon-compatible usernames
func ValidateMastodonUsername(username string) error {
	if username == "" {
		return ErrUsernameEmpty
	}

	// Mastodon username pattern: alphanumeric + underscore, no consecutive underscores
	pattern := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	if !pattern.MatchString(username) {
		return ErrUsernameInvalidCharacters
	}

	// No consecutive underscores
	if strings.Contains(username, "__") {
		return ErrUsernameConsecutiveUnderscores
	}

	// Length constraints
	if len(username) < 1 || len(username) > 30 {
		return ErrUsernameInvalidLength
	}

	// Cannot start or end with underscore
	if strings.HasPrefix(username, "_") || strings.HasSuffix(username, "_") {
		return ErrUsernameInvalidFormat
	}

	return nil
}

// ValidateBio validates account bio/note
func (m *MastodonBusinessLogic) ValidateBio(bio string) error {
	if len(bio) > m.config.MaxBioLength {
		return fmt.Errorf("%w of %d characters", ErrBioTooLong, m.config.MaxBioLength)
	}
	return nil
}

// 4. Media Upload Validation Pattern
// Found in: media upload endpoints

// MediaType represents different media types supported by Mastodon
// MediaType represents the type of media attachment in Mastodon
// These values correspond to the standard Mastodon media attachment types
// used for categorizing uploaded media files
type MediaType string

// Media attachment types supported by Mastodon
const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
	MediaTypeAudio MediaType = "audio"
	MediaTypeGIF   MediaType = "gifv"
)

// ValidateMediaUpload validates media upload according to Mastodon limits
func (m *MastodonBusinessLogic) ValidateMediaUpload(data []byte, mimeType string, mediaType MediaType) error {
	size := int64(len(data))

	switch mediaType {
	case MediaTypeVideo, MediaTypeGIF:
		if size > m.config.VideoUploadLimit {
			return fmt.Errorf("%w of %d bytes", ErrVideoFileTooLarge, m.config.VideoUploadLimit)
		}
	default:
		if size > m.config.MediaUploadLimit {
			return fmt.Errorf("%w of %d bytes", ErrMediaFileTooLarge, m.config.MediaUploadLimit)
		}
	}

	// Validate MIME types
	switch mediaType {
	case MediaTypeImage:
		if !strings.HasPrefix(mimeType, "image/") {
			return fmt.Errorf("%w: %s", ErrInvalidImageMimeType, mimeType)
		}
	case MediaTypeVideo:
		if !strings.HasPrefix(mimeType, "video/") {
			return fmt.Errorf("%w: %s", ErrInvalidVideoMimeType, mimeType)
		}
	case MediaTypeAudio:
		if !strings.HasPrefix(mimeType, "audio/") {
			return fmt.Errorf("%w: %s", ErrInvalidAudioMimeType, mimeType)
		}
	}

	return nil
}

// 5. Filter Pattern
// Found in: content filtering, keyword filtering

// FilterContext represents where a filter applies
type FilterContext string

// Mastodon filter contexts define where content filters should be applied
const (
	FilterContextHome         FilterContext = "home"
	FilterContextNotifications FilterContext = "notifications"
	FilterContextPublic       FilterContext = "public"
	FilterContextThread      FilterContext = "thread"
	FilterContextAccount     FilterContext = "account"
)

// FilterAction represents what action to take when filter matches
type FilterAction string

// Actions that can be taken when a content filter matches
const (
	FilterActionWarn FilterAction = "warn"
	FilterActionHide FilterAction = "hide"
)

// ValidateMastodonFilterKeyword validates filter keyword patterns
func ValidateMastodonFilterKeyword(keyword string) error {
	if keyword == "" {
		return ErrFilterKeywordEmpty
	}

	if len(keyword) > 40 { // Mastodon limit
		return fmt.Errorf("%w of 40 characters", ErrFilterKeywordTooLong)
	}

	return nil
}

// 6. Poll Validation Pattern
// Found in: poll creation, poll voting

// PollOption represents a poll option
type PollOption struct {
	Title string `json:"title"`
	Count int    `json:"votes_count,omitempty"`
}

// ValidatePollOptions validates poll creation
func (m *MastodonBusinessLogic) ValidatePollOptions(options []PollOption, expiresIn int) error {
	if len(options) < 2 {
		return ErrPollTooFewOptions
	}

	if len(options) > m.config.MaxPollOptions {
		return fmt.Errorf("%w %d", ErrPollTooManyOptions, m.config.MaxPollOptions)
	}

	// Validate option titles
	for i, option := range options {
		if option.Title == "" {
			return fmt.Errorf("%w %d", ErrPollOptionEmpty, i+1)
		}
		if len(option.Title) > 50 { // Mastodon limit
			return fmt.Errorf("%w %d of 50 characters", ErrPollOptionTooLong, i+1)
		}
	}

	// Validate expiry
	if expiresIn <= 0 {
		return ErrPollExpiryInvalid
	}

	maxExpiry := int(m.config.MaxPollExpiry.Seconds())
	if expiresIn > maxExpiry {
		return fmt.Errorf("%w %d seconds", ErrPollExpiryTooLong, maxExpiry)
	}

	return nil
}

// 7. Notification Type Pattern
// Found in: notification processing

// NotificationType represents different Mastodon notification types
type NotificationType string

// Mastodon notification types as defined in the Mastodon API specification
const (
	NotificationMention      NotificationType = "mention"
	NotificationStatus       NotificationType = "status"
	NotificationReblog       NotificationType = "reblog"
	NotificationFollow       NotificationType = "follow"
	NotificationFollowRequest NotificationType = "follow_request"
	NotificationFavourite    NotificationType = "favourite"
	NotificationPoll         NotificationType = "poll"
	NotificationUpdate       NotificationType = "update"
	NotificationAdminSignUp  NotificationType = "admin.sign_up"
	NotificationAdminReport  NotificationType = "admin.report"
)

// ValidateNotificationType validates notification type
func ValidateNotificationType(notificationType string) error {
	validTypes := []NotificationType{
		NotificationMention, NotificationStatus, NotificationReblog,
		NotificationFollow, NotificationFollowRequest, NotificationFavourite,
		NotificationPoll, NotificationUpdate, NotificationAdminSignUp,
		NotificationAdminReport,
	}

	for _, validType := range validTypes {
		if notificationType == string(validType) {
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrInvalidNotificationType, notificationType)
}

// 8. Scope Validation Pattern
// Found in: OAuth scopes, permission checking

// Additional OAuth scopes beyond the existing ones
const (
	ScopeFollow       = "write:follows"
	ScopeReadAccounts = "read:accounts"
	ScopeWriteMedia   = "write:media"
	ScopeReadStatuses = "read:statuses"
	ScopeWriteStatuses = "write:statuses"
	ScopePush         = "push"
)

// ValidateMastodonOAuthScopes validates OAuth scope strings for Mastodon API
func ValidateMastodonOAuthScopes(scopes string) ([]string, error) {
	if scopes == "" {
		return []string{"read"}, nil // Default scope
	}

	scopeList := strings.Split(scopes, " ")
	validScopes := make([]string, 0, len(scopeList))

	validScopeMap := map[string]bool{
		"read":          true,
		"write":         true,
		ScopeFollow:     true,
		ScopeReadAccounts: true,
		ScopeWriteMedia:   true,
		ScopeReadStatuses: true,
		ScopeWriteStatuses: true,
		ScopePush:         true,
	}

	for _, scope := range scopeList {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}

		if !validScopeMap[scope] {
			return nil, fmt.Errorf("%w: %s", ErrInvalidOAuthScope, scope)
		}

		validScopes = append(validScopes, scope)
	}

	if len(validScopes) == 0 {
		return []string{"read"}, nil
	}

	return validScopes, nil
}

// 9. ID Generation and Validation Pattern
// Found in: status IDs, account IDs, media IDs

// Snowflake represents a Twitter-like snowflake ID
type Snowflake struct {
	ID        uint64
	Timestamp time.Time
}

// GenerateSnowflakeID generates a Mastodon-compatible snowflake ID
func GenerateSnowflakeID() string {
	// Simple snowflake: timestamp (42 bits) + sequence (22 bits)
	timestamp := time.Now().UnixMilli()
	
	// Ensure timestamp is non-negative to prevent integer overflow
	// when converting to uint64 (addresses gosec G115)
	var id uint64
	if timestamp < 0 {
		// Use 0 for negative timestamps (should not happen in practice)
		id = 0
	} else {
		// Safe conversion: timestamp is guaranteed to be non-negative
		id = uint64(timestamp) << 22
	}
	
	return strconv.FormatUint(id, 10)
}

// ValidateMastodonID validates Mastodon-style IDs
func ValidateMastodonID(id string) error {
	if id == "" {
		return ErrIDEmpty
	}

	// Mastodon IDs are typically numeric strings
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidIDFormat, id)
	}

	return nil
}

// 10. Rate Limiting Pattern
// Found in: API endpoint protection

// RateLimitConfig configures rate limiting for different endpoints
type RateLimitConfig struct {
	PostsPerHour     int
	FollowsPerHour   int
	ReportsPerHour   int
	UploadsPerHour   int
	SearchesPerHour  int
}

// DefaultRateLimits returns Mastodon-compatible rate limits
func DefaultRateLimits() RateLimitConfig {
	return RateLimitConfig{
		PostsPerHour:     300,  // Mastodon default
		FollowsPerHour:   400,  
		ReportsPerHour:   10,   
		UploadsPerHour:   30,   
		SearchesPerHour:  300,  
	}
}

// ValidateRateLimit checks if an action is within rate limits
func (m *MastodonBusinessLogic) ValidateRateLimit(_ context.Context, _, _ string, _ RateLimitConfig) error {
	// This would integrate with a rate limiting service
	// For now, return nil (no rate limiting)
	return nil
}

// Business Logic Execution for Mastodon Operations

// MastodonOperation defines the interface for Mastodon API operations
type MastodonOperation interface {
	Validate(ctx context.Context) error
	Execute(ctx context.Context) error
	GetResponse(ctx context.Context) interface{}
	RecordMetrics(ctx context.Context) error
}

// ExecuteMastodonAPIOperation executes a Mastodon API operation with standard validation and error handling
func (m *MastodonBusinessLogic) ExecuteMastodonAPIOperation(ctx context.Context, operation MastodonOperation) (interface{}, error) {
	// 1. Validate operation
	if err := operation.Validate(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMastodonOperationValidationFailed, err)
	}

	// 2. Execute operation
	if err := operation.Execute(ctx); err != nil {
		if metricErr := operation.RecordMetrics(ctx); metricErr != nil {
			// Log metrics recording failure but don't fail the operation
			zap.L().Error("Failed to record failure metrics", zap.Error(metricErr))
		}
		return nil, fmt.Errorf("%w: %w", ErrMastodonOperationExecutionFailed, err)
	}

	// 3. Get response
	response := operation.GetResponse(ctx)

	// 4. Record success metrics
	if err := operation.RecordMetrics(ctx); err != nil {
		// Log metrics recording failure but don't fail the operation
		zap.L().Error("Failed to record success metrics", zap.Error(err))
	}

	return response, nil
}

// Utility Functions for Mastodon API Compatibility

// FormatMastodonTimestamp formats a time to Mastodon-compatible ISO8601
func FormatMastodonTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// ParseMastodonTimestamp parses a Mastodon timestamp
func ParseMastodonTimestamp(timestamp string) (time.Time, error) {
	layouts := []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		time.RFC3339Nano,
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, timestamp); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("%w: %s", ErrInvalidTimestampFormat, timestamp)
}

// SanitizeHTML removes or escapes HTML content for Mastodon API responses
func SanitizeHTML(content string) string {
	// Basic HTML entity escaping
	content = strings.ReplaceAll(content, "&", "&amp;")
	content = strings.ReplaceAll(content, "<", "&lt;")
	content = strings.ReplaceAll(content, ">", "&gt;")
	content = strings.ReplaceAll(content, "\"", "&quot;")
	content = strings.ReplaceAll(content, "'", "&#39;")
	return content
}

// ExtractHashtags extracts hashtags from content
func ExtractHashtags(content string) []string {
	hashtagRegex := regexp.MustCompile(`#([a-zA-Z0-9_]+)`)
	matches := hashtagRegex.FindAllStringSubmatch(content, -1)
	
	hashtags := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			hashtags = append(hashtags, match[1])
		}
	}
	
	return hashtags
}

// ExtractMentions extracts mentions from content
func ExtractMentions(content string) []string {
	mentionRegex := regexp.MustCompile(`@([a-zA-Z0-9_]+)`)
	matches := mentionRegex.FindAllStringSubmatch(content, -1)
	
	mentions := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			mentions = append(mentions, match[1])
		}
	}
	
	return mentions
}

// Error Types for Mastodon API

// MastodonAPIError represents a Mastodon API-specific error
type MastodonAPIError struct {
	Type        string `json:"error"`
	Description string `json:"error_description,omitempty"`
	StatusCode  int    `json:"-"`
}

// Error implements the error interface
func (e MastodonAPIError) Error() string {
	return fmt.Sprintf("mastodon api error (%s): %s", e.Type, e.Description)
}

// NewMastodonAPIError creates a new Mastodon API error
func NewMastodonAPIError(errorType, description string, statusCode int) MastodonAPIError {
	return MastodonAPIError{
		Type:        errorType,
		Description: description,
		StatusCode:  statusCode,
	}
}

// Common Mastodon API errors
var (
	ErrMastodonInvalidToken     = NewMastodonAPIError("invalid_token", "The access token is invalid", 401)
	ErrMastodonInsufficientScope = NewMastodonAPIError("insufficient_scope", "The request requires higher privileges than provided", 403)
	ErrMastodonRateLimited      = NewMastodonAPIError("rate_limited", "Rate limit exceeded", 429)
	ErrMastodonValidation       = NewMastodonAPIError("validation_failed", "Validation failed", 422)
	ErrMastodonNotFound         = NewMastodonAPIError("record_not_found", "Record not found", 404)
)