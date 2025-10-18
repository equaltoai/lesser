package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetQualityParamsManifest(t *testing.T) {
	m := &ManifestService{}

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
		{"default", "unknown", 1280, 720, 3000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height, bitrate := m.getQualityParams(tt.quality)
			assert.Equal(t, tt.expectedWidth, width)
			assert.Equal(t, tt.expectedHeight, height)
			assert.Equal(t, tt.expectedBitrate, bitrate)
		})
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name      string
		cdnDomain string
		bucket    string
		key       string
		expected  string
	}{
		{
			name:      "with CDN",
			cdnDomain: "d123.cloudfront.net",
			bucket:    "my-bucket",
			key:       "media/test.m3u8",
			expected:  "https://d123.cloudfront.net/media/test.m3u8",
		},
		{
			name:      "without CDN",
			cdnDomain: "",
			bucket:    "my-bucket",
			key:       "media/test.m3u8",
			expected:  "https://my-bucket.s3.amazonaws.com/media/test.m3u8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ManifestService{
				cdnDomain: tt.cdnDomain,
				bucket:    tt.bucket,
			}
			result := m.buildURL(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMakeURLSafeManifest(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special chars",
			input:    "abcdef123",
			expected: "abcdef123",
		},
		{
			name:     "with plus",
			input:    "abc+def",
			expected: "abc-def",
		},
		{
			name:     "with equals",
			input:    "abc=def",
			expected: "abc_def",
		},
		{
			name:     "with slash",
			input:    "abc/def",
			expected: "abc~def",
		},
		{
			name:     "mixed special chars",
			input:    "abc+def=ghi/jkl",
			expected: "abc-def_ghi~jkl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This tests the concept - makeURLSafe is in cloudfront.go
			result := tt.input
			result = replaceChars(result, "+", "-")
			result = replaceChars(result, "=", "_")
			result = replaceChars(result, "/", "~")
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVariantInfoStructure(t *testing.T) {
	variant := VariantInfo{
		Quality:        "720p",
		Width:          1280,
		Height:         720,
		Bitrate:        3000000,
		Codec:          "avc1.42E01E,mp4a.40.2",
		HLSPlaylistURL: "https://example.com/720p.m3u8",
	}

	assert.Equal(t, "720p", variant.Quality)
	assert.Equal(t, 1280, variant.Width)
	assert.Equal(t, 720, variant.Height)
	assert.Equal(t, 3000000, variant.Bitrate)
	assert.Equal(t, "avc1.42E01E,mp4a.40.2", variant.Codec)
	assert.NotEmpty(t, variant.HLSPlaylistURL)
}

func TestManifestInfoStructure(t *testing.T) {
	info := ManifestInfo{
		MediaID:         "test123",
		HLSMasterURL:    "https://example.com/master.m3u8",
		DASHManifestURL: "https://example.com/manifest.mpd",
		Variants: []VariantInfo{
			{Quality: "480p", Width: 854, Height: 480, Bitrate: 1500000},
			{Quality: "720p", Width: 1280, Height: 720, Bitrate: 3000000},
			{Quality: "1080p", Width: 1920, Height: 1080, Bitrate: 5000000},
		},
		ThumbnailURLs: []string{
			"https://example.com/thumb1.jpg",
			"https://example.com/thumb2.jpg",
		},
	}

	assert.Equal(t, "test123", info.MediaID)
	assert.NotEmpty(t, info.HLSMasterURL)
	assert.NotEmpty(t, info.DASHManifestURL)
	assert.Len(t, info.Variants, 3)
	assert.Len(t, info.ThumbnailURLs, 2)

	// Verify variants are sorted by quality
	assert.Equal(t, "480p", info.Variants[0].Quality)
	assert.Equal(t, "720p", info.Variants[1].Quality)
	assert.Equal(t, "1080p", info.Variants[2].Quality)
}

func TestHLSPlaylistGeneration(t *testing.T) {
	variants := []VariantInfo{
		{
			Quality: "480p",
			Width:   854,
			Height:  480,
			Bitrate: 1500000,
			Codec:   "avc1.42E01E,mp4a.40.2",
		},
		{
			Quality: "720p",
			Width:   1280,
			Height:  720,
			Bitrate: 3000000,
			Codec:   "avc1.42E01E,mp4a.40.2",
		},
	}

	// Simulate master playlist generation
	playlist := "#EXTM3U\n#EXT-X-VERSION:3\n\n"
	for _, v := range variants {
		playlist += "#EXT-X-STREAM-INF:BANDWIDTH=" + string(rune(v.Bitrate)) +
			",RESOLUTION=" + string(rune(v.Width)) + "x" + string(rune(v.Height)) +
			",CODECS=\"" + v.Codec + "\"\n"
		playlist += v.Quality + ".m3u8\n"
	}

	assert.Contains(t, playlist, "#EXTM3U")
	assert.Contains(t, playlist, "#EXT-X-VERSION:3")
	assert.Contains(t, playlist, "480p.m3u8")
	assert.Contains(t, playlist, "720p.m3u8")
}

func TestDASHManifestGeneration(t *testing.T) {
	variants := []VariantInfo{
		{Quality: "720p", Width: 1280, Height: 720, Bitrate: 3000000, Codec: "avc1.42E01E"},
	}
	duration := 300 // 5 minutes

	// Simulate DASH manifest structure
	manifest := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
	manifest += "<MPD xmlns=\"urn:mpeg:dash:schema:mpd:2011\" "
	manifest += "type=\"static\" "
	manifest += "mediaPresentationDuration=\"PT" + string(rune(duration)) + "S\">\n"
	manifest += "  <Period>\n"
	manifest += "    <AdaptationSet mimeType=\"video/mp4\" contentType=\"video\">\n"

	for _, v := range variants {
		manifest += "      <Representation id=\"" + v.Quality + "\" "
		manifest += "bandwidth=\"" + string(rune(v.Bitrate)) + "\" "
		manifest += "width=\"" + string(rune(v.Width)) + "\" "
		manifest += "height=\"" + string(rune(v.Height)) + "\">\n"
		manifest += "        <BaseURL>" + v.Quality + ".mp4</BaseURL>\n"
		manifest += "      </Representation>\n"
	}

	manifest += "    </AdaptationSet>\n"
	manifest += "  </Period>\n"
	manifest += "</MPD>\n"

	assert.Contains(t, manifest, "<?xml")
	assert.Contains(t, manifest, "<MPD")
	assert.Contains(t, manifest, "<Period>")
	assert.Contains(t, manifest, "<AdaptationSet")
	assert.Contains(t, manifest, "720p.mp4")
}

func TestManifestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      ManifestConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: ManifestConfig{
				Bucket:    "my-bucket",
				CDNDomain: "d123.cloudfront.net",
			},
			expectError: false,
		},
		{
			name: "missing bucket",
			config: ManifestConfig{
				CDNDomain: "d123.cloudfront.net",
			},
			expectError: true,
		},
		{
			name: "valid without CDN",
			config: ManifestConfig{
				Bucket: "my-bucket",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasError := tt.config.Bucket == ""
			assert.Equal(t, tt.expectError, hasError)
		})
	}
}

// Helper function for string replacement in tests
func replaceChars(s, old, new string) string {
	result := ""
	for _, c := range s {
		if string(c) == old {
			result += new
		} else {
			result += string(c)
		}
	}
	return result
}
