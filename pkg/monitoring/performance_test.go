package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformanceMonitorPutMetric(t *testing.T) {
	t.Parallel()

	client := &stubCloudWatch{}
	pm := &PerformanceMonitor{
		cloudwatch:  client,
		namespace:   "ns",
		environment: "test",
	}

	require.NoError(t, pm.putMetric(context.Background(), MetricData{
		Name:  "Test",
		Value: 1,
		Unit:  types.StandardUnitCount,
		Dimensions: map[string]string{
			"Environment": "test",
		},
	}))

	client.mu.Lock()
	require.Len(t, client.putMetricDataInputs, 1)
	require.NotNil(t, client.putMetricDataInputs[0].Namespace)
	assert.Equal(t, "ns", *client.putMetricDataInputs[0].Namespace)
	client.mu.Unlock()

	client.putMetricDataErr = errors.New("put failed")
	require.Error(t, pm.putMetric(context.Background(), MetricData{
		Name:  "Test2",
		Value: 1,
		Unit:  types.StandardUnitCount,
		Dimensions: map[string]string{
			"Environment": "test",
		},
	}))
}

func TestPerformanceMonitorRecordMethods(t *testing.T) {
	t.Parallel()

	client := &stubCloudWatch{}
	pm := &PerformanceMonitor{
		cloudwatch:  client,
		namespace:   "ns",
		environment: "test",
	}

	require.NoError(t, pm.RecordLatency(context.Background(), "op", 12))
	require.NoError(t, pm.RecordError(context.Background(), "op", "boom"))
	require.NoError(t, pm.RecordDynamoDBConsumedCapacity(context.Background(), "table", "put", 1, 2))
	require.NoError(t, pm.RecordLambdaColdStart(context.Background(), "fn", false, 0))
	require.NoError(t, pm.RecordLambdaColdStart(context.Background(), "fn", true, 10))
	require.NoError(t, pm.RecordSQSQueueDepth(context.Background(), "queue", 42))
	require.NoError(t, pm.RecordQueryComplexity(context.Background(), "q", 7))
	require.NoError(t, pm.RecordCacheHit(context.Background(), "cache", true))
	require.NoError(t, pm.RecordCacheHit(context.Background(), "cache", false))
	require.NoError(t, pm.RecordFederationPerformance(context.Background(), "example.com", "fetch", 10, true))
	require.NoError(t, pm.RecordFederationPerformance(context.Background(), "example.com", "fetch", 10, false))

	client.mu.Lock()
	defer client.mu.Unlock()

	// spot-check we issued multiple CloudWatch writes
	assert.Greater(t, len(client.putMetricDataInputs), 5)
}

func TestBatchMetricsFlush(t *testing.T) {
	t.Parallel()

	client := &stubCloudWatch{}
	pm := &PerformanceMonitor{
		cloudwatch:  client,
		namespace:   "ns",
		environment: "test",
	}

	bm := pm.NewBatchMetrics()
	require.NoError(t, bm.Flush(context.Background()))

	for i := 0; i < 25; i++ {
		bm.Add(MetricData{
			Name:  "Metric",
			Value: float64(i),
			Unit:  types.StandardUnitCount,
			Dimensions: map[string]string{
				"Environment": "test",
			},
		})
	}

	require.NoError(t, bm.Flush(context.Background()))

	client.mu.Lock()
	require.Len(t, client.putMetricDataInputs, 2)
	assert.Len(t, client.putMetricDataInputs[0].MetricData, 20)
	assert.Len(t, client.putMetricDataInputs[1].MetricData, 5)
	client.mu.Unlock()

	client.putMetricDataErr = errors.New("batch failed")
	bm.Add(MetricData{Name: "Metric", Value: 1, Unit: types.StandardUnitCount, Dimensions: map[string]string{"Environment": "test"}})
	require.Error(t, bm.Flush(context.Background()))
}

func TestPerformanceMonitorTracingHelpers(t *testing.T) {
	t.Parallel()

	client := &stubCloudWatch{}
	pm := &PerformanceMonitor{
		cloudwatch:  client,
		namespace:   "ns",
		environment: "test",
	}

	ctx, seg := xray.BeginSegment(context.Background(), "test")
	defer seg.Close(nil)

	require.NoError(t, pm.AddXRayAnnotation(ctx, "k", "v"))
	require.NoError(t, pm.AddXRayMetadata(ctx, "ns", "k", "v"))

	pm.RecordXRayError(ctx, errors.New("boom"))

	require.Error(t, pm.TraceDBQuery(ctx, "GetItem", "table", func(context.Context) error {
		return errors.New("db boom")
	}))

	require.NoError(t, pm.TraceFederationCall(ctx, "example.com", "fetch", func(context.Context) error {
		return nil
	}))

	require.Error(t, pm.TraceFederationCall(ctx, "example.com", "fetch", func(context.Context) error {
		return errors.New("fetch boom")
	}))

	require.NoError(t, pm.TraceGraphQLQuery(ctx, "Query", 7, func(context.Context) error {
		return nil
	}))

	require.Error(t, pm.TraceGraphQLQuery(ctx, "Query", 7, func(context.Context) error {
		return errors.New("graphql boom")
	}))

	require.Error(t, pm.TraceLambdaHandler(ctx, "handler", func(context.Context) error {
		time.Sleep(1 * time.Millisecond)
		return errors.New("lambda boom")
	}))
}

func TestPerformanceMonitorInsightsAndAlarm(t *testing.T) {
	t.Parallel()

	client := &stubCloudWatch{
		getMetricStatisticsOutput: &cloudwatch.GetMetricStatisticsOutput{
			Datapoints: []types.Datapoint{{Average: aws.Float64(1.23)}},
		},
	}
	pm := &PerformanceMonitor{
		cloudwatch:  client,
		namespace:   "ns",
		environment: "test",
	}

	points, err := pm.GetPerformanceInsights(context.Background(), "Metric", time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)
	require.Len(t, points, 1)

	client.getMetricStatisticsErr = errors.New("stats failed")
	_, err = pm.GetPerformanceInsights(context.Background(), "Metric", time.Now().Add(-time.Hour), time.Now())
	require.Error(t, err)

	require.NoError(t, pm.CreateAlarm(context.Background(), "alarm", "Metric", 10, types.ComparisonOperatorGreaterThanThreshold))
	client.putMetricAlarmErr = errors.New("alarm failed")
	require.Error(t, pm.CreateAlarm(context.Background(), "alarm", "Metric", 10, types.ComparisonOperatorGreaterThanThreshold))
}
