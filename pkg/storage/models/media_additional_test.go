package models

import (
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestMediaCategory_Helpers(t *testing.T) {
	t.Parallel()

	require.Equal(t, MediaCategoryGifv, DetermineMediaCategory("image/gif; charset=utf-8"))
	require.Equal(t, MediaCategoryImage, DetermineMediaCategory("IMAGE/PNG"))
	require.Equal(t, MediaCategoryVideo, DetermineMediaCategory("video/mp4"))
	require.Equal(t, MediaCategoryAudio, DetermineMediaCategory("audio/mpeg"))
	require.Equal(t, MediaCategoryUnknown, DetermineMediaCategory("application/octet-stream"))

	require.True(t, IsValidMediaCategory(""))
	require.True(t, IsValidMediaCategory(MediaCategoryImage))
	require.False(t, IsValidMediaCategory(MediaCategory("bogus")))

	cat, ok := NormalizeMediaCategory("")
	require.False(t, ok)
	require.Equal(t, MediaCategory(""), cat)

	cat, ok = NormalizeMediaCategory("bogus")
	require.False(t, ok)
	require.Equal(t, MediaCategoryUnknown, cat)

	cat, ok = NormalizeMediaCategory(" IMAGE ")
	require.True(t, ok)
	require.Equal(t, MediaCategoryImage, cat)
}

func TestMedia_BeforeCreateAndValidate(t *testing.T) {
	t.Parallel()

	m := &Media{
		UserID:      "u1",
		FileName:    "photo.png",
		ContentType: "image/png",
		FileSize:    1024,
		S3Bucket:    "b",
		S3Key:       "k",
		SpoilerText: "  spoiler  ",
	}

	require.NoError(t, m.BeforeCreate())
	require.NotEmpty(t, m.MediaID)
	require.Equal(t, "original", m.Version)
	require.Equal(t, StatusPending, m.Status)
	require.Equal(t, MediaCategoryImage, m.MediaCategory)
	require.NotZero(t, m.ExpiresAt)
	require.Equal(t, "media#"+m.MediaID, m.PK)
	require.Equal(t, "version#"+m.Version, m.SK)
	require.Equal(t, "USER_MEDIA#u1", m.GSI1PK)
	require.Contains(t, m.GSI1SK, "#"+m.MediaID)
	require.Equal(t, "MEDIA_STATUS#"+m.Status, m.GSI2PK)
	require.Equal(t, "CONTENT_TYPE#image", m.GSI3PK)
	require.Equal(t, "spoiler", m.SpoilerText)

	// Explicit category is normalized.
	m2 := &Media{
		UserID:        "u1",
		ContentType:   "image/png",
		FileSize:      1024,
		Status:        StatusPending,
		MediaCategory: MediaCategory(" IMAGE "),
	}
	require.NoError(t, m2.BeforeCreate())
	require.Equal(t, MediaCategoryImage, m2.MediaCategory)
}

func TestMedia_BeforeUpdate_NormalizesAndValidates(t *testing.T) {
	t.Parallel()

	m := &Media{
		MediaID:       "m1",
		UserID:        "u1",
		ContentType:   "video/mp4",
		FileSize:      1024,
		Status:        "READY",
		MediaCategory: MediaCategory(" VIDEO "),
		SpoilerText:   "  spoiler  ",
		UploadedAt:    time.Now().Add(-time.Hour),
	}

	require.NoError(t, m.BeforeUpdate())
	require.Equal(t, MediaCategoryVideo, m.MediaCategory)
	require.Equal(t, "spoiler", m.SpoilerText)
	require.Equal(t, "MEDIA_STATUS#"+m.Status, m.GSI2PK)
	require.Equal(t, "CONTENT_TYPE#video", m.GSI3PK)
}

func TestMedia_Validate_ErrorCases(t *testing.T) {
	t.Parallel()

	// Missing required fields.
	require.Error(t, (&Media{}).Validate())

	// Zero file size.
	err := (&Media{
		MediaID:       "m1",
		UserID:        "u1",
		ContentType:   "image/png",
		Status:        StatusPending,
		FileSize:      0,
		MediaCategory: MediaCategoryImage,
	}).Validate()
	require.ErrorIs(t, err, ErrFileSizeZero)

	// Too large.
	err = (&Media{
		MediaID:       "m1",
		UserID:        "u1",
		ContentType:   "image/png",
		Status:        StatusPending,
		FileSize:      60 * 1024 * 1024,
		MediaCategory: MediaCategoryImage,
	}).Validate()
	require.ErrorIs(t, err, ErrFileSizeTooLarge)

	// Unsupported content type.
	err = (&Media{
		MediaID:       "m1",
		UserID:        "u1",
		ContentType:   "application/json",
		Status:        StatusPending,
		FileSize:      1024,
		MediaCategory: MediaCategoryUnknown,
	}).Validate()
	require.ErrorIs(t, err, ErrUnsupportedContentType)

	// Invalid status.
	err = (&Media{
		MediaID:       "m1",
		UserID:        "u1",
		ContentType:   "image/png",
		Status:        "nope",
		FileSize:      1024,
		MediaCategory: MediaCategoryImage,
	}).Validate()
	require.ErrorIs(t, err, ErrInvalidMediaStatus)

	// Spoiler text too long.
	err = (&Media{
		MediaID:       "m1",
		UserID:        "u1",
		ContentType:   "image/png",
		Status:        StatusPending,
		FileSize:      1024,
		SpoilerText:   strings.Repeat("x", common.MaxStatusSpoiler+1),
		MediaCategory: MediaCategoryImage,
	}).Validate()
	require.Error(t, err)

	// Invalid category.
	err = (&Media{
		MediaID:       "m1",
		UserID:        "u1",
		ContentType:   "image/png",
		Status:        StatusPending,
		FileSize:      1024,
		MediaCategory: MediaCategory("bogus"),
	}).Validate()
	require.ErrorIs(t, err, ErrInvalidMediaCategory)
}

func TestMedia_StateHelpersAndVariants(t *testing.T) {
	t.Parallel()

	m := &Media{
		MediaID:       "m1",
		UserID:        "u1",
		ContentType:   "image/png",
		Status:        StatusPending,
		FileSize:      1024,
		S3Key:         "orig",
		CDNUrl:        "cdn",
		Width:         1000,
		Height:        800,
		MediaCategory: MediaCategoryImage,
	}

	require.False(t, m.IsReady())
	require.False(t, m.IsFailed())
	require.False(t, m.IsProcessing())

	m.SetProcessing()
	require.True(t, m.IsProcessing())

	m.SetFailed("boom")
	require.True(t, m.IsFailed())
	require.Equal(t, "boom", m.Error)
	require.NotNil(t, m.ProcessedAt)

	m.SetProcessed()
	require.True(t, m.IsReady())
	require.Empty(t, m.Error)

	m.MarkUsed()
	require.Equal(t, 1, m.UsageCount)
	require.NotNil(t, m.LastUsedAt)
	require.Equal(t, int64(0), m.ExpiresAt)

	// Variants.
	_, ok := m.GetVariant("thumb")
	require.False(t, ok)
	require.Empty(t, m.GetAvailableVariants())

	m.AddVariant("small", MediaVariant{S3Key: "small", Width: 200, Height: 200, FileSize: 10, ContentType: "image/png"})
	m.AddVariant("large", MediaVariant{S3Key: "large", Width: 800, Height: 800, FileSize: 20, ContentType: "image/png"})
	m.AddVariant("oversize", MediaVariant{S3Key: "oversize", Width: 5000, Height: 5000, FileSize: 30, ContentType: "image/png"})

	variant, ok := m.GetVariant("large")
	require.True(t, ok)
	require.Equal(t, "large", variant.S3Key)

	names := m.GetAvailableVariants()
	require.Len(t, names, 3)

	// Picks best fit within constraints (800x800).
	best := m.GetBestVariant(800, 800)
	require.Equal(t, "large", best.S3Key)

	// When all variants are too large, pick smallest one.
	best = m.GetBestVariant(50, 50)
	require.Equal(t, "small", best.S3Key)

	require.Equal(t, int64(1024+10+20+30), m.GetTotalSize())

	require.True(t, m.IsImage())
	require.False(t, m.IsVideo())
	require.False(t, m.IsAudio())
}
