package observability

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type cloudwatchPutMetricDataStub struct {
	inputs []*cloudwatch.PutMetricDataInput
	err    error
}

func (s *cloudwatchPutMetricDataStub) PutMetricData(_ context.Context, params *cloudwatch.PutMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	s.inputs = append(s.inputs, params)
	return &cloudwatch.PutMetricDataOutput{}, s.err
}

func TestMetricsCollector_FlushPublishesToCloudWatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	stub := &cloudwatchPutMetricDataStub{}

	mc := NewMetricsCollector(stub, "ns", logger)
	mc.RecordMetric("Requests", 1, types.StandardUnitCount, types.Dimension{Name: aws.String("Route"), Value: aws.String("/health")})
	mc.RecordMetric("Requests", 2, types.StandardUnitCount, types.Dimension{Name: aws.String("Route"), Value: aws.String("/health")})
	mc.RecordErrorRate("op", 0, 0)

	mc.Flush()
	require.Len(t, stub.inputs, 1)
	require.Equal(t, "ns", aws.ToString(stub.inputs[0].Namespace))
	require.NotEmpty(t, stub.inputs[0].MetricData)
}

func TestMetricsCollector_FlushHandlesClientError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	stub := &cloudwatchPutMetricDataStub{err: assert.AnError}

	mc := NewMetricsCollector(stub, "ns", logger)
	mc.RecordMetric("Requests", 1, types.StandardUnitCount)
	mc.Flush()
	require.Len(t, stub.inputs, 1)
}

func TestMetricsCollector_FlushWithNoMetricsDoesNotCallCloudWatch(t *testing.T) {
	logger := zaptest.NewLogger(t)
	stub := &cloudwatchPutMetricDataStub{}

	mc := NewMetricsCollector(stub, "ns", logger)
	mc.Flush()

	require.Empty(t, stub.inputs)
}

func TestMetricsCollector_RecordConvenienceMethods(t *testing.T) {
	logger := zaptest.NewLogger(t)
	stub := &cloudwatchPutMetricDataStub{}

	mc := NewMetricsCollector(stub, "ns", logger)
	mc.RecordLatency("GetTimeline", 125*time.Millisecond)
	mc.RecordThroughput("GetTimeline", 42)
	mc.RecordCost("GetTimeline", 0.0123)
	mc.RecordPerformanceMetrics(&PerformanceMetrics{
		ColdStartDuration: 25 * time.Millisecond,
		ExecutionDuration: 75 * time.Millisecond,
		MemoryUsed:        123,
		MemoryAllocated:   456,
		CPUUtilization:    0.3,
		GoroutineCount:    7,
		GCPauseTime:       8 * time.Microsecond,
	})

	mc.Flush()
	require.Len(t, stub.inputs, 1)
	require.NotEmpty(t, stub.inputs[0].MetricData)

	metricNames := make(map[string]struct{}, len(stub.inputs[0].MetricData))
	for _, datum := range stub.inputs[0].MetricData {
		metricNames[aws.ToString(datum.MetricName)] = struct{}{}
	}

	for _, expected := range []string{
		"OperationLatency",
		"OperationThroughput",
		"OperationCost",
		"ColdStartDuration",
		"ExecutionDuration",
		"MemoryUsed",
		"MemoryAllocated",
		"CPUUtilization",
		"GoroutineCount",
		"GCPauseTime",
	} {
		_, ok := metricNames[expected]
		require.Truef(t, ok, "expected metric %q to be flushed", expected)
	}
}

func TestGetEnvironment_PrefersEnvironmentThenStage(t *testing.T) {
	cfg := config.Get()
	origEnv := cfg.Environment
	origStage := cfg.Stage
	t.Cleanup(func() {
		cfg.Environment = origEnv
		cfg.Stage = origStage
	})

	cfg.Environment = "production"
	cfg.Stage = "staging"
	require.Equal(t, "production", getEnvironment())

	cfg.Environment = ""
	cfg.Stage = "staging"
	require.Equal(t, "staging", getEnvironment())

	cfg.Environment = ""
	cfg.Stage = ""
	require.Equal(t, StatusUnknown, getEnvironment())
}

func TestMetricsHelpers(t *testing.T) {
	sum, minVal, maxVal := calculateStats([]float64{3, 1, 2})
	assert.Equal(t, 6.0, sum)
	assert.Equal(t, 1.0, minVal)
	assert.Equal(t, 3.0, maxVal)

	sum, minVal, maxVal = calculateStats(nil)
	assert.Equal(t, 0.0, sum)
	assert.Equal(t, 0.0, minVal)
	assert.Equal(t, 0.0, maxVal)

	assert.Equal(t, 0.0, mathMin(0, 1))
	assert.Equal(t, 1.0, mathMin(2, 1))

	util := calculateCPUUtilization()
	assert.GreaterOrEqual(t, util, 0.0)
	assert.LessOrEqual(t, util, 1.0)

	perf := GetPerformanceMetrics(time.Now().Add(-time.Millisecond), time.Now().Add(-time.Millisecond))
	require.NotNil(t, perf)
}
