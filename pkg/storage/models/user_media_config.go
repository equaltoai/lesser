package models

import (
	"fmt"
	"strings"
	"time"
)

// UserMediaConfig represents a user's media processing configuration and limits
type UserMediaConfig struct {
	// Primary key - using user ID as partition key
	PK string `dynamorm:"pk" json:"pk"` // Format: "USER_MEDIA_CONFIG#{userID}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "CONFIG"

	// Core config data
	UserID   string `json:"user_id"`
	Username string `json:"username"`

	// Processing preferences
	VideoProcessingEnabled   bool `json:"video_processing_enabled"`
	AudioProcessingEnabled   bool `json:"audio_processing_enabled"`
	VideoThumbnailsEnabled   bool `json:"video_thumbnails_enabled"`
	ContentModerationEnabled bool `json:"content_moderation_enabled"`

	// Limits and quotas
	MaxFileSize          int64 `json:"max_file_size"`          // bytes
	MaxVideoSize         int64 `json:"max_video_size"`         // bytes
	MaxAudioSize         int64 `json:"max_audio_size"`         // bytes
	MaxImageSize         int64 `json:"max_image_size"`         // bytes
	MaxVideoDuration     int   `json:"max_video_duration"`     // seconds
	MaxDailyUploads      int   `json:"max_daily_uploads"`      // files per day
	MaxMonthlyUploads    int   `json:"max_monthly_uploads"`    // files per month
	MaxStorageUsage      int64 `json:"max_storage_usage"`      // bytes
	MaxBandwidthPerMonth int64 `json:"max_bandwidth_per_month"` // bytes

	// Budget limits (in microdollars - $1 = 1,000,000 microdollars)
	MonthlyBudgetMicros    int64 `json:"monthly_budget_micros"`
	DailyBudgetMicros      int64 `json:"daily_budget_micros"`
	ProcessingBudgetMicros int64 `json:"processing_budget_micros"` // For MediaConvert, Rekognition, etc.
	StorageBudgetMicros    int64 `json:"storage_budget_micros"`    // For S3 storage
	BandwidthBudgetMicros  int64 `json:"bandwidth_budget_micros"`  // For CDN/transfer

	// Allowed content types
	AllowedImageTypes []string `json:"allowed_image_types"`
	AllowedVideoTypes []string `json:"allowed_video_types"`
	AllowedAudioTypes []string `json:"allowed_audio_types"`

	// Processing quality settings
	ImageQuality     string `json:"image_quality"`      // "low", "medium", "high"
	VideoQuality     string `json:"video_quality"`      // "low", "medium", "high"
	EnableBlurhash   bool   `json:"enable_blurhash"`    // Generate blurhashes for images
	EnableThumbnails bool   `json:"enable_thumbnails"`  // Generate thumbnails

	// Content moderation settings
	ModerationThreshold float64  `json:"moderation_threshold"` // 0.0-1.0, above this is flagged
	AutoRejectNSFW      bool     `json:"auto_reject_nsfw"`
	RequiredLabels      []string `json:"required_labels"`     // Required content labels
	BlockedLabels       []string `json:"blocked_labels"`      // Automatically blocked labels

	// Plan/tier information
	PlanTier        string `json:"plan_tier"`        // "free", "basic", "premium", "enterprise"
	PlanExpiresAt   *time.Time `json:"plan_expires_at,omitempty"`
	IsTrialUser     bool   `json:"is_trial_user"`
	TrialExpiresAt  *time.Time `json:"trial_expires_at,omitempty"`

	// Usage tracking references
	CurrentStorageUsage int64 `json:"current_storage_usage"` // bytes
	CurrentMonthlyUsage int64 `json:"current_monthly_usage"` // bytes processed this month
	LastResetAt         time.Time `json:"last_reset_at"`     // When monthly counters were last reset

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Version for optimistic locking  
	ModelVersion int `dynamorm:"version" json:"model_version"`
}

// TableName returns the DynamoDB table name for the UserMediaConfig model
func (UserMediaConfig) TableName() string {
	return "lesser-main" // Use the main table
}

// BeforeCreate sets up the model before creation
func (umc *UserMediaConfig) BeforeCreate() error {
	now := time.Now()
	umc.CreatedAt = now
	umc.UpdatedAt = now
	umc.LastResetAt = now

	// Set up primary key
	umc.PK = "USER_MEDIA_CONFIG#" + umc.UserID
	umc.SK = "CONFIG"

	// Set defaults if not provided
	umc.setDefaults()

	return umc.Validate()
}

// BeforeUpdate sets up the model before update
func (umc *UserMediaConfig) BeforeUpdate() error {
	umc.UpdatedAt = time.Now()
	return umc.Validate()
}

// setDefaults sets reasonable default values for a new user
func (umc *UserMediaConfig) setDefaults() {
	if umc.PlanTier == "" {
		umc.PlanTier = "free"
	}

	// Default processing preferences
	umc.VideoProcessingEnabled = true
	umc.AudioProcessingEnabled = true  
	umc.VideoThumbnailsEnabled = true
	umc.ContentModerationEnabled = false // Disabled by default for cost reasons

	// Default file size limits based on plan tier
	switch umc.PlanTier {
	case "free":
		umc.MaxImageSize = 5 * 1024 * 1024         // 5MB
		umc.MaxVideoSize = 25 * 1024 * 1024        // 25MB
		umc.MaxAudioSize = 10 * 1024 * 1024        // 10MB
		umc.MaxFileSize = 25 * 1024 * 1024         // 25MB overall max
		umc.MaxVideoDuration = 120                 // 2 minutes
		umc.MaxDailyUploads = 50
		umc.MaxMonthlyUploads = 1000
		umc.MaxStorageUsage = 1 * 1024 * 1024 * 1024    // 1GB
		umc.MaxBandwidthPerMonth = 10 * 1024 * 1024 * 1024 // 10GB
		umc.MonthlyBudgetMicros = 1_000_000        // $1/month
		umc.DailyBudgetMicros = 100_000            // $0.10/day
		umc.ProcessingBudgetMicros = 500_000       // $0.50/month for processing
		umc.StorageBudgetMicros = 300_000          // $0.30/month for storage
		umc.BandwidthBudgetMicros = 200_000        // $0.20/month for bandwidth

	case "basic":
		umc.MaxImageSize = 10 * 1024 * 1024        // 10MB
		umc.MaxVideoSize = 50 * 1024 * 1024        // 50MB
		umc.MaxAudioSize = 20 * 1024 * 1024        // 20MB
		umc.MaxFileSize = 50 * 1024 * 1024         // 50MB overall max
		umc.MaxVideoDuration = 600                 // 10 minutes
		umc.MaxDailyUploads = 200
		umc.MaxMonthlyUploads = 5000
		umc.MaxStorageUsage = 10 * 1024 * 1024 * 1024   // 10GB
		umc.MaxBandwidthPerMonth = 100 * 1024 * 1024 * 1024 // 100GB
		umc.MonthlyBudgetMicros = 10_000_000       // $10/month
		umc.DailyBudgetMicros = 500_000            // $0.50/day
		umc.ProcessingBudgetMicros = 5_000_000     // $5/month for processing
		umc.StorageBudgetMicros = 3_000_000        // $3/month for storage
		umc.BandwidthBudgetMicros = 2_000_000      // $2/month for bandwidth

	case "premium":
		umc.MaxImageSize = 20 * 1024 * 1024        // 20MB
		umc.MaxVideoSize = 200 * 1024 * 1024       // 200MB
		umc.MaxAudioSize = 50 * 1024 * 1024        // 50MB
		umc.MaxFileSize = 200 * 1024 * 1024        // 200MB overall max
		umc.MaxVideoDuration = 3600                // 1 hour
		umc.MaxDailyUploads = 1000
		umc.MaxMonthlyUploads = 25000
		umc.MaxStorageUsage = 100 * 1024 * 1024 * 1024  // 100GB
		umc.MaxBandwidthPerMonth = 1000 * 1024 * 1024 * 1024 // 1TB
		umc.MonthlyBudgetMicros = 50_000_000       // $50/month
		umc.DailyBudgetMicros = 2_000_000          // $2/day
		umc.ProcessingBudgetMicros = 30_000_000    // $30/month for processing
		umc.StorageBudgetMicros = 10_000_000       // $10/month for storage
		umc.BandwidthBudgetMicros = 10_000_000     // $10/month for bandwidth
		umc.ContentModerationEnabled = true        // Enable moderation for premium
	}

	// Default allowed content types
	if len(umc.AllowedImageTypes) == 0 {
		umc.AllowedImageTypes = []string{
			"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp",
		}
	}
	if len(umc.AllowedVideoTypes) == 0 {
		umc.AllowedVideoTypes = []string{
			"video/mp4", "video/webm", "video/quicktime",
		}
	}
	if len(umc.AllowedAudioTypes) == 0 {
		umc.AllowedAudioTypes = []string{
			"audio/mpeg", "audio/mp3", "audio/ogg", "audio/wav", "audio/webm",
		}
	}

	// Default quality settings
	if umc.ImageQuality == "" {
		umc.ImageQuality = "medium"
	}
	if umc.VideoQuality == "" {
		umc.VideoQuality = "medium"
	}

	// Default moderation settings
	if umc.ModerationThreshold == 0 {
		umc.ModerationThreshold = 0.8 // 80% confidence threshold
	}

	// Enable common features
	umc.EnableBlurhash = true
	umc.EnableThumbnails = true
}

// Validate performs validation on the UserMediaConfig
func (umc *UserMediaConfig) Validate() error {
	if strings.TrimSpace(umc.UserID) == "" {
		return fmt.Errorf("UserID is required")
	}
	if strings.TrimSpace(umc.Username) == "" {
		return fmt.Errorf("Username is required")
	}

	// Validate plan tier
	validTiers := map[string]bool{
		"free": true, "basic": true, "premium": true, "enterprise": true,
	}
	if !validTiers[umc.PlanTier] {
		return fmt.Errorf("invalid plan tier: %s", umc.PlanTier)
	}

	// Validate file size limits
	if umc.MaxFileSize <= 0 {
		return fmt.Errorf("MaxFileSize must be greater than 0")
	}
	if umc.MaxVideoSize > umc.MaxFileSize {
		return fmt.Errorf("MaxVideoSize cannot exceed MaxFileSize")
	}
	if umc.MaxImageSize > umc.MaxFileSize {
		return fmt.Errorf("MaxImageSize cannot exceed MaxFileSize")  
	}
	if umc.MaxAudioSize > umc.MaxFileSize {
		return fmt.Errorf("MaxAudioSize cannot exceed MaxFileSize")
	}

	// Validate duration limits
	if umc.MaxVideoDuration < 0 {
		return fmt.Errorf("MaxVideoDuration cannot be negative")
	}

	// Validate upload limits
	if umc.MaxDailyUploads < 0 || umc.MaxMonthlyUploads < 0 {
		return fmt.Errorf("upload limits cannot be negative")
	}
	if umc.MaxDailyUploads > umc.MaxMonthlyUploads {
		return fmt.Errorf("MaxDailyUploads cannot exceed MaxMonthlyUploads")
	}

	// Validate budget limits
	if umc.MonthlyBudgetMicros < 0 || umc.DailyBudgetMicros < 0 {
		return fmt.Errorf("budget limits cannot be negative")
	}

	// Validate moderation threshold
	if umc.ModerationThreshold < 0.0 || umc.ModerationThreshold > 1.0 {
		return fmt.Errorf("ModerationThreshold must be between 0.0 and 1.0")
	}

	// Validate quality settings
	validQualities := map[string]bool{"low": true, "medium": true, "high": true}
	if !validQualities[umc.ImageQuality] {
		return fmt.Errorf("invalid ImageQuality: %s", umc.ImageQuality)
	}
	if !validQualities[umc.VideoQuality] {
		return fmt.Errorf("invalid VideoQuality: %s", umc.VideoQuality)
	}

	return nil
}

// IsAllowedContentType checks if a content type is allowed for this user
func (umc *UserMediaConfig) IsAllowedContentType(contentType string) bool {
	contentType = strings.ToLower(contentType)
	
	if strings.HasPrefix(contentType, "image/") {
		for _, allowed := range umc.AllowedImageTypes {
			if strings.ToLower(allowed) == contentType {
				return true
			}
		}
	} else if strings.HasPrefix(contentType, "video/") {
		for _, allowed := range umc.AllowedVideoTypes {
			if strings.ToLower(allowed) == contentType {
				return true
			}
		}
	} else if strings.HasPrefix(contentType, "audio/") {
		for _, allowed := range umc.AllowedAudioTypes {
			if strings.ToLower(allowed) == contentType {
				return true
			}
		}
	}
	
	return false
}

// GetMaxSizeForType returns the maximum allowed size for a content type
func (umc *UserMediaConfig) GetMaxSizeForType(contentType string) int64 {
	contentType = strings.ToLower(contentType)
	
	if strings.HasPrefix(contentType, "image/") {
		return umc.MaxImageSize
	} else if strings.HasPrefix(contentType, "video/") {
		return umc.MaxVideoSize
	} else if strings.HasPrefix(contentType, "audio/") {
		return umc.MaxAudioSize
	}
	
	return umc.MaxFileSize
}

// CanAfford checks if the user can afford a given cost in microdollars
func (umc *UserMediaConfig) CanAfford(costMicros int64, costType string) bool {
	switch costType {
	case "processing":
		return costMicros <= umc.ProcessingBudgetMicros
	case "storage":
		return costMicros <= umc.StorageBudgetMicros
	case "bandwidth":
		return costMicros <= umc.BandwidthBudgetMicros
	default:
		return costMicros <= umc.MonthlyBudgetMicros
	}
}

// IsWithinStorageLimit checks if adding bytes would exceed storage limit
func (umc *UserMediaConfig) IsWithinStorageLimit(additionalBytes int64) bool {
	return (umc.CurrentStorageUsage + additionalBytes) <= umc.MaxStorageUsage
}

// ShouldResetCounters checks if monthly counters need to be reset
func (umc *UserMediaConfig) ShouldResetCounters() bool {
	now := time.Now()
	lastReset := umc.LastResetAt
	
	// Reset if it's a new month
	return now.Year() != lastReset.Year() || now.Month() != lastReset.Month()
}

// ResetMonthlyCounters resets monthly usage counters
func (umc *UserMediaConfig) ResetMonthlyCounters() {
	umc.CurrentMonthlyUsage = 0
	umc.LastResetAt = time.Now()
}

// UpdateStorageUsage updates the current storage usage
func (umc *UserMediaConfig) UpdateStorageUsage(deltaBytes int64) {
	umc.CurrentStorageUsage += deltaBytes
	if umc.CurrentStorageUsage < 0 {
		umc.CurrentStorageUsage = 0
	}
}

// AddMonthlyUsage adds to the monthly usage counter
func (umc *UserMediaConfig) AddMonthlyUsage(bytes int64) {
	umc.CurrentMonthlyUsage += bytes
}

// UpgradePlan upgrades the user to a higher plan tier
func (umc *UserMediaConfig) UpgradePlan(newTier string, expiresAt *time.Time) error {
	oldTier := umc.PlanTier
	umc.PlanTier = newTier
	umc.PlanExpiresAt = expiresAt
	
	// Update limits based on new tier
	umc.setDefaults()
	
	// Validate the upgrade
	if err := umc.Validate(); err != nil {
		// Rollback on validation failure
		umc.PlanTier = oldTier
		umc.setDefaults()
		return fmt.Errorf("failed to upgrade plan: %w", err)
	}
	
	return nil
}

// IsExpired checks if the user's plan has expired
func (umc *UserMediaConfig) IsExpired() bool {
	if umc.PlanExpiresAt == nil {
		return false
	}
	return time.Now().After(*umc.PlanExpiresAt)
}

// IsTrialExpired checks if the user's trial has expired
func (umc *UserMediaConfig) IsTrialExpired() bool {
	if !umc.IsTrialUser || umc.TrialExpiresAt == nil {
		return false
	}
	return time.Now().After(*umc.TrialExpiresAt)
}