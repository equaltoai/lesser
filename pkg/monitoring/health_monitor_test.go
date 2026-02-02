package monitoring

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTableExistsChecker struct {
	exists bool
	err    error
}

func (s *stubTableExistsChecker) TableExists(_ string) (bool, error) {
	return s.exists, s.err
}

type stubLambda struct {
	out *lambda.GetFunctionConfigurationOutput
	err error
}

func (s *stubLambda) GetFunctionConfiguration(_ context.Context, _ *lambda.GetFunctionConfigurationInput, _ ...func(*lambda.Options)) (*lambda.GetFunctionConfigurationOutput, error) {
	return s.out, s.err
}

type stubSQS struct {
	out *sqs.GetQueueAttributesOutput
	err error
}

func (s *stubSQS) GetQueueAttributes(_ context.Context, _ *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	return s.out, s.err
}

func newTestPerformanceMonitor() (*PerformanceMonitor, *stubCloudWatch) {
	client := &stubCloudWatch{}
	pm := &PerformanceMonitor{
		cloudwatch:  client,
		namespace:   "ns",
		environment: "test",
	}
	return pm, client
}

func TestHealthMonitorUpdateAndGetters(t *testing.T) {
	t.Parallel()

	pm, _ := newTestPerformanceMonitor()
	hm := &HealthMonitor{
		monitor:      pm,
		healthStatus: make(map[string]*ComponentHealth),
	}

	hm.updateComponentHealth("x", HealthStatusWarning, errors.New("boom"), map[string]any{"k": "v"})
	status := hm.GetHealthStatus()
	require.Contains(t, status, "x")
	assert.Equal(t, HealthStatusWarning, status["x"].Status)
	assert.Equal(t, 1, status["x"].ErrorCount)
	assert.NotNil(t, status["x"].LastError)
	assert.Equal(t, "v", status["x"].Metadata["k"])

	hm.updateComponentHealth("x", HealthStatusHealthy, nil, nil)
	status = hm.GetHealthStatus()
	assert.Equal(t, HealthStatusHealthy, status["x"].Status)
	assert.Equal(t, 0, status["x"].ErrorCount)
	assert.Nil(t, status["x"].LastError)

	assert.Equal(t, HealthStatusHealthy, hm.GetOverallHealth())
	hm.updateComponentHealth("y", HealthStatusCritical, errors.New("nope"), nil)
	assert.Equal(t, HealthStatusCritical, hm.GetOverallHealth())
}

func TestHealthMonitorCheckDynamoDBHealth(t *testing.T) {
	pm, _ := newTestPerformanceMonitor()

	t.Run("table exists is healthy", func(t *testing.T) {
		hm := &HealthMonitor{
			monitor:      pm,
			tableChecker: &stubTableExistsChecker{exists: true},
			healthStatus: make(map[string]*ComponentHealth),
		}

		require.NoError(t, hm.CheckDynamoDBHealth(context.Background(), "table"))
		status := hm.GetHealthStatus()
		require.Contains(t, status, "dynamodb.table")
		assert.Equal(t, HealthStatusHealthy, status["dynamodb.table"].Status)
		assert.Equal(t, true, status["dynamodb.table"].Metadata["tableExists"])
	})

	t.Run("client_error_sets_critical", func(t *testing.T) {
		hm := &HealthMonitor{
			monitor:      pm,
			tableChecker: &stubTableExistsChecker{err: errors.New("describe failed")},
			healthStatus: make(map[string]*ComponentHealth),
		}

		require.Error(t, hm.CheckDynamoDBHealth(context.Background(), "table"))
		status := hm.GetHealthStatus()
		require.Contains(t, status, "dynamodb.table")
		assert.Equal(t, HealthStatusCritical, status["dynamodb.table"].Status)
	})

	t.Run("not found is critical", func(t *testing.T) {
		hm := &HealthMonitor{
			monitor:      pm,
			tableChecker: &stubTableExistsChecker{exists: false},
			healthStatus: make(map[string]*ComponentHealth),
		}

		require.Error(t, hm.CheckDynamoDBHealth(context.Background(), "table"))
		status := hm.GetHealthStatus()
		require.Contains(t, status, "dynamodb.table")
		assert.Equal(t, HealthStatusCritical, status["dynamodb.table"].Status)
		assert.Equal(t, false, status["dynamodb.table"].Metadata["tableExists"])
	})
}

func TestHealthMonitorCheckLambdaHealth(t *testing.T) {
	pm, _ := newTestPerformanceMonitor()

	t.Run("status_from_state", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			state lambdaTypes.State
			want  HealthStatus
		}{
			{name: "active", state: lambdaTypes.StateActive, want: HealthStatusHealthy},
			{name: "pending", state: lambdaTypes.StatePending, want: HealthStatusWarning},
			{name: "failed", state: lambdaTypes.StateFailed, want: HealthStatusCritical},
		} {
			t.Run(tc.name, func(t *testing.T) {
				hm := &HealthMonitor{
					monitor: pm,
					lambdaClient: &stubLambda{out: &lambda.GetFunctionConfigurationOutput{
						State:      tc.state,
						Runtime:    lambdaTypes.RuntimeGo1x,
						MemorySize: aws.Int32(128),
						Timeout:    aws.Int32(10),
					}},
					healthStatus: make(map[string]*ComponentHealth),
				}

				require.NoError(t, hm.CheckLambdaHealth(context.Background(), "fn"))
				status := hm.GetHealthStatus()
				require.Contains(t, status, "lambda.fn")
				assert.Equal(t, tc.want, status["lambda.fn"].Status)
				assert.Equal(t, string(tc.state), status["lambda.fn"].Metadata["state"])
			})
		}
	})

	t.Run("client_error_sets_critical", func(t *testing.T) {
		hm := &HealthMonitor{
			monitor:      pm,
			lambdaClient: &stubLambda{err: errors.New("get failed")},
			healthStatus: make(map[string]*ComponentHealth),
		}

		require.Error(t, hm.CheckLambdaHealth(context.Background(), "fn"))
		status := hm.GetHealthStatus()
		require.Contains(t, status, "lambda.fn")
		assert.Equal(t, HealthStatusCritical, status["lambda.fn"].Status)
	})
}

func TestHealthMonitorCheckSQSHealth(t *testing.T) {
	pm, _ := newTestPerformanceMonitor()

	t.Run("status_from_depth", func(t *testing.T) {
		tests := []struct {
			name                 string
			visible, invisible   string
			delayed              string
			want                 HealthStatus
			expectedTotalMessage int
		}{
			{name: "healthy", visible: "10", invisible: "0", delayed: "0", want: HealthStatusHealthy, expectedTotalMessage: 10},
			{name: "warning", visible: "1001", invisible: "0", delayed: "0", want: HealthStatusWarning, expectedTotalMessage: 1001},
			{name: "critical", visible: "10001", invisible: "0", delayed: "0", want: HealthStatusCritical, expectedTotalMessage: 10001},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				hm := &HealthMonitor{
					monitor: pm,
					sqsClient: &stubSQS{out: &sqs.GetQueueAttributesOutput{Attributes: map[string]string{
						"ApproximateNumberOfMessages":           tc.visible,
						"ApproximateNumberOfMessagesNotVisible": tc.invisible,
						"ApproximateNumberOfMessagesDelayed":    tc.delayed,
					}}},
					healthStatus: make(map[string]*ComponentHealth),
				}

				require.NoError(t, hm.CheckSQSHealth(context.Background(), "queue-url"))
				status := hm.GetHealthStatus()
				require.Contains(t, status, "sqs.queue-url")
				assert.Equal(t, tc.want, status["sqs.queue-url"].Status)
				assert.Equal(t, tc.expectedTotalMessage, status["sqs.queue-url"].Metadata["totalMessages"])
			})
		}
	})

	t.Run("client_error_sets_critical", func(t *testing.T) {
		hm := &HealthMonitor{
			monitor:      pm,
			sqsClient:    &stubSQS{err: errors.New("sqs failed")},
			healthStatus: make(map[string]*ComponentHealth),
		}

		require.Error(t, hm.CheckSQSHealth(context.Background(), "queue-url"))
		status := hm.GetHealthStatus()
		require.Contains(t, status, "sqs.queue-url")
		assert.Equal(t, HealthStatusCritical, status["sqs.queue-url"].Status)
	})
}

func TestHealthMonitorRunHealthChecksRecordsOverallMetric(t *testing.T) {
	pm, cw := newTestPerformanceMonitor()

	hm := &HealthMonitor{
		monitor:      pm,
		tableChecker: &stubTableExistsChecker{exists: true},
		lambdaClient: &stubLambda{out: &lambda.GetFunctionConfigurationOutput{
			State:      lambdaTypes.StateActive,
			Runtime:    lambdaTypes.RuntimeGo1x,
			MemorySize: aws.Int32(128),
			Timeout:    aws.Int32(10),
		}},
		sqsClient: &stubSQS{out: &sqs.GetQueueAttributesOutput{Attributes: map[string]string{
			"ApproximateNumberOfMessages":           "10",
			"ApproximateNumberOfMessagesNotVisible": "0",
			"ApproximateNumberOfMessagesDelayed":    "0",
		}}},
		healthStatus: make(map[string]*ComponentHealth),
	}

	hm.runHealthChecks(context.Background(), []HealthCheckComponent{
		{Type: "dynamodb", Identifier: "table"},
		{Type: "lambda", Identifier: "fn"},
		{Type: "sqs", Identifier: "queue-url"},
	})

	cw.mu.Lock()
	defer cw.mu.Unlock()
	require.NotEmpty(t, cw.putMetricDataInputs)

	// The final call should be the SystemHealth metric.
	last := cw.putMetricDataInputs[len(cw.putMetricDataInputs)-1]
	require.NotNil(t, last)
	require.Len(t, last.MetricData, 1)
	require.NotNil(t, last.MetricData[0].MetricName)
	assert.Equal(t, "SystemHealth", *last.MetricData[0].MetricName)
}
