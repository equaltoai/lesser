package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/stretchr/testify/require"
)

func TestRelationshipsFullRound19_ValidationAuthAndServiceErrors(t *testing.T) {
	cfg := round11TestConfig()

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	t.Run("endpoints validate account_id", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: &RelationshipsServiceStub{}})

		ctxUnfollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts//unfollow", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleUnfollowAccountFull(ctxUnfollow))

		ctxBlock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts//block", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleBlockAccountFull(ctxBlock))

		ctxUnblock, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts//unblock", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleUnblockAccountFull(ctxUnblock))

		ctxMute, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts//mute", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleMuteAccountFull(ctxMute))

		ctxUnmute, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts//unmute", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleUnmuteAccountFull(ctxUnmute))
	})

	t.Run("missing token returns unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{RelationshipsSvc: &RelationshipsServiceStub{}})

		ctxFollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/follow", nil, nil, nil)
		require.NoError(t, err)
		ctxFollow.Params["id"] = "bob"
		requireStatus(t, http.StatusUnauthorized)(h.HandleFollowAccountFull(ctxFollow))

		ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships?id=bob", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleGetRelationshipsFull(ctxGet))
	})

	t.Run("service errors return 500", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			RelationshipsSvc: &RelationshipsServiceStub{
				UnfollowFunc: func(context.Context, *relationships.UnfollowCommand) (*relationships.RelationshipResult, error) {
					return nil, errors.New("boom")
				},
				GetRelationshipsFunc: func(context.Context, *relationships.GetRelationshipsQuery) (*relationships.Result, error) {
					return nil, errors.New("boom")
				},
			},
		})

		ctxUnfollow, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/unfollow", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxUnfollow.Params["id"] = "bob"
		requireStatus(t, http.StatusInternalServerError)(h.HandleUnfollowAccountFull(ctxUnfollow))

		ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", readHeaders, map[string]string{"id": "bob"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleGetRelationshipsFull(ctxGet))
	})
}
