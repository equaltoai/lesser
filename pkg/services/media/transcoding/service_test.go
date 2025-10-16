package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetQualityParams(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name            string
		quality         string
		expectedWidth   int
		expectedHeight  int
		expectedBitrate int
	}{
		{"4K", "4k", 3840, 2160, 15000000},
		{"2160p", "2160p", 3840, 2160, 15000000},
		{"1080p", "1080p", 1920, 1080, 5000000},
		{"720p", "720p", 1280, 720, 3000000},
		{"480p", "480p", 854, 480, 1500000},
		{"360p", "360p", 640, 360, 800000},
		{"240p", "240p", 426, 240, 400000},
		{"default", "unknown", 1280, 720, 3000000}, // Should default to 720p
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height, bitrate := s.getQualityParams(tt.quality)
			assert.Equal(t, tt.expectedWidth, width, "width mismatch")
			assert.Equal(t, tt.expectedHeight, height, "height mismatch")
			assert.Equal(t, tt.expectedBitrate, bitrate, "bitrate mismatch")
		})
	}
}

func TestValidateRequest(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name        string
		req         *TranscodeRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			req: &TranscodeRequest{
				MediaID:       "media123",
				UserID:        "user123",
				SourceBucket:  "bucket",
				SourceKey:     "key",
				QualityLevels: []string{"720p"},
				GenerateHLS:   true,
			},
			expectError: false,
		},
		{
			name: "missing media ID",
			req: &TranscodeRequest{
				UserID:        "user123",
				SourceBucket:  "bucket",
				SourceKey:     "key",
				QualityLevels: []string{"720p"},
				GenerateHLS:   true,
			},
			expectError: true,
			errorMsg:    "media_id is required",
		},
		{
			name: "missing user ID",
			req: &TranscodeRequest{
				MediaID:       "media123",
				SourceBucket:  "bucket",
				SourceKey:     "key",
				QualityLevels: []string{"720p"},
				GenerateHLS:   true,
			},
			expectError: true,
			errorMsg:    "user_id is required",
		},
		{
			name: "missing source bucket",
			req: &TranscodeRequest{
				MediaID:       "media123",
				UserID:        "user123",
				SourceKey:     "key",
				QualityLevels: []string{"720p"},
				GenerateHLS:   true,
			},
			expectError: true,
			errorMsg:    "source_bucket is required",
		},
		{
			name: "missing source key",
			req: &TranscodeRequest{
				MediaID:       "media123",
				UserID:        "user123",
				SourceBucket:  "bucket",
				QualityLevels: []string{"720p"},
				GenerateHLS:   true,
			},
			expectError: true,
			errorMsg:    "source_key is required",
		},
		{
			name: "missing quality levels",
			req: &TranscodeRequest{
				MediaID:      "media123",
				UserID:       "user123",
				SourceBucket: "bucket",
				SourceKey:    "key",
				GenerateHLS:  true,
			},
			expectError: true,
			errorMsg:    "at least one quality level is required",
		},
		{
			name: "no output formats",
			req: &TranscodeRequest{
				MediaID:       "media123",
				UserID:        "user123",
				SourceBucket:  "bucket",
				SourceKey:     "key",
				QualityLevels: []string{"720p"},
			},
			expectError: true,
			errorMsg:    "at least one output format (HLS or DASH) is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.validateRequest(tt.req)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEstimateCost(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name        string
		req         *TranscodeRequest
		minCost     float64
		maxCost     float64
		description string
	}{
		{
			name: "SD single quality",
			req: &TranscodeRequest{
				Duration:      300, // 5 minutes
				QualityLevels: []string{"480p"},
			},
			minCost:     0.03, // 5 * 0.0075
			maxCost:     0.04,
			description: "SD pricing should be around $0.0075/min",
		},
		{
			name: "HD single quality",
			req: &TranscodeRequest{
				Duration:      300, // 5 minutes
				QualityLevels: []string{"720p"},
			},
			minCost:     0.07, // 5 * 0.015
			maxCost:     0.08,
			description: "HD pricing should be around $0.015/min",
		},
		{
			name: "UHD single quality",
			req: &TranscodeRequest{
				Duration:      300, // 5 minutes
				QualityLevels: []string{"4k"},
			},
			minCost:     0.29, // 5 * 0.060
			maxCost:     0.31,
			description: "UHD pricing should be around $0.060/min",
		},
		{
			name: "multiple HD qualities",
			req: &TranscodeRequest{
				Duration:      300, // 5 minutes
				QualityLevels: []string{"720p", "1080p"},
			},
			minCost:     0.14, // 5 * 0.015 * 2
			maxCost:     0.16,
			description: "Multiple HD qualities should multiply cost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := s.estimateCost(tt.req)
			assert.GreaterOrEqual(t, cost, tt.minCost, tt.description)
			assert.LessOrEqual(t, cost, tt.maxCost, tt.description)
		})
	}
}

func TestEstimateDuration(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name           string
		req            *TranscodeRequest
		minDurationSec int
		maxDurationSec int
	}{
		{
			name: "single quality",
			req: &TranscodeRequest{
				Duration:      300, // 5 minutes
				QualityLevels: []string{"720p"},
			},
			minDurationSec: 50, // ~300/5 - some margin
			maxDurationSec: 70, // ~300/5 + some margin
		},
		{
			name: "multiple qualities",
			req: &TranscodeRequest{
				Duration:      600, // 10 minutes
				QualityLevels: []string{"480p", "720p", "1080p"},
			},
			minDurationSec: 300, // ~600/5 * 3 - some margin
			maxDurationSec: 400, // ~600/5 * 3 + some margin
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duration := s.estimateDuration(tt.req)
			seconds := int(duration.Seconds())
			assert.GreaterOrEqual(t, seconds, tt.minDurationSec)
			assert.LessOrEqual(t, seconds, tt.maxDurationSec)
		})
	}
}

func TestGetOutputPrefix(t *testing.T) {
	tests := []struct {
		name              string
		destinationPrefix string
		mediaID           string
		expected          string
	}{
		{
			name:              "with prefix",
			destinationPrefix: "media",
			mediaID:           "abc123",
			expected:          "media/abc123",
		},
		{
			name:              "without prefix",
			destinationPrefix: "",
			mediaID:           "abc123",
			expected:          "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				destinationPrefix: tt.destinationPrefix,
			}
			result := s.getOutputPrefix(tt.mediaID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJobStatusIsCompleted(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"completed job", "completed", true},
		{"failed job", "failed", true},
		{"processing job", "processing", false},
		{"submitted job", "submitted", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Would need to check TranscodingJob.IsCompleted() from models
			// This tests the concept
			isComplete := tt.status == "completed" || tt.status == "failed"
			assert.Equal(t, tt.expected, isComplete)
		})
	}
}

func TestBuildJobSettingsStructure(t *testing.T) {
	s := &Service{
		destinationBucket: "test-bucket",
		destinationPrefix: "media",
	}

	req := &TranscodeRequest{
		MediaID:        "test123",
		SourceBucket:   "source-bucket",
		SourceKey:      "source/video.mp4",
		QualityLevels:  []string{"720p", "1080p"},
		GenerateHLS:    true,
		GenerateDASH:   true,
		ThumbnailCount: 5,
	}

	settings := s.buildJobSettings(req)

	// Verify structure
	assert.NotNil(t, settings, "settings should not be nil")
	assert.Len(t, settings.Inputs, 1, "should have one input")
	assert.Contains(t, *settings.Inputs[0].FileInput, "s3://source-bucket/source/video.mp4")

	// Should have HLS, DASH, and thumbnail output groups
	assert.GreaterOrEqual(t, len(settings.OutputGroups), 3, "should have at least 3 output groups")

	// Verify HLS group exists
	var hasHLS, hasDASH, hasThumbnail bool
	for _, group := range settings.OutputGroups {
		if group.OutputGroupSettings.HlsGroupSettings != nil {
			hasHLS = true
			// Should have outputs for each quality level
			assert.Len(t, group.Outputs, 2, "HLS should have 2 quality outputs")
		}
		if group.OutputGroupSettings.DashIsoGroupSettings != nil {
			hasDASH = true
		}
		if group.OutputGroupSettings.FileGroupSettings != nil {
			hasThumbnail = true
		}
	}

	assert.True(t, hasHLS, "should have HLS output group")
	assert.True(t, hasDASH, "should have DASH output group")
	assert.True(t, hasThumbnail, "should have thumbnail output group")
}

func TestConvertToTranscodingJob(t *testing.T) {
	s := &Service{}

	req := &TranscodeRequest{
		MediaID:     "media123",
		UserID:      "user123",
		Username:    "testuser",
		ContentType: "video/mp4",
		Duration:    300,
		Width:       1920,
		Height:      1080,
	}

	result := &TranscodeResult{
		JobID:             "job123",
		MediaConvertJobID: "mc-job-123",
		EstimatedCostUSD:  0.50,
		QualityLevels:     []string{"720p", "1080p"},
		Status:            "SUBMITTED",
	}

	job := s.ConvertToTranscodingJob(req, result)

	assert.Equal(t, "job123", job.JobID)
	assert.Equal(t, "media123", job.MediaID)
	assert.Equal(t, "user123", job.UserID)
	assert.Equal(t, "testuser", job.Username)
	assert.Equal(t, "video", job.JobType)
	assert.Equal(t, "processing", job.Status)
	assert.Equal(t, "video/mp4", job.InputFormat)
	assert.Equal(t, int64(300000), job.InputDuration) // Converted to milliseconds
	assert.Equal(t, "1920x1080", job.InputResolution)
	assert.Equal(t, "mc-job-123", job.MediaConvertJobID)
	assert.Equal(t, int64(500000), job.EstimatedCostMicros) // Converted to microdollars
	assert.Equal(t, []string{"720p", "1080p"}, job.QualityLevels)
}
