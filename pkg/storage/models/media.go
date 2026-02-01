package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
)

// MediaCategory represents the high-level category for a media attachment.
type MediaCategory string

// Media category constants define the canonical classifications for stored media.
const (
	MediaCategoryImage    MediaCategory = "image"
	MediaCategoryVideo    MediaCategory = "video"
	MediaCategoryAudio    MediaCategory = "audio"
	MediaCategoryGifv     MediaCategory = "gifv"
	MediaCategoryDocument MediaCategory = "document"
	MediaCategoryUnknown  MediaCategory = "unknown"
)

var validMediaCategories = map[MediaCategory]struct{}{
	MediaCategoryImage:    {},
	MediaCategoryVideo:    {},
	MediaCategoryAudio:    {},
	MediaCategoryGifv:     {},
	MediaCategoryDocument: {},
	MediaCategoryUnknown:  {},
}

// Media represents a media file (image, video, audio) stored in the system
type Media struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - using media ID as partition key with version as sort key
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "media#{mediaID}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "version#{version}"

	// GSI1 - User media lookup
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1_pk"` // Format: "USER_MEDIA#{userID}"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1_sk"` // Format: "{uploaded_at}#{mediaID}"

	// GSI2 - Status-based queries (pending, processing, ready, failed)
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK" json:"gsi2_pk"` // Format: "MEDIA_STATUS#{status}"
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK" json:"gsi2_sk"` // Format: "{uploaded_at}#{mediaID}"

	// GSI3 - Content type queries
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK" json:"gsi3_pk"` // Format: "CONTENT_TYPE#{content_type}"
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK" json:"gsi3_sk"` // Format: "{uploaded_at}#{mediaID}"

	// Core media data
	MediaID     string `theorydb:"attr:mediaID" json:"media_id"`
	Version     string `theorydb:"attr:version" json:"version"`          // "original", "v1", "v2", etc.
	UserID      string `theorydb:"attr:userID" json:"user_id"`           // Owner of the media
	FileName    string `theorydb:"attr:fileName" json:"file_name"`       // Original filename
	ContentType string `theorydb:"attr:contentType" json:"content_type"` // MIME type
	FileSize    int64  `theorydb:"attr:fileSize" json:"file_size"`       // Size in bytes

	// Storage details
	S3Bucket string `theorydb:"attr:s3Bucket" json:"s3_bucket"`
	S3Key    string `theorydb:"attr:s3Key" json:"s3_key"`
	CDNUrl   string `theorydb:"attr:cdnUrl" json:"cdn_url,omitempty"`

	// Processing status
	Status      string     `theorydb:"attr:status" json:"status"` // "pending", "processing", "ready", "failed"
	ProcessedAt *time.Time `theorydb:"attr:processedAt" json:"processed_at,omitempty"`
	Error       string     `theorydb:"attr:error" json:"error,omitempty"`

	// Media analysis results
	Width    int    `theorydb:"attr:width" json:"width,omitempty"`       // For images/videos
	Height   int    `theorydb:"attr:height" json:"height,omitempty"`     // For images/videos
	Duration int    `theorydb:"attr:duration" json:"duration,omitempty"` // For videos/audio in seconds
	Blurhash string `theorydb:"attr:blurhash" json:"blurhash,omitempty"` // For images

	// Media variants (thumbnails, different sizes, formats)
	Variants map[string]MediaVariant `theorydb:"attr:variants" json:"variants,omitempty"`

	// Media metadata for Mastodon API compatibility
	Description string `theorydb:"attr:description" json:"description,omitempty"` // Alt text description
	Focus       string `theorydb:"attr:focus" json:"focus,omitempty"`             // Focus point for cropping (x,y)
	SpoilerText string `theorydb:"attr:spoilerText" json:"spoiler_text,omitempty"`

	// Content moderation
	IsNSFW          bool     `theorydb:"attr:isNSFW" json:"is_nsfw"`
	ModerationScore float64  `theorydb:"attr:moderationScore" json:"moderation_score"` // 0.0 - 1.0
	Labels          []string `theorydb:"attr:labels" json:"labels,omitempty"`          // Content labels from moderation

	// Client-provided classification (image/video/gifv/etc.)
	MediaCategory MediaCategory `theorydb:"attr:mediaCategory" json:"media_category,omitempty"`

	// Usage tracking
	UsageCount int        `theorydb:"attr:usageCount" json:"usage_count"`
	LastUsedAt *time.Time `theorydb:"attr:lastUsedAt" json:"last_used_at,omitempty"`

	// Timestamps
	UploadedAt time.Time `theorydb:"attr:uploadedAt" json:"uploaded_at"`
	CreatedAt  time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt  time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// TTL for unused media (30 days)
	ExpiresAt int64 `theorydb:"ttl,attr:ttl" json:"expires_at,omitempty"` // Unix timestamp

	// Version for optimistic locking
	ModelVersion int `theorydb:"version,attr:modelVersion" json:"model_version"`
}

// MediaVariant represents a processed variant of the original media
type MediaVariant struct {
	S3Key       string `json:"s3_key"`
	CDNUrl      string `json:"cdn_url,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	FileSize    int64  `json:"file_size"`
	ContentType string `json:"content_type"`
	Quality     string `json:"quality,omitempty"` // "low", "medium", "high"
}

// TableName returns the DynamoDB table backing MediaVariant.
func (MediaVariant) TableName() string {
	return MainTableName
}

// TableName returns the DynamoDB table name for the Media model
func (Media) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (m *Media) BeforeCreate() error {
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now
	m.UploadedAt = now
	m.SpoilerText = strings.TrimSpace(m.SpoilerText)

	// Generate media ID if not provided
	if err := common.ValidateRequiredParam("MediaID", m.MediaID); err != nil {
		m.MediaID = uuid.New().String()
	}

	// Set default version if not provided
	if err := common.ValidateRequiredParam("Version", m.Version); err != nil {
		m.Version = "original"
	}

	// Set default status
	if err := common.ValidateRequiredParam("Status", m.Status); err != nil {
		m.Status = StatusPending
	}

	categoryValue := strings.TrimSpace(string(m.MediaCategory))
	if categoryValue == "" {
		m.MediaCategory = DetermineMediaCategory(m.ContentType)
	} else {
		m.MediaCategory = MediaCategory(strings.ToLower(categoryValue))
	}

	// Initialize usage count
	m.UsageCount = 0

	// Set expiry for unused media (30 days)
	if m.ExpiresAt <= 0 {
		m.ExpiresAt = now.Add(30 * 24 * time.Hour).Unix()
	}

	// Set up primary key
	m.PK = "media#" + m.MediaID
	m.SK = "version#" + m.Version

	// Set up GSI keys
	m.setupGSIKeys()

	return m.Validate()
}

// BeforeUpdate sets up the model before update
func (m *Media) BeforeUpdate() error {
	m.UpdatedAt = time.Now()
	m.SpoilerText = strings.TrimSpace(m.SpoilerText)
	if strings.TrimSpace(string(m.MediaCategory)) == "" {
		m.MediaCategory = DetermineMediaCategory(m.ContentType)
	} else {
		m.MediaCategory = MediaCategory(strings.ToLower(strings.TrimSpace(string(m.MediaCategory))))
	}

	// Update GSI keys in case status or other indexed fields changed
	m.setupGSIKeys()

	return m.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (m *Media) setupGSIKeys() {
	uploadedAtStr := m.UploadedAt.Format(time.RFC3339)

	// GSI1 - User media lookup
	if err := common.ValidateRequiredParam("UserID", m.UserID); err == nil {
		m.GSI1PK = "USER_MEDIA#" + m.UserID
		m.GSI1SK = fmt.Sprintf("%s#%s", uploadedAtStr, m.MediaID)
	} else {
		m.GSI1PK = ""
		m.GSI1SK = ""
	}

	// GSI2 - Status-based queries
	m.GSI2PK = "MEDIA_STATUS#" + m.Status
	m.GSI2SK = fmt.Sprintf("%s#%s", uploadedAtStr, m.MediaID)

	// GSI3 - Content type queries
	if err := common.ValidateRequiredParam("ContentType", m.ContentType); err == nil {
		// Normalize content type for indexing
		contentTypeKey := strings.Split(m.ContentType, "/")[0] // "image", "video", "audio"
		m.GSI3PK = "CONTENT_TYPE#" + contentTypeKey
		m.GSI3SK = fmt.Sprintf("%s#%s", uploadedAtStr, m.MediaID)
	}
}

// Validate performs validation on the Media
func (m *Media) Validate() error {
	if strings.TrimSpace(string(m.MediaCategory)) == "" {
		m.MediaCategory = DetermineMediaCategory(m.ContentType)
	} else {
		m.MediaCategory = MediaCategory(strings.ToLower(strings.TrimSpace(string(m.MediaCategory))))
	}

	if err := common.ValidateRequiredParam("MediaID", strings.TrimSpace(m.MediaID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("UserID", strings.TrimSpace(m.UserID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("ContentType", strings.TrimSpace(m.ContentType)); err != nil {
		return err
	}
	if m.FileSize <= 0 {
		return ErrFileSizeZero
	}

	// Check file size limits (50MB)
	maxSize := int64(50 * 1024 * 1024)
	if m.FileSize > maxSize {
		return fmt.Errorf("%w: %d exceeds maximum %d bytes", ErrFileSizeTooLarge, m.FileSize, maxSize)
	}

	// Validate content type
	if !isValidMediaType(m.ContentType) {
		return fmt.Errorf("%w: %s", ErrUnsupportedContentType, m.ContentType)
	}

	// Validate status
	if !isValidMediaStatus(m.Status) {
		return fmt.Errorf("%w: %s", ErrInvalidMediaStatus, m.Status)
	}

	if m.SpoilerText != "" {
		if err := common.ValidateSpoilerText(m.SpoilerText); err != nil {
			return err
		}
	}

	if !IsValidMediaCategory(m.MediaCategory) {
		return fmt.Errorf("%w: %s", ErrInvalidMediaCategory, m.MediaCategory)
	}

	return nil
}

// MarkUsed increments usage count and removes expiry for used media
func (m *Media) MarkUsed() {
	m.UsageCount++
	now := time.Now()
	m.LastUsedAt = &now
	m.ExpiresAt = 0 // Remove expiry for used media
}

// SetProcessed marks the media as successfully processed
func (m *Media) SetProcessed() {
	m.Status = "ready"
	now := time.Now()
	m.ProcessedAt = &now
	m.Error = ""
}

// SetFailed marks the media as failed with an error message
func (m *Media) SetFailed(errorMsg string) {
	m.Status = StatusFailed
	now := time.Now()
	m.ProcessedAt = &now
	m.Error = errorMsg
}

// SetProcessing marks the media as currently being processed
func (m *Media) SetProcessing() {
	m.Status = StatusProcessing
	m.Error = ""
}

// IsReady returns true if the media is ready for use
func (m *Media) IsReady() bool {
	return m.Status == "ready"
}

// IsFailed returns true if the media processing failed
func (m *Media) IsFailed() bool {
	return m.Status == StatusFailed
}

// IsProcessing returns true if the media is currently being processed
func (m *Media) IsProcessing() bool {
	return m.Status == StatusProcessing
}

// AddVariant adds a processed variant to the media
func (m *Media) AddVariant(name string, variant MediaVariant) {
	if m.Variants == nil {
		m.Variants = make(map[string]MediaVariant)
	}
	m.Variants[name] = variant
	m.UpdatedAt = time.Now()
}

// GetVariant retrieves a specific variant by name
func (m *Media) GetVariant(name string) (MediaVariant, bool) {
	if m.Variants == nil {
		return MediaVariant{}, false
	}
	variant, exists := m.Variants[name]
	return variant, exists
}

// GetBestVariant returns the best variant for the requested dimensions
func (m *Media) GetBestVariant(maxWidth, maxHeight int) MediaVariant {
	if len(m.Variants) == 0 {
		// Return original as fallback
		return MediaVariant{
			S3Key:       m.S3Key,
			CDNUrl:      m.CDNUrl,
			Width:       m.Width,
			Height:      m.Height,
			FileSize:    m.FileSize,
			ContentType: m.ContentType,
		}
	}

	var bestVariant MediaVariant
	var bestScore int

	for _, variant := range m.Variants {
		// Skip variants that are too large
		if variant.Width > maxWidth || variant.Height > maxHeight {
			continue
		}

		// Calculate fitness score (area)
		score := variant.Width * variant.Height
		if score > bestScore {
			bestScore = score
			bestVariant = variant
		}
	}

	// If no suitable variant found, return the smallest one
	if bestScore == 0 {
		for _, variant := range m.Variants {
			area := variant.Width * variant.Height
			if bestScore == 0 || area < bestScore {
				bestScore = area
				bestVariant = variant
			}
		}
	}

	return bestVariant
}

// GetAvailableVariants returns a list of all available variant names
func (m *Media) GetAvailableVariants() []string {
	if m.Variants == nil {
		return []string{}
	}

	variants := make([]string, 0, len(m.Variants))
	for name := range m.Variants {
		variants = append(variants, name)
	}
	return variants
}

// SetModeration sets moderation results
func (m *Media) SetModeration(isNSFW bool, score float64, labels []string) {
	m.IsNSFW = isNSFW
	m.ModerationScore = score
	m.Labels = labels
}

// IsImage returns true if the media is an image
func (m *Media) IsImage() bool {
	return strings.HasPrefix(m.ContentType, "image/")
}

// IsVideo returns true if the media is a video
func (m *Media) IsVideo() bool {
	return strings.HasPrefix(m.ContentType, "video/")
}

// IsAudio returns true if the media is audio
func (m *Media) IsAudio() bool {
	return strings.HasPrefix(m.ContentType, "audio/")
}

// DetermineMediaCategory derives a category from the MIME type when none is provided.
func DetermineMediaCategory(contentType string) MediaCategory {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	switch {
	case contentType == "image/gif":
		return MediaCategoryGifv
	case strings.HasPrefix(contentType, "image/"):
		return MediaCategoryImage
	case strings.HasPrefix(contentType, "video/"):
		return MediaCategoryVideo
	case strings.HasPrefix(contentType, "audio/"):
		return MediaCategoryAudio
	default:
		return MediaCategoryUnknown
	}
}

// IsValidMediaCategory reports whether the provided category is supported.
func IsValidMediaCategory(category MediaCategory) bool {
	if category == "" {
		return true
	}
	normalized := MediaCategory(strings.ToLower(strings.TrimSpace(string(category))))
	_, ok := validMediaCategories[normalized]
	return ok
}

// NormalizeMediaCategory converts arbitrary input into a canonical media category value.
// The boolean return indicates whether the provided category was recognized.
func NormalizeMediaCategory(category string) (MediaCategory, bool) {
	if strings.TrimSpace(category) == "" {
		return "", false
	}

	normalized := MediaCategory(strings.ToLower(strings.TrimSpace(category)))
	if _, ok := validMediaCategories[normalized]; !ok {
		return MediaCategoryUnknown, false
	}

	return normalized, true
}

// GetTotalSize returns the total size including all variants
func (m *Media) GetTotalSize() int64 {
	total := m.FileSize

	if m.Variants != nil {
		for _, variant := range m.Variants {
			total += variant.FileSize
		}
	}

	return total
}

// isValidMediaType checks if the content type is supported
func isValidMediaType(contentType string) bool {
	validTypes := map[string]bool{
		// Images
		"image/jpeg":    true,
		"image/jpg":     true,
		"image/png":     true,
		"image/gif":     true,
		"image/webp":    true,
		"image/svg+xml": true,
		"image/bmp":     true,
		"image/tiff":    true,

		// Videos
		"video/mp4":       true,
		"video/webm":      true,
		"video/ogg":       true,
		"video/avi":       true,
		"video/mov":       true,
		"video/quicktime": true,
		"video/x-msvideo": true,

		// Audio
		"audio/mpeg":  true,
		"audio/mp3":   true,
		"audio/wav":   true,
		"audio/ogg":   true,
		"audio/aac":   true,
		"audio/flac":  true,
		"audio/x-wav": true,
		"audio/webm":  true,
	}

	return validTypes[strings.ToLower(contentType)]
}

// isValidMediaStatus checks if the status is valid
func isValidMediaStatus(status string) bool {
	validStatuses := map[string]bool{
		StatusPending:    true,
		StatusProcessing: true,
		"ready":          true,
		StatusFailed:     true,
	}

	return validStatuses[strings.ToLower(status)]
}

// === BaseModel Interface Implementation ===

// GetPK returns the partition key for this media item
func (m *Media) GetPK() string {
	return m.PK
}

// GetSK returns the sort key for this media item
func (m *Media) GetSK() string {
	return m.SK
}

// UpdateKeys ensures all key fields are properly set
func (m *Media) UpdateKeys() error {
	// Validate required fields first
	if err := common.ValidateRequiredParam("MediaID", m.MediaID); err != nil {
		return fmt.Errorf("%w: %w", ErrMediaIDRequired, err)
	}
	if err := common.ValidateRequiredParam("Version", m.Version); err != nil {
		m.Version = "original" // Set default if not provided
	}

	// Set primary keys
	m.PK = "media#" + m.MediaID
	m.SK = "version#" + m.Version

	// Update GSI keys
	m.setupGSIKeys()

	return nil
}
