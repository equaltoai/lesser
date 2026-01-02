package streaming

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHLSGenerator_PlaylistsAndValidation(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:      "https://cdn.example.com",
		SegmentDuration: 6,
	}

	metadataFromStorage := &MediaMetadata{
		MediaID:     "m1",
		Status:      StatusComplete,
		VideoCodec:  "avc1.640028",
		AudioCodec:  "mp4a.40.2",
		VideoProfile:"High",
		QualitySettings: map[Quality]QualityCodecInfo{
			Quality720p: {
				VideoCodec: "avc1.64001f",
				AudioCodec: "mp4a.40.2",
			},
		},
	}

	storage := &countingStorage{
		metadata: metadataFromStorage,
	}
	gen := NewHLSGenerator(cfg, storage)

	input := &MediaMetadata{
		MediaID:            "m1",
		Duration:           15.0,
		Bitrate:            4000,
		AvailableQualities: []Quality{Quality720p},
		QualitySettings: map[Quality]QualityCodecInfo{
			Quality720p: {Bandwidth: 4500000, Width: 1280, Height: 720},
		},
	}

	manifest, err := gen.GenerateMasterPlaylist("m1", input)
	require.NoError(t, err)
	require.Len(t, manifest.Variants, 1)
	assert.Equal(t, 4500000, manifest.Variants[0].Bandwidth)
	assert.Equal(t, "1280x720", manifest.Variants[0].Resolution)
	assert.Equal(t, "avc1.64001f,mp4a.40.2", manifest.Variants[0].Codecs)

	masterContent := gen.GenerateMasterPlaylistContent(manifest)
	assert.Contains(t, masterContent, "#EXTM3U")
	assert.Contains(t, masterContent, "#EXT-X-VERSION:6")
	assert.Contains(t, masterContent, "playlist.m3u8")

	variant := gen.GenerateVariantPlaylist("m1", Quality720p, input)
	assert.Contains(t, variant, "#EXT-X-PLAYLIST-TYPE:VOD")
	assert.Contains(t, variant, "#EXT-X-ENDLIST")
	// 15s @ 6s segments -> 3 segments, last is 3 seconds
	assert.Contains(t, variant, "#EXTINF:3.000,")

	live := gen.GenerateLivePlaylist("m1", Quality720p, 5, 3)
	assert.Contains(t, live, "#EXT-X-PLAYLIST-TYPE:EVENT")
	assert.Contains(t, live, "#EXT-X-MEDIA-SEQUENCE:5")
	assert.NotContains(t, live, "#EXT-X-ENDLIST")
	assert.Equal(t, 3, strings.Count(live, "segment"))

	vtt := gen.GenerateWebVTTPlaylist("m1", "en", 2)
	assert.Contains(t, vtt, "subtitles/en/segment000.vtt")
	assert.Contains(t, vtt, "subtitles/en/segment001.vtt")

	subtitles := []SubtitleTrack{
		{Name: "English", Language: "en", URI: "subs/en.m3u8", Default: true},
		{Name: "Spanish", Language: "es", URI: "subs/es.m3u8", AutoSelect: true},
	}
	masterWithSubs := gen.GenerateMasterPlaylistWithSubtitles(manifest, subtitles)
	assert.Contains(t, masterWithSubs, "TYPE=SUBTITLES")
	assert.Contains(t, masterWithSubs, "DEFAULT=YES")
	assert.Contains(t, masterWithSubs, "AUTOSELECT=YES")
	assert.Contains(t, masterWithSubs, "SUBTITLES=\"subs\"")

	// Validation
	validVariant := strings.Join([]string{
		"#EXTM3U",
		"#EXT-X-VERSION:6",
		"#EXT-X-TARGETDURATION:6",
		"#EXTINF:6.0,",
		"https://cdn.example.com/media/m1/720p/segment000.ts",
		"#EXT-X-ENDLIST",
		"",
	}, "\n")
	require.NoError(t, gen.ValidatePlaylist(validVariant))

	assert.ErrorContains(t, gen.ValidatePlaylist(""), "playlist too short")
	assert.ErrorContains(t, gen.ValidatePlaylist("M3U\n#EXT-X-VERSION:6\n"), "missing #EXTM3U header")

	missingVersion := strings.Join([]string{"#EXTM3U", "#EXT-X-TARGETDURATION:6", ""}, "\n")
	assert.ErrorContains(t, gen.ValidatePlaylist(missingVersion), "missing #EXT-X-VERSION")

	missingTargetDuration := strings.Join([]string{"#EXTM3U", "#EXT-X-VERSION:6", "#EXTINF:6.0,", "seg.ts", ""}, "\n")
	assert.ErrorContains(t, gen.ValidatePlaylist(missingTargetDuration), "missing #EXT-X-TARGETDURATION")

	duration, err := GetSegmentDurationFromPlaylist(validVariant)
	require.NoError(t, err)
	assert.Equal(t, 6.0, duration)

	_, err = GetSegmentDurationFromPlaylist("#EXTM3U\n")
	assert.ErrorContains(t, err, "target duration not found")
}

func TestHLSGenerator_estimateKeyframePositions(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:      "https://cdn.example.com",
		SegmentDuration: 6,
	}
	gen := NewHLSGenerator(cfg, nil)

	assert.Nil(t, gen.estimateKeyframePositions(nil))
	assert.Nil(t, gen.estimateKeyframePositions(&MediaMetadata{Duration: 0}))

	meta := &MediaMetadata{
		MediaID:      "m1",
		Duration:     10,
		Bitrate:      4000,
		VideoProfile: "High",
	}
	keyframes := gen.estimateKeyframePositions(meta)
	require.NotEmpty(t, keyframes)
	assert.Equal(t, 0.0, keyframes[0].PTS)
	assert.Contains(t, keyframes[0].URI, "/media/m1/")
	assert.GreaterOrEqual(t, keyframes[0].ByteLength, int64(0))
}

func TestHLSGenerator_IFrameMasterPlaylist(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:      "https://cdn.example.com",
		SegmentDuration: 6,
	}
	gen := NewHLSGenerator(cfg, nil)

	manifest := &HLSManifest{
		MediaID: "m1",
		Variants: []HLSVariant{
			{Quality: Quality360p, Bandwidth: 1000, Resolution: "640x360", Codecs: "avc1.42001e,mp4a.40.2"},
			{Quality: Quality720p, Bandwidth: 4000, Resolution: "1280x720", Codecs: "avc1.64001f,mp4a.40.2"},
		},
	}

	content := gen.GenerateIFrameMasterPlaylist(manifest)
	assert.Contains(t, content, "#EXTM3U")
	assert.Contains(t, content, "#EXT-X-I-FRAME-STREAM-INF")
	assert.Contains(t, content, "iframe.m3u8")
	assert.Contains(t, content, "I-FRAME-STREAM-INF")
	assert.Contains(t, content, "BANDWIDTH=1000")
	assert.Contains(t, content, "BANDWIDTH=4000")

	// Ensure template uses correct URL scheme
	assert.Contains(t, content, "https://cdn.example.com/media/m1/360p/iframe.m3u8")
	assert.Contains(t, content, "https://cdn.example.com/media/m1/720p/iframe.m3u8")
}

func TestHLSGenerator_getKeyframePositions_PrefersStorageOverMetadata(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:      "https://cdn.example.com",
		SegmentDuration: 6,
	}

	keyframePlaylist := []byte(strings.Join([]string{
		"#EXTM3U",
		"#EXT-X-VERSION:6",
		"#EXT-X-I-FRAMES-ONLY",
		"#EXT-X-BYTERANGE:5000@1000",
		"#EXTINF:2.000,",
		"segment000.ts",
		"#EXT-X-ENDLIST",
		"",
	}, "\n"))

	storage := &countingStorage{keyframes: keyframePlaylist, metadata: &MediaMetadata{MediaID: "m1"}}
	gen := NewHLSGenerator(cfg, storage)

	meta := &MediaMetadata{
		MediaID:            "m1",
		Duration:           10,
		KeyframePositions:  []float64{0, 2, 4},
		AvailableQualities: []Quality{Quality720p},
	}

	keyframes := gen.getKeyframePositions("m1", Quality720p, meta)
	require.NotEmpty(t, keyframes)
	assert.Equal(t, int64(1000), keyframes[0].ByteOffset)

	// If storage returns no data, fall back to metadata positions.
	storage.keyframes = nil
	keyframes = gen.getKeyframePositions("m1", Quality720p, meta)
	require.NotEmpty(t, keyframes)
	assert.Equal(t, 0.0, keyframes[0].PTS)
}

func TestHLSGenerator_GetCodecsWithMetadata_FallbackToDefaults(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:      "https://cdn.example.com",
		SegmentDuration: 6,
	}
	gen := NewHLSGenerator(cfg, &countingStorage{metadataErr: assert.AnError})

	assert.Equal(t, "avc1.640028,mp4a.40.2", gen.getCodecsWithMetadata("m1", Quality1080p))
	assert.Equal(t, "avc1.42001e,mp4a.40.2", gen.getCodecsWithMetadata("m1", Quality240p))

	// Also exercise the nil-storage fallback.
	gen.storage = nil
	assert.Equal(t, "avc1.64001e,mp4a.40.2", gen.getCodecsWithMetadata("m1", Quality720p))
}

func TestHLSGenerator_ManifestCacheDurations(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:       "https://cdn.example.com",
		SegmentDuration:  6,
		ManifestCacheTTL: 5 * time.Minute,
	}
	gen := NewHLSGenerator(cfg, nil)

	manifest, err := gen.GenerateMasterPlaylist("m1", &MediaMetadata{MediaID: "m1", Duration: 10, AvailableQualities: []Quality{Quality720p}})
	require.NoError(t, err)
	assert.Equal(t, cfg.ManifestCacheTTL, manifest.CacheDuration)
}

