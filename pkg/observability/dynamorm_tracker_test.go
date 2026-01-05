package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type metricsRecorderStub struct {
	calls []struct {
		operation  string
		table      string
		success    bool
		dimensions map[string]string
	}

	err error
}

func (s *metricsRecorderStub) RecordLatency(_ context.Context, operation, table string, _ time.Duration, success bool, dimensions map[string]string) error {
	s.calls = append(s.calls, struct {
		operation  string
		table      string
		success    bool
		dimensions map[string]string
	}{
		operation:  operation,
		table:      table,
		success:    success,
		dimensions: dimensions,
	})
	return s.err
}

func TestLatencyContext_RoundTrip(t *testing.T) {
	ctx := context.Background()

	_, _, ok := GetLatencyContext(ctx)
	assert.False(t, ok)

	ctx = WithLatencyContext(ctx, "op")
	op, start, ok := GetLatencyContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "op", op)
	assert.False(t, start.IsZero())
}

func TestDefaultMetricsRecorder_RecordLatency_CallsCreateFn(t *testing.T) {
	var recorded *models.MetricRecord
	createFn := func(_ context.Context, metric *models.MetricRecord) error {
		recorded = metric
		return nil
	}

	recorder := NewDefaultMetricsRecorder(createFn, "svc")
	err := recorder.RecordLatency(context.Background(), "query", "table", 123*time.Millisecond, true, map[string]string{
		"operation_type": "query",
	})
	require.NoError(t, err)
	require.NotNil(t, recorded)
	assert.Equal(t, "database_operation", recorded.MetricType)
	assert.Equal(t, "svc", recorded.ServiceName)
	assert.Equal(t, "ms", recorded.Unit)
	assert.Equal(t, int64(1), recorded.Count)
	assert.Equal(t, "query", recorded.Dimensions["operation"])
	assert.Equal(t, "table", recorded.Dimensions["table"])
	assert.Equal(t, "true", recorded.Dimensions["success"])
	assert.Equal(t, "query", recorded.Dimensions["operation_type"])
}

func TestDynamORMTracker_TrackQuery_RecordsLatency(t *testing.T) {
	logger := zaptest.NewLogger(t)
	recorder := &metricsRecorderStub{}

	tracker := NewDynamORMTracker(nil, logger, recorder)

	t.Run("success", func(t *testing.T) {
		err := tracker.TrackQuery(context.Background(), "get", "tbl", func() error { return nil })
		require.NoError(t, err)
		require.Len(t, recorder.calls, 1)
		assert.Equal(t, "get", recorder.calls[0].operation)
		assert.Equal(t, "tbl", recorder.calls[0].table)
		assert.True(t, recorder.calls[0].success)
		assert.Equal(t, "query", recorder.calls[0].dimensions["operation_type"])
	})

	t.Run("error", func(t *testing.T) {
		recorder.calls = nil
		boom := errors.New("boom")
		err := tracker.TrackQuery(context.Background(), "get", "tbl", func() error { return boom })
		require.ErrorIs(t, err, boom)
		require.Len(t, recorder.calls, 1)
		assert.False(t, recorder.calls[0].success)
	})

	t.Run("record_error_does_not_fail_query", func(t *testing.T) {
		recorder.calls = nil
		recorder.err = errors.New("record_failed")
		err := tracker.TrackQuery(context.Background(), "get", "tbl", func() error { return nil })
		require.NoError(t, err)
		require.Len(t, recorder.calls, 1)
		recorder.err = nil
	})
}

func TestDynamORMTracker_ShortcutsAndWrapper(t *testing.T) {
	logger := zaptest.NewLogger(t)
	recorder := &metricsRecorderStub{}

	tracker := NewDynamORMTracker(nil, logger, recorder)

	require.NoError(t, tracker.TrackCreate(context.Background(), "tbl", func() error { return nil }))
	require.NoError(t, tracker.TrackUpdate(context.Background(), "tbl", func() error { return nil }))
	require.NoError(t, tracker.TrackDelete(context.Background(), "tbl", func() error { return nil }))
	require.NoError(t, tracker.TrackBatch(context.Background(), "batch", "tbl", 2, func() error { return nil }))

	dm := NewDynamORMMetrics(nil, "tbl", logger, recorder)
	require.NoError(t, dm.TrackRepositoryMethod(context.Background(), "repo", "method", func() error { return nil }))
	require.NoError(t, dm.TrackCreate(context.Background(), "ignored", func() error { return nil }))
	require.NoError(t, dm.TrackUpdate(context.Background(), "ignored", func() error { return nil }))
	require.NoError(t, dm.TrackDelete(context.Background(), "ignored", func() error { return nil }))
	require.NoError(t, dm.TrackQuery(context.Background(), "repo", "method", func() error { return nil }))
	require.NoError(t, dm.TrackBatch(context.Background(), "repo", "batch", 3, func() error { return nil }))
}

func TestRecordRepositoryLatency(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("nil_recorder_noop", func(t *testing.T) {
		RecordRepositoryLatency(ctx, "repo", "method", time.Millisecond, true, logger, nil)
	})

	t.Run("records_with_dimensions", func(t *testing.T) {
		recorder := &metricsRecorderStub{}
		RecordRepositoryLatency(ctx, "repo", "method", time.Millisecond, false, logger, recorder)
		require.Len(t, recorder.calls, 1)
		assert.Equal(t, "repo.method", recorder.calls[0].operation)
		assert.Equal(t, "main", recorder.calls[0].table)
		assert.False(t, recorder.calls[0].success)
		assert.Equal(t, "repo", recorder.calls[0].dimensions["repository"])
		assert.Equal(t, "method", recorder.calls[0].dimensions["method"])
		assert.Equal(t, "repository", recorder.calls[0].dimensions["operation_type"])
	})
}
