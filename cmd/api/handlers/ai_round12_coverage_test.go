package handlers

import (
	"context"
	stdErrors "errors"
	"net/http"
	"testing"

	originalai "github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/auth"
	aisvc "github.com/equaltoai/lesser/pkg/services/ai"
	"github.com/stretchr/testify/require"
)

func TestAI_Round12_GetAIAnalysis_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AISvc: &AIServiceStub{
			GetAnalysisFunc: func(ctx context.Context, query *aisvc.GetAnalysisQuery) (*aisvc.GetAnalysisResult, error) {
				if query.ObjectID == "missing" {
					return nil, stdErrors.New("not found")
				}
				return &aisvc.GetAnalysisResult{
					Analysis: &originalai.AIAnalysis{ObjectID: query.ObjectID, OverallRisk: 0.9},
				}, nil
			},
		},
	}

	t.Run("missing_token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/abc", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["object_id"] = "abc"

		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetAIAnalysisLift(ctx))
	})

	t.Run("invalid_token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/abc", map[string]string{"Authorization": "Bearer bad"}, nil, nil)
		require.NoError(t, err)
		ctx.Params["object_id"] = "abc"

		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetAIAnalysisLift(ctx))
	})

	t.Run("extracts_object_id_from_path_when_param_missing", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/test-object-123", headers, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleGetAIAnalysisLift(ctx))
	})

	t.Run("not_found", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["object_id"] = "missing"

		requireStatus(t, http.StatusNotFound)(handler.HandleGetAIAnalysisLift(ctx))
	})

	t.Run("ok", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + readToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/abc", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["object_id"] = "abc"

		requireStatus(t, http.StatusOK)(handler.HandleGetAIAnalysisLift(ctx))
	})
}

func TestAI_Round12_RequestAIAnalysis_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AISvc: &AIServiceStub{
			QueueForAnalysisFunc: func(ctx context.Context, cmd *aisvc.QueueAnalysisCommand) (*aisvc.QueueAnalysisResult, error) {
				switch cmd.ObjectID {
				case "queue-error":
					return nil, stdErrors.New("boom")
				case "already":
					return &aisvc.QueueAnalysisResult{Queued: false}, nil
				default:
					return &aisvc.QueueAnalysisResult{Queued: true}, nil
				}
			},
			GetAnalysisFunc: func(ctx context.Context, query *aisvc.GetAnalysisQuery) (*aisvc.GetAnalysisResult, error) {
				if query.ObjectID == "already" {
					return &aisvc.GetAnalysisResult{Analysis: &originalai.AIAnalysis{ObjectID: "already"}}, nil
				}
				return nil, stdErrors.New("not found")
			},
		},
	}

	t.Run("missing_token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", nil, nil, map[string]any{"object_id": "abc"})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleRequestAIAnalysisLift(ctx))
	})

	t.Run("invalid_token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{"Authorization": "Bearer bad"}, nil, map[string]any{"object_id": "abc"})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleRequestAIAnalysisLift(ctx))
	})

	t.Run("missing_moderation_scope", func(t *testing.T) {
		readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{"Authorization": "Bearer " + readToken}, nil, map[string]any{"object_id": "abc"})
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(handler.HandleRequestAIAnalysisLift(ctx))
	})

	t.Run("invalid_body", func(t *testing.T) {
		modToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"moderation"})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/ai/analyze", map[string]string{"Authorization": "Bearer " + modToken}, nil, []byte(`{invalid}`))

		requireStatus(t, http.StatusBadRequest)(handler.HandleRequestAIAnalysisLift(ctx))
	})

	t.Run("parse_fallback_success_for_mismatched_content_type", func(t *testing.T) {
		modToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"moderation"})
		headers := map[string]string{
			"Authorization": "Bearer " + modToken,
			"Content-Type":  "application/x-www-form-urlencoded",
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/ai/analyze", headers, nil, []byte(`{"object_id":"abc","object_type":"status","force":true}`))

		requireStatus(t, http.StatusAccepted)(handler.HandleRequestAIAnalysisLift(ctx))
	})

	t.Run("missing_object_id", func(t *testing.T) {
		modToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"moderation"})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{"Authorization": "Bearer " + modToken}, nil, map[string]any{"object_id": ""})
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleRequestAIAnalysisLift(ctx))
	})

	t.Run("queue_error", func(t *testing.T) {
		modToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"moderation"})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{"Authorization": "Bearer " + modToken}, nil, map[string]any{"object_id": "queue-error"})
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.HandleRequestAIAnalysisLift(ctx))
	})

	t.Run("already_exists_returns_existing_analysis", func(t *testing.T) {
		modToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"moderation"})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{"Authorization": "Bearer " + modToken}, nil, map[string]any{"object_id": "already"})
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleRequestAIAnalysisLift(ctx))
	})

	t.Run("ok_queued", func(t *testing.T) {
		modToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"moderation"})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{"Authorization": "Bearer " + modToken}, nil, map[string]any{"object_id": "abc"})
		require.NoError(t, err)

		requireStatus(t, http.StatusAccepted)(handler.HandleRequestAIAnalysisLift(ctx))
	})
}

func TestAI_Round12_GetAIStatsAndSummary_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AISvc: &AIServiceStub{
			GetStatsFunc: func(ctx context.Context, query *aisvc.GetStatsQuery) (*aisvc.GetStatsResult, error) {
				if query.Period == "boom" {
					return nil, stdErrors.New("boom")
				}
				return &aisvc.GetStatsResult{
					Stats: map[string]any{"period": query.Period},
				}, nil
			},
		},
	}

	t.Run("defaults_period_when_missing", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/stats", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleGetAIStatsLift(ctx))
	})

	t.Run("extracts_period_from_path_querystring", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/stats?period=week", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleGetAIStatsLift(ctx))
	})

	t.Run("service_error", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/stats?period=boom", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetAIStatsLift(ctx))
	})

	t.Run("summary_ok", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/capabilities", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleGetAISummaryLift(ctx))
	})
}
