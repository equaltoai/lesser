package streaming

import (
	"encoding/xml"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubMediaStorage struct {
	metadata *MediaMetadata
	err      error
}

func (s stubMediaStorage) GetManifestPath(mediaID string, format MediaFormat, quality Quality) string {
	return ""
}

func (s stubMediaStorage) GetSegmentPath(mediaID string, quality Quality, segmentIndex int) string {
	return ""
}

func (s stubMediaStorage) GetMediaMetadata(mediaID string) (*MediaMetadata, error) {
	return s.metadata, s.err
}

func (s stubMediaStorage) ManifestExists(mediaID string, format MediaFormat) (bool, error) {
	return false, nil
}

func (s stubMediaStorage) GetKeyframeData(mediaID string, quality Quality) ([]byte, error) {
	return nil, nil
}

func TestDASHGenerator_GenerateMPD_AndValidate(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:       "https://cdn.example.com",
		SegmentDuration:  6,
		ManifestCacheTTL: 5 * time.Minute,
	}
	gen := NewDASHGenerator(cfg, nil)

	metadata := &MediaMetadata{
		MediaID:            "m1",
		Duration:           123.4,
		AvailableQualities: []Quality{Quality480p, Quality1080p},
	}

	manifest, err := gen.GenerateMPD("m1", metadata)
	require.NoError(t, err)
	require.Len(t, manifest.Representations, 2)
	assert.Equal(t, "m1", manifest.MediaID)
	assert.Equal(t, metadata.Duration, manifest.Duration)
	assert.Equal(t, "https://cdn.example.com/media/m1/manifest.mpd", manifest.ManifestURL)

	assert.Equal(t, Quality480p, manifest.Representations[0].Quality)
	assert.Equal(t, "https://cdn.example.com/media/m1/480p/", manifest.Representations[0].BaseURL)
	assert.Equal(t, "segment$Number$.m4s", manifest.Representations[0].SegmentTemplate)

	content, err := gen.GenerateMPDContent(manifest)
	require.NoError(t, err)
	require.NoError(t, gen.ValidateMPD(content))

	var mpd MPD
	require.NoError(t, xml.Unmarshal([]byte(content), &mpd))
	require.Len(t, mpd.Periods, 1)
	require.Len(t, mpd.Periods[0].AdaptationSets, 1)
	assert.Equal(t, 1920, mpd.Periods[0].AdaptationSets[0].MaxWidth)
	assert.Equal(t, 1080, mpd.Periods[0].AdaptationSets[0].MaxHeight)
}

func TestDASHGenerator_ValidateMPD_Errors(t *testing.T) {
	cfg := &StreamingConfig{CDNBaseURL: "https://cdn.example.com", SegmentDuration: 6}
	gen := NewDASHGenerator(cfg, nil)

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "InvalidXML",
			content: "<MPD>",
			want:    "invalid MPD XML",
		},
		{
			name:    "MissingType",
			content: `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011"><Period id="0"><AdaptationSet id="0"><Representation id="v0"></Representation></AdaptationSet></Period></MPD>`,
			want:    "missing type attribute",
		},
		{
			name:    "NoPeriods",
			content: `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static"></MPD>`,
			want:    "no periods defined",
		},
		{
			name:    "NoAdaptationSets",
			content: `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static"><Period id="0"></Period></MPD>`,
			want:    "has no adaptation sets",
		},
		{
			name:    "NoRepresentations",
			content: `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static"><Period id="0"><AdaptationSet id="0"></AdaptationSet></Period></MPD>`,
			want:    "has no representations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := gen.ValidateMPD(tt.content)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestDASHGenerator_GenerateLiveMPD_Content_AndValidate(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:       "https://cdn.example.com",
		SegmentDuration:  6,
		ManifestCacheTTL: 5 * time.Minute,
	}

	storage := stubMediaStorage{
		metadata: &MediaMetadata{
			MediaID:            "live1",
			IsLive:             true,
			AvailableQualities: []Quality{Quality720p, Quality4K},
			QualitySettings: map[Quality]QualityCodecInfo{
				Quality4K: {Bandwidth: 25000},
			},
		},
	}
	gen := NewDASHGenerator(cfg, storage)

	manifest, err := gen.GenerateLiveMPD("live1", 0)
	require.NoError(t, err)
	assert.True(t, manifest.IsLive)
	assert.Equal(t, 1800.0, manifest.TimeShiftBufferDepth)
	assert.GreaterOrEqual(t, manifest.SuggestedPresentationDelay, 12.0)
	assert.GreaterOrEqual(t, manifest.MinimumUpdatePeriod, 1.0)
	assert.Equal(t, "https://cdn.example.com/live/live1/manifest.mpd", manifest.ManifestURL)
	require.Len(t, manifest.Representations, 2)

	content, err := gen.GenerateLiveMPDContent(manifest)
	require.NoError(t, err)
	require.NoError(t, gen.ValidateLiveManifest(content))

	// Force a validation error
	err = gen.ValidateLiveManifest(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static"></MPD>`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type='dynamic'")
}

func TestDASHGenerator_GenerateMPDWithSubtitles(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:      "https://cdn.example.com",
		SegmentDuration: 6,
	}
	gen := NewDASHGenerator(cfg, nil)

	subtitles := []SubtitleTrack{
		{Name: "English", Language: "en", URI: "subs/en.m3u8"},
		{Name: "Spanish", Language: "es", URI: "subs/es.m3u8"},
	}

	vodManifest := &DASHManifest{
		MediaID:       "vod1",
		Duration:      60,
		MinBufferTime: 6,
		Representations: []DASHRepresentation{
			{ID: "video_0", Bandwidth: 1000, Width: 640, Height: 360, Codecs: "avc1.42001e", BaseURL: "base", SegmentTemplate: "segment$Number$.m4s"},
		},
	}

	content, err := gen.GenerateMPDWithSubtitles(vodManifest, subtitles)
	require.NoError(t, err)
	var mpd MPD
	require.NoError(t, xml.Unmarshal([]byte(content), &mpd))
	require.Len(t, mpd.Periods, 1)
	require.Len(t, mpd.Periods[0].AdaptationSets, 2)
	assert.Equal(t, "subs", mpd.Periods[0].AdaptationSets[1].ID)
	assert.Equal(t, "text/vtt", mpd.Periods[0].AdaptationSets[1].MimeType)
	assert.Len(t, mpd.Periods[0].AdaptationSets[1].Representations, len(subtitles))

	liveManifest := &DASHManifest{
		MediaID:                    "live2",
		IsLive:                     true,
		AvailabilityStartTime:      time.Now().Add(-time.Hour),
		PublishTime:                time.Now(),
		MinBufferTime:              6,
		TimeShiftBufferDepth:       1800,
		SuggestedPresentationDelay: 12,
		MinimumUpdatePeriod:        6,
		Representations: []DASHRepresentation{
			{ID: "video_0", Bandwidth: 1000, Width: 640, Height: 360, Codecs: "avc1.42001e,mp4a.40.2", BaseURL: "base", SegmentTemplate: "segment-$Time$.m4s"},
		},
	}

	liveContent, err := gen.GenerateMPDWithSubtitles(liveManifest, subtitles)
	require.NoError(t, err)
	var liveMPD LiveMPD
	require.NoError(t, xml.Unmarshal([]byte(liveContent), &liveMPD))
	require.Len(t, liveMPD.Periods, 1)
	require.Len(t, liveMPD.Periods[0].AdaptationSets, 2)
	assert.Equal(t, "subs", liveMPD.Periods[0].AdaptationSets[1].ID)
	assert.Equal(t, "text/vtt", liveMPD.Periods[0].AdaptationSets[1].MimeType)
	assert.Len(t, liveMPD.Periods[0].AdaptationSets[1].Representations, len(subtitles))
}

func TestDASHGenerator_AudioAdaptationSet_AndCodecSelection(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:      "https://cdn.example.com",
		SegmentDuration: 6,
	}
	gen := NewDASHGenerator(cfg, nil)

	audio := gen.GenerateAudioAdaptationSet([]AudioTrack{
		{Language: "en", Bitrate: 128, Codec: "mp4a.40.2", BaseURL: "a/en/"},
		{Language: "es", Bitrate: 96, Codec: "mp4a.40.2", BaseURL: "a/es/"},
	})
	assert.Equal(t, "audio/mp4", audio.MimeType)
	require.Len(t, audio.Representations, 2)
	assert.Equal(t, 128000, audio.Representations[0].Bandwidth)

	assert.Equal(t, "avc1.640028", gen.getCodecs(Quality1080p))
	assert.Equal(t, "avc1.64001e", gen.getCodecs(Quality720p))
	assert.Equal(t, "avc1.42001e", gen.getCodecs(Quality240p))

	meta := &MediaMetadata{
		VideoCodec: "avc1.test",
		QualitySettings: map[Quality]QualityCodecInfo{
			Quality4K: {VideoCodec: "avc1.4k"},
		},
	}
	assert.Equal(t, "avc1.4k,mp4a.40.2", gen.getCodecsForLive(Quality4K, meta))
	assert.Equal(t, "avc1.test,mp4a.40.2", gen.getCodecsForLive(Quality720p, meta))
}

func TestDASHGenerator_LiveBoundsAndValidationErrors(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:      "https://cdn.example.com",
		SegmentDuration: 1, // forces minimumUpdatePeriod floor
	}

	storage := stubMediaStorage{
		metadata: &MediaMetadata{
			MediaID:            "live1",
			IsLive:             true,
			AvailableQualities: []Quality{Quality720p},
		},
	}
	gen := NewDASHGenerator(cfg, storage)

	manifest, err := gen.GenerateLiveMPD("live1", 999999)
	require.NoError(t, err)
	assert.Equal(t, 7200.0, manifest.TimeShiftBufferDepth)
	assert.Equal(t, 1.0, manifest.MinimumUpdatePeriod)

	// Validation error cases
	assert.ErrorContains(t, gen.ValidateLiveManifest("<MPD>"), "invalid live MPD XML")
	assert.ErrorContains(t, gen.ValidateLiveManifest(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="dynamic"></MPD>`), "availabilityStartTime")
	assert.ErrorContains(t, gen.ValidateLiveManifest(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="dynamic" availabilityStartTime="2026-01-02T00:00:00Z"></MPD>`), "publishTime")
	assert.ErrorContains(t, gen.ValidateLiveManifest(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="dynamic" availabilityStartTime="2026-01-02T00:00:00Z" publishTime="2026-01-02T00:00:01Z"></MPD>`), "no periods")
	assert.ErrorContains(t, gen.ValidateLiveManifest(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="dynamic" availabilityStartTime="2026-01-02T00:00:00Z" publishTime="2026-01-02T00:00:01Z"><Period id="0"></Period></MPD>`), "no adaptation sets")
	assert.ErrorContains(t, gen.ValidateLiveManifest(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="dynamic" availabilityStartTime="2026-01-02T00:00:00Z" publishTime="2026-01-02T00:00:01Z"><Period id="0"><AdaptationSet id="0"></AdaptationSet></Period></MPD>`), "no representations")
}
