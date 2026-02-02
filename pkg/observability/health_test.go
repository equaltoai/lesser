package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type tableExistsStub struct {
	exists bool
	err   error
	calls int
}

func (s *tableExistsStub) TableExists(_ string) (bool, error) {
	s.calls++
	return s.exists, s.err
}

type sqsAttributesStub struct {
	out   *sqs.GetQueueAttributesOutput
	err   error
	calls int
}

func (s *sqsAttributesStub) GetQueueAttributes(_ context.Context, _ *sqs.GetQueueAttributesInput, _ ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	s.calls++
	return s.out, s.err
}

func TestHealthChecker_Handlers(t *testing.T) {
	logger := zaptest.NewLogger(t)

	dynamo := &tableExistsStub{exists: true}
	sqsStub := &sqsAttributesStub{
		out: &sqs.GetQueueAttributesOutput{
			Attributes: map[string]string{
				"ApproximateNumberOfMessages":           "10",
				"ApproximateNumberOfMessagesNotVisible": "2",
				"ApproximateNumberOfMessagesDelayed":    "1",
			},
		},
	}

	hc := &HealthChecker{
		logger:       logger,
		tableChecker: dynamo,
		sqsClient:    sqsStub,
		service:      "svc",
		version:      "v1",
		lastChecks:   make(map[string]HealthCheck),
		config: &HealthConfig{
			TableName:        "tbl",
			QueueURL:         "https://example.com/queue",
			CheckTimeout:     time.Second,
			CacheTimeout:     time.Minute,
			DependencyChecks: true,
		},
	}

	t.Run("liveness", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, HealthEndpointLive, nil)
		rec := httptest.NewRecorder()
		hc.LivenessHandler(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, json.Valid(rec.Body.Bytes()))
	})

	t.Run("readiness_ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, HealthEndpointReady, nil)
		rec := httptest.NewRecorder()
		hc.ReadinessHandler(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp HealthResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, HealthStatusHealthy, resp.Status)
		assert.Len(t, resp.Checks, 2)
	})

	t.Run("readiness_failure_is_503", func(t *testing.T) {
		hc.lastChecks = make(map[string]HealthCheck)
		dynamo.err = errors.New("dynamo down")
		req := httptest.NewRequest(http.MethodGet, HealthEndpointReady, nil)
		rec := httptest.NewRecorder()
		hc.ReadinessHandler(rec, req)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		dynamo.err = nil
	})

	t.Run("detailed_includes_summary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, HealthEndpointDetailed, nil)
		rec := httptest.NewRecorder()
		hc.DetailedHandler(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp HealthResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotNil(t, resp.Summary)
		assert.GreaterOrEqual(t, resp.Summary["total_checks"].(float64), float64(1))
	})
}

func TestHealthChecker_CacheAndHelpers(t *testing.T) {
	logger := zaptest.NewLogger(t)

	dynamo := &tableExistsStub{exists: true}

	hc := &HealthChecker{
		logger:       logger,
		tableChecker: dynamo,
		service:      "svc",
		version:      "v1",
		lastChecks:   make(map[string]HealthCheck),
		config: &HealthConfig{
			TableName:        "tbl",
			CheckTimeout:     time.Second,
			CacheTimeout:     time.Hour,
			DependencyChecks: true,
		},
	}

	check1 := hc.checkDynamoDB(context.Background())
	check2 := hc.checkDynamoDB(context.Background())
	assert.Equal(t, check1.Status, check2.Status)
	assert.Equal(t, 1, dynamo.calls)

	hc.lastChecks["dynamodb"] = HealthCheck{LastCheck: time.Now().Add(-hc.config.CacheTimeout - time.Second)}
	_, ok := hc.getCachedCheck("dynamodb")
	assert.False(t, ok)

	assert.Equal(t, 0, parseInt("not-a-number"))
	assert.Equal(t, 123, parseInt("123"))
}

func TestNewHealthChecker_DefaultConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hc := NewHealthChecker(logger, aws.Config{}, "svc", "v1", nil)
	require.NotNil(t, hc)
	require.NotNil(t, hc.config)
	assert.True(t, hc.config.DependencyChecks)
}
