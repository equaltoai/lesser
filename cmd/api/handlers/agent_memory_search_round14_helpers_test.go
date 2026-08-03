package handlers

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

func TestAgentMemorySearchRound14_ParseDateRange(t *testing.T) {
	t.Run("nil range returns nils", func(t *testing.T) {
		start, end, err := parseDateRange(nil)
		require.NoError(t, err)
		require.Nil(t, start)
		require.Nil(t, end)
	})

	t.Run("invalid start returns validation error", func(t *testing.T) {
		_, _, err := parseDateRange(&models.DateRange{Start: "not-a-date", End: "2020-01-01"})
		require.Error(t, err)
	})

	t.Run("invalid end returns validation error", func(t *testing.T) {
		_, _, err := parseDateRange(&models.DateRange{Start: "2020-01-01", End: "not-a-date"})
		require.Error(t, err)
	})

	t.Run("valid start/end normalizes to UTC day bounds", func(t *testing.T) {
		start, end, err := parseDateRange(&models.DateRange{Start: " 2020-01-02 ", End: "2020-01-03"})
		require.NoError(t, err)
		require.NotNil(t, start)
		require.NotNil(t, end)
		require.Equal(t, time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC), *start)
		require.Equal(t, time.Date(2020, 1, 3, 23, 59, 59, 0, time.UTC), *end)
	})
}

func TestAgentMemorySearchRound14_NormalizeLimitAndMode(t *testing.T) {
	require.Equal(t, agentMemorySearchDefaultLimit, normalizeAgentMemorySearchLimit(models.AgentMemorySearchRequest{Limit: 0}))
	require.Equal(t, agentMemorySearchMaxLimit, normalizeAgentMemorySearchLimit(models.AgentMemorySearchRequest{Limit: agentMemorySearchMaxLimit + 10}))
	require.Equal(t, 3, normalizeAgentMemorySearchLimit(models.AgentMemorySearchRequest{Limit: 3}))

	require.Equal(t, "timeline", normalizeAgentMemorySearchMode(""))
	require.Equal(t, "timeline", normalizeAgentMemorySearchMode("  "))
	require.Equal(t, "hybrid", normalizeAgentMemorySearchMode(" HYBRID "))
}

func TestAgentMemorySearchRound14_ValidateModeAndHybridCandidates(t *testing.T) {
	t.Run("validateAgentMemorySearchMode rejects unknown modes", func(t *testing.T) {
		h := &Handler{}
		resp, err := h.validateAgentMemorySearchMode(&apptheory.Context{}, "nope")
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 400, resp.Status)
	})

	t.Run("validateAgentMemorySearchMode blocks hybrid without policy", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: nil})
		resp, err := h.validateAgentMemorySearchMode(&apptheory.Context{}, "hybrid")
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 403, resp.Status)
	})

	t.Run("validateAgentMemorySearchMode allows hybrid when enabled", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.AllowAgents = true

		policy := storageModels.NewAgentInstanceConfig()
		policy.AllowAgents = true
		policy.HybridRetrievalEnabled = true
		policy.HybridRetrievalMaxCandidates = 5000 // verify cap to event cap below

		state := &round10QueryState{agentInstanceConfig: policy}
		h, _, _ := round11NewHandler(t, cfg, state)

		resp, err := h.validateAgentMemorySearchMode(&apptheory.Context{}, "hybrid")
		require.NoError(t, err)
		require.Nil(t, resp)

		require.Equal(t, 2000, h.hybridRetrievalMaxCandidates(&apptheory.Context{}))
	})

	t.Run("hybridCandidateScore filters and scores", func(t *testing.T) {
		now := time.Now().UTC()
		seen := map[string]struct{}{"s1": {}}

		score, ok := hybridCandidateScore(nil, "hello", nil, nil, nil, nil)
		require.False(t, ok)
		require.Equal(t, 0.0, score)

		score, ok = hybridCandidateScore(&storageModels.Status{StatusID: "s0", Deleted: true}, "hello", nil, nil, nil, nil)
		require.False(t, ok)
		require.Equal(t, 0.0, score)

		score, ok = hybridCandidateScore(&storageModels.Status{StatusID: "s1"}, "hello", nil, nil, nil, seen)
		require.False(t, ok)
		require.Equal(t, 0.0, score)

		since := now.Add(1 * time.Hour)
		score, ok = hybridCandidateScore(&storageModels.Status{StatusID: "s2", PublishedAt: now}, "hello", &since, nil, nil, nil)
		require.False(t, ok)
		require.Equal(t, 0.0, score)

		until := now.Add(-1 * time.Hour)
		score, ok = hybridCandidateScore(&storageModels.Status{StatusID: "s3", PublishedAt: now}, "hello", nil, &until, nil, nil)
		require.False(t, ok)
		require.Equal(t, 0.0, score)

		score, ok = hybridCandidateScore(&storageModels.Status{StatusID: "s4", PublishedAt: now, Hashtags: []string{"a"}}, "hello", nil, nil, []string{"b"}, nil)
		require.False(t, ok)
		require.Equal(t, 0.0, score)

		score, ok = hybridCandidateScore(&storageModels.Status{StatusID: "s5", PublishedAt: now, Content: "unrelated"}, "hello", nil, nil, nil, nil)
		require.False(t, ok)
		require.Equal(t, 0.0, score)

		score, ok = hybridCandidateScore(&storageModels.Status{StatusID: "s6", PublishedAt: now, Content: "hello world", Hashtags: []string{"x", "y"}}, "hello", nil, nil, []string{"x"}, nil)
		require.True(t, ok)
		require.Greater(t, score, 0.0)
	})
}
