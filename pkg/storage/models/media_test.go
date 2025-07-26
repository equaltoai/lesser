package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMedia_TableName(t *testing.T) {
	m := &Media{}
	assert.Equal(t, "lesser-main", m.TableName())
}

func TestMedia_BeforeCreate(t *testing.T) {
	tests := []struct {
		name    string
		media   *Media
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid media with minimal data",
			media: &Media{
				UserID:      "user123",
				FileName:    "test.jpg",
				ContentType: "image/jpeg",
				FileSize:    1024,
				S3Bucket:    "test-bucket",
				S3Key:       "test/key",
			},
			wantErr: false,
		},
		{
			name: "valid media with custom media ID",
			media: &Media{
				MediaID:     "custom-media-id",
				UserID:      "user123",
				FileName:    "test.jpg",
				ContentType: "image/jpeg",
				FileSize:    1024,
				S3Bucket:    "test-bucket",
				S3Key:       "test/key",
			},
			wantErr: false,
		},
		{
			name: "missing UserID",
			media: &Media{
				FileName:    "test.jpg",
				ContentType: "image/jpeg",
				FileSize:    1024,
			},
			wantErr: true,
			errMsg:  "UserID is required",
		},
		{
			name: "missing ContentType",
			media: &Media{
				UserID:   "user123",
				FileName: "test.jpg",
				FileSize: 1024,
			},
			wantErr: true,
			errMsg:  "ContentType is required",
		},
		{
			name: "invalid ContentType",
			media: &Media{
				UserID:      "user123",
				FileName:    "test.exe",
				ContentType: "application/exe",
				FileSize:    1024,
			},
			wantErr: true,
			errMsg:  "unsupported content type",
		},
		{
			name: "file too large",
			media: &Media{
				UserID:      "user123",
				FileName:    "huge.jpg",
				ContentType: "image/jpeg",
				FileSize:    100 * 1024 * 1024, // 100MB
			},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.media.BeforeCreate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)

				// Check that timestamps were set
				assert.False(t, tt.media.CreatedAt.IsZero())
				assert.False(t, tt.media.UpdatedAt.IsZero())
				assert.False(t, tt.media.UploadedAt.IsZero())

				// Check that ID was generated if not provided
				assert.NotEmpty(t, tt.media.MediaID)

				// Check defaults
				assert.Equal(t, "original", tt.media.Version)
				assert.Equal(t, "pending", tt.media.Status)
				assert.Equal(t, 0, tt.media.UsageCount)
				assert.NotNil(t, tt.media.ExpiresAt)

				// Check that keys were set correctly
				expectedPK := "media#" + tt.media.MediaID
				assert.Equal(t, expectedPK, tt.media.PK)
				assert.Equal(t, "version#original", tt.media.SK)

				// Check GSI keys
				assert.Equal(t, "USER_MEDIA#user123", tt.media.GSI1PK)
				assert.Contains(t, tt.media.GSI1SK, tt.media.MediaID)
				assert.Equal(t, "MEDIA_STATUS#pending", tt.media.GSI2PK)
			}
		})
	}
}

func TestMedia_Validate(t *testing.T) {
	tests := []struct {
		name    string
		media   *Media
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid media",
			media: &Media{
				MediaID:     "media123",
				UserID:      "user123",
				ContentType: "image/jpeg",
				FileSize:    1024,
				Status:      "pending",
			},
			wantErr: false,
		},
		{
			name: "empty MediaID",
			media: &Media{
				UserID:      "user123",
				ContentType: "image/jpeg",
				FileSize:    1024,
			},
			wantErr: true,
			errMsg:  "MediaID is required",
		},
		{
			name: "zero FileSize",
			media: &Media{
				MediaID:     "media123",
				UserID:      "user123",
				ContentType: "image/jpeg",
				FileSize:    0,
			},
			wantErr: true,
			errMsg:  "FileSize must be greater than 0",
		},
		{
			name: "invalid status",
			media: &Media{
				MediaID:     "media123",
				UserID:      "user123",
				ContentType: "image/jpeg",
				FileSize:    1024,
				Status:      "invalid",
			},
			wantErr: true,
			errMsg:  "invalid media status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.media.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMedia_MarkUsed(t *testing.T) {
	expires := time.Now().Add(24 * time.Hour).Unix()
	media := &Media{
		UsageCount: 0,
		ExpiresAt:  &expires,
	}

	assert.Equal(t, 0, media.UsageCount)
	assert.Nil(t, media.LastUsedAt)
	assert.NotNil(t, media.ExpiresAt)

	media.MarkUsed()

	assert.Equal(t, 1, media.UsageCount)
	assert.NotNil(t, media.LastUsedAt)
	assert.Nil(t, media.ExpiresAt) // Should be cleared for used media

	// Mark used again
	media.MarkUsed()
	assert.Equal(t, 2, media.UsageCount)
}

func TestMedia_SetProcessed(t *testing.T) {
	media := &Media{
		Status: "processing",
		Error:  "some error",
	}

	media.SetProcessed()

	assert.Equal(t, "ready", media.Status)
	assert.NotNil(t, media.ProcessedAt)
	assert.Empty(t, media.Error)
	assert.True(t, time.Since(*media.ProcessedAt) < time.Second)
}

func TestMedia_SetFailed(t *testing.T) {
	media := &Media{
		Status: "processing",
	}

	errorMsg := "processing failed"
	media.SetFailed(errorMsg)

	assert.Equal(t, "failed", media.Status)
	assert.NotNil(t, media.ProcessedAt)
	assert.Equal(t, errorMsg, media.Error)
	assert.True(t, time.Since(*media.ProcessedAt) < time.Second)
}

func TestMedia_SetProcessing(t *testing.T) {
	media := &Media{
		Status: "pending",
		Error:  "some old error",
	}

	media.SetProcessing()

	assert.Equal(t, "processing", media.Status)
	assert.Empty(t, media.Error)
}

func TestMedia_StatusCheckers(t *testing.T) {
	tests := []struct {
		status      string
		isReady     bool
		isFailed    bool
		isProcessing bool
	}{
		{"ready", true, false, false},
		{"failed", false, true, false},
		{"processing", false, false, true},
		{"pending", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			media := &Media{Status: tt.status}

			assert.Equal(t, tt.isReady, media.IsReady())
			assert.Equal(t, tt.isFailed, media.IsFailed())
			assert.Equal(t, tt.isProcessing, media.IsProcessing())
		})
	}
}

func TestMedia_VariantManagement(t *testing.T) {
	media := &Media{}

	// Test getting non-existent variant
	variant, exists := media.GetVariant("thumbnail")
	assert.False(t, exists)
	assert.Equal(t, MediaVariant{}, variant)

	// Test adding variant
	thumbnailVariant := MediaVariant{
		S3Key:       "thumb/key",
		Width:       150,
		Height:      150,
		FileSize:    5000,
		ContentType: "image/jpeg",
	}

	media.AddVariant("thumbnail", thumbnailVariant)

	// Test getting added variant
	variant, exists = media.GetVariant("thumbnail")
	assert.True(t, exists)
	assert.Equal(t, thumbnailVariant, variant)

	// Test available variants
	variants := media.GetAvailableVariants()
	assert.Len(t, variants, 1)
	assert.Contains(t, variants, "thumbnail")

	// Add another variant
	mediumVariant := MediaVariant{
		S3Key:       "medium/key",
		Width:       500,
		Height:      500,
		FileSize:    25000,
		ContentType: "image/jpeg",
	}

	media.AddVariant("medium", mediumVariant)

	variants = media.GetAvailableVariants()
	assert.Len(t, variants, 2)
	assert.Contains(t, variants, "thumbnail")
	assert.Contains(t, variants, "medium")
}

func TestMedia_GetBestVariant(t *testing.T) {
	media := &Media{
		S3Key:       "original/key",
		Width:       1920,
		Height:      1080,
		FileSize:    100000,
		ContentType: "image/jpeg",
	}

	// Test with no variants - should return original
	best := media.GetBestVariant(800, 600)
	assert.Equal(t, "original/key", best.S3Key)
	assert.Equal(t, 1920, best.Width)

	// Add variants
	media.AddVariant("small", MediaVariant{
		S3Key:  "small/key",
		Width:  200,
		Height: 200,
		FileSize: 5000,
	})

	media.AddVariant("medium", MediaVariant{
		S3Key:  "medium/key",
		Width:  500,
		Height: 500,
		FileSize: 25000,
	})

	media.AddVariant("large", MediaVariant{
		S3Key:  "large/key",
		Width:  1000,
		Height: 1000,
		FileSize: 75000,
	})

	// Test getting best variant for 800x600 - should get medium (500x500)
	best = media.GetBestVariant(800, 600)
	assert.Equal(t, "medium/key", best.S3Key)
	assert.Equal(t, 500, best.Width)

	// Test getting best variant for 300x300 - should get small (200x200)
	best = media.GetBestVariant(300, 300)
	assert.Equal(t, "small/key", best.S3Key)
	assert.Equal(t, 200, best.Width)

	// Test getting best variant for 100x100 - should get smallest (small)
	best = media.GetBestVariant(100, 100)
	assert.Equal(t, "small/key", best.S3Key)
	assert.Equal(t, 200, best.Width)
}

func TestMedia_SetModeration(t *testing.T) {
	media := &Media{}

	labels := []string{"safe", "person"}
	media.SetModeration(false, 0.1, labels)

	assert.False(t, media.IsNSFW)
	assert.Equal(t, 0.1, media.ModerationScore)
	assert.Equal(t, labels, media.Labels)

	// Test NSFW content
	nsfwLabels := []string{"adult", "explicit"}
	media.SetModeration(true, 0.9, nsfwLabels)

	assert.True(t, media.IsNSFW)
	assert.Equal(t, 0.9, media.ModerationScore)
	assert.Equal(t, nsfwLabels, media.Labels)
}

func TestMedia_ContentTypeCheckers(t *testing.T) {
	tests := []struct {
		contentType string
		isImage     bool
		isVideo     bool
		isAudio     bool
	}{
		{"image/jpeg", true, false, false},
		{"image/png", true, false, false},
		{"video/mp4", false, true, false},
		{"video/webm", false, true, false},
		{"audio/mpeg", false, false, true},
		{"audio/wav", false, false, true},
		{"application/pdf", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			media := &Media{ContentType: tt.contentType}

			assert.Equal(t, tt.isImage, media.IsImage())
			assert.Equal(t, tt.isVideo, media.IsVideo())
			assert.Equal(t, tt.isAudio, media.IsAudio())
		})
	}
}

func TestMedia_GetTotalSize(t *testing.T) {
	media := &Media{
		FileSize: 100000,
	}

	// No variants
	assert.Equal(t, int64(100000), media.GetTotalSize())

	// Add variants
	media.AddVariant("small", MediaVariant{FileSize: 5000})
	media.AddVariant("medium", MediaVariant{FileSize: 25000})

	// Should include all variants
	expected := int64(100000 + 5000 + 25000)
	assert.Equal(t, expected, media.GetTotalSize())
}

func TestIsValidMediaType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		// Valid image types
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"image/webp", true},
		
		// Valid video types
		{"video/mp4", true},
		{"video/webm", true},
		{"video/ogg", true},
		
		// Valid audio types
		{"audio/mpeg", true},
		{"audio/wav", true},
		{"audio/ogg", true},
		
		// Invalid types
		{"application/pdf", false},
		{"text/plain", false},
		{"application/exe", false},
		{"", false},
		
		// Case sensitivity
		{"IMAGE/JPEG", true}, // Should handle case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := isValidMediaType(tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidMediaStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected bool
	}{
		{"pending", true},
		{"processing", true},
		{"ready", true},
		{"failed", true},
		{"PENDING", true}, // Case insensitive
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := isValidMediaStatus(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMedia_setupGSIKeys(t *testing.T) {
	uploadTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	media := &Media{
		MediaID:     "media123",
		UserID:      "user123",
		Status:      "ready",
		ContentType: "image/jpeg",
		UploadedAt:  uploadTime,
	}

	media.setupGSIKeys()

	expectedTimeStr := "2023-01-01T12:00:00Z"

	assert.Equal(t, "USER_MEDIA#user123", media.GSI1PK)
	assert.Equal(t, expectedTimeStr+"#media123", media.GSI1SK)
	assert.Equal(t, "MEDIA_STATUS#ready", media.GSI2PK)
	assert.Equal(t, expectedTimeStr+"#media123", media.GSI2SK)
	assert.Equal(t, "CONTENT_TYPE#image", media.GSI3PK)
	assert.Equal(t, expectedTimeStr+"#media123", media.GSI3SK)

	// Test with video content type
	media.ContentType = "video/mp4"
	media.setupGSIKeys()
	assert.Equal(t, "CONTENT_TYPE#video", media.GSI3PK)

	// Test with empty UserID
	media.UserID = ""
	media.setupGSIKeys()
	assert.Empty(t, media.GSI1PK)
	assert.Empty(t, media.GSI1SK)
}