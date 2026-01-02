package media

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	streamingpkg "github.com/equaltoai/lesser/pkg/media/streaming"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	storagemocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeMediaStorage struct {
	metadata map[string]*streamingpkg.MediaMetadata
	err      error
}

func (f *fakeMediaStorage) GetManifestPath(string, streamingpkg.MediaFormat, streamingpkg.Quality) string {
	return ""
}
func (f *fakeMediaStorage) GetSegmentPath(string, streamingpkg.Quality, int) string { return "" }
func (f *fakeMediaStorage) ManifestExists(string, streamingpkg.MediaFormat) (bool, error) {
	return false, nil
}
func (f *fakeMediaStorage) GetKeyframeData(string, streamingpkg.Quality) ([]byte, error) {
	return nil, nil
}
func (f *fakeMediaStorage) GetMediaMetadata(mediaID string) (*streamingpkg.MediaMetadata, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.metadata == nil {
		return nil, errors.New("not found")
	}
	if md, ok := f.metadata[mediaID]; ok {
		return md, nil
	}
	return nil, errors.New("not found")
}

type fakeStreamingCloudWatch struct {
	mu sync.Mutex

	statsByMetric map[string]*cloudwatch.GetMetricStatisticsOutput
	errByMetric   map[string]error
	putErr        error

	statsCalls []*cloudwatch.GetMetricStatisticsInput
	putCalls   []*cloudwatch.PutMetricDataInput
}

func (f *fakeStreamingCloudWatch) GetMetricStatistics(_ context.Context, input *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	metric := ""
	if input != nil && input.MetricName != nil {
		metric = *input.MetricName
	}

	f.mu.Lock()
	f.statsCalls = append(f.statsCalls, input)
	out := f.statsByMetric[metric]
	err := f.errByMetric[metric]
	f.mu.Unlock()

	if out == nil {
		out = &cloudwatch.GetMetricStatisticsOutput{}
	}
	return out, err
}

func (f *fakeStreamingCloudWatch) PutMetricData(_ context.Context, input *cloudwatch.PutMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	f.mu.Lock()
	f.putCalls = append(f.putCalls, input)
	err := f.putErr
	f.mu.Unlock()

	if err != nil {
		return nil, err
	}
	return &cloudwatch.PutMetricDataOutput{}, nil
}

func TestStreamingService_GenerateStreamingURL_DefaultsAndSigns(t *testing.T) {
	svc := &streamingService{
		distributionDomain: "cdn.example.com",
		keyPairID:          "K1",
		privateKey:         []byte("secret"),
	}

	out, err := svc.GenerateStreamingURL("m1", "auto")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, Resolution720p, out.Quality)
	assert.Equal(t, ProtocolHLS, out.Protocol)

	parsed, err := url.Parse(out.URL)
	require.NoError(t, err)
	assert.Contains(t, parsed.Path, "/media/m1/720p/index.m3u8")
	q := parsed.Query()
	assert.NotEmpty(t, q.Get("Expires"))
	assert.NotEmpty(t, q.Get("Signature"))
	assert.Equal(t, "K1", q.Get("Key-Pair-Id"))

	out, err = svc.GenerateStreamingURL("m1", "dash")
	require.NoError(t, err)
	assert.Equal(t, "DASH", out.Protocol)
}

func TestStreamingService_signURL_ErrorsOnInvalidURL(t *testing.T) {
	svc := &streamingService{
		keyPairID:  "K1",
		privateKey: []byte("secret"),
	}

	_, err := svc.signURL("://bad", time.Now())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidURL)
}

func TestStreamingService_GenerateManifests_UseMetadataDuration(t *testing.T) {
	storage := &fakeMediaStorage{
		metadata: map[string]*streamingpkg.MediaMetadata{
			"m1": {Duration: 123.45},
		},
	}
	svc := &streamingService{
		distributionDomain: "cdn.example.com",
		mediaStorage:       storage,
	}

	hls, err := svc.GenerateHLSManifest("m1")
	require.NoError(t, err)
	require.NotNil(t, hls)
	assert.Equal(t, 123.45, hls.Duration)
	require.Len(t, hls.Variants, 4)

	dash, err := svc.GenerateDASHManifest("m1")
	require.NoError(t, err)
	require.NotNil(t, dash)
	assert.Equal(t, 123.45, dash.Duration)
	require.Len(t, dash.VideoTracks, 3)
	require.Len(t, dash.AudioTracks, 2)

	svc.mediaStorage = &fakeMediaStorage{err: errors.New("boom")}
	hls, err = svc.GenerateHLSManifest("m1")
	require.NoError(t, err)
	assert.Equal(t, 0.0, hls.Duration)
}

func TestStreamingService_GetStreamingAnalytics_UsesCloudWatchResults(t *testing.T) {
	now := time.Now()
	fcw := &fakeStreamingCloudWatch{
		statsByMetric: map[string]*cloudwatch.GetMetricStatisticsOutput{
			"StreamingEvent":     {Datapoints: []cwtypes.Datapoint{{Timestamp: &now, Sum: float64Ptr(20)}}},
			"RebufferEvents":     {Datapoints: []cwtypes.Datapoint{{Timestamp: &now, Sum: float64Ptr(2)}}},
			"SessionDuration":    {Datapoints: []cwtypes.Datapoint{{Timestamp: &now, Average: float64Ptr(30)}}},
			"BytesTransferred":   {Datapoints: []cwtypes.Datapoint{{Timestamp: &now, Sum: float64Ptr(1234)}}},
			"QualitySwitches":    {Datapoints: []cwtypes.Datapoint{{Timestamp: &now, Sum: float64Ptr(1)}}},
			"unrequested-metric": {Datapoints: []cwtypes.Datapoint{{Timestamp: &now, Sum: float64Ptr(0)}}},
		},
		errByMetric: map[string]error{
			"QualitySwitches": errors.New("boom"),
		},
	}

	svc := &streamingService{
		cloudWatch: fcw,
	}

	analytics, err := svc.GetStreamingAnalytics("m1")
	require.NoError(t, err)
	require.NotNil(t, analytics)
	assert.Equal(t, "m1", analytics.MediaID)
	assert.Equal(t, int64(20), analytics.ViewCount)
	assert.Equal(t, int64(1234), analytics.BandwidthUsed)
	assert.Equal(t, int64(2), analytics.BufferingEvents)
	assert.Equal(t, float64(30), analytics.AverageWatchTime)

	assert.Equal(t, int64(8), analytics.QualityBreakdown[Resolution720p])
	assert.Equal(t, int64(12), analytics.GeographicData["US"])
	assert.Equal(t, int64(0), analytics.PeakConcurrent)
}

func TestStreamingService_RecordStreamingEvent_SendsMetrics(t *testing.T) {
	now := time.Now()
	fcw := &fakeStreamingCloudWatch{}
	svc := &streamingService{cloudWatch: fcw}

	require.NoError(t, svc.RecordStreamingEvent(context.Background(), &StreamingEvent{
		MediaID:     "m1",
		EventType:   "play",
		Quality:     Resolution720p,
		Timestamp:   now,
		BytesLoaded: 100,
	}))

	fcw.mu.Lock()
	defer fcw.mu.Unlock()
	require.Len(t, fcw.putCalls, 1)
	require.Len(t, fcw.putCalls[0].MetricData, 2)
}

func TestStreamingHelpers(t *testing.T) {
	assert.Equal(t, int64(3), sumValues([]float64{1, 2}))
	assert.Equal(t, float64(0), averageValues([]float64{}))
	assert.Equal(t, float64(2), averageValues([]float64{1, 3}))

	assert.Empty(t, extractMetricValues(nil))
	assert.Empty(t, extractMetricValues(&cloudwatch.GetMetricStatisticsOutput{}))
}

func float64Ptr(v float64) *float64 { return &v }

func TestStreamingService_RecordStreamingEvent_ReturnsErrorWhenCloudWatchFails(t *testing.T) {
	fcw := &fakeStreamingCloudWatch{putErr: errors.New("cw down")}
	svc := &streamingService{cloudWatch: fcw}

	err := svc.RecordStreamingEvent(context.Background(), &StreamingEvent{
		MediaID:   "m1",
		EventType: "play",
		Quality:   Resolution720p,
		Timestamp: time.Now(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRecordStreamingEvent)
}

func TestStreamingService_ConstructorsAndStorage(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	t.Run("NewStreamingService", func(t *testing.T) {
		svc, err := NewStreamingService(context.Background(), "cdn.example.com", "K1", []byte("secret"), &fakeMediaStorage{})
		require.NoError(t, err)
		require.NotNil(t, svc)
	})

	t.Run("NewStreamingServiceWithStorage and SetStorage", func(t *testing.T) {
		mockStorage := new(storagemocks.MockRepositoryStorage)
		mockStorage.On("GetLogger").Return(zap.NewNop())
		mockStorage.On("StreamingCloudWatch").Return((*repositories.StreamingCloudWatchRepository)(nil))

		svc, err := NewStreamingServiceWithStorage(context.Background(), "cdn.example.com", "K1", []byte("secret"), &fakeMediaStorage{}, mockStorage)
		require.NoError(t, err)

		impl, ok := svc.(*streamingService)
		require.True(t, ok)
		require.NotNil(t, impl.cloudWatchEnhanced)
		assert.Same(t, mockStorage, impl.storage)

		impl.SetStorage(nil)
		assert.Nil(t, impl.storage)

		impl.SetStorage(mockStorage)
		require.NotNil(t, impl.cloudWatchEnhanced)

		mockStorage.AssertExpectations(t)
	})
}

func TestStreamingService_GenerateStreamingURL_CoversQualityCases(t *testing.T) {
	svc := &streamingService{
		distributionDomain: "cdn.example.com",
		keyPairID:          "K1",
		privateKey:         []byte("secret"),
	}

	tests := []struct {
		quality   string
		wantPath  string
		wantProto string
		wantQual  string
	}{
		{quality: "4k", wantPath: "/media/m1/4k/index.m3u8", wantProto: ProtocolHLS, wantQual: "4k"},
		{quality: "2160p", wantPath: "/media/m1/4k/index.m3u8", wantProto: ProtocolHLS, wantQual: "2160p"},
		{quality: Resolution1080p, wantPath: "/media/m1/1080p/index.m3u8", wantProto: ProtocolHLS, wantQual: Resolution1080p},
		{quality: Resolution720p, wantPath: "/media/m1/720p/index.m3u8", wantProto: ProtocolHLS, wantQual: Resolution720p},
		{quality: Resolution480p, wantPath: "/media/m1/480p/index.m3u8", wantProto: ProtocolHLS, wantQual: Resolution480p},
		{quality: "adaptive", wantPath: "/media/m1/master.m3u8", wantProto: ProtocolHLS, wantQual: "adaptive"},
		{quality: "dash", wantPath: "/media/m1/manifest.mpd", wantProto: "DASH", wantQual: "dash"},
		{quality: "unknown", wantPath: "/media/m1/720p/index.m3u8", wantProto: ProtocolHLS, wantQual: Resolution720p},
	}

	for _, tt := range tests {
		t.Run(tt.quality, func(t *testing.T) {
			out, err := svc.GenerateStreamingURL("m1", tt.quality)
			require.NoError(t, err)
			parsed, err := url.Parse(out.URL)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, parsed.Path)
			assert.Equal(t, tt.wantProto, out.Protocol)
			assert.Equal(t, tt.wantQual, out.Quality)
		})
	}

	// Cover "auto" with CloudWatch enhancement.
	repo := &fakeStreamingCloudWatchRepo{
		quality: &models.StreamingCloudWatchMetrics{
			CacheExpiry: time.Now().Add(time.Hour),
			GeographicMetrics: map[string]models.GeographicMetric{
				"US": {PreferredQuality: Resolution1080p},
			},
			QualityMetrics: map[string]models.QualityMetric{
				Resolution1080p: {BufferingRate: 0.01, AverageLatencyMs: 100, ViewerPercentage: 1.0},
			},
		},
	}
	enhanced := &CloudWatchEnhancedStreamingService{
		cloudWatch: &fakeEnhancedCloudWatch{},
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}
	svc.cloudWatchEnhanced = enhanced

	out, err := svc.GenerateStreamingURL("m1", "auto")
	require.NoError(t, err)
	assert.Equal(t, Resolution1080p, out.Quality)
}

func TestStreamingService_calculateAnalytics_UsesEnhancedData(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{
		quality: &models.StreamingCloudWatchMetrics{
			CacheExpiry: time.Now().Add(time.Hour),
			QualityMetrics: map[string]models.QualityMetric{
				Resolution720p: {ViewerCount: 12},
			},
		},
		geo: &models.StreamingCloudWatchMetrics{
			CacheExpiry: time.Now().Add(time.Hour),
			GeographicMetrics: map[string]models.GeographicMetric{
				"US": {ViewerCount: 7},
			},
		},
		concurrent: &models.StreamingCloudWatchMetrics{
			CacheExpiry: time.Now().Add(time.Hour),
			ConcurrentViewers: models.ConcurrentViewerMetrics{
				PeakViewers: 3,
			},
		},
	}
	enhanced := &CloudWatchEnhancedStreamingService{
		cloudWatch: &fakeEnhancedCloudWatch{},
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	svc := &streamingService{cloudWatchEnhanced: enhanced}
	analytics := svc.calculateAnalytics(map[string]*metricResult{
		"SessionCount":       {name: "SessionCount", values: []float64{12}},
		"RebufferEvents":     {name: "RebufferEvents", values: []float64{1}},
		"AvgSessionDuration": {name: "AvgSessionDuration", values: []float64{10}},
		"BytesTransferred":   {name: "BytesTransferred", values: []float64{100}},
	})

	require.NotNil(t, analytics)
	assert.Equal(t, int64(12), analytics.ViewCount)
	assert.Equal(t, int64(12), analytics.QualityBreakdown[Resolution720p])
	assert.Equal(t, int64(7), analytics.GeographicData["US"])
	assert.Equal(t, int64(3), analytics.PeakConcurrent)
}
