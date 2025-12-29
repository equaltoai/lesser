package lift

import (
	"context"
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

	require.NoError(t, h.HandleWebFingerLift(ctx))
	require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

	resp, ok := ctx.Response.Body.(WebFingerResponse)
	require.True(t, ok)
	require.Equal(t, "acct:alice@example.com", resp.Subject)
	require.NotEmpty(t, resp.Links)

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

	require.NoError(t, h.HandleWebFingerLift(ctx))
	require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
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
	ctx.SetParam("id", "bob")
	require.NoError(t, h.HandleMuteAccountLift(ctx))
	require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

	ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unmute", headers, nil, nil)
	require.NoError(t, err)
	ctx2.SetParam("id", "bob")
	require.NoError(t, h.HandleUnmuteAccountLift(ctx2))
	require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)

	ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/mutes", headers, map[string]string{"limit": "10"}, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleGetMutedAccountsLift(ctx3))
	require.Equal(t, http.StatusOK, ctx3.Response.StatusCode)
	require.Contains(t, ctx3.Response.Headers["Link"], "next-cursor")
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

	require.NoError(t, h.HandleGetEndorsementsLift(ctx))
	require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
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
	ctx.SetParam("object_id", "obj-123")
	require.NoError(t, h.HandleGetAIAnalysisLift(ctx))
	require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

	moderationToken := round11SignAccessToken(t, cfg.JWTSecret, "mod", []string{"moderation"})
	ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/ai/analyze", map[string]string{
		"Authorization": "Bearer " + moderationToken,
	}, nil, map[string]any{"object_id": "obj-123", "object_type": "status"})
	require.NoError(t, err)
	require.NoError(t, h.HandleRequestAIAnalysisLift(ctx2))
	require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)

	ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/stats", nil, map[string]string{
		"period": "day",
	}, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleGetAIStatsLift(ctx3))
	require.Equal(t, http.StatusOK, ctx3.Response.StatusCode)

	ctx4, err := round10NewLiftContext(http.MethodGet, "/api/v1/ai/capabilities", nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, h.HandleGetAISummaryLift(ctx4))
	require.Equal(t, http.StatusOK, ctx4.Response.StatusCode)
}
