package main

import (
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

func TestDLQProcessorHandler_Round14_ScheduledReprocessingError(t *testing.T) {
	p := &fakeDLQProcessor{reprocessErr: errors.New("boom")}
	h := NewDLQProcessorHandler(p, zap.NewNop())
	_, err := h.HandleEventBridge(nil, events.EventBridgeEvent{DetailType: "DLQ Scheduled Reprocessing"})
	require.Error(t, err)
}

func TestDLQHTTPHandlers_Round14_TrendsAndErrorBranches(t *testing.T) {
	prevHandler := handler
	t.Cleanup(func() { handler = prevHandler })

	t.Run("trends 400 on missing service", func(t *testing.T) {
		resp, err := handleTrendsHTTP(&apptheory.Context{Params: map[string]string{}})
		require.NoError(t, err)
		require.Equal(t, 400, resp.Status)
	})

	t.Run("trends 500 when handler missing", func(t *testing.T) {
		handler = nil
		resp, err := handleTrendsHTTP(&apptheory.Context{Params: map[string]string{"service": "notification-processor"}})
		require.NoError(t, err)
		require.Equal(t, 500, resp.Status)
	})

	t.Run("trends 500 on processor error", func(t *testing.T) {
		p := &fakeDLQProcessor{trendsErr: errors.New("nope")}
		handler = NewDLQProcessorHandler(p, zap.NewNop())
		resp, err := handleTrendsHTTP(&apptheory.Context{Params: map[string]string{"service": "notification-processor"}})
		require.NoError(t, err)
		require.Equal(t, 500, resp.Status)
	})

	t.Run("analytics 500 on processor error", func(t *testing.T) {
		p := &fakeDLQProcessor{analyticsErrByService: map[string]error{"notification-processor": errors.New("nope")}}
		handler = NewDLQProcessorHandler(p, zap.NewNop())
		resp, err := handleAnalyticsHTTP(&apptheory.Context{Params: map[string]string{"service": "notification-processor"}})
		require.NoError(t, err)
		require.Equal(t, 500, resp.Status)
	})

	t.Run("search 500 on missing handler", func(t *testing.T) {
		handler = nil
		resp, err := handleSearchHTTP(&apptheory.Context{})
		require.NoError(t, err)
		require.Equal(t, 500, resp.Status)
	})

	t.Run("search 400 on invalid filter body", func(t *testing.T) {
		handler = NewDLQProcessorHandler(&fakeDLQProcessor{}, zap.NewNop())
		resp, err := handleSearchHTTP(&apptheory.Context{Request: apptheory.Request{Body: []byte(`{bad`)}})
		require.NoError(t, err)
		require.Equal(t, 400, resp.Status)
	})

	t.Run("search 500 on processor error", func(t *testing.T) {
		p := &fakeDLQProcessor{searchErr: errors.New("boom")}
		handler = NewDLQProcessorHandler(p, zap.NewNop())
		resp, err := handleSearchHTTP(&apptheory.Context{Request: apptheory.Request{Body: []byte(`{"service":"notification-processor"}`)}})
		require.NoError(t, err)
		require.Equal(t, 500, resp.Status)
	})
}
