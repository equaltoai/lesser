package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

func TestOutboxProcessor_ParseActivityFromRequest_EnforcesSafetyLimits_M3(t *testing.T) {
	op := &OutboxProcessor{logger: zap.NewNop()}

	t.Run("oversized body rejected before parse", func(t *testing.T) {
		ctx := &apptheory.Context{Request: apptheory.Request{Body: bytesOf('x', common.MaxActivitySize+1)}}
		activity, resp := op.parseActivityFromRequest(ctx)
		require.Nil(t, activity)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusRequestEntityTooLarge, resp.Status)
	})

	t.Run("json bomb shape rejected", func(t *testing.T) {
		body := `{"type":"Create","object":` + strings.Repeat(`{"nested":`, common.MaxJSONDepth+2) + `"leaf"` + strings.Repeat(`}`, common.MaxJSONDepth+2) + `}`
		ctx := &apptheory.Context{Request: apptheory.Request{Body: []byte(body)}}
		activity, resp := op.parseActivityFromRequest(ctx)
		require.Nil(t, activity)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})
}

func bytesOf(value byte, size int) []byte {
	out := make([]byte, size)
	for i := range out {
		out[i] = value
	}
	return out
}
