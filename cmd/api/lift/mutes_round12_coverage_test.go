package lift

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
		require.NoError(t, handler.HandleMuteAccountLift(ctxBad))
		require.Equal(t, http.StatusBadRequest, ctxBad.Response.StatusCode)

		ctxNoSvc, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/mute", headers, nil, apimodels.MuteRequest{})
		require.NoError(t, err)
		ctxNoSvc.SetParam("id", "bob")
		require.NoError(t, handler.HandleMuteAccountLift(ctxNoSvc))
		require.Equal(t, http.StatusInternalServerError, ctxNoSvc.Response.StatusCode)

		ctxUnmuteNoSvc, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unmute", headers, nil, nil)
		require.NoError(t, err)
		ctxUnmuteNoSvc.SetParam("id", "bob")
		require.NoError(t, handler.HandleUnmuteAccountLift(ctxUnmuteNoSvc))
		require.Equal(t, http.StatusInternalServerError, ctxUnmuteNoSvc.Response.StatusCode)
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
		ctx.SetParam("id", "bob")
		require.NoError(t, handler.HandleMuteAccountLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
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
		ctx.SetParam("id", "bob")
		require.NoError(t, handler.HandleMuteAccountLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unmute", headers, nil, nil)
		require.NoError(t, err)
		ctx2.SetParam("id", "bob")
		require.NoError(t, handler.HandleUnmuteAccountLift(ctx2))
		require.Equal(t, http.StatusOK, ctx2.Response.StatusCode)
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
		ctx.SetParam("id", "bob")

		require.NoError(t, handler.HandleUnmuteAccountLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
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
		require.NoError(t, handler.HandleGetMutedAccountsLift(ctxErr))
		require.Equal(t, http.StatusInternalServerError, ctxErr.Response.StatusCode)

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
		require.NoError(t, handlerOK.HandleGetMutedAccountsLift(ctxOK))
		require.Equal(t, http.StatusOK, ctxOK.Response.StatusCode)
		require.Contains(t, ctxOK.Response.Headers["Link"], "max_id=")
	})
}
