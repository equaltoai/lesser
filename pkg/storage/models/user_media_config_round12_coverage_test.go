package models

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUserMediaConfig_BeforeCreate_SetsDefaultsAndKeys(t *testing.T) {
	umc := &UserMediaConfig{
		UserID:   "user-1",
		Username: "alice",
	}

	err := umc.BeforeCreate()
	assert.NoError(t, err)

	assert.Equal(t, "USER_MEDIA_CONFIG#user-1", umc.PK)
	assert.Equal(t, SKConfig, umc.SK)
	assert.WithinDuration(t, time.Now(), umc.CreatedAt, 2*time.Second)
	assert.WithinDuration(t, umc.CreatedAt, umc.UpdatedAt, 2*time.Second)
	assert.WithinDuration(t, umc.CreatedAt, umc.LastResetAt, 2*time.Second)

	assert.Equal(t, "free", umc.PlanTier)
	assert.True(t, umc.VideoProcessingEnabled)
	assert.True(t, umc.AudioProcessingEnabled)
	assert.True(t, umc.VideoThumbnailsEnabled)
	assert.False(t, umc.ContentModerationEnabled)

	assert.NotZero(t, umc.MaxFileSize)
	assert.NotZero(t, umc.MaxImageSize)
	assert.NotZero(t, umc.MaxVideoSize)
	assert.NotZero(t, umc.MaxAudioSize)
	assert.NotEmpty(t, umc.AllowedImageTypes)
	assert.NotEmpty(t, umc.AllowedVideoTypes)
	assert.NotEmpty(t, umc.AllowedAudioTypes)

	assert.Equal(t, "medium", umc.ImageQuality)
	assert.Equal(t, "medium", umc.VideoQuality)
	assert.InDelta(t, 0.8, umc.ModerationThreshold, 0.0001)
	assert.True(t, umc.EnableBlurhash)
	assert.True(t, umc.EnableThumbnails)
}

func TestUserMediaConfig_Validate_ErrorCases(t *testing.T) {
	valid := func() *UserMediaConfig {
		umc := &UserMediaConfig{
			UserID:   "user-1",
			Username: "alice",
			PlanTier: "free",

			MaxFileSize:      10,
			MaxImageSize:     10,
			MaxVideoSize:     10,
			MaxAudioSize:     10,
			MaxVideoDuration: 0,

			MaxDailyUploads:   1,
			MaxMonthlyUploads: 2,

			MonthlyBudgetMicros: 1,
			DailyBudgetMicros:   1,

			ModerationThreshold: 0.5,
			ImageQuality:        "medium",
			VideoQuality:        "medium",
		}
		return umc
	}

	t.Run("invalid_plan_tier", func(t *testing.T) {
		umc := valid()
		umc.PlanTier = "nope"
		err := umc.Validate()
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidPlanTier))
	})

	t.Run("invalid_file_size", func(t *testing.T) {
		umc := valid()
		umc.MaxFileSize = 0
		err := umc.Validate()
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidFileSize))
	})

	t.Run("file_size_too_large_video", func(t *testing.T) {
		umc := valid()
		umc.MaxVideoSize = 11
		err := umc.Validate()
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrFileSizeTooLarge))
	})

	t.Run("negative_video_duration", func(t *testing.T) {
		umc := valid()
		umc.MaxVideoDuration = -1
		err := umc.Validate()
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrVideoDurationInvalid))
	})

	t.Run("upload_limits_invalid_negative", func(t *testing.T) {
		umc := valid()
		umc.MaxDailyUploads = -1
		err := umc.Validate()
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrUploadLimitsInvalid))
	})

	t.Run("upload_limits_invalid_daily_gt_monthly", func(t *testing.T) {
		umc := valid()
		umc.MaxDailyUploads = 10
		umc.MaxMonthlyUploads = 1
		err := umc.Validate()
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrUploadLimitsInvalid))
	})

	t.Run("budget_limits_invalid", func(t *testing.T) {
		umc := valid()
		umc.MonthlyBudgetMicros = -1
		err := umc.Validate()
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrBudgetLimitsInvalid))
	})

	t.Run("moderation_threshold_invalid", func(t *testing.T) {
		umc := valid()
		umc.ModerationThreshold = 2
		err := umc.Validate()
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrModerationThresholdInvalid))
	})

	t.Run("quality_invalid", func(t *testing.T) {
		umc := valid()
		umc.ImageQuality = "ultra"
		err := umc.Validate()
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidQualitySetting))
	})
}

func TestUserMediaConfig_ContentTypeAndBudgetsAndCounters(t *testing.T) {
	umc := &UserMediaConfig{
		UserID:   "user-1",
		Username: "alice",
	}
	assert.NoError(t, umc.BeforeCreate())

	assert.True(t, umc.IsAllowedContentType("IMAGE/PNG"))
	assert.False(t, umc.IsAllowedContentType("image/unknown"))
	assert.True(t, umc.IsAllowedContentType("video/mp4"))
	assert.True(t, umc.IsAllowedContentType("audio/ogg"))

	umc.MaxImageSize = 1
	umc.MaxVideoSize = 2
	umc.MaxAudioSize = 3
	umc.MaxFileSize = 4
	assert.Equal(t, int64(1), umc.GetMaxSizeForType("image/png"))
	assert.Equal(t, int64(2), umc.GetMaxSizeForType("video/mp4"))
	assert.Equal(t, int64(3), umc.GetMaxSizeForType("audio/ogg"))
	assert.Equal(t, int64(4), umc.GetMaxSizeForType("application/octet-stream"))

	umc.ProcessingBudgetMicros = 10
	umc.StorageBudgetMicros = 20
	umc.BandwidthBudgetMicros = 30
	umc.MonthlyBudgetMicros = 40
	assert.True(t, umc.CanAfford(10, "processing"))
	assert.False(t, umc.CanAfford(11, "processing"))
	assert.True(t, umc.CanAfford(20, "storage"))
	assert.True(t, umc.CanAfford(30, "bandwidth"))
	assert.True(t, umc.CanAfford(40, "other"))

	umc.MaxStorageUsage = 100
	umc.CurrentStorageUsage = 90
	assert.True(t, umc.IsWithinStorageLimit(10))
	assert.False(t, umc.IsWithinStorageLimit(11))

	umc.UpdateStorageUsage(-200)
	assert.Equal(t, int64(0), umc.CurrentStorageUsage)

	now := time.Now()
	prevMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Add(-time.Second)
	umc.LastResetAt = prevMonth
	assert.True(t, umc.ShouldResetCounters())
	umc.LastResetAt = now
	assert.False(t, umc.ShouldResetCounters())

	umc.CurrentMonthlyUsage = 123
	umc.ResetMonthlyCounters()
	assert.Equal(t, int64(0), umc.CurrentMonthlyUsage)

	umc.AddMonthlyUsage(10)
	assert.Equal(t, int64(10), umc.CurrentMonthlyUsage)
}

func TestUserMediaConfig_UpgradePlanAndExpiryAndUpdateKeys(t *testing.T) {
	umc := &UserMediaConfig{
		UserID:   "user-1",
		Username: "alice",
	}
	assert.NoError(t, umc.BeforeCreate())

	// Invalid upgrade should rollback.
	oldTier := umc.PlanTier
	err := umc.UpgradePlan("nope", nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrPlanUpgradeFailed))
	assert.Equal(t, oldTier, umc.PlanTier)

	// Valid upgrade applies new defaults.
	expires := time.Now().Add(1 * time.Hour)
	assert.NoError(t, umc.UpgradePlan("basic", &expires))
	assert.Equal(t, "basic", umc.PlanTier)
	assert.Equal(t, &expires, umc.PlanExpiresAt)
	assert.Greater(t, umc.MaxFileSize, int64(0))

	// Expiry helpers.
	past := time.Now().Add(-1 * time.Hour)
	umc.PlanExpiresAt = &past
	assert.True(t, umc.IsExpired())

	umc.IsTrialUser = true
	umc.TrialExpiresAt = &past
	assert.True(t, umc.IsTrialExpired())

	// UpdateKeys should validate required fields.
	umc2 := &UserMediaConfig{}
	assert.Error(t, umc2.UpdateKeys())
}

func TestUserMediaConfig_UpgradePlan_PremiumAppliesPremiumDefaults(t *testing.T) {
	umc := &UserMediaConfig{
		UserID:   "user-1",
		Username: "alice",
	}
	assert.NoError(t, umc.BeforeCreate())

	// Valid premium upgrade applies the premium default limits (setDefaults
	// premium branch): bigger file caps, higher quotas, and moderation enabled.
	expires := time.Now().Add(24 * time.Hour)
	assert.NoError(t, umc.UpgradePlan("premium", &expires))
	assert.Equal(t, "premium", umc.PlanTier)
	assert.Equal(t, &expires, umc.PlanExpiresAt)

	assert.Equal(t, int64(20*1024*1024), umc.MaxImageSize)
	assert.Equal(t, int64(200*1024*1024), umc.MaxVideoSize)
	assert.Equal(t, int64(200*1024*1024), umc.MaxFileSize)
	assert.Equal(t, 3600, umc.MaxVideoDuration)
	assert.Equal(t, 1000, umc.MaxDailyUploads)
	assert.Equal(t, 25000, umc.MaxMonthlyUploads)
	assert.Equal(t, int64(100*1024*1024*1024), umc.MaxStorageUsage)
	assert.Equal(t, int64(50_000_000), umc.MonthlyBudgetMicros)
	assert.True(t, umc.ContentModerationEnabled)
}
