package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
)

func TestAgentMemorySearchRound17_ListAgentMemoryEvents(t *testing.T) {
	t.Run("storage unavailable errors", func(t *testing.T) {
		_, err := (&Handler{}).listAgentMemoryEvents(context.Background(), "agent", 10)
		require.Error(t, err)
	})

	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	t.Run("not found returns empty slice", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policy,
			allErrorOnce:        dynamormerrors.ErrItemNotFound,
		})

		events, err := h.listAgentMemoryEvents(context.Background(), "agent", 10)
		require.NoError(t, err)
		require.Empty(t, events)
	})

	t.Run("other errors propagate", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			agentInstanceConfig: policy,
			allErrorOnce:        errors.New("boom"),
		})

		_, err := h.listAgentMemoryEvents(context.Background(), "agent", 10)
		require.Error(t, err)
	})

	t.Run("success returns events in state", func(t *testing.T) {
		state := &round10QueryState{
			agentInstanceConfig: policy,
			agentMemoryEventsByAgent: map[string][]storagemodels.AgentMemoryEvent{
				"agent": {
					{PK: "AGENT#agent", EventType: storagemodels.MemoryEventCorrection, StatusID: "s1", OriginalID: "o1"},
					{PK: "AGENT#agent", EventType: storagemodels.MemoryEventRetraction, StatusID: "s2", OriginalID: "o2"},
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		events, err := h.listAgentMemoryEvents(context.Background(), "agent", 10)
		require.NoError(t, err)
		require.Len(t, events, 2)
	})
}

func TestAgentMemorySearchRound17_BuildAgentMemorySearchResult(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	now := time.Now().UTC()
	state := &round10QueryState{
		agentInstanceConfig: policy,
		statusByID: map[string]storagemodels.Status{
			"s1": {
				PK:             "status#s1",
				SK:             "status#s1",
				StatusID:       "s1",
				AuthorUsername: "agent",
				AuthorID:       cfg.BaseURL() + "/users/agent",
				Content:        "hello world",
				ConversationID: "c1",
				Hashtags:       []string{"go"},
				PublishedAt:    now.Add(-2 * time.Hour),
				CreatedAt:      now.Add(-2 * time.Hour),
				UpdatedAt:      now.Add(-1 * time.Hour),
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", nil, nil, nil)
	require.NoError(t, err)

	req := apimodels.AgentMemorySearchRequest{Query: ""}

	t.Run("tombstone events are ignored", func(t *testing.T) {
		out, _ := h.buildAgentMemorySearchResult(ctx, "agent", req, storagemodels.AgentMemoryEvent{
			EventType:  storagemodels.MemoryEventTombstone,
			StatusID:   "s1",
			OriginalID: "o1",
		}, "o1", nil, nil, nil)
		require.Nil(t, out)
	})

	t.Run("empty status id is ignored", func(t *testing.T) {
		out, _ := h.buildAgentMemorySearchResult(ctx, "agent", req, storagemodels.AgentMemoryEvent{
			EventType:  storagemodels.MemoryEventCorrection,
			StatusID:   "",
			OriginalID: "o1",
		}, "o1", nil, nil, nil)
		require.Nil(t, out)
	})

	t.Run("query mismatch returns nil", func(t *testing.T) {
		badReq := apimodels.AgentMemorySearchRequest{Query: "missing"}
		out, _ := h.buildAgentMemorySearchResult(ctx, "agent", badReq, storagemodels.AgentMemoryEvent{
			EventType:  storagemodels.MemoryEventCorrection,
			StatusID:   "s1",
			OriginalID: "o1",
		}, "o1", nil, nil, nil)
		require.Nil(t, out)
	})

	t.Run("tag filters are applied", func(t *testing.T) {
		out, _ := h.buildAgentMemorySearchResult(ctx, "agent", req, storagemodels.AgentMemoryEvent{
			EventType:  storagemodels.MemoryEventCorrection,
			StatusID:   "s1",
			OriginalID: "o1",
		}, "o1", nil, nil, []string{"rust"})
		require.Nil(t, out)
	})

	t.Run("since/until are applied", func(t *testing.T) {
		since := now.Add(-30 * time.Minute)
		out, _ := h.buildAgentMemorySearchResult(ctx, "agent", req, storagemodels.AgentMemoryEvent{
			EventType:  storagemodels.MemoryEventCorrection,
			StatusID:   "s1",
			OriginalID: "o1",
		}, "o1", &since, nil, nil)
		require.Nil(t, out)
	})

	t.Run("success builds result and status id", func(t *testing.T) {
		okReq := apimodels.AgentMemorySearchRequest{Query: "", IncludeThreads: true}

		state.statusList = []storagemodels.Status{
			state.statusByID["s1"],
			{PK: "status#s2", SK: "status#s2", StatusID: "s2", AuthorUsername: "other", Deleted: false},
		}

		out, statusID := h.buildAgentMemorySearchResult(ctx, "agent", okReq, storagemodels.AgentMemoryEvent{
			EventType:  storagemodels.MemoryEventCorrection,
			StatusID:   "s1",
			OriginalID: "o1",
		}, "o1", nil, nil, []string{"go"})
		require.NotNil(t, out)
		require.Equal(t, "s1", statusID)
		require.NotNil(t, out.Context)
		require.Equal(t, "o1", out.Context.OriginalID)
	})
}

func TestAgentMemorySearchRound17_SearchAgentMemoryThread(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowAgents = true

	policy := storagemodels.NewAgentInstanceConfig()
	policy.AllowAgents = true

	now := time.Now().UTC()
	state := &round10QueryState{
		agentInstanceConfig: policy,
		statusList: []storagemodels.Status{
			{PK: "status#root", SK: "status#root", StatusID: "root", AuthorUsername: "agent", Content: "root", PublishedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour)},
			{PK: "status#reply", SK: "status#reply", StatusID: "reply", AuthorUsername: "agent", Content: "reply", PublishedAt: now.Add(-1 * time.Hour), CreatedAt: now.Add(-1 * time.Hour)},
			{PK: "status#other", SK: "status#other", StatusID: "other", AuthorUsername: "other", Content: "ignored"},
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)
	h.repos.Account().SetEncryptor(noopEncryptor{})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/memory/search", nil, nil, nil)
	require.NoError(t, err)

	t.Run("empty thread id returns empty result", func(t *testing.T) {
		out, err := h.searchAgentMemoryThread(ctx, "agent", "", 10)
		require.NoError(t, err)
		require.Empty(t, out)
	})

	t.Run("success returns single thread result", func(t *testing.T) {
		out, err := h.searchAgentMemoryThread(ctx, "agent", "root", 10)
		require.NoError(t, err)
		require.Len(t, out, 1)
		require.NotNil(t, out[0].Thread)
	})
}
