package lift

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestFollowRequests_Round12_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})

	t.Run("get_follow_requests_unauthorized_missing_header", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{}
		relationshipsSvc := &RelationshipsServiceStub{}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      accountsSvc,
			RelationshipsSvc: relationshipsSvc,
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/follow_requests", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetFollowRequestsLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("get_follow_requests_invalid_token_returns_401", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      &AccountsServiceStub{},
			RelationshipsSvc: &RelationshipsServiceStub{},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/follow_requests", map[string]string{"Authorization": "Bearer not-a-real-jwt"}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetFollowRequestsLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("get_follow_requests_insufficient_scope_returns_403", func(t *testing.T) {
		writeOnlyToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      &AccountsServiceStub{},
			RelationshipsSvc: &RelationshipsServiceStub{},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/follow_requests", map[string]string{"Authorization": "Bearer " + writeOnlyToken}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetFollowRequestsLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("get_follow_requests_get_actor_error_returns_500", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
				return nil, errors.New("boom")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      accountsSvc,
			RelationshipsSvc: &RelationshipsServiceStub{},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/follow_requests", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetFollowRequestsLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get_follow_requests_logic_relationships_error_returns_500", func(t *testing.T) {
		relationshipsSvc := &RelationshipsServiceStub{
			GetPendingFollowRequestsFunc: func(_ context.Context, _ *relationships.GetFollowRequestsQuery) (*relationships.FollowRequestsResult, error) {
				return nil, errors.New("boom")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      &AccountsServiceStub{},
			RelationshipsSvc: relationshipsSvc,
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/follow_requests", nil, nil, nil)
		require.NoError(t, err)

		lockedActor := &activitypub.Actor{PreferredUsername: "alice", ManuallyApprovesFollowers: true}
		require.NoError(t, handler.handleGetFollowRequestsLogic(ctx, lockedActor, "alice"))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get_follow_requests_logic_loops_followers_and_skips_errors", func(t *testing.T) {
		relationshipsSvc := &RelationshipsServiceStub{
			GetPendingFollowRequestsFunc: func(_ context.Context, _ *relationships.GetFollowRequestsQuery) (*relationships.FollowRequestsResult, error) {
				return &relationships.FollowRequestsResult{FollowerIDs: []string{"bob", "carol"}}, nil
			},
		}

		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				if username == "carol" {
					return nil, errors.New("missing")
				}
				return &storage.Account{
					Actor: &activitypub.Actor{
						BaseObject:                activitypub.BaseObject{ID: cfg.ActorURL(username), Type: "Person"},
						PreferredUsername:         username,
						ManuallyApprovesFollowers: true,
						Inbox:                     cfg.BaseURL() + "/inbox/" + username,
						URL:                       cfg.ActorURL(username),
					},
				}, nil
			},
		}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      accountsSvc,
			RelationshipsSvc: relationshipsSvc,
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/follow_requests", nil, nil, nil)
		require.NoError(t, err)

		lockedActor := &activitypub.Actor{PreferredUsername: "alice", ManuallyApprovesFollowers: true}
		require.NoError(t, handler.handleGetFollowRequestsLogic(ctx, lockedActor, "alice"))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("authorize_missing_account_id_param_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      &AccountsServiceStub{},
			RelationshipsSvc: &RelationshipsServiceStub{},
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/follow_requests/bob/authorize", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleAuthorizeFollowRequestLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("authorize_unlocked_account_returns_400", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{Actor: &activitypub.Actor{PreferredUsername: username, ManuallyApprovesFollowers: false}}, nil
			},
		}
		relationshipsSvc := &RelationshipsServiceStub{
			AcceptFollowRequestFunc: func(_ context.Context, _ *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error) {
				return &relationships.RelationshipResult{Relationship: &relationships.RelationshipData{}}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      accountsSvc,
			RelationshipsSvc: relationshipsSvc,
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/follow_requests/bob/authorize", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("account_id", "bob")
		require.NoError(t, handler.HandleAuthorizeFollowRequestLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("authorize_follow_request_not_found_returns_404", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{
					Actor: &activitypub.Actor{
						BaseObject:                activitypub.BaseObject{ID: cfg.ActorURL(username), Type: "Person"},
						PreferredUsername:         username,
						ManuallyApprovesFollowers: true,
					},
				}, nil
			},
		}
		relationshipsSvc := &RelationshipsServiceStub{
			AcceptFollowRequestFunc: func(_ context.Context, _ *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error) {
				return nil, errors.New(common.ErrorFollowRequestNotFound)
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      accountsSvc,
			RelationshipsSvc: relationshipsSvc,
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/follow_requests/bob/authorize", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("account_id", "bob")
		require.NoError(t, handler.HandleAuthorizeFollowRequestLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("authorize_service_error_returns_500", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{
					Actor: &activitypub.Actor{
						BaseObject:                activitypub.BaseObject{ID: cfg.ActorURL(username), Type: "Person"},
						PreferredUsername:         username,
						ManuallyApprovesFollowers: true,
					},
				}, nil
			},
		}
		relationshipsSvc := &RelationshipsServiceStub{
			AcceptFollowRequestFunc: func(_ context.Context, _ *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error) {
				return nil, errors.New("boom")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      accountsSvc,
			RelationshipsSvc: relationshipsSvc,
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/follow_requests/bob/authorize", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("account_id", "bob")
		require.NoError(t, handler.HandleAuthorizeFollowRequestLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("reject_success_covers_reject_flow", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				return &storage.Account{
					Actor: &activitypub.Actor{
						BaseObject:                activitypub.BaseObject{ID: cfg.ActorURL(username), Type: "Person"},
						PreferredUsername:         username,
						ManuallyApprovesFollowers: true,
						Inbox:                     cfg.BaseURL() + "/inbox/" + username,
						URL:                       cfg.ActorURL(username),
					},
				}, nil
			},
		}
		relationshipsSvc := &RelationshipsServiceStub{
			RejectFollowRequestFunc: func(_ context.Context, cmd *relationships.RejectFollowRequestCommand) (*relationships.RelationshipResult, error) {
				return &relationships.RelationshipResult{
					Relationship: &relationships.RelationshipData{
						ID:         cmd.FollowerID,
						Following:  false,
						FollowedBy: false,
					},
				}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc:      accountsSvc,
			RelationshipsSvc: relationshipsSvc,
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/follow_requests/bob/reject", map[string]string{"Authorization": "Bearer " + writeToken}, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("account_id", "bob")
		require.NoError(t, handler.HandleRejectFollowRequestLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("handleFollowRequestOperation_activity_sender_error_branch", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/follow_requests/bob/authorize", nil, nil, nil)
		require.NoError(t, err)

		actor := &activitypub.Actor{PreferredUsername: "alice", ManuallyApprovesFollowers: true}
		rel := &relationships.RelationshipData{ID: "bob"}

		var wg sync.WaitGroup
		wg.Add(1)

		require.NoError(t, handler.handleFollowRequestOperation(ctx, actor, "alice", "bob", followRequestConfig{
			actionType: "accept",
			serviceMethod: func(_ context.Context, _ *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error) {
				return &relationships.RelationshipResult{Relationship: rel}, nil
			},
			activitySender: func(_ context.Context, _, _ string) error {
				defer wg.Done()
				return errors.New("boom")
			},
			logMessage:       "authorized",
			errorLogMessage:  "accept failed",
			activityLogError: "send failed",
		}))

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(250 * time.Millisecond):
			t.Fatal("timed out waiting for activitySender")
		}
	})

	t.Run("sendRejectActivityLift_propagates_get_account_error", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				if username == "bob" {
					return nil, errors.New("no follower")
				}
				return &storage.Account{Actor: &activitypub.Actor{PreferredUsername: username}}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})

		err := handler.sendRejectActivityLift(context.Background(), "bob", "alice")
		require.Error(t, err)
	})

	t.Run("sendFollowResponseActivity_errors_when_followed_missing", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				if username == "alice" {
					return nil, errors.New("no followed")
				}
				return &storage.Account{
					Actor: &activitypub.Actor{
						PreferredUsername: username,
						URL:               cfg.ActorURL(username),
						Inbox:             cfg.BaseURL() + "/inbox/" + username,
					},
				}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})

		err := handler.sendFollowResponseActivity(context.Background(), "bob", "alice", "Accept")
		require.Error(t, err)
	})
}

