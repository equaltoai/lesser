package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestClassifyDynamoDBError(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "none", classifyDynamoDBError(nil))
	assert.Equal(t, "throughput_exceeded", classifyDynamoDBError(errors.New("ProvisionedThroughputExceededException")))
	assert.Equal(t, "resource_not_found", classifyDynamoDBError(errors.New("ResourceNotFoundException")))
	assert.Equal(t, "conditional_check_failed", classifyDynamoDBError(errors.New("ConditionalCheckFailedException")))
	assert.Equal(t, "validation", classifyDynamoDBError(errors.New("ValidationException")))
	assert.Equal(t, "item_collection_size_limit", classifyDynamoDBError(errors.New("ItemCollectionSizeLimitExceededException")))
	assert.Equal(t, "transaction_conflict", classifyDynamoDBError(errors.New("TransactionConflictException")))
	assert.Equal(t, "request_limit_exceeded", classifyDynamoDBError(errors.New("RequestLimitExceededException")))
	assert.Equal(t, "internal_server_error", classifyDynamoDBError(errors.New("InternalServerError")))
	assert.Equal(t, StatusUnknown, classifyDynamoDBError(errors.New("something else")))
}

func TestEnhancedMetricBuffer(t *testing.T) {
	t.Parallel()

	flushCalls := 0
	buffer := &EnhancedMetricBuffer{
		metrics:   make([]cwTypes.MetricDatum, 0, 10),
		maxSize:   10,
		flushSize: 2,
		flushFunc: func([]cwTypes.MetricDatum) error {
			flushCalls++
			return nil
		},
	}

	assert.Equal(t, 0, buffer.Size())
	assert.False(t, buffer.ShouldFlush())
	require.NoError(t, buffer.Flush()) // empty flush is a no-op

	buffer.Add(cwTypes.MetricDatum{MetricName: aws.String("m1")})
	assert.Equal(t, 1, buffer.Size())
	assert.False(t, buffer.ShouldFlush())

	buffer.Add(cwTypes.MetricDatum{MetricName: aws.String("m2")})
	assert.Equal(t, 2, buffer.Size())
	assert.True(t, buffer.ShouldFlush())

	require.NoError(t, buffer.Flush())
	assert.Equal(t, 0, buffer.Size())
	assert.Equal(t, 1, flushCalls)
}

func TestCloudWatchMetricsBuildDimensions(t *testing.T) {
	t.Parallel()

	cwm := &CloudWatchMetrics{
		logger:      zap.NewNop(),
		environment: "test",
		dimensions: map[string]string{
			"Default": "default",
			"Empty":   "",
		},
	}

	dims := cwm.buildDimensions(
		map[string]string{"Base": "base", "Skip": ""},
		map[string]string{"Extra": "extra", "Empty2": ""},
	)
	got := cwDimensionsToMap(dims)

	assert.Equal(t, "default", got["Default"])
	assert.Equal(t, "base", got["Base"])
	assert.Equal(t, "extra", got["Extra"])
	assert.Equal(t, "", got["Empty"])
	assert.NotContains(t, got, "Skip")
	assert.NotContains(t, got, "Empty2")
}

func TestCloudWatchMetricsRecordDynamORMMetrics(t *testing.T) {
	t.Parallel()

	cwm := &CloudWatchMetrics{
		logger:      zap.NewNop(),
		environment: "test",
		dimensions:  map[string]string{},
		buffer: &EnhancedMetricBuffer{
			metrics:   make([]cwTypes.MetricDatum, 0, 100),
			maxSize:   100,
			flushSize: 1000,
			flushFunc: func([]cwTypes.MetricDatum) error { return nil },
		},
	}

	cwm.RecordDynamORMMetrics(context.Background(), DynamORMMetrics{
		Operation: "Query",
		TableName: "table",
		ConsumedCapacity: ConsumedCapacity{
			ReadUnits:  1,
			WriteUnits: 2,
		},
		ItemCount: 3,
		Duration:  10 * time.Millisecond,
		Error:     errors.New("boom"),
	})

	// Latency, read cap, write cap, item count, errors, success rate
	assert.Equal(t, 6, cwm.buffer.Size())
}

func TestCloudWatchMetricsFlushToCloudWatchBatchesAndErrors(t *testing.T) {
	t.Parallel()

	client := &stubCloudWatch{}
	cwm := &CloudWatchMetrics{
		client:      client,
		logger:      zap.NewNop(),
		namespace:   "ns",
		environment: "test",
		dimensions:  map[string]string{},
	}

	require.NoError(t, cwm.flushToCloudWatch(nil))

	metrics := make([]cwTypes.MetricDatum, 0, 25)
	for i := 0; i < 25; i++ {
		metrics = append(metrics, cwTypes.MetricDatum{MetricName: aws.String("m")})
	}

	require.NoError(t, cwm.flushToCloudWatch(metrics))

	client.mu.Lock()
	putCalls := append([]*cloudwatch.PutMetricDataInput(nil), client.putMetricDataInputs...)
	client.mu.Unlock()

	require.Len(t, putCalls, 2)
	require.NotNil(t, putCalls[0].Namespace)
	assert.Equal(t, "ns", *putCalls[0].Namespace)
	assert.Len(t, putCalls[0].MetricData, 20)
	assert.Len(t, putCalls[1].MetricData, 5)

	client.putMetricDataErr = errors.New("put failed")
	require.Error(t, cwm.flushToCloudWatch(metrics[:1]))
}

func TestCloudWatchMetricsDefaultsNewAndFlush(t *testing.T) {
	cfg := DefaultMetricConfig()
	assert.NotEmpty(t, cfg.Namespace)
	assert.Greater(t, cfg.BufferSize, 0)
	assert.Greater(t, cfg.FlushSize, 0)
	assert.True(t, cfg.EnableBatching)

	cfg.FlushSize = 1
	cwm := NewCloudWatchMetrics(aws.Config{}, cfg, zap.NewNop())
	require.NotNil(t, cwm)
	require.NotNil(t, cwm.buffer)

	// Swap in a stub client so flushes don't hit the network.
	stub := &stubCloudWatch{}
	cwm.client = stub
	cwm.buffer.flushFunc = cwm.flushToCloudWatch

	cwm.RecordCostMetrics("op", CostData{
		TotalCostMicroCents:    100,
		DynamoDBCostMicroCents: 50,
		LambdaCostMicroCents:   25,
		S3CostMicroCents:       25,
	})

	// Force a flush path and ensure FlushMetrics can be called.
	cwm.RecordBusinessMetrics("Metric", 1, cwTypes.StandardUnitCount, nil)
	require.NoError(t, cwm.FlushMetrics(context.Background()))

	// Cover addMetric flush error path.
	stub.putMetricDataErr = errors.New("put failed")
	cwm.RecordBusinessMetrics("Metric", 1, cwTypes.StandardUnitCount, nil)

	assert.False(t, containsHelper("abc", "z"))
}
