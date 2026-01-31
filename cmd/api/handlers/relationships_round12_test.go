package lift

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestRelationshipsRound12(t *testing.T) {
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	t.Run("no account IDs provided returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: &AccountsServiceStub{}, RelationshipsSvc: &RelationshipsServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetRelationshipsLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("relationship service error returns 500", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
				return &storage.Account{User: &storage.User{Username: "bob"}}, nil
			},
		}
		relationshipsSvc := &RelationshipsServiceStub{
			GetRelationshipFunc: func(context.Context, string, string) (*relationships.RelationshipData, error) {
				return nil, errors.New("boom")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc, RelationshipsSvc: relationshipsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, map[string]string{"id[0]": "bob"}, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetRelationshipsLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("success skips invalid and missing accounts", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, accountID string) (*storage.Account, error) {
				if len(accountID) > 500 {
					t.Fatalf("unexpected account lookup for invalid long id")
				}
				if accountID == "missing" {
					return nil, errors.New("not found")
				}
				if accountID != "bob" {
					t.Fatalf("unexpected account lookup for %q", accountID)
				}
				return &storage.Account{User: &storage.User{Username: "bob"}}, nil
			},
		}
		relationshipsSvc := &RelationshipsServiceStub{
			GetRelationshipFunc: func(_ context.Context, requesterID, targetID string) (*relationships.RelationshipData, error) {
				if targetID != "bob" {
					t.Fatalf("unexpected relationship lookup for %q", targetID)
				}
				return &relationships.RelationshipData{ID: targetID, Following: true}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc, RelationshipsSvc: relationshipsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, map[string]string{
			"id[0]": strings.Repeat("a", 501),
			"id[1]": "missing",
			"id[2]": "bob",
		}, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleGetRelationshipsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		resp := ctx.Response.Body.([]apimodels.Relationship)
		require.Len(t, resp, 1)
		require.Equal(t, "bob", resp[0].ID)
		require.True(t, resp[0].Following)
	})
}
