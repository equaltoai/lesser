package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestRelationshipsLiftRound19_ErrorBranches(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("missing token returns unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleGetRelationshipsLift(ctx))
	})

	t.Run("insufficient scope returns forbidden", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", headers, map[string]string{"id": "bob"}, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleGetRelationshipsLift(ctx))
	})

	t.Run("no account IDs returns bad request after auth", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", headers, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleGetRelationshipsLift(ctx))
	})

	t.Run("relationship service error returns 500", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		reg := &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
					return &storage.Account{}, nil
				},
			},
			RelationshipsSvc: &RelationshipsServiceStub{
				GetRelationshipFunc: func(context.Context, string, string) (*relationships.RelationshipData, error) {
					return nil, errors.New("boom")
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, reg)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", headers, map[string]string{"id": "bob"}, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetRelationshipsLift(ctx))
	})
}
