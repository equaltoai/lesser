package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_AI_ExplainObject_ObjectAndStatusFallback(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := round12AuthContext("admin")

	// Avoid pulling in boosted-state checks inside convertStatusToObject.
	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	objRepo, ok := storageRepo.Object().(*inmemory.ObjectRepository)
	require.True(t, ok)

	note := map[string]any{
		"id":           "obj-1",
		"type":         "note",
		"attributedTo": "https://localhost/users/alice",
		"content":      "hello",
	}
	require.NoError(t, objRepo.CreateObject(ctx, note))

	expl, err := resolver.Query().ExplainObject(ctx, "obj-1")
	require.NoError(t, err)
	require.NotNil(t, expl)
	require.NotNil(t, expl.Object)
	require.NotEmpty(t, expl.AccessPattern)

	statusRepo, ok := storageRepo.Status().(*inmemory.StatusRepository)
	require.True(t, ok)

	status := &models.Status{
		StatusID:       "status-1",
		AuthorUsername: "alice",
		Content:        "hello from status",
		CreatedAt:      time.Now().Add(-time.Hour),
		UpdatedAt:      time.Now().Add(-time.Minute),
	}
	require.NoError(t, statusRepo.CreateStatus(ctx, status))

	expl, err = resolver.Query().ExplainObject(ctx, "status-1")
	require.NoError(t, err)
	require.NotNil(t, expl)
	require.NotNil(t, expl.Object)
	require.NotEmpty(t, expl.AccessPattern)

	_, err = resolver.Query().ExplainObject(ctx, "missing-id")
	require.Error(t, err)
}

func TestRound12QueryResolvers_AI_StatsAnalysisAndCapabilities(t *testing.T) {
	t.Parallel()

	// With the full test registry, AiAnalysis/AiStats should surface "repository not configured" errors.
	full, _ := newRound12GraphResolver(t)
	_, err := full.Query().AiAnalysis(round12AuthContext("alice"), "obj-1")
	require.Error(t, err)

	_, err = full.Query().AiAnalysis(round12AuthContext("admin"), "obj-1")
	require.Error(t, err)

	_, err = full.Query().AiStats(round12AuthContext("alice"), "DAY")
	require.Error(t, err)

	_, err = full.Query().AiStats(round12AuthContext("admin"), "DAY")
	require.Error(t, err)

	_, err = full.Query().AiCapabilities(round12AuthContext("alice"))
	require.Error(t, err)

	caps, err := full.Query().AiCapabilities(round12AuthContext("admin"))
	require.NoError(t, err)
	require.NotNil(t, caps)
	require.NotNil(t, caps.TextAnalysis)
}
