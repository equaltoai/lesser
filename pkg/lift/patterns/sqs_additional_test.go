package patterns

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type recordingSQSHandler struct {
	calls  int
	last   events.SQSEvent
	retErr error
}

func (r *recordingSQSHandler) HandleSQS(_ *lift.Context, event events.SQSEvent) error {
	r.calls++
	r.last = event
	return r.retErr
}

func newSQSContext(t *testing.T, rawEvent any) *lift.Context {
	t.Helper()

	req := lift.NewRequest(&adapters.Request{
		TriggerType: adapters.TriggerSQS,
		RawEvent:    rawEvent,
		Records: []any{
			map[string]any{"eventSourceARN": "arn:aws:sqs:us-east-1:123456789012:queue-name"},
		},
	})
	return lift.NewContext(context.Background(), req)
}

func TestProcessEventWithTiming_SetsFallbackRequestIDAndLogs(t *testing.T) {
	ctx := newSQSContext(t, events.SQSEvent{})
	logger := zap.NewNop()

	errBoom := errors.New("boom")
	err := ProcessEventWithTiming(ctx, ProcessEventConfig{
		ProcessorName: "queue",
		RequestIDKey:  "requestID",
		RecordCount:   2,
		Logger:        logger,
	}, func(_ *lift.Context) error { return errBoom })
	require.ErrorIs(t, err, errBoom)
	require.NotEmpty(t, ctx.Get("requestID"))

	err = ProcessEventWithTiming(ctx, ProcessEventConfig{
		ProcessorName: "queue",
		RequestIDKey:  "requestID",
		RecordCount:   2,
		Logger:        logger,
	}, func(_ *lift.Context) error { return nil })
	require.NoError(t, err)
}

func TestRegisterSQS_ParsesRawEventForms(t *testing.T) {
	app := lift.New()
	logger := zap.NewNop()

	handler := &recordingSQSHandler{}
	processor := NewSQSProcessor("queue-name", handler, logger)
	RegisterSQS(app, processor)

	router := app.GetEventRouter()

	t.Run("missing raw event", func(t *testing.T) {
		ctx := newSQSContext(t, nil)
		h, err := router.FindEventHandler(ctx)
		require.NoError(t, err)
		require.Error(t, h.HandleEvent(ctx))
	})

	t.Run("direct cast SQSEvent", func(t *testing.T) {
		handler.calls = 0
		ctx := newSQSContext(t, events.SQSEvent{
			Records: []events.SQSMessage{{MessageId: "a"}},
		})
		h, err := router.FindEventHandler(ctx)
		require.NoError(t, err)
		require.NoError(t, h.HandleEvent(ctx))
		require.Equal(t, 1, handler.calls)
		require.Len(t, handler.last.Records, 1)
	})

	t.Run("fallback JSON marshal/unmarshal", func(t *testing.T) {
		handler.calls = 0
		ctx := newSQSContext(t, map[string]any{
			"Records": []map[string]any{{"messageId": "b"}},
		})
		h, err := router.FindEventHandler(ctx)
		require.NoError(t, err)
		require.NoError(t, h.HandleEvent(ctx))
		require.Equal(t, 1, handler.calls)
	})

	t.Run("marshal error", func(t *testing.T) {
		ctx := newSQSContext(t, make(chan int))
		h, err := router.FindEventHandler(ctx)
		require.NoError(t, err)
		require.Error(t, h.HandleEvent(ctx))
	})

	t.Run("parse error", func(t *testing.T) {
		ctx := newSQSContext(t, "not an event")
		h, err := router.FindEventHandler(ctx)
		require.NoError(t, err)
		require.Error(t, h.HandleEvent(ctx))
	})
}

func TestExampleHandlers_AreCallable(t *testing.T) {
	logger := zap.NewNop()

	stream := &ExampleStreamHandler{logger: logger}
	require.NoError(t, stream.HandleStream(nil, events.DynamoDBEvent{
		Records: []events.DynamoDBEventRecord{{EventName: "INSERT", EventID: "1"}},
	}))

	sqs := &ExampleSQSHandler{logger: logger}
	require.NoError(t, sqs.HandleSQS(nil, events.SQSEvent{
		Records: []events.SQSMessage{{MessageId: "a", Body: "b"}},
	}))

	scheduled := &ExampleScheduledHandler{logger: logger}
	require.NoError(t, scheduled.HandleScheduledEvent(nil))
}

func TestCreateSQSApp_WiresSQSHandler(t *testing.T) {
	logger := zap.NewNop()
	handler := &recordingSQSHandler{}

	app := CreateSQSApp("queue-name", handler, logger)
	_, err := app.HandleRequest(context.Background(), map[string]any{
		"Records": []any{
			map[string]any{
				"eventSource":    "aws:sqs",
				"eventSourceARN": "arn:aws:sqs:us-east-1:123456789012:queue-name",
				"messageId":      "a",
				"body":           "b",
				"receiptHandle":  "h",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, handler.calls)
}
