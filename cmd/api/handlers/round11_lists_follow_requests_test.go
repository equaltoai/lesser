package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/services/relationships"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestListsLift_CRUDAndMembership(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	headers := map[string]string{"Authorization": "Bearer " + token}
	jsonHeaders := map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"}

	list := &storagemodels.List{ID: "list-1", Title: "Test", RepliesPolicy: "list"}
	listsSvc := &ListsServiceStub{
		ListUserListsFunc: func(ctx context.Context, query *lists.ListUserListsQuery) (*lists.Result, error) {
			return &lists.Result{Lists: []*storagemodels.List{list}}, nil
		},
		CreateListFunc: func(ctx context.Context, cmd *lists.CreateListCommand) (*lists.ListResult, error) {
			return &lists.ListResult{List: list}, nil
		},
		GetListFunc: func(ctx context.Context, query *lists.GetListQuery) (*storagemodels.List, error) {
			return list, nil
		},
		UpdateListFunc: func(ctx context.Context, cmd *lists.UpdateListCommand) (*lists.ListResult, error) {
			return &lists.ListResult{List: list}, nil
		},
		DeleteListFunc: func(ctx context.Context, cmd *lists.DeleteListCommand) error {
			return nil
		},
		GetListMembersFunc: func(ctx context.Context, query *lists.GetListMembersQuery) (*lists.MembersResult, error) {
			return &lists.MembersResult{
				Members: []*storage.Account{
					{Actor: &activitypub.Actor{PreferredUsername: "bob"}},
				},
			}, nil
		},
		AddToListFunc: func(ctx context.Context, cmd *lists.AddToListCommand) (*lists.MembershipResult, error) {
			return &lists.MembershipResult{}, nil
		},
		RemoveFromListFunc: func(ctx context.Context, cmd *lists.RemoveFromListCommand) (*lists.MembershipResult, error) {
			return &lists.MembershipResult{}, nil
		},
	}

	h := &Handler{
		cfg:      cfg,
		logger:   logger,
		registry: &RegistryStub{ListsSvc: listsSvc},
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists", headers, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleGetListsLift(ctx))

	ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/lists", jsonHeaders, nil, models.CreateListRequest{Title: "Test", RepliesPolicy: "list"})
	require.NoError(t, err)
	requireStatus(t, http.StatusCreated)(h.HandleCreateListLift(ctx2))

	ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists/list-1", headers, nil, nil)
	require.NoError(t, err)
	ctx3.Params["id"] = "list-1"
	requireStatus(t, http.StatusOK)(h.HandleGetListLift(ctx3))

	ctx4, err := round10NewLiftContext(http.MethodPut, "/api/v1/lists/list-1", jsonHeaders, nil, models.UpdateListRequest{Title: "Updated"})
	require.NoError(t, err)
	ctx4.Params["id"] = "list-1"
	requireStatus(t, http.StatusOK)(h.HandleUpdateListLift(ctx4))

	ctx5, err := round10NewLiftContext(http.MethodDelete, "/api/v1/lists/list-1", headers, nil, nil)
	require.NoError(t, err)
	ctx5.Params["id"] = "list-1"
	requireStatus(t, http.StatusOK)(h.HandleDeleteListLift(ctx5))

	ctx6, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists/list-1/accounts", headers, nil, nil)
	require.NoError(t, err)
	ctx6.Params["id"] = "list-1"
	requireStatus(t, http.StatusOK)(h.HandleGetListAccountsLift(ctx6))

	ctx7, err := round10NewLiftContext(http.MethodPost, "/api/v1/lists/list-1/accounts", jsonHeaders, nil, models.AddAccountsRequest{AccountIDs: []string{"bob"}})
	require.NoError(t, err)
	ctx7.Params["id"] = "list-1"
	requireStatus(t, http.StatusOK)(h.HandleAddAccountsToListLift(ctx7))

	ctx8, err := round10NewLiftContext(http.MethodDelete, "/api/v1/lists/list-1/accounts", jsonHeaders, nil, models.RemoveAccountsRequest{AccountIDs: []string{"bob"}})
	require.NoError(t, err)
	ctx8.Params["id"] = "list-1"
	requireStatus(t, http.StatusOK)(h.HandleRemoveAccountsFromListLift(ctx8))
}

func TestFollowRequestsLift(t *testing.T) {
	cfg := round10TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	headers := map[string]string{"Authorization": "Bearer " + token}

	accountsSvc := &AccountsServiceStub{
		GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
			return &storage.Account{
				Actor: &activitypub.Actor{
					BaseObject:                activitypub.BaseObject{ID: "https://example.com/users/" + username},
					PreferredUsername:         username,
					ManuallyApprovesFollowers: true,
					Inbox:                     "https://example.com/inbox/" + username,
					URL:                       "https://example.com/users/" + username,
				},
			}, nil
		},
	}

	relationshipsSvc := &RelationshipsServiceStub{
		GetPendingFollowRequestsFunc: func(ctx context.Context, query *relationships.GetFollowRequestsQuery) (*relationships.FollowRequestsResult, error) {
			return &relationships.FollowRequestsResult{FollowerIDs: nil}, nil
		},
		AcceptFollowRequestFunc: func(ctx context.Context, cmd *relationships.AcceptFollowRequestCommand) (*relationships.RelationshipResult, error) {
			return &relationships.RelationshipResult{
				Relationship: &relationships.RelationshipData{
					ID:         cmd.FollowerID,
					Following:  true,
					FollowedBy: true,
				},
			}, nil
		},
	}

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	h.registry = &RegistryStub{AccountsSvc: accountsSvc, RelationshipsSvc: relationshipsSvc}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/follow_requests", headers, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleGetFollowRequestsLift(ctx))

	ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/follow_requests/bob/authorize", headers, nil, nil)
	require.NoError(t, err)
	ctx2.Params["account_id"] = "bob"
	requireStatus(t, http.StatusOK)(h.HandleAuthorizeFollowRequestLift(ctx2))

	unlockedActor := &activitypub.Actor{PreferredUsername: "alice", ManuallyApprovesFollowers: false}
	ctx3, err := round10NewLiftContext(http.MethodGet, "/api/v1/follow_requests", headers, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.handleGetFollowRequestsLogic(ctx3, unlockedActor, "alice"))
}
