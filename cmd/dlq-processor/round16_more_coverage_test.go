package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

func TestDLQProcessorHandler_Round16_UsesEventContextAndInitializesLogger(t *testing.T) {
	t.Run("sqs handler initializes nop logger and uses request context", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, nil)

		msgCtx := &apptheory.EventContext{RequestID: "req"}
		err := h.HandleSQSMessage(msgCtx, events.SQSMessage{MessageId: "m1"})
		require.NoError(t, err)
		require.NotNil(t, h.logger)
	})

	t.Run("eventbridge handler initializes nop logger and uses request context", func(t *testing.T) {
		p := &fakeDLQProcessor{}
		h := NewDLQProcessorHandler(p, nil)

		_, err := h.HandleEventBridge(&apptheory.EventContext{RequestID: "req"}, events.EventBridgeEvent{DetailType: "Unknown"})
		require.NoError(t, err)
		require.NotNil(t, h.logger)
	})
}

func TestDLQHTTPHandlers_Round16_SuccessPaths(t *testing.T) {
	prevHandler := handler
	t.Cleanup(func() { handler = prevHandler })

	p := &fakeDLQProcessor{}
	handler = NewDLQProcessorHandler(p, zap.NewNop())

	resp, err := handleAnalyticsHTTP(&apptheory.Context{Params: map[string]string{"service": "notification-processor"}})
	require.NoError(t, err)
	require.Equal(t, 200, resp.Status)

	var analytics repositories.DLQAnalytics
	require.NoError(t, json.Unmarshal(resp.Body, &analytics))
	require.Equal(t, "notification-processor", analytics.Service)

	resp, err = handleTrendsHTTP(&apptheory.Context{Params: map[string]string{"service": "notification-processor"}})
	require.NoError(t, err)
	require.Equal(t, 200, resp.Status)

	var trends repositories.DLQTrends
	require.NoError(t, json.Unmarshal(resp.Body, &trends))
	require.NotNil(t, trends.DailyStats)
}

func TestMain_Round16_RecordsBatchFailureWhenHandlerNil(t *testing.T) {
	prevStart := lambdaStartFn
	prevHandler := handler
	t.Cleanup(func() {
		lambdaStartFn = prevStart
		handler = prevHandler
	})

	handler = nil

	var gotHandler any
	lambdaStartFn = func(h any) { gotHandler = h }

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	main()

	require.NotNil(t, gotHandler)
	handlerFn, ok := gotHandler.(func(context.Context, json.RawMessage) (any, error))
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

	respAny, err := handlerFn(context.Background(), raw)
	require.NoError(t, err)

	resp, ok := respAny.(events.SQSEventResponse)
	require.True(t, ok)
	require.Len(t, resp.BatchItemFailures, 1)
	require.Equal(t, "m1", resp.BatchItemFailures[0].ItemIdentifier)
}

func TestMain_Round16_ProcessesAllDLQQueueRoutes(t *testing.T) {
	prevStart := lambdaStartFn
	prevHandler := handler
	t.Cleanup(func() {
		lambdaStartFn = prevStart
		handler = prevHandler
	})

	handler = NewDLQProcessorHandler(&fakeDLQProcessor{}, zap.NewNop())

	var gotHandler any
	lambdaStartFn = func(h any) { gotHandler = h }

	t.Setenv("APP_NAME", "lesser")
	t.Setenv("STAGE", "dev")
	t.Setenv("ENVIRONMENT", "dev")

	main()

	handlerFn, ok := gotHandler.(func(context.Context, json.RawMessage) (any, error))
	require.True(t, ok)

	queueNames := []string{
		"enhanced-federation-queue",
		"export-processor-queue",
		"federation-aggregator-queue",
		"federation-delivery-queue",
		"import-processor-queue",
		"media-processor-queue",
		"notification-processor-queue",
		"push-delivery-queue",
	}

	records := make([]events.SQSMessage, 0, len(queueNames))
	for i, q := range queueNames {
		records = append(records, events.SQSMessage{
			MessageId:      fmt.Sprintf("m%d", i+1),
			Body:           "{}",
			EventSource:    "aws:sqs",
			EventSourceARN: fmt.Sprintf("arn:aws:sqs:us-east-1:123456789012:lesser-dev-%s-dlq", q),
		})
	}

	raw, err := json.Marshal(events.SQSEvent{Records: records})
	require.NoError(t, err)

	respAny, err := handlerFn(context.Background(), raw)
	require.NoError(t, err)

	resp, ok := respAny.(events.SQSEventResponse)
	require.True(t, ok)
	require.Empty(t, resp.BatchItemFailures)
}
