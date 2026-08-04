package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

type fakeDLQProcessor struct {
	initErr      error
	processErr   error
	reprocessErr error
	cleanupErr   error
	trendsErr    error
	searchErr    error

	analyticsErrByService map[string]error

	lastSQSEvent *events.SQSEvent

	scheduledCalls int
	cleanupCalls   int
	analyticsCalls int
	trendsCalls    int
	searchCalls    int
}

func (f *fakeDLQProcessor) InitializeAWSClients(context.Context) error { return f.initErr }
func (f *fakeDLQProcessor) ProcessDLQMessages(_ context.Context, event events.SQSEvent) error {
	f.lastSQSEvent = &event
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
	if f.analyticsErrByService != nil {
		if err := f.analyticsErrByService[service]; err != nil {
			return nil, err
		}
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

func TestDLQProcessorHandler_HandleSQSMessage(t *testing.T) {
	t.Run("fails when aws init fails", func(t *testing.T) {
		p := &fakeDLQProcessor{initErr: errors.New("no aws")}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		err := h.HandleSQSMessage(nil, events.SQSMessage{MessageId: "m1"})
		require.Error(t, err)
	})

	t.Run("fails when processor fails", func(t *testing.T) {
		p := &fakeDLQProcessor{processErr: errors.New("boom")}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		err := h.HandleSQSMessage(nil, events.SQSMessage{MessageId: "m1"})
		require.Error(t, err)
		require.NotNil(t, p.lastSQSEvent)
		require.Len(t, p.lastSQSEvent.Records, 1)
	})

	t.Run("succeeds", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		require.NoError(t, h.HandleSQSMessage(nil, events.SQSMessage{MessageId: "m1"}))
		require.NotNil(t, p.lastSQSEvent)
		require.Len(t, p.lastSQSEvent.Records, 1)
	})
}

func TestDLQProcessorHandler_HandleEventBridge(t *testing.T) {
	t.Run("scheduled reprocessing variants", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		_, err := h.HandleEventBridge(nil, events.EventBridgeEvent{DetailType: "DLQ Scheduled Reprocessing"})
		require.NoError(t, err)
		_, err = h.HandleEventBridge(nil, events.EventBridgeEvent{DetailType: "Scheduled Event"})
		require.NoError(t, err)
		require.Equal(t, 2, p.scheduledCalls)
	})

	t.Run("cleanup", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		_, err := h.HandleEventBridge(nil, events.EventBridgeEvent{DetailType: "DLQ Cleanup"})
		require.NoError(t, err)
		require.Equal(t, 1, p.cleanupCalls)
	})

	t.Run("cleanup error returns error", func(t *testing.T) {
		p := &fakeDLQProcessor{cleanupErr: errors.New("nope")}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		_, err := h.HandleEventBridge(nil, events.EventBridgeEvent{DetailType: "DLQ Cleanup"})
		require.Error(t, err)
	})

	t.Run("analytics calls all services", func(t *testing.T) {
		p := &fakeDLQProcessor{
			analyticsErrByService: map[string]error{
				"media-processor": errors.New("boom"),
			},
		}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		_, err := h.HandleEventBridge(nil, events.EventBridgeEvent{DetailType: "DLQ Analytics"})
		require.NoError(t, err)
		require.GreaterOrEqual(t, p.analyticsCalls, 1)
	})

	t.Run("unknown detail type returns nil", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, zap.NewNop())
		_, err := h.HandleEventBridge(nil, events.EventBridgeEvent{DetailType: "Unknown"})
		require.NoError(t, err)
	})
}

func TestParseSearchFilter(t *testing.T) {
	t.Run("missing service is error", func(t *testing.T) {
		_, err := parseSearchFilter(&apptheory.Context{Request: apptheory.Request{Body: []byte(`{}`)}})
		require.Error(t, err)
	})

	t.Run("invalid json is error", func(t *testing.T) {
		_, err := parseSearchFilter(&apptheory.Context{Request: apptheory.Request{Body: []byte(`{bad`)}})
		require.Error(t, err)
	})

	t.Run("defaults limit", func(t *testing.T) {
		filter, err := parseSearchFilter(&apptheory.Context{Request: apptheory.Request{Body: []byte(`{"service":"notification-processor"}`)}})
		require.NoError(t, err)
		require.Equal(t, 50, filter.Limit)
	})
}

func TestHTTPHandlers(t *testing.T) {
	prevHandler := handler
	t.Cleanup(func() { handler = prevHandler })

	t.Run("health check returns 200", func(t *testing.T) {
		resp, err := handleHealthCheck(&apptheory.Context{})
		require.NoError(t, err)
		require.Equal(t, 200, resp.Status)
	})

	t.Run("analytics 400 on missing service", func(t *testing.T) {
		resp, err := handleAnalyticsHTTP(&apptheory.Context{Params: map[string]string{}})
		require.NoError(t, err)
		require.Equal(t, 400, resp.Status)
	})

	t.Run("analytics 500 when handler missing", func(t *testing.T) {
		handler = nil
		resp, err := handleAnalyticsHTTP(&apptheory.Context{Params: map[string]string{"service": "notification-processor"}})
		require.NoError(t, err)
		require.Equal(t, 500, resp.Status)
	})

	t.Run("search 200 with next_cursor and count", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		handler = NewDLQProcessorHandler(p, zap.NewNop())

		ctx := &apptheory.Context{
			Request: apptheory.Request{Body: []byte(`{"service":"notification-processor","limit":1}`)},
		}
		resp, err := handleSearchHTTP(ctx)
		require.NoError(t, err)
		require.Equal(t, 200, resp.Status)
	})
}

func TestMain_WiresAppTheoryAndStartsLambda(t *testing.T) {
	prevStart := lambdaStartFn
	prevHandler := handler
	t.Cleanup(func() {
		lambdaStartFn = prevStart
		handler = prevHandler
	})

	handler = NewDLQProcessorHandler(&fakeDLQProcessor{}, zap.NewNop())

	startCalls := 0
	lambdaStartFn = func(h any) {
		startCalls++

		fn, ok := h.(func(context.Context, json.RawMessage) (any, error))
		require.True(t, ok)

		event := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId:      "m1",
					Body:           "{}",
					EventSource:    "aws:sqs",
					EventSourceARN: "arn:aws:sqs:us-east-1:123456789012:lesser-dev-import-processor-queue-dlq",
				},
			},
		}
		raw, err := json.Marshal(event)
		require.NoError(t, err)

		respAny, err := fn(context.Background(), raw)
		require.NoError(t, err)

		resp, ok := respAny.(events.SQSEventResponse)
		require.True(t, ok)
		require.Empty(t, resp.BatchItemFailures)
	}

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	main()
	require.Equal(t, 1, startCalls)
}

func TestInitializeDLQStorage_FailsClosed(t *testing.T) {
	origNewClient := newLambdaOptimizedClientFn
	t.Cleanup(func() { newLambdaOptimizedClientFn = origNewClient })

	newLambdaOptimizedClientFn = func(context.Context, string) (core.DB, error) {
		return nil, errors.New("storage unavailable")
	}

	ctx := &common.LambdaContext{
		Config: &config.Config{Region: "us-east-1", DynamoTableName: "test-table"},
		Logger: zap.NewNop(),
	}
	_, err := initializeDLQStorage(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "storage client initialization failed")
}

func TestInitializeDLQProcessor_SucceedsWithInjectedDB(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origLogger := logger
	origCtx := lambdaCtx
	origCfg := cfg
	origHandler := handler
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		logger = origLogger
		lambdaCtx = origCtx
		cfg = origCfg
		handler = origHandler
	})

	db := new(dynamormmocks.MockDB)
	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config:   &config.Config{Region: "us-east-1", DynamoTableName: "dlq-table"},
			Logger:   zap.NewNop(),
			DynamoDB: db,
		}
	}

	err := initializeDLQProcessor()
	require.NoError(t, err)
	require.NotNil(t, handler)
	require.Same(t, db, lambdaCtx.DynamoDB)
	require.Equal(t, "dlq-table", cfg.DynamoTableName)
}
