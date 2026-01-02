package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeS3Client struct {
	mu sync.Mutex

	keys []string

	objects map[string]string

	listErr error
	getErr  map[string]error
}

func (f *fakeS3Client) ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.listErr != nil {
		return nil, f.listErr
	}

	contents := make([]s3types.Object, 0, len(f.keys))
	for _, key := range f.keys {
		keyCopy := key
		contents = append(contents, s3types.Object{Key: &keyCopy})
	}

	return &s3.ListObjectsV2Output{Contents: contents}, nil
}

func (f *fakeS3Client) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := aws.ToString(input.Key)
	if err := f.getErr[key]; err != nil {
		return nil, err
	}

	if f.objects == nil {
		f.objects = make(map[string]string)
	}

	body, ok := f.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}

	return &s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(body)),
	}, nil
}

type fakeCloudWatchClient struct {
	mu sync.Mutex

	putCalls []*cloudwatch.PutMetricDataInput
	putErr   error
}

func (f *fakeCloudWatchClient) GetMetricStatistics(context.Context, *cloudwatch.GetMetricStatisticsInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeCloudWatchClient) PutMetricData(_ context.Context, input *cloudwatch.PutMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.putCalls = append(f.putCalls, input)
	if f.putErr != nil {
		return nil, f.putErr
	}
	return &cloudwatch.PutMetricDataOutput{}, nil
}

type fakeBandwidthStorage struct {
	mu sync.Mutex

	stored []*BandwidthUsage
	usage  []*BandwidthUsage

	storeErr error
	usageErr error
}

func (f *fakeBandwidthStorage) StoreBandwidthUsage(_ context.Context, usage *BandwidthUsage) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.storeErr != nil {
		return f.storeErr
	}
	f.stored = append(f.stored, usage)
	return nil
}

func (f *fakeBandwidthStorage) GetBandwidthUsage(_ context.Context, _, _ time.Time) ([]*BandwidthUsage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.usageErr != nil {
		return nil, f.usageErr
	}
	return f.usage, nil
}

func TestBandwidthAnalytics_ProcessLogFiles_ParsesAndStores(t *testing.T) {
	ctx := context.Background()

	storage := &fakeBandwidthStorage{}
	cw := &fakeCloudWatchClient{}
	s3c := &fakeS3Client{
		keys: []string{"logs/one.tsv", "logs/two.tsv"},
		objects: map[string]string{
			"logs/one.tsv": strings.Join([]string{
				"#Version: 1.0",
				"",
				"2025-01-01\t12:34:56\tIAD53-P1\t1024\t1.2.3.4\tGET\texample.com\t/media/m1/720p/index.m3u8\t200\t-\tua\t-",
				"2025-01-01\t12:34:56\tFRA56-P1\t2048\t1.2.3.4\tGET\texample.com\t/media/m2/4k/index.m3u8\t200\t-\tua\t-",
				"invalid\trow",
				"2025-01-01\t12:34:56\tIAD53-P1\t1024\t1.2.3.4\tGET\texample.com\t/notmedia/skip\t200\t-\tua\t-",
			}, "\n"),
		},
		getErr: map[string]error{
			"logs/two.tsv": errors.New("boom"),
		},
	}

	b := &bandwidthAnalytics{
		s3Client:       s3c,
		cloudWatch:     cw,
		storageService: storage,
	}

	require.NoError(t, b.ProcessLogFiles(ctx, "bucket", "logs/"))

	storage.mu.Lock()
	defer storage.mu.Unlock()
	require.Len(t, storage.stored, 2)
	assert.Equal(t, "m1", storage.stored[0].MediaID)
	assert.Equal(t, Resolution720p, storage.stored[0].Quality)
	assert.Equal(t, "us-east-1", storage.stored[0].Region)
	assert.Equal(t, int64(1024), storage.stored[0].Bytes)

	cw.mu.Lock()
	defer cw.mu.Unlock()
	require.NotEmpty(t, cw.putCalls)
	assert.Equal(t, "Lesser/Bandwidth", aws.ToString(cw.putCalls[0].Namespace))
}

func TestBandwidthAnalytics_sendMetrics_BatchesAtTwenty(t *testing.T) {
	cw := &fakeCloudWatchClient{}
	b := &bandwidthAnalytics{
		s3Client:       &fakeS3Client{},
		cloudWatch:     cw,
		storageService: &fakeBandwidthStorage{},
	}

	records := make([]*BandwidthUsage, 0, 30)
	for i := 0; i < 15; i++ {
		records = append(records, &BandwidthUsage{
			MediaID: "m",
			Bytes:   1,
			Quality: fmt.Sprintf("q%d", i),
			Region:  fmt.Sprintf("r%d", i),
		})
	}

	require.NoError(t, b.sendMetrics(context.Background(), records))

	cw.mu.Lock()
	defer cw.mu.Unlock()
	require.Len(t, cw.putCalls, 2)
	assert.Len(t, cw.putCalls[0].MetricData, 20)
	assert.Len(t, cw.putCalls[1].MetricData, 11)
}

func TestBandwidthAnalytics_GetBandwidthReport_Aggregates(t *testing.T) {
	storage := &fakeBandwidthStorage{
		usage: []*BandwidthUsage{
			{MediaID: "m1", UserID: "u1", Bytes: 100, Quality: Resolution720p, Region: "US"},
			{MediaID: "m1", UserID: "u1", Bytes: 50, Quality: Resolution720p, Region: "US"},
			{MediaID: "m2", UserID: "u2", Bytes: 200, Quality: "4k", Region: "EU"},
		},
	}
	b := &bandwidthAnalytics{
		s3Client:       &fakeS3Client{},
		cloudWatch:     &fakeCloudWatchClient{},
		storageService: storage,
	}

	report, err := b.GetBandwidthReport(context.Background(), "day", time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, int64(350), report.TotalBytes)
	assert.Equal(t, int64(2), report.ByMedia["m1"].Requests)
	assert.Equal(t, int64(150), report.ByMedia["m1"].Bytes)
	assert.Equal(t, int64(1), report.ByMedia["m2"].Requests)
	assert.Equal(t, int64(200), report.ByMedia["m2"].Bytes)
	assert.Equal(t, int64(150), report.ByUser["u1"])
	assert.Equal(t, int64(200), report.ByUser["u2"])
}

func TestBandwidthAnalytics_GetCostBreakdown_GeneratesRecommendations(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	end := time.Now()

	gb := int64(1024 * 1024 * 1024)
	storage := &fakeBandwidthStorage{
		usage: []*BandwidthUsage{
			{MediaID: "m-high", UserID: "u1", Bytes: 200 * gb, Quality: "4k", Region: "US"},
			{MediaID: "m-low", UserID: "u1", Bytes: 10 * gb, Quality: Resolution720p, Region: "US"},
		},
	}
	b := &bandwidthAnalytics{
		s3Client:       &fakeS3Client{},
		cloudWatch:     &fakeCloudWatchClient{},
		storageService: storage,
	}

	breakdown, err := b.GetCostBreakdown(context.Background(), start, end)
	require.NoError(t, err)
	require.NotNil(t, breakdown)
	assert.Greater(t, breakdown.TotalCost, 10.0)
	require.GreaterOrEqual(t, len(breakdown.Recommendations), 2)
}

func TestBandwidthAnalytics_TrackBandwidthUsage_StoresAndSendsMetric(t *testing.T) {
	ctx := context.Background()

	t.Run("storage error", func(t *testing.T) {
		b := &bandwidthAnalytics{
			s3Client:       &fakeS3Client{},
			cloudWatch:     &fakeCloudWatchClient{},
			storageService: &fakeBandwidthStorage{storeErr: errors.New("nope")},
		}
		err := b.TrackBandwidthUsage(ctx, &BandwidthUsage{MediaID: "m", Quality: "q"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrBandwidthUsageStorage)
	})

	t.Run("cloudwatch error", func(t *testing.T) {
		cw := &fakeCloudWatchClient{putErr: errors.New("cw down")}
		b := &bandwidthAnalytics{
			s3Client:       &fakeS3Client{},
			cloudWatch:     cw,
			storageService: &fakeBandwidthStorage{},
		}
		err := b.TrackBandwidthUsage(ctx, &BandwidthUsage{MediaID: "m", Quality: "q", Timestamp: time.Now(), Bytes: 10})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRealtimeMetricSubmission)
	})

	t.Run("success", func(t *testing.T) {
		cw := &fakeCloudWatchClient{}
		st := &fakeBandwidthStorage{}
		b := &bandwidthAnalytics{
			s3Client:       &fakeS3Client{},
			cloudWatch:     cw,
			storageService: st,
		}
		err := b.TrackBandwidthUsage(ctx, &BandwidthUsage{MediaID: "m", Quality: "q", Timestamp: time.Now(), Bytes: 10})
		require.NoError(t, err)

		st.mu.Lock()
		defer st.mu.Unlock()
		require.Len(t, st.stored, 1)

		cw.mu.Lock()
		defer cw.mu.Unlock()
		require.Len(t, cw.putCalls, 1)
		require.Len(t, cw.putCalls[0].MetricData, 1)
		assert.Equal(t, "RealtimeBandwidth", aws.ToString(cw.putCalls[0].MetricData[0].MetricName))
		assert.Equal(t, cwtypes.StandardUnitBytes, cw.putCalls[0].MetricData[0].Unit)
	})
}
