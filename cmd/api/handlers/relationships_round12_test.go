package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	repomocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
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

		requireStatus(t, http.StatusBadRequest)(h.HandleGetRelationshipsLift(ctx))
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

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetRelationshipsLift(ctx))
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

		resp := requireStatus(t, http.StatusOK)(h.HandleGetRelationshipsLift(ctx))

		var body []apimodels.Relationship
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Len(t, body, 1)
		require.Equal(t, common.GenerateNumericID("bob"), body[0].ID)
		require.True(t, body[0].Following)
	})

	t.Run("numeric account ids resolve to canonical public ids", func(t *testing.T) {
		targetActor := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("bob"), Type: "Person"},
			PreferredUsername: "bob",
		}
		targetAccount := &storage.Account{
			User:  &storage.User{Username: "bob"},
			Actor: targetActor,
		}

		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, accountID string) (*storage.Account, error) {
				if accountID == "3133216004869690" {
					return nil, errors.New("not found")
				}
				if accountID == "bob" {
					return targetAccount, nil
				}
				return nil, errors.New("not found")
			},
		}
		relationshipsSvc := &RelationshipsServiceStub{
			GetRelationshipFunc: func(_ context.Context, requesterID, targetID string) (*relationships.RelationshipData, error) {
				require.Equal(t, "alice", requesterID)
				require.Equal(t, cfg.ActorURL("bob"), targetID)
				return &relationships.RelationshipData{ID: "bob", Following: true}, nil
			},
		}

		h, repos, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc, RelationshipsSvc: relationshipsSvc})
		actorRepo := repomocks.NewMockActorRepository()
		actorRepo.On("GetActorByNumericID", mock.Anything, "3133216004869690").Return(targetActor, nil).Once()
		h.repos = &interactionsRound19Repos{MockRepositoryStorage: repos, actor: actorRepo}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/relationships", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, map[string]string{"id[0]": "3133216004869690"}, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleGetRelationshipsLift(ctx))

		var body []apimodels.Relationship
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Len(t, body, 1)
		require.Equal(t, common.GenerateNumericID("bob"), body[0].ID)
		require.True(t, body[0].Following)
		actorRepo.AssertExpectations(t)
	})
}
