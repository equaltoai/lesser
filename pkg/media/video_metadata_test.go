package media

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// createMockMP4Data creates a mock MP4 file with basic atoms for testing
func createMockMP4Data() []byte {
	var buf bytes.Buffer

	// Write ftyp atom (file type box)
	writeAtom(&buf, "ftyp", []byte("isom\x00\x00\x00\x01isom"))

	// Create a mock moov atom with mvhd and trak
	var moovBuf bytes.Buffer

	// Write mvhd atom (movie header)
	mvhdData := make([]byte, 32)
	mvhdData[0] = 0                                    // version 0
	binary.BigEndian.PutUint32(mvhdData[12:16], 1000)  // timescale = 1000
	binary.BigEndian.PutUint32(mvhdData[16:20], 30000) // duration = 30000 (30 seconds at 1000 timescale)
	writeAtom(&moovBuf, "mvhd", mvhdData)

	// Create a mock trak atom with tkhd
	var trakBuf bytes.Buffer

	// Write tkhd atom (track header) - version 0
	tkhdData := make([]byte, 84)
	tkhdData[0] = 0                                    // version 0
	tkhdData[3] = 0x07                                 // flags: track enabled, in movie, in preview
	binary.BigEndian.PutUint32(tkhdData[20:24], 30000) // duration
	// Width and height are fixed point 16.16 at the end
	binary.BigEndian.PutUint32(tkhdData[76:80], 1920<<16) // width = 1920
	binary.BigEndian.PutUint32(tkhdData[80:84], 1080<<16) // height = 1080
	writeAtom(&trakBuf, "tkhd", tkhdData)

	// Create mdia atom with hdlr
	var mdiaBuf bytes.Buffer
	hdlrData := make([]byte, 32)
	copy(hdlrData[8:12], []byte("vide")) // handler type = video
	writeAtom(&mdiaBuf, "hdlr", hdlrData)

	writeAtom(&trakBuf, "mdia", mdiaBuf.Bytes())
	writeAtom(&moovBuf, "trak", trakBuf.Bytes())
	writeAtom(&buf, "moov", moovBuf.Bytes())

	return buf.Bytes()
}

// writeAtom writes an MP4 atom to the buffer
func writeAtom(buf *bytes.Buffer, atomType string, data []byte) {
	size := uint32(len(data) + 8)
	binary.Write(buf, binary.BigEndian, size)
	buf.WriteString(atomType)
	buf.Write(data)
}

func TestParseVideoMetadata(t *testing.T) {
	mockData := createMockMP4Data()

	metadata, err := ParseVideoMetadata(mockData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check that we parsed the metadata correctly
	if metadata.Width != 1920 {
		t.Errorf("Expected width 1920, got %d", metadata.Width)
	}

	if metadata.Height != 1080 {
		t.Errorf("Expected height 1080, got %d", metadata.Height)
	}

	if metadata.Duration != 30000 {
		t.Errorf("Expected duration 30000ms, got %d", metadata.Duration)
	}

	if !metadata.HasVideo {
		t.Error("Expected HasVideo to be true")
	}

	if metadata.Timescale != 1000 {
		t.Errorf("Expected timescale 1000, got %d", metadata.Timescale)
	}

	t.Logf("Parsed metadata: %+v", metadata)
}

func TestParseVideoMetadataWithInvalidData(t *testing.T) {
	// Test with invalid data
	invalidData := []byte("not a video file")

	metadata, err := ParseVideoMetadata(invalidData)
	if err != nil {
		t.Logf("Expected error for invalid data: %v", err)
	}

	// Should still return metadata with fallback values
	if metadata == nil {
		t.Fatal("Expected metadata even with invalid data")
	}

	// Should have reasonable fallback values
	if metadata.Width == 0 || metadata.Height == 0 {
		t.Error("Expected non-zero fallback dimensions")
	}

	t.Logf("Fallback metadata: %+v", metadata)
}

func TestParseVideoMetadataWithSmallFile(t *testing.T) {
	// Test with too small file
	smallData := []byte{0x00, 0x00, 0x00}

	metadata, err := ParseVideoMetadata(smallData)
	if err == nil {
		t.Error("Expected error for too small file")
	}

	// Should still return fallback metadata
	if metadata == nil {
		t.Fatal("Expected fallback metadata for small file")
	}
}

func TestVideoMetadataParserIsValidMP4(t *testing.T) {
	// Test valid MP4 data
	validMP4 := createMockMP4Data()
	parser := NewVideoMetadataParser(validMP4)
	if !parser.isValidMP4() {
		t.Error("Expected valid MP4 to be detected as valid")
	}

	// Test invalid data
	invalidData := []byte("not mp4 data")
	parser = NewVideoMetadataParser(invalidData)
	if parser.isValidMP4() {
		t.Error("Expected invalid data to be detected as invalid")
	}
}

func TestCodecDetection(t *testing.T) {
	testCases := []struct {
		codec   string
		isVideo bool
		isAudio bool
	}{
		{"avc1", true, false},
		{"hev1", true, false},
		{"mp4a", false, true},
		{"ac-3", false, true},
		{"unkn", false, false},
	}

	for _, tc := range testCases {
		t.Run(tc.codec, func(t *testing.T) {
			if isVideoCodec(tc.codec) != tc.isVideo {
				t.Errorf("Expected isVideoCodec(%s) = %t, got %t", tc.codec, tc.isVideo, !tc.isVideo)
			}
			if isAudioCodec(tc.codec) != tc.isAudio {
				t.Errorf("Expected isAudioCodec(%s) = %t, got %t", tc.codec, tc.isAudio, !tc.isAudio)
			}
		})
	}
}

// Benchmark the parser performance
func BenchmarkParseVideoMetadata(b *testing.B) {
	mockData := createMockMP4Data()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseVideoMetadata(mockData)
		if err != nil {
			b.Fatalf("Parsing failed: %v", err)
		}
	}
}
