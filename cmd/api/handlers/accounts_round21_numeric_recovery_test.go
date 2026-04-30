package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func round21LocalUser(username string) storagemodels.User {
	return storagemodels.User{
		PK:        "USER#" + username,
		SK:        storagemodels.SKMetadata,
		Username:  username,
		Role:      "user",
		Approved:  true,
		Version:   1,
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}
}

func round21LocalActor(baseURL, username string) storagemodels.Actor {
	actor := storagemodels.Actor{
		PK:        "ACTOR#" + username,
		SK:        storagemodels.SKProfile,
		Username:  username,
		NumericID: common.GenerateNumericID(username),
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   baseURL + "/users/" + username,
				Type: activitypub.PersonType,
			},
			PreferredUsername: username,
			Name:              username,
		},
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}
	actor.GSI3PK = "DOMAIN#example.com"
	actor.GSI3SK = username
	return actor
}

func TestAccountsRound21_RelationshipListsRejectMissingNumericIDMapping(t *testing.T) {
	cfg := round10TestConfig()
	username := "Agent-0"
	numericID := common.GenerateNumericID(username)

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			username: round21LocalUser(username),
		},
		actorsByUser: map[string]storagemodels.Actor{
			username: round21LocalActor(cfg.BaseURL(), username),
		},
		notFoundPKs: map[string]bool{
			"NUMERIC_ID#" + numericID: true,
		},
	}

	var followersUsername string
	var followingUsername string

	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		RelationshipsSvc: &RelationshipsServiceStub{
			GetFollowersFunc: func(_ context.Context, resolvedUsername string, limit int, cursor string) ([]*storage.Account, string, error) {
				followersUsername = resolvedUsername
				return []*storage.Account{}, "", nil
			},
			GetFollowingFunc: func(_ context.Context, resolvedUsername string, limit int, cursor string) ([]*storage.Account, string, error) {
				followingUsername = resolvedUsername
				return []*storage.Account{}, "", nil
			},
		},
	})

	ctxFollowers, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/"+numericID+"/followers", nil, nil, nil)
	require.NoError(t, err)
	ctxFollowers.Params["id"] = numericID
	requireStatus(t, http.StatusNotFound)(h.HandleGetAccountFollowersLift(ctxFollowers))

	ctxFollowing, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/"+numericID+"/following", nil, nil, nil)
	require.NoError(t, err)
	ctxFollowing.Params["id"] = numericID
	requireStatus(t, http.StatusNotFound)(h.HandleGetAccountFollowingLift(ctxFollowing))

	require.Empty(t, followersUsername)
	require.Empty(t, followingUsername)
}

func TestInteractionsRound21_FollowRejectsMissingNumericIDMapping(t *testing.T) {
	cfg := round10TestConfig()
	followerUsername := "Agent-0"
	targetUsername := "simulacrum"
	targetNumericID := common.GenerateNumericID(targetUsername)

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			followerUsername: round21LocalUser(followerUsername),
			targetUsername:   round21LocalUser(targetUsername),
		},
		actorsByUser: map[string]storagemodels.Actor{
			followerUsername: round21LocalActor(cfg.BaseURL(), followerUsername),
			targetUsername:   round21LocalActor(cfg.BaseURL(), targetUsername),
		},
		notFoundPKs: map[string]bool{
			"NUMERIC_ID#" + targetNumericID: true,
		},
	}

	var gotFollowerID string
	var gotFollowingID string

	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		RelationshipsSvc: &RelationshipsServiceStub{
			FollowFunc: func(_ context.Context, cmd *relationships.FollowCommand) (*relationships.FollowResult, error) {
				gotFollowerID = cmd.FollowerID
				gotFollowingID = cmd.FollowingID
				return &relationships.FollowResult{
					Relationship: &relationships.RelationshipData{
						ID:        cmd.FollowingID,
						Following: true,
					},
				}, nil
			},
		},
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, followerUsername, []string{"follow"})
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts/"+targetNumericID+"/follow", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = targetNumericID

	requireStatus(t, http.StatusNotFound)(h.HandleFollowLift(ctx))
	require.Empty(t, gotFollowerID)
	require.Empty(t, gotFollowingID)
}

func TestAccountsFullRound21_ResolveAccountIDFull_RejectsMissingNumericIDMapping(t *testing.T) {
	cfg := round10TestConfig()
	username := "simulacrum"
	numericID := common.GenerateNumericID(username)

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			username: round21LocalUser(username),
		},
		actorsByUser: map[string]storagemodels.Actor{
			username: round21LocalActor(cfg.BaseURL(), username),
		},
		notFoundPKs: map[string]bool{
			"NUMERIC_ID#" + numericID: true,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	actor, err := h.resolveAccountIDFull(context.Background(), numericID)
	require.Error(t, err)
	require.Nil(t, actor)
	require.Contains(t, strings.ToLower(err.Error()), "not found")
}

func TestAccountsRound21_VerifyCredentialsEnsuresOwnNumericIDStillWorks(t *testing.T) {
	cfg := round10TestConfig()
	username := "Agent-0"

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			username: round21LocalUser(username),
		},
		actorsByUser: map[string]storagemodels.Actor{
			username: round21LocalActor(cfg.BaseURL(), username),
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, gotUsername string) (*storage.Account, error) {
				require.Equal(t, username, gotUsername)
				return &storage.Account{
					User:  &storage.User{Username: username},
					Actor: round21LocalActor(cfg.BaseURL(), username).Actor,
				}, nil
			},
		},
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, username, []string{auth.ScopeRead})
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/verify_credentials", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusOK)(h.HandleVerifyCredentialsLift(ctx))
}

func TestAccountsFullRound21_ResolveAccountIDFull_RejectsLegacyMixedCaseNumericIDRowsWithoutMapping(t *testing.T) {
	cfg := round10TestConfig()
	username := "agent-0"
	requestedNumericID := common.GenerateNumericID(username)

	legacyActor := round21LocalActor(cfg.BaseURL(), "Agent-0")
	legacyActor.PK = "ACTOR#" + username
	legacyActor.Username = username
	legacyActor.GSI3SK = username

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			username: round21LocalUser(username),
		},
		actorsByUser: map[string]storagemodels.Actor{
			username: legacyActor,
		},
		notFoundPKs: map[string]bool{
			"NUMERIC_ID#" + requestedNumericID: true,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	actor, err := h.resolveAccountIDFull(context.Background(), requestedNumericID)
	require.Error(t, err)
	require.Nil(t, actor)
	require.Contains(t, strings.ToLower(err.Error()), "not found")
}
