package handlers

import (
	"context"
	"encoding/json"
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

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetEndorsementsLift(ctx))
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

		resp := requireStatus(t, http.StatusOK)(h.HandleGetEndorsementsLift(ctx))
		var accounts []apimodels.Account
		require.NoError(t, json.Unmarshal(resp.Body, &accounts))
		require.Len(t, accounts, 1)
		require.Equal(t, "carol", accounts[0].Username)
	})
}
