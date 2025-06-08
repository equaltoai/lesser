package main

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessVideo(t *testing.T) {
	// Skip if ffmpeg not available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available, skipping video processing tests")
	}

	ctx := context.Background()

	// Create a small test video
	testVideoData := generateTestVideo(t)

	event := MediaProcessingEvent{
		JobID:    "test-job-123",
		MediaID:  "test-media-456",
		Username: "testuser",
	}

	result, err := processVideo(ctx, testVideoData, event, nil)
	require.NoError(t, err)

	// Verify metadata was extracted
	assert.Greater(t, result.Width, 0)
	assert.Greater(t, result.Height, 0)
	assert.Greater(t, result.Duration, 0)

	// Verify sizes were set
	assert.NotEmpty(t, result.Sizes)
	assert.Contains(t, result.Sizes, "original")
}

func TestProcessAudio(t *testing.T) {
	// Skip if ffmpeg not available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available, skipping audio processing tests")
	}

	ctx := context.Background()

	// Create a small test audio file
	testAudioData := generateTestAudio(t)

	event := MediaProcessingEvent{
		JobID:    "test-job-789",
		MediaID:  "test-media-012",
		Username: "testuser",
	}

	result, err := processAudio(ctx, testAudioData, event, nil)
	require.NoError(t, err)

	// Verify duration was extracted
	assert.Greater(t, result.Duration, 0)

	// Verify sizes were set
	assert.NotEmpty(t, result.Sizes)
	assert.Contains(t, result.Sizes, "original")
}

func generateTestVideo(t *testing.T) []byte {
	// Generate a 1-second test video using ffmpeg
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "testsrc=duration=1:size=320x240:rate=30",
		"-pix_fmt", "yuv420p",
		"-f", "mp4",
		"pipe:1")

	data, err := cmd.Output()
	require.NoError(t, err, "failed to generate test video")
	return data
}

func generateTestAudio(t *testing.T) []byte {
	// Generate a 1-second test audio file using ffmpeg
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "sine=frequency=1000:duration=1",
		"-f", "mp3",
		"pipe:1")

	data, err := cmd.Output()
	require.NoError(t, err, "failed to generate test audio")
	return data
}

// Note: In a real test environment, you would need to:
// 1. Mock AWS services (S3, DynamoDB)
// 2. Set up test environment variables
// 3. Use dependency injection for better testability
