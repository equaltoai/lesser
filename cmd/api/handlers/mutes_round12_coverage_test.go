package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	relationshipsvc "github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestMutesRound12_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("mute/unmute validation + service missing", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctxBad, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts//mute", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleMuteAccountLift(ctxBad))

		ctxNoSvc, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/mute", headers, nil, apimodels.MuteRequest{})
		require.NoError(t, err)
		ctxNoSvc.Params["id"] = "bob"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleMuteAccountLift(ctxNoSvc))

		ctxUnmuteNoSvc, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unmute", headers, nil, nil)
		require.NoError(t, err)
		ctxUnmuteNoSvc.Params["id"] = "bob"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleUnmuteAccountLift(ctxUnmuteNoSvc))
	})

	t.Run("mute parse fallback + service error", func(t *testing.T) {
		var sawNotifications bool
		relationshipsSvc := &RelationshipsServiceStub{
			MuteFunc: func(_ context.Context, cmd *relationshipsvc.MuteCommand) (*relationshipsvc.RelationshipResult, error) {
				sawNotifications = cmd.MuteNotifications
				return nil, errors.New("boom")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relationshipsSvc})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/accounts/bob/mute", headers, nil, []byte(`{invalid}`))
		ctx.Params["id"] = "bob"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleMuteAccountLift(ctx))
		require.False(t, sawNotifications)
	})

	t.Run("mute/unmute success + notifications flag", func(t *testing.T) {
		relationshipsSvc := &RelationshipsServiceStub{
			MuteFunc: func(_ context.Context, cmd *relationshipsvc.MuteCommand) (*relationshipsvc.RelationshipResult, error) {
				require.True(t, cmd.MuteNotifications)
				return &relationshipsvc.RelationshipResult{
					Relationship: &relationshipsvc.RelationshipData{ID: cmd.MutedID, Muting: true, MutingNotifications: cmd.MuteNotifications},
				}, nil
			},
			UnmuteFunc: func(_ context.Context, cmd *relationshipsvc.UnmuteCommand) (*relationshipsvc.RelationshipResult, error) {
				return &relationshipsvc.RelationshipResult{
					Relationship: &relationshipsvc.RelationshipData{ID: cmd.MutedID, Muting: false},
				}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relationshipsSvc})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		notifications := true
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/mute", headers, nil, apimodels.MuteRequest{Notifications: &notifications})
		require.NoError(t, err)
		ctx.Params["id"] = "bob"
		requireStatus(t, http.StatusOK)(handler.HandleMuteAccountLift(ctx))

		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unmute", headers, nil, nil)
		require.NoError(t, err)
		ctx2.Params["id"] = "bob"
		requireStatus(t, http.StatusOK)(handler.HandleUnmuteAccountLift(ctx2))
	})

	t.Run("unmute service error", func(t *testing.T) {
		relationshipsSvc := &RelationshipsServiceStub{
			UnmuteFunc: func(_ context.Context, _ *relationshipsvc.UnmuteCommand) (*relationshipsvc.RelationshipResult, error) {
				return nil, errors.New("boom")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relationshipsSvc})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unmute", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "bob"

		requireStatus(t, http.StatusInternalServerError)(handler.HandleUnmuteAccountLift(ctx))
	})

	t.Run("get muted accounts: limit parsing + service error + actor nil", func(t *testing.T) {
		relationshipsSvc := &RelationshipsServiceStub{
			GetMutedUsersFunc: func(_ context.Context, _ *relationshipsvc.GetMutedUsersQuery) (*relationshipsvc.MutedUsersResult, error) {
				return nil, errors.New("boom")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relationshipsSvc})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctxErr, err := round10NewLiftContext(http.MethodGet, "/api/v1/mutes", headers, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetMutedAccountsLift(ctxErr))

		relationshipsOK := &RelationshipsServiceStub{
			GetMutedUsersFunc: func(_ context.Context, _ *relationshipsvc.GetMutedUsersQuery) (*relationshipsvc.MutedUsersResult, error) {
				return &relationshipsvc.MutedUsersResult{
					MutedUsers: []*storage.Account{
						{Actor: nil},
						{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/users/bob"}, PreferredUsername: "bob"}},
					},
					NextCursor: "next",
				}, nil
			},
		}
		handlerOK, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: relationshipsOK})

		ctxOK, err := round10NewLiftContext(http.MethodGet, "/api/v1/mutes", headers, map[string]string{"limit": "1"}, nil)
		require.NoError(t, err)
		respOK := requireStatus(t, http.StatusOK)(handlerOK.HandleGetMutedAccountsLift(ctxOK))
		require.Contains(t, firstStringValue(respOK.Headers, "link"), "max_id=")
	})
}
