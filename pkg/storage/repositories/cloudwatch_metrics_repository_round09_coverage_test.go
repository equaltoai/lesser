package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubCloudWatchClient struct {
	getMetricStatisticsFn func(context.Context, *cloudwatch.GetMetricStatisticsInput) (*cloudwatch.GetMetricStatisticsOutput, error)
}

func (c *stubCloudWatchClient) GetMetricStatistics(ctx context.Context, input *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	if c.getMetricStatisticsFn != nil {
		return c.getMetricStatisticsFn(ctx, input)
	}
	return &cloudwatch.GetMetricStatisticsOutput{}, nil
}

func cloudWatchClientForTest(t *testing.T, responder func(context.Context, *cloudwatch.GetMetricStatisticsInput) (*cloudwatch.GetMetricStatisticsOutput, error)) cloudWatchAPI {
	t.Helper()
	return &stubCloudWatchClient{getMetricStatisticsFn: responder}
}

func TestCloudWatchMetricsRepository_NewRepository_ErrorPath(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	repo := NewCloudWatchMetricsRepository("ns", "env", zap.NewNop())
	require.NotNil(t, repo)
}

func TestCloudWatchMetricsRepository_GetServiceMetrics_UsesStubbedHTTP(t *testing.T) {
	client := cloudWatchClientForTest(t, func(_ context.Context, input *cloudwatch.GetMetricStatisticsInput) (*cloudwatch.GetMetricStatisticsOutput, error) {
		baseTS := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

		if len(input.ExtendedStatistics) > 0 {
			ext := input.ExtendedStatistics[0]
			dp := cloudwatchTypes.Datapoint{
				Timestamp: aws.Time(baseTS),
			}
			switch ext {
			case "p50":
				dp.ExtendedStatistics = map[string]float64{"p50": 123}
			case "p90":
				dp.Timestamp = aws.Time(baseTS.Add(24 * time.Hour))
				dp.ExtendedStatistics = map[string]float64{"p90": 250}
			case "p99":
				dp.Timestamp = aws.Time(baseTS.Add(48 * time.Hour))
				dp.ExtendedStatistics = map[string]float64{"p99": 500}
			default:
				dp.ExtendedStatistics = map[string]float64{}
			}
			return &cloudwatch.GetMetricStatisticsOutput{
				Datapoints: []cloudwatchTypes.Datapoint{dp},
			}, nil
		}

		return &cloudwatch.GetMetricStatisticsOutput{
			Datapoints: []cloudwatchTypes.Datapoint{
				{
					Timestamp: aws.Time(baseTS),
					Sum:       aws.Float64(5),
				},
			},
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
	client := cloudWatchClientForTest(t, func(_ context.Context, _ *cloudwatch.GetMetricStatisticsInput) (*cloudwatch.GetMetricStatisticsOutput, error) {
		return &cloudwatch.GetMetricStatisticsOutput{}, nil
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
	client := cloudWatchClientForTest(t, func(_ context.Context, _ *cloudwatch.GetMetricStatisticsInput) (*cloudwatch.GetMetricStatisticsOutput, error) {
		ts := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
		return &cloudwatch.GetMetricStatisticsOutput{
			Datapoints: []cloudwatchTypes.Datapoint{
				{
					Timestamp: aws.Time(ts),
					Sum:       aws.Float64(100),
				},
			},
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
	client := cloudWatchClientForTest(t, func(_ context.Context, input *cloudwatch.GetMetricStatisticsInput) (*cloudwatch.GetMetricStatisticsOutput, error) {
		if aws.ToString(input.MetricName) == "FailMe" {
			return nil, errors.New("boom")
		}

		ts := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
		if len(input.Statistics) > 0 {
			switch input.Statistics[0] {
			case cloudwatchTypes.StatisticAverage:
				return &cloudwatch.GetMetricStatisticsOutput{
					Datapoints: []cloudwatchTypes.Datapoint{
						{Timestamp: aws.Time(ts), Average: aws.Float64(10)},
						{Timestamp: aws.Time(ts.Add(5 * time.Minute)), Average: aws.Float64(20)},
					},
				}, nil
			case cloudwatchTypes.StatisticMaximum:
				return &cloudwatch.GetMetricStatisticsOutput{
					Datapoints: []cloudwatchTypes.Datapoint{
						{Timestamp: aws.Time(ts), Maximum: aws.Float64(5)},
						{Timestamp: aws.Time(ts.Add(5 * time.Minute)), Maximum: aws.Float64(7)},
					},
				}, nil
			case cloudwatchTypes.StatisticMinimum:
				return &cloudwatch.GetMetricStatisticsOutput{
					Datapoints: []cloudwatchTypes.Datapoint{
						{Timestamp: aws.Time(ts), Minimum: aws.Float64(2)},
						{Timestamp: aws.Time(ts.Add(5 * time.Minute)), Minimum: aws.Float64(1)},
					},
				}, nil
			}
		}

		return &cloudwatch.GetMetricStatisticsOutput{
			Datapoints: []cloudwatchTypes.Datapoint{
				{Timestamp: aws.Time(ts), Sum: aws.Float64(3)},
			},
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
	repo := NewCloudWatchMetricsRepositoryWithCaching(aws.Config{Region: "us-east-1"}, "ns", "prod", "", zap.NewNop(), nil, nil)
	require.NotNil(t, repo)
	require.NotNil(t, repo.client)

	repo.client = cloudWatchClientForTest(t, func(_ context.Context, input *cloudwatch.GetMetricStatisticsInput) (*cloudwatch.GetMetricStatisticsOutput, error) {
		// Empty datapoints branch.
		if len(input.ExtendedStatistics) > 0 && input.ExtendedStatistics[0] == "p90" {
			return &cloudwatch.GetMetricStatisticsOutput{}, nil
		}

		// Missing key branch (extended stats present but not the requested key).
		ts := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
		return &cloudwatch.GetMetricStatisticsOutput{
			Datapoints: []cloudwatchTypes.Datapoint{
				{
					Timestamp:          aws.Time(ts),
					ExtendedStatistics: map[string]float64{"p50": 111},
				},
			},
		}, nil
	})

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
