package performance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeCloudWatch struct {
	responses map[string]*cloudwatch.GetMetricStatisticsOutput
	errs      map[string]error
}

func metricKey(metricName, statistic, functionName string) string {
	return metricName + "|" + statistic + "|" + functionName
}

func functionNameFromInput(input *cloudwatch.GetMetricStatisticsInput) string {
	if input == nil {
		return ""
	}
	for _, dim := range input.Dimensions {
		if aws.ToString(dim.Name) == "FunctionName" {
			return aws.ToString(dim.Value)
		}
	}
	return ""
}

func statisticFromInput(input *cloudwatch.GetMetricStatisticsInput) string {
	if input == nil {
		return ""
	}
	if len(input.Statistics) == 0 {
		return ""
	}
	return string(input.Statistics[0])
}

func (f *fakeCloudWatch) GetMetricStatistics(_ context.Context, input *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	key := metricKey(aws.ToString(input.MetricName), statisticFromInput(input), functionNameFromInput(input))
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	if resp, ok := f.responses[key]; ok {
		return resp, nil
	}
	return &cloudwatch.GetMetricStatisticsOutput{Datapoints: []types.Datapoint{}}, nil
}

func TestService_getMetricSum(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := &fakeCloudWatch{
		responses: map[string]*cloudwatch.GetMetricStatisticsOutput{
			metricKey("Invocations", string(types.StatisticSum), "fn-empty"): {Datapoints: []types.Datapoint{}},
			metricKey("Invocations", string(types.StatisticSum), "fn-sum"): {
				Datapoints: []types.Datapoint{
					{Sum: aws.Float64(1.5)},
					{Sum: aws.Float64(2.0)},
					{Sum: nil},
				},
			},
		},
		errs: map[string]error{
			metricKey("Invocations", string(types.StatisticSum), "fn-error"): errors.New("cloudwatch down"),
		},
	}

	service := NewService(client, "test", zap.NewNop())

	t.Run("returns zero when no datapoints", func(t *testing.T) {
		total, err := service.getMetricSum(ctx, "AWS/Lambda", "Invocations", time.Now().Add(-time.Hour), time.Now(), "fn-empty")
		require.NoError(t, err)
		require.Equal(t, float64(0), total)
	})

	t.Run("sums datapoints", func(t *testing.T) {
		total, err := service.getMetricSum(ctx, "AWS/Lambda", "Invocations", time.Now().Add(-time.Hour), time.Now(), "fn-sum")
		require.NoError(t, err)
		require.Equal(t, 3.5, total)
	})

	t.Run("returns error when client fails", func(t *testing.T) {
		_, err := service.getMetricSum(ctx, "AWS/Lambda", "Invocations", time.Now().Add(-time.Hour), time.Now(), "fn-error")
		require.Error(t, err)
	})
}

func TestService_getMetricDatapoints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := &fakeCloudWatch{
		responses: map[string]*cloudwatch.GetMetricStatisticsOutput{
			metricKey("Duration", string(types.StatisticAverage), "fn-empty"): {Datapoints: []types.Datapoint{}},
			metricKey("Duration", string(types.StatisticAverage), "fn-values"): {
				Datapoints: []types.Datapoint{
					{Average: aws.Float64(100)},
					{Average: nil},
					{Average: aws.Float64(250)},
				},
			},
		},
		errs: map[string]error{
			metricKey("Duration", string(types.StatisticAverage), "fn-error"): errors.New("cloudwatch down"),
		},
	}

	service := NewService(client, "test", zap.NewNop())

	t.Run("returns empty slice when no datapoints", func(t *testing.T) {
		values, err := service.getMetricDatapoints(ctx, "AWS/Lambda", "Duration", time.Now().Add(-time.Hour), time.Now(), "fn-empty")
		require.NoError(t, err)
		require.Empty(t, values)
	})

	t.Run("returns average values", func(t *testing.T) {
		values, err := service.getMetricDatapoints(ctx, "AWS/Lambda", "Duration", time.Now().Add(-time.Hour), time.Now(), "fn-values")
		require.NoError(t, err)
		require.Equal(t, []float64{100, 250}, values)
	})

	t.Run("returns error when client fails", func(t *testing.T) {
		_, err := service.getMetricDatapoints(ctx, "AWS/Lambda", "Duration", time.Now().Add(-time.Hour), time.Now(), "fn-error")
		require.Error(t, err)
	})
}

func TestService_aggregateMetricsFromFunctions_CollectsAcrossFunctions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := &fakeCloudWatch{
		responses: map[string]*cloudwatch.GetMetricStatisticsOutput{
			metricKey("Invocations", string(types.StatisticSum), "fn-1"): {Datapoints: []types.Datapoint{{Sum: aws.Float64(10)}}},
			metricKey("Errors", string(types.StatisticSum), "fn-1"):      {Datapoints: []types.Datapoint{{Sum: aws.Float64(1)}}},
			metricKey("ColdStarts", string(types.StatisticSum), "fn-1"):  {Datapoints: []types.Datapoint{{Sum: aws.Float64(2)}}},
			metricKey("Duration", string(types.StatisticAverage), "fn-1"): {Datapoints: []types.Datapoint{
				{Average: aws.Float64(100)},
				{Average: aws.Float64(200)},
			}},

			metricKey("Errors", string(types.StatisticSum), "fn-2"):       {Datapoints: []types.Datapoint{{Sum: aws.Float64(2)}}},
			metricKey("ColdStarts", string(types.StatisticSum), "fn-2"):   {Datapoints: []types.Datapoint{}},
			metricKey("Duration", string(types.StatisticAverage), "fn-2"): {Datapoints: []types.Datapoint{{Average: aws.Float64(50)}}},
		},
		errs: map[string]error{
			metricKey("Invocations", string(types.StatisticSum), "fn-2"): errors.New("boom"),
		},
	}

	service := NewService(client, "test", zap.NewNop())

	agg := service.aggregateMetricsFromFunctions(ctx, []string{"fn-1", "fn-2"}, time.Now().Add(-time.Hour), time.Now())
	require.NotNil(t, agg)
	require.Equal(t, int64(10), agg.totalInvocations)
	require.Equal(t, int64(3), agg.totalErrors)
	require.Equal(t, int64(2), agg.coldStarts)
	require.Len(t, agg.durations, 3)
}

func TestService_GetPerformanceMetrics_SucceedsWithCloudWatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	client := &fakeCloudWatch{
		responses: map[string]*cloudwatch.GetMetricStatisticsOutput{
			metricKey("Invocations", string(types.StatisticSum), "lesser-test-graphql"): {Datapoints: []types.Datapoint{{Sum: aws.Float64(5)}}},
			metricKey("Errors", string(types.StatisticSum), "lesser-test-graphql"):      {Datapoints: []types.Datapoint{{Sum: aws.Float64(1)}}},
			metricKey("ColdStarts", string(types.StatisticSum), "lesser-test-graphql"):  {Datapoints: []types.Datapoint{}},
			metricKey("Duration", string(types.StatisticAverage), "lesser-test-graphql"): {Datapoints: []types.Datapoint{
				{Average: aws.Float64(100)},
				{Average: aws.Float64(200)},
			}},

			metricKey("Invocations", string(types.StatisticSum), "lesser-test-api"): {Datapoints: []types.Datapoint{{Sum: aws.Float64(15)}}},
			metricKey("Errors", string(types.StatisticSum), "lesser-test-api"):      {Datapoints: []types.Datapoint{{Sum: aws.Float64(1)}}},
			metricKey("ColdStarts", string(types.StatisticSum), "lesser-test-api"):  {Datapoints: []types.Datapoint{{Sum: aws.Float64(1)}}},
			metricKey("Duration", string(types.StatisticAverage), "lesser-test-api"): {Datapoints: []types.Datapoint{
				{Average: aws.Float64(300)},
				{Average: aws.Float64(400)},
			}},
		},
	}

	service := NewService(client, "test", zap.NewNop())
	report, err := service.GetPerformanceMetrics(ctx, model.ServiceCategoryGraphqlAPI, model.TimePeriodHour)
	require.NoError(t, err)
	require.NotNil(t, report)

	require.Equal(t, model.ServiceCategoryGraphqlAPI, report.Service)
	require.Equal(t, model.TimePeriodHour, report.Period)
	require.Equal(t, model.Duration(300*time.Millisecond), report.P50Latency)
	require.Equal(t, model.Duration(400*time.Millisecond), report.P95Latency)
	require.Equal(t, model.Duration(400*time.Millisecond), report.P99Latency)
	require.InEpsilon(t, 0.1, report.ErrorRate, 1e-9)
	require.InEpsilon(t, float64(20)/float64(time.Hour.Seconds()), report.Throughput, 1e-9)
	require.Equal(t, 1, report.ColdStarts)
}
