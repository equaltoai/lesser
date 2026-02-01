package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type sequenceChecker struct {
	componentType string
	results       []*ComponentHealthResult
	errs          []error
	calls         int
}

func (s *sequenceChecker) GetType() string { return s.componentType }

func (s *sequenceChecker) Check(_ context.Context, identifier string) (*ComponentHealthResult, error) {
	idx := s.calls
	s.calls++

	var res *ComponentHealthResult
	if idx < len(s.results) {
		res = s.results[idx]
	}
	if res == nil {
		res = &ComponentHealthResult{
			Component: identifier,
			Type:      s.componentType,
			Status:    HealthStatusHealthy,
			CheckTime: time.Now(),
		}
	}
	var err error
	if idx < len(s.errs) {
		err = s.errs[idx]
	}
	return res, err
}

func TestNewServerlessHealthMonitorRegistersBuiltIns(t *testing.T) {
	t.Parallel()

	monitor := NewServerlessHealthMonitor(new(mocks.MockDB), zap.NewNop())
	require.NotNil(t, monitor)
	assert.Contains(t, monitor.checkers, "dynamodb")
	assert.Contains(t, monitor.checkers, "lambda")
	assert.Contains(t, monitor.checkers, "sqs")
}

func TestServerlessHealthMonitorDetermineOverallStatus(t *testing.T) {
	t.Parallel()

	monitor := &ServerlessHealthMonitor{logger: zap.NewNop()}

	assert.Equal(t, HealthStatusUnknown, monitor.determineOverallStatus(nil))
	assert.Equal(t, HealthStatusCritical, monitor.determineOverallStatus([]ComponentHealthResult{{Status: HealthStatusCritical}}))
	assert.Equal(t, HealthStatusWarning, monitor.determineOverallStatus([]ComponentHealthResult{{Status: HealthStatusWarning}}))
	assert.Equal(t, HealthStatusUnknown, monitor.determineOverallStatus([]ComponentHealthResult{{Status: HealthStatusUnknown}}))
	assert.Equal(t, HealthStatusHealthy, monitor.determineOverallStatus([]ComponentHealthResult{{Status: HealthStatusHealthy}}))
}

func TestServerlessHealthMonitorCheckComponentUnsupported(t *testing.T) {
	t.Parallel()

	monitor := &ServerlessHealthMonitor{
		logger:   zap.NewNop(),
		checkers: make(map[string]ComponentChecker),
	}

	result := monitor.checkComponent(context.Background(), ComponentCheckConfig{Type: "nope", Identifier: "id"}, HealthCheckOptions{})
	require.NotNil(t, result)
	assert.Equal(t, HealthStatusUnknown, result.Status)
	assert.Contains(t, result.Error, "unsupported component type")
}

func TestServerlessHealthMonitorCheckComponentRetrySuccess(t *testing.T) {
	monitor := &ServerlessHealthMonitor{
		logger:   zap.NewNop(),
		checkers: make(map[string]ComponentChecker),
	}

	checker := &sequenceChecker{
		componentType: "dynamodb",
		results: []*ComponentHealthResult{
			{Component: "id", Type: "dynamodb", Status: HealthStatusCritical, CheckTime: time.Now(), Error: "timeout"},
			{Component: "id", Type: "dynamodb", Status: HealthStatusHealthy, CheckTime: time.Now()},
		},
		errs: []error{
			errors.New("failed"),
			nil,
		},
	}
	monitor.RegisterChecker(checker)

	start := time.Now()
	result := monitor.checkComponent(context.Background(), ComponentCheckConfig{Type: "dynamodb", Identifier: "id", Name: "Friendly"}, HealthCheckOptions{RetryAttempts: 1})
	require.NotNil(t, result)
	assert.Equal(t, HealthStatusHealthy, result.Status)
	assert.Equal(t, 1, result.RetryAttempts)
	assert.Equal(t, "Friendly", result.Metadata["friendly_name"])
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)
}

func TestServerlessHealthMonitorCheckComponentTimeoutConfirm(t *testing.T) {
	monitor := &ServerlessHealthMonitor{
		logger:   zap.NewNop(),
		checkers: make(map[string]ComponentChecker),
	}
	monitor.RegisterChecker(&sequenceChecker{
		componentType: "dynamodb",
		results: []*ComponentHealthResult{
			{Component: "id", Type: "dynamodb", Status: HealthStatusCritical, CheckTime: time.Now(), Error: "boom"},
		},
		errs: []error{errors.New("boom")},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := monitor.checkComponent(ctx, ComponentCheckConfig{Type: "dynamodb", Identifier: "id"}, HealthCheckOptions{RetryAttempts: 1})
	require.NotNil(t, result)
	assert.Equal(t, HealthStatusCritical, result.Status)
	assert.Contains(t, result.Error, "timeout")
}

func TestServerlessHealthMonitorProcessHealthCheckEvent(t *testing.T) {
	logger := zap.NewNop()

	t.Run("invalid_event_returns_error", func(t *testing.T) {
		monitor := NewServerlessHealthMonitor(new(mocks.MockDB), logger)
		_, err := monitor.ProcessHealthCheckEvent(context.Background(), HealthCheckEvent{})
		require.Error(t, err)
	})

	t.Run("stores_results_and_populates_summary", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

		// First component store succeeds, second fails (to hit log-warn path).
		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("Create").Return(errors.New("create failed")).Once()

		monitor := NewServerlessHealthMonitor(mockDB, logger)
		monitor.RegisterChecker(&sequenceChecker{
			componentType: "dynamodb",
			results:       []*ComponentHealthResult{{Component: "table", Type: "dynamodb", Status: HealthStatusHealthy, CheckTime: time.Now()}},
		})
		monitor.RegisterChecker(&sequenceChecker{
			componentType: "lambda",
			results:       []*ComponentHealthResult{{Component: "fn", Type: "lambda", Status: HealthStatusCritical, CheckTime: time.Now(), Error: "timeout"}},
			errs:          []error{errors.New("boom")},
		})

		event := HealthCheckEvent{
			Action: "check_health",
			Components: []ComponentCheckConfig{
				{Type: "dynamodb", Identifier: "table", Name: "Main DB"},
				{Type: "lambda", Identifier: "fn", Name: "API"},
			},
			Options: HealthCheckOptions{
				StoreResults:   true,
				PublishMetrics: true, // metricsPublisher nil -> log fallback
				TimeoutSeconds: 1,
				RetryAttempts:  0,
			},
		}

		resp, err := monitor.ProcessHealthCheckEvent(context.Background(), event)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, 2, resp.Summary.TotalComponents)
		assert.Equal(t, 1, resp.Summary.HealthyComponents)
		assert.Equal(t, 1, resp.Summary.CriticalComponents)
		assert.Equal(t, 1, resp.Summary.FailedChecks)
		assert.Equal(t, HealthStatusCritical, resp.OverallStatus)
		require.Len(t, resp.ComponentResults, 2)
		assert.Equal(t, "Main DB", resp.ComponentResults[0].Metadata["friendly_name"])

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestServerlessHealthMonitorPublishComponentMetrics(t *testing.T) {
	t.Parallel()

	metrics := &CloudWatchMetrics{
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

	monitor := &ServerlessHealthMonitor{
		logger:           zap.NewNop(),
		requestID:        "req",
		metricsPublisher: metrics,
	}

	monitor.publishComponentMetrics(context.Background(), &ComponentHealthResult{
		Component: "table",
		Type:      "dynamodb",
		Status:    HealthStatusHealthy,
		CheckTime: time.Now(),
		LatencyMs: 12,
		Metadata:  map[string]interface{}{"friendly_name": "Main DB"},
	})

	monitor.publishComponentMetrics(context.Background(), &ComponentHealthResult{
		Component:     "fn",
		Type:          "lambda",
		Status:        HealthStatusCritical,
		CheckTime:     time.Now(),
		LatencyMs:     42,
		Error:         "permission denied",
		RetryAttempts: 2,
	})

	assert.GreaterOrEqual(t, metrics.buffer.Size(), 6)
}

func TestServerlessHealthMonitorStoredQueries(t *testing.T) {
	logger := zap.NewNop()

	t.Run("GetStoredResults_errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", "gsi1PK", "=", mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", 1).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("query failed"))

		monitor := NewServerlessHealthMonitor(mockDB, logger)
		_, err := monitor.GetStoredResults(context.Background(), "dynamodb", "table", 1)
		require.Error(t, err)
	})

	t.Run("GetComponentHistory_errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
		mockQuery.On("Limit", 1).Return(mockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("query failed"))

		monitor := NewServerlessHealthMonitor(mockDB, logger)
		_, err := monitor.GetComponentHistory(context.Background(), "dynamodb", "table", 1)
		require.Error(t, err)
	})
}

func TestServerlessHealthMonitorCreateHealthCheckSummary(t *testing.T) {
	logger := zap.NewNop()

	results := []ComponentHealthResult{
		{Status: HealthStatusHealthy, LatencyMs: 10},
		{Status: HealthStatusCritical, LatencyMs: 20},
	}

	t.Run("query_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("query failed"))

		monitor := NewServerlessHealthMonitor(mockDB, logger)
		require.Error(t, monitor.CreateHealthCheckSummary(context.Background(), results))
	})

	t.Run("not_found_creates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		firstQuery := new(mocks.MockQuery)
		saveQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(firstQuery).Once()
		firstQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(firstQuery)
		firstQuery.On("First", mock.Anything).Return(storage.ErrNotFound)

		mockDB.On("Model", mock.Anything).Return(saveQuery).Once()
		saveQuery.On("Create").Return(nil)

		monitor := NewServerlessHealthMonitor(mockDB, logger)
		require.NoError(t, monitor.CreateHealthCheckSummary(context.Background(), results))
	})

	t.Run("existing_updates", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		firstQuery := new(mocks.MockQuery)
		saveQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(firstQuery).Once()
		firstQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(firstQuery)
		firstQuery.On("First", mock.Anything).Return(nil)

		mockDB.On("Model", mock.Anything).Return(saveQuery).Once()
		saveQuery.On("Update", mock.Anything).Return(nil)

		monitor := NewServerlessHealthMonitor(mockDB, logger)
		require.NoError(t, monitor.CreateHealthCheckSummary(context.Background(), results))
	})

	t.Run("storeHealthCheckResult_populates_model", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Create").Return(nil)

		monitor := NewServerlessHealthMonitor(mockDB, logger)
		err := monitor.storeHealthCheckResult(context.Background(), &ComponentHealthResult{
			Component: "table",
			Type:      "dynamodb",
			Status:    HealthStatusCritical,
			CheckTime: time.Now(),
			LatencyMs: 12,
			Error:     "boom",
			Metadata:  map[string]interface{}{"k": "v"},
		})
		require.NoError(t, err)
	})
}

func TestServerlessHealthMonitorRegisterCheckerOverrides(t *testing.T) {
	t.Parallel()

	monitor := NewServerlessHealthMonitor(new(mocks.MockDB), zap.NewNop())
	original := monitor.checkers["dynamodb"]

	custom := NewCustomChecker("dynamodb", func(ctx context.Context, identifier string) (*ComponentHealthResult, error) {
		return &ComponentHealthResult{Component: identifier, Type: "dynamodb", Status: HealthStatusHealthy, CheckTime: time.Now()}, nil
	})
	monitor.RegisterChecker(custom)

	assert.NotEqual(t, original, monitor.checkers["dynamodb"])
}

func TestServerlessHealthMonitorStoreHealthCheckResultUsesModels(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Run(func(args mock.Arguments) {
		_, ok := args.Get(0).(*models.HealthCheckResult)
		require.True(t, ok)
	}).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	monitor := NewServerlessHealthMonitor(mockDB, zap.NewNop())
	require.NoError(t, monitor.storeHealthCheckResult(context.Background(), &ComponentHealthResult{
		Component: "table",
		Type:      "dynamodb",
		Status:    HealthStatusHealthy,
		CheckTime: time.Now(),
		LatencyMs: 5,
	}))
}
