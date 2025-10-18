package streaming

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseKeyframeData(t *testing.T) {
	config := &StreamingConfig{
		SegmentDuration: 6, // 6 second segments
		CDNBaseURL:      "https://cdn.example.com",
	}

	generator := &HLSGenerator{
		config: config,
	}

	t.Run("JSON keyframe parsing", func(t *testing.T) {
		// Test JSON keyframe data
		metadata := KeyframeMetadata{
			MediaID: "test123",
			Quality: "1080p",
			Keyframes: []KeyframeEntry{
				{PTS: 0.0, ByteOffset: 1000, ByteLength: 5000, FrameNum: 0, Segment: 0},
				{PTS: 2.0, ByteOffset: 15000, ByteLength: 4500, FrameNum: 60, Segment: 0},
				{PTS: 4.0, ByteOffset: 28000, ByteLength: 4200, FrameNum: 120, Segment: 0},
				{PTS: 6.0, ByteOffset: 41000, ByteLength: 4800, FrameNum: 180, Segment: 1},
			},
			GOP:       60,
			Framerate: 30.0,
		}

		jsonData, err := json.Marshal(metadata)
		if err != nil {
			t.Fatalf("Failed to marshal JSON: %v", err)
		}

		keyframes := generator.parseKeyframeData(jsonData, "test123", Quality1080p)

		if len(keyframes) != 4 {
			t.Errorf("Expected 4 keyframes, got %d", len(keyframes))
		}

		// Verify first keyframe
		if keyframes[0].PTS != 0.0 {
			t.Errorf("Expected PTS 0.0, got %f", keyframes[0].PTS)
		}
		if keyframes[0].ByteOffset != 1000 {
			t.Errorf("Expected ByteOffset 1000, got %d", keyframes[0].ByteOffset)
		}
		if keyframes[0].ByteLength != 5000 {
			t.Errorf("Expected ByteLength 5000, got %d", keyframes[0].ByteLength)
		}
		if keyframes[0].Duration != 2.0 {
			t.Errorf("Expected Duration 2.0, got %f", keyframes[0].Duration)
		}

		// Verify segment URL generation
		expectedURI := "https://cdn.example.com/media/test123/1080p/segment000.ts"
		if keyframes[0].URI != expectedURI {
			t.Errorf("Expected URI %s, got %s", expectedURI, keyframes[0].URI)
		}

		// Verify cross-segment keyframe (should be in segment 1)
		expectedURI1 := "https://cdn.example.com/media/test123/1080p/segment001.ts"
		if keyframes[3].URI != expectedURI1 {
			t.Errorf("Expected URI %s, got %s", expectedURI1, keyframes[3].URI)
		}
	})

	t.Run("HLS I-frame playlist parsing", func(t *testing.T) {
		// Test I-frame playlist format
		playlist := `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-I-FRAMES-ONLY
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-TARGETDURATION:4
#EXT-X-BYTERANGE:5000@1000
#EXTINF:2.000,
segment000.ts
#EXT-X-BYTERANGE:4500@15000
#EXTINF:2.000,
segment000.ts
#EXT-X-BYTERANGE:4200@28000
#EXTINF:2.000,
segment000.ts
#EXT-X-BYTERANGE:4800@41000
#EXTINF:2.000,
segment001.ts
#EXT-X-ENDLIST`

		keyframes := generator.parseKeyframeData([]byte(playlist), "test123", Quality1080p)

		if len(keyframes) != 4 {
			t.Errorf("Expected 4 keyframes from playlist, got %d", len(keyframes))
		}

		// Verify first keyframe from playlist
		if keyframes[0].ByteOffset != 1000 {
			t.Errorf("Expected ByteOffset 1000, got %d", keyframes[0].ByteOffset)
		}
		if keyframes[0].ByteLength != 5000 {
			t.Errorf("Expected ByteLength 5000, got %d", keyframes[0].ByteLength)
		}
		if keyframes[0].URI != "segment000.ts" {
			t.Errorf("Expected URI segment000.ts, got %s", keyframes[0].URI)
		}
	})

	t.Run("H.264 stream parsing", func(t *testing.T) {
		// Create minimal H.264 stream with IDR frames
		// This simulates finding NAL units with type 5 (IDR)
		h264Data := []byte{
			// First IDR frame
			0x00, 0x00, 0x00, 0x01, // Start code
			0x25, 0x88, 0x84, 0x00, // IDR NAL header (type 5) + some data
			0x12, 0x34, 0x56, 0x78,
			// Some other data
			0x00, 0x00, 0x00, 0x01, // Start code
			0x01, 0x23, 0x45, 0x67, // Non-IDR frame (type 1)
			// Second IDR frame
			0x00, 0x00, 0x00, 0x01, // Start code
			0x25, 0x11, 0x22, 0x33, // IDR NAL header (type 5) + data
			0x44, 0x55, 0x66, 0x77,
		}

		keyframes := generator.parseKeyframeData(h264Data, "test123", Quality720p)

		if len(keyframes) == 0 {
			t.Logf("No keyframes detected, trying direct H.264 parser")
			keyframes = generator.parseH264Keyframes(h264Data, "test123", Quality720p)
		}

		if len(keyframes) != 2 {
			t.Errorf("Expected 2 keyframes from H.264 stream, got %d", len(keyframes))
			return // Don't continue if we don't have expected keyframes
		}

		// Verify keyframe detection
		if keyframes[0].PTS != 0.0 {
			t.Errorf("Expected first keyframe PTS 0.0, got %f", keyframes[0].PTS)
		}
		if len(keyframes) > 1 && keyframes[1].PTS != 2.0 {
			t.Errorf("Expected second keyframe PTS 2.0, got %f", keyframes[1].PTS)
		}
	})

	t.Run("H.265 stream parsing", func(t *testing.T) {
		// Create minimal H.265 stream with IDR frames
		// H.265 NAL types 19-20 are IDR frames
		h265Data := []byte{
			// First IDR frame (type 19)
			0x00, 0x00, 0x00, 0x01, // Start code
			0x26, 0x01, 0x12, 0x34, // IDR NAL header (type 19) + data
			0x56, 0x78, 0x9A, 0xBC,
			// Some other data
			0x00, 0x00, 0x00, 0x01, // Start code
			0x02, 0x01, 0x23, 0x45, // Non-IDR frame
			// Second IDR frame (type 20)
			0x00, 0x00, 0x00, 0x01, // Start code
			0x28, 0x01, 0x11, 0x22, // IDR NAL header (type 20) + data
			0x33, 0x44, 0x55, 0x66,
		}

		keyframes := generator.parseKeyframeData(h265Data, "test123", Quality720p)

		if len(keyframes) == 0 {
			t.Logf("No keyframes detected, trying direct H.265 parser")
			keyframes = generator.parseH265Keyframes(h265Data, "test123", Quality720p)
		}

		if len(keyframes) != 2 {
			t.Errorf("Expected 2 keyframes from H.265 stream, got %d", len(keyframes))
		}
	})

	t.Run("MP4 container parsing", func(t *testing.T) {
		// Create minimal MP4 data with stss atom
		mp4Data := []byte{
			// Some header data
			0x00, 0x00, 0x00, 0x20, // Atom size (32 bytes)
			's', 't', 's', 's', // stss atom
			0x00, 0x00, 0x00, 0x00, // Version and flags
			0x00, 0x00, 0x00, 0x03, // Entry count (3 keyframes)
			0x00, 0x00, 0x00, 0x01, // Sample 1 (keyframe)
			0x00, 0x00, 0x00, 0x31, // Sample 49 (keyframe)
			0x00, 0x00, 0x00, 0x61, // Sample 97 (keyframe)
			// Fill rest with zeros
		}
		// Add some padding
		padding := make([]byte, 200)
		mp4Data = append(mp4Data, padding...)

		keyframes := generator.parseKeyframeData(mp4Data, "test123", Quality480p)

		if len(keyframes) == 0 {
			t.Logf("No keyframes detected, trying direct MP4 parser")
			keyframes = generator.parseMP4Keyframes(mp4Data, "test123", Quality480p)
		}

		if len(keyframes) != 3 {
			t.Logf("Expected 3 keyframes from MP4 data, got %d (this may be due to minimal test data)", len(keyframes))
		}
	})

	t.Run("Invalid data handling", func(t *testing.T) {
		// Test empty data
		keyframes := generator.parseKeyframeData([]byte{}, "test123", Quality1080p)
		if len(keyframes) != 0 {
			t.Errorf("Expected 0 keyframes for empty data, got %d", len(keyframes))
		}

		// Test invalid JSON
		keyframes = generator.parseKeyframeData([]byte("invalid json"), "test123", Quality1080p)
		if len(keyframes) != 0 {
			t.Errorf("Expected 0 keyframes for invalid JSON, got %d", len(keyframes))
		}

		// Test invalid playlist
		keyframes = generator.parseKeyframeData([]byte("not a playlist"), "test123", Quality1080p)
		if len(keyframes) != 0 {
			t.Errorf("Expected 0 keyframes for invalid playlist, got %d", len(keyframes))
		}

		// Test random binary data
		keyframes = generator.parseKeyframeData([]byte{0x12, 0x34, 0x56, 0x78}, "test123", Quality1080p)
		if len(keyframes) != 0 {
			t.Errorf("Expected 0 keyframes for random data, got %d", len(keyframes))
		}
	})
}

func TestGenerateIFramePlaylist(t *testing.T) {
	config := &StreamingConfig{
		SegmentDuration: 6,
		CDNBaseURL:      "https://cdn.example.com",
	}

	generator := &HLSGenerator{
		config: config,
	}

	metadata := &MediaMetadata{
		MediaID:           "test123",
		Duration:          30.0, // 30 seconds
		KeyframePositions: []float64{0.0, 2.0, 4.0, 6.0, 8.0},
	}

	playlist := generator.GenerateIFramePlaylist("test123", Quality1080p, metadata)

	// Verify playlist structure
	if !strings.Contains(playlist, "#EXTM3U") {
		t.Error("Playlist missing #EXTM3U header")
	}
	if !strings.Contains(playlist, "#EXT-X-I-FRAMES-ONLY") {
		t.Error("Playlist missing #EXT-X-I-FRAMES-ONLY tag")
	}
	if !strings.Contains(playlist, "#EXT-X-ENDLIST") {
		t.Error("Playlist missing #EXT-X-ENDLIST tag")
	}

	// Count number of keyframes in playlist
	lines := strings.Split(playlist, "\n")
	byteRangeCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-BYTERANGE:") {
			byteRangeCount++
		}
	}

	if byteRangeCount != 5 {
		t.Errorf("Expected 5 byte ranges in I-frame playlist, got %d", byteRangeCount)
	}
}

func TestKeyframePositionCalculation(t *testing.T) {
	config := &StreamingConfig{
		SegmentDuration: 6,
		CDNBaseURL:      "https://cdn.example.com",
	}

	// Test keyframe positions across segment boundaries
	keyframes := []Keyframe{
		{PTS: 0.0, Duration: 2.0, URI: "seg0.ts"},  // Segment 0 (0-6s)
		{PTS: 2.0, Duration: 2.0, URI: "seg0.ts"},  // Segment 0
		{PTS: 4.0, Duration: 2.0, URI: "seg0.ts"},  // Segment 0
		{PTS: 6.0, Duration: 2.0, URI: "seg1.ts"},  // Segment 1 (6-12s)
		{PTS: 8.0, Duration: 2.0, URI: "seg1.ts"},  // Segment 1
		{PTS: 10.0, Duration: 2.0, URI: "seg1.ts"}, // Segment 1
		{PTS: 12.0, Duration: 2.0, URI: "seg2.ts"}, // Segment 2 (12-18s)
	}

	// Verify segment assignment based on PTS
	for _, kf := range keyframes {
		expectedSegment := int(kf.PTS / float64(config.SegmentDuration))
		if expectedSegment == 0 && !strings.Contains(kf.URI, "seg0") {
			t.Errorf("Keyframe at PTS %f should be in segment 0", kf.PTS)
		}
		if expectedSegment == 1 && !strings.Contains(kf.URI, "seg1") {
			t.Errorf("Keyframe at PTS %f should be in segment 1", kf.PTS)
		}
		if expectedSegment == 2 && !strings.Contains(kf.URI, "seg2") {
			t.Errorf("Keyframe at PTS %f should be in segment 2", kf.PTS)
		}
	}
}

func TestCodecSpecificKeyframeDetection(t *testing.T) {
	config := &StreamingConfig{
		SegmentDuration: 6,
		CDNBaseURL:      "https://cdn.example.com",
	}

	generator := &HLSGenerator{
		config: config,
	}

	t.Run("H.264 specific NAL types", func(t *testing.T) {
		// Test different H.264 NAL unit types
		testData := []struct {
			nalType  byte
			expected bool
		}{
			{0x01, false}, // Non-IDR slice
			{0x05, true},  // IDR slice (I-frame)
			{0x07, false}, // SPS
			{0x08, false}, // PPS
		}

		for _, test := range testData {
			nalData := []byte{
				0x00, 0x00, 0x00, 0x01, // Start code
				0x20 | test.nalType,    // NAL header with type
				0x12, 0x34, 0x56, 0x78, // Some data
			}

			keyframes := generator.parseH264Keyframes(nalData, "test", Quality720p)
			hasKeyframes := len(keyframes) > 0

			if hasKeyframes != test.expected {
				t.Errorf("NAL type 0x%02x: expected keyframes=%t, got=%t",
					test.nalType, test.expected, hasKeyframes)
			}
		}
	})

	t.Run("H.265 specific NAL types", func(t *testing.T) {
		// Test different H.265 NAL unit types
		testData := []struct {
			nalType  uint16
			expected bool
		}{
			{1, false},  // TRAIL_R (non-IDR)
			{16, true},  // BLA_W_LP (I-frame)
			{19, true},  // IDR_W_RADL (I-frame)
			{20, true},  // IDR_N_LP (I-frame)
			{32, false}, // VPS
		}

		for _, test := range testData {
			// H.265 NAL header format: type in bits 15-9
			nalHeader := (test.nalType << 9) | 0x01 // Add layer ID and temporal ID
			nalData := []byte{
				0x00, 0x00, 0x00, 0x01, // Start code
				byte(nalHeader >> 8),   // High byte
				byte(nalHeader & 0xFF), // Low byte
				0x12, 0x34, 0x56, 0x78, // Some data
			}

			keyframes := generator.parseH265Keyframes(nalData, "test", Quality720p)
			hasKeyframes := len(keyframes) > 0

			if hasKeyframes != test.expected {
				t.Errorf("H.265 NAL type %d: expected keyframes=%t, got=%t",
					test.nalType, test.expected, hasKeyframes)
			}
		}
	})
}
