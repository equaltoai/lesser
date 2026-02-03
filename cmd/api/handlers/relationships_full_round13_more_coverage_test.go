package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/stretchr/testify/require"
)

func TestRelationshipsFullRound13_ServiceBackedHandlers(t *testing.T) {
	cfg := round10TestConfig()

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite, auth.ScopeRead})
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	now := time.Now()
	rel := &relationships.RelationshipData{ID: "rel1", CreatedAt: now, UpdatedAt: now}

	relSvc := &RelationshipsServiceStub{
		FollowFunc: func(ctx context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
			return &relationships.FollowResult{Relationship: rel}, nil
		},
		UnfollowFunc: func(ctx context.Context, cmd *relationships.UnfollowCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		BlockFunc: func(ctx context.Context, cmd *relationships.BlockCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		UnblockFunc: func(ctx context.Context, cmd *relationships.UnblockCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		MuteFunc: func(ctx context.Context, cmd *relationships.MuteCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		UnmuteFunc: func(ctx context.Context, cmd *relationships.UnmuteCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{Relationship: rel}, nil
		},
		GetRelationshipsFunc: func(ctx context.Context, query *relationships.GetRelationshipsQuery) (*relationships.Result, error) {
			return &relationships.Result{Relationships: []*relationships.RelationshipData{rel}}, nil
		},
	}

	reg := &RegistryStub{RelationshipsSvc: relSvc}
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, reg)

	t.Run("follow requires account_id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts//follow", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleFollowAccountFull(ctx))
	})

	t.Run("follow insufficient scope", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "bob"

		requireStatus(t, http.StatusForbidden)(h.HandleFollowAccountFull(ctx))
	})

	t.Run("follow success and honors request flags", func(t *testing.T) {
		reblogs := false
		notify := true
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.FollowRequest{
			Reblogs: &reblogs,
			Notify:  &notify,
		})
		require.NoError(t, err)
		ctx.Params["id"] = "bob"

		requireStatus(t, http.StatusOK)(h.HandleFollowAccountFull(ctx))
	})

	t.Run("follow service error returns 500", func(t *testing.T) {
		broken := &RegistryStub{RelationshipsSvc: &RelationshipsServiceStub{
			FollowFunc: func(ctx context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
				return nil, errors.New("boom")
			},
		}}
		h2, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, broken)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "bob"

		requireStatus(t, http.StatusInternalServerError)(h2.HandleFollowAccountFull(ctx))
	})

	t.Run("unfollow/block/unblock/mute/unmute success", func(t *testing.T) {
		ctxUnfollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unfollow", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		ctxUnfollow.Params["id"] = "bob"
		requireStatus(t, http.StatusOK)(h.HandleUnfollowAccountFull(ctxUnfollow))

		ctxBlock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/block", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		ctxBlock.Params["id"] = "bob"
		requireStatus(t, http.StatusOK)(h.HandleBlockAccountFull(ctxBlock))

		ctxUnblock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unblock", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		ctxUnblock.Params["id"] = "bob"
		requireStatus(t, http.StatusOK)(h.HandleUnblockAccountFull(ctxUnblock))

		notifications := true
		ctxMute, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/mute", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.MuteRequest{
			Notifications: &notifications,
		})
		require.NoError(t, err)
		ctxMute.Params["id"] = "bob"
		requireStatus(t, http.StatusOK)(h.HandleMuteAccountFull(ctxMute))

		ctxUnmute, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unmute", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		ctxUnmute.Params["id"] = "bob"
		requireStatus(t, http.StatusOK)(h.HandleUnmuteAccountFull(ctxUnmute))
	})

	t.Run("get relationships validates query", func(t *testing.T) {
		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleGetRelationshipsFull(ctxMissing))

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships?id=bob,charlie", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleGetRelationshipsFull(ctx))
	})
}

