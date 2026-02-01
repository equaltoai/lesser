package transcoding

import (
	"context"
	stderrors "errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeS3Client struct {
	headExists map[string]bool
	headCalls  []string

	listKeysByPrefix map[string][]string
	listErrByPrefix  map[string]error
	listCalls        []string

	putErrByKey map[string]error
	putCalls    []fakeS3PutCall
}

type fakeS3PutCall struct {
	key         string
	contentType string
	body        string
}

func (f *fakeS3Client) HeadObject(_ context.Context, params *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	key := aws.ToString(params.Key)
	f.headCalls = append(f.headCalls, key)
	if f.headExists[key] {
		return &s3.HeadObjectOutput{}, nil
	}
	return nil, stderrors.New("not found")
}

func (f *fakeS3Client) ListObjectsV2(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := aws.ToString(params.Prefix)
	f.listCalls = append(f.listCalls, prefix)
	if err := f.listErrByPrefix[prefix]; err != nil {
		return nil, err
	}
	keys := f.listKeysByPrefix[prefix]

	contents := make([]s3types.Object, 0, len(keys))
	for _, key := range keys {
		contents = append(contents, s3types.Object{Key: aws.String(key)})
	}

	return &s3.ListObjectsV2Output{Contents: contents}, nil
}

func (f *fakeS3Client) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	key := aws.ToString(params.Key)
	if err := f.putErrByKey[key]; err != nil {
		return nil, err
	}
	data, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}
	f.putCalls = append(f.putCalls, fakeS3PutCall{
		key:         key,
		contentType: aws.ToString(params.ContentType),
		body:        string(data),
	})
	return &s3.PutObjectOutput{}, nil
}

func TestManifestService_GenerateHLSMasterPlaylist_round26_coverage(t *testing.T) {
	t.Parallel()

	s3Client := &fakeS3Client{}
	svc := NewManifestService(s3Client, ManifestConfig{Bucket: "bucket", CDNDomain: "cdn.example"}, zap.NewNop())

	err := svc.GenerateHLSMasterPlaylist(context.Background(), "media1", "out", []VariantInfo{
		{Quality: "480p", Width: 854, Height: 480, Bitrate: 1500000, Codec: "avc1"},
		{Quality: "720p", Width: 1280, Height: 720, Bitrate: 3000000, Codec: "avc1"},
	})
	require.NoError(t, err)
	require.Len(t, s3Client.putCalls, 1)
	assert.Equal(t, "out/hls/master.m3u8", s3Client.putCalls[0].key)
	assert.Equal(t, "application/vnd.apple.mpegurl", s3Client.putCalls[0].contentType)
	assert.Contains(t, s3Client.putCalls[0].body, "#EXTM3U")
	assert.Contains(t, s3Client.putCalls[0].body, "480p.m3u8")
	assert.Contains(t, s3Client.putCalls[0].body, "720p.m3u8")
}

func TestManifestService_GenerateHLSMasterPlaylist_upload_error_round26_coverage(t *testing.T) {
	t.Parallel()

	s3Client := &fakeS3Client{
		putErrByKey: map[string]error{
			"out/hls/master.m3u8": stderrors.New("nope"),
		},
	}
	svc := NewManifestService(s3Client, ManifestConfig{Bucket: "bucket"}, zap.NewNop())

	err := svc.GenerateHLSMasterPlaylist(context.Background(), "media1", "out", []VariantInfo{{Quality: "720p", Width: 1280, Height: 720, Bitrate: 3000000, Codec: "avc1"}})
	require.ErrorIs(t, err, ErrManifestGenerationFailed)
}

func TestManifestService_GenerateDASHManifest_round26_coverage(t *testing.T) {
	t.Parallel()

	s3Client := &fakeS3Client{}
	svc := NewManifestService(s3Client, ManifestConfig{Bucket: "bucket"}, zap.NewNop())

	err := svc.GenerateDASHManifest(context.Background(), "media1", "out", []VariantInfo{
		{Quality: "720p", Width: 1280, Height: 720, Bitrate: 3000000, Codec: "avc1.42E01E"},
	}, 300)
	require.NoError(t, err)
	require.Len(t, s3Client.putCalls, 1)
	assert.Equal(t, "out/dash/manifest.mpd", s3Client.putCalls[0].key)
	assert.Equal(t, "application/dash+xml", s3Client.putCalls[0].contentType)
	assert.Contains(t, s3Client.putCalls[0].body, "<MPD")
	assert.Contains(t, s3Client.putCalls[0].body, "mediaPresentationDuration=\"PT300S\"")
	assert.Contains(t, s3Client.putCalls[0].body, "<BaseURL>720p.mp4</BaseURL>")
}

func TestManifestService_GenerateDASHManifest_upload_error_round26_coverage(t *testing.T) {
	t.Parallel()

	s3Client := &fakeS3Client{
		putErrByKey: map[string]error{
			"out/dash/manifest.mpd": stderrors.New("nope"),
		},
	}
	svc := NewManifestService(s3Client, ManifestConfig{Bucket: "bucket"}, zap.NewNop())

	err := svc.GenerateDASHManifest(context.Background(), "media1", "out", []VariantInfo{{Quality: "720p", Width: 1280, Height: 720, Bitrate: 3000000, Codec: "avc1"}}, 60)
	require.ErrorIs(t, err, ErrManifestGenerationFailed)
}

func TestManifestService_GetManifestInfo_round26_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("no_manifests_returns_not_found", func(t *testing.T) {
		s3Client := &fakeS3Client{
			listKeysByPrefix: map[string][]string{
				"out/thumbnails/": {"out/thumbnails/1.jpg"},
			},
		}
		svc := NewManifestService(s3Client, ManifestConfig{Bucket: "bucket"}, zap.NewNop())

		_, err := svc.GetManifestInfo(ctx, "media1", "out")
		require.ErrorIs(t, err, ErrManifestNotFound)
	})

	t.Run("hls_master_with_variants_and_thumbnails", func(t *testing.T) {
		s3Client := &fakeS3Client{
			headExists: map[string]bool{
				"out/hls/master.m3u8": true,
			},
			listKeysByPrefix: map[string][]string{
				"out/hls/":        {"out/hls/master.m3u8", "out/hls/720p.m3u8", "out/hls/readme.txt"},
				"out/thumbnails/": {"out/thumbnails/1.jpg", "out/thumbnails/2.png", "out/thumbnails/skip.txt"},
			},
		}
		svc := NewManifestService(s3Client, ManifestConfig{Bucket: "bucket", CDNDomain: "cdn.example"}, zap.NewNop())

		info, err := svc.GetManifestInfo(ctx, "media1", "out")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "media1", info.MediaID)
		assert.Equal(t, "https://cdn.example/out/hls/master.m3u8", info.HLSMasterURL)
		assert.Empty(t, info.DASHManifestURL)
		require.Len(t, info.Variants, 1)
		assert.Equal(t, "720p", info.Variants[0].Quality)
		assert.Equal(t, "https://cdn.example/out/hls/720p.m3u8", info.Variants[0].HLSPlaylistURL)
		assert.Len(t, info.ThumbnailURLs, 2)
	})

	t.Run("dash_manifest_with_thumbnail_error_still_succeeds", func(t *testing.T) {
		s3Client := &fakeS3Client{
			headExists: map[string]bool{
				"out/dash/manifest.mpd": true,
			},
			listErrByPrefix: map[string]error{
				"out/thumbnails/": stderrors.New("boom"),
			},
		}
		svc := NewManifestService(s3Client, ManifestConfig{Bucket: "bucket"}, zap.NewNop())

		info, err := svc.GetManifestInfo(ctx, "media1", "out")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Empty(t, info.HLSMasterURL)
		assert.Equal(t, "https://bucket.s3.amazonaws.com/out/dash/manifest.mpd", info.DASHManifestURL)
		assert.Empty(t, info.ThumbnailURLs)
	})

	t.Run("hls_variants_error_is_non_fatal", func(t *testing.T) {
		s3Client := &fakeS3Client{
			headExists: map[string]bool{
				"out/hls/master.m3u8": true,
			},
			listErrByPrefix: map[string]error{
				"out/hls/": stderrors.New("boom"),
			},
		}
		svc := NewManifestService(s3Client, ManifestConfig{Bucket: "bucket"}, zap.NewNop())

		info, err := svc.GetManifestInfo(ctx, "media1", "out")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.NotEmpty(t, info.HLSMasterURL)
		assert.Empty(t, info.Variants)
	})
}

func TestManifestService_PreloadManifests_round26_coverage(t *testing.T) {
	t.Parallel()

	s3Client := &fakeS3Client{
		headExists: map[string]bool{
			"m1/hls/master.m3u8": true,
		},
	}
	svc := NewManifestService(s3Client, ManifestConfig{Bucket: "bucket"}, zap.NewNop())

	err := svc.PreloadManifests(context.Background(), []string{"m1", "m2"})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(s3Client.headCalls), 2)
}
