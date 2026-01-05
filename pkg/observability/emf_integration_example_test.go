package observability

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestEMFIntegrationHelpers(t *testing.T) {
	assert.Equal(t, "root", sanitizePathForMetrics(""))
	assert.Equal(t, "root", sanitizePathForMetrics("/"))
	assert.Equal(t, "users_alice", sanitizePathForMetrics("/users/alice"))
	assert.Equal(t, "users_alice", sanitizePathForMetrics("/users/{alice}"))
	assert.Equal(t, "root", sanitizePathForMetrics("/{}"))

	assert.Equal(t, "2xx", getStatusCodeRange(200))
	assert.Equal(t, "3xx", getStatusCodeRange(302))
	assert.Equal(t, "4xx", getStatusCodeRange(404))
	assert.Equal(t, "5xx", getStatusCodeRange(500))
	assert.Equal(t, HealthStatusUnknown, getStatusCodeRange(0))

	assert.Equal(t, "abc", replaceAll("abc", "", "x"))
	assert.Equal(t, "abc", replaceAll("abc", "x", "x"))
	assert.Equal(t, "a_b_c", replaceAll("a/b/c", "/", "_"))

	assert.Equal(t, "none", classifyDynamoDBError(nil))
	assert.Equal(t, "throughput_exceeded", classifyDynamoDBError(&testError{s: "ProvisionedThroughputExceededException"}))
	assert.Equal(t, "resource_not_found", classifyDynamoDBError(&testError{s: "ResourceNotFoundException"}))
	assert.Equal(t, "conditional_check_failed", classifyDynamoDBError(&testError{s: "ConditionalCheckFailedException"}))
	assert.Equal(t, "validation", classifyDynamoDBError(&testError{s: "ValidationException"}))
	assert.Equal(t, "item_collection_size_limit", classifyDynamoDBError(&testError{s: "ItemCollectionSizeLimitExceededException"}))
	assert.Equal(t, "transaction_conflict", classifyDynamoDBError(&testError{s: "TransactionConflictException"}))
	assert.Equal(t, "request_limit_exceeded", classifyDynamoDBError(&testError{s: "RequestLimitExceededException"}))
	assert.Equal(t, "internal_server_error", classifyDynamoDBError(&testError{s: "InternalServerError"}))
	assert.Equal(t, HealthStatusUnknown, classifyDynamoDBError(&testError{s: "something_else"}))
}

type testError struct{ s string }

func (e *testError) Error() string { return e.s }

func TestEMFMetricsService_RecordRequestMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := NewEMFMetricsService(logger)
	require.NotNil(t, svc.collector)

	req := lift.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/health"
	ctx := lift.NewContext(context.Background(), req)
	ctx.Response.StatusCode = 204

	svc.RecordRequestMetrics(ctx, &PerformanceMetrics{
		ExecutionDuration: 25 * time.Millisecond,
		ColdStartDuration: 10 * time.Millisecond,
		MemoryUsed:        123,
	}, nil)

	require.NotEmpty(t, svc.collector.buffer.metrics)
}

func TestEMFMetricsService_RecordDynamoDBMetrics_CoversBranches(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := NewEMFMetricsService(logger)

	svc.RecordDynamoDBMetrics("GetItem", "tbl", 10*time.Millisecond, 1, 2, nil)
	svc.RecordDynamoDBMetrics("PutItem", "tbl", 10*time.Millisecond, 0, 0, assert.AnError)

	require.GreaterOrEqual(t, svc.collector.GetBufferSize(), 6)
}

func TestEMFIntegration_ExampleHandlerAndMigration(t *testing.T) {
	ExampleLambdaHandler()
	_, err := processRequest(nil)
	require.NoError(t, err)

	logger := zaptest.NewLogger(t)
	oldCollector := &MetricsCollector{
		client:  &cloudwatchPutMetricDataStub{},
		metrics: make(map[string]*MetricBuffer),
		logger:  logger,
	}

	oldCollector.RecordMetric("m", 1, types.StandardUnitCount)
	svc := ConvertPollingMetricsToEMF(oldCollector, logger)
	require.NotNil(t, svc)
}

func TestEMFIntegration_MiddlewareAndFlushError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	svc := NewEMFMetricsService(logger)

	req := lift.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/foo"
	ctx := lift.NewContext(context.Background(), req)
	ctx.Response.StatusCode = 204

	mw := CreateEMFPerformanceMonitoringMiddleware(svc)
	handler := mw(lift.HandlerFunc(func(c *lift.Context) error {
		c.Response.StatusCode = 500
		return assert.AnError
	}))

	require.ErrorIs(t, handler.Handle(ctx), assert.AnError)

	// Force FlushMetrics error path via NaN metric value (JSON cannot encode NaN).
	svc.collector.recordMetricWithDimensions("BadMetric", math.NaN(), "Count", map[string]string{"Operation": "op"})
	require.Error(t, svc.FlushMetrics())
	svc.Stop()

	blank := lift.NewContext(context.Background(), lift.NewRequest(nil))
	assert.Equal(t, HealthStatusUnknown, getOperationName(blank))
}
