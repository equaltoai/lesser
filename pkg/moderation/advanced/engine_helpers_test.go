package advanced

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGetHighestSeverity(t *testing.T) {
	// Create a minimal engine for testing
	engine := &Engine{logger: zap.NewNop()}

	tests := []struct {
		name     string
		reasons  []DecisionReason
		expected Severity
	}{
		{
			name:     "empty reasons returns low severity",
			reasons:  []DecisionReason{},
			expected: SeverityLow,
		},
		{
			name: "single critical returns critical",
			reasons: []DecisionReason{
				{Type: "threat", Severity: SeverityCritical},
			},
			expected: SeverityCritical,
		},
		{
			name: "single low returns low",
			reasons: []DecisionReason{
				{Type: "pii", Severity: SeverityLow},
			},
			expected: SeverityLow,
		},
		{
			name: "mixed severities returns highest",
			reasons: []DecisionReason{
				{Type: "pii", Severity: SeverityLow},
				{Type: "toxicity", Severity: SeverityMedium},
				{Type: "threat", Severity: SeverityHigh},
			},
			expected: SeverityHigh,
		},
		{
			name: "critical among mixed returns critical",
			reasons: []DecisionReason{
				{Type: "low", Severity: SeverityLow},
				{Type: "medium", Severity: SeverityMedium},
				{Type: "critical", Severity: SeverityCritical},
				{Type: "high", Severity: SeverityHigh},
			},
			expected: SeverityCritical,
		},
		{
			name: "all same severity returns that severity",
			reasons: []DecisionReason{
				{Type: "a", Severity: SeverityMedium},
				{Type: "b", Severity: SeverityMedium},
				{Type: "c", Severity: SeverityMedium},
			},
			expected: SeverityMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.getHighestSeverity(tt.reasons)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseS3URL(t *testing.T) {
	tests := []struct {
		name           string
		s3URL          string
		expectedBucket string
		expectedKey    string
		expectError    bool
	}{
		{
			name:           "valid s3 URL with simple key",
			s3URL:          "s3://my-bucket/my-key.jpg",
			expectedBucket: "my-bucket",
			expectedKey:    "my-key.jpg",
			expectError:    false,
		},
		{
			name:           "valid s3 URL with nested key",
			s3URL:          "s3://my-bucket/path/to/file.mp4",
			expectedBucket: "my-bucket",
			expectedKey:    "path/to/file.mp4",
			expectError:    false,
		},
		{
			name:           "valid s3 URL with deep nesting",
			s3URL:          "s3://prod-bucket/media/2025/01/01/video.mp4",
			expectedBucket: "prod-bucket",
			expectedKey:    "media/2025/01/01/video.mp4",
			expectError:    false,
		},
		{
			name:        "missing s3:// prefix fails",
			s3URL:       "my-bucket/my-key.jpg",
			expectError: true,
		},
		{
			name:        "http URL fails",
			s3URL:       "https://s3.amazonaws.com/bucket/key",
			expectError: true,
		},
		{
			name:        "bucket only without key fails",
			s3URL:       "s3://my-bucket",
			expectError: true,
		},
		{
			name:        "empty URL fails",
			s3URL:       "",
			expectError: true,
		},
		{
			name:        "s3:// prefix only fails",
			s3URL:       "s3://",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key, err := parseS3URL(tt.s3URL)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedBucket, bucket)
				assert.Equal(t, tt.expectedKey, key)
			}
		})
	}
}

func TestCalculateFrameSamplingStrategy(t *testing.T) {
	// Create a video analyzer for testing
	va := &VideoAnalyzer{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name           string
		duration       time.Duration
		minFrames      int
		maxFrames      int
		expectedFirst  time.Duration
		expectedLast   time.Duration
		checkIntervals bool
		interval       time.Duration
	}{
		{
			name:           "short video (10s) - 5s intervals",
			duration:       10 * time.Second,
			minFrames:      2, // at least first and last
			maxFrames:      5,
			expectedFirst:  0,
			expectedLast:   10 * time.Second,
			checkIntervals: true,
			interval:       5 * time.Second,
		},
		{
			name:           "short video (30s) - 5s intervals",
			duration:       30 * time.Second,
			minFrames:      6, // 0, 5, 10, 15, 20, 25, 30
			maxFrames:      8,
			expectedFirst:  0,
			expectedLast:   30 * time.Second,
			checkIntervals: true,
			interval:       5 * time.Second,
		},
		{
			name:           "medium video (1m) - 10s intervals",
			duration:       60 * time.Second,
			minFrames:      6,
			maxFrames:      8,
			expectedFirst:  0,
			expectedLast:   60 * time.Second,
			checkIntervals: true,
			interval:       10 * time.Second,
		},
		{
			name:          "medium video (2m) - 10s intervals",
			duration:      120 * time.Second,
			minFrames:     10,
			maxFrames:     14,
			expectedFirst: 0,
			expectedLast:  120 * time.Second,
		},
		{
			name:          "long video (5m) - 15s intervals",
			duration:      5 * time.Minute,
			minFrames:     15,
			maxFrames:     25,
			expectedFirst: 0,
			expectedLast:  5 * time.Minute,
		},
		{
			name:          "very long video (10m) - max 20 frames",
			duration:      10 * time.Minute,
			minFrames:     10,
			maxFrames:     22, // max 20 + possibly first/last
			expectedFirst: 0,
			expectedLast:  10 * time.Minute,
		},
		{
			name:          "very long video (30m) - max 20 frames",
			duration:      30 * time.Minute,
			minFrames:     15,
			maxFrames:     22,
			expectedFirst: 0,
			expectedLast:  30 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intervals := va.calculateFrameSamplingStrategy(tt.duration)

			// Check frame count is within expected range
			assert.GreaterOrEqual(t, len(intervals), tt.minFrames, "too few frames")
			assert.LessOrEqual(t, len(intervals), tt.maxFrames, "too many frames")

			// Check first frame is at 0
			if len(intervals) > 0 {
				assert.Equal(t, tt.expectedFirst, intervals[0], "first frame should be at 0")
			}

			// Check last frame is at duration
			if len(intervals) > 0 {
				assert.Equal(t, tt.expectedLast, intervals[len(intervals)-1], "last frame should be at duration")
			}

			// Check intervals are in ascending order
			for i := 1; i < len(intervals); i++ {
				assert.GreaterOrEqual(t, intervals[i], intervals[i-1], "intervals should be ascending")
			}
		})
	}
}

func TestCalculateFrameSamplingStrategy_ShortVideos(t *testing.T) {
	va := &VideoAnalyzer{logger: zap.NewNop()}

	// Test very short video
	intervals := va.calculateFrameSamplingStrategy(5 * time.Second)
	assert.GreaterOrEqual(t, len(intervals), 2) // At least first and last
	assert.Equal(t, time.Duration(0), intervals[0])
	assert.Equal(t, 5*time.Second, intervals[len(intervals)-1])
}

func TestCalculateFrameSamplingStrategy_EdgeCases(t *testing.T) {
	va := &VideoAnalyzer{logger: zap.NewNop()}

	// Test zero duration
	intervals := va.calculateFrameSamplingStrategy(0)
	assert.GreaterOrEqual(t, len(intervals), 1) // Should include at least 0

	// Test 1 second video
	intervals = va.calculateFrameSamplingStrategy(1 * time.Second)
	assert.GreaterOrEqual(t, len(intervals), 1)
}
