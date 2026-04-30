package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHandlerCoverageMargin_InstanceHelperFallbacks(t *testing.T) {
	ctx := context.Background()

	var nilHandler *Handler
	require.False(t, nilHandler.instanceLocked(ctx))
	require.Empty(t, nilHandler.instanceRules(ctx))
	require.Empty(t, nilHandler.instanceExtendedDescription(ctx))
	users, statuses, domains := nilHandler.instanceCounts(ctx)
	require.Zero(t, users)
	require.Zero(t, statuses)
	require.Zero(t, domains)
	require.Nil(t, nilHandler.instanceContactAccount(ctx))
	require.Equal(t, map[string]any{"enabled": false}, nilHandler.instanceTipsConfig(ctx))
	require.False(t, nilHandler.isTranslationEnabled(ctx))

	h := &Handler{cfg: round11TestConfig(), logger: zap.NewNop()}
	require.Empty(t, h.instanceRules(ctx))
	require.Empty(t, h.instanceExtendedDescription(ctx))
	users, statuses, domains = h.instanceCounts(ctx)
	require.Zero(t, users)
	require.Zero(t, statuses)
	require.Zero(t, domains)
	require.Nil(t, h.instanceContactAccount(ctx))
}

func TestHandlerCoverageMargin_FeatureConfigFallbacks(t *testing.T) {
	ctx := context.Background()

	t.Run("trust repo error disables", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{firstErrorOnce: errors.New("boom")})
		h.logger = zap.NewNop()
		out := h.instanceTrustConfig(ctx)
		require.Equal(t, false, out["enabled"])
	})

	t.Run("legacy trust missing key disables", func(t *testing.T) {
		t.Setenv("LESSER_HOST_URL", "https://lab.lesser.host")
		cfg := round11TestConfig()
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		h.logger = zap.NewNop()
		out := h.instanceTrustConfig(ctx)
		require.Equal(t, false, out["enabled"])
	})

	t.Run("tips repo error falls back to legacy config", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.TipEnabled = true
		cfg.TipChainID = 8453
		cfg.TipContractAddress = " 0xabc "
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{firstErrorOnce: errors.New("boom")})
		h.logger = zap.NewNop()
		out := h.instanceTipsConfig(ctx)
		require.Equal(t, true, out["enabled"])
		require.Equal(t, 8453, out["chain_id"])
		require.Equal(t, "0xabc", out["contract_address"])
	})

	t.Run("translation repo error falls back to legacy config", func(t *testing.T) {
		cfg := round11TestConfig()
		cfg.TranslationEnabled = true
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{firstErrorOnce: errors.New("boom")})
		h.logger = zap.NewNop()
		require.True(t, h.isTranslationEnabled(ctx))
	})
}

func TestHandlerCoverageMargin_TranslationAndRelationshipHelpers(t *testing.T) {
	h := &Handler{cfg: round11TestConfig(), logger: zap.NewNop()}

	content, spoiler, language := h.extractTranslatableContent(&activitypub.Note{
		BaseObject: activitypub.BaseObject{Summary: "content warning"},
		Content:    "<p>hello</p>",
	})
	require.Equal(t, "<p>hello</p>", content)
	require.Equal(t, "content warning", spoiler)
	require.Empty(t, language)

	content, spoiler, language = h.extractTranslatableContent(struct{}{})
	require.Empty(t, content)
	require.Empty(t, spoiler)
	require.Empty(t, language)

	require.False(t, relationshipTargetNotFound(nil))
	require.True(t, relationshipTargetNotFound(errors.New("actor not found: bob")))
	require.False(t, relationshipTargetNotFound(errors.New("identity mismatch")))
}
