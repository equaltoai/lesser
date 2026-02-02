package repositories

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubRoundTripper struct {
	fn func(*http.Request) (*http.Response, error)
}

func (s stubRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return s.fn(r) }

func cloudWatchClientForTest(t *testing.T, responder func(req *http.Request) (*http.Response, error)) *cloudwatch.Client {
	t.Helper()
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKIA...", "secret", ""),
		HTTPClient: &http.Client{
			Transport: stubRoundTripper{fn: responder},
		},
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(func(service, region string, _ ...any) (aws.Endpoint, error) {
			return aws.Endpoint{URL: "http://example.com", SigningRegion: "us-east-1"}, nil
		}),
	}
	return cloudwatch.NewFromConfig(cfg)
}

func TestCloudWatchMetricsRepository_NewRepository_ErrorPath(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	repo := NewCloudWatchMetricsRepository("ns", "env", zap.NewNop())
	require.NotNil(t, repo)
}

func TestCloudWatchMetricsRepository_GetServiceMetrics_UsesStubbedHTTP(t *testing.T) {
	// Return a generic successful GetMetricStatisticsResponse with Sum=5 and p50=123.
	client := cloudWatchClientForTest(t, func(req *http.Request) (*http.Response, error) {
		bodyBytes, _ := io.ReadAll(req.Body)
		body := string(bodyBytes)

		sum := "5"
		if strings.Contains(body, "ExtendedStatistics.member.1=p50") {
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(
					`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-01T00:00:00Z</Timestamp><ExtendedStatistics><entry><key>p50</key><value>123</value></entry></ExtendedStatistics></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
				Header: make(http.Header),
			}, nil
		}

		if strings.Contains(body, "ExtendedStatistics.member.1=p90") {
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(
					`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-02T00:00:00Z</Timestamp><ExtendedStatistics><entry><key>p90</key><value>250</value></entry></ExtendedStatistics></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
				Header: make(http.Header),
			}, nil
		}

		if strings.Contains(body, "ExtendedStatistics.member.1=p99") {
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(
					`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-03T00:00:00Z</Timestamp><ExtendedStatistics><entry><key>p99</key><value>500</value></entry></ExtendedStatistics></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
				Header: make(http.Header),
			}, nil
		}

		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(bytes.NewBufferString(
				`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-01T00:00:00Z</Timestamp><Sum>` + sum + `</Sum></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
			Header: make(http.Header),
		}, nil
	})

	repo := &CloudWatchMetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.CloudWatchMetrics](nil, "", zap.NewNop(), nil, "CloudWatchMetricsRepository", "cloudwatch"),
		client:                 client,
		namespace:              "ns",
		environment:            "prod",
	}

	metrics, err := repo.GetServiceMetrics(context.Background(), "svc", time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(5), metrics.RequestCount)
	require.Equal(t, float64(123), metrics.LatencyP50Ms)
	require.GreaterOrEqual(t, metrics.EstimatedCostUSD, 0.0)
}

func TestCloudWatchMetricsRepository_StatisticHelpers_EmptyAndAverage(t *testing.T) {
	client := cloudWatchClientForTest(t, func(_ *http.Request) (*http.Response, error) {
		// Return no datapoints
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(bytes.NewBufferString(
				`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
			Header: make(http.Header),
		}, nil
	})

	repo := &CloudWatchMetricsRepository{client: client, environment: "prod"}
	v, err := repo.getMetricSum(context.Background(), "AWS/Lambda", "Invocations", time.Now().Add(-time.Hour), time.Now(), nil)
	require.NoError(t, err)
	require.Equal(t, 0.0, v)
}

func TestCloudWatchMetricsRepository_CachingPaths(t *testing.T) {
	// Create a repository with an embedded EnhancedBaseRepository, but nil BaseRepository to hit the early return paths.
	enhanced := NewEnhancedBaseRepository[*models.CloudWatchMetrics](nil, "", zap.NewNop(), nil, "CloudWatchMetricsRepository", "cloudwatch")
	enhanced.BaseRepository = nil
	repo := &CloudWatchMetricsRepository{EnhancedBaseRepository: enhanced}

	err := repo.CacheMetrics(context.Background(), "svc", &ServiceMetrics{ServiceName: "svc"})
	require.NoError(t, err)

	_, err = repo.GetCachedMetrics(context.Background(), "svc")
	require.Error(t, err)

	// Now enable BaseRepository and mock Query paths
	mockDB, mockQuery := newMockDBQuery()
	enabled := NewEnhancedBaseRepository[*models.CloudWatchMetrics](mockDB, "tbl", zap.NewNop(), nil, "CloudWatchMetricsRepository", "cloudwatch")
	repo = &CloudWatchMetricsRepository{EnhancedBaseRepository: enabled}

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.CloudWatchMetrics); ok {
			*slice = []*models.CloudWatchMetrics{
				{ServiceName: "svc", Timestamp: time.Now().Add(-time.Minute), CacheExpiry: time.Now().Add(time.Minute)},
			}
		}
	}).Return(nil).Once()

	repo.BaseRepository = enabled.BaseRepository
	cached, err := repo.GetCachedMetrics(context.Background(), "svc")
	require.NoError(t, err)
	require.Equal(t, "svc", cached.ServiceName)

	// Expired cache
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.CloudWatchMetrics); ok {
			expired := &models.CloudWatchMetrics{ServiceName: "svc"}
			expired.CacheExpiry = time.Now().Add(-time.Hour)
			*slice = []*models.CloudWatchMetrics{expired}
		}
	}).Return(nil).Once()
	_, err = repo.GetCachedMetrics(context.Background(), "svc")
	require.Error(t, err)

	// Query error path
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetCachedMetrics(context.Background(), "svc")
	require.Error(t, err)

	// No results path
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if slice, ok := dst.(*[]*models.CloudWatchMetrics); ok {
			*slice = nil
		}
	}).Return(nil).Once()
	_, err = repo.GetCachedMetrics(context.Background(), "svc")
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCloudWatchMetricsRepository_GetCostBreakdown(t *testing.T) {
	client := cloudWatchClientForTest(t, func(_ *http.Request) (*http.Response, error) {
		// Always return Sum=100 and datapoint.
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(bytes.NewBufferString(
				`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-01T00:00:00Z</Timestamp><Sum>100</Sum></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
			Header: make(http.Header),
		}, nil
	})

	repo := &CloudWatchMetricsRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.CloudWatchMetrics](nil, "", zap.NewNop(), nil, "CloudWatchMetricsRepository", "cloudwatch"),
		client:                 client,
		environment:            "prod",
	}

	breakdown, err := repo.GetCostBreakdown(context.Background(), time.Hour)
	require.NoError(t, err)
	require.NotNil(t, breakdown)
	require.GreaterOrEqual(t, breakdown.TotalCost, 0.0)
	require.NotEmpty(t, breakdown.Breakdown)
}

func TestCloudWatchMetricsRepository_MetricStatisticCasesAndErrors(t *testing.T) {
	client := cloudWatchClientForTest(t, func(req *http.Request) (*http.Response, error) {
		bodyBytes, _ := io.ReadAll(req.Body)
		body := string(bodyBytes)
		if strings.Contains(body, "Statistics.member.1=Average") {
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(
					`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-01T00:00:00Z</Timestamp><Average>10</Average></member><member><Timestamp>2020-01-01T00:05:00Z</Timestamp><Average>20</Average></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
				Header: make(http.Header),
			}, nil
		}
		if strings.Contains(body, "Statistics.member.1=Maximum") {
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(
					`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-01T00:00:00Z</Timestamp><Maximum>5</Maximum></member><member><Timestamp>2020-01-01T00:05:00Z</Timestamp><Maximum>7</Maximum></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
				Header: make(http.Header),
			}, nil
		}
		if strings.Contains(body, "Statistics.member.1=Minimum") {
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(
					`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-01T00:00:00Z</Timestamp><Minimum>2</Minimum></member><member><Timestamp>2020-01-01T00:05:00Z</Timestamp><Minimum>1</Minimum></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
				Header: make(http.Header),
			}, nil
		}
		if strings.Contains(body, "MetricName=FailMe") {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewBufferString("boom")), Header: make(http.Header)}, nil
		}
		// Sum default
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(bytes.NewBufferString(
				`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-01T00:00:00Z</Timestamp><Sum>3</Sum></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
			Header: make(http.Header),
		}, nil
	})

	repo := &CloudWatchMetricsRepository{client: client, environment: "prod"}
	start := time.Now().Add(-time.Hour)
	end := time.Now()

	avg, err := repo.getMetricStatistic(context.Background(), "AWS/Lambda", "Invocations", "Average", start, end, nil)
	require.NoError(t, err)
	require.Equal(t, 15.0, avg)

	max, err := repo.getMetricStatistic(context.Background(), "AWS/Lambda", "Invocations", "Maximum", start, end, nil)
	require.NoError(t, err)
	require.Equal(t, 7.0, max)

	min, err := repo.getMetricStatistic(context.Background(), "AWS/Lambda", "Invocations", "Minimum", start, end, nil)
	require.NoError(t, err)
	require.Equal(t, 1.0, min)

	_, err = repo.getMetricSum(context.Background(), "AWS/Lambda", "FailMe", start, end, nil)
	require.Error(t, err)
}

func TestCloudWatchMetricsRepository_CacheMetrics_WritesWhenEnabled(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	enabled := NewEnhancedBaseRepository[*models.CloudWatchMetrics](mockDB, "tbl", zap.NewNop(), nil, "CloudWatchMetricsRepository", "cloudwatch")
	repo := &CloudWatchMetricsRepository{EnhancedBaseRepository: enabled}

	mockQuery.On("Create").Return(nil).Once()
	err := repo.CacheMetrics(context.Background(), "svc", &ServiceMetrics{ServiceName: "svc"})
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestCloudWatchMetricsRepository_WithCachingConstructorAndPercentileEdgeCases(t *testing.T) {
	responder := func(req *http.Request) (*http.Response, error) {
		bodyBytes, _ := io.ReadAll(req.Body)
		body := string(bodyBytes)
		// Empty datapoints branch
		if strings.Contains(body, "ExtendedStatistics.member.1=p90") {
			return &http.Response{
				StatusCode: 200,
				Body: io.NopCloser(bytes.NewBufferString(
					`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
				Header: make(http.Header),
			}, nil
		}
		// Missing key branch (extended stats present but no requested key)
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(bytes.NewBufferString(
				`<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/"><GetMetricStatisticsResult><Datapoints><member><Timestamp>2020-01-01T00:00:00Z</Timestamp><ExtendedStatistics><entry><key>p50</key><value>111</value></entry></ExtendedStatistics></member></Datapoints></GetMetricStatisticsResult></GetMetricStatisticsResponse>`)),
			Header: make(http.Header),
		}, nil
	}

	awsCfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKIA...", "secret", ""),
		HTTPClient:  &http.Client{Transport: stubRoundTripper{fn: responder}},
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(func(service, region string, _ ...any) (aws.Endpoint, error) {
			return aws.Endpoint{URL: "http://example.com", SigningRegion: "us-east-1"}, nil
		}),
	}
	repo := NewCloudWatchMetricsRepositoryWithCaching(awsCfg, "ns", "prod", "", zap.NewNop(), nil, nil)
	require.NotNil(t, repo)
	require.NotNil(t, repo.client)

	repo.client = cloudwatch.NewFromConfig(awsCfg)
	v, err := repo.getMetricPercentile(context.Background(), "AWS/ApiGateway", "Latency", 99, time.Now().Add(-time.Hour), time.Now(), map[string]string{"Stage": "prod"})
	require.NoError(t, err)
	require.Equal(t, 0.0, v)

	v, err = repo.getMetricPercentile(context.Background(), "AWS/ApiGateway", "Latency", 90, time.Now().Add(-time.Hour), time.Now(), map[string]string{"Stage": "prod"})
	require.NoError(t, err)
	require.Equal(t, 0.0, v)
}

func TestCloudWatchMetricsRepository_CalculateEstimatedCost_Tiers(t *testing.T) {
	enhanced := NewEnhancedBaseRepository[*models.CloudWatchMetrics](nil, "", zap.NewNop(), nil, "CloudWatchMetricsRepository", "cloudwatch")
	enhanced.BaseRepository = nil
	repo := &CloudWatchMetricsRepository{EnhancedBaseRepository: enhanced}
	// Push into the highest data transfer tier.
	metrics := &ServiceMetrics{
		RequestCount:      1_000_000,
		DynamoDBReads:     2_000_000,
		DynamoDBWrites:    2_000_000,
		LambdaInvocations: 1_000_000,
		S3Requests:        1_000_000,
		DataTransferBytes: int64(120) * 1024 * 1024 * 1024,
	}
	cost := repo.calculateEstimatedCost(metrics)
	require.GreaterOrEqual(t, cost, 0.0)
}
