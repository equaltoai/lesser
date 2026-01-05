package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestEndorsementsRound12(t *testing.T) {
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	t.Run("service error returns 500", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountPinsFunc: func(context.Context, *accounts.GetAccountPinsQuery) (*accounts.AccountPinsResult, error) {
				return nil, errors.New("boom")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/endorsements", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetEndorsementsLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("success skips nil actors and returns accounts", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountPinsFunc: func(context.Context, *accounts.GetAccountPinsQuery) (*accounts.AccountPinsResult, error) {
				return &accounts.AccountPinsResult{
					PinnedAccounts: []*storage.Account{
						{User: &storage.User{Username: "bob"}},
						{
							User: &storage.User{Username: "carol"},
							Actor: &activitypub.Actor{
								BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/carol", Type: "Person"},
								PreferredUsername: "carol",
							},
						},
					},
				}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/endorsements", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetEndorsementsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		resp := ctx.Response.Body.([]apimodels.Account)
		require.Len(t, resp, 1)
		require.Equal(t, "carol", resp[0].Username)
	})
}
