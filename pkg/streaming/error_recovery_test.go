package streaming

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/pay-theory/lift/pkg/streamer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubStreamerClient struct {
	post func(ctx context.Context, connectionID string, data []byte) error
}

func (c *stubStreamerClient) PostToConnection(ctx context.Context, connectionID string, data []byte) error {
	if c.post == nil {
		return nil
	}
	return c.post(ctx, connectionID, data)
}

func (c *stubStreamerClient) DeleteConnection(context.Context, string) error { return nil }
func (c *stubStreamerClient) GetConnection(context.Context, string) (*streamer.ConnectionInfo, error) {
	return nil, nil
}

type stubJobQueue struct {
	queue func(ctx context.Context, queueName string, messageBody interface{}, delaySeconds int32) error
}

func (q *stubJobQueue) QueueDelayedJob(ctx context.Context, queueName string, messageBody interface{}, delaySeconds int32) error {
	if q.queue == nil {
		return nil
	}
	return q.queue(ctx, queueName, messageBody, delaySeconds)
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cb := NewCircuitBreaker(2, 10*time.Millisecond)
	assert.Equal(t, CircuitBreakerClosed, cb.GetState())
	assert.True(t, cb.CanExecute())

	cb.RecordFailure()
	assert.Equal(t, CircuitBreakerClosed, cb.GetState())

	cb.RecordFailure()
	assert.Equal(t, CircuitBreakerOpen, cb.GetState())
	assert.False(t, cb.CanExecute())

	cb.lastFailureTime = time.Now().Add(-cb.timeout - time.Millisecond)
	assert.True(t, cb.CanExecute())
	assert.Equal(t, CircuitBreakerHalfOpen, cb.GetState())

	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordSuccess()
	assert.Equal(t, CircuitBreakerClosed, cb.GetState())
}

func TestErrorRecoveryManager_ShouldAttemptRecovery(t *testing.T) {
	erm := NewErrorRecoveryManager(nil, nil, nil, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: time.Second,
		MaxRetryDelay:  time.Minute,
		JitterFactor:   0,
		EnableBackoff:  true,
	})

	conn := &models.WebSocketConnection{RetryCount: 1, MaxRetries: 1}
	assert.False(t, erm.shouldAttemptRecovery(conn, errors.New("boom")))

	permanent := appErrors.NewAppError(appErrors.CodeUnauthorized, appErrors.CategoryAuth, "nope")
	conn = &models.WebSocketConnection{State: models.ConnectionStateConnected, RetryCount: 0, MaxRetries: 5}
	assert.False(t, erm.shouldAttemptRecovery(conn, permanent))

	conn = &models.WebSocketConnection{State: models.ConnectionStateConnected, RetryCount: 0, MaxRetries: 5}
	assert.False(t, erm.shouldAttemptRecovery(conn, errors.New("unauthorized: token missing")))

	conn = &models.WebSocketConnection{State: models.ConnectionStateError, RetryCount: 0, MaxRetries: 5}
	conn.Metrics.ConnectionQuality = 0.2
	assert.False(t, erm.shouldAttemptRecovery(conn, errors.New("transient")))

	conn = &models.WebSocketConnection{State: models.ConnectionStateConnected, RetryCount: 0, MaxRetries: 5}
	conn.Metrics.ConnectionQuality = 0.9
	assert.True(t, erm.shouldAttemptRecovery(conn, errors.New("transient")))
}

func TestErrorRecoveryManager_CalculateRetryDelay(t *testing.T) {
	erm := &ErrorRecoveryManager{
		baseRetryDelay: time.Second,
		maxRetryDelay:  10 * time.Second,
		jitterFactor:   0,
		enableBackoff:  true,
	}

	assert.Equal(t, 4*time.Second, erm.calculateRetryDelay(3))

	erm.enableBackoff = false
	assert.Equal(t, time.Second, erm.calculateRetryDelay(99))

	erm.enableBackoff = true
	erm.baseRetryDelay = 10 * time.Second
	erm.maxRetryDelay = 15 * time.Second
	assert.Equal(t, 15*time.Second, erm.calculateRetryDelay(3))
}

func TestErrorRecoveryManager_ScheduleRetryJob(t *testing.T) {
	t.Run("requires job queue", func(t *testing.T) {
		erm := &ErrorRecoveryManager{jobQueue: nil}
		err := erm.scheduleRetryJob(context.Background(), "c1", 1, time.Second, errors.New("boom"))
		require.Error(t, err)
	})

	t.Run("clamps delay seconds and wraps errors", func(t *testing.T) {
		var gotDelay int32
		q := &stubJobQueue{
			queue: func(_ context.Context, queueName string, _ interface{}, delaySeconds int32) error {
				assert.Equal(t, "streaming-retry", queueName)
				gotDelay = delaySeconds
				return errors.New("sqs down")
			},
		}

		erm := &ErrorRecoveryManager{jobQueue: q, logger: zap.NewNop()}
		err := erm.scheduleRetryJob(context.Background(), "c1", 1, 1000*time.Second, errors.New("boom"))
		require.Error(t, err)
		assert.Equal(t, int32(900), gotDelay)

		appErr, ok := appErrors.AsAppError(err)
		require.True(t, ok)
		assert.Equal(t, appErrors.CodeSQSProcessingFailed, appErr.Code)
	})
}

func TestErrorRecoveryManager_MarkConnectionClosedWithError(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	conn := &models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateConnected}

	repo.On("GetConnection", mock.Anything, "c1").Return(conn, nil).Once()
	repo.On("UpdateConnection", mock.Anything, mock.MatchedBy(func(c *models.WebSocketConnection) bool {
		return c.State == models.ConnectionStateClosed && c.CloseCode == 1011 && c.CloseReason != ""
	})).Return(nil).Once()

	erm := &ErrorRecoveryManager{connRepo: repo, logger: zap.NewNop()}
	appErr := appErrors.NewAppError(appErrors.CodeInternal, appErrors.CategoryStreaming, "nope")

	err := erm.markConnectionClosedWithError(context.Background(), "c1", appErr)
	require.Error(t, err)
}

func TestErrorRecoveryManager_HandleConnectionError_NoRecovery(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	conn := &models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateConnected, RetryCount: 5, MaxRetries: 5}

	repo.On("GetConnection", mock.Anything, "c1").Return(conn, nil).Twice()
	repo.On("RecordConnectionError", mock.Anything, "c1", "boom").Return(nil).Once()
	repo.On("UpdateConnection", mock.Anything, mock.Anything).Return(nil).Once()

	erm := NewErrorRecoveryManager(repo, nil, &stubJobQueue{}, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: 0,
		MaxRetryDelay:  time.Second,
		JitterFactor:   0,
		EnableBackoff:  true,
	})

	err := erm.HandleConnectionError(context.Background(), "c1", errors.New("boom"))
	require.Error(t, err)
}

func TestErrorRecoveryManager_HandleConnectionError_AttemptRecovery_SchedulesJob(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	conn := &models.WebSocketConnection{
		ConnectionID: "c1",
		UserID:       "u1",
		State:        models.ConnectionStateConnected,
		MaxRetries:   5,
	}
	conn.Metrics.ConnectionQuality = 0.9

	repo.On("GetConnection", mock.Anything, "c1").Return(conn, nil).Once()
	repo.On("RecordConnectionError", mock.Anything, "c1", "boom").Return(nil).Once()
	repo.On("UpdateConnection", mock.Anything, mock.MatchedBy(func(c *models.WebSocketConnection) bool {
		return c.State == models.ConnectionStateError && c.RetryCount == 1
	})).Return(nil).Once()

	var queued bool
	queue := &stubJobQueue{
		queue: func(_ context.Context, queueName string, _ interface{}, _ int32) error {
			assert.Equal(t, "streaming-retry", queueName)
			queued = true
			return nil
		},
	}

	erm := NewErrorRecoveryManager(repo, nil, queue, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: 0,
		MaxRetryDelay:  time.Second,
		JitterFactor:   0,
		EnableBackoff:  false,
	})

	err := erm.HandleConnectionError(context.Background(), "c1", errors.New("boom"))
	require.NoError(t, err)
	assert.True(t, queued)
}

func TestErrorRecoveryManager_ExecuteRecovery_SuccessAndFailure(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	api := &stubStreamerClient{}
	postCalls := 0
	api.post = func(_ context.Context, _ string, _ []byte) error {
		postCalls++
		if postCalls == 1 {
			return nil // ping succeeds
		}
		return nil // sync succeeds
	}

	queue := &stubJobQueue{}
	erm := NewErrorRecoveryManager(repo, api, queue, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: 0,
		MaxRetryDelay:  time.Second,
		JitterFactor:   0,
		EnableBackoff:  true,
	})

	conn := &models.WebSocketConnection{
		ConnectionID: "c1",
		UserID:       "u1",
		State:        models.ConnectionStateError,
		MaxRetries:   5,
		Streams:      []string{"public"},
	}
	conn.Metrics.ConnectionQuality = 0.9

	repo.On("GetConnection", mock.Anything, "c1").Return(conn, nil).Once()
	repo.On("UpdateConnection", mock.Anything, mock.Anything).Return(nil).Times(2)

	erm.executeRecovery(context.Background(), "c1")
	assert.Equal(t, 2, postCalls)

	// Skip branch when no longer in error state.
	healthy := &models.WebSocketConnection{ConnectionID: "c2", State: models.ConnectionStateConnected}
	repo.On("GetConnection", mock.Anything, "c2").Return(healthy, nil).Once()
	erm.executeRecovery(context.Background(), "c2")
}

func TestErrorRecoveryManager_ProcessRetryJob_Branches(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	erm := NewErrorRecoveryManager(repo, nil, &stubJobQueue{}, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: 0,
		MaxRetryDelay:  time.Second,
		JitterFactor:   0,
		EnableBackoff:  true,
	})

	// Missing connection: skip.
	repo.On("GetConnection", mock.Anything, "missing").Return(nil, errors.New("not found")).Once()
	require.NoError(t, erm.ProcessRetryJob(context.Background(), RetryJobMessage{ConnectionID: "missing"}))

	// Not in error state: skip.
	notError := &models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateConnected}
	repo.On("GetConnection", mock.Anything, "c1").Return(notError, nil).Once()
	require.NoError(t, erm.ProcessRetryJob(context.Background(), RetryJobMessage{ConnectionID: "c1", RetryCount: 0}))

	// Stale retry job: skip.
	errorConn := &models.WebSocketConnection{ConnectionID: "c2", State: models.ConnectionStateError, RetryCount: 2}
	repo.On("GetConnection", mock.Anything, "c2").Return(errorConn, nil).Once()
	require.NoError(t, erm.ProcessRetryJob(context.Background(), RetryJobMessage{ConnectionID: "c2", RetryCount: 1}))

	// Executes recovery; second GetConnection returns non-error to exit early.
	errorConn = &models.WebSocketConnection{ConnectionID: "c3", State: models.ConnectionStateError, RetryCount: 0}
	repo.On("GetConnection", mock.Anything, "c3").Return(errorConn, nil).Once()
	repo.On("GetConnection", mock.Anything, "c3").Return(&models.WebSocketConnection{ConnectionID: "c3", State: models.ConnectionStateConnected}, nil).Once()
	require.NoError(t, erm.ProcessRetryJob(context.Background(), RetryJobMessage{ConnectionID: "c3", RetryCount: 0}))
}

func TestErrorRecoveryManager_ExecuteRecovery_FailureSchedulesRetry(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	api := &stubStreamerClient{
		post: func(context.Context, string, []byte) error { return errors.New("dead") },
	}

	var queued bool
	queue := &stubJobQueue{
		queue: func(_ context.Context, queueName string, _ interface{}, _ int32) error {
			queued = true
			require.Equal(t, "streaming-retry", queueName)
			return nil
		},
	}

	erm := NewErrorRecoveryManager(repo, api, queue, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: 0,
		MaxRetryDelay:  time.Second,
		JitterFactor:   0,
		EnableBackoff:  false,
	})

	conn := &models.WebSocketConnection{
		ConnectionID: "c1",
		UserID:       "u1",
		State:        models.ConnectionStateError,
		MaxRetries:   5,
		RetryCount:   0,
	}
	conn.Metrics.ConnectionQuality = 0.9

	repo.On("GetConnection", mock.Anything, "c1").Return(conn, nil).Once()
	repo.On("UpdateConnection", mock.Anything, mock.Anything).Return(nil).Twice()

	erm.executeRecovery(context.Background(), "c1")
	assert.True(t, queued)
}

func TestErrorRecoveryManager_HealthCheckAndStats(t *testing.T) {
	cfg := DefaultErrorRecoveryConfig()
	require.NotNil(t, cfg)

	repo := testmocks.NewMockStreamingConnectionRepository()

	postCalls := 0
	api := &stubStreamerClient{
		post: func(context.Context, string, []byte) error {
			postCalls++
			return nil
		},
	}

	erm := NewErrorRecoveryManager(repo, api, &stubJobQueue{}, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: 0,
		MaxRetryDelay:  time.Second,
		JitterFactor:   0,
		EnableBackoff:  true,
	})

	conn := &models.WebSocketConnection{
		ConnectionID: "c1",
		State:        models.ConnectionStateConnected,
		MaxRetries:   5,
	}
	conn.Metrics.ConnectionQuality = 0.8
	conn.Metrics.MessagesSent = 10
	conn.Metrics.ErrorCount = 2

	repo.On("GetConnection", mock.Anything, "c1").Return(conn, nil).Once()

	result, err := erm.PerformHealthCheck(context.Background(), "c1")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, postCalls > 0)
	assert.True(t, result.LatencyMs >= 0)
	assert.Equal(t, float64(20), erm.calculatePacketLoss(conn))

	stats := erm.GetRecoveryStats()
	assert.Equal(t, 5, stats["max_retries"])
	assert.NotNil(t, stats["circuit_breaker"])

	// Simulated path when API client missing.
	erm.apiClient = nil
	_ = erm.validateConnectionHealth(context.Background(), conn)
}

func TestErrorRecoveryManager_ConstructorsAndErrorPaths(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	erm := NewErrorRecoveryManager(repo, nil, nil, nil, nil)
	require.NotNil(t, erm)
	require.NotNil(t, erm.logger)
	require.NotNil(t, erm.circuitBreaker)

	repo.On("GetConnection", mock.Anything, "missing").Return(nil, errors.New("db down")).Once()
	err := erm.HandleConnectionError(context.Background(), "missing", errors.New("boom"))
	require.Error(t, err)

	// Circuit breaker prevents recovery.
	conn := &models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateConnected, MaxRetries: 5}
	conn.Metrics.ConnectionQuality = 0.9

	erm.circuitBreaker.state = CircuitBreakerOpen
	erm.circuitBreaker.lastFailureTime = time.Now()

	repo.On("GetConnection", mock.Anything, "c1").Return(conn, nil).Twice()
	repo.On("RecordConnectionError", mock.Anything, "c1", "boom").Return(nil).Once()
	repo.On("UpdateConnection", mock.Anything, mock.Anything).Return(nil).Once()
	err = erm.HandleConnectionError(context.Background(), "c1", errors.New("boom"))
	require.Error(t, err)
}

func TestErrorRecoveryManager_AttemptRecovery_FallbackGoroutine(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	erm := NewErrorRecoveryManager(repo, nil, nil, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: 0,
		MaxRetryDelay:  time.Second,
		JitterFactor:   0,
		EnableBackoff:  false,
	})

	conn := &models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateConnected, MaxRetries: 5}
	conn.Metrics.ConnectionQuality = 0.9

	repo.On("UpdateConnection", mock.Anything, mock.Anything).Return(nil).Once()

	gotRecovery := make(chan struct{}, 1)
	repo.On("GetConnection", mock.Anything, "c1").Return(&models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateConnected}, nil).Run(func(mock.Arguments) {
		select {
		case gotRecovery <- struct{}{}:
		default:
		}
	}).Once()

	require.NoError(t, erm.attemptRecovery(context.Background(), conn, errors.New("boom")))

	require.Eventually(t, func() bool {
		select {
		case <-gotRecovery:
			return true
		default:
			return false
		}
	}, time.Second, 5*time.Millisecond)
}

func TestErrorRecoveryManager_HandleRecoveryFailure_MaxRetriesAndMarkCloseErrors(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()
	erm := NewErrorRecoveryManager(repo, nil, &stubJobQueue{}, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: 0,
		MaxRetryDelay:  time.Second,
		JitterFactor:   0,
		EnableBackoff:  true,
	})

	conn := &models.WebSocketConnection{ConnectionID: "c1", State: models.ConnectionStateError, RetryCount: 1, MaxRetries: 1}
	repo.On("GetConnection", mock.Anything, "c1").Return(nil, errors.New("boom")).Once()
	erm.handleRecoveryFailure(context.Background(), conn)

	repo.On("GetConnection", mock.Anything, "c2").Return(&models.WebSocketConnection{ConnectionID: "c2"}, nil).Once()
	repo.On("UpdateConnection", mock.Anything, mock.Anything).Return(errors.New("boom")).Once()
	_ = erm.markConnectionClosedWithError(context.Background(), "c2", appErrors.NewAppError(appErrors.CodeInternal, appErrors.CategoryStreaming, "x"))
}

func TestErrorRecoveryManager_ResyncAndHealthCheck_Branches(t *testing.T) {
	repo := testmocks.NewMockStreamingConnectionRepository()

	api := &stubStreamerClient{
		post: func(_ context.Context, _ string, data []byte) error {
			// Fail any sync messages.
			if bytes.Contains(data, []byte("connection_sync")) {
				return errors.New("boom")
			}
			return nil
		},
	}

	erm := NewErrorRecoveryManager(repo, api, &stubJobQueue{}, zap.NewNop(), &ErrorRecoveryConfig{
		MaxRetries:     5,
		BaseRetryDelay: 0,
		MaxRetryDelay:  time.Second,
		JitterFactor:   0,
		EnableBackoff:  true,
	})

	// ResynchronizeConnection error branch.
	conn := &models.WebSocketConnection{ConnectionID: "c1", UserID: "u1", Streams: []string{"public"}}
	err := erm.ResynchronizeConnection(context.Background(), conn)
	require.Error(t, err)

	// PerformHealthCheck disconnect branch.
	repo.On("GetConnection", mock.Anything, "c2").Return(&models.WebSocketConnection{ConnectionID: "c2", State: models.ConnectionStateConnected}, nil).Once()
	api.post = func(context.Context, string, []byte) error { return errors.New("dead") }
	result, err := erm.PerformHealthCheck(context.Background(), "c2")
	require.NoError(t, err)
	assert.Equal(t, "disconnect", result.RecommendedAction)

	// Packet loss zero when no sent messages.
	assert.Equal(t, float64(0), erm.calculatePacketLoss(&models.WebSocketConnection{}))
}
