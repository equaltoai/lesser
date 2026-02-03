package handlers

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAgentMemorySearchRound18_ParseAgentMemorySearchRequest(t *testing.T) {
	t.Run("GET parses query params", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", nil, map[string]string{
			"query":           "hello",
			"mode":            "timeline",
			"thread_id":       "thread-1",
			"include_threads": "true",
			"limit":           "7",
			"tags":            "Go, test ,,",
			"since_date":      "2026-01-01",
			"until_date":      "2026-01-02",
		}, nil)
		require.NoError(t, err)

		req, resp, respErr := parseAgentMemorySearchRequest(ctx)
		require.NoError(t, respErr)
		require.Nil(t, resp)
		require.Equal(t, "hello", req.Query)
		require.Equal(t, "timeline", req.Mode)
		require.Equal(t, "thread-1", req.ThreadID)
		require.True(t, req.IncludeThreads)
		require.Equal(t, 7, req.Limit)
		require.Equal(t, []string{"Go", "test"}, req.Tags)
		require.NotNil(t, req.DateRange)
		require.Equal(t, "2026-01-01", req.DateRange.Start)
		require.Equal(t, "2026-01-02", req.DateRange.End)
	})

	t.Run("GET falls back to default limit when invalid", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", nil, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)

		req, resp, respErr := parseAgentMemorySearchRequest(ctx)
		require.NoError(t, respErr)
		require.Nil(t, resp)
		require.Equal(t, agentMemorySearchDefaultLimit, req.Limit)
	})

	t.Run("POST returns 400 on invalid json body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/agents/memory/search", nil, nil, []byte("{"))
		_, resp, respErr := parseAgentMemorySearchRequest(ctx)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})
}

func TestAgentMemorySearchRound18_HelperBranches(t *testing.T) {
	t.Run("splitCommaList trims and drops empty", func(t *testing.T) {
		require.Equal(t, []string{"a", "b", "c"}, splitCommaList("a, b,, ,c"))
	})

	t.Run("normalizeTags normalizes and de-dupes", func(t *testing.T) {
		out := normalizeTags([]string{"Go", "#go", " ", "test"})
		require.Equal(t, []string{"go", "test"}, out)
	})

	t.Run("statusHasAllTags respects normalization and required", func(t *testing.T) {
		require.True(t, statusHasAllTags([]string{"Go"}, nil))
		require.False(t, statusHasAllTags(nil, []string{"go"}))
		require.True(t, statusHasAllTags([]string{"Go", "Test"}, []string{"go"}))
		require.False(t, statusHasAllTags([]string{"Go"}, []string{"missing"}))
	})

	t.Run("relevanceScore handles empty and missing terms", func(t *testing.T) {
		require.Equal(t, 1.0, relevanceScore("", "anything"))
		require.Equal(t, 0.0, relevanceScore("needle", "haystack"))
		require.Equal(t, 0.5, relevanceScore("needle haystack", "haystack only"))
	})

	t.Run("capThreadForAgent covers tail and root branches", func(t *testing.T) {
		require.Nil(t, capThreadForAgent(nil, 0))

		statuses := make([]*storagemodels.Status, 0, 25)
		for i := 0; i < 25; i++ {
			statuses = append(statuses, &storagemodels.Status{StatusID: "s" + strconv.Itoa(i)})
		}
		// If limit exceeds slice length we return the original.
		require.Equal(t, statuses, capThreadForAgent(statuses, 100))

		// Tail-first item shares root status ID -> return tail.
		statuses[0].StatusID = "same"
		statuses[len(statuses)-9].StatusID = "same"
		tailOnly := capThreadForAgent(statuses, 10)
		require.Len(t, tailOnly, 9)

		// Root + tail branch.
		statuses[0].StatusID = "root"
		statuses[len(statuses)-9].StatusID = "different"
		rootPlusTail := capThreadForAgent(statuses, 10)
		require.Len(t, rootPlusTail, 10)
		require.Equal(t, "root", rootPlusTail[0].StatusID)
	})

	t.Run("parseDateRange normalizes start/end boundaries", func(t *testing.T) {
		start, end, err := parseDateRange(&models.DateRange{Start: "2026-01-02", End: "2026-01-02"})
		require.NoError(t, err)
		require.NotNil(t, start)
		require.NotNil(t, end)
		require.Equal(t, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), *start)
		require.Equal(t, time.Date(2026, 1, 2, 23, 59, 59, 0, time.UTC), *end)
	})
}

func TestAgentMemorySearchRound18_ValidateAgentMemorySearchMode(t *testing.T) {
	cfg := round11TestConfig()

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", nil, nil, nil)
	require.NoError(t, err)

	t.Run("hybrid forbidden when repos missing", func(t *testing.T) {
		h := &Handler{cfg: cfg}
		resp, respErr := h.validateAgentMemorySearchMode(ctx, "hybrid")
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("hybrid forbidden when policy disabled", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: &storagemodels.AgentInstanceConfig{HybridRetrievalEnabled: false},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		resp, respErr := h.validateAgentMemorySearchMode(ctx, "hybrid")
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("invalid mode returns validation error", func(t *testing.T) {
		h := &Handler{cfg: cfg}
		resp, respErr := h.validateAgentMemorySearchMode(ctx, "nope")
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})
}
