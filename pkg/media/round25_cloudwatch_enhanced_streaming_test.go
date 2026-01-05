package media

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeStreamingCloudWatchRepo struct {
	mu sync.Mutex

	quality    *models.StreamingCloudWatchMetrics
	geo        *models.StreamingCloudWatchMetrics
	concurrent *models.StreamingCloudWatchMetrics

	getQualityErr    error
	getGeoErr        error
	getConcurrentErr error

	cachedQualityCalls   int
	cachedGeographicCall int
	cachedConcurrentCall int

	lastQualityCache    map[string]models.QualityMetric
	lastGeographicCache map[string]models.GeographicMetric
	lastConcurrentCache models.ConcurrentViewerMetrics

	cacheQualityErr    error
	cacheGeographicErr error
	cacheConcurrentErr error
}

func (f *fakeStreamingCloudWatchRepo) GetQualityBreakdown(context.Context, string) (*models.StreamingCloudWatchMetrics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.quality, f.getQualityErr
}

func (f *fakeStreamingCloudWatchRepo) CacheQualityBreakdown(_ context.Context, _ string, qualityMetrics map[string]models.QualityMetric) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cachedQualityCalls++
	f.lastQualityCache = qualityMetrics
	return f.cacheQualityErr
}

func (f *fakeStreamingCloudWatchRepo) GetGeographicData(context.Context, string) (*models.StreamingCloudWatchMetrics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.geo, f.getGeoErr
}

func (f *fakeStreamingCloudWatchRepo) CacheGeographicData(_ context.Context, _ string, geoMetrics map[string]models.GeographicMetric) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cachedGeographicCall++
	f.lastGeographicCache = geoMetrics
	return f.cacheGeographicErr
}

func (f *fakeStreamingCloudWatchRepo) GetConcurrentViewers(context.Context, string) (*models.StreamingCloudWatchMetrics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.concurrent, f.getConcurrentErr
}

func (f *fakeStreamingCloudWatchRepo) CacheConcurrentViewers(_ context.Context, _ string, metrics models.ConcurrentViewerMetrics) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cachedConcurrentCall++
	f.lastConcurrentCache = metrics
	return f.cacheConcurrentErr
}

type fakeEnhancedCloudWatch struct {
	mu sync.Mutex

	getCalls int
	values   map[string]float64
	errs     map[string]error
}

func (f *fakeEnhancedCloudWatch) GetMetricStatistics(_ context.Context, input *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++

	metric := aws.ToString(input.MetricName)
	if err := f.errs[metric]; err != nil {
		return nil, err
	}

	value := f.values[metric]
	now := time.Now()

	stat := cwtypes.StatisticSum
	if input != nil && len(input.Statistics) > 0 {
		stat = input.Statistics[0]
	}

	dp := cwtypes.Datapoint{Timestamp: &now}
	switch stat {
	case cwtypes.StatisticSum:
		dp.Sum = &value
	case cwtypes.StatisticAverage:
		dp.Average = &value
	case cwtypes.StatisticMaximum:
		dp.Maximum = &value
	}

	return &cloudwatch.GetMetricStatisticsOutput{Datapoints: []cwtypes.Datapoint{dp}}, nil
}

func (f *fakeEnhancedCloudWatch) PutMetricData(context.Context, *cloudwatch.PutMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	return &cloudwatch.PutMetricDataOutput{}, nil
}

func TestCloudWatchEnhancedStreamingService_GetRealQualityBreakdown_UsesCacheWhenFresh(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{
		quality: &models.StreamingCloudWatchMetrics{
			CacheExpiry: time.Now().Add(time.Hour),
			QualityMetrics: map[string]models.QualityMetric{
				Resolution720p:  {ViewerCount: 10},
				Resolution1080p: {ViewerCount: 5},
			},
		},
	}

	cw := &fakeEnhancedCloudWatch{}
	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: cw,
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	out, err := svc.GetRealQualityBreakdown(context.Background(), "m1", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(10), out[Resolution720p])
	assert.Equal(t, int64(5), out[Resolution1080p])

	cw.mu.Lock()
	defer cw.mu.Unlock()
	assert.Equal(t, 0, cw.getCalls)
}

func TestCloudWatchEnhancedStreamingService_GetRealQualityBreakdown_FetchesAndCachesOnMiss(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{}
	cw := &fakeEnhancedCloudWatch{
		values: map[string]float64{
			"StreamingViewers": 100,
		},
	}
	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: cw,
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	out, err := svc.GetRealQualityBreakdown(context.Background(), "m1", 100)
	require.NoError(t, err)
	require.NotEmpty(t, out)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.GreaterOrEqual(t, repo.cachedQualityCalls, 1)
	assert.NotEmpty(t, repo.lastQualityCache)
}

func TestCloudWatchEnhancedStreamingService_GetRealGeographicData_FallsBackWhenCloudWatchFails(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{}
	cw := &fakeEnhancedCloudWatch{
		errs: map[string]error{
			"StreamingViewersByRegion": errors.New("boom"),
		},
	}
	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: cw,
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	out, err := svc.GetRealGeographicData(context.Background(), "m1", 240)
	require.NoError(t, err)
	assert.Equal(t, int64(144), out["US"])
	assert.Equal(t, int64(60), out["EU"])
	assert.Equal(t, int64(36), out["AS"])
}

func TestCloudWatchEnhancedStreamingService_getMetricValue_SelectsLatestAndStatistic(t *testing.T) {
	now := time.Now()
	old := now.Add(-time.Hour)

	svc := &CloudWatchEnhancedStreamingService{
		repo:      &fakeStreamingCloudWatchRepo{},
		logger:    zap.NewNop(),
		namespace: "Lesser/Streaming",
	}

	// Override cloudWatch with a stub that returns two datapoints.
	svc.cloudWatch = &fakeStreamingCloudWatch{
		statsByMetric: map[string]*cloudwatch.GetMetricStatisticsOutput{
			"PeakViewers": {
				Datapoints: []cwtypes.Datapoint{
					{Timestamp: &old, Maximum: float64Ptr(1)},
					{Timestamp: &now, Maximum: float64Ptr(5)},
				},
			},
		},
	}

	val, err := svc.getMetricValue(context.Background(), "PeakViewers", "m1", "", old, now, cwtypes.StatisticMaximum)
	require.NoError(t, err)
	assert.Equal(t, float64(5), val)
}

func TestCloudWatchEnhancedStreamingService_GetOptimalQuality_UsesCachedMetrics(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{
		quality: &models.StreamingCloudWatchMetrics{
			CacheExpiry: time.Now().Add(time.Hour),
			QualityMetrics: map[string]models.QualityMetric{
				Resolution720p:  {BufferingRate: 0.01, AverageLatencyMs: 100, ViewerPercentage: 0.5},
				Resolution1080p: {BufferingRate: 0.20, AverageLatencyMs: 900, ViewerPercentage: 0.5},
			},
			GeographicMetrics: map[string]models.GeographicMetric{
				"US": {PreferredQuality: Resolution1080p},
			},
		},
		geo: &models.StreamingCloudWatchMetrics{
			CacheExpiry: time.Now().Add(time.Hour),
		},
	}
	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: &fakeEnhancedCloudWatch{},
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	quality, err := svc.GetOptimalQuality(context.Background(), "m1", "US")
	require.NoError(t, err)
	assert.Equal(t, Resolution1080p, quality)

	repo.mu.Lock()
	repo.quality = nil
	repo.mu.Unlock()
	quality, err = svc.GetOptimalQuality(context.Background(), "m1", "US")
	require.NoError(t, err)
	assert.Equal(t, Resolution720p, quality)
}

func TestCloudWatchEnhancedStreamingService_GetRealConcurrentMetrics_UsesCacheAndCachesFetch(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{
		concurrent: &models.StreamingCloudWatchMetrics{
			CacheExpiry: time.Now().Add(time.Hour),
			ConcurrentViewers: models.ConcurrentViewerMetrics{
				PeakViewers: 9,
			},
		},
	}

	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: &fakeEnhancedCloudWatch{},
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	peak, err := svc.GetRealConcurrentMetrics(context.Background(), "m1", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(9), peak)

	repo.mu.Lock()
	repo.concurrent = &models.StreamingCloudWatchMetrics{CacheExpiry: time.Now().Add(-time.Minute)}
	repo.mu.Unlock()

	cw := &fakeEnhancedCloudWatch{
		values: map[string]float64{
			"CurrentViewers":  10,
			"PeakViewers":     15,
			"SessionDuration": 12,
		},
	}
	svc.cloudWatch = cw

	peak, err = svc.GetRealConcurrentMetrics(context.Background(), "m1", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(15), peak)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.GreaterOrEqual(t, repo.cachedConcurrentCall, 1)
}

func TestCloudWatchEnhancedStreamingService_GetRealConcurrentMetrics_WarnsOnRepoErrors(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{
		getConcurrentErr:   errors.New("read failed"),
		cacheConcurrentErr: errors.New("cache failed"),
	}
	cw := &fakeEnhancedCloudWatch{
		values: map[string]float64{
			"CurrentViewers":  10,
			"PeakViewers":     15,
			"SessionDuration": 12,
		},
	}
	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: cw,
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	peak, err := svc.GetRealConcurrentMetrics(context.Background(), "m1", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(15), peak)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.GreaterOrEqual(t, repo.cachedConcurrentCall, 1)
}

func TestNewCloudWatchEnhancedStreamingService_SetsDefaults(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{}
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	}
	svc := NewCloudWatchEnhancedStreamingService(cfg, repo, zap.NewNop())
	require.NotNil(t, svc)
	assert.Equal(t, "Lesser/Streaming", svc.namespace)
	require.NotNil(t, svc.cloudWatch)
	require.NotNil(t, svc.repo)
}

func TestCloudWatchEnhancedStreamingService_GetRealGeographicData_FetchesAndCachesOnMiss(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{}
	cw := &fakeEnhancedCloudWatch{
		values: map[string]float64{
			"StreamingViewersByRegion": 10,
		},
		errs: map[string]error{
			"RegionalLatency": errors.New("fail"),
			"CacheHitRate":    errors.New("fail"),
			"BandwidthUsage":  errors.New("fail"),
		},
	}
	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: cw,
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	out, err := svc.GetRealGeographicData(context.Background(), "m1", 100)
	require.NoError(t, err)
	require.NotEmpty(t, out)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.GreaterOrEqual(t, repo.cachedGeographicCall, 1)
	assert.NotEmpty(t, repo.lastGeographicCache)
}

func TestCloudWatchEnhancedStreamingService_fetchSingleQualityMetrics_UsesDefaultsOnErrors(t *testing.T) {
	repo := &fakeStreamingCloudWatchRepo{}
	cw := &fakeEnhancedCloudWatch{
		errs: map[string]error{
			"StreamingViewers": errors.New("boom"),
			"BufferingEvents":  errors.New("boom"),
			"StreamingLatency": errors.New("boom"),
			"StreamingErrors":  errors.New("boom"),
		},
	}
	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: cw,
		repo:       repo,
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	metrics, err := svc.fetchSingleQualityMetrics(context.Background(), "m1", Resolution720p, time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	require.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.ViewerCount)
	assert.Equal(t, float64(0), metrics.BufferingRate)
	assert.Equal(t, int64(500), metrics.AverageLatencyMs)
	assert.Equal(t, int64(1000), metrics.StartupTimeMs)
	assert.Equal(t, float64(0.0001), metrics.ErrorRate)
}

func TestCloudWatchEnhancedStreamingService_getMetricsWithCaching_UnknownMetricTypeFallsThroughAndCaches(t *testing.T) {
	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: &fakeEnhancedCloudWatch{},
		repo:       &fakeStreamingCloudWatchRepo{},
		logger:     zap.NewNop(),
		namespace:  "Lesser/Streaming",
	}

	cached := &models.StreamingCloudWatchMetrics{CacheExpiry: time.Now().Add(time.Hour)}
	result, err := svc.getMetricsWithCaching(
		context.Background(),
		"m1",
		0,
		"other",
		func() (interface{}, error) { return cached, nil },
		func() (interface{}, error) {
			return map[string]models.QualityMetric{
				"q": {ViewerCount: 7},
			}, nil
		},
		func(interface{}) error { return errors.New("cache failed") },
		func() map[string]int64 { return map[string]int64{"fallback": 1} },
		func(data interface{}) map[string]int64 {
			out := make(map[string]int64)
			for k, v := range data.(map[string]models.QualityMetric) {
				out[k] = v.ViewerCount
			}
			return out
		},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(7), result["q"])
}

func TestCloudWatchEnhancedStreamingService_getMetricValueWithDimension_ErrorsOnNoDatapoints(t *testing.T) {
	svc := &CloudWatchEnhancedStreamingService{
		cloudWatch: &fakeStreamingCloudWatch{
			statsByMetric: map[string]*cloudwatch.GetMetricStatisticsOutput{
				"StreamingViewersByRegion": {Datapoints: []cwtypes.Datapoint{}},
			},
		},
		repo:      &fakeStreamingCloudWatchRepo{},
		logger:    zap.NewNop(),
		namespace: "Lesser/Streaming",
	}

	_, err := svc.getMetricValueWithDimension(context.Background(), "StreamingViewersByRegion", "m1", "Region", "US", time.Now().Add(-time.Hour), time.Now(), cwtypes.StatisticSum)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoDataPointsWithDim)
}
