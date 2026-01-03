package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeDLQProcessor struct {
	initErr      error
	processErr   error
	reprocessErr error
	cleanupErr   error
	trendsErr    error
	searchErr    error

	analyticsErrByService map[string]error

	scheduledCalls int
	cleanupCalls   int
	analyticsCalls int
	trendsCalls    int
	searchCalls    int
}

func (f *fakeDLQProcessor) InitializeAWSClients(context.Context) error { return f.initErr }
func (f *fakeDLQProcessor) ProcessDLQMessages(context.Context, events.SQSEvent) error {
	return f.processErr
}
func (f *fakeDLQProcessor) ScheduledReprocessing(context.Context) error {
	f.scheduledCalls++
	return f.reprocessErr
}
func (f *fakeDLQProcessor) CleanupExpiredMessages(context.Context) error {
	f.cleanupCalls++
	return f.cleanupErr
}
func (f *fakeDLQProcessor) GetAnalytics(_ context.Context, service string, timeRange repositories.DLQTimeRange) (*repositories.DLQAnalytics, error) {
	f.analyticsCalls++
	if err := f.analyticsErrByService[service]; err != nil {
		return nil, err
	}
	return &repositories.DLQAnalytics{
		Service:       service,
		TimeRange:     timeRange,
		TotalMessages: 1,
	}, nil
}
func (f *fakeDLQProcessor) GetTrends(context.Context, string, int) (*repositories.DLQTrends, error) {
	f.trendsCalls++
	if f.trendsErr != nil {
		return nil, f.trendsErr
	}
	return &repositories.DLQTrends{DailyStats: map[string]*repositories.DLQDailyStats{}}, nil
}
func (f *fakeDLQProcessor) SearchMessages(context.Context, *repositories.DLQSearchFilter) ([]*models.DLQMessage, string, error) {
	f.searchCalls++
	if f.searchErr != nil {
		return nil, "", f.searchErr
	}
	return []*models.DLQMessage{}, "", nil
}

func TestDLQProcessorHandler_HandleSQS(t *testing.T) {
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	ctx.SetRequestID("req")

	t.Run("fails when aws init fails", func(t *testing.T) {
		p := &fakeDLQProcessor{initErr: errors.New("no aws")}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		err := h.HandleSQS(ctx, events.SQSEvent{Records: []events.SQSMessage{{MessageId: "m1"}}})
		require.Error(t, err)
	})

	t.Run("fails when processor fails", func(t *testing.T) {
		p := &fakeDLQProcessor{processErr: errors.New("boom")}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		err := h.HandleSQS(ctx, events.SQSEvent{Records: []events.SQSMessage{{MessageId: "m1"}}})
		require.Error(t, err)
	})

	t.Run("succeeds", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		require.NoError(t, h.HandleSQS(ctx, events.SQSEvent{Records: []events.SQSMessage{{MessageId: "m1"}}}))
	})
}

func TestDLQProcessorHandler_HandleEventBridge(t *testing.T) {
	baseCtx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	baseCtx.SetRequestID("req")

	t.Run("scheduled reprocessing variants", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		require.NoError(t, h.HandleEventBridge(baseCtx, events.EventBridgeEvent{DetailType: "DLQ Scheduled Reprocessing"}))
		require.NoError(t, h.HandleEventBridge(baseCtx, events.EventBridgeEvent{DetailType: "Scheduled Event"}))
		require.Equal(t, 2, p.scheduledCalls)
	})

	t.Run("cleanup", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		require.NoError(t, h.HandleEventBridge(baseCtx, events.EventBridgeEvent{DetailType: "DLQ Cleanup"}))
		require.Equal(t, 1, p.cleanupCalls)
	})

	t.Run("cleanup error wraps", func(t *testing.T) {
		p := &fakeDLQProcessor{cleanupErr: errors.New("nope")}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		require.Error(t, h.HandleEventBridge(baseCtx, events.EventBridgeEvent{DetailType: "DLQ Cleanup"}))
	})

	t.Run("analytics", func(t *testing.T) {
		p := &fakeDLQProcessor{analyticsErrByService: map[string]error{"media-processor": errors.New("fail one")}}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		require.NoError(t, h.HandleEventBridge(baseCtx, events.EventBridgeEvent{DetailType: "DLQ Analytics"}))
		require.GreaterOrEqual(t, p.analyticsCalls, 1)
	})

	t.Run("unknown detail type", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		require.NoError(t, h.HandleEventBridge(baseCtx, events.EventBridgeEvent{DetailType: "Unknown"}))
		require.Equal(t, 0, p.scheduledCalls)
		require.Equal(t, 0, p.cleanupCalls)
	})

	t.Run("scheduled reprocessing error wraps", func(t *testing.T) {
		p := &fakeDLQProcessor{reprocessErr: errors.New("nope")}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		require.Error(t, h.HandleEventBridge(baseCtx, events.EventBridgeEvent{DetailType: "DLQ Scheduled Reprocessing"}))
	})
}

func TestDLQProcessor_EventExtraction(t *testing.T) {
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))

	_, err := extractSQSEvent(ctx)
	require.Error(t, err)
	_, err = extractEventBridgeEvent(ctx)
	require.Error(t, err)

	ctx.Request.RawEvent = func() {}
	_, err = extractSQSEvent(ctx)
	require.Error(t, err)

	ctx.Request.RawEvent = 5
	_, err = extractSQSEvent(ctx)
	require.Error(t, err)

	ctx.Request.RawEvent = func() {}
	_, err = extractEventBridgeEvent(ctx)
	require.Error(t, err)

	ctx.Request.RawEvent = "nope"
	_, err = extractEventBridgeEvent(ctx)
	require.Error(t, err)

	ctx.Request.RawEvent = events.EventBridgeEvent{DetailType: "Scheduled Event"}
	ev, err := extractEventBridgeEvent(ctx)
	require.NoError(t, err)
	require.Equal(t, "Scheduled Event", ev.DetailType)

	ctx.Request.RawEvent = events.SQSEvent{Records: []events.SQSMessage{{MessageId: "m1"}}}
	sqsEv, err := extractSQSEvent(ctx)
	require.NoError(t, err)
	require.Len(t, sqsEv.Records, 1)
}

func TestDLQProcessor_RequestIDMiddleware_SetsValues(t *testing.T) {
	origLogger := logger
	t.Cleanup(func() { logger = origLogger })
	logger = zap.NewNop()

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	next := lift.HandlerFunc(func(c *lift.Context) error {
		require.NotEmpty(t, c.GetRequestID())
		require.Equal(t, c.GetRequestID(), c.Get("requestID").(string))
		return nil
	})

	require.NoError(t, requestIDMiddleware()(next).Handle(ctx))
}

func TestDLQProcessor_LogHelpers_CoverBranches(t *testing.T) {
	origLogger := logger
	t.Cleanup(func() { logger = origLogger })
	logger = zap.NewNop()

	logRequestCompletion("req", 10*time.Millisecond, nil)
	logRequestCompletion("req", 10*time.Millisecond, errors.New("boom"))

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	ctx.Set("requestID", "req")
	logHandlerError(ctx, lift.NewLiftError("ERR", "nope", 500))
	logHandlerError(ctx, errors.New("boom"))
}

func TestDLQProcessor_MiddlewareAndRoutes(t *testing.T) {
	origLogger := logger
	origHandler := handler
	origLambdaCtx := lambdaCtx
	origStart := lambdaStartFn
	t.Cleanup(func() {
		logger = origLogger
		handler = origHandler
		lambdaCtx = origLambdaCtx
		lambdaStartFn = origStart
	})

	logger = zap.NewNop()
	handler = NewDLQProcessorHandler(&fakeDLQProcessor{}, logger)
	lambdaCtx = &common.LambdaContext{Logger: logger}

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	ctx.Set("requestID", "req")

	next := lift.HandlerFunc(func(*lift.Context) error { return errors.New("boom") })

	require.Error(t, loggingMiddleware()(next).Handle(ctx))
	require.Error(t, errorHandlingMiddleware()(next).Handle(ctx))
	require.Error(t, costTrackingMiddleware()(next).Handle(ctx))

	called := false
	lambdaStartFn = func(_ interface{}) { called = true }
	main()
	require.True(t, called)
}

func TestDLQProcessor_AdminHandlers(t *testing.T) {
	origLogger := logger
	origHandler := handler
	t.Cleanup(func() {
		logger = origLogger
		handler = origHandler
	})

	logger = zap.NewNop()
	handler = NewDLQProcessorHandler(&fakeDLQProcessor{}, logger)

	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	ctx.Set("requestID", "req")

	require.NoError(t, handleHealthCheck(ctx))
	require.NotEmpty(t, ctx.Response.Body)

	analyticsCtx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	analyticsCtx.Set("requestID", "req")
	analyticsCtx.SetParam("service", "svc")
	require.NoError(t, handleAnalytics(analyticsCtx))

	trendsCtx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	trendsCtx.Set("requestID", "req")
	trendsCtx.SetParam("service", "svc")
	require.NoError(t, handleTrends(trendsCtx))

	searchCtx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	searchCtx.Set("requestID", "req")
	body, err := json.Marshal(map[string]any{"service": "svc"})
	require.NoError(t, err)
	searchCtx.Request.Body = body
	require.NoError(t, handleSearch(searchCtx))
}

func TestDLQProcessor_AdminHandlers_ErrorBranches(t *testing.T) {
	origLogger := logger
	origHandler := handler
	t.Cleanup(func() {
		logger = origLogger
		handler = origHandler
	})

	logger = zap.NewNop()
	handler = NewDLQProcessorHandler(&fakeDLQProcessor{
		analyticsErrByService: map[string]error{"svc": errors.New("analytics")},
		trendsErr:             errors.New("trends"),
		searchErr:             errors.New("search"),
	}, logger)

	missingService := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	missingService.Set("requestID", "req")
	require.Error(t, handleAnalytics(missingService))
	require.Error(t, handleTrends(missingService))

	analyticsCtx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	analyticsCtx.Set("requestID", "req")
	analyticsCtx.SetParam("service", "svc")
	require.Error(t, handleAnalytics(analyticsCtx))

	trendsCtx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	trendsCtx.Set("requestID", "req")
	trendsCtx.SetParam("service", "svc")
	require.Error(t, handleTrends(trendsCtx))

	searchCtx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	searchCtx.Set("requestID", "req")
	searchCtx.Request.Body = []byte(`{"service":"svc"}`)
	require.Error(t, handleSearch(searchCtx))

	parseErrCtx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))
	parseErrCtx.Set("requestID", "req")
	parseErrCtx.Request.Body = []byte(`not-json`)
	require.Error(t, handleSearch(parseErrCtx))
}

func TestDLQProcessor_ParseSearchFilter(t *testing.T) {
	ctx := lift.NewContext(context.Background(), lift.NewRequest(&adapters.Request{}))

	_, err := parseSearchFilter(ctx)
	require.Error(t, err)

	ctx.Request.Body = []byte(`not-json`)
	_, err = parseSearchFilter(ctx)
	require.Error(t, err)

	ctx.Request.Body = []byte(`{"service":"svc","limit":0}`)
	filter, err := parseSearchFilter(ctx)
	require.NoError(t, err)
	require.Equal(t, "svc", filter.Service)
	require.Equal(t, 50, filter.Limit)
}

func TestCalculateProcessingCost(t *testing.T) {
	require.Equal(t, int64(20), calculateProcessingCost(500*time.Millisecond))
	require.Greater(t, calculateProcessingCost(2*time.Second), int64(20))
}

func TestDLQProcessor_TopLevelHandlers_Coverage(t *testing.T) {
	origLogger := logger
	origHandler := handler
	t.Cleanup(func() {
		logger = origLogger
		handler = origHandler
	})

	logger = zap.NewNop()
	handler = NewDLQProcessorHandler(&fakeDLQProcessor{}, logger)

	sqsReq := lift.NewRequest(&adapters.Request{
		RawEvent: events.SQSEvent{Records: []events.SQSMessage{{MessageId: "m1"}}},
	})
	sqsCtx := lift.NewContext(context.Background(), sqsReq)
	sqsCtx.SetRequestID("req")
	require.NoError(t, handleSQSEvent(sqsCtx))

	ebReq := lift.NewRequest(&adapters.Request{
		RawEvent: events.EventBridgeEvent{DetailType: "DLQ Cleanup"},
	})
	ebCtx := lift.NewContext(context.Background(), ebReq)
	ebCtx.SetRequestID("req")
	require.NoError(t, handleEventBridgeEvent(ebCtx))
}
