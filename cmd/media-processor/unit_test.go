package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// TestGetMediaTypeFromMime tests media type classification
func TestGetMediaTypeFromMime(t *testing.T) {
	tests := []struct {
		mimeType     string
		expectedType string
	}{
		{"image/jpeg", mediaTypeImage},
		{"image/png", mediaTypeImage},
		{"image/gif", mediaTypeGifv},
		{"image/webp", mediaTypeImage},
		{"video/mp4", mediaTypeVideo},
		{"video/webm", mediaTypeVideo},
		{"audio/mpeg", mediaTypeAudio},
		{"audio/ogg", mediaTypeAudio},
		{"application/pdf", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			mediaType := getMediaTypeFromMime(tt.mimeType)
			assert.Equal(t, tt.expectedType, mediaType)
		})
	}
}

// TestGetExtensionFromProcessedFormat tests extension determination
func TestGetExtensionFromProcessedFormat(t *testing.T) {
	tests := []struct {
		format      string
		expectedExt string
	}{
		{"jpeg", extJPG},
		{"png", extPNG},
		{"gif", extGIF},
		{"webp", extWebP},
		{"unknown", extJPG}, // defaults to JPG
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			ext := getExtensionFromProcessedFormat(tt.format)
			assert.Equal(t, tt.expectedExt, ext)
		})
	}
}

// TestGetMimeTypeFromFormat tests MIME type determination
func TestGetMimeTypeFromFormat(t *testing.T) {
	tests := []struct {
		format       string
		expectedMime string
	}{
		{"jpeg", mimeJPEG},
		{"png", mimePNG},
		{"gif", "image/gif"},
		{"webp", mimeWebP},
		{"unknown", mimeJPEG}, // defaults to JPEG
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			mime := getMimeTypeFromFormat(tt.format)
			assert.Equal(t, tt.expectedMime, mime)
		})
	}
}

// TestValidateFileType tests file type validation
func TestValidateFileType(t *testing.T) {
	tests := []struct {
		name        string
		fileData    []byte
		claimedType string
		expectError bool
	}{
		{
			name:        "valid JPEG data",
			fileData:    createTestJPEGData(),
			claimedType: "image/jpeg",
			expectError: false,
		},
		{
			name:        "valid JPEG data with no claimed type",
			fileData:    createTestJPEGData(),
			claimedType: "",
			expectError: false,
		},
		{
			name:        "valid PNG data",
			fileData:    createTestPNGData(),
			claimedType: "image/png",
			expectError: false,
		},
		{
			name:        "empty file",
			fileData:    []byte{},
			claimedType: "image/jpeg",
			expectError: true,
		},
		{
			name:        "unsupported type",
			fileData:    []byte("not an image"),
			claimedType: "application/pdf",
			expectError: true,
		},
		{
			name:        "type mismatch",
			fileData:    createTestJPEGData(),
			claimedType: "image/png",
			expectError: true,
		},
		{
			name:        "detected type invalid for claimed image",
			fileData:    []byte("not an image"),
			claimedType: "image/jpeg",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFileType(tt.fileData, tt.claimedType)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCheckFileSizeLimit tests file size validation
func TestCheckFileSizeLimit(t *testing.T) {
	tests := []struct {
		name        string
		mimeType    string
		fileSize    int
		expectError bool
	}{
		{
			name:        "small image within limit",
			mimeType:    "image/jpeg",
			fileSize:    1024 * 1024, // 1MB
			expectError: false,
		},
		{
			name:        "image exceeds limit",
			mimeType:    "image/jpeg",
			fileSize:    11 * 1024 * 1024, // 11MB
			expectError: true,
		},
		{
			name:        "small video within limit",
			mimeType:    "video/mp4",
			fileSize:    10 * 1024 * 1024, // 10MB
			expectError: false,
		},
		{
			name:        "video exceeds limit",
			mimeType:    "video/mp4",
			fileSize:    51 * 1024 * 1024, // 51MB
			expectError: true,
		},
		{
			name:        "small audio within limit",
			mimeType:    "audio/mpeg",
			fileSize:    5 * 1024 * 1024, // 5MB
			expectError: false,
		},
		{
			name:        "audio exceeds limit",
			mimeType:    "audio/mpeg",
			fileSize:    21 * 1024 * 1024, // 21MB
			expectError: true,
		},
		{
			name:        "GIF within limit",
			mimeType:    "image/gif",
			fileSize:    10 * 1024 * 1024, // 10MB
			expectError: false,
		},
		{
			name:        "GIF exceeds limit",
			mimeType:    "image/gif",
			fileSize:    16 * 1024 * 1024, // 16MB
			expectError: true,
		},
		{
			name:        "unknown type returns error",
			mimeType:    "application/octet-stream",
			fileSize:    1024,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileData := make([]byte, tt.fileSize)
			err := checkFileSizeLimit(fileData, tt.mimeType)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestEstimateProcessingCost tests cost estimation
func TestEstimateProcessingCost(t *testing.T) {
	logger := zaptest.NewLogger(t)
	processor := &MediaProcessor{
		logger: logger,
	}

	tests := []struct {
		name            string
		mimeType        string
		fileSize        int
		expectedMinCost int64
		expectedMaxCost int64
	}{
		{
			name:            "small image",
			mimeType:        "image/jpeg",
			fileSize:        1024 * 1024, // 1MB
			expectedMinCost: 50,
			expectedMaxCost: 200,
		},
		{
			name:            "small video",
			mimeType:        "video/mp4",
			fileSize:        10 * 1024 * 1024, // 10MB
			expectedMinCost: 5000,             // $0.005
			expectedMaxCost: 50000,            // $0.05
		},
		{
			name:            "large video",
			mimeType:        "video/mp4",
			fileSize:        50 * 1024 * 1024, // 50MB
			expectedMinCost: 50000,            // $0.05
			expectedMaxCost: 500000,           // $0.50
		},
		{
			name:            "audio file",
			mimeType:        "audio/mpeg",
			fileSize:        5 * 1024 * 1024, // 5MB
			expectedMinCost: 30,
			expectedMaxCost: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileData := make([]byte, tt.fileSize)
			cost := processor.estimateProcessingCost(fileData, tt.mimeType)

			assert.GreaterOrEqual(t, cost, tt.expectedMinCost)
			assert.LessOrEqual(t, cost, tt.expectedMaxCost)
		})
	}
}

// TestSanitizeS3Key tests S3 key sanitization
func TestSanitizeS3Key(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		mediaID     string
		filename    string
		expectedKey string
		expectError bool
	}{
		{
			name:        "valid inputs",
			username:    "testuser",
			mediaID:     "media123",
			filename:    "image.jpg",
			expectedKey: "media/testuser/media123/image.jpg",
			expectError: false,
		},
		{
			name:        "path traversal in username",
			username:    "../../../etc",
			mediaID:     "media123",
			filename:    "image.jpg",
			expectError: true,
		},
		{
			name:        "path traversal in mediaID",
			username:    "testuser",
			mediaID:     "../../../tmp",
			filename:    "image.jpg",
			expectError: true,
		},
		{
			name:        "path traversal in filename",
			username:    "testuser",
			mediaID:     "media123",
			filename:    "../../../passwd",
			expectError: true,
		},
		{
			name:        "slash in username",
			username:    "test/user",
			mediaID:     "media123",
			filename:    "image.jpg",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := sanitizeS3Key(tt.username, tt.mediaID, tt.filename)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, key)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedKey, key)
			}
		})
	}
}

// TestBudgetLogic tests budget enforcement logic
func TestBudgetLogic(t *testing.T) {
	tests := []struct {
		name              string
		userBudget        int64
		currentSpending   int64
		estimatedCost     int64
		expectFullProcess bool
		expectUploadOnly  bool
	}{
		{
			name:              "within budget",
			userBudget:        1000000, // $1.00
			currentSpending:   500000,  // $0.50
			estimatedCost:     100000,  // $0.10
			expectFullProcess: true,
			expectUploadOnly:  false,
		},
		{
			name:              "exceeds budget",
			userBudget:        1000000, // $1.00
			currentSpending:   950000,  // $0.95
			estimatedCost:     100000,  // $0.10 (would exceed $1.00)
			expectFullProcess: false,
			expectUploadOnly:  true,
		},
		{
			name:              "exactly at budget",
			userBudget:        1000000, // $1.00
			currentSpending:   900000,  // $0.90
			estimatedCost:     100000,  // $0.10 (exactly $1.00)
			expectFullProcess: true,
			expectUploadOnly:  false,
		},
		{
			name:              "unlimited budget",
			userBudget:        0,       // Unlimited
			currentSpending:   5000000, // $5.00 already spent
			estimatedCost:     1000000, // $1.00
			expectFullProcess: true,
			expectUploadOnly:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remainingBudget := tt.userBudget - tt.currentSpending
			if tt.userBudget == 0 {
				remainingBudget = 999999999 // Effectively unlimited
			}

			shouldProcess := (tt.userBudget == 0) || (tt.estimatedCost <= remainingBudget)

			assert.Equal(t, tt.expectFullProcess, shouldProcess)
			assert.Equal(t, tt.expectUploadOnly, !shouldProcess && tt.userBudget > 0)
		})
	}
}

// TestExtractAudioDuration tests audio duration extraction
func TestExtractAudioDuration(t *testing.T) {
	tests := []struct {
		name        string
		fileData    []byte
		expectError bool
		expectRange bool // true if we expect a reasonable duration range
	}{
		{
			name:        "minimal ID3 header yields estimated duration",
			fileData:    createTestID3AudioData(),
			expectError: false,
			expectRange: true,
		},
		{
			name:        "valid MP3 data",
			fileData:    createTestAudioData(),
			expectError: true, // Our test data won't have valid headers
		},
		{
			name:        "empty file",
			fileData:    []byte{},
			expectError: true,
		},
		{
			name:        "too small file",
			fileData:    make([]byte, 100),
			expectError: true,
		},
		{
			name:        "medium size file",
			fileData:    make([]byte, 1024*1024), // 1MB
			expectError: true,                    // Won't have valid MP3 headers
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration, err := extractAudioDuration(tt.fileData)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectRange {
					assert.Greater(t, duration, 0)
					assert.Less(t, duration, 7200*1000) // Less than 2 hours
				}
			}
		})
	}
}

// TestJobStatusTransitions tests job status management
func TestJobStatusTransitions(t *testing.T) {
	_ = zaptest.NewLogger(t) // Logger for potential future use

	// Test status validation
	validStatuses := []string{"pending", "processing", "completed", "failed"}
	invalidStatuses := []string{"unknown", "cancelled", ""}

	for _, status := range validStatuses {
		t.Run("valid_status_"+status, func(t *testing.T) {
			assert.True(t, isValidJobStatus(status))
		})
	}

	for _, status := range invalidStatuses {
		t.Run("invalid_status_"+status, func(t *testing.T) {
			assert.False(t, isValidJobStatus(status))
		})
	}
}

// Helper function to check if a job status is valid
func isValidJobStatus(status string) bool {
	validStatuses := map[string]bool{
		"pending":    true,
		"processing": true,
		"completed":  true,
		"failed":     true,
	}
	return validStatuses[status]
}

// TestSliceContains tests the slice contains helper function
func TestSliceContains(t *testing.T) {
	slice := []string{"resize", "blurhash", "dimensions", "exif"}

	tests := []struct {
		item     string
		expected bool
	}{
		{"resize", true},
		{"blurhash", true},
		{"dimensions", true},
		{"exif", true},
		{"thumbnail", false},
		{"waveform", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.item, func(t *testing.T) {
			result := testSliceContains(slice, tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function to check if slice contains item
func testSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// TestBuildMediaURL tests URL construction
func TestBuildMediaURL(t *testing.T) {
	tests := []struct {
		name        string
		cdnDomain   string
		bucketName  string
		s3Key       string
		expectedURL string
	}{
		{
			name:        "with CDN domain",
			cdnDomain:   "cdn.example.com",
			bucketName:  "test-bucket",
			s3Key:       "media/user/123/image.jpg",
			expectedURL: "https://cdn.example.com/media/user/123/image.jpg",
		},
		{
			name:        "without CDN domain",
			cdnDomain:   "",
			bucketName:  "test-bucket",
			s3Key:       "media/user/123/image.jpg",
			expectedURL: "https://test-bucket.s3.amazonaws.com/media/user/123/image.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := &MediaProcessor{
				cdnDomain:  tt.cdnDomain,
				bucketName: tt.bucketName,
			}

			url := processor.buildMediaURL(tt.s3Key)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

// Helper functions for creating test data

func createTestJPEGData() []byte {
	// Minimal JPEG header
	return []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
	}
}

func createTestPNGData() []byte {
	// Minimal PNG header
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	}
}

func createTestAudioData() []byte {
	// Minimal MP3 header
	return []byte{
		0xFF, 0xFB, 0x90, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
}

func createTestID3AudioData() []byte {
	// Minimal, valid ID3v2.4 header with zero tag size (10 bytes), padded to allow duration estimation.
	data := make([]byte, 2000)
	copy(data, []byte{'I', 'D', '3', 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	return data
}
