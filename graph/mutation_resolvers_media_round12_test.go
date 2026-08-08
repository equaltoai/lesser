package graph

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/media/transcoding"
	"github.com/stretchr/testify/require"
)

type round12ReadSeekCloser struct {
	*bytes.Reader
	closeErr error
}

func (r *round12ReadSeekCloser) Close() error { return r.closeErr }

type round12BadReadSeeker struct{}

func (round12BadReadSeeker) Read([]byte) (int, error)       { return 0, errors.New("read failed") }
func (round12BadReadSeeker) Seek(int64, int) (int64, error) { return 0, nil }

type round12CloudFront struct{}

func (round12CloudFront) SignStreamingURL(mediaID, format string, quality *string, ttl time.Duration) (string, error) {
	q := "auto"
	if quality != nil {
		q = *quality
	}
	return "https://cdn.local/stream/" + mediaID + "/" + format + "/" + q, nil
}

type round12ManifestService struct {
	preloadCalled bool
}

func (m *round12ManifestService) PreloadManifests(context.Context, []string) error {
	m.preloadCalled = true
	return nil
}

func (m *round12ManifestService) GetManifestInfo(context.Context, string, string) (*transcoding.ManifestInfo, error) {
	return &transcoding.ManifestInfo{
		HLSMasterURL:    "https://cdn.local/hls/master.m3u8",
		DASHManifestURL: "https://cdn.local/dash/manifest.mpd",
		ThumbnailURLs:   []string{"https://cdn.local/thumb.jpg"},
		Variants: []transcoding.VariantInfo{
			{Quality: "480p", Width: 854, Height: 480, Bitrate: 1500000, Codec: "h264", HLSPlaylistURL: "https://cdn.local/hls/480.m3u8", DASHSegmentURL: "https://cdn.local/dash/480.m4s"},
			{Quality: "720p", Width: 1280, Height: 720, Bitrate: 3000000, Codec: "h264", HLSPlaylistURL: "https://cdn.local/hls/720.m3u8", DASHSegmentURL: "https://cdn.local/dash/720.m4s"},
		},
		GeneratedAt: time.Now(),
	}, nil
}

func TestRound12MediaResolver_UploadUpdateAndStreaming(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := resolver.Mutation()
	ctx := round12AuthContext("alice")

	m := mut.(*mutationResolver)

	max := m.getMaxUploadSize()
	require.Greater(t, max, int64(0))

	upload := graphql.Upload{
		File: &round12ReadSeekCloser{
			Reader:   bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xD9}),
			closeErr: errors.New("close failed"),
		},
		Filename: "test.jpg",
	}

	desc := "alt text"
	focus := &model.FocusInput{X: 0.25, Y: -0.25}
	spoiler := "spoiler"
	sensitive := true
	payload, err := mut.UploadMedia(ctx, model.UploadMediaInput{
		File:        upload,
		Description: &desc,
		Focus:       focus,
		Sensitive:   &sensitive,
		SpoilerText: &spoiler,
	})
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.NotNil(t, payload.Media)
	require.NotEmpty(t, payload.UploadID)

	newDesc := "updated alt"
	updated, err := mut.UpdateMedia(ctx, payload.UploadID, model.UpdateMediaInput{
		Description: &newDesc,
		Focus:       &model.FocusInput{X: 0.1, Y: 0.2},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.Description)
	require.Equal(t, newDesc, *updated.Description)

	deleted, err := mut.DeleteMedia(ctx, payload.UploadID)
	require.NoError(t, err)
	require.True(t, deleted)

	stream, err := mut.RequestStreamingURL(ctx, payload.UploadID, nil)
	require.Error(t, err)
	require.Nil(t, stream)

	mediaService := resolver.Registry.Media()
	require.NotNil(t, mediaService)
	manifest := &round12ManifestService{}
	mediaService.SetManifestService(manifest)
	mediaService.SetCloudFrontService(round12CloudFront{})

	stream, err = mut.RequestStreamingURL(ctx, payload.UploadID, nil)
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.NotEmpty(t, stream.URL)
	require.NotEmpty(t, stream.ThumbnailURL)
	require.NotNil(t, stream.Bitrates)
	require.Len(t, stream.Bitrates, 2)

	stream, err = mut.RequestStreamingURL(ctx, payload.UploadID, ptrStreamQuality(model.StreamQualityHigh))
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.Len(t, stream.Bitrates, 1)

	streams, err := mut.PreloadMedia(ctx, []string{payload.UploadID})
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.NotNil(t, streams[0].HlsPlaylistURL)
	require.NotNil(t, streams[0].DashManifestURL)
	require.True(t, manifest.preloadCalled)

	report, err := mut.ReportStreamingQuality(ctx, model.StreamingQualityInput{
		MediaID:         payload.UploadID,
		Quality:         model.StreamQualityMedium,
		BufferingEvents: 1,
		WatchTime:       10,
	})
	require.NoError(t, err)
	require.NotNil(t, report)
	require.True(t, report.Success)
	require.Equal(t, payload.UploadID, report.MediaID)
}

func TestRound12MediaResolver_UploadHelpers(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	m := resolver.Mutation().(*mutationResolver)

	// readUploadFile error cases
	_, err := m.readUploadFile(graphql.Upload{}, 10)
	require.Error(t, err)

	_, err = m.readUploadFile(graphql.Upload{File: round12BadReadSeeker{}}, 10)
	require.Error(t, err)

	_, err = m.readUploadFile(graphql.Upload{File: bytes.NewReader([]byte("too-big"))}, 1)
	require.Error(t, err)

	_, err = m.readUploadFile(graphql.Upload{File: bytes.NewReader(nil)}, 10)
	require.Error(t, err)

	// filename helpers
	name, err := normalizeUploadFilename(graphql.Upload{Filename: "x"}, strPtr("override"), "image/jpeg")
	require.NoError(t, err)
	require.NotEmpty(t, name)

	name, err = normalizeUploadFilename(graphql.Upload{}, nil, "image/jpeg")
	require.NoError(t, err)
	require.NotEmpty(t, name)

	require.Equal(t, "image/jpeg", detectUploadContentType(graphql.Upload{ContentType: " image/jpeg "}, []byte("x")))
	require.NotEmpty(t, detectUploadContentType(graphql.Upload{}, []byte("x")))

	_, err = validateUploadDescription(strPtr(strings.Repeat("a", 2000)))
	require.Error(t, err)
	desc, err := validateUploadDescription(strPtr("  "))
	require.NoError(t, err)
	require.Empty(t, desc)

	_, err = normalizeUploadFocus(&model.FocusInput{X: 2, Y: 0})
	require.Error(t, err)
	_, err = validateUploadSpoilerText(strPtr(strings.Repeat("a", 600)))
	require.Error(t, err)

	_, err = normalizeUploadMediaCategory(ptrMediaCategory(model.MediaCategory("invalid")), "image/jpeg")
	require.Error(t, err)
	_, err = ensureFilenameExtension(" ", "image/jpeg")
	require.Error(t, err)

	name, err = ensureFilenameExtension("file", "invalid/type")
	require.NoError(t, err)
	require.Equal(t, "file.bin", name)

	// getMaxUploadSize: registry config fallback
	resolver.Config = nil
	cfg := resolver.Registry.GetConfig()
	require.NotNil(t, cfg)
	cfg.Config.MaxUploadSize = 123
	require.Equal(t, int64(123), m.getMaxUploadSize())
}

func ptrStreamQuality(v model.StreamQuality) *model.StreamQuality { return &v }

func strPtr(v string) *string { return &v }

func ptrMediaCategory(v model.MediaCategory) *model.MediaCategory { return &v }
