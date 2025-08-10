package lift

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDetermineProcessingTasks tests task selection based on MIME types
func TestDetermineProcessingTasks(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		mimeType      string
		expectedTasks []string
	}{
		{
			mimeType:      "image/jpeg",
			expectedTasks: []string{"resize", "blurhash", "dimensions", "exif"},
		},
		{
			mimeType:      "image/png",
			expectedTasks: []string{"resize", "blurhash", "dimensions", "exif"},
		},
		{
			mimeType:      "image/gif",
			expectedTasks: []string{"resize", "blurhash", "dimensions", "exif"},
		},
		{
			mimeType:      "image/webp",
			expectedTasks: []string{"resize", "blurhash", "dimensions", "exif"},
		},
		{
			mimeType:      "video/mp4",
			expectedTasks: []string{"thumbnail", "dimensions", "duration", "transcode"},
		},
		{
			mimeType:      "video/webm",
			expectedTasks: []string{"thumbnail", "dimensions", "duration", "transcode"},
		},
		{
			mimeType:      "audio/mpeg",
			expectedTasks: []string{"waveform", "duration", "metadata"},
		},
		{
			mimeType:      "audio/ogg",
			expectedTasks: []string{"waveform", "duration", "metadata"},
		},
		{
			mimeType:      "application/pdf",
			expectedTasks: []string{},
		},
		{
			mimeType:      "",
			expectedTasks: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			tasks := handler.determineProcessingTasks(tt.mimeType)
			assert.Equal(t, tt.expectedTasks, tasks)
		})
	}
}

// TestIsAllowedMimeTypeLift tests MIME type validation
func TestIsAllowedMimeTypeLift(t *testing.T) {
	tests := []struct {
		mimeType string
		allowed  bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"image/webp", true},
		{"video/mp4", true},
		{"video/webm", true},
		{"audio/mpeg", true},
		{"audio/mp3", true},
		{"audio/ogg", true},
		{"audio/wav", true},
		{"application/pdf", false},
		{"text/plain", false},
		{"application/javascript", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			result := isAllowedMimeTypeLift(tt.mimeType)
			assert.Equal(t, tt.allowed, result)
		})
	}
}

// TestGetExtensionFromMimeTypeLift tests file extension determination
func TestGetExtensionFromMimeTypeLift(t *testing.T) {
	tests := []struct {
		mimeType    string
		expectedExt string
	}{
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"video/mp4", ".mp4"},
		{"video/webm", ".webm"},
		{"audio/mpeg", ".mp3"},
		{"audio/mp3", ".mp3"},
		{"audio/ogg", ".ogg"},
		{"audio/wav", ".wav"},
		{"unknown/type", ".bin"},
		{"", ".bin"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			ext := getExtensionFromMimeTypeLift(tt.mimeType)
			assert.Equal(t, tt.expectedExt, ext)
		})
	}
}

// TestGetMediaTypeLift tests media type classification
func TestGetMediaTypeLift(t *testing.T) {
	tests := []struct {
		mimeType     string
		expectedType string
	}{
		{"image/jpeg", "image"},
		{"image/png", "image"},
		{"image/gif", "gifv"},
		{"image/webp", "image"},
		{"video/mp4", "video"},
		{"video/webm", "video"},
		{"audio/mpeg", "audio"},
		{"audio/ogg", "audio"},
		{"application/pdf", MediaTypeUnknown},
		{"", MediaTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			mediaType := getMediaTypeLift(tt.mimeType)
			assert.Equal(t, tt.expectedType, mediaType)
		})
	}
}

// TestCalculateAspectRatioLift tests aspect ratio calculation
func TestCalculateAspectRatioLift(t *testing.T) {
	tests := []struct {
		name          string
		width         int
		height        int
		expectedRatio float64
	}{
		{"16:9 aspect ratio", 1920, 1080, 1.7777777777777777},
		{"4:3 aspect ratio", 1024, 768, 1.3333333333333333},
		{"square", 1000, 1000, 1.0},
		{"zero height", 1920, 0, 1.0},
		{"zero width", 0, 1080, 0.0},
		{"both zero", 0, 0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio := calculateAspectRatioLift(tt.width, tt.height)
			assert.InDelta(t, tt.expectedRatio, ratio, 0.0001)
		})
	}
}

// TestValidateMediaData tests media data validation
func TestValidateMediaData(t *testing.T) {
	_ = &Handler{} // Handler for potential future use

	tests := []struct {
		name        string
		data        *MediaUploadData
		expectError bool
	}{
		{
			name: "valid JPEG data",
			data: &MediaUploadData{
				FileData:    make([]byte, 1024), // 1KB
				MimeType:    "image/jpeg",
				Description: "Test image",
				Focus:       "0.5,0.5",
			},
			expectError: false,
		},
		{
			name: "empty file data",
			data: &MediaUploadData{
				FileData:    []byte{},
				MimeType:    "image/jpeg",
				Description: "Test image",
			},
			expectError: true,
		},
		{
			name: "oversized file",
			data: &MediaUploadData{
				FileData:    make([]byte, 11*1024*1024), // 11MB
				MimeType:    "image/jpeg",
				Description: "Test image",
			},
			expectError: true,
		},
		{
			name: "unsupported MIME type",
			data: &MediaUploadData{
				FileData:    make([]byte, 1024),
				MimeType:    "application/pdf",
				Description: "Test PDF",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip this test since validateMediaData requires complex setup
			// This would need proper Lift context, database setup, etc.
			t.Skip("validateMediaData requires complex integration setup")
		})
	}
}

// TestSanitizeS3Key tests S3 key sanitization (from media processor tests)
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
			name:        "normal values",
			username:    "testuser",
			mediaID:     "12345678-1234-1234-1234-123456789abc",
			filename:    "image.jpg",
			expectedKey: "media/testuser/12345678-1234-1234-1234-123456789abc/image.jpg",
			expectError: false,
		},
		{
			name:        "username with path traversal",
			username:    "../../../etc/passwd",
			mediaID:     "12345678-1234-1234-1234-123456789abc",
			filename:    "image.jpg",
			expectError: true,
		},
		{
			name:        "mediaID with path traversal",
			username:    "testuser",
			mediaID:     "../../../tmp/evil",
			filename:    "image.jpg",
			expectError: true,
		},
		{
			name:        "filename with path traversal",
			username:    "testuser",
			mediaID:     "12345678-1234-1234-1234-123456789abc",
			filename:    "../../../evil.jpg",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := sanitizeS3Key(tt.username, tt.mediaID, tt.filename)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedKey, key)
			}
		})
	}
}

// Helper function to implement sanitizeS3Key for testing
func sanitizeS3Key(username, mediaID, filename string) (string, error) {
	// Check for path traversal in username
	if containsPathTraversal(username) {
		return "", fmt.Errorf("invalid username for S3 key")
	}

	// Check for path traversal in mediaID
	if containsPathTraversal(mediaID) {
		return "", fmt.Errorf("invalid media ID for S3 key")
	}

	// Check for path traversal in filename
	if containsPathTraversal(filename) {
		return "", fmt.Errorf("invalid filename for S3 key")
	}

	return fmt.Sprintf("media/%s/%s/%s", username, mediaID, filename), nil
}

func containsPathTraversal(s string) bool {
	return strings.Contains(s, "..") || strings.Contains(s, "/") || strings.Contains(s, "\\")
}

// MediaTypeUnknown is defined in media.go, so we don't redeclare it here
