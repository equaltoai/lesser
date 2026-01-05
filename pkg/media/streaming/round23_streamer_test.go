package streaming

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type countingStorage struct {
	getMetaCalls atomic.Int64

	metadata    *MediaMetadata
	metadataErr error

	segmentPath string
	keyframes   []byte
}

func (s *countingStorage) GetManifestPath(mediaID string, format MediaFormat, quality Quality) string {
	return ""
}

func (s *countingStorage) GetSegmentPath(mediaID string, quality Quality, segmentIndex int) string {
	if s.segmentPath != "" {
		return s.segmentPath
	}
	return fmt.Sprintf("media/%s/%s/segment%03d.ts", mediaID, quality, segmentIndex)
}

func (s *countingStorage) GetMediaMetadata(mediaID string) (*MediaMetadata, error) {
	s.getMetaCalls.Add(1)
	return s.metadata, s.metadataErr
}

func (s *countingStorage) ManifestExists(mediaID string, format MediaFormat) (bool, error) {
	return false, nil
}

func (s *countingStorage) GetKeyframeData(mediaID string, quality Quality) ([]byte, error) {
	return s.keyframes, nil
}

func newTestStreamerWithDeps(t testing.TB, cfg *StreamingConfig) (*Streamer, *MockAnalytics, *fakeDynamormDB) {
	t.Helper()

	db := newFakeDynamormDB()
	mem := newS3Memory()
	s3srv := newTestS3Server(t, cfg.S3Bucket, mem)
	t.Cleanup(s3srv.Close)

	trendingRepo := repositories.NewTrendingRepository(db, zap.NewNop(), nil)
	analytics := &MockAnalytics{analyticsRepo: trendingRepo}

	streamer := NewStreamer(
		cfg,
		analytics,
		newTestS3Client(s3srv.URL),
		nil,
		db,
		zap.NewNop(),
		noopCostTracker{},
	)

	return streamer, analytics, db
}

func TestStreamer_generateManifest_Cache_StatusAndErrors(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:       "https://cdn.example.com",
		S3Bucket:         "test-bucket",
		S3Region:         "us-east-1",
		SegmentDuration:  6,
		ManifestCacheTTL: 50 * time.Millisecond,
		DefaultQuality:   Quality480p,
	}

	streamer, _, db := newTestStreamerWithDeps(t, cfg)

	storage := &countingStorage{
		metadata: &MediaMetadata{
			MediaID:            "m1",
			Status:             StatusComplete,
			Duration:           30,
			AvailableQualities: []Quality{Quality720p},
		},
	}
	streamer.storage = storage

	var generatorCalls int
	generator := func(id string, meta *MediaMetadata) (interface{}, error) {
		generatorCalls++
		return &HLSManifest{MediaID: id, Duration: meta.Duration}, nil
	}

	out1, err := streamer.generateManifest("m1", "hls", generator)
	require.NoError(t, err)
	require.NotNil(t, out1)
	assert.Equal(t, 1, generatorCalls)
	assert.Equal(t, int64(1), storage.getMetaCalls.Load())

	out2, err := streamer.generateManifest("m1", "hls", generator)
	require.NoError(t, err)
	require.NotNil(t, out2)
	assert.Equal(t, 1, generatorCalls)
	assert.Equal(t, int64(1), storage.getMetaCalls.Load())

	// Expire cached entry and confirm we regenerate.
	streamer.manifestCache.Store("hls:m1", &cachedManifest{hlsManifest: out1.(*HLSManifest), generatedAt: time.Now().Add(-time.Hour)})
	_, err = streamer.generateManifest("m1", "hls", generator)
	require.NoError(t, err)
	assert.Equal(t, 2, generatorCalls)

	// Not ready
	streamer.manifestCache.Delete("hls:m1")
	storage.metadata.Status = StatusProcessing
	_, err = streamer.generateManifest("m1", "hls", generator)
	require.Error(t, err)
	var se *StreamingError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, "MEDIA_NOT_READY", se.Code)

	// Storage error
	streamer.manifestCache.Delete("hls:m1")
	storage.metadata.Status = StatusComplete
	storage.metadataErr = errors.New("boom")
	_, err = streamer.generateManifest("m1", "hls", generator)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get media metadata")

	// Generator error
	streamer.manifestCache.Delete("hls:m1")
	storage.metadataErr = nil
	_, err = streamer.generateManifest("m1", "hls", func(string, *MediaMetadata) (interface{}, error) {
		return nil, errors.New("gen boom")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generate hls manifest")

	// RecordManifestGeneration errors are non-fatal
	streamer.manifestCache.Delete("hls:m1")
	db.forceCreateErr = errors.New("analytics boom")
	storage.getMetaCalls.Store(0)
	out, err := streamer.generateManifest("m1", "hls", generator)
	require.NoError(t, err)
	assert.NotNil(t, out)
}

func TestStreamer_GenerateHLSManifest_AndGenerateDASHManifest(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:       "https://cdn.example.com",
		S3Bucket:         "test-bucket",
		S3Region:         "us-east-1",
		SegmentDuration:  6,
		ManifestCacheTTL: time.Minute,
		DefaultQuality:   Quality480p,
	}

	streamer, _, _ := newTestStreamerWithDeps(t, cfg)

	storage := &countingStorage{
		metadata: &MediaMetadata{
			MediaID:            "m1",
			Status:             StatusComplete,
			Duration:           30,
			AvailableQualities: []Quality{Quality720p, Quality1080p},
			VideoCodec:         "avc1.640028",
			AudioCodec:         "mp4a.40.2",
		},
	}
	streamer.storage = storage
	streamer.hlsGenerator = NewHLSGenerator(cfg, storage)
	streamer.dashGenerator = NewDASHGenerator(cfg, storage)

	hls, err := streamer.GenerateHLSManifest("m1")
	require.NoError(t, err)
	assert.Equal(t, "m1", hls.MediaID)

	dash, err := streamer.GenerateDASHManifest("m1")
	require.NoError(t, err)
	assert.Equal(t, "m1", dash.MediaID)
	assert.Len(t, dash.Representations, 2)
}

func TestStreamer_GetSegmentURL_CDNAndPresign(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:       "https://cdn.example.com",
		S3Bucket:         "test-bucket",
		S3Region:         "us-east-1",
		SegmentDuration:  6,
		ManifestCacheTTL: time.Minute,
		DefaultQuality:   Quality480p,
	}

	streamer, _, _ := newTestStreamerWithDeps(t, cfg)

	storage := &countingStorage{
		segmentPath: "media/m1/720p/segment000.ts",
		metadata: &MediaMetadata{
			MediaID:            "m1",
			Status:             StatusComplete,
			Duration:           12,
			AvailableQualities: []Quality{Quality720p},
		},
	}
	streamer.storage = storage

	url, err := streamer.GetSegmentURL("m1", Quality720p, 0)
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example.com/media/m1/720p/segment000.ts", url)

	urls, err := streamer.GetSegmentURLs("m1", Quality720p, 0, 2)
	require.NoError(t, err)
	assert.Len(t, urls, 2)

	_, err = streamer.GetSegmentURL("m1", Quality720p, -1)
	require.Error(t, err)

	_, err = streamer.GetSegmentURL("m1", Quality720p, 100)
	require.Error(t, err)

	_, err = streamer.GetSegmentURL("m1", Quality1080p, 0)
	require.Error(t, err)

	// Presign branch
	streamer.config.CDNBaseURL = ""
	url, err = streamer.GetSegmentURL("m1", Quality720p, 0)
	require.NoError(t, err)
	assert.Contains(t, url, "X-Amz-Signature")
}

func TestStreamer_GetAvailableQualities_AndForUser(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:       "https://cdn.example.com",
		S3Bucket:         "test-bucket",
		S3Region:         "us-east-1",
		SegmentDuration:  6,
		ManifestCacheTTL: time.Minute,
		DefaultQuality:   Quality480p,
	}

	streamer, analytics, _ := newTestStreamerWithDeps(t, cfg)

	storage := &countingStorage{
		metadata: &MediaMetadata{
			MediaID:            "m1",
			Status:             StatusComplete,
			Duration:           12,
			AvailableQualities: []Quality{Quality360p, Quality720p, Quality1080p, Quality4K},
		},
	}
	streamer.storage = storage

	qualities, err := streamer.GetAvailableQualities("m1")
	require.NoError(t, err)
	assert.Len(t, qualities, 4)

	analytics.On("GetStreamingPreferences", mock.Anything, "u1").Return(&Preferences{
		Username:         "u1",
		AutoQuality:      true,
		DataSaverMode:    true,
		MaxBandwidthMbps: 5, // 5 Mbps
	}, nil)

	filtered, err := streamer.GetAvailableQualitiesForUser("m1", "u1")
	require.NoError(t, err)
	assert.NotEmpty(t, filtered)
	for _, q := range filtered {
		assert.NotEqual(t, Quality4K, q.Quality)
		assert.NotEqual(t, Quality1080p, q.Quality)
	}
}

func TestStreamer_SessionLifecycle_StartUpdateEnd(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:       "https://cdn.example.com",
		S3Bucket:         "test-bucket",
		S3Region:         "us-east-1",
		SegmentDuration:  6,
		ManifestCacheTTL: time.Minute,
		DefaultQuality:   Quality480p,
	}

	streamer, _, _ := newTestStreamerWithDeps(t, cfg)

	repo := newInMemoryMediaSessionRepo()
	streamer.SetSessionManager(NewSessionManager(repo, zap.NewNop(), nil))

	session, err := streamer.StartSessionWithPreferences("u1", "m1", FormatHLS, "", &Preferences{
		Username:       "u1",
		AutoQuality:    false,
		DefaultQuality: string(Quality720p),
	})
	require.NoError(t, err)
	assert.Equal(t, Quality720p, session.CurrentQuality)

	// Exercise wrappers
	require.NoError(t, streamer.TrackBandwidth("u1", 123))
	stats, err := streamer.GetBandwidthStats("u1")
	require.NoError(t, err)
	assert.Equal(t, "u1", stats.UserID)

	// Selection with session-aware metrics path (uses SelectQualityWithSession).
	selected := streamer.GetOptimalQualityWithPreferences("u1", 5000, "", &Preferences{
		Username:    "u1",
		AutoQuality: true,
	})
	assert.NotEmpty(t, selected)

	// Update (quality switch + bandwidth + metrics)
	require.NoError(t, streamer.UpdateSession(session.SessionID, Quality1080p, 0, 1234))

	got, err := streamer.GetSession(session.SessionID)
	require.NoError(t, err)
	assert.Equal(t, Quality1080p, got.CurrentQuality)
	assert.Equal(t, int64(1234), got.BytesTransferred)
	assert.GreaterOrEqual(t, got.BufferHealth, 0.0)
	assert.LessOrEqual(t, got.BufferHealth, 1.0)

	require.NoError(t, streamer.EndSession(session.SessionID))
	_, err = streamer.GetSession(session.SessionID)
	require.Error(t, err)

	// Error when session manager isn't set
	streamer.SetSessionManager(nil)
	_, err = streamer.StartSession("u1", "m1", FormatHLS)
	require.NoError(t, err) // no persistence path
	_, err = streamer.GetSession("s-does-not-exist")
	require.Error(t, err)
	assert.Error(t, streamer.UpdateSession("missing", Quality720p, 0, 1))
	assert.Error(t, streamer.EndSession("missing"))
}

func TestStreamer_recordManifestGeneration(t *testing.T) {
	cfg := &StreamingConfig{
		CDNBaseURL:       "https://cdn.example.com",
		S3Bucket:         "test-bucket",
		S3Region:         "us-east-1",
		SegmentDuration:  6,
		ManifestCacheTTL: time.Minute,
		DefaultQuality:   Quality480p,
	}

	streamer, _, db := newTestStreamerWithDeps(t, cfg)

	db.forceCreateErr = errors.New("create boom")
	err := streamer.recordManifestGeneration("m1", "hls", 12.3)
	require.Error(t, err)
}
