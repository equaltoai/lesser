package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	origai "github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/ai"
	relationshipsvc "github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestWebFingerLift_SuccessAndParse(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	registry := &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return &storage.Account{
					Actor: &activitypub.Actor{
						BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/" + username},
						PreferredUsername: username,
						Icon:              &activitypub.Image{URL: "https://example.com/media/avatar.jpg"},
					},
				}, nil
			},
		},
	}

	h := &Handler{cfg: cfg, logger: logger, registry: registry}
	ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/webfinger", nil, map[string]string{
		"resource": "acct:alice@example.com",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleWebFingerLift(ctx))

	var body WebFingerResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "acct:alice@example.com", body.Subject)
	require.NotEmpty(t, body.Links)

	username, domain, parseErr := h.parseWebFingerResourceLift("acct:alice@example.com")
	require.NoError(t, parseErr)
	require.Equal(t, "alice", username)
	require.Equal(t, "example.com", domain)
}

func TestWebFingerLift_MissingResource(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	h := &Handler{cfg: cfg, logger: logger, registry: &RegistryStub{}}
	ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/webfinger", nil, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusBadRequest)(h.HandleWebFingerLift(ctx))
}

func TestMutesLift_Flow(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	headers := map[string]string{"Authorization": "Bearer " + token}

	relationshipsSvc := &RelationshipsServiceStub{
		MuteFunc: func(ctx context.Context, cmd *relationshipsvc.MuteCommand) (*relationshipsvc.RelationshipResult, error) {
			return &relationshipsvc.RelationshipResult{
				Relationship: &relationshipsvc.RelationshipData{
					ID:     cmd.MutedID,
					Muting: true,
				},
			}, nil
		},
		UnmuteFunc: func(ctx context.Context, cmd *relationshipsvc.UnmuteCommand) (*relationshipsvc.RelationshipResult, error) {
			return &relationshipsvc.RelationshipResult{
				Relationship: &relationshipsvc.RelationshipData{
					ID:     cmd.MutedID,
					Muting: false,
				},
			}, nil
		},
		GetMutedUsersFunc: func(ctx context.Context, query *relationshipsvc.GetMutedUsersQuery) (*relationshipsvc.MutedUsersResult, error) {
			return &relationshipsvc.MutedUsersResult{
				MutedUsers: []*storage.Account{
					{Actor: &activitypub.Actor{PreferredUsername: "bob"}},
				},
				NextCursor: "next-cursor",
			}, nil
		},
	}

	h := &Handler{
		cfg:      cfg,
		logger:   logger,
		registry: &RegistryStub{RelationshipsSvc: relationshipsSvc},
	}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/mute", headers, nil, apimodels.MuteRequest{})
	require.NoError(t, err)
	ctx.Params["id"] = "bob"
	requireStatus(t, http.StatusOK)(h.HandleMuteAccountLift(ctx))

	ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unmute", headers, nil, nil)
	require.NoError(t, err)
	ctx2.Params["id"] = "bob"
	requireStatus(t, http.StatusOK)(h.HandleUnmuteAccountLift(ctx2))

	ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/mutes", headers, map[string]string{"limit": "10"}, nil)
	require.NoError(t, err)
	resp3 := requireStatus(t, http.StatusOK)(h.HandleGetMutedAccountsLift(ctx3))
	require.Contains(t, firstStringValue(resp3.Headers, "link"), "next-cursor")
}

func TestEndorsementsLift_Success(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")

	registry := &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountPinsFunc: func(ctx context.Context, query *accounts.GetAccountPinsQuery) (*accounts.AccountPinsResult, error) {
				return &accounts.AccountPinsResult{
					PinnedAccounts: []*storage.Account{
						{Actor: &activitypub.Actor{PreferredUsername: "bob"}},
					},
				}, nil
			},
		},
	}

	h := &Handler{cfg: cfg, logger: logger, registry: registry}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/endorsements", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusOK)(h.HandleGetEndorsementsLift(ctx))
}

func TestAILift_Flows(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)

	analysis := &origai.AIAnalysis{
		ObjectID:   "obj-123",
		ObjectType: "status",
		AnalyzedAt: time.Now(),
	}

	registry := &RegistryStub{
		AISvc: &AIServiceStub{
			GetAnalysisFunc: func(ctx context.Context, query *ai.GetAnalysisQuery) (*ai.GetAnalysisResult, error) {
				return &ai.GetAnalysisResult{Analysis: analysis}, nil
			},
			QueueForAnalysisFunc: func(ctx context.Context, cmd *ai.QueueAnalysisCommand) (*ai.QueueAnalysisResult, error) {
				return &ai.QueueAnalysisResult{Queued: false}, nil
			},
			GetStatsFunc: func(ctx context.Context, query *ai.GetStatsQuery) (*ai.GetStatsResult, error) {
				return &ai.GetStatsResult{Stats: map[string]any{"period": query.Period}}, nil
			},
		},
	}

	h := &Handler{cfg: cfg, logger: logger, registry: registry}

	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/analysis/obj-123", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)
	ctx.Params["object_id"] = "obj-123"
	requireStatus(t, http.StatusOK)(h.HandleGetAIAnalysisLift(ctx))

	moderationToken := round11SignAccessToken(t, cfg.JWTSecret, "mod", []string{"moderation"})
	ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{
		"Authorization": "Bearer " + moderationToken,
	}, nil, map[string]any{"object_id": "obj-123", "object_type": "status"})
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleRequestAIAnalysisLift(ctx2))

	ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/stats", nil, map[string]string{
		"period": "day",
	}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleGetAIStatsLift(ctx3))

	ctx4, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/capabilities", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleGetAISummaryLift(ctx4))
}
