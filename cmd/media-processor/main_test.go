package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/equaltoai/lesser/pkg/media"
	"go.uber.org/zap/zaptest"
)

// createMockMP4Data creates a mock MP4 file with specific duration for testing
func createMockMP4Data(durationMs int, width, height int) []byte {
	var buf bytes.Buffer

	// Write ftyp atom (file type box) with proper MP4 signature
	// This should help Go's http.DetectContentType recognize it as video/mp4
	ftypData := append([]byte("isom"), 0x00, 0x00, 0x00, 0x01)
	ftypData = append(ftypData, []byte("isom")...)
	ftypData = append(ftypData, []byte("mp41")...) // Add MP4 v1 brand
	writeAtom(&buf, "ftyp", ftypData)

	// Create a mock moov atom with mvhd and trak
	var moovBuf bytes.Buffer

	// Write mvhd atom (movie header)
	mvhdData := make([]byte, 32)
	mvhdData[0] = 0                                                 // version 0
	binary.BigEndian.PutUint32(mvhdData[12:16], 1000)               // timescale = 1000
	binary.BigEndian.PutUint32(mvhdData[16:20], uint32(durationMs)) // duration in timescale units
	writeAtom(&moovBuf, "mvhd", mvhdData)

	// Create a mock trak atom with tkhd
	var trakBuf bytes.Buffer

	// Write tkhd atom (track header) - version 0
	tkhdData := make([]byte, 84)
	tkhdData[0] = 0                                                 // version 0
	tkhdData[3] = 0x07                                              // flags: track enabled, in movie, in preview
	binary.BigEndian.PutUint32(tkhdData[20:24], uint32(durationMs)) // duration
	// Width and height are fixed point 16.16 at the end
	binary.BigEndian.PutUint32(tkhdData[76:80], uint32(width)<<16)  // width
	binary.BigEndian.PutUint32(tkhdData[80:84], uint32(height)<<16) // height
	writeAtom(&trakBuf, "tkhd", tkhdData)

	// Create mdia atom with hdlr
	var mdiaBuf bytes.Buffer
	hdlrData := make([]byte, 32)
	copy(hdlrData[8:12], []byte("vide")) // handler type = video
	writeAtom(&mdiaBuf, "hdlr", hdlrData)

	writeAtom(&trakBuf, "mdia", mdiaBuf.Bytes())
	writeAtom(&moovBuf, "trak", trakBuf.Bytes())
	writeAtom(&buf, "moov", moovBuf.Bytes())

	// Add some padding to make it look more like a real MP4 file
	buf.Write(make([]byte, 1024)) // Add 1KB padding

	return buf.Bytes()
}

// writeAtom writes an MP4 atom to the buffer
func writeAtom(buf *bytes.Buffer, atomType string, data []byte) {
	size := uint32(len(data) + 8)
	binary.Write(buf, binary.BigEndian, size)
	buf.WriteString(atomType)
	buf.Write(data)
}

func TestGetVideoMetadata(t *testing.T) {
	logger := zaptest.NewLogger(t)

	mp := &MediaProcessor{
		logger: logger,
	}

	tests := []struct {
		name              string
		durationMs        int
		width             int
		height            int
		expectedDurationS int
		mimeType          string
	}{
		{
			name:              "30 second 1080p video",
			durationMs:        30000,
			width:             1920,
			height:            1080,
			expectedDurationS: 30,
			mimeType:          "video/mp4",
		},
		{
			name:              "2 minute 720p video",
			durationMs:        120000,
			width:             1280,
			height:            720,
			expectedDurationS: 120,
			mimeType:          "video/mp4",
		},
		{
			name:              "10 second 480p video",
			durationMs:        10000,
			width:             854,
			height:            480,
			expectedDurationS: 10,
			mimeType:          "video/mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockData := createMockMP4Data(tt.durationMs, tt.width, tt.height)

			duration, err := mp.getVideoMetadata(mockData, tt.mimeType)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if duration != tt.expectedDurationS {
				t.Errorf("Expected duration %d seconds, got %d", tt.expectedDurationS, duration)
			}
		})
	}
}

func TestGetVideoMetadataWithInvalidData(t *testing.T) {
	logger := zaptest.NewLogger(t)

	mp := &MediaProcessor{
		logger: logger,
	}

	tests := []struct {
		name     string
		data     []byte
		mimeType string
	}{
		{
			name:     "invalid video data",
			data:     []byte("not a video file"),
			mimeType: "video/mp4",
		},
		{
			name:     "empty data",
			data:     []byte{},
			mimeType: "video/mp4",
		},
		{
			name:     "too small data",
			data:     []byte{0x00, 0x00, 0x00},
			mimeType: "video/mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mp.getVideoMetadata(tt.data, tt.mimeType)
			if err == nil {
				t.Error("Expected error for invalid data, got none")
			}
		})
	}
}

func TestExtractVideoMetadata(t *testing.T) {
	logger := zaptest.NewLogger(t)

	mp := &MediaProcessor{
		logger: logger,
	}

	tests := []struct {
		name           string
		durationMs     int
		width          int
		height         int
		expectedWidth  int
		expectedHeight int
		expectedDurMs  int
	}{
		{
			name:           "1080p 45 second video",
			durationMs:     45000,
			width:          1920,
			height:         1080,
			expectedWidth:  1920,
			expectedHeight: 1080,
			expectedDurMs:  45000,
		},
		{
			name:           "720p 2 minute video",
			durationMs:     120000,
			width:          1280,
			height:         720,
			expectedWidth:  1280,
			expectedHeight: 720,
			expectedDurMs:  120000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockData := createMockMP4Data(tt.durationMs, tt.width, tt.height)

			width, height, duration := mp.extractVideoMetadata(mockData)

			if width != tt.expectedWidth {
				t.Errorf("Expected width %d, got %d", tt.expectedWidth, width)
			}

			if height != tt.expectedHeight {
				t.Errorf("Expected height %d, got %d", tt.expectedHeight, height)
			}

			if duration != tt.expectedDurMs {
				t.Errorf("Expected duration %d ms, got %d", tt.expectedDurMs, duration)
			}
		})
	}
}

func TestValidateFileForUserWithVideoDuration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	mp := &MediaProcessor{
		logger: logger,
	}

	tests := []struct {
		name           string
		videoDurationS int
		maxDurationS   int
		expectError    bool
		errorContains  string
	}{
		{
			name:           "video within duration limit",
			videoDurationS: 30,
			maxDurationS:   60,
			expectError:    false,
		},
		{
			name:           "video exceeds duration limit",
			videoDurationS: 90,
			maxDurationS:   60,
			expectError:    true,
			errorContains:  "exceeds user limit",
		},
		{
			name:           "video at exact duration limit",
			videoDurationS: 60,
			maxDurationS:   60,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockData := createMockMP4Data(tt.videoDurationS*1000, 1280, 720)

			config := &MediaConfig{
				MaxVideoDuration: tt.maxDurationS,
			}

			err := mp.validateFileForUser(mockData, "video/mp4", config, "testuser", "test-media-id")

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorContains != "" && !bytes.Contains([]byte(err.Error()), []byte(tt.errorContains)) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestVideoMetadataIntegration(t *testing.T) {
	// Test that the media.ParseVideoMetadata integration works correctly
	mockData := createMockMP4Data(90000, 1920, 1080) // 90 seconds, 1080p

	metadata, err := media.ParseVideoMetadata(mockData)
	if err != nil {
		t.Fatalf("ParseVideoMetadata failed: %v", err)
	}

	if metadata.Width != 1920 {
		t.Errorf("Expected width 1920, got %d", metadata.Width)
	}

	if metadata.Height != 1080 {
		t.Errorf("Expected height 1080, got %d", metadata.Height)
	}

	if metadata.Duration != 90000 {
		t.Errorf("Expected duration 90000ms, got %d", metadata.Duration)
	}

	if metadata.DurationSeconds != 90.0 {
		t.Errorf("Expected duration 90.0 seconds, got %f", metadata.DurationSeconds)
	}

	if !metadata.HasVideo {
		t.Error("Expected HasVideo to be true")
	}
}

func TestVideoMetadataWithDifferentFormats(t *testing.T) {
	// Test different video formats and sizes
	tests := []struct {
		name       string
		durationMs int
		width      int
		height     int
	}{
		{"4K video", 60000, 3840, 2160},
		{"1080p video", 45000, 1920, 1080},
		{"720p video", 30000, 1280, 720},
		{"480p video", 15000, 854, 480},
		{"Short video", 5000, 1280, 720},
		{"Long video", 300000, 1920, 1080}, // 5 minutes
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockData := createMockMP4Data(tt.durationMs, tt.width, tt.height)

			metadata, err := media.ParseVideoMetadata(mockData)
			if err != nil {
				t.Fatalf("ParseVideoMetadata failed for %s: %v", tt.name, err)
			}

			if metadata.Width != tt.width {
				t.Errorf("Expected width %d, got %d", tt.width, metadata.Width)
			}

			if metadata.Height != tt.height {
				t.Errorf("Expected height %d, got %d", tt.height, metadata.Height)
			}

			if metadata.Duration != tt.durationMs {
				t.Errorf("Expected duration %dms, got %d", tt.durationMs, metadata.Duration)
			}

			// Verify duration calculation is consistent
			expectedSeconds := float64(tt.durationMs) / 1000.0
			if abs(metadata.DurationSeconds-expectedSeconds) > 0.1 {
				t.Errorf("Expected duration %f seconds, got %f", expectedSeconds, metadata.DurationSeconds)
			}
		})
	}
}

// Helper function for float comparison
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
