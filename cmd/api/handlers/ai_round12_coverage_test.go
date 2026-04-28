package handlers

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"

	originalai "github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/auth"
	aisvc "github.com/equaltoai/lesser/pkg/services/ai"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAI_Round12_GetAIAnalysis_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {Username: "alice", Role: "user", Approved: true, Version: 1},
			"admin": {Username: "admin", Role: roleAdmin, Approved: true, Version: 1},
			"bob":   {Username: "bob", Role: "user", Approved: true, Version: 1},
		},
		statusByID: map[string]storagemodels.Status{
			"abc":             {StatusID: "abc", AuthorUsername: "alice", AuthorID: "https://example.com/users/alice"},
			"test-object-123": {StatusID: "test-object-123", AuthorUsername: "alice", AuthorID: "https://example.com/users/alice"},
			"missing":         {StatusID: "missing", AuthorUsername: "alice", AuthorID: "https://example.com/users/alice"},
		},
	})
	handler.registry = &RegistryStub{
		AISvc: &AIServiceStub{
			GetAnalysisFunc: func(ctx context.Context, query *aisvc.GetAnalysisQuery) (*aisvc.GetAnalysisResult, error) {
				if query.ObjectID == "missing" {
					return nil, stdErrors.New("not found")
				}
				return &aisvc.GetAnalysisResult{
					Analysis: &originalai.AIAnalysis{
						ObjectID:    query.ObjectID,
						OverallRisk: 0.9,
						TextAnalysis: &originalai.TextAnalysis{
							ContainsPII: true,
							PIIEntities: []originalai.PIIEntity{{
								Type:        originalai.PiiEmail,
								Text:        "alice@example.com",
								Score:       0.99,
								BeginOffset: 0,
								EndOffset:   17,
							}},
						},
					},
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

		resp := requireStatus(t, http.StatusUnauthorized)(handler.HandleGetAIAnalysisLift(ctx))
		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_token", body["error"])
	})

	t.Run("insufficient_scope", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/abc", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["object_id"] = "abc"

		requireStatus(t, http.StatusForbidden)(handler.HandleGetAIAnalysisLift(ctx))
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

		resp := requireStatus(t, http.StatusOK)(handler.HandleGetAIAnalysisLift(ctx))
		var body originalai.AIAnalysis
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Len(t, body.TextAnalysis.PIIEntities, 1)
		require.Empty(t, body.TextAnalysis.PIIEntities[0].Text)
	})

	t.Run("admin sees pii text", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer " + adminToken}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/abc", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["object_id"] = "abc"

		resp := requireStatus(t, http.StatusOK)(handler.HandleGetAIAnalysisLift(ctx))
		var body originalai.AIAnalysis
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Len(t, body.TextAnalysis.PIIEntities, 1)
		require.Equal(t, "alice@example.com", body.TextAnalysis.PIIEntities[0].Text)
	})

	t.Run("non_owner_forbidden", func(t *testing.T) {
		bobToken := round11SignAccessToken(t, cfg.JWTSecret, "bob", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/abc", map[string]string{
			"Authorization": "Bearer " + bobToken,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["object_id"] = "abc"

		requireStatus(t, http.StatusForbidden)(handler.HandleGetAIAnalysisLift(ctx))
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
					return &aisvc.GetAnalysisResult{
						Analysis: &originalai.AIAnalysis{
							ObjectID: "already",
							TextAnalysis: &originalai.TextAnalysis{
								ContainsPII: true,
								PIIEntities: []originalai.PIIEntity{{
									Type:        originalai.PiiEmail,
									Text:        "alice@example.com",
									Score:       0.99,
									BeginOffset: 0,
									EndOffset:   17,
								}},
							},
						},
					}, nil
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

		resp := requireStatus(t, http.StatusForbidden)(handler.HandleRequestAIAnalysisLift(ctx))
		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "insufficient_scope", body["error"])
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

		resp := requireStatus(t, http.StatusOK)(handler.HandleRequestAIAnalysisLift(ctx))
		var body originalai.AIAnalysis
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Len(t, body.TextAnalysis.PIIEntities, 1)
		require.Empty(t, body.TextAnalysis.PIIEntities[0].Text)
	})

	t.Run("already_exists_non_owner_forbidden", func(t *testing.T) {
		modToken := round11SignAccessToken(t, cfg.JWTSecret, "bob", []string{"moderation"})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{"Authorization": "Bearer " + modToken}, nil, map[string]any{"object_id": "already"})
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(handler.HandleRequestAIAnalysisLift(ctx))
	})

	t.Run("ok_queued", func(t *testing.T) {
		modToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"moderation"})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{"Authorization": "Bearer " + modToken}, nil, map[string]any{"object_id": "abc"})
		require.NoError(t, err)

		requireStatus(t, http.StatusAccepted)(handler.HandleRequestAIAnalysisLift(ctx))
	})
}

func TestAI_Round12_RequestAIAnalysisHelpers_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("parse_helper_success", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/ai/analyze", nil, nil, []byte(`{"object_id":"abc","object_type":"status","force":true}`))

		req, resp, err := parseAIAnalysisLiftRequest(ctx)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, "abc", req.ObjectID)
		require.Equal(t, "status", req.ObjectType)
		require.True(t, req.Force)
	})

	t.Run("parse_helper_empty_body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/ai/analyze", nil, nil, nil)

		req, resp, err := parseAIAnalysisLiftRequest(ctx)
		require.Nil(t, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("parse_helper_missing_object_id", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/ai/analyze", nil, nil, []byte(`{"object_id":""}`))

		req, resp, err := parseAIAnalysisLiftRequest(ctx)
		require.Nil(t, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("existing_response_absent_returns_nil", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			AISvc: &AIServiceStub{
				GetAnalysisFunc: func(ctx context.Context, query *aisvc.GetAnalysisQuery) (*aisvc.GetAnalysisResult, error) {
					return nil, nil
				},
			},
		}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/ai/analyze", nil, nil, []byte(`{"object_id":"abc"}`))

		resp, err := handler.existingAIAnalysisResponse(ctx, &auth.Claims{Username: "alice"}, "abc")
		require.NoError(t, err)
		require.Nil(t, resp)
	})
}

func TestAI_Round12_AuthorizationHelpers_Coverage(t *testing.T) {
	ctx := context.Background()
	var nilHandler *Handler

	admin, err := nilHandler.claimsUserHasAdminRole(ctx, nil)
	require.NoError(t, err)
	require.False(t, admin)

	admin, err = (&Handler{}).claimsUserHasAdminRole(ctx, &auth.Claims{Username: "alice"})
	require.NoError(t, err)
	require.False(t, admin)

	owner, err := (&Handler{}).aiAnalysisOwnerUsername(ctx, "status-1")
	require.NoError(t, err)
	require.Empty(t, owner)

	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		statusByID: map[string]storagemodels.Status{
			"actor-id-only": {
				StatusID: "actor-id-only",
				AuthorID: "https://example.com/users/carol",
			},
		},
	})
	owner, err = handler.aiAnalysisOwnerUsername(ctx, "actor-id-only")
	require.NoError(t, err)
	require.Equal(t, "carol", owner)

	liftCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/status-1", nil, nil, nil)
	require.NoError(t, err)
	adminViewer, resp, err := (&Handler{}).authorizeAIAnalysisViewer(liftCtx, &auth.Claims{Username: "alice"}, "status-1")
	require.NoError(t, err)
	require.False(t, adminViewer)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusForbidden, resp.Status)
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
