package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAccountsRound21_RelationshipListsSuppressSuspendedLocalURLFallback(t *testing.T) {
	cfg := round10TestConfig()
	username := "suspended"

	suspendedUser := round21LocalUser(username)
	suspendedUser.Suspended = true

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			username: suspendedUser,
		},
		actorsByUser: map[string]storagemodels.Actor{
			username: round21LocalActor(cfg.BaseURL(), username),
		},
	}

	var followersCalled bool
	var followingCalled bool

	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
				return nil, errors.New("account not found")
			},
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			GetFollowersFunc: func(context.Context, string, int, string) ([]*storage.Account, string, error) {
				followersCalled = true
				return []*storage.Account{}, "", nil
			},
			GetFollowingFunc: func(context.Context, string, int, string) ([]*storage.Account, string, error) {
				followingCalled = true
				return []*storage.Account{}, "", nil
			},
		},
	})

	localActorURL := cfg.BaseURL() + "/users/" + username

	ctxFollowers, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/"+localActorURL+"/followers", nil, nil, nil)
	require.NoError(t, err)
	ctxFollowers.Params["id"] = localActorURL
	requireStatus(t, http.StatusNotFound)(h.HandleGetAccountFollowersLift(ctxFollowers))

	ctxFollowing, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/"+localActorURL+"/following", nil, nil, nil)
	require.NoError(t, err)
	ctxFollowing.Params["id"] = localActorURL
	requireStatus(t, http.StatusNotFound)(h.HandleGetAccountFollowingLift(ctxFollowing))

	require.False(t, followersCalled)
	require.False(t, followingCalled)
}
