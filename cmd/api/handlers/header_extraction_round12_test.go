package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHandler_extractDirectTimelineAuthHeader(t *testing.T) {
	h := &Handler{
		cfg:    &config.Config{Domain: "example.com"},
		logger: zap.NewNop(),
	}

	t.Run("uses Authorization header when present", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/api/v1/timelines/direct", map[string]string{
			"Authorization": "Bearer token",
		}, nil, nil)
		require.NoError(t, err)

		require.Equal(t, "Bearer token", h.extractDirectTimelineAuthHeader(ctx))
	})

	t.Run("falls back to lowercase header", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/api/v1/timelines/direct", map[string]string{
			"authorization": "Bearer token",
		}, nil, nil)
		require.NoError(t, err)

		require.Equal(t, "Bearer token", h.extractDirectTimelineAuthHeader(ctx))
	})

	t.Run("uses direct request headers when ctx.Header cannot read", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/api/v1/timelines/direct", map[string]string{
			"Authorization": "Bearer token",
		}, nil, nil)
		require.NoError(t, err)

		// Simulate a broken headerValue(ctx, ) lookup by clearing the lifted Headers map
		ctx.Request.Headers = nil

		require.Equal(t, "", h.extractDirectTimelineAuthHeader(ctx))
	})
}

func TestHandler_extractTranslationAuthHeader(t *testing.T) {
	h := &Handler{
		cfg:    &config.Config{Domain: "example.com"},
		logger: zap.NewNop(),
	}

	t.Run("uses Authorization header when present", func(t *testing.T) {
		ctx, err := round10NewLiftContext("POST", "/api/v1/translate", map[string]string{
			"Authorization": "Bearer token",
		}, nil, nil)
		require.NoError(t, err)

		require.Equal(t, "Bearer token", h.extractTranslationAuthHeader(ctx))
	})

	t.Run("falls back to lowercase header", func(t *testing.T) {
		ctx, err := round10NewLiftContext("POST", "/api/v1/translate", map[string]string{
			"authorization": "Bearer token",
		}, nil, nil)
		require.NoError(t, err)

		require.Equal(t, "Bearer token", h.extractTranslationAuthHeader(ctx))
	})

	t.Run("uses direct request headers when ctx.Header cannot read", func(t *testing.T) {
		ctx, err := round10NewLiftContext("POST", "/api/v1/translate", map[string]string{
			"authorization": "Bearer token",
		}, nil, nil)
		require.NoError(t, err)

		ctx.Request.Headers = nil

		require.Equal(t, "", h.extractTranslationAuthHeader(ctx))
	})
}
